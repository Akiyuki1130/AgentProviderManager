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
              <n-button @click="onManualRefresh">📝 手动重新获取</n-button>
              <n-button :loading="autoRefreshing" @click="onAutoRefresh">🔄 自动重新获取</n-button>
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
                <n-button size="tiny" @click="batchSelectAll(true)">全选</n-button>
                <n-button size="tiny" @click="batchSelectAll(false)">全不选</n-button>
                <div class="batch-spacer" />
                <n-button size="tiny" type="error" ghost :disabled="!hasSelected" @click="onDeleteSelected">删除所选</n-button>
              </div>
              <div class="batch-row batch-row--wrap">
                <label class="batch-item">思考 <button class="switch" :class="{ on: batchReasoningOn }" @click="batchToggleReasoning" /></label>
                <span class="batch-group-label">推理档位</span>
                <span v-for="v in variantOpts" :key="v" class="batch-pill" @click="batchToggleVariant(v)">{{ v }}</span>
                <span class="batch-group-label">默认档位</span>
                <n-select v-model:value="batchDefault" :options="variantSelectOpts" placeholder="—" style="width: 100px" size="small" clearable @update:value="batchSetDefault" />
              </div>
              <div v-if="settingStore.agent === 'opencode'" class="batch-row batch-row--wrap batch-capability-row">
                <span class="batch-group-label">OpenCode 能力</span>
                <button class="batch-capability-pill" :class="{ checked: batchAttachmentOn }" type="button" @click="batchToggleCapability('attachment')">支持附件</button>
              </div>
              <div class="batch-row batch-row--wrap">
                <span class="batch-group-label">上下文长度</span>
                <n-input v-model:value="batchContext" placeholder="回车应用" size="small" style="width: 100px" @keydown.enter="applyBatchLimit('context')" />
                <span class="batch-group-label">输出长度</span>
                <n-input v-model:value="batchOutput" placeholder="回车应用" size="small" style="width: 100px" @keydown.enter="applyBatchLimit('output')" />
                <n-button size="tiny" type="primary" ghost :loading="autoMatchLoading" :disabled="filteredCards.length === 0" @click="onAutoMatchLimits">自动匹配</n-button>
                <span class="batch-hint">具体以提供商为准，自动数据仅供参考</span>
              </div>
            </div>
            <div class="list-scroll">
              <div v-if="filteredCards.length === 0" class="empty-state">
                <div class="empty-title">尚无模型</div>
                <div class="empty-sub">点击「+ 手动添加模型」，或使用「自动/手动重新获取模型」从 API 拉取。</div>
              </div>
              <ModelCard v-for="card in filteredCards" :key="card.model_id" :card="card" :agent="settingStore.agent" :show-select="true" @delete="removeCard(card)" @update="onCardUpdate" />
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

    <n-modal v-model:show="showHttpConfirm" preset="card" title="⚠️ 安全提示" style="width: 480px" :mask-closable="false">
      <div style="font-size: 13px; line-height: 1.7">
        当前地址使用 <b style="color:#ca5010">http://</b> 明文协议连接。API Key 和请求内容将以明文传输，可能被网络窃听，存在安全风险。
        <br /><br />
        建议改用 <b>https://</b> 加密连接。若该地址仅用于本地开发或内网测试，可确认继续。
      </div>
      <template #footer>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <n-button @click="showHttpConfirm = false">取消</n-button>
          <n-button type="warning" @click="confirmHttpAction">仍要使用</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useSettingStore } from '../stores/setting'
import { registerManageGuard, requestLeave } from '../stores/leaveGuard'
import { NButton, NInput, NSelect, NPopconfirm, NModal } from 'naive-ui'
import ModelCard from '../components/ModelCard.vue'
import type { ProviderSummary, ModelCard as Card, ProviderEdit } from '../types'
import * as api from '../api'

