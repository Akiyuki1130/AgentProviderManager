<template>
  <div class="migrate-page">
    <div class="fluent-card card-pad">
      <div class="card-title">配置文件迁移</div>
      <div class="migrate-subtitle">将一个 Agent 的提供商配置迁移到另一个 Agent，支持覆盖或在目标基础上追加/合并。</div>
      <div class="migrate-controls">
        <div class="ctrl-group">
          <label class="form-label">源 Agent</label>
          <n-select v-model:value="sourceAgent" :options="agentOptions" style="width: 200px" />
        </div>
        <div class="ctrl-arrow">→</div>
        <div class="ctrl-group">
          <label class="form-label">目标 Agent</label>
          <n-select v-model:value="targetAgent" :options="targetOptions" style="width: 200px" />
        </div>
        <n-button size="small" type="primary" :loading="previewing" @click="doPreview">刷新预览</n-button>
        <n-button size="small" @click="swapAgents">交换</n-button>
      </div>
      <div class="migrate-mode">
        <label class="mode-item">
          <input type="radio" value="merge" v-model="mode" />
          <span><b>追加/合并</b> — 在目标配置基础上新增或合并同名提供商的模型</span>
        </label>
        <label class="mode-item">
          <input type="radio" value="overwrite" v-model="mode" />
          <span><b>覆盖</b> — 清空目标的提供商列表，仅保留迁移的提供商</span>
        </label>
      </div>
      <div v-if="previewError" class="migrate-error">{{ previewError }}</div>
    </div>

    <div class="migrate-split">
      <div class="migrate-col fluent-card card-pad">
        <div class="col-head">
          <h3>源 — {{ sourceLabel }}</h3>
          <span class="path-text" :title="sourceSide.path">{{ shortPath(sourceSide.path) }}</span>
        </div>
        <div class="col-meta">
          <span class="badge" :class="sourceSide.exists ? 'badge-ok' : 'badge-warn'">{{ sourceSide.exists ? '文件存在' : '文件不存在' }}</span>
          <span v-if="sourceSide.error" class="meta-err">{{ sourceSide.error }}</span>
          <span v-else class="meta-count">{{ sourceProviders.length }} 个提供商</span>
        </div>
        <div class="select-row">
          <label class="batch-item">全选 <button class="switch" :class="{ on: allSourceSelected }" @click="toggleAllSource" /></label>
          <n-button size="tiny" @click="selectAllSource(false)">全不选</n-button>
          <span class="count-label" style="margin-left:auto">{{ selectedIds.length }} 已选</span>
        </div>
        <div class="provider-check-list">
          <div v-if="sourceProviders.length === 0" class="empty-state">
            <div class="empty-title">无可迁移的提供商</div>
            <div class="empty-sub">该 Agent 配置为空或文件不存在。</div>
          </div>
          <label v-for="p in sourceProviders" :key="p.id" class="p-check" :class="{ checked: selectedSet.has(p.id) }">
            <input type="checkbox" :value="p.id" :checked="selectedSet.has(p.id)" @change="toggleOne(p.id)" />
            <span class="p-name">{{ p.name }}</span>
            <span class="p-id">{{ p.id }}</span>
            <span class="p-sub">{{ p.kind }} · {{ p.model_count }} 模型 · {{ p.has_api_key ? '有Key' : '无Key' }}</span>
          </label>
        </div>
        <div class="raw-block">
          <div class="raw-title">源配置预览 <span class="raw-fmt">{{ sourceSide.format }}</span></div>
          <pre class="raw-pre">{{ sourceSide.raw_text || '(空)' }}</pre>
        </div>
      </div>

      <div class="migrate-col fluent-card card-pad">
        <div class="col-head">
          <h3>目标 — {{ targetLabel }}</h3>
          <span class="path-text" :title="targetSide.path">{{ shortPath(targetSide.path) }}</span>
        </div>
        <div class="col-meta">
          <span class="badge" :class="targetSide.exists ? 'badge-ok' : 'badge-warn'">{{ targetSide.exists ? '文件存在' : '文件不存在' }}</span>
          <span v-if="targetSide.error" class="meta-err">{{ targetSide.error }}</span>
          <span v-else class="meta-count">{{ targetProviders.length }} 个提供商</span>
        </div>
        <div class="provider-read-list">
          <div v-if="targetProviders.length === 0" class="empty-state">
            <div class="empty-title">目标暂无提供商</div>
            <div class="empty-sub">迁移后将创建新的提供商列表。</div>
          </div>
          <div v-for="p in targetProviders" :key="p.id" class="p-read" :class="{ willOverwrite: isOverlap(p.id) }">
            <span class="p-name">{{ p.name }}</span>
            <span class="p-id">{{ p.id }}</span>
            <span class="p-sub">{{ p.kind }} · {{ p.model_count }} 模型</span>
            <span v-if="isOverlap(p.id)" class="overlap-tag">{{ mode === 'overwrite' ? '将被覆盖/删除' : '将被合并' }}</span>
          </div>
        </div>
        <div class="raw-block">
          <div class="raw-title">目标配置预览 <span class="raw-fmt">{{ targetSide.format }}</span></div>
          <pre class="raw-pre">{{ targetSide.raw_text || '(空)' }}</pre>
        </div>
      </div>
    </div>

    <div class="fluent-card card-pad migrate-footer">
      <div class="footer-hint">
        将从 <b>{{ sourceLabel }}</b> 迁移 <b>{{ selectedIds.length }}</b> 个提供商到 <b>{{ targetLabel }}</b>，模式：<b>{{ mode === 'overwrite' ? '覆盖' : '追加/合并' }}</b>。
      </div>
      <div class="footer-actions">
        <n-button type="primary" :disabled="selectedIds.length === 0 || sourceAgent === targetAgent" :loading="migrating" @click="doMigrate">开始迁移</n-button>
        <n-button @click="doPreview">刷新</n-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { NButton, NSelect } from 'naive-ui'
