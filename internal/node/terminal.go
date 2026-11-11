package node

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"vps-auto-deploy/internal/model"
	"vps-auto-deploy/internal/pkg/crypto"
)

type terminalTicketInfo struct {
	NodeID    int64
	ExpiresAt time.Time
}

var (
	ticketLock sync.RWMutex
	ticketMap  = make(map[string]terminalTicketInfo)
)

// IssueTerminalTicket 为目标节点签发一个一次性有效、30秒有效期的终端票据
func IssueTerminalTicket(nodeID int64, ttl time.Duration) string {
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

	ticketMap[ticket] = terminalTicketInfo{
		NodeID:    nodeID,
		ExpiresAt: now.Add(ttl),
	}
	return ticket
}

// ValidateAndConsumeTicket 验证并立即销毁单次票据（防重放攻击）
func ValidateAndConsumeTicket(ticket string, nodeID int64) bool {
	if ticket == "" {
		return false
	}
	ticketLock.Lock()
	defer ticketLock.Unlock()

	info, exists := ticketMap[ticket]
	if !exists {
		return false
	}
	delete(ticketMap, ticket)

	if time.Now().After(info.ExpiresAt) {
		return false
	}
	return info.NodeID == nodeID
}

// TerminalMessage WebSocket 前后端通信帧格式
type TerminalMessage struct {
	Type string `json:"type"`           // "input" | "resize" | "ping" | "pong"
	Data string `json:"data,omitempty"` // 用户按键输入或终端输出内容
	Cols int    `json:"cols,omitempty"` // 终端列数
	Rows int    `json:"rows,omitempty"` // 终端行数
}

// HandleTerminalSession 接管 WebSocket 与宿主机 PTY/SSH 之间的全双工双向交互
func HandleTerminalSession(ws *websocket.Conn, targetNode *model.Node, initCols, initRows int, tempUser, tempPass, tempKey string) {
	defer ws.Close()

	if initCols <= 0 {
		initCols = 100
	}
	if initRows <= 0 {
		initRows = 30
	}

	if targetNode.IsMaster {
		handleLocalPTYSession(ws, initCols, initRows)
	} else {
		handleRemoteSSHSession(ws, targetNode, initCols, initRows, tempUser, tempPass, tempKey)
	}
}

// handleLocalPTYSession 为主控本机启动本地伪终端 (pty)
func handleLocalPTYSession(ws *websocket.Conn, initCols, initRows int) {
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
func handleRemoteSSHSession(ws *websocket.Conn, node *model.Node, initCols, initRows int, tempUser, tempPass, tempKey string) {
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
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31m[ERROR] SSH 连接建立失败: %v\x1b[0m\r\n", err)))
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
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
		_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\x1b[31m[ERROR] 启动远端 Shell 失败: %v\x1b[0m\r\n", err)))
		return
	}

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
