package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const CipherPrefix = "v1:gcm:"

// DeriveKey 从主密钥种子安全派生 32 字节 AES-256 密钥 (带专用 Salt KDF)
func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte("vps-auto-deploy-kdf-salt:" + secret))
	return h[:]
}

// DeriveKeyLegacy 保持对历史版本直接 SHA-256 派生密钥的向后兼容
func DeriveKeyLegacy(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

// EncryptAESGCM 使用 AES-256-GCM 对明文进行加密，返回带版本前缀的 Base64 编码 Nonce+Ciphertext
func EncryptAESGCM(plaintext string, key []byte) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(key)
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

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return CipherPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAESGCM 自动识别版本前缀并验证完整性解密密文 (同时兼容历史无前缀密文)
func DecryptAESGCM(encrypted string, key []byte) (string, error) {
	encrypted = strings.TrimSpace(encrypted)
	if encrypted == "" {
		return "", nil
	}

	rawB64 := encrypted
	if strings.HasPrefix(encrypted, CipherPrefix) {
		rawB64 = strings.TrimPrefix(encrypted, CipherPrefix)
	}

	data, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

