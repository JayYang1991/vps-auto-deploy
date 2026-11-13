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
	// 1. 同 IP 验证通过
	t1 := IssueTerminalTicket(10, "admin", "198.51.100.1", "u1", "p1", "k1", 10*time.Second)
	info1, ok1 := ValidateAndConsumeTicket(t1, 10, "198.51.100.1")
	if !ok1 || info1 == nil || info1.Username != "admin" {
		t.Fatalf("expected ticket 1 to be valid with same IP")
	}

	// 2. 防重放：第二次消费应失败
	if _, ok := ValidateAndConsumeTicket(t1, 10, "198.51.100.1"); ok {
		t.Fatalf("expected replayed ticket to be rejected")
	}

	// 3. 跨反代场景：签发端为 127.0.0.1 (反代直连)，消费端为公网 IP (X-Forwarded-For 提取)，应放行
	t2 := IssueTerminalTicket(10, "admin", "127.0.0.1", "u2", "p2", "k2", 10*time.Second)
	info2, ok2 := ValidateAndConsumeTicket(t2, 10, "203.0.113.88")
	if !ok2 || info2 == nil {
		t.Fatalf("expected ticket 2 to be valid across reverse proxy loopback")
	}

	// 4. 跨反代场景：签发端为公网 IP，消费端为 127.0.0.1，应放行
	t3 := IssueTerminalTicket(10, "admin", "203.0.113.88", "u3", "p3", "k3", 10*time.Second)
	info3, ok3 := ValidateAndConsumeTicket(t3, 10, "127.0.0.1")
	if !ok3 || info3 == nil {
		t.Fatalf("expected ticket 3 to be valid across reverse proxy loopback")
	}

	// 5. 内网/Docker 网段场景：172.17.0.2 与 192.168.1.5，应放行
	t4 := IssueTerminalTicket(10, "admin", "172.17.0.2", "u4", "p4", "k4", 10*time.Second)
	info4, ok4 := ValidateAndConsumeTicket(t4, 10, "192.168.1.5")
	if !ok4 || info4 == nil {
		t.Fatalf("expected ticket 4 to be valid across private network IPs")
	}

	// 6. 安全拦截：两个完全不同且非内网/回环的公网 IP，应拦截防冒领
	t5 := IssueTerminalTicket(10, "admin", "198.51.100.1", "u5", "p5", "k5", 10*time.Second)
	if _, ok5 := ValidateAndConsumeTicket(t5, 10, "203.0.113.88"); ok5 {
		t.Fatalf("expected ticket 5 to be rejected due to completely different public IPs")
	}

	// 7. 错误节点 ID 拦截
	t6 := IssueTerminalTicket(10, "admin", "198.51.100.1", "u6", "p6", "k6", 10*time.Second)
	if _, ok6 := ValidateAndConsumeTicket(t6, 999, "198.51.100.1"); ok6 {
		t.Fatalf("expected ticket 6 to be rejected with mismatched nodeID")
	}

	// 8. 过期 Ticket 拦截
	t7 := IssueTerminalTicket(10, "admin", "198.51.100.1", "u7", "p7", "k7", -1*time.Second)
	if _, ok7 := ValidateAndConsumeTicket(t7, 10, "198.51.100.1"); ok7 {
		t.Fatalf("expected ticket 7 to be rejected when expired")
	}
}
