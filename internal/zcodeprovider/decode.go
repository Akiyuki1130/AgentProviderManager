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

// CompatibilityIssue 描述“本包能读写、但 ZCode 3.14 会拒绝”的问题。
//
// ZCode 的语义是“要么全对、要么全空”：一旦解析失败，它会把整份个人配置当成空，
// 所有供应商在使用端消失。因此这类问题必须在保存前修掉——但又不该让文件打不开，
// 所以校验分成两层：
//
//   - 结构性问题（ValidationError）：JSON 形状、必填字段缺失等，读都读不出来；
//   - 兼容性问题（本类型）：能用、ZCode 不认，允许打开并由调用方向用户提示修复。
type CompatibilityIssue struct {
	Code string `json:"code"`
	Path string `json:"path"`
	Msg  string `json:"msg"`
}

// 兼容性问题的 code 取值。
const (
	IssueBuiltinModelIDs    = "builtin-model-ids"
	IssueUnsupportedGroup   = "unsupported-group"
	IssueUnsupportedLogo    = "unsupported-logo"
	IssueDuplicateProvider  = "duplicate-provider"
	IssueDuplicateModelRule = "duplicate-model-rule"
	IssueUnsupportedAPIType = "unsupported-api-type"
	IssueUnsupportedAccess  = "unsupported-access"
	IssueUnsupportedValue   = "unsupported-value"
)

// compatCollector 收集兼容性问题。为 nil 表示严格模式：兼容性问题直接作为错误返回。
type compatCollector struct {
	issues []CompatibilityIssue
}

// compat 记录一个兼容性问题：严格模式下变成 ValidationError，
// 宽松模式（Inspect）下只收集，不影响解码结果。
func (c *compatCollector) compat(code, path, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	if c == nil {
		return invalid(path, "%s", msg)
	}
	c.issues = append(c.issues, CompatibilityIssue{Code: code, Path: path, Msg: msg})
	return nil
}

func (c *compatCollector) result() []CompatibilityIssue {
	if c == nil || c.issues == nil {
		return nil
	}
	return c.issues
}

// Decode 严格解析个人 provider_config.json。
// schemaVersion 缺失或不是受支持的 1 会被拒绝；未知键同样会被拒绝，
// 因为 ZCode 遇到未知键会把整份配置当作空。
func Decode(data []byte) (*Config, error) {
	doc, err := decodeDoc(data)
	if err != nil {
		return nil, err
	}
	if err := Validate(doc); err != nil {
		return nil, err
	}
	return &Config{doc: doc}, nil
}

// DecodeInspect 解析个人 provider_config.json，并把“ZCode 3.14 会拒绝”的问题
// 以列表形式返回，而不是直接判定文件不可读。结构性问题仍然返回错误。
//
// 供读取路径使用：这样用户能打开一份需要修复的配置，并看到具体问题。
func DecodeInspect(data []byte) (*Config, []CompatibilityIssue, error) {
	doc, err := decodeDoc(data)
	if err != nil {
		return nil, nil, err
	}
	issues, err := Inspect(doc)
	if err != nil {
		return nil, nil, err
	}
	return &Config{doc: doc}, issues, nil
}

// Inspect 只检查“ZCode 3.14 兼容性”，不重复报告结构性问题。
func Inspect(doc map[string]interface{}) ([]CompatibilityIssue, error) {
	c := &compatCollector{}
	if err := validateDoc(doc, c); err != nil {
		return nil, err
	}
	return c.result(), nil
}

func decodeDoc(data []byte) (map[string]interface{}, error) {
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
	return doc, nil
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

// Validate 对已经是 map 形态的文档执行严格校验（结构 + ZCode 兼容性）。
// Encode 之前也会调用它，以保证写出的永远是 ZCode 接受的 canonical 文档。
func Validate(doc map[string]interface{}) error {
	return validateDoc(doc, nil)
}

// validateDoc 是校验的唯一实现。compat 为 nil 时兼容性问题按错误处理；
// 传入 collector 时兼容性问题只收集，结构性问题仍然报错。
func validateDoc(doc map[string]interface{}, compat *compatCollector) error {
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
		if err := validateProviderRules(rules, compat); err != nil {
			return err
		}
	} else {
		return invalid("config", "缺少必填字段 providerConfigRules")
	}
	var autoRules, manualRules []interface{}
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
		if err := validateProviderModelRules("config.modelConfigRules.providerModelRules", rules, compat); err != nil {
			return err
		}
		autoRules, _ = rules.([]interface{})
		manual, ok := mcr["manualProviderModelRules"]
		if !ok {
			return invalid("config.modelConfigRules", "缺少必填字段 manualProviderModelRules")
		}
		if err := validateProviderModelRules("config.modelConfigRules.manualProviderModelRules", manual, compat); err != nil {
			return err
		}
		manualRules, _ = manual.([]interface{})
	} else {
		return invalid("config", "缺少必填字段 modelConfigRules")
	}
	if v, ok := cfg["defaultModelSelection"]; ok {
		if _, ok := v.(map[string]interface{}); !ok {
			return invalid("config.defaultModelSelection", "必须是对象 {...}")
		}
	}
	return checkModelRuleOverlap(autoRules, manualRules, compat)
}

