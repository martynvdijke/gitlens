package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"gitlens/ent"
	"gitlens/internal/account"
	"gitlens/internal/auth"
	"gitlens/internal/config"
	"gitlens/internal/demo"
	"gitlens/internal/middleware"

	"github.com/gin-gonic/gin"
)

// AccountHandler serves the self-service account lifecycle: deleting the
// current account and exporting its data. Both the HTML settings page and the
// native app call the same service.
type AccountHandler struct {
	client  *ent.Client
	svc     *account.Service
	demoCfg config.Demo
}

func NewAccountHandler(client *ent.Client, svc *account.Service) *AccountHandler {
	return &AccountHandler{client: client, svc: svc}
}

// SetDemo configures the shared demo account so deleting it self-heals: the
// account is recreated immediately, letting reviewers exercise deletion while
// keeping the demo available for the next person.
func (h *AccountHandler) SetDemo(cfg config.Demo) {
	h.demoCfg = cfg
}

// restoreDemo recreates the demo account after its own deletion.
func (h *AccountHandler) restoreDemo(c *gin.Context, wasDemo bool) {
	if !wasDemo || !h.demoCfg.Enabled {
		return
	}
	if _, err := demo.Ensure(c.Request.Context(), h.client, h.demoCfg); err != nil {
		log.Printf("account: could not restore demo account: %v", err)
	}
}

// typedConfirmation reads the "confirm" value from a form body or a JSON body.
// Callers compare it with the account login before deleting.
func typedConfirmation(c *gin.Context) string {
	if strings.Contains(strings.ToLower(c.ContentType()), "application/json") {
		var body struct {
			Confirm string `json:"confirm"`
		}
		if err := json.NewDecoder(c.Request.Body).Decode(&body); err == nil {
			return strings.TrimSpace(body.Confirm)
		}
		return ""
	}
	return strings.TrimSpace(c.PostForm("confirm"))
}

func (h *AccountHandler) current(c *gin.Context) (*ent.User, bool) {
	u, err := h.client.User.Get(c.Request.Context(), int(c.GetInt64("user_id")))
	if err != nil {
		return nil, false
	}
	return u, true
}

// DeleteSelf handles the HTML settings form (POST /settings/account/delete).
func (h *AccountHandler) DeleteSelf(c *gin.Context) {
	u, ok := h.current(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !account.ConfirmMatches(u.Login, typedConfirmation(c)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation does not match your login"})
		return
	}
	if err := h.svc.Delete(c.Request.Context(), int64(u.ID)); err != nil {
		h.deleteError(c, err)
		return
	}
	h.restoreDemo(c, u.GithubID == demo.GithubID)
	h.clearCookies(c)
	if middleware.WantsJSON(c) {
		c.Status(http.StatusNoContent)
		return
	}
	c.Header("HX-Redirect", "/")
	c.Status(http.StatusOK)
}

// DeleteAPI handles DELETE /api/v1/me for the native app and scripts. It
// requires the same typed confirmation.
func (h *AccountHandler) DeleteAPI(c *gin.Context) {
	u, ok := h.current(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	if !account.ConfirmMatches(u.Login, typedConfirmation(c)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmation does not match your login"})
		return
	}
	if err := h.svc.Delete(c.Request.Context(), int64(u.ID)); err != nil {
		h.deleteError(c, err)
		return
	}
	h.restoreDemo(c, u.GithubID == demo.GithubID)
	h.clearCookies(c)
	c.Status(http.StatusNoContent)
}

func (h *AccountHandler) deleteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, account.ErrLastAdmin):
		c.JSON(http.StatusConflict, gin.H{"error": "the last admin cannot delete their account; promote another user first"})
	case errors.Is(err, account.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
	case errors.Is(err, account.ErrInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid account"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete account"})
	}
}

func (h *AccountHandler) clearCookies(c *gin.Context) {
	middleware.ClearSessionCookie(c)
	auth.ClearSessionCookie(c)
}

// Export returns a JSON copy of the caller's profile, preferences, tracked
// repositories and API token metadata. Secrets and hashes are never included.
func (h *AccountHandler) Export(c *gin.Context) {
	raw, err := h.svc.Export(c.Request.Context(), c.GetInt64("user_id"))
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not export account data"})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="gitlens-export.json"`)
	c.Data(http.StatusOK, "application/json; charset=utf-8", raw)
}
