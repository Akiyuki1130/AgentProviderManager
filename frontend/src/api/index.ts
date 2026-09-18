import type { ZCodeFormatInfo, LegacyImportPreview, LegacyImportResult } from '../types'

type WailsApp = Record<string, (...args: unknown[]) => Promise<unknown>>

function getApp(): WailsApp | null {
  const w = window as unknown as Record<string, unknown>
  const go = w['go'] as Record<string, unknown> | undefined
  if (go) {
    for (const key of Object.keys(go)) {
      const mod = go[key] as Record<string, unknown>
      if (mod && typeof mod === 'object' && 'App' in mod) {
        return (mod as Record<string, unknown>)['App'] as WailsApp
      }
    }
  }
  return null
}

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const app = getApp()
  if (app && typeof app[method] === 'function') {
    return (await app[method](...args)) as T
  }
  throw new Error(`Wails binding not ready: ${method}`)
}

export async function GetAppInfo(): Promise<Record<string, unknown>> {
  return call('GetAppInfo')
}
export async function GetTargetConfig(): Promise<Record<string, unknown>> {
  return call('GetTargetConfig')
}
export async function ListConfigLocations(): Promise<Record<string, unknown>> {
  return call('ListConfigLocations')
}
export async function ChooseConfigFile(): Promise<Record<string, unknown>> {
  return call('ChooseConfigFile')
}
export async function OpenConfigDir(): Promise<Record<string, unknown>> {
  return call('OpenConfigDir')
}
export async function SetConfigPath(path: string): Promise<Record<string, unknown>> {
  return call('SetConfigPath', path)
}
export async function GetBackupInfo(): Promise<Record<string, unknown>> {
  return call('GetBackupInfo')
}
export async function RestoreLastBackup(backup: string, target: string): Promise<Record<string, unknown>> {
  return call('RestoreLastBackup', backup, target)
}
export async function GetTheme(): Promise<string | null> {
  try { return (await call('GetTheme')) as string | null } catch { return null }
}
export async function SetTheme(theme: string): Promise<Record<string, unknown>> {
  return call('SetTheme', theme)
}
export async function GetLanguage(): Promise<string | null> {
  try { return (await call('GetLanguage')) as string | null } catch { return null }
}
export async function SetLanguage(lang: string): Promise<Record<string, unknown>> {
  return call('SetLanguage', lang)
}
export async function GetOptions(): Promise<Record<string, unknown>> {
  return call('GetOptions')
}
export async function GetAccent(): Promise<string | null> {
  try { return (await call('GetAccent')) as string | null } catch { return null }
}
export async function SetAccent(accent: string): Promise<Record<string, unknown>> {
  return call('SetAccent', accent)
}
export async function GetHttpEnabled(): Promise<boolean> {
  try { return (await call('GetHttpEnabled')) as boolean } catch { return false }
}
export async function SetHttpEnabled(v: boolean): Promise<Record<string, unknown>> {
  return call('SetHttpEnabled', v)
}
export async function GetPrivateHostsAllowed(): Promise<boolean> {
  try { return (await call('GetPrivateHostsAllowed')) as boolean } catch { return false }
}
export async function SetPrivateHostsAllowed(v: boolean): Promise<Record<string, unknown>> {
  return call('SetPrivateHostsAllowed', v)
}
export async function GetAutoFillLimits(): Promise<boolean> {
  try { return (await call('GetAutoFillLimits')) as boolean } catch { return true }
}
export async function SetAutoFillLimits(v: boolean): Promise<Record<string, unknown>> {
  return call('SetAutoFillLimits', v)
}
export async function ListAgents(): Promise<Record<string, unknown>> {
  return call('ListAgents')
}
export async function GetCurrentAgent(): Promise<Record<string, unknown>> {
  return call('GetCurrentAgent')
}
export async function SetCurrentAgent(agent: string): Promise<Record<string, unknown>> {
  return call('SetCurrentAgent', agent)
}
export async function GuessProviderID(baseUrl: string): Promise<string> {
  try { return (await call('GuessProviderID', baseUrl)) as string } catch { return '' }
}
export async function GuessKind(baseUrl: string): Promise<string> {
  try { return (await call('GuessKind', baseUrl)) as string } catch { return 'openai-compatible' }
}
export async function NewProviderID(): Promise<string> {
  return call('NewProviderID') as Promise<string>
}
export async function ListProviders(): Promise<Record<string, unknown>> {
  return call('ListProviders')
}
export async function GetProvider(id: string): Promise<Record<string, unknown>> {
  return call('GetProvider', id)
}
export async function SaveProvider(id: string, provider: unknown): Promise<Record<string, unknown>> {
  return call('SaveProvider', id, provider)
}
export async function DeleteProvider(id: string): Promise<Record<string, unknown>> {
  return call('DeleteProvider', id)
}
export async function DeleteModel(providerId: string, modelId: string): Promise<Record<string, unknown>> {
  return call('DeleteModel', providerId, modelId)
}
export async function FetchModels(baseUrl: string, apiKey: string): Promise<Record<string, unknown>> {
  return call('FetchModels', baseUrl, apiKey)
}
export async function RefreshProviderModels(providerId: string, baseUrl: string, apiKey: string): Promise<Record<string, unknown>> {
  return call('RefreshProviderModels', providerId, baseUrl, apiKey)
}
export async function BuildSingleCard(modelId: string): Promise<Record<string, unknown>> {
  return call('BuildSingleCard', modelId)
}
export async function ApplyModelPresets(cards: unknown): Promise<Record<string, unknown>> {
  return call('ApplyModelPresets', cards)
}
export async function ImportProvider(payload: unknown): Promise<Record<string, unknown>> {
  return call('ImportProvider', payload)
}
export async function ListKeychain(): Promise<Record<string, unknown>> {
  return call('ListKeychain')
}
export async function GetKeychainEntry(id: string): Promise<Record<string, unknown>> {
  return call('GetKeychainEntry', id)
}
export async function SaveKeychainEntry(entry: unknown): Promise<Record<string, unknown>> {
  return call('SaveKeychainEntry', entry)
}
export async function DeleteKeychainEntries(ids: string[]): Promise<Record<string, unknown>> {
  return call('DeleteKeychainEntries', ids)
}
export async function MigratePreview(source: string, target: string): Promise<Record<string, unknown>> {
  return call('MigratePreview', source, target)
}
export async function MigrateExecute(payload: unknown): Promise<Record<string, unknown>> {
  return call('MigrateExecute', payload)
}

// Update methods intentionally go through the dynamic Wails bridge above. This
// keeps the frontend type-checkable while the generated bindings catch up with
// the backend methods.
export async function CheckForUpdate(): Promise<Record<string, unknown>> {
  return call('CheckForUpdate')
}
export async function DownloadUpdate(): Promise<Record<string, unknown>> {
  return call('DownloadUpdate')
}
export async function InstallUpdate(): Promise<Record<string, unknown>> {
  return call('InstallUpdate')
}
export async function GetUpdateStatus(): Promise<Record<string, unknown>> {
  return call('GetUpdateStatus')
}

// ZCode v2 / legacy format detection and the legacy-provider import bridge also
// go through the dynamic Wails bridge above, for the same reason: the frontend
// stays type-checkable while the generated bindings catch up with the backend.
export async function GetZCodeFormat(): Promise<ZCodeFormatInfo> {
  return call<ZCodeFormatInfo>('GetZCodeFormat')
}
export async function PreviewLegacyImport(): Promise<LegacyImportPreview> {
  return call<LegacyImportPreview>('PreviewLegacyImport')
}
export async function ApplyLegacyImport(ids: string[]): Promise<LegacyImportResult> {
  return call<LegacyImportResult>('ApplyLegacyImport', ids)
}
