package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gitlens/ent"
	ghclient "gitlens/internal/github"
	"gitlens/internal/provider"

	"github.com/gin-gonic/gin"
)

func newWorkflowTestEngine(t *testing.T, h *DashboardHandler, userID int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.SetHTMLTemplate(template.Must(template.New("").Parse(
		`{{define "workflow_dispatch_modal"}}WF={{.Workflow}};{{range .Repos}}{{.FullName}}|{{.Provider}}|{{.DefaultBranch}};{{end}}{{end}}` +
			`{{define "workflow_dispatch_results"}}ERR={{.Error}};T={{.Total}};OK={{.Succeeded}};F={{.Failed}};S={{.Skipped}}{{range .Results}};{{.FullName}}|{{.Status}}|{{.Ref}}|{{.Message}}{{end}}{{end}}`)))
	authed := engine.Group("/", func(c *gin.Context) { c.Set("user_id", userID); c.Next() })
	authed.GET("/workflows/dispatch", h.WorkflowDispatchModal)
	authed.POST("/workflows/dispatch", h.RunWorkflow)
	return engine
}

var workflowRepoSeq int64

func createWorkflowRepo(t *testing.T, client *ent.Client, userID int, owner, name, providerName, defaultBranch string) *ent.Repository {
	t.Helper()
	repo, err := client.Repository.Create().
		SetGithubID(atomic.AddInt64(&workflowRepoSeq, 1)).
		SetOwner(owner).SetName(name).
		SetFullName(owner + "/" + name).
		SetHTMLURL("https://example.com/" + owner + "/" + name).
		SetDefaultBranch(defaultBranch).
		SetUserID(userID).
		SetProvider(providerName).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	return repo
}

func postWorkflowForm(engine *gin.Engine, values url.Values) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/workflows/dispatch", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	engine.ServeHTTP(w, req)
	return w
}

func TestWorkflowDispatchModal_ListsReposAndPreset(t *testing.T) {
	fake := &mergeFakeProvider{name: "fake"}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	createWorkflowRepo(t, client, u.ID, "zeta", "one", "fake", "main")
	createWorkflowRepo(t, client, u.ID, "alpha", "two", "forgejo", "develop")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/workflows/dispatch?preset=renovate", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "WF=renovate.yml") {
		t.Errorf("expected renovate preset, got: %s", body)
	}
	if !strings.Contains(body, "alpha/two|forgejo|develop") || !strings.Contains(body, "zeta/one|fake|main") {
		t.Errorf("expected both repos, got: %s", body)
	}
	if strings.Index(body, "alpha/two") > strings.Index(body, "zeta/one") {
		t.Errorf("expected repos sorted by full name, got: %s", body)
	}
}

func TestRunWorkflow_DispatchesSelectedReposWithDefaultRefs(t *testing.T) {
	fake := &mergeFakeProvider{name: "fake"}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	repo1 := createWorkflowRepo(t, client, u.ID, "acme", "one", "fake", "main")
	repo2 := createWorkflowRepo(t, client, u.ID, "acme", "two", "fake", "develop")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))

	w := postWorkflowForm(engine, url.Values{
		"workflow": {"renovate.yml"},
		"inputs":   {`{"dryRun":true}`},
		"repo_ids": {strconv.Itoa(repo1.ID), strconv.Itoa(repo2.ID)},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "T=2;OK=2;F=0;S=0") {
		t.Errorf("expected two successes, got: %s", body)
	}
	if len(fake.dispatchCalls) != 2 {
		t.Fatalf("expected two dispatch calls, got %v", fake.dispatchCalls)
	}
	joined := strings.Join(fake.dispatchCalls, ",")
	if !strings.Contains(joined, "acme/one@renovate.yml:main") || !strings.Contains(joined, "acme/two@renovate.yml:develop") {
		t.Errorf("unexpected dispatch calls: %v", fake.dispatchCalls)
	}
}

func TestRunWorkflow_ExplicitRefOverridesDefaults(t *testing.T) {
	fake := &mergeFakeProvider{name: "fake"}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	repo := createWorkflowRepo(t, client, u.ID, "acme", "one", "fake", "main")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))

	w := postWorkflowForm(engine, url.Values{
		"workflow": {"ci.yaml"},
		"ref":      {"release/1.2"},
		"repo_ids": {strconv.Itoa(repo.ID)},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.dispatchCalls) != 1 || fake.dispatchCalls[0] != "acme/one@ci.yaml:release/1.2" {
		t.Errorf("expected explicit ref, got %v", fake.dispatchCalls)
	}
}

