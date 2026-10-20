package middleware

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CSRF protects cookie-authenticated mutations using Origin/Referer checks.
// Bearer-token requests are exempt because browsers do not attach those
// credentials automatically. Requests without either header are rejected for
// unsafe methods, which is safe for the HTMX UI (browsers always send Origin
// on cross-site and same-site POSTs) and keeps the API token flow working.
func CSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		if strings.HasPrefix(c.GetHeader("Authorization"), bearerPrefix) {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if origin == "" {
			origin = c.GetHeader("Referer")
		}
		if origin == "" || !sameOrigin(c, origin) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "cross-site request rejected"})
			return
		}
		c.Next()
	}
}

func sameOrigin(c *gin.Context, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	allowed := requestHosts(c)
	originHost := stripPort(strings.ToLower(u.Host))
	for _, h := range allowed {
		if originHost == stripPort(strings.ToLower(h)) {
			return true
		}
	}
	return false
}

func requestHosts(c *gin.Context) []string {
	hosts := []string{c.Request.Host}
	if fwd := c.GetHeader("X-Forwarded-Host"); fwd != "" {
		// X-Forwarded-Host may contain a comma-separated chain; the first
		// entry is the host the client actually requested.
		hosts = append(hosts, strings.TrimSpace(strings.Split(fwd, ",")[0]))
	}
	return hosts
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i != -1 && !strings.Contains(host[i+1:], "]") {
		return host[:i]
	}
	return host
}
