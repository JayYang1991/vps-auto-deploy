<template>
  <div class="vultr-provisioner">
    <!-- 1. Vultr Token 状态栏 / 首次绑定卡片 -->
    <div v-if="!vultrConfig.configured || showUpdateToken" class="token-setup-card">
      <div class="setup-header">
        <div class="setup-icon">🔑</div>
        <div>
          <h4 class="setup-title">{{ vultrConfig.configured ? '更换 Vultr API Token' : '绑定 Vultr API Token' }}</h4>
          <p class="setup-desc">
            请前往 Vultr 控制台 (Account ➔ API) 生成具备 <strong>All Access</strong> 权限的 Personal Access Token
          </p>
        </div>
      </div>

      <div class="token-input-row">
        <input
          v-model="tokenInput"
          :type="showTokenPlain ? 'text' : 'password'"
          placeholder="在此粘贴您的 Vultr API Key (例如：42CHARACTERSLONGVULTRAPITOKEN...)"
          class="token-input mono"
        />
        <button class="btn-toggle-eye" @click="showTokenPlain = !showTokenPlain" type="button">
          {{ showTokenPlain ? '🙈' : '👁️' }}
        </button>
        <button class="btn-primary" @click="saveToken" :disabled="savingToken || !tokenInput.trim()">
          <span>{{ savingToken ? '⏳ 验证中...' : '🔗 验证并加密绑定' }}</span>
        </button>
        <button v-if="vultrConfig.configured" class="btn-secondary" @click="showUpdateToken = false">
          取消
        </button>
      </div>

      <div v-if="tokenError" class="token-error-msg">
        ⚠️ {{ tokenError }}
      </div>
    </div>

    <!-- 已经绑定 Token 时的信息胶囊条 -->
    <div v-else class="token-status-bar">
      <div class="status-left">
        <span class="cloud-icon">☁️</span>
        <div class="status-meta">
          <div class="meta-row">
            <span class="provider-badge">Vultr API</span>
            <span class="masked-key mono">{{ vultrConfig.masked_key }}</span>
            <span class="active-dot" title="Token 正常可用"></span>
          </div>
          <div v-if="vultrConfig.account" class="balance-text">
            <span>账户: <strong>{{ vultrConfig.account.email }}</strong></span>
            <span class="dot-divider">·</span>
            <span>可用余额: <strong class="color-success">${{ Math.abs(vultrConfig.account.balance).toFixed(2) }}</strong></span>
            <span class="dot-divider">·</span>
            <span>本月已发生: ${{ vultrConfig.account.pending_charges.toFixed(2) }}</span>
          </div>
        </div>
      </div>

      <div class="status-right">
        <button class="btn-mini-refresh" @click="refreshCatalog" :disabled="loadingCatalog" title="重新从 Vultr 同步区域与机型库存">
          <span>{{ loadingCatalog ? '🔄 正在同步...' : '🔄 刷新库存' }}</span>
        </button>
        <button class="btn-mini-update" @click="showUpdateToken = true">
          ⚙️ 更换 Token
        </button>
      </div>
    </div>

    <!-- 2. 主流程：创建表单 vs 实时终端 -->
    <div v-if="cloudStep === 'form'" class="provision-flow">
      <!-- 步骤 1：选择区域 -->
      <section class="section-card">
        <div class="section-title-row">
          <div class="step-num">1</div>
          <div>
            <h4 class="section-title">选择数据中心区域 (Region)</h4>
            <span class="section-subtitle">已支持亚太高速、北美核心与欧洲数据中心，带自动国旗匹配</span>
          </div>
        </div>

        <!-- 洲际筛选 Tab -->
        <div class="continent-tabs">
          <button
            v-for="c in continents"
            :key="c.key"
            class="continent-btn"
            :class="{ 'continent-btn-active': selectedContinent === c.key }"
            @click="selectedContinent = c.key"
          >
            {{ c.label }}
          </button>
        </div>

        <!-- 区域卡片网格 -->
        <div v-if="loadingCatalog" class="loading-state">
          <span>⏳ 正在从 Vultr 同步全球数据中心与库存...</span>
        </div>
        <div v-else class="regions-grid">
          <div
            v-for="r in filteredRegions"
            :key="r.id"
            class="region-card"
            :class="{ 'region-card-selected': selectedRegion === r.id }"
            @click="selectRegion(r)"
          >
            <div class="region-flag">{{ r.flag_emoji }}</div>
            <div class="region-info">
              <div class="region-city">{{ r.city_zh }}</div>
              <div class="region-meta">
                <span class="region-id mono">{{ r.id }}</span>
                <span class="region-continent">{{ r.continent }}</span>
              </div>
            </div>
            <div v-if="selectedRegion === r.id" class="check-badge">✓</div>
          </div>
        </div>
      </section>

      <!-- 步骤 2：选择机型规格 -->
      <section class="section-card">
        <div class="section-title-row">
          <div class="step-num">2</div>
          <div>
            <h4 class="section-title">选择实例规格套餐 (Plan)</h4>
            <span class="section-subtitle">
              已自动根据选中的 <strong>[{{ currentRegionName }}]</strong> 过滤实时可用库存
            </span>
          </div>
        </div>

        <!-- 机型分类 Tab -->
        <div class="plan-type-tabs">
          <button
            class="plan-tab-btn"
            :class="{ 'plan-tab-active': planTypeFilter === 'all' }"
            @click="planTypeFilter = 'all'"
          >
            全部规格
          </button>
          <button
            class="plan-tab-btn"
            :class="{ 'plan-tab-active': planTypeFilter === 'vc2' }"
            @click="planTypeFilter = 'vc2'"
          >
            💻 标准计算 (vc2)
          </button>
          <button
            class="plan-tab-btn"
            :class="{ 'plan-tab-active': planTypeFilter === 'vhf' }"
            @click="planTypeFilter = 'vhf'"
          >
            ⚡ 高频计算 3GHz+ (vhf)
          </button>
          <button
            class="plan-tab-btn"
            :class="{ 'plan-tab-active': planTypeFilter === 'vhp' }"
            @click="planTypeFilter = 'vhp'"
          >
            🚀 AMD/Intel 高性能 (vhp)
          </button>
        </div>

        <!-- 机型规格网格 -->
        <div class="plans-grid">
          <div
            v-for="p in filteredPlans"
            :key="p.id"
            class="plan-card"
            :class="{
              'plan-card-selected': selectedPlan === p.id,
              'plan-card-disabled': !isPlanInRegion(p)
            }"
            @click="isPlanInRegion(p) && (selectedPlan = p.id)"
          >
            <div class="plan-card-header">
              <span class="plan-id mono">{{ p.id }}</span>
              <span class="plan-price">
                <strong>${{ p.monthly_cost.toFixed(2) }}</strong> <small>/ 月</small>
              </span>
            </div>

            <div class="plan-specs">
              <div class="spec-row">
                <span class="spec-label">核心与内存:</span>
                <span class="spec-val"><strong>{{ p.vcpu_count }} vCPU</strong> · {{ p.ram }} MB</span>
              </div>
              <div class="spec-row">
                <span class="spec-label">高速磁盘:</span>
                <span class="spec-val">{{ p.disk }} GB SSD</span>
              </div>
              <div class="spec-row">
                <span class="spec-label">公网流量:</span>
                <span class="spec-val">{{ (p.bandwidth / 1024).toFixed(1) }} TB</span>
              </div>
            </div>

            <div class="plan-card-footer">
              <span v-if="!isPlanInRegion(p)" class="stock-badge stock-empty">
                该区域无库存
              </span>
              <span v-else-if="selectedPlan === p.id" class="stock-badge stock-selected">
                ✓ 已选择此规格
              </span>
              <span v-else class="stock-badge stock-available">
                有库存
              </span>
            </div>
          </div>
        </div>
      </section>

      <!-- 步骤 3：选择操作系统镜像 -->
      <section class="section-card">
        <div class="section-title-row">
          <div class="step-num">3</div>
          <div>
            <h4 class="section-title">选择操作系统镜像 (OS)</h4>
            <span class="section-subtitle">已自动筛选 64 位原生支持 Systemd 与 BBR 的主流 Linux 镜像</span>
          </div>
        </div>

        <div class="os-family-tabs">
          <button
            class="os-tab-btn"
            :class="{ 'os-tab-active': selectedOSFamily === 'debian' }"
            @click="selectOSFamily('debian')"
          >
            <span>🌀 Debian</span>
            <small>推荐 · 低内存</small>
          </button>
          <button
            class="os-tab-btn"
            :class="{ 'os-tab-active': selectedOSFamily === 'ubuntu' }"
            @click="selectOSFamily('ubuntu')"
          >
            <span>🟠 Ubuntu LTS</span>
            <small>热门 · 生态好</small>
          </button>
          <button
            class="os-tab-btn"
            :class="{ 'os-tab-active': selectedOSFamily === 'rhel' }"
            @click="selectOSFamily('rhel')"
          >
            <span>🟢 Rocky / Alma</span>
            <small>企业级 RHEL</small>
          </button>
          <button
            class="os-tab-btn"
            :class="{ 'os-tab-active': selectedOSFamily === 'other' }"
            @click="selectOSFamily('other')"
          >
            <span>🔵 Fedora / Arch</span>
            <small>新内核</small>
          </button>
        </div>

        <div class="os-version-selector">
          <label>具体系统版本：</label>
          <select v-model="selectedOSID" class="os-select mono">
            <option v-for="o in filteredOSList" :key="o.id" :value="o.id">
              {{ o.name }} {{ o.recommended ? '— ⭐ 官方首选推荐' : '' }}
            </option>
          </select>
        </div>
      </section>

      <!-- 步骤 4：节点参数与安全选项 -->
      <section class="section-card">
        <div class="section-title-row">
          <div class="step-num">4</div>
          <div>
            <h4 class="section-title">节点名称与安全增强设置</h4>
            <span class="section-subtitle">配置面板初始参数与防中间人攻击保护</span>
          </div>
        </div>

        <div class="options-grid">
          <div class="form-group">
            <label>节点友好显示名 <span class="req">*</span></label>
            <input v-model="formReq.name" type="text" placeholder="例如：东京 Vultr-01" />
          </div>

          <div class="form-group">
            <label>面板监听端口</label>
            <input v-model.number="formReq.panel_port" type="number" placeholder="20888" />
          </div>

          <div class="form-group full-width">
            <label class="checkbox-container">
              <input type="checkbox" v-model="formReq.enable_ipv6" />
              <span class="checkbox-title">分配公网 IPv6 地址 (免费开启双栈网络)</span>
            </label>
          </div>

          <div class="form-group full-width" style="background: rgba(99, 102, 241, 0.08); padding: 0.75rem; border-radius: 0.5rem; border: 1px dashed rgba(99, 102, 241, 0.3);">
            <label class="checkbox-container">
              <input type="checkbox" v-model="formReq.enable_tls" />
              <span class="checkbox-title">🛡️ 启用自签 TLS 证书与指纹强绑定 (防中间人劫持 · 强烈推荐)</span>
            </label>
            <div class="field-hint">
              新主机启动后将自动生成 ECDSA TLS 证书并在主控端绑定 SHA-256 唯一指纹，杜绝任何公网路由劫持与流量窃听。
            </div>
          </div>
        </div>
      </section>

      <!-- 底部总结栏与开通按钮 -->
      <div class="bottom-action-bar">
        <div class="summary-box">
          <span class="summary-label">配置汇总:</span>
          <span class="summary-value">
            {{ currentRegionFlag }} {{ currentRegionName }} · {{ currentPlanText }} · {{ currentOSText }}
          </span>
          <span class="summary-price">
            预估费用: <strong>${{ currentPlanCost.toFixed(2) }}</strong> / 月
          </span>
        </div>

        <button class="btn-create-submit" @click="startProvision" :disabled="isSubmitting || !canSubmit">
          <span>🚀 立即开通云主机并自动纳管</span>
        </button>
      </div>
    </div>

    <!-- 3. 部署中与实时日志流终端 -->
    <div v-else class="cloud-terminal-box">
      <!-- 6 阶段可视化步骤条 -->
      <div class="stepper-bar">
        <div
          v-for="s in stages"
          :key="s.num"
          class="stepper-item"
          :class="{
            'step-done': currentStage > s.num || (currentStage === 6 && taskStatus === 'success'),
            'step-current': currentStage === s.num && taskStatus === 'running',
            'step-failed': currentStage === s.num && taskStatus === 'failed'
          }"
        >
          <div class="step-circle">{{ s.num }}</div>
          <span class="step-label">{{ s.title }}</span>
        </div>
      </div>

      <!-- 终端主体 -->
      <div class="terminal-container">
        <div class="terminal-header">
          <div class="terminal-status-info">
            <span class="pulse-indicator" :class="{ 'pulse-active': taskStatus === 'running' }"></span>
            <span class="terminal-title">
              {{ taskStatus === 'running' ? 'Vultr 云端弹性开机与集群纳管进行中...' : (taskStatus === 'success' ? '🎉 云主机开通并纳管成功！' : '❌ 创建过程发生异常') }}
            </span>
          </div>
          <span class="terminal-node-name mono">{{ formReq.name }}</span>
        </div>

        <div class="terminal-body" ref="terminalBodyRef">
          <div v-for="(line, idx) in taskLogs" :key="idx" class="terminal-line" :class="getLineClass(line)">
            {{ line }}
          </div>
        </div>

        <div class="terminal-footer">
          <button
            v-if="taskStatus === 'success'"
            class="btn-primary"
            @click="finishAndSwitch"
          >
            <span>✅ 完成并切换至新节点管理视角</span>
          </button>
          <button
            v-else-if="taskStatus === 'failed'"
            class="btn-secondary"
            @click="cloudStep = 'form'"
          >
            <span>🔄 返回修改参数重试</span>
          </button>
          <span v-else class="deploy-progress-hint">
            实例启动与分配公网 IP 中，全流程通常需 45~90 秒，请保持页面开启...
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, nextTick } from 'vue'
import api, { getSecretPath } from '../api'
import { useToast } from '../composables/useToast'
import { useNodeContext } from '../composables/useNodeContext'

