package handler

import (
	"bytes"
	"encoding/json"
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
        "address": "tls://8.8.8.8"
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

