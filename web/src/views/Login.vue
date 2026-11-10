<template>
  <div style="min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 1rem;">
    <div class="card" style="width: 100%; max-width: 400px; padding: 2rem;">
      <div style="text-align: center; margin-bottom: 2rem;">
        <div style="font-size: 2.5rem; margin-bottom: 0.5rem;">🚀</div>
        <h1 style="font-size: 1.5rem; font-weight: 700; color: #f8fafc;">VPS Auto Deploy</h1>
        <p style="color: #94a3b8; font-size: 0.875rem; margin-top: 0.25rem;">代理与隧道服务管理面板</p>
      </div>

      <form @submit.prevent="handleLogin" style="display: flex; flex-direction: column; gap: 1.25rem;">
        <div>
          <label style="display: block; font-size: 0.875rem; color: #cbd5e1; margin-bottom: 0.5rem;">用户名</label>
          <input v-model="username" class="input" type="text" placeholder="默认 admin" required />
        </div>

        <div>
          <label style="display: block; font-size: 0.875rem; color: #cbd5e1; margin-bottom: 0.5rem;">密码</label>
          <input v-model="password" class="input" type="password" placeholder="默认 admin888" required />
        </div>

        <div style="display: flex; align-items: center; justify-content: space-between; font-size: 0.8rem; color: #94a3b8; user-select: none;">
          <label style="display: flex; align-items: center; gap: 0.45rem; cursor: pointer;">
            <input type="checkbox" v-model="rememberMe" style="accent-color: #3b82f6; width: 0.95rem; height: 0.95rem; cursor: pointer;" />
            <span>记住此设备 (7天内保持登录)</span>
          </label>
          <span style="font-size: 0.725rem; color: #64748b;">
            {{ rememberMe ? '持久会话' : '临时会话 (关窗即销毁)' }}
          </span>
        </div>

        <div v-if="errorMsg" class="badge badge-danger" style="padding: 0.5rem; justify-content: center;">
          {{ errorMsg }}
        </div>

        <button type="submit" class="btn btn-primary" :disabled="loading" style="width: 100%; margin-top: 0.5rem;">
          {{ loading ? '登录中...' : '登 录' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import { setToken, updateSessionIdleTimeout } from '../utils/auth'

const router = useRouter()
const username = ref('admin')
const password = ref('')
const rememberMe = ref(false)
const loading = ref(false)
const errorMsg = ref('')

async function handleLogin() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await api.post('/auth/login', {
      username: username.value,
      password: password.value,
      remember_me: rememberMe.value,
    })
    setToken(res.data.token, rememberMe.value)
    if (res.data.username) {
      localStorage.setItem('username', res.data.username)
    }
    // 异步加载最新配置的空闲超时时长
    try {
      const setRes = await api.get('/settings')
      if (setRes.data && typeof setRes.data.session_idle_timeout === 'number') {
        updateSessionIdleTimeout(setRes.data.session_idle_timeout)
      }
    } catch {
      // 容错处理
    }
    router.push('/')
  } catch (err: any) {
    errorMsg.value = err.response?.data?.error || '登录失败，请检查账号密码'
  } finally {
    loading.value = false
  }
}
</script>

