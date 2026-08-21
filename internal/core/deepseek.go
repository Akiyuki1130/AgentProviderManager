package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func isYAMLPath(path string) bool {
	low := strings.ToLower(path)
	return strings.HasSuffix(low, ".yaml") || strings.HasSuffix(low, ".yml")
}

func loadDeepSeekRaw(path string) (map[string]interface{}, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if strings.TrimSpace(string(data)) == "" {
		return map[string]interface{}{}, "json", nil
	}
	if isYAMLPath(path) {
		var result map[string]interface{}
		if err := yaml.Unmarshal(data, &result); err != nil {
			return nil, "", &ConfigParseError{Msg: fmt.Sprintf("DeepSeek Harness 配置解析失败（YAML）：%v", err)}
		}
		if result == nil {
			result = map[string]interface{}{}
		}
		return result, "yaml", nil
	}
	cleaned := StripJSONCComments(string(data))
	var raw interface{}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		var yResult map[string]interface{}
		if err2 := yaml.Unmarshal(data, &yResult); err2 == nil && yResult != nil {
			return yResult, "yaml", nil
		}
		return nil, "", &ConfigParseError{Msg: fmt.Sprintf("DeepSeek Harness 配置解析失败（JSON）：%v", err)}
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, "", &ConfigParseError{Msg: "配置文件根节点必须是对象 {...}"}
	}
	return m, "json", nil
}

func DeepSeekLoadConfig(path string) (map[string]interface{}, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	m, _, err := loadDeepSeekRaw(path)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func DeepSeekProvidersMap(cfg map[string]interface{}) map[string]interface{} {
	return deepSeekProvidersMap(cfg)
}

func deepSeekProvidersMap(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	merged := map[string]interface{}{}
	if llm, ok := cfg["llm-pi-ai"].(map[string]interface{}); ok {
		if prov, ok := llm["providers"].(map[string]interface{}); ok {
			for k, v := range prov {
				merged[k] = v
			}
		}
	}
	if p, ok := cfg["provider"].(map[string]interface{}); ok {
		for k, v := range p {
			if _, exists := merged[k]; !exists {
				merged[k] = v
			}
		}
	}
	if len(merged) > 0 {
		return merged
	}
	return nil
}

func DeepSeekProviderSource(cfg map[string]interface{}, providerID string) (string, map[string]interface{}) {
	return deepSeekProviderSource(cfg, providerID)
}

func deepSeekProviderSource(cfg map[string]interface{}, providerID string) (string, map[string]interface{}) {
	if cfg == nil {
		return "", nil
	}
	if llm, ok := cfg["llm-pi-ai"].(map[string]interface{}); ok {
		if prov, ok := llm["providers"].(map[string]interface{}); ok {
			if v, ok := prov[providerID]; ok {
				if m, ok := v.(map[string]interface{}); ok {
					return "llm", m
				}
			}
		}
	}
	if prov, ok := cfg["provider"].(map[string]interface{}); ok {
		if v, ok := prov[providerID]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				return "provider", m
			}
		}
	}
	return "", nil
}

func isDeepSeekLLMMode(cfg map[string]interface{}) bool {
	if cfg == nil {
		return false
	}
	if llm, ok := cfg["llm-pi-ai"].(map[string]interface{}); ok {
		if prov, ok := llm["providers"].(map[string]interface{}); ok && len(prov) > 0 {
			return true
		}
		if _, ok := llm["providers"].(map[string]interface{}); ok {
			if _, hasProvider := cfg["provider"]; !hasProvider {
				return true
			}
			if p, ok := cfg["provider"].(map[string]interface{}); !ok || len(p) == 0 {
				return true
			}
		}
	}
	return false
}

func LoadCredentialsForPreview() map[string]string {
	return loadCredentials("")
}

