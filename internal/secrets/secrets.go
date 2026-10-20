// Package secrets provides transparent at-rest encryption for credentials
// stored in the application database.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
)

const prefix = "enc:v1:"

// ErrInvalidCiphertext is returned when a stored value cannot be decrypted.
var ErrInvalidCiphertext = errors.New("invalid encrypted value")

// key returns the configured encryption key material. SETTINGS_ENCRYPTION_KEY
// is preferred; SESSION_SECRET is accepted as a fallback so existing
// deployments gain encryption without a new variable.
func key() []byte {
	material := os.Getenv("SETTINGS_ENCRYPTION_KEY")
	if material == "" {
		material = os.Getenv("SESSION_SECRET")
	}
	if material == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(material))
	return sum[:]
}

// Enabled reports whether at-rest encryption is configured.
func Enabled() bool { return key() != nil }

// Encrypt seals plaintext with AES-256-GCM. With no key configured the value
// is returned unchanged for backwards compatibility; callers can use Enabled
// to warn about that state.
func Encrypt(plain string) (string, error) {
	if plain == "" || strings.HasPrefix(plain, prefix) {
		return plain, nil
	}
	k := key()
	if k == nil {
		return plain, nil
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt opens a value produced by Encrypt. Plaintext legacy values are
// returned unchanged so existing rows keep working after an upgrade.
func Decrypt(stored string) (string, error) {
	if stored == "" || !strings.HasPrefix(stored, prefix) {
		return stored, nil
	}
	k := key()
	if k == nil {
		return "", ErrInvalidCiphertext
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	if len(raw) < gcm.NonceSize() {
		return "", ErrInvalidCiphertext
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrInvalidCiphertext
	}
	return string(plain), nil
}
