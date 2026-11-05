package handlers

import (
	"net/http"

	"gitlens/ent"
	"gitlens/ent/user"
	"gitlens/internal/auth"
	"gitlens/internal/config"
	"gitlens/internal/demo"
	"gitlens/internal/middleware"

	"github.com/gin-gonic/gin"
)

// DemoHandler exposes the shared demo login and the public client bootstrap
// config. Both endpoints are intentionally unauthenticated.
type DemoHandler struct {
	client *ent.Client
	store  *middleware.SessionStore
	cfg    config.Demo
}

func NewDemoHandler(client *ent.Client, store *middleware.SessionStore, cfg config.Demo) *DemoHandler {
	return &DemoHandler{client: client, store: store, cfg: cfg}
}

// Login creates a browser session for the shared demo account.
func (h *DemoHandler) Login(c *gin.Context) {
	if !h.cfg.Enabled {
		c.Status(http.StatusNotFound)
		return
	}
	u, err := h.client.User.Query().Where(user.GithubID(demo.GithubID)).Only(c.Request.Context())
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	sessionID := h.store.Set(int64(u.ID))
	if sessionID == "" {
		c.Status(http.StatusInternalServerError)
		return
	}
	middleware.SetSessionCookie(c, sessionID)
	c.Redirect(http.StatusFound, "/")
}

// Config lets the Flutter app probe whether the demo login is available and
// whether OIDC is enabled. A disabled demo never leaks its token.
func (h *DemoHandler) Config(c *gin.Context) {
	login := ""
	token := ""
	if h.cfg.Enabled {
		login = h.cfg.Login
		token = h.cfg.Token
	}
	c.JSON(http.StatusOK, gin.H{
		"oidc_enabled": auth.LoadConfig().Valid(),
		"demo": gin.H{
			"enabled": h.cfg.Enabled,
			"login":   login,
			"token":   token,
		},
	})
}
