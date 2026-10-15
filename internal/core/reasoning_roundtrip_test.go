package core

import (
	"reflect"
	"strings"
	"testing"

	"agentprovidermanager/internal/zcodeprovider"
)

// 本文件覆盖阶段 A 的 ZCode 档位往返修复：A-1（off 别名保真）、A-2（默认档位回落不再报错）、
// A-7（关闭思考不丢 reasoning 块）、A-8（单元素 off 集合不丢档位）。
// 全部表驱动 + t.TempDir() 沙箱，只用占位值，不读取任何真实用户配置。

// ifaceList 把字符串切片转成 JSON 解码后常见的 []interface{} 形态。
func ifaceList(ss []string) []interface{} {
	out := make([]interface{}, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}

// reasoningV2Doc 构造一份含单个 provider/模型的 v2 文档，模型带 reasoningLevel.values。
// enabled 为 nil 表示不写 config.enabled（模型启用位缺省）。
func reasoningV2Doc(t *testing.T, providerID, modelID string, values []string, enabled *bool) Doc {
	t.Helper()
	cfg := zcodeprovider.NewConfig()
	cfg.UpsertProviderRule(zcodeprovider.NewProviderRule(providerID, providerID, zcodeprovider.GroupStandardPersonal))
	mr := zcodeprovider.NewProviderModelRule(providerID, modelID)
	mcfg := zcodeprovider.RuleConfig(mr)
	if enabled != nil {
		mcfg["enabled"] = *enabled
	}
	mcfg["optionSpecs"] = map[string]interface{}{
		"reasoningLevel": map[string]interface{}{
			"values": ifaceList(values),
			"map":    `reasoningLevel == "disabled" ? {thinking:{type:"disabled"}} : {thinking:{type:"enabled"}}`,
		},
	}
	cfg.UpsertProviderModelRule(mr)
	cfg.SetProviderOrder([]string{providerID})
	return cfg.Doc()
}

func boolPtr(v bool) *bool { return &v }

// v2CardOf 读取指定模型的卡片。
func v2CardOf(t *testing.T, doc Doc, providerID, modelID string) ModelCard {
	t.Helper()
	edit, err := zcodeV2Store{}.Get(doc, providerID)
	if err != nil {
		t.Fatalf("Get(%s)：%v", providerID, err)
	}
	for _, c := range edit.Cards {
		if c.ModelID == modelID {
			return c
		}
	}
	t.Fatalf("文档里没有模型 %q 的卡片：%+v", modelID, edit.Cards)
	return ModelCard{}
}

// payloadOfCard 用一张卡片构造保存载荷。
func payloadOfCard(providerID, kind string, card ModelCard) ProviderPayload {
	return ProviderPayload{
		"id":       providerID,
		"name":     providerID,
		"kind":     kind,
		"base_url": "https://p.example.invalid/v1",
		"api_key":  "sk-placeholder",
		"cards":    []ModelCard{card},
	}
}

// v2ModelValuesOf 从 v2 文档里取某模型的 reasoningLevel.values。
func v2ModelValuesOf(t *testing.T, doc Doc, providerID, modelID string) []string {
	t.Helper()
	cfg := zcodeprovider.NewConfigFromDoc(doc)
	mr := cfg.ProviderModelRule(providerID, modelID)
	if mr == nil {
		t.Fatalf("文档里没有模型规则 %s/%s", providerID, modelID)
	}
	return zcodeprovider.ModelReasoningLevelValues(zcodeprovider.ModelRuleConfig(mr))
}

// TestZCodeReasoningValuesRoundTripFidelity 覆盖 A-1/A-2/A-6/A-8：
// 读卡片 -> 保存 -> 再读回后，reasoningLevel.values 与 off 词原拼法必须保真，且绝不报错。
func TestZCodeReasoningValuesRoundTripFidelity(t *testing.T) {
	cases := []struct {
		name    string
		values  []string
		enabled *bool
	}{
		{"enabled 单元素", []string{"enabled"}, boolPtr(true)},
		{"disabled+enabled", []string{"disabled", "enabled"}, boolPtr(true)},
		{"none 开头的六档", []string{"none", "low", "medium", "high", "xhigh", "max"}, boolPtr(true)},
		{"无 high 无 off", []string{"low", "medium", "xhigh", "max"}, boolPtr(true)},
		{"单元素 max", []string{"max"}, boolPtr(true)},
		{"单元素 disabled", []string{"disabled"}, boolPtr(true)},
		{"minimal 独立档位", []string{"none", "minimal", "high"}, boolPtr(true)},
		// 模型启用位缺省时也应能往返：此时卡片 Reasoning 由集合推断。
		{"缺省 enabled 的单元素 disabled", []string{"disabled"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("LOCALAPPDATA", dir)
			doc := reasoningV2Doc(t, "p", "m", tc.values, tc.enabled)

			card := v2CardOf(t, doc, "p", "m")
			updated, err := zcodeV2Store{}.Upsert(doc, "p", payloadOfCard("p", "openai-compatible", card), SaveOptions{})
			if err != nil {
				t.Fatalf("Upsert 不应报错（A-2）：%v", err)
			}
			got := v2ModelValuesOf(t, updated, "p", "m")
			if !reflect.DeepEqual(got, tc.values) {
				t.Fatalf("values 往返不保真：\n得到 %v\n期望 %v", got, tc.values)
			}
		})
	}
}

// TestZCodeOffAliasNotRewritten 单列断言 A-1：off 词的原拼法（none / disabled）不被改写。
func TestZCodeOffAliasNotRewritten(t *testing.T) {
	for _, offWord := range []string{"none", "disabled", "nothink"} {
		t.Run(offWord, func(t *testing.T) {
			doc := reasoningV2Doc(t, "p", "m", []string{offWord, "low", "high"}, boolPtr(true))
			card := v2CardOf(t, doc, "p", "m")
			if card.OffAlias != offWord {
				t.Fatalf("卡片应记下 off 原拼法 %q，得到 %q", offWord, card.OffAlias)
			}
			updated, err := zcodeV2Store{}.Upsert(doc, "p", payloadOfCard("p", "openai-compatible", card), SaveOptions{})
			if err != nil {
				t.Fatalf("Upsert：%v", err)
			}
			got := v2ModelValuesOf(t, updated, "p", "m")
			if !containsStr(got, offWord) {
				t.Fatalf("off 词应保留原拼法 %q，得到 %v", offWord, got)
			}
			for _, wrong := range []string{"off", "disabled"} {
				if wrong == offWord {
					continue
				}
				if containsStr(got, wrong) {
					t.Fatalf("off 词被改写成了 %q：%v", wrong, got)
				}
			}
		})
	}
}

// TestZCodeMinimalNotFoldedToLow 覆盖 A-6 核心：minimal 是独立档位，绝不折成 low。
func TestZCodeMinimalNotFoldedToLow(t *testing.T) {
	doc := reasoningV2Doc(t, "p", "m", []string{"none", "minimal", "high"}, boolPtr(true))
	card := v2CardOf(t, doc, "p", "m")
	if !containsStr(card.Variants, "minimal") {
		t.Fatalf("卡片应含 minimal，得到 %v", card.Variants)
	}
	if containsStr(card.Variants, "low") {
		t.Fatalf("minimal 不得折成 low：%v", card.Variants)
	}
	updated, err := zcodeV2Store{}.Upsert(doc, "p", payloadOfCard("p", "openai-compatible", card), SaveOptions{})
	if err != nil {
		t.Fatalf("Upsert：%v", err)
	}
	got := v2ModelValuesOf(t, updated, "p", "m")
	if !containsStr(got, "minimal") {
		t.Fatalf("minimal 应往返保留：%v", got)
	}
	if containsStr(got, "low") {
		t.Fatalf("minimal 不得折成 low：%v", got)
	}
}

// TestCardToCfgDefaultVariantFallbackNoError 覆盖 A-2：默认档位缺失时不再报错。
func TestCardToCfgDefaultVariantFallbackNoError(t *testing.T) {
	cases := []struct {
		name     string
		variants []string
		wantDV   string
	}{
		{"无 high 无 off", []string{"low", "medium", "xhigh", "max"}, "max"},
		{"单元素 max", []string{"max"}, "max"},
		{"单元素 enabled", []string{"enabled"}, "enabled"},
		{"仅有 off", []string{"off"}, "off"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card := ModelCard{ModelID: "m", Name: "m", Reasoning: true, Variants: tc.variants}
			cfg, _, err := CardToCfg(card, "openai-compatible")
			if err != nil {
				t.Fatalf("CardToCfg 不应报错（A-2）：%v", err)
			}
			reasoning, _ := cfg["reasoning"].(map[string]interface{})
			if reasoning == nil {
				t.Fatalf("应写出 reasoning 块：%+v", cfg)
			}
			if dv, _ := reasoning["defaultVariant"].(string); dv != tc.wantDV {
				t.Fatalf("默认档位应为 %q，得到 %q", tc.wantDV, dv)
			}
		})
	}
}

