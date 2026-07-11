package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"time"

	"gitlens/ent"
	"gitlens/ent/event"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	"gitlens/internal/github"
	"gitlens/internal/otel"
	"gitlens/internal/provider"
	"gitlens/internal/ws"
)

type Syncer struct {
	client    *ent.Client
	gh        *github.Client
	providers map[string]provider.Provider
	hub       *ws.Hub
	tmpl      *template.Template
}

func NewSyncer(client *ent.Client, gh *github.Client, providers map[string]provider.Provider, hub *ws.Hub) *Syncer {
	return &Syncer{
		client:    client,
		gh:        gh,
		providers: providers,
		hub:       hub,
	}
}

func (s *Syncer) SetTemplate(tmpl *template.Template) {
	s.tmpl = tmpl
}

func (s *Syncer) StartPeriodicSync(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.SyncAll(ctx)
		}
	}
}

func (s *Syncer) SyncAll(ctx context.Context) {
	users, err := s.client.User.Query().All(ctx)
	if err != nil {
		return
	}
	for _, u := range users {
		if !u.SyncedAt.IsZero() {
			elapsed := time.Since(u.SyncedAt)
			if elapsed < time.Duration(u.SyncIntervalMinutes)*time.Minute {
				continue
			}
		}
		s.syncUserRepos(ctx, u)
	}
}

func (s *Syncer) syncUserRepos(ctx context.Context, u *ent.User) {
	repos, err := s.client.Repository.Query().Where(repository.HasUserWith(user.ID(u.ID))).All(ctx)
	if err != nil {
		return
	}
	for _, r := range repos {
		s.SyncOne(ctx, r)
	}
	_, _ = s.client.User.UpdateOne(u).SetSyncedAt(time.Now()).Save(ctx)
}

// getProvider returns the appropriate Provider for the repo, plus the
// user's access token for that provider.
func (s *Syncer) getProvider(u *ent.User, repo *ent.Repository) (provider.Provider, string) {
	providerName := repo.Provider
	if providerName == "" {
		providerName = "github"
	}
	p, ok := s.providers[providerName]
	if !ok || p == nil {
		// Fall back to GitHub for back-compat
		p = s.providers["github"]
		providerName = "github"
	}
	var token string
	switch providerName {
	case "forgejo":
		token = u.ForgejoAccessToken
	default:
		token = u.AccessToken
	}
	return p, token
}

func (s *Syncer) SyncOne(ctx context.Context, repo *ent.Repository) *ent.Repository {
	u, err := repo.QueryUser().Only(ctx)
	if err != nil {
		return repo
	}

	p, token := s.getProvider(u, repo)

	// Snapshot before state for event detection
	beforeReleaseTag := repo.LatestReleaseTag
	beforeWorkflowStatus := repo.WorkflowStatus
	beforeReleaseConclusion := repo.LatestReleaseConclusion

	updated := s.client.Repository.UpdateOne(repo)

	s.syncCommits(ctx, p, token, u, repo, updated)
	s.syncRelease(ctx, p, token, u, repo, updated)
	s.syncWorkflows(ctx, p, token, repo, updated)
	s.syncPullRequests(ctx, p, token, repo, updated)

	if !repo.LatestCommitDate.IsZero() && !repo.LatestReleaseDate.IsZero() {
		leadHours := repo.LatestReleaseDate.Sub(repo.LatestCommitDate).Hours()
		if leadHours > 0 {
			updated.SetAvgLeadTimeHours(leadHours)
		}
	}

	updated.SetSyncedAt(time.Now())
	repo, err = updated.Save(ctx)
	if err != nil {
		return repo
	}

	s.recordEvents(ctx, u, repo, beforeReleaseTag, beforeWorkflowStatus, beforeReleaseConclusion)
	s.recordSnapshot(ctx, repo)

	if s.hub != nil {
		s.broadcastUpdate(repo, u)
	}
	return repo
}

