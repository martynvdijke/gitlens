package provider

import (
	"context"
	"time"

	ghclient "gitlens/internal/github"
)

// GitHubAdapter wraps a *github.Client so it satisfies the Provider
// interface. We can't declare interface conformance inside the github
// package itself because that would create an import cycle (the
// provider package already imports github for its DTOs).
type GitHubAdapter struct {
	*ghclient.Client
}

func NewGitHubAdapter(c *ghclient.Client) *GitHubAdapter {
	return &GitHubAdapter{Client: c}
}

func (a *GitHubAdapter) Name() string { return "github" }

func (a *GitHubAdapter) AuthURL(state, redirectURL string) string {
	return a.Client.AuthorizeURL(state, redirectURL)
}

func (a *GitHubAdapter) ExchangeCode(ctx context.Context, code, redirectURL string) (string, *ghclient.User, error) {
	_ = ctx
	_ = redirectURL
	tok, err := a.Client.GetAccessToken(code)
	if err != nil {
		return "", nil, err
	}
	u, err := a.Client.GetUser(tok)
	if err != nil {
		return "", nil, err
	}
	return tok, u, nil
}

func (a *GitHubAdapter) GetUser(ctx context.Context, token string) (*ghclient.User, error) {
	_ = ctx
	return a.Client.GetUser(token)
}

func (a *GitHubAdapter) ListRepositories(ctx context.Context, token string) ([]*ghclient.Repository, error) {
	_ = ctx
	return a.Client.ListRepositories(token)
}

func (a *GitHubAdapter) GetCommitsSince(ctx context.Context, token, owner, repo, branch string, since time.Time, maxCommits int) ([]*ghclient.Commit, error) {
	_ = ctx
	return a.Client.GetCommitsSince(token, owner, repo, branch, since, maxCommits)
}

func (a *GitHubAdapter) ListReleases(ctx context.Context, token, owner, repo string) ([]*ghclient.Release, error) {
	_ = ctx
	return a.Client.ListReleases(token, owner, repo)
}

func (a *GitHubAdapter) ListPullRequests(ctx context.Context, token, owner, repo string) ([]*ghclient.PullRequest, error) {
	_ = ctx
	return a.Client.ListPullRequests(token, owner, repo)
}

func (a *GitHubAdapter) ListRecentlyMergedPRs(ctx context.Context, token, owner, repo string) ([]*ghclient.PullRequest, error) {
	_ = ctx
	return a.Client.ListRecentlyMergedPRs(token, owner, repo)
}

func (a *GitHubAdapter) GetLatestWorkflowRun(ctx context.Context, token, owner, repo, branch string) (*ghclient.WorkflowRun, error) {
	_ = ctx
	return a.Client.GetLatestWorkflowRun(token, owner, repo, branch)
}

// Compile-time interface check.
var _ Provider = (*GitHubAdapter)(nil)
