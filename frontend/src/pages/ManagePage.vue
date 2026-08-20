<template>
  <div class="manage-page">
    <div class="manage-layout">
      <aside class="provider-sidebar fluent-card">
        <div class="sidebar-head">
          <h3>提供商</h3>
          <span class="sidebar-count">{{ providers.length }}</span>
        </div>
        <div class="sidebar-actions">
          <n-button size="small" type="primary" block @click="onAddProvider">+ 新增提供商</n-button>
        </div>
        <label class="sidebar-hide-builtin">
          <input type="checkbox" v-model="hideBuiltinNoKey" />
          <span>隐藏未填 API Key 的内置提供商</span>
        </label>
        <div class="sidebar-list">
          <div v-if="filteredProviders.length === 0" class="provider-empty">暂无提供商</div>
          <div
            v-for="p in filteredProviders"
            :key="p.id"
            class="provider-item"
            :class="{ selected: currentId === p.id }"
            @click="selectProvider(p.id)"
          >
            <div class="pi-top">
              <span class="pi-id">{{ p.name }}</span>
              <button class="pi-del" title="删除提供商" @click.stop="onDeleteProvider(p.id, p.name)">✕</button>
            </div>
            <div class="pi-sub">{{ p.id }} · {{ p.model_count }} 个模型</div>
            <div class="pi-badges">
              <span class="badge badge-kind">{{ p.kind }}</span>
              <span class="badge" :class="p.has_api_key ? 'badge-ok' : 'badge-warn'">{{ p.has_api_key ? 'API Key ✓' : '无 Key' }}</span>
            </div>
          </div>
        </div>
      </aside>

      <div class="editor-pane">
        <div v-if="!currentProvider" class="editor-empty">
          <div class="empty-title">选择一个提供商进行编辑</div>
          <div class="empty-sub">在左侧选择提供商，或点击「新增提供商」创建</div>
        </div>

        <div v-else class="editor-body">
          <div class="fluent-card card-pad">
            <div class="card-title">提供商设置</div>
            <div class="editor-grid">
              <label class="form-label">Provider ID（修改即重命名）</label>
              <n-input v-model:value="editForm.id" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
              <label class="form-label">显示名称</label>
              <n-input v-model:value="editForm.name" placeholder="例如：我的供应商" />
              <label class="form-label">协议类型 (kind)</label>
              <n-select v-model:value="editForm.kind" :options="kindOptions" style="min-width: 200px" />
              <label class="form-label">Base URL</label>
              <n-input v-model:value="editForm.base_url" placeholder="https://api.example.com/v1" />
              <label class="form-label">API Key</label>
              <div style="display:flex; gap:8px; align-items:center">
                <n-input v-model:value="editForm.api_key" :type="showKey ? 'text' : 'password'" placeholder="sk-..." style="flex:1" />
                <label style="display:flex; gap:4px; align-items:center; font-size:12px; cursor:pointer"><input type="checkbox" v-model="showKey" /> 显示</label>
              </div>
              <label class="form-label">options 额外参数（JSON）</label>
              <n-input v-model:value="editForm.options_json" type="textarea" :rows="3" placeholder='{"headers": {...}}' />
            </div>
            <div class="editor-actions">
              <n-button type="primary" :loading="saving" @click="onSaveProvider">💾 保存提供商</n-button>
              <n-button @click="onDuplicateProvider">📋 复制提供商</n-button>
              <n-button @click="onManualRefresh">📝 手动重新获取模型</n-button>
              <n-button :loading="autoRefreshing" @click="onAutoRefresh">🔄 自动重新获取模型</n-button>
              <n-popconfirm @positive-click="confirmDeleteProvider">
                <template #trigger><n-button type="error" ghost>🗑 删除提供商</n-button></template>
                确定删除提供商「{{ editForm.name || editForm.id }}」吗？
              </n-popconfirm>
              <label style="display:flex; gap:6px; align-items:center; font-size:12px; margin-left:auto"><input type="checkbox" v-model="editForm.api_key_required" /> 必须提供 API Key</label>
            </div>
          </div>

          <div class="fluent-card card-pad list-wrap">
            <div class="list-header">
              <h3>模型列表</h3>
              <div style="display:flex; gap:12px; align-items:center">
                <n-input v-model:value="manageQuery" placeholder="搜索模型 ID / 名称..." clearable style="width: 220px" size="small" />
                <span class="count-label">{{ filteredCards.length }} / {{ editForm.cards.length }}</span>
                <n-button size="small" @click="onAddModel">+ 手动添加模型</n-button>
              </div>
            </div>
            <div class="batch-bar">
              <div class="batch-row">
                <span class="label">批量编辑：</span>
                <label class="batch-item">全选 <button class="switch" :class="{ on: allSelected }" @click="batchSelectAll(true)" /></label>
                <n-button size="tiny" @click="batchSelectAll(false)">全不选</n-button>
                <div class="batch-spacer" />
                <n-button size="tiny" type="error" ghost :disabled="!hasSelected" @click="onDeleteSelected">删除所选</n-button>
              </div>
              <div class="batch-row batch-row--wrap">
                <label class="batch-item">思考 <button class="switch" :class="{ on: batchReasoningOn }" @click="batchToggleReasoning" /></label>
                <n-button size="tiny" @click="batchEnableAllVariants">一键全开思考等级</n-button>
                <span class="batch-group-label">推理档位</span>
                <span v-for="v in variantOpts" :key="v" class="batch-pill" @click="batchToggleVariant(v)">{{ v }}</span>
                <span class="batch-group-label">默认档位</span>
                <n-select v-model:value="batchDefault" :options="variantSelectOpts" placeholder="—" style="width: 100px" size="small" clearable @update:value="batchSetDefault" />
                <span class="batch-group-label">上下文长度</span>
                <n-input v-model:value="batchContext" placeholder="回车应用" size="small" style="width: 100px" @keydown.enter="applyBatchLimit('context')" />
                <span class="batch-group-label">输出长度</span>
                <n-input v-model:value="batchOutput" placeholder="回车应用" size="small" style="width: 100px" @keydown.enter="applyBatchLimit('output')" />
              </div>
            </div>
            <div class="list-scroll">
              <div v-if="filteredCards.length === 0" class="empty-state">
                <div class="empty-title">尚无模型</div>
                <div class="empty-sub">点击「+ 手动添加模型」，或使用「自动/手动重新获取模型」从 API 拉取。</div>
              </div>
              <ModelCard v-for="card in filteredCards" :key="card.model_id" :card="card" :show-select="true" @delete="removeCard(card)" @update="onCardUpdate" />
            </div>
          </div>
        </div>
      </div>
    </div>

    <n-modal v-model:show="showAutoConfirm" preset="card" title="确认自动重新获取" style="width: 480px" :mask-closable="false">
      <div style="font-size: 13px; line-height: 1.6">
        该操作将会直接覆盖现有模型配置，是否确认继续？
      </div>
      <label style="display:flex; gap:6px; align-items:center; font-size:12px; margin-top:12px; cursor:pointer">
        <input type="checkbox" v-model="dontAskAgain" /> 不再提示
      </label>
      <template #footer>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <n-button @click="showAutoConfirm = false">取消</n-button>
          <n-button type="primary" :loading="autoRefreshing" @click="confirmAutoRefresh">确认覆盖</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NInput, NSelect, NPopconfirm, NModal } from 'naive-ui'
