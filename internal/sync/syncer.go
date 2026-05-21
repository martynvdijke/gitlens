package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"log"
	"time"

	"gitlens/ent"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	"gitlens/internal/github"
	"gitlens/internal/ws"
)

type Syncer struct {
	client *ent.Client
	gh     *github.Client
	hub    *ws.Hub
	tmpl   *template.Template
}

func NewSyncer(client *ent.Client, gh *github.Client, hub *ws.Hub) *Syncer {
	return &Syncer{
		client: client,
		gh:     gh,
		hub:    hub,
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

func (s *Syncer) SyncOne(ctx context.Context, repo *ent.Repository) *ent.Repository {
	u, err := repo.QueryUser().Only(ctx)
	if err != nil {
		return repo
	}

	updated := s.client.Repository.UpdateOne(repo)

	s.syncCommits(ctx, u, repo, updated)
	s.syncRelease(ctx, u, repo, updated)
	s.syncWorkflows(ctx, u, repo, updated)
	s.syncPullRequests(ctx, u, repo, updated)

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

	if s.hub != nil {
		s.broadcastUpdate(repo)
	}
	return repo
}

func (s *Syncer) syncCommits(ctx context.Context, u *ent.User, repo *ent.Repository, updated *ent.RepositoryUpdateOne) {
	commits, err := s.gh.GetCommits(u.AccessToken, repo.Owner, repo.Name, repo.DefaultBranch, 50)
	if err != nil {
		log.Printf("Error fetching commits for %s: %v", repo.FullName, err)
		return
	}

	updated.SetTotalCommitsFetched(len(commits))
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
	updated.SetFeatCount(feat)
	updated.SetFixCount(fix)
	updated.SetDocsCount(docs)
	updated.SetChoreCount(chore)
	updated.SetOtherCommitCount(other)
}

func (s *Syncer) syncRelease(ctx context.Context, u *ent.User, repo *ent.Repository, updated *ent.RepositoryUpdateOne) {
	releases, err := s.gh.ListReleases(u.AccessToken, repo.Owner, repo.Name)
	if err != nil {
		log.Printf("Error fetching releases for %s: %v", repo.FullName, err)
		return
	}
	updated.SetReleaseCount(len(releases))

	if len(releases) > 0 {
		updated.SetLatestReleaseTag(releases[0].TagName)
		updated.SetLatestReleaseName(releases[0].Name)
		updated.SetLatestReleaseDate(releases[0].PublishedAt)

		// Check workflow runs without branch filter to catch tag-triggered runs
		run, err := s.gh.GetWorkflowStatus(u.AccessToken, repo.Owner, repo.Name, "")
		if err != nil {
			run, err = s.gh.GetWorkflowStatus(u.AccessToken, repo.Owner, repo.Name, repo.DefaultBranch)
		}
		if err == nil {
			updated.SetLatestReleaseConclusion(run.Conclusion)
		} else {
			updated.SetLatestReleaseConclusion("unknown")
		}
	}
}

func (s *Syncer) syncPullRequests(ctx context.Context, u *ent.User, repo *ent.Repository, updated *ent.RepositoryUpdateOne) {
	prs, err := s.gh.ListPullRequests(u.AccessToken, repo.Owner, repo.Name)
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

func (s *Syncer) syncWorkflows(ctx context.Context, u *ent.User, repo *ent.Repository, updated *ent.RepositoryUpdateOne) {
	runs, err := s.gh.GetWorkflowRuns(u.AccessToken, repo.Owner, repo.Name, repo.DefaultBranch, 30)
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

func (s *Syncer) broadcastUpdate(repo *ent.Repository) {
	if s.tmpl == nil {
		return
	}
	var buf bytes.Buffer
	err := s.tmpl.ExecuteTemplate(&buf, "repo_card", repo)
	if err != nil {
		return
	}
	s.hub.Broadcast(buf.Bytes())
}
