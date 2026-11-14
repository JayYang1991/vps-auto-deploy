package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCFPreferredFileDB(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cftest_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	now := time.Now()
	items := []CFPreferredIP{
		{
			IP:        "1.2.3.4",
			Port:      443,
			Tag:       "HK-Test",
			Name:      "CF-HK-Test-01",
			Colo:      "HK",
			Latency:   "20ms",
			Speed:     "50MB/s",
			RawLink:   "vless://test@1.2.3.4:443",
			CreatedAt: now,
			UpdatedAt: now,
		},
	}

	if err := SaveCFPreferredIPs(items); err != nil {
		t.Fatalf("SaveCFPreferredIPs failed: %v", err)
	}

	list, err := ListCFPreferredIPs("")
	if err != nil {
		t.Fatalf("ListCFPreferredIPs failed: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 item, got %d", len(list))
	}

	t.Logf("List returned item: %+v", list[0])
}
