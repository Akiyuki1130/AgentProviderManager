package ompprovider

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// EffortLevels 是 omp 支持的六档 reasoning effort（不含 off），顺序即规范顺序。
var EffortLevels = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// supportedAPIs 是 omp 的 13 种 wire API。
var supportedAPIs = map[string]bool{
	"openai-completions":      true,
	"openai-responses":        true,
	"openai-codex-responses":  true,
	"azure-openai-responses":  true,
	"anthropic-messages":      true,
	"bedrock-converse-stream": true,
	"google-generative-ai":    true,
	"google-gemini-cli":       true,
	"google-vertex":           true,
	"openrouter-decisions":    true,
	"openai-decisions":        true,
	"workers-ai-systemone":    true,
	"typesafe":                true,
}

// supportedAuths 是 provider.auth 的合法取值。
var supportedAuths = map[string]bool{"apiKey": true, "none": true, "oauth": true}

// supportedInputs 是模型 input 的合法模态。
var supportedInputs = map[string]bool{"text": true, "image": true}

// Efforts 返回 thinking.efforts 中合法的档位，按规范顺序去重；没有则返回 nil。
func Efforts(raw map[string]interface{}) []string {
	if raw == nil {
		return nil
	}
	thinking, ok := raw["thinking"].(map[string]interface{})
	if !ok {
		return nil
	}
	value, ok := thinking["efforts"]
	if !ok || value == nil {
		return nil
	}
	var items []string
	switch typed := value.(type) {
	case []string:
		items = typed
	case []interface{}:
		for _, item := range typed {
			if text, isString := item.(string); isString {
				items = append(items, text)
			}
		}
	default:
		return nil
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[strings.TrimSpace(item)] = true
	}
	out := make([]string, 0, len(EffortLevels))
	for _, level := range EffortLevels {
		if seen[level] {
			out = append(out, level)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Validate 在写盘前校验整份 models 文档，失败时返回中文原因。
func (f *ModelsFile) Validate() error {
	if f == nil || f.root == nil {
		return nil
	}
	if f.root.Kind != yaml.MappingNode {
		return ErrRootNotMapping
	}
	providers := f.providersNode()
	if providers == nil {
		return nil
	}
	for i := 0; i+1 < len(providers.Content); i += 2 {
		idNode := providers.Content[i]
		id := idNode.Value
		if strings.TrimSpace(id) == "" {
			return errors.New("provider ID 不能为空")
		}
		path := "providers." + id
		value := providers.Content[i+1]
		if value.Kind == yaml.MappingNode && hasAliasOrMerge(value) {
			return fmt.Errorf("%s %w", path, ErrReadOnlyNode)
		}
		var raw map[string]interface{}
		if err := value.Decode(&raw); err != nil {
			return fmt.Errorf("%s 解析失败：%w", path, err)
		}
		if raw == nil {
			raw = map[string]interface{}{}
		}
		if err := validateProvider(path, raw); err != nil {
			return err
		}
	}
	return nil
}

// validateProvider 校验单个 provider。
func validateProvider(path string, raw map[string]interface{}) error {
	if api, ok := raw["api"]; ok && api != nil {
		text, err := stringField(api, path+".api")
		if err != nil {
			return err
		}
		if !supportedAPIs[text] {
			return fmt.Errorf("%s.api 的值 %q 不受 omp 支持", path, text)
		}
	}
	authNoneOrOauth := false
	if auth, ok := raw["auth"]; ok && auth != nil {
		text, err := stringField(auth, path+".auth")
		if err != nil {
			return err
		}
		if !supportedAuths[text] {
			return fmt.Errorf("%s.auth 只能是 apiKey、none 或 oauth", path)
		}
		authNoneOrOauth = text == "none" || text == "oauth"
	}
	for _, key := range []string{"baseUrl", "name", "guardrailIdentifier"} {
		if value, ok := raw[key]; ok && value != nil {
			text, err := stringField(value, path+"."+key)
			if err != nil {
				return err
			}
			if strings.TrimSpace(text) == "" {
				return fmt.Errorf("%s.%s 不能为空", path, key)
			}
		}
	}
	if headers, ok := raw["headers"]; ok && headers != nil {
		if err := validateStringMap(headers, path+".headers"); err != nil {
			return err
		}
	}
	for _, key := range []string{"compat", "requestMetadata", "remoteCompaction", "modelOverrides"} {
		if value, ok := raw[key]; ok && value != nil {
			if _, isMap := value.(map[string]interface{}); !isMap {
				return fmt.Errorf("%s.%s 必须是映射对象", path, key)
			}
		}
	}
	if discovery, ok := raw["discovery"]; ok && discovery != nil {
		if err := validateDiscovery(discovery, path+".discovery", raw); err != nil {
			return err
		}
	}

	var models []map[string]interface{}
	if value, ok := raw["models"]; ok && value != nil {
		list, err := toModelList(value)
		if err != nil {
			return fmt.Errorf("%s.models：%w", path, err)
		}
		models = list
	}
	if len(models) == 0 {
		if !providerMapSatisfiesG2(raw) {
			return fmt.Errorf("%s：models 为空时 provider 必须至少提供 baseUrl、apiKey、auth: none、headers 等字段之一", path)
		}
		return nil
	}
	if value, ok := raw["baseUrl"]; !ok || value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
		return fmt.Errorf("%s：models 非空时必须提供 baseUrl", path)
	}
	if !authNoneOrOauth {
		apiKey, ok := raw["apiKey"]
		if !ok || apiKey == nil {
			return fmt.Errorf("%s：models 非空时必须提供 apiKey（auth 为 none 或 oauth 时除外）", path)
		}
		if text, isString := apiKey.(string); isString && strings.TrimSpace(text) == "" {
			return fmt.Errorf("%s：models 非空时必须提供 apiKey（auth 为 none 或 oauth 时除外）", path)
		}
	}
	providerAPI := ""
	if api, ok := raw["api"]; ok && api != nil {
		providerAPI, _ = api.(string)
	}
	seenIDs := make(map[string]bool, len(models))
	for idx, model := range models {
		modelPath := fmt.Sprintf("%s.models[%d]", path, idx)
		id, err := modelID(model)
		if err != nil {
			return fmt.Errorf("%s：%w", modelPath, err)
		}
		if seenIDs[id] {
			return fmt.Errorf("%s：模型 id %q 在同一 provider 内重复", modelPath, id)
		}
		seenIDs[id] = true
		if err := validateModel(modelPath, model, providerAPI); err != nil {
			return err
		}
	}
	return nil
}

// validateModel 校验单个模型。
func validateModel(path string, model map[string]interface{}, providerAPI string) error {
	if api, ok := model["api"]; ok && api != nil {
		text, err := stringField(api, path+".api")
		if err != nil {
			return err
		}
		if !supportedAPIs[text] {
			return fmt.Errorf("%s.api 的值 %q 不受 omp 支持", path, text)
		}
	} else if providerAPI == "" {
		return fmt.Errorf("%s：provider 未提供 api 时模型必须自带 api", path)
	}
	if name, ok := model["name"]; ok && name != nil {
		text, err := stringField(name, path+".name")
		if err != nil {
			return err
		}
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("%s.name 不能为空", path)
		}
	}
	if input, ok := model["input"]; ok && input != nil {
		if err := validateInput(input, path+".input"); err != nil {
			return err
		}
	}
	if thinking, ok := model["thinking"]; ok && thinking != nil {
		if err := validateThinking(thinking, path+".thinking"); err != nil {
			return err
		}
	}
	if cost, ok := model["cost"]; ok && cost != nil {
		if err := validateCost(cost, path+".cost"); err != nil {
			return err
		}
	}
	if promptCache, ok := model["promptCache"]; ok && promptCache != nil {
		if _, isMap := promptCache.(map[string]interface{}); !isMap {
			return fmt.Errorf("%s.promptCache 必须是映射对象", path)
		}
	}
	if headers, ok := model["headers"]; ok && headers != nil {
		if err := validateStringMap(headers, path+".headers"); err != nil {
			return err
		}
	}
	contextWindow, hasContextWindow := modelNumber(model["contextWindow"], path+".contextWindow")
	if _, ok := model["contextWindow"]; ok && !hasContextWindow {
		return fmt.Errorf("%s.contextWindow 必须是正数", path)
	}
	maxTokens, hasMaxTokens := modelNumber(model["maxTokens"], path+".maxTokens")
	if _, ok := model["maxTokens"]; ok && !hasMaxTokens {
		return fmt.Errorf("%s.maxTokens 必须是正数", path)
	}
	if maxContextWindow, ok := modelNumber(model["maxContextWindow"], path+".maxContextWindow"); ok {
		if hasContextWindow && maxContextWindow < contextWindow {
			return fmt.Errorf("%s.maxContextWindow 不能小于 contextWindow", path)
		}
	} else if _, ok := model["maxContextWindow"]; ok {
		return fmt.Errorf("%s.maxContextWindow 必须是正数", path)
	}
	_ = maxTokens
	return nil
}

// validateStringMap 校验字符串映射（headers 一类）。
func validateStringMap(value interface{}, path string) error {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			if _, isString := item.(string); !isString {
				return fmt.Errorf("%s.%s 必须是字符串", path, key)
			}
		}
		return nil
	case map[string]string:
		return nil
	default:
		return fmt.Errorf("%s 必须是映射对象", path)
	}
}