import * as api from '../api'

type ProviderSummary = { id: string; name: string; kind: string; base_url: string; has_api_key: boolean; model_count: number }
type Side = { agent: string; label: string; path: string; exists: boolean; error: string; providers: ProviderSummary[]; raw_text: string; format: string }

const agentOptions = [
  { label: 'ZCode', value: 'zcode' },
  { label: 'OpenCode', value: 'opencode' },
  { label: 'DeepSeek Harness', value: 'deepseek' },
]

const sourceAgent = ref('zcode')
const targetAgent = ref('opencode')
const mode = ref<'merge' | 'overwrite'>('merge')
const previewing = ref(false)
const migrating = ref(false)
const previewError = ref('')
const sourceSide = ref<Side>({ agent: 'zcode', label: 'ZCode', path: '', exists: false, error: '', providers: [], raw_text: '', format: 'json' })
const targetSide = ref<Side>({ agent: 'opencode', label: 'OpenCode', path: '', exists: false, error: '', providers: [], raw_text: '', format: 'json' })
const selectedIds = ref<string[]>([])

const sourceProviders = computed(() => sourceSide.value.providers || [])
const targetProviders = computed(() => targetSide.value.providers || [])
const targetOptions = computed(() => agentOptions)
const sourceLabel = computed(() => labelOf(sourceAgent.value))
const targetLabel = computed(() => labelOf(targetAgent.value))
const selectedSet = computed(() => new Set(selectedIds.value))
const allSourceSelected = computed(() => sourceProviders.value.length > 0 && sourceProviders.value.every((p) => selectedSet.value.has(p.id)))

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
      previewError.value = '源和目标不能相同'
      return
    }
    const res = await (api as unknown as Record<string, (...a: unknown[]) => Promise<unknown>>).MigratePreview(sourceAgent.value, targetAgent.value) as Record<string, unknown>
    if (!res['success']) {
      previewError.value = (res['error'] as string) || '预览失败'
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
    toast('error', '迁移失败', '请至少选择一个提供商')
    return
  }
  const confirmMsg = mode.value === 'overwrite'
    ? `确定以“覆盖”模式迁移 ${selectedIds.value.length} 个提供商到 ${targetLabel.value} 吗？\n目标现有提供商将被清空后写入。`
    : `确定以“追加/合并”模式迁移 ${selectedIds.value.length} 个提供商到 ${targetLabel.value} 吗？\n同名提供商将合并模型。`
  if (!confirm(confirmMsg)) return
  migrating.value = true
  try {
    const res = await (api as unknown as Record<string, (...a: unknown[]) => Promise<unknown>>).MigrateExecute({
      source: sourceAgent.value,
      target: targetAgent.value,
      selected_ids: selectedIds.value,
      mode: mode.value,
    }) as Record<string, unknown>
    if (!res['success']) {
      toast('error', '迁移失败', res['error'] as string)
      return
    }
    toast('success', '迁移成功', `已迁移 ${res['migrated']} 个提供商到 ${targetLabel.value}（${mode.value === 'overwrite' ? '覆盖' : '追加/合并'}）`)
    await doPreview()
    window.dispatchEvent(new CustomEvent('agent-changed', { detail: targetAgent.value }))
  } catch (e) {
    toast('error', '迁移失败', String(e))
  } finally {
    migrating.value = false
  }
}

