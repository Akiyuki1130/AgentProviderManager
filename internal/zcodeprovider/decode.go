package zcodeprovider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// ValidationError 描述 provider_config.json 未通过严格校验的位置与原因。
// strict 校验等价于 zod 的 .strict()：任何未知键都会让整份解码失败。
type ValidationError struct {
	Path string
	Msg  string
}

func (e *ValidationError) Error() string {
	if e.Path == "" {
		return "provider_config.json 校验失败：" + e.Msg
	}
	return fmt.Sprintf("provider_config.json 校验失败（%s）：%s", e.Path, e.Msg)
}

func invalid(path, format string, args ...interface{}) error {
	return &ValidationError{Path: path, Msg: fmt.Sprintf(format, args...)}
}

// Decode 严格解析个人 provider_config.json。
// schemaVersion 缺失或不是受支持的 1 会被拒绝；未知键同样会被拒绝，
// 因为 ZCode 遇到未知键会把整份配置当作空。
func Decode(data []byte) (*Config, error) {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("provider_config.json 解析失败（JSON 格式错误）：%w", err)
	}
	if err := ensureOnlyOneValue(dec); err != nil {
		return nil, err
	}
	doc, ok := v.(map[string]interface{})
	if !ok {
		return nil, invalid("", "根节点必须是对象 {...}")
	}
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return &Config{doc: doc}, nil
}

func ensureOnlyOneValue(dec *json.Decoder) error {
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("provider_config.json 解析失败（JSON 格式错误）：出现多余的内容")
		}
		return fmt.Errorf("provider_config.json 解析失败（JSON 格式错误）：%w", err)
	}
	return nil
}

// Validate 对已经是 map 形态的文档执行严格校验。Encode 之前也会调用它，
// 以保证写出的永远只有 canonical 键。
func Validate(doc map[string]interface{}) error {
	if doc == nil {
		return invalid("", "根节点必须是对象 {...}")
	}
	if err := rejectUnknown("", doc, topLevelKeys); err != nil {
		return err
	}
	rawVersion, ok := doc["schemaVersion"]
	if !ok {
		return invalid("schemaVersion", "缺少必填字段 schemaVersion")
	}
	version, ok := toIntValue(rawVersion)
	if !ok {
		return invalid("schemaVersion", "必须是整数")
	}
	if version != SchemaVersionV1 {
		return invalid("schemaVersion", "不支持的版本 %d（当前仅支持 %d）", version, SchemaVersionV1)
	}
	rawConfig, ok := doc["config"]
	if !ok {
		return invalid("config", "缺少必填字段 config")
	}
	cfg, ok := rawConfig.(map[string]interface{})
	if !ok {
		return invalid("config", "必须是对象 {...}")
	}
	if err := rejectUnknown("config", cfg, configKeys); err != nil {
		return err
	}
	if v, ok := cfg["providerOrder"]; ok {
		if _, ok := stringSliceStrict(v); !ok {
			return invalid("config.providerOrder", "必须是字符串数组")
		}
	}
	if v, ok := cfg["providerConfigRules"]; ok {
		pcr, ok := v.(map[string]interface{})
		if !ok {
			return invalid("config.providerConfigRules", "必须是对象 {...}")
		}
		if err := rejectUnknown("config.providerConfigRules", pcr, providerConfigRulesKeys); err != nil {
			return err
		}
		rules, ok := pcr["providerRules"]
		if !ok {
			return invalid("config.providerConfigRules", "缺少必填字段 providerRules")
		}
		if err := validateProviderRules(rules); err != nil {
			return err
		}
	} else {
		return invalid("config", "缺少必填字段 providerConfigRules")
	}
	if v, ok := cfg["modelConfigRules"]; ok {
		mcr, ok := v.(map[string]interface{})
		if !ok {
			return invalid("config.modelConfigRules", "必须是对象 {...}")
		}
		if err := rejectUnknown("config.modelConfigRules", mcr, modelConfigRulesKeys); err != nil {
			return err
		}
		rules, ok := mcr["providerModelRules"]
		if !ok {
			return invalid("config.modelConfigRules", "缺少必填字段 providerModelRules")
		}
		if err := validateProviderModelRules("config.modelConfigRules.providerModelRules", rules); err != nil {
			return err
		}
		manual, ok := mcr["manualProviderModelRules"]
		if !ok {
			return invalid("config.modelConfigRules", "缺少必填字段 manualProviderModelRules")
		}
		if err := validateProviderModelRules("config.modelConfigRules.manualProviderModelRules", manual); err != nil {
			return err
		}
	} else {
		return invalid("config", "缺少必填字段 modelConfigRules")
	}
	if v, ok := cfg["defaultModelSelection"]; ok {
		if _, ok := v.(map[string]interface{}); !ok {
			return invalid("config.defaultModelSelection", "必须是对象 {...}")
		}
	}
	return nil
}

