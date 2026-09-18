// Package zcodeprovider 实现 ZCode 新版 provider_config.json（个人文件）的纯编解码、
// 格式检测与新旧格式互转，仅依赖标准库。
//
// 设计约束：文档本体用 map[string]interface{} 承载，配合类型化访问器读写，
// 不使用严格结构体。原因是一旦把文档压平进结构体，本包不编辑但 ZCode 合法的字段
// （headers、visibility、logo、builtinModelIds、templateId、enabled、
// optionSpecs.map、inputFormat/outputFormat、apiKeyManagementUrl、
// requiresMfjsToolSchema 等）会在一次保存后被静默抹掉。
//
// 严格校验只影响“允许哪些键/类型”，不改变“保留哪些值”：解码通过后，
// 所有合法键（含本包未建模的合法键）都会原样保留并在编码时写回。
package zcodeprovider

import (
	"encoding/json"
	"math"
)

// SchemaVersionV1 是当前支持的个人 provider_config.json schema 版本。
const SchemaVersionV1 = 1

// provider.config.group 的合法枚举值。
const (
	GroupStandardPersonal = "standard-personal"
	GroupZAIFamily        = "zai-family"
	GroupBigModelFamily   = "bigmodel-family"
)

// access.type 的合法枚举值。
const (
	AccessTypeAPIKey                = "api-key"
	AccessTypeZhipuCodingPlanAPIKey = "zhipu-coding-plan-api-key"
	AccessTypeZhipuAccount          = "zhipu-account"
)

// api.type 的合法枚举值（恰好三个）。
const (
	APITypeAnthropicMessages     = "anthropic-messages"
	APITypeOpenAIChatCompletions = "openai-chat-completions"
	APITypeOpenAIResponses       = "openai-responses"
)

// canonical 键集（用于 zod .strict() 等价的未知键拒绝）。
var (
	topLevelKeys = map[string]bool{
		"schemaVersion": true,
		"config":        true,
	}

	configKeys = map[string]bool{
		"providerOrder":         true,
		"providerConfigRules":   true,
		"modelConfigRules":      true,
		"defaultModelSelection": true,
	}

	providerConfigRulesKeys = map[string]bool{
		"providerRules": true,
	}

	// 注意：内建文件用的是 modelRules，与个人的 providerModelRules 不是同一个
	// schema。这里刻意不包含 modelRules，混用会被当作未知键拒绝。
	modelConfigRulesKeys = map[string]bool{
		"providerModelRules":       true,
		"manualProviderModelRules": true,
	}

	providerRuleKeys = map[string]bool{
		"providerId":   true,
		"providerName": true,
		"templateId":   true,
		"enabled":      true,
		"config":       true,
	}

	providerRuleConfigKeys = map[string]bool{
		"group":            true,
		"logo":             true,
		"access":           true,
		"api":              true,
		"builtinModelIds":  true,
		"personalModelIds": true,
		"modelOrder":       true,
		"visibility":       true,
	}

	// api-key 分支的字段；其余分支的字段逆向未确认，不在此拒绝，原样保留。
	accessAPIKeyKeys = map[string]bool{
		"type":                true,
		"apiKey":              true,
		"apiKeyManagementUrl": true,
	}

	apiKeys = map[string]bool{
		"type":    true,
		"baseUrl": true,
		"headers": true,
	}

	modelRuleKeys = map[string]bool{
		"providerId": true,
		"modelId":    true,
		"config":     true,
	}

	modelRuleConfigKeys = map[string]bool{
		"enabled":     true,
		"properties":  true,
		"optionSpecs": true,
	}

	propertiesKeys = map[string]bool{
		"requiresMfjsToolSchema":        true,
		"contextWindow":                 true,
		"inputFormat":                   true,
		"outputFormat":                  true,
		"supportsToolCall":              true,
		"supportsJsonSchemaOutput":      true,
		"supportsNativeWebSearch":       true,
		"supportsMidConversationSystem": true,
	}

	inputFormatKeys = map[string]bool{
		"supportsText":  true,
		"supportsImage": true,
		"supportsVideo": true,
		"supportsAudio": true,
		"supportsPdf":   true,
	}

	outputFormatKeys = map[string]bool{
		"supportsText": true,
	}

	optionSpecsKeys = map[string]bool{
		"reasoningLevel":  true,
		"maxOutputTokens": true,
	}

	reasoningLevelKeys = map[string]bool{
		"values": true,
		"map":    true,
	}

	maxOutputTokensKeys = map[string]bool{
		"max": true,
		"map": true,
	}
)

