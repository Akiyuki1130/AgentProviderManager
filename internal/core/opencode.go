package core

import (
	"fmt"
	"os"
	"strings"
)

// ConvertOpencodeModel converts an OpenCode model to the internal editor shape
// while retaining every official field for a lossless subsequent save.
func ConvertOpencodeModel(mid string, modelCfg map[string]interface{}, kind string) map[string]interface{} {
	out := deepCopyMap(modelCfg)
	if out == nil {
		out = map[string]interface{}{}
	}
	out["_opencode_raw"] = deepCopyMap(modelCfg)
	if id, ok := out["id"].(string); !ok || strings.TrimSpace(id) == "" {
		out["id"] = mid
	}
	if name, ok := out["name"].(string); ok && strings.TrimSpace(name) == mid {
		delete(out, "name")
	}

	variants := []string{}
	if vm, ok := out["variants"].(map[string]interface{}); ok {
		for key, value := range vm {
			if item, ok := value.(map[string]interface{}); ok && item["disabled"] == true {
				continue
			}
			v := canonicalVariant(key)
			if mapped, ok := OpencodeEffortToVariant[v]; ok {
				v = mapped
			}
			if !ZCodeVariantSet[v] {
				continue
			}
			if !containsStr(variants, v) {
				variants = append(variants, v)
			}
		}
	}
	reasoning, _ := out["reasoning"].(bool)
	if reasoning || len(variants) > 0 {
		if len(variants) == 0 {
			variants = DefaultVariantsFor(kind)
		}
		defaultVariant := ""
		preferredRaw, _ := out["reasoningEffort"].(string)
		if preferredRaw == "" {
			if options, ok := out["options"].(map[string]interface{}); ok {
				preferredRaw, _ = options["reasoningEffort"].(string)
			}
		}
		if preferredRaw != "" {
			preferred := canonicalVariant(preferredRaw)
			if containsStr(variants, preferred) {
				defaultVariant = preferred
			}
		}
		if defaultVariant == "" {
			for _, preferred := range DefaultVariantPreference {
				if containsStr(variants, preferred) {
					defaultVariant = preferred
					break
				}
			}
		}
		if defaultVariant == "" {
			defaultVariant = variants[0]
		}
		out["reasoning"] = map[string]interface{}{"enabled": reasoning, "variants": variants, "defaultVariant": defaultVariant}
	}
	if a, ok := out["attachment"].(bool); ok {
		out["attachment"] = a
	}
	if limit, ok := out["limit"].(map[string]interface{}); ok {
		ctx := ParseTokens(limit["context"])
		outTok := ParseTokens(limit["output"])
		if ctx != nil && outTok != nil {
			out["limit"] = map[string]interface{}{"context": *ctx, "output": *outTok}
		} else {
			delete(out, "limit")
		}
	}
	return out
}

func InferOpencodeKind(baseURL, npm string) string {
	kind := InferKind(baseURL)
	if kind != "anthropic" && strings.Contains(strings.ToLower(npm), "anthropic") {
		kind = "anthropic"
	}
	return kind
}

