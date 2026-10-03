export interface ModelCard {
  model_id: string
  api_returned?: boolean
  name: string
  reasoning: boolean
  variants: string[]
  default_variant: string
  context: string | number
  output: string | number
  select: boolean
	_raw_reasoning_enabled?: boolean | null
	attachment?: boolean
	modalities?: Record<string, unknown>
	headers?: Record<string, string>
	options?: Record<string, unknown>
	_raw_cfg?: Record<string, unknown>
}

export interface ProviderSummary {
  id: string
  name: string
  kind: string
  base_url: string
  has_api_key: boolean
  model_count: number
}

export interface ProviderEdit {
  id: string
  name: string
  kind: string
  base_url: string
  api_key: string
  api_key_required: boolean
  options_json: string
  cards: ModelCard[]
  _raw_provider?: Record<string, unknown>
}

export interface ConfigLocation {
  label: string
  path: string
  exists: boolean
  accessible: boolean
  error: string
  provider_count: number
  provider_ids: string[]
}

export interface KeychainEntry {
  id: string
  name: string
  note: string
  created_at: string
  updated_at: string
  key_hint: string
}

export interface KeychainEntryFull extends KeychainEntry {
  api_key: string
}

export interface ImportPreview {
  path: string
  exists: boolean
  error: string
  providers: ProviderSummary[]
}

/** ZCode 当前生效的配置格式：v2 -> provider_config.json，legacy -> config.json。 */
export type ZCodeConfigFormat = 'v2' | 'legacy' | 'unknown'

export interface ZCodeFormatInfo {
  success: boolean
  error?: string
  format: ZCodeConfigFormat
  path: string
  supports_legacy_import: boolean
}

export interface LegacyImportPreview {
  success: boolean
  error?: string
  providers: ProviderSummary[]
  /** providerId -> 新格式不支持的旧字段名列表 */
  dropped: Record<string, string[]>
}

export interface LegacyImportResult {
  success: boolean
  error?: string
  imported: string[]
  skipped: string[]
  backup: string
}

/** 还原点记录的目标文件格式：v2 / legacy / opencode / deepseek / unknown。 */
export type RestorePointFormat = 'v2' | 'legacy' | 'opencode' | 'deepseek' | 'unknown'

/**
 * 还原点：每次写配置前自动建立的“修改前状态”快照。
 * 只含元数据与校验值，不含任何配置内容或 API Key。
 */
export interface RestorePoint {
  id: string
  created_at: string
  agent_id: string
  agent_label: string
  target_path: string
  format: RestorePointFormat
  /** false 表示建立还原点时该文件还不存在（恢复即回到“文件不存在”的状态）。 */
  existed: boolean
  size_bytes: number
  sha256: string
  fingerprint: string
  app_version: string
  operation: string
  provider_id?: string
  model_id?: string
  note?: string
}

/** ZCode 3.14 会拒绝的兼容性问题（code 见 core 的 issue 常量）。 */
export interface CompatIssue {
  code: string
  path: string
  msg: string
}

export interface RestorePointListResult {
  success: boolean
  error?: string
  points: RestorePoint[]
  target: string
  agent: string
  dir: string
}

export interface RestorePointRestoreResult {
  success: boolean
  error?: string
  id?: string
  target?: string
  /** 仅“恢复为文件不存在”分支返回：被移走的当前文件路径（.removed_<时间戳>）。 */
  removed_path?: string
  /** 回滚前为当前状态自动建立的还原点。 */
  snapshot?: RestorePoint | null
  warning?: string
  target_switched?: boolean
  current_target?: string
}

export interface RestorePointDeleteResult {
  success: boolean
  error?: string
  id?: string
}

export interface RestorePointPruneResult {
  success: boolean
  error?: string
  removed: number
  points: RestorePoint[]
}

export interface RestorePointsDirResult {
  success: boolean
  error?: string
  dir?: string
}

export interface ZCodeCompatResult {
  success: boolean
  error?: string
  path: string
  format: string
  issues: CompatIssue[]
}

export interface ZCodeCompatRepairResult {
  success: boolean
  error?: string
  path?: string
  fixes?: string[]
  issues?: CompatIssue[]
  backup?: string
  restore_point?: RestorePoint | null
  target?: string
}
