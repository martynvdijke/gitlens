package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSessionStoreSetGet(t *testing.T) {
	store := NewSessionStore()
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
	store := NewSessionStore()
	_, ok := store.Get("nonexistent")
	if ok {
		t.Fatal("expected nonexistent session to return false")
	}
}

func TestSessionStoreDelete(t *testing.T) {
	store := NewSessionStore()
	id := store.Set(42)
	store.Delete(id)
	_, ok := store.Get(id)
	if ok {
		t.Fatal("expected deleted session to return false")
	}
}

func TestSessionUniqueIDs(t *testing.T) {
	store := NewSessionStore()
	id1 := store.Set(1)
	id2 := store.Set(2)
	if id1 == id2 {
		t.Fatal("expected unique session IDs")
	}
}

func TestAuthRequiredWithCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewSessionStore()
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
	store := NewSessionStore()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)

	handler := AuthRequired(store)
	handler(c)

	if !c.IsAborted() {
		t.Fatal("expected to be aborted (redirect)")
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