const emit = defineEmits<{
  (e: 'deployed', nodeId: number): void
  (e: 'close'): void
}>()

const { toast } = useToast()
const { fetchNodes, switchNode } = useNodeContext()

// Vultr 配置状态
const vultrConfig = reactive({
  configured: false,
  masked_key: '',
  account: null as any,
  error: '',
})

const showUpdateToken = ref(false)
const tokenInput = ref('')
const showTokenPlain = ref(false)
const savingToken = ref(false)
const tokenError = ref('')

// Catalog 目录树
const loadingCatalog = ref(false)
const catalog = reactive<{
  regions: any[]
  plans: any[]
  os_list: any[]
}>({
  regions: [],
  plans: [],
  os_list: [],
})

// 筛选与选择状态
const continents = [
  { key: 'all', label: '全部区域' },
  { key: 'Asia', label: '🌏 亚太地区' },
  { key: 'North America', label: '🌎 美洲地区' },
  { key: 'Europe', label: '🌍 欧洲地区' },
  { key: 'Oceania', label: '🦘 大洋洲' },
]
const selectedContinent = ref('all')
const selectedRegion = ref('nrt')

const planTypeFilter = ref<'all' | 'vc2' | 'vhf' | 'vhp'>('all')
const selectedPlan = ref('vc2-1c-1gb')

