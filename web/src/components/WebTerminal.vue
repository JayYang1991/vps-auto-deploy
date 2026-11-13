<template>
  <div v-if="modelValue" class="terminal-overlay" :class="{ 'fullscreen-overlay': isFullscreen }">
    <div class="terminal-window" :class="{ 'fullscreen-window': isFullscreen }">
      <!-- 终端头部工具栏 -->
      <div class="terminal-header">
        <div class="header-left">
          <span class="terminal-badge-icon">💻</span>
          <div class="header-info">
            <div class="title-row">
              <span class="node-title">{{ node?.name || '远程终端' }}</span>
              <span v-if="node?.is_master" class="tag-master">本地主控 PTY</span>
              <span v-else class="tag-remote">SSH 信道</span>
              <span class="status-indicator" :class="statusClass">
                <span class="status-dot"></span>
                <span>{{ statusText }}</span>
              </span>
            </div>
            <div class="host-row mono">
              <span>{{ node?.is_master ? '127.0.0.1 (Local)' : `${node?.host}:${node?.ssh_port || 22}` }}</span>
              <span v-if="node?.ssh_user && !node?.is_master" class="user-pill">{{ node.ssh_user }}</span>
            </div>
          </div>
        </div>

        <div class="header-actions">
          <button class="action-btn" title="复制选中文本" @click="handleCopySelection">
            <span>📋 复制</span>
          </button>
          <button class="action-btn" title="清空屏幕" @click="handleClearScreen">
            <span>🧹 清屏</span>
          </button>
          <button class="action-btn" title="重新连接" @click="reconnect" :disabled="connecting">
            <span>🔄 重连</span>
          </button>
          <button class="action-btn" :title="isFullscreen ? '退出全屏' : '全屏切换'" @click="toggleFullscreen">
            <span>{{ isFullscreen ? '🗗 还原' : '⛶ 全屏' }}</span>
          </button>
          <button class="close-btn" title="关闭控制台" @click="close">✕</button>
        </div>
      </div>

      <!-- 快捷常用运维命令栏 -->
      <div v-if="connectionStatus === 'connected'" class="quick-command-bar">
        <span class="quick-title">⚡ 快捷指令:</span>
        <button class="quick-btn" @click="sendCommand('htop\r')">htop 监控</button>
        <button class="quick-btn" @click="sendCommand('free -h && df -h\r')">内存与磁盘</button>
        <button class="quick-btn" @click="sendCommand('ss -tulpn\r')">监听端口</button>
        <button class="quick-btn" @click="sendCommand('journalctl -u vps-panel -n 50 -f\r')">面板实时日志</button>
        <button class="quick-btn" @click="sendCommand('systemctl status vps-panel\r')">面板状态</button>
        <button class="quick-btn" @click="sendCommand('ps aux --sort=-%cpu | head -n 12\r')">高 CPU 进程</button>
        <button class="quick-btn danger" @click="sendCommand('\x03')">Ctrl+C 中断</button>
      </div>

      <!-- 凭据录入表单 (仅在非主控且未保存 SSH 凭据时呈现) -->
      <div v-if="connectionStatus === 'need_credentials'" class="credentials-form-box">
        <div class="cred-card">
          <div class="cred-card-header">
            <span class="card-icon">🔑</span>
            <div>
              <h4 class="card-title">录入 SSH 登录凭据</h4>
              <p class="card-desc">目标主机尚未配置免密凭据，请输入 SSH 账号与密码或私钥以建立终端会话</p>
            </div>
          </div>

          <div class="cred-fields">
            <div class="field-group">
              <label>SSH 用户名</label>
              <input v-model="sshForm.user" class="input" placeholder="root" />
            </div>

            <div class="field-group">
              <label>认证方式</label>
              <div class="auth-radio-group">
                <label>
                  <input type="radio" value="password" v-model="sshForm.authType" /> SSH 密码
                </label>
                <label>
                  <input type="radio" value="key" v-model="sshForm.authType" /> SSH 私钥
                </label>
              </div>
            </div>

            <div v-if="sshForm.authType === 'password'" class="field-group">
              <label>SSH 登录密码</label>
              <input v-model="sshForm.password" type="password" class="input" placeholder="请输入服务器 root 密码" @keyup.enter="submitCredentialsAndConnect" />
            </div>

            <div v-else class="field-group">
              <label>SSH 私钥内容 (OpenSSH / PEM)</label>
              <textarea v-model="sshForm.privateKey" class="textarea mono" rows="4" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----"></textarea>
            </div>

            <div class="field-group checkbox-row">
              <label class="checkbox-label">
                <input type="checkbox" v-model="sshForm.saveToDB" />
                <span>持久化保存加密凭据 (AES-256-GCM 保护，下次自动免密登录)</span>
              </label>
            </div>
          </div>

          <div class="cred-actions">
            <button class="btn btn-secondary" @click="close">取消</button>
            <button class="btn btn-primary" @click="submitCredentialsAndConnect" :disabled="isSubmittingCreds">
              <span>{{ isSubmittingCreds ? '连接中...' : '🚀 验证并开启终端' }}</span>
            </button>
          </div>
        </div>
      </div>

      <!-- 二次安全校验弹窗 (若未通过二次鉴权) -->
      <div v-else-if="connectionStatus === 'need_auth'" class="credentials-form-box">
        <div class="cred-card">
          <div class="cred-card-header">
            <span class="card-icon">🛡️</span>
            <div>
              <h4 class="card-title">主控二次安全验证 (Step-Up Auth)</h4>
              <p class="card-desc">SSH 控制台具有最高特权，请验证当前主控端管理员密码以签发单次连接令牌</p>
            </div>
          </div>

          <div class="cred-fields">
            <div class="field-group">
              <label>主控管理员密码</label>
              <input
                v-model="masterVerifyPass"
                type="password"
                class="input"
                placeholder="请输入主控管理面板登录密码"
                @keyup.enter="handleMasterAuthSubmit"
                autofocus
              />
            </div>
          </div>

          <div class="cred-actions">
            <button class="btn btn-secondary" @click="close">取消</button>
            <button class="btn btn-primary" @click="handleMasterAuthSubmit" :disabled="isAuthenticating || !masterVerifyPass">
              <span>{{ isAuthenticating ? '⏳ 验证中...' : '🔓 验证并进入' }}</span>
            </button>
          </div>
        </div>
      </div>

      <!-- 终端显示视口 -->
      <div v-show="connectionStatus === 'connected' || connectionStatus === 'connecting' || connectionStatus === 'disconnected'" class="terminal-body" ref="terminalContainerWrapper">
        <div ref="terminalCanvas" class="terminal-canvas"></div>
      </div>

      <!-- 移动端虚拟辅助键盘条 (适合触摸屏) -->
      <div v-if="connectionStatus === 'connected'" class="mobile-keyboard-bar">
        <button class="key-btn" @click="sendCommand('\x1b')">ESC</button>
        <button class="key-btn" @click="sendCommand('\t')">TAB</button>
        <button class="key-btn" @click="sendCommand('\x03')">Ctrl+C</button>
        <button class="key-btn" @click="sendCommand('\x1a')">Ctrl+Z</button>
        <button class="key-btn" @click="sendCommand('\x1b[A')">▲</button>
        <button class="key-btn" @click="sendCommand('\x1b[B')">▼</button>
        <button class="key-btn" @click="sendCommand('\x1b[D')">◀</button>
        <button class="key-btn" @click="sendCommand('\x1b[C')">▶</button>
        <button class="key-btn" @click="sendCommand('\r')">Enter</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import api, { getSecretPath } from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'

