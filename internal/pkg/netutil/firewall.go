package netutil

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"vps-auto-deploy/internal/pkg/executil"
)

// InboundPortInfo 描述入站端口信息
type InboundPortInfo struct {
	Port     int
	Protocol string
	Listen   string
}

// IsLoopbackAddress 判断监听地址是否仅限本地回环（无需暴露公网防火墙）
func IsLoopbackAddress(addr string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(addr))
	return trimmed == "127.0.0.1" || trimmed == "localhost" || trimmed == "::1"
}

// AllowInboundPort 针对入站监听地址及端口，若非本地回环则放行防火墙
func AllowInboundPort(port int, protocol string, listen string) error {
	if IsLoopbackAddress(listen) {
		log.Printf("[firewall] 监听地址为本地回环 (%s:%d)，跳过公网防火墙放行", listen, port)
		return nil
	}
	return AllowPort(port, protocol)
}

// AllowPort 探测系统启用的防火墙体系（UFW、Firewalld、iptables、ip6tables）并幂等放行端口
func AllowPort(port int, protocol string) error {
	if port <= 0 || port > 65535 {
		return fmt.Errorf("invalid port number: %d", port)
	}

	if runtime.GOOS != "linux" {
		return nil
	}

	// 非 root 用户无权操作底层防火墙规则，安全跳过
	if os.Geteuid() != 0 {
		log.Printf("[firewall] 当前非 root 运行 (euid=%d)，跳过防火墙端口放行 (%d)", os.Geteuid(), port)
		return nil
	}

	var actionsDone int
	var errs []string

	// 1. 探测并处理 UFW
	if ufwPath, err := exec.LookPath("ufw"); err == nil && ufwPath != "" {
		if out, err := executil.RunCommand(3*time.Second, "ufw", "status"); err == nil {
			if strings.Contains(strings.ToLower(out), "status: active") {
				// 放行 TCP 和 UDP
				if _, err := executil.RunCommand(5*time.Second, "ufw", "allow", fmt.Sprintf("%d/tcp", port)); err != nil {
					errs = append(errs, fmt.Sprintf("ufw tcp: %v", err))
				} else {
					actionsDone++
				}
				if _, err := executil.RunCommand(5*time.Second, "ufw", "allow", fmt.Sprintf("%d/udp", port)); err != nil {
					errs = append(errs, fmt.Sprintf("ufw udp: %v", err))
				} else {
					actionsDone++
				}
				log.Printf("[firewall] UFW 已放行端口 %d (tcp/udp)", port)
			}
		}
	}

	// 2. 探测并处理 Firewalld
	if fwCmdPath, err := exec.LookPath("firewall-cmd"); err == nil && fwCmdPath != "" {
		if out, err := executil.RunCommand(3*time.Second, "firewall-cmd", "--state"); err == nil {
			if strings.Contains(strings.ToLower(out), "running") {
				_, _ = executil.RunCommand(5*time.Second, "firewall-cmd", fmt.Sprintf("--add-port=%d/tcp", port), "--permanent")
				_, _ = executil.RunCommand(5*time.Second, "firewall-cmd", fmt.Sprintf("--add-port=%d/udp", port), "--permanent")
				if _, err := executil.RunCommand(5*time.Second, "firewall-cmd", "--reload"); err != nil {
					errs = append(errs, fmt.Sprintf("firewall-cmd reload: %v", err))
				} else {
					actionsDone++
					log.Printf("[firewall] Firewalld 已放行端口 %d (tcp/udp)", port)
				}
			}
		}
	}

	// 3. 探测并处理 iptables (IPv4)
	if iptablesPath, err := exec.LookPath("iptables"); err == nil && iptablesPath != "" {
		// 检查 TCP 规则是否已存在
		if _, err := executil.RunCommand(3*time.Second, "iptables", "-C", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT"); err != nil {
			if _, err := executil.RunCommand(3*time.Second, "iptables", "-I", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT"); err != nil {
				errs = append(errs, fmt.Sprintf("iptables tcp: %v", err))
			} else {
				actionsDone++
			}
		}
		// 检查 UDP 规则是否已存在
		if _, err := executil.RunCommand(3*time.Second, "iptables", "-C", "INPUT", "-p", "udp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT"); err != nil {
			if _, err := executil.RunCommand(3*time.Second, "iptables", "-I", "INPUT", "-p", "udp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT"); err != nil {
				errs = append(errs, fmt.Sprintf("iptables udp: %v", err))
			} else {
				actionsDone++
			}
		}
	}

	// 4. 探测并处理 ip6tables (IPv6)
	if ip6tablesPath, err := exec.LookPath("ip6tables"); err == nil && ip6tablesPath != "" {
		if _, err := executil.RunCommand(3*time.Second, "ip6tables", "-C", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT"); err != nil {
			if _, err := executil.RunCommand(3*time.Second, "ip6tables", "-I", "INPUT", "-p", "tcp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT"); err != nil {
				// 忽略 IPv6 不支持时的错误
			} else {
				actionsDone++
			}
		}
		if _, err := executil.RunCommand(3*time.Second, "ip6tables", "-C", "INPUT", "-p", "udp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT"); err != nil {
			if _, err := executil.RunCommand(3*time.Second, "ip6tables", "-I", "INPUT", "-p", "udp", "--dport", fmt.Sprintf("%d", port), "-j", "ACCEPT"); err != nil {
				// 忽略 IPv6 不支持时的错误
			} else {
				actionsDone++
			}
		}
	}

	if len(errs) > 0 {
		log.Printf("[firewall] 放行端口 %d 时存在警告: %s", port, strings.Join(errs, "; "))
	}

	return nil
}

// EnsurePortsFirewall 批量幂等放行入站端口
func EnsurePortsFirewall(items []InboundPortInfo) {
	for _, item := range items {
		if !IsLoopbackAddress(item.Listen) && item.Port > 0 {
			_ = AllowPort(item.Port, item.Protocol)
		}
	}
}

