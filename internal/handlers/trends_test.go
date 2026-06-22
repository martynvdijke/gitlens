package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlens/ent"
	"gitlens/ent/enttest"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newTestTrendsHandler(t *testing.T) (*TrendsHandler, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	h := NewTrendsHandler(client)
	return h, client
}

func seedSnapshotData(t *testing.T, client *ent.Client, userID int) (*ent.Repository, *ent.Repository) {
	t.Helper()
	ctx := context.Background()

	repo1, err := client.Repository.Create().
		SetGithubID(100).SetOwner("org").SetName("repo-a").
		SetFullName("org/repo-a").SetHTMLURL("https://github.com/org/repo-a").
		SetDefaultBranch("main").SetUserID(userID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo1: %v", err)
	}

	repo2, err := client.Repository.Create().
		SetGithubID(101).SetOwner("org").SetName("repo-b").
		SetFullName("org/repo-b").SetHTMLURL("https://github.com/org/repo-b").
		SetDefaultBranch("main").SetUserID(userID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo2: %v", err)
	}

	// Snapshots for repo1 at different times
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		ts := now.Add(time.Duration(i*24) * time.Hour)
		_, err := client.MetricSnapshot.Create().
			SetRepoID(repo1.ID).
			SetTimestamp(ts).
			SetFeatCount(2 + i).
			SetFixCount(1).
			SetDocsCount(0).
			SetChoreCount(1).
			SetOtherCommitCount(0).
			SetTotalCommitsFetched(4 + i).
			SetReleaseCount(1).
			SetAvgLeadTimeHours(12.5).
			SetWorkflowSuccessCount(3).
			SetWorkflowFailureCount(1).
			SetWorkflowStatus("success").
			Save(ctx)
		if err != nil {
			t.Fatalf("create snapshot repo1[%d]: %v", i, err)
		}
	}

	// Single snapshot for repo2
	_, err = client.MetricSnapshot.Create().
		SetRepoID(repo2.ID).
		SetTimestamp(now.Add(48 * time.Hour)).
		SetFeatCount(1).
		SetFixCount(2).
		SetDocsCount(1).
		SetChoreCount(0).
		SetOtherCommitCount(1).
		SetTotalCommitsFetched(5).
		SetReleaseCount(2).
		SetAvgLeadTimeHours(24.0).
		SetWorkflowSuccessCount(5).
		SetWorkflowFailureCount(2).
		SetWorkflowStatus("failure").
		Save(ctx)
	if err != nil {
		t.Fatalf("create snapshot repo2: %v", err)
	}

	return repo1, repo2
}

func serveMetricsHistoryRequest(handler gin.HandlerFunc, path string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.GET("/metrics/history", handler)
	req := httptest.NewRequest("GET", path, nil)
	engine.ServeHTTP(w, req)
	return w
}

