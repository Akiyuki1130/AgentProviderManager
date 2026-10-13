<template>
  <div class="import-page">
    <!-- C5 统一页头：标题用既有 tab.import 文案；右侧无顶部主按钮组（获取/导入 CTA 均在所属卡片内），故不带 actions -->
    <PageHeader class="enter-stagger" :style="{ animationDelay: staggerMs(0) + 'ms' }" :title="t('tab.import', lang)" :subtitle="t('import.subtitle', lang)">
      <template #icon><n-icon :component="CloudDownloadOutline" :size="18" /></template>
    </PageHeader>

    <div v-if="manualRefreshInfo" class="refresh-banner enter-stagger" :style="{ animationDelay: staggerMs(1) + 'ms' }">
      <n-icon :component="DocumentTextOutline" /> {{ t('import.refreshBannerBefore', lang) }} <b>{{ manualRefreshInfo.name }}</b> {{ t('import.refreshBannerAfter', lang) }}
      <n-button size="small" @click="clearManualRefresh">{{ t('import.backToNormal', lang) }}</n-button>
    </div>

    <div v-if="showZcodeFormat" class="fluent-card card-pad enter-stagger" :style="{ animationDelay: staggerMs(2) + 'ms' }">
      <div class="card-title">{{ t('zcodeFormat.title', lang) }}</div>
      <div class="format-line">
        <span class="badge" :class="formatBadgeClass">{{ formatLabel }}</span>
        <span class="format-path-label">{{ t('zcodeFormat.pathLabel', lang) }}</span>
        <span class="format-path mono selectable" :title="appStore.zcodeFormatPath">{{ appStore.zcodeFormatPath || t('zcodeFormat.noPath', lang) }}</span>
      </div>
      <div class="format-desc">{{ t('zcodeFormat.desc', lang) }}</div>
      <div v-if="appStore.zcodeFormatError" class="format-error">{{ t('zcodeFormat.error', lang).replace('{error}', appStore.zcodeFormatError) }}</div>
    </div>

    <div v-if="showLegacyImport" class="fluent-card card-pad legacy-card enter-stagger" :style="{ animationDelay: staggerMs(3) + 'ms' }">
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
        <!-- 旧供应商导入行列表：单列，接全局 list-* FLIP 过渡。
             外层 v-if/v-else 模板链语义不变；key 用提供商 id（legacyProviders 按 id 唯一，勾选/应用均以 id 定位），不用 index -->
        <TransitionGroup name="list" tag="div" class="legacy-list">
          <div v-for="p in appStore.legacyProviders" :key="p.id" class="legacy-item">
            <n-checkbox :checked="legacySelected.includes(p.id)" @update:checked="(v: boolean) => toggleLegacy(p.id, v)" />
            <div class="legacy-meta">
              <div class="legacy-name">{{ p.name || p.id }}</div>
              <div class="legacy-sub tnum"><span class="mono">{{ p.id }}</span> · {{ p.kind }} · {{ t('legacyImport.models', lang).replace('{n}', String(p.model_count)) }}</div>
              <div v-if="droppedFields(p.id).length > 0" class="legacy-dropped">
                {{ t('legacyImport.dropped', lang).replace('{fields}', droppedFields(p.id).join(dropSeparator)) }}
              </div>
            </div>
          </div>
        </TransitionGroup>
        <div class="legacy-actions">
          <n-button type="primary" :disabled="legacySelected.length === 0" :loading="applyingLegacy" @click="openLegacyConfirm">{{ t('legacyImport.apply', lang) }}</n-button>
        </div>
      </template>
    </div>

    <div class="fluent-card card-pad enter-stagger" :style="{ animationDelay: secDelay(4) }">
      <div class="card-title">{{ t('import.connTitle', lang) }}</div>
      <div class="conn-grid">
        <label class="form-label">Base URL</label>
        <n-input v-model:value="baseUrl" placeholder="https://api.example.com/v1" @update:value="onBaseUrlInput" />
        <label class="form-label">{{ t('import.kindLabel', lang) }}</label>
        <n-select v-model:value="kind" :options="kindOptions" style="min-width: 180px" />
        <label class="form-label">API Key</label>
        <div style="display:flex; gap:8px; align-items:center">
          <n-input v-model:value="apiKey" :type="showKey ? 'text' : 'password'" placeholder="sk-..." style="flex:1" />
          <label style="display:flex; gap:4px; align-items:center; font-size:11px; cursor:pointer"><input type="checkbox" v-model="showKey" /> {{ t('import.showKey', lang) }}</label>
          <n-button @click="showKeychainPicker = true"><template #icon><n-icon :component="KeyOutline" /></template>{{ t('import.fromKeychain', lang) }}</n-button>
        </div>
        <label class="form-label">Provider ID</label>
        <n-input v-model:value="providerId" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
        <label class="form-label">{{ t('import.providerNameLabel', lang) }}</label>
        <n-input v-model:value="providerName" :placeholder="t('import.providerNamePlaceholder', lang)" />
      </div>
      <div class="fetch-row">
        <n-button type="primary" :loading="fetching" @click="onFetch"><template #icon><n-icon :component="SyncOutline" /></template>{{ t('btn.fetch', lang) }}</n-button>
        <n-button :loading="probing" @click="onProbe"><template #icon><n-icon :component="FlashOutline" /></template>{{ t('import.testConnection', lang) }}</n-button>
        <n-button @click="onClear">{{ t('btn.clear', lang) }}</n-button>
        <span class="status-text tnum" :class="statusKind"><n-icon v-if="statusIcon" :component="statusIcon" />{{ statusText }}</span>
      </div>
    </div>

    <div class="footer-bar fluent-card enter-stagger" :style="{ animationDelay: secDelay(5) }">
      <label style="display:flex; gap:6px; align-items:center; font-size:12px; cursor:pointer"><input type="checkbox" v-model="mergeChk" /> {{ t('import.mergeHint', lang) }}</label>
      <div style="flex:1" />
      <n-button type="primary" :disabled="cards.length === 0" :loading="importing" @click="onImport"><template #icon><n-icon :component="CloudDownloadOutline" /></template>{{ t('btn.import', lang) }}</n-button>
    </div>

    <div class="fluent-card card-pad list-wrap enter-stagger" :style="{ animationDelay: secDelay(6) }">
      <div class="list-header">
        <h3>{{ t('import.modelListTitle', lang) }}</h3>
        <div style="display:flex; gap:12px; align-items:center">
          <n-input v-model:value="searchQuery" :placeholder="t('import.searchPlaceholder', lang)" clearable size="small" style="width: 220px" :aria-label="t('import.searchAria', lang)" @keyup.esc="searchQuery = ''" />
          <span class="count-label tnum">{{ filteredCards.length }} / {{ cards.length }}</span>
          <n-button size="small" @click="onAddModel"><template #icon><n-icon :component="AddOutline" /></template>{{ t('import.addModel', lang) }}</n-button>
        </div>
      </div>
      <div class="batch-bar">
        <div class="batch-row">
          <span class="label">{{ t('import.batchEdit', lang) }}</span>
          <n-button size="small" @click="batchSelectAll(true)">{{ t('import.selectAll', lang) }}</n-button>
          <n-button size="small" @click="batchSelectAll(false)">{{ t('import.selectNone', lang) }}</n-button>
          <div class="batch-spacer" />
          <n-button size="small" type="error" ghost :disabled="!hasSelected" @click="onDeleteSelected"><template #icon><n-icon :component="TrashOutline" /></template>{{ t('import.deleteSelected', lang) }}</n-button>
        </div>
        <div class="batch-row batch-row--wrap">
          <label class="batch-item">{{ t('import.reasoning', lang) }} <button class="switch" :class="{ on: batchReasoning }" role="switch" :aria-checked="batchReasoning" :aria-label="t('import.reasoningAria', lang)" @click="batchToggleReasoning" /></label>
          <span class="batch-group-label">{{ t('import.variantTiers', lang) }}</span>
          <span v-for="v in variantOpts" :key="v" class="pill" role="button" tabindex="0" @click="batchToggleVariant(v)" @keydown.enter.prevent="batchToggleVariant(v)" @keydown.space.prevent="batchToggleVariant(v)">{{ v }}</span>
          <span class="batch-group-label">{{ t('import.defaultTier', lang) }}</span>
          <n-select v-model:value="batchDefault" :options="variantSelectOpts" placeholder="—" style="width: 100px" size="small" clearable @update:value="batchSetDefault" />
        </div>
        <div class="batch-row batch-row--wrap">
          <span class="batch-group-label">{{ t('import.contextLength', lang) }}</span>
          <!-- D10：除回车外补「应用」按钮，提示不依赖隐藏交互 -->
          <n-input v-model:value="batchContext" size="small" style="width: 100px" :placeholder="t('import.enterToApply', lang)" @keydown.enter="applyBatchLimit('context')" />
          <n-button size="small" @click="applyBatchLimit('context')">{{ t('import.apply', lang) }}</n-button>
          <span class="batch-group-label">{{ t('import.outputLength', lang) }}</span>
          <n-input v-model:value="batchOutput" size="small" style="width: 100px" :placeholder="t('import.enterToApply', lang)" @keydown.enter="applyBatchLimit('output')" />
          <n-button size="small" @click="applyBatchLimit('output')">{{ t('import.apply', lang) }}</n-button>
          <n-button size="small" type="primary" ghost :loading="autoMatchLoading" :disabled="filteredCards.length === 0" @click="onAutoMatchLimits">{{ t('import.autoMatch', lang) }}</n-button>
          <span class="batch-hint">{{ t('import.autoMatchHint', lang) }}</span>
        </div>
      </div>
      <div class="list-scroll">
        <!-- D10：重新获取期间给旧列表加半透明遮罩 + spinner（pointer-events:none，不毁布局、不挡交互） -->
        <div v-if="fetching" class="fetch-overlay">
          <n-spin size="medium" />
          <span>{{ t('import.fetchingModels', lang) }}</span>
        </div>
        <div v-if="filteredCards.length === 0 && cards.length === 0" class="empty-state">
          <div class="empty-title">{{ t('import.emptyTitle', lang) }}</div>
          <div class="empty-sub">{{ t('import.emptySub', lang) }}</div>
          <!-- B11 空态可操作化：直接触发既有 onFetch（Base URL 为空时由其给出 warn 提示） -->
          <n-button size="small" type="primary" :loading="fetching" @click="onFetch"><template #icon><n-icon :component="SyncOutline" /></template>{{ t('btn.fetch', lang) }}</n-button>
        </div>
        <div v-else-if="filteredCards.length === 0" class="empty-state">
          <div class="empty-title">{{ t('import.noMatchTitle', lang) }}</div>
          <div class="empty-sub">{{ t('import.noMatchSub', lang).replace('{q}', searchQuery) }}</div>
        </div>
        <!-- 模型卡片为单列纵向列表，接全局 list-* FLIP 过渡；空态保持 v-if 兄弟语义，只包卡片本体。
             key 用 model_id（列表内唯一：手动添加按 model_id 去重、自动匹配按 model_id 回写），不用 index -->
        <TransitionGroup name="list" tag="div" class="card-rows">
          <ModelCard v-for="card in filteredCards" :key="card.model_id" :card="card" :agent="settingStore.agent" :query="searchQuery" :show-select="true" @delete="removeCard(card)" />
        </TransitionGroup>
      </div>
    </div>

    <KeychainPicker v-model:show="showKeychainPicker" @pick="onKeychainPick" />

    <n-modal v-model:show="legacyConfirmShow" preset="card" :title="t('legacyImport.confirmTitle', lang)" style="width: 420px" :mask-closable="false">
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
import { ref, computed, onMounted, watch, type Component } from 'vue'
import { NButton, NInput, NSelect, NCheckbox, NModal, NIcon, NSpin } from 'naive-ui'
import {
  DocumentTextOutline, KeyOutline, SyncOutline, FlashOutline, CloudDownloadOutline,
  AddOutline, TrashOutline, CheckmarkCircleOutline, CloseCircleOutline, AlertCircleOutline,
} from '@vicons/ionicons5'
import { uiPrompt, uiConfirm } from '../stores/uiDialog'
import { useSettingStore } from '../stores/setting'
import { useAppStore } from '../stores/app'
import ModelCard from '../components/ModelCard.vue'
import KeychainPicker from '../components/KeychainPicker.vue'
import PageHeader from '../components/ui/PageHeader.vue'
import { staggerMs } from '../composables/useStagger'
import type { ModelCard as Card, KeychainEntry, ProbeResult } from '../types'
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
const statusText = ref(t('status.ready', lang.value))
const statusKind = ref('')
const fetching = ref(false)
const probing = ref(false)
const importing = ref(false)
const mergeChk = ref(true)
const showKeychainPicker = ref(false)
const manualRefreshInfo = ref<{ id: string; name: string } | null>(null)
const variantOpts = computed(() => settingStore.agent === 'deepseek' ? ['off', 'low', 'high', 'max'] : ['off', 'low', 'medium', 'high', 'xhigh', 'max'])
const batchDefault = ref('')
const batchContext = ref('')
const batchOutput = ref('')
const autoMatchLoading = ref(false)
const hasSelected = computed(() => filteredCards.value.some((c) => c.select))
// 状态行图标：按 statusKind 选语义图标，状态字符不再混进文本
const STATUS_ICONS: Record<string, Component> = { ok: CheckmarkCircleOutline, err: CloseCircleOutline, warn: AlertCircleOutline }
const statusIcon = computed<Component | null>(() => STATUS_ICONS[statusKind.value] || null)

