package handlers

import (
	"log"
	"net/http"
	"os"

	"gitlens/ent"
	"gitlens/ent/user"
	"gitlens/internal/github"
	"gitlens/internal/middleware"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	client *ent.Client
	store  *middleware.SessionStore
	gh     *github.Client
}

func NewAuthHandler(client *ent.Client, store *middleware.SessionStore, gh *github.Client) *AuthHandler {
	return &AuthHandler{client: client, store: store, gh: gh}
}

func (h *AuthHandler) Login(c *gin.Context) {
	redirectURL := os.Getenv("GITHUB_REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = "http://localhost:6270/auth/github/callback"
		log.Println("WARNING: GITHUB_REDIRECT_URL not set, using default: " + redirectURL + ". Set this env var to your deployed callback URL (e.g. https://yourdomain.com/auth/github/callback)")
	}
	clientID := os.Getenv("GITHUB_CLIENT_ID")
	url := "https://github.com/login/oauth/authorize?client_id=" + clientID + "&redirect_uri=" + redirectURL + "&scope=repo,read:user"
	c.Redirect(http.StatusFound, url)
}

func (h *AuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"Error": "Missing authorization code"})
		return
	}

	token, err := h.gh.GetAccessToken(code)
	if err != nil {
		log.Printf("OAuth token exchange error: %v", err)
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Failed to authenticate with GitHub"})
		return
	}

	ghUser, err := h.gh.GetUser(token)
	if err != nil {
		log.Printf("GitHub user fetch error: %v", err)
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Failed to fetch user from GitHub"})
		return
	}

	u, err := h.client.User.Query().Where(user.GithubID(ghUser.ID)).Only(c.Request.Context())
	if ent.IsNotFound(err) {
		count, _ := h.client.User.Query().Count(c.Request.Context())

		create := h.client.User.Create().
			SetGithubID(ghUser.ID).
			SetLogin(ghUser.Login).
			SetAvatarURL(ghUser.AvatarURL).
			SetName(ghUser.Name).
			SetAccessToken(token)

		if count == 0 {
			create.SetIsAdmin(true)
			log.Printf("First user detected — promoting %s to admin", ghUser.Login)
		}

		u, err = create.Save(c.Request.Context())
		if err != nil {
			log.Printf("User create error: %v", err)
			c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Failed to create user"})
			return
		}
	} else if err != nil {
		log.Printf("User query error: %v", err)
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Database error"})
		return
	} else {
		u, err = h.client.User.UpdateOne(u).
			SetLogin(ghUser.Login).
			SetAvatarURL(ghUser.AvatarURL).
			SetName(ghUser.Name).
			SetAccessToken(token).
			Save(c.Request.Context())
		if err != nil {
			log.Printf("User update error: %v", err)
			c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Failed to update user"})
			return
		}
	}

	sessionID := h.store.Set(int64(u.ID))
	middleware.SetSessionCookie(c, sessionID)
	c.Redirect(http.StatusFound, "/")
}

func (h *AuthHandler) Logout(c *gin.Context) {
	sessionID, _ := c.Cookie("gitlens_session")
	if sessionID != "" {
		h.store.Delete(sessionID)
	}
	middleware.ClearSessionCookie(c)
	c.Redirect(http.StatusFound, "/")
}
