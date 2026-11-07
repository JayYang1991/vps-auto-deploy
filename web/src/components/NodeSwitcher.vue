<template>
  <div class="node-switcher-wrapper" ref="wrapperRef">
    <!-- 节点切换胶囊按钮 -->
    <button
      class="node-capsule"
      :class="{ 'node-capsule-remote': !isMasterNode, 'node-capsule-open': isOpen }"
      @click="toggleDropdown"
      type="button"
      title="切换当前纳管的服务器节点"
    >
      <span class="node-icon">{{ isMasterNode ? '🏠' : '🌐' }}</span>
      <span class="node-name">{{ activeNode ? activeNode.name : '本机 (Master)' }}</span>
      
      <span
        v-if="activeNode"
        class="node-ping"
        :class="{
          'ping-good': activeNode.status === 'online' && activeNode.latency_ms < 100,
          'ping-medium': activeNode.status === 'online' && activeNode.latency_ms >= 100,
          'ping-bad': activeNode.status !== 'online'
        }"
      >
        <span class="ping-dot"></span>
        {{ activeNode.status === 'online' ? `${activeNode.latency_ms}ms` : '离线' }}
      </span>

      <span class="dropdown-arrow">▾</span>
    </button>

    <!-- 下拉菜单 -->
    <div v-if="isOpen" class="node-dropdown">
      <div class="dropdown-header">
        <div class="dropdown-title">
          <span>🖥️ 集群节点</span>
          <span class="node-count">({{ nodes.length }})</span>
        </div>
        <button class="manage-btn" @click="handleOpenCluster">
          ⚙️ 节点管理
        </button>
      </div>

      <div class="node-list">
        <div
          v-for="node in nodes"
          :key="node.id"
          class="node-item"
          :class="{ 'node-item-active': node.id === activeNodeId }"
          @click="selectNode(node.id)"
        >
          <div class="node-item-main">
            <span class="item-icon">{{ node.is_master ? '🏠' : '🌐' }}</span>
            <div class="item-info">
              <div class="item-name-row">
                <span class="item-name">{{ node.name }}</span>
                <span v-if="node.is_master" class="badge-master">主控</span>
              </div>
              <span class="item-host">{{ node.host }}:{{ node.panel_port }}</span>
            </div>
          </div>

          <div class="node-item-meta">
            <span
              class="status-indicator"
              :class="{
                'status-online': node.status === 'online',
                'status-offline': node.status !== 'online'
              }"
            >
              {{ node.status === 'online' ? `${node.latency_ms}ms` : '离线' }}
            </span>
            <span v-if="node.id === activeNodeId" class="check-icon">✓</span>
          </div>
        </div>
      </div>

      <div class="dropdown-footer">
        <button class="deploy-shortcut-btn btn-cloud-shortcut" @click="handleCloudShortcut">
          <span>☁️</span>
          <span>云端创建 VPS</span>
        </button>
        <button class="deploy-shortcut-btn" @click="handleDeployShortcut">
          <span>➕</span>
          <span>SSH 部署</span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useNodeContext } from '../composables/useNodeContext'

const emit = defineEmits<{
  (e: 'open-cluster-modal', initialTab?: 'nodes' | 'cloud' | 'deploy'): void
}>()

const {
  activeNodeId,
  activeNode,
  isMasterNode,
  nodes,
  fetchNodes,
  switchNode,
} = useNodeContext()

const isOpen = ref(false)
const wrapperRef = ref<HTMLElement | null>(null)

function toggleDropdown() {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    fetchNodes()
  }
}

function selectNode(id: number) {
  switchNode(id)
  isOpen.value = false
}

function handleOpenCluster() {
  isOpen.value = false
  emit('open-cluster-modal', 'nodes')
}

function handleCloudShortcut() {
  isOpen.value = false
  emit('open-cluster-modal', 'cloud')
}

function handleDeployShortcut() {
  isOpen.value = false
  emit('open-cluster-modal', 'deploy')
}

function handleClickOutside(e: MouseEvent) {
  if (wrapperRef.value && !wrapperRef.value.contains(e.target as Node)) {
    isOpen.value = false
  }
}

let pollTimer: any = null

onMounted(() => {
  fetchNodes()
  document.addEventListener('click', handleClickOutside)
  pollTimer = setInterval(() => {
    fetchNodes()
  }, 30000)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (pollTimer) {
    clearInterval(pollTimer)
  }
})
</script>

<style scoped>
.node-switcher-wrapper {
  position: relative;
  display: inline-block;
}

.node-capsule {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  background: rgba(30, 41, 59, 0.7);
  border: 1px solid rgba(71, 85, 105, 0.4);
  padding: 0.35rem 0.75rem;
  border-radius: 9999px;
  color: #f1f5f9;
  font-size: 0.8125rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s ease;
  backdrop-filter: blur(8px);
}

.node-capsule:hover {
  background: rgba(51, 65, 85, 0.8);
  border-color: rgba(148, 163, 184, 0.5);
}

