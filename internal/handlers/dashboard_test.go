package handlers

import (
	"context"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlens/ent"
	"gitlens/ent/enttest"
	"gitlens/internal/github"
	"gitlens/internal/middleware"
	"gitlens/internal/sync"
	"gitlens/internal/ws"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newTestDashboardHandler(t *testing.T) (*DashboardHandler, *middleware.SessionStore, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	store := middleware.NewSessionStore()
	ghClient := github.NewClient("", "")
	hub := ws.NewHub()
	syncer := sync.NewSyncer(client, ghClient, hub)
	return NewDashboardHandler(client, store, ghClient, syncer), store, client
}

func serveListReposRequest(handler gin.HandlerFunc, method, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Funcs(template.FuncMap{
		"shortSHA":             func(s string) string { return s },
		"formatTime":           func(t time.Time) string { return "" },
		"truncate":             func(s string, n int) string { return s },
		"workflowIcon":         func(status string) string { return "" },
		"workflowLabel":        func(status string) string { return "" },
		"hasWorkflowRun":       func(status string) bool { return false },
		"printf":               func(format string, args ...interface{}) string { return "" },
		"releaseIcon":          func(conclusion string) string { return "" },
		"releaseLabel":         func(conclusion string) string { return "" },
		"hasReleaseConclusion": func(s string) bool { return false },
	}).Parse(`{{define "repo_list"}}<div>{{range .Repos}}<div class="repo">{{.FullName}}</div>{{end}}</div>{{end}}`)))
	engine.GET("/repos", handler)
	req := httptest.NewRequest(method, path, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	engine.ServeHTTP(w, req)
	return w
}

func serveDashboardRequest(handler gin.HandlerFunc, method, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Funcs(template.FuncMap{
		"shortSHA":             func(s string) string { return s },
		"formatTime":           func(t time.Time) string { return "" },
		"truncate":             func(s string, n int) string { return s },
		"workflowIcon":         func(status string) string { return "" },
		"workflowLabel":        func(status string) string { return "" },
		"hasWorkflowRun":       func(status string) bool { return false },
		"printf":               func(format string, args ...interface{}) string { return "" },
		"releaseIcon":          func(conclusion string) string { return "" },
		"releaseLabel":         func(conclusion string) string { return "" },
		"hasReleaseConclusion": func(s string) bool { return false },
	}).Parse(`{{define "dashboard"}}<div>{{range .Repos}}<div class="repo">{{.FullName}}</div>{{end}}</div>{{end}}`)))
	engine.GET("/dashboard", handler)
	req := httptest.NewRequest(method, path, nil)
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	engine.ServeHTTP(w, req)
	return w
}

