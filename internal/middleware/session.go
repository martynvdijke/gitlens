package middleware

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName = "gitlens_session"
	sessionMaxAge     = 7 * 24 * time.Hour
)

type SessionStore struct {
	db *sql.DB
}

func NewSessionStore(db *sql.DB) *SessionStore {
	store := &SessionStore{db: db}
	if err := store.init(); err != nil {
		log.Printf("Failed to initialize session store: %v", err)
	}
	return store
}

func (s *SessionStore) init() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		expires_at DATETIME NOT NULL
	)`); err != nil {
		return err
	}
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC().Format("2006-01-02 15:04:05"))
	return err
}

func (s *SessionStore) generateID() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *SessionStore) Set(userID int64) string {
	id := s.generateID()
	expiresAt := time.Now().UTC().Add(sessionMaxAge)
	_, err := s.db.Exec(
		"INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)",
		id, userID, expiresAt.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		log.Printf("Failed to set session: %v", err)
		return ""
	}
	return id
}

func (s *SessionStore) Get(sessionID string) (int64, bool) {
	var userID int64
	var expiresAt time.Time
	err := s.db.QueryRow(
		"SELECT user_id, expires_at FROM sessions WHERE id = ?",
		sessionID,
	).Scan(&userID, &expiresAt)
	if err != nil {
		return 0, false
	}
	if time.Now().After(expiresAt) {
		s.Delete(sessionID)
		return 0, false
	}
	return userID, true
}

func (s *SessionStore) Delete(sessionID string) {
	_, _ = s.db.Exec("DELETE FROM sessions WHERE id = ?", sessionID)
}

func AuthRequired(store *SessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie(sessionCookieName)
		if err != nil {
			c.Redirect(http.StatusFound, "/")
			c.Abort()
			return
		}
		userID, ok := store.Get(cookie)
		if !ok {
			c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
			c.Redirect(http.StatusFound, "/")
			c.Abort()
			return
		}
		c.Set("session_id", cookie)
		c.Set("user_id", userID)
		c.Next()
	}
}

func SetSessionCookie(c *gin.Context, sessionID string) {
	// Secure the cookie when the request arrived over TLS or behind a TLS-terminating proxy.
	secure := false
	if c.Request != nil {
		secure = c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	}
	c.SetCookie(sessionCookieName, sessionID, int(sessionMaxAge.Seconds()), "/", "", secure, true)
}

func ClearSessionCookie(c *gin.Context) {
	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}

// HTMXOnly aborts with a redirect to / when the request is not an HTMX
// partial request. This prevents direct browser refreshes on push-url
// routes from rendering raw partial templates.
func HTMXOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetHeader("HX-Request") == "" {
			c.Redirect(http.StatusFound, "/")
			c.Abort()
		}
	}
}
