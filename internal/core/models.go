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
	Attachment          bool                   `json:"attachment,omitempty"`
	Modalities          map[string]interface{} `json:"modalities,omitempty"`
	Headers             map[string]string      `json:"headers,omitempty"`
	Options             map[string]interface{} `json:"options,omitempty"`
	RawCfg              map[string]interface{} `json:"_raw_cfg,omitempty"`
}

// ProviderSummary mirrors provider_summary entries.
type ProviderSummary struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	BaseURL    string `json:"base_url"`
	HasAPIKey  bool   `json:"has_api_key"`
	ModelCount int    `json:"model_count"`
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
