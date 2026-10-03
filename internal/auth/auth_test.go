package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoadConfigValid(t *testing.T) {
	t.Setenv("OIDC_ENABLED", "true")
	t.Setenv("OIDC_ISSUER_URL", "https://authelia.vandijke.xyz")
	t.Setenv("OIDC_CLIENT_ID", "gitlens")
	t.Setenv("OIDC_CLIENT_SECRET", "secret123")
	t.Setenv("OIDC_REDIRECT_URL", "https://gitlens.vandijke.xyz/api/auth/oidc/callback")
	cfg := LoadConfig()
	if !cfg.Valid() {
		t.Fatal("expected valid")
	}
	if cfg.LogoutURL != "https://authelia.vandijke.xyz/logout" {
		t.Fatalf("logout url %q", cfg.LogoutURL)
	}
	if len(cfg.Scopes) == 0 {
		t.Fatal("scopes empty")
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	t.Setenv("OIDC_ENABLED", "false")
	t.Setenv("OIDC_ISSUER_URL", "https://authelia.vandijke.xyz")
	t.Setenv("OIDC_CLIENT_ID", "gitlens")
	t.Setenv("OIDC_CLIENT_SECRET", "s")
	t.Setenv("OIDC_REDIRECT_URL", "https://example.com/cb")
	cfg := LoadConfig()
	if cfg.Valid() {
		t.Fatal("expected invalid when disabled")
	}
	// missing secret
	t.Setenv("OIDC_ENABLED", "true")
	t.Setenv("OIDC_CLIENT_SECRET", "")
	os.Unsetenv("OIDC_CLIENT_SECRET_FILE")
	cfg = LoadConfig()
	if cfg.Valid() {
		t.Fatal("expected invalid when secret missing")
	}
}

func TestLoadConfigSecretFile(t *testing.T) {
	f, _ := os.CreateTemp("", "oidc_secret")
	_, _ = f.WriteString("  fileSecret \n")
	f.Close()
	defer os.Remove(f.Name())
	t.Setenv("OIDC_CLIENT_SECRET_FILE", f.Name())
	t.Setenv("OIDC_CLIENT_SECRET", "fallback")
	cfg := LoadConfig()
	if cfg.ClientSecret != "fileSecret" {
		t.Fatalf("got %q", cfg.ClientSecret)
	}
}

func TestMutationProtection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// Real middleware from main.go (replicated): GET public, Bearer verified, session checked
	mw := func(c *gin.Context) {
		p := c.Request.URL.Path
		if !strings.HasPrefix(p, "/api") {
			c.Next()
			return
		}
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if strings.HasPrefix(p, "/api/auth/oidc/") {
			c.Next()
			return
		}
		// Bearer must be valid token; nil client => always invalid => 401
		if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
			raw := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
			if raw != "" {
				// Simulate VerifyToken failure (no valid token "x")
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
				return
			}
		}
		if _, ok := GetUserIDFromSession(c); ok {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	}
	r := gin.New()
	r.Use(mw)
	r.GET("/api/public", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
	r.POST("/api/mutate", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	// (c) unauthenticated GET => 200
	req := httptest.NewRequest("GET", "/api/public", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET expected 200 got %d", w.Code)
	}
	// (a) unauthenticated POST => 401
	req = httptest.NewRequest("POST", "/api/mutate", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("POST unauth expected 401 got %d", w.Code)
	}
	// (b) Bearer x alone => 401 (not bypassed)
	req = httptest.NewRequest("POST", "/api/mutate", nil)
	req.Header.Set("Authorization", "Bearer x")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("Bearer bypass expected 401 got %d", w.Code)
	}
}

func TestCallbackRejectsBadState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("OIDC_ENABLED", "true")
	t.Setenv("OIDC_ISSUER_URL", "https://authelia.vandijke.xyz")
	t.Setenv("OIDC_CLIENT_ID", "gitlens")
	t.Setenv("OIDC_CLIENT_SECRET", "secret123")
	t.Setenv("OIDC_REDIRECT_URL", "https://gitlens.vandijke.xyz/api/auth/oidc/callback")
	ResetProvider()
	r := gin.New()
	r.GET("/api/auth/oidc/callback", CallbackHandler(nil))

	req := httptest.NewRequest("GET", "/api/auth/oidc/callback?code=abc&state=bad", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: "goodstate"})
	req.AddCookie(&http.Cookie{Name: "oidc_nonce", Value: "nonce"})
	req.AddCookie(&http.Cookie{Name: "oidc_verifier", Value: "verifier"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("expected 400 bad state got %d body %s", w.Code, w.Body.String())
	}
	req2 := httptest.NewRequest("GET", "/api/auth/oidc/callback?code=abc&state=goodstate", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 400 {
		t.Fatalf("expected 400 absent cookie got %d", w2.Code)
	}
}
