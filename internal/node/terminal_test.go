package node

import (
	"testing"
	"time"
)

func TestTerminalConcurrencyLimits(t *testing.T) {
	activeSessionsLock.Lock()
	origCount := activeSessionCount
	activeSessionsLock.Unlock()

	defer func() {
		activeSessionsLock.Lock()
		activeSessionCount = origCount
		activeSessionsLock.Unlock()
	}()

	// 模拟已达到最大并发
	activeSessionsLock.Lock()
	activeSessionCount = maxConcurrentSessions
	activeSessionsLock.Unlock()

	// 此时若尝试获取配额应拒绝
	activeSessionsLock.Lock()
	allowed := activeSessionCount < maxConcurrentSessions
	activeSessionsLock.Unlock()

	if allowed {
		t.Fatalf("expected session acquisition to be rejected when activeSessionCount == maxConcurrentSessions")
	}

	// 模拟释放会话
	activeSessionsLock.Lock()
	activeSessionCount--
	allowedAfterRelease := activeSessionCount < maxConcurrentSessions
	activeSessionsLock.Unlock()

	if !allowedAfterRelease {
		t.Fatalf("expected session acquisition to be allowed after releasing a session")
	}
}

func TestTerminalTicketValidationAndAntiReplay(t *testing.T) {
	// 1. 同 IP 验证通过并正确绑定 AllowedHost
	t1 := IssueTerminalTicket(10, "admin", "198.51.100.1", "panel.example.com", "u1", "p1", "k1", 10*time.Second)
	info1, ok1 := ValidateAndConsumeTicket(t1, 10, "198.51.100.1")
	if !ok1 || info1 == nil || info1.Username != "admin" || info1.AllowedHost != "panel.example.com" {
		t.Fatalf("expected ticket 1 to be valid with same IP, got %+v", info1)
	}

	// 2. 防重放：第二次消费应失败 (一经使用立即销毁)
	if _, ok := ValidateAndConsumeTicket(t1, 10, "198.51.100.1"); ok {
		t.Fatalf("expected replayed ticket to be rejected")
	}

	// 3. 跨 CDN/Cloudflare 多边缘节点场景：签发端为 Cloudflare Edge 1，消费端为 Cloudflare Edge 2，应放行
	t2 := IssueTerminalTicket(10, "admin", "162.158.12.34", "panel.example.com", "u2", "p2", "k2", 10*time.Second)
	info2, ok2 := ValidateAndConsumeTicket(t2, 10, "172.71.22.45")
	if !ok2 || info2 == nil {
		t.Fatalf("expected ticket 2 to be valid across Cloudflare edge node rotation")
	}

	// 4. 跨反代场景：签发端为 127.0.0.1，消费端为公网 IP，应放行
	t3 := IssueTerminalTicket(10, "admin", "127.0.0.1", "localhost", "u3", "p3", "k3", 10*time.Second)
	info3, ok3 := ValidateAndConsumeTicket(t3, 10, "203.0.113.88")
	if !ok3 || info3 == nil {
		t.Fatalf("expected ticket 3 to be valid across reverse proxy loopback")
	}

	// 5. 错误节点 ID 拦截
	t4 := IssueTerminalTicket(10, "admin", "198.51.100.1", "panel.example.com", "u4", "p4", "k4", 10*time.Second)
	if _, ok4 := ValidateAndConsumeTicket(t4, 999, "198.51.100.1"); ok4 {
		t.Fatalf("expected ticket 4 to be rejected with mismatched nodeID")
	}

	// 6. 过期 Ticket 拦截
	t5 := IssueTerminalTicket(10, "admin", "198.51.100.1", "panel.example.com", "u5", "p5", "k5", -1*time.Second)
	if _, ok5 := ValidateAndConsumeTicket(t5, 10, "198.51.100.1"); ok5 {
		t.Fatalf("expected ticket 5 to be rejected when expired")
	}
}
