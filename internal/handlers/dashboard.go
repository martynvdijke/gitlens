package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitlens/ent"
	"gitlens/ent/event"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	"gitlens/internal/github"
	mw "gitlens/internal/middleware"
	"gitlens/internal/sync"

	"github.com/gin-gonic/gin"
)

type DORAMetrics struct {
	TotalRepos        int     `json:"totalRepos"`
	TotalReleases     int     `json:"totalReleases"`
	TotalCommits      int     `json:"totalCommits"`
	FeatCount         int     `json:"featCount"`
	FixCount          int     `json:"fixCount"`
	DocsCount         int     `json:"docsCount"`
	ChoreCount        int     `json:"choreCount"`
	OtherCount        int     `json:"otherCount"`
	WorkflowSuccesses int     `json:"workflowSuccesses"`
	WorkflowFailures  int     `json:"workflowFailures"`
	WorkflowPassRate  float64 `json:"workflowPassRate"`
	AvgLeadTimeHours  float64 `json:"avgLeadTimeHours"`
	FeatPct           float64 `json:"featPct"`
	FixPct            float64 `json:"fixPct"`
	DocsPct           float64 `json:"docsPct"`
	ChorePct          float64 `json:"chorePct"`
	ReleasesPerRepo   float64 `json:"releasesPerRepo"`
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
	sort := c.Query("sort")
	q := c.Query("q")

	order, needsPostSort := parseSortParam(sort)
	query := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(order)
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
	if needsPostSort {
		sortByBuildStatus(repos)
	}
	c.HTML(http.StatusOK, "repo_list", gin.H{"Repos": repos, "Query": q, "Sort": sort})
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

// sortByBuildStatus orders repos by workflow severity: failures first, then unknown, then success, then no workflows.
func sortByBuildStatus(repos []*ent.Repository) {
	statusRank := map[string]int{
		"failure": 0,
		"unknown": 1,
		"success": 2,
		"":        3,
	}
	sort.SliceStable(repos, func(i, j int) bool {
		ri := statusRank[repos[i].WorkflowStatus]
		rj := statusRank[repos[j].WorkflowStatus]
		if ri != rj {
			return ri < rj
		}
		return strings.ToLower(repos[i].FullName) < strings.ToLower(repos[j].FullName)
	})
}

// parseSortParam maps a sort query parameter to an ent ordering function.
// Returns (order, needsPostSort) where needsPostSort indicates the repos
// should be re-sorted in Go after fetching (used for custom multi-tier sorts).
func parseSortParam(sort string) (repository.OrderOption, bool) {
	switch sort {
	case "build_status":
		return ent.Desc(repository.FieldUpdatedAt), true
	case "latest_commit":
		return ent.Desc(repository.FieldLatestCommitDate), false
	case "latest_release":
		return ent.Desc(repository.FieldLatestReleaseDate), false
	case "pass_rate":
		return ent.Asc(repository.FieldWorkflowFailureCount), false
	case "name_asc":
		return ent.Asc(repository.FieldFullName), false
	case "name_desc":
		return ent.Desc(repository.FieldFullName), false
	case "synced_at":
		return ent.Desc(repository.FieldSyncedAt), false
	default:
		return ent.Desc(repository.FieldUpdatedAt), false
	}
}

// Index renders the full page with repos tab as default.
// Repo list is lazy-loaded via htmx so the page (including footer) renders instantly.
func (h *DashboardHandler) Index(c *gin.Context) {
	var u *ent.User

	sessionID, err := c.Cookie("gitlens_session")
	if err == nil {
		userID, ok := h.store.Get(sessionID)
		if ok {
			u, _ = h.client.User.Get(c.Request.Context(), int(userID))
		}
	}

	if u != nil {
		// Check if user needs setup (no tracked repos yet)
		count, countErr := h.client.Repository.Query().
			Where(repository.HasUserWith(user.ID(u.ID))).
			Count(c.Request.Context())

		if countErr == nil && count == 0 {
			ghRepos, err := h.gh.ListRepositories(u.AccessToken)
			if err == nil {
				available := make([]*github.Repository, len(ghRepos))
				for i, r := range ghRepos {
					available[i] = &github.Repository{
						ID:            r.ID,
						Owner:         r.Owner,
						Name:          r.Name,
						FullName:      r.FullName,
						Description:   r.Description,
						HTMLURL:       r.HTMLURL,
						Language:      r.Language,
						DefaultBranch: r.DefaultBranch,
					}
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

		// Normal logged-in view — repos lazy-loaded via htmx on the client
		c.HTML(http.StatusOK, "index.html", gin.H{
			"User":      u,
			"Repos":     []*ent.Repository{},
			"Metrics":   computeMetrics(nil),
			"ActiveTab": "repos",
		})
		return
	}

	c.HTML(http.StatusOK, "index.html", gin.H{
		"User":              nil,
		"Repos":             nil,
		"Metrics":           computeMetrics(nil),
		"ActiveTab":         "repos",
		"ForgejoDefaultURL": os.Getenv("FORGEJO_DEFAULT_URL"),
	})
}

type timelineGroup struct {
	Date   string
	Events []feedEvent
}

// groupEventsByDate groups feed events by date label (Today, Yesterday, or "Jan 2").
func groupEventsByDate(events []feedEvent) []timelineGroup {
	if len(events) == 0 {
		return nil
	}
	today := time.Now().Truncate(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)

	var groups []timelineGroup
	var current *timelineGroup

	for _, e := range events {
		eventDate := e.Timestamp.Truncate(24 * time.Hour)
		label := e.Timestamp.Format("Jan 2")
		switch {
		case eventDate.Equal(today):
			label = "Today"
		case eventDate.Equal(yesterday):
			label = "Yesterday"
		}
		if current == nil || current.Date != label {
			if current != nil {
				groups = append(groups, *current)
			}
			current = &timelineGroup{Date: label}
		}
		current.Events = append(current.Events, e)
	}
	if current != nil {
		groups = append(groups, *current)
	}
	return groups
}

// queryHomepageTimeline fetches recent cross-repo events for the homepage timeline.
// Returns the last 7 days of releases, workflow failures, and PR merges, capped at 20.
// Only events for repos belonging to the requesting user are returned.
func (h *DashboardHandler) queryHomepageTimeline(c *gin.Context) []feedEvent {
	userID := c.GetInt64("user_id")
	sinceTime := time.Now().Add(-7 * 24 * time.Hour)

	// Get user's repos for scoping events and name resolution
	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		All(c.Request.Context())
	if len(repos) == 0 {
		return nil
	}

	repoIDSet := make(map[int]struct{ owner, name, full string })
	repoIDs := make([]int, 0, len(repos))
	for _, r := range repos {
		repoIDSet[r.ID] = struct{ owner, name, full string }{r.Owner, r.Name, r.FullName}
		repoIDs = append(repoIDs, r.ID)
	}

	dbEvents, err := h.client.Event.Query().
		Where(
			event.TimestampGTE(sinceTime),
			event.TypeIn(event.TypeRelease, event.TypeWorkflowFailure, event.TypePrMerge),
			event.RepoIDIn(repoIDs...),
		).
		Order(ent.Desc(event.FieldTimestamp)).
		Limit(20).
		All(c.Request.Context())
	if err != nil || len(dbEvents) == 0 {
		return nil
	}

	events := make([]feedEvent, 0, len(dbEvents))
	for _, e := range dbEvents {
		rName, ok := repoIDSet[e.RepoID]
		if !ok {
			continue
		}
		events = append(events, feedEvent{
			ID:        e.ID,
			Type:      string(e.Type),
			Title:     e.Title,
			URL:       e.URL,
			Metadata:  e.Metadata,
			Timestamp: e.Timestamp,
			RepoOwner: rName.owner,
			RepoName:  rName.name,
			RepoFull:  rName.full,
		})
	}
	return events
}

// ReposTab renders the repos tab content (partial, no layout).
func (h *DashboardHandler) ReposTab(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, _ := h.client.User.Get(c.Request.Context(), int(userID))

	sort := c.Query("sort")
	q := c.Query("q")
	order, needsPostSort := parseSortParam(sort)
	query := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(order)
	if q != "" {
		query = query.Where(
			repository.Or(
				repository.FullNameContainsFold(q),
				repository.NameContainsFold(q),
			),
		)
	}
	repos, _ := query.All(c.Request.Context())
	if needsPostSort {
		sortByBuildStatus(repos)
	}

	// Fetch timeline events for the homepage
	events := h.queryHomepageTimeline(c)
	timelineGroups := groupEventsByDate(events)

	c.HTML(http.StatusOK, "repos_tab", gin.H{
		"User":           u,
		"Repos":          repos,
		"ActiveTab":      "repos",
		"Sort":           sort,
		"TimelineGroups": timelineGroups,
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
		"User":       u,
		"PRs":        allPRs,
		"Repos":      repos,
		"FilterRepo": filterRepo,
		"ActiveTab":  "prs",
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

func (h *DashboardHandler) RenovateRebaseAll(c *gin.Context) {
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

	err = h.gh.RebaseAllOpenPRs(u.AccessToken, r.Owner, r.Name)
	if err != nil {
		log.Printf("Error triggering Renovate rebase-all on %s: %v", r.FullName, err)
		c.String(http.StatusInternalServerError, "Failed to trigger Renovate rebase: %v", err)
		return
	}

	c.String(http.StatusOK, "Renovate rebase-all triggered successfully for %s!", r.FullName)
}
