package handlers

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	"gitlens/ent"
	"gitlens/ent/commitactivity"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	appsync "gitlens/internal/sync"

	"github.com/gin-gonic/gin"
)

// YearOverviewHandler serves the Year Overview tab and stats JSON.
type YearOverviewHandler struct {
	client *ent.Client
	syncer *appsync.Syncer
}

func NewYearOverviewHandler(client *ent.Client, syncer *appsync.Syncer) *YearOverviewHandler {
	return &YearOverviewHandler{client: client, syncer: syncer}
}

// BackfillInfo summarizes commit-activity backfill progress across a
// set of repos. Status is the aggregate: running > pending > error > done.
type BackfillInfo struct {
	Status  string `json:"status"`
	Pending int    `json:"pending"`
	Running int    `json:"running"`
	Done    int    `json:"done"`
	Error   int    `json:"error"`
}

// YearStatsResponse is the JSON payload returned by /year-overview/stats.
type YearStatsResponse struct {
	Year           int            `json:"year"`
	TotalCommits   int            `json:"total_commits"`
	ActiveDays     int            `json:"active_days"`
	MostActiveDay  *DayStat       `json:"most_active_day"`
	LongestStreak  int            `json:"longest_streak"`
	CurrentStreak  int            `json:"current_streak"`
	BusiestWeekday string         `json:"busiest_weekday"`
	MonthlyTotals  []int          `json:"monthly_totals"`
	DailyCounts    map[string]int `json:"daily_counts"`
	TopRepos       []RepoStat     `json:"top_repos"`
	RepoID         int            `json:"repo_id,omitempty"`
	RepoName       string         `json:"repo_name,omitempty"`
	Backfill       *BackfillInfo  `json:"backfill,omitempty"`
}

// DayStat represents a single day's commit count.
type DayStat struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// RepoStat represents a repo's total commits for the year.
type RepoStat struct {
	RepoID   int    `json:"repo_id"`
	FullName string `json:"full_name"`
	Commits  int    `json:"commits"`
}

// weekdayNames maps Go's time.Weekday to short English names.
var weekdayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// YearOverview renders the Year Overview tab HTML partial.
// GET /year-overview
func (h *YearOverviewHandler) YearOverview(c *gin.Context) {
	userID := c.GetInt64("user_id")
	u, _ := h.client.User.Get(c.Request.Context(), int(userID))

	now := time.Now().UTC()
	currentYear := now.Year()

	// Build year list (last 10 years)
	years := make([]int, 0, 10)
	for y := currentYear; y > currentYear-10 && y >= 2020; y-- {
		years = append(years, y)
	}

	repos, _ := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		Order(ent.Asc(repository.FieldFullName)).
		All(c.Request.Context())

	c.HTML(http.StatusOK, "year_overview_tab", gin.H{
		"User":        u,
		"Repos":       repos,
		"Years":       years,
		"DefaultYear": currentYear,
		"ActiveTab":   "year",
	})
}

// Stats returns year-level commit statistics as JSON.
// GET /year-overview/stats?year=2025&repo_id=optional
func (h *YearOverviewHandler) Stats(c *gin.Context) {
	userID := c.GetInt64("user_id")
	ctx := c.Request.Context()

	yearStr := c.DefaultQuery("year", strconv.Itoa(time.Now().UTC().Year()))
	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2020 || year > 2100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid year"})
		return
	}

	repoIDStr := c.Query("repo_id")

	repos, err := h.userRepos(ctx, int(userID), repoIDStr)
	if err != nil || len(repos) == 0 {
		c.JSON(http.StatusOK, YearStatsResponse{Year: year})
		return
	}

	// Kick off (or resume) backfills for repos whose history is not yet
	// complete. The UI polls /year-overview/backfill-status meanwhile.
	if h.syncer != nil {
		for _, r := range repos {
			if r.BackfillStatus == "pending" || r.BackfillStatus == "running" {
				h.syncer.MaybeStartBackfill(r.ID)
			}
		}
	}

	repoIDs := make([]int, len(repos))
	repoNameByID := make(map[int]string, len(repos))
	for i, r := range repos {
		repoIDs[i] = r.ID
		repoNameByID[r.ID] = r.FullName
	}

	// Date range for the selected year
	startDate := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC)

	// Query all activity rows for the year, scoped to user's repos
	activities, err := h.client.CommitActivity.Query().
		Where(
			commitactivity.RepoIDIn(repoIDs...),
			commitactivity.DateGTE(startDate),
			commitactivity.DateLTE(endDate),
		).
		All(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	// Compute stats
	stats := computeYearStats(activities, repoNameByID, year, startDate, endDate)

	// If single repo filter, include repo name
	if len(repos) == 1 {
		stats.RepoID = repos[0].ID
		stats.RepoName = repos[0].FullName
	}

	stats.Backfill = aggregateBackfillInfo(repos)

	c.JSON(http.StatusOK, stats)
}