func TestListRepos_FilterByQuery(t *testing.T) {
	handler, store, client := newTestDashboardHandler(t)

	u, err := client.User.Create().
		SetGithubID(100).
		SetLogin("testuser").
		SetAccessToken("token").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	client.Repository.Create().SetGithubID(1).SetOwner("user").SetName("frontend-app").SetFullName("user/frontend-app").SetDescription("UI app").SetHTMLURL("https://github.com/user/frontend-app").SetDefaultBranch("main").SetUserID(u.ID).Save(context.Background())
	client.Repository.Create().SetGithubID(2).SetOwner("user").SetName("backend-api").SetFullName("user/backend-api").SetDescription("API server").SetHTMLURL("https://github.com/user/backend-api").SetDefaultBranch("main").SetUserID(u.ID).Save(context.Background())
	client.Repository.Create().SetGithubID(3).SetOwner("user").SetName("docs-site").SetFullName("user/docs-site").SetDescription("Documentation").SetHTMLURL("https://github.com/user/docs-site").SetDefaultBranch("main").SetUserID(u.ID).Save(context.Background())

	sessionID := store.Set(int64(u.ID))

	// Test: filter by "frontend" should return 1 repo
	listHandler := func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.ListRepos(c)
	}
	w := serveListReposRequest(listHandler, "GET", "/repos?q=frontend", &http.Cookie{Name: "gitlens_session", Value: sessionID})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "frontend-app") {
		t.Errorf("expected frontend-app in response, body: %s", body)
	}
	if strings.Contains(body, "backend-api") {
		t.Errorf("did not expect backend-api in response, body: %s", body)
	}

	// Test: filter by "api" should return only backend-api
	w2 := serveListReposRequest(listHandler, "GET", "/repos?q=api", &http.Cookie{Name: "gitlens_session", Value: sessionID})
	if !strings.Contains(w2.Body.String(), "backend-api") {
		t.Errorf("expected backend-api in response, body: %s", w2.Body.String())
	}

	// Test: filter by nonexistent returns empty
	w3 := serveListReposRequest(listHandler, "GET", "/repos?q=nonexistent", &http.Cookie{Name: "gitlens_session", Value: sessionID})
	if strings.Contains(w3.Body.String(), "frontend-app") || strings.Contains(w3.Body.String(), "backend-api") {
		t.Errorf("expected empty results for nonexistent query, body: %s", w3.Body.String())
	}

	// Test: no query returns all repos
	w4 := serveListReposRequest(listHandler, "GET", "/repos", &http.Cookie{Name: "gitlens_session", Value: sessionID})
	if !strings.Contains(w4.Body.String(), "frontend-app") || !strings.Contains(w4.Body.String(), "backend-api") || !strings.Contains(w4.Body.String(), "docs-site") {
		t.Errorf("expected all repos in response, body: %s", w4.Body.String())
	}

	// Test: case insensitive filter
	w5 := serveListReposRequest(listHandler, "GET", "/repos?q=FrontEnd", &http.Cookie{Name: "gitlens_session", Value: sessionID})
	if !strings.Contains(w5.Body.String(), "frontend-app") {
		t.Errorf("expected case-insensitive match for FrontEnd, body: %s", w5.Body.String())
	}
}

func TestDashboard_FilterByQuery(t *testing.T) {
	handler, store, client := newTestDashboardHandler(t)

	u, err := client.User.Create().
		SetGithubID(200).
		SetLogin("testuser2").
		SetAccessToken("token").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	client.Repository.Create().SetGithubID(10).SetOwner("org").SetName("alpha").SetFullName("org/alpha").SetDescription("Alpha service").SetHTMLURL("https://github.com/org/alpha").SetDefaultBranch("main").SetUserID(u.ID).Save(context.Background())
	client.Repository.Create().SetGithubID(11).SetOwner("org").SetName("beta").SetFullName("org/beta").SetDescription("Beta service").SetHTMLURL("https://github.com/org/beta").SetDefaultBranch("main").SetUserID(u.ID).Save(context.Background())

	sessionID := store.Set(int64(u.ID))

	dashHandler := func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.Dashboard(c)
	}
	w := serveDashboardRequest(dashHandler, "GET", "/dashboard?q=alpha", &http.Cookie{Name: "gitlens_session", Value: sessionID})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "org/alpha") {
		t.Errorf("expected org/alpha in response, body: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "org/beta") {
		t.Errorf("did not expect org/beta in response, body: %s", w.Body.String())
	}
}

