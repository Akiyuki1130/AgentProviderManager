package zcodeprovider

import (
	"fmt"
	"sort"
	"strings"
)

// legacyProviderKnownKeys 是 FromLegacy 能映射的 legacy provider 顶层键。
// 其余键（source、zcode、apiKeyRequired 之外的额外 options 等）都进 DroppedFields。
//
// 键集与 ZCode 3.14 的旧版 zod schema 对齐：enabled / apiFormat / defaultKind /
// headers / apiKeyUrl 都会被映射到新版，因此不再算“丢失”。
var legacyProviderKnownKeys = map[string]bool{
	"id":          true, // provider 映射的键本身即 ID，个别配置会冗余带上，不算丢失
	"name":        true,
	"kind":        true,
	"options":     true,
	"models":      true,
	"enabled":     true,
	"apiFormat":   true,
	"defaultKind": true,
	"headers":     true,
	"apiKeyUrl":   true,
	"api":         true, // 顶层 api 字符串：baseUrl 的兜底来源
	// endpoints 由 DroppedFields 单独按子键处理（baseURL 可能被用掉，paths 一定丢失），
	// 这里列入已知键只是为了不让通用展开把它整块报成丢失。
	"endpoints": true,
}

var legacyOptionKnownKeys = map[string]bool{
	"baseURL": true,
	"apiKey":  true,
}

var legacyEndpointKnownKeys = map[string]bool{
	"baseURL": true,
}

var legacyModelKnownKeys = map[string]bool{
	"limit":           true,
	"reasoning":       true,
	"modalities":      true,
	"contextWindow":   true,
	"maxOutputTokens": true,
	"deleted":         true, // deleted 模型会被跳过（与 ZCode 的迁移一致）
	// 以下键都是 APM 自己写进中间形态的（cards.go 的 CardToCfg）：
	//   - `_apm_capabilities` 是卡片 ↔ 写端之间的内部能力通道（带 `_` 前缀，写盘出口
	//     会被 stripAPMInternalModelKeys 剥离）；
	//   - tool_call / attachment / temperature 是 opencode kind 额外写出的**真实**能力键。
	// 它们都会被 modelRuleFromLegacy 消费（见下方的 legacyInputFormat /
	// legacyOutputFormat / legacyCapabilityPropertyKeys），因此列入已知键，
	// 否则自己写出去的东西会被误报成 DroppedFields。
	"_apm_capabilities": true,
	"tool_call":         true,
	"attachment":        true,
	"temperature":       true,
}

var legacyLimitKnownKeys = map[string]bool{
	"context": true,
	"output":  true,
}

var legacyReasoningKnownKeys = map[string]bool{
	"variants": true,
	// offAlias 是 APM 在“卡片 -> legacy 中间形态”往返时写入的内部字段，
	// 记录 off 词的原始拼法；它不是用户手写的 legacy 字段，列入已知键避免被误报为丢失。
	"offAlias": true,
}

var legacyModalitiesKnownKeys = map[string]bool{
	"input": true,
	// output 是 opencode kind 写出的真实键（`_apm_capabilities.output_modalities` 也覆盖它）：
	// legacyOutputFormat 会用它推 outputFormat.supportsText，因此同样算已知键。
	"output": true,
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
	apiType, err := apiTypeFromLegacy(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("提供商「%s」：%v", pid, err)
	}
	opts, _ := raw["options"].(map[string]interface{})

	rule := NewProviderRule(pid, name, GroupStandardPersonal)
	providerCfg := RuleConfig(rule)
	providerCfg["api"] = NewAPI(apiType, legacyBaseURL(raw, opts))
	if headers, ok := raw["headers"].(map[string]interface{}); ok && len(headers) > 0 {
		api := ProviderAPI(providerCfg)
		api["headers"] = headers
	}
	access := map[string]interface{}{}
	if apiKey := stringValue(opts["apiKey"]); strings.TrimSpace(apiKey) != "" {
		access["type"] = AccessTypeAPIKey
		access["apiKey"] = apiKey
	}
	if keyURL := strings.TrimSpace(stringValue(raw["apiKeyUrl"])); keyURL != "" {
		access["type"] = AccessTypeAPIKey
		access["apiKeyManagementUrl"] = keyURL
	}
	if len(access) > 0 {
		providerCfg["access"] = access
	}
	if enabled, ok := raw["enabled"].(bool); ok {
		rule["enabled"] = enabled
	}

	var modelRules []map[string]interface{}
	if modelsRaw, ok := raw["models"]; ok {
		models, isMap := modelsRaw.(map[string]interface{})
		if !isMap && modelsRaw != nil {
			return nil, nil, fmt.Errorf("提供商「%s」的 models 字段格式错误（应为对象）", pid)
		}
		mids := sortedStringKeys(models)
		// ZCode 的迁移会跳过带 deleted 标记的模型；列表与模型规则都要跳过，
		// 否则这些墓碑模型会重新出现在模型列表里。
		kept := make([]string, 0, len(mids))
		for _, mid := range mids {
			mraw, _ := models[mid].(map[string]interface{})
			if mraw != nil {
				if deleted, ok := mraw["deleted"].(bool); ok && deleted {
					continue
				}
			}
			kept = append(kept, mid)
		}
		providerCfg["personalModelIds"] = toInterfaceSlice(kept)
		for _, mid := range kept {
			mraw, _ := models[mid].(map[string]interface{})
			if mraw == nil {
				continue
			}
			modelRules = append(modelRules, modelRuleFromLegacy(pid, mid, mraw))
		}
	}
	return rule, modelRules, nil
}