const kindOptions = computed(() => [
  { label: t('import.kindOpenai', lang.value), value: 'openai-compatible' },
  { label: t('import.kindAnthropic', lang.value), value: 'anthropic' },
  { label: t('import.kindResponses', lang.value), value: 'responses' },
])
const variantSelectOpts = computed(() => variantOpts.value.map((v) => ({ label: v, value: v })))
const filteredCards = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return cards.value
  return cards.value.filter((c) => ((c.model_id || '') as string).toLowerCase().includes(q) || ((c.name || '') as string).toLowerCase().includes(q))
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

async function doFetch() {  fetching.value = true
  statusText.value = t('status.requesting', lang.value)
  statusKind.value = ''
  try {
    const res = await api.FetchModels(baseUrl.value.trim(), apiKey.value.trim()) as Record<string, unknown>
    if (!res['success']) { statusText.value = String(res['error'] || ''); statusKind.value = 'err'; toast('error', t('import.fetchFailed', lang.value), res['error'] as string); return }
    cards.value = (res['models'] as Card[]) || []
    statusText.value = t('import.fetchSuccess', lang.value).replace('{n}', String(res['count']))
    statusKind.value = 'ok'
  } catch (e) { statusText.value = String(e); statusKind.value = 'err' }
  finally { fetching.value = false }
}

