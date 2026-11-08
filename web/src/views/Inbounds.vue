<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">🌐</span>
          <h1 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">Sing-box 入站节点配置</h1>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.25rem;">
          支持直连首选的 VLESS-XTLS-Vision-Reality 及配合 Cloudflared 隧道防封的 VLESS-WS
        </p>
      </div>

      <div style="display: flex; gap: 0.5rem;">
        <button
          @click="importExistingConfig"
          class="btn btn-secondary"
          :disabled="importing"
          title="从宿主机已有配置（如 /etc/sing-box/config.json）一键同步导入入站节点与用户"
        >
          <span>📥</span>
          <span>{{ importing ? '正在导入...' : '导入宿主机配置' }}</span>
        </button>
        <button @click="openCreateModal" class="btn btn-primary">+ 新增节点</button>
      </div>
    </div>

    <!-- 未安装 sing-box 渐进式空状态引导卡片 (仅在加载完成后判定) -->
    <div v-if="!loading && !singboxInstalled" class="card" style="margin-bottom: 1.5rem; background: rgba(245, 158, 11, 0.08); border-color: rgba(245, 158, 11, 0.35);">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div style="display: flex; align-items: center; gap: 0.75rem;">
          <span style="font-size: 1.6rem;">📦</span>
          <div>
            <div style="font-weight: 600; color: #fbbf24; font-size: 0.95rem;">未检测到已安装的 sing-box 代理核心</div>
            <div style="color: #94a3b8; font-size: 0.825rem; margin-top: 0.25rem;">
              入站节点由底层的 sing-box 驱动并监听。您可以先在此预设规则，或直接进入软件中心一键安装。
            </div>
          </div>
        </div>
        <router-link to="/components?install=sing-box" class="btn btn-primary" style="font-size: 0.825rem; padding: 0.4rem 0.85rem;">
          🚀 前往「软件中心」一键安装
        </router-link>
      </div>
    </div>

    <!-- 加载中指示器 -->
    <div v-if="loading" class="card loading-box">
      <div class="loading-spinner"></div>
      <p class="loading-text">正在加载入站节点配置...</p>
    </div>

    <!-- 节点列表空状态 -->
    <div v-else-if="inbounds.length === 0" class="card" style="text-align: center; padding: 2.5rem 1.5rem;">
      <div style="font-size: 2.25rem; margin-bottom: 0.5rem;">🌐</div>
      <h3 style="font-size: 1.1rem; font-weight: 600; color: #f8fafc; margin-bottom: 0.5rem;">暂无已配置的入站节点</h3>
      <p style="color: #94a3b8; font-size: 0.85rem; max-width: 540px; margin: 0 auto 1.25rem; line-height: 1.5;">
        若在安装面板前宿主机已安装 Sing-box 并在 <code>/etc/sing-box/config.json</code> 中配置过节点，点击下方按钮可一键导入至面板集中管理；亦可直接新增首个节点。
      </p>
      <div style="display: flex; justify-content: center; gap: 0.75rem; flex-wrap: wrap;">
        <button @click="importExistingConfig" class="btn btn-secondary" :disabled="importing">
          <span>📥</span>
          <span>{{ importing ? '正在导入...' : '一键导入宿主机已有配置' }}</span>
        </button>
        <button @click="openCreateModal" class="btn btn-primary">+ 一键配置首个 VLESS-Reality 节点</button>
      </div>
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 1rem;">
      <div v-for="ib in inbounds" :key="ib.id" class="card" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div>
          <div style="display: flex; align-items: center; gap: 0.75rem;">
            <span style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">{{ ib.tag }}</span>
            <span :class="['badge', ib.protocol === 'vless-reality' ? 'badge-success' : 'badge-warning']">
              {{ ib.protocol }}
            </span>
            <span :class="['badge', ib.enabled ? 'badge-success' : 'badge-secondary']">
              {{ ib.enabled ? '已启用' : '已停用' }}
            </span>
          </div>

          <div style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.5rem; display: flex; gap: 1.5rem; flex-wrap: wrap;">
            <span>监听端口: <b style="color: #f1f5f9;">{{ ib.port }}</b></span>
            <span>绑定地址: <b style="color: #f1f5f9;">{{ ib.listen }}</b></span>
            <span v-if="ib.protocol === 'vless-reality'">
              SNI 伪装: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).server_name }}</b>
            </span>
            <span v-else-if="ib.protocol === 'vless-ws'">
              WS Path: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).path || '/ws' }}</b>
              <span v-if="parseSpec(ib.spec_json).group_domain" style="margin-left: 0.75rem;">
                分组域名: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).group_domain }}</b>
              </span>
            </span>
          </div>
        </div>

        <div style="display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap;">
          <button
            @click="toggleInbound(ib)"
            :class="['btn', ib.enabled ? 'btn-secondary' : 'btn-primary']"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
          >
            {{ ib.enabled ? '⏸️ 停用' : '▶️ 启用' }}
          </button>
          <button @click="showShareModal(ib)" class="btn btn-secondary" style="padding: 0.4rem 0.75rem; font-size: 0.8rem;">
            📱 二维码 / 节点链接
          </button>
          <button @click="openEditModal(ib)" class="btn btn-secondary" style="padding: 0.4rem 0.75rem; font-size: 0.8rem;">
            ✏️ 编辑
          </button>
          <button @click="deleteInbound(ib.id)" class="btn btn-danger" style="padding: 0.4rem 0.75rem; font-size: 0.8rem;">
            🗑️ 删除
          </button>
        </div>
      </div>
    </div>

    <!-- 新增/编辑弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content">
        <h3 style="font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem;">
          {{ modal.isEdit ? '编辑入站节点' : '新建入站节点' }}
        </h3>

        <form @submit.prevent="submitInbound" style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">协议类型</label>
            <select v-model="form.protocol" class="input" @change="onProtocolChange">
              <option value="vless-reality">VLESS-Vision-Reality (直连首选、极速免证书)</option>
              <option value="vless-ws">VLESS-WS (与 Cloudflared 配合使用)</option>
            </select>
          </div>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">节点标识 Tag</label>
              <input v-model="form.tag" class="input" required placeholder="如 vless-reality-443" />
            </div>
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">监听端口</label>
              <input v-model.number="form.port" type="number" class="input" required placeholder="443" />
            </div>
          </div>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">绑定监听地址</label>
              <input v-model="form.listen" class="input" placeholder="如 :: 或 0.0.0.0 或 127.0.0.1" />
            </div>
            <div style="display: flex; align-items: center; padding-top: 1.25rem;">
              <label style="display: flex; align-items: center; gap: 0.5rem; font-size: 0.875rem; color: #f1f5f9; cursor: pointer;">
                <input type="checkbox" v-model="form.enabled" style="accent-color: #38bdf8; width: 1.1rem; height: 1.1rem;" />
                启用此入站规则
              </label>
            </div>
          </div>

          <!-- Reality 专属参数 -->
          <div v-if="form.protocol === 'vless-reality'" style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="display: flex; justify-content: space-between; align-items: center;">
              <span style="font-weight: 500; font-size: 0.875rem; color: #38bdf8;">Reality 伪装与密钥配置</span>
              <button type="button" @click="generateRealityKeys" class="btn btn-secondary" style="font-size: 0.75rem; padding: 0.2rem 0.5rem;">
                ⚡ 自动生成密钥对与 ShortID
              </button>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">伪装目标域名 (SNI)</label>
                <input v-model="realitySpec.server_name" class="input" placeholder="dl.google.com" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">客户端指纹 (Fingerprint)</label>
                <input v-model="realitySpec.fingerprint" class="input" placeholder="chrome" />
              </div>
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">服务端私钥 (Private Key)</label>
              <input v-model="realitySpec.private_key" class="input" required placeholder="点击右上角自动生成" />
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">客户端公钥 (Public Key - pbk)</label>
              <input v-model="realitySpec.public_key" class="input" required placeholder="点击右上角自动生成" />
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">Short ID (短 ID)</label>
              <input v-model="realitySpec.short_id" class="input" required placeholder="8位十六进制" />
            </div>
          </div>

          <!-- VLESS-WS 专属参数 -->
          <div v-else style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;">
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.25rem;">分组指定域名 (SNI / Host)</label>
              <input v-model="wsSpec.group_domain" class="input" placeholder="如 argo.example.com 或留空继承默认" />
              <div style="font-size: 0.725rem; color: #64748b; margin-top: 0.25rem;">
                生成订阅时，优选 IP 作为连接目标，该域名作为分组 TLS SNI 与 HTTP Host。留空时自动回退继承全局优选域名或穿透域名。
              </div>
            </div>

            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.25rem;">WebSocket Path</label>
              <input v-model="wsSpec.path" class="input" placeholder="/ws" />
            </div>
          </div>

          <!-- 用户鉴权与 Name 来源说明 -->
          <div style="background-color: rgba(30, 41, 59, 0.5); border: 1px dashed #334155; padding: 0.75rem 1rem; border-radius: 0.5rem; font-size: 0.8rem; color: #94a3b8;">
            <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.5rem;">
              <span>👥 <strong>用户列表 (users)</strong>：已自动绑定当前所有启用的团队成员。</span>
              <router-link to="/services?tab=subscriptions&sub=members" class="btn btn-secondary" style="font-size: 0.725rem; padding: 0.2rem 0.5rem; text-decoration: none;">
                管理成员与 Name ↗
              </router-link>
            </div>
            <div style="font-size: 0.725rem; color: #64748b; margin-top: 0.35rem; line-height: 1.4;">
              底层 sing-box 节点中的 <code style="color: #cbd5e1;">users[].name</code> 与 <code style="color: #cbd5e1;">uuid</code> 由系统全局多租户统一管理并自动同步生效，无需在每个入站中重复录入。
            </div>
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary">取消</button>
            <button type="submit" class="btn btn-primary">保存并应用</button>
          </div>
        </form>
      </div>
    </div>

    <!-- 节点链接与二维码分享弹窗 -->
    <div v-if="shareModal.show" class="modal-overlay" @click.self="shareModal.show = false">
      <div class="modal-content" style="text-align: center;">
        <h3 style="font-size: 1.125rem; font-weight: 600; margin-bottom: 1rem;">
          {{ shareModal.title }}
        </h3>

        <div style="background-color: white; padding: 1rem; border-radius: 0.5rem; display: inline-block; margin-bottom: 1rem;">
          <qrcode-vue :value="shareModal.uri" :size="200" level="M" />
        </div>

        <p style="color: #94a3b8; font-size: 0.8rem; margin-bottom: 0.5rem;">可使用 Shadowrocket / v2rayN 等客户端直接扫码导入</p>

        <textarea
          :value="shareModal.uri"
          readonly
          class="input"
          rows="3"
          style="font-size: 0.75rem; margin-bottom: 1rem; word-break: break-all;"
        ></textarea>

        <div style="display: flex; justify-content: center; gap: 0.75rem;">
          <button @click="copyShareUri" class="btn btn-primary">复制节点链接</button>
          <button @click="shareModal.show = false" class="btn btn-secondary">关闭</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import QrcodeVue from 'qrcode.vue'
