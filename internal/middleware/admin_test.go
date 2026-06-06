package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlens/ent"
	"gitlens/ent/enttest"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newAdminTestClient(t *testing.T) *ent.Client {
	t.Helper()
	return enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
}

func createTestAdminUser(t *testing.T, client *ent.Client) int {
	t.Helper()
	u, err := client.User.Create().
		SetGithubID(100).
		SetLogin("admin").
		SetAccessToken("tok").
		SetIsAdmin(true).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func createTestNonAdminUser(t *testing.T, client *ent.Client) int {
	t.Helper()
	u, err := client.User.Create().
		SetGithubID(101).
		SetLogin("user").
		SetAccessToken("tok").
		SetIsAdmin(false).
		Save(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestAdminRequired_AllowsAdmin(t *testing.T) {
	client := newAdminTestClient(t)
	adminID := createTestAdminUser(t, client)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/admin", nil)
	c.Set("user_id", int64(adminID))

	AdminRequired(client)(c)

	if c.IsAborted() {
		t.Fatal("expected admin to be allowed through middleware")
	}
}

func TestAdminRequired_BlocksNonAdmin(t *testing.T) {
	client := newAdminTestClient(t)
	userID := createTestNonAdminUser(t, client)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/admin", nil)
	c.Set("user_id", int64(userID))

	AdminRequired(client)(c)

	if !c.IsAborted() {
		t.Fatal("expected non-admin to be blocked by middleware")
	}
}

func TestAdminRequired_BlocksNoUserID(t *testing.T) {
	client := newAdminTestClient(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/admin", nil)

	AdminRequired(client)(c)

	if !c.IsAborted() {
		t.Fatal("expected request without user_id to be blocked")
	}
}

func TestAdminRequired_BlocksInvalidUserIDType(t *testing.T) {
	client := newAdminTestClient(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/admin", nil)
	c.Set("user_id", "not-an-int64")

	AdminRequired(client)(c)

	if !c.IsAborted() {
		t.Fatal("expected request with invalid user_id type to be blocked")
	}
}

func TestAdminRequired_HTMXRedirectHeader(t *testing.T) {
	client := newAdminTestClient(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/admin", nil)
	c.Request.Header.Set("HX-Request", "true")

	AdminRequired(client)(c)

	if !c.IsAborted() {
		t.Fatal("expected HTMX request to be blocked")
	}
	if w.Header().Get("HX-Redirect") != "/" {
		t.Fatalf("expected HX-Redirect header to '/', got '%s'", c.GetHeader("HX-Redirect"))
	}
}

func TestAdminRequired_BlocksNonexistentUser(t *testing.T) {
	client := newAdminTestClient(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/admin", nil)
	c.Set("user_id", int64(99999))

	AdminRequired(client)(c)

	if !c.IsAborted() {
		t.Fatal("expected request with nonexistent user_id to be blocked")
	}
}
