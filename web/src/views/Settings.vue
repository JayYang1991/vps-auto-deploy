<template>
  <div style="max-width: 680px;">
    <div style="display: flex; align-items: center; gap: 0.5rem; margin-bottom: 1.5rem;">
      <span style="font-size: 1.5rem;">⚙️</span>
      <h1 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">面板设置与安全</h1>
    </div>

    <!-- 系统参数 -->
    <div class="card" style="margin-bottom: 1.5rem;">
      <h3 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem; color: #38bdf8;">系统基础配置</h3>
      <form @submit.prevent="saveSettings" style="display: flex; flex-direction: column; gap: 1rem;">
        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
            面板安全路径 (Secret Base Path)
          </label>
          <div style="display: flex; align-items: center; gap: 0.5rem;">
            <span style="color: #64748b;">/</span>
            <input v-model="settings.secret_path" class="input" required style="font-family: monospace;" />
            <span style="color: #64748b;">/</span>
          </div>
          <p style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem;">
            用于隐藏管理面板入口（如 /a8f9d2/），修改后请务必记牢新 URL，未授权请求将直接返回 404。
          </p>
        </div>

        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
            公网 IP 强制覆盖 (可选)
          </label>
          <input v-model="settings.public_ip" class="input" placeholder="留空则自动检测主机公网 IP" />
          <p style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem;">
            若 VPS 处于 NAT 之后或使用了 CDN/中转，可在此处指定生成订阅和节点链接时填入的 IP 或域名。
          </p>
        </div>

        <button type="submit" class="btn btn-primary" style="align-self: flex-start;">保存系统设置</button>
      </form>
    </div>

    <!-- 修改管理员密码 -->
    <div class="card">
      <h3 style="font-size: 1.1rem; font-weight: 600; margin-bottom: 1rem; color: #fbbf24;">修改管理员密码</h3>
      <form @submit.prevent="updatePassword" style="display: flex; flex-direction: column; gap: 1rem;">
        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">旧密码</label>
          <input v-model="passForm.old_password" type="password" class="input" required />
        </div>

        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">新密码</label>
          <input v-model="passForm.new_password" type="password" class="input" required />
        </div>

        <div>
          <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">确认新密码</label>
          <input v-model="passForm.confirm_password" type="password" class="input" required />
        </div>

        <button type="submit" class="btn btn-primary" style="align-self: flex-start;">更新密码</button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, onMounted } from 'vue'
import api from '../api'
import { useToast } from '../composables/useToast'

const { toast } = useToast()

const settings = reactive({
  secret_path: '',
  public_ip: '',
})

const passForm = reactive({
  old_password: '',
  new_password: '',
  confirm_password: '',
})

async function loadSettings() {
  try {
    const res = await api.get('/settings')
    settings.secret_path = res.data.secret_path
    settings.public_ip = res.data.public_ip
  } catch (err: any) {
    toast.error('加载系统设置失败: ' + (err.response?.data?.error || err.message))
  }
}

async function saveSettings() {
  try {
    await api.post('/settings', settings)
    toast.success('系统设置已更新！如果修改了安全路径，请使用新路径重新访问。', 5000)
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

async function updatePassword() {
  if (passForm.new_password !== passForm.confirm_password) {
    toast.warning('两次输入的新密码不一致！')
    return
  }

  try {
    await api.post('/auth/password', {
      old_password: passForm.old_password,
      new_password: passForm.new_password,
    })
    toast.success('密码修改成功，请使用新密码重新登录！', 3000)
    setTimeout(() => {
      localStorage.removeItem('token')
      window.location.hash = '#/login'
    }, 1200)
  } catch (err: any) {
    toast.error('修改密码失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  loadSettings()
})
</script>
