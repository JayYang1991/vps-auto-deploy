package node

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/notifier"
)

// TerminalTicketInfo 包含一次性终端令牌绑定的元数据与凭据载荷
type TerminalTicketInfo struct {
	NodeID      int64
	ExpiresAt   time.Time
	Username    string
	ClientIP    string
	AllowedHost string
	TempUser    string
	TempPass    string
	TempKey     string
}

var (
	ticketLock sync.RWMutex
	ticketMap  = make(map[string]TerminalTicketInfo)

	activeSessionsLock sync.Mutex
	activeSessionCount int
)

const (
	maxConcurrentSessions = 10
	terminalPongWait      = 60 * time.Second
	terminalPingPeriod    = (terminalPongWait * 9) / 10
	terminalReadLimit     = 64 * 1024
	maxTerminalTickets    = 1000
)

// IssueTerminalTicket 为目标节点签发一个一次性有效、30秒有效期的终端票据，并安全绑定临时凭据与合法来源域名
func IssueTerminalTicket(nodeID int64, username, clientIP, allowedHost, tempUser, tempPass, tempKey string, ttl time.Duration) string {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	ticket := hex.EncodeToString(b)

	ticketLock.Lock()
	defer ticketLock.Unlock()

	// 清理已过期的历史 ticket 并限制容量上限 (Anti-DoS)
	now := time.Now()
	for k, v := range ticketMap {
		if now.After(v.ExpiresAt) {
			delete(ticketMap, k)
		}
	}
	if len(ticketMap) >= maxTerminalTickets {
		for k := range ticketMap {
			delete(ticketMap, k)
			break
		}
	}

	ticketMap[ticket] = TerminalTicketInfo{
		NodeID:      nodeID,
		ExpiresAt:   now.Add(ttl),
		Username:    username,
		ClientIP:    clientIP,
		AllowedHost: allowedHost,
		TempUser:    tempUser,
		TempPass:    tempPass,
		TempKey:     tempKey,
	}
	return ticket
}

// IsLoopbackOrPrivate 检查 IP 地址是否属于本机回环或私有内网网段
func IsLoopbackOrPrivate(ipStr string) bool {
	ipStr = strings.TrimSpace(ipStr)
	if ipStr == "" {
		return false
	}
	if ipStr == "127.0.0.1" || ipStr == "::1" || strings.HasPrefix(ipStr, "127.") || strings.HasPrefix(ipStr, "::ffff:127.") {
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

// ValidateAndConsumeTicket 验证并立即销毁单次票据（防重放攻击与中间人冒领），并返回绑定的凭据元数据
func ValidateAndConsumeTicket(ticket string, nodeID int64, clientIP ...string) (*TerminalTicketInfo, bool) {
	if ticket == "" {
		return nil, false
	}
	ticketLock.Lock()
	defer ticketLock.Unlock()

	info, exists := ticketMap[ticket]
	if !exists {
		return nil, false
	}
	delete(ticketMap, ticket)

	if time.Now().After(info.ExpiresAt) {
		return nil, false
	}
	if info.NodeID != nodeID {
		return nil, false
	}

	// 记录真实消费客户端 IP 迁移情况 (如 CDN 跨边缘节点分发、多出口漫游)
	if len(clientIP) > 0 && clientIP[0] != "" && info.ClientIP != "" && info.ClientIP != clientIP[0] {
		log.Printf("[TerminalTicket] 客户端 IP 迁移: issued=%s, current=%s, nodeID=%d (单次票据消费成功)", info.ClientIP, clientIP[0], nodeID)
	}

	return &info, true
}

// RecordTerminalAudit 记录终端特权运维审计日志
func RecordTerminalAudit(event, username, clientIP, nodeName, targetAddr, details string) {
	nowStr := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] [AUDIT-TERMINAL] event=%s user=%q ip=%s node=%q target=%s details=%q\n",
		nowStr, event, username, clientIP, nodeName, targetAddr, details)

	// 控制台标准输出
	log.Print(entry)

	// 写入持久化审计日志文件
	auditPaths := []string{
		"/etc/vps-auto-deploy/audit.log",
		"audit.log",
		filepath.Join(os.TempDir(), "vps-panel-audit.log"),
	}

	for _, p := range auditPaths {
		_ = os.MkdirAll(filepath.Dir(p), 0750)
		/* #nosec G304 - internal fixed audit log paths */
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err == nil {
			_, _ = f.WriteString(entry)
			_ = f.Close()
			break
		}
	}

	if event == "CONNECT" || event == "CONNECTED" {
		notifier.Send(notifier.Event{
			Type:      notifier.EventTerminalSession,
			Level:     notifier.LevelWarn,
			Title:     "特权 Web 终端控制台已建立连接",
			Message:   fmt.Sprintf("用户 %s 从 %s 建立了对目标 [%s] (%s) 的交互式终端会话: %s", username, clientIP, nodeName, targetAddr, details),
			ClientIP:  clientIP,
			Username:  username,
			NodeName:  nodeName,
			Timestamp: time.Now(),
			Metadata: map[string]string{
				"event":       event,
				"target_addr": targetAddr,
			},
		})
	} else if event == "CONNECT_FAILED" {
		notifier.Send(notifier.Event{
			Type:      notifier.EventTerminalSession,
			Level:     notifier.LevelCritical,
			Title:     "特权 Web 终端连接建立失败",
			Message:   fmt.Sprintf("用户 %s 从 %s 尝试连接目标 [%s] (%s) 失败: %s", username, clientIP, nodeName, targetAddr, details),
			ClientIP:  clientIP,
			Username:  username,
			NodeName:  nodeName,
			Timestamp: time.Now(),
			Metadata: map[string]string{
				"event":       event,
				"target_addr": targetAddr,
			},
		})
	}
}

