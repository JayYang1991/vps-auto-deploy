package configgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vps-auto-deploy/internal/model"
)

func TestImportExistingSingboxConfig(t *testing.T) {
	// 初始化测试内存/临时数据库
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// 构造测试 sing-box 配置文件
	configJSON := `{
  "log": {
    "level": "info"
  },
  "inbounds": [
    {
      "type": "vless",
      "tag": "vless-reality-20443",
      "listen": "::",
      "listen_port": 20443,
      "users": [
        {
          "name": "jason",
          "uuid": "c3a6027a-7d96-4018-a5d9-34c2e0fbee37",
          "flow": "xtls-rprx-vision"
        }
      ],
      "tls": {
        "enabled": true,
        "server_name": "dl.google.com",
        "reality": {
          "enabled": true,
          "handshake": {
            "server": "dl.google.com",
            "server_port": 443
          },
          "private_key": "private_key_sample",
          "short_id": [
            "a4bde05d"
          ]
        }
      }
    },
    {
      "type": "vless",
      "tag": "vless-ws-10086",
      "listen": "127.0.0.1",
      "listen_port": 10086,
      "users": [
        {
          "name": "jason",
          "uuid": "c3a6027a-7d96-4018-a5d9-34c2e0fbee37"
        }
      ],
      "transport": {
        "path": "/myws",
        "type": "ws"
      }
    }
  ],
  "outbounds": [
    {
      "tag": "direct",
      "type": "direct"
    },
    {
      "tag": "block",
      "type": "block"
    },
    {
      "tag": "my-warp",
      "type": "socks",
      "server": "127.0.0.1",
      "server_port": 40000
    }
  ]
}`

	cfgFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgFile, []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write test config file: %v", err)
	}

	stats, err := ImportExistingSingboxConfig(cfgFile)
	if err != nil {
		t.Fatalf("ImportExistingSingboxConfig failed: %v", err)
	}

	if stats.InboundsImported != 2 {
		t.Errorf("expected 2 inbounds imported, got %d", stats.InboundsImported)
	}
	if stats.MembersImported != 1 {
		t.Errorf("expected 1 member imported, got %d", stats.MembersImported)
	}
	if stats.OutboundsImported != 1 {
		t.Errorf("expected 1 custom outbound imported, got %d", stats.OutboundsImported)
	}

	// 验证 SQLite 中数据已正确插入
	var count int
	_ = model.DB.QueryRow("SELECT COUNT(*) FROM inbounds WHERE tag IN ('vless-reality-20443', 'vless-ws-10086')").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 inbounds in DB, got %d", count)
	}

	var memberName string
	_ = model.DB.QueryRow("SELECT name FROM members WHERE uuid = 'c3a6027a-7d96-4018-a5d9-34c2e0fbee37'").Scan(&memberName)
	if memberName != "jason" {
		t.Errorf("expected member name 'jason', got '%s'", memberName)
	}

	var outboundTag string
	_ = model.DB.QueryRow("SELECT tag FROM outbounds WHERE tag = 'my-warp'").Scan(&outboundTag)
	if outboundTag != "my-warp" {
		t.Errorf("expected outbound tag 'my-warp', got '%s'", outboundTag)
	}
}

