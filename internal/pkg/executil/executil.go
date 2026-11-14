package executil

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"
)

var validServiceName = regexp.MustCompile(`^[a-zA-Z0-9_-]+(\.service)?$`)

// ValidateServiceName 校验 systemd 服务名合法性
func ValidateServiceName(name string) error {
	if !validServiceName.MatchString(name) {
		return fmt.Errorf("invalid service name: %q", name)
	}
	return nil
}

// ValidatePort 严格校验端口合法性，防止注入
func ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port number: %d", port)
	}
	return nil
}

// SafeRun 强制带超时且禁止直接调用 shell 解释器执行 (防范命令注入)
func SafeRun(ctx context.Context, timeout time.Duration, bin string, args ...string) ([]byte, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 安全检查：禁止直接调用 shell 解释器执行拼接命令
	if bin == "sh" || bin == "bash" || bin == "/bin/sh" || bin == "/bin/bash" {
		return nil, errors.New("security violation: shell interpreter execution is forbidden")
	}

	// #nosec G204 - bin and args are parameterized without shell expansion
	cmd := exec.CommandContext(ctx, bin, args...)
	return cmd.CombinedOutput()
}

// RunCommand 执行命令并捕获输出 (禁止通过 shell 解释器执行)
func RunCommand(timeout time.Duration, name string, args ...string) (string, error) {
	if name == "sh" || name == "bash" || name == "/bin/sh" || name == "/bin/bash" {
		return "", errors.New("security violation: shell interpreter execution is forbidden")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// #nosec G204 - name and args are explicitly passed as argument slice, no shell expansion
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("cmd failed: %s %v: %w (output: %s)", name, args, err, string(out))
	}
	return string(out), nil
}

// StreamJournalctlLogs 启动一个命令并返回其 stdout 扫描行 channel
func StreamJournalctlLogs(ctx context.Context, unit string, lines int) (<-chan string, error) {
	if err := ValidateServiceName(unit); err != nil {
		return nil, err
	}

	outChan := make(chan string, 100)
	args := []string{"-u", unit, "-f", "-n", fmt.Sprintf("%d", lines), "--no-pager", "-o", "cat"}
	if os.Geteuid() != 0 {
		args = append([]string{"--user"}, args...)
	}
	// #nosec G204 - journalctl binary and args are strictly parameterized
	cmd := exec.CommandContext(ctx, "journalctl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	go func() {
		defer close(outChan)
		defer cmd.Wait()

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case outChan <- scanner.Text():
			}
		}
	}()

	return outChan, nil
}


