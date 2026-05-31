package github

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
	TokenURL     string
	APIURL       string

	RateLimitRemaining int
	RateLimitReset     int64
}

func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		TokenURL: "https://github.com/login/oauth/access_token",
		APIURL:   "https://api.github.com",
	}
}

type ghUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	Name      string `json:"name"`
}

type ghRepo struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description"`
	HTMLURL       string `json:"html_url"`
	Language      string `json:"language"`
	DefaultBranch string `json:"default_branch"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type ghCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message   string `json:"message"`
		Committer struct {
			Date string `json:"date"`
		} `json:"committer"`
	} `json:"commit"`
}

type ghRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
}

type ghWorkflowRuns struct {
	WorkflowRuns []ghWorkflowRun `json:"workflow_runs"`
}

type ghWorkflowRun struct {
	ID         int64  `json:"id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type User struct {
	ID        int64
	Login     string
	AvatarURL string
	Name      string
}

type Repository struct {
	ID            int64
	Owner         string
	Name          string
	FullName      string
	Description   string
	HTMLURL       string
	Language      string
	DefaultBranch string
}

type Commit struct {
	SHA     string
	Message string
	Date    time.Time
}

type Release struct {
	TagName     string
	Name        string
	PublishedAt time.Time
}

type WorkflowRun struct {
	ID         int64
	Conclusion string
}

type PullRequest struct {
	Number    int
	Title     string
	Author    string
	CreatedAt time.Time
	HTMLURL   string
	HeadRef   string
	BaseRef   string
}

type ghPullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	CreatedAt string `json:"created_at"`
	HTMLURL   string `json:"html_url"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	MergeableState string `json:"mergeable_state"`
	MergedAt       string `json:"merged_at"`
	Merged         bool   `json:"merged"`
}

func (c *Client) doRequest(method, urlStr, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("doing request: %w", err)
	}

	if v := resp.Header.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.RateLimitRemaining = n
		}
	}
	if v := resp.Header.Get("X-RateLimit-Reset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.RateLimitReset = n
		}
	}

	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("GitHub API error: %s", resp.Status)
	}
	return resp, nil
}

func (c *Client) GetAccessToken(code string) (string, error) {
	form := url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code":          {code},
	}
	req, err := http.NewRequest("POST", c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchanging code: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("OAuth error: %s", result.Error)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}
	return result.AccessToken, nil
}

func (c *Client) GetUser(token string) (*User, error) {
	resp, err := c.doRequest("GET", c.APIURL+"/user", token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var u ghUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("decoding user: %w", err)
	}
	return &User{
		ID:        u.ID,
		Login:     u.Login,
		AvatarURL: u.AvatarURL,
		Name:      u.Name,
	}, nil
}

func (c *Client) ListRepositories(token string) ([]*Repository, error) {
	var allRepos []*Repository
	page := 1
	for {
		url := fmt.Sprintf("%s/user/repos?per_page=100&page=%d&sort=updated", c.APIURL, page)
		resp, err := c.doRequest("GET", url, token, nil)
		if err != nil {
			return nil, err
		}

		var repos []ghRepo
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding repos: %w", err)
		}
		resp.Body.Close()

		if len(repos) == 0 {
			break
		}

		for _, r := range repos {
			allRepos = append(allRepos, &Repository{
				ID:            r.ID,
				Owner:         r.Owner.Login,
				Name:          r.Name,
				FullName:      r.FullName,
				Description:   r.Description,
				HTMLURL:       r.HTMLURL,
				Language:      r.Language,
				DefaultBranch: r.DefaultBranch,
			})
		}
		page++
	}
	return allRepos, nil
}

func (c *Client) GetLatestCommit(token, owner, repo, branch string) (*Commit, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=1&sha=%s", c.APIURL, owner, repo, branch)
	resp, err := c.doRequest("GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var commits []ghCommit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, fmt.Errorf("decoding commits: %w", err)
	}
	if len(commits) == 0 {
		return nil, fmt.Errorf("no commits found")
	}

	t, _ := time.Parse(time.RFC3339, commits[0].Commit.Committer.Date)
	return &Commit{
		SHA:     commits[0].SHA,
		Message: strings.Split(commits[0].Commit.Message, "\n")[0],
		Date:    t,
	}, nil
}

// getCommitsPage fetches a single page of commits from the GitHub API.
func (c *Client) getCommitsPage(token, owner, repo, branch string, perPage, page int) ([]*Commit, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d&page=%d&sha=%s", c.APIURL, owner, repo, perPage, page, branch)
	resp, err := c.doRequest("GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ghCommits []ghCommit
	if err := json.NewDecoder(resp.Body).Decode(&ghCommits); err != nil {
		return nil, fmt.Errorf("decoding commits: %w", err)
	}

	var commits []*Commit
	for _, c := range ghCommits {
		t, _ := time.Parse(time.RFC3339, c.Commit.Committer.Date)
		commits = append(commits, &Commit{
			SHA:     c.SHA,
			Message: strings.Split(c.Commit.Message, "\n")[0],
			Date:    t,
		})
	}
	return commits, nil
}

// GetCommits fetches up to perPage commits from the default branch (page 1).
// Use GetCommitsSince for paginated or incremental fetching.
func (c *Client) GetCommits(token, owner, repo, branch string, perPage int) ([]*Commit, error) {
	return c.getCommitsPage(token, owner, repo, branch, perPage, 1)
}

// GetCommitsSince fetches commits since a given time with rate-limit awareness.
// Uses GitHub's `since` parameter for incremental fetching.
// If since is zero time, fetches recent commits up to maxCommits.
// Adds inter-page delays and respects X-RateLimit-Remaining headers.
func (c *Client) GetCommitsSince(token, owner, repo, branch string, since time.Time, maxCommits int) ([]*Commit, error) {
	const perPage = 100
	if maxCommits <= 0 || maxCommits > 500 {
		maxCommits = 500
	}

	var allCommits []*Commit
	page := 1

	for len(allCommits) < maxCommits {
		if c.RateLimitRemaining < 10 {
			if c.RateLimitReset > 0 {
				wait := time.Until(time.Unix(c.RateLimitReset, 0))
				if wait > 0 && wait < 60*time.Second {
					log.Printf("Rate limit low (%d remaining), waiting %v before next request...", c.RateLimitRemaining, wait)
					time.Sleep(wait)
				}
			}
		}

		u := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d&page=%d&sha=%s",
			c.APIURL, owner, repo, perPage, page, branch)
		if !since.IsZero() {
			u += "&since=" + url.QueryEscape(since.Format(time.RFC3339))
		}
		resp, err := c.doRequest("GET", u, token, nil)
		if err != nil {
			return allCommits, err
		}

		var ghCommits []ghCommit
		if err := json.NewDecoder(resp.Body).Decode(&ghCommits); err != nil {
			resp.Body.Close()
			return allCommits, fmt.Errorf("decoding commits: %w", err)
		}
		resp.Body.Close()

		if len(ghCommits) == 0 {
			break
		}

		remaining := maxCommits - len(allCommits)
		for i, c := range ghCommits {
			if i >= remaining {
				break
			}
			t, _ := time.Parse(time.RFC3339, c.Commit.Committer.Date)
			allCommits = append(allCommits, &Commit{
				SHA:     c.SHA,
				Message: strings.Split(c.Commit.Message, "\n")[0],
				Date:    t,
			})
		}

		if len(ghCommits) < perPage {
			break
		}

		page++
		time.Sleep(100 * time.Millisecond)
	}

	return allCommits, nil
}

func ParseCommitType(msg string) string {
	// Parse conventional commit prefix like "feat:", "fix:", "feat(scope):", etc.
	msg = strings.TrimSpace(msg)
	colonIdx := strings.Index(msg, ":")
	if colonIdx == -1 {
		return "other"
	}
	prefix := msg[:colonIdx]
	// Handle scope like "feat(scope)"
	if parenIdx := strings.Index(prefix, "("); parenIdx != -1 {
		prefix = prefix[:parenIdx]
	}
	switch strings.ToLower(strings.TrimSpace(prefix)) {
	case "feat", "feature":
		return "feat"
	case "fix", "bugfix", "bug":
		return "fix"
	case "docs", "documentation":
		return "docs"
	case "chore", "refactor", "test", "style", "perf", "ci", "build", "revert":
		return "chore"
	default:
		return "other"
	}
}

func (c *Client) GetLatestRelease(token, owner, repo string) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.APIURL, owner, repo)
	resp, err := c.doRequest("GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}

	t, _ := time.Parse(time.RFC3339, r.PublishedAt)
	return &Release{
		TagName:     r.TagName,
		Name:        r.Name,
		PublishedAt: t,
	}, nil
}

func (c *Client) ListReleases(token, owner, repo string) ([]*Release, error) {
	var all []*Release
	page := 1
	for {
		url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100&page=%d", c.APIURL, owner, repo, page)
		resp, err := c.doRequest("GET", url, token, nil)
		if err != nil {
			return all, err
		}

		var ghReleases []ghRelease
		if err := json.NewDecoder(resp.Body).Decode(&ghReleases); err != nil {
			resp.Body.Close()
			return all, fmt.Errorf("decoding releases: %w", err)
		}
		resp.Body.Close()

		if len(ghReleases) == 0 {
			break
		}
		for _, r := range ghReleases {
			t, _ := time.Parse(time.RFC3339, r.PublishedAt)
			all = append(all, &Release{TagName: r.TagName, Name: r.Name, PublishedAt: t})
		}
		page++
	}
	return all, nil
}

func (c *Client) GetLatestWorkflowRun(token, owner, repo, branch string) (*WorkflowRun, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=1&status=completed", c.APIURL, owner, repo)
	if branch != "" {
		u += "&branch=" + url.QueryEscape(branch)
	}
	resp, err := c.doRequest("GET", u, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var runs ghWorkflowRuns
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		return nil, fmt.Errorf("decoding workflow runs: %w", err)
	}
	if len(runs.WorkflowRuns) == 0 {
		return nil, fmt.Errorf("no completed workflow runs found")
	}

	run := runs.WorkflowRuns[0]
	return &WorkflowRun{ID: run.ID, Conclusion: run.Conclusion}, nil
}

func (c *Client) GetWorkflowRuns(token, owner, repo, branch string, perPage int) ([]*WorkflowRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=%d&branch=%s", c.APIURL, owner, repo, perPage, branch)
	resp, err := c.doRequest("GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var runs ghWorkflowRuns
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		return nil, fmt.Errorf("decoding workflow runs: %w", err)
	}
	var result []*WorkflowRun
	for _, r := range runs.WorkflowRuns {
		result = append(result, &WorkflowRun{ID: r.ID, Conclusion: r.Conclusion})
	}
	return result, nil
}

func (c *Client) ListPullRequests(token, owner, repo string) ([]*PullRequest, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=open&per_page=50&sort=created&direction=desc", c.APIURL, owner, repo)
	resp, err := c.doRequest("GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ghPRs []ghPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&ghPRs); err != nil {
		return nil, fmt.Errorf("decoding pull requests: %w", err)
	}

	var prs []*PullRequest
	for _, pr := range ghPRs {
		t, _ := time.Parse(time.RFC3339, pr.CreatedAt)
		prs = append(prs, &PullRequest{
			Number:    pr.Number,
			Title:     pr.Title,
			Author:    pr.User.Login,
			CreatedAt: t,
			HTMLURL:   pr.HTMLURL,
			HeadRef:   pr.Head.Ref,
			BaseRef:   pr.Base.Ref,
		})
	}
	return prs, nil
}

func (c *Client) ListRecentlyMergedPRs(token, owner, repo string) ([]*PullRequest, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls?state=closed&sort=updated&direction=desc&per_page=10", c.APIURL, owner, repo)
	resp, err := c.doRequest("GET", url, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var ghPRs []ghPullRequest
	if err := json.NewDecoder(resp.Body).Decode(&ghPRs); err != nil {
		return nil, fmt.Errorf("decoding pull requests: %w", err)
	}

	var prs []*PullRequest
	for _, pr := range ghPRs {
		if pr.MergedAt == "" {
			continue
		}
		t, _ := time.Parse(time.RFC3339, pr.CreatedAt)
		prs = append(prs, &PullRequest{
			Number:    pr.Number,
			Title:     pr.Title,
			Author:    pr.User.Login,
			CreatedAt: t,
			HTMLURL:   pr.HTMLURL,
			HeadRef:   pr.Head.Ref,
			BaseRef:   pr.Base.Ref,
		})
	}
	return prs, nil
}

type mergePRRequest struct {
	MergeMethod string `json:"merge_method"`
}

type mergePRResponse struct {
	Merged  bool   `json:"merged"`
	Message string `json:"message"`
}

func (c *Client) MergePullRequest(token, owner, repo string, prNumber int) (bool, string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/merge", c.APIURL, owner, repo, prNumber)
	body := mergePRRequest{MergeMethod: "merge"}
	var buf strings.Builder
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return false, "", fmt.Errorf("encoding merge request: %w", err)
	}
	resp, err := c.doRequest("PUT", url, token, strings.NewReader(buf.String()))
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	var result mergePRResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, "", fmt.Errorf("decoding merge response: %w", err)
	}
	return result.Merged, result.Message, nil
}

func (c *Client) GetWorkflowStatus(token, owner, repo, branch string) (*WorkflowRun, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=1&status=completed", c.APIURL, owner, repo)
	if branch != "" {
		u += "&branch=" + url.QueryEscape(branch)
	}
	resp, err := c.doRequest("GET", u, token, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var runs ghWorkflowRuns
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		return nil, fmt.Errorf("decoding workflow runs: %w", err)
	}
	if len(runs.WorkflowRuns) == 0 {
		return nil, fmt.Errorf("no workflow runs found")
	}

	run := runs.WorkflowRuns[0]
	return &WorkflowRun{
		ID:         run.ID,
		Conclusion: run.Conclusion,
	}, nil
}
