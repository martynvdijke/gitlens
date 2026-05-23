package github

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAccessToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"test_token_123"}`))
	}))
	defer srv.Close()

	c := NewClient("client_id", "client_secret")
	c.TokenURL = srv.URL

	token, err := c.GetAccessToken("code123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "test_token_123" {
		t.Fatalf("expected test_token_123, got %s", token)
	}
}

func TestGetAccessTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"error":"bad_verification_code"}`))
	}))
	defer srv.Close()

	c := NewClient("client_id", "client_secret")
	c.TokenURL = srv.URL

	_, err := c.GetAccessToken("bad_code")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test_token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":12345,"login":"octocat","avatar_url":"https://avatars.example.com/u/12345","name":"Octo Cat"}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	u, err := c.GetUser("test_token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.ID != 12345 || u.Login != "octocat" {
		t.Fatalf("unexpected user: %+v", u)
	}
}

func TestListRepositories(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			w.Write([]byte(`[
				{"id":1,"name":"repo1","full_name":"user/repo1","html_url":"https://github.com/user/repo1","default_branch":"main","owner":{"login":"user"}},
				{"id":2,"name":"repo2","full_name":"user/repo2","html_url":"https://github.com/user/repo2","default_branch":"main","language":"Go","owner":{"login":"user"}}
			]`))
		} else {
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	repos, err := c.ListRepositories("test_token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	if repos[0].FullName != "user/repo1" {
		t.Errorf("expected user/repo1, got %s", repos[0].FullName)
	}
}

func TestGetLatestCommit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"sha":"abc123def456",
				"commit":{
					"message":"Fix the thing\n\nMore details here",
					"committer":{"date":"2024-06-15T10:30:00Z"}
				}
			}
		]`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	commit, err := c.GetLatestCommit("token", "user", "repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.SHA != "abc123def456" {
		t.Errorf("expected abc123def456, got %s", commit.SHA)
	}
	if commit.Message != "Fix the thing" {
		t.Errorf("expected 'Fix the thing', got '%s'", commit.Message)
	}
	if commit.Date.IsZero() {
		t.Error("expected non-zero date")
	}
}

func TestGetLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tag_name":"v1.0.0","name":"Version 1.0.0","published_at":"2024-01-15T08:00:00Z"}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	release, err := c.GetLatestRelease("token", "user", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if release.TagName != "v1.0.0" {
		t.Errorf("expected v1.0.0, got %s", release.TagName)
	}
}

func TestGetWorkflowStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workflow_runs":[{"id":42,"status":"completed","conclusion":"success"}]}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	run, err := c.GetWorkflowStatus("token", "user", "repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.ID != 42 || run.Conclusion != "success" {
		t.Errorf("unexpected run: %+v", run)
	}
}

func TestGetLatestCommit_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	_, err := c.GetLatestCommit("token", "user", "repo", "main")
	if err == nil {
		t.Fatal("expected error for empty commits list")
	}
}

func TestGetLatestRelease_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	_, err := c.GetLatestRelease("token", "user", "repo")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestGetLatestWorkflowRun_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workflow_runs":[{"id":99,"status":"completed","conclusion":"success"}]}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	run, err := c.GetLatestWorkflowRun("token", "user", "repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.ID != 99 || run.Conclusion != "success" {
		t.Errorf("expected id=99, conclusion=success, got %+v", run)
	}
}

func TestGetLatestWorkflowRun_WithTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		branch := r.URL.Query().Get("branch")
		if branch != "v1.0.0" {
			t.Errorf("expected branch=v1.0.0, got %s", branch)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workflow_runs":[{"id":42,"status":"completed","conclusion":"success"}]}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	run, err := c.GetLatestWorkflowRun("token", "user", "repo", "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.ID != 42 {
		t.Errorf("expected id=42, got %d", run.ID)
	}
}

func TestGetLatestWorkflowRun_NoRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	_, err := c.GetLatestWorkflowRun("token", "user", "repo", "main")
	if err == nil {
		t.Fatal("expected error for empty workflow runs")
	}
}

func TestGetLatestWorkflowRun_EmptyBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		branch := r.URL.Query().Get("branch")
		if branch != "" {
			t.Errorf("expected empty branch, got %s", branch)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workflow_runs":[{"id":7,"status":"completed","conclusion":"failure"}]}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	run, err := c.GetLatestWorkflowRun("token", "user", "repo", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if run.Conclusion != "failure" {
		t.Errorf("expected failure, got %s", run.Conclusion)
	}
}

func TestGetWorkflowStatus_NoRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	_, err := c.GetWorkflowStatus("token", "user", "repo", "main")
	if err == nil {
		t.Fatal("expected error for empty workflow runs")
	}
}

func TestTruncateNewlineInCommitMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{
				"sha":"abc",
				"commit":{
					"message":"feat: add stuff\n\nCo-authored-by: someone",
					"committer":{"date":"2024-01-01T00:00:00Z"}
				}
			}
		]`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	commit, err := c.GetLatestCommit("token", "user", "repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Message != "feat: add stuff" {
		t.Errorf("expected 'feat: add stuff', got '%s'", commit.Message)
	}
}