func (s *Syncer) syncCommits(ctx context.Context, p provider.Provider, token string, u *ent.User, repo *ent.Repository, updated *ent.RepositoryUpdateOne) {
	var since time.Time
	if !repo.SyncedAt.IsZero() {
		since = repo.SyncedAt
	}
	commits, err := p.GetCommitsSince(ctx, token, repo.Owner, repo.Name, repo.DefaultBranch, since, 500)
	if err != nil {
		log.Printf("Error fetching commits for %s: %v", repo.FullName, err)
		return
	}

	total := repo.TotalCommitsFetched
	if repo.SyncedAt.IsZero() {
		total = len(commits)
	} else {
		total += len(commits)
	}
	updated.SetTotalCommitsFetched(total)

	if len(commits) > 0 {
		updated.SetLatestCommitSha(commits[0].SHA)
		updated.SetLatestCommitMessage(commits[0].Message)
		updated.SetLatestCommitDate(commits[0].Date)
	}

	var feat, fix, docs, chore, other int
	for _, c := range commits {
		switch github.ParseCommitType(c.Message) {
		case "feat":
			feat++
		case "fix":
			fix++
		case "docs":
			docs++
		case "chore":
			chore++
		default:
			other++
		}
	}
	if repo.SyncedAt.IsZero() {
		updated.SetFeatCount(feat)
		updated.SetFixCount(fix)
		updated.SetDocsCount(docs)
		updated.SetChoreCount(chore)
		updated.SetOtherCommitCount(other)
	} else {
		updated.SetFeatCount(repo.FeatCount + feat)
		updated.SetFixCount(repo.FixCount + fix)
		updated.SetDocsCount(repo.DocsCount + docs)
		updated.SetChoreCount(repo.ChoreCount + chore)
		updated.SetOtherCommitCount(repo.OtherCommitCount + other)
	}
}

func (s *Syncer) syncRelease(ctx context.Context, p provider.Provider, token string, u *ent.User, repo *ent.Repository, updated *ent.RepositoryUpdateOne) {
	releases, err := p.ListReleases(ctx, token, repo.Owner, repo.Name)
	if err != nil {
		log.Printf("Error fetching releases for %s: %v", repo.FullName, err)
		return
	}
	updated.SetReleaseCount(len(releases))

	if len(releases) > 0 {
		updated.SetLatestReleaseTag(releases[0].TagName)
		updated.SetLatestReleaseName(releases[0].Name)
		updated.SetLatestReleaseDate(releases[0].PublishedAt)

		// Get the latest completed workflow run for the release (try tag, then default branch)
		run, err := p.GetLatestWorkflowRun(ctx, token, repo.Owner, repo.Name, releases[0].TagName)
		if err != nil {
			run, err = p.GetLatestWorkflowRun(ctx, token, repo.Owner, repo.Name, repo.DefaultBranch)
		}
		if err == nil {
			updated.SetLatestReleaseConclusion(run.Conclusion)
		} else {
			updated.SetLatestReleaseConclusion("unknown")
		}
	}
}

func (s *Syncer) syncPullRequests(ctx context.Context, p provider.Provider, token string, repo *ent.Repository, updated *ent.RepositoryUpdateOne) {
	prs, err := p.ListPullRequests(ctx, token, repo.Owner, repo.Name)
	if err != nil {
		log.Printf("Error fetching pull requests for %s: %v", repo.FullName, err)
		return
	}

	updated.SetOpenPrCount(len(prs))

	if len(prs) > 0 {
		type prSummary struct {
			Number    int    `json:"n"`
			Title     string `json:"t"`
			Author    string `json:"a"`
			CreatedAt string `json:"c"`
			HTMLURL   string `json:"h"`
			HeadRef   string `json:"hr"`
			BaseRef   string `json:"br"`
		}
		var summaries []prSummary
		for _, pr := range prs {
			summaries = append(summaries, prSummary{
				Number:    pr.Number,
				Title:     pr.Title,
				Author:    pr.Author,
				CreatedAt: pr.CreatedAt.Format(time.RFC3339),
				HTMLURL:   pr.HTMLURL,
				HeadRef:   pr.HeadRef,
				BaseRef:   pr.BaseRef,
			})
		}
		data, err := json.Marshal(summaries)
		if err == nil {
			updated.SetPullRequests(string(data))
		}
	} else {
		updated.SetPullRequests("[]")
	}
}

