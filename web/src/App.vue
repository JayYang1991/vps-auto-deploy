<template>
  <div v-if="isLoginRoute">
    <router-view />
  </div>

  <div v-else style="min-height: 100vh; display: flex; flex-direction: column;">
    <!-- 顶部导航栏 -->
    <header style="background-color: #1e293b; border-bottom: 1px solid #334155; padding: 0.75rem 1.5rem; display: flex; justify-content: space-between; align-items: center; position: sticky; top: 0; z-index: 40;">
      <div style="display: flex; align-items: center; gap: 0.75rem;">
        <span style="font-size: 1.5rem;">🚀</span>
        <div>
          <span style="font-weight: 700; font-size: 1.125rem; color: #f8fafc;">VPS Auto Deploy</span>
          <span style="font-size: 0.75rem; color: #64748b; margin-left: 0.5rem;">v1.0.0</span>
        </div>
      </div>

      <nav style="display: flex; align-items: center; gap: 0.5rem;">
        <router-link
          v-for="item in navItems"
          :key="item.path"
          :to="item.path"
          :class="['btn', currentRoute === item.path ? 'btn-primary' : 'btn-secondary']"
          style="padding: 0.4rem 0.8rem; font-size: 0.85rem;"
        >
          <span>{{ item.icon }}</span>
          <span>{{ item.title }}</span>
        </router-link>

        <button @click="handleLogout" class="btn btn-secondary" style="padding: 0.4rem 0.8rem; font-size: 0.85rem; margin-left: 1rem; color: #f87171;">
          🚪 退出
        </button>
      </nav>
    </header>

    <!-- 主体区域 -->
    <main style="flex: 1; max-width: 1200px; width: 100%; margin: 0 auto; padding: 2rem 1.5rem;">
      <router-view />
    </main>

    <!-- 页脚 -->
    <footer style="border-top: 1px solid #1e293b; padding: 1.25rem; text-align: center; color: #64748b; font-size: 0.75rem;">
      VPS Auto Deploy &copy; 2026 &middot; Sing-box &amp; Cloudflared 一体化管理面板
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()

const isLoginRoute = computed(() => route.path === '/login')
const currentRoute = computed(() => route.path)

const navItems = [
  { path: '/', title: '仪表盘 & 组件', icon: '📊' },
  { path: '/inbounds', title: '入站节点 (Reality)', icon: '🌐' },
  { path: '/tunnels', title: 'Argo 隧道', icon: '🚇' },
  { path: '/subscriptions', title: '成员与订阅', icon: '👥' },
  { path: '/settings', title: '设置', icon: '⚙️' },
]

function handleLogout() {
  localStorage.removeItem('token')
  router.push('/login')
}
</script>

