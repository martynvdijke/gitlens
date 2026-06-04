package handlers

import (
	"net/http"
	"time"

	"gitlens/ent"
	"gitlens/ent/repository"
	"gitlens/ent/user"

	"github.com/gin-gonic/gin"
)

type ChartHandler struct {
	client *ent.Client
}

func NewChartHandler(client *ent.Client) *ChartHandler {
	return &ChartHandler{client: client}
}

func parseSince(since string) time.Time {
	switch since {
	case "24h":
		return time.Now().Add(-24 * time.Hour)
	case "7d":
		return time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	case "90d":
		return time.Now().Add(-90 * 24 * time.Hour)
	default:
		return time.Time{} // zero means no filter
	}
}

// DORAChartData is the JSON payload returned by the /charts/data endpoint.
type DORAChartData struct {
	Metrics *DORAMetrics           `json:"metrics"`
	Repos   []RepoChartData        `json:"repos"`
}

type RepoChartData struct {
	FullName           string  `json:"fullName"`
	TotalCommitsFetched int     `json:"totalCommitsFetched"`
	WorkflowStatus     string  `json:"workflowStatus"`
	ReleaseCount       int     `json:"releaseCount"`
	AvgLeadTimeHours   float64 `json:"avgLeadTimeHours"`
}

// Data returns chart data as JSON, optionally filtered by a since parameter.
func (h *ChartHandler) Data(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, err := h.client.User.Get(c.Request.Context(), int(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User not found"})
		return
	}

	since := parseSince(c.Query("since"))

	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(u.ID))).
		Order(ent.Desc(repository.FieldUpdatedAt)).
		All(c.Request.Context())

	// Optionally filter repos that have data newer than since
	var filtered []*ent.Repository
	if since.IsZero() {
		filtered = repos
	} else {
		for _, r := range repos {
			if !r.UpdatedAt.IsZero() && r.UpdatedAt.After(since) {
				filtered = append(filtered, r)
			}
		}
	}
	if filtered == nil {
		filtered = repos
	}

	metrics := computeMetrics(filtered)

	var repoData []RepoChartData
	for _, r := range filtered {
		repoData = append(repoData, RepoChartData{
			FullName:            r.FullName,
			TotalCommitsFetched: r.TotalCommitsFetched,
			WorkflowStatus:      r.WorkflowStatus,
			ReleaseCount:        r.ReleaseCount,
			AvgLeadTimeHours:    r.AvgLeadTimeHours,
		})
	}

	c.JSON(http.StatusOK, DORAChartData{
		Metrics: metrics,
		Repos:   repoData,
	})
}


