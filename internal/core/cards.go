package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"agentprovidermanager/internal/zcodeprovider"
)

// cardsContextMetaKeys / cardsOutputMetaKeys 是 provider /models 响应里上下文与输出上限的
// 键表，**顺序即优先级**（越靠前越优先，第一个能解析出正整数的键胜出）。
//
// 前 5 个键是既有口径（不得调整顺序），后面的是本次追加的别名：
//   - 上下文：max_input_tokens / context_size
//   - 输出：output_token_limit / max_new_tokens
var cardsContextMetaKeys = []string{
	"context_length", "max_context_length", "max_model_len", "context_window", "context",
	"max_input_tokens", "context_size",
}

var cardsOutputMetaKeys = []string{
	"max_completion_tokens", "max_output_tokens", "max_tokens", "output_length", "output",
	"output_token_limit", "max_new_tokens",
}

// BuildModelCards turns raw model pairs into frontend cards. When autoFill is
// true, empty context/output fall back to the built-in capability table
// (provider-returned values always win).
//
// Vendor 与补全开关无关：厂商分类纯展示、不写盘，因此无论 autoFill 真假都填。
//
// 本函数没有站点上下文，等价于 BuildModelCardsForSite(models, autoFill, "", "")。
func BuildModelCards(models []struct {
	ID   string
	Meta map[string]interface{}
}, autoFill bool) []ModelCard {
	return BuildModelCardsForSite(models, autoFill, "", "")
}

// BuildModelCardsForSite 是带站点上下文的读卡入口：把 provider 的 baseUrl 与 api.type
// 交给能力表（LookupModelCapabilityForSite），使离线目录与站点级规则也能参与补全。
//
// 优先级：
//   - ctx/out：provider 返回的值永远胜出（沿用 emptyToken 判定）；只有 autoFill 为真且
//     provider 没给时，才用能力表兜底。
//   - _supported_levels：provider 明确给出的档位是 API 事实，与 _vendor 同级，始终写入；
//     provider 没给时用能力表档位兜底——同样与补全开关无关（该字段只做展示与种子，见函数内注释）。
//   - Variants / DefaultVariant：思考已开时，用同一组支持档位自动勾选（集合里没有 off 就不勾
//     off）。这是**会被写进配置的值**，因此与 ctx/out 一样受 autoFill 门控。
//   - Reasoning：metaReasoning 给出明确值时以它为准，否则沿用 guessReasoningFromCapability
//     的推断（能力表档位 + 关键词表）。
//   - _vendor：与补全开关无关，始终填。
//   - Capabilities / _capabilities_inferred（中立能力位，阶段 E1）：provider 自报部分
//     是 API 事实，与 _supported_levels 同级、autoFill 为假也填；能力表部分是推断，只在
//     autoFill 为真时进 Capabilities，而 _capabilities_inferred 只做展示，两者都不写盘。
//
// baseURL 为空时结果与 BuildModelCards 完全一致（站点层不命中、离线目录层关闭）。
// apiType 取不到时传空串：限定 apiType 的站点规则会被跳过（fail-closed），而不是误命中。
func BuildModelCardsForSite(models []struct {
	ID   string
	Meta map[string]interface{}
}, autoFill bool, baseURL, apiType string) []ModelCard {
	cards := make([]ModelCard, 0, len(models))
	for _, m := range models {
		meta := m.Meta
		if meta == nil {
			meta = map[string]interface{}{}
		}
		ctx := metaToken(meta, cardsContextMetaKeys)
		out := metaToken(meta, cardsOutputMetaKeys)
		// 能力表查一次，供补全、厂商分类与思考判定复用（避免每个模型各跑几趟正则）。
		cap := LookupModelCapabilityForSite(m.ID, baseURL, apiType)
		// Provider-returned values win; fall back to the built-in capability
		// table only when the provider did not report a value.
		if autoFill {
			if emptyToken(ctx) && cap.Context > 0 {
				ctx = cap.Context
			}
			if emptyToken(out) && cap.Output > 0 {
				out = cap.Output
			}
		}
		// API 明确声明的思考支持态优先于按模型名/档位做的推断。
		reasoning := guessReasoningFromCapability(m.ID, cap)
		reasoningDeclared := false
		if v, known := metaReasoning(meta); known {
			reasoning = v
			reasoningDeclared = true
		}
		// provider 明确给出的档位优先（API 事实，与 _vendor 同级）；provider 没给时用能力表
		// 推断值兜底——**不受 autoFill 约束**，与下方同样只做展示的 _capabilities_inferred 一致。
		//
		// 为什么与 ctx/out 区别对待：autoFill（「导入时自动补全模型参数」）管的是
		// 「会被写进配置的值」，而 _supported_levels 从不写盘（写盘出口会剥离内部键），也不参与
		// 任何自动写值——它只驱动展示（档位自适应弱化）、种子（开思考时用支持集合）与「按能力表
		// 补全档位」这个显式动作，因此把开关关掉不该让档位推断一并消失（2026-10-15 阶段 F 修正）。
		// 注意区分下面由同一组档位派生的 Variants：那个才是「会被写进配置的值」，受开关门控。
		levels := metaLevels(meta)
		if len(levels) == 0 {
			levels = cap.ReasoningLevels
		}
		// provider 自报的档位枚举本身就是「该模型支持思考」这一事实：能力表与关键词都认不出这个
		// id 时上面的推断会给出 false，卡片于是「列出了可选档位却声明不支持思考」——前端据此禁用
		// 思考开关、批量档位补全也会把它当作「用户的显式决定」跳过。因此 provider 自报档位含非
		// off 档时补正为支持；provider **显式**声明过思考支持态时以它为准，绝不覆盖。
		if !reasoningDeclared && !reasoning && hasNonOffLevel(levels) {
			reasoning = true
		}
		// 「导入新供应商」时自动勾选支持的思考强度（2026-10-17）：思考已开且档位里有非 off 档
		// 时，直接把上面解析出的支持档位作为卡片的**初始选择**，让卡片上勾着的档位就是将要写进
		// 配置的档位。此前的做法是留 nil、由写盘侧回落 DefaultVariantsFor（写死的 {off,high,max}），
		// 于是卡片上一个档位都没勾、写出去还多了一个 off——glm-5.3 这类档位集合不含 off 的模型
		// 会被静默写成「可以关闭思考」。
		//
		// 开关口径：variants 会被写进配置，因此与 ctx/out、capabilities 同属「自动补全」开关的
		// 管辖范围（_supported_levels 只是展示与种子，照旧与开关无关）。
		var variants []string
		var defaultVariant string
		if autoFill && reasoning && hasNonOffLevel(levels) {
			variants = cloneStrings(levels)
			defaultVariant = PickDefaultVariant(variants)
		}
		// 中立能力位（阶段 E1，见 MODEL-MANAGEMENT-PLAN.md 10.2.2）：provider 自报的能力位
		// 是 API 事实，与 _supported_levels 口径一致——**autoFill 为假也照填**；能力表那部分
		// 是推断，只在 autoFill 为真时补进 Capabilities（绝不改动上面 ctx/out/档位推断的既有行为）。
		// 推断结果另有独立出口 _capabilities_inferred，供前端展示来源与「按能力表补全」按钮。
		// 「只记真」的口径（显式 false 一律丢弃）见 providerMetaFeatures 的注释。
		inferred := providerMetaFeatures(meta)
		mergedInferred := mergeFeatures(inferred, cap.Features)
		effective := inferred
		if autoFill {
			effective = mergedInferred
		}
		var capabilities *ModelCapabilities
		if effective.HasAny() {
			caps := effective
			capabilities = &caps
		}
		// HasAny() 为假时留 nil：空推断值在 DTO 里只会多出一个无意义的空对象。
		var inferredCapabilities *ModelCapabilities
		if mergedInferred.HasAny() {
			merged := mergedInferred
			inferredCapabilities = &merged
		}
		card := ModelCard{
			ModelID:              m.ID,
			APIReturned:          len(meta) > 0,
			Name:                 m.ID,
			Reasoning:            reasoning,
			Variants:             variants,
			DefaultVariant:       defaultVariant,
			Context:              ctx,
			Output:               out,
			Select:               true,
			Vendor:               cap.Vendor,
			SupportedLevels:      levels,
			Capabilities:         capabilities,
			CapabilitiesInferred: inferredCapabilities,
		}
		cards = append(cards, card)
	}
	return cards
}

