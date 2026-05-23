package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"gitlens/ent/enttest"
	"gitlens/internal/github"
	"gitlens/internal/sync"
	"gitlens/internal/ws"

	"github.com/gin-gonic/gin"
	_ "github.com/mattn/go-sqlite3"
)

func newTestWebhookHandler(t *testing.T, secret string) *WebhookHandler {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+t.TempDir()+"/test.db?_fk=1")
	ghClient := github.NewClient("", "")
	hub := ws.NewHub()
	go hub.Run()
	syncer := sync.NewSyncer(client, ghClient, hub)
	return NewWebhookHandler(client, syncer, secret)
}

func TestHandlePush_NonPushEvent(t *testing.T) {
	handler := newTestWebhookHandler(t, "")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("X-GitHub-Event", "pull_request")
	handler.HandlePush(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandlePush_InvalidJSON(t *testing.T) {
	handler := newTestWebhookHandler(t, "")

	w := httptest.NewRecorder()
	c, engine := gin.CreateTestContext(w)
	engine.POST("/webhook/github", handler.HandlePush)
	c.Request = httptest.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte(`not json`)))
	c.Request.Header.Set("X-GitHub-Event", "push")
	engine.ServeHTTP(w, c.Request)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlePush_EmptyPayload(t *testing.T) {
	handler := newTestWebhookHandler(t, "")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte(`{"ref":"","repository":{"id":0}}`)))
	c.Request.Header.Set("X-GitHub-Event", "push")
	handler.HandlePush(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandlePush_UnknownRepo(t *testing.T) {
	handler := newTestWebhookHandler(t, "")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte(`{"ref":"refs/heads/main","repository":{"id":999999}}`)))
	c.Request.Header.Set("X-GitHub-Event", "push")
	handler.HandlePush(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandlePush_WithSecret_ValidSignature(t *testing.T) {
	secret := "test-secret"
	payload := `{"ref":"refs/heads/main","repository":{"id":1}}`

	handler := newTestWebhookHandler(t, secret)

	client := handler.client
	u, _ := client.User.Create().
		SetGithubID(700).SetLogin("webhookuser").SetAccessToken("tok").Save(context.Background())
	client.Repository.Create().
		SetGithubID(1).SetOwner("test").SetName("repo").
		SetFullName("test/repo").SetHTMLURL("https://github.com/test/repo").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(context.Background())

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte(payload)))
	c.Request.Header.Set("X-GitHub-Event", "push")
	c.Request.Header.Set("X-Hub-Signature-256", sig)
	handler.HandlePush(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandlePush_WithSecret_InvalidSignature(t *testing.T) {
	handler := newTestWebhookHandler(t, "test-secret")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.POST("/webhook/github", handler.HandlePush)
	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte(`{"ref":"refs/heads/main","repository":{"id":1}}`)))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandlePush_WithSecret_MissingSignature(t *testing.T) {
	handler := newTestWebhookHandler(t, "test-secret")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	engine := gin.New()
	engine.POST("/webhook/github", handler.HandlePush)
	req := httptest.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte(`{"ref":"refs/heads/main","repository":{"id":1}}`)))
	req.Header.Set("X-GitHub-Event", "push")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandlePush_NoSecret(t *testing.T) {
	handler := newTestWebhookHandler(t, "")

	client := handler.client
	u, _ := client.User.Create().
		SetGithubID(701).SetLogin("webhookuser2").SetAccessToken("tok").Save(context.Background())
	client.Repository.Create().
		SetGithubID(2).SetOwner("test2").SetName("repo2").
		SetFullName("test2/repo2").SetHTMLURL("https://github.com/test2/repo2").
		SetDefaultBranch("main").SetUserID(u.ID).
		Save(context.Background())

	payload := `{"ref":"refs/heads/main","repository":{"id":2}}`

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/webhook/github", bytes.NewReader([]byte(payload)))
	c.Request.Header.Set("X-GitHub-Event", "push")
	handler.HandlePush(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
