<template>
  <div v-if="confirmState.isOpen" class="modal-overlay" @click.self="handleCancel">
    <div class="modal-content confirm-box" role="dialog" aria-modal="true">
      <div class="confirm-header">
        <div :class="['confirm-icon', confirmState.danger ? 'icon-danger' : 'icon-info']">
          <span v-if="confirmState.danger">⚠️</span>
          <span v-else>❓</span>
        </div>
        <div class="confirm-title">{{ confirmState.title }}</div>
      </div>

      <div class="confirm-body">
        <p>{{ confirmState.message }}</p>
      </div>

      <div class="confirm-footer">
        <button
          @click="handleCancel"
          class="btn btn-secondary"
          type="button"
        >
          {{ confirmState.cancelText || '取消' }}
        </button>
        <button
          @click="handleConfirm"
          :class="['btn', confirmState.danger ? 'btn-danger' : 'btn-primary']"
          type="button"
          autofocus
        >
          {{ confirmState.confirmText || '确定' }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import { useConfirm } from '../composables/useConfirm'

const { confirmState, handleConfirm, handleCancel } = useConfirm()

function handleKeyDown(e: KeyboardEvent) {
  if (!confirmState.value.isOpen) return
  if (e.key === 'Escape') {
    handleCancel()
  } else if (e.key === 'Enter') {
    handleConfirm()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeyDown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeyDown)
})
</script>

<style scoped>
.confirm-box {
  max-width: 440px;
  width: 90%;
  background-color: #1e293b;
  border: 1px solid #334155;
  border-radius: 0.75rem;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5), 0 8px 10px -6px rgba(0, 0, 0, 0.5);
  animation: confirm-pop 0.2s cubic-bezier(0.16, 1, 0.3, 1);
}

@keyframes confirm-pop {
  from {
    opacity: 0;
    transform: scale(0.92);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

.confirm-header {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.75rem;
}

.confirm-icon {
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 9999px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.125rem;
  flex-shrink: 0;
}

.icon-danger {
  background-color: rgba(239, 68, 68, 0.15);
  border: 1px solid rgba(239, 68, 68, 0.3);
}

.icon-info {
  background-color: rgba(59, 130, 246, 0.15);
  border: 1px solid rgba(59, 130, 246, 0.3);
}

.confirm-title {
  font-size: 1.1rem;
  font-weight: 600;
  color: #f8fafc;
}

.confirm-body {
  color: #94a3b8;
  font-size: 0.9rem;
  line-height: 1.5;
  margin-bottom: 1.5rem;
  white-space: pre-wrap;
}

.confirm-footer {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
}
</style>

