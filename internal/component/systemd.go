package component

import (
	"fmt"
	"os"
	"path/filepath"
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

func (s *SystemdManager) runSystemctl(timeout time.Duration, args ...string) (string, error) {
	cmdArgs := args
	if IsUserMode() {
		cmdArgs = append([]string{"--user"}, args...)
	}
	return executil.RunCommand(timeout, "systemctl", cmdArgs...)
}

// IsActive 检查服务是否活跃
func (s *SystemdManager) IsActive() bool {
	out, err := s.runSystemctl(3*time.Second, "is-active", s.UnitName)
	if err == nil && strings.TrimSpace(out) == "active" {
		return true
	}
	return false
}

// Status 获取服务当前状态
// 精确识别正在崩溃重启中的 auto-restart 异常状态
func (s *SystemdManager) Status() string {
	out, err := s.runSystemctl(3*time.Second, "show", "-p", "ActiveState,SubState", s.UnitName)
	if err == nil && out != "" {
		var activeState, subState string
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "ActiveState=") {
				activeState = strings.TrimPrefix(line, "ActiveState=")
			} else if strings.HasPrefix(line, "SubState=") {
				subState = strings.TrimPrefix(line, "SubState=")
			}
		}
		if subState == "auto-restart" {
			return "failed"
		}
		if activeState != "" {
			return activeState
		}
	}

	out, _ = s.runSystemctl(3*time.Second, "is-active", s.UnitName)
	return strings.TrimSpace(out)
}

// Start 启动服务
func (s *SystemdManager) Start() error {
	_, err := s.runSystemctl(10*time.Second, "start", s.UnitName)
	return err
}

// Stop 停止服务
func (s *SystemdManager) Stop() error {
	_, err := s.runSystemctl(10*time.Second, "stop", s.UnitName)
	return err
}

// Restart 重启服务
func (s *SystemdManager) Restart() error {
	_, err := s.runSystemctl(15*time.Second, "restart", s.UnitName)
	return err
}

// ReloadOrRestart 热重载或重启
func (s *SystemdManager) ReloadOrRestart() error {
	_, err := s.runSystemctl(15*time.Second, "reload-or-restart", s.UnitName)
	return err
}

// Enable 设为开机自启
func (s *SystemdManager) Enable() error {
	_, err := s.runSystemctl(5*time.Second, "enable", s.UnitName)
	return err
}

// Disable 取消开机自启
func (s *SystemdManager) Disable() error {
	_, err := s.runSystemctl(5*time.Second, "disable", s.UnitName)
	return err
}

// DaemonReload 重新载入 systemd 配置
func DaemonReload() error {
	args := []string{"daemon-reload"}
	if IsUserMode() {
		args = []string{"--user", "daemon-reload"}
	}
	_, err := executil.RunCommand(5*time.Second, "systemctl", args...)
	return err
}

// GetSystemdUnitPath 获取 service 文件写入路径
func GetSystemdUnitPath(unitName string) string {
	if !IsUserMode() {
		return filepath.Join("/etc/systemd/system", unitName)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join("/etc/systemd/system", unitName)
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, unitName)
}

// WriteServiceUnit 写入服务单元并重载 daemon
func WriteServiceUnit(unitName, content string) error {
	if IsUserMode() {
		// 用户模式下，WantedBy 不能为 multi-user.target，应为 default.target
		content = strings.ReplaceAll(content, "WantedBy=multi-user.target", "WantedBy=default.target")
		// 移除 root 模式特有的用户配置行与内核特权指令
		lines := strings.Split(content, "\n")
		var filtered []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "User=") ||
				strings.HasPrefix(trimmed, "CapabilityBoundingSet=") ||
				strings.HasPrefix(trimmed, "AmbientCapabilities=") {
				continue
			}
			filtered = append(filtered, line)
		}
		content = strings.Join(filtered, "\n")
	}

	path := GetSystemdUnitPath(unitName)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("write systemd unit error: %w", err)
	}
	return DaemonReload()
}

// RemoveServiceUnit 移除服务单元
func RemoveServiceUnit(unitName string) error {
	path := GetSystemdUnitPath(unitName)
	_ = os.Remove(path)
	if IsUserMode() {
		_ = os.Remove(filepath.Join("/etc/systemd/system", unitName))
	}
	return DaemonReload()
}
