import { ref } from 'vue'

export interface ConfirmOptions {
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
}

interface ConfirmState extends ConfirmOptions {
  isOpen: boolean
  resolve: (value: boolean) => void
}

const confirmState = ref<ConfirmState>({
  isOpen: false,
  title: '提示',
  message: '',
  confirmText: '确定',
  cancelText: '取消',
  danger: false,
  resolve: () => {},
})

export function useConfirm() {
  function showConfirm(options: ConfirmOptions): Promise<boolean> {
    return new Promise((resolve) => {
      confirmState.value = {
        isOpen: true,
        title: options.title || (options.danger ? '危险操作确认' : '操作确认'),
        message: options.message,
        confirmText: options.confirmText || '确定',
        cancelText: options.cancelText || '取消',
        danger: !!options.danger,
        resolve,
      }
    })
  }

  function handleConfirm() {
    confirmState.value.isOpen = false
    confirmState.value.resolve(true)
  }

  function handleCancel() {
    confirmState.value.isOpen = false
    confirmState.value.resolve(false)
  }

  return {
    confirmState,
    showConfirm,
    handleConfirm,
    handleCancel,
  }
}

