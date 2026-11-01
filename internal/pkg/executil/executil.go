package executil

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// RunCommand 执行 shell 命令并捕获输出
func RunCommand(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("cmd failed: %s %v: %w (output: %s)", name, args, err, string(out))
	}
	return string(out), nil
}

// StreamJournalctlLogs 启动一个命令并返回其 stdout 扫描行 channel
func StreamJournalctlLogs(ctx context.Context, unit string, lines int) (<-chan string, error) {
	outChan := make(chan string, 100)
	args := []string{"-u", unit, "-f", "-n", fmt.Sprintf("%d", lines), "--no-pager", "-o", "cat"}
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

