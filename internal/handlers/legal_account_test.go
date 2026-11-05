package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"gitlens/ent"
	"gitlens/internal/account"
	"gitlens/internal/config"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func testLegalTemplates(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"appVersion": func() string { return "test" },
	}).Parse(`{{define "tab_bar"}}{{end}}{{define "bottom_nav"}}{{end}}{{define "landing_page"}}{{end}}{{define "available_repos"}}{{end}}`)
	if err != nil {
		t.Fatalf("parse stub templates: %v", err)
	}
	tmpl, err = tmpl.ParseFiles(
		"../../views/index.html",
		"../../views/privacy.html",
		"../../views/terms.html",
		"../../views/account_delete.html",
	)
	if err != nil {
		t.Fatalf("parse views: %v", err)
	}
	return tmpl
}

func serveLegalRequest(t *testing.T, h *LegalHandler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	r := gin.New()
	r.SetHTMLTemplate(testLegalTemplates(t))
	r.GET(path, func(c *gin.Context) {
		switch path {
		case "/privacy":
			h.Privacy(c)
		case "/terms":
			h.Terms(c)
		case "/account/delete":
			h.AccountDelete(c)
		}
	})
	req := httptest.NewRequest(method, path, nil)
	r.ServeHTTP(w, req)
	return w
}

func TestLegalPages_PublicAndConfigured(t *testing.T) {
	h := NewLegalHandler(nil, nil, config.Legal{
		ControllerName:       "Example B.V.",
		ContactEmail:         "privacy@example.com",
		ContactAddress:       "1 Example Street",
		HostingRegion:        "the Netherlands",
		DPOEmail:             "dpo@example.com",
		EURepresentative:     "Example EU Representative GmbH",
		SupervisoryAuthority: "the Dutch Data Protection Authority",
	})
	for _, path := range []string{"/privacy", "/terms", "/account/delete"} {
		w := serveLegalRequest(t, h, http.MethodGet, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", path, w.Code, w.Body.String())
		}
		t.Logf("%s body: %q", path, w.Body.String()[:min(200, len(w.Body.String()))])
		if !strings.Contains(w.Body.String(), "GitLens") {
			t.Fatalf("%s: expected GitLens layout", path)
		}
	}
	privacy := serveLegalRequest(t, h, http.MethodGet, "/privacy").Body.String()
	for _, want := range []string{"Example B.V.", "privacy@example.com", "1 Example Street", "the Netherlands", "GitHub", "Forgejo", "OAuth access tokens", "API tokens", "Umami", "16", "EU data protection", "Article 6", "Article 17", "Article 22", "72 hours", "dpo@example.com", "Example EU Representative GmbH", "the Dutch Data Protection Authority"} {
		if !strings.Contains(privacy, want) {
			t.Errorf("privacy page missing %q", want)
		}
	}
	terms := serveLegalRequest(t, h, http.MethodGet, "/terms").Body.String()
	if !strings.Contains(terms, "/privacy") {
		t.Error("terms page should link to the privacy policy")
	}
	del := serveLegalRequest(t, h, http.MethodGet, "/account/delete").Body.String()
	for _, want := range []string{"Delete your account", "What is removed", "What stays", "last admin", "privacy@example.com", "Google Play", "Article 17", "generated sample data"} {
		if !strings.Contains(del, want) {
			t.Errorf("deletion page missing %q", want)
		}
	}
}

func TestLegalPages_FallbackWhenUnconfigured(t *testing.T) {
	h := NewLegalHandler(nil, nil, config.Legal{})
	body := serveLegalRequest(t, h, http.MethodGet, "/privacy").Body.String()
	if !strings.Contains(body, "the operator of this GitLens instance") {
		t.Error("expected neutral controller fallback")
	}
	if !strings.Contains(body, "the European Union") {
		t.Error("expected hosting region fallback")
	}
	if strings.Contains(body, "mailto:") {
		t.Error("must not invent a contact address")
	}
	for _, invented := range []string{"The data protection officer", "controller's representative", "data protection officer can be reached"} {
		if strings.Contains(body, invented) {
			t.Errorf("must not invent identity %q", invented)
		}
	}
}