const selectedOSFamily = ref<'debian' | 'ubuntu' | 'rhel' | 'other'>('debian')
const selectedOSID = ref<number>(2136)

const formReq = reactive({
  name: '东京 Vultr-01',
  panel_port: 20888,
  enable_ipv6: true,
  enable_tls: true,
})

// 阶段与终端
const cloudStep = ref<'form' | 'terminal'>('form')
const currentStage = ref(1)
const taskStatus = ref<'running' | 'success' | 'failed'>('running')
const taskLogs = ref<string[]>([])
const terminalBodyRef = ref<HTMLElement | null>(null)
let eventSource: EventSource | null = null
let deployedNodeId = 0
const isSubmitting = ref(false)

const stages = [
  { num: 1, title: '绑定密钥' },
  { num: 2, title: '创建实例' },
  { num: 3, title: '分配 IP' },
  { num: 4, title: 'SSH 就绪' },
  { num: 5, title: '极速部署' },
  { num: 6, title: '握手纳管' },
]

// 联动计算属性
const filteredRegions = computed(() => {
  if (selectedContinent.value === 'all') return catalog.regions
  return catalog.regions.filter((r) => r.continent === selectedContinent.value)
})

const filteredPlans = computed(() => {
  let list = catalog.plans
  if (planTypeFilter.value !== 'all') {
    list = list.filter((p) => p.type === planTypeFilter.value)
  }
  return list.slice(0, 16)
})

