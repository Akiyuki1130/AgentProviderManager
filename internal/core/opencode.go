package core

import (
	"fmt"
	"os"
	"strings"
)

func ConvertOpencodeModel(mid string, modelCfg map[string]interface{}, kind string) map[string]interface{} {
	if modelCfg == nil {
		modelCfg = map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if name, ok := modelCfg["name"].(string); ok && strings.TrimSpace(name) != "" && strings.TrimSpace(name) != mid {
		n := strings.TrimSpace(name)
		if len(n) > MaxProviderNameLen {
			n = n[:MaxProviderNameLen]
		}
		out["name"] = n
	}
	reasoningCfg := modelCfg["reasoning"]
	variantsCfg, _ := modelCfg["variants"].(map[string]interface{})
	optionsCfg, _ := modelCfg["options"].(map[string]interface{})
	hasThinking := false
	if optionsCfg != nil {
		if thinking, ok := optionsCfg["thinking"].(map[string]interface{}); ok && thinking["type"] != nil {
			hasThinking = true
		}
	}
	explicit := []string{}
	if variantsCfg != nil {
		for key, value := range variantsCfg {
			if !ZCodeVariantSet[key] {
				continue
			}
			if m, ok := value.(map[string]interface{}); ok && m["disabled"] == true {
				continue
			}
			translated := key
			if v, ok := OpencodeEffortToVariant[key]; ok {
				translated = v
			}
			found := false
			for _, e := range explicit {
				if e == translated {
					found = true
					break
				}
			}
			if !found {
				explicit = append(explicit, translated)
			}
		}
	}
	reasoningEnabled := len(explicit) > 0 || reasoningCfg == true || hasThinking
	if m, ok := reasoningCfg.(map[string]interface{}); ok && m["enabled"] != false {
		if m["enabled"] != false && (reasoningCfg != nil) {
			// handles reasoning: {enabled: true/false}
		}
		if reasoningCfg != nil {
			if mm, ok := reasoningCfg.(map[string]interface{}); ok && mm["enabled"] != false {
				reasoningEnabled = reasoningEnabled || true
			}
		}
	}
	if len(explicit) > 0 || reasoningCfg == true || hasThinking {
		reasoningEnabled = true
	}
	if mm, ok := reasoningCfg.(map[string]interface{}); ok && mm["enabled"] == false {
		reasoningEnabled = len(explicit) > 0 || hasThinking
	}
	if reasoningEnabled {
		var variants []string
		if len(explicit) > 0 {
			for _, v := range explicit {
				if containsStr(OpencodeEffortOrder, v) || v == "off" {
					variants = append(variants, v)
				}
			}
			// off first
			var offPart, rest []string
			for _, v := range variants {
				if v == "off" {
					offPart = append(offPart, v)
				} else {
					rest = append(rest, v)
				}
			}
			variants = append(offPart, rest...)
			if len(variants) == 0 {
				variants = DefaultVariantsFor(kind)
			}
		} else {
			variants = DefaultVariantsFor(kind)
		}
		// pick default
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
	if limit, ok := modelCfg["limit"].(map[string]interface{}); ok {
		ctx := ParseTokens(limit["context"])
		outTok := ParseTokens(limit["output"])
		if ctx != nil && outTok != nil {
			out["limit"] = map[string]interface{}{"context": *ctx, "output": *outTok}
		}
	}
	return out
}

func InferOpencodeKind(baseURL, npm string) string {
	kind := InferKind(baseURL)
	if kind != "anthropic" && npm != "" && strings.Contains(strings.ToLower(npm), "anthropic") {
		kind = "anthropic"
	}
	return kind
}

func ConvertOpencodeProvider(providerID string, raw map[string]interface{}, kind *string) (string, map[string]interface{}, error) {
	pid := strings.TrimSpace(providerID)
	if !ProviderIDValid(pid) {
		return "", nil, fmt.Errorf("Provider ID「%s」只能包含字母、数字、下划线、短横线或冒号", providerID)
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	name := pid
	if n, ok := raw["name"].(string); ok && strings.TrimSpace(n) != "" {
		name = n
	}
	if len(name) > MaxProviderNameLen {
		name = name[:MaxProviderNameLen]
	}
	options, _ := raw["options"].(map[string]interface{})
	if options == nil {
		options = map[string]interface{}{}
	}
	baseURL, _ := options["baseURL"].(string)
	apiKey, _ := options["apiKey"].(string)
	npm, _ := raw["npm"].(string)
	resolvedKind := ""
	if kind != nil {
		resolvedKind = *kind
	} else {
		resolvedKind = InferOpencodeKind(baseURL, npm)
		if rawKind, ok := raw["kind"].(string); ok && strings.TrimSpace(rawKind) != "" {
			resolvedKind = ConfigKindToUIKind(strings.TrimSpace(rawKind), baseURL)
		}
	}
	kindNorm, err := ValidateKind(resolvedKind)
	if err != nil {
		return "", nil, err
	}
	extra := map[string]interface{}{}
	for k, v := range options {
		if k != "baseURL" && k != "apiKey" {
			extra[k] = v
		}
	}
	models := map[string]interface{}{}
	if rawModels, ok := raw["models"].(map[string]interface{}); ok {
		for mid, modelCfg := range rawModels {
			mid = strings.TrimSpace(mid)
			if mid == "" {
				return "", nil, fmt.Errorf("Provider「%s」存在空的模型 ID", pid)
			}
			if len(mid) > MaxModelIDLength {
				return "", nil, fmt.Errorf("Provider「%s」的模型 ID「%s」过长", pid, ShortText(mid, 40))
			}
			mm, _ := modelCfg.(map[string]interface{})
			models[mid] = ConvertOpencodeModel(mid, mm, kindNorm)
		}
		if len(models) > MaxModels {
			return "", nil, fmt.Errorf("Provider「%s」的模型数量不能超过 %d 个", pid, MaxModels)
		}
	}
	opts, err := BuildOptions(baseURL, apiKey, strings.TrimSpace(apiKey) != "", extra)
	if err != nil {
		// If baseURL is empty, BuildOptions will fail; use minimal options
		opts = map[string]interface{}{"baseURL": baseURL}
		if strings.TrimSpace(apiKey) != "" {
			opts["apiKey"] = strings.TrimSpace(apiKey)
			opts["apiKeyRequired"] = true
		}
		for k, v := range extra {
			opts[k] = v
		}
	}
	cfg := map[string]interface{}{
		"name":    name,
		"kind":    KindToConfig[kindNorm],
		"source":  "custom",
		"options": opts,
		"models":  models,
	}
	return pid, cfg, nil
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
	if providers == nil || len(providers) == 0 {
		item.Error = "该文件中没有可导入的 provider 配置"
		return item
	}
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
		apiKey, _ := opts["apiKey"].(string)
		hasKey := strings.TrimSpace(apiKey) != ""
		modelCount := 0
		if models, ok := m["models"].(map[string]interface{}); ok {
			modelCount = len(models)
		}
		kind := "openai-compatible"
		if baseURL != "" {
			kind = InferOpencodeKind(baseURL, "")
			if npm, ok := m["npm"].(string); ok {
				kind = InferOpencodeKind(baseURL, npm)
			}
		}
		item.Providers = append(item.Providers, ProviderSummary{
			ID: pid, Name: stringOr(m["name"], pid), Kind: kind,
			BaseURL: baseURL, HasAPIKey: hasKey, ModelCount: modelCount,
		})
	}
	// sort
	for i := 0; i < len(item.Providers); i++ {
		for j := i + 1; j < len(item.Providers); j++ {
			if strings.ToLower(item.Providers[j].ID) < strings.ToLower(item.Providers[i].ID) {
				item.Providers[i], item.Providers[j] = item.Providers[j], item.Providers[i]
			}
		}
	}
	return item
}

func ImportOpencodeProviders(zcodeConfig, opencodeConfig map[string]interface{}, selectedIDs []string, merge bool) (map[string]interface{}, []string, []string, error) {
	// dedup
	seen := map[string]bool{}
	var deduped []string
	for _, id := range selectedIDs {
		if !seen[id] {
			seen[id] = true
			deduped = append(deduped, id)
		}
	}
	result := deepCopyMap(zcodeConfig)
	if _, ok := result["provider"]; !ok {
		result["provider"] = map[string]interface{}{}
	}
	targetProviders, _ := result["provider"].(map[string]interface{})
	if targetProviders == nil {
		targetProviders = map[string]interface{}{}
		result["provider"] = targetProviders
	}
	opencodeProviders, _ := opencodeConfig["provider"].(map[string]interface{})
	if opencodeProviders == nil {
		return nil, nil, nil, fmt.Errorf("opencode 配置中没有 provider")
	}
	var imported, merged []string
	for _, pid := range deduped {
		raw, ok := opencodeProviders[pid]
		if !ok {
			return nil, nil, nil, fmt.Errorf("提供商「%s」不存在于 opencode 配置", pid)
		}
		m, _ := raw.(map[string]interface{})
		newPID, cfg, err := ConvertOpencodeProvider(pid, m, nil)
		if err != nil {
			return nil, nil, nil, err
		}
		if _, exists := targetProviders[newPID]; exists {
			if merge {
				mergedCfg, err := MergeProviderIntoConfig(map[string]interface{}{"provider": map[string]interface{}{newPID: targetProviders[newPID]}}, newPID, cfg, true)
				if err != nil {
					return nil, nil, nil, err
				}
				targetProviders[newPID] = mergedCfg["provider"].(map[string]interface{})[newPID]
				merged = append(merged, newPID)
			} else {
				targetProviders[newPID] = cfg
				imported = append(imported, newPID)
			}
		} else {
			targetProviders[newPID] = cfg
			imported = append(imported, newPID)
		}
	}
	return result, imported, merged, nil
}

func SanitizeZCodeProvider(providerID string, raw map[string]interface{}) (string, map[string]interface{}, error) {
	pid := strings.TrimSpace(providerID)
	if !ProviderIDValid(pid) {
		return "", nil, fmt.Errorf("Provider ID「%s」只能包含字母、数字、下划线、短横线或冒号", providerID)
	}
	if raw == nil {
		return "", nil, fmt.Errorf("提供商「%s」的配置格式错误（期望 JSON 对象）", pid)
	}
	cfg := deepCopyMap(raw)
	opts, _ := cfg["options"].(map[string]interface{})
	baseURL := ""
	if opts != nil {
		baseURL, _ = opts["baseURL"].(string)
	}
	kind, _ := cfg["kind"].(string)
	if strings.TrimSpace(kind) == "" {
		kind = InferKind(baseURL)
	} else {
		kind = ConfigKindToUIKind(strings.TrimSpace(kind), baseURL)
	}
	cfg["kind"] = KindToConfig[kind]
	if _, ok := cfg["options"].(map[string]interface{}); !ok {
		cfg["options"] = map[string]interface{}{}
	}
	if _, ok := cfg["models"].(map[string]interface{}); !ok {
		cfg["models"] = map[string]interface{}{}
	}
	return pid, cfg, nil
}

func ImportZCodeProviders(zcodeConfig, sourceConfig map[string]interface{}, selectedIDs []string, merge bool) (map[string]interface{}, []string, []string, error) {
	seen := map[string]bool{}
	var deduped []string
	for _, id := range selectedIDs {
		if !seen[id] {
			seen[id] = true
			deduped = append(deduped, id)
		}
	}
	result := deepCopyMap(zcodeConfig)
	if _, ok := result["provider"]; !ok {
		result["provider"] = map[string]interface{}{}
	}
	targetProviders, _ := result["provider"].(map[string]interface{})
	if targetProviders == nil {
		targetProviders = map[string]interface{}{}
		result["provider"] = targetProviders
	}
	sourceProviders, _ := sourceConfig["provider"].(map[string]interface{})
	if sourceProviders == nil {
		return nil, nil, nil, fmt.Errorf("源配置中没有 provider")
	}
	var imported, merged []string
	for _, pid := range deduped {
		raw, ok := sourceProviders[pid]
		if !ok {
			return nil, nil, nil, fmt.Errorf("提供商「%s」不存在于源配置", pid)
		}
		m, _ := raw.(map[string]interface{})
		if m == nil {
			return nil, nil, nil, fmt.Errorf("提供商「%s」的配置格式错误（期望 JSON 对象）", pid)
		}
		newPID, cfg, err := SanitizeZCodeProvider(pid, m)
		if err != nil {
			return nil, nil, nil, err
		}
			if _, exists := targetProviders[newPID]; exists {
				if merge {
					mergedCfg, err := MergeProviderIntoConfig(map[string]interface{}{"provider": map[string]interface{}{newPID: targetProviders[newPID]}}, newPID, cfg, true)
					if err != nil {
						return nil, nil, nil, err
					}
					targetProviders[newPID] = mergedCfg["provider"].(map[string]interface{})[newPID]
					merged = append(merged, newPID)
				} else {
					targetProviders[newPID] = cfg
					imported = append(imported, newPID)
				}
			} else {
				targetProviders[newPID] = cfg
				imported = append(imported, newPID)
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
	if providers == nil || len(providers) == 0 {
		item.Error = "该文件中没有可导入的 provider 配置"
		return item
	}
	item.Providers = BuildProviderSummary(cfg)
	return item
}

func isNotExist(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

func stringOr(v interface{}, fallback string) string {
	if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return fallback
}
