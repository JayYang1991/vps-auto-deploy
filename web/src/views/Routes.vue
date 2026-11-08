<template>
  <div>
    <!-- 头部操作与说明 -->
    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.5rem; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">🔀</span>
          <h2 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">Sing-box 路由分流规则 (Route Rules)</h2>
          <span class="badge badge-primary" style="font-size: 0.72rem;">Sing-box 1.14 规范</span>
        </div>
        <p style="color: #94a3b8; font-size: 0.85rem; margin-top: 0.25rem;">
          流量进入节点后，按自上而下的优先级顺序（首个命中生效，First-Match-Win）调度至对应出口或拦截。
        </p>
      </div>

      <div style="display: flex; gap: 0.75rem;">
        <button @click="openCreateModal()" class="btn btn-primary">+ 新增路由规则</button>
      </div>
    </div>

    <!-- 未安装 sing-box 渐进式空状态引导卡片 -->
    <div v-if="!loading && !singboxInstalled" class="card" style="margin-bottom: 1.5rem; background: rgba(245, 158, 11, 0.08); border-color: rgba(245, 158, 11, 0.35);">
      <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 1rem;">
        <div style="display: flex; align-items: center; gap: 0.75rem;">
          <span style="font-size: 1.6rem;">📦</span>
          <div>
            <div style="font-weight: 600; color: #fbbf24; font-size: 0.95rem;">未检测到已安装的 sing-box 代理核心</div>
            <div style="color: #94a3b8; font-size: 0.825rem; margin-top: 0.25rem;">
              路由分流规则由底层的 sing-box 核心驱动。您可先预先配置规则，或前往软件中心一键安装。
            </div>
          </div>
        </div>
        <router-link to="/components?install=sing-box" class="btn btn-primary" style="font-size: 0.825rem; padding: 0.4rem 0.85rem;">
          🚀 前往「软件中心」一键安装
        </router-link>
      </div>
    </div>

    <!-- 常用推荐预设胶囊栏 (One-Click Smart Presets) -->
    <div class="card" style="margin-bottom: 1.5rem; background: rgba(30, 41, 59, 0.4); border-color: #334155; padding: 0.85rem 1.15rem;">
      <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.75rem;">
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.1rem;">💡</span>
          <span style="font-size: 0.825rem; font-weight: 600; color: #cbd5e1;">常用工业级预设 (点击一键注入):</span>
        </div>
        <div style="display: flex; flex-wrap: wrap; gap: 0.5rem;">
          <button
            v-for="preset in presets"
            :key="preset.name"
            @click="applyPreset(preset)"
            class="btn btn-secondary preset-chip"
            type="button"
            :title="preset.desc"
          >
            <span>{{ preset.icon }}</span>
            <span>{{ preset.name }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- 加载中指示器 -->
    <div v-if="loading" class="card loading-box">
      <div class="loading-spinner"></div>
      <p class="loading-text">正在加载路由分流规则...</p>
    </div>

    <!-- 规则列表空状态 -->
    <div v-else-if="routes.length === 0" class="card" style="text-align: center; padding: 3rem;">
      <div style="font-size: 2.25rem; margin-bottom: 0.5rem;">🔀</div>
      <p style="color: #94a3b8; margin-bottom: 1.25rem;">暂无已配置的路由分流规则，点击上方预设或按钮添加。</p>
      <button @click="openCreateModal()" class="btn btn-primary">+ 添加路由规则</button>
    </div>

    <!-- 规则列表 -->
    <div v-else style="display: flex; flex-direction: column; gap: 0.85rem;">
      <div
        v-for="(r, index) in routes"
        :key="r.id"
        class="card route-card"
        :style="{ opacity: r.enabled ? '1' : '0.65' }"
      >
        <div style="display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem;">
          <!-- 规则主要信息 -->
          <div style="flex: 1; min-width: 280px;">
            <div style="display: flex; align-items: center; gap: 0.6rem; flex-wrap: wrap;">
              <!-- 优先级序号 -->
              <span class="priority-badge" :title="'执行次序: 第 ' + (index + 1) + ' 位 (优先级权重: ' + r.priority + ')'">
                #{{ index + 1 }}
              </span>

              <span style="font-size: 1.05rem; font-weight: 600; color: #f8fafc;">{{ r.name }}</span>

              <!-- 动作徽标 -->
              <span v-if="r.action === 'reject' || r.outbound_tag === 'block'" class="badge badge-danger">
                🚫 阻断 (reject)
              </span>
              <span v-else-if="r.outbound_tag === 'direct'" class="badge badge-success">
                ⚡ 本机直连 (direct)
              </span>
              <span v-else class="badge badge-primary">
                🚀 转发出站: {{ r.outbound_tag }}
              </span>

              <!-- 启停状态 -->
              <span :class="['badge', r.enabled ? 'badge-success' : 'badge-secondary']">
                {{ r.enabled ? '已启用' : '已停用' }}
              </span>

              <span v-if="r.invert" class="badge badge-warning" style="font-size: 0.68rem;">
                🔄 反向匹配
              </span>
            </div>

            <!-- 规则匹配条件展示标签胶囊 -->
            <div class="conditions-container">
              <span v-if="r.ip_is_private" class="cond-tag cond-tag-net">
                🔒 私有局域网放行 (RFC 1918)
              </span>

              <span v-if="r.protocol" class="cond-tag cond-tag-proto">
                嗅探: {{ r.protocol }}
              </span>

              <span v-if="r.rule_set" class="cond-tag cond-tag-rs">
                规则集: {{ r.rule_set }}
              </span>

              <span v-if="r.domain_suffix" class="cond-tag cond-tag-domain">
                域名后缀: {{ r.domain_suffix }}
              </span>

              <span v-if="r.domain_keyword" class="cond-tag cond-tag-domain">
                关键词: {{ r.domain_keyword }}
              </span>

              <span v-if="r.domains" class="cond-tag cond-tag-domain">
                完整域名: {{ r.domains }}
              </span>

              <span v-if="r.ip_cidr" class="cond-tag cond-tag-net">
                CIDR: {{ r.ip_cidr }}
              </span>

              <span v-if="r.port || r.port_range" class="cond-tag cond-tag-proto">
                端口: {{ [r.port, r.port_range].filter(Boolean).join(', ') }}
              </span>

              <span v-if="r.inbound_tags" class="cond-tag cond-tag-inbound">
                限定入站: {{ r.inbound_tags }}
              </span>
            </div>
          </div>

          <!-- 操作按钮组 -->
          <div style="display: flex; align-items: center; gap: 0.4rem; flex-wrap: wrap;">
            <button
              @click="moveRoute(index, -1)"
              :disabled="index === 0 || reordering"
              class="btn btn-secondary icon-btn"
              title="上移优先级"
            >
              ↑
            </button>
            <button
              @click="moveRoute(index, 1)"
              :disabled="index === routes.length - 1 || reordering"
              class="btn btn-secondary icon-btn"
              title="下移优先级"
            >
              ↓
            </button>
            <button
              @click="toggleRoute(r)"
              class="btn btn-secondary"
              style="padding: 0.35rem 0.65rem; font-size: 0.775rem;"
            >
              {{ r.enabled ? '⏸ 停用' : '▶ 启用' }}
            </button>
            <button
              @click="openEditModal(r)"
              class="btn btn-secondary"
              style="padding: 0.35rem 0.65rem; font-size: 0.775rem;"
            >
              ✏️ 编辑
            </button>
            <button
              @click="deleteRoute(r)"
              class="btn btn-danger"
              style="padding: 0.35rem 0.65rem; font-size: 0.775rem;"
            >
              🗑️ 删除
            </button>
          </div>
        </div>
      </div>

      <!-- 底部兜底出口提示 -->
      <div class="card final-outbound-bar">
        <div style="display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 0.5rem;">
          <div style="display: flex; align-items: center; gap: 0.6rem;">
            <span style="font-size: 1.15rem;">🏁</span>
            <div>
              <span style="font-weight: 600; color: #f8fafc; font-size: 0.875rem;">默认兜底出口 (Final Outbound):</span>
              <span style="margin-left: 0.5rem; color: #34d399; font-family: monospace; font-weight: 600;">⚡ direct (本机公网网卡直连)</span>
            </div>
          </div>
          <span style="font-size: 0.75rem; color: #94a3b8;">未命中上方任何规则时的默认动作</span>
        </div>
      </div>
    </div>

    <!-- 创建 / 编辑路由规则弹窗 -->
    <div v-if="modal.show" class="modal-overlay" @click.self="modal.show = false">
      <div class="modal-content" style="max-width: 680px; max-height: 90vh; overflow-y: auto;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.25rem;">
          <h3 style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">
            {{ modal.isEdit ? '编辑路由分流规则' : '新增路由分流规则' }}
          </h3>
          <button @click="modal.show = false" class="btn btn-secondary" style="padding: 0.2rem 0.5rem; font-size: 0.8rem;">✕</button>
        </div>

        <form @submit.prevent="saveRoute">
          <div style="display: flex; flex-direction: column; gap: 1rem;">
            <!-- 基本信息 -->
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0.75rem;">
              <div>
                <label class="form-label">规则名称 <span style="color: #ef4444;">*</span></label>
                <input v-model="form.name" class="input" placeholder="例如: 阻断 BT 版权下载 / AI 流媒体中转" required />
              </div>
              <div>
                <label class="form-label">执行优先级权重</label>
                <input v-model.number="form.priority" type="number" class="input" min="1" placeholder="10" />
              </div>
            </div>

            <!-- 出口与动作选择 -->
            <div style="background-color: #0f172a; padding: 1rem; border-radius: 0.5rem; border: 1px solid #1e293b;">
              <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
                <div>
                  <label class="form-label">匹配动作 (Action)</label>
                  <select v-model="form.action" class="input" @change="onActionChange">
                    <option value="route">route (转发到出站)</option>
                    <option value="reject">reject (直接阻断拦截)</option>
                  </select>
                </div>
                <div>
                  <label class="form-label">目标出站出口 (Target Outbound)</label>
                  <select v-model="form.outbound_tag" class="input" :disabled="form.action === 'reject'">
                    <option value="direct">⚡ direct (本机公网直接出站)</option>
                    <option value="block">🚫 block (静默丢弃/阻断)</option>
                    <optgroup v-if="customOutbounds.length > 0" label="已配置的出站/中转代理">
                      <option v-for="ob in customOutbounds" :key="ob.tag" :value="ob.tag">
                        🚀 {{ ob.tag }} ({{ ob.type }})
                      </option>
                    </optgroup>
                  </select>
                </div>
              </div>
            </div>

            <!-- 条件积木区 -->
            <div style="border-top: 1px solid #1e293b; padding-top: 0.75rem;">
              <h4 style="font-size: 0.9rem; font-weight: 600; color: #94a3b8; margin-bottom: 0.75rem;">
                匹配条件配置 (多条件交集匹配)
              </h4>

              <div style="display: flex; flex-direction: column; gap: 0.85rem;">
                <!-- 1. 规则集选择 (Sing-box 1.14 Rule Set) -->
                <div>
                  <label class="form-label">
                    📦 Sing-box 1.14 规则集 (Rule-Set)
                    <span style="font-size: 0.72rem; color: #64748b; font-weight: normal; margin-left: 0.4rem;">
                      (支持逗号分隔，如 geosite-cn, geoip-cn)
                    </span>
                  </label>
                  <!-- 快捷选择胶囊 -->
                  <div style="display: flex; flex-wrap: wrap; gap: 0.35rem; margin-bottom: 0.4rem;">
                    <span
                      v-for="rs in popularRuleSets"
                      :key="rs.tag"
                      @click="appendRuleSet(rs.tag)"
                      class="badge badge-secondary ruleset-chip"
                      :title="'点击追加 ' + rs.tag"
                    >
                      + {{ rs.label }}
                    </span>
                  </div>
                  <input v-model="form.rule_set" class="input" placeholder="例如: geosite-category-ads-all, geosite-cn, geoip-cn" />
                </div>

                <!-- 2. 域名规则 -->
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
                  <div>
                    <label class="form-label">域名后缀 (Domain Suffix)</label>
                    <input v-model="form.domain_suffix" class="input" placeholder="例如: google.com, openai.com" />
                  </div>
                  <div>
                    <label class="form-label">域名关键词 (Domain Keyword)</label>
                    <input v-model="form.domain_keyword" class="input" placeholder="例如: netflix, spotify" />
                  </div>
                </div>

                <div>
                  <label class="form-label">完整域名匹配 (Exact Domain)</label>
                  <input v-model="form.domains" class="input" placeholder="例如: dl.google.com, chatgpt.com" />
                </div>

                <!-- 3. IP 与网段 -->
                <div style="display: grid; grid-template-columns: 1fr 2fr; gap: 0.75rem; align-items: center;">
                  <div style="padding-top: 1.25rem;">
                    <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer; color: #cbd5e1; font-size: 0.825rem;">
                      <input type="checkbox" v-model="form.ip_is_private" />
                      <span>🔒 私网 IP 放行</span>
                    </label>
                  </div>
                  <div>
                    <label class="form-label">IP / CIDR 网段列表</label>
                    <input v-model="form.ip_cidr" class="input" placeholder="例如: 10.0.0.0/8, 192.168.1.0/24" />
                  </div>
                </div>

                <!-- 4. 协议与端口 -->
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
                  <div>
                    <label class="form-label">嗅探协议 (Protocol)</label>
                    <input v-model="form.protocol" class="input" placeholder="例如: bittorrent, tls, http" />
                  </div>
                  <div>
                    <label class="form-label">目标端口 / 端口范围</label>
                    <input v-model="form.port" class="input" placeholder="例如: 80, 443 或范围 8000:9000" />
                  </div>
                </div>

                <!-- 5. 入站限定与反向匹配 -->
                <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem;">
                  <div>
                    <label class="form-label">限定入站节点 (Inbound Tags，留空为全部)</label>
                    <input v-model="form.inbound_tags" class="input" placeholder="例如: vless-in, socks-in" />
                  </div>
                  <div style="display: flex; align-items: center; gap: 1rem; padding-top: 1.4rem;">
                    <label style="display: flex; align-items: center; gap: 0.4rem; cursor: pointer; color: #cbd5e1; font-size: 0.825rem;">
                      <input type="checkbox" v-model="form.invert" />
                      <span>🔄 反向匹配 (Invert)</span>
                    </label>
                    <label style="display: flex; align-items: center; gap: 0.4rem; cursor: pointer; color: #cbd5e1; font-size: 0.825rem;">
                      <input type="checkbox" v-model="form.enabled" />
                      <span>立即启用</span>
                    </label>
                  </div>
                </div>

                <!-- 6. 高级自定义规则集 URL (折叠) -->
                <div>
                  <details style="font-size: 0.8rem; color: #94a3b8; background: #0f172a; padding: 0.5rem 0.75rem; border-radius: 0.4rem; border: 1px solid #1e293b;">
                    <summary style="cursor: pointer; color: #38bdf8;">⚙️ 自定义规则集远程 URL (可选高级映射)</summary>
                    <div style="margin-top: 0.5rem;">
                      <label style="display: block; font-size: 0.72rem; color: #64748b; margin-bottom: 0.25rem;">
                        每行一个格式: tag=https://.../*.srs (留空将自动使用 SagerNet 官方官方二进制规则源)
                      </label>
                      <textarea
                        v-model="form.rule_set_urls"
                        class="input"
                        rows="3"
                        placeholder="my-rules=https://example.com/rules.srs"
                        style="font-family: monospace; font-size: 0.75rem;"
                      ></textarea>
                    </div>
                  </details>
                </div>
              </div>
            </div>
          </div>

          <!-- 底部按钮 -->
          <div style="display: flex; justify-content: flex-end; gap: 0.75rem; margin-top: 1.5rem; border-top: 1px solid #1e293b; padding-top: 1rem;">
            <button type="button" @click="modal.show = false" class="btn btn-secondary">取消</button>
            <button type="submit" class="btn btn-primary" :disabled="saving">
              {{ saving ? '保存中...' : '保存路由规则' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import api from '../api'
import { useToast } from '../composables/useToast'
import { useConfirm } from '../composables/useConfirm'

interface RouteRuleItem {
  id: number
  name: string
  action: string
  outbound_tag: string
  inbound_tags: string
  domains: string
  domain_suffix: string
  domain_keyword: string
  ip_cidr: string
  ip_is_private: boolean
  port: string
  port_range: string
  protocol: string
  rule_set: string
  rule_set_urls: string
  invert: boolean
  priority: number
  enabled: boolean
}

interface OutboundItem {
  id: number
  tag: string
  type: string
  enabled: boolean
  is_system: boolean
}

const { toast } = useToast()
const { showConfirm } = useConfirm()

const routes = ref<RouteRuleItem[]>([])
const outbounds = ref<OutboundItem[]>([])
const loading = ref(true)
const saving = ref(false)
const reordering = ref(false)
const singboxInstalled = ref(true)

const customOutbounds = computed(() => {
  return outbounds.value.filter((o) => o.tag !== 'direct' && o.tag !== 'block')
})

const popularRuleSets = [
  { tag: 'geosite-category-ads-all', label: '广告拦截' },
  { tag: 'geosite-cn', label: '国内域名' },
  { tag: 'geoip-cn', label: '国内 IP' },
  { tag: 'geosite-openai', label: 'OpenAI/ChatGPT' },
  { tag: 'geosite-netflix', label: 'Netflix' },
  { tag: 'geosite-youtube', label: 'YouTube' },
  { tag: 'geosite-telegram', label: 'Telegram' },
  { tag: 'geosite-github', label: 'GitHub' },
  { tag: 'geosite-geolocation-!cn', label: '非中国区网站' },
]

const presets = [
  {
    icon: '🛡️',
    name: '阻断 BT 下载',
    desc: '嗅探并拦截 BitTorrent 流量，防止国外 VPS 收到版权投诉封机',
    rule: {
      name: '阻断 BT/P2P 下载 (防版权封机)',
      action: 'reject',
      outbound_tag: 'block',
      protocol: 'bittorrent',
      priority: 2,
      enabled: true,
    },
  },
  {
    icon: '🔒',
    name: '私网放行',
    desc: '局域网 RFC 1918 私有地址直接放行直连',
    rule: {
      name: '私有网络直连',
      action: 'route',
      outbound_tag: 'direct',
      ip_is_private: true,
      priority: 1,
      enabled: true,
    },
  },
  {
    icon: '🚫',
    name: '拦截广告',
    desc: '基于 geosite-category-ads-all 拦截常见广告与恶意追踪',
    rule: {
      name: '广告与追踪流量拦截',
      action: 'reject',
      outbound_tag: 'block',
      rule_set: 'geosite-category-ads-all',
      priority: 3,
      enabled: true,
    },
  },
  {
    icon: '🇨🇳',
    name: '国内直连',
    desc: '国内常见域名与 IP 地址直接走本机直连出口',
    rule: {
      name: '国内域名与 IP 直连',
      action: 'route',
      outbound_tag: 'direct',
      rule_set: 'geosite-cn, geoip-cn',
      domain_suffix: 'cn, com.cn',
      priority: 9,
      enabled: true,
    },
  },
  {
    icon: '🤖',
    name: 'AI/流媒体专线',
    desc: '将 OpenAI、Claude 与流媒体调度至指定落地节点',
    rule: {
      name: 'AI 与流媒体专线中转',
      action: 'route',
      outbound_tag: 'direct',
      rule_set: 'geosite-openai, geosite-netflix, geosite-youtube',
      priority: 5,
      enabled: true,
    },
  },
]

const modal = reactive({
  show: false,
  isEdit: false,
  editId: 0,
})

const form = reactive({
  name: '',
  action: 'route',
  outbound_tag: 'direct',
  inbound_tags: '',
  domains: '',
  domain_suffix: '',
  domain_keyword: '',
  ip_cidr: '',
  ip_is_private: false,
  port: '',
  port_range: '',
  protocol: '',
  rule_set: '',
  rule_set_urls: '',
  invert: false,
  priority: 10,
  enabled: true,
})

function onActionChange() {
  if (form.action === 'reject') {
    form.outbound_tag = 'block'
  } else if (form.outbound_tag === 'block') {
    form.outbound_tag = 'direct'
  }
}

function appendRuleSet(tag: string) {
  const current = form.rule_set.split(',').map((s) => s.trim()).filter(Boolean)
  if (!current.includes(tag)) {
    current.push(tag)
    form.rule_set = current.join(', ')
  }
}

function openCreateModal() {
  modal.isEdit = false
  modal.editId = 0
  Object.assign(form, {
    name: '',
    action: 'route',
    outbound_tag: 'direct',
    inbound_tags: '',
    domains: '',
    domain_suffix: '',
    domain_keyword: '',
    ip_cidr: '',
    ip_is_private: false,
    port: '',
    port_range: '',
    protocol: '',
    rule_set: '',
    rule_set_urls: '',
    invert: false,
    priority: (routes.value.length + 1) * 2,
    enabled: true,
  })
  modal.show = true
}

function openEditModal(r: RouteRuleItem) {
  modal.isEdit = true
  modal.editId = r.id
  Object.assign(form, {
    name: r.name,
    action: r.action || (r.outbound_tag === 'block' ? 'reject' : 'route'),
    outbound_tag: r.outbound_tag,
    inbound_tags: r.inbound_tags,
    domains: r.domains,
    domain_suffix: r.domain_suffix,
    domain_keyword: r.domain_keyword,
    ip_cidr: r.ip_cidr,
    ip_is_private: r.ip_is_private,
    port: r.port,
    port_range: r.port_range,
    protocol: r.protocol,
    rule_set: r.rule_set,
    rule_set_urls: r.rule_set_urls,
    invert: r.invert,
    priority: r.priority,
    enabled: r.enabled,
  })
  modal.show = true
}

async function applyPreset(preset: any) {
  openCreateModal()
  Object.assign(form, preset.rule)
  // 如果是中转预设且有自定义中转出站，默认选中首个自定义出站
  if (preset.name.includes('AI') && customOutbounds.value.length > 0) {
    form.outbound_tag = customOutbounds.value[0].tag
  }
}

async function loadData() {
  loading.value = true
  try {
    const [compRes, routesRes, obRes] = await Promise.all([
      api.get('/components'),
      api.get('/routes'),
      api.get('/outbounds'),
    ])

    const comps = compRes.data || []
    const sb = comps.find((c: any) => c.name === 'sing-box')
    singboxInstalled.value = sb ? sb.installed : true

    routes.value = routesRes.data || []
    outbounds.value = obRes.data || []
  } catch (err: any) {
    toast.error('加载路由数据失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

async function saveRoute() {
  if (!form.name.trim()) {
    toast.warning('请填写规则名称')
    return
  }

  saving.value = true
  try {
    const payload = {
      ...form,
      name: form.name.trim(),
      outbound_tag: form.outbound_tag,
      action: form.action,
    }

    if (modal.isEdit) {
      await api.put(`/routes/${modal.editId}`, payload)
      toast.success('路由规则修改成功')
    } else {
      await api.post('/routes', payload)
      toast.success('路由规则创建成功')
    }
    modal.show = false
    await loadData()
  } catch (err: any) {
    toast.error('保存失败: ' + (err.response?.data?.error || err.message))
  } finally {
    saving.value = false
  }
}

async function toggleRoute(r: RouteRuleItem) {
  try {
    const res = await api.post(`/routes/${r.id}/toggle`)
    r.enabled = res.data.enabled
    toast.success(r.enabled ? `已启用规则: ${r.name}` : `已停用规则: ${r.name}`)
  } catch (err: any) {
    toast.error('切换状态失败: ' + (err.response?.data?.error || err.message))
  }
}

async function deleteRoute(r: RouteRuleItem) {
  const ok = await showConfirm({
    title: '确认删除路由规则',
    message: `确定要删除规则「${r.name}」吗？删除后将立即从 Sing-box 配置中移除。`,
    confirmText: '确认删除',
    danger: true,
  })
  if (!ok) return

  try {
    await api.delete(`/routes/${r.id}`)
    toast.success('规则已删除')
    await loadData()
  } catch (err: any) {
    toast.error('删除失败: ' + (err.response?.data?.error || err.message))
  }
}

async function moveRoute(index: number, delta: number) {
  const targetIndex = index + delta
  if (targetIndex < 0 || targetIndex >= routes.value.length) return

  const newOrder = [...routes.value]
  const temp = newOrder[index]
  newOrder[index] = newOrder[targetIndex]
  newOrder[targetIndex] = temp

  routes.value = newOrder
  reordering.value = true

  try {
    const ids = newOrder.map((item) => item.id)
    const res = await api.post('/routes/reorder', { ids })
    routes.value = res.data || newOrder
    toast.success('优先级排序已更新')
  } catch (err: any) {
    toast.error('更新排序失败: ' + (err.response?.data?.error || err.message))
    await loadData()
  } finally {
    reordering.value = false
  }
}

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.route-card {
  transition: all 0.2s ease;
  border-left: 3px solid #38bdf8;
}

.route-card:hover {
  border-color: #0284c7;
}

.priority-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background-color: #0f172a;
  border: 1px solid #334155;
  color: #38bdf8;
  font-family: monospace;
  font-weight: 700;
  font-size: 0.8rem;
}

.preset-chip {
  font-size: 0.775rem;
  padding: 0.3rem 0.65rem;
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  transition: all 0.15s ease;
}

.preset-chip:hover {
  border-color: #38bdf8;
  color: #f8fafc;
}

.ruleset-chip {
  cursor: pointer;
  font-size: 0.72rem;
  padding: 0.25rem 0.5rem;
  transition: all 0.15s ease;
}

.ruleset-chip:hover {
  background-color: #0284c7;
  color: #ffffff;
}

.conditions-container {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
  margin-top: 0.6rem;
}

.cond-tag {
  font-size: 0.75rem;
  padding: 0.2rem 0.55rem;
  border-radius: 0.375rem;
  font-family: monospace;
  display: inline-flex;
  align-items: center;
}

.cond-tag-net {
  background: rgba(16, 185, 129, 0.12);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.25);
}

.cond-tag-proto {
  background: rgba(239, 68, 68, 0.12);
  color: #f87171;
  border: 1px solid rgba(239, 68, 68, 0.25);
}

.cond-tag-rs {
  background: rgba(168, 85, 247, 0.12);
  color: #c084fc;
  border: 1px solid rgba(168, 85, 247, 0.25);
}

.cond-tag-domain {
  background: rgba(56, 189, 248, 0.12);
  color: #38bdf8;
  border: 1px solid rgba(56, 189, 248, 0.25);
}

.cond-tag-inbound {
  background: rgba(245, 158, 11, 0.12);
  color: #fbbf24;
  border: 1px solid rgba(245, 158, 11, 0.25);
}

.icon-btn {
  padding: 0.35rem 0.6rem;
  font-size: 0.85rem;
  font-weight: 700;
}

.final-outbound-bar {
  background: #0f172a;
  border: 1px dashed #334155;
  padding: 0.85rem 1.25rem;
  border-radius: 0.5rem;
}

.form-label {
  display: block;
  font-size: 0.75rem;
  color: #94a3b8;
  margin-bottom: 0.25rem;
}
</style>
