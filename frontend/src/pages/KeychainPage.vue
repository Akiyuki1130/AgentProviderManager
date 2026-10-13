<template>
  <div class="page">
    <!-- C5：页头独立于卡片，主操作（添加项目）收进 actions；页头是页面级主操作区，故用默认（medium）尺寸与工具条 small 按钮分级 -->
    <PageHeader class="enter-stagger" :style="{ animationDelay: staggerMs(0) + 'ms' }" :title="t('keychain.title', lang)">
      <template #icon><n-icon :component="KeyOutline" :size="18" /></template>
      <template #actions>
        <n-button type="primary" @click="openAdd">
          <template #icon><n-icon :component="AddOutline" /></template>
          {{ t('keychain.add', lang) }}
        </n-button>
      </template>
    </PageHeader>

    <div class="fluent-card card-pad enter-stagger" :style="{ animationDelay: staggerMs(1) + 'ms' }">
      <div class="kc-toolbar">
        <n-input v-model:value="query" :placeholder="t('keychain.searchPlaceholder', lang)" clearable size="small" style="width: 260px" :aria-label="t('keychain.searchAria', lang)" />
        <span class="kc-count tnum">{{ t('keychain.itemCount', lang).replace('{shown}', String(filteredEntries.length)).replace('{total}', String(entries.length)) }}</span>
        <div style="flex:1" />
        <span class="kc-path" :title="kcPath">{{ kcPath }}</span>
        <n-button size="small" @click="showAll = !showAll">
          <template #icon><n-icon :component="showAll ? EyeOffOutline : EyeOutline" /></template>
          {{ showAll ? t('keychain.hideAllKeys', lang) : t('keychain.showAllKeys', lang) }}
        </n-button>
        <n-button size="small" :disabled="selectedIds.size === 0" @click="copySelected">
          <template #icon><n-icon :component="CopyOutline" /></template>
          {{ t('keychain.copySelected', lang) }}
        </n-button>
        <n-popconfirm @positive-click="deleteSelected">
          <template #trigger>
            <n-button size="small" type="error" ghost :disabled="selectedIds.size === 0">
              <template #icon><n-icon :component="TrashOutline" /></template>
              {{ t('keychain.deleteSelected', lang) }}
            </n-button>
          </template>
          {{ t('keychain.deleteSelectedConfirm', lang).replace('{n}', String(selectedIds.size)) }}
        </n-popconfirm>
      </div>
    </div>

    <div class="fluent-card card-pad list-wrap enter-stagger" :style="{ animationDelay: staggerMs(2) + 'ms' }">
      <div class="list-header">
        <h3>{{ t('keychain.listTitle', lang) }}</h3>
        <label style="display:flex; gap:6px; align-items:center; font-size:12px; cursor:pointer"><input type="checkbox" :checked="allChecked" @change="toggleAll" /> {{ t('keychain.selectAll', lang) }}</label>
      </div>
      <div class="kc-list">
        <!-- D10：加载中 / 加载失败与空态分离（错误 ≠ 空态），拉取失败不再伪装「暂无条目」 -->
        <div v-if="loading && entries.length === 0" class="kc-state">
          <n-spin size="medium" />
          <div class="empty-sub">{{ t('keychain.loading', lang) }}</div>
        </div>
        <div v-else-if="loadError" class="kc-state">
          <n-icon :component="WarningOutline" :size="22" class="kc-state-icon" />
          <div class="empty-title">{{ t('keychain.loadFailed', lang) }}</div>
          <div class="empty-sub">{{ loadError }}</div>
          <n-button size="small" :loading="loading" @click="loadKeychain">
            <template #icon><n-icon :component="RefreshOutline" /></template>
            {{ t('keychain.retry', lang) }}
          </n-button>
        </div>
        <div v-else-if="filteredEntries.length === 0" class="empty-state">
          <div class="empty-title">{{ t('keychain.emptyTitle', lang) }}</div>
          <div class="empty-sub">{{ t('keychain.emptySub', lang) }}</div>
        </div>
        <!-- 单列条目列表接全局 list-* FLIP 过渡：只包条目本体，空态保持 v-if 兄弟语义；
             key 用后端唯一条目 id（删除/查询接口均按 id 定位），不用 index；
             刷新期间旧列表降透明度提示加载（不清空列表，保留 FLIP） -->
        <TransitionGroup name="list" tag="div" class="kc-rows" :class="{ 'kc-refreshing': loading }">
          <div v-for="e in filteredEntries" :key="e.id" class="kc-row" :class="{ selected: selectedIds.has(e.id) }">
            <n-checkbox :checked="selectedIds.has(e.id)" @update:checked="(v: boolean) => v ? selectedIds.add(e.id) : selectedIds.delete(e.id)" />
            <div class="kc-main">
              <div class="kc-name">{{ e.name }}<span v-if="linkCount(e.id) > 0" class="badge badge-ok kc-link-badge"><n-icon :component="LinkOutline" :size="11" /> {{ t('keychain.providerBadge', lang).replace('{n}', String(linkCount(e.id))) }}</span></div>
              <div class="kc-key">{{ displayKey(e) }}</div>
              <div v-if="e.note" class="kc-note">{{ e.note }}</div>
            </div>
            <div class="kc-time tnum">{{ formatTime(e.updated_at || e.created_at) }}</div>
            <div class="kc-actions">
              <button class="kc-act" @click="copyKey(e)">{{ t('keychain.copy', lang) }}</button>
              <button class="kc-act" @click="toggleReveal(e)">{{ isRevealed(e) ? t('keychain.hide', lang) : t('keychain.show', lang) }}</button>
              <button class="kc-act" @click="useInImport(e)">{{ t('keychain.useInImport', lang) }}</button>
              <button class="kc-act" @click="openEdit(e)">{{ t('keychain.edit', lang) }}</button>
              <n-popconfirm @positive-click="deleteOne(e.id)">
                <template #trigger><button class="kc-act kc-act-del">{{ t('keychain.delete', lang) }}</button></template>
                {{ t('keychain.deleteOneConfirm', lang).replace('{name}', e.name) }}
              </n-popconfirm>
            </div>
          </div>
        </TransitionGroup>
      </div>
    </div>

    <!-- Add/Edit Modal -->
    <n-modal v-model:show="showModal" preset="card" :title="editingId ? t('keychain.editTitle', lang) : t('keychain.addTitle', lang)" style="width: 480px">
      <div style="display:flex; flex-direction:column; gap:12px">
        <div>
          <label class="form-label">{{ t('keychain.formName', lang) }}</label>
          <n-input v-model:value="modalForm.name" :placeholder="t('keychain.formNamePlaceholder', lang)" />
        </div>
        <div>
          <label class="form-label">API Key</label>
          <n-input v-model:value="modalForm.api_key" type="password" placeholder="sk-..." show-password-on="click" />
        </div>
        <div>
          <label class="form-label">{{ t('keychain.formNote', lang) }}</label>
          <n-input v-model:value="modalForm.note" :placeholder="t('keychain.formNotePlaceholder', lang)" />
        </div>
        <div v-if="modalError" style="color:var(--err); font-size:12px">{{ modalError }}</div>
      </div>
      <template #footer>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <n-button @click="showModal = false">{{ t('common.cancel', lang) }}</n-button>
          <n-button type="primary" :loading="saving" @click="saveModal">{{ t('keychain.save', lang) }}</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted, watch } from 'vue'
