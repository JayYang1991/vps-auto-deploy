<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem;">
      <div>
        <h2 style="font-size: 1.25rem; font-weight: 600;">团队成员与订阅中心</h2>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.2rem;">
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
              <span style="font-size: 1.15rem; font-weight: 600;">{{ m.name }}</span>
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

        <button @click="qrModal.show = false" class="btn btn-secondary">关闭</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import QrcodeVue from 'qrcode.vue'
import api from '../api'

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
  } catch (err) {
    console.error(err)
  }
}

function getSubFullURL(subToken: string, target: string): string {
  const host = window.location.origin
  return `${host}/sub/${subToken}?target=${target}`
}

function copySubURL(subToken: string, target: string) {
  const url = getSubFullURL(subToken, target)
  navigator.clipboard.writeText(url)
  alert('订阅链接已复制到剪贴板！可以直接粘贴到 Clash/Singbox 客户端使用。')
}

function openSubQR(memberName: string, subToken: string, target: string) {
  qrModal.title = `${memberName} - ${target.toUpperCase()} 订阅码`
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
    form.name = ''
    await loadMembers()
  } catch (err: any) {
    alert('添加失败: ' + (err.response?.data?.error || err.message))
  }
}

async function resetToken(id: number) {
  if (!confirm('确定要重置该成员的订阅码吗？重置后原有的订阅链接将立即失效！')) return
  try {
    await api.post(`/members/${id}/reset-token`)
    await loadMembers()
    alert('订阅码已重置！')
  } catch (err: any) {
    alert('重置失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteMember(id: number) {
  if (!confirm('确定要删除该成员吗？')) return
  try {
    await api.delete(`/members/${id}`)
    await loadMembers()
  } catch (err: any) {
    alert('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  loadMembers()
})
</script>

