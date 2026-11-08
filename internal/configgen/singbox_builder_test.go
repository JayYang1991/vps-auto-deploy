package configgen

import (
	"os"
	"path/filepath"
	"testing"

	"vps-auto-deploy/internal/component/singbox"
	"vps-auto-deploy/internal/model"
)

func TestRollbackSingbox(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	oldContent := []byte(`{"log":{"level":"info"}}`)
	newContent := []byte(`{"log":{"level":"debug"},"invalid": true}`)

	// 1. 模拟写入新配置
	if err := os.WriteFile(cfgPath, newContent, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	driver := singbox.NewDriver()
	// 2. 执行回滚 (wasActive = false，避免没有 systemd 时调用报错)
	rollbackSingbox(cfgPath, oldContent, driver, false)

	// 3. 验证内容已恢复为 oldContent
	restored, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read restored config: %v", err)
	}
	if string(restored) != string(oldContent) {
		t.Errorf("expected restored content %s, got %s", string(oldContent), string(restored))
	}

	// 4. 测试原配置为空时的回滚 (应删除目标文件)
	rollbackSingbox(cfgPath, nil, driver, false)
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted when oldConfig is empty")
	}
}

func TestGenerateServerConfig(t *testing.T) {
	// 初始化 SQLite DB
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := model.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	defer db.Close()

	cfg, err := GenerateServerConfig()
	if err != nil {
		t.Fatalf("GenerateServerConfig failed: %v", err)
	}
	if cfg == nil {
		t.Fatalf("expected non-nil SingboxConfig")
	}
	// 验证默认必有 direct 和 block 出站
	hasDirect := false
	hasBlock := false
	for _, ob := range cfg.Outbounds {
		if t, ok := ob["type"].(string); ok {
			if t == "direct" {
				hasDirect = true
			} else if t == "block" {
				hasBlock = true
			}
		}
	}
	if !hasDirect || !hasBlock {
		t.Errorf("expected both direct and block outbounds, got %v", cfg.Outbounds)
	}
}
