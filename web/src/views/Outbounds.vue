<template>
  <div>
    <!-- 头部操作与说明 -->
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">📤</span>
          <h2 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">Sing-box 出站规则配置 (Outbounds)</h2>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.25rem;">
          配置服务端网络出口流向，支持本机直连 (Direct)、广告拦截 (Block)、SOCKS5/WARP 中继或链式多跳转发。
        </p>
      </div>

      <div style="display: flex; gap: 0.75rem;">
        <button @click="openCreateModal" class="btn btn-primary">+ 新增出站规则</button>
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
              出站规则由底层的 sing-box 核心驱动。您可先预先配置规则，或前往软件中心一键安装。
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
      <p class="loading-text">正在加载出站规则配置...</p>
    </div>

    <!-- 出站规则列表空状态 -->
    <div v-else-if="outbounds.length === 0" class="card" style="text-align: center; padding: 3rem;">
      <div style="font-size: 2.25rem; margin-bottom: 0.5rem;">📤</div>
      <p style="color: #94a3b8; margin-bottom: 1.25rem;">暂无已配置的出站规则，点击下方按钮添加出站规则。</p>
      <button @click="openCreateModal" class="btn btn-primary">+ 添加出站规则</button>
    </div>

    <div v-else style="display: flex; flex-direction: column; gap: 1rem;">
      <div
        v-for="ob in outbounds"
        :key="ob.id"
        class="card"
        style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;"
      >
        <div>
          <div style="display: flex; align-items: center; gap: 0.75rem;">
            <span style="font-size: 1.1rem; font-weight: 600; color: #f8fafc;">{{ ob.tag }}</span>
            <span :class="['badge', getTypeBadge(ob.type)]">
              {{ ob.type }}
            </span>
            <span v-if="ob.is_system" class="badge badge-secondary" style="font-size: 0.68rem;">
              系统预置
            </span>
            <span :class="['badge', ob.enabled ? 'badge-success' : 'badge-secondary']">
              {{ ob.enabled ? '已启用' : '已停用' }}
            </span>
            <span style="color: #64748b; font-size: 0.75rem;">
              优先级: {{ ob.priority }}
            </span>
          </div>

          <div style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.5rem; display: flex; gap: 1.5rem; flex-wrap: wrap;">
            <span v-if="ob.server">
              上游地址: <b style="color: #38bdf8;">{{ ob.server }}:{{ ob.server_port }}</b>
            </span>
            <span v-else-if="ob.type === 'direct'">
              出口目标: <b style="color: #34d399;">本机公网网卡直接出站</b>
            </span>
            <span v-else-if="ob.type === 'block'">
              出口动作: <b style="color: #f87171;">静默丢弃 / 拦截</b>
            </span>
            <span v-else>
              出口参数: <b style="color: #f1f5f9;">自定义配置</b>
            </span>
          </div>
        </div>

        <div style="display: flex; align-items: center; gap: 0.6rem;">
          <!-- 快捷启停开关 -->
          <button
            @click="toggleOutbound(ob.id)"
            class="btn btn-secondary"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
            :title="ob.enabled ? '点击停用该出站' : '点击启用该出站'"
          >
            {{ ob.enabled ? '⏸ 停用' : '▶ 启用' }}
          </button>

          <!-- 编辑按钮 -->
          <button
            @click="openEditModal(ob)"
            class="btn btn-secondary"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
          >
            ✏️ 编辑
          </button>

          <!-- 删除按钮 (系统预置 direct/block 禁止删除) -->
          <button
            v-if="!ob.is_system"
            @click="deleteOutbound(ob)"
            class="btn btn-danger"
            style="padding: 0.4rem 0.75rem; font-size: 0.8rem;"
          >
            🗑️ 删除
          </button>
        </div>
      </div>
    </div>

    <!-- 新建/编辑出站规则弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content" style="max-width: 620px;">
        <h3 style="font-size: 1.25rem; font-weight: 600; margin-bottom: 1.25rem; color: #f8fafc;">
          {{ modal.isEdit ? '编辑出站规则' : '新建出站规则' }}
        </h3>

        <!-- 模式切换：仅在新建出站时提供一键中转选项 -->
        <div v-if="!modal.isEdit" style="display: flex; gap: 0.5rem; margin-bottom: 1.25rem; background: #0f172a; padding: 0.25rem; border-radius: 0.5rem;">
          <button
            type="button"
            :class="['btn', createMode === 'relay' ? 'btn-primary' : 'btn-secondary']"
            style="flex: 1; font-size: 0.825rem; padding: 0.45rem; justify-content: center;"
            @click="switchCreateMode('relay')"
          >
            ⚡ 从集群节点快速创建中转 (推荐)
          </button>
          <button
            type="button"
            :class="['btn', createMode === 'manual' ? 'btn-primary' : 'btn-secondary']"
            style="flex: 1; font-size: 0.825rem; padding: 0.45rem; justify-content: center;"
            @click="switchCreateMode('manual')"
          >
            🛠️ 手动输入模式
          </button>
        </div>

        <!-- 节点中转级联选择器工作区 -->
        <div v-if="!modal.isEdit && createMode === 'relay'" style="background: rgba(30, 41, 59, 0.7); border: 1px solid #334155; border-radius: 0.5rem; padding: 1rem; margin-bottom: 1rem; display: flex; flex-direction: column; gap: 0.85rem;">
          <div style="font-size: 0.85rem; font-weight: 600; color: #38bdf8; display: flex; align-items: center; gap: 0.4rem;">
            <span>🌐</span> 目标落地节点与入站协议选择 (自动排除本节点防环路)
          </div>

          <div v-if="loadingRelayCandidates" style="font-size: 0.8rem; color: #94a3b8; padding: 0.5rem 0;">
            ⏳ 正在探测全集群可用节点入站...
          </div>
          <div v-else-if="relayCandidates.length === 0" style="font-size: 0.8rem; color: #f59e0b; background: rgba(245, 158, 11, 0.1); padding: 0.75rem; border-radius: 0.35rem;">
            ⚠️ 当前集群中没有其他在线节点或未配置启用中的入站协议，请先在其他节点添加入站或切换至手动输入模式。
          </div>
          <div v-else style="display: flex; flex-direction: column; gap: 0.75rem;">
            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">1. 目标落地节点</label>
                <select v-model="selectedRelayNodeId" class="input" @change="onRelayNodeChange">
                  <option v-for="node in relayCandidates" :key="node.node_id" :value="node.node_id">
                    {{ node.node_name }} ({{ node.host }})
                  </option>
                </select>
              </div>

              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">2. 目标入站协议</label>
                <select v-model="selectedRelayInboundId" class="input" @change="fetchAndApplyRelayOutbound">
                  <option v-for="ib in currentRelayInbounds" :key="ib.id" :value="ib.id">
                    {{ ib.summary }}
                  </option>
                </select>
              </div>
            </div>

            <!-- 地址偏好：若关联了 Cloudflare 穿透隧道，允许二选一 -->
            <div v-if="selectedInboundHasTunnel" style="font-size: 0.8rem; color: #cbd5e1; display: flex; gap: 1.25rem; align-items: center; margin-top: 0.25rem;">
              <span style="color: #94a3b8;">连接偏好:</span>
              <label style="display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
                <input type="radio" v-model="selectedRelayAddrType" value="ip" @change="fetchAndApplyRelayOutbound" />
                公网直连 IP ({{ currentRelayNode?.host }})
              </label>
              <label style="display: flex; align-items: center; gap: 0.35rem; cursor: pointer;">
                <input type="radio" v-model="selectedRelayAddrType" value="tunnel" @change="fetchAndApplyRelayOutbound" />
                🚇 Cloudflare 穿透域名 ({{ selectedInbound?.tunnel_domain }})
              </label>
            </div>

            <div v-if="relayGeneratedSummary" style="background: rgba(16, 185, 129, 0.12); border: 1px solid rgba(16, 185, 129, 0.35); border-radius: 0.35rem; padding: 0.6rem 0.85rem; font-size: 0.8rem; color: #34d399;">
              ✅ {{ relayGeneratedSummary }} (已自动计算并注入加密密钥，可在下方查看参数或微调后保存)
            </div>
          </div>
        </div>

        <form @submit.prevent="submitOutbound" style="display: flex; flex-direction: column; gap: 1rem;">
          <div>
            <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
              出站标识 Tag (必须唯一)
            </label>
            <input
              v-model="form.tag"
              class="input"
              required
              :disabled="modal.isEdit && modal.isSystem"
              placeholder="如 direct, warp-out, socks-proxy"
            />
          </div>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 1rem;">
            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">出站协议类型</label>
              <select
                v-model="form.type"
                class="input"
                :disabled="modal.isEdit && modal.isSystem"
                @change="onTypeChange"
              >
                <option value="direct">Direct (本机直连出站)</option>
                <option value="block">Block (拦截阻断)</option>
                <option value="socks">SOCKS5 (上游前置代理 / WARP)</option>
                <option value="http">HTTP Proxy (上游 HTTP 代理)</option>
                <option value="shadowsocks">Shadowsocks (节点转发)</option>
                <option value="vless">VLESS (上游中继节点)</option>
                <option value="trojan">Trojan (上游 TLS 代理)</option>
                <option value="hysteria2">Hysteria 2 (上游 UDP 极速节点)</option>
                <option value="custom">Custom (高级自定义 JSON 片段)</option>
              </select>
            </div>

            <div>
              <label style="display: block; font-size: 0.85rem; color: #cbd5e1; margin-bottom: 0.35rem;">
                优先级权重 (Priority)
              </label>
              <input
                v-model.number="form.priority"
                type="number"
                class="input"
                min="1"
                max="999"
                required
                placeholder="1 (越小越优先)"
              />
              <span style="font-size: 0.72rem; color: #64748b; margin-top: 0.2rem; display: block;">
                排序在最前方的规则将作为默认缺省出站
              </span>
            </div>
          </div>

          <!-- SOCKS5 / HTTP 参数 -->
          <div
            v-if="form.type === 'socks' || form.type === 'http'"
            style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游服务器地址</label>
                <input v-model="form.server" class="input" placeholder="127.0.0.1 或 域名" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口</label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="1080" required />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">认证用户名 (可选)</label>
                <input v-model="proxySpec.username" class="input" placeholder="留空则无认证" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">认证密码 (可选)</label>
                <input v-model="proxySpec.password" type="password" class="input" placeholder="留空则无密码" />
              </div>
            </div>
          </div>

          <!-- Shadowsocks 参数 -->
          <div
            v-else-if="form.type === 'shadowsocks'"
            style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游 SS 服务器</label>
                <input v-model="form.server" class="input" placeholder="ss.example.com" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口</label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="8388" required />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">加密方法 (Method)</label>
                <select v-model="ssSpec.method" class="input">
                  <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm</option>
                  <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm</option>
                  <option value="aes-128-gcm">aes-128-gcm</option>
                  <option value="aes-256-gcm">aes-256-gcm</option>
                  <option value="chacha20-ietf-poly1305">chacha20-ietf-poly1305</option>
                </select>
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">密码 (Password)</label>
                <input v-model="ssSpec.password" class="input" required placeholder="SS 密码" />
              </div>
            </div>
          </div>

          <!-- VLESS 出站参数 -->
          <div
            v-else-if="form.type === 'vless'"
            style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游 VLESS 服务器</label>
                <input v-model="form.server" class="input" placeholder="vless.example.com" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口</label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="443" required />
              </div>
            </div>

            <div>
              <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">UUID</label>
              <input v-model="vlessSpec.uuid" class="input" required placeholder="用户 UUID" />
            </div>
          </div>

          <!-- Trojan / Hysteria 2 出站参数 -->
          <div
            v-else-if="form.type === 'trojan' || form.type === 'hysteria2'"
            style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.75rem;"
          >
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">上游服务器地址</label>
                <input v-model="form.server" class="input" placeholder="domain.com 或 IP" required />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">端口</label>
                <input v-model.number="form.server_port" type="number" class="input" placeholder="443" required />
              </div>
            </div>

            <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">密码 (Password)</label>
                <input v-model="extraSpec.password" class="input" required placeholder="节点认证密码" />
              </div>
              <div>
                <label style="display: block; font-size: 0.75rem; color: #94a3b8; margin-bottom: 0.25rem;">TLS SNI 域名 (可选)</label>
                <input v-model="extraSpec.server_name" class="input" placeholder="留空默认使用服务器地址" />
              </div>
            </div>
          </div>

          <!-- Custom 自定义 JSON 参数 -->
          <div
            v-else-if="form.type === 'custom'"
            style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; display: flex; flex-direction: column; gap: 0.5rem;"
          >
            <label style="display: block; font-size: 0.75rem; color: #94a3b8;">自定义出站 JSON 对象 (例如 WireGuard/WARP 配置)</label>
            <textarea
              v-model="customSpecJSON"
              class="input"
              rows="6"
              style="font-family: monospace; font-size: 0.75rem;"
              placeholder='{\n  "type": "wireguard",\n  "server": "162.159.192.1",\n  "server_port": 2408\n}'
            ></textarea>
          </div>

          <div style="display: flex; align-items: center; gap: 0.5rem; margin-top: 0.5rem;">
            <input type="checkbox" id="ob-enabled" v-model="form.enabled" />
            <label for="ob-enabled" style="font-size: 0.85rem; color: #cbd5e1; cursor: pointer;">
              立即启用该出站
            </label>
          </div>

          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary">取消</button>
            <button type="submit" class="btn btn-primary">保存出站规则</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import api from '../api'