// TestCardToCfgReasoningOffConvergesToOff 覆盖 S1：直接调用 CardToCfg 且 Reasoning=false +
// 非空 variants 时，variants 必须收敛为单元素 ["off"]（不是保留完整集合，也不是清空），
// 且不返回 error。收敛后的 off 原拼法由 OffAlias 保真。
func TestCardToCfgReasoningOffConvergesToOff(t *testing.T) {
	card := ModelCard{
		ModelID: "m", Name: "m", Reasoning: false,
		Variants: []string{"off", "high"}, OffAlias: "disabled",
	}
	cfg, _, err := CardToCfg(card, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg：%v", err)
	}
	reasoning, _ := cfg["reasoning"].(map[string]interface{})
	if reasoning == nil {
		t.Fatalf("关闭思考时 reasoning 块应保留（收敛为 off）：%+v", cfg)
	}
	variants, _ := reasoning["variants"].([]string)
	if !reflect.DeepEqual(variants, []string{"off"}) {
		t.Fatalf("关闭思考时 variants 应收敛为 [off]（S1）：%v", variants)
	}
	if reasoning["offAlias"] != "disabled" {
		t.Fatalf("关闭思考时 offAlias 应保留以便拼法保真：%v", reasoning["offAlias"])
	}
	if reasoning["enabled"] != false {
		t.Fatalf("enabled 应写 false 作为内部语义标记：%v", reasoning["enabled"])
	}

	// 关闭思考 + variants 为空：无从表达档位，按 A-8 语义不写 reasoning 块。
	empty := ModelCard{ModelID: "m", Name: "m", Reasoning: false}
	cfg2, _, err := CardToCfg(empty, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg（空档位）：%v", err)
	}
	if _, ok := cfg2["reasoning"]; ok {
		t.Fatalf("关闭思考且无档位时不应写 reasoning 块：%+v", cfg2)
	}
}

