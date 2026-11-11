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
// PI Agent（omp）的 modelRoles：只支持 "default" 一个角色（契约 §5）。
// selectors 由后端扫描 models.yml 生成，前端不得自行拼装。
// 空 selector 表示删除该角色键；config.yml/config.yaml 都不存在而 settings.json 存在时后端拒绝（G18）。
export type OMPModelRoles = {
  success?: boolean
  /** config.yml 路径（默认模型的写入口）。 */
  path?: string
  /** config.yml 是否已存在。 */
  exists?: boolean
  /** models.yml / models.json 路径；首次使用前该文件可能还不存在。 */
  models_path?: string
  /** 模型文件是否存在；false 表示 omp 还没写过配置，不是错误。 */
  models_exists?: boolean
  roles?: Record<string, string>
  selectors?: string[]
  /** 只存在旧版 models.json 时为 true，界面提示先运行一次 omp 完成迁移。 */
  needs_migration?: boolean
  error?: string
  error_code?: string
}
export async function GetOMPModelRoles(): Promise<OMPModelRoles> {
  return call<OMPModelRoles>('GetOMPModelRoles')
}
export async function SetOMPModelRole(role: string, selector: string): Promise<OMPModelRoles> {
  return call<OMPModelRoles>('SetOMPModelRole', role, selector)
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
/**
 * 获取模型列表的可选参数（契约 §4）：只有下面三个键会被后端识别。
 * - headers：额外请求头（例如需要鉴权头/版本头的网关）。
 * - kind：UI 协议类型（anthropic / responses / openai-compatible）；PI Agent 读取时按它
 *   决定对接哪种 api 形态。
 * - auth_header：自定义鉴权头名（默认 Authorization）。
 * 空对象与省略第三参等价，后端不得把它当成「清空」语义。
 */
export type FetchModelsOptions = {
  headers?: Record<string, string>
  kind?: string
  auth_header?: string
}
export async function FetchModels(baseUrl: string, apiKey: string, opts?: FetchModelsOptions): Promise<Record<string, unknown>> {
  const payload: Record<string, unknown> = {}
  const headers = opts && opts.headers ? opts.headers : undefined
  if (headers && Object.keys(headers).length > 0) payload['headers'] = headers
  if (opts && opts.kind) payload['kind'] = opts.kind
  if (opts && opts.auth_header) payload['auth_header'] = opts.auth_header
  // 始终按契约的三参调用：后端还没重新生成绑定时，多出的第三参会被生成的定参函数忽略，
  // 与「省略第三参」等价；这样后端就位后无需再改前端。
  return call('FetchModels', baseUrl, apiKey, payload)
}
export async function RefreshProviderModels(providerId: string, baseUrl: string, apiKey: string): Promise<Record<string, unknown>> {
  return call('RefreshProviderModels', providerId, baseUrl, apiKey)
}
export async function BuildSingleCard(modelId: string): Promise<Record<string, unknown>> {
  return call('BuildSingleCard', modelId)
}
// 带站点上下文的单卡构建：手动添加模型时与拉取路径同源（离线目录 + 站点规则的档位/能力位
// 也会参与推断）。apiType 归一与 ApplyModelPresetsForSite 共用 normalizeApiType。
export async function BuildSingleCardForSite(modelId: string, baseUrl: string, apiType: string): Promise<Record<string, unknown>> {
  return call('BuildSingleCardForSite', modelId, baseUrl || '', normalizeApiType(apiType))
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
    // PI Agent（omp）原生 api 名：openai-completions 是它的 chat-completions 叫法，
    // 站点规则表用的是 ZCode 的 api.type，所以这里归一后再交给后端。
    case 'openai-completions':
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