// Config 是个人 provider_config.json 文档的薄封装。doc 始终是解码后的原始文档，
// 访问器只在其上读写 canonical 路径，因此未建模的合法字段不会被丢弃。
type Config struct {
	doc map[string]interface{}
}

// NewConfig 返回一份结构完整、可直接编码的空 v2 配置。
func NewConfig() *Config {
	return &Config{doc: map[string]interface{}{
		"schemaVersion": SchemaVersionV1,
		"config": map[string]interface{}{
			"providerConfigRules": map[string]interface{}{
				"providerRules": []interface{}{},
			},
			"modelConfigRules": map[string]interface{}{
				"providerModelRules":       []interface{}{},
				"manualProviderModelRules": []interface{}{},
			},
		},
	}}
}

// NewConfigFromDoc 采用给定文档作为底层存储（不做深拷贝、不校验）。
func NewConfigFromDoc(doc map[string]interface{}) *Config {
	if doc == nil {
		doc = map[string]interface{}{}
	}
	return &Config{doc: doc}
}

// Doc 返回内部文档引用。调用方可直接读写，但需自行维持 canonical 形状；
// 若写入了非法键，Encode 会在编码前拒绝。
func (c *Config) Doc() map[string]interface{} {
	if c == nil {
		return nil
	}
	return c.doc
}

// SchemaVersion 返回 schemaVersion，缺失或非整数时返回 0。
func (c *Config) SchemaVersion() int {
	if c == nil {
		return 0
	}
	if n, ok := toIntValue(c.doc["schemaVersion"]); ok {
		return int(n)
	}
	return 0
}

// SetSchemaVersion 写入 schemaVersion。
func (c *Config) SetSchemaVersion(v int) {
	c.doc["schemaVersion"] = v
}

func (c *Config) configObj() map[string]interface{} {
	m, _ := c.doc["config"].(map[string]interface{})
	return m
}

func (c *Config) ensureConfigObj() map[string]interface{} {
	m, _ := c.doc["config"].(map[string]interface{})
	if m == nil {
		m = map[string]interface{}{}
		c.doc["config"] = m
	}
	return m
}

func (c *Config) ensureChild(parent map[string]interface{}, key string) map[string]interface{} {
	m, _ := parent[key].(map[string]interface{})
	if m == nil {
		m = map[string]interface{}{}
		parent[key] = m
	}
	return m
}

func (c *Config) ensureArray(parent map[string]interface{}, key string) []interface{} {
	arr, ok := parent[key].([]interface{})
	if !ok {
		arr = []interface{}{}
		parent[key] = arr
	}
	return arr
}

// ProviderOrder 返回原始 providerOrder（缺省为 nil）。
func (c *Config) ProviderOrder() []string {
	if c == nil {
		return nil
	}
	return stringSlice(c.configObj()["providerOrder"])
}

// SetProviderOrder 写入 providerOrder。
func (c *Config) SetProviderOrder(ids []string) {
	cfg := c.ensureConfigObj()
	if ids == nil {
		ids = []string{}
	}
	cfg["providerOrder"] = toInterfaceSlice(ids)
}

// ProviderRules 返回 providerConfigRules.providerRules 的条目引用。
func (c *Config) ProviderRules() []map[string]interface{} {
	if c == nil {
		return nil
	}
	rules, _ := c.configObj()["providerConfigRules"].(map[string]interface{})
	if rules == nil {
		return nil
	}
	return mapSlice(rules["providerRules"])
}

// ProviderRule 按 providerId 查找 providerRule，未找到返回 nil。
func (c *Config) ProviderRule(providerID string) map[string]interface{} {
	for _, r := range c.ProviderRules() {
		if RuleProviderID(r) == providerID {
			return r
		}
	}
	return nil
}

