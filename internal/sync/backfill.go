package sync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"gitlens/ent"
	"gitlens/ent/commitactivity"
	"gitlens/ent/repository"
	"gitlens/internal/github"
)

const (
	backfillPerPage    = 100
	backfillPageDelay  = 150 * time.Millisecond
	backfillStaleAfter = 15 * time.Minute
)

// backfillHorizon is the earliest date the backfill covers; matches the
// earliest year selectable in the Year Overview UI.
var backfillHorizon = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// setCommitActivity writes ABSOLUTE day counts for the given repo:
// existing rows are overwritten, new rows created. Used by the backfill
// walker, whose fetched windows are authoritative. Caller must hold the
// repo lock.
func (s *Syncer) setCommitActivity(ctx context.Context, repoID int, dayCounts map[string]int) {
	for dayStr, count := range dayCounts {
		day, err := time.Parse("2006-01-02", dayStr)
		if err != nil {
			continue
		}
		existing, err := s.client.CommitActivity.Query().
			Where(commitactivity.RepoID(repoID), commitactivity.Date(day)).
			Only(ctx)
		if err != nil {
			if !ent.IsNotFound(err) {
				log.Printf("backfill: error querying activity repo %d day %s: %v", repoID, dayStr, err)
				continue
			}
			_, err = s.client.CommitActivity.Create().
				SetRepoID(repoID).SetDate(day).SetCommitCount(count).
				Save(ctx)
			if err != nil {
				log.Printf("backfill: error creating activity repo %d day %s: %v", repoID, dayStr, err)
			}
			continue
		}
		if existing.CommitCount != count {
			if _, err = existing.Update().SetCommitCount(count).Save(ctx); err != nil {
				log.Printf("backfill: error updating activity repo %d day %s: %v", repoID, dayStr, err)
			}
		}
	}
}

// tryStartBackfill atomically transitions a repo's backfill status to
// "running". Returns false if another job is already running (and not
// stale) — this is the concurrency guard against duplicate jobs.
func (s *Syncer) tryStartBackfill(ctx context.Context, repoID int) bool {
	staleBefore := time.Now().Add(-backfillStaleAfter)
	n, err := s.client.Repository.Update().
		Where(
			repository.ID(repoID),
			repository.Or(
				repository.BackfillStatusEQ("pending"),
				repository.And(
					repository.BackfillStatusEQ("running"),
					repository.BackfillUpdatedAtLT(staleBefore),
				),
			),
		).
		SetBackfillStatus("running").
		SetBackfillError("").
		SetBackfillUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		log.Printf("backfill: failed to claim repo %d: %v", repoID, err)
		return false
	}
	return n > 0
}

// MaybeStartBackfill kicks off a background backfill for the repo if it
// is pending (or a previous run went stale). Safe to call repeatedly;
// the atomic status transition dedupes concurrent callers.
func (s *Syncer) MaybeStartBackfill(repoID int) {
	ctx := context.Background()
	if !s.tryStartBackfill(ctx, repoID) {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("backfill: panic for repo %d: %v", repoID, r)
			}
		}()
		s.BackfillRepo(ctx, repoID)
	}()
}

// finishBackfill persists the terminal backfill state for a repo.
func (s *Syncer) finishBackfill(ctx context.Context, repoID int, status string, cursorPage int, errMsg string) {
	_, err := s.client.Repository.UpdateOneID(repoID).
		SetBackfillStatus(status).
		SetBackfillCursorPage(cursorPage).
		SetBackfillError(errMsg).
		SetBackfillUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		log.Printf("backfill: failed to persist state for repo %d: %v", repoID, err)
	}
}

// BackfillRepo walks the repo's commit history backwards (newest page
// first), writing absolute per-day counts into CommitActivity until it
// reaches the horizon year or exhausts history. It persists its cursor
// so rate-limit pauses and crashes are resumable. Assumes status has
// already been transitioned to "running" by the caller.
func (s *Syncer) BackfillRepo(ctx context.Context, repoID int) {
	lock := s.repoLock(repoID)
	lock.Lock()
	defer lock.Unlock()

	repo, err := s.client.Repository.Get(ctx, repoID)
	if err != nil {
		log.Printf("backfill: repo %d not found: %v", repoID, err)
		return
	}
	u, err := repo.QueryUser().Only(ctx)
	if err != nil {
		s.finishBackfill(ctx, repoID, "error", repo.BackfillCursorPage, fmt.Sprintf("user lookup: %v", err))
		return
	}
	p, token := s.getProvider(u, repo)

	page := repo.BackfillCursorPage
	if page < 1 {
		page = 1
	}

	pending := make(map[string]int)      // day -> count, not yet flushed
	dayFirstPage := make(map[string]int) // day -> page where it first appeared

	// flush writes all pending days strictly older than cutoffDay —
	// those are complete because later pages only contain older
	// commits. Empty cutoff flushes everything.
	flush := func(cutoffDay string) {
		for day, count := range pending {
			if cutoffDay == "" || day < cutoffDay {
				s.setCommitActivity(ctx, repoID, map[string]int{day: count})
				delete(pending, day)
			}
		}
	}
	// resumePage returns the page to restart from if the run is
	// interrupted: the page where the oldest unflushed day first
	// appeared (that day's data spans pages and must be refetched).
	resumePage := func(fallback int) int {
		oldest := ""
		for day := range pending {
			if oldest == "" || day < oldest {
				oldest = day
			}
		}
		if oldest == "" {
			return fallback
		}
		return dayFirstPage[oldest]
	}

	for {
		commits, hasMore, err := p.ListCommitsPage(ctx, token, repo.Owner, repo.Name, repo.DefaultBranch, page, backfillPerPage)
		if err != nil {
			var rlErr *github.RateLimitError
			if errors.As(err, &rlErr) {
				// Rate limited: pause and resume later from the
				// oldest unflushed day's first page.
				s.finishBackfill(ctx, repoID, "pending", resumePage(page), rlErr.Error())
				return
			}
			s.finishBackfill(ctx, repoID, "error", resumePage(page), err.Error())
			return
		}
		if len(commits) == 0 {
			break // history exhausted
		}

		reachedHorizon := false
		for _, c := range commits {
			d := c.Date.UTC()
			if d.Before(backfillHorizon) {
				reachedHorizon = true
				continue
			}
			day := d.Format("2006-01-02")
			if _, ok := pending[day]; !ok {
				dayFirstPage[day] = page
			}
			pending[day]++
		}

		oldestInPage := commits[len(commits)-1].Date.UTC()
		// Days strictly older than this page's oldest day are complete.
		flush(oldestInPage.Format("2006-01-02"))

		// Persist progress after each page.
		if _, err := s.client.Repository.UpdateOneID(repoID).
			SetBackfillOldestDate(oldestInPage).
			SetBackfillCursorPage(page).
			SetBackfillUpdatedAt(time.Now()).
			Save(ctx); err != nil {
			log.Printf("backfill: failed to persist progress for repo %d: %v", repoID, err)
		}

		page++
		if reachedHorizon || !hasMore {
			break
		}
		time.Sleep(backfillPageDelay)
	}

	// Final flush: all remaining pending days are complete.
	flush("")
	s.finishBackfill(ctx, repoID, "done", 0, "")
}
