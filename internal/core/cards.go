package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildModelCards turns raw model pairs into frontend cards. When autoFill is
// true, empty context/output fall back to the common-model preset table
// (provider-returned values always win).
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
		// Provider-returned values win; fall back to the common-model preset
		// only when the provider did not report a value.
		if autoFill {
			if p, ok := LookupModelLimits(m.ID); ok {
				if emptyToken(ctx) && p.Context > 0 {
					ctx = p.Context
				}
				if emptyToken(out) && p.Output > 0 {
					out = p.Output
				}
			}
		}
		cards = append(cards, ModelCard{
			ModelID:     m.ID,
			APIReturned: len(meta) > 0,
			Name:        m.ID,
			Reasoning:   GuessReasoning(m.ID),
			Variants:    nil,
			Context:     ctx,
			Output:      out,
			Select:      true,
		})
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
				key = canonicalVariant(key)
				if ZCodeVariantSet[key] && !containsStr(variants, key) {
					variants = append(variants, key)
				}
			}
		}
	}
	if len(variants) == 0 {
		if efforts, ok := modelCfg["reasoningEfforts"].(map[string]interface{}); ok {
			for key, value := range efforts {
				key = canonicalVariant(key)
				if key == "off" || value != nil {
					if ZCodeVariantSet[key] && !containsStr(variants, key) {
						variants = append(variants, key)
					}
				}
			}
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
	card := ModelCard{
		ModelID: modelID, APIReturned: false, Name: name,
		Reasoning: reasoningEnabled, RawReasoningEnabled: rawEnabled,
		Variants: variants, DefaultVariant: defaultVariant, Context: limit["context"],
		Output: limit["output"], Select: false, RawCfg: deepCopyMap(modelCfg),
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

	if card.Reasoning {
		variants := NormalizeVariants(card.Variants)
		if len(variants) == 0 {
			variants = DefaultVariantsFor(kind)
		}
		dv := strings.TrimSpace(strings.ToLower(card.DefaultVariant))
		if dv == "" {
			dv = DefaultVariantFor(kind)
		}
		if !containsStr(variants, dv) {
			return nil, "", fmt.Errorf("模型「%s」的默认推理档位「%s」不在已选档位中", modelID, dv)
		}
		enabled := true
		if card.RawReasoningEnabled != nil {
			enabled = *card.RawReasoningEnabled
		} else if raw, ok := rawCfg["reasoning"].(map[string]interface{}); ok {
			if e, ok := raw["enabled"].(bool); ok && !e {
				enabled = false
			}
		}
		cfg["reasoning"] = map[string]interface{}{
			"enabled":        enabled,
			"variants":       variants,
			"defaultVariant": dv,
		}
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