// ConvertOpencodeProvider converts a native OpenCode provider to the internal
// editor shape. The original provider is retained for native write-back.
func ConvertOpencodeProvider(providerID string, raw map[string]interface{}, kind *string) (string, map[string]interface{}, error) {
	pid := strings.TrimSpace(providerID)
	if !ProviderIDValid(pid) {
		return "", nil, fmt.Errorf("Provider ID「%s」只能包含字母、数字、下划线、短横线或冒号", providerID)
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	name := stringOr(raw["name"], pid)
	opts, _ := raw["options"].(map[string]interface{})
	if opts == nil {
		opts = map[string]interface{}{}
	}
	baseURL := stringOr(opts["baseURL"], "")
	apiKey := stringOr(opts["apiKey"], "")
	npm := stringOr(raw["npm"], "")
	resolved := InferOpencodeKind(baseURL, npm)
	if kind != nil && strings.TrimSpace(*kind) != "" {
		resolved = *kind
	}
	if rk := stringOr(raw["kind"], ""); rk != "" {
		resolved = ConfigKindToUIKind(rk, baseURL)
	}
	kindNorm, err := ValidateKind(resolved)
	if err != nil {
		return "", nil, err
	}
	extra := map[string]interface{}{}
	for k, v := range opts {
		if k != "baseURL" && k != "apiKey" && k != "apiKeyRequired" {
			extra[k] = v
		}
	}
	models := map[string]interface{}{}
	if rawModels, ok := raw["models"].(map[string]interface{}); ok {
		for mid, value := range rawModels {
			mm, _ := value.(map[string]interface{})
			models[mid] = ConvertOpencodeModel(mid, mm, kindNorm)
		}
	}
	optsInternal, err := BuildOptions(baseURL, apiKey, strings.TrimSpace(apiKey) != "", extra)
	if err != nil {
		optsInternal = deepCopyMap(opts)
	}
	cfg := map[string]interface{}{
		"name": name, "kind": KindToConfig[kindNorm], "source": "custom",
		"options": optsInternal, "models": models, "_opencode_raw": deepCopyMap(raw),
	}
	return pid, cfg, nil
}

// OpenCodeModelFromCfg emits only fields accepted by the OpenCode model schema.
func OpenCodeModelFromCfg(modelID string, cfg map[string]interface{}) map[string]interface{} {
	out := deepCopyMap(cfg)
	delete(out, "_opencode_raw")
	delete(out, "source")
	delete(out, "kind")
	delete(out, "reasoningEffort")
	if id, ok := out["id"].(string); !ok || strings.TrimSpace(id) == "" {
		out["id"] = modelID
	}
	if reasoning, ok := out["reasoning"].(map[string]interface{}); ok {
		enabled, _ := reasoning["enabled"].(bool)
		variants := NormalizeVariants(reasoning["variants"])
		out["reasoning"] = enabled
		if len(variants) > 0 {
			vm := map[string]interface{}{}
			for _, v := range variants {
				name := v
				if v == "off" {
					name = "none"
				}
				vm[name] = map[string]interface{}{}
			}
			out["variants"] = vm
			defaultVariant, _ := reasoning["defaultVariant"].(string)
			defaultVariant = canonicalVariant(defaultVariant)
			if !containsStr(variants, defaultVariant) {
				defaultVariant = variants[0]
			}
			if defaultVariant == "off" {
				defaultVariant = "none"
			}
			options, _ := out["options"].(map[string]interface{})
			if options == nil {
				options = map[string]interface{}{}
			}
			options["reasoningEffort"] = defaultVariant
			out["options"] = options
		}
	} else if _, ok := out["reasoning"].(bool); !ok {
		delete(out, "reasoning")
	}
	if limit, ok := out["limit"].(map[string]interface{}); ok {
		ctx := ParseTokens(limit["context"])
		outTok := ParseTokens(limit["output"])
		if ctx != nil && outTok != nil {
			out["limit"] = map[string]interface{}{"context": *ctx, "output": *outTok}
		} else {
			delete(out, "limit")
		}
	}
	return out
}

// OpenCodeProviderFromCfg converts an internal provider to native OpenCode.
func OpenCodeProviderFromCfg(providerCfg map[string]interface{}) map[string]interface{} {
	raw, _ := providerCfg["_opencode_raw"].(map[string]interface{})
	if raw == nil {
		raw, _ = providerCfg["_raw_provider"].(map[string]interface{})
	}
	out := deepCopyMap(raw)
	if out == nil {
		out = map[string]interface{}{}
	}
	name := stringOr(providerCfg["name"], "")
	if name != "" {
		out["name"] = name
	}
	opts, _ := providerCfg["options"].(map[string]interface{})
	if opts == nil {
		opts = map[string]interface{}{}
	}
	kind := ConfigKindToUIKind(stringOr(providerCfg["kind"], ""), stringOr(opts["baseURL"], ""))
	nativeOpts := map[string]interface{}{}
	for k, v := range opts {
		if k != "apiKeyRequired" {
			nativeOpts[k] = v
		}
	}
	out["options"] = nativeOpts
	if kind == "anthropic" {
		out["npm"] = "@ai-sdk/anthropic"
	} else if kind == "responses" {
		out["npm"] = "@ai-sdk/openai"
	} else {
		out["npm"] = "@ai-sdk/openai-compatible"
	}
	models := map[string]interface{}{}
	if mm, ok := providerCfg["models"].(map[string]interface{}); ok {
		for id, value := range mm {
			cfg, _ := value.(map[string]interface{})
			models[id] = OpenCodeModelFromCfg(id, cfg)
		}
	}
	out["models"] = models
	delete(out, "kind")
	delete(out, "source")
	delete(out, "apiKeyRequired")
	return out
}

func OpenCodeConfigFromInternal(cfg map[string]interface{}) map[string]interface{} {
	out := deepCopyMap(cfg)
	providers, _ := out["provider"].(map[string]interface{})
	for id, raw := range providers {
		if m, ok := raw.(map[string]interface{}); ok {
			providers[id] = OpenCodeProviderFromCfg(m)
		}
	}
	return out
}

func OpenCodeSaveProvider(path, providerID string, providerCfg map[string]interface{}, fingerprint string) error {
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	providers, _ := cfg["provider"].(map[string]interface{})
	if providers == nil {
		providers = map[string]interface{}{}
		cfg["provider"] = providers
	}
	providers[providerID] = OpenCodeProviderFromCfg(providerCfg)
	return WriteConfig(path, cfg, fingerprint)
}

func OpencodeImportPreview(path string) ImportPreview {
	item := ImportPreview{Path: path}
	if path == "" || isNotExist(path) {
		return item
	}
	item.Exists = true
	cfg, err := LoadConfig(path)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	providers, _ := cfg["provider"].(map[string]interface{})
	if len(providers) == 0 {
		item.Error = "该文件中没有可导入的 provider 配置"
		return item
	}
	for pid, raw := range providers {
		m, _ := raw.(map[string]interface{})
		if m == nil {
			continue
		}
		opts, _ := m["options"].(map[string]interface{})
		base := stringOr(opts["baseURL"], "")
		key := stringOr(opts["apiKey"], "")
		models, _ := m["models"].(map[string]interface{})
		item.Providers = append(item.Providers, ProviderSummary{ID: pid, Name: stringOr(m["name"], pid), Kind: InferOpencodeKind(base, stringOr(m["npm"], "")), BaseURL: base, HasAPIKey: strings.TrimSpace(key) != "", ModelCount: len(models)})
	}
	return item
}

func SanitizeZCodeProvider(providerID string, raw map[string]interface{}) (string, map[string]interface{}, error) {
	pid := strings.TrimSpace(providerID)
	if !ProviderIDValid(pid) {
		return "", nil, fmt.Errorf("Provider ID 无效")
	}
	if raw == nil {
		return "", nil, fmt.Errorf("提供商配置格式错误")
	}
	return pid, deepCopyMap(raw), nil
}

func ImportOpencodeProviders(zcodeConfig, opencodeConfig map[string]interface{}, selectedIDs []string, merge bool) (map[string]interface{}, []string, []string, error) {
	result := deepCopyMap(zcodeConfig)
	providers, _ := result["provider"].(map[string]interface{})
	if providers == nil {
		providers = map[string]interface{}{}
		result["provider"] = providers
	}
	source, _ := opencodeConfig["provider"].(map[string]interface{})
	if source == nil {
		return nil, nil, nil, fmt.Errorf("opencode 配置中没有 provider")
	}
	seen := map[string]bool{}
	var imported, merged []string
	for _, id := range selectedIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		raw, ok := source[id].(map[string]interface{})
		if !ok {
			return nil, nil, nil, fmt.Errorf("提供商「%s」不存在", id)
		}
		_, converted, err := ConvertOpencodeProvider(id, raw, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		if _, exists := providers[id]; exists && merge {
			old, ok := providers[id].(map[string]interface{})
			if !ok {
				return nil, nil, nil, fmt.Errorf("目标配置中的 provider「%s」格式错误", id)
			}
			old = deepCopyMap(old)
			for k, v := range converted {
				if k != "models" {
					old[k] = deepCopyValue(v)
				}
			}
			oldModels, _ := old["models"].(map[string]interface{})
			if oldModels == nil {
				oldModels = map[string]interface{}{}
			}
			incomingModels, _ := converted["models"].(map[string]interface{})
			for mid, model := range incomingModels {
				oldModels[mid] = deepCopyValue(model)
			}
			old["models"] = oldModels
			providers[id] = old
			merged = append(merged, id)
		} else {
			providers[id] = converted
			imported = append(imported, id)
		}
	}
	return result, imported, merged, nil
}

func ImportZCodeProviders(zcodeConfig, sourceConfig map[string]interface{}, selectedIDs []string, merge bool) (map[string]interface{}, []string, []string, error) {
	result := deepCopyMap(zcodeConfig)
	providers, _ := result["provider"].(map[string]interface{})
	if providers == nil {
		providers = map[string]interface{}{}
		result["provider"] = providers
	}
	source, _ := sourceConfig["provider"].(map[string]interface{})
	if source == nil {
		return nil, nil, nil, fmt.Errorf("源配置中没有 provider")
	}
	seen := map[string]bool{}
	var imported, merged []string
	for _, id := range selectedIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		raw, ok := source[id].(map[string]interface{})
		if !ok {
			return nil, nil, nil, fmt.Errorf("提供商「%s」不存在", id)
		}
		if old, ok := providers[id].(map[string]interface{}); ok && merge {
			for k, v := range raw {
				old[k] = v
			}
			providers[id] = old
			merged = append(merged, id)
		} else {
			providers[id] = deepCopyMap(raw)
			imported = append(imported, id)
		}
	}
	return result, imported, merged, nil
}

func ZCodeConfigMergePreview(path string) ImportPreview {
	item := ImportPreview{Path: path}
	if path == "" || isNotExist(path) {
		return item
	}
	item.Exists = true
	cfg, err := LoadConfig(path)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	providers, _ := cfg["provider"].(map[string]interface{})
	if len(providers) == 0 {
		item.Error = "该文件中没有可导入的 provider 配置"
		return item
	}
	item.Providers = BuildProviderSummary(cfg)
	return item
}

func isNotExist(path string) bool { _, err := os.Stat(path); return os.IsNotExist(err) }
func stringOr(v interface{}, fallback string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}