import api from '../api'
import { useToast, copyToClipboard } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

const { toast } = useToast()
const { showConfirm } = useConfirm()

const inbounds = ref<any[]>([])
const loading = ref(true)
const singboxInstalled = ref(true)

const modal = reactive({
  show: false,
  isEdit: false,
  id: 0,
})

const form = reactive({
  tag: '',
  protocol: 'vless-reality',
  listen: '::',
  port: 443,
  enabled: true,
})

const realitySpec = reactive({
  flow: 'xtls-rprx-vision',
  server_name: 'dl.google.com',
  handshake_server: 'dl.google.com',
  handshake_port: 443,
  private_key: '',
  public_key: '',
  short_id: '',
  fingerprint: 'chrome',
})

const wsSpec = reactive({
  path: '/ws',
  group_domain: '',
})

const shareModal = reactive({
  show: false,
  title: '',
  uri: '',
})

const importing = ref(false)

async function importExistingConfig() {
  importing.value = true
  try {
    const res = await api.post('/inbounds/import')
    toast.success(res.data.message || '导入成功！')
    await loadInbounds()
  } catch (err: any) {
    toast.error('导入宿主机配置失败: ' + (err.response?.data?.error || err.message))
  } finally {
    importing.value = false
  }
}

async function loadInbounds() {
  loading.value = true
  try {
    const res = await api.get('/inbounds')
    inbounds.value = res.data || []

    const compRes = await api.get('/components')
    const sb = (compRes.data || []).find((c: any) => c.name === 'sing-box')
    if (sb) {
      singboxInstalled.value = sb.installed
    }
  } catch (err: any) {
    toast.error('加载入站节点列表失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

function parseSpec(jsonStr: string) {
  try {
    return JSON.parse(jsonStr)
  } catch {
    return {}
  }
}

async function openCreateModal() {
  modal.isEdit = false
  modal.id = 0
  modal.show = true
  form.protocol = 'vless-reality'
  form.listen = '::'
  form.port = 443
  form.tag = 'vless-reality-443'
  form.enabled = true
  realitySpec.server_name = 'dl.google.com'
  realitySpec.handshake_server = 'dl.google.com'
  wsSpec.path = '/ws'
  wsSpec.group_domain = ''
  await generateRealityKeys()
}

function openEditModal(ib: any) {
  modal.isEdit = true
  modal.id = ib.id
  form.tag = ib.tag
  form.protocol = ib.protocol
  form.listen = ib.listen || (ib.protocol === 'vless-ws' ? '127.0.0.1' : '::')
  form.port = ib.port
  form.enabled = !!ib.enabled

  const spec = parseSpec(ib.spec_json)
  if (ib.protocol === 'vless-reality') {
    realitySpec.flow = spec.flow || 'xtls-rprx-vision'
    realitySpec.server_name = spec.server_name || 'dl.google.com'
    realitySpec.handshake_server = spec.handshake_server || spec.server_name || 'dl.google.com'
    realitySpec.handshake_port = spec.handshake_port || 443
    realitySpec.private_key = spec.private_key || ''
    realitySpec.public_key = spec.public_key || ''
    realitySpec.short_id = spec.short_id || ''
    realitySpec.fingerprint = spec.fingerprint || 'chrome'
  } else if (ib.protocol === 'vless-ws') {
    wsSpec.path = spec.path || '/ws'
    wsSpec.group_domain = spec.group_domain || spec.host || ''
  }
  modal.show = true
}

async function toggleInbound(ib: any) {
  try {
    const res = await api.post(`/inbounds/${ib.id}/toggle`)
    ib.enabled = res.data.enabled
    toast.success(`入站规则 ${ib.tag} 已${res.data.enabled ? '启用' : '停用'}`)
  } catch (err: any) {
    toast.error('切换状态失败: ' + (err.response?.data?.error || err.message))
    await loadInbounds()
  }
}

async function generateRealityKeys() {
  try {
    const res = await api.post('/inbounds/reality-keys')
    realitySpec.private_key = res.data.private_key
    realitySpec.public_key = res.data.public_key
    realitySpec.short_id = res.data.short_id
  } catch (err: any) {
    toast.error('生成密钥失败: ' + (err.response?.data?.error || err.message))
  }
}

function onProtocolChange() {
  if (form.protocol === 'vless-reality') {
    form.listen = '::'
    form.port = 443
    if (!modal.isEdit) {
      form.tag = 'vless-reality-443'
    }
  } else {
    form.listen = '127.0.0.1'
    form.port = 10086
    if (!modal.isEdit) {
      form.tag = 'vless-ws-10086'
    }
  }
}

async function submitInbound() {
  let specObj: any = {}
  if (form.protocol === 'vless-reality') {
    if (!realitySpec.server_name) {
      realitySpec.server_name = 'dl.google.com'
    }
    if (!realitySpec.handshake_server) {
      realitySpec.handshake_server = realitySpec.server_name
    }
    specObj = realitySpec
  } else {
    specObj = {
      path: wsSpec.path || '/ws',
      group_domain: wsSpec.group_domain || '',
      host: wsSpec.group_domain || '',
    }
  }

  try {
    const payload = {
      tag: form.tag,
      protocol: form.protocol,
      listen: form.listen || (form.protocol === 'vless-ws' ? '127.0.0.1' : '::'),
      port: form.port,
      spec: specObj,
      enabled: form.enabled,
    }

    if (modal.isEdit) {
      await api.put(`/inbounds/${modal.id}`, payload)
      toast.success('入站规则已更新并重载！')
    } else {
      await api.post('/inbounds', payload)
      toast.success('入站节点已创建并重载！')
    }
    modal.show = false
    await loadInbounds()
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteInbound(id: number) {
  const confirmed = await showConfirm({
    title: '确认删除入站节点',
    message: '确定要删除该入站节点吗？删除后 sing-box 配置将自动重写并热重载生效。',
    confirmText: '确认删除',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.delete(`/inbounds/${id}`)
    toast.success('节点已成功删除！')
    await loadInbounds()
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

async function showShareModal(ib: any) {
  try {
    const membersRes = await api.get('/members')
    const members = membersRes.data || []
    if (members.length === 0) {
      toast.warning('请先在「成员与订阅」中创建至少一个成员账号！')
      return
    }
    const m = members[0]
    const spec = parseSpec(ib.spec_json)

    const sysRes = await api.get('/system/overview')
    const ip = sysRes.data.public_ip

    const q = new URLSearchParams({
      encryption: 'none',
      flow: spec.flow || 'xtls-rprx-vision',
      security: 'reality',
      sni: spec.server_name,
      fp: spec.fingerprint || 'chrome',
      pbk: spec.public_key,
      sid: spec.short_id,
      type: 'tcp',
    })

    const uri = `vless://${m.uuid}@${ip}:${ib.port}?${q.toString()}#${encodeURIComponent(ib.tag)}`
    shareModal.title = `${ib.tag} (默认用户: ${m.name})`
    shareModal.uri = uri
    shareModal.show = true
  } catch (err: any) {
    toast.error('获取节点信息失败: ' + (err.response?.data?.error || err.message))
  }
}

function copyShareUri() {
  copyToClipboard(shareModal.uri, '节点链接')
}

onMounted(() => {
  loadInbounds()
  window.addEventListener('node-context-changed', loadInbounds)
})

onUnmounted(() => {
  window.removeEventListener('node-context-changed', loadInbounds)
})
</script>
