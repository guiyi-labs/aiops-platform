<script setup lang="ts">
import { CheckCircle2, AlertTriangle, Info, X, XCircle } from 'lucide-vue-next'
import { ref } from 'vue'

export type ToastType = 'success' | 'error' | 'warning' | 'info'

interface ToastItem {
  id: number
  type: ToastType
  message: string
  duration: number
}

const props = withDefaults(defineProps<{
  /** Global position: top-right (default), top-center, bottom-right */
  position?: 'top-right' | 'top-center' | 'bottom-right'
}>(), {
  position: 'top-right',
})

const toasts = ref<ToastItem[]>([])
let nextId = 0

const icons: Record<ToastType, typeof CheckCircle2> = {
  success: CheckCircle2,
  error: XCircle,
  warning: AlertTriangle,
  info: Info,
}

const colors: Record<ToastType, string> = {
  success: 'toast--success',
  error: 'toast--error',
  warning: 'toast--warning',
  info: 'toast--info',
}

function show(type: ToastType, message: string, duration = 3500) {
  const id = nextId++
  toasts.value.push({ id, type, message, duration })
  if (duration > 0) {
    setTimeout(() => dismiss(id), duration)
  }
  return id
}

function dismiss(id: number) {
  const idx = toasts.value.findIndex(t => t.id === id)
  if (idx !== -1) toasts.value.splice(idx, 1)
}

function success(message: string, duration?: number) { return show('success', message, duration) }
function error(message: string, duration?: number) { return show('error', message, duration ?? 5000) }
function warning(message: string, duration?: number) { return show('warning', message, duration) }
function info(message: string, duration?: number) { return show('info', message, duration) }

// Expose for parent usage
defineExpose({ show, success, error, warning, info, dismiss })
</script>

<template>
  <Teleport to="body">
    <div :class="['toast-container', `toast-container--${position}`]" aria-live="polite">
      <TransitionGroup name="toast">
        <div
          v-for="t in toasts"
          :key="t.id"
          :class="['toast', colors[t.type]]"
          role="alert"
        >
          <component :is="icons[t.type]" :size="16" class="toast-icon" />
          <span class="toast-message">{{ t.message }}</span>
          <button type="button" class="toast-close" aria-label="关闭" @click="dismiss(t.id)">
            <X :size="14" />
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toast-container {
  position: fixed;
  z-index: 99999;
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: none;
  max-width: 400px;
}
.toast-container--top-right { top: 20px; right: 20px; }
.toast-container--top-center { top: 20px; left: 50%; transform: translateX(-50%); }
.toast-container--bottom-right { bottom: 20px; right: 20px; }

.toast {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 16px;
  border-radius: 8px;
  pointer-events: auto;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
  font-size: 13px;
  font-weight: 500;
  line-height: 1.4;
}

.toast--success {
  color: #166534;
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
}
.toast--error {
  color: #991b1b;
  background: #fef2f2;
  border: 1px solid #fecaca;
}
.toast--warning {
  color: #92400e;
  background: #fffbeb;
  border: 1px solid #fde68a;
}
.toast--info {
  color: #1e40af;
  background: #eff6ff;
  border: 1px solid #bfdbfe;
}

.toast-icon { flex: 0 0 auto; }
.toast-message { flex: 1; min-width: 0; }
.toast-close {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  color: inherit;
  opacity: 0.5;
  background: none;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: opacity 0.15s ease;
}
.toast-close:hover { opacity: 1; }

/* Transition */
.toast-enter-active { animation: toast-in 0.3s cubic-bezier(0.22, 1, 0.36, 1); }
.toast-leave-active { animation: toast-out 0.2s ease forwards; }
.toast-move { transition: transform 0.3s ease; }

@keyframes toast-in {
  from { opacity: 0; transform: translateX(24px) scale(0.96); }
  to { opacity: 1; transform: translateX(0) scale(1); }
}
@keyframes toast-out {
  from { opacity: 1; transform: translateX(0) scale(1); }
  to { opacity: 0; transform: translateX(24px) scale(0.96); }
}
</style>