func TestRunWorkflow_NormalizesWorkflowPrefix(t *testing.T) {
	fake := &mergeFakeProvider{name: "fake"}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	repo := createWorkflowRepo(t, client, u.ID, "acme", "one", "fake", "main")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))

	w := postWorkflowForm(engine, url.Values{
		"workflow": {".github/workflows/renovate.yml"},
		"repo_ids": {strconv.Itoa(repo.ID)},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.dispatchCalls) != 1 || fake.dispatchCalls[0] != "acme/one@renovate.yml:main" {
		t.Errorf("expected normalized workflow name, got %v", fake.dispatchCalls)
	}
}

func TestRunWorkflow_DuplicateRepoIDsDispatchOnce(t *testing.T) {
	fake := &mergeFakeProvider{name: "fake"}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	repo := createWorkflowRepo(t, client, u.ID, "acme", "one", "fake", "main")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))
	id := strconv.Itoa(repo.ID)

	w := postWorkflowForm(engine, url.Values{
		"workflow": {"renovate.yml"},
		"repo_ids": {id, id},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(fake.dispatchCalls) != 1 {
		t.Errorf("expected one dispatch call, got %v", fake.dispatchCalls)
	}
	if !strings.Contains(w.Body.String(), "T=1;OK=1") {
		t.Errorf("expected one result, got: %s", w.Body.String())
	}
}

func TestRunWorkflow_ForgejoRepoIsSkipped(t *testing.T) {
	fake := &mergeFakeProvider{name: "fake"}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	repo := createWorkflowRepo(t, client, u.ID, "acme", "fj", "forgejo", "main")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))

	w := postWorkflowForm(engine, url.Values{
		"workflow": {"renovate.yml"},
		"repo_ids": {strconv.Itoa(repo.ID)},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "T=1;OK=0;F=0;S=1") || !strings.Contains(body, "not supported for Forgejo") {
		t.Errorf("expected Forgejo skip, got: %s", body)
	}
	if len(fake.dispatchCalls) != 0 {
		t.Errorf("expected no provider calls, got %v", fake.dispatchCalls)
	}
}

func TestRunWorkflow_ProviderUnsupportedIsSkipped(t *testing.T) {
	fake := &mergeFakeProvider{
		name: "fake",
		dispatchFn: func(owner, repo, workflow, ref string, inputs map[string]any) error {
			return provider.ErrUnsupported
		},
	}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	repo := createWorkflowRepo(t, client, u.ID, "acme", "one", "fake", "main")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))

	w := postWorkflowForm(engine, url.Values{
		"workflow": {"renovate.yml"},
		"repo_ids": {strconv.Itoa(repo.ID)},
	})
	body := w.Body.String()
	if !strings.Contains(body, "T=1;OK=0;F=0;S=1") || !strings.Contains(body, "not supported for this provider") {
		t.Errorf("expected unsupported skip, got: %s", body)
	}
}

func TestRunWorkflow_PartialFailureIsReportedAndContinues(t *testing.T) {
	fake := &mergeFakeProvider{
		name: "fake",
		dispatchFn: func(owner, repo, workflow, ref string, inputs map[string]any) error {
			if repo == "missing" {
				return &ghclient.APIError{StatusCode: http.StatusNotFound, Status: "404 Not Found"}
			}
			return nil
		},
	}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	good := createWorkflowRepo(t, client, u.ID, "acme", "good", "fake", "main")
	bad := createWorkflowRepo(t, client, u.ID, "acme", "missing", "fake", "main")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))

	w := postWorkflowForm(engine, url.Values{
		"workflow": {"renovate.yml"},
		"repo_ids": {strconv.Itoa(good.ID), strconv.Itoa(bad.ID)},
	})
	body := w.Body.String()
	if !strings.Contains(body, "T=2;OK=1;F=1;S=0") {
		t.Errorf("expected one success and one failure, got: %s", body)
	}
	if !strings.Contains(body, "Workflow not found on the default branch") {
		t.Errorf("expected friendly 404 message, got: %s", body)
	}
	if len(fake.dispatchCalls) != 2 {
		t.Errorf("expected both repos attempted, got %v", fake.dispatchCalls)
	}
}

