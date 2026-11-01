<template>
  <div>
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem;">
      <div>
        <h2 style="font-size: 1.25rem; font-weight: 600;">Sing-box 入站节点</h2>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.2rem;">
          支持直连首选的 VLESS-XTLS-Vision-Reality 及配合 Cloudflared 隧道防封的 VLESS-WS
        </p>
      </div>

      <div style="display: flex; gap: 0.75rem;">
        <button @click="openPreview" class="btn btn-secondary">🔍 预览 JSON 配置</button>
        <button @click="openCreateModal" class="btn btn-primary">+ 新增节点</button>
      </div>
    </div>

    <!-- 节点列表 -->
    <div v-if="inbounds.length === 0" class="card" style="text-align: center; padding: 3rem;">
      <div style="font-size: 2rem; margin-bottom: 0.5rem;">🌐</div>
      <p style="color: #94a3b8; margin-bottom: 1rem;">暂无已配置的入站节点，点击上方按钮一键创建 VLESS-Reality 节点。</p>
      <button @click="openCreateModal" class="btn btn-primary">+ 一键配置首个 VLESS-Reality 节点</button>
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 1rem;">
      <div v-for="ib in inbounds" :key="ib.id" class="card" style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div>
          <div style="display: flex; align-items: center; gap: 0.75rem;">
            <span style="font-size: 1.1rem; font-weight: 600;">{{ ib.tag }}</span>
            <span :class="['badge', ib.protocol === 'vless-reality' ? 'badge-success' : 'badge-warning']">
              {{ ib.protocol }}
            </span>
            <span :class="['badge', ib.enabled ? 'badge-success' : 'badge-secondary']">
              {{ ib.enabled ? '已启用' : '已停用' }}
            </span>
          </div>

          <div style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.5rem; display: flex; gap: 1.5rem;">
            <span>监听端口: <b style="color: #f1f5f9;">{{ ib.port }}</b></span>
            <span>绑定地址: <b style="color: #f1f5f9;">{{ ib.listen }}</b></span>
            <span v-if="ib.protocol === 'vless-reality'">
              SNI 伪装: <b style="color: #38bdf8;">{{ parseSpec(ib.spec_json).server_name }}</b>
            </span>
          </div>
        </div>

        <div style="display: flex; gap: 0.5rem;">
          <button @click="showShareModal(ib)" class="btn btn-secondary">
            📱 二维码 / 节点链接
          </button>
          <button @click="deleteInbound(ib.id)" class="btn btn-danger" style="padding: 0.5rem 0.75rem;">
            删除
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
                <input v-model="realitySpec.server_name" class="input" placeholder="www.microsoft.com" required />
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
              <label style="display: block; font-size: 0.85rem; color: #94a3b8; margin-bottom: 0.25rem;">WebSocket Path</label>
              <input v-model="wsSpec.path" class="input" placeholder="/ws" />
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
          <button @click="copyText(shareModal.uri)" class="btn btn-primary">复制节点链接</button>
          <button @click="shareModal.show = false" class="btn btn-secondary">关闭</button>
        </div>
      </div>
    </div>

    <!-- 配置预览弹窗 -->
    <div v-if="previewModal.show" class="modal-overlay" @click.self="previewModal.show = false">
      <div class="modal-content" style="max-width: 750px; max-height: 85vh; display: flex; flex-direction: column;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
          <h3 style="font-size: 1.125rem; font-weight: 600;">Sing-box 服务端完整配置预览</h3>
          <button @click="previewModal.show = false" class="btn btn-secondary" style="padding: 0.25rem 0.5rem;">关闭</button>
        </div>
        <pre style="flex: 1; background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; overflow-y: auto; font-size: 0.8rem; line-height: 1.4;">{{ previewModal.content }}</pre>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import QrcodeVue from 'qrcode.vue'
import api from '../api'

const inbounds = ref<any[]>([])

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
  server_name: 'www.microsoft.com',
  handshake_server: 'www.microsoft.com',
  handshake_port: 443,
  private_key: '',
  public_key: '',
  short_id: '',
  fingerprint: 'chrome',
})

const wsSpec = reactive({
  path: '/ws',
})

const shareModal = reactive({
  show: false,
  title: '',
  uri: '',
})

const previewModal = reactive({
  show: false,
  content: '',
})

async function loadInbounds() {
  try {
    const res = await api.get('/inbounds')
    inbounds.value = res.data || []
  } catch (err) {
    console.error(err)
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
  modal.show = true
  form.protocol = 'vless-reality'
  form.port = 443
  form.tag = 'vless-reality-443'
  form.enabled = true
  await generateRealityKeys()
}

async function generateRealityKeys() {
  try {
    const res = await api.post('/inbounds/reality-keys')
    realitySpec.private_key = res.data.private_key
    realitySpec.public_key = res.data.public_key
    realitySpec.short_id = res.data.short_id
  } catch (err) {
    console.error(err)
  }
}

function onProtocolChange() {
  if (form.protocol === 'vless-reality') {
    form.port = 443
    form.tag = 'vless-reality-443'
  } else {
    form.port = 10086
    form.tag = 'vless-ws-10086'
  }
}

async function submitInbound() {
  let specObj = {}
  if (form.protocol === 'vless-reality') {
    specObj = realitySpec
  } else {
    specObj = wsSpec
  }

  try {
    await api.post('/inbounds', {
      tag: form.tag,
      protocol: form.protocol,
      listen: form.protocol === 'vless-ws' ? '127.0.0.1' : '::',
      port: form.port,
      spec: specObj,
      enabled: form.enabled,
    })
    modal.show = false
    await loadInbounds()
  } catch (err: any) {
    alert('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteInbound(id: number) {
  if (!confirm('确定要删除该入站节点吗？删除后 sing-box 将自动重载生效。')) return
  try {
    await api.delete(`/inbounds/${id}`)
    await loadInbounds()
  } catch (err: any) {
    alert('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

async function showShareModal(ib: any) {
  try {
    const membersRes = await api.get('/members')
    const members = membersRes.data || []
    if (members.length === 0) {
      alert('请先在"成员与订阅"中创建至少一个成员！')
      return
    }
    const m = members[0]
    const spec = parseSpec(ib.spec_json)

    // 拼接 vless 协议链接
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
  } catch (err) {
    console.error(err)
  }
}

async function openPreview() {
  try {
    const res = await api.get('/inbounds/preview')
    previewModal.content = JSON.stringify(res.data, null, 2)
    previewModal.show = true
  } catch (err: any) {
    alert('获取预览配置失败: ' + (err.response?.data?.error || err.message))
  }
}

function copyText(txt: string) {
  navigator.clipboard.writeText(txt)
  alert('已复制到剪贴板！')
}

onMounted(() => {
  loadInbounds()
})
</script>

