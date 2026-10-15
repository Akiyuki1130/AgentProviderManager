package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildModelCards turns raw model pairs into frontend cards. When autoFill is
// true, empty context/output fall back to the built-in capability table
// (provider-returned values always win).
//
// Vendor 与补全开关无关：厂商分类纯展示、不写盘，因此无论 autoFill 真假都填；
// SupportedLevels 只在 autoFill 为真时给出（它服务于阶段 C 的档位自适应）。
func BuildModelCards(models []struct {
	ID   string
	Meta map[string]interface{}
}, autoFill bool) []ModelCard {
	cards := make([]ModelCard, 0, len(models))
	for _, m := range models {
		meta := m.Meta
		if meta == nil {
			meta = map[string]interface{}{}
		}
		ctx := metaToken(meta, []string{"context_length", "max_context_length", "max_model_len", "context_window", "context"})
		out := metaToken(meta, []string{"max_completion_tokens", "max_output_tokens", "max_tokens", "output_length", "output"})
		// 能力表查一次，供补全、厂商分类与思考判定复用（避免每个模型各跑一遍正则三趟）。
		cap, _ := LookupModelCapability(m.ID)
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
		card := ModelCard{
			ModelID:     m.ID,
			APIReturned: len(meta) > 0,
			Name:        m.ID,
			Reasoning:   guessReasoningFromCapability(m.ID, cap),
			Variants:    nil,
			Context:     ctx,
			Output:      out,
			Select:      true,
			Vendor:      cap.Vendor,
		}
		if autoFill {
			card.SupportedLevels = cap.ReasoningLevels
		}
		cards = append(cards, card)
	}
	return cards
}

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

func CfgToCard(modelID string, modelCfg map[string]interface{}) ModelCard {
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
	cap, _ := LookupModelCapability(modelID)
	// 厂商判定不依赖任何开关，纯展示，始终填。
	card.Vendor = cap.Vendor
	// 只在能力表确实给出档位时填 SupportedLevels（cap.ReasoningLevels 已规范化 + 排序）；
	// 为空则保持 nil，前端按「未推断到」处理。绝不覆盖上面从配置读到的 variants。
	if len(cap.ReasoningLevels) > 0 {
		card.SupportedLevels = cap.ReasoningLevels
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
		cfg["attachment"] = card.Attachment
		if card.Modalities != nil {
			cfg["modalities"] = deepCopyMap(card.Modalities)
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
