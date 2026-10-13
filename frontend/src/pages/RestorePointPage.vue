<template>
  <div class="restore-page">
    <!-- ZCode 兼容性问题：issues 为空时不显示该卡片 -->
    <div v-if="compatIssues.length > 0" class="fluent-card card-pad compat-card">
      <div class="compat-head">
        <span class="compat-icon">⚠</span>
        <div class="card-title compat-title">{{ t('restorePoints.compatTitle', lang) }}</div>
        <div style="flex:1" />
        <n-button size="small" type="primary" color="#ca5010" :loading="fixing" @click="onRepairCompat">
          {{ t('restorePoints.compatFix', lang) }}
        </n-button>
      </div>
      <div class="compat-desc">{{ t('restorePoints.compatDesc', lang) }}</div>
      <div v-if="compatPath" class="compat-path" :title="compatPath">
        {{ t('restorePoints.compatPath', lang).replace('{path}', compatPath) }}
      </div>
      <div class="compat-list">
        <div v-for="(issue, i) in compatIssues" :key="`${issue.code}:${i}`" class="compat-item">
          <div class="compat-msg">{{ issue.msg }}</div>
          <div v-if="issue.path" class="compat-issue-path">{{ issue.path }}</div>
        </div>
      </div>
    </div>

    <div class="fluent-card card-pad">
      <div class="card-title">{{ t('restorePoints.title', lang) }}</div>
      <div class="rp-desc">{{ t('restorePoints.desc', lang) }}</div>
      <div class="rp-toolbar">
        <span class="rp-dir-label">{{ t('restorePoints.dirLabel', lang) }}</span>
        <span class="rp-dir" :title="dir">{{ dir || t('restorePoints.noDir', lang) }}</span>
        <div style="flex:1" />
        <span class="rp-count">{{ t('restorePoints.count', lang).replace('{n}', String(points.length)) }}</span>
        <n-button size="small" @click="onOpenDir">{{ t('restorePoints.openDir', lang) }}</n-button>
        <n-button size="small" :loading="pruning" @click="onPrune">{{ t('restorePoints.prune', lang) }}</n-button>
        <n-button size="small" @click="openExport">{{ t('restorePoints.exportBtn', lang) }}</n-button>
        <n-button size="small" :loading="openingBackup" @click="openImport">{{ t('restorePoints.importBtn', lang) }}</n-button>
      </div>
    </div>

    <div class="fluent-card card-pad list-wrap">
      <div class="list-header">
        <h3>{{ t('restorePoints.listTitle', lang) }}</h3>
        <span class="count-label">{{ points.length }}</span>
      </div>
      <div v-if="loading && points.length === 0" class="empty-state">
        <div class="empty-sub">{{ t('restorePoints.loading', lang) }}</div>
      </div>
      <div v-else-if="points.length === 0" class="empty-state">
        <div class="empty-title">{{ t('restorePoints.emptyTitle', lang) }}</div>
        <div class="empty-sub">{{ t('restorePoints.empty', lang) }}</div>
      </div>
      <div v-else class="rp-table-wrap">
        <table class="rp-table">
          <thead>
            <tr>
              <th>{{ t('restorePoints.colTime', lang) }}</th>
              <th>{{ t('restorePoints.colAgent', lang) }}</th>
              <th>{{ t('restorePoints.colFormat', lang) }}</th>
              <th>{{ t('restorePoints.colOperation', lang) }}</th>
              <th>{{ t('restorePoints.colObject', lang) }}</th>
              <th>{{ t('restorePoints.colSize', lang) }}</th>
              <th>{{ t('restorePoints.colStatus', lang) }}</th>
              <th class="rp-th-action">{{ t('restorePoints.colAction', lang) }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="p in points" :key="p.id" :title="p.target_path">
              <td class="rp-time">
                {{ formatTime(p.created_at) }}
                <div v-if="p.note" class="rp-note">{{ p.note }}</div>
              </td>
              <td>{{ p.agent_label || p.agent_id || '—' }}</td>
              <td>
                <span class="format-badge" :class="`format-badge--${p.format || 'unknown'}`">{{ formatLabel(p.format) }}</span>
              </td>
              <td>{{ opLabel(p.operation) }}</td>
              <td class="rp-object">{{ objectLabel(p) }}</td>
              <td class="rp-size">{{ formatSize(p.size_bytes) }}</td>
              <td>
                <span v-if="!p.existed" class="rp-missing">{{ t('restorePoints.missing', lang) }}</span>
                <span v-else class="rp-exists">{{ t('restorePoints.stateExists', lang) }}</span>
              </td>
              <td>
                <div class="rp-actions">
                  <n-button size="tiny" @click="openRestore(p)">{{ t('restorePoints.restore', lang) }}</n-button>
                  <n-popconfirm @positive-click="onDelete(p)">
                    <template #trigger>
                      <n-button size="tiny" type="error" ghost>{{ t('restorePoints.delete', lang) }}</n-button>
                    </template>
                    {{ t('restorePoints.deleteConfirm', lang) }}
                  </n-popconfirm>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <n-modal v-model:show="confirmShow" preset="card" :title="t('restorePoints.confirmTitle', lang)" style="width: 480px" :mask-closable="false">
      <div v-if="pending" class="rp-confirm">
        <div class="rp-confirm-line rp-confirm-path">
          {{ t('restorePoints.confirmPath', lang).replace('{path}', pending.target_path) }}
        </div>
        <div v-if="!pending.existed" class="rp-confirm-line rp-confirm-warn">
          {{ t('restorePoints.confirmMissing', lang) }}
        </div>
        <div v-else-if="pathDiffers" class="rp-confirm-line rp-confirm-warn">
          {{ t('restorePoints.confirmOverwrite', lang) }}
        </div>
        <div class="rp-confirm-meta">
          {{ formatTime(pending.created_at) }} · {{ pending.agent_label || pending.agent_id }} ·
          {{ formatLabel(pending.format) }} · {{ opLabel(pending.operation) }}
        </div>
      </div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <n-button :disabled="restoring" @click="confirmShow = false">{{ t('common.cancel', lang) }}</n-button>
          <n-button type="primary" :loading="restoring" @click="doRestore">{{ t('restorePoints.confirmOk', lang) }}</n-button>
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="showExport" preset="card" :title="t('restorePoints.exportTitle', lang)" style="width: 520px" :mask-closable="false">
      <div class="bk-form">
        <label class="bk-label">{{ t('restorePoints.bkScope', lang) }}</label>
        <div class="bk-radios">
          <n-radio-group v-model:value="exportScope">
            <n-radio value="all">{{ t('restorePoints.bkScopeAll', lang) }}</n-radio>
            <n-radio value="some">{{ t('restorePoints.bkScopeSome', lang) }}</n-radio>
          </n-radio-group>
        </div>
        <div v-if="exportScope === 'some'" class="bk-list-box">
          <div v-if="exportProviders.length === 0" class="bk-hint">{{ t('restorePoints.bkNoProviders', lang) }}</div>
          <label v-for="p in exportProviders" :key="p.id" class="bk-check">
            <n-checkbox :checked="exportSelected.includes(p.id)" @update:checked="(v: boolean) => toggleExportProvider(p.id, v)" />
            <span class="bk-pname">{{ p.name || p.id }}</span>
            <span class="bk-psub">{{ p.id }}</span>
          </label>
        </div>
        <label class="bk-label">{{ t('restorePoints.bkKeys', lang) }}</label>
        <div class="bk-radios">
          <n-radio-group v-model:value="exportKeys">
            <n-radio value="no">{{ t('restorePoints.bkKeysNo', lang) }}</n-radio>
            <n-radio value="yes">{{ t('restorePoints.bkKeysYes', lang) }}</n-radio>
          </n-radio-group>
        </div>
        <label class="bk-label">{{ t('restorePoints.bkKeyMode', lang) }}</label>
        <div class="bk-radios">
          <n-radio-group v-model:value="exportMode">
            <n-radio value="plain">{{ t('restorePoints.bkModePlain', lang) }}</n-radio>
            <n-radio value="sealed">{{ t('restorePoints.bkModeSealed', lang) }}</n-radio>
            <n-radio value="password">{{ t('restorePoints.bkModePassword', lang) }}</n-radio>
          </n-radio-group>
        </div>
        <n-alert v-if="exportMode === 'plain'" :type="exportKeys === 'yes' ? 'warning' : 'info'" size="small" :show-icon="false">
          {{ exportKeys === 'yes' ? t('restorePoints.bkPlainWarn', lang) : t('restorePoints.bkPlainInfo', lang) }}
        </n-alert>
        <n-alert v-else-if="exportMode === 'sealed'" type="info" size="small" :show-icon="false">{{ t('restorePoints.bkSealedInfo', lang) }}</n-alert>
        <template v-else>
          <n-alert type="info" size="small" :show-icon="false">{{ t('restorePoints.bkPasswordInfo', lang) }}</n-alert>
          <div class="bk-pw">
            <n-input v-model:value="exportPassword" type="password" show-password-on="click" :placeholder="t('restorePoints.bkPassword', lang)" />
            <n-input v-model:value="exportPassword2" type="password" show-password-on="click" :placeholder="t('restorePoints.bkPasswordConfirm', lang)" />
          </div>
        </template>
        <div v-if="exportError" class="bk-error">{{ exportError }}</div>
      </div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <n-button :disabled="exporting" @click="showExport = false">{{ t('common.cancel', lang) }}</n-button>
          <n-button type="primary" :loading="exporting" :disabled="exportScope === 'some' && exportSelected.length === 0" @click="onExport">📤 {{ t('restorePoints.exportDo', lang) }}</n-button>
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="showKeysWarn" preset="card" :title="'⚠️ ' + t('restorePoints.bkKeysWarnTitle', lang)" style="width: 420px" :mask-closable="false">
      <div class="bk-warn-body">{{ t('restorePoints.bkKeysWarnBody', lang) }}</div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <n-button :disabled="exporting" @click="showKeysWarn = false">{{ t('common.cancel', lang) }}</n-button>
          <n-button type="error" :loading="exporting" @click="confirmPlainKeysExport">{{ t('restorePoints.bkKeysWarnConfirm', lang) }}</n-button>
        </div>
      </template>
    </n-modal>

    <n-modal v-model:show="showImport" preset="card" :title="t('restorePoints.importTitle', lang)" style="width: 520px" :mask-closable="false">
      <div class="bk-form">
        <div class="bk-path" :title="importPath">{{ importPath }}</div>
        <div v-if="inspecting" class="bk-hint">{{ t('restorePoints.inspecting', lang) }}</div>
        <template v-else-if="inspect && !inspect.needs_password">
          <div class="bk-info">
            <div class="bk-info-row"><span>{{ t('restorePoints.bkAgent', lang) }}</span><span>{{ inspect.agent_label || inspect.agent || '—' }}</span></div>
            <div class="bk-info-row"><span>{{ t('restorePoints.bkCreated', lang) }}</span><span>{{ formatTime(inspect.created_at || '') }}</span></div>
            <div class="bk-info-row"><span>{{ t('restorePoints.bkProviders', lang) }}</span><span>{{ inspect.provider_count ?? 0 }}</span></div>
            <div class="bk-info-row"><span>{{ t('restorePoints.bkKeyMode', lang) }}</span><span class="badge" :class="(inspect.include_keys && inspect.key_mode === 'plain') ? 'badge-warn' : 'badge-ok'">{{ keyModeLabel(inspect) }}</span></div>
          </div>
          <div class="bk-list-box">
            <div v-for="p in inspect.providers || []" :key="p.id" class="bk-provider">
              <span class="bk-pname">{{ p.name || p.id }}</span>
              <span class="bk-psub">{{ p.id }} · {{ t('restorePoints.bkModels', lang).replace('{n}', String(p.model_count)) }} · {{ p.has_api_key ? t('restorePoints.bkHasKey', lang) : t('restorePoints.bkNoKey', lang) }}</span>
            </div>
          </div>
          <label class="bk-label">{{ t('restorePoints.bkConflict', lang) }}</label>
          <div class="bk-radios">
            <n-radio-group v-model:value="conflict">
              <n-radio value="skip">{{ t('restorePoints.bkConflictSkip', lang) }}</n-radio>
              <n-radio value="overwrite">{{ t('restorePoints.bkConflictOverwrite', lang) }}</n-radio>
            </n-radio-group>
          </div>
        </template>
        <template v-else-if="inspect">
          <div class="bk-hint">{{ t('restorePoints.bkNeedsPassword', lang) }}</div>
          <div style="display: flex; gap: 8px">
            <n-input v-model:value="importPassword" type="password" show-password-on="click" :placeholder="t('restorePoints.bkPassword', lang)" style="flex:1" />
            <n-button :loading="inspecting" @click="runInspect(importPassword)">{{ t('restorePoints.unlock', lang) }}</n-button>
          </div>
        </template>
        <div v-if="importError" class="bk-error">{{ importError }}</div>
      </div>
      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px">
          <n-button :disabled="importing" @click="showImport = false">{{ t('common.cancel', lang) }}</n-button>
          <n-button type="primary" :loading="importing" :disabled="!inspect || inspect.needs_password || (inspect.provider_count ?? 0) === 0" @click="onImportBackup">{{ t('restorePoints.importDo', lang) }}</n-button>
        </div>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { NButton, NPopconfirm, NModal, NInput, NRadioGroup, NRadio, NCheckbox, NAlert } from 'naive-ui'
import { useSettingStore } from '../stores/setting'
import type { CompatIssue, RestorePoint, ProviderSummary, BackupInspectResult } from '../types'
import { t } from '../i18n'
import * as api from '../api'

const settingStore = useSettingStore()
const lang = computed(() => settingStore.resolvedLang)

const points = ref<RestorePoint[]>([])
const dir = ref('')
const currentTarget = ref('')
const loading = ref(false)
const pruning = ref(false)
const restoring = ref(false)
const pending = ref<RestorePoint | null>(null)
const confirmShow = ref(false)
const compatIssues = ref<CompatIssue[]>([])
const compatPath = ref('')
const fixing = ref(false)

// ---- 备份导出/导入 ----
const EXPORT_PREF_KEY = 'apm:backupExportPref'
const showExport = ref(false)
const exporting = ref(false)
const exportError = ref('')
const exportScope = ref<'all' | 'some'>('all')
const exportKeys = ref<'no' | 'yes'>('no')
const exportMode = ref<'plain' | 'sealed' | 'password'>('sealed')
const exportPassword = ref('')
const exportPassword2 = ref('')
const showKeysWarn = ref(false)
const exportProviders = ref<ProviderSummary[]>([])
const exportSelected = ref<string[]>([])
const openingBackup = ref(false)
const showImport = ref(false)
const importing = ref(false)
const inspecting = ref(false)
const importPath = ref('')
const inspect = ref<BackupInspectResult | null>(null)
const importPassword = ref('')
const importError = ref('')
const conflict = ref<'skip' | 'overwrite'>('skip')

const KNOWN_OPS = ['save_provider', 'delete_provider', 'delete_model', 'import_legacy', 'import_provider', 'migrate', 'restore', 'manual']

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

// ISO 时间本地化为 2026-09-26 11:30:05
function formatTime(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}

function formatLabel(format: string): string {
  if (format === 'v2') return t('restorePoints.formatV2', lang.value)
  if (format === 'legacy') return t('restorePoints.formatLegacy', lang.value)
  if (format === 'opencode') return t('restorePoints.formatOpencode', lang.value)
  if (format === 'deepseek') return t('restorePoints.formatDeepseek', lang.value)
  return t('restorePoints.formatUnknown', lang.value)
}

function opLabel(op: string): string {
  const code = (op || '').trim()
  if (!code) return t('restorePoints.op.manual', lang.value)
  if (KNOWN_OPS.includes(code)) return t(`restorePoints.op.${code}`, lang.value)
  return code
}

function objectLabel(p: RestorePoint): string {
  const parts = [p.provider_id, p.model_id].filter((v) => !!v)
  return parts.length > 0 ? parts.join(' / ') : '—'
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${i === 0 ? v : v.toFixed(1)} ${units[i]}`
}

function samePath(a: string, b: string): boolean {
  return a.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase() === b.replace(/\\/g, '/').replace(/\/+$/, '').toLowerCase()
}

// 与当前目标不一致（或回滚时被后端切了目标）都属于“会覆盖该路径上的现有文件”的情况
const pathDiffers = computed(() => {
  const p = pending.value
  if (!p) return false
  if (!currentTarget.value) return false
  return !samePath(p.target_path, currentTarget.value)
})

async function loadPoints() {
  loading.value = true
  try {
    const res = await api.GetRestorePoints()
    if (res.success) {
      points.value = res.points || []
      dir.value = res.dir || ''
      currentTarget.value = res.target || ''
    } else {
      toast('error', t('restorePoints.loadFailed', lang.value), res.error || '')
    }
  } catch (e) {
    toast('error', t('restorePoints.loadFailed', lang.value), String(e))
  } finally {
    loading.value = false
  }
}

// 兼容性检查只读且可能因文件不存在而无结果，失败时静默（不显示卡片）
async function loadCompat() {
  try {
    const res = await api.GetZCodeCompat()
    if (res && res.success) {
      compatIssues.value = res.issues || []
      compatPath.value = res.path || ''
    } else {
      compatIssues.value = []
      compatPath.value = ''
    }
  } catch {
    compatIssues.value = []
    compatPath.value = ''
  }
}

async function onOpenDir() {
  try {
    const res = await api.OpenRestorePointsDir()
    if (res.success) {
      if (res.dir) dir.value = res.dir
    } else {
      toast('error', t('restorePoints.openFailed', lang.value), res.error || '')
    }
  } catch (e) {
    toast('error', t('restorePoints.openFailed', lang.value), String(e))
  }
}

async function onPrune() {
  pruning.value = true
  try {
    const res = await api.PruneRestorePoints()
    if (!res.success) {
      toast('error', t('restorePoints.pruneFailed', lang.value), res.error || '')
      return
    }
    points.value = res.points || []
    const n = res.removed || 0
    if (n > 0) toast('success', t('restorePoints.pruneTitle', lang.value), t('restorePoints.pruneResult', lang.value).replace('{n}', String(n)))
    else toast('info', t('restorePoints.pruneTitle', lang.value), t('restorePoints.pruneNone', lang.value))
  } catch (e) {
    toast('error', t('restorePoints.pruneFailed', lang.value), String(e))
  } finally {
    pruning.value = false
  }
}

async function onDelete(p: RestorePoint) {
  try {
    const res = await api.DeleteRestorePoint(p.id)
    if (!res.success) {
      toast('error', t('restorePoints.deleteFailed', lang.value), res.error || '')
      return
    }
    toast('success', t('restorePoints.deleteTitle', lang.value), t('restorePoints.deleteSuccess', lang.value))
    await loadPoints()
  } catch (e) {
    toast('error', t('restorePoints.deleteFailed', lang.value), String(e))
  }
}

function openRestore(p: RestorePoint) {
  pending.value = p
  confirmShow.value = true
}

async function doRestore() {
  const p = pending.value
  if (!p) return
  restoring.value = true
  try {
    const res = await api.RestoreRestorePoint(p.id)
    if (!res.success) {
      toast('error', t('restorePoints.restoreFailed', lang.value), res.error || '')
      return
    }
    confirmShow.value = false
    const lines = [t('restorePoints.restoreBody', lang.value).replace('{time}', formatTime(p.created_at)).replace('{path}', res.target || p.target_path)]
    if (res.removed_path) lines.push(t('restorePoints.restoreRemoved', lang.value).replace('{path}', res.removed_path))
    if (res.snapshot && res.snapshot.id) lines.push(t('restorePoints.restoreSnapshot', lang.value).replace('{id}', res.snapshot.id))
    if (res.target_switched) lines.push(t('restorePoints.restoreSwitched', lang.value).replace('{path}', res.current_target || res.target || p.target_path))
    toast('success', t('restorePoints.restoreSuccess', lang.value), lines.join('\n'))
    if (res.warning) toast('warning', t('restorePoints.restoreWarning', lang.value), res.warning)
    pending.value = null
    await Promise.all([loadPoints(), loadCompat()])
  } catch (e) {
    toast('error', t('restorePoints.restoreFailed', lang.value), String(e))
  } finally {
    restoring.value = false
  }
}

async function onRepairCompat() {
  fixing.value = true
  try {
    const res = await api.RepairZCodeCompat()
    if (!res.success) {
      toast('error', t('restorePoints.compatFailed', lang.value), res.error || '')
      return
    }
    const fixes = res.fixes || []
    compatIssues.value = res.issues || []
    if (res.path) compatPath.value = res.path
    if (fixes.length > 0) {
      const detail = fixes.map((f) => `· ${f}`).join('\n')
      toast('success', t('restorePoints.compatFixedTitle', lang.value),
        `${t('restorePoints.compatFixed', lang.value).replace('{n}', String(fixes.length))}\n${detail}`)
    } else {
      toast('info', t('restorePoints.compatNoIssues', lang.value))
    }
    if (compatIssues.value.length > 0) {
      toast('warning', t('restorePoints.compatFixedTitle', lang.value),
        t('restorePoints.compatRemaining', lang.value).replace('{n}', String(compatIssues.value.length)))
    }
    await Promise.all([loadCompat(), loadPoints()])
  } catch (e) {
    toast('error', t('restorePoints.compatFailed', lang.value), String(e))
  } finally {
    fixing.value = false
  }
}

// ---- 备份导出 ----
function backupStamp(): string {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}${p(d.getMonth() + 1)}${p(d.getDate())}-${p(d.getHours())}${p(d.getMinutes())}`
}
function basename(p: string): string {
  return p.split(/[\\/]/).pop() || p
}

function openExport() {
  exportError.value = ''
  try {
    const raw = localStorage.getItem(EXPORT_PREF_KEY)
    if (raw) {
      const pref = JSON.parse(raw) as { scope?: string; keys?: string; mode?: string }
      if (pref.scope === 'all' || pref.scope === 'some') exportScope.value = pref.scope
      if (pref.keys === 'no' || pref.keys === 'yes') exportKeys.value = pref.keys
      if (pref.mode === 'dpapi') exportMode.value = 'sealed' // 旧版「本机加密」偏好迁移
      else if (pref.mode === 'plain' || pref.mode === 'sealed' || pref.mode === 'password') exportMode.value = pref.mode
    }
  } catch { /* 预设损坏则用默认值 */ }
  exportSelected.value = []
  exportPassword.value = ''
  exportPassword2.value = ''
  showExport.value = true
  void api.ListProviders().then((res) => {
    exportProviders.value = res['success'] ? ((res['providers'] as ProviderSummary[]) || []) : []
  }).catch(() => { exportProviders.value = [] })
}

function toggleExportProvider(id: string, v: boolean) {
  if (v) {
    if (!exportSelected.value.includes(id)) exportSelected.value = [...exportSelected.value, id]
  } else {
    exportSelected.value = exportSelected.value.filter((x) => x !== id)
  }
}

async function onExport() {
  exportError.value = ''
  if (exportMode.value === 'password') {
    if (!exportPassword.value) { exportError.value = t('restorePoints.bkPasswordRequired', lang.value); return }
    if (exportPassword.value !== exportPassword2.value) { exportError.value = t('restorePoints.bkPasswordMismatch', lang.value); return }
  }
  if (exportKeys.value === 'yes' && exportMode.value === 'plain') {
    showKeysWarn.value = true // 明文含密钥是不可逆的泄露面，先二次确认
    return
  }
  await runExport()
}

async function confirmPlainKeysExport() {
  showKeysWarn.value = false
  await runExport()
}

async function runExport() {
  exporting.value = true
  try {
    const pick = await api.ChooseBackupSavePath(`apm-backup-${settingStore.agent}-${backupStamp()}.json`)
    if (!pick.success) {
      // 原生保存对话框取消时后端也返回 success:false（error: 已取消），不当作错误提示
      if (pick.error && pick.error !== '已取消') toast('error', t('restorePoints.exportFailed', lang.value), pick.error)
      return
    }
    const ids = exportScope.value === 'all' ? exportProviders.value.map((p) => p.id) : [...exportSelected.value]
    const res = await api.ExportBackup({
      path: pick.path || '',
      provider_ids: ids,
      include_keys: exportKeys.value === 'yes',
      key_mode: exportMode.value,
      password: exportMode.value === 'password' ? exportPassword.value : '',
    })
    if (!res.success) {
      exportError.value = res.error || ''
      toast('error', t('restorePoints.exportFailed', lang.value), res.error || '')
      return
    }
    try { localStorage.setItem(EXPORT_PREF_KEY, JSON.stringify({ scope: exportScope.value, keys: exportKeys.value, mode: exportMode.value })) } catch { /* ignore */ }
    toast('success', t('restorePoints.exportTitle', lang.value),
      t('restorePoints.exportResult', lang.value).replace('{n}', String(res.provider_count ?? ids.length)).replace('{path}', basename(pick.path || '')))
    showExport.value = false
    exportPassword.value = ''
    exportPassword2.value = ''
  } catch (e) {
    exportError.value = String(e)
    toast('error', t('restorePoints.exportFailed', lang.value), String(e))
  } finally { exporting.value = false }
}

// ---- 备份导入 ----
function keyModeLabel(res: BackupInspectResult): string {
  const mode = res.key_mode === 'plain' ? t('restorePoints.bkModePlain', lang.value)
    : res.key_mode === 'sealed' ? t('restorePoints.bkModeSealed', lang.value)
    : res.key_mode === 'password' ? t('restorePoints.bkModePassword', lang.value)
    : (res.key_mode || '—')
  return `${mode} · ${res.include_keys ? t('restorePoints.bkHasKey', lang.value) : t('restorePoints.bkNoKey', lang.value)}`
}

async function openImport() {
  openingBackup.value = true
  try {
    const res = await api.OpenBackupFile()
    if (!res.success) {
      if (res.error && res.error !== '已取消') toast('error', t('restorePoints.importTitle', lang.value), res.error)
      return
    }
    importPath.value = res.path || ''
    inspect.value = null
    importPassword.value = ''
    importError.value = ''
    conflict.value = 'skip'
    showImport.value = true
    await runInspect('')
  } catch (e) {
    toast('error', t('restorePoints.importTitle', lang.value), String(e))
  } finally { openingBackup.value = false }
}

async function runInspect(password: string) {
  inspecting.value = true
  importError.value = ''
  try {
    const res = await api.InspectBackup(importPath.value, password)
    if (!res.success) {
      // 「需要密码/密码错误」响应走 success:false + needs_password:true，保留密码面板
      if (res.needs_password) {
        inspect.value = res
        importError.value = password ? res.error || '' : ''
        return
      }
      importError.value = res.error || t('restorePoints.importFailed', lang.value)
      inspect.value = null
      return
    }
    inspect.value = res
  } finally { inspecting.value = false }
}

async function onImportBackup() {
  if (!inspect.value) return
  importError.value = ''
  importing.value = true
  try {
    const res = await api.ImportBackup(importPath.value, importPassword.value, conflict.value)
    if (!res.success) {
      importError.value = res.error || ''
      toast('error', t('restorePoints.importFailed', lang.value), res.error || '')
      return
    }
    toast('success', t('restorePoints.importTitle', lang.value),
      t('restorePoints.importResult', lang.value).replace('{imported}', String((res.imported || []).length)).replace('{skipped}', String((res.skipped || []).length)))
    const failed = res.failed || []
    if (failed.length > 0) importError.value = t('restorePoints.importPartial', lang.value) + '\n' + failed.map((f) => `· ${f.id}：${f.error}`).join('\n')
    else showImport.value = false
    // 导入会创建新的还原点，刷新列表
    await loadPoints()
  } catch (e) {
    importError.value = String(e)
    toast('error', t('restorePoints.importFailed', lang.value), String(e))
  } finally { importing.value = false }
}

onMounted(async () => {
  await Promise.all([loadPoints(), loadCompat()])
})
</script>

<style scoped>
.restore-page { display: flex; flex-direction: column; gap: 12px; }
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: var(--fluent-shadow-md); border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 12px; }
.rp-desc { font-size: 11px; color: var(--fluent-text-soft); }
.rp-toolbar { display: flex; align-items: center; gap: 8px; margin-top: 10px; flex-wrap: wrap; }
.rp-dir-label { font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; }
.rp-dir { font-size: 11px; color: var(--fluent-text); word-break: break-all; min-width: 0; }
.rp-count { font-size: 12px; color: var(--fluent-text-soft); }
.list-wrap { display: flex; flex-direction: column; gap: 8px; }
.list-header { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; }
.list-header h3 { font-size: 14px; font-weight: 600; }
.count-label { font-size: 12px; color: var(--fluent-text-soft); }
.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 32px; color: var(--fluent-text-soft); text-align: center; }
.empty-title { font-size: 14px; font-weight: 600; color: var(--fluent-text); }
.empty-sub { font-size: 12px; }
.rp-table-wrap { overflow-x: auto; }
.rp-table { width: 100%; border-collapse: collapse; font-size: 12px; }
.rp-table th {
  text-align: left; font-size: 11px; font-weight: 600; color: var(--fluent-text-soft);
  padding: 6px 10px; border-bottom: 1px solid var(--fluent-border); white-space: nowrap;
}
.rp-table td { padding: 8px 10px; border-bottom: 1px solid var(--fluent-border); vertical-align: middle; }
.rp-table tbody tr:last-child td { border-bottom: none; }
.rp-table tbody tr:hover { background: color-mix(in srgb, var(--accent, #0078d4) 6%, transparent); }
.rp-th-action { text-align: right; }
.rp-time { white-space: nowrap; }
.rp-note { font-size: 10px; color: var(--fluent-text-soft); margin-top: 2px; max-width: 220px; }
.rp-object { word-break: break-all; max-width: 200px; }
.rp-size { white-space: nowrap; }
.rp-missing { color: #ca5010; font-size: 11px; white-space: nowrap; }
.rp-exists { color: var(--fluent-text-soft); font-size: 11px; white-space: nowrap; }
.rp-actions { display: flex; gap: 6px; justify-content: flex-end; }
.format-badge { font-size: 11px; font-weight: 600; border-radius: 4px; padding: 2px 8px; border: 1px solid var(--fluent-border); background: var(--fluent-bg); white-space: nowrap; }
.format-badge--v2 { color: #107c10; border-color: color-mix(in srgb, #107c10 40%, transparent); background: color-mix(in srgb, #107c10 10%, transparent); }
.format-badge--legacy { color: #ca5010; border-color: color-mix(in srgb, #ca5010 40%, transparent); background: color-mix(in srgb, #ca5010 10%, transparent); }
.format-badge--opencode { color: #0078d4; border-color: color-mix(in srgb, #0078d4 40%, transparent); background: color-mix(in srgb, #0078d4 10%, transparent); }
.format-badge--deepseek { color: #8a5cf6; border-color: color-mix(in srgb, #8a5cf6 40%, transparent); background: color-mix(in srgb, #8a5cf6 10%, transparent); }
.format-badge--unknown { color: var(--fluent-text-soft); }
.compat-card {
  display: flex; flex-direction: column; gap: 6px;
  border-color: color-mix(in srgb, #ca5010 40%, transparent);
  background: color-mix(in srgb, #ca5010 6%, var(--fluent-card-bg));
}
.compat-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.compat-title { margin-bottom: 0; }
.compat-icon { color: #ca5010; font-size: 15px; }
.compat-desc { font-size: 11px; color: var(--fluent-text-soft); }
.compat-path { font-size: 11px; color: var(--fluent-text); word-break: break-all; }
.compat-list { display: flex; flex-direction: column; gap: 6px; margin-top: 4px; }
.compat-item { border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; background: var(--fluent-card-bg); }
.compat-msg { font-size: 12px; line-height: 1.5; }
.compat-issue-path { font-size: 11px; color: var(--fluent-text-soft); word-break: break-all; margin-top: 2px; }
.rp-confirm { display: flex; flex-direction: column; gap: 8px; font-size: 13px; line-height: 1.7; }
.rp-confirm-path { word-break: break-all; }
.rp-confirm-warn { font-size: 12px; color: #ca5010; }
.rp-confirm-meta { font-size: 11px; color: var(--fluent-text-soft); }
.badge { font-size: 10px; border: 1px solid var(--fluent-border); border-radius: 999px; padding: 0 6px; line-height: 16px; white-space: nowrap; }
.badge-ok { color: #107c10; border-color: #107c10; background: rgba(16,124,16,0.08); }
.badge-warn { color: #ca5010; border-color: #ca5010; background: rgba(202,80,16,0.08); }
.bk-form { display: flex; flex-direction: column; gap: 10px; font-size: 12px; }
.bk-label { font-size: 12px; color: var(--fluent-text-soft); }
.bk-hint { font-size: 12px; color: var(--fluent-text-soft); }
.bk-path { font-size: 11px; color: var(--fluent-text-soft); word-break: break-all; }
.bk-radios :deep(.n-radio-group) { display: flex; flex-direction: column; gap: 4px; }
.bk-list-box { display: flex; flex-direction: column; gap: 6px; max-height: 180px; overflow-y: auto; border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 10px; background: var(--fluent-bg); }
.bk-check { display: flex; gap: 8px; align-items: center; cursor: pointer; min-width: 0; }
.bk-provider { display: flex; gap: 12px; align-items: baseline; justify-content: space-between; min-width: 0; }
.bk-pname { font-size: 12px; font-weight: 600; word-break: break-all; }
.bk-psub { font-size: 11px; color: var(--fluent-text-soft); word-break: break-all; text-align: right; }
.bk-info { display: flex; flex-direction: column; gap: 4px; border: 1px solid var(--fluent-border); border-radius: 8px; padding: 8px 12px; background: var(--fluent-bg); }
.bk-info-row { display: flex; gap: 12px; align-items: center; justify-content: space-between; }
.bk-info-row > span:first-child { font-size: 11px; color: var(--fluent-text-soft); white-space: nowrap; }
.bk-pw { display: flex; flex-direction: column; gap: 8px; }
.bk-warn-body { font-size: 13px; line-height: 1.6; color: var(--fluent-text); }
.bk-error { color: #d13438; font-size: 12px; white-space: pre-line; }
</style>
