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
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { NButton, NPopconfirm, NModal } from 'naive-ui'
import { useSettingStore } from '../stores/setting'
import type { CompatIssue, RestorePoint } from '../types'
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
</style>