func validateProviderRules(raw interface{}) error {
	arr, ok := raw.([]interface{})
	if !ok {
		return invalid("config.providerConfigRules.providerRules", "必须是数组")
	}
	for i, item := range arr {
		path := fmt.Sprintf("config.providerConfigRules.providerRules[%d]", i)
		rule, ok := item.(map[string]interface{})
		if !ok {
			return invalid(path, "必须是对象 {...}")
		}
		if err := rejectUnknown(path, rule, providerRuleKeys); err != nil {
			return err
		}
		id, ok := rule["providerId"]
		if !ok {
			return invalid(path, "缺少必填字段 providerId")
		}
		if s, ok := id.(string); !ok || strings.TrimSpace(s) == "" {
			return invalid(path+".providerId", "必须是非空字符串")
		}
		if v, ok := rule["providerName"]; ok {
			if _, ok := v.(string); !ok {
				return invalid(path+".providerName", "必须是字符串")
			}
		}
		if v, ok := rule["templateId"]; ok {
			if _, ok := v.(string); !ok {
				return invalid(path+".templateId", "必须是字符串")
			}
		}
		if v, ok := rule["enabled"]; ok {
			if _, ok := v.(bool); !ok {
				return invalid(path+".enabled", "必须是布尔值")
			}
		}
		rawCfg, ok := rule["config"]
		if !ok {
			return invalid(path, "缺少必填字段 config")
		}
		pcfg, ok := rawCfg.(map[string]interface{})
		if !ok {
			return invalid(path+".config", "必须是对象 {...}")
		}
		if err := validateProviderConfig(path+".config", pcfg); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderConfig(path string, pcfg map[string]interface{}) error {
	if err := rejectUnknown(path, pcfg, providerRuleConfigKeys); err != nil {
		return err
	}
	group, ok := pcfg["group"]
	if !ok {
		return invalid(path, "缺少必填字段 group")
	}
	groupStr, ok := group.(string)
	if !ok {
		return invalid(path+".group", "必须是字符串")
	}
	switch groupStr {
	case GroupStandardPersonal, GroupZAIFamily, GroupBigModelFamily:
	default:
		return invalid(path+".group", "取值必须是 %s/%s/%s 之一", GroupStandardPersonal, GroupZAIFamily, GroupBigModelFamily)
	}
	if v, ok := pcfg["builtinModelIds"]; ok {
		if _, ok := stringSliceStrict(v); !ok {
			return invalid(path+".builtinModelIds", "必须是字符串数组")
		}
	}
	if v, ok := pcfg["personalModelIds"]; ok {
		if _, ok := stringSliceStrict(v); !ok {
			return invalid(path+".personalModelIds", "必须是字符串数组")
		}
	}
	if v, ok := pcfg["modelOrder"]; ok {
		if _, ok := stringSliceStrict(v); !ok {
			return invalid(path+".modelOrder", "必须是字符串数组")
		}
	}
	if v, ok := pcfg["access"]; ok {
		access, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".access", "必须是对象 {...}")
		}
		if err := validateAccess(path+".access", access); err != nil {
			return err
		}
	}
	if v, ok := pcfg["api"]; ok {
		api, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".api", "必须是对象 {...}")
		}
		if err := validateAPI(path+".api", api); err != nil {
			return err
		}
	}
	return nil
}