func TestMetricsHistory_ReturnsAllSnapshots(t *testing.T) {
	handler, client := newTestTrendsHandler(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(100).SetLogin("testuser").SetAccessToken("tok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	seedSnapshotData(t, client, u.ID)

	h := func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.MetricsHistory(c)
	}

	w := serveMetricsHistoryRequest(h, "/metrics/history")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp MetricsHistoryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	// 3 from repo1 + 1 from repo2
	if len(resp.Snapshots) != 4 {
		t.Fatalf("expected 4 snapshots, got %d", len(resp.Snapshots))
	}

	// Verify first snapshot fields
	snap := resp.Snapshots[0]
	if snap.RepoName == "" {
		t.Error("expected repo_name to be set")
	}
	if snap.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
	if snap.FeatCount == 0 && snap.FixCount == 0 {
		t.Error("expected non-zero metric fields")
	}
}

func TestMetricsHistory_FilterByRepoID(t *testing.T) {
	handler, client := newTestTrendsHandler(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(200).SetLogin("filteruser").SetAccessToken("tok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo1, _ := seedSnapshotData(t, client, u.ID)

	h := func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.MetricsHistory(c)
	}

	w := serveMetricsHistoryRequest(h, "/metrics/history?repo_id="+itoa(repo1.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp MetricsHistoryResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Only repo1's 3 snapshots
	if len(resp.Snapshots) != 3 {
		t.Fatalf("expected 3 snapshots for repo filter, got %d", len(resp.Snapshots))
	}
	for _, s := range resp.Snapshots {
		if s.RepoID != repo1.ID {
			t.Errorf("expected repo_id %d, got %d", repo1.ID, s.RepoID)
		}
	}
}

func TestMetricsHistory_FilterByDateRange(t *testing.T) {
	handler, client := newTestTrendsHandler(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(300).SetLogin("daterange").SetAccessToken("tok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	seedSnapshotData(t, client, u.ID)

	h := func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.MetricsHistory(c)
	}

	// Only snapshots within June 2 (1 snapshot: repo1 day2)
	w := serveMetricsHistoryRequest(h, "/metrics/history?since=2024-06-02T00:00:00Z&until=2024-06-02T23:59:59Z")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp MetricsHistoryResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Snapshots) != 1 {
		t.Fatalf("expected 1 snapshot in date range, got %d", len(resp.Snapshots))
	}
}

func TestMetricsHistory_NoSnapshots(t *testing.T) {
	handler, client := newTestTrendsHandler(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(400).SetLogin("nosnaps").SetAccessToken("tok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Create a repo but no snapshots
	_, err = client.Repository.Create().
		SetGithubID(200).SetOwner("u").SetName("empty").
		SetFullName("u/empty").SetHTMLURL("https://github.com/u/empty").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	h := func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.MetricsHistory(c)
	}

	w := serveMetricsHistoryRequest(h, "/metrics/history")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp MetricsHistoryResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Snapshots) != 0 {
		t.Errorf("expected 0 snapshots, got %d", len(resp.Snapshots))
	}
}

func TestMetricsHistory_NoRepos(t *testing.T) {
	handler, client := newTestTrendsHandler(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(500).SetLogin("norepos").SetAccessToken("tok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	h := func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.MetricsHistory(c)
	}

	w := serveMetricsHistoryRequest(h, "/metrics/history")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp MetricsHistoryResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Snapshots) != 0 {
		t.Errorf("expected 0 snapshots for user with no repos, got %d", len(resp.Snapshots))
	}
}

func TestMetricsHistory_SnapshotsAreOrderedByTimestamp(t *testing.T) {
	handler, client := newTestTrendsHandler(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(600).SetLogin("ordered").SetAccessToken("tok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo, err := client.Repository.Create().
		SetGithubID(300).SetOwner("u").SetName("ordered").
		SetFullName("u/ordered").SetHTMLURL("https://github.com/u/ordered").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	// Insert snapshots out of order (day 3, day 1, day 2)
	now := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	for _, offset := range []int{48, 0, 24} { // hour offsets
		_, err := client.MetricSnapshot.Create().
			SetRepoID(repo.ID).
			SetTimestamp(now.Add(time.Duration(offset) * time.Hour)).
			SetTotalCommitsFetched(1).
			Save(ctx)
		if err != nil {
			t.Fatalf("create snapshot: %v", err)
		}
	}

	h := func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.MetricsHistory(c)
	}

	w := serveMetricsHistoryRequest(h, "/metrics/history")
	var resp MetricsHistoryResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	if len(resp.Snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(resp.Snapshots))
	}

	// Verify ascending order by timestamp
	for i := 1; i < len(resp.Snapshots); i++ {
		t1, _ := time.Parse(time.RFC3339, resp.Snapshots[i-1].Timestamp)
		t2, _ := time.Parse(time.RFC3339, resp.Snapshots[i].Timestamp)
		if !t1.Before(t2) && !t1.Equal(t2) {
			t.Errorf("snapshots not in ascending order at index %d: %s >= %s", i, t1, t2)
		}
	}
}

// itoa is a helper to convert int to string for query params.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