func (s *Syncer) syncWorkflows(ctx context.Context, p provider.Provider, token string, repo *ent.Repository, updated *ent.RepositoryUpdateOne) {
	// Forgejo Actions is opt-in; we report "unknown" without an HTTP call.
	if repo.Provider == "forgejo" {
		updated.SetWorkflowStatus("unknown")
		return
	}
	// The github client has GetWorkflowRuns which is not part of the
	// Provider interface (GitHub-specific). Fall back to s.gh for now.
	runs, err := s.gh.GetWorkflowRuns(token, repo.Owner, repo.Name, repo.DefaultBranch, 30)
	if err != nil {
		log.Printf("Error fetching workflows for %s: %v", repo.FullName, err)
		return
	}

	var success, failure int
	for _, r := range runs {
		switch r.Conclusion {
		case "success":
			success++
		case "failure":
			failure++
		}
	}
	updated.SetWorkflowSuccessCount(success)
	updated.SetWorkflowFailureCount(failure)

	if len(runs) > 0 {
		updated.SetWorkflowStatus(runs[0].Conclusion)
		updated.SetWorkflowRunID(runs[0].ID)
	}
}

func (s *Syncer) SyncOneByGithubID(ctx context.Context, githubID int64) {
	r, err := s.client.Repository.Query().Where(repository.GithubID(githubID)).Only(ctx)
	if err != nil {
		return
	}
	s.SyncOne(ctx, r)
}

func (s *Syncer) recordEvents(ctx context.Context, u *ent.User, repo *ent.Repository, beforeReleaseTag, beforeWorkflowStatus, beforeReleaseConclusion string) {
	otel.TraceDBQuery(ctx, "event_record", func(ctx context.Context) (struct{}, error) {
		s.recordReleaseEvent(ctx, repo, beforeReleaseTag, beforeReleaseConclusion)
		s.recordWorkflowFailureEvent(ctx, repo, beforeWorkflowStatus)
		s.recordPRMergeEvents(ctx, u, repo)
		return struct{}{}, nil
	})
}

func (s *Syncer) recordReleaseEvent(ctx context.Context, repo *ent.Repository, beforeTag, beforeConclusion string) {
	if repo.LatestReleaseTag == "" || repo.LatestReleaseTag == beforeTag {
		return
	}
	// Check if we already recorded this release event
	exists, _ := s.client.Event.Query().
		Where(event.RepoID(repo.ID)).
		Where(event.TypeEQ(event.TypeRelease)).
		Where(event.TitleEQ(repo.LatestReleaseTag)).
		Exist(ctx)
	if exists {
		return
	}

	meta, _ := json.Marshal(map[string]string{
		"conclusion": repo.LatestReleaseConclusion,
	})
	s.client.Event.Create().
		SetRepoID(repo.ID).
		SetType(event.TypeRelease).
		SetTitle(repo.LatestReleaseTag).
		SetURL(fmt.Sprintf("%s/releases/tag/%s", repo.HTMLURL, repo.LatestReleaseTag)).
		SetMetadata(string(meta)).
		SetTimestamp(repo.LatestReleaseDate).
		Save(ctx)
}