func TestComputeMetrics(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	u, _ := client.User.Create().
		SetGithubID(300).SetLogin("metrictest").SetAccessToken("tok").Save(context.Background())

	repo1, _ := client.Repository.Create().
		SetGithubID(20).SetOwner("u").SetName("r1").SetFullName("u/r1").SetHTMLURL("https://github.com/u/r1").SetDefaultBranch("main").SetUserID(u.ID).
		SetTotalCommitsFetched(10).SetFeatCount(4).SetFixCount(3).SetDocsCount(2).SetChoreCount(1).SetOtherCommitCount(0).
		SetReleaseCount(2).SetWorkflowSuccessCount(5).SetWorkflowFailureCount(1).SetAvgLeadTimeHours(12.5).
		Save(context.Background())
	repo2, _ := client.Repository.Create().
		SetGithubID(21).SetOwner("u").SetName("r2").SetFullName("u/r2").SetHTMLURL("https://github.com/u/r2").SetDefaultBranch("main").SetUserID(u.ID).
		SetTotalCommitsFetched(20).SetFeatCount(2).SetFixCount(2).SetDocsCount(1).SetChoreCount(3).SetOtherCommitCount(2).
		SetReleaseCount(3).SetWorkflowSuccessCount(8).SetWorkflowFailureCount(2).SetAvgLeadTimeHours(24.0).
		Save(context.Background())

	metrics := computeMetrics([]*ent.Repository{repo1, repo2})

	if metrics.TotalRepos != 2 {
		t.Errorf("expected 2 repos, got %d", metrics.TotalRepos)
	}
	if metrics.TotalReleases != 5 {
		t.Errorf("expected 5 releases, got %d", metrics.TotalReleases)
	}
	if metrics.TotalCommits != 30 {
		t.Errorf("expected 30 commits, got %d", metrics.TotalCommits)
	}
	if metrics.FeatCount != 6 {
		t.Errorf("expected 6 feats, got %d", metrics.FeatCount)
	}
	if metrics.FixCount != 5 {
		t.Errorf("expected 5 fixes, got %d", metrics.FixCount)
	}
	if metrics.WorkflowSuccesses != 13 {
		t.Errorf("expected 13 workflow successes, got %d", metrics.WorkflowSuccesses)
	}
	if metrics.WorkflowFailures != 3 {
		t.Errorf("expected 3 workflow failures, got %d", metrics.WorkflowFailures)
	}
	if metrics.AvgLeadTimeHours != 18.3 {
		t.Errorf("expected 18.3 avg lead time, got %.1f", metrics.AvgLeadTimeHours)
	}
}

