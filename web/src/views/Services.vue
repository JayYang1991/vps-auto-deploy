<template>
  <div>
    <!-- 业务配置中心标题说明 -->
    <div style="margin-bottom: 1.5rem; display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">🛠️</span>
          <h1 style="font-size: 1.45rem; font-weight: 700; color: #f8fafc;">业务与服务配置中心</h1>
          <span class="badge badge-secondary" style="font-size: 0.75rem;">网络与规则</span>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.35rem; line-height: 1.5;">
          集中配置与管理入站代理协议、公网反代隧道穿透、成员权限及多协议客户端订阅分发。
        </p>
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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Inbounds from './Inbounds.vue'
import Tunnels from './Tunnels.vue'
import Subscriptions from './Subscriptions.vue'

const route = useRoute()
const router = useRouter()

const tabs = [
  {
    key: 'inbounds',
    title: '入站节点',
    subLabel: 'Reality / VLESS',
    icon: '🌐',
    component: Inbounds,
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

const currentTabComponent = computed(() => {
  const found = tabs.find((t) => t.key === activeTab.value)
  return found ? found.component : Inbounds
})

function selectTab(key: string) {
  activeTab.value = key
  router.replace({ path: '/services', query: { tab: key } })
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