import ModelCard from '../components/ModelCard.vue'
import type { ProviderSummary, ModelCard as Card, ProviderEdit } from '../types'
import * as api from '../api'

const router = useRouter()
const providers = ref<ProviderSummary[]>([])
const currentId = ref<string | null>(null)
const currentProvider = ref<ProviderEdit | null>(null)
const editForm = ref<ProviderEdit>({ id: '', name: '', kind: 'openai-compatible', base_url: '', api_key: '', api_key_required: false, options_json: '', cards: [] })
const showKey = ref(false)
const manageQuery = ref('')
const saving = ref(false)
const hideBuiltinNoKey = ref(localStorage.getItem('zcpm.hideBuiltinNoKey') !== '0')
const variantOpts = ['off', 'low', 'medium', 'high', 'xhigh', 'max']
const batchDefault = ref('')
const batchContext = ref('')
const batchOutput = ref('')
const showAutoConfirm = ref(false)
const dontAskAgain = ref(false)
const autoRefreshing = ref(false)

const kindOptions = [
  { label: 'OpenAI 兼容 (openai-compatible)', value: 'openai-compatible' },
  { label: 'Anthropic 兼容 (anthropic)', value: 'anthropic' },
  { label: 'Responses 兼容 (responses)', value: 'responses' },
]
const variantSelectOpts = computed(() => variantOpts.map((v) => ({ label: v, value: v })))

