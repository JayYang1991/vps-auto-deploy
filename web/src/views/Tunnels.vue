<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">🚇</span>
          <h1 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">Cloudflared 隧道穿透管理 (Argo)</h1>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.25rem;">
          通过 Cloudflare 边缘反代 VPS 本地 VLESS-WS 端口，免去服务器公网端口暴露，有效拯救被阻断的 IP
        </p>
      </div>

      <button @click="openCreateModal" class="btn btn-primary">+ 新建 Argo 隧道</button>
    </div>

    <!-- 未安装 cloudflared 渐进式空状态引导卡片 -->
    <div v-if="!cloudflaredInstalled" class="card" style="margin-bottom: 1.5rem; background: rgba(245, 158, 11, 0.08); border-color: rgba(245, 158, 11, 0.35);">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div style="display: flex; align-items: center; gap: 0.75rem;">
          <span style="font-size: 1.6rem;">📦</span>
          <div>
            <div style="font-weight: 600; color: #fbbf24; font-size: 0.95rem;">未检测到已安装的 cloudflared 客户端</div>
            <div style="color: #94a3b8; font-size: 0.825rem; margin-top: 0.25rem;">
              Argo 隧道穿透由底层的 cloudflared 守护进程提供。您可先配置映射，或进入软件中心一键安装。
            </div>
          </div>
        </div>
        <router-link to="/components?install=cloudflared" class="btn btn-primary" style="font-size: 0.825rem; padding: 0.4rem 0.85rem;">
          🚀 前往「软件中心」一键安装
        </router-link>
      </div>
    </div>

    <!-- 列表展示 -->
    <div v-if="tunnels.length === 0" class="card" style="text-align: center; padding: 3rem;">
      <div style="font-size: 2.25rem; margin-bottom: 0.5rem;">🚇</div>
      <p style="color: #94a3b8; margin-bottom: 1.25rem;">暂无已配置的 Cloudflared 隧道，点击下方按钮一键创建。</p>
      <button @click="openCreateModal" class="btn btn-primary">+ 一键创建快速测试隧道 (Quick Tunnel)</button>
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 1rem;">
      <div v-for="t in tunnels" :key="t.id" class="card" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div>
          <div style="display: flex; align-items: center; gap: 0.75rem;">
            <span style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">{{ t.name }}</span>
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
              <span v-else style="color: #fbbf24;">(正在获取 Cloudflare 分配域名...)</span>
            </span>
          </div>
        </div>

        <div style="display: flex; gap: 0.5rem;">
          <button v-if="t.domain" @click="copyDomain(t.domain)" class="btn btn-secondary">
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
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
              Zero Trust Token 或安装命令
            </label>
            <textarea
              v-model="form.token"
              class="input"
              rows="3"
              placeholder="支持直接粘贴 CF 页面生成的完整命令 (如 cloudflared.exe service install eyJh...) 或只填入 Token"
              required
            ></textarea>
            <p style="color: #64748b; font-size: 0.75rem; margin-top: 0.25rem;">
              💡 系统会自动从命令中提取 Token。隧道连通后，请在 Cloudflare 网页下一步的「Public Hostname」页面将域名反代到本地端口。
            </p>

            <div style="margin-top: 0.75rem;">
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
                绑定的自定义域名 (选填)
              </label>
              <input
                v-model="form.domain"
                class="input"
                placeholder="如 argo.yourdomain.com（在 CF 控制台 Public Hostname 绑定的域名，用于面板展示与订阅导出）"
              />
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
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

const { toast } = useToast()
const { showConfirm } = useConfirm()

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
  } catch (err: any) {
    toast.error('加载隧道列表失败: ' + (err.response?.data?.error || err.message))
  }
}

function openCreateModal() {
  modal.show = true
  form.name = 'argo-vless'
  form.mode = 'quick'
  form.target_port = 10086
}

function cleanToken(raw: string): string {
  if (!raw) return ''
  const oneLine = raw.replace(/[\r\n\t]+/g, ' ').trim()
  const match = oneLine.match(/(?:--token|install)\s+([A-Za-z0-9_\-\.]+)/)
  if (match && match[1]) {
    return match[1]
  }
  return oneLine.replace(/\s+/g, '')
}

async function submitTunnel() {
  try {
    const payload = {
      ...form,
      token: cleanToken(form.token),
    }
    await api.post('/tunnels', payload)
    modal.show = false
    toast.success('隧道创建成功，正在启动...')
    await loadTunnels()
    // 轮询 2 次更新 quick 域名
    if (form.mode === 'quick') {
      setTimeout(loadTunnels, 3000)
      setTimeout(loadTunnels, 6000)
    }
  } catch (err: any) {
    toast.error('创建隧道失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteTunnel(id: number) {
  const confirmed = await showConfirm({
    title: '确认停止并删除隧道',
    message: '确定要删除并停止该 Cloudflare 隧道进程吗？',
    confirmText: '确认删除',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.delete(`/tunnels/${id}`)
    toast.success('隧道已停止并删除！')
    await loadTunnels()
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

function copyDomain(domain: string) {
  copyToClipboard(domain, '隧道域名')
}

onMounted(() => {
  loadTunnels()
})
</script>
