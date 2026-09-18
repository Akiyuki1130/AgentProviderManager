<template>
  <div class="import-page">
    <div v-if="manualRefreshInfo" class="refresh-banner">
      <span>📝</span> 正在为提供商 <b>{{ manualRefreshInfo.name }}</b> 手动重新获取模型（可挑选后导入）
      <n-button size="tiny" @click="clearManualRefresh">返回普通导入</n-button>
    </div>

    <div class="fluent-card card-pad">
      <div class="card-title">{{ t('zcodeFormat.title', lang) }}</div>
      <div class="format-line">
        <span class="format-badge" :class="`format-badge--${appStore.zcodeFormat}`">{{ formatLabel }}</span>
        <span class="format-path-label">{{ t('zcodeFormat.pathLabel', lang) }}</span>
        <span class="format-path" :title="appStore.zcodeFormatPath">{{ appStore.zcodeFormatPath || t('zcodeFormat.noPath', lang) }}</span>
      </div>
      <div class="format-desc">{{ t('zcodeFormat.desc', lang) }}</div>
      <div v-if="appStore.zcodeFormatError" class="format-error">{{ t('zcodeFormat.error', lang).replace('{error}', appStore.zcodeFormatError) }}</div>
    </div>

    <div v-if="showLegacyImport" class="fluent-card card-pad legacy-card">
      <div class="card-title">{{ t('legacyImport.title', lang) }}</div>
      <div class="legacy-desc">{{ t('legacyImport.desc', lang) }}</div>
      <div class="legacy-desc">{{ t('legacyImport.orderNote', lang) }}</div>
      <div v-if="appStore.legacyPreviewLoading" class="legacy-hint">{{ t('legacyImport.loading', lang) }}</div>
      <div v-else-if="appStore.legacyProviders.length === 0" class="legacy-hint">{{ t('legacyImport.empty', lang) }}</div>
      <template v-else>
        <label class="legacy-select-all">
          <n-checkbox :checked="allLegacySelected" :indeterminate="hasLegacySelection && !allLegacySelected" @update:checked="toggleAllLegacy" />
          <span>{{ t('legacyImport.selectAll', lang) }}</span>
        </label>
        <div class="legacy-list">
          <div v-for="p in appStore.legacyProviders" :key="p.id" class="legacy-item">
            <n-checkbox :checked="legacySelected.includes(p.id)" @update:checked="(v: boolean) => toggleLegacy(p.id, v)" />
            <div class="legacy-meta">
              <div class="legacy-name">{{ p.name || p.id }}</div>
              <div class="legacy-sub">{{ p.id }} · {{ p.kind }} · {{ t('legacyImport.models', lang).replace('{n}', String(p.model_count)) }}</div>
              <div v-if="droppedFields(p.id).length > 0" class="legacy-dropped">
                {{ t('legacyImport.dropped', lang).replace('{fields}', droppedFields(p.id).join(dropSeparator)) }}
              </div>
            </div>
          </div>
        </div>
        <div class="legacy-actions">
          <n-button type="primary" :disabled="legacySelected.length === 0" :loading="applyingLegacy" @click="openLegacyConfirm">{{ t('legacyImport.apply', lang) }}</n-button>
        </div>
      </template>
    </div>

    <div class="fluent-card card-pad">
      <div class="card-title">连接配置</div>
      <div class="conn-grid">
        <label class="form-label">Base URL</label>
        <n-input v-model:value="baseUrl" placeholder="https://api.example.com/v1" @update:value="onBaseUrlInput" />
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
      <n-button type="primary" color="#107c10" :disabled="cards.length === 0" :loading="importing" @click="onImport">⬇ 导入到当前 Agent</n-button>
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
        <div class="batch-row">
          <span class="label">批量编辑：</span>
          <n-button size="tiny" @click="batchSelectAll(true)">全选</n-button>
          <n-button size="tiny" @click="batchSelectAll(false)">全不选</n-button>
          <div class="batch-spacer" />
          <n-button size="tiny" type="error" ghost :disabled="!hasSelected" @click="onDeleteSelected">删除所选</n-button>
        </div>
        <div class="batch-row batch-row--wrap">
          <label class="batch-item">思考 <button class="switch" :class="{ on: batchReasoning }" @click="batchToggleReasoning" /></label>
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
          <n-input v-model:value="batchContext" size="small" style="width: 100px" placeholder="回车应用" @keydown.enter="applyBatchLimit('context')" />
          <span class="batch-group-label">输出长度</span>
          <n-input v-model:value="batchOutput" size="small" style="width: 100px" placeholder="回车应用" @keydown.enter="applyBatchLimit('output')" />
          <n-button size="tiny" type="primary" ghost :loading="autoMatchLoading" :disabled="filteredCards.length === 0" @click="onAutoMatchLimits">自动匹配</n-button>
          <span class="batch-hint">具体以提供商为准，自动数据仅供参考</span>
        </div>
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
        <ModelCard v-for="card in filteredCards" :key="card.model_id" :card="card" :agent="settingStore.agent" :show-select="true" @delete="removeCard(card)" />
      </div>
    </div>

    <n-modal v-model:show="legacyConfirmShow" preset="card" :title="t('legacyImport.confirmTitle', lang)" style="width: 460px" :mask-closable="false">
      <div style="font-size: 13px; line-height: 1.7">
        {{ t('legacyImport.confirmBody', lang).replace('{n}', String(legacySelected.length)) }}
      </div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <n-button :disabled="applyingLegacy" @click="legacyConfirmShow = false">{{ t('common.cancel', lang) }}</n-button>
          <n-button type="primary" :loading="applyingLegacy" @click="applyLegacyImport">{{ t('legacyImport.confirmOk', lang) }}</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { NButton, NInput, NSelect, NCheckbox, NModal } from 'naive-ui'
