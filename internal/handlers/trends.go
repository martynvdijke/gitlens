package handlers

import (
	"net/http"
	"strconv"
	"time"

	"gitlens/ent"
	"gitlens/ent/metricsnapshot"
	"gitlens/ent/repository"
	"gitlens/ent/user"

	"github.com/gin-gonic/gin"
)

type TrendsHandler struct {
	client *ent.Client
}

func NewTrendsHandler(client *ent.Client) *TrendsHandler {
	return &TrendsHandler{client: client}
}

// snapshotJSON is the JSON representation of a single MetricSnapshot row.
type snapshotJSON struct {
	ID                   int     `json:"id"`
	RepoID               int     `json:"repo_id"`
	RepoName             string  `json:"repo_name"`
	Timestamp            string  `json:"timestamp"`
	FeatCount            int     `json:"feat_count"`
	FixCount             int     `json:"fix_count"`
	DocsCount            int     `json:"docs_count"`
	ChoreCount           int     `json:"chore_count"`
	OtherCommitCount     int     `json:"other_commit_count"`
	TotalCommitsFetched  int     `json:"total_commits_fetched"`
	ReleaseCount         int     `json:"release_count"`
	AvgLeadTimeHours     float64 `json:"avg_lead_time_hours"`
	WorkflowSuccessCount int     `json:"workflow_success_count"`
	WorkflowFailureCount int     `json:"workflow_failure_count"`
	WorkflowStatus       string  `json:"workflow_status"`
}

// MetricsHistoryResponse is the JSON response from /metrics/history.
type MetricsHistoryResponse struct {
	Snapshots []snapshotJSON `json:"snapshots"`
}

// MetricsHistory returns time-series snapshot data.
// GET /metrics/history?since=ISO8601&until=ISO8601&repo_id=int
func (h *TrendsHandler) MetricsHistory(c *gin.Context) {
	userID := c.GetInt64("user_id")

	sinceStr := c.Query("since")
	untilStr := c.Query("until")
	repoIDStr := c.Query("repo_id")

	// Parse date range filters
	var since, until time.Time
	if sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			// Try ISO 8601 without timezone
			t, err = time.Parse("2006-01-02T15:04:05", sinceStr)
			if err != nil {
				t, err = time.Parse("2006-01-02", sinceStr)
			}
		}
		if err == nil {
			since = t
		}
	}
	if untilStr != "" {
		t, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05", untilStr)
			if err != nil {
				t, err = time.Parse("2006-01-02", untilStr)
			}
		}
		if err == nil {
			until = t
		}
	}

	// Get user's repos for scoping
	repos, err := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		All(c.Request.Context())
	if err != nil || len(repos) == 0 {
		c.JSON(http.StatusOK, MetricsHistoryResponse{Snapshots: []snapshotJSON{}})
		return
	}

	// Build repo ID -> name map and filter set
	repoNames := make(map[int]string)
	repoIDs := make([]int, 0, len(repos))
	for _, r := range repos {
		repoNames[r.ID] = r.FullName
		repoIDs = append(repoIDs, r.ID)
	}

	// Build snapshot query
	query := h.client.MetricSnapshot.Query().
		Where(metricsnapshot.RepoIDIn(repoIDs...)).
		Order(ent.Asc(metricsnapshot.FieldTimestamp))

	// Apply optional repo_id filter
	if repoIDStr != "" {
		rid, err := strconv.Atoi(repoIDStr)
		if err == nil {
			if _, ok := repoNames[rid]; ok {
				query = query.Where(metricsnapshot.RepoID(rid))
			}
		}
	}

	// Apply date range filters
	if !since.IsZero() {
		query = query.Where(metricsnapshot.TimestampGTE(since))
	}
	if !until.IsZero() {
		query = query.Where(metricsnapshot.TimestampLTE(until))
	}

	// Limit to 2000 rows for performance
	snapshots, err := query.Limit(2000).All(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query snapshots"})
		return
	}

	result := make([]snapshotJSON, 0, len(snapshots))
	for _, s := range snapshots {
		result = append(result, snapshotJSON{
			ID:                   s.ID,
			RepoID:               s.RepoID,
			RepoName:             repoNames[s.RepoID],
			Timestamp:            s.Timestamp.Format(time.RFC3339),
			FeatCount:            s.FeatCount,
			FixCount:             s.FixCount,
			DocsCount:            s.DocsCount,
			ChoreCount:           s.ChoreCount,
			OtherCommitCount:     s.OtherCommitCount,
			TotalCommitsFetched:  s.TotalCommitsFetched,
			ReleaseCount:         s.ReleaseCount,
			AvgLeadTimeHours:     s.AvgLeadTimeHours,
			WorkflowSuccessCount: s.WorkflowSuccessCount,
			WorkflowFailureCount: s.WorkflowFailureCount,
			WorkflowStatus:       s.WorkflowStatus,
		})
	}

	c.JSON(http.StatusOK, MetricsHistoryResponse{Snapshots: result})
}

// TrendsPage renders the trends dashboard HTML partial.
// GET /trends
func (h *TrendsHandler) TrendsPage(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, _ := h.client.User.Get(c.Request.Context(), int(userID))

	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(ent.Asc(repository.FieldFullName)).
		All(c.Request.Context())

	c.HTML(http.StatusOK, "trends_tab", gin.H{
		"User":      u,
		"Repos":     repos,
		"ActiveTab": "trends",
	})
}
