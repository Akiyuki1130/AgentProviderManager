<template>
  <n-modal v-model:show="visible" preset="card" title="从钥匙串选择" style="width: 480px" :mask-closable="false">
    <div class="kp-body">
      <n-input v-model:value="query" size="small" clearable placeholder="搜索名称 / 备注..." />
      <!-- D10：加载中/失败与空态分离（错误 ≠ 空态），拉取失败显示错误行 +「重试」，不再伪装「暂无条目」 -->
      <div v-if="loading" class="kp-list kp-state">
        <n-spin size="medium" />
      </div>
      <div v-else-if="loadError" class="kp-list kp-state">
        <div class="kp-error-line">
          <n-icon :component="WarningOutline" :size="16" class="kp-error-icon" />
          <span class="kp-error-text">{{ loadError }}</span>
          <n-button size="small" @click="load">
            <template #icon><n-icon :component="RefreshOutline" /></template>
            重试
          </n-button>
        </div>
      </div>
      <TransitionGroup v-else name="list" tag="div" class="kp-list">
        <div v-if="filtered.length === 0" key="empty" class="empty-state">
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
      </TransitionGroup>
    </div>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { NModal, NInput, NSpin, NButton, NIcon } from 'naive-ui'
import { WarningOutline, RefreshOutline } from '@vicons/ionicons5'
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
// D10：加载态与错误态（错误 ≠ 空态）
const loading = ref(false)
const loadError = ref('')

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return entries.value
  return entries.value.filter((e) => e.name.toLowerCase().includes(q) || (e.note || '').toLowerCase().includes(q))
})

// D10：失败时不再清空列表伪装「暂无条目」，而是记录错误并展示错误行 +「重试」
async function load() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await api.ListKeychain() as Record<string, unknown>
    if (res['success']) {
      entries.value = (res['entries'] as KeychainEntry[]) || []
    } else {
      entries.value = []
      loadError.value = (res['error'] as string) || '后端返回未知错误'
    }
  } catch (e) {
    entries.value = []
    loadError.value = String(e)
  } finally { loading.value = false }
}

watch(() => props.show, (v) => { if (v) { query.value = ''; void load() } })

function onPick(e: KeychainEntry) {
  emit('pick', e)
  emit('update:show', false)
}
</script>

<style scoped>
.kp-body { display: flex; flex-direction: column; gap: 8px; }
/* position:relative 是全局 .list-leave-active{position:absolute} 的脱流锚点（A4 FLIP） */
.kp-list { position: relative; display: flex; flex-direction: column; gap: 8px; max-height: 340px; overflow-y: auto; min-height: 120px; }
.kp-row { display: flex; gap: 8px; align-items: center; padding: 10px 12px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; cursor: pointer; }
.kp-row:hover { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
.kp-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.kp-name { display: flex; gap: 6px; align-items: center; min-width: 0; }
.kp-name-text { font-size: 13px; font-weight: 600; word-break: break-all; }
.kp-note { font-size: 11px; color: var(--fluent-text-soft); word-break: break-all; }
/* key_hint 徽标不再本地重定义：改用全局控件规范层 .badge（B10 单一定义源） */
.empty-state { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px; padding: 32px; color: var(--fluent-text-soft); text-align: center; }
/* D10：加载/错误占位（复用 .kp-list 的 min-height 与滚动区，不毁布局） */
.kp-state { display: flex; align-items: center; justify-content: center; }
.kp-error-line { display: flex; gap: 8px; align-items: center; font-size: 12px; color: var(--err); background: var(--err-soft); border: 1px solid color-mix(in srgb, var(--err) 40%, transparent); border-radius: 8px; padding: 8px 12px; }
.kp-error-icon { color: var(--err); flex-shrink: 0; }
.kp-error-text { word-break: break-all; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
</style>
