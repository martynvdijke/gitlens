package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

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
	bgCtx  context.Context
}

func NewDashboardHandler(client *ent.Client, store *mw.SessionStore, gh *github.Client, syncer *sync.Syncer) *DashboardHandler {
	return &DashboardHandler{client: client, store: store, gh: gh, syncer: syncer, bgCtx: context.Background()}
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
	var newIDs []int
	for _, r := range ghRepos {
		exists, err := h.client.Repository.Query().
			Where(
				repository.HasUserWith(user.ID(u.ID)),
				repository.GithubID(r.ID),
			).
			Exist(ctx)
		if err != nil {
			log.Printf("Error checking repo existence %s: %v", r.FullName, err)
			continue
		}
		if exists {
			continue
		}

		repo, err := h.client.Repository.Create().
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
			log.Printf("Error creating repo %s: %v", r.FullName, err)
			continue
		}
		newIDs = append(newIDs, repo.ID)
	}

	// Sync new repos in background so the HTTP request returns quickly.
	// WebSocket broadcasts push updated repo cards as each sync completes.
	go func() {
		for _, id := range newIDs {
			r, err := h.client.Repository.Get(h.bgCtx, id)
			if err != nil {
				log.Printf("Error fetching new repo ID %d: %v", id, err)
				continue
			}
			h.syncer.SyncOne(h.bgCtx, r)
		}
	}()

	repos, _ := h.client.Repository.Query().
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

// PRQueueItem represents a pull request in the unified cross-repo queue.
type PRQueueItem struct {
	RepoID       int
	RepoFullName string
	Number       int
	Title        string
	Author       string
	CreatedAt    string
	HTMLURL      string
	HeadRef      string
	BaseRef      string
}

// prSummary is the JSON-unmarshalled form of a pull request stored on a Repository.
type prSummary struct {
	Number    int    `json:"n"`
	Title     string `json:"t"`
	Author    string `json:"a"`
	CreatedAt string `json:"c"`
	HTMLURL   string `json:"h"`
	HeadRef   string `json:"hr"`
	BaseRef   string `json:"br"`
}

// Index renders the full page with repos tab as default.
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

	// Post-login: if user has no tracked repos, fetch available ones from GitHub
	// and show the repo selector instead of an empty dashboard.
	if u != nil && len(repos) == 0 {
		ghRepos, err := h.gh.ListRepositories(u.AccessToken)
		if err == nil {
			available := make([]*github.Repository, 0, len(ghRepos))
			for _, r := range ghRepos {
				available = append(available, &github.Repository{
					ID:            r.ID,
					Owner:         r.Owner,
					Name:          r.Name,
					FullName:      r.FullName,
					Description:   r.Description,
					HTMLURL:       r.HTMLURL,
					Language:      r.Language,
					DefaultBranch: r.DefaultBranch,
				})
			}
			c.HTML(http.StatusOK, "index.html", gin.H{
				"User":      u,
				"Repos":     available,
				"Metrics":   computeMetrics(nil),
				"ActiveTab": "repos",
				"ShowSetup": true,
			})
			return
		}
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"User":      u,
		"Repos":     repos,
		"Metrics":   computeMetrics(repos),
		"ActiveTab": "repos",
	})
}

// ReposTab renders the repos tab content (partial, no layout).
func (h *DashboardHandler) ReposTab(c *gin.Context) {
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

	c.HTML(http.StatusOK, "repos_tab", gin.H{
		"User":      u,
		"Repos":     repos,
		"ActiveTab": "repos",
	})
}

// PRsTab renders the unified cross-repo PR queue.
func (h *DashboardHandler) PRsTab(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, _ := h.client.User.Get(c.Request.Context(), int(userID))

	repos, err := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(c.Request.Context())
	if err != nil {
		c.HTML(http.StatusInternalServerError, "prs_tab", gin.H{"Error": "Failed to fetch repositories"})
		return
	}

	var allPRs []PRQueueItem
	for _, r := range repos {
		if r.PullRequests == "" {
			continue
		}
		var prs []prSummary
		if err := json.Unmarshal([]byte(r.PullRequests), &prs); err != nil {
			continue
		}
		for _, pr := range prs {
			allPRs = append(allPRs, PRQueueItem{
				RepoID:       r.ID,
				RepoFullName: r.FullName,
				Number:       pr.Number,
				Title:        pr.Title,
				Author:       pr.Author,
				CreatedAt:    pr.CreatedAt,
				HTMLURL:      pr.HTMLURL,
				HeadRef:      pr.HeadRef,
				BaseRef:      pr.BaseRef,
			})
		}
	}

	// Sort newest first
	sort.Slice(allPRs, func(i, j int) bool {
		return allPRs[i].CreatedAt > allPRs[j].CreatedAt
	})

	filterRepo := c.Query("repo")
	if filterRepo != "" {
		var filtered []PRQueueItem
		for _, pr := range allPRs {
			if strings.EqualFold(pr.RepoFullName, filterRepo) {
				filtered = append(filtered, pr)
			}
		}
		allPRs = filtered
	}

	c.HTML(http.StatusOK, "prs_tab", gin.H{
		"User":      u,
		"PRs":       allPRs,
		"Repos":     repos,
		"FilterRepo": filterRepo,
		"ActiveTab": "prs",
	})
}

