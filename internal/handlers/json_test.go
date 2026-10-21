package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlens/ent/event"

	"github.com/gin-gonic/gin"
)

func TestListRepos_JSONNegotiation(t *testing.T) {
	handler, _, client := newTestDashboardHandler(t)
	u := client.User.Create().SetGithubID(10).SetLogin("jsonuser").SetAccessToken("secret-token").SaveX(context.Background())
	client.Repository.Create().
		SetGithubID(1).
		SetOwner("acme").
		SetName("rocket").
		SetFullName("acme/rocket").
		SetDescription("Launch codes").
		SetHTMLURL("https://github.com/acme/rocket").
		SetDefaultBranch("main").
		SetProvider("github").
		SetWorkflowStatus("success").
		SetUserID(u.ID).
		SaveX(context.Background())

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/repos", func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.ListRepos(c)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/repos?q=rocket", nil)
	req.Header.Set("Accept", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Repos []struct {
			FullName string `json:"full_name"`
		} `json:"repos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Repos) != 1 || body.Repos[0].FullName != "acme/rocket" {
		t.Fatalf("unexpected repos payload: %s", w.Body.String())
	}
}

func TestPRsTab_JSONDoesNotLeakAccessToken(t *testing.T) {
	handler, _, client := newTestDashboardHandler(t)
	u := client.User.Create().SetGithubID(11).SetLogin("jsonpr").SetAccessToken("super-secret").SaveX(context.Background())
	client.Repository.Create().
		SetGithubID(2).
		SetOwner("acme").
		SetName("rocket").
		SetFullName("acme/rocket").
		SetHTMLURL("https://github.com/acme/rocket").
		SetDefaultBranch("main").
		SetPullRequests(`[{"n":7,"t":"Fix hatch","a":"ada","c":"2026-01-01T00:00:00Z","h":"https://github.com/acme/rocket/pull/7","hr":"fix","br":"main","ms":"clean","bs":"success"}]`).
		SetUserID(u.ID).
		SaveX(context.Background())

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/prs", func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.PRsTab(c)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/prs", nil)
	req.Header.Set("Accept", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if containsSecret(w.Body.String(), "super-secret") {
		t.Fatalf("response leaked the user's access token: %s", w.Body.String())
	}
	var body struct {
		PRs []struct {
			Number int `json:"number"`
		} `json:"prs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.PRs) != 1 || body.PRs[0].Number != 7 {
		t.Fatalf("unexpected PR payload: %s", w.Body.String())
	}
}

func TestFeed_JSONNegotiation(t *testing.T) {
	handler, client, uid := newTestFeedHandler(t)
	client.Event.Create().
		SetRepoID(1).
		SetType(event.TypeRelease).
		SetTitle("v1.0.0").
		SetURL("https://example.test/release").
		SetTimestamp(time.Now()).
		SaveX(context.Background())

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("user_id", int64(uid)) })
	engine.GET("/feed", handler.Feed)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/feed", nil)
	req.Header.Set("Accept", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Groups []struct {
			Events []struct {
				Title string `json:"title"`
			} `json:"events"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Groups) == 0 || len(body.Groups[0].Events) == 0 || body.Groups[0].Events[0].Title != "v1.0.0" {
		t.Fatalf("unexpected feed payload: %s", w.Body.String())
	}
}

func containsSecret(body, secret string) bool {
	return len(body) > 0 && len(secret) > 0 && (stringIndex(body, secret) >= 0)
}

func stringIndex(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
