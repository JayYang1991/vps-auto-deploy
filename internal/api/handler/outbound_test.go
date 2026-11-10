package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
)

func TestLoadSingboxFromDiskAndFullConfigSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test_sync.db")
	_, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	tempDir := t.TempDir()
	cfgPath := filepath.Join(tempDir, "config.json")
	singbox.SetConfigPathForTest(cfgPath)
	defer singbox.SetConfigPathForTest("")

	// 准备包含 custom dns、入站与出站的磁盘配置
	diskJSON := `{
  "log": {
    "level": "warn",
    "timestamp": true
  },
  "dns": {
    "servers": [
      {
        "tag": "google",
        "type": "tls",
        "server": "8.8.8.8"
      }
    ]
  },
  "inbounds": [
    {
      "tag": "disk-vless",
      "type": "vless",
      "listen": "::",
      "listen_port": 10443,
      "users": [
        {
          "name": "disk-user",
          "uuid": "e6a0d4c8-3561-460f-8975-f5e1ad872912"
        }
      ]
    }
  ],
  "outbounds": [
    {
      "tag": "direct",
      "type": "direct"
    },
    {
      "tag": "disk-socks",
      "type": "socks",
      "server": "1.2.3.4",
      "server_port": 1080
    }
  ]
}`
	if err := os.WriteFile(cfgPath, []byte(diskJSON), 0644); err != nil {
		t.Fatalf("write disk config failed: %v", err)
	}

	r := gin.New()
	r.POST("/api/singbox/load-from-disk", LoadSingboxFromDisk)
	r.GET("/api/singbox/config", GetSingboxFullConfig)

	// 1. 调用从磁盘加载覆盖
	reqLoad, _ := http.NewRequest(http.MethodPost, "/api/singbox/load-from-disk", bytes.NewReader([]byte("{}")))
	reqLoad.Header.Set("Content-Type", "application/json")
	wLoad := httptest.NewRecorder()
	r.ServeHTTP(wLoad, reqLoad)

	if wLoad.Code != http.StatusOK {
		t.Fatalf("expected 200 on load-from-disk, got %d: %s", wLoad.Code, wLoad.Body.String())
	}

	// 2. 调用查看完整配置与同步状态
	reqGet, _ := http.NewRequest(http.MethodGet, "/api/singbox/config", nil)
	wGet := httptest.NewRecorder()
	r.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 on get config, got %d: %s", wGet.Code, wGet.Body.String())
	}

	var res map[string]interface{}
	if err := json.Unmarshal(wGet.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal config resp failed: %v", err)
	}

	isSynced, _ := res["is_synced"].(bool)
	if !isSynced {
		t.Errorf("expected is_synced == true after loadFromDisk, got false. raw_json: %v, disk_raw_json: %v", res["raw_json"], res["disk_raw_json"])
	}

	// 验证 custom dns 块被无损还原
	cfgMap, _ := res["config"].(map[string]interface{})
	if cfgMap["dns"] == nil {
		t.Errorf("expected dns section preserved in config, got nil")
	}
}

func TestDeleteOutboundWithReferencingRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test_outbound_ref.db")
	_, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	r := gin.New()
	r.DELETE("/outbounds/:id", DeleteOutbound)
	r.PUT("/outbounds/:id", UpdateOutbound)

	// 1. 插入测试出站
	res, err := model.DB.Exec(
		"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
		"hk-node", "shadowsocks", "1.1.1.1", 8388, `{"method":"chacha20-ietf-poly1305","password":"test"}`, 10,
	)
	if err != nil {
		t.Fatalf("failed to insert outbound: %v", err)
	}
	obID, _ := res.LastInsertId()

	// 2. 插入绑定该出站的路由规则
	_, err = model.DB.Exec(`
		INSERT INTO routes (name, action, outbound_tag, inbound_tags, domains, domain_suffix, domain_keyword, ip_cidr, ip_is_private, port, port_range, protocol, rule_set, rule_set_urls, invert, priority, enabled, created_at, updated_at)
		VALUES ('流媒体专线路由', 'route', 'hk-node', '', '', '', '', '', 0, '', '', '', '', '', 0, 10, 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatalf("failed to insert route: %v", err)
	}

	// 3. 尝试删除该出站 -> 应被依赖拦截拒绝 (400 Bad Request)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("/outbounds/%d", obID), nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on deleting referenced outbound, got %d: %s", w.Code, w.Body.String())
	}
	var resBody map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resBody)
	if resBody["error"] == "" || !bytes.Contains(w.Body.Bytes(), []byte("流媒体专线路由")) {
		t.Fatalf("expected error mentioning referencing route, got: %v", resBody["error"])
	}

	// 4. 重命名该出站 -> 路由的 outbound_tag 自动级联更新
	updatePayload := `{
		"tag": "hk-node-v2",
		"type": "shadowsocks",
		"server": "1.1.1.1",
		"server_port": 8388,
		"spec": {"method":"chacha20-ietf-poly1305","password":"test"},
		"priority": 10,
		"enabled": true
	}`
	wUpdate := httptest.NewRecorder()
	reqUpdate, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/outbounds/%d", obID), bytes.NewReader([]byte(updatePayload)))
	reqUpdate.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(wUpdate, reqUpdate)

	if wUpdate.Code != http.StatusOK {
		t.Fatalf("expected 200 on update outbound, got %d: %s", wUpdate.Code, wUpdate.Body.String())
	}

	var updatedOutboundTag string
	_ = model.DB.QueryRow("SELECT outbound_tag FROM routes WHERE name = '流媒体专线路由'").Scan(&updatedOutboundTag)
	if updatedOutboundTag != "hk-node-v2" {
		t.Fatalf("expected route outbound_tag cascade updated to 'hk-node-v2', got: %s", updatedOutboundTag)
	}
}

