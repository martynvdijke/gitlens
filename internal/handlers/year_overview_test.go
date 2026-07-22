package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gitlens/ent"
	"gitlens/ent/enttest"
	ghclient "gitlens/internal/github"
	"gitlens/internal/provider"
	"gitlens/internal/sync"
	"gitlens/internal/ws"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newYearOverviewTestHandler(t *testing.T, providers map[string]provider.Provider) (*YearOverviewHandler, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	ghClient := ghclient.NewClient("", "")
	syncer := sync.NewSyncer(client, ghClient, providers, ws.NewHub())
	return NewYearOverviewHandler(client, syncer), client
}

func newYearOverviewEngine(h *YearOverviewHandler, userID int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	authed := engine.Group("/", func(c *gin.Context) { c.Set("user_id", userID); c.Next() })
	authed.GET("/year-overview/stats", h.Stats)
	authed.GET("/year-overview/backfill-status", h.BackfillStatus)
	authed.POST("/year-overview/refresh", h.Refresh)
	return engine
}

func createYearRepo(t *testing.T, client *ent.Client, userID int, name, backfillStatus string) *ent.Repository {
	t.Helper()
	ctx := context.Background()
	q := client.Repository.Create().
		SetGithubID(int64(userID*1000 + len(name))).SetOwner("u").SetName(name).
		SetFullName("u/" + name).SetHTMLURL("https://example.com/u/" + name).
		SetDefaultBranch("main").SetUserID(userID).
		SetBackfillStatus(backfillStatus).
		SetBackfillUpdatedAt(time.Now())
	repo, err := q.Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	return repo
}

func doJSON(engine *gin.Engine, method, path string) (int, map[string]any) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	engine.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestStats_IncludesBackfillInfo(t *testing.T) {
	h, client := newYearOverviewTestHandler(t, map[string]provider.Provider{})
	u := createMergeUser(t, client)
	createYearRepo(t, client, u.ID, "yr-run", "running")
	engine := newYearOverviewEngine(h, int64(u.ID))

	code, body := doJSON(engine, "GET", "/year-overview/stats?year=2025")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	bf, ok := body["backfill"].(map[string]any)
	if !ok {
		t.Fatalf("expected backfill info in stats, got: %v", body)
	}
	if bf["status"] != "running" {
		t.Errorf("expected backfill status running, got: %v", bf["status"])
	}
	if bf["running"].(float64) != 1 {
		t.Errorf("expected running count 1, got: %v", bf["running"])
	}
}

func TestBackfillStatus_Aggregates(t *testing.T) {
	h, client := newYearOverviewTestHandler(t, map[string]provider.Provider{})
	u := createMergeUser(t, client)
	createYearRepo(t, client, u.ID, "yr-done", "done")
	createYearRepo(t, client, u.ID, "yr-err", "error")
	engine := newYearOverviewEngine(h, int64(u.ID))

	code, body := doJSON(engine, "GET", "/year-overview/backfill-status")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["status"] != "error" {
		t.Errorf("expected aggregate status error, got: %v", body["status"])
	}

	// Single-repo filter reports just that repo.
	r := createYearRepo(t, client, u.ID, "yr-done2", "done")
	_, body = doJSON(engine, "GET", "/year-overview/backfill-status?repo_id="+strconv.Itoa(r.ID))
	if body["status"] != "done" {
		t.Errorf("expected repo-scoped status done, got: %v", body["status"])
	}
}

func TestRefresh_ResetsAndBackfills(t *testing.T) {
	// Fake provider with no history: backfill completes immediately.
	fake := &mergeFakeProvider{name: "fake", mergeFn: func(n int) (bool, string, error) {
		return false, "", nil
	}}
	h, client := newYearOverviewTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	repo := createYearRepo(t, client, u.ID, "yr-refresh", "done")
	if _, err := client.Repository.UpdateOne(repo).SetProvider("fake").Save(context.Background()); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	engine := newYearOverviewEngine(h, int64(u.ID))

	code, body := doJSON(engine, "POST", "/year-overview/refresh")
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["status"] != "pending" || body["repos"].(float64) != 1 {
		t.Errorf("unexpected refresh response: %v", body)
	}

	// Background backfill runs with the fake provider and completes.
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, err := client.Repository.Get(context.Background(), repo.ID)
		if err != nil {
			t.Fatalf("get repo: %v", err)
		}
		if r.BackfillStatus == "done" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("backfill did not complete; status=%q err=%q", r.BackfillStatus, r.BackfillError)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
