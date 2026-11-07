package model

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
	"vps-auto-deploy/internal/pkg/crypto"
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

	CREATE TABLE IF NOT EXISTS outbounds (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tag TEXT UNIQUE NOT NULL,
		type TEXT NOT NULL,
		server TEXT NOT NULL DEFAULT '',
		server_port INTEGER NOT NULL DEFAULT 0,
		spec_json TEXT NOT NULL DEFAULT '{}',
		priority INTEGER NOT NULL DEFAULT 10,
		enabled INTEGER NOT NULL DEFAULT 1,
		is_system INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		host TEXT NOT NULL,
		ssh_port INTEGER NOT NULL DEFAULT 22,
		panel_port INTEGER NOT NULL DEFAULT 20888,
		secret_path TEXT NOT NULL DEFAULT '',
		username TEXT NOT NULL DEFAULT 'admin',
		password_encrypted TEXT NOT NULL DEFAULT '',
		api_token TEXT NOT NULL DEFAULT '',
		is_master INTEGER NOT NULL DEFAULT 0,
		use_tls INTEGER NOT NULL DEFAULT 0,
		cert_fingerprint TEXT NOT NULL DEFAULT '',
		ssh_host_key TEXT NOT NULL DEFAULT '',
		cloud_provider TEXT NOT NULL DEFAULT '',
		cloud_instance_id TEXT NOT NULL DEFAULT '',
		region TEXT NOT NULL DEFAULT '',
		plan TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'online',
		os_info TEXT NOT NULL DEFAULT '',
		latency_ms INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	`
	if _, err := DB.Exec(schema); err != nil {
		return err
	}

	// 增量字段平滑迁移 (防御 MITM 指纹字段与云主机元数据)
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN use_tls INTEGER NOT NULL DEFAULT 0")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN cert_fingerprint TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN ssh_host_key TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN cloud_provider TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN cloud_instance_id TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN region TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN plan TEXT NOT NULL DEFAULT ''")

	return nil
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

	// 5. 初始化默认系统出站 (direct / block)
	var outboundCount int
	err = DB.QueryRow("SELECT COUNT(*) FROM outbounds").Scan(&outboundCount)
	if err == nil && outboundCount == 0 {
		now := time.Now()
		_, _ = DB.Exec(
			"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			"direct", "direct", "", 0, "{}", 1, 1, 1, now, now,
		)
		_, _ = DB.Exec(
			"INSERT INTO outbounds (tag, type, server, server_port, spec_json, priority, enabled, is_system, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			"block", "block", "", 0, "{}", 99, 1, 1, now, now,
		)
		log.Println("[Init] Created default direct & block outbounds")
	}

	// 6. 初始化默认主控本机节点 (ID=1)
	var nodeCount int
	err = DB.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&nodeCount)
	if err == nil && nodeCount == 0 {
		secretPath, _ := GetSetting("secret_path")
		now := time.Now()
		_, _ = DB.Exec(
			"INSERT INTO nodes (id, name, host, ssh_port, panel_port, secret_path, username, password_encrypted, api_token, is_master, status, os_info, latency_ms, created_at, updated_at) VALUES (1, ?, '127.0.0.1', 22, 20888, ?, 'admin', '', '', 1, 'online', ?, 0, ?, ?)",
			"🏠 本机 (Master)",
			secretPath,
			runtime.GOOS+"/"+runtime.GOARCH,
			now,
			now,
		)
		log.Println("[Init] Created default master node")
	}

	return nil
}

// GetNode 获取单个节点信息
func GetNode(id int64) (*Node, error) {
	row := DB.QueryRow(
		"SELECT id, name, host, ssh_port, panel_port, secret_path, username, password_encrypted, api_token, is_master, use_tls, cert_fingerprint, ssh_host_key, cloud_provider, cloud_instance_id, region, plan, status, os_info, latency_ms, created_at, updated_at FROM nodes WHERE id = ?",
		id,
	)
	var n Node
	var isMasterInt, useTLSInt int
	err := row.Scan(
		&n.ID, &n.Name, &n.Host, &n.SSHPort, &n.PanelPort, &n.SecretPath,
		&n.Username, &n.PasswordEncrypted, &n.APIToken, &isMasterInt,
		&useTLSInt, &n.CertFingerprint, &n.SSHHostKey,
		&n.CloudProvider, &n.CloudInstanceID, &n.Region, &n.Plan,
		&n.Status, &n.OSInfo, &n.LatencyMs, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	n.IsMaster = isMasterInt == 1
	n.UseTLS = useTLSInt == 1
	return &n, nil
}

// ListNodes 列出所有节点（主控本机始终排在首位）
func ListNodes() ([]Node, error) {
	rows, err := DB.Query(
		"SELECT id, name, host, ssh_port, panel_port, secret_path, username, password_encrypted, api_token, is_master, use_tls, cert_fingerprint, ssh_host_key, cloud_provider, cloud_instance_id, region, plan, status, os_info, latency_ms, created_at, updated_at FROM nodes ORDER BY is_master DESC, id ASC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		var isMasterInt, useTLSInt int
		if err := rows.Scan(
			&n.ID, &n.Name, &n.Host, &n.SSHPort, &n.PanelPort, &n.SecretPath,
			&n.Username, &n.PasswordEncrypted, &n.APIToken, &isMasterInt,
			&useTLSInt, &n.CertFingerprint, &n.SSHHostKey,
			&n.CloudProvider, &n.CloudInstanceID, &n.Region, &n.Plan,
			&n.Status, &n.OSInfo, &n.LatencyMs, &n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, err
		}
		n.IsMaster = isMasterInt == 1
		n.UseTLS = useTLSInt == 1
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// CreateNode 创建新节点记录
func CreateNode(n *Node) (int64, error) {
	now := time.Now()
	n.CreatedAt = now
	n.UpdatedAt = now
	isMasterInt := 0
	if n.IsMaster {
		isMasterInt = 1
	}
	useTLSInt := 0
	if n.UseTLS {
		useTLSInt = 1
	}
	res, err := DB.Exec(
		"INSERT INTO nodes (name, host, ssh_port, panel_port, secret_path, username, password_encrypted, api_token, is_master, use_tls, cert_fingerprint, ssh_host_key, cloud_provider, cloud_instance_id, region, plan, status, os_info, latency_ms, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		n.Name, n.Host, n.SSHPort, n.PanelPort, n.SecretPath, n.Username, n.PasswordEncrypted, n.APIToken, isMasterInt, useTLSInt, n.CertFingerprint, n.SSHHostKey, n.CloudProvider, n.CloudInstanceID, n.Region, n.Plan, n.Status, n.OSInfo, n.LatencyMs, n.CreatedAt, n.UpdatedAt,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	n.ID = id
	return id, nil
}

// UpdateNode 更新节点配置
func UpdateNode(n *Node) error {
	now := time.Now()
	n.UpdatedAt = now
	useTLSInt := 0
	if n.UseTLS {
		useTLSInt = 1
	}
	_, err := DB.Exec(
		"UPDATE nodes SET name = ?, host = ?, ssh_port = ?, panel_port = ?, secret_path = ?, username = ?, password_encrypted = ?, api_token = ?, use_tls = ?, cert_fingerprint = ?, ssh_host_key = ?, cloud_provider = ?, cloud_instance_id = ?, region = ?, plan = ?, status = ?, os_info = ?, latency_ms = ?, updated_at = ? WHERE id = ?",
		n.Name, n.Host, n.SSHPort, n.PanelPort, n.SecretPath, n.Username, n.PasswordEncrypted, n.APIToken, useTLSInt, n.CertFingerprint, n.SSHHostKey, n.CloudProvider, n.CloudInstanceID, n.Region, n.Plan, n.Status, n.OSInfo, n.LatencyMs, n.UpdatedAt, n.ID,
	)
	return err
}

// UpdateNodeStatus 快速更新节点状态与延迟
func UpdateNodeStatus(id int64, status string, latencyMs int64) error {
	_, err := DB.Exec("UPDATE nodes SET status = ?, latency_ms = ?, updated_at = ? WHERE id = ?", status, latencyMs, time.Now(), id)
	return err
}

// UpdateNodeToken 快速更新节点的 API Token
func UpdateNodeToken(id int64, token string) error {
	_, err := DB.Exec("UPDATE nodes SET api_token = ?, updated_at = ? WHERE id = ?", token, time.Now(), id)
	return err
}

// DeleteNode 删除节点（主控本机禁止删除）
func DeleteNode(id int64) error {
	_, err := DB.Exec("DELETE FROM nodes WHERE id = ? AND is_master = 0", id)
	return err
}

// GetVultrAPIKey 获取解密后的 Vultr API Token
func GetVultrAPIKey() (string, error) {
	enc, err := GetSetting("vultr_api_key_encrypted")
	if err != nil || enc == "" {
		return "", err
	}
	jwtSecret, _ := GetSetting("jwt_secret")
	key := crypto.DeriveKey(jwtSecret)
	return crypto.DecryptAESGCM(enc, key)
}

// SetVultrAPIKey 加密存储 Vultr API Token
func SetVultrAPIKey(apiKey string) error {
	jwtSecret, _ := GetSetting("jwt_secret")
	key := crypto.DeriveKey(jwtSecret)
	enc, err := crypto.EncryptAESGCM(apiKey, key)
	if err != nil {
		return err
	}
	return SetSetting("vultr_api_key_encrypted", enc)
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

