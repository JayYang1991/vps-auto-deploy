<template>
  <div>
    <!-- 主机信息概览 -->
    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 1rem; margin-bottom: 2rem;">
      <div class="card">
        <div style="color: #94a3b8; font-size: 0.875rem;">公网 IPv4</div>
        <div style="font-size: 1.25rem; font-weight: 600; margin-top: 0.5rem; color: #38bdf8;">
          {{ overview.public_ip || '检测中...' }}
        </div>
      </div>

      <div class="card">
        <div style="color: #94a3b8; font-size: 0.875rem;">系统架构</div>
        <div style="font-size: 1.25rem; font-weight: 600; margin-top: 0.5rem; color: #f1f5f9;">
          {{ overview.os }} / {{ overview.arch }} ({{ overview.num_cpu }} 核)
        </div>
      </div>

      <div class="card">
        <div style="color: #94a3b8; font-size: 0.875rem; display: flex; justify-content: space-between; align-items: center;">
          <span>BBR 加速</span>
          <button v-if="!overview.bbr" @click="enableBBR" class="btn btn-secondary" style="padding: 0.2rem 0.5rem; font-size: 0.75rem;">
            一键开启
          </button>
        </div>
        <div style="font-size: 1.25rem; font-weight: 600; margin-top: 0.5rem;">
          <span :class="['badge', overview.bbr ? 'badge-success' : 'badge-warning']">
            {{ overview.bbr ? '已启用 (BBR 运行中)' : '未启用' }}
          </span>
        </div>
      </div>

      <div class="card">
        <div style="color: #94a3b8; font-size: 0.875rem;">安全访问路径</div>
        <div style="font-size: 1.25rem; font-weight: 600; margin-top: 0.5rem; color: #a78bfa;">
          /{{ overview.secret_path }}/
        </div>
      </div>
    </div>

    <!-- 组件中心 / 服务管理 -->
    <div style="margin-bottom: 1.5rem; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
      <div>
        <h2 style="font-size: 1.25rem; font-weight: 600;">服务组件中心</h2>
        <p style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.2rem;">
          初始部署仅安装管理服务，可按需一键安装、升级或卸载代理核心与隧道服务
        </p>
      </div>

      <div style="display: flex; gap: 0.75rem; align-items: center;">
        <div style="display: flex; background: #0f172a; padding: 0.25rem; border-radius: 0.5rem; border: 1px solid #334155;">
          <button
            v-for="cat in categories"
            :key="cat.key"
            @click="activeCategory = cat.key"
            :class="['btn', activeCategory === cat.key ? 'btn-primary' : 'btn-secondary']"
            style="padding: 0.3rem 0.75rem; font-size: 0.8rem; border: none;"
          >
            {{ cat.label }}
          </button>
        </div>

        <button @click="checkForUpdates" class="btn btn-secondary" :disabled="checkingUpdates">
          {{ checkingUpdates ? '正在查询官方发布...' : '🔄 检查版本更新' }}
        </button>
      </div>
    </div>

    <!-- 组件卡片网格 -->
    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(360px, 1fr)); gap: 1.5rem;">
      <div
        v-for="c in filteredComponents"
        :key="c.name"
        class="card"
        style="display: flex; flex-direction: column; justify-content: space-between;"
      >
        <div>
          <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 0.75rem;">
            <div>
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <h3 style="font-size: 1.15rem; font-weight: 600;">{{ c.display_name || c.name }}</h3>
                <span class="badge badge-secondary" style="font-size: 0.7rem;">
                  {{ c.category === 'proxy' ? '代理核心' : '隧道穿透' }}
                </span>
              </div>
              <p style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.35rem; line-height: 1.4;">
                {{ c.description }}
              </p>
            </div>

            <span :class="['badge', getStatusBadge(c.running_status)]" style="white-space: nowrap;">
              {{ getStatusText(c.running_status) }}
            </span>
          </div>

          <!-- 版本对比信息 -->
          <div style="background-color: #0f172a; padding: 0.75rem; border-radius: 0.5rem; font-size: 0.85rem; margin-bottom: 1.25rem; border: 1px solid #1e293b;">
            <div style="display: flex; justify-content: space-between; margin-bottom: 0.35rem;">
              <span style="color: #94a3b8;">当前安装版本:</span>
              <span style="font-weight: 500;">{{ c.installed_version || '尚未安装' }}</span>
            </div>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="color: #94a3b8;">官方最新版本:</span>
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="color: #38bdf8; font-weight: 500;">{{ c.latest_version || '未获取 (点击上方检查)' }}</span>
                <span v-if="c.has_update" class="badge badge-warning" style="font-size: 0.65rem;">可更新</span>
              </div>
            </div>
          </div>
        </div>

        <div>
          <!-- 已安装服务控制栏 -->
          <div v-if="c.installed" style="margin-bottom: 0.75rem;">
            <div style="display: flex; gap: 0.5rem; margin-bottom: 0.5rem;">
              <button
                @click="componentAction(c.name, c.active ? 'stop' : 'start')"
                :class="['btn', c.active ? 'btn-danger' : 'btn-success']"
                style="flex: 1; padding: 0.4rem 0.6rem; font-size: 0.8rem;"
              >
                {{ c.active ? '⏹ 停止服务' : '▶ 启动服务' }}
              </button>
              <button
                @click="componentAction(c.name, 'restart')"
                class="btn btn-secondary"
                style="flex: 1; padding: 0.4rem 0.6rem; font-size: 0.8rem;"
              >
                🔄 重启
              </button>
              <button
                @click="openLogs(c.name)"
                class="btn btn-secondary"
                style="padding: 0.4rem 0.6rem; font-size: 0.8rem;"
                title="查看实时运行日志"
              >
                📜 日志
              </button>
            </div>

            <div style="display: flex; gap: 0.5rem;">
              <button
                @click="startInstall(c)"
                :class="['btn', c.has_update ? 'btn-primary' : 'btn-secondary']"
                style="flex: 2; padding: 0.4rem 0.6rem; font-size: 0.8rem;"
              >
                {{ c.has_update ? '🚀 升级至最新版 (' + c.latest_version + ')' : '🔄 重新覆盖安装' }}
              </button>
              <button
                @click="confirmUninstall(c)"
                class="btn btn-danger"
                style="flex: 1; padding: 0.4rem 0.6rem; font-size: 0.8rem; background-color: transparent; border-color: #ef4444;"
              >
                🗑️ 卸载
              </button>
            </div>
          </div>

          <!-- 未安装状态按钮 -->
          <div v-else>
            <button
              @click="startInstall(c)"
              class="btn btn-primary"
              style="width: 100%; font-weight: 600;"
            >
              📥 一键安装 {{ c.display_name }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 实时安装/升级终端进度弹窗 -->
    <div v-if="installModal.show" class="modal-overlay" @click.self="handleInstallModalBackdrop">
      <div class="modal-content" style="max-width: 700px; display: flex; flex-direction: column;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
          <div style="display: flex; align-items: center; gap: 0.5rem;">
            <span v-if="installModal.status === 'running'" style="font-size: 1.25rem;">⏳</span>
            <span v-else-if="installModal.status === 'success'" style="font-size: 1.25rem;">✅</span>
            <span v-else style="font-size: 1.25rem;">❌</span>
            <h3 style="font-size: 1.15rem; font-weight: 600;">
              {{ installModal.title }}
            </h3>
          </div>

          <button
            v-if="installModal.status !== 'running'"
            @click="closeInstallModal"
            class="btn btn-secondary"
            style="padding: 0.25rem 0.6rem; font-size: 0.75rem;"
          >
            关闭
          </button>
        </div>

        <!-- 终端日志框 -->
        <div
          ref="installLogContainer"
          style="background-color: #0b0f19; border: 1px solid #1e293b; padding: 1rem; border-radius: 0.5rem; height: 350px; overflow-y: auto; font-family: 'JetBrains Mono', monospace; font-size: 0.8rem; line-height: 1.5; color: #cbd5e1;"
        >
          <div v-for="(line, idx) in installModal.logs" :key="idx" style="margin-bottom: 3px; word-break: break-all;">
            <span v-if="line.includes('[SUCCESS]')" style="color: #34d399; font-weight: 600;">{{ line }}</span>
            <span v-else-if="line.includes('[ERROR]')" style="color: #f87171; font-weight: 600;">{{ line }}</span>
            <span v-else-if="line.includes('[WARN]')" style="color: #fbbf24;">{{ line }}</span>
            <span v-else style="color: #94a3b8;">{{ line }}</span>
          </div>

          <div v-if="installModal.status === 'running'" style="display: flex; align-items: center; gap: 0.5rem; color: #38bdf8; margin-top: 0.5rem;">
            <span style="display: inline-block; animation: pulse 1.5s infinite;">● 正在执行中，请勿关闭窗口...</span>
          </div>
        </div>

        <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
          <button
            v-if="installModal.status === 'running'"
            class="btn btn-secondary"
            disabled
            style="opacity: 0.6;"
          >
            正在安装，请稍候...
          </button>
          <button
            v-else
            @click="closeInstallModal"
            class="btn btn-primary"
          >
            完成并刷新
          </button>
        </div>
      </div>
    </div>

    <!-- 实时运行日志弹窗 (SSE journalctl) -->
    <div v-if="logModal.show" class="modal-overlay" @click.self="closeLogs">
      <div class="modal-content" style="max-width: 800px; height: 80vh; display: flex; flex-direction: column;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
          <h3 style="font-size: 1.125rem; font-weight: 600;">
            服务日志: <span style="color: #38bdf8;">{{ logModal.unit }}</span>
          </h3>
          <div style="display: flex; gap: 0.5rem;">
            <button @click="logModal.lines = []" class="btn btn-secondary" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;">
              清空
            </button>
            <button @click="closeLogs" class="btn btn-secondary" style="padding: 0.25rem 0.5rem; font-size: 0.75rem;">
              关闭
            </button>
          </div>
        </div>

        <div
          ref="logContainer"
          style="flex: 1; background-color: #0b0f19; border: 1px solid #1e293b; padding: 1rem; border-radius: 0.5rem; overflow-y: auto; font-family: monospace; font-size: 0.8rem; line-height: 1.4;"
        >
          <div v-for="(line, idx) in logModal.lines" :key="idx" style="white-space: pre-wrap; word-break: break-all; margin-bottom: 2px;">
            {{ line }}
          </div>
          <div v-if="logModal.lines.length === 0" style="color: #64748b; text-align: center; margin-top: 2rem;">
            正在连接日志流...
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, nextTick } from 'vue'
import api, { getSecretPath } from '../api'