const filteredProviders = computed(() => {
  if (!hideBuiltinNoKey.value) return providers.value
  return providers.value.filter((p) => !(p.id.startsWith('builtin:') && !p.has_api_key))
})
const filteredCards = computed(() => {
  const q = manageQuery.value.trim().toLowerCase()
  if (!q) return editForm.value.cards
  return editForm.value.cards.filter((c) => c.model_id.toLowerCase().includes(q) || c.name.toLowerCase().includes(q))
})
const allSelected = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.select))
const hasSelected = computed(() => filteredCards.value.some((c) => c.select))
const batchReasoningOn = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.reasoning))

watch(hideBuiltinNoKey, (v) => { localStorage.setItem('zcpm.hideBuiltinNoKey', v ? '1' : '0') })

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

async function loadProviders() {
  const res = await api.ListProviders() as Record<string, unknown>
  if (res['success']) providers.value = (res['providers'] as ProviderSummary[]) || []
}
async function selectProvider(id: string) {
  currentId.value = id
  const res = await api.GetProvider(id) as Record<string, unknown>
  if (res['success']) {
    const p = res['provider'] as ProviderEdit
    currentProvider.value = p
    editForm.value = { ...p, cards: [...p.cards] }
  } else toast('error', '加载失败', res['error'] as string)
}

async function onAddProvider() {
  const name = prompt('请输入 Provider 显示名称（ID 将自动生成，可稍后修改）：', '我的供应商')
  if (name === null) return
  const id = await api.NewProviderID() as string
  editForm.value = { id, name: name || id, kind: 'openai-compatible', base_url: '', api_key: '', api_key_required: false, options_json: '', cards: [] }
  currentProvider.value = { ...editForm.value }
  currentId.value = id
}

async function onSaveProvider() {
  saving.value = true
  try {
    const payload: Record<string, unknown> = {
      id: editForm.value.id,
      name: editForm.value.name,
      kind: editForm.value.kind,
      base_url: editForm.value.base_url,
      api_key: editForm.value.api_key,
      api_key_required: editForm.value.api_key_required,
      options_json: editForm.value.options_json,
      cards: editForm.value.cards,
      _raw_provider: currentProvider.value?._raw_provider,
    }
    const res = await api.SaveProvider(currentId.value || editForm.value.id, payload) as Record<string, unknown>
    if (res['success']) {
      toast('success', '保存成功', `已保存 ${res['count']} 个模型到提供商 ${res['provider_id']}`)
      currentId.value = res['provider_id'] as string
      await loadProviders()
      if (currentId.value) await selectProvider(currentId.value)
    } else toast('error', '保存失败', res['error'] as string)
  } finally { saving.value = false }
}

async function onDuplicateProvider() {
  if (!currentProvider.value) { toast('error', '复制失败', '请先选择一个提供商'); return }
  const newId = await api.NewProviderID() as string
  editForm.value = { ...editForm.value, id: newId, name: editForm.value.name + ' (副本)' }
  currentProvider.value = null
  currentId.value = newId
  toast('success', '已复制', `新 ID：${newId}，请修改后保存`)
}

async function confirmDeleteProvider() {
  const id = currentId.value!
  const res = await api.DeleteProvider(id) as Record<string, unknown>
  if (res['success']) { toast('success', '已删除', `提供商「${id}」已删除`); currentId.value = null; currentProvider.value = null; await loadProviders() }
  else toast('error', '删除失败', res['error'] as string)
}
async function onDeleteProvider(id: string, _name: string) {
  currentId.value = id
  await selectProvider(id)
}

