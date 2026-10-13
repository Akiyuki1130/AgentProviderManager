<template>
  <div class="migrate-page">
    <!-- C5 统一页头：原卡内标题/副标题移入 PageHeader，不再重复渲染。
         刷新预览/交换紧贴 Agent 选择器、开始迁移随底部摘要成组，均非可独立上移的页级主操作，故 header 不带 actions -->
    <PageHeader class="enter-stagger" :style="{ animationDelay: staggerMs(0) + 'ms' }" :title="t('migrate.title', lang)" :subtitle="t('migrate.subtitle', lang)">
      <template #icon><n-icon :component="ArrowForwardOutline" :size="20" /></template>
    </PageHeader>

    <div class="fluent-card card-pad enter-stagger" :style="{ animationDelay: staggerMs(1) + 'ms' }">
      <div class="migrate-controls">
        <div class="ctrl-group">
          <label class="form-label">{{ t('migrate.sourceAgent', lang) }}</label>
          <n-select v-model:value="sourceAgent" :options="agentOptions" style="width: 200px" />
        </div>
        <div class="ctrl-arrow"><n-icon :component="ArrowForwardOutline" /></div>
        <div class="ctrl-group">
          <label class="form-label">{{ t('migrate.targetAgent', lang) }}</label>
          <n-select v-model:value="targetAgent" :options="targetOptions" style="width: 200px" />
        </div>
        <n-button size="small" type="primary" :loading="previewing" @click="doPreview">{{ t('migrate.refreshPreview', lang) }}</n-button>
        <n-button size="small" @click="swapAgents">{{ t('migrate.swap', lang) }}</n-button>
      </div>
      <div class="migrate-mode">
        <label class="mode-item">
          <input type="radio" value="merge" v-model="mode" />
          <span><b>{{ t('migrate.modeMerge', lang) }}</b> — {{ t('migrate.modeMergeDesc', lang) }}</span>
        </label>
        <label class="mode-item">
          <input type="radio" value="overwrite" v-model="mode" />
          <span><b>{{ t('migrate.modeOverwrite', lang) }}</b> — {{ t('migrate.modeOverwriteDesc', lang) }}</span>
        </label>
      </div>
      <div v-if="previewError" class="migrate-error">{{ previewError }}</div>
    </div>

    <div class="migrate-split enter-stagger" :style="{ animationDelay: staggerMs(2) + 'ms' }">
      <div class="migrate-col fluent-card card-pad">
        <div class="col-head">
          <h3>{{ t('migrate.source', lang) }} — {{ sourceLabel }}</h3>
          <span class="path-text mono selectable" :title="sourceSide.path">{{ shortPath(sourceSide.path) }}</span>
        </div>
        <div class="col-meta">
          <span class="badge" :class="sourceSide.exists ? 'badge-ok' : 'badge-warn'">{{ sourceSide.exists ? t('migrate.fileExists', lang) : t('migrate.fileMissing', lang) }}</span>
          <span v-if="sourceSide.error" class="meta-err">{{ sourceSide.error }}</span>
          <span v-else class="meta-count tnum">{{ t('migrate.providerCount', lang).replace('{n}', String(sourceProviders.length)) }}</span>
        </div>
        <div class="select-row">
          <label class="batch-item">{{ t('migrate.selectAll', lang) }} <button class="switch" :class="{ on: allSourceSelected }" @click="toggleAllSource" /></label>
          <n-button size="small" @click="selectAllSource(false)">{{ t('migrate.selectNone', lang) }}</n-button>
          <span class="count-label tnum" style="margin-left:auto">{{ t('migrate.selectedCount', lang).replace('{n}', String(selectedIds.length)) }}</span>
        </div>
        <div class="provider-check-list">
          <div v-if="sourceProviders.length === 0" class="empty-state">
            <div class="empty-title">{{ t('migrate.emptySourceTitle', lang) }}</div>
            <div class="empty-sub">{{ t('migrate.emptySourceSub', lang) }}</div>
            <!-- B11 空态可操作化：直接触发既有 doPreview 重新读取两侧配置 -->
            <n-button size="small" :loading="previewing" @click="doPreview"><template #icon><n-icon :component="SyncOutline" /></template>{{ t('migrate.refreshPreview', lang) }}</n-button>
          </div>
          <label v-for="p in sourceProviders" :key="p.id" class="p-check" :class="{ checked: selectedSet.has(p.id) }">
            <input type="checkbox" :value="p.id" :checked="selectedSet.has(p.id)" @change="toggleOne(p.id)" />
            <span class="p-name">{{ p.name }}</span>
            <span class="p-id mono">{{ p.id }}</span>
            <span class="p-sub tnum">{{ p.kind }} · {{ t('migrate.modelCount', lang).replace('{n}', String(p.model_count)) }} · {{ p.has_api_key ? t('migrate.hasKey', lang) : t('migrate.noKey', lang) }}</span>
          </label>
        </div>
        <div class="raw-block">
          <div class="raw-title">{{ t('migrate.sourcePreview', lang) }} <span class="badge">{{ sourceSide.format }}</span></div>
          <pre class="raw-pre">{{ sourceSide.raw_text || t('migrate.emptyRaw', lang) }}</pre>
        </div>
      </div>

      <div class="migrate-col fluent-card card-pad">
        <div class="col-head">
          <h3>{{ t('migrate.target', lang) }} — {{ targetLabel }}</h3>
          <span class="path-text mono selectable" :title="targetSide.path">{{ shortPath(targetSide.path) }}</span>
        </div>
        <div class="col-meta">
          <span class="badge" :class="targetSide.exists ? 'badge-ok' : 'badge-warn'">{{ targetSide.exists ? t('migrate.fileExists', lang) : t('migrate.fileMissing', lang) }}</span>
          <span v-if="targetSide.error" class="meta-err">{{ targetSide.error }}</span>
          <span v-else class="meta-count tnum">{{ t('migrate.providerCount', lang).replace('{n}', String(targetProviders.length)) }}</span>
        </div>
        <div class="provider-read-list">
          <div v-if="targetProviders.length === 0" class="empty-state">
            <div class="empty-title">{{ t('migrate.emptyTargetTitle', lang) }}</div>
            <div class="empty-sub">{{ t('migrate.emptyTargetSub', lang) }}</div>
          </div>
          <div v-for="p in targetProviders" :key="p.id" class="p-read" :class="{ willOverwrite: isOverlap(p.id) }">
            <span class="p-name">{{ p.name }}</span>
            <span class="p-id mono">{{ p.id }}</span>
            <span class="p-sub tnum">{{ p.kind }} · {{ t('migrate.modelCount', lang).replace('{n}', String(p.model_count)) }}</span>
            <span v-if="isOverlap(p.id)" class="overlap-tag">{{ mode === 'overwrite' ? t('migrate.willOverwrite', lang) : t('migrate.willMerge', lang) }}</span>
          </div>
        </div>
        <div class="raw-block">
          <div class="raw-title">{{ t('migrate.targetPreview', lang) }} <span class="badge">{{ targetSide.format }}</span></div>
          <pre class="raw-pre">{{ targetSide.raw_text || t('migrate.emptyRaw', lang) }}</pre>
        </div>
      </div>
    </div>

    <div class="fluent-card card-pad migrate-footer enter-stagger" :style="{ animationDelay: staggerMs(3) + 'ms' }">
      <div class="footer-hint">{{ footerHint }}</div>
      <div class="footer-actions">
        <n-button type="primary" :disabled="selectedIds.length === 0 || sourceAgent === targetAgent" :loading="migrating" @click="doMigrate">{{ t('migrate.start', lang) }}</n-button>
        <n-button size="small" @click="doPreview">{{ t('migrate.refresh', lang) }}</n-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { NButton, NSelect, NIcon } from 'naive-ui'