// UpsertProviderRule 按 providerId 新增或替换 providerRule。
func (c *Config) UpsertProviderRule(rule map[string]interface{}) {
	if rule == nil {
		return
	}
	cfg := c.ensureConfigObj()
	pcr := c.ensureChild(cfg, "providerConfigRules")
	arr := c.ensureArray(pcr, "providerRules")
	id := RuleProviderID(rule)
	for i, raw := range arr {
		existing, _ := raw.(map[string]interface{})
		if existing != nil && RuleProviderID(existing) == id {
			arr[i] = rule
			return
		}
	}
	pcr["providerRules"] = append(arr, rule)
}

// RemoveProviderRule 删除指定 providerRule，返回是否实际删除。
func (c *Config) RemoveProviderRule(providerID string) bool {
	if c == nil {
		return false
	}
	rules, _ := c.configObj()["providerConfigRules"].(map[string]interface{})
	if rules == nil {
		return false
	}
	arr, ok := rules["providerRules"].([]interface{})
	if !ok {
		return false
	}
	for i, raw := range arr {
		existing, _ := raw.(map[string]interface{})
		if existing != nil && RuleProviderID(existing) == providerID {
			rules["providerRules"] = append(arr[:i:i], arr[i+1:]...)
			return true
		}
	}
	return false
}

// ProviderModelRules 返回 modelConfigRules.providerModelRules 的条目引用。
func (c *Config) ProviderModelRules() []map[string]interface{} {
	if c == nil {
		return nil
	}
	rules, _ := c.configObj()["modelConfigRules"].(map[string]interface{})
	if rules == nil {
		return nil
	}
	return mapSlice(rules["providerModelRules"])
}

// ManualProviderModelRules 返回 manualProviderModelRules 的条目引用。
func (c *Config) ManualProviderModelRules() []map[string]interface{} {
	if c == nil {
		return nil
	}
	rules, _ := c.configObj()["modelConfigRules"].(map[string]interface{})
	if rules == nil {
		return nil
	}
	return mapSlice(rules["manualProviderModelRules"])
}

// ProviderModelRule 按 (providerId, modelId) 查找，未找到返回 nil。
func (c *Config) ProviderModelRule(providerID, modelID string) map[string]interface{} {
	for _, r := range c.ProviderModelRules() {
		if ModelRuleProviderID(r) == providerID && ModelRuleModelID(r) == modelID {
			return r
		}
	}
	return nil
}

// UpsertProviderModelRule 按 (providerId, modelId) 新增或替换。
func (c *Config) UpsertProviderModelRule(rule map[string]interface{}) {
	if rule == nil {
		return
	}
	mcr := c.ensureChild(c.ensureConfigObj(), "modelConfigRules")
	arr := c.ensureArray(mcr, "providerModelRules")
	pid, mid := ModelRuleProviderID(rule), ModelRuleModelID(rule)
	for i, raw := range arr {
		existing, _ := raw.(map[string]interface{})
		if existing != nil && ModelRuleProviderID(existing) == pid && ModelRuleModelID(existing) == mid {
			arr[i] = rule
			return
		}
	}
	mcr["providerModelRules"] = append(arr, rule)
}

// DefaultModelSelection 返回 defaultModelSelection（可能为 nil）。
func (c *Config) DefaultModelSelection() map[string]interface{} {
	if c == nil {
		return nil
	}
	m, _ := c.configObj()["defaultModelSelection"].(map[string]interface{})
	return m
}

// SetDefaultModelSelection 写入 defaultModelSelection。
func (c *Config) SetDefaultModelSelection(v map[string]interface{}) {
	c.ensureConfigObj()["defaultModelSelection"] = v
}

