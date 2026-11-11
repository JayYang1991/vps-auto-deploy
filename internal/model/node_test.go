package model

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"vps-auto-deploy/internal/pkg/crypto"
)

func TestNodeCRUD(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "node_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// Verify master node is created
	nodes, err := ListNodes()
	if err != nil {
		t.Fatalf("ListNodes failed: %v", err)
	}
	if len(nodes) == 0 {
		t.Fatalf("Expected at least 1 node (master), got 0")
	}
	if !nodes[0].IsMaster {
		t.Fatalf("First node should be master")
	}

	// Create a remote node
	remote := &Node{
		Name:              "Tokyo JP-1",
		Host:              "1.2.3.4",
		SSHPort:           22,
		PanelPort:         20888,
		SecretPath:        "tokyo123",
		Username:          "admin",
		PasswordEncrypted: "enc-password-base64",
		APIToken:          "mock-token-abc",
		IsMaster:          false,
		UseTLS:            true,
		CertFingerprint:   "aa:bb:cc",
		SSHHostKey:        "ssh-ed25519-mock",
		CloudProvider:     "vultr",
		CloudInstanceID:   "subid-12345",
		Region:            "nrt",
		Plan:              "vc2-1c-1gb",
		Status:            "online",
		OSInfo:            "linux/amd64",
		LatencyMs:         45,
	}

	id, err := CreateNode(remote)
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}
	if id <= 1 {
		t.Fatalf("Expected ID > 1, got %d", id)
	}

	// Fetch node
	fetched, err := GetNode(id)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	if fetched.Name != "Tokyo JP-1" || fetched.Host != "1.2.3.4" || fetched.IsMaster || fetched.CloudProvider != "vultr" || fetched.Region != "nrt" {
		t.Fatalf("Fetched node mismatch: %+v", fetched)
	}

	// Test Vultr API Key Encrypt / Decrypt
	err = SetVultrAPIKey("VULTR_SECRET_TOKEN_XYZ_123")
	if err != nil {
		t.Fatalf("SetVultrAPIKey failed: %v", err)
	}
	apiKey, err := GetVultrAPIKey()
	if err != nil || apiKey != "VULTR_SECRET_TOKEN_XYZ_123" {
		t.Fatalf("GetVultrAPIKey failed: got %s, err: %v", apiKey, err)
	}

	// Test Key Separation & Settings Cache
	encKey1 := GetEncryptionKey()
	if len(encKey1) != 32 {
		t.Fatalf("expected 32 bytes encryption key, got %d", len(encKey1))
	}
	// Verify cache hits
	_ = SetSetting("cache_test_key", "hello_cache")
	cachedVal, err := GetSetting("cache_test_key")
	if err != nil || cachedVal != "hello_cache" {
		t.Fatalf("expected cached val hello_cache, got %s, err: %v", cachedVal, err)
	}

	// Update status
	_ = UpdateNodeStatus(id, "offline", 999)
	fetched, _ = GetNode(id)
	if fetched.Status != "offline" || fetched.LatencyMs != 999 {
		t.Fatalf("UpdateNodeStatus failed: %+v", fetched)
	}

	// Try deleting master node (should NOT delete)
	_ = DeleteNode(1)
	master, err := GetNode(1)
	if err != nil || master == nil {
		t.Fatalf("Master node should not be deleted")
	}

	// Delete remote node
	err = DeleteNode(id)
	if err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}
	_, err = GetNode(id)
	if err == nil {
		t.Fatalf("Node %d should have been deleted", id)
	}
}

func TestMigrateLegacyEncryptedData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "migrate_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "migrate.db")
	_, err = InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	jwtSecret, _ := GetSetting("jwt_secret")
	legacyKey := crypto.DeriveKey(jwtSecret)

	// 模拟老版本：使用 legacyKey 加密 SSH 密码与节点密码
	origPass := "MySecretSSHPassword123!"
	legacySSHPwdEnc, err := crypto.EncryptAESGCM(origPass, legacyKey)
	if err != nil {
		t.Fatalf("Encrypt error: %v", err)
	}

	remoteNode := &Node{
		Name:                 "Legacy Node",
		Host:                 "8.8.8.8",
		SSHPort:              22,
		PanelPort:            20888,
		SecretPath:           "legacy-entry",
		Username:             "admin",
		PasswordEncrypted:    legacySSHPwdEnc,
		SSHUser:              "root",
		SSHPasswordEncrypted: legacySSHPwdEnc,
		Status:               "online",
	}
	nodeID, err := CreateNode(remoteNode)
	if err != nil {
		t.Fatalf("CreateNode failed: %v", err)
	}

	// 模拟升级场景：生成了一个全新的独立 data_encryption_key
	newRandomKey := []byte("12345678901234567890123456789012")
	_ = SetSetting("data_encryption_key", hex.EncodeToString(newRandomKey))
	ClearSettingsCache()

	// 1. 验证 DecryptSensitiveData 能够平滑回退解密老数据
	decryptedBeforeMigration, err := DecryptSensitiveData(legacySSHPwdEnc)
	if err != nil || decryptedBeforeMigration != origPass {
		t.Fatalf("expected fallback decryption to recover origPass, got %s (err: %v)", decryptedBeforeMigration, err)
	}

	// 2. 执行自动无缝数据迁移
	if err := MigrateLegacyEncryptedData(); err != nil {
		t.Fatalf("MigrateLegacyEncryptedData failed: %v", err)
	}

	// 3. 验证数据库中节点记录已被自动重加密为新密钥
	migratedNode, err := GetNode(nodeID)
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}
	// 此时使用 newRandomKey 直接解密应当成功
	decryptedDirectlyWithNewKey, err := crypto.DecryptAESGCM(migratedNode.SSHPasswordEncrypted, newRandomKey)
	if err != nil || decryptedDirectlyWithNewKey != origPass {
		t.Fatalf("expected migrated ciphertext to be decryptable by newKey, got: %s (err: %v)", decryptedDirectlyWithNewKey, err)
	}
}