// validateDiscovery 校验 discovery 的协议约束。
func validateDiscovery(value interface{}, path string, provider map[string]interface{}) error {
	mapping, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s 必须是映射对象", path)
	}
	discoveryType := ""
	if item, ok := mapping["type"]; ok && item != nil {
		text, err := stringField(item, path+".type")
		if err != nil {
			return err
		}
		discoveryType = text
	}
	if discoveryType == "" {
		return fmt.Errorf("%s.type 不能为空", path)
	}
	if providerAPI, ok := provider["api"]; !ok || providerAPI == nil || providerAPI == "" {
		if discoveryType != "proxy" {
			return fmt.Errorf("%s：provider 未提供 api 时 discovery.type 只能是 proxy", path)
		}
	}
	if flag, ok := mapping["injectV1"]; ok && flag != nil {
		if enabled, isBool := flag.(bool); isBool && enabled && discoveryType != "openai-models-list" {
			return fmt.Errorf("%s.injectV1 只支持 openai-models-list", path)
		}
	}
	if flag, ok := mapping["advertisedReasoning"]; ok && flag != nil {
		if enabled, isBool := flag.(bool); isBool && enabled {
			switch discoveryType {
			case "openai-models-list", "lm-studio", "proxy":
			default:
				return fmt.Errorf("%s.advertisedReasoning 只支持 openai-models-list、lm-studio 或 proxy", path)
			}
		}
	}
	return nil
}

