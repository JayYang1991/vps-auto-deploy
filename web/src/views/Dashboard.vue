<template>
  <div>
    <!-- 头部欢迎与快速状态 -->
    <div style="margin-bottom: 2rem; display: flex; justify-content: space-between; align-items: flex-start; flex-wrap: wrap; gap: 1rem;">
      <div>
        <div style="display: flex; align-items: center; gap: 0.5rem;">
          <span style="font-size: 1.5rem;">📊</span>
          <h1 style="font-size: 1.5rem; font-weight: 700; color: #f8fafc;">系统概览与监控</h1>
          <span class="badge badge-success" style="font-size: 0.75rem;">控制端运行中</span>
        </div>
        <p style="color: #94a3b8; font-size: 0.875rem; margin-top: 0.4rem;">
          实时监控主机硬件指标、底层网络协议与各代理/穿透组件的运行健康状态。
        </p>
      </div>

      <div style="display: flex; gap: 0.75rem;">
        <router-link to="/components" class="btn btn-primary" style="padding: 0.45rem 1rem;">
          <span>📦</span>
          <span>进入软件中心</span>
        </router-link>
      </div>
    </div>

    <!-- 主机硬件与系统信息卡片 -->
    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 1.25rem; margin-bottom: 2rem;">
      <div class="card">
        <div style="color: #94a3b8; font-size: 0.85rem; display: flex; justify-content: space-between;">
          <span>公网 IPv4</span>
          <span>🌐</span>
        </div>
        <div style="font-size: 1.35rem; font-weight: 700; margin-top: 0.5rem; color: #38bdf8;">
          {{ displayPublicIP }}
        </div>
        <div style="color: #64748b; font-size: 0.75rem; margin-top: 0.35rem;">用于入站连接与 DNS 解析</div>
      </div>

      <div class="card">
        <div style="color: #94a3b8; font-size: 0.85rem; display: flex; justify-content: space-between;">
          <span>系统与 CPU</span>
          <span>💻</span>
        </div>
        <div style="font-size: 1.2rem; font-weight: 600; margin-top: 0.5rem; color: #f1f5f9;">
          {{ overview.os || 'Linux' }} / {{ overview.arch || 'amd64' }}
        </div>
        <div style="color: #64748b; font-size: 0.75rem; margin-top: 0.35rem;">
          {{ overview.num_cpu || 1 }} 逻辑核心
        </div>
      </div>

      <div class="card">
        <div style="color: #94a3b8; font-size: 0.85rem; display: flex; justify-content: space-between; align-items: center;">
          <span>TCP BBR 加速</span>
          <button
            v-if="!overview.bbr"
            @click="enableBBR"
            class="btn btn-secondary"
            style="padding: 0.2rem 0.5rem; font-size: 0.75rem;"
          >
            一键开启
          </button>
        </div>
        <div style="font-size: 1.2rem; font-weight: 600; margin-top: 0.5rem;">
          <span :class="['badge', overview.bbr ? 'badge-success' : 'badge-warning']">
            {{ overview.bbr ? '✓ 已启用 (BBR 运行中)' : '未启用' }}
          </span>
        </div>
        <div style="color: #64748b; font-size: 0.75rem; margin-top: 0.35rem;">显著降低高丢包线路延迟</div>
      </div>

      <div class="card">
        <div style="color: #94a3b8; font-size: 0.85rem; display: flex; justify-content: space-between;">
          <span>安全访问路径</span>
          <span>🔒</span>
        </div>
        <div style="font-size: 1.2rem; font-weight: 600; margin-top: 0.5rem; color: #a78bfa;">
          /{{ overview.secret_path }}/
        </div>
        <div style="color: #64748b; font-size: 0.75rem; margin-top: 0.35rem;">URL 防扫描伪装路径</div>
      </div>
    </div>

    <!-- 基础软件与核心驱动状态看板 -->
    <div class="card" style="margin-bottom: 2rem;">
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1.25rem;">
        <div>
          <h2 style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">基础设施与软件状态</h2>
          <p style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.25rem;">
            底层代理核心与穿透服务的部署与健康状态汇总
          </p>
        </div>
        <router-link to="/components" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.35rem 0.75rem;">
          管理所有组件 ➔
        </router-link>
      </div>

      <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 1rem;">
        <div
          v-for="c in overview.components"
          :key="c.name"
          style="background-color: #0f172a; border: 1px solid #334155; border-radius: 0.5rem; padding: 1rem; display: flex; justify-content: space-between; align-items: center;"
        >
          <div>
            <div style="display: flex; align-items: center; gap: 0.5rem;">
              <span style="font-weight: 600; font-size: 0.95rem; color: #f8fafc;">{{ c.display_name || c.name }}</span>
              <span class="badge badge-secondary" style="font-size: 0.65rem;">
                {{ getCategoryLabel(c.category) }}
              </span>
            </div>
            <div style="color: #94a3b8; font-size: 0.8rem; margin-top: 0.35rem;">
              版本: {{ c.installed_version || '未安装' }}
            </div>
          </div>

          <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 0.5rem;">
            <span :class="['badge', getStatusBadge(c.running_status)]">
              {{ getStatusText(c.running_status) }}
            </span>
            <router-link
              v-if="!c.installed"
              :to="'/components?install=' + c.name"
              class="btn btn-primary"
              style="padding: 0.2rem 0.5rem; font-size: 0.75rem;"
            >
              去安装
            </router-link>
          </div>
        </div>
      </div>
    </div>

    <!-- 业务快捷导航与入口 -->
    <div>
      <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 1rem;">
        <h2 style="font-size: 1.15rem; font-weight: 600; color: #f8fafc;">
          🛠️ 业务与服务配置入口
        </h2>
        <router-link to="/services" class="btn btn-secondary" style="font-size: 0.8rem; padding: 0.35rem 0.75rem;">
          前往配置中心 ➔
        </router-link>
      </div>

      <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 1.25rem;">
        <router-link to="/services?tab=singbox&sub=inbounds" class="card" style="text-decoration: none; color: inherit; transition: transform 0.15s ease, border-color 0.15s ease; border-color: #334155;">
          <div style="display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem;">
            <span style="font-size: 1.5rem;">🌐</span>
            <h3 style="font-size: 1.05rem; font-weight: 600; color: #f8fafc;">Sing-box 核心配置 (入站/出站)</h3>
          </div>
          <p style="color: #94a3b8; font-size: 0.825rem; line-height: 1.45;">
            配置 VLESS-Reality 入站节点、出站网络出口及分流规则，支持实时语法检验。
          </p>
        </router-link>

        <router-link to="/services?tab=cloudflared" class="card" style="text-decoration: none; color: inherit; transition: transform 0.15s ease, border-color 0.15s ease; border-color: #334155;">
          <div style="display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem;">
            <span style="font-size: 1.5rem;">🚇</span>
            <h3 style="font-size: 1.05rem; font-weight: 600; color: #f8fafc;">Cloudflare Argo 隧道</h3>
          </div>
          <p style="color: #94a3b8; font-size: 0.825rem; line-height: 1.45;">
            配置 Cloudflare Zero Trust 隧道穿透，拯救被墙 IP 或实现无公网 IP 节点映射。
          </p>
        </router-link>

        <router-link to="/services?tab=subscriptions" class="card" style="text-decoration: none; color: inherit; transition: transform 0.15s ease, border-color 0.15s ease; border-color: #334155;">
          <div style="display: flex; align-items: center; gap: 0.75rem; margin-bottom: 0.5rem;">
            <span style="font-size: 1.5rem;">👥</span>
            <h3 style="font-size: 1.05rem; font-weight: 600; color: #f8fafc;">成员与客户端订阅</h3>
          </div>
          <p style="color: #94a3b8; font-size: 0.825rem; line-height: 1.45;">
            管理授权用户，一键生成 Clash Verge、Sing-box、v2rayN 客户端聚合订阅。
          </p>
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import api from '../api'
import { useToast } from '../composables/useToast'
import { useNodeContext } from '../composables/useNodeContext'