function onAddModel() {
  const mid = prompt('请输入模型 ID（即发送给 API 的模型名）：', 'gpt-4o-mini')
  if (!mid || !mid.trim()) return
  if (editForm.value.cards.some((c) => c.model_id === mid.trim())) { toast('info', '提示', `模型「${mid.trim()}」已在列表中。`); return }
  editForm.value.cards.push({ model_id: mid.trim(), name: mid.trim(), reasoning: false, variants: [], default_variant: '', context: '', output: '', select: true })
}

function removeCard(card: Card) {
  const idx = editForm.value.cards.indexOf(card)
  if (idx >= 0) editForm.value.cards.splice(idx, 1)
}

function onCardUpdate() { /* reactive */ }

function batchSelectAll(v: boolean) { for (const c of filteredCards.value) c.select = v }
function batchToggleReasoning() {
  const next = !batchReasoningOn.value
  for (const c of filteredCards.value) { c.reasoning = next; if (next && (!c.variants || c.variants.length === 0)) { c.variants = ['off', 'high', 'max']; c.default_variant = 'max' } else if (!next) { c.variants = []; c.default_variant = '' } }
}
function batchEnableAllVariants() {
  for (const c of filteredCards.value) {
    c.reasoning = true
    c.variants = [...variantOpts]
    if (!c.default_variant || !c.variants.includes(c.default_variant)) c.default_variant = 'max'
  }
}
function batchToggleVariant(v: string) {
  for (const c of filteredCards.value) {
    const arr = c.variants || []
    const idx = arr.indexOf(v)
    if (idx >= 0) arr.splice(idx, 1)
    else { arr.push(v); c.reasoning = true }
    c.variants = [...arr]
    if (c.default_variant && !arr.includes(c.default_variant)) c.default_variant = arr[0] || ''
  }
}
function batchSetDefault(v: string) {
  if (!v) return
  for (const c of filteredCards.value) { c.reasoning = true; if (!c.variants.includes(v)) c.variants.push(v); c.default_variant = v }
  batchDefault.value = ''
}
function applyBatchLimit(field: 'context' | 'output') {
  const val = field === 'context' ? batchContext.value : batchOutput.value
  for (const c of filteredCards.value) (c as Record<string, unknown>)[field] = val
}
function onDeleteSelected() {
  const selected = filteredCards.value.filter((c) => c.select)
  if (selected.length === 0) return
  if (!confirm(`确定删除选中的 ${selected.length} 个模型吗？`)) return
  for (const c of selected) {
    const idx = editForm.value.cards.indexOf(c)
    if (idx >= 0) editForm.value.cards.splice(idx, 1)
  }
}

function onManualRefresh() {
  if (!currentId.value || !currentProvider.value) { toast('error', '操作失败', '请先选择一个提供商'); return }
  sessionStorage.setItem('zcode-pm:manualRefresh', JSON.stringify({
    providerId: editForm.value.id,
    providerName: editForm.value.name,
    baseUrl: editForm.value.base_url,
    apiKey: editForm.value.api_key,
    kind: editForm.value.kind,
  }))
  router.push('/import')
}

function onAutoRefresh() {
  if (!currentId.value) return
  if (localStorage.getItem('zcode-pm:skipAutoRefreshConfirm') === '1') {
    void doAutoRefresh()
    return
  }
  dontAskAgain.value = false
  showAutoConfirm.value = true
}

async function confirmAutoRefresh() {
  if (dontAskAgain.value) localStorage.setItem('zcode-pm:skipAutoRefreshConfirm', '1')
  showAutoConfirm.value = false
  await doAutoRefresh()
}

async function doAutoRefresh() {
  if (!currentId.value) return
  autoRefreshing.value = true
  try {
    const res = await api.RefreshProviderModels(currentId.value, editForm.value.base_url, editForm.value.api_key) as Record<string, unknown>
    if (!res['success']) { toast('error', '获取失败', res['error'] as string); return }
    const models = (res['models'] as Card[]) || []
    toast('success', '获取成功', `成功获取 ${models.length} 个模型，已覆盖当前列表`)
    editForm.value.cards = models
  } finally {
    autoRefreshing.value = false
  }
}

onMounted(loadProviders)
</script>

