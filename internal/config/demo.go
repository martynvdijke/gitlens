package config

import (
	"net/url"
	"os"
	"strings"
)

// Demo describes the shared, public demo account used for app-store review.
// The token is intentionally public and deterministic so reviewers can use it
// without secrets; only its SHA-256 hash is stored in the database.
type Demo struct {
	Enabled bool
	Login   string
	Token   string
}

const (
	defaultDemoLogin = "demo"
	defaultDemoToken = "glt_demo_public_preview_2026"
	demoDefaultHost  = "gitlens.vandijke.xyz"
)

// LoadDemo reads the DEMO_USER_* environment variables. When
// DEMO_USER_ENABLED is unset the demo account is enabled only on the canonical
// GitLens host (derived from APP_URL), so local and self-hosted instances stay
// untouched by default.
func LoadDemo() Demo {
	login := strings.TrimSpace(os.Getenv("DEMO_USER_LOGIN"))
	if login == "" {
		login = defaultDemoLogin
	}
	token := strings.TrimSpace(os.Getenv("DEMO_USER_TOKEN"))
	if token == "" {
		token = defaultDemoToken
	}

	enabled := false
	if raw, ok := os.LookupEnv("DEMO_USER_ENABLED"); ok {
		v := strings.TrimSpace(strings.ToLower(raw))
		enabled = v == "true" || v == "1"
	} else if appURL := strings.TrimSpace(os.Getenv("APP_URL")); appURL != "" {
		if u, err := url.Parse(appURL); err == nil {
			enabled = strings.EqualFold(u.Hostname(), demoDefaultHost)
		}
	}

	return Demo{Enabled: enabled, Login: login, Token: token}
}
