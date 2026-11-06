package component

import (
	"fmt"
	"os"
	"strings"
	"time"

	"vps-auto-deploy/internal/pkg/executil"
)

// SystemdManager 管理 systemd 服务单元
type SystemdManager struct {
	UnitName string
}

func NewSystemdManager(unitName string) *SystemdManager {
	return &SystemdManager{UnitName: unitName}
}

// IsActive 检查服务是否活跃
func (s *SystemdManager) IsActive() bool {
	out, err := executil.RunCommand(3*time.Second, "systemctl", "is-active", s.UnitName)
	if err == nil && strings.TrimSpace(out) == "active" {
		return true
	}
	return false
}

// Status 获取服务当前状态
func (s *SystemdManager) Status() string {
	out, _ := executil.RunCommand(3*time.Second, "systemctl", "is-active", s.UnitName)
	return strings.TrimSpace(out)
}

// Start 启动服务
func (s *SystemdManager) Start() error {
	_, err := executil.RunCommand(10*time.Second, "systemctl", "start", s.UnitName)
	return err
}

// Stop 停止服务
func (s *SystemdManager) Stop() error {
	_, err := executil.RunCommand(10*time.Second, "systemctl", "stop", s.UnitName)
	return err
}

// Restart 重启服务
func (s *SystemdManager) Restart() error {
	_, err := executil.RunCommand(15*time.Second, "systemctl", "restart", s.UnitName)
	return err
}

// ReloadOrRestart 热重载或重启
func (s *SystemdManager) ReloadOrRestart() error {
	_, err := executil.RunCommand(15*time.Second, "systemctl", "reload-or-restart", s.UnitName)
	return err
}

// Enable 设为开机自启
func (s *SystemdManager) Enable() error {
	_, err := executil.RunCommand(5*time.Second, "systemctl", "enable", s.UnitName)
	return err
}

// Disable 取消开机自启
func (s *SystemdManager) Disable() error {
	_, err := executil.RunCommand(5*time.Second, "systemctl", "disable", s.UnitName)
	return err
}

// DaemonReload 重新载入 systemd 配置
func DaemonReload() error {
	_, err := executil.RunCommand(5*time.Second, "systemctl", "daemon-reload")
	return err
}

// WriteServiceUnit 写入 /etc/systemd/system/<unit>.service
func WriteServiceUnit(unitName, content string) error {
	path := fmt.Sprintf("/etc/systemd/system/%s", unitName)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write systemd unit error: %w", err)
	}
	return DaemonReload()
}

// RemoveServiceUnit 移除 /etc/systemd/system/<unit>.service
func RemoveServiceUnit(unitName string) error {
	path := fmt.Sprintf("/etc/systemd/system/%s", unitName)
	_ = os.Remove(path)
	return DaemonReload()
}