// checkModelRuleOverlap 复刻 ZCode 的 superRefine：同一个 (providerId, modelId)
// 不能同时出现在 providerModelRules 与 manualProviderModelRules 里。
func checkModelRuleOverlap(autoRules, manualRules []interface{}, compat *compatCollector) error {
	if len(autoRules) == 0 || len(manualRules) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, raw := range autoRules {
		rule, _ := raw.(map[string]interface{})
		if rule == nil {
			continue
		}
		seen[ModelRuleProviderID(rule)+"\x00"+ModelRuleModelID(rule)] = true
	}
	for i, raw := range manualRules {
		rule, _ := raw.(map[string]interface{})
		if rule == nil {
			continue
		}
		key := ModelRuleProviderID(rule) + "\x00" + ModelRuleModelID(rule)
		if !seen[key] {
			continue
		}
		path := fmt.Sprintf("config.modelConfigRules.manualProviderModelRules[%d]", i)
		if err := compat.compat(IssueDuplicateModelRule, path,
			"同一 Provider/Model（%s / %s）不能同时出现在 providerModelRules 与 manualProviderModelRules，ZCode 会判定整份配置非法",
			ModelRuleProviderID(rule), ModelRuleModelID(rule)); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderRules(raw interface{}, compat *compatCollector) error {
	arr, ok := raw.([]interface{})
	if !ok {
		return invalid("config.providerConfigRules.providerRules", "必须是数组")
	}
	seenIDs := map[string]int{}
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
		s, ok := id.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return invalid(path+".providerId", "必须是非空字符串")
		}
		// ZCode 用 superRefine 要求 providerId 唯一；重复会让整份配置失效。
		if prev, dup := seenIDs[s]; dup {
			if err := compat.compat(IssueDuplicateProvider, path+".providerId",
				"providerId「%s」重复（第 %d 条与之重复），ZCode 会判定整份配置非法", s, prev+1); err != nil {
				return err
			}
		} else {
			seenIDs[s] = i
		}
		if v, ok := rule["providerName"]; ok && v != nil {
			name, ok := v.(string)
			if !ok {
				return invalid(path+".providerName", "必须是字符串或 null")
			}
			if strings.TrimSpace(name) == "" {
				if err := compat.compat(IssueUnsupportedValue, path+".providerName",
					"providerName 不能是空字符串，ZCode 会判定整份配置非法"); err != nil {
					return err
				}
			}
		}
		if v, ok := rule["templateId"]; ok && v != nil {
			tpl, ok := v.(string)
			if !ok {
				return invalid(path+".templateId", "必须是字符串或 null")
			}
			if strings.TrimSpace(tpl) == "" {
				if err := compat.compat(IssueUnsupportedValue, path+".templateId",
					"templateId 不能是空字符串，ZCode 会判定整份配置非法"); err != nil {
					return err
				}
			}
		}
		if v, ok := rule["enabled"]; ok && v != nil {
			if _, ok := v.(bool); !ok {
				return invalid(path+".enabled", "必须是布尔值或 null")
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
		if err := validateProviderConfig(path+".config", pcfg, compat); err != nil {
			return err
		}
	}
	return nil
}

// validateProviderConfig 校验个人文件的 providerRule.config。
//
// 与 ZCode 3.14 的 zod 对齐后，个人规则是 qo.omit({builtinModelIds})：
// group 只能 standard-personal（缺省或 null 也可），其余字段都可缺省或为 null。
func validateProviderConfig(path string, pcfg map[string]interface{}, compat *compatCollector) error {
	if err := rejectUnknown(path, pcfg, providerRuleConfigKeys); err != nil {
		return err
	}
	if group, ok := pcfg["group"]; ok && group != nil {
		groupStr, ok := group.(string)
		if !ok {
			return invalid(path+".group", "必须是字符串或 null")
		}
		if groupStr != GroupStandardPersonal {
			if err := compat.compat(IssueUnsupportedGroup, path+".group",
				"个人文件的 config.group 只能是 %s（当前为 %q）；zai-family / bigmodel-family 只能出现在 ZCode 内建文件里",
				GroupStandardPersonal, groupStr); err != nil {
				return err
			}
		}
	}
	if logo, ok := pcfg["logo"]; ok && logo != nil {
		if err := validateLogo(path+".logo", logo, compat); err != nil {
			return err
		}
	}
	if v, ok := pcfg["builtinModelIds"]; ok {
		if _, ok := stringSliceStrict(v); !ok {
			return invalid(path+".builtinModelIds", "必须是字符串数组或 null")
		}
		if err := compat.compat(IssueBuiltinModelIDs, path+".builtinModelIds",
			"个人文件不接受 builtinModelIds（只有 ZCode 内建配置可以声明），ZCode 会判定整份配置非法；请改用 personalModelIds 或删除该键"); err != nil {
			return err
		}
	}
	if v, ok := pcfg["personalModelIds"]; ok {
		if _, ok := stringSliceStrict(v); !ok {
			return invalid(path+".personalModelIds", "必须是字符串数组或 null")
		}
	}
	if v, ok := pcfg["modelOrder"]; ok {
		if _, ok := stringSliceStrict(v); !ok {
			return invalid(path+".modelOrder", "必须是字符串数组或 null")
		}
	}
	if v, ok := pcfg["visibility"]; ok && v != nil {
		vis, ok := v.(string)
		if !ok {
			return invalid(path+".visibility", "必须是字符串或 null")
		}
		if !ValidVisibility(vis) {
			if err := compat.compat(IssueUnsupportedValue, path+".visibility",
				"visibility 只能是 %s / %s（当前为 %q）", VisibilityVisible, VisibilityHidden, vis); err != nil {
				return err
			}
		}
	}
	if v, ok := pcfg["access"]; ok && v != nil {
		access, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".access", "必须是对象 {...} 或 null")
		}
		if err := validateAccess(path+".access", access, compat); err != nil {
			return err
		}
	}
	if v, ok := pcfg["api"]; ok && v != nil {
		api, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".api", "必须是对象 {...} 或 null")
		}
		if err := validateAPI(path+".api", api, compat); err != nil {
			return err
		}
	}
	return nil
}