// legacyBaseURL 复刻 ZCode 迁移时的 baseUrl 取值顺序：
// options.baseURL → endpoints.baseURL → 顶层 api 字符串。
func legacyBaseURL(raw, opts map[string]interface{}) string {
	if base := strings.TrimSpace(stringValue(opts["baseURL"])); base != "" {
		return base
	}
	if endpoints, ok := raw["endpoints"].(map[string]interface{}); ok {
		if base := strings.TrimSpace(stringValue(endpoints["baseURL"])); base != "" {
			return base
		}
	}
	return strings.TrimSpace(stringValue(raw["api"]))
}

func modelRuleFromLegacy(pid, mid string, mraw map[string]interface{}) map[string]interface{} {
	rule := NewProviderModelRule(pid, mid)
	mcfg := RuleConfig(rule)

	properties := map[string]interface{}{}
	optionSpecs := map[string]interface{}{}

	// 上下文窗口：模型级 contextWindow 优先于 limit.context（与 ZCode 的迁移一致）。
	if n, ok := positiveIntValue(mraw["contextWindow"]); ok {
		properties["contextWindow"] = n
	} else {
		limit, _ := mraw["limit"].(map[string]interface{})
		if n, ok := positiveIntValue(limit["context"]); ok {
			properties["contextWindow"] = n
		}
	}
	if n, ok := positiveIntValue(mraw["maxOutputTokens"]); ok {
		optionSpecs["maxOutputTokens"] = map[string]interface{}{"max": n}
	} else {
		limit, _ := mraw["limit"].(map[string]interface{})
		if n, ok := positiveIntValue(limit["output"]); ok {
			optionSpecs["maxOutputTokens"] = map[string]interface{}{"max": n}
		}
	}

	if values := legacyReasoningVariants(mraw); len(values) > 0 {
		optionSpecs["reasoningLevel"] = map[string]interface{}{"values": toInterfaceSlice(values)}
	}

	if format := legacyInputFormat(mraw); len(format) > 0 {
		properties["inputFormat"] = format
	}
	if format := legacyOutputFormat(mraw); len(format) > 0 {
		properties["outputFormat"] = format
	}
	// 布尔能力位只来自卡片侧的内部通道，且只在**明确为真**时写：未知/假一律省略键。
	// 绝不写 requiresMfjsToolSchema（本轮不做）与 optionSpecs.*.map（那是注入表达式，
	// 由 ZCode 内建数据提供，APM 无从推断）。
	caps := apmCapabilities(mraw)
	for _, item := range legacyCapabilityPropertyKeys {
		if apmCapabilityTrue(caps, item.capability) {
			properties[item.property] = true
		}
	}

	if len(properties) > 0 {
		mcfg["properties"] = properties
	}
	if len(optionSpecs) > 0 {
		mcfg["optionSpecs"] = optionSpecs
	}
	// 注意：这里绝不写 mcfg["enabled"]。v2 modelRule 的 config.enabled 是「该模型是否
	// 启用」的启用位（内建数据里只出现在 templateModelRules/builtinProviderModelRules），
	// 与「是否开启思考」无关。legacy 中间形态的 reasoning.enabled 只服务于卡片/DSH 语义，
	// 写进 config.enabled 会把模型在列表里误关。
	return rule
}

