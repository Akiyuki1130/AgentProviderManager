<template>
  <div class="import-page">
    <div v-if="refreshMode" class="refresh-banner">
      <span>🔄</span> 正在为提供商 <b>{{ refreshMode.name }}</b> 重新获取模型列表
      <n-button size="tiny" @click="cancelRefresh">取消</n-button>
    </div>

    <div class="fluent-card card-pad">
      <div class="card-title">连接配置</div>
      <div class="conn-grid">
        <label class="form-label">Base URL</label>
        <n-input v-model:value="baseUrl" placeholder="https://api.example.com/v1" />
        <label class="form-label">协议类型 (kind)</label>
        <n-select v-model:value="kind" :options="kindOptions" style="min-width: 180px" />
        <label class="form-label">API Key</label>
        <div style="display:flex; gap:8px; align-items:center">
          <n-input v-model:value="apiKey" :type="showKey ? 'text' : 'password'" placeholder="sk-..." style="flex:1" />
          <label style="display:flex; gap:4px; align-items:center; font-size:11px; cursor:pointer"><input type="checkbox" v-model="showKey" /> 显示</label>
        </div>
        <label class="form-label">Provider ID</label>
        <n-input v-model:value="providerId" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        <label class="form-label">Provider 显示名称</label>
        <n-input v-model:value="providerName" placeholder="例如：我的供应商" />
      </div>
      <div class="fetch-row">
        <n-button type="primary" :loading="fetching" @click="onFetch">🔄 获取模型列表</n-button>
        <n-button @click="onClear">清空</n-button>
        <span class="status-text" :class="statusKind">{{ statusText }}</span>
      </div>
    </div>

    <div class="footer-bar fluent-card">
      <label style="display:flex; gap:6px; align-items:center; font-size:12px; cursor:pointer"><input type="checkbox" v-model="mergeChk" /> 若 Provider 已存在则合并模型（否则覆盖）</label>
      <div style="flex:1" />
      <n-button type="primary" color="#107c10" :disabled="cards.length === 0" :loading="importing" @click="onImport">⬇ 导入到 ZCode</n-button>
    </div>

    <div class="fluent-card card-pad list-wrap">
      <div class="list-header">
        <h3>模型列表</h3>
        <div style="display:flex; gap:12px; align-items:center">
          <n-input v-model:value="searchQuery" placeholder="搜索模型 ID / 名称..." clearable size="small" style="width: 220px" />
          <span class="count-label">{{ filteredCards.length }} / {{ cards.length }}</span>
          <n-button size="small" @click="onAddModel">+ 手动添加模型</n-button>
        </div>
      </div>
      <div class="batch-bar">
        <span class="label">批量编辑：</span>
        <label class="batch-item">全选 <button class="switch" :class="{ on: allSelected }" @click="batchSelectAll(true)" /></label>
        <n-button size="tiny" @click="batchSelectAll(false)">全不选</n-button>
        <label class="batch-item">思考 <button class="switch" :class="{ on: batchReasoning }" @click="batchToggleReasoning" /></label>
        <span class="batch-group-label">推理档位</span>
        <span v-for="v in variantOpts" :key="v" class="batch-pill" @click="batchToggleVariant(v)">{{ v }}</span>
        <span class="batch-group-label">默认档位</span>
        <n-select v-model:value="batchDefault" :options="variantSelectOpts" placeholder="—" style="width: 100px" size="small" clearable @update:value="batchSetDefault" />
        <span class="batch-group-label">上下文长度</span>
        <n-input v-model:value="batchContext" size="small" style="width: 100px" placeholder="回车应用" @keydown.enter="applyBatchLimit('context')" />
        <span class="batch-group-label">输出长度</span>
        <n-input v-model:value="batchOutput" size="small" style="width: 100px" placeholder="回车应用" @keydown.enter="applyBatchLimit('output')" />
      </div>
      <div class="list-scroll">
        <div v-if="filteredCards.length === 0 && cards.length === 0" class="empty-state">
          <div class="empty-title">尚未获取模型</div>
          <div class="empty-sub">输入 Base URL 和 API Key，点击「获取模型列表」。</div>
        </div>
        <div v-else-if="filteredCards.length === 0" class="empty-state">
          <div class="empty-title">没有匹配的模型</div>
          <div class="empty-sub">未找到与「{{ searchQuery }}」匹配的模型。</div>
        </div>
        <ModelCard v-for="card in filteredCards" :key="card.model_id" :card="card" :show-select="true" @delete="removeCard(card)" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { NButton, NInput, NSelect } from 'naive-ui'