import { NButton, NInput, NCheckbox, NIcon, NModal, NPopconfirm, NSpin } from 'naive-ui'
import { KeyOutline, AddOutline, CopyOutline, TrashOutline, EyeOutline, EyeOffOutline, LinkOutline, WarningOutline, RefreshOutline } from '@vicons/ionicons5'
import PageHeader from '../components/ui/PageHeader.vue'
import { staggerMs } from '../composables/useStagger'
import { useSettingStore } from '../stores/setting'
import { t } from '../i18n'
import type { KeychainEntry } from '../types'
import * as api from '../api'

const settingStore = useSettingStore()
const lang = computed(() => settingStore.resolvedLang)

const entries = ref<KeychainEntry[]>([])
// D10：列表拉取的加载态与错误态（错误 ≠ 空态），供列表区占位与「重试」按钮使用
const loading = ref(false)
const loadError = ref('')
const query = ref('')
const showAll = ref(false)
const revealed = reactive(new Set<string>())
const selectedIds = reactive(new Set<string>())
const secrets = reactive(new Map<string, string>())
const kcPath = ref('')
const kcLinks = ref<Record<string, string>>({})
const showModal = ref(false)
const editingId = ref('')
const modalForm = reactive({ name: '', api_key: '', note: '' })
const modalError = ref('')
const saving = ref(false)

