package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlens/ent"
	"gitlens/ent/enttest"
	"gitlens/ent/repository"
	"gitlens/internal/github"
	"gitlens/internal/ws"

	_ "github.com/mattn/go-sqlite3"
)

func newTestSyncer(t *testing.T) (*Syncer, *ent.Client) {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	ghClient := github.NewClient("", "")
	hub := ws.NewHub()
	syncer := NewSyncer(client, ghClient, hub)
	return syncer, client
}

func TestSyncOne_AllAPIs(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(100).
		SetLogin("testuser").
		SetAccessToken("test_token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo, err := client.Repository.Create().
		SetGithubID(1).SetOwner("user").SetName("testrepo").
		SetFullName("user/testrepo").SetHTMLURL("https://github.com/user/testrepo").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	apiPages := make(map[string]int)
	apiCalls := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		w.Header().Set("Content-Type", "application/json")
		apiPages[r.URL.Path]++
		page := apiPages[r.URL.Path]
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits"):
			w.Write([]byte(`[
				{"sha":"a1b2c3","commit":{"message":"feat: add feature","committer":{"date":"2024-06-01T10:00:00Z"}}},
				{"sha":"d4e5f6","commit":{"message":"fix: bug fix","committer":{"date":"2024-06-02T10:00:00Z"}}}
			]`))
		case strings.HasSuffix(r.URL.Path, "/releases"):
			if page == 1 {
				w.Write([]byte(`[
					{"tag_name":"v1.0.0","name":"Version 1","published_at":"2024-06-15T10:00:00Z"}
				]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case strings.HasSuffix(r.URL.Path, "/actions/runs"):
			w.Write([]byte(`{"workflow_runs":[
				{"id":1,"status":"completed","conclusion":"success"},
				{"id":2,"status":"completed","conclusion":"failure"}
			]}`))
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			w.Write([]byte(`[
				{"number":1,"title":"Fix issue","user":{"login":"dev1"},"created_at":"2024-06-10T08:00:00Z","html_url":"https://github.com/user/testrepo/pull/1","head":{"ref":"fix-branch"},"base":{"ref":"main"}}
			]`))
		default:
			t.Logf("Unexpected API call: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	result := syncer.SyncOne(ctx, repo)
	if result == nil {
		t.Fatal("SyncOne returned nil")
	}

	// Reload from DB
	synced, err := client.Repository.Get(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get synced repo: %v", err)
	}

	// Verify commit data
	if synced.TotalCommitsFetched != 2 {
		t.Errorf("expected 2 commits fetched, got %d", synced.TotalCommitsFetched)
	}
	if synced.LatestCommitSha != "a1b2c3" {
		t.Errorf("expected latest commit SHA 'a1b2c3', got '%s'", synced.LatestCommitSha)
	}
	if synced.LatestCommitMessage != "feat: add feature" {
		t.Errorf("expected latest commit message 'feat: add feature', got '%s'", synced.LatestCommitMessage)
	}
	if synced.LatestCommitDate.IsZero() {
		t.Error("expected latest commit date to be set")
	}
	if synced.FeatCount != 1 {
		t.Errorf("expected 1 feat, got %d", synced.FeatCount)
	}
	if synced.FixCount != 1 {
		t.Errorf("expected 1 fix, got %d", synced.FixCount)
	}

	// Verify release data
	if synced.LatestReleaseTag != "v1.0.0" {
		t.Errorf("expected v1.0.0, got %s", synced.LatestReleaseTag)
	}
	if synced.LatestReleaseName != "Version 1" {
		t.Errorf("expected 'Version 1', got '%s'", synced.LatestReleaseName)
	}
	if synced.ReleaseCount != 1 {
		t.Errorf("expected 1 release, got %d", synced.ReleaseCount)
	}

	// Verify workflow data
	if synced.WorkflowSuccessCount != 1 {
		t.Errorf("expected 1 workflow success, got %d", synced.WorkflowSuccessCount)
	}
	if synced.WorkflowFailureCount != 1 {
		t.Errorf("expected 1 workflow failure, got %d", synced.WorkflowFailureCount)
	}
	if synced.WorkflowStatus != "success" {
		t.Errorf("expected workflow status 'success', got '%s'", synced.WorkflowStatus)
	}
	if synced.WorkflowRunID != 1 {
		t.Errorf("expected workflow run id 1, got %d", synced.WorkflowRunID)
	}

	// Verify PR data
	if synced.OpenPrCount != 1 {
		t.Errorf("expected 1 open PR, got %d", synced.OpenPrCount)
	}

	// Verify sync time was set
	if synced.SyncedAt.IsZero() {
		t.Error("expected synced_at to be set")
	}

	// Verify API was called
	if apiCalls < 4 {
		t.Errorf("expected at least 4 API calls (commits, releases, workflows, pulls), got %d", apiCalls)
	}
}

func TestSyncOne_EmptyRepo(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(200).
		SetLogin("emptytest").
		SetAccessToken("token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo, err := client.Repository.Create().
		SetGithubID(2).SetOwner("user").SetName("empty").
		SetFullName("user/empty").SetHTMLURL("https://github.com/user/empty").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	apiPages := make(map[string]int)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		apiPages[r.URL.Path]++
		page := apiPages[r.URL.Path]
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits"):
			w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/releases"):
			if page == 1 {
				w.Write([]byte(`[]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case strings.HasSuffix(r.URL.Path, "/actions/runs"):
			w.Write([]byte(`{"workflow_runs":[]}`))
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			w.Write([]byte(`[]`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	result := syncer.SyncOne(ctx, repo)
	if result == nil {
		t.Fatal("SyncOne returned nil")
	}

	synced, err := client.Repository.Get(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get synced repo: %v", err)
	}

	if synced.TotalCommitsFetched != 0 {
		t.Errorf("expected 0 commits, got %d", synced.TotalCommitsFetched)
	}
	if synced.ReleaseCount != 0 {
		t.Errorf("expected 0 releases, got %d", synced.ReleaseCount)
	}
	if synced.WorkflowSuccessCount != 0 || synced.WorkflowFailureCount != 0 {
		t.Errorf("expected 0 workflow results, got success=%d failure=%d", synced.WorkflowSuccessCount, synced.WorkflowFailureCount)
	}
	if synced.OpenPrCount != 0 {
		t.Errorf("expected 0 open PRs, got %d", synced.OpenPrCount)
	}
	if synced.SyncedAt.IsZero() {
		t.Error("expected synced_at to be set after sync")
	}
}

func TestSyncOne_ReleaseWorkflowFallback(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(300).
		SetLogin("releasetest").
		SetAccessToken("token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo, err := client.Repository.Create().
		SetGithubID(3).SetOwner("user").SetName("release-repo").
		SetFullName("user/release-repo").SetHTMLURL("https://github.com/user/release-repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	apiPages := make(map[string]int)
	apiCalls := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalls++
		w.Header().Set("Content-Type", "application/json")
		apiPages[r.URL.Path]++
		page := apiPages[r.URL.Path]
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits"):
			w.Write([]byte(`[{"sha":"abc","commit":{"message":"feat: release","committer":{"date":"2024-06-01T10:00:00Z"}}}]`))
		case strings.HasSuffix(r.URL.Path, "/releases"):
			if page == 1 {
				w.Write([]byte(`[{"tag_name":"v2.0.0","name":"Version 2","published_at":"2024-06-15T10:00:00Z"}]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case strings.HasSuffix(r.URL.Path, "/actions/runs"):
			// First call with tag branch fails, should fallback to default branch
			if r.URL.Query().Get("branch") == "v2.0.0" {
				w.Write([]byte(`{"workflow_runs":[]}`)) // No runs for tag
			} else {
				w.Write([]byte(`{"workflow_runs":[{"id":5,"status":"completed","conclusion":"success"}]}`))
			}
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			w.Write([]byte(`[]`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	result := syncer.SyncOne(ctx, repo)
	if result == nil {
		t.Fatal("SyncOne returned nil")
	}

	synced, err := client.Repository.Get(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get synced repo: %v", err)
	}

	if synced.LatestReleaseConclusion != "success" {
		t.Errorf("expected release conclusion 'success' after fallback, got '%s'", synced.LatestReleaseConclusion)
	}
}

func TestSyncOne_ReleaseConclusionUnknown(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(400).
		SetLogin("noreleaserun").
		SetAccessToken("token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo, err := client.Repository.Create().
		SetGithubID(4).SetOwner("user").SetName("no-wf").
		SetFullName("user/no-wf").SetHTMLURL("https://github.com/user/no-wf").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	apiPages := make(map[string]int)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		apiPages[r.URL.Path]++
		page := apiPages[r.URL.Path]
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits"):
			w.Write([]byte(`[{"sha":"abc","commit":{"message":"chore: init","committer":{"date":"2024-01-01T00:00:00Z"}}}]`))
		case strings.HasSuffix(r.URL.Path, "/releases"):
			if page == 1 {
				w.Write([]byte(`[{"tag_name":"v1.0.0","name":"v1","published_at":"2024-06-01T00:00:00Z"}]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case strings.HasSuffix(r.URL.Path, "/actions/runs"):
			w.Write([]byte(`{"workflow_runs":[]}`)) // No runs at all
		default:
			w.Write([]byte(`[]`))
		}
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	result := syncer.SyncOne(ctx, repo)
	if result == nil {
		t.Fatal("SyncOne returned nil")
	}

	synced, err := client.Repository.Get(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get synced repo: %v", err)
	}

	if synced.LatestReleaseConclusion != "unknown" {
		t.Errorf("expected 'unknown' release conclusion, got '%s'", synced.LatestReleaseConclusion)
	}
}

func TestPullRequestsJSON(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(500).
		SetLogin("prtest").
		SetAccessToken("token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo, err := client.Repository.Create().
		SetGithubID(5).SetOwner("user").SetName("pr-test").
		SetFullName("user/pr-test").SetHTMLURL("https://github.com/user/pr-test").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	apiPages := make(map[string]int)
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		apiPages[r.URL.Path]++
		page := apiPages[r.URL.Path]
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits"):
			w.Write([]byte(`[{"sha":"a","commit":{"message":"fix: stuff","committer":{"date":"2024-01-01T00:00:00Z"}}}]`))
		case strings.HasSuffix(r.URL.Path, "/releases"):
			if page == 1 {
				w.Write([]byte(`[]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case strings.HasSuffix(r.URL.Path, "/actions/runs"):
			w.Write([]byte(`{"workflow_runs":[]}`))
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			w.Write([]byte(`[
				{"number":1,"title":"PR One","user":{"login":"dev1"},"created_at":"2024-06-01T08:00:00Z","html_url":"https://github.com/user/pr-test/pull/1","head":{"ref":"feature-a"},"base":{"ref":"main"}},
				{"number":2,"title":"PR Two","user":{"login":"dev2"},"created_at":"2024-06-02T08:00:00Z","html_url":"https://github.com/user/pr-test/pull/2","head":{"ref":"feature-b"},"base":{"ref":"main"}}
			]`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	syncer.SyncOne(ctx, repo)

	synced, err := client.Repository.Get(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get synced repo: %v", err)
	}

	if synced.OpenPrCount != 2 {
		t.Errorf("expected 2 open PRs, got %d", synced.OpenPrCount)
	}
	if synced.PullRequests == "" {
		t.Fatal("expected pull_requests JSON to be set")
	}
	if len(synced.PullRequests) < 10 {
		t.Errorf("pull_requests JSON too short: %s", synced.PullRequests)
	}
}

func TestSyncOneByGithubID(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(600).
		SetLogin("syncbyid").
		SetAccessToken("token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = client.Repository.Create().
		SetGithubID(6).SetOwner("user").SetName("byid-repo").
		SetFullName("user/byid-repo").SetHTMLURL("https://github.com/user/byid-repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/commits"):
			w.Write([]byte(`[{"sha":"abc","commit":{"message":"feat: init","committer":{"date":"2024-01-01T00:00:00Z"}}}]`))
		case strings.HasSuffix(r.URL.Path, "/releases"):
			w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/actions/runs"):
			w.Write([]byte(`{"workflow_runs":[]}`))
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			w.Write([]byte(`[]`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	syncer.SyncOneByGithubID(ctx, 6)

	synced, err := client.Repository.Query().Where(repository.GithubID(6)).Only(ctx)
	if err != nil {
		t.Fatalf("get synced repo: %v", err)
	}
	if synced.TotalCommitsFetched != 1 {
		t.Errorf("expected 1 commit, got %d", synced.TotalCommitsFetched)
	}
}

func TestSyncOneByGithubID_NotFound(t *testing.T) {
	syncer, _ := newTestSyncer(t)
	ctx := context.Background()

	syncer.SyncOneByGithubID(ctx, 99999)
}

func TestSyncAll_SkipsRecentSync(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	_, err := client.User.Create().
		SetGithubID(700).
		SetLogin("skiptest").
		SetAccessToken("token").
		SetSyncIntervalMinutes(5).
		SetSyncedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	calls := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	syncer.SyncAll(ctx)

	if calls != 0 {
		t.Errorf("expected 0 API calls for recently synced user, got %d", calls)
	}
}

func TestSyncAll_SyncsWhenOverdue(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(800).
		SetLogin("overdue").
		SetAccessToken("token").
		SetSyncIntervalMinutes(1).
		SetSyncedAt(time.Now().Add(-10 * time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = client.Repository.Create().
		SetGithubID(8).SetOwner("user").SetName("overdue-repo").
		SetFullName("user/overdue-repo").SetHTMLURL("https://github.com/user/overdue-repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	calls := 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	syncer.SyncAll(ctx)

	if calls < 4 {
		t.Errorf("expected at least 4 API calls for overdue user, got %d", calls)
	}
}

func TestSyncAll_NoUsers(t *testing.T) {
	syncer, _ := newTestSyncer(t)
	ctx := context.Background()

	syncer.SyncAll(ctx)
}

func TestSyncOne_UserNotFound(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(900).
		SetLogin("nouserowner").
		SetAccessToken("token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo, err := client.Repository.Create().
		SetGithubID(9).SetOwner("user").SetName("nouser-repo").
		SetFullName("user/nouser-repo").SetHTMLURL("https://github.com/user/nouser-repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	// Delete the user to simulate missing user
	client.User.DeleteOne(u).Exec(ctx)

	result := syncer.SyncOne(ctx, repo)
	if result == nil {
		t.Fatal("SyncOne returned nil")
	}
}

func TestSyncOne_APIErrors(t *testing.T) {
	syncer, client := newTestSyncer(t)
	ctx := context.Background()

	u, err := client.User.Create().
		SetGithubID(900).
		SetLogin("apierrortest").
		SetAccessToken("token").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	repo, err := client.Repository.Create().
		SetGithubID(9).SetOwner("user").SetName("error-repo").
		SetFullName("user/error-repo").SetHTMLURL("https://github.com/user/error-repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer apiSrv.Close()
	syncer.gh.APIURL = apiSrv.URL

	result := syncer.SyncOne(ctx, repo)
	if result == nil {
		t.Fatal("SyncOne returned nil")
	}

	synced, err := client.Repository.Get(ctx, repo.ID)
	if err != nil {
		t.Fatalf("get synced repo: %v", err)
	}

	if synced.SyncedAt.IsZero() {
		t.Error("expected synced_at to be set even after API errors")
	}
}