import { ArrowForwardOutline, SyncOutline } from '@vicons/ionicons5'
import { uiConfirm } from '../stores/uiDialog'
import { useSettingStore } from '../stores/setting'
import { t } from '../i18n'
import PageHeader from '../components/ui/PageHeader.vue'
import { staggerMs } from '../composables/useStagger'
import * as api from '../api'

type ProviderSummary = { id: string; name: string; kind: string; base_url: string; has_api_key: boolean; model_count: number }
type Side = { agent: string; label: string; path: string; exists: boolean; error: string; providers: ProviderSummary[]; raw_text: string; format: string }

const settingStore = useSettingStore()
const lang = computed(() => settingStore.resolvedLang)

const agentOptions = [
  { label: 'ZCode', value: 'zcode' },
  { label: 'DeepSeek Harness', value: 'deepseek' },
]

const sourceAgent = ref('zcode')
const targetAgent = ref('deepseek')
const mode = ref<'merge' | 'overwrite'>('merge')
const previewing = ref(false)
const migrating = ref(false)
const previewError = ref('')
const sourceSide = ref<Side>({ agent: 'zcode', label: 'ZCode', path: '', exists: false, error: '', providers: [], raw_text: '', format: 'json' })
const targetSide = ref<Side>({ agent: 'deepseek', label: 'DeepSeek Harness', path: '', exists: false, error: '', providers: [], raw_text: '', format: 'json' })
const selectedIds = ref<string[]>([])

