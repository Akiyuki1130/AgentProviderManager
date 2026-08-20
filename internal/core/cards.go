package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildModelCards turns raw model pairs into frontend cards.
func BuildModelCards(models []struct{ ID string; Meta map[string]interface{} }) []ModelCard {
	cards := make([]ModelCard, 0, len(models))
	for _, m := range models {
		meta := m.Meta
		if meta == nil {
			meta = map[string]interface{}{}
		}
		ctx := metaToken(meta, []string{"context_length", "max_context_length", "max_model_len", "context_window", "context"})
		out := metaToken(meta, []string{"max_completion_tokens", "max_output_tokens", "max_tokens", "output_length", "output"})
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

func BuildSingleCard(modelID string) (ModelCard, error) {
	mid := strings.TrimSpace(modelID)
	if mid == "" {
		return ModelCard{}, fmt.Errorf("模型 ID 不能为空")
	}
	if len(mid) > MaxModelIDLength {
		return ModelCard{}, fmt.Errorf("模型 ID 不能超过 %d 个字符", MaxModelIDLength)
	}
	cards := BuildModelCards([]struct{ ID string; Meta map[string]interface{} }{{ID: mid, Meta: map[string]interface{}{}}})
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
	var variants []string
	var defaultVariant string
	var rawEnabled *bool
	if reasoning, ok := modelCfg["reasoning"].(map[string]interface{}); ok {
		if e, ok := reasoning["enabled"].(bool); ok {
			enabled = e
			b := e
			rawEnabled = &b
		}
		variants = NormalizeVariants(reasoning["variants"])
		if dv, ok := reasoning["defaultVariant"].(string); ok {
			dv = strings.TrimSpace(strings.ToLower(dv))
			if containsStr(variants, dv) {
				defaultVariant = dv
			}
		}
	}
	name := modelID
	if n, ok := modelCfg["name"].(string); ok && strings.TrimSpace(n) != "" {
		name = n
	}
	return ModelCard{
		ModelID:             modelID,
		APIReturned:         false,
		Name:                name,
		Reasoning:           enabled || len(variants) > 0,
		RawReasoningEnabled: rawEnabled,
		Variants:            variants,
		DefaultVariant:      defaultVariant,
		Context:             limit["context"],
		Output:              limit["output"],
		Select:              true,
		RawCfg:              deepCopyMap(modelCfg),
	}
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
	delete(cfg, "reasoning")
	delete(cfg, "limit")
	delete(cfg, "modalities")
	delete(cfg, "name")
	cfg["name"] = name

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
