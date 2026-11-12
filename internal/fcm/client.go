// Package fcm sends Android push notifications through Firebase Cloud
// Messaging (HTTP v1 API). OAuth2 service-account tokens are minted with the
// standard library only, so no Google SDK is required.
package fcm

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultTokenURL = "https://oauth2.googleapis.com/token"
	defaultBaseURL  = "https://fcm.googleapis.com"
	fcmScope        = "https://www.googleapis.com/auth/firebase.messaging"
	// refreshSkew renews the access token shortly before it expires.
	refreshSkew = 60 * time.Second
)

// ErrInvalidToken marks an FCM response that means the device token is no
// longer usable (unregistered or malformed). Callers delete the stored token.
var ErrInvalidToken = errors.New("fcm: invalid or unregistered device token")

// serviceAccount is the subset of a Google service-account JSON we need.
type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// Client is a Firebase Cloud Messaging HTTP v1 client. It caches an OAuth2
// access token in memory and refreshes it as needed.
type Client struct {
	projectID  string
	email      string
	privateKey *rsa.PrivateKey
	tokenURI   string
	http       *http.Client

	// tokenURL and baseURL are overridable in tests.
	tokenURL string
	baseURL  string
	now      func() time.Time

	mu          sync.Mutex
	accessToken string
	expiry      time.Time
}

// New parses a Google service-account JSON document and returns a client.
// It returns nil when projectID or the JSON is empty/invalid; only a generic
// warning is logged so credentials are never written to the log.
func New(projectID, serviceAccountJSON string) *Client {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(serviceAccountJSON) == "" {
		return nil
	}
	var sa serviceAccount
	if err := json.Unmarshal([]byte(serviceAccountJSON), &sa); err != nil || sa.ClientEmail == "" || sa.PrivateKey == "" {
		log.Printf("fcm: service account configuration is invalid")
		return nil
	}
	key, err := parsePrivateKey(sa.PrivateKey)
	if err != nil {
		log.Printf("fcm: service account private key could not be parsed")
		return nil
	}
	tokenURI := sa.TokenURI
	if tokenURI == "" {
		tokenURI = defaultTokenURL
	}
	return &Client{
		projectID:  projectID,
		email:      sa.ClientEmail,
		privateKey: key,
		tokenURI:   tokenURI,
		http:       &http.Client{Timeout: 15 * time.Second},
		tokenURL:   tokenURI,
		baseURL:    defaultBaseURL,
		now:        time.Now,
	}
}

// parsePrivateKey accepts PKCS#8 (Google's format) or PKCS#1 PEM blocks.
func parsePrivateKey(pemKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemKey))
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := k.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, errors.New("not an RSA key")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// jwtClaims is the OAuth2 JWT bearer assertion payload.
type jwtClaims struct {
	Iss   string `json:"iss"`
	Scope string `json:"scope"`
	Aud   string `json:"aud"`
	Iat   int64  `json:"iat"`
	Exp   int64  `json:"exp"`
}

// signJWT builds an RS256 JWT for the service-account OAuth2 flow.
func (c *Client) signJWT(now time.Time) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(jwtClaims{
		Iss:   c.email,
		Scope: fcmScope,
		Aud:   c.tokenURI,
		Iat:   now.Unix(),
		Exp:   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(claims)
	signingInput := header + "." + payload
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, c.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// accessToken returns a cached OAuth2 access token, minting a new one when it
// is missing or close to expiry.
func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.accessToken != "" && c.now().Before(c.expiry.Add(-refreshSkew)) {
		tok := c.accessToken
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	tok, exp, err := c.fetchToken(ctx)
	if err != nil {
		return "", err
	}

	c.mu.Lock()
	c.accessToken = tok
	c.expiry = exp
	c.mu.Unlock()
	return tok, nil
}

// fetchToken exchanges a signed JWT for an OAuth2 access token.
func (c *Client) fetchToken(ctx context.Context) (string, time.Time, error) {
	jwt, err := c.signJWT(c.now())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fcm: sign jwt: %w", err)
	}
	form := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + jwt
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("fcm: token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		// Do not include the response body: it can echo credential material.
		return "", time.Time{}, fmt.Errorf("fcm: token exchange failed: %s", resp.Status)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AccessToken == "" {
		return "", time.Time{}, errors.New("fcm: token response missing access_token")
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	return out.AccessToken, c.now().Add(time.Duration(out.ExpiresIn) * time.Second), nil
}

// sendMessage is the FCM HTTP v1 request envelope.
type sendMessage struct {
	Message message `json:"message"`
}

type message struct {
	Token        string            `json:"token"`
	Notification notification      `json:"notification"`
	Data         map[string]string `json:"data,omitempty"`
	Android      androidConfig     `json:"android"`
}

type notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type androidConfig struct {
	Priority     string              `json:"priority"`
	Notification androidNotification `json:"notification"`
}

type androidNotification struct {
	ChannelID string `json:"channel_id"`
}

// Send delivers a notification to a single device token.
func (c *Client) Send(ctx context.Context, deviceToken, title, body string, data map[string]string) error {
	if c == nil {
		return nil
	}
	if deviceToken == "" {
		return ErrInvalidToken
	}
	tok, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(sendMessage{Message: message{
		Token:        deviceToken,
		Notification: notification{Title: title, Body: body},
		Data:         data,
		Android: androidConfig{
			Priority:     "high",
			Notification: androidNotification{ChannelID: "gitlens"},
		},
	}})
	if err != nil {
		return fmt.Errorf("fcm: marshal: %w", err)
	}

	url := fmt.Sprintf("%s/v1/projects/%s/messages:send", c.baseURL, c.projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("fcm: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("fcm: request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusUnauthorized {
		// The cached token is no longer valid; force a refresh next call.
		c.mu.Lock()
		c.accessToken = ""
		c.expiry = time.Time{}
		c.mu.Unlock()
	}
	if resp.StatusCode >= 300 {
		if isInvalidTokenResponse(resp.StatusCode, respBody) {
			return ErrInvalidToken
		}
		return fmt.Errorf("fcm: send failed: %s", resp.Status)
	}
	return nil
}

// isInvalidTokenResponse recognises the FCM error codes that mean the device
// token itself is unusable.
func isInvalidTokenResponse(status int, body []byte) bool {
	if status == http.StatusNotFound {
		return true
	}
	if status == http.StatusBadRequest {
		s := string(body)
		return strings.Contains(s, "INVALID_ARGUMENT") || strings.Contains(s, "UNREGISTERED")
	}
	return false
}

// Validate fetches an access token, proving the service-account credentials
// work. It is used by the admin "test" button.
func (c *Client) Validate(ctx context.Context) error {
	if c == nil {
		return errors.New("fcm: not configured")
	}
	_, err := c.getAccessToken(ctx)
	return err
}