const { toast } = useToast()
const { activeNode } = useNodeContext()

const overview = ref<any>({
  public_ip: '',
  os: '',
  arch: '',
  num_cpu: 0,
  bbr: false,
  components: [],
  secret_path: '',
})

// 优先采用系统实际检测的公网 IP；若检测值为空或 127.0.0.1 且当前为受管节点，优雅回退至该节点的直连 Host
const displayPublicIP = computed(() => {
  if (overview.value.public_ip && overview.value.public_ip !== '127.0.0.1') {
    return overview.value.public_ip
  }
  if (activeNode.value && !activeNode.value.is_master && activeNode.value.host) {
    return activeNode.value.host
  }
  return overview.value.public_ip || '检测中...'
})

async function loadOverview() {
  try {
    const res = await api.get('/system/overview')
    if (res.data) {
      overview.value = {
        ...overview.value,
        ...res.data,
      }
    }
  } catch (err: any) {
    console.error('加载系统概览失败', err)
  }

  try {
    const compRes = await api.get('/components')
    if (compRes.data) {
      overview.value.components = compRes.data
    }
  } catch (err: any) {
    console.error('加载组件概览失败', err)
  }
}

async function enableBBR() {
  try {
    await api.post('/system/bbr')
    toast.success('BBR 拥塞控制算法已成功开启！')
    await loadOverview()
  } catch (err: any) {
    toast.error('开启失败: ' + (err.response?.data?.error || err.message))
  }
}

function getCategoryLabel(cat: string) {
  const map: Record<string, string> = {
    proxy: '代理核心',
    tunnel: '隧道穿透',
    converter: '订阅与规则',
    network: '网络工具',
  }
  return map[cat] || cat
}

function getStatusBadge(status: string) {
  switch (status) {
    case 'running':
    case 'active':
      return 'badge-success'
    case 'activating':
    case 'reloading':
      return 'badge-info'
    case 'deactivating':
      return 'badge-warning'
    case 'stopped':
    case 'inactive':
      return 'badge-secondary'
    case 'not_installed':
      return 'badge-warning'
    case 'failed':
      return 'badge-danger'
    default:
      return 'badge-secondary'
  }
}

function getStatusText(status: string) {
  switch (status) {
    case 'running':
    case 'active':
      return '已激活 (运行中)'
    case 'activating':
      return '激活中 (启动中)'
    case 'reloading':
      return '重载中'
    case 'deactivating':
      return '停止中'
    case 'stopped':
    case 'inactive':
      return '未激活 (已停止)'
    case 'not_installed':
      return '未安装'
    case 'failed':
      return '异常 (失败)'
    default:
      return status || '未激活'
  }
}

onMounted(() => {
  loadOverview()
  window.addEventListener('node-context-changed', loadOverview)
})

onUnmounted(() => {
  window.removeEventListener('node-context-changed', loadOverview)
})
</script>
