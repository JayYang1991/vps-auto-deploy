<template>
  <div>
    <!-- 头部说明与操作栏 -->
    <div style="margin-bottom: 2rem; display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">📦</span>
          <h1 style="font-size: 1.5rem; font-weight: 700; color: #f8fafc;">软件与扩展中心</h1>
          <span class="badge badge-secondary" style="font-size: 0.75rem;">生命周期管理</span>
        </div>
        <p style="color: #94a3b8; font-size: 0.875rem; margin-top: 0.4rem; max-width: 650px; line-height: 1.5;">
          初始部署仅运行面板控制端。按需在此安装、更新与守护代理核心与网络隧道。
          如果宿主机此前已预装二进制文件，系统将自动识别其版本与运行状态。
        </p>
      </div>

      <div style="display: flex; gap: 0.75rem; align-items: center; flex-wrap: wrap;">
        <!-- 分类选择器 -->
        <div style="display: flex; background: #0f172a; padding: 0.25rem; border-radius: 0.5rem; border: 1px solid #334155;">
          <button
            v-for="cat in categories"
            :key="cat.key"
            @click="activeCategory = cat.key"
            :class="['btn', activeCategory === cat.key ? 'btn-primary' : 'btn-secondary']"
            style="padding: 0.35rem 0.85rem; font-size: 0.8rem; border: none;"
          >
            {{ cat.label }}
          </button>
        </div>

        <button @click="checkForUpdates" class="btn btn-secondary" :disabled="checkingUpdates">
          <span>🔄</span>
          <span>{{ checkingUpdates ? '正在查询发布...' : '检查版本更新' }}</span>
        </button>
      </div>
    </div>

    <!-- 组件卡片网格 -->
    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(360px, 1fr)); gap: 1.5rem; margin-bottom: 2.5rem;">
      <div
        v-for="c in filteredComponents"
        :key="c.name"
        :id="'component-' + c.name"
        class="card"
        :style="{
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          border: highlightedName === c.name ? '1px solid #38bdf8' : '1px solid #334155',
          boxShadow: highlightedName === c.name ? '0 0 15px rgba(56, 189, 248, 0.2)' : 'none',
          transition: 'all 0.3s ease'
        }"
      >
        <div>
          <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 0.75rem;">
            <div>
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <h3 style="font-size: 1.2rem; font-weight: 600; color: #f8fafc;">{{ c.display_name || c.name }}</h3>
                <span class="badge badge-secondary" style="font-size: 0.7rem;">
                  {{ getCategoryLabel(c.category) }}
                </span>
              </div>
              <p style="color: #94a3b8; font-size: 0.825rem; margin-top: 0.4rem; line-height: 1.45;">
                {{ c.description }}
              </p>
            </div>

            <span :class="['badge', getStatusBadge(c.running_status)]" style="white-space: nowrap; margin-left: 0.5rem;">
              {{ getStatusText(c.running_status) }}
            </span>
          </div>

          <!-- 版本对比信息框 -->
          <div style="background-color: #0f172a; padding: 0.85rem; border-radius: 0.5rem; font-size: 0.85rem; margin-bottom: 1.25rem; border: 1px solid #1e293b;">
            <div style="display: flex; justify-content: space-between; margin-bottom: 0.45rem;">
              <span style="color: #94a3b8;">当前本地版本:</span>
              <span :style="{ fontWeight: 600, color: c.installed ? '#34d399' : '#94a3b8' }">
                {{ c.installed_version || '未安装' }}
              </span>
            </div>
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="color: #94a3b8;">官方最新版本:</span>
              <div style="display: flex; align-items: center; gap: 0.5rem;">
                <span style="color: #38bdf8; font-weight: 500;">
                  {{ c.latest_version || '点击上方检查' }}
                </span>
                <span v-if="c.has_update" class="badge badge-warning" style="font-size: 0.65rem;">可升级</span>
              </div>
            </div>
          </div>
        </div>

        <div>
          <!-- 已安装服务控制栏 -->
          <div v-if="c.installed" style="margin-bottom: 0.5rem;">
            <div style="display: flex; gap: 0.5rem; margin-bottom: 0.5rem;">
              <button
                @click="componentAction(c.name, c.active ? 'stop' : 'start')"
                :class="['btn', c.active ? 'btn-danger' : 'btn-success']"
                style="flex: 1; padding: 0.45rem 0.6rem; font-size: 0.825rem;"
              >
                {{ c.active ? '⏹ 停止' : '▶ 启动' }}
              </button>
              <button
                @click="componentAction(c.name, 'restart')"
                class="btn btn-secondary"
                style="flex: 1; padding: 0.45rem 0.6rem; font-size: 0.825rem;"
              >
                🔄 重启
              </button>
              <button
                @click="openLogs(c.name)"
                class="btn btn-secondary"
                style="padding: 0.45rem 0.6rem; font-size: 0.825rem;"
                title="查看运行日志"
              >
                📜 日志
              </button>
              <router-link
                v-if="getConfigRoute(c.name)"
                :to="getConfigRoute(c.name)"
                class="btn btn-secondary"
                style="padding: 0.45rem 0.6rem; font-size: 0.825rem; text-decoration: none;"
                title="前往业务配置"
              >
                🛠️ 配置
              </router-link>
            </div>

            <div style="display: flex; gap: 0.5rem;">
              <button
                @click="startInstall(c)"
                :class="['btn', c.has_update ? 'btn-primary' : 'btn-secondary']"
                style="flex: 2; padding: 0.45rem 0.6rem; font-size: 0.825rem;"
              >
                {{ c.has_update ? '🚀 升级至 ' + c.latest_version : '🔄 重新安装' }}
              </button>
              <button
                @click="confirmUninstall(c)"
                class="btn btn-danger"
                style="flex: 1; padding: 0.45rem 0.6rem; font-size: 0.825rem; background-color: transparent; border-color: #ef4444;"
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
              style="width: 100%; font-weight: 600; padding: 0.55rem 1rem;"
            >
              📥 一键安装 {{ c.display_name }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 实时安装/升级终端进度抽屉/弹窗 -->
    <div v-if="installModal.show" class="modal-overlay" @click.self="handleInstallModalBackdrop">
      <div class="modal-content" style="max-width: 720px; display: flex; flex-direction: column;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
          <div style="display: flex; align-items: center; gap: 0.6rem;">
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
            ✕ 关闭
          </button>
        </div>

        <!-- 终端日志黑框 -->
        <div
          ref="installLogContainer"
          style="background-color: #0b0f19; border: 1px solid #1e293b; padding: 1rem; border-radius: 0.5rem; height: 360px; overflow-y: auto; font-family: 'JetBrains Mono', monospace; font-size: 0.8rem; line-height: 1.5; color: #cbd5e1;"
        >
          <div v-for="(line, idx) in installModal.logs" :key="idx" style="margin-bottom: 3px; word-break: break-all;">
            <span v-if="line.includes('[SUCCESS]')" style="color: #34d399; font-weight: 600;">{{ line }}</span>
            <span v-else-if="line.includes('[ERROR]')" style="color: #f87171; font-weight: 600;">{{ line }}</span>
            <span v-else-if="line.includes('[WARN]')" style="color: #fbbf24;">{{ line }}</span>
            <span v-else style="color: #94a3b8;">{{ line }}</span>
          </div>

          <div v-if="installModal.status === 'running'" style="display: flex; align-items: center; gap: 0.5rem; color: #38bdf8; margin-top: 0.5rem;">
            <span style="display: inline-block; animation: pulse 1.5s infinite;">● 任务正在执行中，请稍候...</span>
          </div>
        </div>

        <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
          <button
            v-if="installModal.status === 'running'"
            class="btn btn-secondary"
            disabled
            style="opacity: 0.6;"
          >
            后台安装中...
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
import { useRoute } from 'vue-router'
import api, { getSecretPath } from '../api'
import { useToast } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

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

const route = useRoute()
const { toast } = useToast()
const { showConfirm } = useConfirm()

const components = ref<ServiceStatus[]>([])
const categories = [
  { key: 'all', label: '全部服务' },
  { key: 'proxy', label: '代理核心' },
  { key: 'tunnel', label: '隧道穿透' },
  { key: 'converter', label: '订阅与规则' },
]
const activeCategory = ref('all')
const checkingUpdates = ref(false)
const highlightedName = ref('')

const filteredComponents = computed(() => {
  const comps = components.value || []
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

async function loadComponents(checkUpdates = false) {
  try {
    const res = await api.get(`/components${checkUpdates ? '?check_updates=true' : ''}`)
    if (res.data) {
      components.value = res.data
    }
  } catch (err: any) {
    toast.error('加载组件列表失败: ' + (err.response?.data?.error || err.message))
  }
}

async function checkForUpdates() {
  checkingUpdates.value = true
  try {
    await loadComponents(true)
    toast.success('已拉取官方最新版本信息！')
  } catch (err: any) {
    toast.error('检查更新失败: ' + (err.response?.data?.error || err.message))
  } finally {
    checkingUpdates.value = false
  }
}

async function startInstall(c: ServiceStatus) {
  const actionText = c.installed ? '升级/重装' : '安装'
  const confirmed = await showConfirm({
    title: `确认${actionText}`,
    message: `确定要从官方源 ${actionText} ${c.display_name || c.name} 吗？系统将在后台下载并配置服务。`,
    confirmText: `开始${actionText}`,
  })
  if (!confirmed) return

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
    toast.error('提交安装任务失败')
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
      toast.success(`${c.display_name || c.name} 安装成功！`)
    } else if (e.data.includes('[ERROR]')) {
      installModal.status = 'failed'
      toast.error(`${c.display_name || c.name} 安装过程中遇到错误`)
    }
    nextTick(() => {
      if (installLogContainer.value) {
        installLogContainer.value.scrollTop = installLogContainer.value.scrollHeight
      }
    })
  })

  es.onerror = () => {
    pollInstallStatus(c.name)
  }
  installModal.eventSource = es

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
  loadComponents()
}