// TestZCodeCloseThinkingConvergesToOff 覆盖 S1 端到端：
// 关闭思考后 values 收敛为单元素 off 原拼法，且绝不把「关闭思考」写成
// v2 modelRule 的 config.enabled=false。
func TestZCodeCloseThinkingConvergesToOff(t *testing.T) {
	doc := reasoningV2Doc(t, "p", "m", []string{"disabled", "high"}, boolPtr(true))
	card := v2CardOf(t, doc, "p", "m")
	// 模拟 UI 关闭思考：Reasoning=false，同时明示 rawDisabled，集合收敛由 CardToCfg 完成。
	card.Reasoning = false
	card.RawReasoningEnabled = boolPtr(false)

	updated, err := zcodeV2Store{}.Upsert(doc, "p", payloadOfCard("p", "openai-compatible", card), SaveOptions{})
	if err != nil {
		t.Fatalf("Upsert：%v", err)
	}
	cfg := zcodeprovider.NewConfigFromDoc(updated)
	mcfg := zcodeprovider.ModelRuleConfig(cfg.ProviderModelRule("p", "m"))
	if enabled, ok := zcodeprovider.ModelEnabled(mcfg); !ok || !enabled {
		t.Fatalf("关闭思考绝不能把 config.enabled 写成 false（那是模型启用位）：ok=%v enabled=%v", ok, enabled)
	}
	got := zcodeprovider.ModelReasoningLevelValues(mcfg)
	if !reflect.DeepEqual(got, []string{"disabled"}) {
		t.Fatalf("关闭思考后 values 应收敛为单元素 off 原拼法（S1）：%v", got)
	}
	// 读回必须自洽：单元素 off → Reasoning=false。
	if reCard := v2CardOf(t, updated, "p", "m"); reCard.Reasoning {
		t.Fatalf("关闭思考写回后读回应仍为关闭：%+v", reCard)
	}
}

