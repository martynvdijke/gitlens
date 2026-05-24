package handlers

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlens/ent"
	"gitlens/ent/enttest"
	"gitlens/ent/event"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newTestFeedHandler(t *testing.T) (*FeedHandler, *ent.Client, int) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")

	u, err := client.User.Create().
		SetGithubID(100).
		SetLogin("testuser").
		SetAccessToken("token").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return NewFeedHandler(client), client, u.ID
}

func serveFeedRequest(handler gin.HandlerFunc, method, path string, userID int) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("user_id", int64(userID))
	})
	engine.SetHTMLTemplate(template.Must(template.New("").Funcs(template.FuncMap{
		"contains":  func(s, substr string) bool { return true },
		"timeSince": func(t time.Time) string { return "1h ago" },
		"eventIcon": func(t string) string { return "" },
	}).Parse(`{{define "feed"}}<div class="feed-section">{{if gt .Count 0}}{{range .Groups}}<div class="feed-date-group">{{.Date}}{{range .Events}}<div class="feed-event">{{.Title}}</div>{{end}}</div>{{end}}{{else}}<div class="feed-empty">no events</div>{{end}}</div>{{end}}`)))
	engine.GET("/feed", handler)
	engine.POST("/feed/filter", handler)
	req := httptest.NewRequest(method, path, http.NoBody)
	engine.ServeHTTP(w, req)
	return w
}

func TestFeed_Empty(t *testing.T) {
	handler, _, uid := newTestFeedHandler(t)

	w := serveFeedRequest(handler.Feed, "GET", "/feed", uid)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !contains(w.Body.String(), "no events") {
		t.Errorf("expected empty state, got: %s", w.Body.String())
	}
}

func TestFeed_WithEvents(t *testing.T) {
	handler, client, uid := newTestFeedHandler(t)

	now := time.Now()
	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeRelease).
		SetTitle("v1.0.0").
		SetURL("https://github.com/test/repo/releases/v1.0.0").
		SetTimestamp(now).
		Save(context.Background())

	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeWorkflowFailure).
		SetTitle("CI failed on main").
		SetURL("https://github.com/test/repo/actions/1").
		SetTimestamp(now.Add(-2 * time.Hour)).
		Save(context.Background())

	w := serveFeedRequest(handler.Feed, "GET", "/feed", uid)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "v1.0.0") {
		t.Errorf("expected title v1.0.0 in response, got: %s", body)
	}
	if !contains(body, "CI failed on main") {
		t.Errorf("expected 'CI failed on main' in response, got: %s", body)
	}
}

func TestFeed_FilterByType(t *testing.T) {
	handler, client, uid := newTestFeedHandler(t)

	now := time.Now()
	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeRelease).
		SetTitle("v2.0.0").
		SetURL("https://github.com/test/repo/releases/v2.0.0").
		SetTimestamp(now).
		Save(context.Background())

	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypePrMerge).
		SetTitle("feat: add magic").
		SetURL("https://github.com/test/repo/pull/42").
		SetTimestamp(now).
		Save(context.Background())

	w := serveFeedRequest(handler.Feed, "GET", "/feed?types=release", uid)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "v2.0.0") {
		t.Errorf("expected v2.0.0 in filtered response, got: %s", body)
	}
	if contains(body, "feat: add magic") {
		t.Errorf("did not expect PR event in release-only filter, got: %s", body)
	}
}

func TestFeed_FilterByTime(t *testing.T) {
	handler, client, uid := newTestFeedHandler(t)

	now := time.Now()
	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeRelease).
		SetTitle("recent release").
		SetURL("https://github.com/test/repo/releases/1").
		SetTimestamp(now).
		Save(context.Background())

	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeRelease).
		SetTitle("old release").
		SetURL("https://github.com/test/repo/releases/2").
		SetTimestamp(now.Add(-30 * 24 * time.Hour)). // 30 days ago
		Save(context.Background())

	w := serveFeedRequest(handler.Feed, "GET", "/feed?since=24h", uid)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "recent release") {
		t.Errorf("expected recent release in 24h filter, got: %s", body)
	}
	if contains(body, "old release") {
		t.Errorf("did not expect old release in 24h filter, got: %s", body)
	}
}

func TestFeed_DateGrouping(t *testing.T) {
	handler, client, uid := newTestFeedHandler(t)

	now := time.Now()
	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeRelease).
		SetTitle("today event").
		SetURL("https://github.com/test/repo/releases/today").
		SetTimestamp(now).
		Save(context.Background())

	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeRelease).
		SetTitle("yesterday event").
		SetURL("https://github.com/test/repo/releases/yesterday").
		SetTimestamp(now.Add(-25 * time.Hour)).
		Save(context.Background())

	w := serveFeedRequest(handler.Feed, "GET", "/feed?since=7d", uid)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "Today") {
		t.Errorf("expected 'Today' group in response, got: %s", body)
	}
	if !contains(body, "today event") {
		t.Errorf("expected 'today event' in response, got: %s", body)
	}
}

func TestFeed_PostFilter(t *testing.T) {
	handler, client, uid := newTestFeedHandler(t)

	now := time.Now()
	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeWorkflowFailure).
		SetTitle("ci failure").
		SetURL("https://github.com/test/repo/actions/1").
		SetTimestamp(now).
		Save(context.Background())

	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypePrMerge).
		SetTitle("pr merge").
		SetURL("https://github.com/test/repo/pull/1").
		SetTimestamp(now).
		Save(context.Background())

	w := serveFeedRequest(handler.FeedFilter, "POST", "/feed/filter", uid)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 on POST, got %d", w.Code)
	}
	body := w.Body.String()
	if !contains(body, "ci failure") {
		t.Errorf("expected ci failure in POST response, got: %s", body)
	}
}

// contains is a helper to check substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
