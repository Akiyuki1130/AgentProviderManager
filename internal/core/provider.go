package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

func BuildOptions(baseURL, apiKey string, apiKeyRequired bool, extra map[string]interface{}) (map[string]interface{}, error) {
	baseURL = NormalizeBaseURL(baseURL)
	if ok, msg := ValidateBaseURL(baseURL); !ok {
		return nil, fmt.Errorf("%s", msg)
	}
	opts := map[string]interface{}{}
	for k, v := range extra {
		opts[k] = v
	}
	opts["baseURL"] = baseURL
	if strings.TrimSpace(apiKey) != "" {
		opts["apiKey"] = strings.TrimSpace(apiKey)
	} else {
		delete(opts, "apiKey")
	}
	if apiKeyRequired {
		opts["apiKeyRequired"] = true
	} else {
		delete(opts, "apiKeyRequired")
	}
	return opts, nil
}

func BuildProviderCfg(providerID, providerName, baseURL, apiKey string, cards []ModelCard, kind string) (string, map[string]interface{}, map[string]interface{}, error) {
	pid := strings.TrimSpace(providerID)
	if !ProviderIDValid(pid) {
		return "", nil, nil, fmt.Errorf("Provider ID 只能包含字母、数字、下划线、短横线或冒号（且不能为空）")
	}
	kindNorm, err := ValidateKind(kind)
	if err != nil {
		return "", nil, nil, err
	}
	name := strings.TrimSpace(providerName)
	if name == "" {
		name = pid
	}
	if len(name) > MaxProviderNameLen {
		return "", nil, nil, fmt.Errorf("Provider 名称不能超过 %d 个字符", MaxProviderNameLen)
	}
	if len(baseURL) > 2048 {
		return "", nil, nil, fmt.Errorf("Base URL 不能超过 2048 个字符")
	}
	if len(apiKey) > MaxAPIKeyLength {
		return "", nil, nil, fmt.Errorf("API Key 不能超过 %d 个字符", MaxAPIKeyLength)
	}
	models, err := CardsToCfgMap(cards, kindNorm)
	if err != nil {
		return "", nil, nil, err
	}
	opts, err := BuildOptions(baseURL, apiKey, strings.TrimSpace(apiKey) != "", nil)
	if err != nil {
		return "", nil, nil, err
	}
	cfg := map[string]interface{}{
		"name":    name,
		"kind":    KindToConfig[kindNorm],
		"source":  "custom",
		"options": opts,
		"models":  models,
	}
	return pid, cfg, models, nil
}

func BuildProviderCfgEditor(provider map[string]interface{}) (string, map[string]interface{}, error) {
	pidRaw, _ := provider["id"].(string)
	pid := strings.TrimSpace(pidRaw)
	if !ProviderIDValid(pid) {
		return "", nil, fmt.Errorf("Provider ID 只能包含字母、数字、下划线、短横线或冒号（且不能为空）")
	}
	nameRaw := provider["name"]
	name := ""
	if s, ok := nameRaw.(string); ok {
		name = strings.TrimSpace(s)
	}
	if name == "" {
		name = pid
	}
	if len(name) > MaxProviderNameLen {
		return "", nil, fmt.Errorf("Provider 名称不能超过 %d 个字符", MaxProviderNameLen)
	}
	kindRaw, _ := provider["kind"].(string)
	kindNorm, err := ValidateKind(kindRaw)
	if err != nil {
		// Allow empty -> default, but ValidateKind handles that
		if kindRaw == "" {
			kindNorm = "openai-compatible"
		} else {
			return "", nil, err
		}
	}
	baseURL, _ := provider["base_url"].(string)
	apiKey, _ := provider["api_key"].(string)
	if len(baseURL) > 2048 {
		return "", nil, fmt.Errorf("Base URL 不能超过 2048 个字符")
	}
	if len(apiKey) > MaxAPIKeyLength {
		return "", nil, fmt.Errorf("API Key 不能超过 %d 个字符", MaxAPIKeyLength)
	}
	apiKeyRequired, _ := provider["api_key_required"].(bool)
	extra, err := RawOptions(provider["options_json"])
	if err != nil {
		return "", nil, err
	}
	rawProvider, _ := provider["_raw_provider"].(map[string]interface{})
	cfg := deepCopyMap(rawProvider)
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	opts, err := BuildOptions(baseURL, apiKey, apiKeyRequired, extra)
	if err != nil {
		return "", nil, err
	}
	var models map[string]interface{}
	var cards []ModelCard
	if cardsRaw, ok := provider["cards"]; ok {
		// cards can be []ModelCard or []interface{}
		switch v := cardsRaw.(type) {
		case []ModelCard:
			cards = v
		case []interface{}:
			for _, e := range v {
				if m, ok := e.(map[string]interface{}); ok {
					b, _ := json.Marshal(m)
					var c ModelCard
					_ = json.Unmarshal(b, &c)
					cards = append(cards, c)
				}
			}
		}
		if len(cards) > 0 {
			models, err = CardsToCfgMap(cards, kindNorm)
			if err != nil {
				return "", nil, err
			}
		} else {
			models = map[string]interface{}{}
		}
	} else {
		models = map[string]interface{}{}
	}
	cfg["name"] = name
	cfg["kind"] = KindToConfig[kindNorm]
	cfg["source"] = "custom"
	cfg["options"] = opts
	cfg["models"] = models
	delete(cfg, "_raw_provider")
	return pid, cfg, nil
}

