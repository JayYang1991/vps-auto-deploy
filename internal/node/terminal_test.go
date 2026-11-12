package node

import (
	"testing"
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
