package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentprovidermanager/internal/dshprovider"
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
		// version/refs/records 是 0.2 的布局键（records 是插件的登录记录，不是密钥引用）。
		if k == "version" || k == "refs" || k == "records" {
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
			seen := map[string]bool{}
			for k, v := range m {
				lk := strings.ToLower(strings.TrimSpace(k))
				switch lk {
				case "none", "disabled", "nothink":
					lk = "off"
				case "minimal":
					lk = "low"
				}
				if lk == "off" {
					if !seen[lk] {
						variants = append(variants, lk)
						seen[lk] = true
					}
					continue
				}
				if v == nil {
					continue
				}
				if lk != "low" && lk != "high" && lk != "max" {
					if vv, ok := OpencodeEffortToVariant[lk]; ok {
						lk = vv
					} else {
						continue
					}
				}
				if !seen[lk] {
					variants = append(variants, lk)
					seen[lk] = true
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
				if preferred, ok := modelCfg["reasoningEffort"].(string); ok {
					preferred = canonicalVariant(preferred)
					if containsStr(variants, preferred) {
						defaultVariant = preferred
					}
				}
				if defaultVariant == "" {
					for _, pref := range DefaultVariantPreference {
						if containsStr(variants, pref) {
							defaultVariant = pref
							break
						}
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
		} else if api, ok := raw["api"].(string); ok && strings.TrimSpace(api) != "" {
			// 0.2 的路由用 api 声明协议（openai-completions / openai-responses /
			// anthropic-messages），据此判定 UI 类型。
			resolvedKind = kindFromDeepSeekAPI(api, baseURL)
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
			if len(mid) > MaxModelIDLength {
				return "", nil, fmt.Errorf("Provider「%s」的模型 ID「%s」过长", pid, ShortText(mid, 40))
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
	// 把可透传给 pi-ai 路由（headers、timeoutMs、compat…）的字段交给调用方，
	// 迁移到别处或经编辑器保存时才不会丢。
	optionsJSON := ""
	if extra := deepSeekRouteExtras(opts); len(extra) > 0 {
		if b, merr := json.Marshal(extra); merr == nil {
			optionsJSON = string(b)
		}
	}
	return &ProviderEdit{ID: pid, Name: name, Kind: stringOr(normalized["kind"], "openai-compatible"), BaseURL: baseURL, APIKey: apiKey, APIKeyRequired: apiKey != "", OptionsJSON: optionsJSON, Cards: cards, RawProvider: deepCopyMap(raw)}, nil
}

func DeepSeekImportPreview(path string) ImportPreview {
	if strings.TrimSpace(path) == "" {
		path = DeepSeekSettingsPath()
	}
	item := ImportPreview{Path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return item
	}
	item.Exists = true

	format := DetectDeepSeekFormatAt(path)
	var providers map[string]interface{}
	var err error
	switch format {
	case dshprovider.FormatProfileDesktop, dshprovider.FormatProfileWeb, dshprovider.FormatProfileOther:
		// 0.2：provider 在 profile patch 的 llm-pi-ai 条目里；home 级 patch 覆盖时以它为准。
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			item.Error = fmt.Sprintf("读取配置文件失败：%v", rerr)
			return item
		}
		patch, perr := dshprovider.LoadPatch(data)
		if perr != nil {
			item.Error = perr.Error()
			return item
		}
		providers, err = dshEffectiveProviders(newDeepSeekProfileDoc(path, patch))
	default:
		var cfg map[string]interface{}
		cfg, err = DeepSeekLoadConfig(path)
		if err == nil {
			providers = deepSeekProvidersMap(cfg)
		}
	}
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if len(providers) == 0 {
		item.Error = "该文件中没有可管理的 provider 配置"
		return item
	}
	item.Providers = deepSeekSummariesFromProviders(providers)
	return item
}

// DeepSeekSaveProvider updates a provider in the existing on-disk snapshot.
// It is kept as a compatibility wrapper for callers that only have a path.
func DeepSeekSaveProvider(path string, providerID string, providerCfg map[string]interface{}, fingerprint string) error {
	return DeepSeekSaveProviderWithMerge(path, providerID, providerCfg, fingerprint, false)
}

// DeepSeekSaveProviderWithMerge 是低层写入入口，直接接收已构建好的 providerCfg。
// 它没有 UI 卡片载荷，无法判断“用户显式关闭推理”，因此不会应用
// dshApplyExplicitReasoningOff；生产链路一律走 backend 的 store.Upsert，
// 由后者用 payload 里的卡片补齐该语义。
func DeepSeekSaveProviderWithMerge(path string, providerID string, providerCfg map[string]interface{}, fingerprint string, mergeModels bool) error {
	cfg, err := DeepSeekLoadConfig(path)
	if err != nil {
		return err
	}
	return DeepSeekSaveProviderInConfigWithMerge(path, cfg, providerID, providerCfg, fingerprint, mergeModels)
}

func DeepSeekSaveProviderInConfig(path string, cfg map[string]interface{}, providerID string, providerCfg map[string]interface{}, fingerprint string) error {
	return DeepSeekSaveProviderInConfigWithMerge(path, cfg, providerID, providerCfg, fingerprint, false)
}

func DeepSeekSaveProviderInConfigWithMerge(path string, cfg map[string]interface{}, providerID string, providerCfg map[string]interface{}, fingerprint string, mergeModels bool) error {
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	// 文档变换与 legacy backend 共用同一条路径，避免两处逻辑漂移。
	deepSeekUpsertEntry(cfg, providerID, providerID, providerCfg, mergeModels)
	deepSeekApplyDefaultModel(cfg, deepSeekProvidersMap(cfg))
	if err := flushDeepSeekCredentials(cfg); err != nil {
		return err
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
	// DeepSeek only accepts these four effort names. Keep the source enabled
	// state independently from the selected variants so cross-agent migration
	// does not silently turn reasoning off or on.
	enabled := card.Reasoning
	explicit := false
	if card.RawReasoningEnabled != nil {
		// The editor updates this field whenever the user toggles reasoning, so
		// it preserves both the source state and an explicit UI change.
		enabled = *card.RawReasoningEnabled
		explicit = true
	}
	switch {
	case explicit && !enabled:
		// 显式关闭：DSH 用 reasoningEfforts: false 表示。
		delete(m, "reasoningEffort")
		m["reasoningEfforts"] = false
	case enabled:
		delete(m, "reasoningEffort")
		re := map[string]interface{}{}
		for _, v := range card.Variants {
			v = canonicalVariant(v)
			if v != "off" && v != "low" && v != "high" && v != "max" {
				continue
			}
			if v == "off" {
				re[v] = nil
			} else {
				re[v] = v
			}
		}
		if len(re) == 0 {
			re = map[string]interface{}{"off": nil, "high": "high", "max": "max"}
		}
		m["reasoningEfforts"] = re
		// DeepSeek represents the selected default effort in a separate native
		// field. Keep it when the source card has one, including an explicit off.
		defaultVariant := canonicalVariant(card.DefaultVariant)
		if value, ok := re[defaultVariant]; ok {
			if defaultVariant == "off" {
				m["reasoningEffort"] = "off"
			} else {
				m["reasoningEffort"] = value
			}
		} else {
			delete(m, "reasoningEffort")
		}
	default:
		// 卡片没有任何推理信息（既非显式关闭，也没有档位）：此时不能写
		// reasoningEfforts: false——那是 DSH 的“显式关闭推理”标记，会把原本
		// 没有推理信息的模型静默关掉推理。保持 RawCfg 里已有的推理键不动。
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

// saveDeepSeekCredential 写入一条凭据 ref。
//
// DSH 0.2 的凭据文件是 version: 1 + refs/records 布局：0.1 的扁平顶层键会被
// DSH 拒绝，因此写盘一律走 dshprovider（内部会把扁平布局升级为 version 1，
// 并保留 records 与注释）。
func saveDeepSeekCredential(envName, apiKey string) error {
	return dshprovider.SetCredentialRef(DeepSeekCredentialsPath(), envName, apiKey)
}
