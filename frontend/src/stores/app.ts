import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ProviderSummary, ZCodeConfigFormat } from '../types'
import * as api from '../api'

export const useAppStore = defineStore('app', () => {
  const providers = ref<ProviderSummary[]>([])
  const targetPath = ref('')
  const backups = ref<string[]>([])
  const latestBackup = ref('')
  const loading = ref(false)

  // ZCode 配置格式检测（v2 / legacy / unknown）与旧供应商导入预览
  const zcodeFormat = ref<ZCodeConfigFormat>('unknown')
  const zcodeFormatPath = ref('')
  const zcodeFormatError = ref('')
  const supportsLegacyImport = ref(false)
  const legacyProviders = ref<ProviderSummary[]>([])
  const legacyDropped = ref<Record<string, string[]>>({})
  const legacyPreviewLoading = ref(false)

  async function fetchProviders() {
    const res = await api.ListProviders() as Record<string, unknown>
    if (res['success']) {
      providers.value = (res['providers'] as ProviderSummary[]) || []
      targetPath.value = (res['target'] as string) || targetPath.value
    }
  }

  async function fetchTargetInfo() {
    try {
      const info = await api.GetTargetConfig() as Record<string, unknown>
      targetPath.value = (info['path'] as string) || ''
      backups.value = (info['backups'] as string[]) || []
      latestBackup.value = (info['latest_backup'] as string) || ''
    } catch { /* ignore */ }
  }

  async function fetchZCodeFormat() {
    try {
      const res = await api.GetZCodeFormat()
      zcodeFormat.value = res && (res.format === 'v2' || res.format === 'legacy') ? res.format : 'unknown'
      zcodeFormatPath.value = res && typeof res.path === 'string' ? res.path : ''
      supportsLegacyImport.value = !!res && res.success === true && res.supports_legacy_import === true
      zcodeFormatError.value = res && res.success === true ? '' : ((res && res.error) || '')
    } catch (e) {
      // 生成的 Wails 绑定可能尚未就绪：按 unknown 处理，不阻塞页面。
      zcodeFormat.value = 'unknown'
      zcodeFormatPath.value = ''
      supportsLegacyImport.value = false
      zcodeFormatError.value = String(e)
    }
  }

  async function fetchLegacyPreview() {
    legacyPreviewLoading.value = true
    try {
      const res = await api.PreviewLegacyImport()
      if (res && res.success) {
        legacyProviders.value = res.providers || []
        legacyDropped.value = res.dropped || {}
      } else {
        legacyProviders.value = []
        legacyDropped.value = {}
      }
    } catch {
      legacyProviders.value = []
      legacyDropped.value = {}
    } finally {
      legacyPreviewLoading.value = false
    }
  }

  return {
    providers, targetPath, backups, latestBackup, loading, fetchProviders, fetchTargetInfo,
    zcodeFormat, zcodeFormatPath, zcodeFormatError, supportsLegacyImport,
    legacyProviders, legacyDropped, legacyPreviewLoading, fetchZCodeFormat, fetchLegacyPreview,
  }
})