// userRepos returns the authenticated user's repos, optionally filtered
// by a repo_id query-string value ("" = all repos).
func (h *YearOverviewHandler) userRepos(ctx context.Context, userID int, repoIDStr string) ([]*ent.Repository, error) {
	repoQuery := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(userID)))

	if repoIDStr != "" {
		repoID, err := strconv.Atoi(repoIDStr)
		if err != nil {
			return nil, err
		}
		repoQuery = repoQuery.Where(repository.ID(repoID))
	}
	return repoQuery.All(ctx)
}

// aggregateBackfillInfo counts per-repo backfill states and derives an
// aggregate status (running > pending > error > done).
func aggregateBackfillInfo(repos []*ent.Repository) *BackfillInfo {
	info := &BackfillInfo{Status: "done"}
	for _, r := range repos {
		switch r.BackfillStatus {
		case "running":
			info.Running++
		case "pending":
			info.Pending++
		case "error":
			info.Error++
		default:
			info.Done++
		}
	}
	switch {
	case info.Running > 0:
		info.Status = "running"
	case info.Pending > 0:
		info.Status = "pending"
	case info.Error > 0:
		info.Status = "error"
	}
	return info
}

// BackfillStatus returns aggregate backfill progress as JSON.
// GET /year-overview/backfill-status?repo_id=optional
func (h *YearOverviewHandler) BackfillStatus(c *gin.Context) {
	userID := c.GetInt64("user_id")
	ctx := c.Request.Context()

	repos, err := h.userRepos(ctx, int(userID), c.Query("repo_id"))
	if err != nil || len(repos) == 0 {
		c.JSON(http.StatusOK, &BackfillInfo{Status: "done"})
		return
	}
	c.JSON(http.StatusOK, aggregateBackfillInfo(repos))
}

// Refresh forces a full history recount for the user's repos: status is
// reset to pending with the cursor cleared, then the backfill restarts
// in the background.
// POST /year-overview/refresh?repo_id=optional
func (h *YearOverviewHandler) Refresh(c *gin.Context) {
	userID := c.GetInt64("user_id")
	ctx := c.Request.Context()

	repos, err := h.userRepos(ctx, int(userID), c.Query("repo_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repo_id"})
		return
	}

	now := time.Now()
	refreshed := 0
	for _, r := range repos {
		_, err := h.client.Repository.UpdateOneID(r.ID).
			SetBackfillStatus("pending").
			SetBackfillCursorPage(0).
			SetBackfillError("").
			SetBackfillUpdatedAt(now).
			Save(ctx)
		if err != nil {
			continue
		}
		refreshed++
		if h.syncer != nil {
			h.syncer.MaybeStartBackfill(r.ID)
		}
	}
	c.JSON(http.StatusOK, gin.H{"status": "pending", "repos": refreshed})
}