function onFetch() {
  if (!baseUrl.value.trim()) { statusText.value = t('import.baseUrlRequired', lang.value); statusKind.value = 'warn'; return }
  void doFetch()
}

function formatProbe(res: ProbeResult): { kind: string; text: string } {
  if (!res.models_ok) return { kind: 'err', text: res.error || res.error_code || t('import.modelsUnavailable', lang.value) }
  const models = t('import.probeModels', lang.value).replace('{n}', String(res.models_count ?? 0)).replace('{ms}', String(res.models_ms ?? '—'))
  if (res.chat_skipped) return { kind: 'ok', text: `${models} · ${t('import.probeChatSkipped', lang.value)}` }
  if (!res.chat_ok) return { kind: 'warn', text: t('import.probeChatFailed', lang.value).replace('{error}', res.error || t('import.unknownError', lang.value)) }
  return { kind: 'ok', text: `${models} · ${t('import.probeChatOk', lang.value).replace('{ms}', String(res.chat_ms ?? '—'))}` }
}

async function onProbe() {
  if (!baseUrl.value.trim()) { statusText.value = t('import.baseUrlRequired', lang.value); statusKind.value = 'warn'; return }
  probing.value = true
  statusText.value = t('import.probing', lang.value)
  statusKind.value = ''
  try {
    const res = await api.TestConnectionRaw(baseUrl.value.trim(), apiKey.value.trim(), kind.value)
    const line = formatProbe(res)
    statusText.value = line.text
    statusKind.value = line.kind
  } catch (e) { statusText.value = String(e); statusKind.value = 'err' }
  finally { probing.value = false }
}

