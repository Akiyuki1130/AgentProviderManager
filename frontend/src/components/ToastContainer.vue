<template>
  <div class="toast-container">
    <TransitionGroup name="toast">
      <div v-for="t in toasts" :key="t.id" class="toast" :class="t.type">
        <div class="toast-title">{{ t.title }}</div>
        <div v-if="t.message" class="toast-msg">{{ t.message }}</div>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

interface Toast { id: number; type: string; title: string; message: string }
const toasts = ref<Toast[]>([])
let nextId = 1

function showToast(type: string, title: string, message = '') {
  const id = nextId++
  toasts.value.push({ id, type, title, message })
  setTimeout(() => { toasts.value = toasts.value.filter((t) => t.id !== id) }, 3500)
}

defineExpose({ showToast })
// Also expose globally
declare global { interface Window { __toast?: (type: string, title: string, message?: string) => void } }
import { onMounted } from 'vue'
onMounted(() => { window.__toast = showToast })
</script>

<style scoped>
.toast-container { position: fixed; bottom: 24px; right: 16px; z-index: 9999; display: flex; flex-direction: column; gap: 8px; pointer-events: none; }
.toast { background: var(--fluent-card-bg); border-left: 3px solid #888; border-radius: 8px; padding: 12px 16px; max-width: 360px; box-shadow: var(--fluent-shadow-md); pointer-events: auto; }
.toast.success { border-left-color: #107c10; }
.toast.error { border-left-color: #d13438; }
.toast.info { border-left-color: #0078d4; }
.toast.warning { border-left-color: #ca5010; }
.toast-title { font-weight: 600; font-size: 13px; }
.toast-msg { font-size: 12px; color: var(--fluent-text-soft); white-space: pre-wrap; margin-top: 4px; }
.toast-enter-active { transition: all 0.25s ease; }
.toast-enter-from { opacity: 0; transform: translateX(20px); }
.toast-leave-active { transition: all 0.2s ease; position: absolute; }
.toast-leave-to { opacity: 0; transform: translateX(20px); }
.toast-move { transition: transform 0.25s ease; }
</style>
