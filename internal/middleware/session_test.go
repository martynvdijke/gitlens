package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newTestSessionStore(t *testing.T) *SessionStore {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/sessions.db?_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewSessionStore(db)
}

func TestSessionStoreSetGet(t *testing.T) {
	store := newTestSessionStore(t)
	id := store.Set(42)
	uid, ok := store.Get(id)
	if !ok {
		t.Fatal("expected session to exist")
	}
	if uid != 42 {
		t.Fatalf("expected 42, got %d", uid)
	}
}

func TestSessionStoreGetInvalid(t *testing.T) {
	store := newTestSessionStore(t)
	_, ok := store.Get("nonexistent")
	if ok {
		t.Fatal("expected nonexistent session to return false")
	}
}

func TestSessionStoreDelete(t *testing.T) {
	store := newTestSessionStore(t)
	id := store.Set(42)
	store.Delete(id)
	_, ok := store.Get(id)
	if ok {
		t.Fatal("expected deleted session to return false")
	}
}

func TestSessionUniqueIDs(t *testing.T) {
	store := newTestSessionStore(t)
	id1 := store.Set(1)
	id2 := store.Set(2)
	if id1 == id2 {
		t.Fatal("expected unique session IDs")
	}
}

func TestAuthRequiredWithCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestSessionStore(t)
	sessionID := store.Set(42)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})

	handler := AuthRequired(store)
	handler(c)

	if c.IsAborted() {
		t.Fatal("expected not to be aborted")
	}
	uid, _ := c.Get("user_id")
	if uid != int64(42) {
		t.Fatalf("expected user_id 42, got %v", uid)
	}
}

func TestAuthRequiredWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := newTestSessionStore(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)

	handler := AuthRequired(store)
	handler(c)

	if !c.IsAborted() {
		t.Fatal("expected to be aborted (redirect)")
	}
}

func TestSessionStore_InitPreservesValidSessions(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/sessions.db?_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Simulate a session that was stored with UTC format (our fixed format)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		expires_at DATETIME NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().Add(7 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	_, err = db.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)", "test_valid", int64(42), expiry)
	if err != nil {
		t.Fatal(err)
	}

	// init() runs cleanup - valid sessions should survive
	store := NewSessionStore(db)
	uid, ok := store.Get("test_valid")
	if !ok {
		t.Fatal("expected valid session to survive init() cleanup")
	}
	if uid != 42 {
		t.Fatalf("expected uid 42, got %d", uid)
	}
}

func TestSessionStore_InitCleansExpiredSessions(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/sessions.db?_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		user_id INTEGER NOT NULL,
		expires_at DATETIME NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	expiry := time.Now().UTC().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")
	_, err = db.Exec("INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)", "test_expired", int64(99), expiry)
	if err != nil {
		t.Fatal(err)
	}

	// init() runs cleanup - expired sessions should be deleted
	store := NewSessionStore(db)
	_, ok := store.Get("test_expired")
	if ok {
		t.Fatal("expected expired session to be cleaned by init()")
	}
}

func TestSetClearSessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	SetSessionCookie(c, "test_session_id")
	c.Writer.WriteHeader(http.StatusOK)

	cookies := w.Result().Cookies()
	var found bool
	for _, cookie := range cookies {
		if cookie.Name == sessionCookieName {
			found = true
			if cookie.Value != "test_session_id" {
				t.Fatalf("expected test_session_id, got %s", cookie.Value)
			}
		}
	}
	if !found {
		t.Fatal("session cookie not set")
	}
}
