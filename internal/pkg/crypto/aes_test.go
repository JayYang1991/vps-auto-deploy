package crypto

import (
	"strings"
	"testing"
)

func TestAESGCMEncryptDecrypt(t *testing.T) {
	masterSecret := "my-very-secret-jwt-key-123456"
	key := DeriveKey(masterSecret)

	rawPassword := "RemoteSecretPassword!#%888"

	encrypted, err := EncryptAESGCM(rawPassword, key)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}

	if encrypted == rawPassword {
		t.Fatalf("Encrypted password should not match plaintext")
	}

	decrypted, err := DecryptAESGCM(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt error: %v", err)
	}

	if decrypted != rawPassword {
		t.Fatalf("Decrypted %s != raw %s", decrypted, rawPassword)
	}

	// Test with wrong key
	wrongKey := DeriveKey("wrong-key")
	_, err = DecryptAESGCM(encrypted, wrongKey)
	if err == nil {
		t.Fatalf("Expected error when decrypting with wrong key, got nil")
	}

	// Test prefix and backward compatibility
	if !strings.HasPrefix(encrypted, CipherPrefix) {
		t.Fatalf("Expected encrypted text to have prefix %s, got %s", CipherPrefix, encrypted)
	}

	// Test legacy un-prefixed ciphertext decryption
	legacyCipher := strings.TrimPrefix(encrypted, CipherPrefix)
	decryptedLegacy, err := DecryptAESGCM(legacyCipher, key)
	if err != nil {
		t.Fatalf("Failed to decrypt legacy un-prefixed ciphertext: %v", err)
	}
	if decryptedLegacy != rawPassword {
		t.Fatalf("Decrypted legacy %s != raw %s", decryptedLegacy, rawPassword)
	}
}