// legacyReasoningVariants 把 legacy 的 reasoning.variants 映射为 reasoningLevel.values。
//
// ZCode 代码里的 resolveLegacyReasoningLevel 会把新值 "disabled" 映射回旧的 level，
// 且内建数据的取值集合是 ["disabled","enabled"] / ["low","high","max"]，因此规范 off 值
// "off" 需要改名为 "disabled"。
//
// off 词的具体拼法必须保真（修 A-1）：values 里的 off 词要落在该模型原始集合内——
// 用户配置里原本是什么就写回什么。规则：
//   - reasoning["offAlias"]（APM 卡片往返时记下的原拼法）存在时，用它统一替换 off 词；
//   - 缺失时，仅把规范拼法 "off" 兜底改名成 ZCode 侧规范拼法 "disabled"，
//     而 none / nothink / disabled 这类原拼法原样保留（它们就是用户配置里的拼法）。
func legacyReasoningVariants(mraw map[string]interface{}) []string {
	reasoning, _ := mraw["reasoning"].(map[string]interface{})
	if reasoning == nil {
		return nil
	}
	raw := stringSlice(reasoning["variants"])
	if len(raw) == 0 {
		return nil
	}
	offAlias := ""
	switch strings.ToLower(strings.TrimSpace(stringValue(reasoning["offAlias"]))) {
	case "disabled", "none", "nothink":
		offAlias = strings.ToLower(strings.TrimSpace(stringValue(reasoning["offAlias"])))
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, v := range raw {
		v = strings.ToLower(strings.TrimSpace(v))
		switch v {
		case "off":
			if offAlias != "" {
				v = offAlias
			} else {
				v = "disabled"
			}
		case "none", "disabled", "nothink":
			if offAlias != "" {
				v = offAlias
			}
			// 无 offAlias 时保留用户原拼法，不强行改写。
		}
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// legacyInputFormatKeys 是中立输入模态词到 v2 `inputFormat.supports*` 键的映射，
// 顺序与 ZCode 的 canonical 键序（order.go 的 inputFormatOrder）一致。
var legacyInputFormatKeys = []struct {
	word string
	key  string
}{
	{"text", "supportsText"},
	{"image", "supportsImage"},
	{"video", "supportsVideo"},
	{"audio", "supportsAudio"},
	{"pdf", "supportsPdf"},
}

// v2ImpliedTextKey 是 v2 输入面里唯一「**缺键即默认支持**」的键（只影响读端，写端仍只写真）。
const v2ImpliedTextKey = "supportsText"

// InputFormatModalities 把 v2 `properties.inputFormat` 读成「已知为真」的输入模态词，
// 按 canonical 键序（text,image,video,audio,pdf）返回；没有 inputFormat 段（nil）时返回 nil。
//
// **supportsText 走 implied（缺键即真）语义**：只有显式 `false` 才排除 text——缺键、null、
// 非布尔一律按「默认支持文本」处理。依据与生成脚本的输入侧口径完全一致
// （scripts/sync-model-catalog.mjs 的 IMPLIED_TRUE_KEYS）：ZCode 的输入面里 text 是**默认项**，
// 其余模态都是在 text 之上追加的增量；ZCode 自己写个人规则时 `inputFormatValue` 的默认值就是
// `{supportsText: n?.supportsText ?? true, supportsImage: … ?? false, …}`，而它的最小增量
// 写入器（`n===r ? delete e[t] : e[t]=n`）会把「与继承值相等」的键删掉——因此 ZCode 设置页
// 编辑过（或经 APM 保存过）的个人规则通常根本不带 supportsText 键。
// 若按严格 `=== true` 读，这类规则会被读成「输入 = image」之类**不含 text** 的集合；
// 而中立层的模态是整字段「前层胜出」，这个残缺集合会把下层（离线目录 / 站点规则）给出的
// 完整集合整段挡住（本缺陷的一手证据与后果见 capability.go 的站点层注释）。
//
// image / video / audio / pdf 与 outputFormat 一律维持严格 `=== true`：它们是叠加在 text 之上
// 的增量，缺键只说明这条规则没提到它，当成「支持」就是凭空猜测。outputFormat 更是只有
// supportsText 一位、表达不了非文本输出，「缺键即默认 text」会抹掉下层对 TTS 这类模型的
// 正确读数（详见生成脚本里 IMPLIED_TRUE_KEYS 的注释）。
func InputFormatModalities(format map[string]interface{}) []string {
	if format == nil {
		return nil
	}
	var out []string
	for _, item := range legacyInputFormatKeys {
		if item.key == v2ImpliedTextKey {
			if b, ok := format[item.key].(bool); ok && !b {
				continue
			}
			out = append(out, item.word)
			continue
		}
		if b, _ := format[item.key].(bool); b {
			out = append(out, item.word)
		}
	}
	return out
}

// legacyCapabilityPropertyKeys 是中立布尔能力位到 v2 `properties` 键的映射（写端唯一口径，
// 见 MODEL-MANAGEMENT-PLAN.md 10.2.3）。顺序即 propertiesOrder 里这几个键的顺序。
var legacyCapabilityPropertyKeys = []struct {
	capability string
	property   string
}{
	{"tool_call", "supportsToolCall"},
	{"structured_output", "supportsJsonSchemaOutput"},
	{"native_web_search", "supportsNativeWebSearch"},
	{"mid_conv_system", "supportsMidConversationSystem"},
}

// apmCapabilities 读取卡片 ↔ 写端之间的内部能力通道 `_apm_capabilities`。
//
// 该键由 cards.go 的 CardToCfg 写入（数据源是 card.Capabilities），带 `_` 前缀，写盘出口
// 由 stripAPMInternalModelKeys 统一剥离，因此它永远不会落到用户配置里。形状见
// MODEL-MANAGEMENT-PLAN.md 10.2.4②。
func apmCapabilities(mraw map[string]interface{}) map[string]interface{} {
	m, _ := mraw["_apm_capabilities"].(map[string]interface{})
	return m
}

// apmCapabilityTrue 判断某个能力位是否**明确为真**。只认 bool true：键缺失、nil、false 或
// 别的类型一律为假。写入端据此省略键——v2 的 overlay 语义里省略 = 不覆盖，而 null 是显式
// 清空、false 是显式关闭，两者都绝不能写。
func apmCapabilityTrue(caps map[string]interface{}, key string) bool {
	v, ok := caps[key].(bool)
	return ok && v
}

// legacyModalitiesInput 读中间形态里真实键 `modalities.input` 的原始词表。
// 这里不做词表过滤：规范词表由 legacyInputFormatKeys 负责（未列出的词自然取不到键）。
func legacyModalitiesInput(mraw map[string]interface{}) []string {
	modalities, _ := mraw["modalities"].(map[string]interface{})
	return stringSlice(modalities["input"])
}

// legacyInputFormat 把中间形态里**明确列出**的输入模态映射为 v2 的
// `inputFormat.supports{Text,Image,Video,Audio,Pdf}`（5 个词全部映射）。
//
// 早先这里只映射 image/video，注释理由是「不推断 text/audio/pdf，避免编造用户数据」。
// 扩到全部 5 词并不是放宽这条纪律：本函数读的是**数据本身**——用户配置或导入数据里明确
// 列出的模态（真实键 `modalities.input` 与内部通道 `_apm_capabilities.input_modalities`），
// 如实转写而已。「不推断」依然成立：没有列出的模态一律不写键，绝不因为缺失就补上支持。
//
// 两个来源取并集（同一位不重复）：`modalities.input` 是 opencode kind 与导入数据里的真实键，
// `_apm_capabilities` 是卡片侧的能力声明。
func legacyInputFormat(mraw map[string]interface{}) map[string]interface{} {
	words := map[string]bool{}
	for _, modality := range legacyModalitiesInput(mraw) {
		words[strings.ToLower(strings.TrimSpace(modality))] = true
	}
	for _, modality := range stringSlice(apmCapabilities(mraw)["input_modalities"]) {
		words[strings.ToLower(strings.TrimSpace(modality))] = true
	}
	format := map[string]interface{}{}
	for _, item := range legacyInputFormatKeys {
		if words[item.word] {
			format[item.key] = true
		}
	}
	return format
}

// legacyOutputFormat 由输出模态推 v2 的 `outputFormat`：v2 目前只有 supportsText 一个键，
// 且只在**明确含 text** 时写真。来源同样是真实键 `modalities.output` 与内部通道
// `_apm_capabilities.output_modalities` 的并集。读不出 text 时返回 nil，调用方据此省略整键。
func legacyOutputFormat(mraw map[string]interface{}) map[string]interface{} {
	modalities, _ := mraw["modalities"].(map[string]interface{})
	for _, list := range [][]string{
		stringSlice(modalities["output"]),
		stringSlice(apmCapabilities(mraw)["output_modalities"]),
	} {
		for _, modality := range list {
			if strings.EqualFold(strings.TrimSpace(modality), "text") {
				return map[string]interface{}{"supportsText": true}
			}
		}
	}
	return nil
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
		// 用与卡片读端同一个读取器（InputFormatModalities）：v2 的 5 个输入位全数转写，
		// supportsText 走「缺键即真」的 implied 语义，其余四位严格只认 true。
		// 于是「v2 → legacy」不再丢掉 text（旧实现只搬 image/video，往返一次就把 text 丢了），
		// 而 legacy 的 `modalities.input` 恰好就是完整集合语义，转写是忠实的。
		if inputs := InputFormatModalities(format); len(inputs) > 0 {
			out["modalities"] = map[string]interface{}{"input": toInterfaceSlice(inputs)}
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
	// endpoints 只有 baseURL 会被当作 baseUrl 兜底用掉；paths 等其余内容在 v2 里
	// 没有对应位置（api.type 只承载“用哪种 API”，不承载路径后缀），因此照实上报。
	if endpoints, ok := legacyProvider["endpoints"].(map[string]interface{}); ok {
		opts, _ := legacyProvider["options"].(map[string]interface{})
		known := map[string]bool{}
		if endpointsBaseURLUsed(legacyProvider, opts) {
			known["baseURL"] = true
		}
		collectUnknown(set, "endpoints", endpoints, known)
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

// apiTypeFromLegacy 复刻 ZCode 3.14 迁移时的 api.type 取值顺序：
// apiFormat → defaultKind → kind → endpoints.paths → 兜底 chat-completions。
//
// kind 存在但无法映射时返回错误（与旧行为一致）：宁可直接报错，
// 也不要把提供商写成一个错误的 API 类型。
func apiTypeFromLegacy(raw map[string]interface{}) (string, error) {
	for _, key := range []string{"apiFormat", "defaultKind"} {
		v := strings.TrimSpace(stringValue(raw[key]))
		if v == "" {
			continue
		}
		if t, ok := apiTypeFromFormat(v); ok {
			return t, nil
		}
	}
	kind := strings.TrimSpace(stringValue(raw["kind"]))
	if t, err := apiTypeFromLegacyKind(kind); err == nil {
		return t, nil
	} else if kind != "" {
		return "", err
	}
	if t, ok := apiTypeFromEndpoints(raw); ok {
		return t, nil
	}
	return APITypeOpenAIChatCompletions, nil
}

func apiTypeFromFormat(v string) (string, bool) {
	switch v {
	case APITypeAnthropicMessages, APITypeOpenAIChatCompletions, APITypeOpenAIResponses:
		return v, true
	}
	return "", false
}

// apiTypeFromEndpoints 用 endpoints.paths 判定 API 类型（ZCode 的兜底顺序）。
func apiTypeFromEndpoints(raw map[string]interface{}) (string, bool) {
	endpoints, ok := raw["endpoints"].(map[string]interface{})
	if !ok {
		return "", false
	}
	paths, ok := endpoints["paths"].(map[string]interface{})
	if !ok {
		return "", false
	}
	if _, ok := paths["anthropic"]; ok {
		return APITypeAnthropicMessages, true
	}
	if _, ok := paths["openai"]; ok {
		return APITypeOpenAIResponses, true
	}
	if _, ok := paths["openai-compatible"]; ok {
		return APITypeOpenAIChatCompletions, true
	}
	return "", false
}

// endpointsBaseURLUsed 判断 endpoints.baseURL 是否被用作最终的 baseUrl。
func endpointsBaseURLUsed(raw, opts map[string]interface{}) bool {
	if strings.TrimSpace(stringValue(opts["baseURL"])) != "" {
		return false
	}
	endpoints, ok := raw["endpoints"].(map[string]interface{})
	if !ok {
		return false
	}
	return strings.TrimSpace(stringValue(endpoints["baseURL"])) != ""
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
