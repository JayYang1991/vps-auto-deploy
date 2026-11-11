package node

import (
	"crypto/rand"
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
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
)

// TerminalTicketInfo 包含一次性终端令牌绑定的元数据与凭据载荷
type TerminalTicketInfo struct {
	NodeID    int64
	ExpiresAt time.Time
	Username  string
	ClientIP  string
	TempUser  string
	TempPass  string
	TempKey   string
}

var (
	ticketLock sync.RWMutex
	ticketMap  = make(map[string]TerminalTicketInfo)
)

// IssueTerminalTicket 为目标节点签发一个一次性有效、30秒有效期的终端票据，并安全绑定临时凭据
func IssueTerminalTicket(nodeID int64, username, clientIP, tempUser, tempPass, tempKey string, ttl time.Duration) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	ticket := hex.EncodeToString(b)

	ticketLock.Lock()
	defer ticketLock.Unlock()

	// 清理已过期的历史 ticket
	now := time.Now()
	for k, v := range ticketMap {
		if now.After(v.ExpiresAt) {
			delete(ticketMap, k)
		}
	}

	ticketMap[ticket] = TerminalTicketInfo{
		NodeID:    nodeID,
		ExpiresAt: now.Add(ttl),
		Username:  username,
		ClientIP:  clientIP,
		TempUser:  tempUser,
		TempPass:  tempPass,
		TempKey:   tempKey,
	}
	return ticket
}

// ValidateAndConsumeTicket 验证并立即销毁单次票据（防重放攻击），并返回绑定的凭据元数据
func ValidateAndConsumeTicket(ticket string, nodeID int64) (*TerminalTicketInfo, bool) {
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
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err == nil {
			_, _ = f.WriteString(entry)
			_ = f.Close()
			break
		}
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

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(initRows),
		Cols: uint16(initCols),
	})
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n[ERROR] 启动本地伪终端失败: %v\r\n", err)))
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

	var closeOnce sync.Once
	done := make(chan struct{})

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

			if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
				var ctrl TerminalMessage
				if err := json.Unmarshal(payload, &ctrl); err == nil && ctrl.Type != "" {
					switch ctrl.Type {
					case "resize":
						if ctrl.Cols > 0 && ctrl.Rows > 0 {
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
	jwtSecret, _ := model.GetSetting("jwt_secret")
	key := crypto.DeriveKey(jwtSecret)

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
		if dec, err := crypto.DecryptAESGCM(node.SSHPasswordEncrypted, key); err == nil {
			rawPass = dec
		}
	}

	rawKey := tempKey
	if rawKey == "" && node.SSHKeyEncrypted != "" {
		if dec, err := crypto.DecryptAESGCM(node.SSHKeyEncrypted, key); err == nil {
			rawKey = dec
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
		_ = ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m[ERROR] 该远程节点尚未配置 SSH 登录凭据（密码或私钥），请先在弹窗中录入凭据！\x1b[0m\r\n"))
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
			if node.SSHHostKey != "" && currentFP != node.SSHHostKey {
				return fmt.Errorf("【安全拦截】SSH 主机指纹不匹配！已记录: %s, 实际: %s (防中间人冒充)", node.SSHHostKey, currentFP)
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
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31m[ERROR] SSH 连接建立失败: %v\x1b[0m\r\n", err)))
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		RecordTerminalAudit("CONNECT_FAILED", user, clientIP, node.Name, targetAddr, fmt.Sprintf("创建 Session 失败: %v", err))
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31m[ERROR] 创建 SSH Session 失败: %v\x1b[0m\r\n", err)))
		return
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", initRows, initCols, modes); err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31m[ERROR] 请求 PTY 失败: %v\x1b[0m\r\n", err)))
		return
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n[ERROR] 打开 StdinPipe 失败: %v\r\n", err)))
		return
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n[ERROR] 打开 StdoutPipe 失败: %v\r\n", err)))
		return
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n[ERROR] 打开 StderrPipe 失败: %v\r\n", err)))
		return
	}

	if err := session.Shell(); err != nil {
		RecordTerminalAudit("CONNECT_FAILED", user, clientIP, node.Name, targetAddr, fmt.Sprintf("启动 Shell 失败: %v", err))
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31m[ERROR] 启动远端 Shell 失败: %v\x1b[0m\r\n", err)))
		return
	}

	RecordTerminalAudit("CONNECTED", user, clientIP, node.Name, targetAddr, "远端 Shell 交互就绪")
	_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[32m✔ SSH 安全终端建立成功！已连接至 %s (%s)\x1b[0m\r\n\r\n", node.Name, node.Host)))

	var closeOnce sync.Once
	done := make(chan struct{})

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

			if msgType == websocket.TextMessage || msgType == websocket.BinaryMessage {
				var ctrl TerminalMessage
				if err := json.Unmarshal(payload, &ctrl); err == nil && ctrl.Type != "" {
					switch ctrl.Type {
					case "resize":
						if ctrl.Cols > 0 && ctrl.Rows > 0 {
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
