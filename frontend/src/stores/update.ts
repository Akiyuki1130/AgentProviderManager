import { defineStore } from 'pinia'
import { ref } from 'vue'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import * as api from '../api'

export interface UpdateStatus {
  currentVersion: string
  latestVersion: string
  available: boolean
  checking: boolean
  downloading: boolean
  downloadProgress: number
  ready: boolean
  error: string
}

type UpdatePayload = Record<string, unknown>

function isRecord(value: unknown): value is UpdatePayload {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function firstValue(payload: UpdatePayload, ...keys: string[]): unknown {
  for (const key of keys) {
    if (key in payload) return payload[key]
  }
  return undefined
}

function asString(value: unknown): string | undefined {
  return typeof value === 'string' ? value : undefined
}

function asBoolean(value: unknown): boolean | undefined {
  return typeof value === 'boolean' ? value : undefined
}

function asProgress(value: unknown): number | undefined {
  const numberValue = typeof value === 'number'
    ? value
    : typeof value === 'string' && value.trim() !== ''
      ? Number(value)
      : NaN
  if (!Number.isFinite(numberValue)) return undefined
  return Math.max(0, Math.min(100, numberValue))
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

export const useUpdateStore = defineStore('update', () => {
  const currentVersion = ref('')
  const latestVersion = ref('')
  const available = ref(false)
  const checking = ref(false)
  const downloading = ref(false)
  const downloadProgress = ref(0)
  const ready = ref(false)
  const error = ref('')

  function applyStatus(raw: unknown) {
    const payload = isRecord(raw) ? raw : {}
    const current = asString(firstValue(payload, 'currentVersion', 'current_version', 'version'))
    const latest = asString(firstValue(payload, 'latestVersion', 'latest_version', 'new_version'))
    const isAvailable = asBoolean(firstValue(payload, 'available', 'update_available'))
    const isChecking = asBoolean(firstValue(payload, 'checking'))
    const isDownloading = asBoolean(firstValue(payload, 'downloading'))
    const progress = asProgress(firstValue(payload, 'downloadProgress', 'download_progress', 'progress', 'percent'))
    const isReady = asBoolean(firstValue(payload, 'ready', 'downloaded', 'installed'))
    const statusError = asString(firstValue(payload, 'error', 'message'))

    if (current !== undefined) currentVersion.value = current
    if (latest !== undefined) latestVersion.value = latest
    if (isAvailable !== undefined) available.value = isAvailable
    if (isChecking !== undefined) checking.value = isChecking
    if (isDownloading !== undefined) downloading.value = isDownloading
    if (progress !== undefined) downloadProgress.value = progress
    if (isReady !== undefined) ready.value = isReady
    if (statusError !== undefined) error.value = statusError
  }

  function onProgress(...data: unknown[]) {
    const payload = isRecord(data[0]) ? data[0] : { progress: data[0] }
    const progress = asProgress(firstValue(payload, 'downloadProgress', 'download_progress', 'progress', 'percent'))
    if (progress !== undefined) downloadProgress.value = progress

    const isReady = asBoolean(firstValue(payload, 'ready', 'downloaded'))
    if (isReady !== undefined) ready.value = isReady

    const isDownloading = asBoolean(firstValue(payload, 'downloading'))
    if (isDownloading !== undefined) {
      downloading.value = isDownloading
    } else if (!ready.value) {
      downloading.value = progress === undefined || progress < 100
    }

    const statusError = asString(firstValue(payload, 'error', 'message'))
    if (statusError !== undefined) error.value = statusError
  }

  // EventsOn is kept optional at runtime so the store remains usable in a
  // browser/type-check environment where Wails has not injected its runtime.
  let stopProgressListener: (() => void) | undefined
  function listenForProgress() {
    if (stopProgressListener || typeof window === 'undefined') return
    try {
      stopProgressListener = EventsOn('update:progress', onProgress)
    } catch {
      // The Wails runtime is unavailable outside the desktop application.
    }
  }
  listenForProgress()

  async function check(manual = false): Promise<void> {
    if (checking.value) return
    checking.value = true
    error.value = ''
    try {
      const result = await api.CheckForUpdate()
      applyStatus(result)
    } catch (cause) {
      // Manual checks surface errors; background callers can retry quietly.
      if (manual) error.value = errorMessage(cause)
    } finally {
      checking.value = false
    }
  }

  async function download(): Promise<void> {
    if (downloading.value || ready.value) return
    downloading.value = true
    downloadProgress.value = 0
    error.value = ''
    try {
      const result = await api.DownloadUpdate()
      applyStatus(result)
      if (!ready.value && downloadProgress.value >= 100) {
        downloading.value = false
      }
    } catch (cause) {
      downloading.value = false
      error.value = errorMessage(cause)
    }
  }

  async function install(): Promise<void> {
    error.value = ''
    try {
      const result = await api.InstallUpdate()
      applyStatus(result)
      // Installation is explicit; the helper replaces the executable and restarts it.
    } catch (cause) {
      error.value = errorMessage(cause)
    }
  }

  async function refresh(): Promise<void> {
    try {
      const result = await api.GetUpdateStatus()
      applyStatus(result)
    } catch (cause) {
      error.value = errorMessage(cause)
    }
  }

  return {
    currentVersion,
    latestVersion,
    available,
    checking,
    downloading,
    downloadProgress,
    ready,
    error,
    check,
    download,
    install,
    refresh,
  }
})
