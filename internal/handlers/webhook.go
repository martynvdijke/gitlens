package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"gitoverviewer/ent"
	"gitoverviewer/ent/repository"
	"gitoverviewer/internal/sync"

	"github.com/gin-gonic/gin"
)

type WebhookHandler struct {
	client *ent.Client
	syncer *sync.Syncer
	secret string
}

func NewWebhookHandler(client *ent.Client, syncer *sync.Syncer, secret string) *WebhookHandler {
	return &WebhookHandler{client: client, syncer: syncer, secret: secret}
}

type pushPayload struct {
	Ref        string `json:"ref"`
	Repository struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
}

func (h *WebhookHandler) HandlePush(c *gin.Context) {
	event := c.GetHeader("X-GitHub-Event")
	if event != "push" {
		c.Status(http.StatusOK)
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("Webhook: error reading body: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	if h.secret != "" {
		sig := c.GetHeader("X-Hub-Signature-256")
		if sig == "" {
			log.Printf("Webhook: missing signature")
			c.Status(http.StatusUnauthorized)
			return
		}
		mac := hmac.New(sha256.New, []byte(h.secret))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			log.Printf("Webhook: invalid signature")
			c.Status(http.StatusUnauthorized)
			return
		}
	}

	var payload pushPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("Webhook: error parsing payload: %v", err)
		c.Status(http.StatusBadRequest)
		return
	}

	if payload.Ref == "" || payload.Repository.ID == 0 {
		c.Status(http.StatusOK)
		return
	}

	ctx := c.Request.Context()
	repo, err := h.client.Repository.Query().
		Where(repository.GithubID(payload.Repository.ID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			c.Status(http.StatusOK)
			return
		}
		log.Printf("Webhook: error finding repo %d: %v", payload.Repository.ID, err)
		c.Status(http.StatusInternalServerError)
		return
	}

	log.Printf("Webhook: push to %s/%s — syncing", repo.Owner, repo.Name)
	go h.syncer.SyncOne(context.Background(), repo)

	c.Status(http.StatusOK)
}