// computeYearStats computes all year-in-review stats from a flat list of CommitActivity rows.
func computeYearStats(activities []*ent.CommitActivity, repoNameByID map[int]string, year int, startDate, endDate time.Time) YearStatsResponse {
	resp := YearStatsResponse{
		Year:          year,
		MonthlyTotals: make([]int, 12),
		DailyCounts:   make(map[string]int),
	}

	if len(activities) == 0 {
		return resp
	}

	// Aggregate
	repoCommits := make(map[int]int) // repo_id -> total
	var totalCommits int
	dayMap := make(map[string]int) // "2006-01-02" -> count
	weekdaySums := make([]int, 7)  // Sunday=0, Monday=1, ...

	var maxDayCount int
	var maxDayDate string

	for _, a := range activities {
		count := a.CommitCount
		totalCommits += count

		dayKey := a.Date.UTC().Format("2006-01-02")
		dayMap[dayKey] += count
		weekdaySums[a.Date.UTC().Weekday()] += count

		monthIdx := a.Date.UTC().Month() - 1 // 0-indexed
		resp.MonthlyTotals[monthIdx] += count

		repoCommits[a.RepoID] += count

		// Track most active day
		dayCount := dayMap[dayKey]
		if dayCount > maxDayCount {
			maxDayCount = dayCount
			maxDayDate = dayKey
		}
	}

	resp.TotalCommits = totalCommits
	resp.ActiveDays = len(dayMap)

	if maxDayDate != "" {
		resp.MostActiveDay = &DayStat{Date: maxDayDate, Count: maxDayCount}
	}

	// Busiest weekday (by total commits)
	busiestIdx := 0
	for i := 1; i < 7; i++ {
		if weekdaySums[i] > weekdaySums[busiestIdx] {
			busiestIdx = i
		}
	}
	resp.BusiestWeekday = weekdayNames[busiestIdx]

	// Longest streak
	resp.LongestStreak = computeLongestStreak(dayMap)
	resp.CurrentStreak = computeCurrentStreak(dayMap, year, startDate, endDate)

	// Daily counts (sparse map for heatmap)
	resp.DailyCounts = dayMap

	// Top repos (sorted by commit count, top 10)
	topRepos := make([]RepoStat, 0, len(repoCommits))
	for rid, count := range repoCommits {
		topRepos = append(topRepos, RepoStat{
			RepoID:   rid,
			FullName: repoNameByID[rid],
			Commits:  count,
		})
	}
	sort.Slice(topRepos, func(i, j int) bool {
		return topRepos[i].Commits > topRepos[j].Commits
	})
	if len(topRepos) > 10 {
		topRepos = topRepos[:10]
	}
	resp.TopRepos = topRepos

	return resp
}

// computeLongestStreak returns the longest consecutive calendar-day streak
// where commit_count > 0.
func computeLongestStreak(dayMap map[string]int) int {
	if len(dayMap) == 0 {
		return 0
	}

	// Sort active days
	days := make([]string, 0, len(dayMap))
	for d := range dayMap {
		days = append(days, d)
	}
	sort.Strings(days)

	longest := 1
	current := 1
	for i := 1; i < len(days); i++ {
		prev, _ := time.Parse("2006-01-02", days[i-1])
		curr, _ := time.Parse("2006-01-02", days[i])
		if curr.Sub(prev).Hours() == 24 {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 1
		}
	}
	return longest
}

// computeCurrentStreak returns the trailing streak ending on the last day
// of the year (or today if the year is the current year).
func computeCurrentStreak(dayMap map[string]int, year int, startDate, endDate time.Time) int {
	now := time.Now().UTC()

	var lastCheck time.Time
	if year == now.Year() {
		lastCheck = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	} else {
		lastCheck = endDate
	}

	// Walk backwards from lastCheck, checking each day
	streak := 0
	cursor := lastCheck
	// Max 366 iterations (leap year)
	for i := 0; i < 366; i++ {
		dayKey := cursor.Format("2006-01-02")
		if _, ok := dayMap[dayKey]; ok {
			streak++
		} else {
			break
		}
		cursor = cursor.AddDate(0, 0, -1)
	}
	return streak
}
