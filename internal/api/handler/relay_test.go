package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/model"
)

func TestRelayCandidatesAndBuildOutbound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	// 1. 插入一个本地入站规则
	res, err := model.DB.Exec(
		"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"vless-in", "vless-reality", "::", 443, `{"server_name":"dl.google.com","public_key":"mockPub123=","short_id":"abcd1234"}`,
	)
	if err != nil {
		t.Fatalf("insert inbound failed: %v", err)
	}
	ibID, _ := res.LastInsertId()

	r := gin.New()
	r.GET("/api/cluster/relay-candidates", GetRelayCandidates)
	r.POST("/api/cluster/build-relay-outbound", BuildRelayOutbound)

	// 2. 测试当 callerNodeID = 2 (模拟在从节点视角) 请求候选列表 -> 应当包含主控 (ID=1) 及其入站
	reqGet := httptest.NewRequest(http.MethodGet, "/api/cluster/relay-candidates", nil)
	reqGet.Header.Set("X-Target-Node-ID", "2")
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", wGet.Code, wGet.Body.String())
	}

	var candidates []map[string]interface{}
	if err := json.Unmarshal(wGet.Body.Bytes(), &candidates); err != nil {
		t.Fatalf("unmarshal candidates failed: %v", err)
	}

	if len(candidates) == 0 {
		t.Fatalf("expected at least 1 candidate node (master), got 0")
	}

	// 3. 测试 BuildRelayOutbound (将主控的 Inbound 转为 Outbound)
	buildPayload := map[string]interface{}{
		"target_node_id": 1,
		"inbound_id":    ibID,
		"addr_type":     "ip",
	}
	payloadBytes, _ := json.Marshal(buildPayload)
	reqPost := httptest.NewRequest(http.MethodPost, "/api/cluster/build-relay-outbound", bytes.NewReader(payloadBytes))
	reqPost.Header.Set("Content-Type", "application/json")
	wPost := httptest.NewRecorder()
	r.ServeHTTP(wPost, reqPost)

	if wPost.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d: %s", wPost.Code, wPost.Body.String())
	}

	var ob model.Outbound
	if err := json.Unmarshal(wPost.Body.Bytes(), &ob); err != nil {
		t.Fatalf("unmarshal outbound failed: %v", err)
	}

	if ob.Type != "vless" {
		t.Errorf("expected type vless, got %s", ob.Type)
	}
	if ob.ServerPort != 443 {
		t.Errorf("expected server_port 443, got %d", ob.ServerPort)
	}
	if !bytes.Contains([]byte(ob.SpecJSON), []byte("dl.google.com")) {
		t.Errorf("spec_json missing server_name: %s", ob.SpecJSON)
	}
}
