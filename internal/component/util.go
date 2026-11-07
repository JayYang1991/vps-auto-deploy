package component

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"vps-auto-deploy/internal/pkg/executil"
)

// FindBinary 动态检测系统中的可执行文件路径
// 优先在系统 PATH 中查找，其次查找常见的候选路径
func FindBinary(name string, candidates ...string) string {
	if path, err := exec.LookPath(name); err == nil {
		return path
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

var versionRegex = regexp.MustCompile(`(?:version\s+|Xray\s+|v)?(\d+\.\d+(?:\.\d+)?(?:-[a-zA-Z0-9.]+)?|\d{4}\.\d+\.\d+)`)

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
