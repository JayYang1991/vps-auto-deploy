package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
)

func setupInboundTestDB(t *testing.T) string {
	tmpDB, err := os.CreateTemp("", "inbound_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp db: %v", err)
	}
	dbPath := tmpDB.Name()
	tmpDB.Close()

	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	model.DB = db
	return dbPath
}

func TestInboundCRUDAndIdempotentDelete(t *testing.T) {
	os.Setenv("SKIP_SERVICE_RELOAD", "1")
	defer os.Unsetenv("SKIP_SERVICE_RELOAD")

	gin.SetMode(gin.TestMode)
	dbPath := setupInboundTestDB(t)
	defer os.Remove(dbPath)

	r := gin.New()
	r.GET("/inbounds", ListInbounds)
	r.DELETE("/inbounds/:id", DeleteInbound)

	// 1. Insert a test inbound
	res, err := model.DB.Exec(
		"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"socks-test", "socks", "::", 10086, `{"username":"user","password":"pwd"}`,
	)
	if err != nil {
		t.Fatalf("failed to insert test inbound: %v", err)
	}
	id, _ := res.LastInsertId()

	// 2. Delete the inbound (first time)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/inbounds/10086", nil)
	r.ServeHTTP(w, req)
	// Notice: request with nonexistent id should return 200 OK (idempotent)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for nonexistent id, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodDelete, fmt.Sprintf("/inbounds/%d", id), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on first delete, got %d: %s", w.Code, w.Body.String())
	}

	// Verify inbound is deleted from DB
	var count int
	_ = model.DB.QueryRow("SELECT COUNT(*) FROM inbounds WHERE id = ?", id).Scan(&count)
	if count != 0 {
		t.Fatalf("inbound should be deleted from DB, but count is %d", count)
	}

	// 3. Delete the SAME inbound again (second time, simulating duplicate/retry call)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/inbounds/%d", id), nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 on idempotent second delete, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(w2.Body.Bytes(), &resp)
	if resp["message"] != "inbound already deleted" {
		t.Fatalf("expected 'inbound already deleted' message, got: %s", resp["message"])
	}
}
