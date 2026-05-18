package handlers

import (
	"log"
	"net/http"
	"strconv"

	"gitoverviewer/ent"
	"gitoverviewer/ent/repository"
	"gitoverviewer/ent/user"
	"gitoverviewer/internal/github"
	mw "gitoverviewer/internal/middleware"
	"gitoverviewer/internal/sync"

	"github.com/gin-gonic/gin"
)

type SettingsHandler struct {
	client *ent.Client
	store  *mw.SessionStore
	gh     *github.Client
	syncer *sync.Syncer
}

func NewSettingsHandler(client *ent.Client, store *mw.SessionStore, gh *github.Client, syncer *sync.Syncer) *SettingsHandler {
	return &SettingsHandler{client: client, store: store, gh: gh, syncer: syncer}
}

func (h *SettingsHandler) Index(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.client.User.Get(c.Request.Context(), int(userID))
	if err != nil {
		c.String(http.StatusInternalServerError, "User not found")
		return
	}

	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(c.Request.Context())

	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	webhookURL := scheme + "://" + c.Request.Host + "/webhook/github"

	c.HTML(http.StatusOK, "settings", gin.H{
		"User":        u,
		"Repos":       repos,
		"WebhookURL":  webhookURL,
	})
}

func (h *SettingsHandler) UpdateInterval(c *gin.Context) {
	userID := c.GetInt64("user_id")
	minutesStr := c.PostForm("interval")
	minutes, err := strconv.Atoi(minutesStr)
	if err != nil || minutes < 1 || minutes > 1440 {
		c.String(http.StatusBadRequest, "Invalid interval (1-1440 minutes)")
		return
	}

	_, err = h.client.User.UpdateOneID(int(userID)).
		SetSyncIntervalMinutes(minutes).
		Save(c.Request.Context())
	if err != nil {
		log.Printf("Error updating interval: %v", err)
		c.String(http.StatusInternalServerError, "Failed to update interval")
		return
	}

	c.String(http.StatusOK, "Sync interval updated to %d minutes", minutes)
}

func (h *SettingsHandler) AvailableRepos(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.client.User.Get(c.Request.Context(), int(userID))
	if err != nil {
		c.String(http.StatusInternalServerError, "User not found")
		return
	}

	ghRepos, err := h.gh.ListRepositories(u.AccessToken)
	if err != nil {
		log.Printf("Error listing GitHub repos: %v", err)
		c.HTML(http.StatusInternalServerError, "available_repos", gin.H{"Error": "Failed to fetch repositories from GitHub"})
		return
	}

	trackedIDs := make(map[int64]bool)
	tracked, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		All(c.Request.Context())
	for _, r := range tracked {
		trackedIDs[r.GithubID] = true
	}

	var untracked []*github.Repository
	for _, r := range ghRepos {
		if !trackedIDs[r.ID] {
			untracked = append(untracked, r)
		}
	}

	c.HTML(http.StatusOK, "available_repos", gin.H{
		"Repos": untracked,
	})
}

func (h *SettingsHandler) SelectRepos(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.client.User.Get(c.Request.Context(), int(userID))
	if err != nil {
		c.String(http.StatusInternalServerError, "User not found")
		return
	}

	ids := c.PostFormArray("repo_ids")
	if len(ids) == 0 {
		c.String(http.StatusBadRequest, "No repositories selected")
		return
	}

	ghRepos, err := h.gh.ListRepositories(u.AccessToken)
	if err != nil {
		log.Printf("Error listing repos: %v", err)
		c.String(http.StatusInternalServerError, "Failed to fetch repositories")
		return
	}

	selected := make(map[int64]*github.Repository)
	for _, r := range ghRepos {
		for _, idStr := range ids {
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				continue
			}
			if r.ID == id {
				selected[id] = r
				break
			}
		}
	}

	ctx := c.Request.Context()
	for _, r := range selected {
		exists, _ := h.client.Repository.Query().
			Where(
				repository.HasUserWith(user.ID(u.ID)),
				repository.GithubID(r.ID),
			).
			Exist(ctx)
		if exists {
			continue
		}

		_, err := h.client.Repository.Create().
			SetGithubID(r.ID).
			SetOwner(r.Owner).
			SetName(r.Name).
			SetFullName(r.FullName).
			SetDescription(r.Description).
			SetHTMLURL(r.HTMLURL).
			SetLanguage(r.Language).
			SetDefaultBranch(r.DefaultBranch).
			SetUserID(u.ID).
			Save(ctx)
		if err != nil {
			log.Printf("Error saving repo %s: %v", r.FullName, err)
		} else {
			log.Printf("Added repo %s", r.FullName)
		}
	}

	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(ctx)

	for _, r := range repos {
		h.syncer.SyncOne(ctx, r)
	}

	repos, _ = h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(ctx)

	repos, _ = h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(ctx)

	c.HTML(http.StatusOK, "dashboard", gin.H{
		"User":    u,
		"Repos":   repos,
		"Metrics": computeMetrics(repos),
	})
}

func (h *SettingsHandler) RemoveRepo(c *gin.Context) {
	userID := c.GetInt64("user_id")
	repoID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid repo ID")
		return
	}

	ctx := c.Request.Context()
	n, err := h.client.Repository.Delete().
		Where(
			repository.ID(repoID),
			repository.HasUserWith(user.ID(int(userID))),
		).
		Exec(ctx)
	if err != nil {
		log.Printf("Error removing repo %d: %v", repoID, err)
		c.String(http.StatusInternalServerError, "Failed to remove repository")
		return
	}
	if n == 0 {
		c.String(http.StatusNotFound, "Repository not found")
		return
	}

	c.Status(http.StatusOK)
}