async function confirmUninstall(c: ServiceStatus) {
  const name = c.display_name || c.name
  const confirmed = await showConfirm({
    title: `危险操作：卸载 ${name}`,
    message: `确定要卸载 ${name} 吗？这将会停止并注销其 Systemd 服务守护，并从磁盘删除可执行文件。`,
    confirmText: '确认卸载',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.post(`/components/${c.name}/action`, { action: 'uninstall' })
    toast.success(`${name} 卸载完成`)
    loadComponents()
  } catch (err: any) {
    toast.error(`卸载失败: ${err.response?.data?.error || err.message}`)
  }
}

async function componentAction(name: string, action: string) {
  try {
    await api.post(`/components/${name}/action`, { action })
    const actionLabel = action === 'start' ? '启动' : action === 'stop' ? '停止' : '重启'
    toast.success(`已下发 ${actionLabel} 指令`)
    await loadComponents()
  } catch (err: any) {
    toast.error(`操作失败: ` + (err.response?.data?.error || err.message))
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

function getCategoryLabel(cat: string) {
  const map: Record<string, string> = {
    proxy: '代理核心',
    tunnel: '隧道穿透',
    converter: '订阅与规则',
    network: '网络工具',
  }
  return map[cat] || cat
}

function getConfigRoute(name: string) {
  if (name === 'sing-box') return '/services?tab=singbox&sub=inbounds'
  if (name === 'cloudflared') return '/services?tab=cloudflared'
  if (name === 'subconverter') return '/services?tab=subscriptions'
  return ''
}

function getStatusBadge(status: string) {
  switch (status) {
    case 'running':
    case 'active':
      return 'badge-success'
    case 'activating':
    case 'reloading':
      return 'badge-info'
    case 'deactivating':
      return 'badge-warning'
    case 'stopped':
    case 'inactive':
      return 'badge-secondary'
    case 'not_installed':
      return 'badge-warning'
    case 'failed':
      return 'badge-danger'
    default:
      return 'badge-secondary'
  }
}

function getStatusText(status: string) {
  switch (status) {
    case 'running':
    case 'active':
      return '已激活 (运行中)'
    case 'activating':
      return '激活中 (启动中)'
    case 'reloading':
      return '重载中'
    case 'deactivating':
      return '停止中'
    case 'stopped':
    case 'inactive':
      return '未激活 (已停止)'
    case 'not_installed':
      return '未安装'
    case 'failed':
      return '异常 (失败)'
    default:
      return status || '未激活'
  }
}

onMounted(async () => {
  await loadComponents()

  // 检查是否从外部路由带有 ?install=xxx
  const target = route.query.install as string
  if (target) {
    highlightedName.value = target
    const targetComp = components.value.find((c) => c.name === target)
    if (targetComp && !targetComp.installed) {
      startInstall(targetComp)
    }
  }
})
</script>

<style scoped>
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.4; }
}
</style>

