<template>
  <div class="toast-wrapper">
    <transition-group name="toast-slide" tag="div" class="toast-list">
      <div
        v-for="item in toasts"
        :key="item.id"
        :class="['toast-card', `toast-${item.type}`]"
      >
        <span class="toast-icon">
          <span v-if="item.type === 'success'">✓</span>
          <span v-else-if="item.type === 'error'">✕</span>
          <span v-else-if="item.type === 'warning'">⚠️</span>
          <span v-else>ℹ️</span>
        </span>
        <div class="toast-message">{{ item.message }}</div>
        <button @click="toast.remove(item.id)" class="toast-close" title="关闭">
          ✕
        </button>
      </div>
    </transition-group>
  </div>
</template>

<script setup lang="ts">
import { useToast } from '../composables/useToast'

const { toasts, toast } = useToast()
</script>

<style scoped>
.toast-wrapper {
  position: fixed;
  top: 1.25rem;
  right: 1.25rem;
  z-index: 9999;
  pointer-events: none;
  max-width: 420px;
  width: calc(100% - 2.5rem);
}

.toast-list {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.toast-card {
  pointer-events: auto;
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.4), 0 8px 10px -6px rgba(0, 0, 0, 0.4);
  backdrop-filter: blur(8px);
  border: 1px solid transparent;
  transition: all 0.25s ease;
}

.toast-success {
  background-color: #064e3b;
  border-color: #059669;
  color: #a7f3d0;
}

.toast-error {
  background-color: #7f1d1d;
  border-color: #dc2626;
  color: #fecaca;
}

.toast-warning {
  background-color: #78350f;
  border-color: #d97706;
  color: #fde68a;
}

.toast-info {
  background-color: #1e3a8a;
  border-color: #2563eb;
  color: #bfdbfe;
}

.toast-icon {
  font-size: 1rem;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.toast-message {
  flex: 1;
  line-height: 1.4;
  word-break: break-word;
}

.toast-close {
  background: transparent;
  border: none;
  color: inherit;
  opacity: 0.6;
  cursor: pointer;
  padding: 0.2rem;
  font-size: 0.75rem;
  transition: opacity 0.15s;
  flex-shrink: 0;
}

.toast-close:hover {
  opacity: 1;
}

/* Animations */
.toast-slide-enter-active,
.toast-slide-leave-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

.toast-slide-enter-from {
  opacity: 0;
  transform: translateX(30px) scale(0.95);
}

.toast-slide-leave-to {
  opacity: 0;
  transform: translateY(-20px) scale(0.95);
}
</style>

