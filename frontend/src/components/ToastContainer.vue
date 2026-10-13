<template>
  <div class="toast-container" aria-live="polite">
    <TransitionGroup name="toast">
      <div
        v-for="t in toasts"
        :key="t.id"
        class="toast"
        :class="t.type"
        @mouseenter="pause(t.id)"
        @mouseleave="resume(t.id)"
      >
        <span class="toast-icon"><n-icon :component="iconOf(t.type)" :size="16" /></span>
        <div class="toast-text">
          <div class="toast-title">{{ t.title }}</div>
          <div v-if="t.msg" class="toast-msg selectable">{{ t.msg }}</div>
        </div>
        <button class="toast-close" aria-label="关闭" @click="remove(t.id)">
          <n-icon :component="CloseOutline" :size="14" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount } from 'vue'
import { NIcon } from 'naive-ui'
import {
  CheckmarkCircleOutline,
  InformationCircleOutline,
  AlertCircleOutline,
  CloseCircleOutline,
  CloseOutline,
} from '@vicons/ionicons5'
import type { Component } from 'vue'
import { useToast, pushToast, type ToastType } from '../composables/useToast'

const { toasts, remove, pause, resume } = useToast()

function iconOf(type: ToastType): Component {
  switch (type) {
    case 'success':
      return CheckmarkCircleOutline
    case 'warning':
      return AlertCircleOutline
    case 'error':
      return CloseCircleOutline
    default:
      return InformationCircleOutline
  }
}

// 兼容全局桥：各页面沿用 window.__toast(type, title, msg) 调用
function showToast(type: string, title: string, msg?: string) {
  const t = (['info', 'success', 'warning', 'error'] as const).includes(type as ToastType)
    ? (type as ToastType)
    : 'info'
  pushToast(t, title, msg || '')
}
let bridgeInstalled = false
onMounted(() => {
  ;(window as unknown as Record<string, unknown>).__toast = showToast
  bridgeInstalled = true
})
onBeforeUnmount(() => {
  if (bridgeInstalled && (window as unknown as Record<string, unknown>).__toast === showToast) {
    delete (window as unknown as Record<string, unknown>).__toast
  }
})
</script>

<style scoped>
.toast-container {
  position: fixed;
  left: 20px;
  bottom: 20px;
  z-index: 3000;
  display: flex;
  flex-direction: column-reverse;
  align-items: flex-start;
  gap: 10px;
  pointer-events: none;
}
.toast {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-width: 220px;
  max-width: 380px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--fluent-card-bg);
  border: 1px solid var(--fluent-border);
  box-shadow: var(--fluent-shadow-lg);
  pointer-events: auto;
  transition: background-color var(--dur-standard, 0.2s) var(--ease-standard),
    border-color var(--dur-standard, 0.2s) var(--ease-standard), box-shadow 0.15s;
}
.toast:hover { box-shadow: 0 10px 30px rgba(0, 0, 0, 0.18); }
.toast-icon { flex-shrink: 0; display: flex; align-items: center; padding-top: 1px; }
.toast.success .toast-icon { color: var(--ok); }
.toast.info .toast-icon { color: var(--accent, #0078d4); }
.toast.warning .toast-icon { color: var(--warn); }
.toast.error .toast-icon { color: var(--err); }
.toast-text { flex: 1; min-width: 0; }
.toast-title { font-weight: 600; font-size: 13px; line-height: 1.4; word-break: break-word; }
.toast-msg { font-size: 12px; color: var(--fluent-text-soft); white-space: pre-wrap; word-break: break-word; margin-top: 3px; line-height: 1.5; }
.toast-close {
  flex-shrink: 0; border: none; background: transparent; cursor: pointer;
  padding: 2px; border-radius: 4px; color: var(--fluent-text-soft);
  display: flex; align-items: center; justify-content: center;
  transition: background 0.15s, color 0.15s;
}
.toast-close:hover { background: var(--fluent-border); color: var(--fluent-text); }

/* 从左滑入（--dur-toast），原位淡出（脱离文档流不抽动），其余 FLIP 补位 */
.toast-enter-active { transition: opacity var(--dur-toast, 0.3s) var(--ease-standard), transform var(--dur-toast, 0.3s) var(--ease-standard); }
.toast-enter-from { opacity: 0; transform: translateX(-120%); }
.toast-leave-active { transition: opacity var(--dur-toast, 0.3s) var(--ease-standard); position: absolute; }
.toast-leave-to { opacity: 0; }
.toast-move { transition: transform var(--dur-toast, 0.3s) var(--ease-standard); }
</style>