const props = defineProps<{
  modelValue: boolean
  node: any
  cachedMasterPassword?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'authenticated', pass: string): void
}>()

const { toast } = useToast()

const terminalCanvas = ref<HTMLDivElement | null>(null)
const terminalContainerWrapper = ref<HTMLDivElement | null>(null)
const isFullscreen = ref(false)
const connectionStatus = ref<'connecting' | 'connected' | 'disconnected' | 'need_credentials' | 'need_auth'>('connecting')

const masterVerifyPass = ref('')
const isAuthenticating = ref(false)
const connecting = ref(false)
const isSubmittingCreds = ref(false)

const sshForm = reactive({
  user: 'root',
  authType: 'password' as 'password' | 'key',
  password: '',
  privateKey: '',
  saveToDB: true,
})

let term: Terminal | null = null
let fitAddon: FitAddon | null = null
let socket: WebSocket | null = null
let resizeObserver: ResizeObserver | null = null

const statusClass = computed(() => {
  switch (connectionStatus.value) {
    case 'connected': return 'status-online'
    case 'connecting': return 'status-connecting'
    case 'disconnected': return 'status-offline'
    default: return 'status-idle'
  }
})

const statusText = computed(() => {
  switch (connectionStatus.value) {
    case 'connected': return '已连接'
    case 'connecting': return '正在握手...'
    case 'disconnected': return '已断开'
    case 'need_credentials': return '待配置凭据'
    case 'need_auth': return '待二次鉴权'
    default: return '离线'
  }
})

