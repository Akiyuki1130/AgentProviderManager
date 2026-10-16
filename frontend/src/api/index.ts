import type {
  ZCodeFormatInfo,
  LegacyImportPreview,
  LegacyImportResult,
  RestorePointListResult,
  RestorePointRestoreResult,
  RestorePointDeleteResult,
  RestorePointPruneResult,
  RestorePointsDirResult,
  ZCodeCompatResult,
  ZCodeCompatRepairResult,
  ProbeResult,
  KeychainLinksResult,
  SimpleResult,
  PathPickResult,
  BackupExportOptions,
  ExportBackupResult,
  BackupInspectResult,
  ImportBackupResult,
} from '../types'

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
// C2：「允许自填推理档位」开关有后端持久化（后端据此放宽档位名校验），
// 与纯客户端的 reasoningAllIntensities 不同，故照抄 autoFillLimits 的写法与错误归一。
export async function GetCustomVariantsEnabled(): Promise<boolean> {
  try { return (await call('GetCustomVariantsEnabled')) as boolean } catch { return false }
}
export async function SetCustomVariantsEnabled(v: boolean): Promise<Record<string, unknown>> {
  return call('SetCustomVariantsEnabled', v)
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
// 站点级「自动匹配」：除预设表外，后端还会用离线目录与站点级能力规则取值，因此要带上
// 当前表单的 Base URL 与协议类型。
//
// apiType 归一：后端站点规则里的 apiTypeMatch 用的是 ZCode 的 api.type
// （anthropic-messages / openai-chat-completions / openai-responses），而表单里给的是
// UI kind（anthropic / responses / openai-compatible），在这里做最小映射；
// 已经是 api.type 的值原样透传。空 kind 按 openai-chat-completions 处理（与
// internal/zcodeprovider/convert.go 的 apiTypeFromLegacyKind 一致）；无法识别的**非空**
// kind 在后端会直接报错，这里静默按 chat-completions 处理，但两个调用页面的 kind 下拉
// 只有上面这几个值，该分支在实际 UI 路径上不可达。
function normalizeApiType(value: string): string {
  switch ((value || '').trim().toLowerCase()) {
    case 'anthropic':
      return 'anthropic-messages'
    case 'responses':
    case 'openai':
      return 'openai-responses'
    case 'openai-compatible':
      return 'openai-chat-completions'
    case 'anthropic-messages':
    case 'openai-chat-completions':
    case 'openai-responses':
      return (value || '').trim().toLowerCase()
    default:
      return 'openai-chat-completions'
  }
}
export async function ApplyModelPresetsForSite(cards: unknown, baseUrl: string, apiType: string): Promise<Record<string, unknown>> {
  return call('ApplyModelPresetsForSite', cards, baseUrl || '', normalizeApiType(apiType))
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
/** 界面上确认安装后先领一次性令牌，InstallUpdate 校验令牌通过才会替换并重启。 */
export async function PrepareUpdateInstall(): Promise<Record<string, unknown>> {
  return call('PrepareUpdateInstall')
}
export async function InstallUpdate(token: string): Promise<Record<string, unknown>> {
  return call('InstallUpdate', token)
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

// 还原点（配置修改前的自动快照）与 ZCode 兼容性检查/一键修复同样走上面的动态 Wails 桥。
export async function GetRestorePoints(): Promise<RestorePointListResult> {
  return call<RestorePointListResult>('GetRestorePoints')
}
export async function RestoreRestorePoint(id: string): Promise<RestorePointRestoreResult> {
  return call<RestorePointRestoreResult>('RestoreRestorePoint', id)
}
export async function DeleteRestorePoint(id: string): Promise<RestorePointDeleteResult> {
  return call<RestorePointDeleteResult>('DeleteRestorePoint', id)
}
export async function PruneRestorePoints(): Promise<RestorePointPruneResult> {
  return call<RestorePointPruneResult>('PruneRestorePoints')
}
export async function OpenRestorePointsDir(): Promise<RestorePointsDirResult> {
  return call<RestorePointsDirResult>('OpenRestorePointsDir')
}
export async function GetZCodeCompat(): Promise<ZCodeCompatResult> {
  return call<ZCodeCompatResult>('GetZCodeCompat')
}
export async function RepairZCodeCompat(): Promise<ZCodeCompatRepairResult> {
  return call<ZCodeCompatRepairResult>('RepairZCodeCompat')
}

// 连接探测、钥匙串关联与跨 Agent 备份导出/导入同样走上面的动态 Wails 桥。
export async function TestConnectionRaw(baseURL: string, apiKey: string, kind: string): Promise<ProbeResult> {
  return call<ProbeResult>('TestConnectionRaw', baseURL, apiKey, kind)
}
export async function TestProviderConnection(providerID: string, baseURLOverride: string, apiKeyOverride: string): Promise<ProbeResult> {
  return call<ProbeResult>('TestProviderConnection', providerID, baseURLOverride, apiKeyOverride)
}
export async function GetKeychainLinks(): Promise<KeychainLinksResult> {
  return call<KeychainLinksResult>('GetKeychainLinks')
}
export async function SetKeychainLink(providerID: string, entryID: string): Promise<SimpleResult> {
  return call<SimpleResult>('SetKeychainLink', providerID, entryID)
}
export async function RemoveKeychainLink(providerID: string): Promise<SimpleResult> {
  return call<SimpleResult>('RemoveKeychainLink', providerID)
}
export async function ChooseBackupSavePath(suggestedName: string): Promise<PathPickResult> {
  return call<PathPickResult>('ChooseBackupSavePath', suggestedName)
}
export async function ExportBackup(opts: BackupExportOptions): Promise<ExportBackupResult> {
  return call<ExportBackupResult>('ExportBackup', opts)
}
export async function OpenBackupFile(): Promise<PathPickResult> {
  return call<PathPickResult>('OpenBackupFile')
}
export async function InspectBackup(path: string, password: string): Promise<BackupInspectResult> {
  return call<BackupInspectResult>('InspectBackup', path, password)
}
export async function ImportBackup(path: string, password: string, conflict: 'skip' | 'overwrite'): Promise<ImportBackupResult> {
  return call<ImportBackupResult>('ImportBackup', path, password, conflict)
}
