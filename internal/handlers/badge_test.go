package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlens/ent/enttest"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func TestBadgeSVG_Structure(t *testing.T) {
	svg := badgeSVG("passing", "#3fb950")
	if !strings.HasPrefix(svg, `<svg`) {
		t.Errorf("expected SVG output, got: %s", svg[:30])
	}
	if !strings.Contains(svg, "passing") {
		t.Errorf("expected 'passing' in badge")
	}
	if !strings.Contains(svg, "#3fb950") {
		t.Errorf("expected green color")
	}
	if !strings.Contains(svg, "workflow") {
		t.Errorf("expected 'workflow' label")
	}
}

func TestBadgeSVG_Failing(t *testing.T) {
	svg := badgeSVG("failing", "#f85149")
	if !strings.Contains(svg, "failing") {
		t.Errorf("expected 'failing' in badge")
	}
	if !strings.Contains(svg, "#f85149") {
		t.Errorf("expected red color")
	}
}

func TestBadgeSVG_Unknown(t *testing.T) {
	svg := badgeSVG("unknown", "#6e7681")
	if !strings.Contains(svg, "unknown") {
		t.Errorf("expected 'unknown' in badge")
	}
}

func TestBadgeSVG_Dimensions(t *testing.T) {
	svg := badgeSVG("passing", "#3fb950")
	if !strings.Contains(svg, `width="140"`) {
		t.Errorf("expected width 140 (70+70)")
	}
	if !strings.Contains(svg, `height="20"`) {
		t.Errorf("expected height 20")
	}
}

func TestBadgeHandler_RepoNotFound(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	handler := NewBadgeHandler(client)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/badge/nonexistent/owner", nil)
	c.Params = []gin.Param{
		{Key: "owner", Value: "nonexistent"},
		{Key: "repo", Value: "owner"},
	}
	handler.Badge(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "unknown") {
		t.Errorf("expected 'unknown' badge for missing repo")
	}
}

func TestBadgeHandler_RepoWithWorkflowStatus(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	handler := NewBadgeHandler(client)

	u, _ := client.User.Create().
		SetGithubID(500).SetLogin("badgeuser").SetAccessToken("tok").Save(context.Background())

	client.Repository.Create().
		SetGithubID(100).SetOwner("test-owner").SetName("test-repo").
		SetFullName("test-owner/test-repo").SetHTMLURL("https://github.com/test-owner/test-repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		SetWorkflowStatus("success").
		Save(context.Background())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/badge/test-owner/test-repo", nil)
	c.Params = []gin.Param{
		{Key: "owner", Value: "test-owner"},
		{Key: "repo", Value: "test-repo"},
	}
	handler.Badge(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "passing") {
		t.Errorf("expected 'passing' badge, got: %s", body)
	}
	if !strings.Contains(body, "#3fb950") {
		t.Errorf("expected green color for success")
	}
}

func TestBadgeHandler_RepoWithFailureStatus(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	handler := NewBadgeHandler(client)

	u, _ := client.User.Create().
		SetGithubID(600).SetLogin("badgeuser2").SetAccessToken("tok").Save(context.Background())

	client.Repository.Create().
		SetGithubID(101).SetOwner("fail-org").SetName("fail-repo").
		SetFullName("fail-org/fail-repo").SetHTMLURL("https://github.com/fail-org/fail-repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		SetWorkflowStatus("failure").
		Save(context.Background())

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/badge/fail-org/fail-repo", nil)
	c.Params = []gin.Param{
		{Key: "owner", Value: "fail-org"},
		{Key: "repo", Value: "fail-repo"},
	}
	handler.Badge(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "failing") {
		t.Errorf("expected 'failing' badge, got: %s", body)
	}
	if !strings.Contains(body, "#f85149") {
		t.Errorf("expected red color for failure")
	}
}

func TestBadgeContentType(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	handler := NewBadgeHandler(client)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/badge/x/y", nil)
	c.Params = []gin.Param{
		{Key: "owner", Value: "x"},
		{Key: "repo", Value: "y"},
	}
	handler.Badge(c)

	ct := w.Header().Get("Content-Type")
	if ct != "image/svg+xml" {
		t.Errorf("expected image/svg+xml Content-Type, got %s", ct)
	}
}
