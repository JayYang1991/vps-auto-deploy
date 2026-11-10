import axios from 'axios'
import { getToken, clearToken, recordActivity } from '../utils/auth'

// 从当前 URL 提取安全前缀
export function getSecretPath(): string {
  const parts = window.location.pathname.split('/').filter(Boolean)
  if (parts.length > 0) {
    return parts[0]
  }
  return localStorage.getItem('secret_path') || ''
}

const secret = getSecretPath()
const baseURL = secret ? `/${secret}/api` : '/api'

const api = axios.create({
  baseURL,
  timeout: 60000,
})

api.interceptors.request.use((config) => {
  const token = getToken()
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }

  // 记录请求活跃
  recordActivity()

  // 跨节点上下文透传：若当前选中的不是主控本机，注入 X-Target-Node-ID
  const activeNodeId = localStorage.getItem('active_node_id')
  if (activeNodeId && activeNodeId !== '1' && activeNodeId !== '0') {
    config.headers['X-Target-Node-ID'] = activeNodeId
  }

  return config
})

api.interceptors.response.use(
  (res) => {
    recordActivity()
    return res
  },
  (error) => {
    if (error.response?.status === 401) {
      clearToken('unauthorized')
      window.location.hash = '#/login'
    }
    return Promise.reject(error)
  }
)

export default api

