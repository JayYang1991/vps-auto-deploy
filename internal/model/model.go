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
	Protocol  string    `json:"protocol"` // "vless-reality" | "vless-ws" | "hysteria2" | "trojan" | "shadowsocks" | "vmess" | "tuic" | "socks" | "mixed"
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
	Type       string    `json:"type"`       // "direct" | "block" | "socks" | "http" | "shadowsocks" | "vless" | "trojan" | "hysteria2" | "wireguard" | "custom"
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

// SocksSpec 是 SOCKS 入站协议参数
type SocksSpec struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// MixedSpec 是 Mixed (HTTP & SOCKS5) 入站协议参数
type MixedSpec struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// ShadowsocksInboundSpec 是 Shadowsocks 入站参数
type ShadowsocksInboundSpec struct {
	Method   string `json:"method"`
	Password string `json:"password"`
}

// TrojanInboundSpec 是 Trojan 入站参数
type TrojanInboundSpec struct {
	Password        string `json:"password,omitempty"`
	ServerName      string `json:"server_name,omitempty"`
	CertificatePath string `json:"certificate_path,omitempty"`
	KeyPath         string `json:"key_path,omitempty"`
}

// Hysteria2InboundSpec 是 Hysteria 2 入站参数
type Hysteria2InboundSpec struct {
	Password              string `json:"password,omitempty"`
	ServerName            string `json:"server_name,omitempty"`
	CertificatePath       string `json:"certificate_path,omitempty"`
	KeyPath               string `json:"key_path,omitempty"`
	UpMbps                int    `json:"up_mbps,omitempty"`
	DownMbps              int    `json:"down_mbps,omitempty"`
	ObfsPassword          string `json:"obfs_password,omitempty"`
	IgnoreClientBandwidth bool   `json:"ignore_client_bandwidth,omitempty"`
}

// VMessInboundSpec 是 VMess 入站参数
type VMessInboundSpec struct {
	Network     string `json:"network"` // "tcp" | "ws"
	Path        string `json:"path,omitempty"`
	GroupDomain string `json:"group_domain,omitempty"`
}

// TUICInboundSpec 是 TUIC v5 入站参数
type TUICInboundSpec struct {
	Password          string `json:"password,omitempty"`
	CongestionControl string `json:"congestion_control,omitempty"` // "bbr" | "cubic"
	ServerName        string `json:"server_name,omitempty"`
	CertificatePath   string `json:"certificate_path,omitempty"`
	KeyPath           string `json:"key_path,omitempty"`
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
	SubPrefix         string    `json:"sub_prefix"`       // 订阅节点名称前缀 (如 "🇭🇰 香港主控", "[JP-01]")
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

// RouteRule sing-box 1.14 路由分流规则
type RouteRule struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`           // 规则名称/描述
	Action        string    `json:"action"`         // "route" | "reject" | "direct" (sing-box 1.14 规范)
	OutboundTag   string    `json:"outbound_tag"`   // 目标出站 tag ("direct", "block", 或自定义 outbound tag)
	InboundTags   string    `json:"inbound_tags"`   // 限制匹配的入站 tag (逗号分隔，空代表匹配全部)
	Domains       string    `json:"domains"`        // 完整域名匹配 (逗号或换行分隔)
	DomainSuffix  string    `json:"domain_suffix"`  // 域名后缀匹配 (逗号或换行分隔，如 ".google.com, .openai.com")
	DomainKeyword string    `json:"domain_keyword"` // 域名关键词匹配 (逗号或换行分隔，如 "google, netflix")
	IPCIDR        string    `json:"ip_cidr"`        // IP CIDR 列表 (逗号或换行分隔，如 "192.168.1.0/24")
	IPIsPrivate   bool      `json:"ip_is_private"`  // 是否匹配局域网私有 IP (sing-box 1.14 原生支持替代 geoip:private)
	Port          string    `json:"port"`           // 目标端口 (逗号分隔，如 "80,443")
	PortRange     string    `json:"port_range"`     // 端口范围 (如 "8000:9000")
	Protocol      string    `json:"protocol"`       // 嗅探协议 (逗号分隔，如 "bittorrent")
	RuleSet       string    `json:"rule_set"`       // sing-box 1.14 标准规则集 (逗号分隔，如 "geosite-category-ads-all,geosite-cn,geoip-cn")
	RuleSetURLs   string    `json:"rule_set_urls"`  // 自定义规则集 URL 映射 (每行一个格式: tag=url)
	Invert        bool      `json:"invert"`         // 是否反向匹配
	Priority      int       `json:"priority"`       // 优先级 (数值越小越靠前优先匹配)
	Enabled       bool      `json:"enabled"`        // 是否启用
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// DB 全局数据库对象
var DB *sql.DB

