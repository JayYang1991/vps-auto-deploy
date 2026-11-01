package model

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

// InitDB 初始化 SQLite 数据库与数据迁移
func InitDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db dir error: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite db error: %w", err)
	}

	DB = db

	if err := migrate(); err != nil {
		return nil, fmt.Errorf("migrate db error: %w", err)
	}

	if err := initDefaults(); err != nil {
		return nil, fmt.Errorf("init defaults error: %w", err)
	}

	return db, nil
}

func migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		uuid TEXT UNIQUE NOT NULL,
		sub_token TEXT UNIQUE NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS inbounds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tag TEXT UNIQUE NOT NULL,
		protocol TEXT NOT NULL,
		listen TEXT NOT NULL DEFAULT '::',
		port INTEGER NOT NULL,
		spec_json TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS tunnels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		mode TEXT NOT NULL DEFAULT 'quick',
		token TEXT NOT NULL DEFAULT '',
		domain TEXT NOT NULL DEFAULT '',
		target_port INTEGER NOT NULL,
		edge_ip_version TEXT NOT NULL DEFAULT 'auto',
		protocol TEXT NOT NULL DEFAULT 'auto',
		status TEXT NOT NULL DEFAULT 'stopped',
		enabled INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`
	_, err := DB.Exec(schema)
	return err
}

func initDefaults() error {
	// 1. 初始化 JWT 密钥
	if _, err := GetSetting("jwt_secret"); err != nil {
		b := make([]byte, 32)
		rand.Read(b)
		SetSetting("jwt_secret", hex.EncodeToString(b))
	}

	// 2. 初始化管理员账号（默认 admin / admin888，提示用户首次登录修改）
	if _, err := GetSetting("admin_username"); err != nil {
		SetSetting("admin_username", "admin")
	}
	if _, err := GetSetting("admin_password_hash"); err != nil {
		hash, _ := bcrypt.GenerateFromPassword([]byte("admin888"), bcrypt.DefaultCost)
		SetSetting("admin_password_hash", string(hash))
	}

	// 3. 初始化安全访问入口 (Secret Base Path)
	if _, err := GetSetting("secret_path"); err != nil {
		b := make([]byte, 4)
		rand.Read(b)
		SetSetting("secret_path", hex.EncodeToString(b))
	}

	// 4. 若无成员，初始化默认所有者成员
	var memberCount int
	err := DB.QueryRow("SELECT COUNT(*) FROM members").Scan(&memberCount)
	if err == nil && memberCount == 0 {
		subBytes := make([]byte, 8)
		rand.Read(subBytes)
		now := time.Now()
		_, _ = DB.Exec(
			"INSERT INTO members (name, uuid, sub_token, enabled, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)",
			"Default Owner",
			uuid.New().String(),
			hex.EncodeToString(subBytes),
			now,
			now,
		)
		log.Println("[Init] Created default owner member")
	}

	return nil
}

// GetSetting 获取单个配置
func GetSetting(key string) (string, error) {
	var val string
	err := DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&val)
	if err != nil {
		return "", err
	}
	return val, nil
}

// SetSetting 写入或更新配置
func SetSetting(key, val string) error {
	_, err := DB.Exec("INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, val)
	return err
}

