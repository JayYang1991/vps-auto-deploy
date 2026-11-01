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

const router = useRouter()
const username = ref('admin')
const password = ref('')
const loading = ref(false)
const errorMsg = ref('')

async function handleLogin() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await api.post('/auth/login', {
      username: username.value,
      password: password.value,
    })
    localStorage.setItem('token', res.data.token)
    localStorage.setItem('username', res.data.username)
    router.push('/')
  } catch (err: any) {
    errorMsg.value = err.response?.data?.error || '登录失败，请检查账号密码'
  } finally {
    loading.value = false
  }
}
</script>

