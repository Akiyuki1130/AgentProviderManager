<template>
  <div class="page">
    <div class="fluent-card card-pad">
      <div class="card-title">🔑 API Key 钥匙串</div>
      <div class="kc-toolbar">
        <n-input v-model:value="query" placeholder="搜索名称 / 备注..." clearable size="small" style="width: 260px" />
        <span class="kc-count">{{ filteredEntries.length }} / {{ entries.length }} 个项目</span>
        <div style="flex:1" />
        <span class="kc-path" :title="kcPath">{{ kcPath }}</span>
        <n-button size="small" @click="showAll = !showAll">{{ showAll ? '隐藏全部 Key' : '显示全部 Key' }}</n-button>
        <n-button size="small" :disabled="selectedIds.size === 0" @click="copySelected">复制所选</n-button>
        <n-popconfirm @positive-click="deleteSelected">
          <template #trigger><n-button size="small" type="error" ghost :disabled="selectedIds.size === 0">删除所选</n-button></template>
          确定删除选中的 {{ selectedIds.size }} 个 API Key 吗？
        </n-popconfirm>
        <n-button type="primary" size="small" @click="openAdd">+ 添加项目</n-button>
      </div>
    </div>

    <div class="fluent-card card-pad list-wrap">
      <div class="list-header">
        <h3>项目列表</h3>
        <label style="display:flex; gap:6px; align-items:center; font-size:12px; cursor:pointer"><input type="checkbox" :checked="allChecked" @change="toggleAll" /> 全选</label>
      </div>
      <div class="kc-list">
        <div v-if="filteredEntries.length === 0" class="empty-state">
          <div class="empty-title">暂无 API Key</div>
          <div class="empty-sub">点击「+ 添加项目」保存你的 API Key。</div>
        </div>
        <div v-for="e in filteredEntries" :key="e.id" class="kc-row" :class="{ selected: selectedIds.has(e.id) }">
          <n-checkbox :checked="selectedIds.has(e.id)" @update:checked="(v: boolean) => v ? selectedIds.add(e.id) : selectedIds.delete(e.id)" />
          <div class="kc-main">
            <div class="kc-name">{{ e.name }}</div>
            <div class="kc-key">{{ displayKey(e) }}</div>
            <div v-if="e.note" class="kc-note">{{ e.note }}</div>
          </div>
          <div class="kc-time">{{ e.updated_at || e.created_at }}</div>
          <div class="kc-actions">
            <button class="kc-act" @click="copyKey(e)">复制</button>
            <button class="kc-act" @click="toggleReveal(e)">{{ isRevealed(e) ? '隐藏' : '显示' }}</button>
            <button class="kc-act" @click="useInImport(e)">填入导入页</button>
            <button class="kc-act" @click="openEdit(e)">编辑</button>
            <n-popconfirm @positive-click="deleteOne(e.id)">
              <template #trigger><button class="kc-act kc-act-del">删除</button></template>
              确定删除「{{ e.name }}」吗？
            </n-popconfirm>
          </div>
        </div>
      </div>
    </div>

    <!-- Add/Edit Modal -->
    <n-modal v-model:show="showModal" preset="card" :title="editingId ? '编辑 API Key' : '添加 API Key'" style="width: 480px">
      <div style="display:flex; flex-direction:column; gap:12px">
        <div>
          <label class="form-label">项目名称</label>
          <n-input v-model:value="modalForm.name" placeholder="例如：我的 AI 项目" />
        </div>
        <div>
          <label class="form-label">API Key</label>
          <n-input v-model:value="modalForm.api_key" type="password" placeholder="sk-..." show-password-on="click" />
        </div>
        <div>
          <label class="form-label">备注（可选）</label>
          <n-input v-model:value="modalForm.note" placeholder="例如：https://api.example.com/v1" />
        </div>
        <div v-if="modalError" style="color:#d13438; font-size:12px">{{ modalError }}</div>
      </div>
      <template #footer>
        <div style="display:flex; justify-content:flex-end; gap:8px">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="saving" @click="saveModal">保存</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted, watch } from 'vue'
import { NButton, NInput, NCheckbox, NModal, NPopconfirm } from 'naive-ui'
import type { KeychainEntry } from '../types'
import * as api from '../api'

const entries = ref<KeychainEntry[]>([])
const query = ref('')
const showAll = ref(false)
const revealed = reactive(new Set<string>())
const selectedIds = reactive(new Set<string>())
const secrets = reactive(new Map<string, string>())
const kcPath = ref('')
const showModal = ref(false)
const editingId = ref('')
const modalForm = reactive({ name: '', api_key: '', note: '' })
const modalError = ref('')
const saving = ref(false)

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

