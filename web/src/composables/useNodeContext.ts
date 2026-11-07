import { ref, computed } from 'vue'
import api from '../api'

export interface NodeItem {
  id: number
  name: string
  host: string
  ssh_port: number
  panel_port: number
  secret_path: string
  username: string
  is_master: boolean
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

  async function fetchNodes() {
    loadingNodes.value = true
    try {
      const res = await api.get<NodeItem[]>('/nodes')
      nodes.value = res.data || []
      // 若当前选中的节点不存在于列表中，自动回滚至主控本机
      if (!nodes.value.some((n) => n.id === activeNodeId.value)) {
        activeNodeId.value = 1
        localStorage.setItem('active_node_id', '1')
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
  }

  return {
    activeNodeId,
    activeNode,
    isMasterNode,
    nodes,
    loadingNodes,
    fetchNodes,
    switchNode,
  }
}