const router = useRouter()
const settingStore = useSettingStore()
const providers = ref<ProviderSummary[]>([])
const currentId = ref<string | null>(null)
const currentProvider = ref<ProviderEdit | null>(null)
const editForm = ref<ProviderEdit>({ id: '', name: '', kind: 'openai-compatible', base_url: '', api_key: '', api_key_required: false, options_json: '', cards: [] })
const savedProvider = ref<ProviderEdit | null>(null)
const showKey = ref(false)
const manageQuery = ref('')
const saving = ref(false)
const hideBuiltinNoKey = ref(localStorage.getItem('zcpm.hideBuiltinNoKey') !== '0')
const variantOpts = computed(() => settingStore.agent === 'deepseek' ? ['off', 'low', 'high', 'max'] : ['off', 'low', 'medium', 'high', 'xhigh', 'max'])
const batchDefault = ref('')
const batchContext = ref('')
const batchOutput = ref('')
const autoMatchLoading = ref(false)
const showAutoConfirm = ref(false)
const dontAskAgain = ref(false)
const autoRefreshing = ref(false)
const showHttpConfirm = ref(false)
const pendingHttpAction = ref<'save' | 'refresh' | null>(null)

const kindOptions = [
  { label: 'OpenAI 兼容 (openai-compatible)', value: 'openai-compatible' },
  { label: 'Anthropic 兼容 (anthropic)', value: 'anthropic' },
  { label: 'Responses 兼容 (responses)', value: 'responses' },
]
const variantSelectOpts = computed(() => variantOpts.value.map((v) => ({ label: v, value: v })))

const filteredProviders = computed(() => {
  if (!hideBuiltinNoKey.value) return providers.value
  return providers.value.filter((p) => !(p.id.startsWith('builtin:') && !p.has_api_key))
})
const filteredCards = computed(() => {
  const q = manageQuery.value.trim().toLowerCase()
  if (!q) return editForm.value.cards
  return editForm.value.cards.filter((c) => ((c.model_id || '') as string).toLowerCase().includes(q) || ((c.name || '') as string).toLowerCase().includes(q))
})
const allSelected = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.select))
const hasSelected = computed(() => filteredCards.value.some((c) => c.select))
const batchReasoningOn = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.reasoning))
const batchAttachmentOn = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.attachment === true))

watch(hideBuiltinNoKey, (v) => { localStorage.setItem('zcpm.hideBuiltinNoKey', v ? '1' : '0') })

// ---- 未保存更改检测 ----
function deepClone<T>(v: T): T { return JSON.parse(JSON.stringify(v)) as T }
function deepEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true
  if (a === null || b === null) return a === b
  if (typeof a !== 'object' || typeof b !== 'object') return false
  const aArr = Array.isArray(a), bArr = Array.isArray(b)
  if (aArr !== bArr) return false
  if (aArr) {
    const aa = a as unknown[], bb = b as unknown[]
    if (aa.length !== bb.length) return false
    return aa.every((v, i) => deepEqual(v, bb[i]))
  }
  const ao = a as Record<string, unknown>, bo = b as Record<string, unknown>
  const ak = Object.keys(ao), bk = Object.keys(bo)
  if (ak.length !== bk.length) return false
  return ak.every((k) => Object.prototype.hasOwnProperty.call(bo, k) && deepEqual(ao[k], bo[k]))
}
// select 仅用于界面批量选择，不属于要保存的配置，比较时忽略
function normalizedProvider(p: ProviderEdit): unknown {
  return {
    id: p.id, name: p.name, kind: p.kind, base_url: p.base_url,
    api_key: p.api_key, api_key_required: p.api_key_required, options_json: p.options_json,
    cards: (p.cards || []).map((c) => {
      const o: Record<string, unknown> = {}
      for (const k of Object.keys(c)) if (k !== 'select') o[k] = (c as unknown as Record<string, unknown>)[k]
      return o
    }),
  }
}
const isDirty = computed(() => savedProvider.value !== null && !deepEqual(normalizedProvider(editForm.value), normalizedProvider(savedProvider.value)))

