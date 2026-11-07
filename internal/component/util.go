package component

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"vps-auto-deploy/internal/pkg/executil"
)

// IsUserMode 判断当前运行环境是否为普通用户空间 (非 Root)
func IsUserMode() bool {
	return os.Geteuid() != 0
}

// GetDefaultInstallDir 获取默认二进制安装路径
func GetDefaultInstallDir() string {
	if !IsUserMode() {
		return "/usr/local/bin"
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/usr/local/bin"
	}
	dir := filepath.Join(home, ".local", "bin")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// GetDefaultBinaryPath 获取组件可执行文件安装目标全路径
func GetDefaultBinaryPath(name string) string {
	return filepath.Join(GetDefaultInstallDir(), name)
}

// GetDefaultConfigDir 获取组件配置根目录
func GetDefaultConfigDir(name string) string {
	if !IsUserMode() {
		return filepath.Join("/etc", name)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join("/etc", name)
	}
	dir := filepath.Join(home, ".config", name)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// GetDefaultComponentDir 获取目录资产型组件的独立运行时工作目录
// Root 模式: /etc/vps-auto-deploy/components/<name>
// 非 Root 模式: ~/.config/vps-auto-deploy/components/<name>
func GetDefaultComponentDir(name string) string {
	if !IsUserMode() {
		dir := filepath.Join("/etc/vps-auto-deploy/components", name)
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		dir := filepath.Join("/etc/vps-auto-deploy/components", name)
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	dir := filepath.Join(home, ".config", "vps-auto-deploy", "components", name)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// GetDefaultConfigPath 获取组件默认配置文件路径
func GetDefaultConfigPath(name, fileName string) string {
	return filepath.Join(GetDefaultConfigDir(name), fileName)
}

// FindBinary 动态检测系统中的可执行文件路径
// 优先查找系统 PATH，其次用户空间 ~/.local/bin 与常见候选路径
func FindBinary(name string, candidates ...string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		userBin := filepath.Join(home, ".local", "bin", name)
		if fi, err := os.Stat(userBin); err == nil && !fi.IsDir() {
			return userBin
		}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// IsProcessRunning 检查是否有同名进程正在运行
func IsProcessRunning(name string) bool {
	out, err := executil.RunCommand(2*time.Second, "pgrep", "-x", name)
	return err == nil && strings.TrimSpace(out) != ""
}

var versionRegex = regexp.MustCompile(`(?:version\s+|v)?(\d+\.\d+(?:\.\d+)?(?:-[a-zA-Z0-9.]+)?|\d{4}\.\d+\.\d+)`)

// ExtractVersion 从命令输出中提取版本号
func ExtractVersion(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matches := versionRegex.FindStringSubmatch(line)
		if len(matches) > 1 && matches[1] != "" {
			return matches[1]
		}
	}
	return strings.TrimSpace(output)
}
