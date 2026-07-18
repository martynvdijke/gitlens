package deploy_test

import (
	"os"
	"testing"

	"gitlens/internal/deploy"
)

func TestLoadTargets_Empty(t *testing.T) {
	os.Unsetenv("DEPLOY_TARGETS")
	os.Unsetenv("DEPLOY_TARGETS_FILE")
	targets, err := deploy.LoadTargets()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if targets != nil {
		t.Fatalf("expected nil targets, got %v", targets)
	}
}

func TestLoadTargets_FromEnv(t *testing.T) {
	os.Setenv("DEPLOY_TARGETS", `[{"repository":"test/repo","image":"ghcr.io/test/repo","container":"test-app","tag_strategy":"release_tag"}]`)
	defer os.Unsetenv("DEPLOY_TARGETS")

	targets, err := deploy.LoadTargets()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Repository != "test/repo" {
		t.Fatalf("expected test/repo, got %s", targets[0].Repository)
	}
	if targets[0].Image != "ghcr.io/test/repo" {
		t.Fatalf("expected ghcr.io/test/repo, got %s", targets[0].Image)
	}
	if targets[0].Container != "test-app" {
		t.Fatalf("expected test-app, got %s", targets[0].Container)
	}
}

func TestLoadTargets_DefaultsTagStrategy(t *testing.T) {
	os.Setenv("DEPLOY_TARGETS", `[{"repository":"test/repo","image":"img","container":"c"}]`)
	defer os.Unsetenv("DEPLOY_TARGETS")

	targets, err := deploy.LoadTargets()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if targets[0].TagStrategy != deploy.TagStrategyReleaseTag {
		t.Fatalf("expected release_tag, got %s", targets[0].TagStrategy)
	}
}

func TestLoadTargets_FromFile(t *testing.T) {
	os.Unsetenv("DEPLOY_TARGETS")
	file, err := os.CreateTemp(t.TempDir(), "targets-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	file.WriteString(`[{"repository":"f/repo","image":"img","container":"c"}]`)
	os.Setenv("DEPLOY_TARGETS_FILE", file.Name())
	defer os.Unsetenv("DEPLOY_TARGETS_FILE")

	targets, err := deploy.LoadTargets()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Repository != "f/repo" {
		t.Fatalf("expected f/repo, got %s", targets[0].Repository)
	}
}

func TestMatchTarget_Found(t *testing.T) {
	targets := []deploy.Target{
		{Repository: "org/alpha", Image: "alpha", Container: "a"},
		{Repository: "org/beta", Image: "beta", Container: "b"},
	}
	m := deploy.MatchTarget(targets, "org/beta")
	if m == nil {
		t.Fatal("expected match, got nil")
	}
	if m.Container != "b" {
		t.Fatalf("expected container b, got %s", m.Container)
	}
}

func TestMatchTarget_NotFound(t *testing.T) {
	targets := []deploy.Target{
		{Repository: "org/alpha", Image: "alpha", Container: "a"},
	}
	m := deploy.MatchTarget(targets, "org/gamma")
	if m != nil {
		t.Fatalf("expected nil, got %v", m)
	}
}

func TestNormalizeTag_ReleaseTag(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"v0.0.1", "0.0.1"},
		{"latest", "latest"},
	}
	for _, tt := range tests {
		got := deploy.NormalizeTag(tt.in, deploy.TagStrategyReleaseTag)
		if got != tt.want {
			t.Errorf("NormalizeTag(%q, release_tag) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeTag_Latest(t *testing.T) {
	got := deploy.NormalizeTag("v3.0.0", deploy.TagStrategyLatest)
	if got != "latest" {
		t.Fatalf("expected latest, got %s", got)
	}
}

func TestPrereleasesAllowed_Default(t *testing.T) {
	os.Unsetenv("DEPLOY_ALLOW_PRERELEASE")
	if deploy.PrereleasesAllowed() {
		t.Fatal("expected false by default")
	}
}

func TestPrereleasesAllowed_Enabled(t *testing.T) {
	os.Setenv("DEPLOY_ALLOW_PRERELEASE", "true")
	defer os.Unsetenv("DEPLOY_ALLOW_PRERELEASE")
	if !deploy.PrereleasesAllowed() {
		t.Fatal("expected true")
	}
}

func TestDeployBackend_Default(t *testing.T) {
	os.Unsetenv("DEPLOY_BACKEND")
	if got := deploy.DeployBackend(); got != "api" {
		t.Fatalf("expected api, got %s", got)
	}
}

func TestDeployBackend_Compose(t *testing.T) {
	os.Setenv("DEPLOY_BACKEND", "compose")
	defer os.Unsetenv("DEPLOY_BACKEND")
	if got := deploy.DeployBackend(); got != "compose" {
		t.Fatalf("expected compose, got %s", got)
	}
}
