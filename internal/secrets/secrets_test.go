package secrets

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Setenv("SETTINGS_ENCRYPTION_KEY", "test-key-material")
	sealed, err := Encrypt("super-secret-token")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if sealed == "super-secret-token" {
		t.Fatal("expected ciphertext, got plaintext")
	}
	plain, err := Decrypt(sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if plain != "super-secret-token" {
		t.Fatalf("Decrypt = %q, want original", plain)
	}
}

func TestDecryptPreservesLegacyPlaintext(t *testing.T) {
	t.Setenv("SETTINGS_ENCRYPTION_KEY", "test-key-material")
	plain, err := Decrypt("legacy-plain")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if plain != "legacy-plain" {
		t.Fatalf("Decrypt = %q", plain)
	}
}

func TestEncryptIsStableForCiphertext(t *testing.T) {
	t.Setenv("SETTINGS_ENCRYPTION_KEY", "test-key-material")
	sealed, err := Encrypt("token")
	if err != nil {
		t.Fatal(err)
	}
	again, err := Encrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if again != sealed {
		t.Fatal("re-encrypting ciphertext should be a no-op")
	}
}

func TestDecryptWrongKeyFails(t *testing.T) {
	t.Setenv("SETTINGS_ENCRYPTION_KEY", "first-key")
	sealed, err := Encrypt("token")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SETTINGS_ENCRYPTION_KEY", "second-key")
	if _, err := Decrypt(sealed); err == nil {
		t.Fatal("expected decryption with wrong key to fail")
	}
}