function toggleFullscreen() {
  isFullscreen.value = !isFullscreen.value
  nextTick(() => {
    fitAddon?.fit()
    syncTerminalSize()
  })
}

function handleOverlayClick() {
  // 不强制关闭，避免误触丢失正在执行的 Shell 任务
}

function close() {
  destroyTerminal()
  connectionStatus.value = 'connecting'
  sshForm.password = ''
  sshForm.privateKey = ''
  emit('update:modelValue', false)
}

function sendCommand(cmd: string) {
  if (socket && socket.readyState === WebSocket.OPEN) {
    socket.send(JSON.stringify({ type: 'input', data: cmd }))
    term?.focus()
  }
}

function handleCopySelection() {
  if (!term) return
  const text = term.getSelection()
  if (!text) {
    toast.warning('请先在终端中鼠标选中文本')
    return
  }
  copyToClipboard(text, '终端选中文本')
}

function handleClearScreen() {
  term?.clear()
  term?.focus()
}

// 彻底销毁终端实例与监听器，释放 DOM 与长连接
function destroyTerminal() {
  if (socket) {
    try {
      socket.close()
    } catch {
      // ignore
    }
    socket = null
  }

  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }

  if (term) {
    try {
      term.dispose()
    } catch {
      // ignore
    }
    term = null
    fitAddon = null
  }

  if (terminalCanvas.value) {
    terminalCanvas.value.innerHTML = ''
  }
}

// 初始化并重新挂载 xterm.js 实例
function initTerminal() {
  destroyTerminal()

  term = new Terminal({
    cursorBlink: true,
    cursorStyle: 'block',
    fontSize: 13,
    fontFamily: '"JetBrains Mono", "Fira Code", "Cascadia Code", Menlo, monospace',
    theme: {
      background: '#0a0f1d',
      foreground: '#e2e8f0',
      cursor: '#38bdf8',
      cursorAccent: '#0a0f1d',
      selectionBackground: 'rgba(56, 189, 248, 0.35)',
      black: '#1e293b',
      red: '#f87171',
      green: '#4ade80',
      yellow: '#facc15',
      blue: '#60a5fa',
      magenta: '#c084fc',
      cyan: '#38bdf8',
      white: '#f8fafc',
    },
    convertEol: true,
    scrollback: 5000,
  })

  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)

  if (terminalCanvas.value) {
    term.open(terminalCanvas.value)
    nextTick(() => {
      try {
        fitAddon?.fit()
        syncTerminalSize()
      } catch {
        // ignore
      }
    })
  }

  // 监听键盘按键并发送至后端
  term.onData(data => {
    if (socket && socket.readyState === WebSocket.OPEN) {
      socket.send(JSON.stringify({ type: 'input', data }))
    }
  })

  // 监听窗口尺寸变化
  if (terminalContainerWrapper.value && window.ResizeObserver) {
    resizeObserver = new ResizeObserver(() => {
      try {
        fitAddon?.fit()
        syncTerminalSize()
      } catch {
        // ignore
      }
    })
    resizeObserver.observe(terminalContainerWrapper.value)
  }
}

function syncTerminalSize() {
  if (term && socket && socket.readyState === WebSocket.OPEN) {
    const cols = term.cols
    const rows = term.rows
    socket.send(JSON.stringify({ type: 'resize', cols, rows }))
  }
}

