package model

import (
	"os"
	"path/filepath"
	"testing"
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
	if fetched.Name != "Tokyo JP-1" || fetched.Host != "1.2.3.4" || fetched.IsMaster {
		t.Fatalf("Fetched node mismatch: %+v", fetched)
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
