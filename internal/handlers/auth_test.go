package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gitlens/ent"
	"gitlens/ent/enttest"
	"gitlens/internal/github"
	"gitlens/internal/middleware"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func testSessionDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/sessions.db?_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestAuthHandler(t *testing.T, ghTokenURL, ghAPIURL string) (*AuthHandler, *middleware.SessionStore, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	store := middleware.NewSessionStore(testSessionDB(t))
	ghClient := github.NewClient("test_client_id", "test_client_secret")
	if ghTokenURL != "" {
		ghClient.TokenURL = ghTokenURL
	}
	if ghAPIURL != "" {
		ghClient.APIURL = ghAPIURL
	}
	return NewAuthHandler(client, store, ghClient), store, client
}

func serveTestRequest(handler gin.HandlerFunc, method, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("index.html").Parse(`<html>{{.Error}}</html>`)))
	engine.GET("/test/:action", handler)
	engine.GET("/test/:action/repos", handler)
	engine.POST("/test/:action", handler)
	req := httptest.NewRequest(method, path, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	engine.ServeHTTP(w, req)
	return w
}

func serveTestRequestPath(handler gin.HandlerFunc, method, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Funcs(template.FuncMap{
		"shortSHA":           func(s string) string { return s },
		"formatTime":         func(t time.Time) string { return "" },
		"truncate":           func(s string, n int) string { return s },
		"workflowIcon":       func(status string) string { return "" },
		"workflowLabel":      func(status string) string { return "" },
		"hasWorkflowRun":     func(status string) bool { return false },
		"printf":             func(format string, args ...interface{}) string { return "" },
		"releaseIcon":        func(conclusion string) string { return "" },
		"releaseLabel":       func(conclusion string) string { return "" },
		"hasReleaseConclusion": func(s string) bool { return false },
	}).Parse(`<html>{{.}}</html>`)))
	if handler != nil {
		engine.GET("/repos", handler)
	}
	req := httptest.NewRequest(method, path, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	engine.ServeHTTP(w, req)
	return w
}

func TestLogin_RedirectsToGitHub(t *testing.T) {
	t.Setenv("GITHUB_CLIENT_ID", "test_client_id")
	t.Setenv("GITHUB_REDIRECT_URL", "")

	handler, _, _ := newTestAuthHandler(t, "", "")
	w := serveTestRequest(handler.Login, "GET", "/test/login")

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Fatal("expected Location header")
	}
	expected := "https://github.com/login/oauth/authorize?client_id=test_client_id&redirect_uri=http://localhost:6270/auth/github/callback&scope=repo,read:user"
	if loc != expected {
		t.Fatalf("unexpected redirect URL:\n  got:  %s\n  want: %s", loc, expected)
	}
}

func TestLogin_UsesCustomRedirectURL(t *testing.T) {
	t.Setenv("GITHUB_CLIENT_ID", "test_client_id")
	t.Setenv("GITHUB_REDIRECT_URL", "https://example.com/auth/github/callback")

	handler, _, _ := newTestAuthHandler(t, "", "")
	w := serveTestRequest(handler.Login, "GET", "/test/login")

	loc := w.Header().Get("Location")
	expected := "https://github.com/login/oauth/authorize?client_id=test_client_id&redirect_uri=https://example.com/auth/github/callback&scope=repo,read:user"
	if loc != expected {
		t.Fatalf("unexpected redirect URL:\n  got:  %s\n  want: %s", loc, expected)
	}
}

