package node

import (
	"os"
	"strings"
	"testing"
	"time"

	"vps-auto-deploy/internal/model"
)

func TestEnsureMasterSSHKeyPairEncrypted(t *testing.T) {
	_, err := model.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	privPEM, pubAuth, err := EnsureMasterSSHKeyPair()
	if err != nil {
		t.Fatalf("EnsureMasterSSHKeyPair failed: %v", err)
	}
	if !strings.Contains(privPEM, "OPENSSH PRIVATE KEY") {
		t.Errorf("expected valid OpenSSH private key PEM")
	}
	if !strings.Contains(pubAuth, "ssh-ed25519") {
		t.Errorf("expected valid ssh-ed25519 public key")
	}

	// 验证私钥在数据库中被加密，明文键被清空
	encPriv, errEnc := model.GetSetting("cloud_ssh_private_key_encrypted")
	if errEnc != nil || encPriv == "" {
		t.Fatalf("expected encrypted private key to be saved in db")
	}
	if encPriv == privPEM {
		t.Fatalf("private key was saved in plaintext!")
	}
	legacyPriv, _ := model.GetSetting("cloud_ssh_private_key")
	if legacyPriv != "" {
		t.Errorf("legacy plaintext cloud_ssh_private_key should be empty, got: %s", legacyPriv)
	}

	// 再次调用，能够正确解密还原原私钥
	priv2, pub2, err := EnsureMasterSSHKeyPair()
	if err != nil || priv2 != privPEM || pub2 != pubAuth {
		t.Errorf("expected idempotent decrypted key retrieval")
	}
}

func TestDeployTaskPubSub(t *testing.T) {
	task := CreateDeployTask("Node 1", "192.168.1.100")
	if task.ID == "" {
		t.Fatalf("Expected non-empty task ID")
	}

	sub := task.Subscribe()
	defer task.Unsubscribe(sub)

	task.AddLog("Step 1: Connecting")
	task.AddLog("Step 2: Installing")

	select {
	case msg := <-sub:
		if len(msg) == 0 {
			t.Errorf("Expected log message, got empty")
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("Timed out waiting for log")
	}
}

func TestRandomGenerators(t *testing.T) {
	pwd := GenerateRandomPassword(16)
	if len(pwd) != 16 {
		t.Errorf("Expected password length 16, got %d", len(pwd))
	}

	hexStr := GenerateRandomHex(4)
	if len(hexStr) != 8 {
		t.Errorf("Expected 8 hex characters, got %d", len(hexStr))
	}
}

func TestComputeFileSHA256(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_sha256_*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, _ = tmpFile.WriteString("hello vps-panel")
	_ = tmpFile.Close()

	hash, err := computeFileSHA256(tmpFile.Name())
	if err != nil {
		t.Fatalf("computeFileSHA256 error: %v", err)
	}

	// echo -n "hello vps-panel" | sha256sum -> 1da683dc7d6bb150af63073139a60bf2c0dc7ea229f88e748905dfee41420e61
	expected := "1da683dc7d6bb150af63073139a60bf2c0dc7ea229f88e748905dfee41420e61"
	if hash != expected {
		t.Errorf("Expected %s, got %s", expected, hash)
	}
}

func TestFetchRemoteNodeInventory_SSRFIntercept(t *testing.T) {
	targetNode := &model.Node{
		Name:      "Malicious Metadata Node",
		Host:      "169.254.169.254",
		PanelPort: 20888,
		UseTLS:    false,
		APIToken:  "dummy_token",
	}

	_, _, err := FetchRemoteNodeInventory(targetNode)
	if err == nil {
		t.Fatalf("expected SSRF error when connecting to 169.254.169.254, got nil")
	}
	if !strings.Contains(err.Error(), "SSRF") {
		t.Fatalf("expected SSRF intercept message, got: %v", err)
	}
}