<style scoped>
.manage-page { display: flex; flex-direction: column; flex: 1; min-height: 0; height: 100%; }
.manage-layout { display: grid; grid-template-columns: 260px 1fr; gap: 12px; flex: 1; min-height: 0; height: 100%; overflow: hidden; }
.provider-sidebar { display: flex; flex-direction: column; padding: 16px; min-height: 0; height: 100%; overflow: hidden; align-self: stretch; }
.sidebar-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.sidebar-head h3 { font-size: 14px; font-weight: 600; }
.sidebar-count { font-size: 12px; color: var(--fluent-text-soft); background: var(--fluent-border); border-radius: 999px; padding: 1px 8px; }
.sidebar-actions { margin-bottom: 8px; }
.sidebar-hide-builtin { display: flex; gap: 6px; align-items: center; font-size: 11px; color: var(--fluent-text-soft); margin-bottom: 8px; cursor: pointer; }
.provider-empty { padding: 16px; text-align: center; font-size: 12px; color: var(--fluent-text-soft); }
.sidebar-list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 8px; min-height: 0; }
.provider-item { background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 10px; cursor: pointer; }
.provider-item.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
.pi-top { display: flex; align-items: center; gap: 8px; }
.pi-id { font-size: 13px; font-weight: 600; flex: 1; word-break: break-all; }
.pi-del { color: #d13438; width: 22px; height: 22px; border-radius: 6px; border: 1px solid transparent; background: transparent; cursor: pointer; }
.pi-del:hover { background: rgba(209,52,56,0.1); border-color: #d13438; }
.pi-sub { font-size: 11px; color: var(--fluent-text-soft); margin-top: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pi-badges { display: flex; gap: 4px; margin-top: 6px; flex-wrap: wrap; }
.badge { font-size: 10px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 0 6px; line-height: 16px; }
.badge-kind { background: var(--fluent-bg); }
.badge-ok { color: #107c10; border-color: #107c10; background: rgba(16,124,16,0.08); }
.badge-warn { color: #ca5010; border-color: #ca5010; background: rgba(202,80,16,0.08); }
.editor-pane { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; height: 100%; gap: 12px; overflow-y: auto; overscroll-behavior: contain; padding-right: 2px; }
.editor-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; flex: 1; gap: 8px; text-align: center; color: var(--fluent-text-soft); background: var(--fluent-card-bg); border: 1px dashed var(--fluent-border); border-radius: 10px; padding: 32px; }
.empty-title { font-size: 16px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
.editor-body { display: flex; flex-direction: column; gap: 12px; }
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: var(--fluent-shadow-md); border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.editor-grid { display: grid; grid-template-columns: auto 1fr; gap: 8px 12px; align-items: center; }
.form-label { font-size: 12px; color: var(--fluent-text-soft); white-space: nowrap; }
.editor-actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 12px; align-items: center; }
.list-wrap { display: flex; flex-direction: column; gap: 8px; }
.list-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; }
.list-header h3 { font-size: 14px; font-weight: 600; }
.count-label { font-size: 12px; color: var(--fluent-text-soft); }
.batch-bar { display: flex; flex-direction: column; gap: 8px; padding: 8px 12px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; }
.batch-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.batch-row--wrap { align-items: center; }
.batch-spacer { flex: 1; }
.batch-bar .label { font-size: 12px; font-weight: 600; }
.batch-item { display: inline-flex; gap: 4px; align-items: center; font-size: 11px; }
.batch-group-label { font-size: 11px; color: var(--fluent-text-soft); font-weight: 600; }
.batch-pill { font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 2px 8px; cursor: pointer; }
.batch-pill:hover { border-color: var(--accent); }
.list-scroll { display: flex; flex-direction: column; }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 24px; color: var(--fluent-text-soft); text-align: center; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
.switch { position: relative; width: 40px; height: 20px; border-radius: 10px; background: rgba(0,0,0,0.18); border: none; cursor: pointer; }
.switch::after { content: ""; position: absolute; top: 4px; left: 4px; width: 12px; height: 12px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.3); transition: transform 0.2s; }
.switch.on { background: var(--accent); }
.switch.on::after { transform: translateX(20px); }
@media (max-width: 900px) { .manage-layout { grid-template-columns: 1fr; } .provider-sidebar { height: auto; max-height: 40vh; } }
</style>
