package handlers

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"

	"gitlens/ent"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	"gitlens/internal/github"
	mw "gitlens/internal/middleware"
	"gitlens/internal/sync"

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

	sessionID, err := c.Cookie("gitlens_session")
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

	q := c.Query("q")
	query := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(ent.Desc(repository.FieldUpdatedAt))
	if q != "" {
		query = query.Where(
			repository.Or(
				repository.FullNameContainsFold(q),
				repository.NameContainsFold(q),
			),
		)
	}
	repos, _ := query.All(c.Request.Context())

	c.HTML(http.StatusOK, "dashboard", gin.H{
		"User":    u,
		"Repos":   repos,
		"Metrics": computeMetrics(repos),
	})
}

func (h *DashboardHandler) ListRepos(c *gin.Context) {
	userID := c.GetInt64("user_id")
	q := c.Query("q")

	query := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(ent.Desc(repository.FieldUpdatedAt))
	if q != "" {
		query = query.Where(
			repository.Or(
				repository.FullNameContainsFold(q),
				repository.NameContainsFold(q),
			),
		)
	}
	repos, err := query.All(c.Request.Context())
	if err != nil {
		c.HTML(http.StatusInternalServerError, "repo_list", gin.H{"Error": "Failed to fetch repositories"})
		return
	}
	c.HTML(http.StatusOK, "repo_list", gin.H{"Repos": repos, "Query": q})
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

func (h *DashboardHandler) ListPullRequests(c *gin.Context) {
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

	type prSummary struct {
		Number    int    `json:"n"`
		Title     string `json:"t"`
		Author    string `json:"a"`
		CreatedAt string `json:"c"`
		HTMLURL   string `json:"h"`
		HeadRef   string `json:"hr"`
		BaseRef   string `json:"br"`
	}
	var prs []prSummary
	if r.PullRequests != "" {
		json.Unmarshal([]byte(r.PullRequests), &prs)
	}

	c.HTML(http.StatusOK, "pr_list", gin.H{
		"Repo":  r,
		"PRs":   prs,
		"Count": len(prs),
	})
}

func (h *DashboardHandler) MergePR(c *gin.Context) {
	userID := c.GetInt64("user_id")
	repoID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid repo ID")
		return
	}
	prNumber, err := strconv.Atoi(c.Param("number"))
	if err != nil {
		c.String(http.StatusBadRequest, "Invalid PR number")
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

	u, err := h.client.User.Get(ctx, int(userID))
	if err != nil {
		c.String(http.StatusInternalServerError, "User not found")
		return
	}

	merged, msg, err := h.gh.MergePullRequest(u.AccessToken, r.Owner, r.Name, prNumber)
	if err != nil {
		log.Printf("Error merging PR #%d for %s: %v", prNumber, r.FullName, err)
		c.String(http.StatusInternalServerError, "Failed to merge PR: %v", err)
		return
	}
	if !merged {
		c.String(http.StatusConflict, "Merge failed: %s", msg)
		return
	}

	r = h.syncer.SyncOne(ctx, r)
	c.HTML(http.StatusOK, "repo_card", r)
}

func (h *DashboardHandler) MergeAllPRs(c *gin.Context) {
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

	u, err := h.client.User.Get(ctx, int(userID))
	if err != nil {
		c.String(http.StatusInternalServerError, "User not found")
		return
	}

	prs, err := h.gh.ListPullRequests(u.AccessToken, r.Owner, r.Name)
	if err != nil {
		log.Printf("Error listing PRs for merge-all on %s: %v", r.FullName, err)
		c.String(http.StatusInternalServerError, "Failed to list pull requests")
		return
	}

	var failed []int
	for _, pr := range prs {
		merged, _, err := h.gh.MergePullRequest(u.AccessToken, r.Owner, r.Name, pr.Number)
		if err != nil || !merged {
			failed = append(failed, pr.Number)
		}
	}

	r = h.syncer.SyncOne(ctx, r)

	if len(failed) > 0 {
		c.String(http.StatusOK, "Merged %d PR(s). Failed: %v", len(prs)-len(failed), failed)
	} else {
		c.String(http.StatusOK, "All %d PR(s) merged successfully!", len(prs))
	}
}
