package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestDeriveRealityKeyHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/inbounds/derive-reality-key", DeriveRealityKey)

	// 1. 成功推导
	priv := "u8Vj9iT15Z7dD9mK2qN_4wA5eR8tY1uI3oP5aS7dF9g" // 43 chars raw url-safe base64 (32 bytes)
	body := fmt.Sprintf(`{"private_key":"%s"}`, priv)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/inbounds/derive-reality-key", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res["public_key"] == "" {
		t.Fatalf("expected non-empty public_key")
	}

	// 2. 空私钥
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/inbounds/derive-reality-key", strings.NewReader(`{"private_key":""}`))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty private key, got %d", w2.Code)
	}

	// 3. 非法私钥
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/inbounds/derive-reality-key", strings.NewReader(`{"private_key":"invalid-key"}`))
	req3.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid private key, got %d", w3.Code)
	}
}

func TestCreateInboundAutoDerivePublicKey(t *testing.T) {
	os.Setenv("SKIP_SERVICE_RELOAD", "1")
	defer os.Unsetenv("SKIP_SERVICE_RELOAD")

	gin.SetMode(gin.TestMode)
	dbPath := setupInboundTestDB(t)
	defer os.Remove(dbPath)

	r := gin.New()
	r.POST("/inbounds", CreateInbound)

	priv := "u8Vj9iT15Z7dD9mK2qN_4wA5eR8tY1uI3oP5aS7dF9g"
	reqPayload := fmt.Sprintf(`{
		"tag": "vless-reality-test",
		"protocol": "vless-reality",
		"port": 443,
		"spec": {
			"server_name": "dl.google.com",
			"private_key": "%s",
			"public_key": "",
			"short_id": "1234abcd"
		},
		"enabled": true
	}`, priv)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/inbounds", strings.NewReader(reqPayload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 验证落盘的 spec_json 自动补齐了 public_key
	var specJSON string
	err := model.DB.QueryRow("SELECT spec_json FROM inbounds WHERE tag = 'vless-reality-test'").Scan(&specJSON)
	if err != nil {
		t.Fatalf("query inbound failed: %v", err)
	}

	var rSpec model.VLESSRealitySpec
	if err := json.Unmarshal([]byte(specJSON), &rSpec); err != nil {
		t.Fatalf("unmarshal spec_json failed: %v", err)
	}

	if rSpec.PublicKey == "" {
		t.Fatalf("expected public_key to be auto-derived, but got empty string")
	}
}
