import { ref, computed } from 'vue'
import api from '../api'

export interface NodeItem {
  id: number
  name: string
  sub_prefix?: string
  host: string
  ssh_port: number
  panel_port: number
  secret_path: string
  username: string
  is_master: boolean
  use_tls: boolean
  cert_fingerprint: string
  ssh_host_key: string
  cloud_provider?: string
  cloud_instance_id?: string
  region?: string
  plan?: string
  status: 'online' | 'offline' | 'deploying' | 'error'
  os_info: string
  latency_ms: number
  created_at: string
  updated_at: string
}

const activeNodeId = ref<number>(
  parseInt(localStorage.getItem('active_node_id') || '1', 10)
)

const nodes = ref<NodeItem[]>([])
const loadingNodes = ref<boolean>(false)

export function useNodeContext() {
  const activeNode = computed(() => {
    return nodes.value.find((n) => n.id === activeNodeId.value) || nodes.value[0] || null
  })

  const isMasterNode = computed(() => {
    return !activeNode.value || activeNode.value.id === 1 || activeNode.value.is_master
  })

  async function pingNode(id: number) {
    if (id <= 1) return
    try {
      const res = await api.post<{ latency_ms: number; status: 'online' | 'offline' | 'error' }>(`/nodes/${id}/ping`)
      if (res.data) {
        const target = nodes.value.find((n) => n.id === id)
        if (target) {
          target.status = res.data.status
          target.latency_ms = res.data.latency_ms
        }
      }
    } catch (err) {
      // 忽略前端单独测速错误，避免阻塞主流程
    }
  }

  async function fetchNodes() {
    loadingNodes.value = true
    try {
      const res = await api.get<NodeItem[]>('/nodes')
      nodes.value = res.data || []
      // 若当前选中的节点不存在于列表中，自动回滚至主控本机
      if (!nodes.value.some((n) => n.id === activeNodeId.value)) {
        activeNodeId.value = 1
        localStorage.setItem('active_node_id', '1')
      } else if (activeNodeId.value > 1) {
        // 后台静默刷新当前活跃被纳管节点的真实延迟与在线状态
        pingNode(activeNodeId.value)
      }
    } catch (err) {
      console.error('Failed to fetch nodes:', err)
    } finally {
      loadingNodes.value = false
    }
  }

  function switchNode(id: number) {
    if (activeNodeId.value === id) return
    activeNodeId.value = id
    localStorage.setItem('active_node_id', id.toString())
    window.dispatchEvent(new CustomEvent('node-context-changed', { detail: { nodeId: id } }))
    if (id > 1) {
      pingNode(id)
    }
  }

  return {
    activeNodeId,
    activeNode,
    isMasterNode,
    nodes,
    loadingNodes,
    fetchNodes,
    switchNode,
    pingNode,
  }
}

