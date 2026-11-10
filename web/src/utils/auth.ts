import { ref } from 'vue'

const BROADCAST_CHANNEL_NAME = 'vps_panel_auth'
const LAST_ACTIVITY_KEY = 'vps_last_activity'
const IDLE_TIMEOUT_KEY = 'session_idle_timeout'
const REMEMBER_ME_KEY = 'vps_remember_me'
const TOKEN_KEY = 'token'

let broadcastChannel: BroadcastChannel | null = null
if (typeof window !== 'undefined' && 'BroadcastChannel' in window) {
  try {
    broadcastChannel = new BroadcastChannel(BROADCAST_CHANNEL_NAME)
  } catch {
    broadcastChannel = null
  }
}

// 缓存配置的空闲超时时间 (分钟)，默认 30 分钟，0 为禁用
const initialTimeout = parseInt(localStorage.getItem(IDLE_TIMEOUT_KEY) || '30', 10)
export const sessionIdleTimeoutMinutes = ref<number>(isNaN(initialTimeout) ? 30 : initialTimeout)

let checkIntervalId: number | null = null
let lastThrottledRecord = 0
const logoutCallbacks: Array<(reason?: string) => void> = []

export function onAuthLogout(cb: (reason?: string) => void) {
  logoutCallbacks.push(cb)
}

function triggerLogoutCallbacks(reason?: string) {
  logoutCallbacks.forEach((cb) => {
    try {
      cb(reason)
    } catch (e) {
      console.error('logout callback error:', e)
    }
  })
}

// 获取有效 Token：优先读取 sessionStorage (临时会话)，兜底读取 localStorage (持久会话)
export function getToken(): string | null {
  if (typeof window === 'undefined') return null
  const sessionToken = sessionStorage.getItem(TOKEN_KEY)
  if (sessionToken) return sessionToken
  return localStorage.getItem(TOKEN_KEY)
}

// 是否勾选了“记住我”
export function isRemembered(): boolean {
  return localStorage.getItem(REMEMBER_ME_KEY) === 'true'
}

// 存储 Token：若 rememberMe 为 true 存 localStorage，否则存 sessionStorage (关浏览器即销毁)
export function setToken(token: string, rememberMe = false) {
  if (typeof window === 'undefined') return
  if (rememberMe) {
    localStorage.setItem(TOKEN_KEY, token)
    localStorage.setItem(REMEMBER_ME_KEY, 'true')
    sessionStorage.removeItem(TOKEN_KEY)
  } else {
    sessionStorage.setItem(TOKEN_KEY, token)
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(REMEMBER_ME_KEY)
  }

  recordActivity()
  postAuthMessage({ type: 'login' })
}

// 清除凭据与会话活跃记录
export function clearToken(reason?: string, notifyOthers = true) {
  if (typeof window === 'undefined') return
  sessionStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(REMEMBER_ME_KEY)
  localStorage.removeItem(LAST_ACTIVITY_KEY)

  if (notifyOthers) {
    postAuthMessage({ type: 'logout', reason })
  }
  triggerLogoutCallbacks(reason)
}

// 更新空闲超时配置 (分钟)
export function updateSessionIdleTimeout(minutes: number) {
  sessionIdleTimeoutMinutes.value = minutes
  localStorage.setItem(IDLE_TIMEOUT_KEY, String(minutes))
}

// 记录用户当前活跃时刻 (所有多标签页通过 localStorage 共享)
export function recordActivity() {
  if (typeof window === 'undefined') return
  const now = Date.now()
  localStorage.setItem(LAST_ACTIVITY_KEY, String(now))
}

// 节流记录用户输入/点击活跃操作 (每 3 秒最多写入一次，降低开销)
function throttledRecordActivity() {
  const now = Date.now()
  if (now - lastThrottledRecord > 3000) {
    lastThrottledRecord = now
    recordActivity()
  }
}

// 广播跨标签页认证事件
function postAuthMessage(msg: { type: 'login' | 'logout' | 'activity'; reason?: string; timestamp?: number }) {
  if (broadcastChannel) {
    try {
      broadcastChannel.postMessage(msg)
    } catch {
      // 容错处理
    }
  }
  // 借助 localStorage 的 storage 事件作为跨标签页广播兜底
  try {
    localStorage.setItem('vps_auth_sync_event', JSON.stringify({ ...msg, _t: Date.now() }))
  } catch {
    // 忽略
  }
}

// 启动空闲超时全局检测与多标签页通信监听
export function initSessionSecurity() {
  if (typeof window === 'undefined') return

  // 1. 监听用户交互行为 (鼠标、键盘、点击、触摸滚动)
  const activityEvents = ['mousedown', 'mousemove', 'keydown', 'scroll', 'touchstart']
  activityEvents.forEach((ev) => {
    window.addEventListener(ev, throttledRecordActivity, { passive: true })
  })

  // 2. 监听跨标签页消息 (BroadcastChannel)
  if (broadcastChannel) {
    broadcastChannel.onmessage = (event) => {
      handleSyncMessage(event.data)
    }
  }

  // 3. 监听跨标签页 Storage 事件 (兜底方案)
  window.addEventListener('storage', (e) => {
    if (e.key === 'vps_auth_sync_event' && e.newValue) {
      try {
        const data = JSON.parse(e.newValue)
        handleSyncMessage(data)
      } catch {
        // 忽略
      }
    } else if (e.key === TOKEN_KEY && !e.newValue && !sessionStorage.getItem(TOKEN_KEY)) {
      // 另一标签页直接删除了 localStorage 的 token
      clearToken('remote_logout', false)
    }
  })

  // 4. 定时心跳检查是否超时 (每 5 秒轮询一次)
  if (checkIntervalId) clearInterval(checkIntervalId)
  checkIntervalId = window.setInterval(() => {
    checkIdleTimeout()
  }, 5000)
}

function handleSyncMessage(data: any) {
  if (!data || !data.type) return
  if (data.type === 'logout') {
    // 收到其他标签页注销通知，静默清理本地并重定向
    sessionStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(TOKEN_KEY)
    localStorage.removeItem(REMEMBER_ME_KEY)
    triggerLogoutCallbacks(data.reason || 'remote_logout')
  }
}

function checkIdleTimeout() {
  const token = getToken()
  if (!token) return

  // 超时设为 0 代表禁用自动退出
  const timeoutMinutes = sessionIdleTimeoutMinutes.value
  if (timeoutMinutes <= 0) return

  const lastActivityStr = localStorage.getItem(LAST_ACTIVITY_KEY)
  const lastActivity = lastActivityStr ? parseInt(lastActivityStr, 10) : 0
  if (!lastActivity) {
    recordActivity()
    return
  }

  const elapsedMs = Date.now() - lastActivity
  const maxAllowedMs = timeoutMinutes * 60 * 1000

  if (elapsedMs >= maxAllowedMs) {
    // 触发空闲超时退出
    clearToken('idle_timeout')
  }
}