func validateAccess(path string, access map[string]interface{}) error {
	rawType, ok := access["type"]
	if !ok {
		return invalid(path, "缺少必填字段 type")
	}
	accessType, ok := rawType.(string)
	if !ok {
		return invalid(path+".type", "必须是字符串")
	}
	switch accessType {
	case AccessTypeAPIKey:
		if err := rejectUnknown(path, access, accessAPIKeyKeys); err != nil {
			return err
		}
	case AccessTypeZhipuCodingPlanAPIKey, AccessTypeZhipuAccount:
		// 这两个分支的字段逆向未确认。为免误判为未知键而丢配置，
		// 这里只校验 type，其余字段原样保留。
	default:
		return invalid(path+".type", "取值必须是 %s/%s/%s 之一",
			AccessTypeAPIKey, AccessTypeZhipuCodingPlanAPIKey, AccessTypeZhipuAccount)
	}
	if v, ok := access["apiKey"]; ok {
		if _, ok := v.(string); !ok {
			return invalid(path+".apiKey", "必须是字符串")
		}
	}
	if v, ok := access["apiKeyManagementUrl"]; ok {
		if _, ok := v.(string); !ok {
			return invalid(path+".apiKeyManagementUrl", "必须是字符串")
		}
	}
	return nil
}

func validateAPI(path string, api map[string]interface{}) error {
	if err := rejectUnknown(path, api, apiKeys); err != nil {
		return err
	}
	rawType, ok := api["type"]
	if !ok {
		return invalid(path, "缺少必填字段 type")
	}
	apiType, ok := rawType.(string)
	if !ok {
		return invalid(path+".type", "必须是字符串")
	}
	switch apiType {
	case APITypeAnthropicMessages, APITypeOpenAIChatCompletions, APITypeOpenAIResponses:
	default:
		return invalid(path+".type", "取值必须是 %s/%s/%s 之一",
			APITypeAnthropicMessages, APITypeOpenAIChatCompletions, APITypeOpenAIResponses)
	}
	rawBase, ok := api["baseUrl"]
	if !ok {
		return invalid(path, "缺少必填字段 baseUrl")
	}
	if _, ok := rawBase.(string); !ok {
		return invalid(path+".baseUrl", "必须是字符串")
	}
	if v, ok := api["headers"]; ok {
		if _, ok := v.(map[string]interface{}); !ok {
			return invalid(path+".headers", "必须是对象 {...}")
		}
	}
	return nil
}

func validateProviderModelRules(prefix string, raw interface{}) error {
	arr, ok := raw.([]interface{})
	if !ok {
		return invalid(prefix, "必须是数组")
	}
	for i, item := range arr {
		path := fmt.Sprintf("%s[%d]", prefix, i)
		rule, ok := item.(map[string]interface{})
		if !ok {
			return invalid(path, "必须是对象 {...}")
		}
		if err := rejectUnknown(path, rule, modelRuleKeys); err != nil {
			return err
		}
		if err := requireNonEmptyString(rule, "providerId", path); err != nil {
			return err
		}
		if err := requireNonEmptyString(rule, "modelId", path); err != nil {
			return err
		}
		rawCfg, ok := rule["config"]
		if !ok {
			return invalid(path, "缺少必填字段 config")
		}
		mcfg, ok := rawCfg.(map[string]interface{})
		if !ok {
			return invalid(path+".config", "必须是对象 {...}")
		}
		if err := validateModelRuleConfig(path+".config", mcfg); err != nil {
			return err
		}
	}
	return nil
}

func validateModelRuleConfig(path string, mcfg map[string]interface{}) error {
	if err := rejectUnknown(path, mcfg, modelRuleConfigKeys); err != nil {
		return err
	}
	if v, ok := mcfg["enabled"]; ok {
		if _, ok := v.(bool); !ok {
			return invalid(path+".enabled", "必须是布尔值")
		}
	}
	if v, ok := mcfg["properties"]; ok {
		props, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".properties", "必须是对象 {...}")
		}
		if err := validateProperties(path+".properties", props); err != nil {
			return err
		}
	}
	if v, ok := mcfg["optionSpecs"]; ok {
		specs, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".optionSpecs", "必须是对象 {...}")
		}
		if err := validateOptionSpecs(path+".optionSpecs", specs); err != nil {
			return err
		}
	}
	return nil
}