function discardChanges() {
  if (savedProvider.value) editForm.value = deepClone(savedProvider.value)
}

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

async function loadProviders() {
  const res = await api.ListProviders() as Record<string, unknown>
  if (res['success']) providers.value = (res['providers'] as ProviderSummary[]) || []
}
function selectProvider(id: string) {
  if (id === currentId.value) return
  requestLeave(() => { void loadProvider(id) })
}

async function loadProvider(id: string) {
  currentId.value = id
  const res = await api.GetProvider(id) as Record<string, unknown>
  if (res['success']) {
    const p = res['provider'] as ProviderEdit
    currentProvider.value = p
    editForm.value = { ...p, cards: p.cards.map((c) => ({ ...c })) }
    savedProvider.value = deepClone(p)
  } else toast('error', '加载失败', res['error'] as string)
}

function onAddProvider() {
  const name = prompt('请输入 Provider 显示名称（ID 将自动生成，可稍后修改）：', '我的供应商')
  if (name === null) return
  requestLeave(() => { void createProvider(name) })
}

async function createProvider(name: string) {
  const id = await api.NewProviderID() as string
  editForm.value = { id, name: name || id, kind: 'openai-compatible', base_url: '', api_key: '', api_key_required: false, options_json: '', cards: [] }
  currentProvider.value = { ...editForm.value }
  currentId.value = id
  savedProvider.value = deepClone(editForm.value)
}

function isHttpUrl(url: string): boolean {
  return url.trim().toLowerCase().startsWith('http://')
}

function onSaveProvider() {
  if (isHttpUrl(editForm.value.base_url)) { pendingHttpAction.value = 'save'; showHttpConfirm.value = true; return }
  void saveProvider()
}

async function saveProvider(): Promise<boolean> {
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
      if (currentId.value) await loadProvider(currentId.value)
      return true
    } else {
      toast('error', '保存失败', res['error'] as string)
      return false
    }
  } finally { saving.value = false }
}

function confirmDeleteProvider() {
  requestLeave(() => { void deleteProvider() })
}

async function deleteProvider() {
  const id = currentId.value!
  const res = await api.DeleteProvider(id) as Record<string, unknown>
  if (res['success']) { toast('success', '已删除', `提供商「${id}」已删除`); currentId.value = null; currentProvider.value = null; savedProvider.value = null; await loadProviders() }
  else toast('error', '删除失败', res['error'] as string)
}
function onAddModel() {
  const mid = prompt('请输入模型 ID（即发送给 API 的模型名）：', 'gpt-4o-mini')
  if (!mid || !mid.trim()) return
  if (editForm.value.cards.some((c) => c.model_id === mid.trim())) { toast('info', '提示', `模型「${mid.trim()}」已在列表中。`); return }
  editForm.value.cards.push({ model_id: mid.trim(), name: mid.trim(), reasoning: false, variants: [], default_variant: '', context: '', output: '', select: false })
}

function removeCard(card: Card) {
  const idx = editForm.value.cards.indexOf(card)
  if (idx >= 0) editForm.value.cards.splice(idx, 1)
}

function onCardUpdate() { /* reactive */ }

