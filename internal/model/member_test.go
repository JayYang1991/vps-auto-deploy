package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListMembersWithStringTimestamp(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "member_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	_, err = InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// 模拟被纳管节点上存储的各种字符串时间戳形态
	formats := []string{
		"2026-11-10 12:05:43",
		"2026-11-10 12:05:43.123",
		"2026-11-10",
		"2026-11-10T12:05:43",
		"2026-11-10T12:05:43Z",
		"Tue Nov 10 12:05:43 2026",
		"2026-11-10 12:05:43.123456789 +0800 CST m=+0.001000000",
		"2026-11-10 12:05:43 +0800 CST",
		"2026-11-10T12:05:43.179567071+08:00",
	}
	for i, f := range formats {
		_, err = DB.Exec(`INSERT INTO members (name, uuid, sub_token, enabled, created_at, updated_at) 
			VALUES (?, ?, ?, 1, ?, ?)`,
			"worker-user-"+string(rune('A'+i)),
			"uuid-"+string(rune('A'+i)),
			"tok-"+string(rune('A'+i)),
			f, f,
		)
		if err != nil {
			t.Fatalf("Insert failed for %s: %v", f, err)
		}
		rows, err := DB.Query("SELECT created_at FROM members WHERE uuid = ?", "uuid-"+string(rune('A'+i)))
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}
		for rows.Next() {
			var tm any
			_ = rows.Scan(&tm)
			t.Logf("format %q -> driver returned type %T: %v", f, tm, tm)
		}
		rows.Close()
	}

	members, err := ListMembers()
	if err != nil {
		t.Fatalf("ListMembers failed: %v", err)
	}

	t.Logf("Successfully fetched %d members", len(members))
}