const sourceProviders = computed(() => sourceSide.value.providers || [])
const targetProviders = computed(() => targetSide.value.providers || [])
const targetOptions = computed(() => agentOptions)
const sourceLabel = computed(() => labelOf(sourceAgent.value))
const targetLabel = computed(() => labelOf(targetAgent.value))
const selectedSet = computed(() => new Set(selectedIds.value))
const allSourceSelected = computed(() => sourceProviders.value.length > 0 && sourceProviders.value.every((p) => selectedSet.value.has(p.id)))
const footerHint = computed(() => t('migrate.footerHint', lang.value)
  .replace('{source}', sourceLabel.value)
  .replace('{count}', String(selectedIds.value.length))
  .replace('{target}', targetLabel.value)
  .replace('{mode}', mode.value === 'overwrite' ? t('migrate.modeOverwrite', lang.value) : t('migrate.modeMerge', lang.value)))

function labelOf(id: string) {
  const found = agentOptions.find((o) => o.value === id)
  return found ? found.label : id
}
function shortPath(p: string) {
  if (!p) return ''
  if (p.length > 60) return '...' + p.slice(-57)
  return p
}
function isOverlap(id: string) {
  return selectedSet.value.has(id)
}
function toggleOne(id: string) {
  const s = new Set(selectedIds.value)
  if (s.has(id)) s.delete(id)
  else s.add(id)
  selectedIds.value = Array.from(s)
}
function toggleAllSource() {
  if (allSourceSelected.value) selectAllSource(false)
  else selectAllSource(true)
}
function selectAllSource(v: boolean) {
  if (v) selectedIds.value = sourceProviders.value.map((p) => p.id)
  else selectedIds.value = []
}
function swapAgents() {
  const tmp = sourceAgent.value
  sourceAgent.value = targetAgent.value
  targetAgent.value = tmp
}
function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

async function doPreview() {
  previewing.value = true
  previewError.value = ''
  try {
    if (sourceAgent.value === targetAgent.value) {
      previewError.value = t('migrate.sameAgents', lang.value)
      return
    }
    const res = await (api as unknown as Record<string, (...a: unknown[]) => Promise<unknown>>).MigratePreview(sourceAgent.value, targetAgent.value) as Record<string, unknown>
    if (!res['success']) {
      previewError.value = (res['error'] as string) || t('migrate.previewFailed', lang.value)
      return
    }
    const preview = res['preview'] as { source: Side; target: Side }
    sourceSide.value = preview.source || sourceSide.value
    targetSide.value = preview.target || targetSide.value
    // keep selection only for existing ids
    const valid = new Set(sourceSide.value.providers.map((p) => p.id))
    selectedIds.value = selectedIds.value.filter((id) => valid.has(id))
  } catch (e) {
    previewError.value = String(e)
  } finally {
    previewing.value = false
  }
}

async function doMigrate() {
  if (selectedIds.value.length === 0) {
    toast('error', t('migrate.failed', lang.value), t('migrate.noneSelected', lang.value))
    return
  }
  const overwrite = mode.value === 'overwrite'
  const confirmed = await uiConfirm({
    title: t('migrate.confirmTitle', lang.value),
    content: t(overwrite ? 'migrate.confirmOverwriteBody' : 'migrate.confirmMergeBody', lang.value)
      .replace('{n}', String(selectedIds.value.length))
      .replace('{target}', targetLabel.value),
    detail: t(overwrite ? 'migrate.confirmOverwriteDetail' : 'migrate.confirmMergeDetail', lang.value),
    // 覆盖模式会清空目标配置，按破坏性操作走 danger 确认
    danger: overwrite,
  })
  if (!confirmed) return
  migrating.value = true
  try {
    const res = await (api as unknown as Record<string, (...a: unknown[]) => Promise<unknown>>).MigrateExecute({
      source: sourceAgent.value,
      target: targetAgent.value,
      selected_ids: selectedIds.value,
      mode: mode.value,
    }) as Record<string, unknown>
    if (!res['success']) {
      toast('error', t('migrate.failed', lang.value), res['error'] as string)
      return
    }
    toast('success', t('migrate.successTitle', lang.value), t('migrate.successBody', lang.value)
      .replace('{n}', String(res['migrated']))
      .replace('{target}', targetLabel.value)
      .replace('{mode}', t(mode.value === 'overwrite' ? 'migrate.modeOverwrite' : 'migrate.modeMerge', lang.value)))
    await doPreview()
    window.dispatchEvent(new CustomEvent('agent-changed', { detail: targetAgent.value }))
  } catch (e) {
    toast('error', t('migrate.failed', lang.value), String(e))
  } finally {
    migrating.value = false
  }
}

