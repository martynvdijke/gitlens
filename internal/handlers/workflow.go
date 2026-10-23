package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"

	"gitlens/ent"
	"gitlens/ent/repository"
	"gitlens/ent/user"
	"gitlens/internal/github"
	"gitlens/internal/provider"

	"github.com/gin-gonic/gin"
)

const (
	// workflowDispatchConcurrency bounds the number of in-flight GitHub
	// dispatch requests so a large selection does not burst the API.
	workflowDispatchConcurrency = 4
	// GitHub accepts at most 10 workflow_dispatch inputs per run.
	maxWorkflowDispatchInputs = 10
)

type workflowDispatchRepo struct {
	ID            int
	FullName      string
	Provider      string
	DefaultBranch string
}

type workflowDispatchModalData struct {
	Repos    []workflowDispatchRepo
	Workflow string
	Error    string
}

type workflowDispatchResult struct {
	FullName string
	Provider string
	Ref      string
	Status   string // success, failed, or skipped
	Message  string
}

type workflowDispatchResultsData struct {
	Error     string
	Results   []workflowDispatchResult
	Total     int
	Succeeded int
	Failed    int
	Skipped   int
}

type workflowDispatchRequest struct {
	Workflow string `form:"workflow" json:"workflow"`
	Ref      string `form:"ref" json:"ref"`
	Inputs   string `form:"inputs" json:"inputs"`
	RepoIDs  []int  `form:"repo_ids" json:"repo_ids"`
}

// WorkflowDispatchModal renders the "Run workflow" modal. It lists every
// repository tracked by the signed-in user; Forgejo repos are shown but
// disabled because provider dispatch is unsupported.
func (h *DashboardHandler) WorkflowDispatchModal(c *gin.Context) {
	userID := c.GetInt64("user_id")
	ctx := c.Request.Context()

	repos, err := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		All(ctx)
	if err != nil {
		log.Printf("workflow dispatch modal: loading repositories: %v", err)
		c.HTML(http.StatusOK, "workflow_dispatch_modal", workflowDispatchModalData{Error: "Failed to load repositories."})
		return
	}

	sort.Slice(repos, func(i, j int) bool {
		return strings.ToLower(repos[i].FullName) < strings.ToLower(repos[j].FullName)
	})

	vm := make([]workflowDispatchRepo, 0, len(repos))
	for _, r := range repos {
		p := r.Provider
		if p == "" {
			p = "github"
		}
		vm = append(vm, workflowDispatchRepo{
			ID:            r.ID,
			FullName:      r.FullName,
			Provider:      p,
			DefaultBranch: r.DefaultBranch,
		})
	}

	data := workflowDispatchModalData{Repos: vm}
	if c.Query("preset") == "renovate" {
		data.Workflow = "renovate.yml"
	}
	c.HTML(http.StatusOK, "workflow_dispatch_modal", data)
}

// RunWorkflow dispatches a workflow_dispatch event to each selected
// repository. Individual failures do not abort the remaining dispatches;
// the response renders a per-repo result list.
func (h *DashboardHandler) RunWorkflow(c *gin.Context) {
	userID := c.GetInt64("user_id")
	ctx := c.Request.Context()

	var req workflowDispatchRequest
	if err := c.ShouldBind(&req); err != nil {
		renderWorkflowDispatchError(c, "Invalid request.")
		return
	}

	workflowFile, err := normalizeWorkflowID(req.Workflow)
	if err != nil {
		renderWorkflowDispatchError(c, err.Error())
		return
	}

	repoIDs := uniqueInts(req.RepoIDs)
	if len(repoIDs) == 0 {
		renderWorkflowDispatchError(c, "Select at least one repository.")
		return
	}

	inputs, err := parseWorkflowInputs(req.Inputs)
	if err != nil {
		renderWorkflowDispatchError(c, err.Error())
		return
	}

	repos, err := h.client.Repository.Query().
		Where(
			repository.IDIn(repoIDs...),
			repository.HasUserWith(user.ID(int(userID))),
		).
		All(ctx)
	if err != nil {
		log.Printf("workflow dispatch: loading repositories: %v", err)
		renderWorkflowDispatchError(c, "Failed to load repositories.")
		return
	}
	if len(repos) != len(repoIDs) {
		renderWorkflowDispatchError(c, "One or more selected repositories were not found.")
		return
	}

	u, err := h.client.User.Get(ctx, int(userID))
	if err != nil {
		log.Printf("workflow dispatch: loading user %d: %v", userID, err)
		renderWorkflowDispatchError(c, "Failed to load user.")
		return
	}

	// Preserve the order of the submitted repo IDs so the result list
	// matches the picker even if the database returns rows differently.
	byID := make(map[int]*ent.Repository, len(repos))
	for _, r := range repos {
		byID[r.ID] = r
	}
	ordered := make([]*ent.Repository, 0, len(repoIDs))
	for _, id := range repoIDs {
		if r, ok := byID[id]; ok {
			ordered = append(ordered, r)
		}
	}

	results := h.dispatchWorkflow(ctx, u, ordered, workflowFile, req.Ref, inputs)
	c.HTML(http.StatusOK, "workflow_dispatch_results", results)
}