type mergeRequest struct {
	RepoID   int `json:"repo_id" form:"repo_id"`
	PRNumber int `json:"pr_number" form:"pr_number"`
}

// MergeSinglePR merges a single PR from the unified queue.
func (h *DashboardHandler) MergeSinglePR(c *gin.Context) {
	userID := c.GetInt64("user_id")

	var req mergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid request")
		return
	}

	ctx := c.Request.Context()
	r, err := h.client.Repository.Query().
		Where(
			repository.ID(req.RepoID),
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

	merged, msg, err := h.gh.MergePullRequest(u.AccessToken, r.Owner, r.Name, req.PRNumber)
	if err != nil {
		log.Printf("Error merging PR #%d for %s: %v", req.PRNumber, r.FullName, err)
		c.String(http.StatusInternalServerError, "Failed to merge: %v", err)
		return
	}
	if !merged {
		c.String(http.StatusConflict, "Merge failed: %s", msg)
		return
	}

	h.syncer.SyncOne(ctx, r)
	c.String(http.StatusOK, "PR #%d merged successfully", req.PRNumber)
}

// BatchMergePRs merges selected PRs from the unified queue.
func (h *DashboardHandler) BatchMergePRs(c *gin.Context) {
	userID := c.GetInt64("user_id")
	ctx := c.Request.Context()

	prIDs := c.PostFormArray("pr_ids")
	if len(prIDs) == 0 {
		c.String(http.StatusBadRequest, "No PRs selected")
		return
	}

	u, err := h.client.User.Get(ctx, int(userID))
	if err != nil {
		c.String(http.StatusInternalServerError, "User not found")
		return
	}

	var merged []string
	var failed []string

	for _, id := range prIDs {
		parts := strings.SplitN(id, ":", 2)
		if len(parts) != 2 {
			failed = append(failed, id)
			continue
		}
		repoID, err := strconv.Atoi(parts[0])
		if err != nil {
			failed = append(failed, id)
			continue
		}
		prNumber, err := strconv.Atoi(parts[1])
		if err != nil {
			failed = append(failed, id)
			continue
		}

		r, err := h.client.Repository.Query().
			Where(
				repository.ID(repoID),
				repository.HasUserWith(user.ID(int(userID))),
			).
			Only(ctx)
		if err != nil {
			failed = append(failed, fmt.Sprintf("#%d (repo not found)", prNumber))
			continue
		}

		ok, msg, err := h.gh.MergePullRequest(u.AccessToken, r.Owner, r.Name, prNumber)
		if err != nil || !ok {
			reason := msg
			if err != nil {
				reason = err.Error()
			}
			failed = append(failed, fmt.Sprintf("#%d (%s)", prNumber, reason))
			continue
		}
		merged = append(merged, fmt.Sprintf("#%d", prNumber))
		h.syncer.SyncOne(ctx, r)
	}

	total := len(merged) + len(failed)
	if len(failed) == 0 {
		c.String(http.StatusOK, "All %d PR(s) merged successfully!", total)
	} else {
		c.String(http.StatusOK, "Merged %d/%d. Failed: %s", len(merged), total, strings.Join(failed, ", "))
	}
}

// MetricsTab renders the DORA metrics page.
func (h *DashboardHandler) MetricsTab(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, _ := h.client.User.Get(c.Request.Context(), int(userID))

	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(c.Request.Context())

	c.HTML(http.StatusOK, "metrics_tab", gin.H{
		"User":      u,
		"Repos":     repos,
		"Metrics":   computeMetrics(repos),
		"ActiveTab": "metrics",
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
