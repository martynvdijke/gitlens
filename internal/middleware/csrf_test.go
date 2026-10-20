package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func csrfRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CSRF())
	r.POST("/mutate", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func TestCSRFAllowsSameOrigin(t *testing.T) {
	r := csrfRouter()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/mutate", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("same-origin POST = %d, want 200", w.Code)
	}
}

func TestCSRFRejectsCrossOrigin(t *testing.T) {
	r := csrfRouter()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/mutate", nil)
	req.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST = %d, want 403", w.Code)
	}
}

func TestCSRFRejectsMissingOrigin(t *testing.T) {
	r := csrfRouter()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/mutate", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("POST without Origin = %d, want 403", w.Code)
	}
}

func TestCSRFExemptsBearerRequests(t *testing.T) {
	r := csrfRouter()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/mutate", nil)
	req.Header.Set("Authorization", "Bearer glt_token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("bearer POST = %d, want 200", w.Code)
	}
}

func TestCSRFAllowsRefererFallback(t *testing.T) {
	r := csrfRouter()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/mutate", nil)
	req.Header.Set("Referer", "http://example.com/settings")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("referer POST = %d, want 200", w.Code)
	}
}