const filteredOSList = computed(() => {
  return catalog.os_list.filter((o) => {
    const fam = o.family?.toLowerCase() || ''
    if (selectedOSFamily.value === 'debian') return fam === 'debian'
    if (selectedOSFamily.value === 'ubuntu') return fam === 'ubuntu'
    if (selectedOSFamily.value === 'rhel') return fam === 'rocky linux' || fam === 'almalinux' || fam === 'centos'
    return fam === 'fedora' || fam === 'arch linux'
  })
})

const currentRegionName = computed(() => {
  const r = catalog.regions.find((x) => x.id === selectedRegion.value)
  return r ? r.city_zh : selectedRegion.value
})

const currentRegionFlag = computed(() => {
  const r = catalog.regions.find((x) => x.id === selectedRegion.value)
  return r ? r.flag_emoji : '🌐'
})

const currentPlanText = computed(() => {
  const p = catalog.plans.find((x) => x.id === selectedPlan.value)
  return p ? `${p.vcpu_count} vCPU ${p.ram}MB` : selectedPlan.value
})

const currentPlanCost = computed(() => {
  const p = catalog.plans.find((x) => x.id === selectedPlan.value)
  return p ? p.monthly_cost : 5.0
})

const currentOSText = computed(() => {
  const o = catalog.os_list.find((x) => x.id === selectedOSID.value)
  return o ? o.name : 'Debian'
})