import ModelCard from '../components/ModelCard.vue'
import type { ModelCard as Card } from '../types'
import * as api from '../api'

const baseUrl = ref('')
const kind = ref('openai-compatible')
const apiKey = ref('')
const providerId = ref('')
const providerName = ref('')
const showKey = ref(false)
const cards = ref<Card[]>([])
const searchQuery = ref('')
const statusText = ref('就绪。请输入地址和密钥后点击获取。')
const statusKind = ref('')
const fetching = ref(false)
const importing = ref(false)
const mergeChk = ref(true)
const refreshMode = ref<{ id: string; name: string } | null>(null)
const variantOpts = ['off', 'low', 'medium', 'high', 'xhigh', 'max']
const batchDefault = ref('')
const batchContext = ref('')
const batchOutput = ref('')

const kindOptions = [
  { label: 'OpenAI 兼容 (openai-compatible)', value: 'openai-compatible' },
  { label: 'Anthropic 兼容 (anthropic)', value: 'anthropic' },
  { label: 'Responses 兼容 (responses)', value: 'responses' },
]
const variantSelectOpts = computed(() => variantOpts.map((v) => ({ label: v, value: v })))
const filteredCards = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return cards.value
  return cards.value.filter((c) => c.model_id.toLowerCase().includes(q) || c.name.toLowerCase().includes(q))
})
const allSelected = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.select))
const batchReasoning = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.reasoning))

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

let guessTimer: ReturnType<typeof setTimeout> | null = null
function onBaseUrlInput() {
  if (guessTimer) clearTimeout(guessTimer)
  guessTimer = setTimeout(async () => {
    const url = baseUrl.value.trim()
    if (!url) return
    const pid = await api.GuessProviderID(url) as string
    if (pid && (!providerId.value || providerId.value === '')) { providerId.value = pid; providerName.value = providerName.value || pid }
    const k = await api.GuessKind(url) as string
    if (k) kind.value = k
  }, 400)
}

async function onFetch() {
  if (!baseUrl.value.trim()) { statusText.value = '请先填写 Base URL'; statusKind.value = 'warn'; return }
  fetching.value = true
  statusText.value = '正在请求 /models 接口...'
  statusKind.value = ''
  try {
    const res = await api.FetchModels(baseUrl.value.trim(), apiKey.value.trim()) as Record<string, unknown>
    if (!res['success']) { statusText.value = '✗ ' + (res['error'] as string); statusKind.value = 'err'; toast('error', '获取失败', res['error'] as string); return }
    cards.value = (res['models'] as Card[]) || []
    statusText.value = `✓ 成功获取 ${res['count']} 个模型`
    statusKind.value = 'ok'
  } catch (e) { statusText.value = '✗ ' + String(e); statusKind.value = 'err' }
  finally { fetching.value = false }
}