interface ServiceStatus {
  name: string
  display_name: string
  description: string
  category: string
  installed: boolean
  installed_version: string
  latest_version: string
  has_update: boolean
  active: boolean
  running_status: string
}

const overview = ref<any>({
  public_ip: '',
  os: '',
  arch: '',
  num_cpu: 0,
  bbr: false,
  components: [] as ServiceStatus[],
  secret_path: '',
})

const categories = [
  { key: 'all', label: '全部服务' },
  { key: 'proxy', label: '代理核心' },
  { key: 'tunnel', label: '隧道穿透' },
]
const activeCategory = ref('all')
const checkingUpdates = ref(false)

const filteredComponents = computed(() => {
  const comps = overview.value.components || []
  if (activeCategory.value === 'all') return comps
  return comps.filter((c: ServiceStatus) => c.category === activeCategory.value)
})

// 安装/升级任务进度模态框
const installModal = reactive({
  show: false,
  title: '',
  name: '',
  status: 'idle', // 'running' | 'success' | 'failed'
  logs: [] as string[],
  eventSource: null as EventSource | null,
  pollTimer: null as any,
})
const installLogContainer = ref<HTMLElement | null>(null)

// 运行日志模态框
const logModal = reactive({
  show: false,
  unit: '',
  lines: [] as string[],
  eventSource: null as EventSource | null,
})
const logContainer = ref<HTMLElement | null>(null)

