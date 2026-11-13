package notifier

import (
	"time"
)

// 事件类型常量枚举
const (
	EventLoginSuccess    = "LOGIN_SUCCESS"     // 登录成功
	EventLoginFailed     = "LOGIN_FAILED"      // 登录失败
	EventIPLocked        = "IP_LOCKED"         // IP 防爆破触发封禁锁定
	EventCredsRevealed   = "CREDS_REVEALED"    // 查看节点机密凭据
	EventPasswordChanged = "PASSWORD_CHANGED"  // 管理员修改密码
	EventSessionRevoked  = "SESSION_REVOKED"   // 会话强制全量注销
	EventTerminalSession = "TERMINAL_SESSION"  // SSH / PTY 终端会话连接或断开
	EventNodeDeploy      = "NODE_DEPLOY"       // 节点部署 / 纳管
	EventNodeDeleted     = "NODE_DELETED"      // 节点移出集群
	EventCloudDestroyed  = "CLOUD_DESTROYED"   // 云主机实例物理销毁
	EventConfigReload    = "CONFIG_RELOAD"     // 核心网络配置重载
)

// 告警严重级别
const (
	LevelInfo     = "INFO"
	LevelWarn     = "WARN"
	LevelCritical = "CRITICAL"
)

// Event 描述一次轻量审计安全事件
type Event struct {
	Type      string            `json:"type"`
	Level     string            `json:"level"`
	Title     string            `json:"title"`
	Message   string            `json:"message"`
	ClientIP  string            `json:"client_ip"`
	Username  string            `json:"username,omitempty"`
	NodeName  string            `json:"node_name,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// AllEventCategories 返回所有受支持的告警分类
func AllEventCategories() []string {
	return []string{
		EventLoginSuccess,
		EventLoginFailed,
		EventIPLocked,
		EventCredsRevealed,
		EventPasswordChanged,
		EventSessionRevoked,
		EventTerminalSession,
		EventNodeDeploy,
		EventNodeDeleted,
		EventCloudDestroyed,
		EventConfigReload,
	}
}

