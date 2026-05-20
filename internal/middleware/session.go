package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName = "gitlens_session"
	sessionMaxAge     = 7 * 24 * time.Hour
)

type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]int64
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]int64),
	}
}

func (s *SessionStore) generateID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *SessionStore) Set(userID int64) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.generateID()
	s.sessions[id] = userID
	return id
}

func (s *SessionStore) Get(sessionID string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	userID, ok := s.sessions[sessionID]
	return userID, ok
}

func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
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
	c.SetCookie(sessionCookieName, sessionID, int(sessionMaxAge.Seconds()), "/", "", false, true)
}

func ClearSessionCookie(c *gin.Context) {
	c.SetCookie(sessionCookieName, "", -1, "/", "", false, true)
}
