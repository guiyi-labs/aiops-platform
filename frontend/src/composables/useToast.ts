import { ref } from 'vue'

export type ToastType = 'success' | 'error' | 'warning' | 'info'

export interface ToastItem {
  id: number
  type: ToastType
  message: string
  duration: number
}

// Module-level singleton so any component can call toast.success(...) without
// needing a ref to the AppToast instance.
const toasts = ref<ToastItem[]>([])
let nextId = 0

function dismiss(id: number) {
  const idx = toasts.value.findIndex(t => t.id === id)
  if (idx !== -1) toasts.value.splice(idx, 1)
}

function show(type: ToastType, message: string, duration = 3500) {
  const id = nextId++
  toasts.value.push({ id, type, message, duration })
  if (duration > 0) {
    setTimeout(() => dismiss(id), duration)
  }
  return id
}

export function useToast() {
  function success(message: string, duration?: number) { return show('success', message, duration) }
  function error(message: string, duration?: number) { return show('error', message, duration ?? 5000) }
  function warning(message: string, duration?: number) { return show('warning', message, duration) }
  function info(message: string, duration?: number) { return show('info', message, duration) }

  return { toasts, show, success, error, warning, info, dismiss }
}