// 申请 Ticket 并发起终端 WebSocket 会话
async function startTerminalSession(masterPassword?: string) {
  if (!props.node) return

  connecting.value = true
  connectionStatus.value = 'connecting'

  const pass = masterPassword || props.cachedMasterPassword || masterVerifyPass.value
  if (!pass) {
    connectionStatus.value = 'need_auth'
    connecting.value = false
    return
  }

  try {
    const ticketPayload: Record<string, any> = {
      master_password: pass,
    }
    if (!props.node.is_master && !props.node.has_ssh_auth) {
      if (sshForm.user) ticketPayload.temp_user = sshForm.user
      if (sshForm.authType === 'password' && sshForm.password) {
        ticketPayload.temp_pass = sshForm.password
      }
      if (sshForm.authType === 'key' && sshForm.privateKey) {
        ticketPayload.temp_key = sshForm.privateKey
      }
    }

    const resp = await api.post(`/nodes/${props.node.id}/terminal-ticket`, ticketPayload)

    const { ticket, is_master, has_ssh_auth } = resp.data
    emit('authenticated', pass)

    // 若非主控且目标节点尚未配置 SSH 凭据，引导用户录入
    if (!is_master && !has_ssh_auth && !sshForm.password && !sshForm.privateKey) {
      connectionStatus.value = 'need_credentials'
      connecting.value = false
      return
    }

    connectWebSocket(ticket)
  } catch (err: any) {
    connecting.value = false
    if (err.response?.status === 403) {
      toast.error('主控密码验证失败，请重新输入')
      connectionStatus.value = 'need_auth'
    } else {
      toast.error(err.response?.data?.error || '请求终端凭据失败: ' + err.message)
      connectionStatus.value = 'disconnected'
    }
  }
}

function handleMasterAuthSubmit() {
  if (!masterVerifyPass.value) {
    toast.warning('请输入主控管理员密码')
    return
  }
  isAuthenticating.value = true
  startTerminalSession(masterVerifyPass.value).finally(() => {
    isAuthenticating.value = false
  })
}

async function submitCredentialsAndConnect() {
  if (!sshForm.password && !sshForm.privateKey) {
    toast.warning('请输入 SSH 登录密码或私钥')
    return
  }

  isSubmittingCreds.value = true
  const pass = props.cachedMasterPassword || masterVerifyPass.value

  try {
    if (sshForm.saveToDB) {
      await api.post(`/nodes/${props.node.id}/ssh-credentials`, {
        master_password: pass,
        ssh_user: sshForm.user || 'root',
        ssh_password: sshForm.authType === 'password' ? sshForm.password : '',
        ssh_private_key: sshForm.authType === 'key' ? sshForm.privateKey : '',
      })
      toast.success('SSH 凭据已加密保存')
      props.node.has_ssh_auth = true
    }

    // 重新拉起连接
    await startTerminalSession(pass)
  } catch (err: any) {
    toast.error('保存凭据或连接失败: ' + (err.response?.data?.error || err.message))
  } finally {
    isSubmittingCreds.value = false
  }
}

function connectWebSocket(ticket: string) {
  initTerminal()
  term?.reset()
  term?.writeln('\x1b[36m正在建立双工通信流...\x1b[0m')

  const secret = getSecretPath()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const cols = term?.cols || 110
  const rows = term?.rows || 32

  const url = `${protocol}//${window.location.host}/${secret}/api/nodes/${props.node.id}/terminal?ticket=${ticket}&cols=${cols}&rows=${rows}`

  socket = new WebSocket(url)
  socket.binaryType = 'arraybuffer'

  socket.onopen = () => {
    connecting.value = false
    connectionStatus.value = 'connected'
    nextTick(() => {
      setTimeout(() => {
        try {
          fitAddon?.fit()
          syncTerminalSize()
          term?.focus()
        } catch {
          // ignore
        }
      }, 60)
    })
  }

  socket.onmessage = event => {
    if (typeof event.data === 'string') {
      try {
        const msg = JSON.parse(event.data)
        if (msg.type === 'pong') return
      } catch {
        term?.write(event.data)
      }
    } else {
      const u8 = new Uint8Array(event.data)
      term?.write(u8)
    }
  }

  socket.onerror = () => {
    connecting.value = false
    connectionStatus.value = 'disconnected'
    term?.writeln('\r\n\x1b[31m[WebSocket 通信连接异常]\x1b[0m')
  }

  socket.onclose = event => {
    connecting.value = false
    connectionStatus.value = 'disconnected'
    const reasonInfo = event.reason ? ` (${event.reason})` : ''
    if (event.code === 1000) {
      term?.writeln(`\r\n\x1b[33m[终端会话已断开${reasonInfo}]\x1b[0m`)
    } else {
      term?.writeln(`\r\n\x1b[33m[终端会话已断开 - 代码 ${event.code}${reasonInfo}]\x1b[0m`)
    }
  }
}

