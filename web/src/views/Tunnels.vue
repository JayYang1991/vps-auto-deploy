<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem;">
      <div>
        <h2 style="font-size: 1.25rem; font-weight: 600;">Cloudflared 隧道管理 (Argo)</h2>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.2rem;">
          通过 Cloudflare 边缘反代 VPS 本地 VLESS-WS 端口，免去服务器公网端口暴露，有效拯救被阻断的 IP
        </p>
      </div>

      <button @click="openCreateModal" class="btn btn-primary">+ 新建 Argo 隧道</button>
    </div>

    <!-- 未安装 cloudflared 提示横幅 -->
    <div v-if="!cloudflaredInstalled" style="background-color: rgba(245, 158, 11, 0.1); border: 1px solid rgba(245, 158, 11, 0.3); border-radius: 0.5rem; padding: 0.75rem 1rem; margin-bottom: 1.5rem; display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.5rem;">
      <div style="display: flex; align-items: center; gap: 0.5rem; color: #fbbf24; font-size: 0.85rem;">
        <span>💡</span>
        <span>当前尚未安装 <b>cloudflared</b> 客户端。您可在此配置隧道，保存后前往「仪表盘」一键安装 cloudflared 即可自动连接 Cloudflare 边缘节点！</span>
      </div>
      <router-link to="/" class="btn btn-secondary" style="padding: 0.25rem 0.6rem; font-size: 0.75rem; white-space: nowrap; border-color: #fbbf24; color: #fbbf24;">
        前往安装
      </router-link>
    </div>

    <div v-if="tunnels.length === 0" class="card" style="text-align: center; padding: 3rem;">
      <div style="font-size: 2rem; margin-bottom: 0.5rem;">🚇</div>
      <p style="color: #94a3b8; margin-bottom: 1rem;">暂无已配置的 Cloudflared 隧道。</p>
      <button @click="openCreateModal" class="btn btn-primary">+ 一键创建快速测试隧道 (Quick Tunnel)</button>
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 1rem;">
      <div v-for="t in tunnels" :key="t.id" class="card" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div>
          <div style="display: flex; align-items: center; gap: 0.75rem;">
            <span style="font-size: 1.1rem; font-weight: 600;">{{ t.name }}</span>
            <span :class="['badge', t.mode === 'quick' ? 'badge-warning' : 'badge-success']">
              {{ t.mode === 'quick' ? 'Quick 临时隧道' : 'Zero Trust Token' }}
            </span>
            <span :class="['badge', t.status === 'running' ? 'badge-success' : 'badge-secondary']">
              {{ t.status }}
            </span>
          </div>

          <div style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.5rem; display: flex; gap: 1.5rem; flex-wrap: wrap;">
            <span>本地转发端口: <b style="color: #f1f5f9;">127.0.0.1:{{ t.target_port }}</b></span>
            <span>
              边缘域名:
              <b v-if="t.domain" style="color: #38bdf8;">{{ t.domain }}</b>
              <span v-else style="color: #fbbf24;">(正在从日志获取域名...)</span>
            </span>
          </div>
        </div>

        <div style="display: flex; gap: 0.5rem;">
          <button v-if="t.domain" @click="copyText(t.domain)" class="btn btn-secondary">
            📋 复制域名
          </button>
          <button @click="deleteTunnel(t.id)" class="btn btn-danger" style="padding: 0.5rem 0.75rem;">
            删除
          </button>
        </div>
      </div>
    </div>

    <!-- 新建隧道弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content">
        <h3 style="font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem;">新建 Cloudflare 隧道</h3>

        <form @submit.prevent="submitTunnel" style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">隧道名称</label>
            <input v-model="form.name" class="input" required placeholder="如 argo-test" />
          </div>

          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">隧道类型</label>
            <select v-model="form.mode" class="input">
              <option value="quick">Quick Tunnel (免登录试用，自动生成 *.trycloudflare.com)</option>
              <option value="token">Token 模式 (自建 Cloudflare Zero Trust 隧道，绑定自定义域名)</option>
            </select>
          </div>

          <div v-if="form.mode === 'token'">
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">Zero Trust Token</label>
            <textarea v-model="form.token" class="input" rows="3" placeholder="eyJhIjoi..." required></textarea>
            <div style="margin-top: 0.5rem;">
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">绑定的自定义域名</label>
              <input v-model="form.domain" class="input" placeholder="argo.yourdomain.com" required />
            </div>
          </div>

          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">本地代理入站端口</label>
            <input v-model.number="form.target_port" type="number" class="input" required placeholder="10086" />
            <p style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem;">请确保已在【入站节点】中添加对应端口的 VLESS-WS 监听。</p>
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary">取消</button>
            <button type="submit" class="btn btn-primary">启动隧道</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import api from '../api'

const tunnels = ref<any[]>([])
const cloudflaredInstalled = ref(true)

const modal = reactive({
  show: false,
})

const form = reactive({
  name: 'argo-vless',
  mode: 'quick',
  token: '',
  domain: '',
  target_port: 10086,
})

async function loadTunnels() {
  try {
    const res = await api.get('/tunnels')
    tunnels.value = res.data || []

    const compRes = await api.get('/components')
    const cf = (compRes.data || []).find((c: any) => c.name === 'cloudflared')
    if (cf) {
      cloudflaredInstalled.value = cf.installed
    }
  } catch (err) {
    console.error(err)
  }
}

function openCreateModal() {
  modal.show = true
  form.name = 'argo-vless'
  form.mode = 'quick'
  form.target_port = 10086
}

async function submitTunnel() {
  try {
    await api.post('/tunnels', form)
    modal.show = false
    await loadTunnels()
    // 轮询 3 次更新 quick 域名
    if (form.mode === 'quick') {
      setTimeout(loadTunnels, 3000)
      setTimeout(loadTunnels, 6000)
    }
  } catch (err: any) {
    alert('创建隧道失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteTunnel(id: number) {
  if (!confirm('确定要删除并停止该隧道吗？')) return
  try {
    await api.delete(`/tunnels/${id}`)
    await loadTunnels()
  } catch (err: any) {
    alert('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

function copyText(txt: string) {
  navigator.clipboard.writeText(txt)
  alert('已复制到剪贴板！')
}

onMounted(() => {
  loadTunnels()
})
</script>