func TestRunWorkflow_ForeignRepoIsRejected(t *testing.T) {
	fake := &mergeFakeProvider{name: "fake"}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	other, err := client.User.Create().
		SetGithubID(8888).SetLogin("other").SetAccessToken("tok").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	foreign := createWorkflowRepo(t, client, other.ID, "other", "repo", "fake", "main")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))

	w := postWorkflowForm(engine, url.Values{
		"workflow": {"renovate.yml"},
		"repo_ids": {strconv.Itoa(foreign.ID)},
	})
	body := w.Body.String()
	if !strings.Contains(body, "ERR=One or more selected repositories were not found.") {
		t.Errorf("expected ownership error, got: %s", body)
	}
	if len(fake.dispatchCalls) != 0 {
		t.Errorf("expected no dispatch calls, got %v", fake.dispatchCalls)
	}
}

func TestRunWorkflow_ValidationErrors(t *testing.T) {
	fake := &mergeFakeProvider{name: "fake"}
	h, client := newMergeTestHandler(t, map[string]provider.Provider{"fake": fake})
	u := createMergeUser(t, client)
	repo := createWorkflowRepo(t, client, u.ID, "acme", "one", "fake", "main")
	engine := newWorkflowTestEngine(t, h, int64(u.ID))
	id := strconv.Itoa(repo.ID)

	tooMany := make(map[string]any, maxWorkflowDispatchInputs+1)
	for i := 0; i <= maxWorkflowDispatchInputs; i++ {
		tooMany[fmt.Sprintf("k%d", i)] = "v"
	}
	tooManyJSON := mustJSON(t, tooMany)

	cases := []struct {
		name   string
		values url.Values
		want   string
	}{
		{"missing workflow", url.Values{"repo_ids": {id}}, "Enter a workflow file name"},
		{"no repos", url.Values{"workflow": {"renovate.yml"}}, "Select at least one repository"},
		{"invalid inputs", url.Values{"workflow": {"renovate.yml"}, "repo_ids": {id}, "inputs": {"["}}, "valid JSON object"},
		{"nested input", url.Values{"workflow": {"renovate.yml"}, "repo_ids": {id}, "inputs": {`{"a":{"b":1}}`}}, "must be a string, number, or boolean"},
		{"too many inputs", url.Values{"workflow": {"renovate.yml"}, "repo_ids": {id}, "inputs": {tooManyJSON}}, "at most 10"},
		{"nested path", url.Values{"workflow": {".github/workflows/sub/renovate.yml"}, "repo_ids": {id}}, "must be a file name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := postWorkflowForm(engine, tc.values)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 partial, got %d: %s", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), tc.want) {
				t.Errorf("expected %q, got: %s", tc.want, w.Body.String())
			}
		})
	}
	if len(fake.dispatchCalls) != 0 {
		t.Errorf("expected no dispatch calls, got %v", fake.dispatchCalls)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func TestWorkflowTemplatesRender(t *testing.T) {
	tmpl := template.Must(template.ParseFiles("../../views/workflow.html"))

	var buf bytes.Buffer
	err := tmpl.ExecuteTemplate(&buf, "workflow_dispatch_modal", workflowDispatchModalData{
		Workflow: "renovate.yml",
		Repos: []workflowDispatchRepo{
			{ID: 1, FullName: "acme/one", Provider: "github", DefaultBranch: "main"},
			{ID: 2, FullName: "acme/two", Provider: "forgejo", DefaultBranch: "main"},
		},
	})
	if err != nil {
		t.Fatalf("render modal: %v", err)
	}
	modal := buf.String()
	for _, want := range []string{"renovate.yml", "acme/one", "acme/two", "unsupported", "workflow-dispatch-modal"} {
		if !strings.Contains(modal, want) {
			t.Errorf("modal missing %q", want)
		}
	}

	buf.Reset()
	err = tmpl.ExecuteTemplate(&buf, "workflow_dispatch_results", workflowDispatchResultsData{
		Total:     2,
		Succeeded: 1,
		Failed:    1,
		Results: []workflowDispatchResult{
			{FullName: "acme/one", Status: "success", Ref: "main", Message: "Workflow run queued."},
			{FullName: "acme/two", Status: "failed", Ref: "main", Message: "Permission denied."},
		},
	})
	if err != nil {
		t.Fatalf("render results: %v", err)
	}
	results := buf.String()
	for _, want := range []string{"Dispatched to 1 of 2 repositories", "acme/one", "failed", "Permission denied."} {
		if !strings.Contains(results, want) {
			t.Errorf("results missing %q", want)
		}
	}
}
