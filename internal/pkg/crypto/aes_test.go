package crypto

import (
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
}
