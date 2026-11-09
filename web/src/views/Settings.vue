<template>
  <div style="max-width: 820px;">
    <div style="display: flex; align-items: center; gap: 0.5rem; margin-bottom: 1.5rem;">
      <span style="font-size: 1.5rem;">⚙️</span>
      <h1 style="font-size: 1.35rem; font-weight: 700; color: #f8fafc;">面板设置与安全</h1>
    </div>

    <!-- 1. 版本与离线安装包升级专区 -->
    <div class="card" style="margin-bottom: 1.5rem; border: 1px solid rgba(99, 102, 241, 0.3); background: linear-gradient(180deg, rgba(30, 41, 59, 0.7) 0%, rgba(15, 23, 42, 0.8) 100%);">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.25rem;">📦</span>
          <h3 style="font-size: 1.1rem; font-weight: 600; color: #818cf8; margin: 0;">面板版本与安装包升级</h3>
        </div>
        <div style="display: flex; gap: 0.5rem; align-items: center;">
          <span class="badge" style="background: rgba(99, 102, 241, 0.2); color: #a5b4fc; border: 1px solid rgba(99, 102, 241, 0.4); padding: 0.25rem 0.6rem; border-radius: 9999px; font-size: 0.8rem; font-weight: 600;">
            当前版本: {{ sysInfo.version || 'v1.0.1' }}
          </span>
          <span class="badge" style="background: rgba(56, 189, 248, 0.15); color: #38bdf8; border: 1px solid rgba(56, 189, 248, 0.3); padding: 0.25rem 0.6rem; border-radius: 9999px; font-size: 0.8rem;">
            {{ sysInfo.os }}/{{ sysInfo.arch }}
          </span>
        </div>
      </div>

      <!-- 当前运行程序指纹 -->
      <div style="display: flex; align-items: center; justify-content: space-between; background: rgba(15, 23, 42, 0.6); padding: 0.6rem 0.85rem; border-radius: 0.375rem; border: 1px solid #334155; margin-bottom: 1rem; font-size: 0.8125rem;">
        <div style="display: flex; align-items: center; gap: 0.5rem; overflow: hidden;">
          <span style="color: #94a3b8; flex-shrink: 0;">运行程序指纹 (SHA-256):</span>
          <code style="color: #cbd5e1; font-family: monospace; text-overflow: ellipsis; overflow: hidden; white-space: nowrap;">
            {{ sysInfo.sha256 || '计算中...' }}
          </code>
        </div>
        <button
          v-if="sysInfo.sha256"
          type="button"
          class="btn-text-copy"
          @click="copyToClipboard(sysInfo.sha256, '运行程序 SHA-256 指纹')"
          style="flex-shrink: 0; color: #818cf8; background: transparent; border: none; cursor: pointer; font-size: 0.75rem; padding: 0.2rem 0.5rem;"
        >
          📋 复制
        </button>
      </div>

      <!-- 拖拽/点击上传区域 -->
      <div
        class="upload-dropzone"
        :class="{ 'dropzone-active': isDragging }"
        @dragover.prevent="isDragging = true"
        @dragleave.prevent="isDragging = false"
        @drop.prevent="handleDrop"
        @click="triggerFileInput"
        style="border: 2px dashed #475569; border-radius: 0.5rem; padding: 1.5rem; text-align: center; cursor: pointer; background: rgba(15, 23, 42, 0.4); transition: all 0.2s;"
      >
        <input
          ref="fileInputRef"
          type="file"
          style="display: none;"
          @change="handleFileSelected"
        />
        <div v-if="!uploading">
          <div style="font-size: 2rem; margin-bottom: 0.5rem;">📥</div>
          <div style="font-size: 0.95rem; font-weight: 600; color: #f8fafc; margin-bottom: 0.25rem;">
            点击或拖拽上传新版安装包
          </div>
          <div style="font-size: 0.8rem; color: #94a3b8;">
            支持 <code>vps-panel</code> 单二进制文件，或包含该程序的 <code>.tar.gz</code> / <code>.zip</code> 发布包
          </div>
        </div>
        <div v-else>
          <div style="font-size: 1.5rem; margin-bottom: 0.5rem;">⏳</div>
          <div style="font-size: 0.9rem; font-weight: 600; color: #38bdf8; margin-bottom: 0.5rem;">
            正在上传并校验安装包 ELF 架构与完整性...
          </div>
          <div style="width: 100%; max-width: 300px; height: 6px; background: #334155; border-radius: 3px; margin: 0 auto; overflow: hidden;">
            <div style="height: 100%; background: #6366f1; width: 100%; animation: indeterminate 1.5s infinite linear;"></div>
          </div>
        </div>
      </div>

      <!-- 已导入安装包列表 -->
      <div v-if="packages.length > 0" style="margin-top: 1.25rem;">
        <h4 style="font-size: 0.9rem; font-weight: 600; color: #cbd5e1; margin-bottom: 0.75rem; display: flex; align-items: center; justify-content: space-between;">
          <span>📦 已就绪的安装包仓库 ({{ packages.length }})</span>
          <span style="font-size: 0.75rem; color: #94a3b8; font-weight: normal;">点击下方操作升级本机或分发至集群</span>
        </h4>

        <div style="display: flex; flex-direction: column; gap: 0.75rem;">
          <div
            v-for="pkg in packages"
            :key="pkg.package_id"
            style="background: rgba(15, 23, 42, 0.7); border: 1px solid #334155; border-radius: 0.5rem; padding: 0.85rem; display: flex; flex-direction: column; gap: 0.6rem;"
          >
            <div style="display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 0.5rem;">
              <div>
                <div style="display: flex; align-items: center; gap: 0.5rem;">
                  <span style="font-weight: 600; color: #f8fafc; font-size: 0.9rem;">{{ pkg.file_name }}</span>
                  <span
                    :style="{
                      background: pkg.match_current_arch ? 'rgba(16, 185, 129, 0.2)' : 'rgba(245, 158, 11, 0.2)',
                      color: pkg.match_current_arch ? '#34d399' : '#fbbf24',
                      border: `1px solid ${pkg.match_current_arch ? 'rgba(16, 185, 129, 0.4)' : 'rgba(245, 158, 11, 0.4)'}`,
                      padding: '0.15rem 0.45rem',
                      borderRadius: '0.25rem',
                      fontSize: '0.75rem',
                      fontWeight: '600'
                    }"
                  >
                    {{ pkg.arch }}
                  </span>
                  <span style="font-size: 0.75rem; color: #818cf8; background: rgba(99, 102, 241, 0.15); padding: 0.15rem 0.45rem; border-radius: 0.25rem;">
                    {{ pkg.version }}
                  </span>
                </div>
                <div style="font-size: 0.75rem; color: #64748b; margin-top: 0.25rem; font-family: monospace;">
                  大小: {{ (pkg.size / 1024 / 1024).toFixed(2) }} MB | SHA-256: {{ pkg.sha256.substring(0, 16) }}...
                </div>
              </div>

              <!-- 操作按钮 -->
              <div style="display: flex; gap: 0.5rem; align-items: center;">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="!pkg.match_current_arch || isLocalUpgrading"
                  @click="triggerLocalUpgrade(pkg)"
                  style="font-size: 0.8125rem; padding: 0.35rem 0.75rem;"
                  :title="pkg.match_current_arch ? '将当前主机面板升级到此版本' : '此安装包架构与本机不匹配'"
                >
                  ⚡ 升级本机节点
                </button>
                <button
                  type="button"
                  class="btn"
                  @click="toggleClusterDrawer(pkg)"
                  style="font-size: 0.8125rem; padding: 0.35rem 0.75rem; background: rgba(56, 189, 248, 0.2); color: #38bdf8; border: 1px solid rgba(56, 189, 248, 0.4);"
                >
                  🚀 分发至集群节点 ({{ remoteNodes.length }})
                </button>
              </div>
            </div>

            <!-- 纳管节点分发选择面板 (点击展开) -->
            <div
              v-if="activeDistPkgId === pkg.package_id"
              style="margin-top: 0.5rem; border-top: 1px dashed #334155; padding-top: 0.75rem; background: rgba(15, 23, 42, 0.9); border-radius: 0.375rem; padding: 0.75rem;"
            >
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 0.5rem;">
                <span style="font-size: 0.8125rem; font-weight: 600; color: #38bdf8;">
                  🚀 选择要免密推送升级的纳管节点：
                </span>
                <div style="display: flex; gap: 0.5rem;">
                  <button
                    type="button"
                    class="btn-link"
                    @click="selectAllNodes"
                    style="font-size: 0.75rem; color: #818cf8; background: none; border: none; cursor: pointer;"
                  >
                    全选
                  </button>
                  <button
                    type="button"
                    class="btn-link"
                    @click="deselectAllNodes"
                    style="font-size: 0.75rem; color: #94a3b8; background: none; border: none; cursor: pointer;"
                  >
                    取消全选
                  </button>
                </div>
              </div>

              <div v-if="remoteNodes.length === 0" style="font-size: 0.8rem; color: #64748b; padding: 0.5rem 0;">
                暂无纳管节点，可前往主控导航「纳管服务器」纳管更多 VPS。
              </div>

              <div v-else style="display: flex; flex-direction: column; gap: 0.4rem; max-height: 200px; overflow-y: auto;">
                <label
                  v-for="node in remoteNodes"
                  :key="node.id"
                  style="display: flex; align-items: center; justify-content: space-between; padding: 0.4rem 0.6rem; background: rgba(30, 41, 59, 0.5); border-radius: 0.25rem; cursor: pointer;"
                >
                  <div style="display: flex; align-items: center; gap: 0.5rem;">
                    <input
                      type="checkbox"
                      :value="node.id"
                      v-model="selectedNodeIds"
                      style="accent-color: #6366f1;"
                    />
                    <span style="font-size: 0.8125rem; color: #f8fafc; font-weight: 500;">{{ node.name }}</span>
                    <span style="font-size: 0.75rem; color: #64748b;">({{ node.host }})</span>
                    <span style="font-size: 0.7rem; color: #38bdf8; background: rgba(56, 189, 248, 0.1); padding: 0.1rem 0.3rem; border-radius: 0.2rem;">
                      {{ node.os_info || 'Linux (amd64)' }}
                    </span>
                  </div>

                  <div style="display: flex; align-items: center; gap: 0.5rem;">
                    <span
                      :style="{
                        fontSize: '0.75rem',
                        color: node.status === 'online' ? '#10b981' : '#f43f5e',
                        fontWeight: '500'
                      }"
                    >
                      {{ node.status === 'online' ? '● 在线' : '● 离线' }}
                    </span>
                    <span v-if="upgradingNodes[node.id]" style="font-size: 0.75rem; color: #818cf8;">
                      ⏳ 升级中...
                    </span>
                    <button
                      type="button"
                      class="btn"
                      @click.stop="upgradeSingleRemoteNode(node, pkg)"
                      :disabled="upgradingNodes[node.id]"
                      style="font-size: 0.75rem; padding: 0.15rem 0.45rem; background: #4f46e5; color: #ffffff;"
                    >
                      🚀 免密升级
                    </button>
                  </div>
                </label>
              </div>

              <!-- 批量操作 -->
              <div v-if="remoteNodes.length > 0" style="margin-top: 0.75rem; display: flex; justify-content: flex-end; gap: 0.5rem;">
                <button
                  type="button"
                  class="btn btn-primary"
                  :disabled="selectedNodeIds.length === 0 || isBatchUpgrading"
                  @click="batchUpgradeRemoteNodes(pkg)"
                  style="font-size: 0.8125rem; padding: 0.35rem 0.85rem;"
                >
                  {{ isBatchUpgrading ? '⏳ 批量升级中...' : `⚡ 一键免密批量升级已选 (${selectedNodeIds.length})` }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 2. 系统基础参数 -->
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

    <!-- 3. 修改管理员密码 -->
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

    <!-- 4. 本地升级全屏重连探活浮层 (Reconnection Overlay) -->
    <div
      v-if="reconnecting"
      style="position: fixed; inset: 0; background: rgba(15, 23, 42, 0.85); backdrop-filter: blur(8px); z-index: 9999; display: flex; align-items: center; justify-content: center; padding: 1.5rem;"
    >
      <div style="background: rgba(30, 41, 59, 0.95); border: 1px solid #475569; border-radius: 0.75rem; padding: 2rem; max-width: 440px; width: 100%; text-align: center; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);">
        <div style="font-size: 3rem; margin-bottom: 1rem; animation: spin 2s linear infinite; display: inline-block;">
          ⚡
        </div>
        <h3 style="font-size: 1.2rem; font-weight: 700; color: #f8fafc; margin-bottom: 0.5rem;">
          服务平滑重启中...
        </h3>
        <p style="font-size: 0.875rem; color: #94a3b8; line-height: 1.5; margin-bottom: 1.25rem;">
          新版二进制程序已原子落盘生效，Systemd 守护进程正在重新载入并拉起服务，正在持续探活等待重连...
        </p>
        <div style="display: flex; align-items: center; justify-content: center; gap: 0.5rem; font-size: 0.8125rem; color: #38bdf8;">
          <span class="pulse-indicator pulse-active"></span>
          <span>已发起探活 {{ reconnectAttempts }} 次...</span>
        </div>
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

// 系统环境与版本概览
const sysInfo = reactive({
  version: '',
  os: 'linux',
  arch: 'amd64',
  sha256: '',
})

// 安装包仓库与上传
const packages = ref<any[]>([])
const uploading = ref(false)
const isDragging = ref(false)
const fileInputRef = ref<HTMLInputElement | null>(null)
const isLocalUpgrading = ref(false)
const reconnecting = ref(false)
const reconnectAttempts = ref(0)

// 纳管节点与集群分发
const remoteNodes = ref<any[]>([])
const activeDistPkgId = ref('')
const selectedNodeIds = ref<number[]>([])
const isBatchUpgrading = ref(false)
const upgradingNodes = reactive<Record<number, boolean>>({})

// 系统基础配置与密码表单
const settings = reactive({
  secret_path: '',
  public_ip: '',
})

const passForm = reactive({
  old_password: '',
  new_password: '',
  confirm_password: '',
})

async function loadSystemOverview() {
  try {
    const res = await api.get('/system/overview')
    sysInfo.version = res.data.version || 'v1.0.1'
    sysInfo.os = res.data.os || 'linux'
    sysInfo.arch = res.data.arch || 'amd64'
    sysInfo.sha256 = res.data.sha256 || ''
  } catch {
    // 忽略错误
  }
}

async function loadPackages() {
  try {
    const res = await api.get('/system/upgrade/packages')
    packages.value = res.data || []
  } catch {
    packages.value = []
  }
}

async function loadRemoteNodes() {
  try {
    const res = await api.get('/nodes')
    remoteNodes.value = (res.data || []).filter((n: any) => !n.is_master)
  } catch {
    remoteNodes.value = []
  }
}

async function loadSettings() {
  try {
    const res = await api.get('/settings')
    settings.secret_path = res.data.secret_path
    settings.public_ip = res.data.public_ip
  } catch (err: any) {
    toast.error('加载系统设置失败: ' + (err.response?.data?.error || err.message))
  }
}

function triggerFileInput() {
  if (uploading.value) return
  fileInputRef.value?.click()
}

function handleFileSelected(event: Event) {
  const target = event.target as HTMLInputElement
  if (target.files && target.files.length > 0) {
    uploadPackageFile(target.files[0])
    target.value = ''
  }
}

function handleDrop(event: DragEvent) {
  isDragging.value = false
  if (event.dataTransfer?.files && event.dataTransfer.files.length > 0) {
    uploadPackageFile(event.dataTransfer.files[0])
  }
}

async function uploadPackageFile(file: File) {
  uploading.value = true
  const formData = new FormData()
  formData.append('package', file)

  try {
    const res = await api.post('/system/upgrade/package', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    toast.success(`安装包上传成功！架构: ${res.data.arch}，版本: ${res.data.version}`)
    await loadPackages()
  } catch (err: any) {
    toast.error('安装包校验或上传失败: ' + (err.response?.data?.error || err.message))
  } finally {
    uploading.value = false
  }
}

// 触发升级本机节点
async function triggerLocalUpgrade(pkg: any) {
  const ok = await showConfirm({
    title: '确认升级本机面板服务',
    message: `即将使用安装包 ${pkg.file_name} (${pkg.arch} - ${pkg.version}) 替换本机程序。升级后服务将平滑重启约 2~3 秒，页面将自动重连，是否继续？`,
    confirmText: '立即开始升级',
    danger: true,
  })
  if (!ok) return

  isLocalUpgrading.value = true
  try {
    await api.post('/system/upgrade/local', { package_id: pkg.package_id })
    // 进入全屏重连探活浮层
    startReconnectionLoop()
  } catch (err: any) {
    toast.error('触发本地升级失败: ' + (err.response?.data?.error || err.message))
    isLocalUpgrading.value = false
  }
}

// 本地升级后探活自愈重连
function startReconnectionLoop() {
  reconnecting.value = true
  reconnectAttempts.value = 0

  const interval = setInterval(async () => {
    reconnectAttempts.value++
    try {
      const res = await api.get('/system/overview')
      if (res.status === 200) {
        clearInterval(interval)
        reconnecting.value = false
        isLocalUpgrading.value = false
        toast.success('🎉 本地节点已成功平滑重启并升级完成！', 4000)
        await loadSystemOverview()
        await loadPackages()
      }
    } catch {
      // 正在重启中，等待下一次轮询
    }

    if (reconnectAttempts.value > 30) {
      clearInterval(interval)
      reconnecting.value = false
      isLocalUpgrading.value = false
      toast.warning('探活等待已达上限，请手动刷新页面检查服务状态。')
    }
  }, 1500)
}

// 展开/收起纳管节点分发选择面板
function toggleClusterDrawer(pkg: any) {
  if (activeDistPkgId.value === pkg.package_id) {
    activeDistPkgId.value = ''
  } else {
    activeDistPkgId.value = pkg.package_id
    selectedNodeIds.value = remoteNodes.value.map(n => n.id)
  }
}

function selectAllNodes() {
  selectedNodeIds.value = remoteNodes.value.map(n => n.id)
}

function deselectAllNodes() {
  selectedNodeIds.value = []
}

// 单个纳管节点免密升级
async function upgradeSingleRemoteNode(node: any, pkg: any) {
  upgradingNodes[node.id] = true
  try {
    await api.post('/nodes/upgrade', {
      node_id: node.id,
      mode: 'api',
      package_id: pkg.package_id,
      force: false,
      host: node.host,
    })
    toast.success(`已向节点 ${node.name} 触发免密平滑升级任务！可在「纳管服务器」查看实时日志流。`)
  } catch (err: any) {
    toast.error(`节点 ${node.name} 升级启动失败: ` + (err.response?.data?.error || err.message))
  } finally {
    upgradingNodes[node.id] = false
  }
}

// 批量免密升级选中的纳管节点
async function batchUpgradeRemoteNodes(pkg: any) {
  if (selectedNodeIds.value.length === 0) {
    toast.warning('请至少勾选一个纳管节点')
    return
  }

  const ok = await showConfirm({
    title: '确认批量免密升级',
    message: `即将向已选中的 ${selectedNodeIds.value.length} 个纳管节点推送安装包 ${pkg.file_name} 并触发平滑重启，是否继续？`,
    confirmText: '立即批量升级',
    danger: false,
  })
  if (!ok) return

  isBatchUpgrading.value = true
  let successCount = 0
  let failCount = 0

  for (const id of selectedNodeIds.value) {
    const node = remoteNodes.value.find(n => n.id === id)
    if (!node) continue
    upgradingNodes[id] = true
    try {
      await api.post('/nodes/upgrade', {
        node_id: id,
        mode: 'api',
        package_id: pkg.package_id,
        force: false,
        host: node.host,
      })
      successCount++
    } catch {
      failCount++
    } finally {
      upgradingNodes[id] = false
    }
  }

  isBatchUpgrading.value = false
  if (failCount === 0) {
    toast.success(`🎉 成功触发 ${successCount} 个节点的免密升级任务！`)
  } else {
    toast.warning(`批量升级已启动：成功 ${successCount} 个，失败 ${failCount} 个`)
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
  loadSystemOverview()
  loadPackages()
  loadRemoteNodes()
  loadSettings()
})
</script>

<style scoped>
.upload-dropzone:hover {
  border-color: #6366f1 !important;
  background: rgba(99, 102, 241, 0.08) !important;
}
.dropzone-active {
  border-color: #38bdf8 !important;
  background: rgba(56, 189, 248, 0.12) !important;
}

@keyframes indeterminate {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.pulse-indicator {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #38bdf8;
}

.pulse-active {
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(56, 189, 248, 0.7); }
  70% { transform: scale(1); box-shadow: 0 0 0 6px rgba(56, 189, 248, 0); }
  100% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(56, 189, 248, 0); }
}
</style>
