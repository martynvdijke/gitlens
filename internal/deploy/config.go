package deploy

import (
	"encoding/json"
	"os"
	"strings"
)

// TagStrategy controls how release tags are translated to image tags.
type TagStrategy string

const (
	TagStrategyReleaseTag TagStrategy = "release_tag"
	TagStrategyLatest     TagStrategy = "latest"
)

// Target maps a GitHub repository to a Docker image and container.
type Target struct {
	Repository  string      `json:"repository"`
	Image       string      `json:"image"`
	Container   string      `json:"container"`
	TagStrategy TagStrategy `json:"tag_strategy"`
}

// LoadTargets reads deploy targets from DEPLOY_TARGETS JSON env var,
// falling back to DEPLOY_TARGETS_FILE if the env var is unset.
// Returns nil, nil when no targets are configured (safe default).
func LoadTargets() ([]Target, error) {
	raw := os.Getenv("DEPLOY_TARGETS")
	if raw == "" {
		file := os.Getenv("DEPLOY_TARGETS_FILE")
		if file != "" {
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, err
			}
			raw = string(data)
		}
	}
	if raw == "" {
		return nil, nil
	}
	var targets []Target
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		return nil, err
	}
	for i := range targets {
		if targets[i].TagStrategy == "" {
			targets[i].TagStrategy = TagStrategyReleaseTag
		}
	}
	return targets, nil
}

// MatchTarget returns the first target whose Repository matches repoFullName,
// or nil if no match is found.
func MatchTarget(targets []Target, repoFullName string) *Target {
	for _, t := range targets {
		if t.Repository == repoFullName {
			return &t
		}
	}
	return nil
}

// NormalizeTag converts a GitHub release tag to an image tag based on strategy.
//   - release_tag: strips leading "v" prefix ("v1.2.3" → "1.2.3")
//   - latest: returns "latest"
func NormalizeTag(releaseTag string, strategy TagStrategy) string {
	if strategy == TagStrategyLatest {
		return "latest"
	}
	return strings.TrimPrefix(releaseTag, "v")
}

// PrereleasesAllowed returns true when DEPLOY_ALLOW_PRERELEASE=true.
func PrereleasesAllowed() bool {
	return os.Getenv("DEPLOY_ALLOW_PRERELEASE") == "true"
}

// GotifyConfig returns the Gotify base URL and app token from env vars.
func GotifyConfig() (baseURL, token string) {
	return os.Getenv("GOTIFY_URL"), os.Getenv("GOTIFY_TOKEN")
}

// DeployBackend returns the configured deploy backend ("api" or "compose").
func DeployBackend() string {
	backend := os.Getenv("DEPLOY_BACKEND")
	if backend == "" {
		return "api"
	}
	return backend
}
