package auth

import (
	"os"
	"strings"
)

// Config holds OIDC RP configuration.
type Config struct {
	Enabled     bool
	IssuerURL   string
	ClientID    string
	ClientSecret string
	RedirectURL string
	Scopes      []string
	LogoutURL   string
	SessionSecret string
}

func LoadConfig() Config {
	enabled := true
	if v, ok := os.LookupEnv("OIDC_ENABLED"); ok {
		enabled = v == "true" || v == "1"
	}
	c := Config{Enabled: enabled}
	// secret: file preferred
	if f := os.Getenv("OIDC_CLIENT_SECRET_FILE"); f != "" {
		if b, err := os.ReadFile(f); err == nil {
			c.ClientSecret = strings.TrimSpace(string(b))
		}
	}
	if c.ClientSecret == "" {
		c.ClientSecret = strings.TrimSpace(os.Getenv("OIDC_CLIENT_SECRET"))
	}
	c.IssuerURL = strings.TrimSuffix(strings.TrimSpace(os.Getenv("OIDC_ISSUER_URL")), "/")
	c.ClientID = strings.TrimSpace(os.Getenv("OIDC_CLIENT_ID"))
	c.RedirectURL = strings.TrimSpace(os.Getenv("OIDC_REDIRECT_URL"))
	if s := os.Getenv("OIDC_SCOPES"); s != "" {
		c.Scopes = strings.Fields(s)
	} else {
		c.Scopes = []string{"openid", "email", "profile", "groups"}
	}
	if u := strings.TrimSpace(os.Getenv("OIDC_LOGOUT_URL")); u != "" {
		c.LogoutURL = u
	} else if c.IssuerURL != "" {
		c.LogoutURL = c.IssuerURL + "/logout"
	}
	c.SessionSecret = strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	return c
}

func (c Config) Valid() bool {
	return c.Enabled && c.IssuerURL != "" && c.ClientID != "" && c.ClientSecret != "" && c.RedirectURL != ""
}