// validateLogo 校验 logo：ZCode 3.14 起 logo 是 {type:"builtin", key} 或 null，
// 早期出现过的字符串 URL 形状会被 ZCode 拒绝。
func validateLogo(path string, logo interface{}, compat *compatCollector) error {
	obj, ok := logo.(map[string]interface{})
	if !ok {
		return compat.compat(IssueUnsupportedLogo, path,
			"logo 必须是 {\"type\":\"%s\",\"key\":\"...\"} 或 null，ZCode 会判定整份配置非法", LogoTypeBuiltin)
	}
	var bad []string
	for k := range obj {
		if !logoKeys[k] {
			bad = append(bad, k)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		if err := compat.compat(IssueUnsupportedLogo, path,
			"logo 只接受 %s 两个键，多出的键（%s）会被 ZCode 拒绝", "type/key", strings.Join(bad, ", ")); err != nil {
			return err
		}
	}
	if t, ok := obj["type"]; !ok {
		if err := compat.compat(IssueUnsupportedLogo, path, "logo 缺少 type（只能是 %s）", LogoTypeBuiltin); err != nil {
			return err
		}
	} else if s, ok := t.(string); !ok || s != LogoTypeBuiltin {
		if err := compat.compat(IssueUnsupportedLogo, path+".type", "logo.type 只能是 %s", LogoTypeBuiltin); err != nil {
			return err
		}
	}
	if k, ok := obj["key"]; !ok {
		if err := compat.compat(IssueUnsupportedLogo, path, "logo 缺少 key（非空字符串）"); err != nil {
			return err
		}
	} else if s, ok := k.(string); !ok || strings.TrimSpace(s) == "" {
		if err := compat.compat(IssueUnsupportedLogo, path+".key", "logo.key 必须是非空字符串"); err != nil {
			return err
		}
	}
	return nil
}

// validateAccess 校验 access。三种 type 的已知键集都已按 ZCode 3.14 的 zod 对齐：
// api-key / zhipu-coding-plan-api-key 共用 {type,apiKey,apiKeyManagementUrl}，
// zhipu-account 用 {type,accountType,mode,entitled}。
func validateAccess(path string, access map[string]interface{}, compat *compatCollector) error {
	rawType, ok := access["type"]
	if !ok {
		return invalid(path, "缺少必填字段 type")
	}
	accessType, ok := rawType.(string)
	if !ok {
		return invalid(path+".type", "必须是字符串")
	}
	allowed := accessAPIKeyKeys
	switch accessType {
	case AccessTypeAPIKey, AccessTypeZhipuCodingPlanAPIKey:
	case AccessTypeZhipuAccount:
		allowed = accessZhipuAccountKeys
	default:
		// 取值不合法会让 ZCode 判定整份配置非法，但文件仍需可读以便修复。
		if err := compat.compat(IssueUnsupportedAccess, path+".type", "access.type 只能是 %s/%s/%s（当前为 %q）",
			AccessTypeAPIKey, AccessTypeZhipuCodingPlanAPIKey, AccessTypeZhipuAccount, accessType); err != nil {
			return err
		}
		allowed = nil // 未知分支：不做键集校验，原样保留
	}
	if allowed != nil {
		var bad []string
		for k := range access {
			if !allowed[k] {
				bad = append(bad, k)
			}
		}
		if len(bad) > 0 {
			sort.Strings(bad)
			if err := compat.compat(IssueUnsupportedAccess, path,
				"access.type=%s 不接受键 %s，ZCode 会判定整份配置非法", accessType, strings.Join(bad, ", ")); err != nil {
				return err
			}
		}
	}
	if v, ok := access["apiKey"]; ok && v != nil {
		if _, ok := v.(string); !ok {
			return invalid(path+".apiKey", "必须是字符串或 null")
		}
	}
	if v, ok := access["apiKeyManagementUrl"]; ok && v != nil {
		if _, ok := v.(string); !ok {
			return invalid(path+".apiKeyManagementUrl", "必须是字符串或 null")
		}
	}
	if v, ok := access["accountType"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return invalid(path+".accountType", "必须是字符串或 null")
		}
		if !ValidAccountType(s) {
			if err := compat.compat(IssueUnsupportedAccess, path+".accountType",
				"accountType 只能是 %s / %s（当前为 %q）", AccountTypeZAI, AccountTypeBigModel, s); err != nil {
				return err
			}
		}
	}
	if v, ok := access["mode"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return invalid(path+".mode", "必须是字符串或 null")
		}
		if !ValidAccountMode(s) {
			if err := compat.compat(IssueUnsupportedAccess, path+".mode",
				"mode 只能是 %s 之一（当前为 %q）", strings.Join(AccountModes, " / "), s); err != nil {
				return err
			}
		}
	}
	if v, ok := access["entitled"]; ok && v != nil {
		if _, ok := v.(bool); !ok {
			return invalid(path+".entitled", "必须是布尔值或 null")
		}
	}
	return nil
}

