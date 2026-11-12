package fcm

import (
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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// testKeyPEM generates an RSA key and returns it with its PKCS#8 PEM form.
func testKeyPEM(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return key, string(pemBytes)
}

func serviceAccountJSON(email, privateKey, tokenURI string) string {
	b, _ := json.Marshal(map[string]string{
		"client_email": email,
		"private_key":  privateKey,
		"token_uri":    tokenURI,
	})
	return string(b)
}

// TestSignJWT verifies the RS256 signature and the OAuth2 claims.
func TestSignJWT(t *testing.T) {
	key, pemKey := testKeyPEM(t)
	sa := serviceAccountJSON("svc@example.iam.gserviceaccount.com", pemKey, "https://oauth2.example/token")
	c := New("proj", sa)
	if c == nil {
		t.Fatal("expected client, got nil")
	}

	now := time.Unix(1700000000, 0)
	token, err := c.signJWT(now)
	if err != nil {
		t.Fatalf("signJWT: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT parts, got %d", len(parts))
	}

	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	digest := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}

	headerBytes, _ := base64.RawURLEncoding.DecodeString(parts[0])
	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("decode header: %v", err)
	}
	if header["alg"] != "RS256" || header["typ"] != "JWT" {
		t.Fatalf("unexpected header: %v", header)
	}

	claimBytes, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims jwtClaims
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if claims.Iss != "svc@example.iam.gserviceaccount.com" {
		t.Errorf("iss = %q", claims.Iss)
	}
	if claims.Scope != fcmScope {
		t.Errorf("scope = %q", claims.Scope)
	}
	if claims.Aud != "https://oauth2.example/token" {
		t.Errorf("aud = %q", claims.Aud)
	}
	if claims.Iat != now.Unix() {
		t.Errorf("iat = %d, want %d", claims.Iat, now.Unix())
	}
	if claims.Exp != now.Add(time.Hour).Unix() {
		t.Errorf("exp = %d, want %d", claims.Exp, now.Add(time.Hour).Unix())
	}
}

func TestNewInvalid(t *testing.T) {
	cases := []struct {
		name      string
		projectID string
		json      string
	}{
		{"empty project", "", `{"client_email":"a","private_key":"b"}`},
		{"empty json", "proj", ""},
		{"invalid json", "proj", "not-json"},
		{"missing email", "proj", `{"private_key":"x"}`},
		{"invalid private key", "proj", `{"client_email":"a@b","private_key":"not-a-pem"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := New(tc.projectID, tc.json); got != nil {
				t.Fatalf("expected nil client, got %+v", got)
			}
		})
	}
}

func TestNilClient(t *testing.T) {
	var c *Client
	if err := c.Send(context.Background(), "tok", "title", "body", nil); err != nil {
		t.Fatalf("nil Send should be a no-op, got %v", err)
	}
	if err := c.Validate(context.Background()); err == nil {
		t.Fatal("nil Validate should return an error")
	}
}

// newTestClient builds a client pointed at a test server. The handler is
// switched per test via the returned sendStatus/sendBody pointers.
func newTestClient(t *testing.T, sendStatus *int, sendBody *string) *Client {
	t.Helper()
	_, pemKey := testKeyPEM(t)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse form: %v", err)
			}
			if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
			}
			if r.Form.Get("assertion") == "" {
				t.Error("missing assertion")
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"test-token","expires_in":3600}`))
		case strings.HasPrefix(r.URL.Path, "/v1/projects/"):
			w.WriteHeader(*sendStatus)
			w.Write([]byte(*sendBody))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	sa := serviceAccountJSON("svc@example.com", pemKey, srv.URL+"/token")
	c := New("proj", sa)
	if c == nil {
		t.Fatal("expected client")
	}
	c.baseURL = srv.URL
	c.tokenURL = srv.URL + "/token"
	return c
}

func TestSendInvalidTokenClassification(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"not found", http.StatusNotFound, `{"error":{"status":"NOT_FOUND"}}`, true},
		{"bad request invalid argument", http.StatusBadRequest, `{"error":{"status":"INVALID_ARGUMENT"}}`, true},
		{"bad request unregistered", http.StatusBadRequest, `{"error":{"details":[{"errorCode":"UNREGISTERED"}]}}`, true},
		{"server error", http.StatusInternalServerError, `{"error":{"status":"INTERNAL"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := tc.status, tc.body
			c := newTestClient(t, &status, &body)
			err := c.Send(context.Background(), "device-token", "Title", "Body", map[string]string{"k": "v"})
			if tc.want {
				if !errors.Is(err, ErrInvalidToken) {
					t.Fatalf("expected ErrInvalidToken, got %v", err)
				}
			} else if err == nil {
				t.Fatal("expected error")
			} else if errors.Is(err, ErrInvalidToken) {
				t.Fatalf("did not expect ErrInvalidToken, got %v", err)
			}
		})
	}
}

func TestSendSuccessAndValidate(t *testing.T) {
	status := http.StatusOK
	body := `{"name":"projects/proj/messages/1"}`
	c := newTestClient(t, &status, &body)

	if err := c.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := c.Send(context.Background(), "device-token", "Title", "Body", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// A second send should reuse the cached token (no extra token request).
	if err := c.Send(context.Background(), "device-token", "Title", "Body", nil); err != nil {
		t.Fatalf("second Send: %v", err)
	}
}

func TestIsInvalidTokenResponse(t *testing.T) {
	if !isInvalidTokenResponse(http.StatusNotFound, nil) {
		t.Error("404 should be invalid token")
	}
	if isInvalidTokenResponse(http.StatusOK, nil) {
		t.Error("200 should not be invalid token")
	}
}
