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

func (s *Syncer) StartPeriodicSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Println("Starting periodic sync...")
			s.SyncAll(ctx)
		}
	}
}

func (s *Syncer) SyncAll(ctx context.Context) {
	users, err := s.client.User.Query().All(ctx)
	if err != nil {
		log.Printf("Error fetching users for sync: %v", err)
		return
	}
	for _, u := range users {
		repos, err := s.client.Repository.Query().Where(repository.HasUserWith(user.ID(u.ID))).All(ctx)
		if err != nil {
			log.Printf("Error fetching repos for user %d: %v", u.ID, err)
			continue
		}
		for _, r := range repos {
			s.SyncOne(ctx, r)
		}
	}
}

func (s *Syncer) SyncOne(ctx context.Context, repo *ent.Repository) *ent.Repository {
	u, err := repo.QueryUser().Only(ctx)
	if err != nil {
		log.Printf("Error fetching user for repo %s: %v", repo.FullName, err)
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
		log.Printf("Error saving repo %s: %v", repo.FullName, err)
		return repo
	}

	if s.hub != nil {
		s.broadcastUpdate(repo)
	}

	return repo
}

func (s *Syncer) broadcastUpdate(repo *ent.Repository) {
	if s.tmpl == nil {
		return
	}
	var buf bytes.Buffer
	err := s.tmpl.ExecuteTemplate(&buf, "repo_card", repo)
	if err != nil {
		log.Printf("Error rendering repo card for broadcast: %v", err)
		return
	}
	s.hub.Broadcast(buf.Bytes())
}