const canSubmit = computed(() => {
  return selectedRegion.value && selectedPlan.value && selectedOSID.value && formReq.name.trim()
})

function isPlanInRegion(plan: any): boolean {
  if (!plan.locations || !Array.isArray(plan.locations)) return true
  return plan.locations.includes(selectedRegion.value)
}

function selectRegion(r: any) {
  selectedRegion.value = r.id
  formReq.name = `${r.city} Vultr-01`
}

function selectOSFamily(fam: 'debian' | 'ubuntu' | 'rhel' | 'other') {
  selectedOSFamily.value = fam
  const match = filteredOSList.value[0]
  if (match) {
    selectedOSID.value = match.id
  }
}

// 加载配置
async function loadVultrConfig() {
  try {
    const res = await api.get('/nodes/cloud/vultr/config')
    vultrConfig.configured = res.data.configured
    vultrConfig.masked_key = res.data.masked_key
    vultrConfig.account = res.data.account
    vultrConfig.error = res.data.error || ''

    if (vultrConfig.configured && !vultrConfig.error) {
      await loadCatalog(false)
    }
  } catch (err: any) {
    console.error('加载 Vultr 配置失败', err)
  }
}

// 保存 Token
async function saveToken() {
  if (!tokenInput.value.trim()) return
  savingToken.value = true
  tokenError.value = ''
  try {
    const res = await api.post('/nodes/cloud/vultr/config', {
      api_key: tokenInput.value.trim(),
    })
    vultrConfig.configured = true
    vultrConfig.masked_key = res.data.masked_key
    vultrConfig.account = res.data.account
    vultrConfig.error = ''
    showUpdateToken.value = false
    tokenInput.value = ''
    toast.success('Vultr Token 验证通过并已绑定！')
    await loadCatalog(true)
  } catch (err: any) {
    tokenError.value = err.response?.data?.error || err.message || 'Token 验证失败'
    toast.error(tokenError.value)
  } finally {
    savingToken.value = false
  }
}

