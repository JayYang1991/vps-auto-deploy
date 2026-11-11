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
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
	"vps-auto-deploy/internal/pkg/crypto"
)

// InitDB 初始化 SQLite 数据库与数据迁移
func InitDB(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	// 强化配置目录权限 (0700: 仅所有者可读写访问)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create db dir error: %w", err)
	}
	_ = os.Chmod(dir, 0700)

	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite db error: %w", err)
	}

	// 强化数据库文件权限 (0600: 仅所有者读写，严防本地低权限越权脱库)
	if fi, err := os.Stat(dbPath); err == nil && fi.Mode().Perm() != 0600 {
		_ = os.Chmod(dbPath, 0600)
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
		sub_prefix TEXT NOT NULL DEFAULT '',
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
		ssh_user TEXT NOT NULL DEFAULT 'root',
		ssh_password_encrypted TEXT NOT NULL DEFAULT '',
		ssh_key_encrypted TEXT NOT NULL DEFAULT '',
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

	CREATE TABLE IF NOT EXISTS cf_preferred_ips (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ip TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 443,
		tag TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		colo TEXT NOT NULL DEFAULT '',
		latency TEXT NOT NULL DEFAULT '',
		speed TEXT NOT NULL DEFAULT '',
		raw_link TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_cf_preferred_ips_ip ON cf_preferred_ips(ip);

	CREATE TABLE IF NOT EXISTS routes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		action TEXT NOT NULL DEFAULT 'route',
		outbound_tag TEXT NOT NULL,
		inbound_tags TEXT NOT NULL DEFAULT '',
		domains TEXT NOT NULL DEFAULT '',
		domain_suffix TEXT NOT NULL DEFAULT '',
		domain_keyword TEXT NOT NULL DEFAULT '',
		ip_cidr TEXT NOT NULL DEFAULT '',
		ip_is_private INTEGER NOT NULL DEFAULT 0,
		port TEXT NOT NULL DEFAULT '',
		port_range TEXT NOT NULL DEFAULT '',
		protocol TEXT NOT NULL DEFAULT '',
		rule_set TEXT NOT NULL DEFAULT '',
		rule_set_urls TEXT NOT NULL DEFAULT '',
		invert INTEGER NOT NULL DEFAULT 0,
		priority INTEGER NOT NULL DEFAULT 10,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_routes_priority ON routes(priority ASC);
	`
	if _, err := DB.Exec(schema); err != nil {
		return err
	}

	// 增量字段平滑迁移 (防御 MITM 指纹字段与云主机元数据、节点前缀)
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN sub_prefix TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN use_tls INTEGER NOT NULL DEFAULT 0")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN cert_fingerprint TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN ssh_host_key TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN cloud_provider TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN cloud_instance_id TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN region TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN plan TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN ssh_user TEXT NOT NULL DEFAULT 'root'")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN ssh_password_encrypted TEXT NOT NULL DEFAULT ''")
	_, _ = DB.Exec("ALTER TABLE nodes ADD COLUMN ssh_key_encrypted TEXT NOT NULL DEFAULT ''")

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
			"INSERT INTO nodes (id, name, sub_prefix, host, ssh_port, panel_port, secret_path, username, password_encrypted, api_token, is_master, status, os_info, latency_ms, created_at, updated_at) VALUES (1, ?, '🇭🇰 本地主控', '127.0.0.1', 22, 20888, ?, 'admin', '', '', 1, 'online', ?, 0, ?, ?)",
			"🏠 本机 (Master)",
			secretPath,
			runtime.GOOS+"/"+runtime.GOARCH,
			now,
			now,
		)
		log.Println("[Init] Created default master node")
	}

	// 7. 初始化默认路由分流规则 (sing-box 1.14: 私网直连与BT防版权阻断)
	var routeCount int
	err = DB.QueryRow("SELECT COUNT(*) FROM routes").Scan(&routeCount)
	if err == nil && routeCount == 0 {
		now := time.Now()
		_, _ = DB.Exec(
			`INSERT INTO routes (name, action, outbound_tag, inbound_tags, domains, domain_suffix, domain_keyword, ip_cidr, ip_is_private, port, port_range, protocol, rule_set, rule_set_urls, invert, priority, enabled, created_at, updated_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"私有网络直连", "route", "direct", "", "", "", "", "", 1, "", "", "", "", "", 0, 1, 1, now, now,
		)
		_, _ = DB.Exec(
			`INSERT INTO routes (name, action, outbound_tag, inbound_tags, domains, domain_suffix, domain_keyword, ip_cidr, ip_is_private, port, port_range, protocol, rule_set, rule_set_urls, invert, priority, enabled, created_at, updated_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"阻断 BT/P2P 下载 (防版权封机)", "reject", "block", "", "", "", "", "", 0, "", "", "bittorrent", "", "", 0, 2, 1, now, now,
		)
		log.Println("[Init] Created default sing-box route rules")
	}

	return nil
}

// GetNode 获取单个节点信息
// ParseTime 安全解析 SQLite 中存储的各类时间形态 (time.Time, string, []byte, int64)
func ParseTime(val any) time.Time {
	if val == nil {
		return time.Now()
	}
	switch v := val.(type) {
	case time.Time:
		return v
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return time.Now()
		}
		layouts := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05.999999999-07:00",
			"2006-01-02 15:04:05.999999999",
			"2006-01-02 15:04:05-07:00",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02",
			time.ANSIC,
			time.UnixDate,
			time.RubyDate,
			time.RFC822,
			time.RFC822Z,
			time.RFC850,
			time.RFC1123,
			time.RFC1123Z,
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, v); err == nil {
				return t
			}
		}
		if len(v) >= 19 {
			if t, err := time.Parse("2006-01-02 15:04:05", v[:19]); err == nil {
				return t
			}
			if t, err := time.Parse("2006-01-02T15:04:05", v[:19]); err == nil {
				return t
			}
		}
		return time.Now()
	case []byte:
		return ParseTime(string(v))
	case int64:
		return time.Unix(v, 0)
	default:
		return time.Now()
	}
}

func GetNode(id int64) (*Node, error) {
	row := DB.QueryRow(
		"SELECT id, name, sub_prefix, host, ssh_port, panel_port, secret_path, username, password_encrypted, api_token, is_master, use_tls, cert_fingerprint, ssh_host_key, ssh_user, ssh_password_encrypted, ssh_key_encrypted, cloud_provider, cloud_instance_id, region, plan, status, os_info, latency_ms, created_at, updated_at FROM nodes WHERE id = ?",
		id,
	)
	var n Node
	var isMasterInt, useTLSInt int
	var createdAtRaw, updatedAtRaw any
	err := row.Scan(
		&n.ID, &n.Name, &n.SubPrefix, &n.Host, &n.SSHPort, &n.PanelPort, &n.SecretPath,
		&n.Username, &n.PasswordEncrypted, &n.APIToken, &isMasterInt,
		&useTLSInt, &n.CertFingerprint, &n.SSHHostKey,
		&n.SSHUser, &n.SSHPasswordEncrypted, &n.SSHKeyEncrypted,
		&n.CloudProvider, &n.CloudInstanceID, &n.Region, &n.Plan,
		&n.Status, &n.OSInfo, &n.LatencyMs, &createdAtRaw, &updatedAtRaw,
	)
	if err != nil {
		return nil, err
	}
	n.IsMaster = isMasterInt == 1
	n.UseTLS = useTLSInt == 1
	n.HasSSHAuth = n.SSHPasswordEncrypted != "" || n.SSHKeyEncrypted != ""
	n.CreatedAt = ParseTime(createdAtRaw)
	n.UpdatedAt = ParseTime(updatedAtRaw)
	return &n, nil
}

// ListNodes 列出所有节点（主控本机始终排在首位）
func ListNodes() ([]Node, error) {
	rows, err := DB.Query(
		"SELECT id, name, sub_prefix, host, ssh_port, panel_port, secret_path, username, password_encrypted, api_token, is_master, use_tls, cert_fingerprint, ssh_host_key, ssh_user, ssh_password_encrypted, ssh_key_encrypted, cloud_provider, cloud_instance_id, region, plan, status, os_info, latency_ms, created_at, updated_at FROM nodes ORDER BY is_master DESC, id ASC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var n Node
		var isMasterInt, useTLSInt int
		var createdAtRaw, updatedAtRaw any
		if err := rows.Scan(
			&n.ID, &n.Name, &n.SubPrefix, &n.Host, &n.SSHPort, &n.PanelPort, &n.SecretPath,
			&n.Username, &n.PasswordEncrypted, &n.APIToken, &isMasterInt,
			&useTLSInt, &n.CertFingerprint, &n.SSHHostKey,
			&n.SSHUser, &n.SSHPasswordEncrypted, &n.SSHKeyEncrypted,
			&n.CloudProvider, &n.CloudInstanceID, &n.Region, &n.Plan,
			&n.Status, &n.OSInfo, &n.LatencyMs, &createdAtRaw, &updatedAtRaw,
		); err != nil {
			return nil, err
		}
		n.IsMaster = isMasterInt == 1
		n.UseTLS = useTLSInt == 1
		n.HasSSHAuth = n.SSHPasswordEncrypted != "" || n.SSHKeyEncrypted != ""
		n.CreatedAt = ParseTime(createdAtRaw)
		n.UpdatedAt = ParseTime(updatedAtRaw)
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
	if n.SSHUser == "" {
		n.SSHUser = "root"
	}
	res, err := DB.Exec(
		"INSERT INTO nodes (name, sub_prefix, host, ssh_port, panel_port, secret_path, username, password_encrypted, api_token, is_master, use_tls, cert_fingerprint, ssh_host_key, ssh_user, ssh_password_encrypted, ssh_key_encrypted, cloud_provider, cloud_instance_id, region, plan, status, os_info, latency_ms, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		n.Name, n.SubPrefix, n.Host, n.SSHPort, n.PanelPort, n.SecretPath, n.Username, n.PasswordEncrypted, n.APIToken, isMasterInt, useTLSInt, n.CertFingerprint, n.SSHHostKey, n.SSHUser, n.SSHPasswordEncrypted, n.SSHKeyEncrypted, n.CloudProvider, n.CloudInstanceID, n.Region, n.Plan, n.Status, n.OSInfo, n.LatencyMs, n.CreatedAt, n.UpdatedAt,
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
	if n.SSHUser == "" {
		n.SSHUser = "root"
	}
	_, err := DB.Exec(
		"UPDATE nodes SET name = ?, sub_prefix = ?, host = ?, ssh_port = ?, panel_port = ?, secret_path = ?, username = ?, password_encrypted = ?, api_token = ?, use_tls = ?, cert_fingerprint = ?, ssh_host_key = ?, ssh_user = ?, ssh_password_encrypted = ?, ssh_key_encrypted = ?, cloud_provider = ?, cloud_instance_id = ?, region = ?, plan = ?, status = ?, os_info = ?, latency_ms = ?, updated_at = ? WHERE id = ?",
		n.Name, n.SubPrefix, n.Host, n.SSHPort, n.PanelPort, n.SecretPath, n.Username, n.PasswordEncrypted, n.APIToken, useTLSInt, n.CertFingerprint, n.SSHHostKey, n.SSHUser, n.SSHPasswordEncrypted, n.SSHKeyEncrypted, n.CloudProvider, n.CloudInstanceID, n.Region, n.Plan, n.Status, n.OSInfo, n.LatencyMs, n.UpdatedAt, n.ID,
	)
	return err
}

// UpdateNodeSSHCredentials 专门更新节点的 SSH 登录凭据
func UpdateNodeSSHCredentials(id int64, sshUser, sshPassEnc, sshKeyEnc string) error {
	if sshUser == "" {
		sshUser = "root"
	}
	_, err := DB.Exec(
		"UPDATE nodes SET ssh_user = ?, ssh_password_encrypted = ?, ssh_key_encrypted = ?, updated_at = ? WHERE id = ?",
		sshUser, sshPassEnc, sshKeyEnc, time.Now(), id,
	)
	return err
}

// UpdateNodePrefix 专门更新节点名称与订阅前缀
func UpdateNodePrefix(id int64, name, subPrefix string) error {
	_, err := DB.Exec("UPDATE nodes SET name = ?, sub_prefix = ?, updated_at = ? WHERE id = ?", name, subPrefix, time.Now(), id)
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

// ListMembers 列出数据库中所有成员
func ListMembers() ([]Member, error) {
	rows, err := DB.Query("SELECT id, name, uuid, sub_token, enabled, created_at, updated_at FROM members ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Member
	for rows.Next() {
		var m Member
		var enabledInt int
		var createdAtRaw, updatedAtRaw any
		if err := rows.Scan(&m.ID, &m.Name, &m.UUID, &m.SubToken, &enabledInt, &createdAtRaw, &updatedAtRaw); err != nil {
			return nil, err
		}
		m.Enabled = enabledInt == 1
		m.CreatedAt = ParseTime(createdAtRaw)
		m.UpdatedAt = ParseTime(updatedAtRaw)
		list = append(list, m)
	}
	return list, nil
}

// SyncMembers 将主控端成员无损合并至本地（保留受管节点本地原有的用户，增量同步与更新主控成员）
func SyncMembers(members []Member) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO members (name, uuid, sub_token, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(uuid) DO UPDATE SET
			name = excluded.name,
			sub_token = excluded.sub_token,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range members {
		enabledInt := 0
		if m.Enabled {
			enabledInt = 1
		}
		if _, err := stmt.Exec(m.Name, m.UUID, m.SubToken, enabledInt, m.CreatedAt, m.UpdatedAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ReplaceMembers 全量原子替换本地成员数据
func ReplaceMembers(members []Member) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM members"); err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT INTO members (id, name, uuid, sub_token, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, m := range members {
		enabledInt := 0
		if m.Enabled {
			enabledInt = 1
		}
		if _, err := stmt.Exec(m.ID, m.Name, m.UUID, m.SubToken, enabledInt, m.CreatedAt, m.UpdatedAt); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ListRouteRules 查询全部路由分流规则 (按 priority ASC, id ASC)
func ListRouteRules() ([]RouteRule, error) {
	rows, err := DB.Query(`
		SELECT id, name, action, outbound_tag, inbound_tags, domains, domain_suffix, domain_keyword, 
		       ip_cidr, ip_is_private, port, port_range, protocol, rule_set, rule_set_urls, 
		       invert, priority, enabled, created_at, updated_at 
		FROM routes 
		ORDER BY priority ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []RouteRule
	for rows.Next() {
		var r RouteRule
		var ipPrivateInt, invertInt, enabledInt int
		var createdAtRaw, updatedAtRaw any
		err := rows.Scan(
			&r.ID, &r.Name, &r.Action, &r.OutboundTag, &r.InboundTags,
			&r.Domains, &r.DomainSuffix, &r.DomainKeyword, &r.IPCIDR,
			&ipPrivateInt, &r.Port, &r.PortRange, &r.Protocol, &r.RuleSet,
			&r.RuleSetURLs, &invertInt, &r.Priority, &enabledInt,
			&createdAtRaw, &updatedAtRaw,
		)
		if err != nil {
			return nil, err
		}
		r.IPIsPrivate = ipPrivateInt == 1
		r.Invert = invertInt == 1
		r.Enabled = enabledInt == 1
		r.CreatedAt = ParseTime(createdAtRaw)
		r.UpdatedAt = ParseTime(updatedAtRaw)
		list = append(list, r)
	}
	return list, nil
}

// GetRouteRule 获取单条路由分流规则
func GetRouteRule(id int64) (*RouteRule, error) {
	row := DB.QueryRow(`
		SELECT id, name, action, outbound_tag, inbound_tags, domains, domain_suffix, domain_keyword, 
		       ip_cidr, ip_is_private, port, port_range, protocol, rule_set, rule_set_urls, 
		       invert, priority, enabled, created_at, updated_at 
		FROM routes WHERE id = ?
	`, id)

	var r RouteRule
	var ipPrivateInt, invertInt, enabledInt int
	var createdAtRaw, updatedAtRaw any
	err := row.Scan(
		&r.ID, &r.Name, &r.Action, &r.OutboundTag, &r.InboundTags,
		&r.Domains, &r.DomainSuffix, &r.DomainKeyword, &r.IPCIDR,
		&ipPrivateInt, &r.Port, &r.PortRange, &r.Protocol, &r.RuleSet,
		&r.RuleSetURLs, &invertInt, &r.Priority, &enabledInt,
		&createdAtRaw, &updatedAtRaw,
	)
	if err != nil {
		return nil, err
	}
	r.IPIsPrivate = ipPrivateInt == 1
	r.Invert = invertInt == 1
	r.Enabled = enabledInt == 1
	r.CreatedAt = ParseTime(createdAtRaw)
	r.UpdatedAt = ParseTime(updatedAtRaw)
	return &r, nil
}

// CreateRouteRule 创建路由分流规则
func CreateRouteRule(r *RouteRule) (int64, error) {
	now := time.Now()
	ipPrivateInt := 0
	if r.IPIsPrivate {
		ipPrivateInt = 1
	}
	invertInt := 0
	if r.Invert {
		invertInt = 1
	}
	enabledInt := 1
	if !r.Enabled {
		enabledInt = 0
	}
	if r.Action == "" {
		r.Action = "route"
	}
	if r.OutboundTag == "" {
		r.OutboundTag = "direct"
	}

	res, err := DB.Exec(`
		INSERT INTO routes (
			name, action, outbound_tag, inbound_tags, domains, domain_suffix, domain_keyword,
			ip_cidr, ip_is_private, port, port_range, protocol, rule_set, rule_set_urls,
			invert, priority, enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.Name, r.Action, r.OutboundTag, r.InboundTags, r.Domains, r.DomainSuffix, r.DomainKeyword,
		r.IPCIDR, ipPrivateInt, r.Port, r.PortRange, r.Protocol, r.RuleSet, r.RuleSetURLs,
		invertInt, r.Priority, enabledInt, now, now,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateRouteRule 更新路由分流规则
func UpdateRouteRule(r *RouteRule) error {
	now := time.Now()
	ipPrivateInt := 0
	if r.IPIsPrivate {
		ipPrivateInt = 1
	}
	invertInt := 0
	if r.Invert {
		invertInt = 1
	}
	enabledInt := 1
	if !r.Enabled {
		enabledInt = 0
	}
	if r.Action == "" {
		r.Action = "route"
	}
	if r.OutboundTag == "" {
		r.OutboundTag = "direct"
	}

	_, err := DB.Exec(`
		UPDATE routes SET
			name = ?, action = ?, outbound_tag = ?, inbound_tags = ?, domains = ?, domain_suffix = ?, domain_keyword = ?,
			ip_cidr = ?, ip_is_private = ?, port = ?, port_range = ?, protocol = ?, rule_set = ?, rule_set_urls = ?,
			invert = ?, priority = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, r.Name, r.Action, r.OutboundTag, r.InboundTags, r.Domains, r.DomainSuffix, r.DomainKeyword,
		r.IPCIDR, ipPrivateInt, r.Port, r.PortRange, r.Protocol, r.RuleSet, r.RuleSetURLs,
		invertInt, r.Priority, enabledInt, now, r.ID,
	)
	return err
}

// DeleteRouteRule 删除路由分流规则
func DeleteRouteRule(id int64) error {
	_, err := DB.Exec("DELETE FROM routes WHERE id = ?", id)
	return err
}

// ToggleRouteRule 切换路由规则启用/停用状态
func ToggleRouteRule(id int64) (bool, error) {
	row := DB.QueryRow("SELECT enabled FROM routes WHERE id = ?", id)
	var enabledInt int
	if err := row.Scan(&enabledInt); err != nil {
		return false, err
	}
	newStatus := 1
	if enabledInt == 1 {
		newStatus = 0
	}
	_, err := DB.Exec("UPDATE routes SET enabled = ?, updated_at = ? WHERE id = ?", newStatus, time.Now(), id)
	return newStatus == 1, err
}

// ReorderRouteRules 按给定 ID 列表顺序重置各规则的 priority
func ReorderRouteRules(ids []int64) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE routes SET priority = ?, updated_at = ? WHERE id = ?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()
	for idx, id := range ids {
		priority := idx + 1
		if _, err := stmt.Exec(priority, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}