// TerminalMessage WebSocket 前后端通信帧格式
type TerminalMessage struct {
	Type string `json:"type"`           // "input" | "resize" | "ping" | "pong"
	Data string `json:"data,omitempty"` // 用户按键输入或终端输出内容
	Cols int    `json:"cols,omitempty"` // 终端列数
	Rows int    `json:"rows,omitempty"` // 终端行数
}

// HandleTerminalSession 接管 WebSocket 与宿主机 PTY/SSH 之间的全双工双向交互
func HandleTerminalSession(ws *websocket.Conn, targetNode *model.Node, initCols, initRows int, ticketInfo *TerminalTicketInfo) {
	defer ws.Close()

	activeSessionsLock.Lock()
	if activeSessionCount >= maxConcurrentSessions {
		activeSessionsLock.Unlock()
		sendTerminalErrorAndClose(ws, "[ERROR] 活跃终端会话数已达并发上限 (10)，请关闭已有终端后再试。")
		return
	}
	activeSessionCount++
	activeSessionsLock.Unlock()

	defer func() {
		activeSessionsLock.Lock()
		activeSessionCount--
		activeSessionsLock.Unlock()
	}()

	if initCols <= 0 {
		initCols = 100
	}
	if initRows <= 0 {
		initRows = 30
	}

	user := "admin"
	clientIP := "unknown"
	tempUser, tempPass, tempKey := "", "", ""
	if ticketInfo != nil {
		if ticketInfo.Username != "" {
			user = ticketInfo.Username
		}
		if ticketInfo.ClientIP != "" {
			clientIP = ticketInfo.ClientIP
		}
		tempUser = ticketInfo.TempUser
		tempPass = ticketInfo.TempPass
		tempKey = ticketInfo.TempKey
	}

	if targetNode.IsMaster {
		handleLocalPTYSession(ws, initCols, initRows, user, clientIP)
	} else {
		handleRemoteSSHSession(ws, targetNode, initCols, initRows, user, clientIP, tempUser, tempPass, tempKey)
	}
}

// sendTerminalErrorAndClose 向客户端发送格式化错误提示，优雅关闭 WebSocket 并等待缓冲区刷新
func sendTerminalErrorAndClose(ws *websocket.Conn, errMsg string) {
	msg := fmt.Sprintf("\r\n\x1b[31m%s\x1b[0m\r\n", errMsg)
	_ = ws.WriteMessage(websocket.TextMessage, []byte(msg))
	_ = ws.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session error"),
		time.Now().Add(time.Second))
	time.Sleep(800 * time.Millisecond)
}