// metaToken 按键表顺序取第一个能解析成正整数 token 数的值；全部读不出返回空串。
func metaToken(meta map[string]interface{}, keys []string) interface{} {
	for _, k := range keys {
		if v, ok := meta[k]; ok {
			if parsed := ParseTokens(v); parsed != nil {
				return *parsed
			}
		}
	}
	return ""
}

// metaLevels 从 provider 返回的模型 meta 里读出「API 明确声明的推理档位」。
//
// 按优先级识别（先给出非空原始值的来源胜出；该来源的值全被白名单滤掉时继续看下一来源）：
//  1. thinking_level / thinkingLevel / reasoning_effort / reasoning_efforts（字符串或字符串数组）
//  2. supported_efforts / reasoning.efforts / reasoning.supported_efforts（数组或字符串）
//  3. reasoning_options（数组；只取 type == "effort" 的 values 数组，toggle 与
//     budget_tokens 描述的不是档位枚举，一律不产档位）
//  4. capabilities（对象）：只声明「是否支持思考」，不给出档位枚举，因此不产档位，
//     由 metaReasoning 消费
//
// 归一交给 normalizeCapabilityLevels（白名单过滤 / 别名折叠 / 固定顺序排序）；空结果返回
// nil 而非空切片，以配合 DTO 的 omitempty。任何畸形输入都只是「读不出档位」，绝不 panic。
func metaLevels(meta map[string]interface{}) []string {
	if len(meta) == 0 {
		return nil
	}
	// reasoning 未必是对象（可能是 bool / 字符串），类型断言失败时按「没有」处理。
	reasoning, _ := meta["reasoning"].(map[string]interface{})
	sources := [][]string{
		metaStringList(meta["thinking_level"]),
		metaStringList(meta["thinkingLevel"]),
		metaStringList(meta["reasoning_effort"]),
		metaStringList(meta["reasoning_efforts"]),
		metaStringList(meta["supported_efforts"]),
		metaStringList(reasoning["efforts"]),
		metaStringList(reasoning["supported_efforts"]),
		metaReasoningOptionLevels(meta["reasoning_options"]),
	}
	for _, raw := range sources {
		if len(raw) == 0 {
			continue
		}
		if levels := normalizeCapabilityLevels(raw); len(levels) > 0 {
			return levels
		}
	}
	return nil
}

// metaReasoning 从 meta 里读出「API 是否声明支持思考」，返回 (值, 是否已知)。
//
// 取值顺序（第一个给出明确布尔的来源胜出）：
//  1. reasoning / thinking：布尔值，或对象里的 enabled / supported；
//  2. capabilities.reasoning / capabilities.thinking：布尔值；
//  3. supported_parameters：列表里出现 reasoning 或 include_reasoning 视为支持。
//     列表存在但没有列出这两项时**不做推断**（不猜），返回「未知」。
//
// 全部读不出（或全是否定证据之外的模糊形态）时返回 (false, false)，由调用方回落到
// 基于能力表与模型名的推断。
func metaReasoning(meta map[string]interface{}) (bool, bool) {
	if len(meta) == 0 {
		return false, false
	}
	for _, key := range []string{"reasoning", "thinking"} {
		if v, known := metaBool(meta[key]); known {
			return v, true
		}
		if m, ok := meta[key].(map[string]interface{}); ok {
			for _, inner := range []string{"enabled", "supported"} {
				if v, known := metaBool(m[inner]); known {
					return v, true
				}
			}
		}
	}
	if caps, ok := meta["capabilities"].(map[string]interface{}); ok {
		for _, key := range []string{"reasoning", "thinking"} {
			if v, known := metaBool(caps[key]); known {
				return v, true
			}
		}
	}
	for _, p := range metaStringList(meta["supported_parameters"]) {
		switch strings.ToLower(p) {
		case "reasoning", "include_reasoning":
			return true, true
		}
	}
	return false, false
}

// metaBool 只认真正的布尔值；字符串 / 数字等形态一律视为「未知」，避免把
// "false"（字符串）之类的表示误读成布尔语义。
func metaBool(value interface{}) (bool, bool) {
	if b, ok := value.(bool); ok {
		return b, true
	}
	return false, false
}

// metaReasoningOptionLevels 从 reasoning_options 里取 type == "effort" 的 values。
// toggle（开关）与 budget_tokens（token 预算）不是档位枚举，一律不产档位；
// 元素形状不对（nil、非对象、缺 type、values 不是字符串列表）时跳过，绝不 panic。
func metaReasoningOptionLevels(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	var out []string
	for _, item := range items {
		opt, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		kind, _ := opt["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(kind), "effort") {
			continue
		}
		out = append(out, metaStringList(opt["values"])...)
	}
	return out
}

// metaStringList 把 meta 里的档位值解析成字符串切片，兼容字符串、[]string 与
// []interface{}（元素只取字符串）；字符串会按逗号 / 分号 / 竖线 / 空白切分。
// 其它类型（数字、布尔、对象、nil）一律视为「读不出」，返回 nil。
func metaStringList(value interface{}) []string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		// strings.Fields 已按任意空白（含换行、制表符）切分，这里只补分隔符。
		return strings.Fields(strings.NewReplacer(",", " ", ";", " ", "|", " ").Replace(v))
	case []string:
		return trimMetaStrings(v)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return trimMetaStrings(out)
	}
	return nil
}