// D6：ISO 时间本地化为 YYYY-MM-DD HH:mm:ss（与 RestorePointPage.formatTime 同款做法），非法值原样回落
function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

const filteredEntries = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return entries.value
  return entries.value.filter((e) => e.name.toLowerCase().includes(q) || (e.note || '').toLowerCase().includes(q))
})
const allChecked = computed(() => filteredEntries.value.length > 0 && filteredEntries.value.every((e) => selectedIds.has(e.id)))

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

// D10：进入页/刷新期间维护 loading，失败记录 loadError（列表区展示错误态 +「重试」，不再伪装空态）
async function loadKeychain() {
  loading.value = true
  loadError.value = ''
  try {
    const res = await api.ListKeychain() as Record<string, unknown>
    if (res['success']) {
      entries.value = (res['entries'] as KeychainEntry[]) || []
      kcPath.value = (res['path'] as string) || ''
    } else {
      loadError.value = (res['error'] as string) || t('keychain.unknownError', lang.value)
      toast('error', t('keychain.loadFailed', lang.value), loadError.value)
    }
  } catch (e) {
    loadError.value = String(e)
    toast('error', t('keychain.loadFailed', lang.value), loadError.value)
  } finally { loading.value = false }
}

// 钥匙串 ↔ 提供商关联：值等于本条目 ID 的 link 数量（键前缀是 agent，无需区分）
async function loadLinks() {
  try {
    const res = await api.GetKeychainLinks()
    kcLinks.value = res.success ? (res.links || {}) : {}
  } catch { kcLinks.value = {} }
}
function linkCount(entryId: string): number {
  let n = 0
  for (const v of Object.values(kcLinks.value)) if (v === entryId) n++
  return n
}

// D10：显示判定统一走 isRevealed。showAll=true 时 revealed 集合是「例外隐藏」名单，
// 原实现 `showAll || revealed.has` 直接全显，导致行内「隐藏」按钮点了无效、与文案反语义；
// 现在隐藏的条目回落到 key_hint。
function displayKey(e: KeychainEntry): string {
  if (isRevealed(e)) {
    return secrets.get(e.id) || e.key_hint || '••••••••'
  }
  return e.key_hint || '••••••••'
}
// 全局 showAll 与行内例外集合合成后的「当前是否可见」：条目按钮「显示/隐藏」文案以此为准
function isRevealed(e: KeychainEntry): boolean { return showAll.value ? !revealed.has(e.id) : revealed.has(e.id) }

async function toggleReveal(e: KeychainEntry) {
  if (showAll.value) {
    if (revealed.has(e.id)) revealed.delete(e.id)
    else revealed.add(e.id)
    return
  }
  if (revealed.has(e.id)) { revealed.delete(e.id); return }
  try {
    const res = await api.GetKeychainEntry(e.id) as Record<string, unknown>
    if (res['success']) {
      const entry = res['entry'] as Record<string, unknown>
      secrets.set(e.id, entry['api_key'] as string || entry['apiKey'] as string || '')
      revealed.add(e.id)
    }
  } catch { /* ignore */ }
}

