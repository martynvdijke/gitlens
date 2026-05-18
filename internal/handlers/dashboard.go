package handlers

import (
	"math"
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

type DORAMetrics struct {
	TotalRepos          int
	TotalReleases       int
	TotalCommits        int
	FeatCount           int
	FixCount            int
	DocsCount           int
	ChoreCount          int
	OtherCount          int
	WorkflowSuccesses   int
	WorkflowFailures    int
	WorkflowPassRate    float64
	AvgLeadTimeHours    float64
	FeatPct             float64
	FixPct              float64
	DocsPct             float64
	ChorePct            float64
	ReleasesPerRepo     float64
}

type DashboardHandler struct {
	client *ent.Client
	store  *mw.SessionStore
	gh     *github.Client
	syncer *sync.Syncer
}

func NewDashboardHandler(client *ent.Client, store *mw.SessionStore, gh *github.Client, syncer *sync.Syncer) *DashboardHandler {
	return &DashboardHandler{client: client, store: store, gh: gh, syncer: syncer}
}

func computeMetrics(repos []*ent.Repository) *DORAMetrics {
	m := &DORAMetrics{
		TotalRepos: len(repos),
	}
	var leadSamples, leadSum float64
	for _, r := range repos {
		m.TotalReleases += r.ReleaseCount
		m.TotalCommits += r.TotalCommitsFetched
		m.FeatCount += r.FeatCount
		m.FixCount += r.FixCount
		m.DocsCount += r.DocsCount
		m.ChoreCount += r.ChoreCount
		m.OtherCount += r.OtherCommitCount
		m.WorkflowSuccesses += r.WorkflowSuccessCount
		m.WorkflowFailures += r.WorkflowFailureCount
		if r.AvgLeadTimeHours > 0 {
			leadSum += r.AvgLeadTimeHours
			leadSamples++
		}
	}
	totalCommits := m.FeatCount + m.FixCount + m.DocsCount + m.ChoreCount + m.OtherCount
	if totalCommits > 0 {
		m.FeatPct = roundPct(float64(m.FeatCount) / float64(totalCommits) * 100)
		m.FixPct = roundPct(float64(m.FixCount) / float64(totalCommits) * 100)
		m.DocsPct = roundPct(float64(m.DocsCount) / float64(totalCommits) * 100)
		m.ChorePct = roundPct(float64(m.ChoreCount) / float64(totalCommits) * 100)
	}
	totalWorkflows := m.WorkflowSuccesses + m.WorkflowFailures
	if totalWorkflows > 0 {
		m.WorkflowPassRate = roundPct(float64(m.WorkflowSuccesses) / float64(totalWorkflows) * 100)
	}
	if leadSamples > 0 {
		m.AvgLeadTimeHours = math.Round(leadSum/leadSamples*10) / 10
	}
	if m.TotalRepos > 0 {
		m.ReleasesPerRepo = math.Round(float64(m.TotalReleases)/float64(m.TotalRepos)*10) / 10
	}
	return m
}

func roundPct(v float64) float64 {
	return math.Round(v*10) / 10
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
		"User":    u,
		"Repos":   repos,
		"Metrics": computeMetrics(repos),
	})
}

func (h *DashboardHandler) Dashboard(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, _ := h.client.User.Get(c.Request.Context(), int(userID))
	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(c.Request.Context())

	c.HTML(http.StatusOK, "dashboard", gin.H{
		"User":    u,
		"Repos":   repos,
		"Metrics": computeMetrics(repos),
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

func (h *DashboardHandler) ImportAllRepos(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.client.User.Get(c.Request.Context(), int(userID))
	if err != nil {
		c.String(http.StatusInternalServerError, "User not found")
		return
	}

	ghRepos, err := h.gh.ListRepositories(u.AccessToken)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "repo_list", gin.H{"Error": "Failed to fetch repositories from GitHub"})
		return
	}

	ctx := c.Request.Context()
	for _, r := range ghRepos {
		exists, _ := h.client.Repository.Query().
			Where(
				repository.HasUserWith(user.ID(u.ID)),
				repository.GithubID(r.ID),
			).
			Exist(ctx)
		if exists {
			continue
		}

		h.client.Repository.Create().
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

	c.HTML(http.StatusOK, "repo_list", gin.H{"Repos": repos})
}