watch([sourceAgent, targetAgent], () => { void doPreview() })

onMounted(() => { void doPreview() })
</script>

<style scoped>
.migrate-page { display: flex; flex-direction: column; gap: 12px; }
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: var(--fluent-shadow-md); border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; }
.migrate-subtitle { font-size: 12px; color: var(--fluent-text-soft); margin-top: 4px; }
.migrate-controls { display: flex; gap: 12px; align-items: flex-end; flex-wrap: wrap; margin-top: 12px; }
.ctrl-group { display: flex; flex-direction: column; gap: 6px; }
.ctrl-arrow { font-size: 18px; padding-bottom: 6px; color: var(--fluent-text-soft); }
.form-label { font-size: 12px; color: var(--fluent-text-soft); }
.migrate-mode { display: flex; gap: 16px; flex-wrap: wrap; margin-top: 12px; }
.mode-item { display: flex; gap: 6px; align-items: center; font-size: 12px; cursor: pointer; }
.migrate-error { margin-top: 8px; font-size: 12px; color: #d13438; background: rgba(209,52,56,0.08); border: 1px solid rgba(209,52,56,0.2); border-radius: 8px; padding: 8px 12px; }
.migrate-split { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
@media (max-width: 1100px) { .migrate-split { grid-template-columns: 1fr; } }
.migrate-col { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.col-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.col-head h3 { font-size: 13px; font-weight: 600; }
.path-text { font-size: 10px; color: var(--fluent-text-soft); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 280px; }
.col-meta { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.badge { font-size: 10px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 0 6px; line-height: 16px; }
.badge-ok { color: #107c10; border-color: #107c10; background: rgba(16,124,16,0.08); }
.badge-warn { color: #ca5010; border-color: #ca5010; background: rgba(202,80,16,0.08); }
.meta-err { font-size: 11px; color: #d13438; }
.meta-count { font-size: 11px; color: var(--fluent-text-soft); }
.select-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; padding: 6px 8px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; }
.batch-item { display: inline-flex; gap: 4px; align-items: center; font-size: 11px; }
.count-label { font-size: 11px; color: var(--fluent-text-soft); }
.switch { position: relative; width: 40px; height: 20px; border-radius: 10px; background: rgba(0,0,0,0.18); border: none; cursor: pointer; }
.switch::after { content: ""; position: absolute; top: 4px; left: 4px; width: 12px; height: 12px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.3); transition: transform 0.2s; }
.switch.on { background: var(--accent); }
.switch.on::after { transform: translateX(20px); }
.provider-check-list, .provider-read-list { display: flex; flex-direction: column; gap: 6px; max-height: 300px; overflow-y: auto; min-height: 80px; }
.p-check { display: grid; grid-template-columns: auto 1fr; gap: 2px 8px; padding: 8px 10px; border: 1px solid var(--fluent-border); border-radius: 8px; cursor: pointer; background: var(--fluent-bg); align-items: center; }
.p-check.checked { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, transparent); }
.p-check input { grid-row: 1 / 3; }
.p-name { font-size: 12px; font-weight: 600; }
.p-id { font-size: 10px; color: var(--fluent-text-soft); }
.p-sub { font-size: 10px; color: var(--fluent-text-soft); grid-column: 2; }
.p-read { display: flex; flex-direction: column; gap: 2px; padding: 8px 10px; border: 1px solid var(--fluent-border); border-radius: 8px; background: var(--fluent-bg); }
.p-read.willOverwrite { border-color: #d13438; background: rgba(209,52,56,0.06); }
.overlap-tag { font-size: 10px; color: #d13438; font-weight: 600; }
.raw-block { display: flex; flex-direction: column; gap: 6px; }
.raw-title { font-size: 12px; font-weight: 600; display: flex; gap: 8px; align-items: center; }
.raw-fmt { font-size: 10px; color: var(--fluent-text-soft); border: 1px solid var(--fluent-border); border-radius: 999px; padding: 0 6px; }
.raw-pre { font-size: 11px; line-height: 1.5; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 10px; max-height: 360px; overflow: auto; white-space: pre-wrap; word-break: break-all; }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 16px; color: var(--fluent-text-soft); text-align: center; border: 1px dashed var(--fluent-border); border-radius: 8px; }
.empty-title { font-size: 12px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 11px; }
.migrate-footer { display: flex; gap: 12px; align-items: center; justify-content: space-between; flex-wrap: wrap; }
.footer-hint { font-size: 12px; color: var(--fluent-text-soft); }
.footer-actions { display: flex; gap: 8px; }
</style>
