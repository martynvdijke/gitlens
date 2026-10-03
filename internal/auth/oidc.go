package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"

	"gitlens/ent"
	"gitlens/ent/user"
)

type oidcProvider struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
	issuer   string
	clientID string
}

var (
	oidcMu    sync.Mutex
	oidcCache *oidcProvider
)

func getProvider(ctx context.Context, cfg Config) (*oidcProvider, error) {
	oidcMu.Lock()
	defer oidcMu.Unlock()
	if oidcCache != nil && oidcCache.issuer == cfg.IssuerURL && oidcCache.clientID == cfg.ClientID {
		return oidcCache, nil
	}
	p, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, err
	}
	oidcCache = &oidcProvider{
		provider: p,
		verifier: p.Verifier(&oidc.Config{ClientID: cfg.ClientID}),
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     p.Endpoint(),
			RedirectURL:  cfg.RedirectURL,
			Scopes:       cfg.Scopes,
		},
		issuer:   cfg.IssuerURL,
		clientID: cfg.ClientID,
	}
	return oidcCache, nil
}

// ResetProvider clears cache (for tests).
func ResetProvider() {
	oidcMu.Lock()
	defer oidcMu.Unlock()
	oidcCache = nil
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func oidcSecure(c *gin.Context) bool {
	return c.Request != nil && (c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https")
}

func setOIDCCookie(c *gin.Context, name, value string) {
	secure := oidcSecure(c)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(name, value, 300, "/", "", secure, true)
}

func clearOIDCCookies(c *gin.Context) {
	secure := oidcSecure(c)
	for _, n := range []string{"oidc_state", "oidc_nonce", "oidc_verifier"} {
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(n, "", -1, "/", "", secure, true)
	}
}

func clearLegacySessionCookie(c *gin.Context) {
	secure := oidcSecure(c)
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("gitlens_session", "", -1, "/", "", secure, true)
}

type oidcClaims struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	PreferredName string `json:"preferred_username"`
	Nonce         string `json:"nonce"`
}

func StatusHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"enabled": LoadConfig().Valid()})
}

func LoginHandler(entClient *ent.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := LoadConfig()
		if !cfg.Valid() {
			c.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		p, err := getProvider(ctx, cfg)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "oidc discovery failed"})
			return
		}
		state, err := randHex(16)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
			return
		}
		nonce, err := randHex(16)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate nonce"})
			return
		}
		verifier := oauth2.GenerateVerifier()
		setOIDCCookie(c, "oidc_state", state)
		setOIDCCookie(c, "oidc_nonce", nonce)
		setOIDCCookie(c, "oidc_verifier", verifier)
		url := p.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("nonce", nonce))
		c.Redirect(http.StatusFound, url)
	}
}

func CallbackHandler(entClient *ent.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := LoadConfig()
		if !cfg.Valid() {
			c.JSON(http.StatusNotFound, gin.H{"error": "oidc disabled"})
			return
		}
		stateCookie, err1 := c.Cookie("oidc_state")
		nonceCookie, err2 := c.Cookie("oidc_nonce")
		verifier, err3 := c.Cookie("oidc_verifier")
		if err1 != nil || err2 != nil || err3 != nil || stateCookie == "" || nonceCookie == "" || verifier == "" {
			clearOIDCCookies(c)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
			return
		}
		qState := c.Query("state")
		if qState == "" || subtle.ConstantTimeCompare([]byte(qState), []byte(stateCookie)) != 1 {
			clearOIDCCookies(c)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid state"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
		defer cancel()
		p, err := getProvider(ctx, cfg)
		if err != nil {
			clearOIDCCookies(c)
			c.JSON(http.StatusBadGateway, gin.H{"error": "oidc discovery failed"})
			return
		}
		code := c.Query("code")
		if code == "" {
			clearOIDCCookies(c)
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing code"})
			return
		}
		token, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
		if err != nil {
			clearOIDCCookies(c)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "code exchange failed"})
			return
		}
		raw, ok := token.Extra("id_token").(string)
		if !ok || raw == "" {
			clearOIDCCookies(c)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "no id_token"})
			return
		}
		idToken, err := p.verifier.Verify(ctx, raw)
		if err != nil {
			clearOIDCCookies(c)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid id_token"})
			return
		}
		var claims oidcClaims
		if err := idToken.Claims(&claims); err != nil {
			clearOIDCCookies(c)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}
		if subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonceCookie)) != 1 {
			clearOIDCCookies(c)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid nonce"})
			return
		}
		if claims.Email == "" || !claims.EmailVerified {
			clearOIDCCookies(c)
			c.JSON(http.StatusForbidden, gin.H{"error": "verified email required"})
			return
		}
		claims.Email = strings.ToLower(strings.TrimSpace(claims.Email))
		// groups claim presence (vs value) decides whether role/groups sync
		var rawClaims map[string]any
		groups := []string{}
		groupsPresent := false
		var rawGroups any
		if err := idToken.Claims(&rawClaims); err == nil {
			rawGroups = rawClaims["groups"]
			if rawGroups != nil {
				groupsPresent = true
				if arr, ok := rawGroups.([]any); ok {
					for _, v := range arr {
						if s, ok := v.(string); ok {
							groups = append(groups, s)
						}
					}
				}
			}
		}
		sub := cfg.IssuerURL + "|" + claims.Sub
		u, err := linkOrProvision(c.Request.Context(), entClient, sub, claims, groups, groupsPresent)
		if err != nil {
			clearOIDCCookies(c)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "user provisioning failed"})
			return
		}
		clearOIDCCookies(c)
		SetSessionCookie(c, int64(u.ID))
		clearLegacySessionCookie(c)
		c.Redirect(http.StatusFound, "/")
	}
}