import { useSettingStore } from '../stores/setting'
import { useAppStore } from '../stores/app'
import ModelCard from '../components/ModelCard.vue'
import type { ModelCard as Card } from '../types'
import { t } from '../i18n'
import * as api from '../api'

const settingStore = useSettingStore()
const appStore = useAppStore()
const lang = computed(() => settingStore.resolvedLang)
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
const manualRefreshInfo = ref<{ id: string; name: string } | null>(null)
const variantOpts = computed(() => settingStore.agent === 'deepseek' ? ['off', 'low', 'high', 'max'] : ['off', 'low', 'medium', 'high', 'xhigh', 'max'])
const batchDefault = ref('')
const batchContext = ref('')
const batchOutput = ref('')
const autoMatchLoading = ref(false)
const hasSelected = computed(() => filteredCards.value.some((c) => c.select))

const kindOptions = [
  { label: 'OpenAI 兼容 (openai-compatible)', value: 'openai-compatible' },
  { label: 'Anthropic 兼容 (anthropic)', value: 'anthropic' },
  { label: 'Responses 兼容 (responses)', value: 'responses' },
]
const variantSelectOpts = computed(() => variantOpts.value.map((v) => ({ label: v, value: v })))
const filteredCards = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return cards.value
  return cards.value.filter((c) => ((c.model_id || '') as string).toLowerCase().includes(q) || ((c.name || '') as string).toLowerCase().includes(q))
})
const allSelected = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.select))
const batchReasoning = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.reasoning))
const batchAttachmentOn = computed(() => filteredCards.value.length > 0 && filteredCards.value.every((c) => c.attachment === true))

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

