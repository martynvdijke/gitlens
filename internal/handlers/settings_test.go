package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlens/ent/enttest"
	"gitlens/ent/repository"
	"gitlens/internal/github"
	"gitlens/internal/middleware"
	"gitlens/internal/provider"
	"gitlens/internal/sync"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newTestSettingsHandler(t *testing.T, ghAPIURL string) (*SettingsHandler, *middleware.SessionStore) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	store := middleware.NewSessionStore(testSessionDB(t))
	ghClient := github.NewClient("", "")
	if ghAPIURL != "" {
		ghClient.APIURL = ghAPIURL
	}
	syncer := sync.NewSyncer(client, ghClient, map[string]provider.Provider{"github": provider.NewGitHubAdapter(ghClient)}, nil)
	h := NewSettingsHandler(client, store, ghClient, map[string]provider.Provider{"github": provider.NewGitHubAdapter(ghClient)}, syncer)
	h.bgCtx = t.Context()
	return h, store
}

func TestSettingsHandler_UpdateInterval_InvalidInput(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(800).SetLogin("su1").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("interval=abc"))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateInterval(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSettingsHandler_UpdateInterval_TooLow(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(801).SetLogin("su2").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("interval=0"))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateInterval(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSettingsHandler_UpdateInterval_TooHigh(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(802).SetLogin("su3").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("interval=1441"))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateInterval(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSettingsHandler_UpdateInterval_Success(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(803).SetLogin("su4").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("interval=30"))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateInterval(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "30") {
		t.Fatalf("expected body to contain '30', got: %s", w.Body.String())
	}

	updated, _ := client.User.Get(context.Background(), int(u.ID))
	if updated.SyncIntervalMinutes != 30 {
		t.Fatalf("expected interval 30, got %d", updated.SyncIntervalMinutes)
	}
}

func TestSettingsHandler_RemoveRepo_InvalidID(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(804).SetLogin("ru1").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Params = []gin.Param{{Key: "id", Value: "notanumber"}}
	c.Request = httptest.NewRequest("DELETE", "/repos/notanumber", nil)
	handler.RemoveRepo(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSettingsHandler_RemoveRepo_NotFound(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(805).SetLogin("ru2").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Params = []gin.Param{{Key: "id", Value: "9999"}}
	c.Request = httptest.NewRequest("DELETE", "/repos/9999", nil)
	handler.RemoveRepo(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSettingsHandler_RemoveRepo_Success(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(806).SetLogin("ru3").SetAccessToken("tok").Save(context.Background())

	repo, _ := client.Repository.Create().
		SetGithubID(200).SetOwner("test").SetName("toremove").
		SetFullName("test/toremove").SetHTMLURL("https://github.com/test/toremove").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Params = []gin.Param{{Key: "id", Value: "1"}}
	c.Request = httptest.NewRequest("DELETE", "/repos/1", nil)
	handler.RemoveRepo(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	count, _ := client.Repository.Query().Where(repository.ID(repo.ID)).Exist(context.Background())
	if count {
		t.Fatal("expected repo to be deleted")
	}
}

func TestSettingsHandler_SelectRepos_NoReposSelected(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(807).SetLogin("sel1").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(""))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.SelectRepos(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "No repositories selected") {
		t.Fatalf("expected error message, got: %s", w.Body.String())
	}
}

func TestSettingsHandler_UpdateUmami_Success(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(900).SetLogin("umami1").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("umami_url=https://umami.example.com&umami_site_id=abc-123"))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateUmami(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "saved") {
		t.Fatalf("expected success message, got: %s", w.Body.String())
	}

	updated, _ := client.User.Get(context.Background(), int(u.ID))
	if updated.UmamiURL != "https://umami.example.com" {
		t.Fatalf("expected umami_url 'https://umami.example.com', got %q", updated.UmamiURL)
	}
	if updated.UmamiSiteID != "abc-123" {
		t.Fatalf("expected umami_site_id 'abc-123', got %q", updated.UmamiSiteID)
	}
}

func TestSettingsHandler_UpdateUmami_Clear(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(901).SetLogin("umami2").SetAccessToken("tok").
		SetUmamiURL("https://umami.example.com").SetUmamiSiteID("abc-123").
		Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("umami_url=&umami_site_id="))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateUmami(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cleared") {
		t.Fatalf("expected clear message, got: %s", w.Body.String())
	}

	updated, _ := client.User.Get(context.Background(), int(u.ID))
	if updated.UmamiURL != "" {
		t.Fatalf("expected empty umami_url, got %q", updated.UmamiURL)
	}
	if updated.UmamiSiteID != "" {
		t.Fatalf("expected empty umami_site_id, got %q", updated.UmamiSiteID)
	}
}

func TestSettingsHandler_UpdateUmami_PartialInput(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, _ := client.User.Create().SetGithubID(902).SetLogin("umami3").SetAccessToken("tok").Save(context.Background())

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("umami_url=https://umami.example.com&umami_site_id="))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateUmami(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Both Umami URL and Site ID are required") {
		t.Fatalf("expected error about both fields required, got: %s", w.Body.String())
	}
}

func TestSettingsHandler_UpdateEinkMode_Enable(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, err := client.User.Create().SetGithubID(1001).SetLogin("eink1").SetAccessToken("tok").Save(context.Background())
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("enabled=true"))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateEinkMode(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "enabled") {
		t.Fatalf("expected 'enabled' in response, got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "classList.add('eink-mode')") {
		t.Fatalf("expected live-toggle script in response, got: %s", w.Body.String())
	}

	updated, err := client.User.Get(context.Background(), int(u.ID))
	if err != nil {
		t.Fatalf("failed to fetch user: %v", err)
	}
	if !updated.EinkMode {
		t.Fatal("expected eink_mode to be true after enabling")
	}
}

func TestSettingsHandler_UpdateEinkMode_Disable(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, err := client.User.Create().SetGithubID(1002).SetLogin("eink2").SetAccessToken("tok").SetEinkMode(true).Save(context.Background())
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader("enabled=false"))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateEinkMode(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "disabled") {
		t.Fatalf("expected 'disabled' in response, got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "classList.remove('eink-mode')") {
		t.Fatalf("expected live-toggle script in response, got: %s", w.Body.String())
	}

	updated, err := client.User.Get(context.Background(), int(u.ID))
	if err != nil {
		t.Fatalf("failed to fetch user: %v", err)
	}
	if updated.EinkMode {
		t.Fatal("expected eink_mode to be false after disabling")
	}
}

func TestSettingsHandler_UpdateEinkMode_EmptyForm(t *testing.T) {
	handler, _ := newTestSettingsHandler(t, "")
	client := handler.client
	u, err := client.User.Create().SetGithubID(1003).SetLogin("eink3").SetAccessToken("tok").SetEinkMode(true).Save(context.Background())
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", int64(u.ID))
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(""))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handler.UpdateEinkMode(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "disabled") {
		t.Fatalf("expected 'disabled' in response, got: %s", w.Body.String())
	}

	updated, err := client.User.Get(context.Background(), int(u.ID))
	if err != nil {
		t.Fatalf("failed to fetch user: %v", err)
	}
	if updated.EinkMode {
		t.Fatal("expected eink_mode to be false when form value is not 'true'")
	}
}
