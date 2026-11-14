<template>
  <div>
    <!-- 业务配置中心标题说明 -->
    <div style="margin-bottom: 1.5rem; display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">🛠️</span>
          <h1 style="font-size: 1.45rem; font-weight: 700; color: #f8fafc;">业务与服务配置中心</h1>
          <span class="badge badge-secondary" style="font-size: 0.75rem;">按组件配置</span>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.35rem; line-height: 1.5;">
          按照底层受管组件分类配置：包括 Sing-box 核心的出入站与路由、Cloudflared 反代隧道穿透、以及客户端多协议订阅分发。
        </p>
      </div>
    </div>

    <!-- 一级分类：受管组件导航栏 (Component Selector) -->
    <div class="tabs-container">
      <div class="segmented-bar">
        <button
          v-for="comp in componentTabs"
          :key="comp.key"
          @click="selectComponent(comp.key)"
          :class="['tab-btn', activeComponent === comp.key ? 'tab-btn-active' : '']"
          type="button"
        >
          <span class="tab-icon">{{ comp.icon }}</span>
          <span class="tab-title">{{ comp.title }}</span>
          <span class="tab-desc">{{ comp.subLabel }}</span>
        </button>
      </div>
    </div>

    <!-- Sing-box 组件专属配置区 (含二级出入站导航与全局操作) -->
    <div v-if="activeComponent === 'singbox'" class="component-workspace">
      <!-- Sing-box 二级导航与操作工具栏 -->
      <div class="sub-nav-bar">
        <div class="sub-segmented-bar">
          <button
            @click="selectSingboxSubTab('inbounds')"
            :class="['sub-tab-btn', singboxSubTab === 'inbounds' ? 'sub-tab-active' : '']"
            type="button"
          >
            <span>🌐</span>
            <span>入站节点 (Inbounds)</span>
          </button>
          <button
            @click="selectSingboxSubTab('outbounds')"
            :class="['sub-tab-btn', singboxSubTab === 'outbounds' ? 'sub-tab-active' : '']"
            type="button"
          >
            <span>📤</span>
            <span>出站规则 (Outbounds)</span>
          </button>
          <button
            @click="selectSingboxSubTab('routes')"
            :class="['sub-tab-btn', singboxSubTab === 'routes' ? 'sub-tab-active' : '']"
            type="button"
          >
            <span>🔀</span>
            <span>路由分流 (Routes)</span>
          </button>
        </div>

        <!-- Sing-box 专属操作：配置透视与热重载 -->
        <div style="display: flex; gap: 0.6rem; align-items: center; flex-wrap: wrap;">
          <button @click="openConfigViewer" class="btn btn-secondary" style="font-size: 0.825rem; padding: 0.4rem 0.8rem;" title="预览与语法预检当前 Sing-box 完整配置，支持查看磁盘与面板配置差异">
            <span>🔍</span>
            <span>配置透视与对比</span>
          </button>
          <button @click="loadFromDisk" class="btn btn-secondary" style="font-size: 0.825rem; padding: 0.4rem 0.8rem;" :disabled="syncingFromDisk" title="读取宿主机磁盘上的 /etc/sing-box/config.json 反向同步覆盖面板数据库（包含入站与出站）">
            <span>📥</span>
            <span>{{ syncingFromDisk ? '同步中...' : '从磁盘同步到面板' }}</span>
          </button>
          <button @click="reloadConfig" class="btn btn-secondary" style="font-size: 0.825rem; padding: 0.4rem 0.8rem;" :disabled="reloading" title="以面板数据库为准重新渲染配置落盘并热重载内核">
            <span>⚡</span>
            <span>{{ reloading ? '重载中...' : '强制热重载' }}</span>
          </button>
        </div>
      </div>

      <!-- 二级子视图工作区 -->
      <div class="tab-content-area" :key="subKey">
        <keep-alive>
          <Inbounds v-if="singboxSubTab === 'inbounds'" />
          <Outbounds v-else-if="singboxSubTab === 'outbounds'" />
          <Routes v-else-if="singboxSubTab === 'routes'" />
        </keep-alive>
      </div>
    </div>

    <!-- Cloudflared 组件专属配置区 -->
    <div v-else-if="activeComponent === 'cloudflared'" class="component-workspace">
      <div class="tab-content-area">
        <keep-alive>
          <Tunnels />
        </keep-alive>
      </div>
    </div>

    <!-- 成员与订阅规则组件专属配置区 -->
    <div v-else-if="activeComponent === 'subscriptions'" class="component-workspace">
      <div class="tab-content-area" :key="subKey">
        <keep-alive>
          <Subscriptions />
        </keep-alive>
      </div>
    </div>

    <!-- Sing-box 服务端完整配置透视弹窗 -->
    <div v-if="configViewer.show" class="modal-overlay">
      <div class="modal-content" style="max-width: 840px; max-height: 88vh; display: flex; flex-direction: column;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem; flex-wrap: wrap; gap: 0.5rem;">
          <div>
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="font-size: 1.25rem;">📜</span>
              <h3 style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">Sing-box 服务端配置透视</h3>
              <span :class="['badge', configViewer.check_ok ? 'badge-success' : 'badge-danger']">
                {{ configViewer.check_ok ? '✓ 语法校验通过' : '❌ 校验异常' }}
              </span>
            </div>
            <div style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem; font-family: monospace;">
              配置路径: {{ configViewer.config_path }}
            </div>
          </div>

          <div style="display: flex; gap: 0.5rem; flex-wrap: wrap;">
            <button @click="loadFromDisk" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.35rem 0.75rem;" :disabled="syncingFromDisk" title="读取磁盘文件反向同步覆盖面板数据库">
              📥 从磁盘同步到面板
            </button>
            <button @click="copyConfig" class="btn btn-primary" style="font-size: 0.8rem; padding: 0.35rem 0.75rem;">
              📋 复制当前配置
            </button>
            <button @click="configViewer.show = false" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.35rem 0.75rem;">
              ✕ 关闭
            </button>
          </div>
        </div>

        <!-- 视图切换：磁盘真实文件 vs 面板动态构建 -->
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.75rem; background: #0b0f19; padding: 0.4rem 0.75rem; border-radius: 0.4rem; border: 1px solid #1e293b; flex-wrap: wrap; gap: 0.5rem;">
          <div style="display: flex; gap: 0.4rem; align-items: center;">
            <span style="font-size: 0.75rem; color: #94a3b8;">透视视角:</span>
            <button
              @click="configViewer.viewMode = 'disk'"
              :class="['btn', configViewer.viewMode === 'disk' ? 'btn-primary' : 'btn-secondary']"
              style="font-size: 0.75rem; padding: 0.2rem 0.6rem;"
              type="button"
            >
              📄 磁盘文件内容 {{ configViewer.file_exists ? '(已落盘)' : '(未落盘)' }}
            </button>
            <button
              @click="configViewer.viewMode = 'generated'"
              :class="['btn', configViewer.viewMode === 'generated' ? 'btn-primary' : 'btn-secondary']"
              style="font-size: 0.75rem; padding: 0.2rem 0.6rem;"
              type="button"
            >
              ⚡ 面板数据库生成
            </button>
          </div>
          <div v-if="configViewer.file_exists && (configViewer.is_synced || isConfigContentEqual(configViewer.disk_raw_json, configViewer.raw_json))" style="display: flex; align-items: center; gap: 0.5rem;">
            <span class="badge badge-success" style="font-size: 0.7rem;">
              ✓ 磁盘与面板数据 100% 同步
            </span>
          </div>
          <div v-else-if="configViewer.file_exists" style="display: flex; align-items: center; gap: 0.5rem; flex-wrap: wrap;">
            <span class="badge badge-warning" style="font-size: 0.7rem;">
              ⚠️ 磁盘配置与面板存在差异
            </span>
            <button
              @click="loadFromDisk"
              class="btn btn-primary"
              style="font-size: 0.72rem; padding: 0.2rem 0.6rem; background: #d97706; border-color: #f59e0b;"
              :disabled="syncingFromDisk"
              type="button"
            >
              {{ syncingFromDisk ? '⏳ 导入并同步中...' : '📥 以磁盘为准导入面板' }}
            </button>
            <button
              @click="reloadConfig"
              class="btn btn-secondary"
              style="font-size: 0.72rem; padding: 0.2rem 0.6rem;"
              :disabled="reloading"
              type="button"
            >
              {{ reloading ? '⏳ 写入中...' : '⚡ 以面板为准写入磁盘' }}
            </button>
          </div>
        </div>

        <div v-if="!configViewer.check_ok" style="background: rgba(239, 68, 68, 0.15); border: 1px solid rgba(239, 68, 68, 0.3); padding: 0.6rem 0.85rem; border-radius: 0.5rem; margin-bottom: 0.75rem; color: #fca5a5; font-size: 0.8rem;">
          {{ configViewer.check_output }}
        </div>

        <!-- 格式化代码阅读器 -->
        <pre
          style="flex: 1; background-color: #0b0f19; border: 1px solid #1e293b; padding: 1rem; border-radius: 0.5rem; overflow-y: auto; font-family: 'JetBrains Mono', monospace; font-size: 0.825rem; line-height: 1.45; color: #cbd5e1;"
        >{{ configViewer.viewMode === 'disk' ? (configViewer.disk_raw_json || '磁盘上配置文件暂无内容') : configViewer.raw_json }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Inbounds from './Inbounds.vue'
import Outbounds from './Outbounds.vue'
import Routes from './Routes.vue'
import Tunnels from './Tunnels.vue'
import Subscriptions from './Subscriptions.vue'
import api from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

const route = useRoute()
const router = useRouter()
const { toast } = useToast()
const { showConfirm } = useConfirm()

const componentTabs = [
  {
    key: 'singbox',
    title: 'Sing-box 核心',
    subLabel: '入站 / 出站 / 路由',
    icon: '📦',
  },
  {
    key: 'cloudflared',
    title: 'Cloudflared 隧道',
    subLabel: 'Argo 穿透映射',
    icon: '🚇',
  },
  {
    key: 'subscriptions',
    title: '订阅与规则分发',
    subLabel: '多协议与规则模板',
    icon: '👥',
  },
]

const activeComponent = ref('singbox')
const singboxSubTab = ref<'inbounds' | 'outbounds' | 'routes'>('inbounds')
const reloading = ref(false)
const syncingFromDisk = ref(false)
const subKey = ref(0)

const configViewer = reactive({
  show: false,
  raw_json: '',
  disk_raw_json: '',
  config_path: '',
  check_ok: true,
  check_output: '',
  file_exists: false,
  is_synced: false,
  viewMode: 'disk' as 'disk' | 'generated',
})

function isConfigContentEqual(a: string, b: string): boolean {
  if (!a || !b) return false
  if (a.trim() === b.trim()) return true
  try {
    const objA = JSON.parse(a)
    const objB = JSON.parse(b)
    return JSON.stringify(objA) === JSON.stringify(objB)
  } catch {
    return false
  }
}

function selectComponent(key: string) {
  activeComponent.value = key
  syncRoute()
}

function selectSingboxSubTab(sub: 'inbounds' | 'outbounds' | 'routes') {
  singboxSubTab.value = sub
  syncRoute()
}

function syncRoute() {
  const query: Record<string, string> = { tab: activeComponent.value }
  if (activeComponent.value === 'singbox') {
    query.sub = singboxSubTab.value
  } else if (activeComponent.value === 'subscriptions') {
    const sub = (route.query.sub as string) || localStorage.getItem('subscriptions_active_sub_section') || 'members'
    query.sub = sub
  }
  router.replace({ path: '/services', query })
}

async function openConfigViewer() {
  try {
    const res = await api.get('/singbox/config')
    configViewer.raw_json = res.data.raw_json || ''
    configViewer.disk_raw_json = res.data.disk_raw_json || ''
    configViewer.config_path = res.data.config_path || ''
    configViewer.check_ok = res.data.check_ok
    configViewer.check_output = res.data.check_output || ''
    configViewer.file_exists = res.data.file_exists
    configViewer.is_synced = res.data.is_synced ?? false
    // 若磁盘文件存在且有内容，优先默认展示真实磁盘文件
    if (res.data.file_exists && res.data.disk_raw_json) {
      configViewer.viewMode = 'disk'
    } else {
      configViewer.viewMode = 'generated'
    }
    configViewer.show = true
  } catch (err: any) {
    toast.error('获取完整配置失败: ' + (err.response?.data?.error || err.message))
  }
}

async function copyConfig() {
  const content = configViewer.viewMode === 'disk' ? (configViewer.disk_raw_json || configViewer.raw_json) : configViewer.raw_json
  await copyToClipboard(content, '完整配置 JSON')
}

async function reloadConfig() {
  reloading.value = true
  try {
    const res = await api.post('/singbox/reload')
    toast.success(res.data.message || 'sing-box 服务已重载最新配置！')
    if (configViewer.show) {
      await openConfigViewer()
    }
  } catch (err: any) {
    toast.error('重载配置失败: ' + (err.response?.data?.error || err.message))
  } finally {
    reloading.value = false
  }
}

async function loadFromDisk() {
  const ok = await showConfirm({
    title: '确认从磁盘反向同步并覆盖面板数据库？',
    message: '此操作将读取当前宿主机磁盘上的配置文件，反向提取并覆盖面板数据库中的入站节点、出站规则与路由分流，并自动完成双向规范化回盘与对齐。是否继续？',
    confirmText: '确认从磁盘同步',
    danger: true,
  })
  if (!ok) return

  syncingFromDisk.value = true
  try {
    const res = await api.post('/singbox/load-from-disk')
    toast.success(res.data.message || '已成功从磁盘加载并同步到面板数据库！')
    if (configViewer.show) {
      await openConfigViewer()
    }
    // 强制刷新子组件数据
    subKey.value++
    window.dispatchEvent(new CustomEvent('node-context-changed'))
  } catch (err: any) {
    toast.error('从磁盘同步失败: ' + (err.response?.data?.error || err.message))
  } finally {
    syncingFromDisk.value = false
  }
}

function parseRouteParams() {
  const tab = (route.query.tab as string) || ''
  const sub = (route.query.sub as string) || ''

  // 兼容历史快捷跳转：/services?tab=inbounds, /services?tab=outbounds, /services?tab=routes, /services?tab=tunnels
  if (tab === 'inbounds') {
    activeComponent.value = 'singbox'
    singboxSubTab.value = 'inbounds'
    return
  }
  if (tab === 'outbounds') {
    activeComponent.value = 'singbox'
    singboxSubTab.value = 'outbounds'
    return
  }
  if (tab === 'routes') {
    activeComponent.value = 'singbox'
    singboxSubTab.value = 'routes'
    return
  }
  if (tab === 'tunnels') {
    activeComponent.value = 'cloudflared'
    return
  }
  if (tab === 'subconverter' || tab === 'cf_preferred' || tab === 'cfip' || tab === 'inventory' || tab === 'nodelist') {
    activeComponent.value = 'subscriptions'
    const targetSub = (tab === 'cfip' ? 'cf_preferred' : (tab === 'nodelist' ? 'inventory' : tab))
    localStorage.setItem('subscriptions_active_sub_section', targetSub)
    return
  }

  if (componentTabs.some((c) => c.key === tab)) {
    activeComponent.value = tab
  } else {
    activeComponent.value = 'singbox'
  }

  if (sub === 'outbounds' || sub === 'inbounds' || sub === 'routes') {
    singboxSubTab.value = sub
  }
}

watch(
  () => [route.query.tab, route.query.sub],
  () => {
    parseRouteParams()
  },
  { immediate: true }
)

onMounted(() => {
  parseRouteParams()
})
</script>

<style scoped>
.tabs-container {
  margin-bottom: 1.25rem;
  border-bottom: 1px solid #334155;
  padding-bottom: 0.75rem;
}

.segmented-bar {
  display: inline-flex;
  background-color: #0f172a;
  padding: 0.3rem;
  border-radius: 0.65rem;
  border: 1px solid #334155;
  gap: 0.35rem;
  flex-wrap: wrap;
}

.tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.5rem 1rem;
  border-radius: 0.5rem;
  border: 1px solid transparent;
  background: transparent;
  color: #94a3b8;
  font-size: 0.875rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.tab-btn:hover {
  color: #f1f5f9;
  background-color: #1e293b;
}

.tab-btn-active {
  background-color: #1e293b;
  color: #38bdf8;
  border-color: #38bdf8;
  font-weight: 600;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.25);
}