// handleLocalPTYSession 为主控本机启动本地伪终端 (pty)
func handleLocalPTYSession(ws *websocket.Conn, initCols, initRows int, user, clientIP string) {
	startTime := time.Now()
	RecordTerminalAudit("CONNECT", user, clientIP, "本地主控", "127.0.0.1 (Local PTY)", "启动本机 Bash 控制台")
	defer func() {
		duration := time.Since(startTime).Round(time.Second)
		RecordTerminalAudit("DISCONNECT", user, clientIP, "本地主控", "127.0.0.1 (Local PTY)", fmt.Sprintf("会话已关闭，持续时长: %v", duration))
	}()

	shell := "/bin/bash"
	if _, err := exec.LookPath(shell); err != nil {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
	)

	safeRows := uint16(32)
	if initRows >= 5 && initRows <= 1000 {
		safeRows = uint16(initRows)
	}
	safeCols := uint16(110)
	if initCols >= 10 && initCols <= 1000 {
		safeCols = uint16(initCols)
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: safeRows,
		Cols: safeCols,
	})
	if err != nil {
		sendTerminalErrorAndClose(ws, fmt.Sprintf("[ERROR] 启动本地伪终端失败: %v", err))
		return
	}
	defer func() {
		_ = ptmx.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	_ = ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[32m✔ 已连接至本地主控端控制台 Shell (Local PTY)\x1b[0m\r\n\r\n"))

	ws.SetReadLimit(terminalReadLimit)
	_ = ws.SetReadDeadline(time.Now().Add(terminalPongWait))
	ws.SetPongHandler(func(string) error {
		_ = ws.SetReadDeadline(time.Now().Add(terminalPongWait))
		return nil
	})

	var closeOnce sync.Once
	done := make(chan struct{})

	// 0. 服务端定时心跳 Ping，侦测半开连接与网络断流
	go func() {
		ticker := time.NewTicker(terminalPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		}
	}()

	// 1. PTY stdout -> WebSocket
	go func() {
		defer closeOnce.Do(func() { close(done) })
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				if wErr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// 2. WebSocket -> PTY stdin
	go func() {
		defer closeOnce.Do(func() { close(done) })
		for {
			msgType, payload, err := ws.ReadMessage()
			if err != nil {
				return
			}
			_ = ws.SetReadDeadline(time.Now().Add(terminalPongWait))

			if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
				var ctrl TerminalMessage
				if err := json.Unmarshal(payload, &ctrl); err == nil && ctrl.Type != "" {
					switch ctrl.Type {
					case "resize":
						if ctrl.Cols >= 10 && ctrl.Cols <= 1000 && ctrl.Rows >= 5 && ctrl.Rows <= 1000 {
							_ = pty.Setsize(ptmx, &pty.Winsize{
								Rows: uint16(ctrl.Rows),
								Cols: uint16(ctrl.Cols),
							})
						}
					case "input":
						_, _ = ptmx.Write([]byte(ctrl.Data))
					case "ping":
						_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"pong"}`))
					}
				} else {
					// 纯文本/原始字节流输入
					_, _ = ptmx.Write(payload)
				}
			}
		}
	}()

	<-done
}

// handleRemoteSSHSession 建立至远程纳管节点的 SSH PTY 会话
func handleRemoteSSHSession(ws *websocket.Conn, node *model.Node, initCols, initRows int, user, clientIP, tempUser, tempPass, tempKey string) {
	sshUser := node.SSHUser
	if sshUser == "" {
		sshUser = "root"
	}
	if tempUser != "" {
		sshUser = tempUser
	}

	var authMethods []ssh.AuthMethod

	// 优先使用传入的临时凭据或解密已持久化的凭据
	rawPass := tempPass
	if rawPass == "" && node.SSHPasswordEncrypted != "" {
		if dec, err := model.DecryptSensitiveData(node.SSHPasswordEncrypted); err == nil && dec != "" {
			rawPass = dec
		}
	}

	rawKey := tempKey
	if rawKey == "" && node.SSHKeyEncrypted != "" {
		if dec, err := model.DecryptSensitiveData(node.SSHKeyEncrypted); err == nil && dec != "" {
			rawKey = dec
		}
	}

	// 智能备选回退：若未单独设置 SSH 专用密码，尝试使用该节点的面板管理密码作为 SSH 密码尝试
	if rawPass == "" && rawKey == "" && node.PasswordEncrypted != "" {
		if dec, err := model.DecryptSensitiveData(node.PasswordEncrypted); err == nil && dec != "" {
			rawPass = dec
		}
	}

	if rawKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(rawKey))
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}
	if rawPass != "" {
		authMethods = append(authMethods, ssh.Password(rawPass))
	}

	if len(authMethods) == 0 {
		sendTerminalErrorAndClose(ws, "[ERROR] 该远程节点尚未配置 SSH 登录凭据（密码或私钥），请先在弹窗中录入凭据！")
		return
	}

	sshPort := node.SSHPort
	if sshPort <= 0 {
		sshPort = 22
	}

	targetAddr := fmt.Sprintf("%s:%d", node.Host, sshPort)
	startTime := time.Now()
	RecordTerminalAudit("CONNECT_ATTEMPT", user, clientIP, node.Name, targetAddr, fmt.Sprintf("SSH发起连接 (登录账号: %s)", sshUser))

	defer func() {
		duration := time.Since(startTime).Round(time.Second)
		RecordTerminalAudit("DISCONNECT", user, clientIP, node.Name, targetAddr, fmt.Sprintf("SSH会话已结束，持续时长: %v", duration))
	}()

	// TOFU 主机指纹比对机制
	sshConfig := &ssh.ClientConfig{
		User: sshUser,
		Auth: authMethods,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			currentFP := ssh.FingerprintSHA256(key)
			if node.SSHHostKey != "" && subtle.ConstantTimeCompare([]byte(currentFP), []byte(node.SSHHostKey)) != 1 {
				return fmt.Errorf("【安全拦截】SSH 主机指纹不匹配！已记录: %s, 实际: %s (防中间人冒充)", node.SSHHostKey, currentFP)
			}
			if node.SSHHostKey == "" {
				node.SSHHostKey = currentFP
				_ = model.UpdateNodeSSHHostKey(node.ID, currentFP)
			}
			return nil
		},
		Timeout: 10 * time.Second,
	}

	addr := net.JoinHostPort(node.Host, strconv.Itoa(sshPort))
	_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[33m正在建立 SSH 安全信道至 %s (%s:%d)...\x1b[0m\r\n", node.Name, node.Host, sshPort)))

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		RecordTerminalAudit("CONNECT_FAILED", user, clientIP, node.Name, targetAddr, fmt.Sprintf("握手失败: %v", err))
		sendTerminalErrorAndClose(ws, fmt.Sprintf("[ERROR] SSH 连接建立失败: %v", err))
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		RecordTerminalAudit("CONNECT_FAILED", user, clientIP, node.Name, targetAddr, fmt.Sprintf("创建 Session 失败: %v", err))
		sendTerminalErrorAndClose(ws, fmt.Sprintf("[ERROR] 创建 SSH Session 失败: %v", err))
		return
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	safeSSHRows := initRows
	if initRows < 5 || initRows > 1000 {
		safeSSHRows = 32
	}
	safeSSHCols := initCols
	if initCols < 10 || initCols > 1000 {
		safeSSHCols = 110
	}
	if err := session.RequestPty("xterm-256color", safeSSHRows, safeSSHCols, modes); err != nil {
		sendTerminalErrorAndClose(ws, fmt.Sprintf("[ERROR] 请求 PTY 失败: %v", err))
		return
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		sendTerminalErrorAndClose(ws, fmt.Sprintf("[ERROR] 打开 StdinPipe 失败: %v", err))
		return
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		sendTerminalErrorAndClose(ws, fmt.Sprintf("[ERROR] 打开 StdoutPipe 失败: %v", err))
		return
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		sendTerminalErrorAndClose(ws, fmt.Sprintf("[ERROR] 打开 StderrPipe 失败: %v", err))
		return
	}

	if err := session.Shell(); err != nil {
		RecordTerminalAudit("CONNECT_FAILED", user, clientIP, node.Name, targetAddr, fmt.Sprintf("启动 Shell 失败: %v", err))
		sendTerminalErrorAndClose(ws, fmt.Sprintf("[ERROR] 启动远端 Shell 失败: %v", err))
		return
	}

	RecordTerminalAudit("CONNECTED", user, clientIP, node.Name, targetAddr, "远端 Shell 交互就绪")
	_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[32m✔ SSH 安全终端建立成功！已连接至 %s (%s)\x1b[0m\r\n\r\n", node.Name, node.Host)))

	ws.SetReadLimit(terminalReadLimit)
	_ = ws.SetReadDeadline(time.Now().Add(terminalPongWait))
	ws.SetPongHandler(func(string) error {
		_ = ws.SetReadDeadline(time.Now().Add(terminalPongWait))
		return nil
	})

	var closeOnce sync.Once
	done := make(chan struct{})

	// 0. 定时心跳 Ping，侦测半开连接与网络断流
	go func() {
		ticker := time.NewTicker(terminalPingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
					closeOnce.Do(func() { close(done) })
					return
				}
			}
		}
	}()

	// 1. SSH stdout/stderr -> WebSocket
	reader := io.MultiReader(stdoutPipe, stderrPipe)
	go func() {
		defer closeOnce.Do(func() { close(done) })
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				if wErr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); wErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// 2. WebSocket -> SSH stdinPipe
	go func() {
		defer closeOnce.Do(func() { close(done) })
		for {
			msgType, payload, err := ws.ReadMessage()
			if err != nil {
				return
			}
			_ = ws.SetReadDeadline(time.Now().Add(terminalPongWait))

			if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
				var ctrl TerminalMessage
				if err := json.Unmarshal(payload, &ctrl); err == nil && ctrl.Type != "" {
					switch ctrl.Type {
					case "resize":
						if ctrl.Cols >= 10 && ctrl.Cols <= 1000 && ctrl.Rows >= 5 && ctrl.Rows <= 1000 {
							_ = session.WindowChange(ctrl.Rows, ctrl.Cols)
						}
					case "input":
						_, _ = stdinPipe.Write([]byte(ctrl.Data))
					case "ping":
						_ = ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"pong"}`))
					}
				} else {
					_, _ = stdinPipe.Write(payload)
				}
			}
		}
	}()

	<-done
}