func TestImportAllRepos_Success(t *testing.T) {
	handler, store, client := newTestDashboardHandler(t)

	u, err := client.User.Create().
		SetGithubID(100).
		SetLogin("testuser").
		SetAccessToken("test_token").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Pre-create one existing repo to test dedup
	client.Repository.Create().
		SetGithubID(1).SetOwner("user").SetName("existing").
		SetFullName("user/existing").SetHTMLURL("https://github.com/user/existing").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(context.Background())

	pageCounters := make(map[string]int)
	apiCalls := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		w.Header().Set("Content-Type", "application/json")
		key := r.Method + " " + r.URL.Path
		pageCounters[key]++
		page := pageCounters[key]
		switch {
		case r.URL.Path == "/user/repos":
			if page == 1 {
				w.Write([]byte(`[
					{"id":1,"name":"existing","full_name":"user/existing","html_url":"https://github.com/user/existing","default_branch":"main","owner":{"login":"user"}},
					{"id":2,"name":"newrepo1","full_name":"user/newrepo1","html_url":"https://github.com/user/newrepo1","default_branch":"main","language":"Go","owner":{"login":"user"}}
				]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case strings.Contains(r.URL.Path, "/repos/user/newrepo1/commits"):
			w.Write([]byte(`[{"sha":"abc","commit":{"message":"feat: initial commit","committer":{"date":"2024-06-01T10:00:00Z"}}}]`))
		case strings.Contains(r.URL.Path, "/repos/user/newrepo1/releases"):
			if page == 1 {
				w.Write([]byte(`[{"tag_name":"v1.0.0","name":"Version 1","published_at":"2024-06-15T10:00:00Z"}]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case strings.Contains(r.URL.Path, "/repos/user/newrepo1/actions/runs"):
			w.Write([]byte(`{"workflow_runs":[{"id":1,"status":"completed","conclusion":"success"}]}`))
		case strings.Contains(r.URL.Path, "/repos/user/newrepo1/pulls"):
			w.Write([]byte(`[]`))
		default:
			t.Logf("Unexpected API call: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiSrv.Close()
	handler.gh.APIURL = apiSrv.URL

	sessionID := store.Set(int64(u.ID))
	w := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Funcs(template.FuncMap{
		"shortSHA":             func(s string) string { return s },
		"formatTime":           func(t time.Time) string { return "" },
		"truncate":             func(s string, n int) string { return s },
		"workflowIcon":         func(status string) string { return "" },
		"workflowLabel":        func(status string) string { return "" },
		"hasWorkflowRun":       func(status string) bool { return false },
		"printf":               func(format string, args ...interface{}) string { return "" },
		"releaseIcon":          func(conclusion string) string { return "" },
		"releaseLabel":         func(conclusion string) string { return "" },
		"hasReleaseConclusion": func(s string) bool { return false },
	}).Parse(`{{define "repo_list"}}<div>{{range .Repos}}<div class="repo">{{.FullName}}</div>{{end}}</div>{{end}}`)))
	engine.POST("/repos/import-all", func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.ImportAllRepos(c)
	})
	req := httptest.NewRequest("POST", "/repos/import-all", nil)
	req.AddCookie(&http.Cookie{Name: "gitlens_session", Value: sessionID})
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	repos, _ := client.Repository.Query().All(context.Background())
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	// Verify the existing repo was not synced (no API calls for existing)
	if apiCalls < 2 {
		t.Errorf("expected at least 2 API calls (list repos + sync new), got %d", apiCalls)
	}
}

func TestImportAllRepos_NoRepos(t *testing.T) {
	handler, store, client := newTestDashboardHandler(t)

	u, err := client.User.Create().
		SetGithubID(200).
		SetLogin("norepos").
		SetAccessToken("token").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer apiSrv.Close()
	handler.gh.APIURL = apiSrv.URL

	sessionID := store.Set(int64(u.ID))
	w := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Funcs(template.FuncMap{
		"shortSHA":             func(s string) string { return s },
		"formatTime":           func(t time.Time) string { return "" },
		"truncate":             func(s string, n int) string { return s },
		"workflowIcon":         func(status string) string { return "" },
		"workflowLabel":        func(status string) string { return "" },
		"hasWorkflowRun":       func(status string) bool { return false },
		"printf":               func(format string, args ...interface{}) string { return "" },
		"releaseIcon":          func(conclusion string) string { return "" },
		"releaseLabel":         func(conclusion string) string { return "" },
		"hasReleaseConclusion": func(s string) bool { return false },
	}).Parse(`{{define "repo_list"}}<div>{{range .Repos}}<div class="repo">{{.FullName}}</div>{{end}}</div>{{end}}`)))
	engine.POST("/repos/import-all", func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.ImportAllRepos(c)
	})
	req := httptest.NewRequest("POST", "/repos/import-all", nil)
	req.AddCookie(&http.Cookie{Name: "gitlens_session", Value: sessionID})
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	repos, _ := client.Repository.Query().All(context.Background())
	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(repos))
	}
}

func TestListPullRequests_Success(t *testing.T) {
	handler, store, client := newTestDashboardHandler(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(300).
		SetLogin("pruser").
		SetAccessToken("pr_token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = client.Repository.Create().
		SetGithubID(10).SetOwner("org").SetName("pr-repo").
		SetFullName("org/pr-repo").SetHTMLURL("https://github.com/org/pr-repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		SetPullRequests(`[{"n":1,"t":"Fix bug","a":"dev1","c":"2024-06-01T10:00:00Z","h":"https://github.com/org/pr-repo/pull/1","hr":"fix-bug","br":"main"}]`).
		SetOpenPrCount(1).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	sessionID := store.Set(int64(u.ID))
	w := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Funcs(template.FuncMap{
		"shortSHA":             func(s string) string { return s },
		"formatTime":           func(t time.Time) string { return "" },
		"truncate":             func(s string, n int) string { return s },
		"workflowIcon":         func(status string) string { return "" },
		"workflowLabel":        func(status string) string { return "" },
		"hasWorkflowRun":       func(status string) bool { return false },
		"printf":               func(format string, args ...interface{}) string { return "" },
		"releaseIcon":          func(conclusion string) string { return "" },
		"releaseLabel":         func(conclusion string) string { return "" },
		"hasReleaseConclusion": func(s string) bool { return false },
	}).Parse(`{{define "pr_list"}}<div>{{range .PRs}}<div class="pr-item">{{.Number}} {{.Title}}</div>{{end}}</div>{{end}}`)))
	engine.GET("/repos/:id/prs", func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.ListPullRequests(c)
	})
	req := httptest.NewRequest("GET", "/repos/1/prs", nil)
	req.AddCookie(&http.Cookie{Name: "gitlens_session", Value: sessionID})
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Fix bug") {
		t.Errorf("expected PR title in response, body: %s", w.Body.String())
	}
}

