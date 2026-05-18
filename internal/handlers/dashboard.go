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

type DashboardHandler struct {
	client *ent.Client
	store  *mw.SessionStore
	gh     *github.Client
	syncer *sync.Syncer
}

func NewDashboardHandler(client *ent.Client, store *mw.SessionStore, gh *github.Client, syncer *sync.Syncer) *DashboardHandler {
	return &DashboardHandler{client: client, store: store, gh: gh, syncer: syncer}
}

func (h *DashboardHandler) Index(c *gin.Context) {
	var u *ent.User
	var repos []*ent.Repository

	sessionID, err := c.Cookie("gitoverviewer_session")
	if err == nil {
		userID, ok := h.store.Get(sessionID)
		if ok {
			u, _ = h.client.User.Get(c.Request.Context(), int(userID))
			if u != nil {
				repos, _ = h.client.Repository.Query().
					Where(repository.HasUserWith(user.ID(u.ID))).
					Order(ent.Desc(repository.FieldUpdatedAt)).
					All(c.Request.Context())
			}
		}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"User":  u,
		"Repos": repos,
	})
}

func (h *DashboardHandler) ListRepos(c *gin.Context) {
	userID := c.GetInt64("user_id")
	repos, err := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(c.Request.Context())
	if err != nil {
		c.HTML(http.StatusInternalServerError, "repo_list", gin.H{"Error": "Failed to fetch repositories"})
		return
	}
	c.HTML(http.StatusOK, "repo_list", gin.H{"Repos": repos})
}

func (h *DashboardHandler) SyncRepos(c *gin.Context) {
	userID := c.GetInt64("user_id")
	ctx := c.Request.Context()

	u, err := h.client.User.Get(ctx, int(userID))
	if err != nil {
		c.HTML(http.StatusInternalServerError, "repo_list", gin.H{"Error": "User not found"})
		return
	}

	ghRepos, err := h.gh.ListRepositories(u.AccessToken)
	if err != nil {
		log.Printf("Error listing repos: %v", err)
		c.HTML(http.StatusInternalServerError, "repo_list", gin.H{"Error": "Failed to fetch repositories from GitHub"})
		return
	}

	for _, r := range ghRepos {
		exists, err := h.client.Repository.Query().
			Where(
				repository.HasUserWith(user.ID(u.ID)),
				repository.GithubID(r.ID),
			).
			Exist(ctx)
		if err != nil {
			log.Printf("Error checking repo existence: %v", err)
			continue
		}
		if exists {
			continue
		}

		_, err = h.client.Repository.Create().
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
		}
	}

	repos, err := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(ctx)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "repo_list", gin.H{"Error": "Database error"})
		return
	}

	for _, r := range repos {
		h.syncer.SyncOne(ctx, r)
	}

	repos, _ = h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(ctx)

	c.HTML(http.StatusOK, "repo_list", gin.H{"Repos": repos})
}

func (h *DashboardHandler) SyncRepo(c *gin.Context) {
	userID := c.GetInt64("user_id")
	repoID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid repo ID")
		return
	}

	ctx := c.Request.Context()
	r, err := h.client.Repository.Query().
		Where(
			repository.ID(repoID),
			repository.HasUserWith(user.ID(int(userID))),
		).
		Only(ctx)
	if err != nil {
		c.String(http.StatusNotFound, "Repository not found")
		return
	}

	r = h.syncer.SyncOne(ctx, r)
	c.HTML(http.StatusOK, "repo_card", r)
}
