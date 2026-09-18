package zcodeprovider

import (
	"fmt"
	"sort"
	"strings"
)

// legacyProviderKnownKeys 是 FromLegacy 能映射的 legacy provider 顶层键。
// 其余键（source、zcode、apiKeyRequired 之外的额外 options 等）都进 DroppedFields。
var legacyProviderKnownKeys = map[string]bool{
	"id":      true, // provider 映射的键本身即 ID，个别配置会冗余带上，不算丢失
	"name":    true,
	"kind":    true,
	"options": true,
	"models":  true,
}

var legacyOptionKnownKeys = map[string]bool{
	"baseURL": true,
	"apiKey":  true,
}

var legacyModelKnownKeys = map[string]bool{
	"limit":      true,
	"reasoning":  true,
	"modalities": true,
}

var legacyLimitKnownKeys = map[string]bool{
	"context": true,
	"output":  true,
}

var legacyReasoningKnownKeys = map[string]bool{
	"variants": true,
}

var legacyModalitiesKnownKeys = map[string]bool{
	"input": true,
}

// FromLegacy 把旧版 ZCode config.json 的 provider 映射转换为 v2 个人配置。
//
// 返回值第二个元素是无法映射的字段清单，形如 "providerID:field"（点分路径），
// 已排序去重，供调用方向用户提示“这些字段在转换后会丢失”。
func FromLegacy(legacy map[string]interface{}) (*Config, []string, error) {
	cfg := NewConfig()
	if legacy == nil {
		return cfg, nil, nil
	}
	rawProviders, hasProviders := legacy["provider"]
	if !hasProviders || rawProviders == nil {
		return cfg, nil, nil
	}
	providers, ok := rawProviders.(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("legacy 配置中的 provider 字段格式错误（应为对象）")
	}
	ids := sortedStringKeys(providers)
	// legacy 的 provider 是无序映射：这里按 ID 排序写入，保证输出稳定。
	// providerOrder 显式列出；config.group 一律为 standard-personal。
	cfg.SetProviderOrder(ids)

	var dropped []string
	for _, pid := range ids {
		raw, ok := providers[pid].(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("提供商「%s」的配置格式错误（应为对象）", pid)
		}
		rule, modelRules, err := providerRuleFromLegacy(pid, raw)
		if err != nil {
			return nil, nil, err
		}
		cfg.UpsertProviderRule(rule)
		for _, mr := range modelRules {
			cfg.UpsertProviderModelRule(mr)
		}
		for _, field := range DroppedFields(raw) {
			dropped = append(dropped, pid+":"+field)
		}
	}
	sort.Strings(dropped)
	return cfg, dedupeSorted(dropped), nil
}

func providerRuleFromLegacy(pid string, raw map[string]interface{}) (map[string]interface{}, []map[string]interface{}, error) {
	name := stringValue(raw["name"])
	if strings.TrimSpace(name) == "" {
		name = pid
	}
	apiType, err := apiTypeFromLegacyKind(stringValue(raw["kind"]))
	if err != nil {
		return nil, nil, fmt.Errorf("提供商「%s」：%v", pid, err)
	}
	opts, _ := raw["options"].(map[string]interface{})
	baseURL := stringValue(opts["baseURL"])

	rule := NewProviderRule(pid, name, GroupStandardPersonal)
	providerCfg := RuleConfig(rule)
	providerCfg["api"] = NewAPI(apiType, baseURL)
	if apiKey := stringValue(opts["apiKey"]); strings.TrimSpace(apiKey) != "" {
		providerCfg["access"] = NewAPIKeyAccess(apiKey)
	}

	var modelRules []map[string]interface{}
	if modelsRaw, ok := raw["models"]; ok {
		models, isMap := modelsRaw.(map[string]interface{})
		if !isMap && modelsRaw != nil {
			return nil, nil, fmt.Errorf("提供商「%s」的 models 字段格式错误（应为对象）", pid)
		}
		mids := sortedStringKeys(models)
		providerCfg["personalModelIds"] = toInterfaceSlice(mids)
		for _, mid := range mids {
			mraw, _ := models[mid].(map[string]interface{})
			if mraw == nil {
				continue
			}
			modelRules = append(modelRules, modelRuleFromLegacy(pid, mid, mraw))
		}
	}
	return rule, modelRules, nil
}