watch([sourceAgent, targetAgent], () => { void doPreview() })

onMounted(() => { void doPreview() })
</script>

<style scoped>
.migrate-page { display: flex; flex-direction: column; gap: 12px; }
/* .fluent-card/.badge/.switch 均用 global.css 控件规范层单一定义（B10 去重，dark 轨道随全局修复） */
.card-pad { padding: 16px; }
.migrate-controls { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-top: 12px; }
.ctrl-group { display: flex; flex-direction: column; gap: 6px; }
.ctrl-arrow { font-size: 18px; padding-bottom: 6px; color: var(--fluent-text-soft); }
.form-label { font-size: 12px; color: var(--fluent-text-soft); }
.migrate-mode { display: flex; gap: 16px; flex-wrap: wrap; margin-top: 12px; }
.mode-item { display: flex; gap: 6px; align-items: center; font-size: 12px; cursor: pointer; }
.migrate-error { margin-top: 8px; font-size: 12px; color: var(--err); background: var(--err-soft); border: 1px solid color-mix(in srgb, var(--err) 25%, transparent); border-radius: 8px; padding: 8px 12px; }
.migrate-split { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
@media (max-width: 1100px) { .migrate-split { grid-template-columns: 1fr; } }
.migrate-col { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.col-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.col-head h3 { font-size: 13px; font-weight: 600; }
.path-text { font-size: 10px; color: var(--fluent-text-soft); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 280px; }
.col-meta { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.meta-err { font-size: 11px; color: var(--err); }
.meta-count { font-size: 11px; color: var(--fluent-text-soft); }
.select-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; padding: 6px 8px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; }
.batch-item { display: inline-flex; gap: 4px; align-items: center; font-size: 11px; }
.count-label { font-size: 11px; color: var(--fluent-text-soft); }
.provider-check-list, .provider-read-list { display: flex; flex-direction: column; gap: 6px; max-height: 300px; overflow-y: auto; min-height: 80px; }
.p-check { display: grid; grid-template-columns: auto 1fr; gap: 2px 8px; padding: 8px 10px; border: 1px solid var(--fluent-border); border-radius: 8px; cursor: pointer; background: var(--fluent-bg); align-items: center; }
.p-check.checked { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
.p-check input { grid-row: 1 / 3; }
.p-name { font-size: 12px; font-weight: 600; }
.p-id { font-size: 10px; color: var(--fluent-text-soft); }
.p-sub { font-size: 10px; color: var(--fluent-text-soft); grid-column: 2; }
.p-read { display: flex; flex-direction: column; gap: 2px; padding: 8px 10px; border: 1px solid var(--fluent-border); border-radius: 8px; background: var(--fluent-bg); }
.p-read.willOverwrite { border-color: var(--err); background: var(--err-soft); }
.overlap-tag { font-size: 10px; color: var(--err); font-weight: 600; }
.raw-block { display: flex; flex-direction: column; gap: 6px; }
.raw-title { font-size: 12px; font-weight: 600; display: flex; gap: 8px; align-items: center; }
.raw-pre { font-size: 11px; line-height: 1.5; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 10px; max-height: 360px; overflow: auto; white-space: pre-wrap; word-break: break-all; }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 16px; color: var(--fluent-text-soft); text-align: center; border: 1px dashed var(--fluent-border); border-radius: 8px; }
.empty-title { font-size: 12px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 11px; }
.migrate-footer { display: flex; gap: 12px; align-items: center; justify-content: space-between; flex-wrap: wrap; }
.footer-hint { font-size: 12px; color: var(--fluent-text-soft); }
.footer-actions { display: flex; gap: 8px; }
</style>
