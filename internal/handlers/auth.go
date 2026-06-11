package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"

	"gitlens/ent"
	"gitlens/ent/user"
	"gitlens/internal/github"
	"gitlens/internal/middleware"
	"gitlens/internal/provider"

	"github.com/gin-gonic/gin"
)

const oauthStateCookieName = "oauth_state"
const oauthStateMaxAge = 600 // 10 minutes in seconds

type AuthHandler struct {
	client    *ent.Client
	store     *middleware.SessionStore
	gh        *github.Client
	providers map[string]provider.Provider
}

func NewAuthHandler(client *ent.Client, store *middleware.SessionStore, gh *github.Client, providers map[string]provider.Provider) *AuthHandler {
	return &AuthHandler{client: client, store: store, gh: gh, providers: providers}
}

// generateState returns a random hex string suitable for OAuth CSRF protection.
func generateState() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// setOAuthStateCookie stores a short-lived cookie with the OAuth state
// value. Extra data (e.g. forgejo instance URL) may be appended as a
// pipe-delimited suffix: state|instanceBase.
func setOAuthStateCookie(c *gin.Context, value string) {
	secure := false
	if c.Request != nil {
		secure = c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	}
	c.SetCookie(oauthStateCookieName, value, oauthStateMaxAge, "/", "", secure, true)
}

// clearOAuthStateCookie removes the OAuth state cookie.
func clearOAuthStateCookie(c *gin.Context) {
	c.SetCookie(oauthStateCookieName, "", -1, "/", "", false, true)
}

// readOAuthStateCookie returns the raw value of the OAuth state cookie.
func readOAuthStateCookie(c *gin.Context) string {
	v, err := c.Cookie(oauthStateCookieName)
	if err != nil {
		return ""
	}
	return v
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

// LoginForgejo starts the Forgejo OAuth flow. It accepts an optional
// ?instance=<base_url> query parameter. If FORGEJO_DEFAULT_URL is set
// and no instance is provided, that default is used. If neither is
// available, a 400 is returned (the frontend should have already
// prompted for an instance URL before calling this endpoint).
func (h *AuthHandler) LoginForgejo(c *gin.Context) {
	p, ok := h.providers["forgejo"]
	if !ok || p == nil {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"Error": "Forgejo is not configured. Set FORGEJO_CLIENT_ID and FORGEJO_CLIENT_SECRET."})
		return
	}

	instance := c.Query("instance")
	state := generateState()
	redirectURL := os.Getenv("FORGEJO_REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = os.Getenv("APP_URL")
		if redirectURL != "" {
			redirectURL = redirectURL + "/auth/forgejo/callback"
		} else {
			redirectURL = "http://localhost:6270/auth/forgejo/callback"
		}
	}

	// Build the AuthURL with the (optional) instance override.
	// The provider's AuthURL method uses its default URL; we use
	// a type assertion to pass the instance override.
	var authURL string
	if fj, ok := p.(*provider.ForgejoAdapter); ok {
		authURL = fj.AuthURLFor(instance, state, redirectURL)
	} else {
		authURL = p.AuthURL(state, redirectURL)
	}

	if authURL == "" {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"Error": "Could not determine Forgejo instance URL. Provide an ?instance= parameter or set FORGEJO_DEFAULT_URL."})
		return
	}

	// Store state + instance in the cookie so the callback can
	// validate and reconstruct the instance URL.
	cookieValue := state
	if instance != "" {
		cookieValue += "|" + instance
	}
	setOAuthStateCookie(c, cookieValue)

	c.Redirect(http.StatusFound, authURL)
}