// validateInput 校验模型输入模态。
func validateInput(value interface{}, path string) error {
	items := make([]interface{}, 0, 2)
	switch typed := value.(type) {
	case string:
		items = append(items, typed)
	case []interface{}:
		items = typed
	case []string:
		for _, item := range typed {
			items = append(items, item)
		}
	default:
		return fmt.Errorf("%s 必须是字符串或字符串数组", path)
	}
	for _, item := range items {
		text, isString := item.(string)
		if !isString {
			return fmt.Errorf("%s 的元素必须是字符串", path)
		}
		if !supportedInputs[text] {
			return fmt.Errorf("%s 只支持 text 与 image，实际是 %q", path, text)
		}
	}
	return nil
}

// validateThinking 校验 thinking 结构。
func validateThinking(value interface{}, path string) error {
	mapping, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s 必须是映射对象", path)
	}
	mode := ""
	if item, ok := mapping["mode"]; ok && item != nil {
		text, err := stringField(item, path+".mode")
		if err != nil {
			return err
		}
		mode = strings.TrimSpace(text)
	}
	if mode == "" {
		return fmt.Errorf("%s.mode 不能为空", path)
	}
	hasEfforts := false
	if item, ok := mapping["efforts"]; ok && item != nil {
		hasEfforts = true
		levels, err := levelList(item, path+".efforts")
		if err != nil {
			return err
		}
		if len(levels) == 0 {
			return fmt.Errorf("%s.efforts 不能为空", path)
		}
		if defaultLevel, ok := mapping["defaultLevel"]; ok && defaultLevel != nil {
			text, err := stringField(defaultLevel, path+".defaultLevel")
			if err != nil {
				return err
			}
			if !containsString(levels, text) {
				return fmt.Errorf("%s.defaultLevel %q 必须在 efforts 里", path, text)
			}
		}
	}
	_, hasLevels := mapping["levels"]
	_, hasMin := mapping["minLevel"]
	_, hasMax := mapping["maxLevel"]
	if !hasEfforts && !hasLevels && !(hasMin && hasMax) {
		return fmt.Errorf("%s：必须提供 efforts、levels 或 minLevel+maxLevel 之一", path)
	}
	return nil
}