func TestParseCommitType_Feat(t *testing.T) {
	if tp := ParseCommitType("feat: add login"); tp != "feat" {
		t.Errorf("expected feat, got %s", tp)
	}
}

func TestParseCommitType_FeatScope(t *testing.T) {
	if tp := ParseCommitType("feat(auth): add login"); tp != "feat" {
		t.Errorf("expected feat, got %s", tp)
	}
}

func TestParseCommitType_Feature(t *testing.T) {
	if tp := ParseCommitType("feature: big thing"); tp != "feat" {
		t.Errorf("expected feat, got %s", tp)
	}
}

func TestParseCommitType_Fix(t *testing.T) {
	if tp := ParseCommitType("fix: crash on load"); tp != "fix" {
		t.Errorf("expected fix, got %s", tp)
	}
}

func TestParseCommitType_FixScope(t *testing.T) {
	if tp := ParseCommitType("fix(parser): handle null"); tp != "fix" {
		t.Errorf("expected fix, got %s", tp)
	}
}

func TestParseCommitType_Bug(t *testing.T) {
	if tp := ParseCommitType("bug: wrong color"); tp != "fix" {
		t.Errorf("expected fix, got %s", tp)
	}
}

func TestParseCommitType_Docs(t *testing.T) {
	if tp := ParseCommitType("docs: update readme"); tp != "docs" {
		t.Errorf("expected docs, got %s", tp)
	}
}

func TestParseCommitType_Chore(t *testing.T) {
	if tp := ParseCommitType("chore: bump deps"); tp != "chore" {
		t.Errorf("expected chore, got %s", tp)
	}
}

func TestParseCommitType_Refactor(t *testing.T) {
	if tp := ParseCommitType("refactor: clean up"); tp != "chore" {
		t.Errorf("expected chore, got %s", tp)
	}
}

func TestParseCommitType_NoPrefix(t *testing.T) {
	if tp := ParseCommitType("fix stuff"); tp != "other" {
		t.Errorf("expected other, got %s", tp)
	}
}

func TestParseCommitType_Empty(t *testing.T) {
	if tp := ParseCommitType(""); tp != "other" {
		t.Errorf("expected other, got %s", tp)
	}
}

func TestParseCommitType_UpperCase(t *testing.T) {
	if tp := ParseCommitType("FEAT: add"); tp != "feat" {
		t.Errorf("expected feat, got %s", tp)
	}
}

func TestGetCommits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"sha":"a","commit":{"message":"feat: one","committer":{"date":"2024-01-01T00:00:00Z"}}},
			{"sha":"b","commit":{"message":"fix: two","committer":{"date":"2024-01-02T00:00:00Z"}}},
			{"sha":"c","commit":{"message":"chore: three","committer":{"date":"2024-01-03T00:00:00Z"}}}
		]`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	commits, err := c.GetCommits("token", "user", "repo", "main", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(commits))
	}
	if commits[0].Message != "feat: one" {
		t.Errorf("expected 'feat: one', got '%s'", commits[0].Message)
	}
}

func TestListReleases(t *testing.T) {
	page := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		w.Header().Set("Content-Type", "application/json")
		if page == 1 {
			w.Write([]byte(`[
				{"tag_name":"v2","name":"Two","published_at":"2024-06-01T00:00:00Z"},
				{"tag_name":"v1","name":"One","published_at":"2024-01-01T00:00:00Z"}
			]`))
		} else {
			w.Write([]byte(`[]`))
		}
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	releases, err := c.ListReleases("token", "user", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(releases))
	}
	if releases[0].TagName != "v2" {
		t.Errorf("expected v2 first, got %s", releases[0].TagName)
	}
}

func TestGetWorkflowRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workflow_runs":[
			{"id":1,"status":"completed","conclusion":"success"},
			{"id":2,"status":"completed","conclusion":"failure"}
		]}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	runs, err := c.GetWorkflowRuns("token", "user", "repo", "main", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].Conclusion != "success" || runs[1].Conclusion != "failure" {
		t.Errorf("unexpected conclusions: %+v", runs)
	}
}

func TestListReleases_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.APIURL = srv.URL

	releases, err := c.ListReleases("token", "user", "repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(releases) != 0 {
		t.Fatalf("expected 0 releases, got %d", len(releases))
	}
}