// CallbackForgejo handles the OAuth callback from a Forgejo instance.
func (h *AuthHandler) CallbackForgejo(c *gin.Context) {
	p, ok := h.providers["forgejo"]
	if !ok || p == nil {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"Error": "Forgejo is not configured"})
		return
	}

	code := c.Query("code")
	if code == "" {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"Error": "Missing authorization code"})
		return
	}

	// Validate CSRF state.
	queryState := c.Query("state")
	cookieVal := readOAuthStateCookie(c)
	clearOAuthStateCookie(c)

	if cookieVal == "" || queryState == "" {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"Error": "OAuth state mismatch"})
		return
	}

	// Cookie format: state|instanceBase (instance is optional).
	storedState := cookieVal
	var instanceBase string
	if idx := indexOfPipe(cookieVal); idx >= 0 {
		storedState = cookieVal[:idx]
		instanceBase = cookieVal[idx+1:]
	}

	if storedState != queryState {
		c.HTML(http.StatusBadRequest, "index.html", gin.H{"Error": "OAuth state mismatch"})
		return
	}

	redirectURL := os.Getenv("FORGEJO_REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = os.Getenv("APP_URL")
		if redirectURL != "" {
			redirectURL = redirectURL + "/auth/forgejo/callback"
		} else {
			redirectURL = "http://localhost:6270/auth/forgejo/callback"
		}
	}

	// Exchange code — pass the instance base so ExchangeCodeFor can
	// use the correct instance.
	var token string
	var fjUser *github.User
	var err error
	if fa, ok := p.(*provider.ForgejoAdapter); ok && instanceBase != "" {
		token, fjUser, err = fa.ExchangeCodeFor(c.Request.Context(), instanceBase, code, redirectURL)
	} else {
		token, fjUser, err = p.ExchangeCode(c.Request.Context(), code, redirectURL)
	}
	if err != nil {
		log.Printf("Forgejo OAuth token exchange/identity error: %v", err)
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Failed to authenticate with Forgejo"})
		return
	}

	// Resolve the instance URL for storage.
	forgejoURL := instanceBase
	if forgejoURL == "" {
		if fa, ok := p.(*provider.ForgejoAdapter); ok {
			forgejoURL = fa.DefaultURL()
		}
	}

	// Upsert User by (forgejo_id, forgejo_url).
	// First try to find by forgejo_id on the same instance.
	u, err := h.client.User.Query().
		Where(user.ForgejoID(fjUser.ID)).
		Where(user.ForgejoURL(forgejoURL)).
		Only(c.Request.Context())
	if ent.IsNotFound(err) {
		// Check if this user already exists via GitHub (same web session).
		currentUserID := c.GetInt64("user_id")
		if currentUserID > 0 {
			// User is logged in via GitHub — link the forgejo account.
			u, err = h.client.User.UpdateOneID(int(currentUserID)).
				SetForgejoID(fjUser.ID).
				SetForgejoLogin(fjUser.Login).
				SetForgejoAvatarURL(fjUser.AvatarURL).
				SetForgejoName(fjUser.Name).
				SetForgejoAccessToken(token).
				SetForgejoURL(forgejoURL).
				Save(c.Request.Context())
			if err != nil {
				log.Printf("User update error (link forgejo): %v", err)
				c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Failed to update user"})
				return
			}
		} else {
			// New user — create with forgejo credentials.
			count, _ := h.client.User.Query().Count(c.Request.Context())
			create := h.client.User.Create().
				SetForgejoID(fjUser.ID).
				SetForgejoLogin(fjUser.Login).
				SetForgejoAvatarURL(fjUser.AvatarURL).
				SetForgejoName(fjUser.Name).
				SetForgejoAccessToken(token).
				SetForgejoURL(forgejoURL)
			if count == 0 {
				create.SetIsAdmin(true)
				log.Printf("First user detected — promoting %s to admin", fjUser.Login)
			}
			u, err = create.Save(c.Request.Context())
			if err != nil {
				log.Printf("User create error: %v", err)
				c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Failed to create user"})
				return
			}
		}
	} else if err != nil {
		log.Printf("User query error: %v", err)
		c.HTML(http.StatusInternalServerError, "index.html", gin.H{"Error": "Database error"})
		return
	} else {
		// Existing forgejo user — update token and info.
		u, err = h.client.User.UpdateOne(u).
			SetForgejoLogin(fjUser.Login).
			SetForgejoAvatarURL(fjUser.AvatarURL).
			SetForgejoName(fjUser.Name).
			SetForgejoAccessToken(token).
			SetForgejoURL(forgejoURL).
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

// indexOfPipe returns the index of the first '|' in s, or -1 if not found.
func indexOfPipe(s string) int {
	for i, r := range s {
		if r == '|' {
			return i
		}
	}
	return -1
}