import { useToast } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

interface OutboundItem {
  id: number
  tag: string
  type: string
  server: string
  server_port: number
  spec_json: string
  priority: number
  enabled: boolean
  is_system: boolean
}

interface RelayInbound {
  id: number
  tag: string
  protocol: string
  port: number
  spec_json: string
  tunnel_domain?: string
  summary: string
}

interface RelayNode {
  node_id: number
  node_name: string
  host: string
  inbounds: RelayInbound[]
}

const { toast } = useToast()
const { showConfirm } = useConfirm()

const outbounds = ref<OutboundItem[]>([])
const loading = ref(true)
const singboxInstalled = ref(true)

const createMode = ref<'relay' | 'manual'>('relay')
const relayCandidates = ref<RelayNode[]>([])
const loadingRelayCandidates = ref(false)
const selectedRelayNodeId = ref<number | null>(null)
const selectedRelayInboundId = ref<number | null>(null)
const selectedRelayAddrType = ref<'ip' | 'tunnel'>('ip')
const relayGeneratedSummary = ref('')
const relayGeneratedSpec = ref<any>(null)

const currentRelayNode = computed(() => {
  return relayCandidates.value.find(n => n.node_id === selectedRelayNodeId.value)
})

const currentRelayInbounds = computed(() => {
  return currentRelayNode.value ? currentRelayNode.value.inbounds : []
})

