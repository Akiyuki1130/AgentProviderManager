<template>
  <n-modal v-model:show="visible" preset="card" title="🔑 从钥匙串选择" style="width: 480px" :mask-closable="false">
    <div class="kp-body">
      <n-input v-model:value="query" size="small" clearable placeholder="搜索名称 / 备注..." />
      <div class="kp-list">
        <div v-if="filtered.length === 0" class="empty-state">
          <div class="empty-title">暂无 API Key</div>
          <div class="empty-sub">先在「API Key 钥匙串」页面添加项目，再回来选择。</div>
        </div>
        <div v-for="e in filtered" :key="e.id" class="kp-row" @click="onPick(e)">
          <div class="kp-main">
            <div class="kp-name">
              <span class="kp-name-text">{{ e.name }}</span>
              <span class="badge">{{ e.key_hint }}</span>
            </div>
            <div v-if="e.note" class="kp-note">{{ e.note }}</div>
          </div>
        </div>
      </div>
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { NModal, NInput } from 'naive-ui'
import type { KeychainEntry } from '../types'
import * as api from '../api'

const props = defineProps<{ show: boolean }>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
  (e: 'pick', entry: KeychainEntry): void
}>()

const visible = computed({
  get: () => props.show,
  set: (v: boolean) => emit('update:show', v),
})

const entries = ref<KeychainEntry[]>([])
const query = ref('')

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return entries.value
  return entries.value.filter((e) => e.name.toLowerCase().includes(q) || (e.note || '').toLowerCase().includes(q))
})

async function load() {
  try {
    const res = await api.ListKeychain() as Record<string, unknown>
    if (res['success']) entries.value = (res['entries'] as KeychainEntry[]) || []
    else entries.value = []
  } catch { entries.value = [] }
}

watch(() => props.show, (v) => { if (v) { query.value = ''; void load() } })

function onPick(e: KeychainEntry) {
  emit('pick', e)
  emit('update:show', false)
}
</script>

<style scoped>
.kp-body { display: flex; flex-direction: column; gap: 8px; }
.kp-list { display: flex; flex-direction: column; gap: 8px; max-height: 340px; overflow-y: auto; min-height: 120px; }
.kp-row { display: flex; gap: 8px; align-items: center; padding: 10px 12px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; cursor: pointer; }
.kp-row:hover { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
.kp-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.kp-name { display: flex; gap: 6px; align-items: center; min-width: 0; }
.kp-name-text { font-size: 13px; font-weight: 600; word-break: break-all; }
.kp-note { font-size: 11px; color: var(--fluent-text-soft); word-break: break-all; }
.badge { font-size: 10px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 0 6px; line-height: 16px; white-space: nowrap; background: var(--fluent-card-bg); }
.empty-state { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; padding: 32px; color: var(--fluent-text-soft); text-align: center; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
</style>
