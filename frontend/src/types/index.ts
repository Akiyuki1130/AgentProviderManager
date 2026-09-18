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
