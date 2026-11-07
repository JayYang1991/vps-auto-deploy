<template>
  <div v-if="isLoginRoute">
    <router-view />
    <ToastContainer />
    <ConfirmModal />
  </div>

  <div v-else style="min-height: 100vh; display: flex; flex-direction: column;">
    <!-- 顶部导航栏 -->
    <header class="app-header">
      <div class="brand-box">
        <span style="font-size: 1.5rem;">🚀</span>
        <div>
          <span class="brand-title">VPS Auto Deploy</span>
          <span class="brand-version">v1.2.0</span>
        </div>
      </div>

      <nav class="nav-container">
        <!-- 版块一：基础软件与安装 -->
        <div class="nav-group">
          <span class="nav-group-label">基础软件</span>
          <div class="nav-group-items">
            <router-link
              to="/"
              :class="['nav-pill', currentRoute === '/' ? 'nav-pill-active' : '']"
            >
              <span>📊</span>
              <span>系统概览</span>
            </router-link>

            <router-link
              to="/components"
              :class="['nav-pill', currentRoute === '/components' ? 'nav-pill-active' : '']"
            >
              <span>📦</span>
              <span>软件中心</span>
            </router-link>
          </div>
        </div>

        <div class="nav-divider"></div>

        <!-- 版块二：业务配置中心 -->
        <div class="nav-group">
          <span class="nav-group-label">业务配置</span>
          <div class="nav-group-items">
            <router-link
              to="/services"
              :class="['nav-pill', isServicesActive ? 'nav-pill-active' : '']"
            >
              <span>🛠️</span>
              <span>配置中心</span>
            </router-link>
          </div>
        </div>

        <div class="nav-divider"></div>

        <!-- 面板管理 -->
        <div class="nav-group">
          <div class="nav-group-items">
            <router-link
              to="/settings"
              :class="['nav-pill', currentRoute === '/settings' ? 'nav-pill-active' : '']"
            >
              <span>⚙️</span>
              <span>设置</span>
            </router-link>

            <button @click="handleLogout" class="nav-pill nav-pill-logout" type="button">
              <span>🚪</span>
              <span>退出</span>
            </button>
          </div>
        </div>
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

    <!-- 全局非阻塞反馈组件 -->
    <ToastContainer />
    <ConfirmModal />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ToastContainer from './components/ToastContainer.vue'
import ConfirmModal from './components/ConfirmModal.vue'
import { useConfirm } from './composables/useConfirm'

const route = useRoute()
const router = useRouter()
const { showConfirm } = useConfirm()

const isLoginRoute = computed(() => route.path === '/login')
const currentRoute = computed(() => route.path)

const isServicesActive = computed(() => {
  const p = route.path
  return p === '/services' || p === '/inbounds' || p === '/tunnels' || p === '/subscriptions'
})

async function handleLogout() {
  const confirmed = await showConfirm({
    title: '确认退出登录',
    message: '确定要退出当前面板管理会话吗？',
    confirmText: '退出登录',
  })
  if (!confirmed) return

  localStorage.removeItem('token')
  router.push('/login')
}
</script>

<style scoped>
.app-header {
  background-color: #1e293b;
  border-bottom: 1px solid #334155;
  padding: 0.65rem 1.5rem;
  display: flex;
  justify-content: space-between;
  align-items: center;
  position: sticky;
  top: 0;
  z-index: 40;
  flex-wrap: wrap;
  gap: 1rem;
}

.brand-box {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.brand-title {
  font-weight: 700;
  font-size: 1.125rem;
  color: #f8fafc;
}

.brand-version {
  font-size: 0.75rem;
  color: #64748b;
  margin-left: 0.4rem;
}

.nav-container {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
}

.nav-group {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.nav-group-label {
  font-size: 0.68rem;
  color: #94a3b8;
  font-weight: 600;
  padding: 0.15rem 0.45rem;
  background-color: #0f172a;
  border: 1px solid #334155;
  border-radius: 4px;
  letter-spacing: 0.02em;
}

.nav-group-items {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.nav-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.75rem;
  font-size: 0.835rem;
  font-weight: 500;
  border-radius: 0.5rem;
  color: #cbd5e1;
  text-decoration: none;
  background-color: #1e293b;
  border: 1px solid #334155;
  transition: all 0.15s ease;
  cursor: pointer;
}

.nav-pill:hover {
  background-color: #334155;
  color: #ffffff;
  border-color: #475569;
}

.nav-pill-active {
  background-color: #3b82f6;
  border-color: #3b82f6;
  color: #ffffff;
  box-shadow: 0 2px 8px rgba(59, 130, 246, 0.35);
}

.nav-pill-logout {
  color: #f87171;
  border-color: rgba(239, 68, 68, 0.3);
  background-color: rgba(239, 68, 68, 0.08);
}

.nav-pill-logout:hover {
  background-color: rgba(239, 68, 68, 0.2);
  color: #fca5a5;
  border-color: #ef4444;
}

.nav-divider {
  width: 1px;
  height: 22px;
  background-color: #334155;
}

@media (max-width: 900px) {
  .nav-group-label {
    display: none;
  }
  .nav-divider {
    display: none;
  }
}
</style>
