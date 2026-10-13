<template>
  <n-modal
    :show="confirmState !== null"
    preset="card"
    :title="confirmState?.title || '请确认'"
    style="width: 420px"
    :mask-closable="false"
    @update:show="(v: boolean) => { if (!v) settleConfirm(false) }"
  >
    <div class="ud-content">{{ confirmState?.content }}</div>
    <div v-if="confirmState?.detail" class="ud-detail selectable">{{ confirmState.detail }}</div>
    <template #footer>
      <div class="ud-actions">
        <n-button @click="settleConfirm(false)">{{ confirmState?.negativeText || '取消' }}</n-button>
        <n-button :type="confirmState?.danger ? 'error' : 'primary'" @click="settleConfirm(true)">
          {{ confirmState?.positiveText || '确定' }}
        </n-button>
      </div>
    </template>
  </n-modal>

  <n-modal
    :show="promptState !== null"
    preset="card"
    :title="promptState?.title"
    style="width: 480px"
    :mask-closable="false"
    @update:show="(v: boolean) => { if (!v) settlePrompt(null) }"
  >
    <label v-if="promptState?.label" class="ud-label">{{ promptState.label }}</label>
    <n-input
      ref="inputRef"
      v-model:value="promptValue"
      :placeholder="promptState?.placeholder"
      @keyup.enter="submit"
    />
    <div v-if="promptError" class="ud-error">{{ promptError }}</div>
    <template #footer>
      <div class="ud-actions">
        <n-button @click="settlePrompt(null)">取消</n-button>
        <n-button type="primary" @click="submit">{{ promptState?.confirmText || '确定' }}</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { NModal, NButton, NInput } from 'naive-ui'
import type { InputInst } from 'naive-ui'
import {
  confirmState, promptState, settleConfirm, settlePrompt,
} from '../../stores/uiDialog'

const inputRef = ref<InputInst | null>(null)
const promptValue = ref('')
const promptError = ref<string | null>(null)

watch(promptState, async (opts) => {
  if (!opts) return
  promptValue.value = opts.initialValue ?? ''
  promptError.value = null
  await nextTick()
  inputRef.value?.focus()
})

function submit() {
  const opts = promptState.value
  if (!opts) return
  const value = promptValue.value.trim()
  const err = opts.validate ? opts.validate(value) : null
  if (err) {
    promptError.value = err
    return
  }
  settlePrompt(value === '' ? null : value)
}
</script>

<style scoped>
.ud-content { font-size: 13px; line-height: 1.7; }
.ud-detail {
  margin-top: 8px; font-size: 12px; color: var(--fluent-text-soft);
  word-break: break-all; font-family: Consolas, 'Cascadia Code', monospace;
}
.ud-label { display: block; font-size: 12px; color: var(--fluent-text-soft); margin-bottom: 6px; }
.ud-error { margin-top: 6px; font-size: 12px; color: var(--err); }
.ud-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