.tab-icon {
  font-size: 1.05rem;
}

.tab-title {
  font-weight: 600;
}

.tab-desc {
  font-size: 0.725rem;
  color: #64748b;
  margin-left: 0.25rem;
  padding: 0.1rem 0.35rem;
  background-color: #0b0f19;
  border-radius: 4px;
}

.tab-btn-active .tab-desc {
  color: #7dd3fc;
  background-color: rgba(56, 189, 248, 0.12);
}

/* 二级子导航栏 */
.sub-nav-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1.25rem;
  padding: 0.65rem 0.85rem;
  background-color: rgba(15, 23, 42, 0.65);
  border: 1px solid #1e293b;
  border-radius: 0.65rem;
  flex-wrap: wrap;
  gap: 0.75rem;
}

.sub-segmented-bar {
  display: inline-flex;
  background-color: #0b0f19;
  padding: 0.25rem;
  border-radius: 0.5rem;
  border: 1px solid #334155;
  gap: 0.25rem;
}

.sub-tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.35rem 0.85rem;
  border-radius: 0.4rem;
  border: 1px solid transparent;
  background: transparent;
  color: #94a3b8;
  font-size: 0.825rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.sub-tab-btn:hover {
  color: #f1f5f9;
  background-color: #1e293b;
}

.sub-tab-active {
  background-color: #1e293b;
  color: #38bdf8;
  border-color: #38bdf8;
  font-weight: 600;
}

.component-workspace {
  animation: fade-in 0.2s ease-in-out;
}

.tab-content-area {
  animation: fade-in 0.2s ease-in-out;
}

@keyframes fade-in {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