function onClear() { cards.value = []; searchQuery.value = ''; statusText.value = '就绪。请输入地址和密钥后点击获取。'; statusKind.value = '' }
function cancelRefresh() { refreshMode.value = null }
function onAddModel() {
  const mid = prompt('请输入模型 ID：', 'gpt-4o-mini')
  if (!mid || !mid.trim()) return
  if (cards.value.some((c) => c.model_id === mid.trim())) { toast('info', '提示', `模型「${mid.trim()}」已在列表中。`); return }
  cards.value.push({ model_id: mid.trim(), name: mid.trim(), reasoning: false, variants: [], default_variant: '', context: '', output: '', select: true })
}
function removeCard(card: Card) { const idx = cards.value.indexOf(card); if (idx >= 0) cards.value.splice(idx, 1) }
async function onImport() {
  const selected = cards.value.filter((c) => c.select)
  if (selected.length === 0) { toast('error', '导入失败', '请至少选择一个模型'); return }
  importing.value = true
  try {
    const payload = { provider_id: providerId.value || providerName.value, provider_name: providerName.value || providerId.value, base_url: baseUrl.value, api_key: apiKey.value, kind: kind.value, cards: selected, merge_models: mergeChk.value }
    const res = await api.ImportProvider(payload) as Record<string, unknown>
    if (res['success']) toast('success', '导入成功', `成功导入 ${res['count']} 个模型到 ${res['provider_id']}`)
    else toast('error', '导入失败', res['error'] as string)
  } finally { importing.value = false }
}
function batchSelectAll(v: boolean) { for (const c of filteredCards.value) c.select = v }
function batchToggleReasoning() {
  const next = !batchReasoning.value
  for (const c of filteredCards.value) { c.reasoning = next; if (next && (!c.variants || c.variants.length === 0)) { c.variants = ['off', 'high', 'max']; c.default_variant = 'max' } else if (!next) { c.variants = []; c.default_variant = '' } }
}
function batchToggleVariant(v: string) {
  for (const c of filteredCards.value) { const arr = c.variants || []; const idx = arr.indexOf(v); if (idx >= 0) arr.splice(idx, 1); else { arr.push(v); c.reasoning = true }; c.variants = [...arr]; if (c.default_variant && !arr.includes(c.default_variant)) c.default_variant = arr[0] || '' }
}
function batchSetDefault(v: string) { if (!v) return; for (const c of filteredCards.value) { c.reasoning = true; if (!c.variants.includes(v)) c.variants.push(v); c.default_variant = v }; batchDefault.value = '' }
function applyBatchLimit(field: 'context' | 'output') { const val = field === 'context' ? batchContext.value : batchOutput.value; for (const c of filteredCards.value) (c as Record<string, unknown>)[field] = val }

onMounted(async () => {
  try { const id = await api.NewProviderID() as string; if (!providerId.value) providerId.value = id } catch { /* ignore */ }
})
</script>

<style scoped>
.import-page { display: flex; flex-direction: column; gap: 12px; }
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: var(--fluent-shadow-md); border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.conn-grid { display: grid; grid-template-columns: auto 1fr auto 1fr; gap: 8px 12px; align-items: center; }
.form-label { font-size: 12px; color: var(--fluent-text-soft); }
.fetch-row { display: flex; gap: 12px; align-items: center; margin-top: 12px; flex-wrap: wrap; }
.status-text { font-size: 12px; color: var(--fluent-text-soft); }
.status-text.ok { color: #107c10; }
.status-text.err { color: #d13438; }
.status-text.warn { color: #ca5010; }
.footer-bar { display: flex; gap: 12px; align-items: center; padding: 12px 16px; flex-wrap: wrap; }
.refresh-banner { display: flex; gap: 8px; align-items: center; background: color-mix(in srgb, var(--accent) 10%, transparent); border: 1px solid var(--accent); border-radius: 8px; padding: 8px 12px; font-size: 12px; }
.list-wrap { display: flex; flex-direction: column; gap: 8px; }
.list-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; }
.list-header h3 { font-size: 14px; font-weight: 600; }
.count-label { font-size: 12px; color: var(--fluent-text-soft); }
.batch-bar { display: flex; gap: 8px; align-items: center; padding: 8px 12px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; flex-wrap: wrap; }
.batch-bar .label { font-size: 12px; font-weight: 600; }
.batch-item { display: inline-flex; gap: 4px; align-items: center; font-size: 11px; }
.batch-group-label { font-size: 11px; color: var(--fluent-text-soft); font-weight: 600; }
.batch-pill { font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 2px 8px; cursor: pointer; }
.batch-pill:hover { border-color: var(--accent); }
.list-scroll { display: flex; flex-direction: column; }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px; color: var(--fluent-text-soft); text-align: center; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
.switch { position: relative; width: 40px; height: 20px; border-radius: 10px; background: rgba(0,0,0,0.18); border: none; cursor: pointer; }
.switch::after { content: ""; position: absolute; top: 4px; left: 4px; width: 12px; height: 12px; border-radius: 50%; background: #fff; box-shadow: 0 1px 3px rgba(0,0,0,0.3); transition: transform 0.2s; }
.switch.on { background: var(--accent); }
.switch.on::after { transform: translateX(20px); }
@media (max-width: 700px) { .conn-grid { grid-template-columns: 1fr; } }
</style>