async function loadAllSecrets() {
  for (const e of entries.value) {
    if (!secrets.has(e.id)) {
      try {
        const res = await api.GetKeychainEntry(e.id) as Record<string, unknown>
        if (res['success']) {
          const entry = res['entry'] as Record<string, unknown>
          secrets.set(e.id, entry['api_key'] as string || entry['apiKey'] as string || '')
        }
      } catch { /* ignore */ }
    }
  }
}

async function fetchSecret(id: string): Promise<string> {
	const cached = secrets.get(id)
	if (cached) return cached
	const res = await api.GetKeychainEntry(id) as Record<string, unknown>
	if (!res['success']) return ''
	const entry = res['entry'] as Record<string, unknown>
	const key = entry['api_key'] as string || entry['apiKey'] as string || ''
	if (key) secrets.set(id, key)
	return key
}

async function writeClipboard(text: string): Promise<boolean> {
	try {
		const { ClipboardSetText } = await import('../../wailsjs/runtime/runtime')
		if (await ClipboardSetText(text)) return true
	} catch { /* fall back to browser clipboard */ }
	try {
		await navigator.clipboard.writeText(text)
		return true
	} catch {
		return false
	}
}

async function copyKey(e: KeychainEntry) {
	try {
		const key = await fetchSecret(e.id)
		if (key && await writeClipboard(key)) toast('success', t('keychain.copied', lang.value), t('keychain.copiedKey', lang.value))
		else toast('error', t('keychain.copyFailed', lang.value), t('keychain.clipboardFailed', lang.value))
	} catch (err) { toast('error', t('keychain.copyFailed', lang.value), String(err)) }
}

async function copySelected() {
	await loadAllSecrets()
	const keys = Array.from(selectedIds).map((id) => secrets.get(id) || '').filter(Boolean)
	if (keys.length > 0 && await writeClipboard(keys.join('\n'))) toast('success', t('keychain.copied', lang.value), t('keychain.copiedCount', lang.value).replace('{n}', String(keys.length)))
	else if (keys.length > 0) toast('error', t('keychain.copyFailed', lang.value), t('keychain.clipboardFailed', lang.value))
}

async function useInImport(e: KeychainEntry) {
	try {
		const key = await fetchSecret(e.id)
		if (!key) { toast('error', t('keychain.fillFailed', lang.value), t('keychain.cannotReadKey', lang.value)); return }
		sessionStorage.setItem('zcode-pm:fillApiKey', key)
		toast('success', t('keychain.filled', lang.value), t('keychain.filledHint', lang.value).replace('{name}', e.name))
	} catch (err) { toast('error', t('keychain.fillFailed', lang.value), String(err)) }
}

function openAdd() { editingId.value = ''; modalForm.name = ''; modalForm.api_key = ''; modalForm.note = ''; modalError.value = ''; showModal.value = true }
function openEdit(e: KeychainEntry) {
  editingId.value = e.id
  modalForm.name = e.name
  modalForm.note = e.note
  modalForm.api_key = secrets.get(e.id) || ''
  modalError.value = ''
  showModal.value = true
  // Load full key if not cached
  if (!secrets.has(e.id)) {
    api.GetKeychainEntry(e.id).then((res) => {
      const r = res as Record<string, unknown>
      if (r['success']) {
        const entry = r['entry'] as Record<string, unknown>
        modalForm.api_key = entry['api_key'] as string || entry['apiKey'] as string || ''
        secrets.set(e.id, modalForm.api_key)
      }
    })
  }
}

