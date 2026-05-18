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
	c.tokenURL = srv.URL

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
	c.tokenURL = srv.URL

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
	c.apiURL = srv.URL

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
	c.apiURL = srv.URL

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
	c.apiURL = srv.URL

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
	c.apiURL = srv.URL

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
	c.apiURL = srv.URL

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
	c.apiURL = srv.URL

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
	c.apiURL = srv.URL

	_, err := c.GetLatestRelease("token", "user", "repo")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestGetWorkflowStatus_NoRuns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"workflow_runs":[]}`))
	}))
	defer srv.Close()

	c := NewClient("", "")
	c.apiURL = srv.URL

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
	c.apiURL = srv.URL

	commit, err := c.GetLatestCommit("token", "user", "repo", "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if commit.Message != "feat: add stuff" {
		t.Errorf("expected 'feat: add stuff', got '%s'", commit.Message)
	}
}
