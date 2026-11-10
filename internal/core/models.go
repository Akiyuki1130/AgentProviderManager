package core

// ModelCard mirrors the frontend-friendly model card.
type ModelCard struct {
	ModelID             string                 `json:"model_id"`
	APIReturned         bool                   `json:"api_returned"`
	Name                string                 `json:"name"`
	Reasoning           bool                   `json:"reasoning"`
	Variants            []string               `json:"variants"`
	DefaultVariant      string                 `json:"default_variant"`
	Context             interface{}            `json:"context"`
	Output              interface{}            `json:"output"`
	Select              bool                   `json:"select"`
	RawReasoningEnabled *bool                  `json:"_raw_reasoning_enabled,omitempty"`
	// OffAlias 记录配置里 off 档位的原始拼法（disabled / none / nothink）。
	// ZCode 的档位集合里 off 词随家族变化，写回时必须用原拼法，否则模型 map 表达式匹配不到。
	OffAlias string `json:"_off_alias,omitempty"`
	// _vendor / _supported_levels 是 APM 内部字段，绝不写回配置（写盘出口统一由
	// cards.go 的 stripAPMInternalModelKeys 剥离）。两者的取值口径见 capability.go 的
	// ModelCapability.Source（分层顺序在 lookupCapabilityForSite）：
	//   - _vendor：厂商判定（detectVendor）或覆盖层给出，纯展示，与补全开关无关；
	//   - _supported_levels：**优先来自 provider 自报的档位枚举**（cards.go 的 metaLevels，
	//     那是 API 事实，即使 autoFill 关闭也写），provider 没给时才在 autoFill 为真时
	//     回落到能力表档位。
	Vendor          string                 `json:"_vendor,omitempty"`
	SupportedLevels []string               `json:"_supported_levels,omitempty"`
	// Capabilities 是用户可编辑的**中立能力位**（输入/输出模态、工具调用、结构化输出等），
	// 三个 harness 的写入端都只读它、各自映射到自己的键名（映射约束见 MODEL-MANAGEMENT-PLAN.md
	// 第 10.2.3）。后端从能力分层表与 provider 自报字段推断后填充，用户可覆盖。
	Capabilities *ModelCapabilities `json:"capabilities,omitempty"`
	// _capabilities_inferred 是「能力表算出来的原始推断值」，只用于前端展示来源与
	// 「按能力表补全」按钮；它绝不写盘（写入端只读 Capabilities）。
	CapabilitiesInferred *ModelCapabilities `json:"_capabilities_inferred,omitempty"`
	Attachment          bool                   `json:"attachment,omitempty"`
	Modalities          map[string]interface{} `json:"modalities,omitempty"`
	Headers             map[string]string      `json:"headers,omitempty"`
	Options             map[string]interface{} `json:"options,omitempty"`
	RawCfg              map[string]interface{} `json:"_raw_cfg,omitempty"`
}

// MergeModelCards 复刻 MergeProviderIntoConfig(mergeModels=true) 的模型合并语义：
// 保留 existing 的顺序，incoming 中同 ID 的覆盖 existing，新 ID 追加到末尾。
// 新版 ZCode backend 的 Upsert 是“模型集合整体替换”，导入路径需要显式并入已有模型。
func MergeModelCards(existing, incoming []ModelCard) []ModelCard {
	merged := make([]ModelCard, 0, len(existing)+len(incoming))
	index := make(map[string]int, len(existing))
	for _, c := range existing {
		index[c.ModelID] = len(merged)
		merged = append(merged, c)
	}
	for _, c := range incoming {
		if i, ok := index[c.ModelID]; ok {
			merged[i] = c
			continue
		}
		index[c.ModelID] = len(merged)
		merged = append(merged, c)
	}
	return merged
}

// ProviderSummary mirrors provider_summary entries.
type ProviderSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	BaseURL    string `json:"base_url"`
	HasAPIKey  bool   `json:"has_api_key"`
	ModelCount int    `json:"model_count"`
	// ReadOnly 与 ProviderEdit.ReadOnly 同源：omp 侧只读的 provider（协议不可建模、
	// 缺 baseUrl、只有 models.json 未迁移等）在列表里就要能画出只读徽标，不必等详情。
	// 其它 harness 恒为 false。
	ReadOnly bool `json:"read_only"`
}

// ProviderEdit mirrors the editor provider object.
type ProviderEdit struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Kind           string                 `json:"kind"`
	BaseURL        string                 `json:"base_url"`
	APIKey         string                 `json:"api_key"`
	APIKeyRequired bool                   `json:"api_key_required"`
	OptionsJSON    string                 `json:"options_json"`
	Cards          []ModelCard            `json:"cards"`
	RawProvider    map[string]interface{} `json:"_raw_provider,omitempty"`
	// 以下四个字段是 omp 专用只读信息。其它 harness 不填，omitempty 保证现有载荷不变。
	// ReadOnly 为 true 时后端仍允许整体删除，但拒绝保存 / 部分写（见计划 §4.7）。
	ReadOnly       bool   `json:"read_only,omitempty"`
	ReadOnlyReason string `json:"read_only_reason,omitempty"`
	// APIKeyKind 区分 omp 的 apiKey 三种形态：literal | env | command | none。
	APIKeyKind string `json:"api_key_kind,omitempty"`
	// RawAPI 是 omp 原始 api 值，只在协议不可建模（只读）时用于展示。
	RawAPI string `json:"raw_api,omitempty"`
}

// OpencodePreview mirrors opencode_import_preview.
type ImportPreview struct {
	Path      string            `json:"path"`
	Exists    bool              `json:"exists"`
	Error     string            `json:"error"`
	Providers []ProviderSummary `json:"providers"`
}

// ConfigLocation mirrors detect_config_locations entries.
type ConfigLocation struct {
	Label         string   `json:"label"`
	Path          string   `json:"path"`
	Exists        bool     `json:"exists"`
	Accessible    bool     `json:"accessible"`
	Error         string   `json:"error"`
	ProviderCount int      `json:"provider_count"`
	ProviderIDs   []string `json:"provider_ids"`
	// NeedsMigration 只对 omp 的 models.json 为 true：该文件只能只读解析，
	// 写盘前必须让 omp 自己迁移成 models.yml（G1）。其它 harness 恒为 false。
	NeedsMigration bool `json:"needs_migration"`
}

// KeychainEntry mirrors a keychain entry.
type KeychainEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	APIKey    string `json:"api_key"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// FetchResult mirrors fetch_models results.
type FetchResult struct {
	Models []ModelCard `json:"models"`
	Count  int         `json:"count"`
}

// StripAPMInternalModelKeys 导出 cards.go 里 stripAPMInternalModelKeys 的可见性包装，
// 供 app 层没有 backend 的兜底写盘路径（TODO #21：CardsToCfgMap + MergeProviderIntoConfig
// + WriteConfig）在落盘前剥掉 _raw_cfg / _off_alias / _vendor / _supported_levels /
// _apm_capabilities 等 APM 内部键。唯一实现仍在 cards.go，这里不得复制逻辑。
// 支持的入参形状与 cards.go 一致：单 provider 配置 {models:{...}} 或整份文档 {provider:{...}}。
func StripAPMInternalModelKeys(providerCfg map[string]interface{}) {
	stripAPMInternalModelKeys(providerCfg)
}