async function onKeychainPick(e: KeychainEntry) {
  try {
    const res = await api.GetKeychainEntry(e.id)
    if (!res.success) { toast('error', t('import.fillFailed', lang.value), (res.error as string) || ''); return }
    const entry = res.entry as Record<string, unknown>
    const key = (entry['api_key'] as string) || (entry['apiKey'] as string) || ''
    if (!key) { toast('error', t('import.fillFailed', lang.value), t('import.cannotReadKey', lang.value)); return }
    apiKey.value = key
    toast('success', t('import.filled', lang.value), t('import.filledHint', lang.value).replace('{name}', e.name))
  } catch (err) { toast('error', t('import.fillFailed', lang.value), String(err)) }
}

function onClear() { cards.value = []; searchQuery.value = ''; statusText.value = t('status.ready', lang.value); statusKind.value = '' }
function clearManualRefresh() {
  manualRefreshInfo.value = null
  sessionStorage.removeItem('zcode-pm:manualRefresh')
}
async function onAddModel() {
  const mid = await uiPrompt({ title: t('import.addModel', lang.value), label: t('import.modelIdLabel', lang.value), placeholder: t('import.modelIdPlaceholder', lang.value), initialValue: 'gpt-4o-mini' })
  if (!mid) return
  if (cards.value.some((c) => c.model_id === mid)) { toast('info', t('import.notice', lang.value), t('import.modelExists', lang.value).replace('{id}', mid)); return }
  cards.value.push({ model_id: mid, name: mid, reasoning: false, variants: [], default_variant: '', context: '', output: '', select: true })
}
function removeCard(card: Card) { const idx = cards.value.indexOf(card); if (idx >= 0) cards.value.splice(idx, 1) }
async function onDeleteSelected() {
  const selected = filteredCards.value.filter((c) => c.select)
  if (selected.length === 0) return
  if (!(await uiConfirm({ title: t('import.deleteSelectedTitle', lang.value), content: t('import.deleteSelectedConfirm', lang.value).replace('{n}', String(selected.length)), danger: true }))) return
  for (const c of selected) {
    const idx = cards.value.indexOf(c)
    if (idx >= 0) cards.value.splice(idx, 1)
  }
}
function onImport() {
  const selected = cards.value.filter((c) => c.select)
  if (selected.length === 0) { toast('error', t('import.importFailed', lang.value), t('import.noModelSelected', lang.value)); return }
  void doImport()
}

