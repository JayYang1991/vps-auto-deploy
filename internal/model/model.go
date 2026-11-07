package model

import (
	"database/sql"
	"time"
)

// Setting 键值对系统配置
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Member 小团队成员/订阅主体
type Member struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	UUID      string    `json:"uuid"`
	SubToken  string    `json:"sub_token"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Inbound sing-box 入站配置
type Inbound struct {
	ID        int64     `json:"id"`
	Tag       string    `json:"tag"`
	Protocol  string    `json:"protocol"` // "vless-reality" | "vless-ws"
	Listen    string    `json:"listen"`   // "::" 或 "127.0.0.1"
	Port      int       `json:"port"`     // 端口
	SpecJSON  string    `json:"spec_json"` // JSON 字符串
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Outbound sing-box 出站配置
type Outbound struct {
	ID         int64     `json:"id"`
	Tag        string    `json:"tag"`
	Type       string    `json:"type"`       // "direct" | "block" | "socks" | "http" | "shadowsocks" | "vless" | "custom"
	Server     string    `json:"server"`     // 上游目标服务器/IP
	ServerPort int       `json:"server_port"`
	SpecJSON   string    `json:"spec_json"`  // 详细参数 JSON
	Priority   int       `json:"priority"`   // 排序权重 (越小越优先)
	Enabled    bool      `json:"enabled"`
	IsSystem   bool      `json:"is_system"`  // 系统预置出站 (如 direct, block 禁止删除)
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// VLESSRealitySpec 是 VLESS-Reality 协议特定的参数
type VLESSRealitySpec struct {
	Flow            string `json:"flow"`             // 默认 xtls-rprx-vision
	ServerName      string `json:"server_name"`      // SNI (默认 dl.google.com)
	HandshakeServer string `json:"handshake_server"` // 目标服务器 (默认 dl.google.com)
	HandshakePort   int    `json:"handshake_port"`   // 目标端口 (默认 443)
	PrivateKey      string `json:"private_key"`      // Reality 私钥 (服务端)
	PublicKey       string `json:"public_key"`       // Reality 公钥 (客户端)
	ShortID         string `json:"short_id"`         // 8位十六进制
	Fingerprint     string `json:"fingerprint"`      // 默认 chrome
}

// VLESSWSSpec 是 VLESS-WS 协议参数 (搭配 Cloudflared)
type VLESSWSSpec struct {
	Path        string `json:"path"`         // ws path (如 /ws)
	Host        string `json:"host"`         // ws host header
	GroupDomain string `json:"group_domain"` // 分组指定域名 (SNI / Host)
	TunnelID    int64  `json:"tunnel_id"`    // 关联的 Cloudflared 隧道 ID
}

// Tunnel Cloudflared 隧道
type Tunnel struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	Mode           string         `json:"mode"`              // "quick" 或 "token"
	Token          string         `json:"token"`             // Cloudflare Zero Trust Token
	Domain         string         `json:"domain"`            // quick 模式下抓取的 trycloudflare.com 域名或自定义域名
	TargetPort     int            `json:"target_port"`       // 本地代理服务入站端口 (如 10086)
	EdgeIPVersion  string         `json:"edge_ip_version"`   // "auto" | "4" | "6"
	Protocol       string         `json:"protocol"`          // "auto" | "http2" | "quic"
	Status         string         `json:"status"`            // "stopped" | "running" | "error"
	Enabled        bool           `json:"enabled"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Node 纳管服务器节点
type Node struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	Host              string    `json:"host"`
	SSHPort           int       `json:"ssh_port"`
	PanelPort         int       `json:"panel_port"`
	SecretPath        string    `json:"secret_path"`
	Username          string    `json:"username"`
	PasswordEncrypted string    `json:"-"`
	APIToken          string    `json:"-"`
	IsMaster          bool      `json:"is_master"`
	UseTLS            bool      `json:"use_tls"`          // 是否启用 HTTPS 加密通信
	CertFingerprint   string    `json:"cert_fingerprint"` // 目标节点 TLS 证书 SHA-256 锁定指纹 (防 MITM)
	SSHHostKey        string    `json:"ssh_host_key"`     // 目标机器 SSH 主机指纹 (TOFU 防冒充)
	CloudProvider     string    `json:"cloud_provider"`   // "" (自建/手动) | "vultr"
	CloudInstanceID   string    `json:"cloud_instance_id"`// 云厂商实例唯一 ID
	Region            string    `json:"region"`           // 数据中心区域代码 (如 nrt, sgp)
	Plan              string    `json:"plan"`             // 实例规格代码 (如 vc2-1c-1gb)
	Status            string    `json:"status"`           // "online" | "offline" | "deploying" | "error"
	OSInfo            string    `json:"os_info"`
	LatencyMs         int64     `json:"latency_ms"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// DB 全局数据库对象
var DB *sql.DB

