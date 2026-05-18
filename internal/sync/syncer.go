package sync

import (
	"bytes"
	"context"
	"html/template"
	"log"
	"time"

	"gitoverviewer/ent"
	"gitoverviewer/ent/repository"
	"gitoverviewer/ent/user"
	"gitoverviewer/internal/github"
	"gitoverviewer/internal/ws"
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
		log.Printf("Error fetching repos for user %d: %v", u.ID, err)
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

	commit, err := s.gh.GetLatestCommit(u.AccessToken, repo.Owner, repo.Name, repo.DefaultBranch)
	if err != nil {
		log.Printf("Error fetching commit for %s: %v", repo.FullName, err)
	} else {
		updated.SetLatestCommitSha(commit.SHA)
		updated.SetLatestCommitMessage(commit.Message)
		updated.SetLatestCommitDate(commit.Date)
	}

	release, err := s.gh.GetLatestRelease(u.AccessToken, repo.Owner, repo.Name)
	if err != nil {
		log.Printf("Error fetching release for %s: %v", repo.FullName, err)
	} else {
		updated.SetLatestReleaseTag(release.TagName)
		updated.SetLatestReleaseName(release.Name)
		updated.SetLatestReleaseDate(release.PublishedAt)
	}

	workflow, err := s.gh.GetWorkflowStatus(u.AccessToken, repo.Owner, repo.Name, repo.DefaultBranch)
	if err != nil {
		log.Printf("Error fetching workflow for %s: %v", repo.FullName, err)
		updated.SetWorkflowStatus("unknown")
	} else {
		updated.SetWorkflowStatus(workflow.Conclusion)
		updated.SetWorkflowRunID(workflow.ID)
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