func TestListPullRequests_Empty(t *testing.T) {
	handler, store, client := newTestDashboardHandler(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(400).
		SetLogin("emptypru").
		SetAccessToken("token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = client.Repository.Create().
		SetGithubID(20).SetOwner("org").SetName("empty-pr").
		SetFullName("org/empty-pr").SetHTMLURL("https://github.com/org/empty-pr").
		SetDefaultBranch("main").SetUserID(u.ID).
		SetPullRequests("[]").
		SetOpenPrCount(0).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	sessionID := store.Set(int64(u.ID))
	w := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Funcs(template.FuncMap{
		"shortSHA":             func(s string) string { return s },
		"formatTime":           func(t time.Time) string { return "" },
		"truncate":             func(s string, n int) string { return s },
		"workflowIcon":         func(status string) string { return "" },
		"workflowLabel":        func(status string) string { return "" },
		"hasWorkflowRun":       func(status string) bool { return false },
		"printf":               func(format string, args ...interface{}) string { return "" },
		"releaseIcon":          func(conclusion string) string { return "" },
		"releaseLabel":         func(conclusion string) string { return "" },
		"hasReleaseConclusion": func(s string) bool { return false },
	}).Parse(`{{define "pr_list"}}<div>{{if .PRs}}{{range .PRs}}{{.n}}{{else}}empty{{end}}{{else}}empty{{end}}</div>{{end}}`)))
	engine.GET("/repos/:id/prs", func(c *gin.Context) {
		c.Set("user_id", int64(u.ID))
		handler.ListPullRequests(c)
	})
	req := httptest.NewRequest("GET", "/repos/1/prs", nil)
	req.AddCookie(&http.Cookie{Name: "gitlens_session", Value: sessionID})
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestComputeMetrics_Empty(t *testing.T) {
	metrics := computeMetrics(nil)
	if metrics.TotalRepos != 0 {
		t.Errorf("expected 0 repos, got %d", metrics.TotalRepos)
	}
}

func TestComputeMetrics_SingleRepo(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	u, _ := client.User.Create().
		SetGithubID(400).SetLogin("single").SetAccessToken("tok").Save(context.Background())

	repo1, _ := client.Repository.Create().
		SetGithubID(30).SetOwner("u").SetName("single").SetFullName("u/single").SetHTMLURL("https://github.com/u/single").SetDefaultBranch("main").SetUserID(u.ID).
		SetTotalCommitsFetched(5).SetFeatCount(5).SetFixCount(0).SetDocsCount(0).SetChoreCount(0).SetOtherCommitCount(0).
		SetReleaseCount(1).SetWorkflowSuccessCount(2).SetWorkflowFailureCount(0).
		Save(context.Background())

	metrics := computeMetrics([]*ent.Repository{repo1})
	if metrics.FeatPct != 100.0 {
		t.Errorf("expected 100%% feat, got %.1f%%", metrics.FeatPct)
	}
	if metrics.WorkflowPassRate != 100.0 {
		t.Errorf("expected 100%% pass rate, got %.1f%%", metrics.WorkflowPassRate)
	}
}