function batchSelectAll(v: boolean) { for (const c of filteredCards.value) c.select = v }
function batchToggleReasoning() {
  const next = !batchReasoningOn.value
  const onVariants = settingStore.reasoningAllIntensities ? [...variantOpts.value] : ['off', 'high', 'max']
  for (const c of filteredCards.value) { c.reasoning = next; if (next && (!c.variants || c.variants.length === 0)) { c.variants = [...onVariants]; c.default_variant = 'high' } else if (!next) { c.variants = []; c.default_variant = '' } }
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
  for (const c of filteredCards.value) {
    c.reasoning = true
    // 后端返回的模型 variants 可能为 null，需要兜底
    const arr = c.variants || []
    if (!arr.includes(v)) arr.push(v)
    c.variants = [...arr]
    c.default_variant = v
  }
  batchDefault.value = ''
}
function batchToggleCapability(field: 'attachment') {
  const next = !filteredCards.value.every((c) => c[field] === true)
  for (const c of filteredCards.value) c[field] = next
}
function applyBatchLimit(field: 'context' | 'output') {
  const val = field === 'context' ? batchContext.value : batchOutput.value
  for (const c of filteredCards.value) (c as Record<string, unknown>)[field] = val
}
async function onAutoMatchLimits() {
  const list = filteredCards.value
  if (list.length === 0) return
  const hasFilled = list.some((c) => isLimitFilled(c.context) || isLimitFilled(c.output))
  if (hasFilled && !confirm('部分模型已填写上下文/输出长度，自动匹配会用预设值覆盖这些已填写的值，是否确认继续？')) {
    return
  }
  autoMatchLoading.value = true
  try {
    const res = await api.ApplyModelPresets(list)
    if (!res['success']) { toast('error', '自动匹配失败', (res['error'] as string) || ''); return }
    const updated = (res['cards'] as Card[]) || []
    const byId = new Map(updated.map((c) => [c.model_id, c]))
    for (let i = 0; i < editForm.value.cards.length; i++) {
      const m = byId.get(editForm.value.cards[i].model_id)
      if (m) editForm.value.cards[i] = m
    }
    toast('success', '自动匹配', `已为 ${res['filled'] ?? 0} 个模型填充上下文/输出长度`)
  } catch (e) {
    toast('error', '自动匹配失败', String(e))
  } finally {
    autoMatchLoading.value = false
  }
}

function isLimitFilled(v: string | number | undefined): boolean {
  if (v === undefined || v === null) return false
  if (typeof v === 'string') return v.trim() !== ''
  return v > 0
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
  if (isHttpUrl(editForm.value.base_url)) { pendingHttpAction.value = 'refresh'; showHttpConfirm.value = true; return }
  if (localStorage.getItem('zcode-pm:skipAutoRefreshConfirm') === '1') {
    void doAutoRefresh()
    return
  }
  dontAskAgain.value = false
  showAutoConfirm.value = true
}

async function confirmHttpAction() {
  showHttpConfirm.value = false
  const action = pendingHttpAction.value
  pendingHttpAction.value = null
  if (action === 'save') await saveProvider()
  else if (action === 'refresh') await doAutoRefresh()
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
    // 重新获取后默认全不选，避免误操作批量编辑/删除
    for (const c of models) c.select = false
    editForm.value.cards = models
  } finally {
    autoRefreshing.value = false
  }
}

function onAgentChanged() {
  currentId.value = null
  currentProvider.value = null
  editForm.value = { id: '', name: '', kind: 'openai-compatible', base_url: '', api_key: '', api_key_required: false, options_json: '', cards: [] }
  savedProvider.value = null
  void loadProviders()
}
onMounted(() => {
  registerManageGuard(() => isDirty.value, saveProvider, discardChanges)
  void loadProviders()
  window.addEventListener('agent-changed', onAgentChanged)
})
onUnmounted(() => {
  registerManageGuard(null, null, null)
  window.removeEventListener('agent-changed', onAgentChanged)
})
</script>