func newAccountHandlerTest(t *testing.T) (*AccountHandler, *ent.Client, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/handler.db?_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, user_id INTEGER NOT NULL, expires_at DATETIME NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return NewAccountHandler(client, account.New(client, db)), client, db
}

func serveAccountRequest(t *testing.T, h *AccountHandler, userID int64, method, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	r := gin.New()
	handler := func(c *gin.Context) {
		if userID > 0 {
			c.Set("user_id", userID)
		}
		switch {
		case path == "/settings/account/delete":
			h.DeleteSelf(c)
		case method == http.MethodDelete:
			h.DeleteAPI(c)
		case path == "/api/v1/me/export":
			h.Export(c)
		}
	}
	r.POST("/settings/account/delete", handler)
	r.DELETE("/api/v1/me", handler)
	r.GET("/api/v1/me/export", handler)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	r.ServeHTTP(w, req)
	return w
}

func TestAccountDelete_RequiresAuthAndConfirmation(t *testing.T) {
	h, client, _ := newAccountHandlerTest(t)
	ctx := context.Background()
	u, err := client.User.Create().SetGithubID(1).SetLogin("target").SetAccessToken("token").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if w := serveAccountRequest(t, h, 0, http.MethodPost, "/settings/account/delete", "application/x-www-form-urlencoded", "confirm=target"); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", w.Code)
	}
	form := url.Values{"confirm": {"wrong"}}.Encode()
	if w := serveAccountRequest(t, h, int64(u.ID), http.MethodPost, "/settings/account/delete", "application/x-www-form-urlencoded", form); w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for wrong confirmation, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := client.User.Get(ctx, u.ID); err != nil {
		t.Fatalf("user must survive a failed confirmation: %v", err)
	}
}

func TestAccountDelete_LastAdminRefused(t *testing.T) {
	h, client, _ := newAccountHandlerTest(t)
	ctx := context.Background()
	u, err := client.User.Create().SetGithubID(1).SetLogin("only-admin").SetAccessToken("token").SetIsAdmin(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"confirm": {"only-admin"}}.Encode()
	w := serveAccountRequest(t, h, int64(u.ID), http.MethodPost, "/settings/account/delete", "application/x-www-form-urlencoded", form)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := client.User.Get(ctx, u.ID); err != nil {
		t.Fatalf("last admin must survive: %v", err)
	}
}

func TestAccountDelete_JSONConfirmation(t *testing.T) {
	h, client, _ := newAccountHandlerTest(t)
	ctx := context.Background()
	keeper, err := client.User.Create().SetGithubID(1).SetLogin("admin").SetAccessToken("token").SetIsAdmin(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	u, err := client.User.Create().SetGithubID(2).SetLogin("target").SetAccessToken("token").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = keeper

	w := serveAccountRequest(t, h, int64(u.ID), http.MethodDelete, "/api/v1/me", "application/json", `{"confirm":"target"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := client.User.Get(ctx, u.ID); !ent.IsNotFound(err) {
		t.Fatalf("expected deleted user, got %v", err)
	}
}

func TestAccountExport_Handler(t *testing.T) {
	h, client, _ := newAccountHandlerTest(t)
	ctx := context.Background()
	u, err := client.User.Create().
		SetGithubID(3).SetLogin("exporter").SetAccessToken("github-secret").
		Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ApiToken.Create().SetUserID(u.ID).SetName("ci").SetTokenHash("hash-secret").Save(ctx); err != nil {
		t.Fatal(err)
	}
	w := serveAccountRequest(t, h, int64(u.ID), http.MethodGet, "/api/v1/me/export", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, secret := range []string{"github-secret", "hash-secret"} {
		if strings.Contains(body, secret) {
			t.Fatalf("export leaked %q", secret)
		}
	}
	if !strings.Contains(body, `"login": "exporter"`) || !strings.Contains(body, `"name": "ci"`) {
		t.Fatalf("unexpected export: %s", body)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("application/json")) && !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("expected JSON content type")
	}
}
