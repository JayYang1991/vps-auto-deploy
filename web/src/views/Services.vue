<template>
  <div>
    <!-- 业务配置中心标题说明与全局诊断操作栏 -->
    <div style="margin-bottom: 1.5rem; display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">🛠️</span>
          <h1 style="font-size: 1.45rem; font-weight: 700; color: #f8fafc;">业务与服务配置中心</h1>
          <span class="badge badge-secondary" style="font-size: 0.75rem;">网络与规则</span>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.35rem; line-height: 1.5;">
          集中配置与管理入站协议节点、出站网络出口、公网反代隧道穿透及多协议客户端订阅分发。
        </p>
      </div>

      <!-- 全局配置诊断与查看 -->
      <div style="display: flex; gap: 0.6rem; align-items: center; flex-wrap: wrap;">
        <button @click="openConfigViewer" class="btn btn-secondary" style="font-size: 0.825rem;">
          <span>📜</span>
          <span>查看当前完整配置</span>
        </button>
        <button @click="reloadConfig" class="btn btn-secondary" style="font-size: 0.825rem;" :disabled="reloading">
          <span>⚡</span>
          <span>{{ reloading ? '重载中...' : '强制热重载' }}</span>
        </button>
      </div>
    </div>

    <!-- 分类工作区分段导航器 (Segmented Tabs) -->
    <div class="tabs-container">
      <div class="segmented-bar">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          @click="selectTab(tab.key)"
          :class="['tab-btn', activeTab === tab.key ? 'tab-btn-active' : '']"
          type="button"
        >
          <span class="tab-icon">{{ tab.icon }}</span>
          <span class="tab-title">{{ tab.title }}</span>
          <span class="tab-desc">{{ tab.subLabel }}</span>
        </button>
      </div>
    </div>

    <!-- 动态业务配置工作区 -->
    <div class="tab-content-area">
      <keep-alive>
        <component :is="currentTabComponent" />
      </keep-alive>
    </div>

    <!-- Sing-box 服务端完整配置透视弹窗 -->
    <div v-if="configViewer.show" class="modal-overlay" @click.self="configViewer.show = false">
      <div class="modal-content" style="max-width: 840px; max-height: 88vh; display: flex; flex-direction: column;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem; flex-wrap: wrap; gap: 0.5rem;">
          <div>
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="font-size: 1.25rem;">📜</span>
              <h3 style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">Sing-box 服务端完整配置透视</h3>
              <span :class="['badge', configViewer.check_ok ? 'badge-success' : 'badge-danger']">
                {{ configViewer.check_ok ? '✓ 语法校验通过' : '❌ 校验异常' }}
              </span>
            </div>
            <div style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem; font-family: monospace;">
              落盘路径: {{ configViewer.config_path }}
            </div>
          </div>

          <div style="display: flex; gap: 0.5rem;">
            <button @click="copyConfig" class="btn btn-primary" style="font-size: 0.8rem; padding: 0.35rem 0.75rem;">
              📋 复制全部配置
            </button>
            <button @click="configViewer.show = false" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.35rem 0.75rem;">
              ✕ 关闭
            </button>
          </div>
        </div>

        <div v-if="!configViewer.check_ok" style="background: rgba(239, 68, 68, 0.15); border: 1px solid rgba(239, 68, 68, 0.3); padding: 0.6rem 0.85rem; border-radius: 0.5rem; margin-bottom: 0.75rem; color: #fca5a5; font-size: 0.8rem;">
          {{ configViewer.check_output }}
        </div>

        <!-- 格式化代码阅读器 -->
        <pre
          style="flex: 1; background-color: #0b0f19; border: 1px solid #1e293b; padding: 1rem; border-radius: 0.5rem; overflow-y: auto; font-family: 'JetBrains Mono', monospace; font-size: 0.825rem; line-height: 1.45; color: #cbd5e1;"
        >{{ configViewer.raw_json }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Inbounds from './Inbounds.vue'
import Outbounds from './Outbounds.vue'
import Tunnels from './Tunnels.vue'
import Subscriptions from './Subscriptions.vue'
import api from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'

const route = useRoute()
const router = useRouter()
const { toast } = useToast()

const tabs = [
  {
    key: 'inbounds',
    title: '入站节点',
    subLabel: 'Reality / VLESS',
    icon: '🌐',
    component: Inbounds,
  },
  {
    key: 'outbounds',
    title: '出站规则',
    subLabel: 'Direct / SOCKS / WARP',
    icon: '📤',
    component: Outbounds,
  },
  {
    key: 'tunnels',
    title: '隧道穿透',
    subLabel: 'Cloudflare Argo',
    icon: '🚇',
    component: Tunnels,
  },
  {
    key: 'subscriptions',
    title: '成员与订阅',
    subLabel: 'Clash / Sing-box',
    icon: '👥',
    component: Subscriptions,
  },
]

const activeTab = ref('inbounds')
const reloading = ref(false)

const configViewer = reactive({
  show: false,
  raw_json: '',
  config_path: '',
  check_ok: true,
  check_output: '',
})

const currentTabComponent = computed(() => {
  const found = tabs.find((t) => t.key === activeTab.value)
  return found ? found.component : Inbounds
})

function selectTab(key: string) {
  activeTab.value = key
  router.replace({ path: '/services', query: { tab: key } })
}

async function openConfigViewer() {
  try {
    const res = await api.get('/singbox/config')
    configViewer.raw_json = res.data.raw_json || ''
    configViewer.config_path = res.data.config_path || ''
    configViewer.check_ok = res.data.check_ok
    configViewer.check_output = res.data.check_output || ''
    configViewer.show = true
  } catch (err: any) {
    toast.error('获取完整配置失败: ' + (err.response?.data?.error || err.message))
  }
}

async function copyConfig() {
  await copyToClipboard(configViewer.raw_json, '完整配置 JSON')
}

async function reloadConfig() {
  reloading.value = true
  try {
    const res = await api.post('/singbox/reload')
    toast.success(res.data.message || 'sing-box 服务已重载最新配置！')
  } catch (err: any) {
    toast.error('重载配置失败: ' + (err.response?.data?.error || err.message))
  } finally {
    reloading.value = false
  }
}

watch(
  () => route.query.tab,
  (newTab) => {
    if (newTab && typeof newTab === 'string') {
      const exists = tabs.some((t) => t.key === newTab)
      if (exists) {
        activeTab.value = newTab
      }
    }
  },
  { immediate: true }
)

onMounted(() => {
  const initial = route.query.tab as string
  if (initial && tabs.some((t) => t.key === initial)) {
    activeTab.value = initial
  }
})
</script>

<style scoped>
.tabs-container {
  margin-bottom: 1.75rem;
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