func TestCallback_MissingCode(t *testing.T) {
	handler, _, _ := newTestAuthHandler(t, "", "")
	w := serveTestRequest(handler.Callback, "GET", "/test/callback")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCallback_TokenExchangeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"bad_verification_code"}`))
	}))
	defer srv.Close()

	handler, _, _ := newTestAuthHandler(t, srv.URL, "")
	w := serveTestRequest(handler.Callback, "GET", "/test/callback?code=bad_code")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCallback_Success(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"test_token_123"}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":12345,"login":"octocat","avatar_url":"https://avatars.example.com/u/12345","name":"Octo Cat"}`))
	}))
	defer apiSrv.Close()

	handler, store, client := newTestAuthHandler(t, tokenSrv.URL, apiSrv.URL)
	w := serveTestRequest(handler.Callback, "GET", "/test/callback?code=valid_code")

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected redirect to /, got %s", loc)
	}

	count, err := client.User.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("user count error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	cookies := w.Result().Cookies()
	var sessionCookie string
	for _, cookie := range cookies {
		if cookie.Name == "gitlens_session" {
			sessionCookie = cookie.Value
			break
		}
	}
	if sessionCookie == "" {
		t.Fatal("expected session cookie")
	}
	if _, ok := store.Get(sessionCookie); !ok {
		t.Fatal("expected session to exist in store")
	}
}

func TestCallback_ExistingUser(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"new_token"}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":12345,"login":"octocat","avatar_url":"https://avatars.example.com/u/12345","name":"Octo Cat"}`))
	}))
	defer apiSrv.Close()

	handler, _, client := newTestAuthHandler(t, tokenSrv.URL, apiSrv.URL)

	_, err := client.User.Create().
		SetGithubID(12345).
		SetLogin("octocat").
		SetAccessToken("old_token").
		Save(context.Background())
	if err != nil {
		t.Fatalf("pre-create user error: %v", err)
	}

	w := serveTestRequest(handler.Callback, "GET", "/test/callback?code=valid_code")

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	count, err := client.User.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("user count error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	u, err := client.User.Query().Only(context.Background())
	if err != nil {
		t.Fatalf("user query error: %v", err)
	}
	if u.AccessToken != "new_token" {
		t.Fatalf("expected access_token to be updated to 'new_token', got '%s'", u.AccessToken)
	}
}

func TestLogout(t *testing.T) {
	handler, store, _ := newTestAuthHandler(t, "", "")

	sessionID := store.Set(42)

	w := serveTestRequest(handler.Logout, "POST", "/test/logout",
		&http.Cookie{Name: "gitlens_session", Value: sessionID})

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected redirect to /, got %s", loc)
	}

	if _, ok := store.Get(sessionID); ok {
		t.Fatal("expected session to be deleted")
	}

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "gitlens_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected session cookie in response")
	}
	if sessionCookie.MaxAge >= 0 {
		t.Fatalf("expected negative max-age for cookie deletion, got %d", sessionCookie.MaxAge)
	}
}

func TestLogout_NoSession(t *testing.T) {
	handler, _, _ := newTestAuthHandler(t, "", "")
	w := serveTestRequest(handler.Logout, "POST", "/test/logout")

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
}

func TestCallback_WithAccessTokenInResponse(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"test_token_123"}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":         99999,
			"login":      "newuser",
			"avatar_url": "https://avatars.example.com/u/99999",
			"name":       "New User",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer apiSrv.Close()

	handler, store, client := newTestAuthHandler(t, tokenSrv.URL, apiSrv.URL)
	w := serveTestRequest(handler.Callback, "GET", "/test/callback?code=valid_code")

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	u, err := client.User.Query().Only(context.Background())
	if err != nil {
		t.Fatalf("user query error: %v", err)
	}
	if u.GithubID != 99999 {
		t.Fatalf("expected github_id 99999, got %d", u.GithubID)
	}
	if u.Login != "newuser" {
		t.Fatalf("expected login 'newuser', got '%s'", u.Login)
	}
	if u.AccessToken != "test_token_123" {
		t.Fatalf("expected access_token 'test_token_123', got '%s'", u.AccessToken)
	}

	var sessionID string
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "gitlens_session" {
			sessionID = cookie.Value
			break
		}
	}
	if sessionID == "" {
		t.Fatal("expected session cookie")
	}
	if uid, ok := store.Get(sessionID); !ok || uid != int64(u.ID) {
		t.Fatalf("expected session for user %d, got %d", u.ID, uid)
	}
}

func TestCallback_GitHubUserFetchError(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"test_token"}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer apiSrv.Close()

	handler, _, _ := newTestAuthHandler(t, tokenSrv.URL, apiSrv.URL)
	w := serveTestRequest(handler.Callback, "GET", "/test/callback?code=valid_code")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
