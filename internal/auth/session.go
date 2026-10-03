package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	SessionCookieName = "gitlens_session_oidc"
	sessionDuration   = 24 * time.Hour
)

func sessionSecret() string {
	return LoadConfig().SessionSecret
}

// createSessionValue returns base64(userID|expiry|sig); empty secret => fail-closed.
func createSessionValue(userID int64, expiry time.Time) (string, error) {
	secret := sessionSecret()
	if secret == "" {
		return "", fmt.Errorf("SESSION_SECRET not configured")
	}
	msg := fmt.Sprintf("%d|%d", userID, expiry.Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))
	raw := fmt.Sprintf("%s|%s", msg, sig)
	return base64.RawURLEncoding.EncodeToString([]byte(raw)), nil
}

func validateSessionValue(val string) (int64, bool) {
	secret := sessionSecret()
	if secret == "" {
		return 0, false
	}
	b, err := base64.RawURLEncoding.DecodeString(val)
	if err != nil {
		return 0, false
	}
	parts := strings.Split(string(b), "|")
	if len(parts) != 3 {
		return 0, false
	}
	msg := parts[0] + "|" + parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msg))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expected)) {
		return 0, false
	}
	uid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	if time.Now().Unix() > exp {
		return 0, false
	}
	return uid, true
}

// SetSessionCookie creates signed session cookie; fail-closed if secret missing.
func SetSessionCookie(c *gin.Context, userID int64) {
	exp := time.Now().Add(sessionDuration)
	val, err := createSessionValue(userID, exp)
	if err != nil {
		// fail-closed: do not set forgeable cookie
		return
	}
	secure := c.Request != nil && (c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(SessionCookieName, val, int(sessionDuration.Seconds()), "/", "", secure, true)
}

func ClearSessionCookie(c *gin.Context) {
	secure := c.Request != nil && (c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(SessionCookieName, "", -1, "/", "", secure, true)
}

func GetUserIDFromSession(c *gin.Context) (int64, bool) {
	v, err := c.Cookie(SessionCookieName)
	if err != nil || v == "" {
		return 0, false
	}
	return validateSessionValue(v)
}

// RequireAuthMiddleware protects /api mutations: if bearer token present defer? else check oidc session OR legacy session.
// For this app we check OIDC signed cookie + fallback to legacy gitlens_session via provided checker func.
func RequireAuthForMutations(legacyCheck func(*gin.Context) (int64, bool)) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only protect mutations; GET stays public
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		// Skip non-/api paths - but this middleware will be mounted on /api group, so all are /api
		// Check OIDC session
		if uid, ok := GetUserIDFromSession(c); ok {
			c.Set("user_id", uid)
			c.Set("oidc_user_id", uid)
			c.Next()
			return
		}
		// Legacy session fallback
		if legacyCheck != nil {
			if uid, ok := legacyCheck(c); ok {
				c.Set("user_id", uid)
				c.Next()
				return
			}
		}
		// Also allow Bearer token (api tokens) — check Authorization header via context? handled separately but also 401 here
		// If Authorization Bearer present, let it pass to next handler that may check tokens; but if no token validation here, return 401
		// We return 401 directly; token routes have their own middleware
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
	}
}