func modelRuleFromLegacy(pid, mid string, mraw map[string]interface{}) map[string]interface{} {
	rule := NewProviderModelRule(pid, mid)
	mcfg := RuleConfig(rule)

	properties := map[string]interface{}{}
	optionSpecs := map[string]interface{}{}

	limit, _ := mraw["limit"].(map[string]interface{})
	if n, ok := positiveIntValue(limit["context"]); ok {
		properties["contextWindow"] = n
	}
	if n, ok := positiveIntValue(limit["output"]); ok {
		optionSpecs["maxOutputTokens"] = map[string]interface{}{"max": n}
	}

	if values := legacyReasoningVariants(mraw); len(values) > 0 {
		optionSpecs["reasoningLevel"] = map[string]interface{}{"values": toInterfaceSlice(values)}
	}

	if format := legacyInputFormat(mraw); len(format) > 0 {
		properties["inputFormat"] = format
	}

	if len(properties) > 0 {
		mcfg["properties"] = properties
	}
	if len(optionSpecs) > 0 {
		mcfg["optionSpecs"] = optionSpecs
	}
	return rule
}

// legacyReasoningVariants 把 legacy 的 reasoning.variants 映射为 reasoningLevel.values。
//
// ZCode 代码里的 resolveLegacyReasoningLevel 会把新值 "disabled" 映射回旧的 level，
// 且内建数据的取值集合是 ["disabled","enabled"] / ["low","high","max"]，
// 因此这里必须把旧值 "off" 改名为 "disabled"。
func legacyReasoningVariants(mraw map[string]interface{}) []string {
	reasoning, _ := mraw["reasoning"].(map[string]interface{})
	if reasoning == nil {
		return nil
	}
	raw := stringSlice(reasoning["variants"])
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, v := range raw {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "off" {
			v = "disabled"
		}
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// legacyInputFormat 只映射映射表明确给出的 image / video，
// 不推断 text/audio/pdf，避免编造用户数据。
func legacyInputFormat(mraw map[string]interface{}) map[string]interface{} {
	modalities, _ := mraw["modalities"].(map[string]interface{})
	if modalities == nil {
		return nil
	}
	format := map[string]interface{}{}
	for _, modality := range stringSlice(modalities["input"]) {
		switch strings.ToLower(strings.TrimSpace(modality)) {
		case "image":
			format["supportsImage"] = true
		case "video":
			format["supportsVideo"] = true
		}
	}
	return format
}

// ToLegacy 把 v2 个人配置转换为旧版 config.json 文档形状 {"provider": {...}}。
// 该转换是有损的：providerOrder、modelOrder、visibility、logo、headers 等
// 在 legacy schema 中没有对应位置，会被丢弃。
func ToLegacy(c *Config) (map[string]interface{}, error) {
	if c == nil || c.doc == nil {
		return nil, fmt.Errorf("Config 为 nil")
	}
	byProvider := map[string][]map[string]interface{}{}
	for _, mr := range c.ProviderModelRules() {
		pid := ModelRuleProviderID(mr)
		byProvider[pid] = append(byProvider[pid], mr)
	}

	providers := map[string]interface{}{}
	for _, rule := range c.ProviderRules() {
		pid := RuleProviderID(rule)
		if pid == "" {
			continue
		}
		providerCfg := RuleConfig(rule)
		kind, err := legacyKindFromAPIType(ProviderAPIType(providerCfg))
		if err != nil {
			return nil, fmt.Errorf("提供商「%s」：%v", pid, err)
		}
		name := RuleProviderName(rule)
		if strings.TrimSpace(name) == "" {
			name = pid
		}
		options := map[string]interface{}{}
		if api := ProviderAPI(providerCfg); api != nil {
			if baseURL, ok := api["baseUrl"].(string); ok {
				options["baseURL"] = baseURL
			}
		}
		if ProviderAccessType(providerCfg) == AccessTypeAPIKey {
			if apiKey := ProviderAccessAPIKey(providerCfg); apiKey != "" {
				options["apiKey"] = apiKey
			}
		}
		models := map[string]interface{}{}
		for _, mr := range byProvider[pid] {
			mid := ModelRuleModelID(mr)
			if mid == "" {
				continue
			}
			models[mid] = legacyModelConfig(ModelRuleConfig(mr))
		}
		providers[pid] = map[string]interface{}{
			"name":    name,
			"kind":    kind,
			"options": options,
			"models":  models,
		}
	}
	return map[string]interface{}{"provider": providers}, nil
}

func legacyModelConfig(mcfg map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	limit := map[string]interface{}{}
	if n, ok := ModelContextWindow(mcfg); ok {
		limit["context"] = n
	}
	if n, ok := ModelMaxOutputTokens(mcfg); ok {
		limit["output"] = n
	}
	if len(limit) > 0 {
		out["limit"] = limit
	}
	if values := ModelReasoningLevelValues(mcfg); len(values) > 0 {
		variants := make([]interface{}, 0, len(values))
		for _, v := range values {
			if v == "disabled" {
				v = "off"
			}
			variants = append(variants, v)
		}
		out["reasoning"] = map[string]interface{}{"variants": variants}
	}
	if format := ModelInputFormat(mcfg); format != nil {
		var inputs []interface{}
		if b, _ := format["supportsImage"].(bool); b {
			inputs = append(inputs, "image")
		}
		if b, _ := format["supportsVideo"].(bool); b {
			inputs = append(inputs, "video")
		}
		if len(inputs) > 0 {
			out["modalities"] = map[string]interface{}{"input": inputs}
		}
	}
	return out
}

// DroppedFields 返回 legacy provider 中无法映射进 v2 的字段名（点分路径，排序去重）。
// 未知对象会展开到叶子，例如顶层 zcode 对象会产生 zcode.modified、zcode.priority；
// options 里的额外项产生 options.apiKeyRequired；每个模型下产生 models.<id>.<field>。
func DroppedFields(legacyProvider map[string]interface{}) []string {
	if legacyProvider == nil {
		return nil
	}
	set := map[string]bool{}
	collectUnknown(set, "", legacyProvider, legacyProviderKnownKeys)

	if options, ok := legacyProvider["options"].(map[string]interface{}); ok {
		collectUnknown(set, "options", options, legacyOptionKnownKeys)
	}
	if models, ok := legacyProvider["models"].(map[string]interface{}); ok {
		for mid, raw := range models {
			mraw, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			prefix := "models." + mid
			collectUnknown(set, prefix, mraw, legacyModelKnownKeys)
			if limit, ok := mraw["limit"].(map[string]interface{}); ok {
				collectUnknown(set, prefix+".limit", limit, legacyLimitKnownKeys)
			}
			if reasoning, ok := mraw["reasoning"].(map[string]interface{}); ok {
				collectUnknown(set, prefix+".reasoning", reasoning, legacyReasoningKnownKeys)
			}
			if modalities, ok := mraw["modalities"].(map[string]interface{}); ok {
				collectUnknown(set, prefix+".modalities", modalities, legacyModalitiesKnownKeys)
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// collectUnknown 收集 m 中不在 known 里的键；遇到未知对象会继续展开到叶子，
// 便于精确指出丢失位置（如 zcode.modified）。known 为 nil 表示全部视为未知。
func collectUnknown(set map[string]bool, prefix string, m map[string]interface{}, known map[string]bool) {
	for key, value := range m {
		if known != nil && known[key] {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		if sub, ok := value.(map[string]interface{}); ok && len(sub) > 0 {
			collectUnknown(set, path, sub, nil)
			continue
		}
		set[path] = true
	}
}

func apiTypeFromLegacyKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "openai-compatible":
		return APITypeOpenAIChatCompletions, nil
	case "openai":
		return APITypeOpenAIResponses, nil
	case "responses":
		// 旧别名：core.NormalizeConfigKinds 会把 responses 归一为 openai。
		return APITypeOpenAIResponses, nil
	case "anthropic":
		return APITypeAnthropicMessages, nil
	case "":
		// 与 core.BuildProviderCfgEditor 对空 kind 的默认一致。
		return APITypeOpenAIChatCompletions, nil
	default:
		return "", fmt.Errorf("无法映射的 legacy kind「%s」", kind)
	}
}

func legacyKindFromAPIType(apiType string) (string, error) {
	switch apiType {
	case APITypeOpenAIChatCompletions:
		return "openai-compatible", nil
	case APITypeOpenAIResponses:
		return "openai", nil
	case APITypeAnthropicMessages:
		return "anthropic", nil
	case "":
		return "", fmt.Errorf("缺少 api.type，无法映射为 legacy kind")
	default:
		return "", fmt.Errorf("无法映射的 api.type「%s」", apiType)
	}
}

func sortedStringKeys(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dedupeSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	var last string
	for i, v := range in {
		if i > 0 && v == last {
			continue
		}
		out = append(out, v)
		last = v
	}
	return out
}