async function loadKeychain() {
  try {
    const res = await api.ListKeychain() as Record<string, unknown>
    if (res['success']) {
      entries.value = (res['entries'] as KeychainEntry[]) || []
      kcPath.value = (res['path'] as string) || ''
    } else toast('error', '钥匙串加载失败', res['error'] as string)
  } catch (e) { toast('error', '钥匙串加载失败', String(e)) }
}

function displayKey(e: KeychainEntry): string {
  if (showAll.value || revealed.has(e.id)) {
    return secrets.get(e.id) || e.key_hint || '••••••••'
  }
  return e.key_hint || '••••••••'
}
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
		if (key && await writeClipboard(key)) toast('success', '已复制', '✓ 已复制 API Key')
		else toast('error', '复制失败', '无法访问系统剪贴板')
	} catch (err) { toast('error', '复制失败', String(err)) }
}

async function copySelected() {
	await loadAllSecrets()
	const keys = Array.from(selectedIds).map((id) => secrets.get(id) || '').filter(Boolean)
	if (keys.length > 0 && await writeClipboard(keys.join('\n'))) toast('success', '已复制', `✓ 已复制 ${keys.length} 个 API Key`)
	else if (keys.length > 0) toast('error', '复制失败', '无法访问系统剪贴板')
}

async function useInImport(e: KeychainEntry) {
	try {
		const key = await fetchSecret(e.id)
		if (!key) { toast('error', '填入失败', '无法读取 API Key'); return }
		sessionStorage.setItem('zcode-pm:fillApiKey', key)
		toast('success', '已填入', `✓ 已把「${e.name}」的 API Key 准备填入导入页，请切换到导入页`)
	} catch (err) { toast('error', '填入失败', String(err)) }
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
  if (!modalForm.name.trim()) { modalError.value = '请填写项目名称'; return }
  if (!modalForm.api_key.trim()) { modalError.value = '请填写 API Key'; return }
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
  else toast('error', '删除失败', res['error'] as string)
}
async function deleteSelected() {
  const ids = Array.from(selectedIds)
  const res = await api.DeleteKeychainEntries(ids) as Record<string, unknown>
  if (res['success']) { selectedIds.clear(); await loadKeychain() }
  else toast('error', '删除失败', res['error'] as string)
}
function toggleAll(e: Event) {
  const checked = (e.target as HTMLInputElement).checked
  if (checked) filteredEntries.value.forEach((en) => selectedIds.add(en.id))
  else filteredEntries.value.forEach((en) => selectedIds.delete(en.id))
}

watch(showAll, (enabled) => { if (enabled) void loadAllSecrets() })

onMounted(loadKeychain)
</script>

<style scoped>
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: var(--fluent-shadow-md); border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.kc-toolbar { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
.kc-count { font-size: 12px; color: var(--fluent-text-soft); }
.kc-path { font-size: 10px; color: var(--fluent-text-soft); max-width: 280px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.list-wrap { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
.list-header { display: flex; align-items: center; justify-content: space-between; }
.list-header h3 { font-size: 14px; font-weight: 600; }
.kc-list { display: flex; flex-direction: column; }
.kc-row { display: flex; gap: 12px; align-items: center; padding: 10px 12px; background: var(--fluent-bg); border: 1px solid var(--fluent-border); border-radius: 8px; margin-bottom: 8px; }
.kc-row.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
.kc-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.kc-name { font-size: 13px; font-weight: 600; word-break: break-all; }
.kc-key { font-family: Consolas, monospace; font-size: 12px; color: var(--fluent-text-soft); word-break: break-all; }
.kc-note { font-size: 11px; color: var(--fluent-text-soft); word-break: break-all; }
.kc-time { font-size: 10px; color: var(--fluent-text-soft); white-space: nowrap; }
.kc-actions { display: flex; gap: 4px; flex-wrap: wrap; }
.kc-act { font-size: 11px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 2px 10px; background: transparent; cursor: pointer; }
.kc-act:hover { border-color: var(--accent); }
.kc-act-del { color: #d13438; border-color: #d13438; }
.kc-act-del:hover { background: rgba(209,52,56,0.08); }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px; color: var(--fluent-text-soft); text-align: center; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
.form-label { font-size: 12px; color: var(--fluent-text-soft); }
</style>