// TestMergeV2ModelRuleConfigKeepsLevelsOnEmptyIncoming 覆盖 A-8：
// incoming 完全没有 optionSpecs（关闭思考且无档位）时，prev 的整个 optionSpecs 必须带过来。
func TestMergeV2ModelRuleConfigKeepsLevelsOnEmptyIncoming(t *testing.T) {
	prev := map[string]interface{}{
		"optionSpecs": map[string]interface{}{
			"reasoningLevel": map[string]interface{}{
				"values": []interface{}{"disabled"},
				"map":    "custom-map",
			},
		},
	}
	incoming := map[string]interface{}{}
	mergeV2ModelRuleConfig(incoming, prev)
	specs := zcodeprovider.ModelOptionSpecs(incoming)
	rl, _ := specs["reasoningLevel"].(map[string]interface{})
	if rl == nil {
		t.Fatalf("incoming 无 optionSpecs 时应整体保留 prev 的 optionSpecs：%+v", incoming)
	}
	if got := zcodeprovider.ModelReasoningLevelValues(incoming); !reflect.DeepEqual(got, []string{"disabled"}) {
		t.Fatalf("单元素 off 档位不应丢失：%v", got)
	}
	if rl["map"] != "custom-map" {
		t.Fatalf("map 不应丢失：%v", rl["map"])
	}
}

// TestMergeV2ModelRuleConfigKeepsReasoningLevelWhenIncomingHasOtherSpec 覆盖 A-8 的另一分支：
// incoming 写了 optionSpecs 但没有 reasoningLevel 时，prev 的 reasoningLevel 整项带过来。
func TestMergeV2ModelRuleConfigKeepsReasoningLevelWhenIncomingHasOtherSpec(t *testing.T) {
	prev := map[string]interface{}{
		"optionSpecs": map[string]interface{}{
			"reasoningLevel": map[string]interface{}{
				"values": []interface{}{"disabled", "high"},
				"map":    "custom-map",
			},
		},
	}
	incoming := map[string]interface{}{
		"optionSpecs": map[string]interface{}{
			"maxOutputTokens": map[string]interface{}{"max": 4096},
		},
	}
	mergeV2ModelRuleConfig(incoming, prev)
	got := zcodeprovider.ModelReasoningLevelValues(incoming)
	if !reflect.DeepEqual(got, []string{"disabled", "high"}) {
		t.Fatalf("incoming 未写 reasoningLevel 时应保留 prev 的档位：%v", got)
	}
	rl, _ := zcodeprovider.ModelOptionSpecs(incoming)["reasoningLevel"].(map[string]interface{})
	if rl["map"] != "custom-map" {
		t.Fatalf("map 应保留：%v", rl["map"])
	}
	if _, ok := zcodeprovider.ModelOptionSpecs(incoming)["maxOutputTokens"]; !ok {
		t.Fatalf("incoming 自己写的 maxOutputTokens 不应丢：%+v", incoming)
	}
}

// TestCanonicalVariantNoMinimalFold 直接锁住 canonicalVariant 的新语义。
func TestCanonicalVariantNoMinimalFold(t *testing.T) {
	cases := map[string]string{
		"none":      "off",
		"disabled":  "off",
		"nothink":   "off",
		"NONE":      "off",
		"minimal":   "minimal",
		" MINIMAL ": "minimal",
		"low":       "low",
		"High":      "high",
		"enabled":   "enabled",
		"":          "",
	}
	for in, want := range cases {
		if got := canonicalVariant(in); got != want {
			t.Errorf("canonicalVariant(%q) = %q，want %q", in, got, want)
		}
	}
}

// TestUIVariantOptionsIncludeMinimal 锁住档位全集与默认集合的分工（契约第 4 节）。
func TestUIVariantOptionsIncludeMinimal(t *testing.T) {
	for kind, opts := range UIVariantOptionsByKind {
		if !containsStr(opts, "minimal") {
			t.Errorf("%s 的 UI 档位全集应含 minimal：%v", kind, opts)
		}
	}
	for kind, def := range DefaultVariantsByKind {
		if strings.Join(def, ",") != "off,high,max" {
			t.Errorf("%s 的默认勾选集合应保持 off,high,max：%v", kind, def)
		}
	}
	// minimal 与 off 别名都必须落在 ZCodeVariantSet 内（别名用于识别）。
	for _, v := range []string{"minimal", "off", "disabled", "none", "nothink", "enabled"} {
		if !ZCodeVariantSet[v] {
			t.Errorf("ZCodeVariantSet 应包含 %q：%v", v, ZCodeVariantOptions)
		}
	}
}