func ProviderToEdit(config map[string]interface{}, providerID string) (*ProviderEdit, error) {
	providers, _ := config["provider"].(map[string]interface{})
	if providers == nil {
		return nil, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	raw, ok := providers[providerID].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	opts, _ := raw["options"].(map[string]interface{})
	if opts == nil {
		opts = map[string]interface{}{}
	}
	extra := map[string]interface{}{}
	for k, v := range opts {
		if k != "baseURL" && k != "apiKey" && k != "apiKeyRequired" {
			extra[k] = v
		}
	}
	kindStr, _ := raw["kind"].(string)
	baseURLStr, _ := opts["baseURL"].(string)
	kind := ConfigKindToUIKind(kindStr, baseURLStr)
	models, _ := raw["models"].(map[string]interface{})
	if models == nil {
		models = map[string]interface{}{}
	}
	// 非 nil 空切片：cards 没有 omitempty，零模型时 nil 会编码成 JSON null，
	// 前端拿到 null 会在 cards.map 上抛错。
	cards := make([]ModelCard, 0)
	for mid, mcfg := range models {
		mm, _ := mcfg.(map[string]interface{})
		cards = append(cards, CfgToCard(mid, mm))
	}
	// sort by model_id lower
	for i := 0; i < len(cards); i++ {
		for j := i + 1; j < len(cards); j++ {
			if strings.ToLower(cards[j].ModelID) < strings.ToLower(cards[i].ModelID) {
				cards[i], cards[j] = cards[j], cards[i]
			}
		}
	}
	optionsJSON := ""
	if len(extra) > 0 {
		b, _ := json.Marshal(extra)
		optionsJSON = string(b)
	}
	name := providerID
	if n, ok := raw["name"].(string); ok && strings.TrimSpace(n) != "" {
		name = n
	}
	apiKey, _ := opts["apiKey"].(string)
	apiKeyRequired, _ := opts["apiKeyRequired"].(bool)
	return &ProviderEdit{
		ID:             providerID,
		Name:           name,
		Kind:           kind,
		BaseURL:        baseURLStr,
		APIKey:         apiKey,
		APIKeyRequired: apiKeyRequired,
		OptionsJSON:    optionsJSON,
		Cards:          cards,
		RawProvider:    deepCopyMap(raw),
	}, nil
}

func BuildProviderSummary(config map[string]interface{}) []ProviderSummary {
	providers, _ := config["provider"].(map[string]interface{})
	if providers == nil {
		// 空配置也要给 []，nil 会被编码成 JSON null。
		return []ProviderSummary{}
	}
	out := make([]ProviderSummary, 0, len(providers))
	for pid, raw := range providers {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		opts, _ := m["options"].(map[string]interface{})
		if opts == nil {
			opts = map[string]interface{}{}
		}
		baseURL, _ := opts["baseURL"].(string)
		kindStr, _ := m["kind"].(string)
		kind := ConfigKindToUIKind(kindStr, baseURL)
		apiKey, _ := opts["apiKey"].(string)
		hasKey := strings.TrimSpace(apiKey) != ""
		name := pid
		if n, ok := m["name"].(string); ok && strings.TrimSpace(n) != "" {
			name = n
		}
		modelCount := 0
		if models, ok := m["models"].(map[string]interface{}); ok {
			modelCount = len(models)
		}
		out = append(out, ProviderSummary{
			ID:         pid,
			Name:       name,
			Kind:       kind,
			BaseURL:    baseURL,
			HasAPIKey:  hasKey,
			ModelCount: modelCount,
		})
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if strings.ToLower(out[j].ID) < strings.ToLower(out[i].ID) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
