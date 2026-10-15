/**
 * 能力位的规范词表（顺序即后端 NormalizeModalities 的归一化顺序，也是界面上的展示顺序）。
 * 放在这里是因为 ModelCard 与两个页面的批量栏都要用同一份清单：本阶段的文件独占清单不允许
 * 新建独立常量模块，仓库里既有的 constants/variants.ts 又不在可改范围内，故与类型同址。
 */
export const CAP_INPUT_MODALITIES = ['text', 'image', 'video', 'audio', 'pdf'] as const
export const CAP_OUTPUT_MODALITIES = ['text', 'audio', 'image', 'video', 'pdf'] as const
/** 六个布尔能力位的 JSON 键（顺序即界面展示顺序）。 */
export const CAP_FLAG_KEYS = ['tool_call', 'structured_output', 'native_web_search', 'mid_conv_system', 'attachment', 'temperature'] as const

/**
 * harness 中立的能力集合（对应后端 internal/core/model_features.go 的 ModelFeatures）。
 *
 * **三态语义：键不存在 = 未知**。前端只写「已知为真」：勾选写 `true`，取消勾选把键删掉，
 * 绝不写 `false`——留空表示「不知道」，由目标客户端自己的默认值兜底，写成 `false` 会变成
 * 「确认不支持」这种前端无从判断的断言。
 * 模态是集合语义：键不存在 = 未知；键存在即「已知支持集合」（因此不做半填）。
 */
export interface ModelCapabilities {
  input_modalities?: string[]
  output_modalities?: string[]
  tool_call?: boolean
  structured_output?: boolean
  native_web_search?: boolean
  mid_conv_system?: boolean
  attachment?: boolean
  temperature?: boolean
  /** 推断来源标识（内部字段，只读展示）。 */
  _source?: string
}

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
	/** 配置里 off 档位的原始拼法（disabled / none / nothink），前端原样透传、不解释语义。 */
	_off_alias?: string
	/**
	 * APM 依据内建模型能力表推断的厂商（小写标识，未知为 unknown）。纯展示、不写盘。
	 */
	_vendor?: string
	/**
	 * APM 推断的该模型支持档位集合（规范化后的 off/强度档）。用于 UI 自适应启用；
	 * 为空表示未推断到（展示全集不高亮）。绝不写回配置。
	 */
	_supported_levels?: string[]
	/**
	 * 用户可编辑的权威能力值，保存时随卡片原样提交（写入端只读它，不改键名）。
	 * 与下面两个 opencode 形态的旧字段（attachment / modalities）互不影响：
	 * 能力区只读写 capabilities，旧字段原样保留、不做迁移。
	 */
	capabilities?: ModelCapabilities
	/**
	 * 能力表 / provider 自报推断出的原始能力值，只读，绝不落盘（下划线前缀会被后端统一剥离）。
	 * 用于在卡片上标出「推断支持」并提供「采纳推断」，不自动写入 capabilities。
	 */
	_capabilities_inferred?: ModelCapabilities
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

/** 连接探测结果（TestConnectionRaw / TestProviderConnection 共用）。 */
export interface ProbeResult {
  success: boolean
  error?: string
  error_code?: string
  /** 两项检查都通过的整体结论。 */
  ok?: boolean
  models_ok?: boolean
  models_count?: number
  models_ms?: number
  chat_ok?: boolean
  chat_ms?: number
  /** 没有可用模型列表时跳过 chat 探测。 */
  chat_skipped?: boolean
}

/** 提供商 ↔ 钥匙串条目的关联：`${agent}|${providerID}` -> entryID。 */
export interface KeychainLinksResult {
  success: boolean
  error?: string
  links?: Record<string, string>
}

export interface SimpleResult {
  success: boolean
  error?: string
}

export interface PathPickResult {
  success: boolean
  error?: string
  path?: string
}

/** 备份文件内的单个提供商摘要（不含密钥内容）。 */
export interface BackupProviderSummary {
  id: string
  name: string
  kind: string
  model_count: number
  has_api_key: boolean
}

export interface BackupExportOptions {
  path: string
  provider_ids: string[]
  include_keys: boolean
  key_mode: 'plain' | 'sealed' | 'password'
  password: string
}

export interface ExportBackupResult {
  success: boolean
  error?: string
  path?: string
  provider_count?: number
  key_mode?: string
}

export interface BackupInspectResult {
  success: boolean
  error?: string
  /** 文件用密码加密且未提供正确密码时为 true。 */
  needs_password?: boolean
  agent?: string
  agent_label?: string
  created_at?: string
  app_version?: string
  include_keys?: boolean
  key_mode?: string
  provider_count?: number
  providers?: BackupProviderSummary[]
}

export interface BackupImportFailure {
  id: string
  error: string
}

export interface ImportBackupResult {
  success: boolean
  error?: string
  imported?: string[]
  skipped?: string[]
  failed?: BackupImportFailure[]
}