async function doFetch() {  fetching.value = true
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

function onFetch() {
  if (!baseUrl.value.trim()) { statusText.value = '请先填写 Base URL'; statusKind.value = 'warn'; return }
  void doFetch()
}

function onClear() { cards.value = []; searchQuery.value = ''; statusText.value = '就绪。请输入地址和密钥后点击获取。'; statusKind.value = '' }
function clearManualRefresh() {
  manualRefreshInfo.value = null
  sessionStorage.removeItem('zcode-pm:manualRefresh')
}
function onAddModel() {
  const mid = prompt('请输入模型 ID：', 'gpt-4o-mini')
  if (!mid || !mid.trim()) return
  if (cards.value.some((c) => c.model_id === mid.trim())) { toast('info', '提示', `模型「${mid.trim()}」已在列表中。`); return }
  cards.value.push({ model_id: mid.trim(), name: mid.trim(), reasoning: false, variants: [], default_variant: '', context: '', output: '', select: true })
}
function removeCard(card: Card) { const idx = cards.value.indexOf(card); if (idx >= 0) cards.value.splice(idx, 1) }
function onDeleteSelected() {
  const selected = filteredCards.value.filter((c) => c.select)
  if (selected.length === 0) return
  if (!confirm(`确定删除选中的 ${selected.length} 个模型吗？`)) return
  for (const c of selected) {
    const idx = cards.value.indexOf(c)
    if (idx >= 0) cards.value.splice(idx, 1)
  }
}
function onImport() {
  const selected = cards.value.filter((c) => c.select)
  if (selected.length === 0) { toast('error', '导入失败', '请至少选择一个模型'); return }
  void doImport()
}

async function doImport() {
  const selected = cards.value.filter((c) => c.select)
  if (selected.length === 0) { toast('error', '导入失败', '请至少选择一个模型'); return }
  importing.value = true
  try {
    let pid = providerId.value.trim() || providerName.value.trim()
    if (!pid) { pid = await api.NewProviderID() as string }
    providerId.value = pid
    const payload = { provider_id: pid, provider_name: providerName.value || pid, base_url: baseUrl.value, api_key: apiKey.value, kind: kind.value, cards: selected, merge_models: mergeChk.value }
    const res = await api.ImportProvider(payload) as Record<string, unknown>
    if (res['success']) {
      toast('success', '导入成功', `成功导入 ${res['count']} 个模型到 ${res['provider_id']}`)
      if (manualRefreshInfo.value) {
        manualRefreshInfo.value = null
        sessionStorage.removeItem('zcode-pm:manualRefresh')
      }
    }
    else toast('error', '导入失败', res['error'] as string)
  } finally { importing.value = false }
}
function batchSelectAll(v: boolean) { for (const c of filteredCards.value) c.select = v }
function batchToggleReasoning() {
  const next = !batchReasoning.value
  const onVariants = settingStore.reasoningAllIntensities ? [...variantOpts.value] : ['off', 'high', 'max']
  for (const c of filteredCards.value) { c.reasoning = next; c._raw_reasoning_enabled = next; if (next && (!c.variants || c.variants.length === 0)) { c.variants = [...onVariants]; c.default_variant = 'high' } else if (!next) { c.variants = []; c.default_variant = '' } }
}
function batchToggleVariant(v: string) {
  for (const c of filteredCards.value) { const arr = c.variants || []; const idx = arr.indexOf(v); if (idx >= 0) arr.splice(idx, 1); else { arr.push(v); c.reasoning = true; c._raw_reasoning_enabled = true }; c.variants = [...arr]; if (c.default_variant && !arr.includes(c.default_variant)) c.default_variant = arr[0] || '' }
}
function batchSetDefault(v: string) {
  if (!v) return
  for (const c of filteredCards.value) {
    c.reasoning = true
    c._raw_reasoning_enabled = true
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
function applyBatchLimit(field: 'context' | 'output') { const val = field === 'context' ? batchContext.value : batchOutput.value; for (const c of filteredCards.value) (c as Record<string, unknown>)[field] = val }

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
    for (let i = 0; i < cards.value.length; i++) {
      const m = byId.get(cards.value[i].model_id)
      if (m) cards.value[i] = m
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

// ---- ZCode 配置格式展示 & 旧供应商导入到新版 ----
const legacySelected = ref<string[]>([])
const legacyConfirmShow = ref(false)
const applyingLegacy = ref(false)

const showLegacyImport = computed(() => appStore.zcodeFormat === 'v2' && appStore.supportsLegacyImport)
const formatLabel = computed(() => {
  if (appStore.zcodeFormat === 'v2') return t('zcodeFormat.v2', lang.value)
  if (appStore.zcodeFormat === 'legacy') return t('zcodeFormat.legacy', lang.value)
  return t('zcodeFormat.unknown', lang.value)
})
const dropSeparator = computed(() => (lang.value === 'zh' || lang.value === 'ja' ? '、' : ', '))
const allLegacySelected = computed(() => appStore.legacyProviders.length > 0 && appStore.legacyProviders.every((p) => legacySelected.value.includes(p.id)))
const hasLegacySelection = computed(() => appStore.legacyProviders.some((p) => legacySelected.value.includes(p.id)))

function droppedFields(id: string): string[] {
  return appStore.legacyDropped[id] || []
}
function toggleAllLegacy(v: boolean) {
  legacySelected.value = v ? appStore.legacyProviders.map((p) => p.id) : []
}
function toggleLegacy(id: string, v: boolean) {
  if (v) {
    if (!legacySelected.value.includes(id)) legacySelected.value = [...legacySelected.value, id]
  } else {
    legacySelected.value = legacySelected.value.filter((x) => x !== id)
  }
}

function openLegacyConfirm() {
  if (legacySelected.value.length === 0) {
    toast('info', t('legacyImport.title', lang.value), t('legacyImport.noneSelected', lang.value))
    return
  }
  legacyConfirmShow.value = true
}

async function applyLegacyImport() {
  const ids = [...legacySelected.value]
  if (ids.length === 0) return
  applyingLegacy.value = true
  try {
    const res = await api.ApplyLegacyImport(ids)
    if (res.success) {
      const imported = res.imported || []
      const skipped = res.skipped || []
      const backup = res.backup || ''
      legacyConfirmShow.value = false
      legacySelected.value = []
      toast('success', t('legacyImport.successTitle', lang.value),
        t('legacyImport.successBody', lang.value).replace('{n}', String(imported.length)).replace('{backup}', backup || '—'))
      if (skipped.length > 0) {
        toast('info', t('legacyImport.successTitle', lang.value), t('legacyImport.skipped', lang.value).replace('{n}', String(skipped.length)))
      }
      await appStore.fetchZCodeFormat()
      await appStore.fetchProviders()
      if (appStore.supportsLegacyImport) await appStore.fetchLegacyPreview()
    } else {
      toast('error', t('legacyImport.failedTitle', lang.value), res.error || '')
    }
  } catch (e) {
    toast('error', t('legacyImport.failedTitle', lang.value), String(e))
  } finally {
    applyingLegacy.value = false
  }
}

onMounted(async () => {
  const fillApiKey = sessionStorage.getItem('zcode-pm:fillApiKey')
  if (fillApiKey) {
    apiKey.value = fillApiKey
    sessionStorage.removeItem('zcode-pm:fillApiKey')
  }
  const raw = sessionStorage.getItem('zcode-pm:manualRefresh')
  if (raw) {
    try {
      const data = JSON.parse(raw) as { providerId: string; providerName: string; baseUrl: string; apiKey: string; kind: string }
      if (data.baseUrl) baseUrl.value = data.baseUrl
      if (data.apiKey) apiKey.value = data.apiKey
      if (data.providerId) providerId.value = data.providerId
      if (data.providerName) providerName.value = data.providerName
      if (data.kind) kind.value = data.kind
      manualRefreshInfo.value = { id: data.providerId || data.providerName || '', name: data.providerName || data.providerId || '' }
      statusText.value = '已自动填入提供商信息，点击「获取模型列表」后可挑选模型并导入。'
      statusKind.value = ''
    } catch { /* ignore parse error */ }
  }

  await appStore.fetchZCodeFormat()
  if (appStore.zcodeFormat === 'v2' && appStore.supportsLegacyImport) {
    await appStore.fetchLegacyPreview()
    legacySelected.value = appStore.legacyProviders.map((p) => p.id)
  }
})
</script>

<style scoped>
.import-page { display: flex; flex-direction: column; gap: 12px; }
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: var(--fluent-shadow-md); border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.conn-grid { display: grid; grid-template-columns: minmax(0, auto) minmax(0, 1fr) minmax(0, auto) minmax(0, 1fr); gap: 8px 12px; align-items: center; }
.conn-grid > * { min-width: 0; }
.conn-grid :deep(.n-input), .conn-grid :deep(.n-select) { width: 100%; max-width: 100%; min-width: 0; }
.form-label { min-width: 0; font-size: 12px; color: var(--fluent-text-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.fetch-row { display: flex; gap: 12px; align-items: center; margin-top: 12px; flex-wrap: wrap; }
.status-text { font-size: 12px; color: var(--fluent-text-soft); }
.status-text.ok { color: #107c10; }
.status-text.err { color: #d13438; }
.status-text.warn { color: #ca5010; }
.footer-bar { display: flex; gap: 12px; align-items: center; padding: 12px 16px; flex-wrap: wrap; }
.refresh-banner { display: flex; gap: 8px; align-items: center; background: color-mix(in srgb, var(--accent) 10%, transparent); border: 1px solid var(--accent); border-radius: 8px; padding: 8px 12px; font-size: 12px; }
.format-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.format-badge { font-size: 11px; font-weight: 600; border-radius: 4px; padding: 2px 8px; border: 1px solid var(--fluent-border); background: var(--fluent-bg); }
.format-badge--v2 { color: #107c10; border-color: color-mix(in srgb, #107c10 40%, transparent); background: color-mix(in srgb, #107c10 10%, transparent); }
.format-badge--legacy { color: #ca5010; border-color: color-mix(in srgb, #ca5010 40%, transparent); background: color-mix(in srgb, #ca5010 10%, transparent); }
.format-badge--unknown { color: var(--fluent-text-soft); }
.format-path-label { font-size: 11px; color: var(--fluent-text-soft); }
.format-path { font-size: 11px; color: var(--fluent-text); word-break: break-all; min-width: 0; }
.format-desc { font-size: 11px; color: var(--fluent-text-soft); margin-top: 6px; }
.format-error { font-size: 11px; color: #d13438; margin-top: 4px; }
.legacy-card { display: flex; flex-direction: column; gap: 8px; }
.legacy-desc { font-size: 11px; color: var(--fluent-text-soft); }
.legacy-hint { font-size: 12px; color: var(--fluent-text-soft); }
.legacy-select-all { display: inline-flex; gap: 6px; align-items: center; font-size: 12px; cursor: pointer; }
.legacy-list { display: flex; flex-direction: column; gap: 6px; }
.legacy-item { display: flex; gap: 8px; align-items: flex-start; border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; }
.legacy-meta { min-width: 0; flex: 1; }
.legacy-name { font-size: 13px; font-weight: 600; }
.legacy-sub { font-size: 11px; color: var(--fluent-text-soft); margin-top: 2px; word-break: break-all; }
.legacy-dropped { font-size: 11px; color: #ca5010; margin-top: 4px; }
.legacy-actions { display: flex; justify-content: flex-end; }
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