func validateProperties(path string, props map[string]interface{}) error {
	if err := rejectUnknown(path, props, propertiesKeys); err != nil {
		return err
	}
	boolFields := []string{
		"requiresMfjsToolSchema",
		"supportsToolCall",
		"supportsJsonSchemaOutput",
		"supportsNativeWebSearch",
		"supportsMidConversationSystem",
	}
	for _, key := range boolFields {
		if v, ok := props[key]; ok {
			if _, ok := v.(bool); !ok {
				return invalid(path+"."+key, "必须是布尔值")
			}
		}
	}
	if v, ok := props["contextWindow"]; ok {
		if _, ok := positiveIntValue(v); !ok {
			return invalid(path+".contextWindow", "必须是正整数")
		}
	}
	if v, ok := props["inputFormat"]; ok {
		format, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".inputFormat", "必须是对象 {...}")
		}
		if err := rejectUnknown(path+".inputFormat", format, inputFormatKeys); err != nil {
			return err
		}
		if err := validateBoolMap(path+".inputFormat", format); err != nil {
			return err
		}
	}
	if v, ok := props["outputFormat"]; ok {
		format, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".outputFormat", "必须是对象 {...}")
		}
		if err := rejectUnknown(path+".outputFormat", format, outputFormatKeys); err != nil {
			return err
		}
		if err := validateBoolMap(path+".outputFormat", format); err != nil {
			return err
		}
	}
	return nil
}

func validateBoolMap(path string, m map[string]interface{}) error {
	for key, value := range m {
		if _, ok := value.(bool); !ok {
			return invalid(path+"."+key, "必须是布尔值")
		}
	}
	return nil
}

func validateOptionSpecs(path string, specs map[string]interface{}) error {
	if err := rejectUnknown(path, specs, optionSpecsKeys); err != nil {
		return err
	}
	if v, ok := specs["reasoningLevel"]; ok {
		level, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".reasoningLevel", "必须是对象 {...}")
		}
		if err := rejectUnknown(path+".reasoningLevel", level, reasoningLevelKeys); err != nil {
			return err
		}
		values, ok := level["values"]
		if !ok {
			return invalid(path+".reasoningLevel", "缺少必填字段 values")
		}
		if _, ok := stringSliceStrict(values); !ok {
			return invalid(path+".reasoningLevel.values", "必须是字符串数组")
		}
		// map 是可选字段，不校验取值范围以外的内容；不得自行编造其值。
		if v, ok := level["map"]; ok {
			if _, ok := v.(string); !ok {
				return invalid(path+".reasoningLevel.map", "必须是字符串")
			}
		}
	}
	if v, ok := specs["maxOutputTokens"]; ok {
		out, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".maxOutputTokens", "必须是对象 {...}")
		}
		if err := rejectUnknown(path+".maxOutputTokens", out, maxOutputTokensKeys); err != nil {
			return err
		}
		max, ok := out["max"]
		if !ok {
			return invalid(path+".maxOutputTokens", "缺少必填字段 max")
		}
		if _, ok := positiveIntValue(max); !ok {
			return invalid(path+".maxOutputTokens.max", "必须是正整数")
		}
		if v, ok := out["map"]; ok {
			if _, ok := v.(string); !ok {
				return invalid(path+".maxOutputTokens.map", "必须是字符串")
			}
		}
	}
	return nil
}

func requireNonEmptyString(m map[string]interface{}, key, path string) error {
	raw, ok := m[key]
	if !ok {
		return invalid(path, "缺少必填字段 %s", key)
	}
	s, ok := raw.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return invalid(path+"."+key, "必须是非空字符串")
	}
	return nil
}

func rejectUnknown(path string, m map[string]interface{}, allowed map[string]bool) error {
	var bad []string
	for k := range m {
		if !allowed[k] {
			bad = append(bad, k)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return invalid(path, "存在未知键（严格校验不允许）：%s", strings.Join(bad, ", "))
}
