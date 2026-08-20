import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ProviderSummary } from '../types'
import * as api from '../api'

export const useAppStore = defineStore('app', () => {
  const providers = ref<ProviderSummary[]>([])
  const targetPath = ref('')
  const backups = ref<string[]>([])
  const latestBackup = ref('')
  const loading = ref(false)

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

  return { providers, targetPath, backups, latestBackup, loading, fetchProviders, fetchTargetInfo }
})
