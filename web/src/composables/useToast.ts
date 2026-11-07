import { ref } from 'vue'

export interface ToastItem {
  id: number
  type: 'success' | 'error' | 'warning' | 'info'
  message: string
  duration: number
}

const toasts = ref<ToastItem[]>([])
let counter = 0

export function useToast() {
  function add(type: ToastItem['type'], message: string, duration = 3000) {
    const id = ++counter
    toasts.value.push({ id, type, message, duration })

    if (duration > 0) {
      setTimeout(() => {
        remove(id)
      }, duration)
    }
  }

  function remove(id: number) {
    toasts.value = toasts.value.filter((t) => t.id !== id)
  }

  const toast = {
    success: (msg: string, duration = 3000) => add('success', msg, duration),
    error: (msg: string, duration = 4500) => add('error', msg, duration),
    warning: (msg: string, duration = 3500) => add('warning', msg, duration),
    info: (msg: string, duration = 3000) => add('info', msg, duration),
    remove,
  }

  return {
    toasts,
    toast,
  }
}

export async function copyToClipboard(text: string, label = '内容'): Promise<boolean> {
  const { toast } = useToast()
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      // 兼容非 HTTPS 或不支持 clipboard API 的场景
      const textArea = document.createElement('textarea')
      textArea.value = text
      textArea.style.position = 'fixed'
      textArea.style.opacity = '0'
      textArea.style.left = '-999999px'
      document.body.appendChild(textArea)
      textArea.focus()
      textArea.select()
      document.execCommand('copy')
      document.body.removeChild(textArea)
    }
    toast.success(`${label}已复制到剪贴板！`)
    return true
  } catch (err: any) {
    toast.error(`复制失败: ${err.message || '未知错误'}`)
    return false
  }
}

