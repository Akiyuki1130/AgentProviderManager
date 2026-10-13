<template>
  <Transition name="bar-slide">
    <div v-if="visible" class="sticky-save-bar">
      <span class="sticky-save-bar-hint">{{ hintText }}</span>
      <div class="sticky-save-bar-actions">
        <n-button :disabled="saving" @click="emit('reset')">{{ resetText }}</n-button>
        <n-button type="primary" :loading="saving" :disabled="!canSave || saving" @click="emit('save')">
          {{ saveText }}
        </n-button>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { NButton } from 'naive-ui'

withDefaults(
  defineProps<{
    visible: boolean
    saving?: boolean
    canSave?: boolean
    hintText?: string
    saveText?: string
    resetText?: string
  }>(),
  {
    saving: false,
    canSave: true,
    hintText: '未保存的更改',
    saveText: '保存',
    resetText: '放弃更改',
  },
)

const emit = defineEmits<{
  save: []
  reset: []
}>()
</script>

<style scoped>
.sticky-save-bar {
  position: sticky;
  bottom: 0;
  z-index: 10;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
  padding: 12px 20px;
  margin-top: 12px;
  background: var(--fluent-card-bg);
  border: 1px solid var(--fluent-border);
  /* 容器档位 12px，与全局 .fluent-card 一致 */
  border-radius: 12px;
  box-shadow: var(--fluent-shadow-md);
  transition: background-color var(--dur-standard) var(--ease-standard),
    border-color var(--dur-standard) var(--ease-standard),
    box-shadow var(--dur-standard) var(--ease-standard);
}
.sticky-save-bar-hint { flex: 1; opacity: 0.7; font-size: 13px; }
.sticky-save-bar-actions { display: flex; align-items: center; gap: 8px; }

.bar-slide-enter-active,
.bar-slide-leave-active {
  transition: opacity var(--dur-standard, 0.2s) ease, transform var(--dur-standard, 0.2s) ease;
}
/* 位移基准 8px，与 fade-slide 一致 */
.bar-slide-enter-from,
.bar-slide-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
