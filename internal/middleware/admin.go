package middleware

import (
	"context"
	"net/http"

	"gitlens/ent"

	"github.com/gin-gonic/gin"
)

// AdminRequired returns a middleware that ensures the authenticated user has
// is_admin = true.  It must be placed after AuthRequired (which sets user_id
// in the gin context).
func AdminRequired(client *ent.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			abortForbidden(c)
			return
		}

		id, ok := userID.(int64)
		if !ok {
			abortForbidden(c)
			return
		}

		user, err := client.User.Get(context.Background(), int(id))
		if err != nil || !user.IsAdmin {
			abortForbidden(c)
			return
		}

		c.Next()
	}
}

func abortForbidden(c *gin.Context) {
	if c.GetHeader("HX-Request") != "" {
		c.Header("HX-Redirect", "/")
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Redirect(http.StatusFound, "/")
	c.Abort()
}
