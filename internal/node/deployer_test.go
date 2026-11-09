package node

import (
	"os"
	"testing"
	"time"
)

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