// RenovateAll dispatches the Renovate workflow to every repository tracked by
// the signed-in user, using each repository's default branch. It is the
// one-click counterpart to the workflow-dispatch modal.
func (h *DashboardHandler) RenovateAll(c *gin.Context) {
	userID := c.GetInt64("user_id")
	ctx := c.Request.Context()

	repos, err := h.client.Repository.Query().
		Where(repository.HasUserWith(user.ID(int(userID)))).
		All(ctx)
	if err != nil {
		log.Printf("renovate all: loading repositories: %v", err)
		renderWorkflowDispatchError(c, "Failed to load repositories.")
		return
	}
	if len(repos) == 0 {
		renderWorkflowDispatchError(c, "No tracked repositories. Import repositories first.")
		return
	}

	sort.Slice(repos, func(i, j int) bool {
		return strings.ToLower(repos[i].FullName) < strings.ToLower(repos[j].FullName)
	})

	u, err := h.client.User.Get(ctx, int(userID))
	if err != nil {
		log.Printf("renovate all: loading user %d: %v", userID, err)
		renderWorkflowDispatchError(c, "Failed to load user.")
		return
	}

	results := h.dispatchWorkflow(ctx, u, repos, "renovate.yml", "", nil)
	c.HTML(http.StatusOK, "workflow_dispatch_results", results)
}

// dispatchWorkflow dispatches workflowFile to every repository in repos,
// honouring the default-branch fallback and skipping providers that do not
// support workflow dispatch. Individual failures do not abort the remaining
// dispatches.
func (h *DashboardHandler) dispatchWorkflow(ctx context.Context, u *ent.User, repos []*ent.Repository, workflowFile, ref string, inputs map[string]any) workflowDispatchResultsData {
	results := make([]workflowDispatchResult, len(repos))
	sem := make(chan struct{}, workflowDispatchConcurrency)
	var wg sync.WaitGroup

	for i, r := range repos {
		repoRef := strings.TrimSpace(ref)
		if repoRef == "" {
			repoRef = r.DefaultBranch
			if repoRef == "" {
				repoRef = "main"
			}
		}
		providerName := r.Provider
		if providerName == "" {
			providerName = "github"
		}

		// Skip Forgejo explicitly rather than relying on the adapter's
		// ErrUnsupported, so a missing provider registration can never
		// fall back to dispatching a Forgejo repo against GitHub.
		if providerName == "forgejo" {
			results[i] = workflowDispatchResult{
				FullName: r.FullName,
				Provider: providerName,
				Ref:      repoRef,
				Status:   "skipped",
				Message:  "Workflow dispatch is not supported for Forgejo repositories.",
			}
			continue
		}

		wg.Add(1)
		go func(i int, r *ent.Repository, ref, providerName string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			p, token := h.syncer.ProviderFor(u, r)
			err := p.DispatchWorkflow(ctx, token, r.Owner, r.Name, workflowFile, ref, inputs)

			result := workflowDispatchResult{
				FullName: r.FullName,
				Provider: providerName,
				Ref:      ref,
			}
			switch {
			case err == nil:
				result.Status = "success"
				result.Message = "Workflow run queued."
			case errors.Is(err, provider.ErrUnsupported):
				result.Status = "skipped"
				result.Message = "Workflow dispatch is not supported for this provider."
			default:
				result.Status = "failed"
				result.Message = friendlyWorkflowDispatchError(err)
				log.Printf("workflow dispatch failed for %s: %v", r.FullName, err)
			}
			results[i] = result
		}(i, r, repoRef, providerName)
	}

	wg.Wait()

	data := workflowDispatchResultsData{Results: results, Total: len(results)}
	for _, r := range results {
		switch r.Status {
		case "success":
			data.Succeeded++
		case "failed":
			data.Failed++
		case "skipped":
			data.Skipped++
		}
	}
	return data
}

// normalizeWorkflowID trims the input and accepts either a bare workflow
// file name or a value pasted with the .github/workflows/ prefix.
func normalizeWorkflowID(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, ".github/workflows/")
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("Enter a workflow file name, for example renovate.yml.")
	}
	if strings.ContainsAny(s, `/\`) || strings.Contains(s, "..") {
		return "", errors.New("Workflow must be a file name (for example renovate.yml) or a numeric workflow ID.")
	}
	return s, nil
}

// parseWorkflowInputs validates the optional JSON inputs textarea. GitHub
// accepts a flat object of string, number, or boolean values.
func parseWorkflowInputs(raw string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return nil, nil
	}

	var inputs map[string]any
	if err := json.Unmarshal([]byte(raw), &inputs); err != nil {
		return nil, errors.New("Inputs must be a valid JSON object.")
	}
	if len(inputs) > maxWorkflowDispatchInputs {
		return nil, fmt.Errorf("GitHub accepts at most %d workflow inputs.", maxWorkflowDispatchInputs)
	}
	for key, value := range inputs {
		if strings.TrimSpace(key) == "" {
			return nil, errors.New("Input names cannot be empty.")
		}
		switch value.(type) {
		case string, bool, float64:
			// Supported workflow_dispatch input value types.
		default:
			return nil, fmt.Errorf("Input %q must be a string, number, or boolean.", key)
		}
	}
	if len(inputs) == 0 {
		return nil, nil
	}
	return inputs, nil
}

func uniqueInts(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func friendlyWorkflowDispatchError(err error) string {
	var rateLimit *github.RateLimitError
	if errors.As(err, &rateLimit) {
		return "GitHub rate limit exhausted; try again later."
	}

	var apiErr *github.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusNotFound:
			return "Workflow not found on the default branch. Check the workflow file name."
		case http.StatusForbidden:
			return "Permission denied. Re-authenticate with access to this repository's Actions."
		case http.StatusUnauthorized:
			return "Authentication failed. Sign in again."
		case http.StatusUnprocessableEntity:
			return "GitHub rejected the ref or inputs. Check the values and try again."
		case http.StatusTooManyRequests:
			return "GitHub rate limit exhausted; try again later."
		}
		return fmt.Sprintf("GitHub returned %s.", apiErr.Status)
	}

	return "Failed to dispatch workflow."
}

func renderWorkflowDispatchError(c *gin.Context, message string) {
	c.HTML(http.StatusOK, "workflow_dispatch_results", workflowDispatchResultsData{Error: message})
}