func (s *Syncer) recordWorkflowFailureEvent(ctx context.Context, repo *ent.Repository, beforeStatus string) {
	status := repo.WorkflowStatus
	if status != "failure" && status != "cancelled" {
		return
	}
	if status == beforeStatus {
		return
	}
	// Check if we already recorded this failure
	exists, _ := s.client.Event.Query().
		Where(event.RepoID(repo.ID)).
		Where(event.TypeEQ(event.TypeWorkflowFailure)).
		Where(event.TitleContains("build #")).
		Exist(ctx)
	if !exists || beforeStatus == "" {
		meta, _ := json.Marshal(map[string]any{
			"run_id": repo.WorkflowRunID,
			"status": status,
		})
		s.client.Event.Create().
			SetRepoID(repo.ID).
			SetType(event.TypeWorkflowFailure).
			SetTitle(fmt.Sprintf("CI %s — build #%d", status, repo.WorkflowRunID)).
			SetURL(fmt.Sprintf("%s/actions/runs/%d", repo.HTMLURL, repo.WorkflowRunID)).
			SetMetadata(string(meta)).
			SetTimestamp(time.Now()).
			Save(ctx)
	}
}

func (s *Syncer) recordPRMergeEvents(ctx context.Context, u *ent.User, repo *ent.Repository) {
	p, token := s.getProvider(u, repo)
	mergedPRs, err := p.ListRecentlyMergedPRs(ctx, token, repo.Owner, repo.Name)
	if err != nil {
		log.Printf("Error fetching merged PRs for %s: %v", repo.FullName, err)
		return
	}
	for _, pr := range mergedPRs {
		title := fmt.Sprintf("#%d %s", pr.Number, pr.Title)
		exists, _ := s.client.Event.Query().
			Where(event.RepoID(repo.ID)).
			Where(event.TypeEQ(event.TypePrMerge)).
			Where(event.TitleEQ(title)).
			Exist(ctx)
		if exists {
			continue
		}
		meta, _ := json.Marshal(map[string]any{
			"author":   pr.Author,
			"number":   pr.Number,
			"base_ref": pr.BaseRef,
		})
		s.client.Event.Create().
			SetRepoID(repo.ID).
			SetType(event.TypePrMerge).
			SetTitle(title).
			SetURL(pr.HTMLURL).
			SetMetadata(string(meta)).
			SetTimestamp(pr.CreatedAt).
			Save(ctx)
	}
}

func (s *Syncer) recordSnapshot(ctx context.Context, repo *ent.Repository) {
	_, err := otel.TraceDBQuery(ctx, "snapshot_record", func(ctx context.Context) (*ent.MetricSnapshot, error) {
		return s.client.MetricSnapshot.Create().
			SetRepoID(repo.ID).
			SetTimestamp(time.Now()).
			SetFeatCount(repo.FeatCount).
			SetFixCount(repo.FixCount).
			SetDocsCount(repo.DocsCount).
			SetChoreCount(repo.ChoreCount).
			SetOtherCommitCount(repo.OtherCommitCount).
			SetTotalCommitsFetched(repo.TotalCommitsFetched).
			SetReleaseCount(repo.ReleaseCount).
			SetAvgLeadTimeHours(repo.AvgLeadTimeHours).
			SetWorkflowSuccessCount(repo.WorkflowSuccessCount).
			SetWorkflowFailureCount(repo.WorkflowFailureCount).
			SetWorkflowStatus(repo.WorkflowStatus).
			Save(ctx)
	})
	if err != nil {
		log.Printf("Error recording metric snapshot for %s: %v", repo.FullName, err)
	}
}

func (s *Syncer) broadcastUpdate(repo *ent.Repository, u *ent.User) {
	if s.tmpl == nil {
		return
	}
	var buf bytes.Buffer
	err := s.tmpl.ExecuteTemplate(&buf, "repo_card", repo)
	if err != nil {
		return
	}
	// Wrap in hx-swap-oob so HTMX replaces only the targeted repo card
	// instead of replacing the entire #repo-grid innerHTML.
	msg := fmt.Sprintf(`<div id="repo-card-%d" hx-swap-oob="outerHTML">%s</div>`, repo.ID, buf.String())
	s.hub.BroadcastToUser(int64(u.ID), []byte(msg))
}