async function loadOverview(checkUpdates = false) {
  try {
    const res = await api.get('/system/overview')
    overview.value = res.data

    // 同时拉取最新组件信息
    const compRes = await api.get(`/components${checkUpdates ? '?check_updates=true' : ''}`)
    if (compRes.data) {
      overview.value.components = compRes.data
    }
  } catch (err) {
    console.error(err)
  }
}

async function checkForUpdates() {
  checkingUpdates.value = true
  try {
    await loadOverview(true)
  } finally {
    checkingUpdates.value = false
  }
}

async function enableBBR() {
  try {
    await api.post('/system/bbr')
    alert('BBR 拥塞控制算法已成功开启！')
    loadOverview()
  } catch (err: any) {
    alert('开启失败: ' + (err.response?.data?.error || err.message))
  }
}

async function startInstall(c: ServiceStatus) {
  const actionText = c.installed ? '升级/重装' : '安装'
  if (!confirm(`确定要从 GitHub ${actionText} ${c.display_name || c.name} 吗？`)) return

  installModal.name = c.name
  installModal.title = `${actionText} ${c.display_name || c.name}`
  installModal.logs = []
  installModal.status = 'running'
  installModal.show = true

  try {
    await api.post(`/components/${c.name}/install`, { version: 'latest' })
  } catch (err: any) {
    installModal.status = 'failed'
    installModal.logs.push(`[ERROR] 提交任务失败: ${err.response?.data?.error || err.message}`)
    return
  }

  // 建立 SSE 流接收安装日志
  const secret = getSecretPath()
  const token = localStorage.getItem('token') || ''
  const sseUrl = `/${secret}/api/components/${c.name}/install-logs?token=${token}`

  if (installModal.eventSource) {
    installModal.eventSource.close()
  }

  const es = new EventSource(sseUrl)
  es.addEventListener('log', (e: MessageEvent) => {
    installModal.logs.push(e.data)
    if (e.data.includes('[SUCCESS]')) {
      installModal.status = 'success'
    } else if (e.data.includes('[ERROR]')) {
      installModal.status = 'failed'
    }
    nextTick(() => {
      if (installLogContainer.value) {
        installLogContainer.value.scrollTop = installLogContainer.value.scrollHeight
      }
    })
  })

  es.onerror = () => {
    // 轮询辅助检查状态
    pollInstallStatus(c.name)
  }
  installModal.eventSource = es

  // 轮询辅助检测是否结束
  pollInstallStatus(c.name)
}

