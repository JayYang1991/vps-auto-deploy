<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">👥</span>
          <h1 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">团队成员与订阅分发中心</h1>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.25rem;">
          每个成员拥有独立 UUID 与免登录专属订阅链接，支持随时吊销或重新生成，共用同一服务端节点
        </p>
      </div>

      <button @click="modal.show = true" class="btn btn-primary">+ 添加成员</button>
    </div>

    <!-- 成员卡片列表 -->
    <div style="display: flex; flex-direction: column; gap: 1rem;">
      <div v-for="m in members" :key="m.id" class="card">
        <div style="display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem; margin-bottom: 1rem;">
          <div>
            <div style="display: flex; align-items: center; gap: 0.75rem;">
              <span style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">{{ m.name }}</span>
              <span :class="['badge', m.enabled ? 'badge-success' : 'badge-secondary']">
                {{ m.enabled ? '正常' : '已禁用' }}
              </span>
            </div>
            <div style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.35rem; font-family: monospace;">
              UUID: {{ m.uuid }}
            </div>
          </div>

          <div style="display: flex; gap: 0.5rem;">
            <button @click="resetToken(m.id)" class="btn btn-secondary" style="font-size: 0.8rem;">
              🔄 重置订阅码
            </button>
            <button v-if="members.length > 1" @click="deleteMember(m.id)" class="btn btn-danger" style="font-size: 0.8rem;">
              删除
            </button>
          </div>
        </div>

        <!-- 订阅格式一键获取 -->
        <div style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
          <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;">
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="color: #38bdf8; font-weight: 600; font-size: 0.875rem;">Clash (Mihomo) 订阅</span>
              <span style="color: #64748b; font-size: 0.75rem;">(推荐：Clash Verge Rev / Flclash / Stash)</span>
            </div>
            <div style="display: flex; gap: 0.5rem;">
              <button @click="copySubURL(m.sub_token, 'clash')" class="btn btn-primary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                📋 复制 Clash 链接
              </button>
              <button @click="openSubQR(m.name, m.sub_token, 'clash')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                📱 扫码
              </button>
            </div>
          </div>

          <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;">
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="color: #34d399; font-weight: 600; font-size: 0.875rem;">Sing-box 官方客户端订阅</span>
              <span style="color: #64748b; font-size: 0.75rem;">(SFI / SFA / SFM 客户端)</span>
            </div>
            <div style="display: flex; gap: 0.5rem;">
              <button @click="copySubURL(m.sub_token, 'singbox')" class="btn btn-primary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                📋 复制 Sing-box 链接
              </button>
              <button @click="openSubQR(m.name, m.sub_token, 'singbox')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                📱 扫码
              </button>
            </div>
          </div>

          <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 0.5rem;">
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="color: #fbbf24; font-weight: 600; font-size: 0.875rem;">通用 Base64 / URI 订阅</span>
              <span style="color: #64748b; font-size: 0.75rem;">(Shadowrocket / v2rayN)</span>
            </div>
            <div style="display: flex; gap: 0.5rem;">
              <button @click="copySubURL(m.sub_token, 'base64')" class="btn btn-primary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                📋 复制通用链接
              </button>
              <button @click="openSubQR(m.name, m.sub_token, 'base64')" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.25rem 0.6rem;">
                📱 扫码
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 添加成员弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content">
        <h3 style="font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem;">添加团队成员</h3>

        <form @submit.prevent="createMember" style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">成员名称 / 设备备注</label>
            <input v-model="form.name" class="input" required placeholder="如 Jason-iPhone 或 团队成员A" />
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary">取消</button>
            <button type="submit" class="btn btn-primary">添加并分配专属订阅</button>
          </div>
        </form>
      </div>
    </div>

    <!-- 订阅二维码弹窗 -->
    <div v-if="qrModal.show" class="modal-overlay" @click.self="qrModal.show = false">
      <div class="modal-content" style="text-align: center;">
        <h3 style="font-size: 1.125rem; font-weight: 600; margin-bottom: 1rem;">
          {{ qrModal.title }}
        </h3>

        <div style="background-color: white; padding: 1rem; border-radius: 0.5rem; display: inline-block; margin-bottom: 1rem;">
          <qrcode-vue :value="qrModal.url" :size="220" level="M" />
        </div>

        <p style="color: #94a3b8; font-size: 0.8rem; margin-bottom: 0.5rem;">打开客户端订阅中心扫码导入</p>

        <textarea
          :value="qrModal.url"
          readonly
          class="input"
          rows="2"
          style="font-size: 0.75rem; margin-bottom: 1rem; word-break: break-all;"
        ></textarea>

        <div style="display: flex; justify-content: center; gap: 0.75rem;">
          <button @click="copyModalUrl" class="btn btn-primary">复制订阅链接</button>
          <button @click="qrModal.show = false" class="btn btn-secondary">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import QrcodeVue from 'qrcode.vue'
import api from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

const { toast } = useToast()
const { showConfirm } = useConfirm()

const members = ref<any[]>([])

const modal = reactive({
  show: false,
})

const form = reactive({
  name: '',
})

const qrModal = reactive({
  show: false,
  title: '',
  url: '',
})

async function loadMembers() {
  try {
    const res = await api.get('/members')
    members.value = res.data || []
  } catch (err: any) {
    toast.error('加载成员列表失败: ' + (err.response?.data?.error || err.message))
  }
}

function getSubFullURL(subToken: string, target: string): string {
  const host = window.location.origin
  return `${host}/sub/${subToken}?target=${target}`
}

function copySubURL(subToken: string, target: string) {
  const url = getSubFullURL(subToken, target)
  copyToClipboard(url, `${target.toUpperCase()} 订阅链接`)
}

function copyModalUrl() {
  copyToClipboard(qrModal.url, '订阅链接')
}

function openSubQR(memberName: string, subToken: string, target: string) {
  qrModal.title = `${memberName} - ${target.toUpperCase()} 订阅二维码`
  qrModal.url = getSubFullURL(subToken, target)
  qrModal.show = true
}

async function createMember() {
  try {
    await api.post('/members', {
      name: form.name,
      enabled: true,
    })
    modal.show = false
    toast.success(`成员 ${form.name} 创建成功！`)
    form.name = ''
    await loadMembers()
  } catch (err: any) {
    toast.error('添加失败: ' + (err.response?.data?.error || err.message))
  }
}

async function resetToken(id: number) {
  const confirmed = await showConfirm({
    title: '重置订阅码',
    message: '确定要重置该成员的订阅码吗？重置后原有的订阅链接将立即失效！',
    confirmText: '确认重置',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.post(`/members/${id}/reset-token`)
    toast.success('订阅码已成功重置！原链接已失效。')
    await loadMembers()
  } catch (err: any) {
    toast.error('重置失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteMember(id: number) {
  const confirmed = await showConfirm({
    title: '删除成员',
    message: '确定要删除该成员吗？删除后该成员名下的所有客户端连接配置将无法再同步节点。',
    confirmText: '确认删除',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.delete(`/members/${id}`)
    toast.success('成员已删除！')
    await loadMembers()
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  loadMembers()
})
</script>
