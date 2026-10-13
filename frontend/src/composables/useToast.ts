import { ref, readonly } from 'vue'

export type ToastType = 'info' | 'success' | 'warning' | 'error'

export interface ToastItem {
  id: number
  type: ToastType
  title: string
  msg: string
  duration: number
  timer: number | null
  remaining: number
  paused: boolean
  createdAt: number
}

const toasts = ref<ToastItem[]>([])
let seq = 0
const MAX_TOASTS = 5

function clearTimer(t: ToastItem) {
  if (t.timer !== null) {
    window.clearTimeout(t.timer)
    t.timer = null
  }
}

function remove(id: number) {
  const idx = toasts.value.findIndex((t) => t.id === id)
  if (idx === -1) return
  clearTimer(toasts.value[idx])
  toasts.value.splice(idx, 1)
}

function startTimer(t: ToastItem) {
  clearTimer(t)
  if (t.remaining <= 0) {
    remove(t.id)
    return
  }
  t.paused = false
  t.timer = window.setTimeout(() => {
    t.remaining = 0
    remove(t.id)
  }, t.remaining)
}

export function pushToast(type: ToastType, title: string, msg = '', duration?: number): number {
  const ms = duration ?? (type === 'error' ? 5200 : 3500)
  const id = ++seq
  const t: ToastItem = {
    id,
    type,
    title,
    msg,
    duration: ms,
    timer: null,
    remaining: ms,
    paused: false,
    createdAt: Date.now(),
  }
  toasts.value.push(t)
  while (toasts.value.length > MAX_TOASTS) remove(toasts.value[0].id)
  startTimer(t)
  return id
}

/** hover 暂停：按 createdAt 结算剩余时长 */
function pause(id: number) {
  const t = toasts.value.find((x) => x.id === id)
  if (!t || t.paused) return
  clearTimer(t)
  t.remaining = Math.max(0, t.remaining - (Date.now() - t.createdAt))
  t.paused = true
}

function resume(id: number) {
  const t = toasts.value.find((x) => x.id === id)
  if (!t || !t.paused) return
  t.createdAt = Date.now()
  startTimer(t)
}

export function useToast() {
  return {
    toasts: readonly(toasts),
    push: pushToast,
    remove,
    pause,
    resume,
  }
}