func loadCredentials(credsPath string) map[string]string {
	if credsPath == "" {
		credsPath = DeepSeekCredentialsPath()
	}
	data, err := os.ReadFile(credsPath)
	if err != nil {
		return map[string]string{}
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for k, v := range raw {
		if k == "version" || k == "refs" {
			continue
		}
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	if refs, ok := raw["refs"].(map[string]interface{}); ok {
		for k, v := range refs {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func WriteDeepSeekYAML(path string, cfg map[string]interface{}, fingerprint string) error {
	return writeDeepSeekConfig(path, cfg, fingerprint)
}

func ConvertDeepSeekModel(mid string, modelCfg map[string]interface{}, kind string) map[string]interface{} {
	if modelCfg == nil {
		modelCfg = map[string]interface{}{}
	}
	if id, ok := modelCfg["id"].(string); ok && strings.TrimSpace(id) != "" {
		mid = strings.TrimSpace(id)
	}
	out := deepCopyMap(modelCfg)
	if out == nil {
		out = map[string]interface{}{}
	}
	out["_dsh_raw"] = deepCopyMap(modelCfg)
	if name, ok := modelCfg["name"].(string); ok && strings.TrimSpace(name) != "" && strings.TrimSpace(name) != mid {
		n := strings.TrimSpace(name)
		if len(n) > MaxProviderNameLen {
			n = n[:MaxProviderNameLen]
		}
		out["name"] = n
	}
	if re := modelCfg["reasoningEfforts"]; re != nil {
		if re == false {
			// Explicit non-reasoning marker – do not emit reasoning block.
		} else if m, ok := re.(map[string]interface{}); ok && len(m) > 0 {
			variants := []string{}
			for k, v := range m {
				if v == nil && strings.ToLower(strings.TrimSpace(k)) == "off" {
					variants = append(variants, "off")
					continue
				}
				if v == nil {
					continue
				}
				lk := strings.ToLower(strings.TrimSpace(k))
				if ZCodeVariantSet[lk] {
					variants = append(variants, lk)
				} else if vv, ok := OpencodeEffortToVariant[lk]; ok {
					variants = append(variants, vv)
				}
			}
			if len(variants) > 0 {
				var offPart, rest []string
				for _, v := range variants {
					if v == "off" {
						offPart = append(offPart, v)
					} else {
						rest = append(rest, v)
					}
				}
				ordered := []string{}
				for _, v := range OpencodeEffortOrder {
					for _, r := range rest {
						if r == v {
							ordered = append(ordered, r)
							break
						}
					}
				}
				for _, r := range rest {
					found := false
					for _, o := range ordered {
						if o == r {
							found = true
							break
						}
					}
					if !found {
						ordered = append(ordered, r)
					}
				}
				variants = append(offPart, ordered...)
				if len(variants) == 0 {
					variants = DefaultVariantsFor(kind)
				}
				defaultVariant := ""
				for _, pref := range DefaultVariantPreference {
					if containsStr(variants, pref) {
						defaultVariant = pref
						break
					}
				}
				if defaultVariant == "" && len(variants) > 0 {
					defaultVariant = variants[0]
				}
				if !(len(variants) == 1 && variants[0] == "off") {
					out["reasoning"] = map[string]interface{}{
						"enabled":        true,
						"variants":       variants,
						"defaultVariant": defaultVariant,
					}
				}
			}
		}
	}
	if limit, ok := modelCfg["limit"].(map[string]interface{}); ok {
		ctx := ParseTokens(limit["context"])
		outTok := ParseTokens(limit["output"])
		if ctx != nil && outTok != nil {
			out["limit"] = map[string]interface{}{"context": *ctx, "output": *outTok}
		}
	} else {
		var ctxVal, outVal interface{}
		if v, ok := modelCfg["contextWindow"]; ok {
			ctxVal = v
		} else if v, ok := modelCfg["context_window"]; ok {
			ctxVal = v
		}
		if v, ok := modelCfg["maxTokens"]; ok {
			outVal = v
		} else if v, ok := modelCfg["max_tokens"]; ok {
			outVal = v
		}
		if ctxVal != nil || outVal != nil {
			ctx := ParseTokens(ctxVal)
			outTok := ParseTokens(outVal)
			if ctx != nil && outTok != nil {
				out["limit"] = map[string]interface{}{"context": *ctx, "output": *outTok}
			} else if ctx != nil {
				out["limit"] = map[string]interface{}{"context": *ctx}
			} else if outTok != nil {
				out["limit"] = map[string]interface{}{"output": *outTok}
			}
		}
	}
	return out
}

func ConvertDeepSeekProvider(providerID string, raw map[string]interface{}, kind *string) (string, map[string]interface{}, error) {
	pid := strings.TrimSpace(providerID)
	if !ProviderIDValid(pid) {
		return "", nil, fmt.Errorf("Provider ID「%s」只能包含字母、数字、下划线、短横线或冒号", providerID)
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	name := pid
	if n, ok := raw["displayName"].(string); ok && strings.TrimSpace(n) != "" {
		name = strings.TrimSpace(n)
	} else if n, ok := raw["name"].(string); ok && strings.TrimSpace(n) != "" {
		name = strings.TrimSpace(n)
	}
	if len(name) > MaxProviderNameLen {
		name = name[:MaxProviderNameLen]
	}
	baseURL, _ := raw["baseURL"].(string)
	apiKey, _ := raw["apiKey"].(string)
	apiKeyEnv, _ := raw["apiKeyEnv"].(string)
	if opts, ok := raw["options"].(map[string]interface{}); ok {
		if baseURL == "" {
			baseURL, _ = opts["baseURL"].(string)
		}
		if apiKey == "" {
			apiKey, _ = opts["apiKey"].(string)
		}
		if apiKeyEnv == "" {
			apiKeyEnv, _ = opts["apiKeyEnv"].(string)
		}
	}
	if apiKey == "" && apiKeyEnv != "" {
		creds := loadCredentials("")
		if v, ok := creds[apiKeyEnv]; ok {
			apiKey = v
		} else if v := os.Getenv(apiKeyEnv); v != "" {
			apiKey = v
		}
	}
	resolvedKind := ""
	if kind != nil {
		resolvedKind = *kind
	} else {
		resolvedKind = InferKind(baseURL)
		if rk, ok := raw["kind"].(string); ok && strings.TrimSpace(rk) != "" {
			resolvedKind = ConfigKindToUIKind(strings.TrimSpace(rk), baseURL)
		} else if api, ok := raw["api"].(string); ok {
			low := strings.ToLower(api)
			if strings.Contains(low, "anthropic") {
				resolvedKind = "anthropic"
			}
		}
	}
	kindNorm, err := ValidateKind(resolvedKind)
	if err != nil {
		return "", nil, err
	}
	extra := map[string]interface{}{}
	for k, v := range raw {
		if k != "baseURL" && k != "apiKey" && k != "apiKeyEnv" && k != "models" && k != "displayName" && k != "name" {
			extra[k] = v
		}
	}
	modelsOut := map[string]interface{}{}
	if rawModels, ok := raw["models"].([]interface{}); ok {
		for _, entry := range rawModels {
			mm, _ := entry.(map[string]interface{})
			if mm == nil {
				continue
			}
			mid, _ := mm["id"].(string)
			mid = strings.TrimSpace(mid)
			if mid == "" {
				continue
			}
			if len(mid) > MaxModelIDLength {
				return "", nil, fmt.Errorf("Provider「%s」的模型 ID「%s」过长", pid, ShortText(mid, 40))
			}
			modelsOut[mid] = ConvertDeepSeekModel(mid, mm, kindNorm)
		}
	} else if rawModelsMap, ok := raw["models"].(map[string]interface{}); ok {
		for mid, modelCfg := range rawModelsMap {
			mid = strings.TrimSpace(mid)
			if mid == "" {
				continue
			}
			mm, _ := modelCfg.(map[string]interface{})
			modelsOut[mid] = ConvertDeepSeekModel(mid, mm, kindNorm)
		}
	}
	if len(modelsOut) > MaxModels {
		return "", nil, fmt.Errorf("Provider「%s」的模型数量不能超过 %d 个", pid, MaxModels)
	}
	optsExtra := map[string]interface{}{}
	for k, v := range extra {
		optsExtra[k] = v
	}
	opts, err := BuildOptions(baseURL, apiKey, strings.TrimSpace(apiKey) != "", optsExtra)
	if err != nil {
		opts = map[string]interface{}{"baseURL": baseURL}
		if strings.TrimSpace(apiKey) != "" {
			opts["apiKey"] = strings.TrimSpace(apiKey)
			opts["apiKeyRequired"] = true
		}
		for k, v := range optsExtra {
			opts[k] = v
		}
	}
	cfg := map[string]interface{}{
		"name":    name,
		"kind":    KindToConfig[kindNorm],
		"source":  "custom",
		"options": opts,
		"models":  modelsOut,
	}
	return pid, cfg, nil
}

func DeepSeekProviderToEdit(providerID string, raw map[string]interface{}) (*ProviderEdit, error) {
	pid, normalized, err := ConvertDeepSeekProvider(providerID, raw, nil)
	if err != nil {
		return nil, err
	}
	opts, _ := normalized["options"].(map[string]interface{})
	baseURL := stringOr(opts["baseURL"], "")
	apiKey := stringOr(opts["apiKey"], "")
	cards := make([]ModelCard, 0)
	models, _ := normalized["models"].(map[string]interface{})
	for mid, value := range models {
		mm, _ := value.(map[string]interface{})
		card := CfgToCard(mid, mm)
		if card.Context == "" {
			card.Context = mm["contextWindow"]
		}
		if card.Output == "" {
			card.Output = mm["maxTokens"]
		}
		cards = append(cards, card)
	}
	for i := 0; i < len(cards); i++ {
		for j := i + 1; j < len(cards); j++ {
			if strings.ToLower(cards[j].ModelID) < strings.ToLower(cards[i].ModelID) {
				cards[i], cards[j] = cards[j], cards[i]
			}
		}
	}
	name := stringOr(normalized["name"], pid)
	return &ProviderEdit{ID: pid, Name: name, Kind: stringOr(normalized["kind"], "openai-compatible"), BaseURL: baseURL, APIKey: apiKey, APIKeyRequired: apiKey != "", Cards: cards, RawProvider: deepCopyMap(raw)}, nil
}

func DeepSeekImportPreview(path string) ImportPreview {
	if strings.TrimSpace(path) == "" {
		path = DeepSeekSettingsPath()
	}
	item := ImportPreview{Path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return item
	}
	cfg, err := DeepSeekLoadConfig(path)
	if err != nil {
		item.Exists = true
		item.Error = err.Error()
		return item
	}
	item.Exists = true
	providers := deepSeekProvidersMap(cfg)
	if providers == nil || len(providers) == 0 {
		item.Error = "该文件中没有可管理的 provider 配置"
		return item
	}
	creds := loadCredentials("")
	for pid, raw := range providers {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		baseURL, _ := m["baseURL"].(string)
		apiKey, _ := m["apiKey"].(string)
		apiKeyEnv, _ := m["apiKeyEnv"].(string)
		if opts, ok := m["options"].(map[string]interface{}); ok {
			if baseURL == "" {
				baseURL, _ = opts["baseURL"].(string)
			}
			if apiKey == "" {
				apiKey, _ = opts["apiKey"].(string)
			}
			if apiKeyEnv == "" {
				apiKeyEnv, _ = opts["apiKeyEnv"].(string)
			}
		}
		hasKey := strings.TrimSpace(apiKey) != ""
		if !hasKey && apiKeyEnv != "" {
			if v, ok := creds[apiKeyEnv]; ok && strings.TrimSpace(v) != "" {
				hasKey = true
			} else if v := os.Getenv(apiKeyEnv); strings.TrimSpace(v) != "" {
				hasKey = true
			}
		}
		modelCount := 0
		if arr, ok := m["models"].([]interface{}); ok {
			modelCount = len(arr)
		} else if mp, ok := m["models"].(map[string]interface{}); ok {
			modelCount = len(mp)
		}
		kind := InferKind(baseURL)
		if api, ok := m["api"].(string); ok && strings.Contains(strings.ToLower(api), "anthropic") {
			kind = "anthropic"
		}
		name := stringOr(m["displayName"], stringOr(m["name"], pid))
		item.Providers = append(item.Providers, ProviderSummary{
			ID: pid, Name: name, Kind: kind,
			BaseURL: baseURL, HasAPIKey: hasKey, ModelCount: modelCount,
		})
	}
	for i := 0; i < len(item.Providers); i++ {
		for j := i + 1; j < len(item.Providers); j++ {
			if strings.ToLower(item.Providers[j].ID) < strings.ToLower(item.Providers[i].ID) {
				item.Providers[i], item.Providers[j] = item.Providers[j], item.Providers[i]
			}
		}
	}
	return item
}

// DeepSeekSaveProvider updates a provider in the existing on-disk snapshot.
// It is kept as a compatibility wrapper for callers that only have a path.
func DeepSeekSaveProvider(path string, providerID string, providerCfg map[string]interface{}, fingerprint string) error {
	cfg, err := DeepSeekLoadConfig(path)
	if err != nil {
		return err
	}
	return DeepSeekSaveProviderInConfig(path, cfg, providerID, providerCfg, fingerprint)
}

func DeepSeekSaveProviderInConfig(path string, cfg map[string]interface{}, providerID string, providerCfg map[string]interface{}, fingerprint string) error {
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	src, _ := deepSeekProviderSource(cfg, providerID)
	llmMode := isDeepSeekLLMMode(cfg)
	if src == "llm" {
		llmMode = true
	} else if src == "provider" {
		llmMode = false
	}
	if llmMode {
		llm, _ := cfg["llm-pi-ai"].(map[string]interface{})
		if llm == nil {
			llm = map[string]interface{}{}
			cfg["llm-pi-ai"] = llm
		}
		providers, _ := llm["providers"].(map[string]interface{})
		if providers == nil {
			providers = map[string]interface{}{}
			llm["providers"] = providers
		}
		opts, _ := providerCfg["options"].(map[string]interface{})
		baseURL, _ := opts["baseURL"].(string)
		apiKey, _ := opts["apiKey"].(string)
		modelsMap, _ := providerCfg["models"].(map[string]interface{})
		kind, _ := providerCfg["kind"].(string)
		uiKind := ConfigKindToUIKind(kind, baseURL)
		entry := map[string]interface{}{}
		if existing, ok := providers[providerID].(map[string]interface{}); ok {
			entry = deepCopyMap(existing)
		}
		// Preserve the original array order; new cards are appended deterministically.
		ordered := make([]interface{}, 0, len(modelsMap))
		seen := map[string]bool{}
		if old, ok := entry["models"].([]interface{}); ok {
			for _, value := range old {
				mm, _ := value.(map[string]interface{})
				mid := strings.TrimSpace(stringOr(mm["id"], ""))
				if mid == "" || seen[mid] {
					continue
				}
				if next, ok := modelsMap[mid].(map[string]interface{}); ok {
					ordered = append(ordered, dshModelToRaw(CfgToCard(mid, next), uiKind))
					seen[mid] = true
				}
			}
		}
		ids := make([]string, 0, len(modelsMap))
		for mid := range modelsMap {
			if !seen[mid] {
				ids = append(ids, mid)
			}
		}
		sort.Strings(ids)
		for _, mid := range ids {
			mm, _ := modelsMap[mid].(map[string]interface{})
			ordered = append(ordered, dshModelToRaw(CfgToCard(mid, mm), uiKind))
		}
		entry["displayName"] = providerCfg["name"]
		entry["baseURL"] = baseURL
		envName, _ := entry["apiKeyEnv"].(string)
		if strings.TrimSpace(apiKey) != "" {
			if strings.TrimSpace(envName) == "" {
				envName = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(providerID, "-", "_"), ":", "_")) + "_API_KEY"
			}
			if err := saveDeepSeekCredential(envName, apiKey); err != nil {
				return err
			}
			entry["apiKeyEnv"] = envName
			delete(entry, "apiKey")
		} else if strings.TrimSpace(envName) == "" {
			delete(entry, "apiKey")
		}
		if uiKind == "anthropic" {
			entry["api"] = "anthropic-messages"
		} else if uiKind == "responses" {
			entry["api"] = "openai-responses"
		} else {
			entry["api"] = "openai-completions"
		}
		entry["models"] = ordered
		providers[providerID] = entry
	} else {
		providers, _ := cfg["provider"].(map[string]interface{})
		if providers == nil {
			providers = map[string]interface{}{}
			cfg["provider"] = providers
		}
		providers[providerID] = deepCopyMap(providerCfg)
	}
	return writeDeepSeekConfig(path, cfg, fingerprint)
}

func dshModelToRaw(card ModelCard, _ string) map[string]interface{} {
	m := deepCopyMap(card.RawCfg)
	if m == nil {
		m = map[string]interface{}{}
	}
	delete(m, "_dsh_raw")
	delete(m, "_opencode_raw")
	delete(m, "reasoning")
	delete(m, "limit")
	delete(m, "modalities")
	delete(m, "zcode")
	delete(m, "source")
	delete(m, "kind")
	if rawModalities, ok := card.RawCfg["modalities"].(map[string]interface{}); ok {
		if input, ok := rawModalities["input"].([]interface{}); ok && len(input) > 0 {
			m["input"] = input
		}
	}
	delete(m, "id")
	delete(m, "name")
	m["id"] = card.ModelID
	if card.Name != "" && card.Name != card.ModelID {
		m["name"] = card.Name
	}
	_, nativeDSH := card.RawCfg["_dsh_raw"]
	if nativeDSH && card.Reasoning && len(card.Variants) > 0 {
		re := map[string]interface{}{}
		for _, v := range card.Variants {
			v = strings.ToLower(strings.TrimSpace(v))
			if v != "off" && v != "low" && v != "high" && v != "max" {
				continue
			}
			if v == "off" {
				re[v] = nil
			} else {
				re[v] = v
			}
		}
		if len(re) > 1 {
			m["reasoningEfforts"] = re
		} else {
			m["reasoningEfforts"] = false
		}
	} else if nativeDSH && card.Reasoning {
		m["reasoningEfforts"] = map[string]interface{}{"off": nil, "high": "high", "max": "max"}
	} else if nativeDSH {
		m["reasoningEfforts"] = false
	} else {
		delete(m, "reasoningEfforts")
	}
	if card.Context != nil || card.Output != nil {
		ctx := ParseTokens(card.Context)
		out := ParseTokens(card.Output)
		if ctx != nil {
			m["contextWindow"] = *ctx
		}
		if out != nil {
			m["maxTokens"] = *out
		}
	}
	return m
}

func writeDeepSeekConfig(path string, cfg map[string]interface{}, fingerprint string) error {
	format := "json"
	if isYAMLPath(path) {
		format = "yaml"
	}
	dir := filepath.Dir(path)
	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	var data []byte
	var err error
	if format == "yaml" {
		data, err = yaml.Marshal(cfg)
		if err != nil {
			return err
		}
	} else {
		data, err = json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	}
	d := dir
	if d == "" {
		d = "."
	}
	tmpFile, err := os.CreateTemp(d, filepath.Base(path)+".tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = tmpFile.Sync()
	tmpFile.Close()
	if fingerprint != "" {
		cur, _ := FileFingerprint(path)
		if cur != fingerprint {
			_ = os.Remove(tmpName)
			return fmt.Errorf("配置文件在操作期间被其他程序修改，请重试")
		}
	}
	return os.Rename(tmpName, path)
}

func saveDeepSeekCredential(envName, apiKey string) error {
	credsPath := DeepSeekCredentialsPath()
	dir := filepath.Dir(credsPath)
	_ = os.MkdirAll(dir, 0700)
	var existing map[string]interface{}
	data, err := os.ReadFile(credsPath)
	if err == nil && len(data) > 0 {
		_ = yaml.Unmarshal(data, &existing)
	}
	if existing == nil {
		existing = map[string]interface{}{}
	}
	if refs, ok := existing["refs"].(map[string]interface{}); ok {
		refs[envName] = apiKey
	} else {
		existing[envName] = apiKey
	}
	out, err := yaml.Marshal(existing)
	if err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp(dir, ".credentials.tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(out); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = tmpFile.Sync()
	tmpFile.Close()
	_ = os.Chmod(tmpName, 0600)
	return os.Rename(tmpName, credsPath)
}
