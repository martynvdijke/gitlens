package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
	tokenURL     string
	apiURL       string
}

func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		tokenURL: "https://github.com/login/oauth/access_token",
		apiURL:   "https://api.github.com",
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
	req, err := http.NewRequest("POST", c.tokenURL, strings.NewReader(form.Encode()))
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
	resp, err := c.doRequest("GET", c.apiURL+"/user", token, nil)
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
		url := fmt.Sprintf("%s/user/repos?per_page=100&page=%d&sort=updated", c.apiURL, page)
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
	url := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=1&sha=%s", c.apiURL, owner, repo, branch)
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

func (c *Client) GetCommits(token, owner, repo, branch string, perPage int) ([]*Commit, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits?per_page=%d&sha=%s", c.apiURL, owner, repo, perPage, branch)
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
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", c.apiURL, owner, repo)
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
		url := fmt.Sprintf("%s/repos/%s/%s/releases?per_page=100&page=%d", c.apiURL, owner, repo, page)
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

func (c *Client) GetWorkflowRuns(token, owner, repo, branch string, perPage int) ([]*WorkflowRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=%d&branch=%s", c.apiURL, owner, repo, perPage, branch)
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

func (c *Client) GetWorkflowStatus(token, owner, repo, branch string) (*WorkflowRun, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/actions/runs?per_page=1&branch=%s&status=completed", c.apiURL, owner, repo, branch)
	resp, err := c.doRequest("GET", url, token, nil)
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