// trimMetaStrings 去掉元素两端空白并丢弃空串；全空时返回 nil。
func trimMetaStrings(items []string) []string {
	var out []string
	for _, raw := range items {
		if s := strings.TrimSpace(raw); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// providerMetaFeatures 从 provider `/models` 的模型元数据里读「provider 自报的能力位」。
// 这是 **API 事实**（与 metaLevels / metaReasoning 同级），不是 APM 的推断。
//
// 只认下面这些键（其余键一概不看，避免把别家 API 的同名但语义不同的字段误读进来）：
//   - 输入模态：modalities.input、input_modalities、architecture.input_modalities
//   - 输出模态：modalities.output、output_modalities、architecture.output_modalities
//   - 布尔位：tool_call / supports_tool_call；structured_output / supports_structured_output /
//     json_schema；native_web_search / supports_native_web_search / web_search；
//     mid_conv_system / supports_mid_conversation_system；attachment / supports_attachment；
//     temperature / supports_temperature
//   - capabilities 子对象里的同名子键：tool_call / structured_output / native_web_search /
//     attachment / temperature
//
// 统一口径（贯穿卡片层与写入端）：**只记录为真的位**。
//   - 多键冲突时「任一为真即为真」（正向集合语义，模态取并集）；
//   - provider 显式说 false 时**丢弃**（记成 nil），不记成 false——中立层的写盘策略是
//     「只写真、留空让目标 harness 自己的默认值生效」，而记下 false 既没有落点，又容易在
//     展示上被误读成「已确认不支持」；
//   - 类型不符（字符串 "true"、数字、对象等）一律忽略（fail-closed，与 metaBool 同口径）；
//   - InputModalities / OutputModalities 同理只存「已知支持」的正向集合，未知 = nil。
//
// 任何畸形输入都只是「读不出能力位」，绝不 panic。
func providerMetaFeatures(meta map[string]interface{}) ModelFeatures {
	var features ModelFeatures
	if len(meta) == 0 {
		return features
	}
	// capabilities 未必是对象（可能是字符串 / 布尔），断言失败时按「没有」处理。
	sub, _ := meta["capabilities"].(map[string]interface{})

	features.InputModalities = NormalizeInputModalities(metaFeatureModalities(meta,
		"modalities.input", "input_modalities", "architecture.input_modalities"))
	features.OutputModalities = NormalizeOutputModalities(metaFeatureModalities(meta,
		"modalities.output", "output_modalities", "architecture.output_modalities"))

	features.ToolCall = metaFeatureFlag(meta, sub,
		[]string{"tool_call", "supports_tool_call"}, []string{"tool_call"})
	features.StructuredOutput = metaFeatureFlag(meta, sub,
		[]string{"structured_output", "supports_structured_output", "json_schema"}, []string{"structured_output"})
	features.NativeWebSearch = metaFeatureFlag(meta, sub,
		[]string{"native_web_search", "supports_native_web_search", "web_search"}, []string{"native_web_search"})
	features.MidConvSystem = metaFeatureFlag(meta, sub,
		[]string{"mid_conv_system", "supports_mid_conversation_system"}, nil)
	features.Attachment = metaFeatureFlag(meta, sub,
		[]string{"attachment", "supports_attachment"}, []string{"attachment"})
	features.Temperature = metaFeatureFlag(meta, sub,
		[]string{"temperature", "supports_temperature"}, []string{"temperature"})

	// Source 只在确实读到了能力位时标注，避免零值结构体上留一个无意义的来源标记。
	// 它是 APM 内部字段（`_source`），写入端从不落盘。
	if features.HasAny() {
		features.Source = "provider"
	}
	return features
}

// metaFeatureFlag 按「任一来源为真即为真」读一个能力布尔位：topKeys 是顶层候选键，
// subKeys 是 capabilities 子对象里的同名子键。
//
// 只有**已知为真**才返回非 nil；显式 false 与类型不符一样被丢弃（见 providerMetaFeatures
// 的口径说明）。nil 表示「未知」，与「不支持」不同——未知不落盘。
func metaFeatureFlag(meta, sub map[string]interface{}, topKeys, subKeys []string) *bool {
	for _, key := range topKeys {
		if v, known := metaBool(meta[key]); known && v {
			return featureBool(true)
		}
	}
	for _, key := range subKeys {
		if v, known := metaBool(sub[key]); known && v {
			return featureBool(true)
		}
	}
	return nil
}

// metaFeatureModalities 按「任一来源都算数」的并集语义收集模态列表：paths 支持
// "a" 与 "a.b" 两种深度，任一段缺失或不是对象时该路径整体忽略（fail-closed）。
// 返回的是原始词串（未规范化），由调用方交给 NormalizeInput/OutputModalities 过滤未知词。
func metaFeatureModalities(meta map[string]interface{}, paths ...string) []string {
	var out []string
	for _, path := range paths {
		out = append(out, metaStringList(metaPathValue(meta, path))...)
	}
	return out
}

// metaPathValue 取 meta 里的取值，path 支持 "a" / "a.b" 两种深度。任一段缺失、或中间段
// 不是对象时返回 nil（宁可不读，也不猜）。
func metaPathValue(meta map[string]interface{}, path string) interface{} {
	if meta == nil {
		return nil
	}
	segments := strings.Split(path, ".")
	current := meta
	for i, segment := range segments {
		value, ok := current[segment]
		if !ok {
			return nil
		}
		if i == len(segments)-1 {
			return value
		}
		next, ok := value.(map[string]interface{})
		if !ok {
			return nil
		}
		current = next
	}
	return nil
}

func BuildSingleCard(modelID string, autoFill bool) (ModelCard, error) {
	mid := strings.TrimSpace(modelID)
	if mid == "" {
		return ModelCard{}, fmt.Errorf("模型 ID 不能为空")
	}
	if len(mid) > MaxModelIDLength {
		return ModelCard{}, fmt.Errorf("模型 ID 不能超过 %d 个字符", MaxModelIDLength)
	}
	cards := BuildModelCards([]struct {
		ID   string
		Meta map[string]interface{}
	}{{ID: mid, Meta: map[string]interface{}{}}}, autoFill)
	return cards[0], nil
}

// CfgToCard 把配置里的模型节点还原成编辑器卡片。
//
// 保持既有签名与行为：等价于 CfgToCardForSite(modelID, modelCfg, "", "")。
func CfgToCard(modelID string, modelCfg map[string]interface{}) ModelCard {
	return CfgToCardForSite(modelID, modelCfg, "", "")
}

// CfgToCardForSite 是带站点上下文的读卡入口：把 provider 的 baseUrl 与 api.type 交给
// 能力表（LookupModelCapabilityForSite），使离线目录与站点级规则也能参与
// _vendor / _supported_levels 的推断。
//
// 中立能力位（阶段 E1）：配置里读到的能力（内部通道 _apm_capabilities / opencode 真实键 /
// v2 形状的 properties.*）填进 card.Capabilities；能力表的推断值在**有已知能力位时**填进
// _capabilities_inferred（为空则留 nil，避免每张卡片带一个无意义的空对象）。
// 既有的 card.Attachment / card.Modalities 行为完全不变（opencode 往返契约依赖它们）。
//
// 与 CfgToCard 共用同一实现体（后者只是转调本函数并传空 site），因此不存在两套逻辑；
// baseURL 为空时结果与 CfgToCard 完全一致。
// apiType 取不到时传空串：限定 apiType 的站点规则会被跳过（fail-closed），而不是误命中。
func CfgToCardForSite(modelID string, modelCfg map[string]interface{}, baseURL, apiType string) ModelCard {
	if modelCfg == nil {
		modelCfg = map[string]interface{}{}
	}
	var limit map[string]interface{}
	if l, ok := modelCfg["limit"].(map[string]interface{}); ok {
		limit = l
	} else {
		limit = map[string]interface{}{}
	}
	var enabled bool
	var enabledKnown bool
	// 非 nil 空切片：variants 没有 omitempty，nil 会编码成 JSON null。
	variants := make([]string, 0)
	var defaultVariant string
	var rawEnabled *bool
	// offAlias 记录配置里 off 档位的原始拼法（disabled/none/nothink），
	// 写回时必须用它，否则 ZCode 的 map 表达式匹配不到（见 ModelCard.OffAlias）。
	var offAlias string
	if reasoning, ok := modelCfg["reasoning"].(map[string]interface{}); ok {
		if e, ok := reasoning["enabled"].(bool); ok {
			enabled = e
			enabledKnown = true
			b := e
			rawEnabled = &b
		}
		// NormalizeVariants 对 nil / 全被过滤掉的输入返回 nil，直接赋值会把上面初始化好的
		// 空切片又变回 nil，所以只在确实有档位时覆盖（长度语义与原来完全一致）。
		if nv := NormalizeVariants(reasoning["variants"]); len(nv) > 0 {
			variants = nv
		}
		// 原始串在这里才能看到：NormalizeVariants 已把 off 词折成 "off"，
		// 必须在规范化之前记下原拼法。
		offAlias = firstOffAlias(reasoning["variants"])
		if dv, ok := reasoning["defaultVariant"].(string); ok {
			dv = canonicalVariant(dv)
			if containsStr(variants, dv) {
				defaultVariant = dv
			}
		}
	} else if enabledRaw, ok := modelCfg["reasoning"].(bool); ok {
		enabled = enabledRaw
		enabledKnown = true
		b := enabledRaw
		rawEnabled = &b
		if vm, ok := modelCfg["variants"].(map[string]interface{}); ok {
			for key, value := range vm {
				if item, ok := value.(map[string]interface{}); ok && item["disabled"] == true {
					continue
				}
				if offAlias == "" {
					offAlias = offAliasFromVariant(key)
				}
				key = canonicalVariant(key)
				if (CustomVariantsAllowed() || ZCodeVariantSet[key]) && !containsStr(variants, key) {
					variants = append(variants, key)
				}
			}
		}
		variants = sortVariantsCanonical(variants)
	}
	if len(variants) == 0 {
		if efforts, ok := modelCfg["reasoningEfforts"].(map[string]interface{}); ok {
			for key, value := range efforts {
				if offAlias == "" {
					offAlias = offAliasFromVariant(key)
				}
				key = canonicalVariant(key)
				if key == "off" || value != nil {
					if (CustomVariantsAllowed() || ZCodeVariantSet[key]) && !containsStr(variants, key) {
						variants = append(variants, key)
					}
				}
			}
			variants = sortVariantsCanonical(variants)
			if !enabledKnown {
				enabled = len(variants) > 1 || (len(variants) == 1 && variants[0] != "off")
			}
			if enabled {
				b := enabled
				rawEnabled = &b
			}
		}
	}
	name := modelID
	if n, ok := modelCfg["name"].(string); ok && strings.TrimSpace(n) != "" {
		name = n
	}
	reasoningEnabled := enabled
	if !enabledKnown {
		reasoningEnabled = len(variants) > 0 && !(len(variants) == 1 && variants[0] == "off")
	}
	// 中间 legacy 显式带 offAlias（v2/DSH 读入侧会写）时优先采信它：这是配置里
	// off 词的确切拼法，比从 variants 里回推更可靠。
	if reasoning, ok := modelCfg["reasoning"].(map[string]interface{}); ok {
		if a, ok := reasoning["offAlias"].(string); ok {
			if s := strings.TrimSpace(strings.ToLower(a)); s != "" {
				offAlias = s
			}
		}
	}
	card := ModelCard{
		ModelID: modelID, APIReturned: false, Name: name,
		Reasoning: reasoningEnabled, RawReasoningEnabled: rawEnabled,
		Variants: variants, DefaultVariant: defaultVariant, OffAlias: offAlias,
		Context: limit["context"], Output: limit["output"], Select: false,
		RawCfg: deepCopyMap(modelCfg),
	}
	// 厂商与支持档位是 APM 依据内建能力表做的独立推断（纯展示/档位自适应），
	// 与配置里当前选中的档位集合（variants）互不影响，绝不覆盖 variants。
	//
	// 阶段 C1：此前只有 BuildModelCards（拉取新模型）填 _supported_levels，
	// 编辑已有 provider 的 CfgToCard 路径没填，导致档位自适应只在导入新模型时生效。
	// 这里补上，使 ManagePage 编辑已有 provider 时也能拿到支持态。
	// 带上站点上下文后，离线目录与站点级规则（例如 opencode 网关专属档位）同样参与推断。
	cap := LookupModelCapabilityForSite(modelID, baseURL, apiType)
	// 厂商判定不依赖任何开关，纯展示，始终填。
	card.Vendor = cap.Vendor
	// 只在能力表确实给出档位时填 SupportedLevels（cap.ReasoningLevels 已规范化 + 排序）；
	// 为空则保持 nil，前端按「未推断到」处理。绝不覆盖上面从配置读到的 variants。
	if len(cap.ReasoningLevels) > 0 {
		card.SupportedLevels = cap.ReasoningLevels
	}
	// 中立能力位（阶段 E1，见 MODEL-MANAGEMENT-PLAN.md 10.2.2/10.2.4②）：配置里的取值就是
	// 事实，按「前者优先、后者只填空缺」读回三个来源：
	//   a) _apm_capabilities —— APM 卡片 ↔ 写入端之间的内部通道（键名冻结在 10.2.4②）；
	//   b) opencode 真实键 —— modalities{input,output} / attachment / tool_call / temperature；
	//   c) ZCode v2 形状的 properties.* —— 若中间形态里直接出现了 v2 的键（backend_zcode_v2
	//      将来透传 properties 时即可读到）。
	// 三个读回器与 providerMetaFeatures 同口径：只记真值，false / 未知一律不记。
	if features := mergeFeatures(
		mergeFeatures(
			cardFeaturesFromInternal(modelCfg["_apm_capabilities"]),
			cardFeaturesFromOpenCodeKeys(modelCfg),
		),
		cardFeaturesFromV2Properties(modelCfg["properties"]),
	); features.HasAny() {
		caps := features
		card.Capabilities = &caps
	}
	// _capabilities_inferred 是「能力表算出来的原始推断值」，只供展示与「按能力表补全」，
	// 绝不写盘（写入端只读 Capabilities）。本函数没有 autoFill 参数，因此不做开关判定——
	// 能力表给出任何已知能力位就填；**为空则留 nil**：空推断值在 DTO 里只会多出一个
	// `_capabilities_inferred: {}`，让每张未知模型的卡片都带上无意义的噪声。
	if tableFeatures := cap.Features; tableFeatures.HasAny() {
		inferred := tableFeatures
		card.CapabilitiesInferred = &inferred
	}
	if v, ok := modelCfg["attachment"].(bool); ok {
		card.Attachment = v
	}
	if mm, ok := modelCfg["modalities"].(map[string]interface{}); ok {
		card.Modalities = deepCopyMap(mm)
	}
	if h, ok := modelCfg["headers"].(map[string]interface{}); ok {
		card.Headers = map[string]string{}
		for k, v := range h {
			if s, ok := v.(string); ok {
				card.Headers[k] = s
			}
		}
	}
	if o, ok := modelCfg["options"].(map[string]interface{}); ok {
		card.Options = deepCopyMap(o)
	}
	return card
}

func CardToCfg(card ModelCard, kind string) (map[string]interface{}, string, error) {
	modelID := strings.TrimSpace(card.ModelID)
	if modelID == "" {
		return nil, "", fmt.Errorf("模型 ID 不能为空")
	}
	if len(modelID) > MaxModelIDLength {
		return nil, "", fmt.Errorf("模型 ID「%s」过长", ShortText(modelID, 40))
	}
	name := strings.TrimSpace(card.Name)
	if name == "" {
		name = modelID
	}
	if len(name) > MaxProviderNameLen {
		return nil, "", fmt.Errorf("模型「%s」的显示名称不能超过 %d 个字符", modelID, MaxProviderNameLen)
	}

	rawCfg := card.RawCfg
	if rawCfg == nil {
		rawCfg = map[string]interface{}{}
	}
	cfg := deepCopyMap(rawCfg)
	isOpenCode := kind == "opencode" || rawCfg["_opencode_raw"] != nil
	delete(cfg, "reasoning")
	delete(cfg, "limit")
	if !isOpenCode {
		delete(cfg, "modalities")
		delete(cfg, "headers")
		delete(cfg, "temperature")
		delete(cfg, "tool_call")
		delete(cfg, "attachment")
		delete(cfg, "options")
	}
	delete(cfg, "name")
	cfg["name"] = name
	if isOpenCode {
		// 中立能力位的反向映射（阶段 E1，见 MODEL-MANAGEMENT-PLAN.md 10.2.2/10.2.3）：
		// opencode 是唯一有真实键落点的 kind，这里把 card.Capabilities 映射回它的键名。
		//
		// attachment 保持既有语义：该键恒以布尔落盘（既有往返契约要求它始终存在），
		// 能力层只在明确说「支持」时把值抬成 true，绝不把已知为真的位降回 false。
		attachment := card.Attachment
		if card.Capabilities != nil && card.Capabilities.Attachment != nil && *card.Capabilities.Attachment {
			attachment = true
		}
		cfg["attachment"] = attachment
		// modalities 取并集：card.Modalities 里带的原始值（可能来自 v2 形状或用户手写的
		// opencode 配置）一个都不能丢、顺序不变，能力层的已知支持项追加去重。两边都空时
		// 整键不写（与既有「Modalities 为 nil 就不写」的行为一致）。
		if modalities := cardMergeOpenCodeModalities(card.Modalities, card.Capabilities); modalities != nil {
			cfg["modalities"] = modalities
		}
		if card.Headers != nil {
			h := map[string]interface{}{}
			for k, v := range card.Headers {
				h[k] = v
			}
			cfg["headers"] = h
		}
		if card.Options != nil {
			cfg["options"] = deepCopyMap(card.Options)
		}
		// tool_call / temperature 此前完全靠 rawCfg 深拷贝原样透传；现在能力层是唯一数据源：
		// 已知为真写 true，显式 false 删除该键，未知不动（详见 cardApplyOpenCodeFeatureBool）。
		if card.Capabilities != nil {
			cardApplyOpenCodeFeatureBool(cfg, "tool_call", card.Capabilities.ToolCall)
			cardApplyOpenCodeFeatureBool(cfg, "temperature", card.Capabilities.Temperature)
		}
	}
	// 内部通道 `_apm_capabilities`（冻结契约 10.2.4②）：**所有 kind** 都写进 legacy 中间形态，
	// 供 ZCode v2 / DSH 的写入端读取（opencode 另有上面的真实键）。内容是 card.Capabilities 的
	// 编码：只含「已知为真」的布尔位、模态按规范序，整体为空时连键都不写。
	// 先删除 rawCfg 深拷贝可能带进来的陈旧通道，避免用户清空能力后旧值还残留在中间形态里
	// （它虽会被 stripAPMInternalModelKeys 剥离，但残留会让 FromLegacy 读到过期数据）。
	delete(cfg, "_apm_capabilities")
	if internal := cardCapabilitiesToInternal(card.Capabilities); internal != nil {
		cfg["_apm_capabilities"] = internal
	}

	// 推理块（S1 / A-2 / A-7 / A-8）：只要有可表达的档位（card.Reasoning 为真，或集合非空）
	// 就写 reasoning 块。
	//
	// S1 语义（关键）：ZCode 没有独立的思考开关字段，它的「关闭思考」= 档位集合里选中 off 词
	// （modelApiRules 的 map 表达式 reasoningLevel == "disabled" ? {thinking:{disabled}} : {enabled}
	// 即证明了这一点）。因此当 card.Reasoning=false 且原集合非空时，必须把 variants 收敛为
	// 单元素 ["off"]——既不能保留完整集合（读回会被 CfgToCard 由非 off 档反推成 Reasoning=true，
	// 开关翻回开启，这正是本次改动引入的回退），也不是清空（清空会让 mergeV2ModelRuleConfig
	// 把 prev 的 values 原样带回，同样翻回开启）。收敛成 ["off"] 后，写回经
	// legacyReasoningVariants 兜底成 ["disabled"]，读回 values=["disabled"] → Reasoning=false，自洽。
	variants := NormalizeVariants(card.Variants)
	if !card.Reasoning && len(variants) > 0 {
		// 关闭思考且原本有档位集合：收敛为单元素 off（原拼法由 OffAlias 保真）。
		variants = []string{"off"}
	}
	if card.Reasoning && len(variants) == 0 {
		// 集合为空且声明开启思考：走默认集合兜底。
		variants = DefaultVariantsFor(kind)
	}
	if card.Reasoning || len(variants) > 0 {
		// 默认档位回落（修 A-2）：用户选择优先（规范化且落在集合内），否则按
		// DefaultVariantPreference 顺序挑，再否则首个非 off、首个元素。
		// 集合非空即可确定默认值——这里绝不返回 error。
		dv := canonicalVariant(card.DefaultVariant)
		if dv == "" || !containsStr(variants, dv) {
			dv = PickDefaultVariant(variants)
		}
		enabled := true
		if !card.Reasoning {
			// 关闭思考（A-7）：ZCode 里“关闭思考”= 档位集合里选中 off 词，
			// 而不是 config.enabled=false。这里的 enabled=false 只是 APM 内部语义
			// 标记，供 DSH 等格式表达“显式关闭”；它绝不能被当成 v2 modelRule 的
			// config.enabled（那是模型启用位，见 mergeV2ModelRuleConfig 的注释）。
			enabled = false
		} else if card.RawReasoningEnabled != nil {
			enabled = *card.RawReasoningEnabled
		} else if raw, ok := rawCfg["reasoning"].(map[string]interface{}); ok {
			if e, ok := raw["enabled"].(bool); ok && !e {
				enabled = false
			}
		}
		reasoning := map[string]interface{}{
			"enabled":        enabled,
			"variants":       variants,
			"defaultVariant": dv,
		}
		// off 词的原始拼法只在集合里确实含 off 时才回填：写回时必须以原拼法落盘，
		// 否则 ZCode 的 map 表达式匹配不到（修 A-1）。
		if containsStr(variants, "off") {
			if alias := normalizeOffAlias(card.OffAlias); alias != "" {
				reasoning["offAlias"] = alias
			}
		}
		cfg["reasoning"] = reasoning
	}

	// context / output independent
	hasCtx := card.Context != nil && card.Context != ""
	hasOut := card.Output != nil && card.Output != ""
	rawLimit := map[string]interface{}{}
	if rl, ok := rawCfg["limit"].(map[string]interface{}); ok {
		rawLimit = rl
	}
	ctxVal := ParseTokens(card.Context)
	outVal := ParseTokens(card.Output)
	if hasCtx && ctxVal == nil {
		if !equalJSON(card.Context, rawLimit["context"]) {
			return nil, "", fmt.Errorf("模型「%s」的上下文长度必须是有效正整数（最大 %d）", modelID, MaxTokenLimit)
		}
		if v, ok := rawLimit["context"]; ok {
			ctxVal = interfaceToIntPtr(v)
			if ctxVal == nil && v != nil && v != "" {
				// preserve raw value as-is if unparseable but unchanged
				hasCtx = true
			}
		} else {
			hasCtx = false
		}
	}
	if hasOut && outVal == nil {
		if !equalJSON(card.Output, rawLimit["output"]) {
			return nil, "", fmt.Errorf("模型「%s」的输出长度必须是有效正整数（最大 %d）", modelID, MaxTokenLimit)
		}
		if v, ok := rawLimit["output"]; ok {
			outVal = interfaceToIntPtr(v)
			if outVal == nil && v != nil && v != "" {
				hasOut = true
			}
		} else {
			hasOut = false
		}
	}
	if hasCtx || hasOut {
		limit := map[string]interface{}{}
		if ctxVal != nil {
			limit["context"] = *ctxVal
		} else if hasCtx {
			if v, ok := rawLimit["context"]; ok {
				limit["context"] = v
			}
		}
		if outVal != nil {
			limit["output"] = *outVal
		} else if hasOut {
			if v, ok := rawLimit["output"]; ok {
				limit["output"] = v
			}
		}
		if len(limit) > 0 {
			cfg["limit"] = limit
		}
	}
	return cfg, modelID, nil
}

func CardsToCfgMap(cards []ModelCard, kind string) (map[string]interface{}, error) {
	if len(cards) > MaxModels {
		return nil, fmt.Errorf("模型数量不能超过 %d 个", MaxModels)
	}
	models := map[string]interface{}{}
	for _, card := range cards {
		cfg, modelID, err := CardToCfg(card, kind)
		if err != nil {
			return nil, err
		}
		if _, exists := models[modelID]; exists {
			return nil, fmt.Errorf("存在重复的模型 ID：%s", modelID)
		}
		models[modelID] = cfg
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("请至少填写一个有效的模型")
	}
	return models, nil
}

func RawOptions(value interface{}) (map[string]interface{}, error) {
	if value == nil {
		return nil, nil
	}
	if m, ok := value.(map[string]interface{}); ok {
		if len(m) == 0 {
			return nil, nil
		}
		return m, nil
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return nil, nil
	}
	if len(text) > MaxOptionsLength {
		return nil, fmt.Errorf("options JSON 过长（上限 %dKB）", MaxOptionsLength/1024)
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("options JSON 解析失败：%v", err)
	}
	m, ok := parsed.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("options 必须是 JSON 对象 {...}")
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}

func containsStr(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}

// offAliasFromVariant 判断一个原始档位串是否是 off 别名，是则返回其小写去空格拼法；
// 否则返回空串。用于从配置原串里识别 off 词（disabled/none/nothink）。
func offAliasFromVariant(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "disabled", "none", "nothink":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return ""
	}
}

// firstOffAlias 从原始 variants 值里取第一个 off 词的原始拼法。
// 支持 string / []string / []interface{} 三种形态（与 NormalizeVariants 一致）。
func firstOffAlias(value interface{}) string {
	var items []string
	switch v := value.(type) {
	case string:
		items = []string{v}
	case []string:
		items = v
	case []interface{}:
		for _, e := range v {
			if s, ok := e.(string); ok {
				items = append(items, s)
			}
		}
	}
	for _, raw := range items {
		if alias := offAliasFromVariant(raw); alias != "" {
			return alias
		}
	}
	return ""
}

// normalizeOffAlias 校验并归一 off 别名，只接受 ZCode 认得的 off 词拼法。
// 空串或未知拼法返回空串，调用方据此决定不写 offAlias（走格式侧兜底）。
func normalizeOffAlias(value string) string {
	return offAliasFromVariant(value)
}

// stripAPMInternalModelKeys 剥离 APM 在“卡片 -> legacy 中间形态”往返时写入、但
// ZCode legacy config.json 不认的内部键。
//
// 修 M2：offAlias 是 APM 内部键（记录 off 词原拼法），只服务于 FromLegacy 的转换；
// legacy 写盘路径不经 FromLegacy，也就没有任何清理出口，若直接并入会把 offAlias 落到
// 用户配置里污染文档。v2 路径不经此函数（FromLegacy 只消费不落盘），无需修改。
//
// 只剥离本次新增的 offAlias 与带下划线前缀的内部键（_raw_* / _off_alias 等）；像
// reasoning.enabled / reasoning.defaultVariant 这类 legacy 形状字段属既有行为，保持不动，
// 不扩大改动面。
//
// 阶段 E1 的中立能力通道 `_apm_capabilities`（CardToCfg 在**所有 kind** 下写入）也走这条
// 统一剥离：它以 `_` 前缀命中下面的通用规则，因此**永远不会落盘**——它只在内存里充当
// 「卡片 → ZCode v2 / DSH 写入端」的通道（opencode 另有自己的真实键，不经此通道）。
//
// 支持两种输入形状（修 B1）：
//  1. 单个 provider 的配置 {models: {...}}（BuildProviderCfgEditor 的产物）；
//  2. 整份文档 {provider: {"<id>": {models: {...}}}}（MergeProviderIntoConfig 在
//     zcodeLegacyStore.Upsert 里的产物）。早期版本只处理形状 1，导致唯一的 legacy 生产
//     调用点（传整份文档）其实是空操作，offAlias 照样落盘。
func stripAPMInternalModelKeys(providerCfg map[string]interface{}) {
	if providerCfg == nil {
		return
	}
	stripInternalModelKeysInModels(providerCfg["models"])
	// 形状 2：整份文档，逐个 provider 处理其 models。
	if providers, ok := providerCfg["provider"].(map[string]interface{}); ok {
		for _, pv := range providers {
			if pcfg, ok := pv.(map[string]interface{}); ok {
				stripInternalModelKeysInModels(pcfg["models"])
			}
		}
	}
}

// stripInternalModelKeysInModels 对一个 models 表（modelID -> modelCfg）就地剥离内部键。
func stripInternalModelKeysInModels(modelsVal interface{}) {
	models, _ := modelsVal.(map[string]interface{})
	for _, value := range models {
		modelCfg, _ := value.(map[string]interface{})
		if modelCfg == nil {
			continue
		}
		for key := range modelCfg {
			// 带下划线前缀的内部 meta（_raw_cfg / _off_alias / _vendor / _supported_levels 等）
			// 均为 APM 内部字段，legacy 配置不认，一律剥离。
			if strings.HasPrefix(key, "_") {
				delete(modelCfg, key)
			}
		}
		reasoning, _ := modelCfg["reasoning"].(map[string]interface{})
		if reasoning == nil {
			continue
		}
		delete(reasoning, "offAlias")
	}
}

// cardCapabilitiesToInternal 把中立能力位编码成内部通道 `_apm_capabilities`
// （冻结契约见 MODEL-MANAGEMENT-PLAN.md 10.2.4②）。
//
// 只收录**已知为真**的布尔位与**已知支持**的模态：未知 / 假一律省略整个键。理由是中立层的
// 落盘策略「只写真、留空让目标 harness 自己的默认值生效」——记下 false 既没有落点（各
// harness 的键值域与默认值不同），又容易在展示上被误读成「已确认不支持」。
//
// 全部为空时返回 nil：调用方据此连 `_apm_capabilities` 这个键都不写。`Source` 不进通道
// （它只是展示用来源标记，绝不是能力数据）。
func cardCapabilitiesToInternal(caps *ModelCapabilities) map[string]interface{} {
	if caps == nil {
		return nil
	}
	out := map[string]interface{}{}
	if in := NormalizeInputModalities(caps.InputModalities); len(in) > 0 {
		out["input_modalities"] = cardStringsToAny(in)
	}
	if outMods := NormalizeOutputModalities(caps.OutputModalities); len(outMods) > 0 {
		out["output_modalities"] = cardStringsToAny(outMods)
	}
	for _, item := range []struct {
		key   string
		value *bool
	}{
		{"tool_call", caps.ToolCall},
		{"structured_output", caps.StructuredOutput},
		{"native_web_search", caps.NativeWebSearch},
		{"mid_conv_system", caps.MidConvSystem},
		{"attachment", caps.Attachment},
		{"temperature", caps.Temperature},
	} {
		if item.value != nil && *item.value {
			out[item.key] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// cardStringsToAny 把 []string 包成 []interface{}。内部通道里的模态数组用这种表示，与
// legacy 中间形态里既有数组（例如 v2 读入侧写的 modalities.input）保持一致，读端无论按
// []string 还是 []interface{} 断言都能拿到值（JSON 形态完全相同）。
func cardStringsToAny(items []string) []interface{} {
	out := make([]interface{}, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

// cardMergeOpenCodeModalities 把中立层的模态并进配置里既有的 modalities（opencode 真实键）。
//
// 既有值来自 RawCfg（可能是 v2 形状、用户手写的 opencode 配置或上一轮中间形态），它们
// **一个都不能丢、顺序不变**——opencode 往返契约依赖这一点；能力层的已知支持项追加去重
// （大小写不敏感）。
//
// existing == nil 且能力层没有模态时返回 nil：调用方据此不写该键，与既有「card.Modalities
// 为 nil 就不写 modalities」的行为逐字节一致（existing 非 nil 时即便为空对象也照写）。
func cardMergeOpenCodeModalities(existing map[string]interface{}, caps *ModelCapabilities) map[string]interface{} {
	var out map[string]interface{}
	if existing != nil {
		out = deepCopyMap(existing)
	}
	if caps != nil {
		// 先规范再并：模态词表外的未知词一律丢弃（opencode 的 enum 只认 5 个词，
		// 写进去会让整份 opencode 配置校验失败），规范序只作用于新增项，既有顺序不动。
		out = cardAppendModalities(out, "input", NormalizeInputModalities(caps.InputModalities))
		out = cardAppendModalities(out, "output", NormalizeOutputModalities(caps.OutputModalities))
	}
	if out == nil {
		return nil
	}
	return out
}

// cardAppendModalities 把 add 里的模态并入 out[key]（字符串数组）：保留既有元素与顺序，
// 追加尚未出现的项。out 为 nil 且确有新增时才新建 map；没有任何新增时原样返回（包括 nil）。
func cardAppendModalities(out map[string]interface{}, key string, add []string) map[string]interface{} {
	if len(add) == 0 {
		return out
	}
	if out == nil {
		out = map[string]interface{}{}
	}
	existing := metaStringList(out[key])
	merged := make([]interface{}, 0, len(existing)+len(add))
	seen := make(map[string]bool, len(existing)+len(add))
	for _, raw := range existing {
		word := strings.ToLower(strings.TrimSpace(raw))
		if seen[word] {
			continue
		}
		seen[word] = true
		merged = append(merged, raw)
	}
	for _, word := range add {
		if seen[word] {
			continue
		}
		seen[word] = true
		merged = append(merged, word)
	}
	out[key] = merged
	return out
}

// cardApplyOpenCodeFeatureBool 把一个中立层布尔位写进 opencode 的真实键（只写真、不写 false）：
//   - 已知为真 → 写 true；
//   - 显式 false（用户在能力区取消勾选）→ 删除该键：既避免写 false，又能清掉 RawCfg 深拷贝
//     里可能残留的陈旧 true，留空让 opencode 自己的默认值生效；
//   - 未知（nil）→ 不动，沿用 RawCfg 里已有的值（保持既有往返行为：rawCfg 里的值本来就会
//     被深拷贝透传）。
func cardApplyOpenCodeFeatureBool(cfg map[string]interface{}, key string, value *bool) {
	switch {
	case value != nil && *value:
		cfg[key] = true
	case value != nil:
		delete(cfg, key)
	}
}

// cardFeaturesFromInternal 读回内部通道 `_apm_capabilities`（键名冻结在 10.2.4②）。
//
// 该通道由 CardToCfg 写出，只含已知为真的位；这里按同一口径读回：**非布尔真一律视为未知**
// （false、类型不符、键缺失都返回 nil），保持三态语义。模态数组同样只取已知词表内的项。
func cardFeaturesFromInternal(value interface{}) ModelFeatures {
	raw, _ := value.(map[string]interface{})
	if raw == nil {
		return ModelFeatures{}
	}
	return ModelFeatures{
		InputModalities:  NormalizeInputModalities(metaStringList(raw["input_modalities"])),
		OutputModalities: NormalizeOutputModalities(metaStringList(raw["output_modalities"])),
		ToolCall:         metaFeatureTrue(raw["tool_call"]),
		StructuredOutput: metaFeatureTrue(raw["structured_output"]),
		NativeWebSearch:  metaFeatureTrue(raw["native_web_search"]),
		MidConvSystem:    metaFeatureTrue(raw["mid_conv_system"]),
		Attachment:       metaFeatureTrue(raw["attachment"]),
		Temperature:      metaFeatureTrue(raw["temperature"]),
	}
}

// cardFeaturesFromOpenCodeKeys 读回 opencode 形态的真实键（opencode_schema.json 的
// modalities{input,output} / attachment / tool_call / temperature）。用户配置里的取值就是
// **事实**，与 provider 自报同口径：只记真值，false / 未知不记（未知 = nil）。
func cardFeaturesFromOpenCodeKeys(modelCfg map[string]interface{}) ModelFeatures {
	var features ModelFeatures
	if modalities, ok := modelCfg["modalities"].(map[string]interface{}); ok {
		features.InputModalities = NormalizeInputModalities(metaStringList(modalities["input"]))
		features.OutputModalities = NormalizeOutputModalities(metaStringList(modalities["output"]))
	}
	features.Attachment = metaFeatureTrue(modelCfg["attachment"])
	features.ToolCall = metaFeatureTrue(modelCfg["tool_call"])
	features.Temperature = metaFeatureTrue(modelCfg["temperature"])
	return features
}

// cardFeaturesFromV2Properties 读回 ZCode v2 形状的 properties.* 能力键（键表见
// MODEL-MANAGEMENT-PLAN.md 10.1.2）：inputFormat.supports{Text,Image,Video,Audio,Pdf}、
// outputFormat.supportsText、supportsToolCall、supportsJsonSchemaOutput、
// supportsNativeWebSearch、supportsMidConversationSystem。
//
// 这些键出现在中间形态里时就是文档里写明的事实，但 false 仍按未知处理：中立层没有「不支持」
// 的落点，把 v2 的默认值当成明确结论带去别的 harness 只会制造错误信息。不做
// requiresMfjsToolSchema / optionSpecs.*.map（本轮明确不做）。
//
// 输入模态走 zcodeprovider.InputFormatModalities 这一条**唯一**读路径：ZCode 原生的
// inputFormat.supportsText 是「缺键即默认支持」（ZCode 的最小增量写入会删掉与继承值相等的
// 键），只有显式 false 才排除 text；其余四位与 outputFormat 保持严格只认 true。
// 内层通道 `_apm_capabilities` **不适用**这条：它读的是本函数上方 cardFeaturesFromInternal
// 的三态契约（缺键 = 未知），两条路径互不污染。
func cardFeaturesFromV2Properties(value interface{}) ModelFeatures {
	properties, _ := value.(map[string]interface{})
	if properties == nil {
		return ModelFeatures{}
	}
	var features ModelFeatures
	if inFormat, ok := properties["inputFormat"].(map[string]interface{}); ok {
		features.InputModalities = NormalizeInputModalities(zcodeprovider.InputFormatModalities(inFormat))
	}
	if outFormat, ok := properties["outputFormat"].(map[string]interface{}); ok {
		if v, known := metaBool(outFormat["supportsText"]); known && v {
			features.OutputModalities = NormalizeOutputModalities([]string{"text"})
		}
	}
	features.ToolCall = metaFeatureTrue(properties["supportsToolCall"])
	features.StructuredOutput = metaFeatureTrue(properties["supportsJsonSchemaOutput"])
	features.NativeWebSearch = metaFeatureTrue(properties["supportsNativeWebSearch"])
	features.MidConvSystem = metaFeatureTrue(properties["supportsMidConversationSystem"])
	return features
}

// metaFeatureTrue 只认「显式布尔真」；false、缺键、类型不符一律返回 nil（未知 = 不落盘）。
// 与 metaFeatureFlag 的区别：用于**已经确定键名**的读取场景（内部通道 / opencode / v2 键），
// 不做多键「任一为真」的合并。
func metaFeatureTrue(value interface{}) *bool {
	if v, known := metaBool(value); known && v {
		return featureBool(true)
	}
	return nil
}

// PickDefaultVariant 按固定规则为一个非空档位集合挑默认档位（修 A-2）。
//
// 顺序：
//  1. 按 DefaultVariantPreference 顺序挑集合里第一个匹配项（high 优先）；
//  2. 否则取集合首个非 off 档位；
//  3. 再否则取集合首个元素。
//
// 集合为空时返回 DefaultVariantFor("")（即 "high"，不含 kind 维度）。绝不返回 error——
// 只要集合非空就能确定一个默认值。
func PickDefaultVariant(variants []string) string {
	if len(variants) == 0 {
		return DefaultVariantFor("")
	}
	for _, pref := range DefaultVariantPreference {
		if containsStr(variants, pref) {
			return pref
		}
	}
	for _, v := range variants {
		if v != "off" {
			return v
		}
	}
	return variants[0]
}

func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	b, _ := json.Marshal(m)
	var out map[string]interface{}
	_ = json.Unmarshal(b, &out)
	if out == nil {
		return map[string]interface{}{}
	}
	return out
}

func equalJSON(a, b interface{}) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func interfaceToIntPtr(v interface{}) *int {
	return ParseTokens(v)
}