// levelList 把档位列表校验并归一。
func levelList(value interface{}, path string) ([]string, error) {
	var items []interface{}
	switch typed := value.(type) {
	case []interface{}:
		items = typed
	case []string:
		for _, item := range typed {
			if !isEffortLevel(item) {
				return nil, fmt.Errorf("%s 含有非法档位 %q（只支持 minimal、low、medium、high、xhigh、max）", path, item)
			}
		}
		return typed, nil
	default:
		return nil, fmt.Errorf("%s 必须是字符串数组", path)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, isString := item.(string)
		if !isString {
			return nil, fmt.Errorf("%s 的元素必须是字符串", path)
		}
		if !isEffortLevel(text) {
			return nil, fmt.Errorf("%s 含有非法档位 %q（只支持 minimal、low、medium、high、xhigh、max）", path, text)
		}
		out = append(out, text)
	}
	return out, nil
}

// validateCost 校验 cost 的四个必需数值字段。
func validateCost(value interface{}, path string) error {
	mapping, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("%s 必须是映射对象", path)
	}
	for _, key := range []string{"input", "output", "cacheRead", "cacheWrite"} {
		item, ok := mapping[key]
		if !ok || item == nil {
			return fmt.Errorf("%s.%s 缺失（cost 必须包含 input、output、cacheRead、cacheWrite）", path, key)
		}
		if _, isNumber := toFloat(item); !isNumber {
			return fmt.Errorf("%s.%s 必须是数值", path, key)
		}
	}
	return nil
}

// providerMapSatisfiesG2 判断 models 为空时 provider 是否仍被 omp 接受。
func providerMapSatisfiesG2(raw map[string]interface{}) bool {
	for _, key := range []string{"baseUrl", "apiKey", "guardrailIdentifier"} {
		if value, ok := raw[key]; ok && value != nil {
			if text, isString := value.(string); isString && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	if auth, ok := raw["auth"]; ok {
		if text, isString := auth.(string); isString && text == "none" {
			return true
		}
	}
	if flag, ok := raw["disableStrictTools"]; ok {
		if enabled, isBool := flag.(bool); isBool && enabled {
			return true
		}
	}
	for _, key := range []string{"headers", "compat", "requestMetadata", "remoteCompaction", "modelOverrides", "discovery"} {
		if value, ok := raw[key]; ok && value != nil {
			if mapping, isMap := value.(map[string]interface{}); isMap && len(mapping) > 0 {
				return true
			}
		}
	}
	return false
}

// modelNumber 读取正数模型参数。
func modelNumber(value interface{}, path string) (float64, bool) {
	if value == nil {
		return 0, false
	}
	number, ok := toFloat(value)
	if !ok || number <= 0 {
		return 0, false
	}
	return number, true
}

// toFloat 把数值归一成 float64。
func toFloat(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	}
	return 0, false
}

// isEffortLevel 判断是否为六档之一。
func isEffortLevel(value string) bool {
	for _, level := range EffortLevels {
		if level == value {
			return true
		}
	}
	return false
}

// containsString 判断切片是否包含指定值。
func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
