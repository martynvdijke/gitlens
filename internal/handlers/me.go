package handlers

import (
	"net/http"

	"gitlens/ent"

	"github.com/gin-gonic/gin"
)

// MeHandler exposes the authenticated user to native clients. The web UI gets
// the same information from server-rendered templates.
type MeHandler struct {
	client *ent.Client
}

func NewMeHandler(client *ent.Client) *MeHandler {
	return &MeHandler{client: client}
}

type meUser struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email,omitempty"`
	Role      string `json:"role"`
}

// Me returns the current user resolved by AuthOrToken.
func (h *MeHandler) Me(c *gin.Context) {
	u, err := h.client.User.Get(c.Request.Context(), int(c.GetInt64("user_id")))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	role := "user"
	if u.IsAdmin {
		role = "admin"
	}
	c.JSON(http.StatusOK, gin.H{
		"user": meUser{
			ID:        u.ID,
			Username:  u.Login,
			Name:      u.Name,
			AvatarURL: u.AvatarURL,
			Email:     u.OidcEmail,
			Role:      role,
		},
	})
}