function reconnect() {
  destroyTerminal()
  startTerminalSession()
}

watch(
  () => props.modelValue,
  val => {
    if (val) {
      destroyTerminal()
      connectionStatus.value = 'connecting'
      sshForm.password = ''
      sshForm.privateKey = ''
      if (props.node?.ssh_user) {
        sshForm.user = props.node.ssh_user
      } else {
        sshForm.user = 'root'
      }
      nextTick(() => {
        startTerminalSession()
      })
    } else {
      destroyTerminal()
    }
  }
)

watch(
  () => props.node?.id,
  (newId, oldId) => {
    if (props.modelValue && newId && newId !== oldId) {
      destroyTerminal()
      connectionStatus.value = 'connecting'
      sshForm.password = ''
      sshForm.privateKey = ''
      if (props.node?.ssh_user) {
        sshForm.user = props.node.ssh_user
      } else {
        sshForm.user = 'root'
      }
      nextTick(() => {
        startTerminalSession()
      })
    }
  }
)

onBeforeUnmount(() => {
  destroyTerminal()
})
</script>

<style scoped>
.terminal-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  background-color: rgba(3, 7, 18, 0.85);
  backdrop-filter: blur(8px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
}
.fullscreen-overlay {
  padding: 0;
}

.terminal-window {
  width: 90vw;
  max-width: 1280px;
  height: 82vh;
  background-color: #0a0f1d;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.7);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: all 0.2s ease;
}
.fullscreen-window {
  width: 100vw;
  max-width: none;
  height: 100vh;
  border-radius: 0;
  border: none;
}