const selectedInbound = computed(() => {
  return currentRelayInbounds.value.find(ib => ib.id === selectedRelayInboundId.value)
})

const selectedInboundHasTunnel = computed(() => {
  return !!(selectedInbound.value && selectedInbound.value.tunnel_domain)
})

const modal = reactive({
  show: false,
  isEdit: false,
  isSystem: false,
  id: 0,
})

const form = reactive({
  tag: '',
  type: 'direct',
  server: '',
  server_port: 0,
  priority: 10,
  enabled: true,
})

const proxySpec = reactive({
  username: '',
  password: '',
})

const ssSpec = reactive({
  method: '2022-blake3-aes-128-gcm',
  password: '',
})

const vlessSpec = reactive({
  uuid: '',
})

const extraSpec = reactive({
  password: '',
  server_name: '',
})

const customSpecJSON = ref('')

async function loadOutbounds() {
  loading.value = true
  try {
    const res = await api.get('/outbounds')
    outbounds.value = res.data || []

    const compRes = await api.get('/components')
    const sb = (compRes.data || []).find((c: any) => c.name === 'sing-box')
    if (sb) {
      singboxInstalled.value = sb.installed
    }
  } catch (err: any) {
    toast.error('加载出站规则失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

function getTypeBadge(type: string) {
  switch (type) {
    case 'direct': return 'badge-success'
    case 'block': return 'badge-danger'
    case 'socks':
    case 'http': return 'badge-warning'
    case 'shadowsocks':
    case 'vless':
    case 'trojan':
    case 'hysteria2':
    case 'wireguard': return 'badge-primary'
    default: return 'badge-secondary'
  }
}

function onTypeChange() {
  if (form.type === 'direct' || form.type === 'block') {
    form.server = ''
    form.server_port = 0
  }
}

async function loadRelayCandidates() {
  loadingRelayCandidates.value = true
  try {
    const res = await api.get('/cluster/relay-candidates')
    relayCandidates.value = res.data || []
    if (relayCandidates.value.length > 0) {
      selectedRelayNodeId.value = relayCandidates.value[0].node_id
      if (relayCandidates.value[0].inbounds && relayCandidates.value[0].inbounds.length > 0) {
        selectedRelayInboundId.value = relayCandidates.value[0].inbounds[0].id
        await fetchAndApplyRelayOutbound()
      }
    } else {
      createMode.value = 'manual'
    }
  } catch (err: any) {
    createMode.value = 'manual'
  } finally {
    loadingRelayCandidates.value = false
  }
}

function onRelayNodeChange() {
  if (currentRelayInbounds.value.length > 0) {
    selectedRelayInboundId.value = currentRelayInbounds.value[0].id
    fetchAndApplyRelayOutbound()
  } else {
    selectedRelayInboundId.value = null
    relayGeneratedSummary.value = ''
    relayGeneratedSpec.value = null
  }
}

async function fetchAndApplyRelayOutbound() {
  if (!selectedRelayNodeId.value || !selectedRelayInboundId.value) return
  try {
    const res = await api.post('/cluster/build-relay-outbound', {
      target_node_id: selectedRelayNodeId.value,
      inbound_id: selectedRelayInboundId.value,
      addr_type: selectedRelayAddrType.value,
    })
    const generated = res.data
    form.tag = generated.tag
    form.type = generated.type
    form.server = generated.server
    form.server_port = generated.server_port
    form.priority = 10

    try {
      const parsed = JSON.parse(generated.spec_json || '{}')
      relayGeneratedSpec.value = parsed
      customSpecJSON.value = JSON.stringify(parsed, null, 2)
      if (parsed.uuid) vlessSpec.uuid = parsed.uuid
      if (parsed.password) {
        extraSpec.password = parsed.password
        ssSpec.password = parsed.password
      }
      if (parsed.method) ssSpec.method = parsed.method
      if (parsed.tls?.server_name) extraSpec.server_name = parsed.tls.server_name
    } catch {
      relayGeneratedSpec.value = null
    }

    const nodeName = currentRelayNode.value ? currentRelayNode.value.node_name : '目标节点'
    relayGeneratedSummary.value = `已自动从【${nodeName}】解析并注入 ${form.type.toUpperCase()} 中转协议与加密参数`
  } catch (err: any) {
    toast.error('解析中转配置失败: ' + (err.response?.data?.error || err.message))
  }
}

function switchCreateMode(mode: 'relay' | 'manual') {
  createMode.value = mode
  if (mode === 'relay') {
    if (relayCandidates.value.length === 0) {
      loadRelayCandidates()
    } else if (selectedRelayNodeId.value && selectedRelayInboundId.value) {
      fetchAndApplyRelayOutbound()
    }
  } else {
    relayGeneratedSummary.value = ''
    relayGeneratedSpec.value = null
  }
}

function openCreateModal() {
  modal.isEdit = false
  modal.isSystem = false
  modal.id = 0
  modal.show = true

  createMode.value = 'relay'
  relayGeneratedSummary.value = ''
  relayGeneratedSpec.value = null
  selectedRelayAddrType.value = 'ip'

  form.tag = ''
  form.type = 'vless'
  form.server = ''
  form.server_port = 443
  form.priority = 10
  form.enabled = true

  proxySpec.username = ''
  proxySpec.password = ''
  ssSpec.method = '2022-blake3-aes-128-gcm'
  ssSpec.password = ''
  vlessSpec.uuid = ''
  extraSpec.password = ''
  extraSpec.server_name = ''
  customSpecJSON.value = ''

  loadRelayCandidates()
}

function openEditModal(ob: OutboundItem) {
  modal.isEdit = true
  modal.isSystem = ob.is_system
  modal.id = ob.id
  modal.show = true

  form.tag = ob.tag
  form.type = ob.type
  form.server = ob.server
  form.server_port = ob.server_port
  form.priority = ob.priority
  form.enabled = ob.enabled

  proxySpec.username = ''
  proxySpec.password = ''
  ssSpec.method = '2022-blake3-aes-128-gcm'
  ssSpec.password = ''
  vlessSpec.uuid = ''
  extraSpec.password = ''
  extraSpec.server_name = ''
  customSpecJSON.value = ''

  try {
    const spec = JSON.parse(ob.spec_json || '{}')
    if (ob.type === 'socks' || ob.type === 'http') {
      proxySpec.username = spec.username || ''
      proxySpec.password = spec.password || ''
    } else if (ob.type === 'shadowsocks') {
      ssSpec.method = spec.method || '2022-blake3-aes-128-gcm'
      ssSpec.password = spec.password || ''
    } else if (ob.type === 'vless') {
      vlessSpec.uuid = spec.uuid || ''
    } else if (ob.type === 'trojan' || ob.type === 'hysteria2') {
      extraSpec.password = spec.password || ''
      extraSpec.server_name = spec.server_name || (spec.tls?.server_name) || ''
    } else if (ob.type === 'custom') {
      customSpecJSON.value = JSON.stringify(spec, null, 2)
    }
  } catch {
    customSpecJSON.value = ob.spec_json || ''
  }
}

async function submitOutbound() {
  let specObj: any = {}
  if (!modal.isEdit && createMode.value === 'relay' && relayGeneratedSpec.value && Object.keys(relayGeneratedSpec.value).length > 0) {
    // 采用中转助手自动解析生成的完备参数
    specObj = { ...relayGeneratedSpec.value }
    if (form.type === 'vless' && vlessSpec.uuid) specObj.uuid = vlessSpec.uuid
    if (form.type === 'shadowsocks') {
      if (ssSpec.password) specObj.password = ssSpec.password
      if (ssSpec.method) specObj.method = ssSpec.method
    }
    if (form.type === 'trojan' || form.type === 'hysteria2') {
      if (extraSpec.password) specObj.password = extraSpec.password
      if (extraSpec.server_name && specObj.tls) specObj.tls.server_name = extraSpec.server_name
    }
  } else if (form.type === 'socks' || form.type === 'http') {
    if (proxySpec.username) specObj.username = proxySpec.username
    if (proxySpec.password) specObj.password = proxySpec.password
  } else if (form.type === 'shadowsocks') {
    specObj.method = ssSpec.method
    specObj.password = ssSpec.password
  } else if (form.type === 'vless') {
    specObj.uuid = vlessSpec.uuid
  } else if (form.type === 'trojan' || form.type === 'hysteria2') {
    specObj.password = extraSpec.password
    if (extraSpec.server_name) {
      specObj.tls = {
        enabled: true,
        server_name: extraSpec.server_name,
        insecure: true,
      }
    }
  } else if (form.type === 'custom') {
    try {
      specObj = JSON.parse(customSpecJSON.value || '{}')
    } catch {
      toast.error('自定义参数必须是合法的 JSON 格式！')
      return
    }
  }

  const payload = {
    tag: form.tag,
    type: form.type,
    server: form.server,
    server_port: form.server_port,
    spec: specObj,
    priority: form.priority,
    enabled: form.enabled,
  }

  try {
    if (modal.isEdit) {
      await api.put(`/outbounds/${modal.id}`, payload)
      toast.success('出站规则已更新并热重载！')
    } else {
      await api.post('/outbounds', payload)
      toast.success('出站规则已添加并生效！')
    }
    modal.show = false
    await loadOutbounds()
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  }
}

async function toggleOutbound(id: number) {
  try {
    await api.post(`/outbounds/${id}/toggle`)
    toast.success('已切换出站状态并重载配置！')
    await loadOutbounds()
  } catch (err: any) {
    toast.error('切换状态失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteOutbound(ob: OutboundItem) {
  const confirmed = await showConfirm({
    title: '确认删除出站规则',
    message: `确定要删除出站规则 [${ob.tag}] 吗？删除后 sing-box 将自动重写配置并生效。`,
    confirmText: '确认删除',
    danger: true,
  })
  if (!confirmed) return

  try {
    await api.delete(`/outbounds/${ob.id}`)
    toast.success('出站规则已成功删除！')
    await loadOutbounds()
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  loadOutbounds()
  window.addEventListener('node-context-changed', loadOutbounds)
})

onUnmounted(() => {
  window.removeEventListener('node-context-changed', loadOutbounds)
})
</script>

