<template>
  <div class="page">
    <div class="fluent-card card-pad">
      <div class="card-title">合并其他 config.json 到当前配置</div>
      <div class="oc-path-row">
        <n-input v-model:value="mergePath" readonly placeholder="config.json 路径" style="flex:1; font-family: monospace; font-size: 12px" />
        <n-button @click="onBrowse">浏览...</n-button>
        <n-button :disabled="!mergePath" @click="loadPreview">重新加载</n-button>
      </div>
      <div class="oc-status" :class="statusKind">{{ statusText }}</div>
      <div class="oc-toolbar">
        <n-button size="small" @click="selectAll(true)">全选</n-button>
        <n-button size="small" @click="selectAll(false)">全不选</n-button>
      </div>
      <div v-for="p in preview.providers" :key="p.id" class="oc-provider" :class="{ selected: selected.has(p.id) }" @click="toggle(p.id)">
        <n-checkbox :checked="selected.has(p.id)" @update:checked="(v: boolean) => v ? selected.add(p.id) : selected.delete(p.id)" @click.stop />
        <div class="oc-main">
          <div class="oc-name">{{ p.name }}</div>
          <div class="oc-sub">
            <span class="badge">{{ p.kind }}</span>
            <span class="badge" :class="p.has_api_key ? 'badge-ok' : 'badge-warn'">{{ p.has_api_key ? '有 Key' : '无 Key' }}</span>
            <span class="badge">{{ p.model_count }} 个模型</span>
          </div>
        </div>
      </div>
      <div class="oc-foot">
        <label style="display:flex; gap:6px; align-items:center; font-size:12px; cursor:pointer"><input type="checkbox" v-model="mergeChk" /> 已存在的 Provider 合并模型（否则覆盖）</label>
        <div style="flex:1" />
        <n-button type="primary" :disabled="selected.size === 0" :loading="importing" @click="onMerge">⬇ 合并所选到当前配置</n-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { NButton, NInput, NCheckbox } from 'naive-ui'
import type { ProviderSummary } from '../types'
import * as api from '../api'

const mergePath = ref('')
const preview = reactive<{ providers: ProviderSummary[]; exists: boolean; error: string }>({ providers: [], exists: false, error: '' })
const selected = reactive(new Set<string>())
const mergeChk = ref(true)
const importing = ref(false)
const statusText = ref('尚未选择文件，请点击「浏览...」选择要合并的 config.json。')
const statusKind = ref('')

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

async function loadPreview() {
  if (!mergePath.value.trim()) { statusText.value = '请先选择要合并的配置文件'; statusKind.value = 'err'; return }
  try {
    const res = await api.PreviewConfigMerge(mergePath.value) as Record<string, unknown>
    if (!res['success']) { statusText.value = '读取失败：' + (res['error'] as string); statusKind.value = 'err'; return }
    preview.providers = (res['providers'] as ProviderSummary[]) || []
    preview.exists = !!res['exists']
    preview.error = (res['error'] as string) || ''
    if (preview.error) { statusText.value = '读取失败：' + preview.error; statusKind.value = 'err' }
    else if (preview.providers.length === 0) { statusText.value = '该文件中没有可导入的 provider 配置。'; statusKind.value = 'err' }
    else { statusText.value = `找到 ${preview.providers.length} 个可合并的提供商，勾选后合并到当前配置。`; statusKind.value = 'ok' }
  } catch (e) { statusText.value = String(e); statusKind.value = 'err' }
}

async function onBrowse() {
  const res = await api.ChooseMergeFile() as Record<string, unknown>
  if (res['success']) { mergePath.value = res['path'] as string; await loadPreview() }
  else if (!res['cancelled']) toast('error', '选择失败', res['error'] as string)
}

function toggle(id: string) { if (selected.has(id)) selected.delete(id); else selected.add(id) }
function selectAll(v: boolean) { if (v) preview.providers.forEach((p) => selected.add(p.id)); else selected.clear() }

async function onMerge() {
  if (selected.size === 0) return
  importing.value = true
  try {
    const res = await api.MergeConfig({ path: mergePath.value, selected_ids: Array.from(selected), merge: mergeChk.value }) as Record<string, unknown>
    if (res['success']) { toast('success', '合并成功', `成功合并 ${Array.from(selected).length} 个提供商`); await loadPreview() }
    else toast('error', '合并失败', res['error'] as string)
  } finally { importing.value = false }
}
</script>

<style scoped>
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: var(--fluent-shadow-md); border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.oc-path-row { display: flex; gap: 8px; align-items: center; margin-bottom: 8px; }
.oc-status { font-size: 12px; color: var(--fluent-text-soft); margin-bottom: 8px; }
.oc-status.ok { color: #107c10; }
.oc-status.err { color: #d13438; }
.oc-toolbar { display: flex; gap: 8px; margin-bottom: 8px; }
.oc-provider { display: flex; gap: 12px; align-items: center; padding: 8px 12px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; margin-bottom: 8px; cursor: pointer; }
.oc-provider.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
.oc-main { display: flex; flex-direction: column; gap: 4px; }
.oc-name { font-size: 13px; font-weight: 600; }
.oc-sub { display: flex; gap: 4px; flex-wrap: wrap; }
.badge { font-size: 10px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 0 6px; line-height: 16px; }
.badge-ok { color: #107c10; border-color: #107c10; background: rgba(16,124,16,0.08); }
.badge-warn { color: #ca5010; border-color: #ca5010; background: rgba(202,80,16,0.08); }
.oc-foot { display: flex; gap: 12px; align-items: center; margin-top: 12px; padding-top: 12px; border-top: 1px solid var(--fluent-border); }
</style>