.node-capsule-remote {
  border-color: rgba(99, 102, 241, 0.4);
  background: rgba(99, 102, 241, 0.12);
}

.node-capsule-remote:hover {
  background: rgba(99, 102, 241, 0.2);
  border-color: rgba(99, 102, 241, 0.6);
}

.node-capsule-open {
  border-color: #6366f1;
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.2);
}

.node-icon {
  font-size: 0.95rem;
}

.node-name {
  max-width: 140px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-ping {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  font-size: 0.7rem;
  padding: 0.1rem 0.45rem;
  border-radius: 9999px;
  font-family: ui-monospace, monospace;
}

.ping-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.ping-good {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
}
.ping-good .ping-dot {
  background: #10b981;
}

.ping-medium {
  background: rgba(245, 158, 11, 0.15);
  color: #fbbf24;
}
.ping-medium .ping-dot {
  background: #f59e0b;
}

.ping-bad {
  background: rgba(239, 68, 68, 0.15);
  color: #f87171;
}
.ping-bad .ping-dot {
  background: #ef4444;
}

.dropdown-arrow {
  font-size: 0.75rem;
  color: #94a3b8;
  margin-left: 0.15rem;
}

/* 下拉菜单面板 */
.node-dropdown {
  position: absolute;
  top: calc(100% + 0.5rem);
  left: 0;
  width: 290px;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 0.75rem;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5), 0 8px 10px -6px rgba(0, 0, 0, 0.5);
  z-index: 100;
  overflow: hidden;
  backdrop-filter: blur(12px);
  animation: slideDown 0.15s ease-out;
}

@keyframes slideDown {
  from {
    opacity: 0;
    transform: translateY(-6px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.dropdown-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.65rem 0.85rem;
  border-bottom: 1px solid #1e293b;
  background: #1e293b;
}

.dropdown-title {
  font-size: 0.8125rem;
  font-weight: 600;
  color: #f8fafc;
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.node-count {
  font-size: 0.75rem;
  color: #94a3b8;
}

.manage-btn {
  background: transparent;
  border: none;
  color: #818cf8;
  font-size: 0.75rem;
  cursor: pointer;
  padding: 0.2rem 0.4rem;
  border-radius: 0.25rem;
  transition: all 0.15s ease;
}

.manage-btn:hover {
  background: rgba(129, 140, 248, 0.15);
  color: #a5b4fc;
}

.node-list {
  max-height: 240px;
  overflow-y: auto;
  padding: 0.35rem;
}

.node-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.55rem 0.65rem;
  border-radius: 0.5rem;
  cursor: pointer;
  transition: background 0.15s ease;
}

.node-item:hover {
  background: #1e293b;
}

.node-item-active {
  background: rgba(99, 102, 241, 0.15);
  border: 1px solid rgba(99, 102, 241, 0.3);
}

.node-item-main {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  overflow: hidden;
}

.item-icon {
  font-size: 1.1rem;
}

.item-info {
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.item-name-row {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.item-name {
  font-size: 0.8125rem;
  font-weight: 500;
  color: #f1f5f9;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 130px;
}

.badge-master {
  font-size: 0.625rem;
  background: rgba(59, 130, 246, 0.2);
  color: #60a5fa;
  padding: 0.05rem 0.3rem;
  border-radius: 0.25rem;
}

.item-host {
  font-size: 0.7rem;
  color: #64748b;
  font-family: ui-monospace, monospace;
}

.node-item-meta {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.status-indicator {
  font-size: 0.675rem;
  font-family: ui-monospace, monospace;
  padding: 0.1rem 0.35rem;
  border-radius: 0.25rem;
}

.status-online {
  color: #34d399;
  background: rgba(16, 185, 129, 0.1);
}

.status-offline {
  color: #f87171;
  background: rgba(239, 68, 68, 0.1);
}

.check-icon {
  color: #6366f1;
  font-size: 0.875rem;
  font-weight: bold;
}

.dropdown-footer {
  padding: 0.45rem 0.5rem;
  border-top: 1px solid #1e293b;
  background: #0b1120;
  display: flex;
  gap: 0.4rem;
}

.deploy-shortcut-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.35rem;
  background: rgba(99, 102, 241, 0.12);
  border: 1px dashed rgba(99, 102, 241, 0.4);
  color: #a5b4fc;
  padding: 0.4rem 0.25rem;
  border-radius: 0.45rem;
  font-size: 0.725rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.deploy-shortcut-btn:hover {
  background: rgba(99, 102, 241, 0.22);
  color: #ffffff;
  border-color: #6366f1;
}

.btn-cloud-shortcut {
  background: rgba(14, 165, 233, 0.12);
  border-color: rgba(14, 165, 233, 0.4);
  color: #7dd3fc;
}

.btn-cloud-shortcut:hover {
  background: rgba(14, 165, 233, 0.22);
  color: #ffffff;
  border-color: #0ea5e9;
}
</style>