// 加载 Catalog
async function loadCatalog(refresh: boolean) {
  loadingCatalog.value = true
  try {
    const res = await api.get(`/nodes/cloud/vultr/catalog?refresh=${refresh ? 'true' : 'false'}`)
    catalog.regions = res.data.regions || []
    catalog.plans = res.data.plans || []
    catalog.os_list = res.data.os_list || []

    // 默认选中东京或第一个亚太区域
    if (!catalog.regions.some((r) => r.id === selectedRegion.value)) {
      const defaultR = catalog.regions.find((r) => r.id === 'nrt') || catalog.regions[0]
      if (defaultR) selectRegion(defaultR)
    }

    // 默认选中 Debian 12
    const recOS = catalog.os_list.find((o) => o.recommended) || catalog.os_list[0]
    if (recOS) selectedOSID.value = recOS.id
  } catch (err: any) {
    toast.error('加载 Vultr 产品目录失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loadingCatalog.value = false
  }
}

function refreshCatalog() {
  loadCatalog(true)
}

function scrollToBottom() {
  nextTick(() => {
    if (terminalBodyRef.value) {
      terminalBodyRef.value.scrollTop = terminalBodyRef.value.scrollHeight
    }
  })
}

function getLineClass(line: string) {
  if (line.includes('[SUCCESS]')) return 'line-success'
  if (line.includes('[ERROR]')) return 'line-error'
  if (line.includes('【阶段') || line.includes('IP:')) return 'line-highlight'
  return ''
}

// 启动弹性开机流水线
async function startProvision() {
  if (!canSubmit.value) return

  isSubmitting.value = true
  cloudStep.value = 'terminal'
  currentStage.value = 1
  taskStatus.value = 'running'
  taskLogs.value = []

  try {
    const res = await api.post('/nodes/cloud/vultr/provision', {
      name: formReq.name.trim(),
      region: selectedRegion.value,
      plan: selectedPlan.value,
      os_id: selectedOSID.value,
      enable_ipv6: formReq.enable_ipv6,
      panel_port: formReq.panel_port || 20888,
      enable_tls: formReq.enable_tls,
    })

    const taskId = res.data.task_id
    const secret = getSecretPath()
    const token = localStorage.getItem('token') || ''
    const streamURL = `/${secret}/api/nodes/deploy-logs?task_id=${taskId}&token=${token}`

    if (eventSource) eventSource.close()
    eventSource = new EventSource(streamURL)

    eventSource.onmessage = (event) => {
      const line = event.data
      taskLogs.value.push(line)
      scrollToBottom()

      // 阶段推进识别
      if (line.includes('【阶段 1/6】')) currentStage.value = 1
      else if (line.includes('【阶段 2/6】')) currentStage.value = 2
      else if (line.includes('【阶段 3/6】')) currentStage.value = 3
      else if (line.includes('【阶段 4/6】')) currentStage.value = 4
      else if (line.includes('【阶段 5/6】')) currentStage.value = 5
      else if (line.includes('【阶段 6/6】')) currentStage.value = 6

      if (line.includes('[SUCCESS]')) {
        taskStatus.value = 'success'
        currentStage.value = 6
        eventSource?.close()
        fetchNodes().then(() => {
          const match = useNodeContext().nodes.value.find((n) => n.name === formReq.name)
          if (match) deployedNodeId = match.id
        })
      } else if (line.includes('[ERROR]')) {
        taskStatus.value = 'failed'
        eventSource?.close()
      }
    }

    eventSource.onerror = () => {
      eventSource?.close()
      if (taskStatus.value === 'running') {
        taskStatus.value = 'failed'
        taskLogs.value.push('[ERROR] 日志流异常中断')
      }
    }
  } catch (err: any) {
    taskStatus.value = 'failed'
    taskLogs.value.push('[ERROR] 启动任务失败: ' + (err.response?.data?.error || err.message))
  } finally {
    isSubmitting.value = false
  }
}

function finishAndSwitch() {
  if (deployedNodeId > 0) {
    switchNode(deployedNodeId)
    toast.success('已切换至新开通的云主机视角！')
  } else {
    toast.success('开机纳管完成！')
  }
  emit('close')
}

onMounted(() => {
  loadVultrConfig()
})

onUnmounted(() => {
  if (eventSource) eventSource.close()
})
</script>

<style scoped>
.vultr-provisioner {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

/* 顶部状态条 */
.token-setup-card {
  background: rgba(99, 102, 241, 0.08);
  border: 1px solid rgba(99, 102, 241, 0.3);
  padding: 1rem 1.25rem;
  border-radius: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.setup-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.setup-icon {
  font-size: 1.5rem;
}

.setup-title {
  margin: 0;
  font-size: 0.95rem;
  color: #f1f5f9;
}

.setup-desc {
  margin: 0.15rem 0 0 0;
  font-size: 0.75rem;
  color: #94a3b8;
}

.token-input-row {
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.token-input {
  flex: 1;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.45rem;
  padding: 0.55rem 0.75rem;
  color: #f8fafc;
  font-size: 0.8125rem;
  outline: none;
}

.token-input:focus {
  border-color: #6366f1;
}

.btn-toggle-eye {
  background: #334155;
  border: none;
  padding: 0.55rem 0.65rem;
  border-radius: 0.45rem;
  cursor: pointer;
}

.token-error-msg {
  font-size: 0.75rem;
  color: #f87171;
}

.token-status-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: rgba(15, 23, 42, 0.8);
  border: 1px solid #334155;
  border-radius: 0.65rem;
  padding: 0.65rem 1rem;
}

.status-left {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.cloud-icon {
  font-size: 1.4rem;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 0.45rem;
}

.provider-badge {
  font-size: 0.675rem;
  background: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
  padding: 0.05rem 0.35rem;
  border-radius: 0.25rem;
  font-weight: 600;
}

.masked-key {
  font-size: 0.75rem;
  color: #cbd5e1;
}

.active-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #10b981;
}

.balance-text {
  font-size: 0.72rem;
  color: #94a3b8;
  margin-top: 0.2rem;
}

.color-success {
  color: #34d399;
}

.dot-divider {
  margin: 0 0.35rem;
  color: #475569;
}

.status-right {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.btn-mini-refresh,
.btn-mini-update {
  background: #1e293b;
  border: 1px solid #334155;
  color: #94a3b8;
  font-size: 0.72rem;
  padding: 0.3rem 0.6rem;
  border-radius: 0.35rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-mini-refresh:hover,
.btn-mini-update:hover {
  background: #334155;
  color: #ffffff;
}

/* 步骤块 */
.provision-flow {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.section-card {
  background: #0b1120;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  padding: 1rem 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.section-title-row {
  display: flex;
  align-items: center;
  gap: 0.65rem;
}

.step-num {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #4f46e5;
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.75rem;
  font-weight: 700;
}

.section-title {
  margin: 0;
  font-size: 0.875rem;
  font-weight: 600;
  color: #f8fafc;
}

.section-subtitle {
  font-size: 0.7rem;
  color: #64748b;
}

/* 区域 */
.continent-tabs {
  display: flex;
  gap: 0.4rem;
}

.continent-btn {
  background: transparent;
  border: 1px solid #334155;
  color: #94a3b8;
  padding: 0.25rem 0.65rem;
  border-radius: 0.35rem;
  font-size: 0.75rem;
  cursor: pointer;
}

.continent-btn-active {
  background: rgba(99, 102, 241, 0.2);
  border-color: #6366f1;
  color: #a5b4fc;
  font-weight: 600;
}

.regions-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: 0.6rem;
  max-height: 240px;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.region-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 0.65rem;
  display: flex;
  align-items: center;
  gap: 0.6rem;
  cursor: pointer;
  position: relative;
  transition: all 0.15s ease;
}

.region-card:hover {
  border-color: #475569;
}

.region-card-selected {
  background: rgba(99, 102, 241, 0.15);
  border-color: #6366f1;
}

.region-flag {
  font-size: 1.5rem;
}

.region-city {
  font-size: 0.775rem;
  font-weight: 500;
  color: #f1f5f9;
}

.region-meta {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  margin-top: 0.15rem;
}

.region-id {
  font-size: 0.675rem;
  color: #818cf8;
}

.region-continent {
  font-size: 0.65rem;
  color: #64748b;
}

.check-badge {
  position: absolute;
  top: 0.4rem;
  right: 0.4rem;
  font-size: 0.75rem;
  color: #818cf8;
  font-weight: bold;
}

/* 机型 */
.plan-type-tabs {
  display: flex;
  gap: 0.4rem;
}

.plan-tab-btn {
  background: transparent;
  border: 1px solid #334155;
  color: #94a3b8;
  padding: 0.25rem 0.65rem;
  border-radius: 0.35rem;
  font-size: 0.75rem;
  cursor: pointer;
}

.plan-tab-active {
  background: rgba(99, 102, 241, 0.2);
  border-color: #6366f1;
  color: #a5b4fc;
  font-weight: 600;
}

.plans-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(210px, 1fr));
  gap: 0.65rem;
  max-height: 260px;
  overflow-y: auto;
  padding-right: 0.25rem;
}

.plan-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 0.75rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.plan-card:hover:not(.plan-card-disabled) {
  border-color: #475569;
}

.plan-card-selected {
  background: rgba(99, 102, 241, 0.15);
  border-color: #6366f1;
}

.plan-card-disabled {
  opacity: 0.4;
  cursor: not-allowed;
  filter: grayscale(0.8);
}

.plan-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.plan-id {
  font-size: 0.75rem;
  font-weight: 600;
  color: #f1f5f9;
}

.plan-price {
  font-size: 0.85rem;
  color: #34d399;
}

.plan-specs {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  font-size: 0.7rem;
}

.spec-row {
  display: flex;
  justify-content: space-between;
}

.spec-label {
  color: #64748b;
}

.spec-val {
  color: #cbd5e1;
}

.stock-badge {
  font-size: 0.65rem;
  padding: 0.1rem 0.35rem;
  border-radius: 0.25rem;
}

.stock-available {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
}

.stock-selected {
  background: rgba(99, 102, 241, 0.25);
  color: #a5b4fc;
  font-weight: 600;
}

.stock-empty {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}

/* 系统选择 */
.os-family-tabs {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 0.5rem;
}

.os-tab-btn {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.5rem;
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.15rem;
  color: #cbd5e1;
  font-size: 0.775rem;
  cursor: pointer;
  transition: all 0.15s ease;
}

.os-tab-btn small {
  font-size: 0.65rem;
  color: #64748b;
}

.os-tab-active {
  background: rgba(99, 102, 241, 0.2);
  border-color: #6366f1;
  color: #f8fafc;
}

.os-tab-active small {
  color: #a5b4fc;
}

.os-version-selector {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  font-size: 0.775rem;
  color: #94a3b8;
}

.os-select {
  flex: 1;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.45rem;
  padding: 0.5rem 0.65rem;
  color: #f1f5f9;
  font-size: 0.775rem;
  outline: none;
}

/* 选项表单 */
.options-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.85rem;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.full-width {
  grid-column: 1 / -1;
}

.form-group label {
  font-size: 0.75rem;
  font-weight: 500;
  color: #cbd5e1;
}

.req {
  color: #f87171;
}

.form-group input {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.45rem;
  padding: 0.55rem 0.75rem;
  color: #f8fafc;
  font-size: 0.8125rem;
  outline: none;
}

.checkbox-container {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}

.checkbox-title {
  font-size: 0.8125rem;
  font-weight: 600;
  color: #f1f5f9;
}

.field-hint {
  font-size: 0.72rem;
  color: #94a3b8;
  line-height: 1.45;
  margin-top: 0.25rem;
}

/* 底部操作条 */
.bottom-action-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 0.75rem;
  padding: 0.85rem 1.25rem;
  position: sticky;
  bottom: 0;
  z-index: 10;
  box-shadow: 0 -10px 25px -5px rgba(0, 0, 0, 0.4);
}

.summary-box {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
}

.summary-label {
  font-size: 0.7rem;
  color: #64748b;
}

.summary-value {
  font-size: 0.8125rem;
  font-weight: 500;
  color: #f1f5f9;
}

.summary-price {
  font-size: 0.75rem;
  color: #94a3b8;
}

.summary-price strong {
  color: #34d399;
}

.btn-create-submit {
  background: linear-gradient(135deg, #4f46e5 0%, #7c3aed 100%);
  color: #ffffff;
  border: none;
  padding: 0.7rem 1.5rem;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s ease;
  box-shadow: 0 4px 12px rgba(79, 70, 229, 0.35);
}

.btn-create-submit:hover:not(:disabled) {
  opacity: 0.95;
  transform: translateY(-1px);
}

.btn-create-submit:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 终端与步骤条 */
.cloud-terminal-box {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.stepper-bar {
  display: flex;
  justify-content: space-between;
  background: #0b1120;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  padding: 0.75rem 1.25rem;
}

.stepper-item {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  opacity: 0.4;
  transition: all 0.2s ease;
}

.step-circle {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: #334155;
  color: #cbd5e1;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.7rem;
  font-weight: bold;
}

.step-label {
  font-size: 0.75rem;
  color: #94a3b8;
}

.step-current {
  opacity: 1;
}

.step-current .step-circle {
  background: #4f46e5;
  color: #ffffff;
  box-shadow: 0 0 10px rgba(99, 102, 241, 0.6);
}

.step-current .step-label {
  color: #818cf8;
  font-weight: 600;
}

.step-done {
  opacity: 0.9;
}

.step-done .step-circle {
  background: #10b981;
  color: #ffffff;
}

.step-done .step-label {
  color: #34d399;
}

.step-failed {
  opacity: 1;
}

.step-failed .step-circle {
  background: #ef4444;
  color: #ffffff;
}

.step-failed .step-label {
  color: #f87171;
}

.terminal-container {
  display: flex;
  flex-direction: column;
  background: #020617;
  border: 1px solid #1e293b;
  border-radius: 0.75rem;
  overflow: hidden;
  height: 440px;
}

.terminal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.65rem 1rem;
  background: #0f172a;
  border-bottom: 1px solid #1e293b;
}

.terminal-status-info {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.pulse-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #94a3b8;
}

.pulse-active {
  background: #10b981;
  box-shadow: 0 0 8px #10b981;
  animation: pulseAnim 1.5s infinite;
}

@keyframes pulseAnim {
  0% { opacity: 0.4; }
  50% { opacity: 1; }
  100% { opacity: 0.4; }
}

.terminal-title {
  font-size: 0.8125rem;
  font-weight: 600;
  color: #f1f5f9;
}

.terminal-node-name {
  font-size: 0.75rem;
  color: #64748b;
}

.terminal-body {
  flex: 1;
  padding: 0.85rem;
  overflow-y: auto;
  font-family: ui-monospace, monospace;
  font-size: 0.75rem;
  line-height: 1.6;
  color: #94a3b8;
}

.terminal-line {
  white-space: pre-wrap;
  word-break: break-all;
}

.line-success {
  color: #34d399;
  font-weight: 600;
}

.line-error {
  color: #f87171;
  font-weight: 600;
}

.line-highlight {
  color: #93c5fd;
}

.terminal-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  padding: 0.75rem 1rem;
  background: #0f172a;
  border-top: 1px solid #1e293b;
}

.deploy-progress-hint {
  font-size: 0.75rem;
  color: #64748b;
}

.btn-primary {
  background: #4f46e5;
  color: #ffffff;
  border: none;
  padding: 0.6rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.85rem;
  font-weight: 600;
  cursor: pointer;
}

.btn-secondary {
  background: #334155;
  color: #cbd5e1;
  border: none;
  padding: 0.6rem 1.25rem;
  border-radius: 0.5rem;
  font-size: 0.85rem;
  cursor: pointer;
}

.mono {
  font-family: ui-monospace, monospace;
}
</style>
