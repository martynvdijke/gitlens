package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlens/ent"
	"gitlens/ent/enttest"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newTestGitHubAppHandler(t *testing.T) (*GitHubAppHandler, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	return NewGitHubAppHandler(client), client
}

func TestGitHubApp_HandleInstallation_Created(t *testing.T) {
	handler, client := newTestGitHubAppHandler(t)

	u, _ := client.User.Create().
		SetGithubID(1000).SetLogin("testuser").SetAccessToken("token").Save(context.Background())

	payload := map[string]any{
		"action": "created",
		"installation": map[string]any{
			"id": 12345,
			"account": map[string]any{
				"login": "testuser",
				"id":    1000,
			},
			"repositories_url": "https://api.github.com/installations/12345/repos",
		},
		"repositories": []map[string]any{
			{
				"id":        200,
				"name":      "app-repo-1",
				"full_name": "testuser/app-repo-1",
				"owner":     map[string]any{"login": "testuser"},
			},
			{
				"id":        201,
				"name":      "app-repo-2",
				"full_name": "testuser/app-repo-2",
				"owner":     map[string]any{"login": "testuser"},
			},
		},
	}

	body, _ := json.Marshal(payload)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github-app", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.HandleInstallation(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	count, err := client.Repository.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("count error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 repos created, got %d", count)
	}

	// Verify user association
	repos, _ := client.Repository.Query().All(context.Background())
	for _, r := range repos {
		uid, err := r.QueryUser().OnlyID(context.Background())
		if err != nil {
			t.Fatalf("query user edge: %v", err)
		}
		if uid != u.ID {
			t.Errorf("expected repo user ID %d, got %d", u.ID, uid)
		}
	}
}

func TestGitHubApp_HandleInstallation_NoMatchingUser(t *testing.T) {
	handler, _ := newTestGitHubAppHandler(t)

	payload := map[string]any{
		"action": "created",
		"installation": map[string]any{
			"id": 12345,
			"account": map[string]any{
				"login": "nonexistent-user",
				"id":    99999,
			},
		},
		"repositories": []map[string]any{
			{
				"id":        300,
				"name":      "some-repo",
				"full_name": "nonexistent-user/some-repo",
				"owner":     map[string]any{"login": "nonexistent-user"},
			},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github-app", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.HandleInstallation(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGitHubApp_HandleInstallation_Removed(t *testing.T) {
	handler, client := newTestGitHubAppHandler(t)

	u, _ := client.User.Create().
		SetGithubID(2000).SetLogin("removaltest").SetAccessToken("token").Save(context.Background())

	client.Repository.Create().
		SetGithubID(400).SetOwner("removaltest").SetName("gone-repo").
		SetFullName("removaltest/gone-repo").SetHTMLURL("https://github.com/removaltest/gone-repo").
		SetDefaultBranch("main").SetUserID(u.ID).Save(context.Background())

	payload := map[string]any{
		"action": "removed",
		"installation": map[string]any{
			"id": 12345,
			"account": map[string]any{
				"login": "removaltest",
				"id":    2000,
			},
		},
		"repositories": []map[string]any{
			{
				"id":        400,
				"name":      "gone-repo",
				"full_name": "removaltest/gone-repo",
				"owner":     map[string]any{"login": "removaltest"},
			},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github-app", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.HandleInstallation(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	count, _ := client.Repository.Query().Count(context.Background())
	if count != 0 {
		t.Fatalf("expected 0 repos after removal, got %d", count)
	}
}

func TestGitHubApp_HandleInstallation_Deleted(t *testing.T) {
	handler, _ := newTestGitHubAppHandler(t)

	payload := map[string]any{
		"action": "deleted",
		"installation": map[string]any{
			"id": 12345,
			"account": map[string]any{
				"login": "anyone",
				"id":    3000,
			},
		},
	}

	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github-app", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.HandleInstallation(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGitHubApp_HandleInstallation_InvalidPayload(t *testing.T) {
	handler, _ := newTestGitHubAppHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github-app", bytes.NewReader([]byte(`{invalid json`)))
	c.Request.Header.Set("Content-Type", "application/json")
	handler.HandleInstallation(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGitHubApp_SetupAutoWebhooks_NoAppID(t *testing.T) {
	handler, client := newTestGitHubAppHandler(t)

	u, _ := client.User.Create().
		SetGithubID(3000).SetLogin("webhooktest").SetAccessToken("token").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/repos/setup-webhooks", nil)
	c.Set("user_id", int64(u.ID))
	handler.SetupAutoWebhooks(c)

	if !strings.Contains(w.Body.String(), "GITHUB_APP_ID not set") {
		t.Errorf("expected GITHUB_APP_ID error, got: %s", w.Body.String())
	}
}