/* Header */
.terminal-header {
  padding: 0.75rem 1.25rem;
  background-color: #0f172a;
  border-bottom: 1px solid #1e293b;
  display: flex;
  align-items: center;
  justify-content: space-between;
  user-select: none;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
.terminal-badge-icon {
  font-size: 1.5rem;
}
.title-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.node-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: #f8fafc;
}
.tag-master {
  font-size: 0.7rem;
  background-color: rgba(56, 189, 248, 0.15);
  color: #38bdf8;
  border: 1px solid rgba(56, 189, 248, 0.3);
  padding: 0.1rem 0.4rem;
  border-radius: 0.25rem;
}
.tag-remote {
  font-size: 0.7rem;
  background-color: rgba(168, 85, 247, 0.15);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.3);
  padding: 0.1rem 0.4rem;
  border-radius: 0.25rem;
}
.status-indicator {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.75rem;
  padding: 0.1rem 0.5rem;
  border-radius: 9999px;
}
.status-online {
  background-color: rgba(34, 197, 94, 0.15);
  color: #4ade80;
}
.status-connecting {
  background-color: rgba(234, 179, 8, 0.15);
  color: #facc15;
}
.status-offline {
  background-color: rgba(239, 68, 68, 0.15);
  color: #f87171;
}
.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: currentColor;
}
.host-row {
  font-size: 0.75rem;
  color: #94a3b8;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.15rem;
}
.user-pill {
  background-color: #1e293b;
  padding: 0.05rem 0.35rem;
  border-radius: 0.2rem;
  color: #cbd5e1;
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.action-btn {
  background-color: #1e293b;
  color: #e2e8f0;
  border: 1px solid #334155;
  font-size: 0.8rem;
  padding: 0.35rem 0.7rem;
  border-radius: 0.375rem;
  cursor: pointer;
  transition: all 0.15s;
}
.action-btn:hover:not(:disabled) {
  background-color: #334155;
  color: #ffffff;
}
.action-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.close-btn {
  background: transparent;
  border: none;
  color: #94a3b8;
  font-size: 1.1rem;
  cursor: pointer;
  padding: 0.3rem 0.5rem;
  border-radius: 0.375rem;
  transition: all 0.15s;
}
.close-btn:hover {
  background-color: rgba(239, 68, 68, 0.2);
  color: #f87171;
}

/* Quick Commands */
.quick-command-bar {
  background-color: #0b1329;
  border-bottom: 1px solid #1e293b;
  padding: 0.4rem 1rem;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  flex-wrap: wrap;
}
.quick-title {
  font-size: 0.75rem;
  color: #94a3b8;
  margin-right: 0.25rem;
}
.quick-btn {
  background-color: #162036;
  border: 1px solid #27354f;
  color: #cbd5e1;
  font-size: 0.75rem;
  padding: 0.2rem 0.55rem;
  border-radius: 0.25rem;
  cursor: pointer;
  transition: all 0.15s;
}
.quick-btn:hover {
  background-color: #1e2e4f;
  color: #38bdf8;
  border-color: #38bdf8;
}
.quick-btn.danger:hover {
  background-color: rgba(239, 68, 68, 0.2);
  color: #f87171;
  border-color: #f87171;
}

/* Credentials and Auth Box */
.credentials-form-box {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
}
.cred-card {
  width: 100%;
  max-width: 520px;
  background-color: #0f172a;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  padding: 1.5rem;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.5);
}
.cred-card-header {
  display: flex;
  gap: 0.75rem;
  margin-bottom: 1.25rem;
}
.card-icon {
  font-size: 1.75rem;
}
.card-title {
  font-size: 1.05rem;
  font-weight: 600;
  color: #f8fafc;
  margin: 0 0 0.25rem 0;
}
.card-desc {
  font-size: 0.8rem;
  color: #94a3b8;
  margin: 0;
  line-height: 1.4;
}
.cred-fields {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.field-group label {
  display: block;
  font-size: 0.825rem;
  color: #cbd5e1;
  margin-bottom: 0.35rem;
}
.input, .textarea {
  width: 100%;
  background-color: #0a0f1d;
  border: 1px solid #334155;
  color: #f8fafc;
  padding: 0.5rem 0.75rem;
  border-radius: 0.375rem;
  font-size: 0.875rem;
  box-sizing: border-box;
}
.input:focus, .textarea:focus {
  outline: none;
  border-color: #38bdf8;
}
.mono {
  font-family: monospace;
}
.auth-radio-group {
  display: flex;
  gap: 1.5rem;
  font-size: 0.85rem;
  color: #e2e8f0;
}
.checkbox-row {
  margin-top: 0.25rem;
}
.checkbox-label {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8rem;
  color: #94a3b8;
  cursor: pointer;
}
.cred-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
  margin-top: 1.5rem;
}

/* Terminal Body */
.terminal-body {
  flex: 1;
  position: relative;
  background-color: #0a0f1d;
  padding: 0.5rem;
  overflow: hidden;
}
.terminal-canvas {
  width: 100%;
  height: 100%;
}

/* Mobile Keyboard */
.mobile-keyboard-bar {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  background-color: #0f172a;
  border-top: 1px solid #1e293b;
  padding: 0.35rem 0.75rem;
  overflow-x: auto;
}
.key-btn {
  background-color: #1e293b;
  border: 1px solid #334155;
  color: #cbd5e1;
  font-size: 0.75rem;
  font-weight: 500;
  padding: 0.25rem 0.6rem;
  border-radius: 0.25rem;
  cursor: pointer;
  white-space: nowrap;
}
.key-btn:active {
  background-color: #38bdf8;
  color: #0a0f1d;
}

/* Button UI */
.btn {
  padding: 0.45rem 1rem;
  border-radius: 0.375rem;
  font-size: 0.85rem;
  cursor: pointer;
  border: none;
  font-weight: 500;
  transition: all 0.15s;
}
.btn-primary {
  background-color: #0284c7;
  color: #ffffff;
}
.btn-primary:hover:not(:disabled) {
  background-color: #0369a1;
}
.btn-primary:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn-secondary {
  background-color: #1e293b;
  color: #cbd5e1;
}
.btn-secondary:hover {
  background-color: #334155;
}
</style>