func LogoutHandler(c *gin.Context) {
	ClearSessionCookie(c)
	clearLegacySessionCookie(c)
	cfg := LoadConfig()
	if cfg.IssuerURL != "" {
		c.Redirect(http.StatusFound, cfg.LogoutURL)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func isAdminGroup(groups []string) bool {
	for _, g := range groups {
		if g == "admins" {
			return true
		}
	}
	return false
}

func linkOrProvision(ctx context.Context, client *ent.Client, sub string, claims oidcClaims, groups []string, groupsPresent bool) (*ent.User, error) {
	var groupsJSON string
	if groupsPresent {
		b, _ := json.Marshal(groups)
		groupsJSON = string(b)
	}
	// by oidc_sub
	if u, err := client.User.Query().Where(user.OidcSub(sub)).Only(ctx); err == nil {
		upd := client.User.UpdateOne(u).SetOidcEmail(claims.Email)
		if groupsPresent {
			upd = upd.SetGroups(groupsJSON).SetIsAdmin(isAdminGroup(groups))
		}
		if claims.Name != "" {
			upd = upd.SetName(claims.Name)
		}
		if uu, err := upd.Save(ctx); err == nil {
			return uu, nil
		}
		return u, nil
	}
	// by email (case-insensitive: we store lowercased)
	if u, err := client.User.Query().Where(user.OidcEmail(claims.Email)).Only(ctx); err == nil {
		upd := client.User.UpdateOne(u).SetOidcSub(sub)
		if groupsPresent {
			upd = upd.SetGroups(groupsJSON).SetIsAdmin(isAdminGroup(groups))
		}
		if uu, err := upd.Save(ctx); err == nil {
			return uu, nil
		}
		return u, nil
	}
	// also try legacy? users without oidc_email but login matching email local part — fallback to none, provision
	// Provision new user. Need github_id unique — use negative random or derive from sub hash? Ent requires github_id unique.
	// Use a placeholder github_id based on existing max + random. Simplest: try incrementing from high value.
	// Find unused github_id
	var gid int64 = 9000000000 // high offset for oidc users
	for {
		_, err := client.User.Query().Where(user.GithubID(gid)).Only(ctx)
		if ent.IsNotFound(err) {
			break
		}
		gid++
	}
	login := claims.PreferredName
	if login == "" {
		login = claims.Name
	}
	if login == "" {
		if i := strings.Index(claims.Email, "@"); i > 0 {
			login = claims.Email[:i]
		} else {
			login = claims.Email
		}
	}
	// ensure login uniqueness by suffixing if needed
	baseLogin := login
	for i := 2; ; i++ {
		_, err := client.User.Query().Where(user.Login(login)).Only(ctx)
		if ent.IsNotFound(err) {
			break
		}
		login = baseLogin + "-" + itoa(i)
		if i > 100 {
			break
		}
	}
	create := client.User.Create().
		SetGithubID(gid).
		SetLogin(login).
		SetAccessToken("oidc").
		SetOidcSub(sub).
		SetOidcEmail(claims.Email)
	if groupsPresent {
		create = create.SetGroups(groupsJSON).SetIsAdmin(isAdminGroup(groups))
	} else {
		// no groups claim: keep default is_admin logic below (first user admin)
	}
	if claims.Name != "" {
		create = create.SetName(claims.Name)
	}
	// first user becomes admin (single-admin by design) if no groups claim present
	count, _ := client.User.Query().Count(ctx)
	if count == 0 && !groupsPresent {
		create = create.SetIsAdmin(true)
	}
	return create.Save(ctx)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for n := i; n > 0; n /= 10 {
		p--
		b[p] = byte('0' + n%10)
	}
	return string(b[p:])
}