<style scoped>
.manage-page { display: flex; flex-direction: column; flex: 1; min-height: 0; height: 100%; }
.manage-layout { display: grid; grid-template-columns: 244px 1fr; gap: 12px; flex: 1; min-height: 0; height: 100%; overflow: hidden; }
.provider-sidebar { display: flex; flex-direction: column; padding: 16px 16px 16px; min-height: 0; height: 100%; overflow: hidden; align-self: stretch; }
.sidebar-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.sidebar-head h3 { font-size: 14px; font-weight: 600; margin: 0; }
.sidebar-count { font-size: 12px; color: var(--fluent-text-soft); background: var(--fluent-border); border-radius: 999px; padding: 1px 8px; }
.sidebar-actions { margin-bottom: 8px; }
.sidebar-hide-builtin { display: flex; gap: 6px; align-items: center; font-size: 11px; color: var(--fluent-text-soft); margin-bottom: 8px; cursor: pointer; }
.provider-empty { padding: 16px; text-align: center; font-size: 12px; color: var(--fluent-text-soft); }
.sidebar-list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 8px; min-height: 0; }
.provider-item { background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; padding: 10px; cursor: pointer; }
.provider-item.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
.pi-top { display: flex; align-items: center; gap: 8px; }
.pi-id { font-size: 13px; font-weight: 600; flex: 1; word-break: break-all; }
.pi-sub { font-size: 11px; color: var(--fluent-text-soft); margin-top: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.pi-badges { display: flex; gap: 4px; margin-top: 6px; flex-wrap: nowrap; min-width: 0; }
.badge { font-size: 10px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 0 6px; line-height: 16px; white-space: nowrap; }
.badge-kind { background: var(--fluent-bg); }
.badge-ok { color: #107c10; border-color: #107c10; background: rgba(16,124,16,0.08); }
.badge-warn { color: #ca5010; border-color: #ca5010; background: rgba(202,80,16,0.08); }
.editor-pane { display: flex; flex-direction: column; flex: 1; min-width: 0; min-height: 0; height: 100%; gap: 12px; overflow-y: auto; overscroll-behavior: contain; padding-right: 2px; }
.editor-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; flex: 1; gap: 8px; text-align: center; color: var(--fluent-text-soft); background: var(--fluent-card-bg); border: 1px dashed var(--fluent-border); border-radius: 10px; padding: 32px; }
.empty-title { font-size: 16px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
.editor-body { display: flex; flex-direction: column; gap: 12px; }
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: none; border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.editor-grid { display: grid; grid-template-columns: minmax(0, auto) minmax(0, 1fr); gap: 8px 12px; align-items: center; }
.editor-grid > * { min-width: 0; }
.editor-grid :deep(.n-input), .editor-grid :deep(.n-select) { width: 100%; max-width: 100%; min-width: 0; }
.form-label { min-width: 0; font-size: 12px; color: var(--fluent-text-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.editor-actions { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 12px; align-items: center; overflow: visible; }
.editor-actions > * { flex-shrink: 0; }

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
.batch-hint { font-size: 10px; color: var(--fluent-text-soft); opacity: 0.75; }
.batch-pill { font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 2px 8px; cursor: pointer; }
.batch-pill:hover { border-color: var(--accent); }
.batch-capability-row { border-top: 1px solid var(--fluent-border); padding-top: 8px; }
.batch-capability-pill { font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 3px 10px; cursor: pointer; background: var(--fluent-card-bg); color: inherit; }
.batch-capability-pill:hover, .batch-capability-pill.checked { border-color: var(--accent); }
.batch-capability-pill.checked { background: color-mix(in srgb, var(--accent) 12%, transparent); color: var(--accent); }
.list-scroll { display: flex; flex-direction: column; min-height: 0; }
@media (max-width: 1099px) { .manage-layout { grid-template-columns: 1fr; } .provider-sidebar { height: auto; max-height: 32vh; } }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 24px; color: var(--fluent-text-soft); text-align: center; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
.switch { position: relative; width: 40px; height: 20px; border-radius: 10px; background: rgba(0,0,0,0.18); border: none; cursor: pointer; }
.switch::after { content: ""; position: absolute; top: 4px; left: 4px; width: 12px; height: 12px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.3); transition: transform 0.2s; }
.switch.on { background: var(--accent); }
.switch.on::after { transform: translateX(20px); }
</style>