async function saveModal() {
  modalError.value = ''
  if (!modalForm.name.trim()) { modalError.value = t('keychain.nameRequired', lang.value); return }
  if (!modalForm.api_key.trim()) { modalError.value = t('keychain.keyRequired', lang.value); return }
  saving.value = true
  try {
    const res = await api.SaveKeychainEntry({ id: editingId.value, name: modalForm.name, api_key: modalForm.api_key, note: modalForm.note }) as Record<string, unknown>
    if (res['success']) { showModal.value = false; await loadKeychain() }
    else modalError.value = res['error'] as string
  } finally { saving.value = false }
}

async function deleteOne(id: string) {
  const res = await api.DeleteKeychainEntries([id]) as Record<string, unknown>
  if (res['success']) await loadKeychain()
  else toast('error', t('keychain.deleteFailed', lang.value), res['error'] as string)
}
async function deleteSelected() {
  const ids = Array.from(selectedIds)
  const res = await api.DeleteKeychainEntries(ids) as Record<string, unknown>
  if (res['success']) { selectedIds.clear(); await loadKeychain() }
  else toast('error', t('keychain.deleteFailed', lang.value), res['error'] as string)
}
function toggleAll(e: Event) {
  const checked = (e.target as HTMLInputElement).checked
  if (checked) filteredEntries.value.forEach((en) => selectedIds.add(en.id))
  else filteredEntries.value.forEach((en) => selectedIds.delete(en.id))
}

watch(showAll, (enabled) => { if (enabled) void loadAllSecrets() })

onMounted(() => { void loadKeychain(); void loadLinks() })
</script>

<style scoped>
/* B10：.fluent-card / .badge / .badge-ok 用 global.css 规范层定义，此处不再重复 */
.card-pad { padding: 16px; }
.kc-toolbar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.kc-count { font-size: 12px; color: var(--fluent-text-soft); }
.kc-path { font-size: 10px; color: var(--fluent-text-soft); max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.list-wrap { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
.list-header { display: flex; align-items: center; justify-content: space-between; }
.list-header h3 { font-size: 14px; font-weight: 600; }
.kc-list { display: flex; flex-direction: column; }
/* FLIP 过渡容器：保持原列布局；relative 供 list-leave-active 的绝对定位保位（global.css） */
.kc-rows { display: flex; flex-direction: column; position: relative; }
.kc-row { display: flex; gap: 12px; align-items: center; padding: 10px 12px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; margin-bottom: 8px; }
.kc-row.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
.kc-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.kc-name { font-size: 13px; font-weight: 600; word-break: break-all; }
/* 链接徽标仅保留版式变体（行内定位），外观走 global.css 的 .badge/.badge-ok */
.kc-link-badge { margin-left: 6px; font-weight: 400; vertical-align: middle; white-space: nowrap; }
.kc-key { font-family: Consolas, monospace; font-size: 12px; color: var(--fluent-text-soft); word-break: break-all; }
.kc-note { font-size: 11px; color: var(--fluent-text-soft); word-break: break-all; }
.kc-time { font-size: 10px; color: var(--fluent-text-soft); white-space: nowrap; }
.kc-actions { display: flex; gap: 4px; flex-wrap: wrap; }
.kc-act { font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 2px 10px; background: transparent; cursor: pointer; }
.kc-act:hover { border-color: var(--accent); }
.kc-act-del { color: var(--err); border-color: color-mix(in srgb, var(--err) 40%, transparent); }
.kc-act-del:hover { background: var(--err-soft); }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px; color: var(--fluent-text-soft); text-align: center; }
/* D10：加载/错误占位（与空态同布局节奏）；刷新期间旧列表降透明度 */
.kc-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px; color: var(--fluent-text-soft); text-align: center; }
.kc-state-icon { color: var(--err); }
.kc-refreshing { opacity: 0.55; transition: opacity var(--dur-standard, 0.2s) var(--ease-standard, cubic-bezier(0.33, 0, 0, 1)); }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
.form-label { font-size: 12px; color: var(--fluent-text-soft); }
</style>