// EffectiveProviderOrder 复刻 ZCode 的运行时语义：先按 providerOrder 取存在的
// provider（列了但不存在的 id 静默丢弃），再把未列出的 provider 追加到末尾。
func (c *Config) EffectiveProviderOrder() []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range c.ProviderOrder() {
		if id == "" || seen[id] || c.ProviderRule(id) == nil {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	for _, r := range c.ProviderRules() {
		id := RuleProviderID(r)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// EffectiveModelOrder 复刻 ZCode 的运行时语义：modelOrder 缺省时由
// personalModelIds 推导顺序。
func EffectiveModelOrder(providerCfg map[string]interface{}) []string {
	if order := ProviderModelOrder(providerCfg); len(order) > 0 {
		return order
	}
	return ProviderPersonalModelIDs(providerCfg)
}

// ---- providerRule 字段访问器 ----

// RuleProviderID 返回 providerRule.providerId。
func RuleProviderID(rule map[string]interface{}) string {
	return stringValue(rule["providerId"])
}

// RuleProviderName 返回 providerRule.providerName。
func RuleProviderName(rule map[string]interface{}) string {
	return stringValue(rule["providerName"])
}

// RuleTemplateID 返回 providerRule.templateId 及其是否存在。
func RuleTemplateID(rule map[string]interface{}) (string, bool) {
	v, ok := rule["templateId"]
	if !ok {
		return "", false
	}
	return stringValue(v), true
}

// RuleEnabled 返回 providerRule.enabled 及其是否存在。
func RuleEnabled(rule map[string]interface{}) (bool, bool) {
	v, ok := rule["enabled"].(bool)
	return v, ok
}

// RuleConfig 返回 providerRule.config。
func RuleConfig(rule map[string]interface{}) map[string]interface{} {
	m, _ := rule["config"].(map[string]interface{})
	return m
}

// ---- provider config 字段访问器 ----

// ProviderGroup 返回 config.group。
func ProviderGroup(providerCfg map[string]interface{}) string {
	return stringValue(providerCfg["group"])
}

// ProviderLogo 返回 config.logo（未建模，原样透传）。
func ProviderLogo(providerCfg map[string]interface{}) interface{} {
	return providerCfg["logo"]
}

// ProviderVisibility 返回 config.visibility（未建模，原样透传）。
func ProviderVisibility(providerCfg map[string]interface{}) interface{} {
	return providerCfg["visibility"]
}

// ProviderAccess 返回 config.access。
func ProviderAccess(providerCfg map[string]interface{}) map[string]interface{} {
	m, _ := providerCfg["access"].(map[string]interface{})
	return m
}

// ProviderAccessType 返回 config.access.type。
func ProviderAccessType(providerCfg map[string]interface{}) string {
	return stringValue(ProviderAccess(providerCfg)["type"])
}

// ProviderAccessAPIKey 返回 config.access.apiKey（api-key 分支）。
func ProviderAccessAPIKey(providerCfg map[string]interface{}) string {
	return stringValue(ProviderAccess(providerCfg)["apiKey"])
}

// ProviderAccessAPIKeyManagementURL 返回 config.access.apiKeyManagementUrl。
func ProviderAccessAPIKeyManagementURL(providerCfg map[string]interface{}) string {
	return stringValue(ProviderAccess(providerCfg)["apiKeyManagementUrl"])
}

// ProviderAPI 返回 config.api。
func ProviderAPI(providerCfg map[string]interface{}) map[string]interface{} {
	m, _ := providerCfg["api"].(map[string]interface{})
	return m
}

// ProviderAPIType 返回 config.api.type。
func ProviderAPIType(providerCfg map[string]interface{}) string {
	return stringValue(ProviderAPI(providerCfg)["type"])
}

// ProviderAPIBaseURL 返回 config.api.baseUrl。
func ProviderAPIBaseURL(providerCfg map[string]interface{}) string {
	return stringValue(ProviderAPI(providerCfg)["baseUrl"])
}

// ProviderAPIHeaders 返回 config.api.headers（未建模，原样透传）。
func ProviderAPIHeaders(providerCfg map[string]interface{}) map[string]interface{} {
	m, _ := ProviderAPI(providerCfg)["headers"].(map[string]interface{})
	return m
}

// ProviderBuiltinModelIDs 返回 config.builtinModelIds。
func ProviderBuiltinModelIDs(providerCfg map[string]interface{}) []string {
	return stringSlice(providerCfg["builtinModelIds"])
}

// ProviderPersonalModelIDs 返回 config.personalModelIds。
func ProviderPersonalModelIDs(providerCfg map[string]interface{}) []string {
	return stringSlice(providerCfg["personalModelIds"])
}

// ProviderModelOrder 返回 config.modelOrder。
func ProviderModelOrder(providerCfg map[string]interface{}) []string {
	return stringSlice(providerCfg["modelOrder"])
}

func (c *Config) providerConfig(providerID string) map[string]interface{} {
	return RuleConfig(c.ProviderRule(providerID))
}

// SetProviderName 写入 providerRule.providerName。
func (c *Config) SetProviderName(providerID, name string) bool {
	rule := c.ProviderRule(providerID)
	if rule == nil {
		return false
	}
	rule["providerName"] = name
	return true
}

// SetProviderGroup 写入 providerRule.config.group。
func (c *Config) SetProviderGroup(providerID, group string) bool {
	cfg := c.providerConfig(providerID)
	if cfg == nil {
		return false
	}
	cfg["group"] = group
	return true
}

// SetProviderAPI 写入 providerRule.config.api（type 与 baseUrl）。
func (c *Config) SetProviderAPI(providerID, apiType, baseURL string) bool {
	cfg := c.providerConfig(providerID)
	if cfg == nil {
		return false
	}
	api := c.ensureChild(cfg, "api")
	api["type"] = apiType
	api["baseUrl"] = baseURL
	return true
}

// SetProviderAPIKeyAccess 写入 providerRule.config.access（api-key 分支）。
func (c *Config) SetProviderAPIKeyAccess(providerID, apiKey string) bool {
	cfg := c.providerConfig(providerID)
	if cfg == nil {
		return false
	}
	access := c.ensureChild(cfg, "access")
	access["type"] = AccessTypeAPIKey
	access["apiKey"] = apiKey
	return true
}

// SetProviderPersonalModelIDs 写入 providerRule.config.personalModelIds。
func (c *Config) SetProviderPersonalModelIDs(providerID string, ids []string) bool {
	cfg := c.providerConfig(providerID)
	if cfg == nil {
		return false
	}
	cfg["personalModelIds"] = toInterfaceSlice(ids)
	return true
}

// ---- providerModelRule 字段访问器 ----

// ModelRuleProviderID 返回 providerModelRule.providerId。
func ModelRuleProviderID(rule map[string]interface{}) string {
	return stringValue(rule["providerId"])
}

// ModelRuleModelID 返回 providerModelRule.modelId。
func ModelRuleModelID(rule map[string]interface{}) string {
	return stringValue(rule["modelId"])
}

// ModelRuleConfig 返回 providerModelRule.config。
func ModelRuleConfig(rule map[string]interface{}) map[string]interface{} {
	m, _ := rule["config"].(map[string]interface{})
	return m
}

// ModelEnabled 返回 config.enabled 及其是否存在。
func ModelEnabled(modelCfg map[string]interface{}) (bool, bool) {
	v, ok := modelCfg["enabled"].(bool)
	return v, ok
}

// ModelProperties 返回 config.properties。
func ModelProperties(modelCfg map[string]interface{}) map[string]interface{} {
	m, _ := modelCfg["properties"].(map[string]interface{})
	return m
}

// ModelOptionSpecs 返回 config.optionSpecs。
func ModelOptionSpecs(modelCfg map[string]interface{}) map[string]interface{} {
	m, _ := modelCfg["optionSpecs"].(map[string]interface{})
	return m
}

// ModelContextWindow 返回 config.properties.contextWindow 及其是否存在。
func ModelContextWindow(modelCfg map[string]interface{}) (int, bool) {
	return positiveIntValue(ModelProperties(modelCfg)["contextWindow"])
}

// ModelMaxOutputTokens 返回 config.optionSpecs.maxOutputTokens.max 及其是否存在。
func ModelMaxOutputTokens(modelCfg map[string]interface{}) (int, bool) {
	specs := ModelOptionSpecs(modelCfg)
	opt, _ := specs["maxOutputTokens"].(map[string]interface{})
	return positiveIntValue(opt["max"])
}

// ModelReasoningLevelValues 返回 config.optionSpecs.reasoningLevel.values。
func ModelReasoningLevelValues(modelCfg map[string]interface{}) []string {
	specs := ModelOptionSpecs(modelCfg)
	level, _ := specs["reasoningLevel"].(map[string]interface{})
	return stringSlice(level["values"])
}

// ModelReasoningLevelMap 返回 reasoningLevel.map（未建模值，原样透传）。
func ModelReasoningLevelMap(modelCfg map[string]interface{}) (string, bool) {
	return optionalStringValue(ModelOptionSpecs(modelCfg)["reasoningLevel"], "map")
}

// ModelMaxOutputTokensMap 返回 maxOutputTokens.map（未建模值，原样透传）。
func ModelMaxOutputTokensMap(modelCfg map[string]interface{}) (string, bool) {
	return optionalStringValue(ModelOptionSpecs(modelCfg)["maxOutputTokens"], "map")
}

// ModelInputFormat 返回 config.properties.inputFormat。
func ModelInputFormat(modelCfg map[string]interface{}) map[string]interface{} {
	m, _ := ModelProperties(modelCfg)["inputFormat"].(map[string]interface{})
	return m
}

// ModelOutputFormat 返回 config.properties.outputFormat。
func ModelOutputFormat(modelCfg map[string]interface{}) map[string]interface{} {
	m, _ := ModelProperties(modelCfg)["outputFormat"].(map[string]interface{})
	return m
}

// ---- canonical 构造器（供 FromLegacy 等使用） ----

// NewProviderRule 构造一条 canonical providerRule。
func NewProviderRule(providerID, providerName, group string) map[string]interface{} {
	rule := map[string]interface{}{
		"providerId": providerID,
		"config":     map[string]interface{}{"group": group},
	}
	if providerName != "" {
		rule["providerName"] = providerName
	}
	return rule
}

// NewProviderModelRule 构造一条 canonical providerModelRule。
func NewProviderModelRule(providerID, modelID string) map[string]interface{} {
	return map[string]interface{}{
		"providerId": providerID,
		"modelId":    modelID,
		"config":     map[string]interface{}{},
	}
}

// NewAPIKeyAccess 构造 api-key 判别分支。
func NewAPIKeyAccess(apiKey string) map[string]interface{} {
	return map[string]interface{}{
		"type":   AccessTypeAPIKey,
		"apiKey": apiKey,
	}
}

// NewAPI 构造 api 对象。
func NewAPI(apiType, baseURL string) map[string]interface{} {
	return map[string]interface{}{
		"type":    apiType,
		"baseUrl": baseURL,
	}
}

// ---- 内部小工具 ----

func stringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}

func optionalStringValue(container interface{}, key string) (string, bool) {
	m, _ := container.(map[string]interface{})
	if m == nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func mapSlice(v interface{}) []map[string]interface{} {
	arr, ok := v.([]interface{})
	if !ok {
		if typed, ok := v.([]map[string]interface{}); ok {
			return typed
		}
		return nil
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, raw := range arr {
		if m, ok := raw.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringSlice(v interface{}) []string {
	switch arr := v.(type) {
	case []string:
		out := make([]string, len(arr))
		copy(out, arr)
		return out
	case []interface{}:
		out := make([]string, 0, len(arr))
		for _, raw := range arr {
			if s, ok := raw.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case nil:
		return nil
	default:
		return nil
	}
}

// stringSliceStrict 仅在值确为字符串数组时返回 ok=true（空数组合法）。
func stringSliceStrict(v interface{}) ([]string, bool) {
	switch arr := v.(type) {
	case []string:
		out := make([]string, len(arr))
		copy(out, arr)
		return out, true
	case []interface{}:
		out := make([]string, 0, len(arr))
		for _, raw := range arr {
			s, ok := raw.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

func toInterfaceSlice(ss []string) []interface{} {
	out := make([]interface{}, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}

func toIntValue(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return i, true
		}
		f, err := n.Float64()
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) {
			return 0, false
		}
		return int64(f), true
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || n != math.Trunc(n) {
			return 0, false
		}
		return int64(n), true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	default:
		return 0, false
	}
}

func positiveIntValue(v interface{}) (int, bool) {
	n, ok := toIntValue(v)
	if !ok || n <= 0 {
		return 0, false
	}
	return int(n), true
}