function pollInstallStatus(name: string) {
  if (installModal.pollTimer) clearInterval(installModal.pollTimer)

  installModal.pollTimer = setInterval(async () => {
    if (!installModal.show) {
      clearInterval(installModal.pollTimer)
      return
    }
    try {
      const res = await api.get(`/components/${name}/install-status`)
      if (res.data) {
        if (res.data.logs && res.data.logs.length > installModal.logs.length) {
          installModal.logs = res.data.logs
        }
        if (res.data.status === 'success') {
          installModal.status = 'success'
          clearInterval(installModal.pollTimer)
        } else if (res.data.status === 'failed') {
          installModal.status = 'failed'
          clearInterval(installModal.pollTimer)
        }
      }
    } catch {
      // ignore
    }
  }, 1500)
}

function handleInstallModalBackdrop() {
  if (installModal.status !== 'running') {
    closeInstallModal()
  }
}

function closeInstallModal() {
  if (installModal.eventSource) {
    installModal.eventSource.close()
    installModal.eventSource = null
  }
  if (installModal.pollTimer) {
    clearInterval(installModal.pollTimer)
    installModal.pollTimer = null
  }
  installModal.show = false
  loadOverview()
}

async function confirmUninstall(c: ServiceStatus) {
  const name = c.display_name || c.name
  if (!confirm(`警告：确定要卸载 ${name} 吗？这将会停止并注销其 Systemd 服务并删除可执行文件。`)) return

  try {
    await api.post(`/components/${c.name}/action`, { action: 'uninstall' })
    alert(`${name} 卸载成功！`)
    loadOverview()
  } catch (err: any) {
    alert(`卸载失败: ${err.response?.data?.error || err.message}`)
  }
}

async function componentAction(name: string, action: string) {
  try {
    await api.post(`/components/${name}/action`, { action })
    await loadOverview()
  } catch (err: any) {
    alert(`操作失败: ` + (err.response?.data?.error || err.message))
  }
}

function openLogs(unit: string) {
  logModal.unit = unit
  logModal.lines = []
  logModal.show = true

  const secret = getSecretPath()
  const token = localStorage.getItem('token') || ''
  const url = `/${secret}/api/system/logs?unit=${unit}&token=${token}`

  if (logModal.eventSource) {
    logModal.eventSource.close()
  }

  const es = new EventSource(url)
  es.onmessage = (e) => {
    logModal.lines.push(e.data)
    nextTick(() => {
      if (logContainer.value) {
        logContainer.value.scrollTop = logContainer.value.scrollHeight
      }
    })
  }
  es.onerror = () => {
    // 保持重连
  }
  logModal.eventSource = es
}

function closeLogs() {
  if (logModal.eventSource) {
    logModal.eventSource.close()
    logModal.eventSource = null
  }
  logModal.show = false
}

function getStatusBadge(status: string) {
  if (status === 'running') return 'badge-success'
  if (status === 'stopped') return 'badge-secondary'
  if (status === 'not_installed') return 'badge-warning'
  return 'badge-danger'
}

function getStatusText(status: string) {
  switch (status) {
    case 'running': return '运行中'
    case 'stopped': return '已停止'
    case 'not_installed': return '未安装'
    case 'failed': return '异常'
    default: return status
  }
}

onMounted(() => {
  loadOverview()
})
</script>

<style scoped>
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>