async function doImport() {
  const selected = cards.value.filter((c) => c.select)
  if (selected.length === 0) { toast('error', t('import.importFailed', lang.value), t('import.noModelSelected', lang.value)); return }
  importing.value = true
  try {
    let pid = providerId.value.trim() || providerName.value.trim()
    if (!pid) { pid = await api.NewProviderID() as string }
    providerId.value = pid
    const payload = { provider_id: pid, provider_name: providerName.value || pid, base_url: baseUrl.value, api_key: apiKey.value, kind: kind.value, cards: selected, merge_models: mergeChk.value }
    const res = await api.ImportProvider(payload) as Record<string, unknown>
    if (res['success']) {
      toast('success', t('import.importSuccess', lang.value), t('import.importSuccessBody', lang.value).replace('{n}', String(res['count'])).replace('{provider}', String(res['provider_id'])))
      if (manualRefreshInfo.value) {
        manualRefreshInfo.value = null
        sessionStorage.removeItem('zcode-pm:manualRefresh')
      }
    }
    else toast('error', t('import.importFailed', lang.value), res['error'] as string)
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
function applyBatchLimit(field: 'context' | 'output') { const val = field === 'context' ? batchContext.value : batchOutput.value; for (const c of filteredCards.value) (c as Record<string, unknown>)[field] = val }

async function onAutoMatchLimits() {
  const list = filteredCards.value
  if (list.length === 0) return
  const hasFilled = list.some((c) => isLimitFilled(c.context) || isLimitFilled(c.output))
  if (hasFilled && !(await uiConfirm({ title: t('import.autoMatch', lang.value), content: t('import.autoMatchConfirm', lang.value), danger: true }))) {
    return
  }
  autoMatchLoading.value = true
  try {
    const res = await api.ApplyModelPresets(list)
    if (!res['success']) { toast('error', t('import.autoMatchFailed', lang.value), (res['error'] as string) || ''); return }
    const updated = (res['cards'] as Card[]) || []
    const byId = new Map(updated.map((c) => [c.model_id, c]))
    for (let i = 0; i < cards.value.length; i++) {
      const m = byId.get(cards.value[i].model_id)
      if (m) cards.value[i] = m
    }
    toast('success', t('import.autoMatch', lang.value), t('import.autoMatchDone', lang.value).replace('{n}', String(res['filled'] ?? 0)))
  } catch (e) {
    toast('error', t('import.autoMatchFailed', lang.value), String(e))
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
// Agent 隔离：格式卡/旧版导入卡是 ZCode 配置体系专属概念，DeepSeek Harness 下不展示也不探测；
// 隐藏两节后，后续区块的 enter-stagger 序号前移 2，避免 DeepSeek 页面入场动画出现空档延迟
const showZcodeFormat = computed(() => settingStore.agent === 'zcode')
function secDelay(n: number): string {
  return staggerMs(showZcodeFormat.value || n < 4 ? n : n - 2) + 'ms'
}

const legacySelected = ref<string[]>([])
const legacyConfirmShow = ref(false)
const applyingLegacy = ref(false)

const showLegacyImport = computed(() => showZcodeFormat.value && appStore.zcodeFormat === 'v2' && appStore.supportsLegacyImport)
const formatLabel = computed(() => {
  if (appStore.zcodeFormat === 'v2') return t('zcodeFormat.v2', lang.value)
  if (appStore.zcodeFormat === 'legacy') return t('zcodeFormat.legacy', lang.value)
  return t('zcodeFormat.unknown', lang.value)
})
// 格式徽标复用全局 .badge 三档语义色（B10 去重），未知态用 .badge 默认中性态
const formatBadgeClass = computed(() => {
  if (appStore.zcodeFormat === 'v2') return 'badge-ok'
  if (appStore.zcodeFormat === 'legacy') return 'badge-warn'
  return ''
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
      statusText.value = t('import.refreshFilledStatus', lang.value)
      statusKind.value = ''
    } catch { /* ignore parse error */ }
  }

  if (showZcodeFormat.value) await loadZcodeFormatSections()
})

// 顶栏切回 ZCode Agent 时补探测（DeepSeek→ZCode 页面不重挂载，避免格式卡停在 unknown/旧数据）
watch(showZcodeFormat, (visible) => {
  if (visible) void loadZcodeFormatSections()
})

async function loadZcodeFormatSections() {
  await appStore.fetchZCodeFormat()
  if (appStore.zcodeFormat === 'v2' && appStore.supportsLegacyImport) {
    await appStore.fetchLegacyPreview()
    legacySelected.value = appStore.legacyProviders.map((p) => p.id)
  }
}
</script>

<style scoped>
.import-page { display: flex; flex-direction: column; gap: 12px; }
/* .fluent-card/.switch/.badge/.pill 均用 global.css 控件规范层单一定义（B10 去重） */
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.conn-grid { display: grid; grid-template-columns: minmax(0, auto) minmax(0, 1fr) minmax(0, auto) minmax(0, 1fr); gap: 8px 12px; align-items: center; }
.conn-grid > * { min-width: 0; }
.conn-grid :deep(.n-input), .conn-grid :deep(.n-select) { width: 100%; max-width: 100%; min-width: 0; }
.form-label { min-width: 0; font-size: 12px; color: var(--fluent-text-soft); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.fetch-row { display: flex; gap: 12px; align-items: center; margin-top: 12px; flex-wrap: wrap; }
.status-text { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; color: var(--fluent-text-soft); border-radius: 6px; padding: 2px 8px; }
.status-text.ok { color: var(--ok); background: var(--ok-soft); }
.status-text.err { color: var(--err); background: var(--err-soft); }
.status-text.warn { color: var(--warn); background: var(--warn-soft); }
.footer-bar { display: flex; gap: 12px; align-items: center; padding: 12px 16px; flex-wrap: wrap; }
.refresh-banner { display: flex; gap: 8px; align-items: center; background: color-mix(in srgb, var(--accent) 10%, transparent); border: 1px solid var(--accent); border-radius: 8px; padding: 8px 12px; font-size: 12px; }
.format-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.format-path-label { font-size: 11px; color: var(--fluent-text-soft); }
.format-path { font-size: 11px; color: var(--fluent-text); word-break: break-all; min-width: 0; }
.format-desc { font-size: 11px; color: var(--fluent-text-soft); margin-top: 6px; }
.format-error { font-size: 11px; color: var(--err); margin-top: 4px; }
.legacy-card { display: flex; flex-direction: column; gap: 8px; }
.legacy-desc { font-size: 11px; color: var(--fluent-text-soft); }
.legacy-hint { font-size: 12px; color: var(--fluent-text-soft); }
.legacy-select-all { display: inline-flex; gap: 6px; align-items: center; font-size: 12px; cursor: pointer; }
/* relative：TransitionGroup list-* 过渡的 leave 绝对定位保位需要 */
.legacy-list { display: flex; flex-direction: column; gap: 6px; position: relative; }
.legacy-item { display: flex; gap: 8px; align-items: center; border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; }
.legacy-meta { min-width: 0; flex: 1; }
.legacy-name { font-size: 13px; font-weight: 600; }
.legacy-sub { font-size: 11px; color: var(--fluent-text-soft); margin-top: 2px; word-break: break-all; }
.legacy-dropped { font-size: 11px; color: var(--warn); margin-top: 4px; }
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
.list-scroll { display: flex; flex-direction: column; position: relative; }
/* D10：fetching 期间覆盖旧列表的半透明遮罩；absolute 脱流 + pointer-events:none，不改变布局 */
.fetch-overlay {
  position: absolute; inset: 0; z-index: 1;
  display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 8px;
  font-size: 12px; color: var(--fluent-text-soft);
  background: color-mix(in srgb, var(--fluent-card-bg) 62%, transparent);
  pointer-events: none; border-radius: 8px;
}
/* FLIP 过渡容器：保持原列布局；relative 供 list-leave-active 的绝对定位保位（global.css） */
.card-rows { display: flex; flex-direction: column; position: relative; }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px; color: var(--fluent-text-soft); text-align: center; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
@media (max-width: 700px) { .conn-grid { grid-template-columns: 1fr; } }
</style>
