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

    <!-- 组件管理商店 -->
    <div style="margin-bottom: 1.5rem; display: flex; justify-content: space-between; align-items: center;">
      <h2 style="font-size: 1.25rem; font-weight: 600;">核心服务组件</h2>
      <button @click="loadOverview" class="btn btn-secondary">刷新状态</button>
    </div>

    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(340px, 1fr)); gap: 1.5rem;">
      <div v-for="c in overview.components" :key="c.name" class="card" style="display: flex; flex-direction: column; justify-content: space-between;">
        <div>
          <div style="display: flex; justify-content: space-between; align-items: flex-start; margin-bottom: 1rem;">
            <div>
              <h3 style="font-size: 1.125rem; font-weight: 600;">{{ c.name }}</h3>
              <p style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.2rem;">
                {{ c.name === 'sing-box' ? '下一代通用代理内核 (VLESS/Reality/WS)' : 'Cloudflare Zero Trust / Argo 隧道客户端' }}
              </p>
            </div>
            <span :class="['badge', getStatusBadge(c.running_status)]">
              {{ c.running_status }}
            </span>
          </div>

          <div style="background-color: #0f172a; padding: 0.75rem; border-radius: 0.5rem; font-size: 0.85rem; margin-bottom: 1rem;">
            <div style="display: flex; justify-content: space-between; margin-bottom: 0.35rem;">
              <span style="color: #94a3b8;">已安装版本:</span>
              <span style="font-weight: 500;">{{ c.installed_version || '未安装' }}</span>
            </div>
            <div style="display: flex; justify-content: space-between;">
              <span style="color: #94a3b8;">官方最新版:</span>
              <span style="color: #38bdf8; font-weight: 500;">{{ c.latest_version || '点击下方获取/安装' }}</span>
            </div>
          </div>
        </div>

        <div>
          <!-- 启停控制 -->
          <div style="display: flex; gap: 0.5rem; margin-bottom: 0.75rem;">
            <button
              v-if="c.installed"
              @click="componentAction(c.name, c.active ? 'stop' : 'start')"
              :class="['btn', c.active ? 'btn-danger' : 'btn-success']"
              style="flex: 1;"
            >
              {{ c.active ? '停止服务' : '启动服务' }}
            </button>
            <button
              v-if="c.installed"
              @click="componentAction(c.name, 'restart')"
              class="btn btn-secondary"
              style="flex: 1;"
            >
              重启
            </button>
            <button
              @click="openLogs(c.name)"
              class="btn btn-secondary"
              style="padding: 0.5rem 0.75rem;"
              title="查看实时运行日志"
            >
              📜 日志
            </button>
          </div>

          <!-- 安装/更新按钮 -->
          <button
            @click="installComponent(c.name)"
            class="btn btn-primary"
            style="width: 100%;"
            :disabled="installingMap[c.name]"
          >
            {{ installingMap[c.name] ? '正在后台下载安装最新版本...' : (c.installed ? '检查并升级至最新版' : '一键安装最新版本') }}
          </button>
        </div>
      </div>
    </div>

    <!-- 实时日志弹窗 (SSE) -->
    <div v-if="logModal.show" class="modal-overlay" @click.self="closeLogs">
      <div class="modal-content" style="max-width: 800px; height: 80vh; display: flex; flex-direction: column;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
          <h3 style="font-size: 1.125rem; font-weight: 600;">
            实时日志: <span style="color: #38bdf8;">{{ logModal.unit }}</span>
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
          style="flex: 1; background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; overflow-y: auto; font-family: monospace; font-size: 0.8rem; line-height: 1.4;"
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
import { ref, reactive, onMounted, nextTick } from 'vue'
import api, { getSecretPath } from '../api'

interface ServiceStatus {
  name: string
  installed: boolean
  installed_version: string
  latest_version: string
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

const installingMap = reactive<Record<string, boolean>>({})

const logModal = reactive({
  show: false,
  unit: '',
  lines: [] as string[],
  eventSource: null as EventSource | null,
})
const logContainer = ref<HTMLElement | null>(null)

async function loadOverview() {
  try {
    const res = await api.get('/system/overview')
    overview.value = res.data
  } catch (err) {
    console.error(err)
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

async function installComponent(name: string) {
  if (!confirm(`确定要从 GitHub 安装/升级 ${name} 到最新版本吗？`)) return
  installingMap[name] = true
  try {
    await api.post(`/components/${name}/install`)
    alert(`已提交 ${name} 安装任务，系统正在后台下载并配置，请稍后刷新查看。`)
    setTimeout(loadOverview, 5000)
  } catch (err: any) {
    alert('安装请求失败: ' + (err.response?.data?.error || err.message))
  } finally {
    installingMap[name] = false
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
    // 自动重连或保持
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

onMounted(() => {
  loadOverview()
})
</script>