func TestForceSyncSingboxConfig(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_sync.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	// 1. 先在数据库里插入一个旧入站与旧出站
	now := filepath.Join("2026-01-01")
	_, err = model.DB.Exec(
		"INSERT INTO inbounds (tag, protocol, listen, port, spec_json, enabled, created_at, updated_at) VALUES ('old-ib', 'vless', '::', 9999, '{}', 1, ?, ?)",
		now, now,
	)
	if err != nil {
		t.Fatalf("failed to seed old inbound: %v", err)
	}
	_, err = model.DB.Exec(
		"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES ('old-ob', 'socks', '1.1.1.1', 1080, '{}', 10, 1, 0, ?, ?)",
		now, now,
	)
	if err != nil {
		t.Fatalf("failed to seed old outbound: %v", err)
	}

	// 2. 构造磁盘文件：不含 old-ib 和 old-ob，但含 new-ib 和更新的 direct 以及 new-hy2 出站
	diskJSON := `{
  "inbounds": [
    {
      "tag": "new-ib",
      "type": "vless",
      "listen": "0.0.0.0",
      "listen_port": 8443,
      "users": [
        {
          "name": "alice",
          "uuid": "e4a7027a-7d96-4018-a5d9-34c2e0fbee99"
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
      "tag": "block",
      "type": "block"
    },
    {
      "tag": "new-hy2",
      "type": "hysteria2",
      "server": "hy2.example.com",
      "server_port": 443,
      "password": "secret_pass_123"
    }
  ]
}`
	cfgFile := filepath.Join(t.TempDir(), "disk_config.json")
	if err := os.WriteFile(cfgFile, []byte(diskJSON), 0644); err != nil {
		t.Fatalf("failed to write disk config: %v", err)
	}

	// 3. 执行从磁盘强制加载覆盖
	stats, err := ForceSyncSingboxConfig(cfgFile)
	if err != nil {
		t.Fatalf("ForceSyncSingboxConfig failed: %v", err)
	}

	if stats.InboundsCreated != 1 {
		t.Errorf("expected 1 inbound created, got %d", stats.InboundsCreated)
	}
	if stats.InboundsDeleted != 1 {
		t.Errorf("expected 1 old inbound deleted, got %d", stats.InboundsDeleted)
	}
	if stats.OutboundsCreated != 1 {
		t.Errorf("expected 1 outbound created, got %d", stats.OutboundsCreated)
	}
	if stats.OutboundsDeleted != 1 {
		t.Errorf("expected 1 old outbound deleted, got %d", stats.OutboundsDeleted)
	}
	if stats.MembersImported != 1 {
		t.Errorf("expected 1 member imported, got %d", stats.MembersImported)
	}

	// 4. 验证数据库结果：old-ib 和 old-ob 已被清理，new-ib 和 new-hy2 存在
	var oldIbCount int
	_ = model.DB.QueryRow("SELECT COUNT(*) FROM inbounds WHERE tag = 'old-ib'").Scan(&oldIbCount)
	if oldIbCount != 0 {
		t.Errorf("expected old-ib deleted, count = %d", oldIbCount)
	}

	var newIbPort int
	_ = model.DB.QueryRow("SELECT port FROM inbounds WHERE tag = 'new-ib'").Scan(&newIbPort)
	if newIbPort != 8443 {
		t.Errorf("expected new-ib port 8443, got %d", newIbPort)
	}

	var oldObCount int
	_ = model.DB.QueryRow("SELECT COUNT(*) FROM outbounds WHERE tag = 'old-ob'").Scan(&oldObCount)
	if oldObCount != 0 {
		t.Errorf("expected old-ob deleted, count = %d", oldObCount)
	}

	var newHy2Pass string
	var newHy2Spec string
	_ = model.DB.QueryRow("SELECT spec_json FROM outbounds WHERE tag = 'new-hy2'").Scan(&newHy2Spec)
	if !strings.Contains(newHy2Spec, "secret_pass_123") {
		t.Errorf("expected spec_json containing secret_pass_123, got %s", newHy2Pass)
	}
}

func TestForceSyncSingboxConfigWithRouteRules(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_routes.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer db.Close()

	diskJSON := `{
  "inbounds": [
    {
      "tag": "vless-in",
      "type": "vless",
      "listen": "::",
      "listen_port": 443
    }
  ],
  "outbounds": [
    {
      "tag": "warp-out",
      "type": "socks",
      "server": "127.0.0.1",
      "server_port": 40000
    }
  ],
  "route": {
    "rules": [
      {
        "inbound": ["vless-in"],
        "outbound": "warp-out"
      },
      {
        "domain_suffix": [".google.com", ".youtube.com"],
        "outbound": "warp-out"
      }
    ]
  }
}`
	cfgFile := filepath.Join(t.TempDir(), "route_config.json")
	if err := os.WriteFile(cfgFile, []byte(diskJSON), 0644); err != nil {
		t.Fatalf("failed to write disk config: %v", err)
	}

	stats, err := ForceSyncSingboxConfig(cfgFile)
	if err != nil {
		t.Fatalf("ForceSyncSingboxConfig failed: %v", err)
	}

	if stats.RoutesCreated != 2 {
		t.Errorf("expected 2 routes created, got %d", stats.RoutesCreated)
	}

	routes, err := model.ListRouteRules()
	if err != nil {
		t.Fatalf("ListRouteRules failed: %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes in database, got %d", len(routes))
	}

	// 验证第一条：inbound 直通 outbound
	r0 := routes[0]
	if r0.InboundTags != "vless-in" || r0.OutboundTag != "warp-out" {
		t.Errorf("expected inbound=vless-in -> warp-out, got inbound=%s, outbound=%s", r0.InboundTags, r0.OutboundTag)
	}
	if !strings.Contains(r0.Name, "vless-in") {
		t.Errorf("expected route name containing vless-in, got %s", r0.Name)
	}

	// 验证第二条：domain_suffix
	r1 := routes[1]
	if !strings.Contains(r1.DomainSuffix, ".google.com") || r1.OutboundTag != "warp-out" {
		t.Errorf("expected domain_suffix containing .google.com -> warp-out, got suffix=%s, outbound=%s", r1.DomainSuffix, r1.OutboundTag)
	}
}


