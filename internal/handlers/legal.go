package handlers

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"gitlens/ent"
	"gitlens/internal/auth"
	"gitlens/internal/config"
	"gitlens/internal/middleware"

	"github.com/gin-gonic/gin"
)

// LegalData is the identity of the instance operator plus the deployment
// facts the published policy may assert. Values are configuration or read from
// the live admin config, so the text cannot claim a processor the server does
// not run.
type LegalData struct {
	Controller           string
	ContactEmail         string
	ContactAddress       string
	HostingRegion        string
	DPOEmail             string
	EURepresentative     string
	SupervisoryAuthority string
	OIDC                 bool
	Forgejo              bool
	Traces               bool
	Notifications        bool
}

// LegalHandler serves the public privacy policy, terms and account-deletion
// pages. They are reachable without a session because app stores and data
// protection law both require them to be readable before sign-up.
type LegalHandler struct {
	client *ent.Client
	store  *middleware.SessionStore
	cfg    config.Legal
}

func NewLegalHandler(client *ent.Client, store *middleware.SessionStore, cfg config.Legal) *LegalHandler {
	return &LegalHandler{client: client, store: store, cfg: cfg}
}

func (h *LegalHandler) legal(c *gin.Context) LegalData {
	d := LegalData{
		Controller:    "the operator of this GitLens instance",
		HostingRegion: "the European Union",
		OIDC:          auth.LoadConfig().Valid(),
		Forgejo:       strings.TrimSpace(os.Getenv("FORGEJO_CLIENT_ID")) != "",
		Traces:        strings.TrimSpace(os.Getenv("OTEL_ENDPOINT")) != "",
	}
	if v := strings.TrimSpace(h.cfg.ControllerName); v != "" {
		d.Controller = v
	}
	d.ContactEmail = h.cfg.ContactEmail
	d.ContactAddress = h.cfg.ContactAddress
	d.DPOEmail = h.cfg.DPOEmail
	d.EURepresentative = h.cfg.EURepresentative
	d.SupervisoryAuthority = h.cfg.SupervisoryAuthority
	if v := strings.TrimSpace(h.cfg.HostingRegion); v != "" {
		d.HostingRegion = v
	}

	if h.client != nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if cfg, err := h.client.AdminConfig.Get(ctx, 1); err == nil {
			if cfg.OtelEndpoint != "" {
				d.Traces = true
			}
			if cfg.GotifyURL != "" || cfg.GotifyToken != "" || cfg.TelegramBotToken != "" || cfg.TelegramChatID != "" {
				d.Notifications = true
			}
		}
	}
	if strings.TrimSpace(os.Getenv("GOTIFY_URL")) != "" || strings.TrimSpace(os.Getenv("GOTIFY_TOKEN")) != "" {
		d.Notifications = true
	}
	return d
}

// currentUser returns the signed-in user when a session cookie is present.
// The pages never require it, but the deletion instructions are friendlier
// when we know who is asking.
func (h *LegalHandler) currentUser(c *gin.Context) *ent.User {
	if h.client == nil {
		return nil
	}
	var userID int64
	if id, ok := auth.GetUserIDFromSession(c); ok {
		userID = id
	} else if h.store != nil {
		if cookie, err := c.Cookie("gitlens_session"); err == nil {
			if id, ok := h.store.Get(cookie); ok {
				userID = id
			}
		}
	}
	if userID == 0 {
		return nil
	}
	u, err := h.client.User.Get(c.Request.Context(), int(userID))
	if err != nil {
		return nil
	}
	return u
}

func (h *LegalHandler) base(c *gin.Context, title, description string) gin.H {
	return gin.H{
		"AppName":         "GitLens",
		"Year":            time.Now().Year(),
		"Title":           title,
		"MetaDescription": description,
		"Legal":           h.legal(c),
		"User":            h.currentUser(c),
		"OIDCEnabled":     auth.LoadConfig().Valid(),
	}
}

func (h *LegalHandler) Privacy(c *gin.Context) {
	d := h.base(c, "Privacy", "What GitLens stores, why, who processes it, and how to get it back or delete it.")
	d["LegalPage"] = "privacy"
	c.HTML(http.StatusOK, "index.html", d)
}

func (h *LegalHandler) Terms(c *gin.Context) {
	d := h.base(c, "Terms", "The rules for using this GitLens instance.")
	d["LegalPage"] = "terms"
	c.HTML(http.StatusOK, "index.html", d)
}

func (h *LegalHandler) AccountDelete(c *gin.Context) {
	d := h.base(c, "Delete your account", "How to delete a GitLens account and what happens to the data afterwards.")
	d["LegalPage"] = "account_delete"
	c.HTML(http.StatusOK, "index.html", d)
}