// validateAPI 校验 api。
//
// 个人规则里 api 允许缺省或为 null；baseUrl 允许缺省 / null / 任意字符串
// （ZCode 只对内建规则校验 URL 格式）。
func validateAPI(path string, api map[string]interface{}, compat *compatCollector) error {
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
		if err := compat.compat(IssueUnsupportedAPIType, path+".type", "api.type 只能是 %s/%s/%s（当前为 %q）",
			APITypeAnthropicMessages, APITypeOpenAIChatCompletions, APITypeOpenAIResponses, apiType); err != nil {
			return err
		}
	}
	if rawBase, ok := api["baseUrl"]; ok && rawBase != nil {
		if _, ok := rawBase.(string); !ok {
			return invalid(path+".baseUrl", "必须是字符串或 null")
		}
	}
	if v, ok := api["headers"]; ok && v != nil {
		headers, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".headers", "必须是对象 {...} 或 null")
		}
		for key, value := range headers {
			if _, ok := value.(string); !ok {
				if err := compat.compat(IssueUnsupportedValue, path+".headers."+key, "headers 的值必须是字符串"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// validateProviderModelRules 校验 providerModelRules / manualProviderModelRules。
//
// 两张表共用同一套键集：ZCode 对 manualProviderModelRules 有一层“旧写法归一化”
// （normalizeLegacyManualRules 会把自动规则形状的 config 收敛成手动形状），
// 因此按自动规则的全集校验不会误报——真正非法的键本来就会被拒绝。
func validateProviderModelRules(prefix string, raw interface{}, compat *compatCollector) error {
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
		if err := validateModelRuleConfig(path+".config", mcfg, compat); err != nil {
			return err
		}
	}
	return nil
}

func validateModelRuleConfig(path string, mcfg map[string]interface{}, compat *compatCollector) error {
	if err := rejectUnknown(path, mcfg, modelRuleConfigKeys); err != nil {
		return err
	}
	if v, ok := mcfg["enabled"]; ok && v != nil {
		if _, ok := v.(bool); !ok {
			return invalid(path+".enabled", "必须是布尔值或 null")
		}
	}
	if v, ok := mcfg["properties"]; ok && v != nil {
		props, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".properties", "必须是对象 {...} 或 null")
		}
		if err := validateProperties(path+".properties", props, compat); err != nil {
			return err
		}
	}
	if v, ok := mcfg["optionSpecs"]; ok && v != nil {
		specs, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".optionSpecs", "必须是对象 {...} 或 null")
		}
		if err := validateOptionSpecs(path+".optionSpecs", specs, compat); err != nil {
			return err
		}
	}
	return nil
}

func validateProperties(path string, props map[string]interface{}, compat *compatCollector) error {
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
		if v, ok := props[key]; ok && v != nil {
			if _, ok := v.(bool); !ok {
				return invalid(path+"."+key, "必须是布尔值或 null")
			}
		}
	}
	if v, ok := props["contextWindow"]; ok && v != nil {
		if _, ok := positiveIntValue(v); !ok {
			return invalid(path+".contextWindow", "必须是正整数或 null")
		}
	}
	if v, ok := props["inputFormat"]; ok && v != nil {
		format, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".inputFormat", "必须是对象 {...} 或 null")
		}
		if err := rejectUnknown(path+".inputFormat", format, inputFormatKeys); err != nil {
			return err
		}
		if err := validateBoolMap(path+".inputFormat", format); err != nil {
			return err
		}
	}
	if v, ok := props["outputFormat"]; ok && v != nil {
		format, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".outputFormat", "必须是对象 {...} 或 null")
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
		if value == nil {
			continue
		}
		if _, ok := value.(bool); !ok {
			return invalid(path+"."+key, "必须是布尔值或 null")
		}
	}
	return nil
}

func validateOptionSpecs(path string, specs map[string]interface{}, compat *compatCollector) error {
	if err := rejectUnknown(path, specs, optionSpecsKeys); err != nil {
		return err
	}
	if v, ok := specs["reasoningLevel"]; ok && v != nil {
		level, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".reasoningLevel", "必须是对象 {...} 或 null")
		}
		if err := rejectUnknown(path+".reasoningLevel", level, reasoningLevelKeys); err != nil {
			return err
		}
		if values, ok := level["values"]; ok && values != nil {
			if _, ok := stringSliceStrict(values); !ok {
				return invalid(path+".reasoningLevel.values", "必须是字符串数组或 null")
			}
		}
		// map 是可选字段，不校验取值范围以外的内容；不得自行编造其值。
		if v, ok := level["map"]; ok && v != nil {
			if _, ok := v.(string); !ok {
				return invalid(path+".reasoningLevel.map", "必须是字符串或 null")
			}
		}
	}
	if v, ok := specs["maxOutputTokens"]; ok && v != nil {
		out, ok := v.(map[string]interface{})
		if !ok {
			return invalid(path+".maxOutputTokens", "必须是对象 {...} 或 null")
		}
		if err := rejectUnknown(path+".maxOutputTokens", out, maxOutputTokensKeys); err != nil {
			return err
		}
		if max, ok := out["max"]; ok && max != nil {
			if _, ok := positiveIntValue(max); !ok {
				return invalid(path+".maxOutputTokens.max", "必须是正整数或 null")
			}
		}
		if v, ok := out["map"]; ok && v != nil {
			if _, ok := v.(string); !ok {
				return invalid(path+".maxOutputTokens.map", "必须是字符串或 null")
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
