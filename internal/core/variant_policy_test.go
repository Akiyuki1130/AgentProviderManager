package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// 本文件覆盖阶段 C 的「允许自定义推理档位」包级策略：
//   - NormalizeVariants 在开关打开时跳过白名单、仍归一别名与去重；
//   - CardToCfg 尊重该开关，但关闭思考（S1 语义）不受影响；
//   - dshModelToRaw 写端永远只认 DSH 固定 7 档（自填名不落盘），读端在开关打开时保留自填名；
//   - CfgToCard 补齐 SupportedLevels / Vendor，且绝不改动配置里已读到的 variants。
//
// 每个用例都用 t.Cleanup 把全局开关还原为默认 false，避免污染其它测试。

// setCustomVariantsForTest 设置开关并在用例结束时还原为默认 false。
func setCustomVariantsForTest(t *testing.T, on bool) {
	t.Helper()
	SetCustomVariantsAllowed(on)
	t.Cleanup(func() { SetCustomVariantsAllowed(false) })
}

func TestNormalizeVariantsCustomSwitch(t *testing.T) {
	cases := []struct {
		name   string
		allow  bool
		input  interface{}
		expect []string
	}{
		{
			name:   "关闭时白名单过滤且别名归一",
			allow:  false,
			input:  []string{"off", "very-high", "disabled"},
			expect: []string{"off"},
		},
		{
			name:   "打开时保留自定义档位",
			allow:  true,
			input:  []string{"off", "very-high", "disabled"},
			expect: []string{"off", "very-high"},
		},
		{
			name:   "打开时字符串别名仍归一",
			allow:  true,
			input:  "disabled",
			expect: []string{"off"},
		},
		{
			name:   "打开时空串丢弃、重复去重且顺序保持",
			allow:  true,
			input:  []string{"very-high", "", "very-high", "  ", "low", "LOW"},
			expect: []string{"very-high", "low"},
		},
		{
			name:   "打开时超长自定义档位被拒绝",
			allow:  true,
			input:  []string{"ok", strings.Repeat("x", MaxVariantLen+1)},
			expect: []string{"ok"},
		},
		{
			name:   "打开时恰好等于长度上限的自定义档位保留",
			allow:  true,
			input:  []string{strings.Repeat("y", MaxVariantLen)},
			expect: []string{strings.Repeat("y", MaxVariantLen)},
		},
		{
			name:   "打开时含控制字符的自定义档位被拒绝",
			allow:  true,
			input:  []string{"good", "bad\tname", "line\nbreak"},
			expect: []string{"good"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setCustomVariantsForTest(t, tc.allow)
			got := NormalizeVariants(tc.input)
			if !reflect.DeepEqual(got, tc.expect) {
				t.Fatalf("NormalizeVariants(%#v) allow=%v = %#v, 期望 %#v", tc.input, tc.allow, got, tc.expect)
			}
		})
	}
}

func TestCardToCfgCustomSwitch(t *testing.T) {
	card := ModelCard{
		ModelID: "m", Reasoning: true,
		Variants: []string{"off", "very-high"}, DefaultVariant: "very-high",
	}

	cases := []struct {
		name       string
		allow      bool
		expectVars []string
	}{
		{
			name:       "打开时自定义档位不被过滤",
			allow:      true,
			expectVars: []string{"off", "very-high"},
		},
		{
			name:       "关闭时自定义档位被滤掉",
			allow:      false,
			expectVars: []string{"off"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setCustomVariantsForTest(t, tc.allow)
			cfg, _, err := CardToCfg(card, "openai-compatible")
			if err != nil {
				t.Fatalf("CardToCfg: %v", err)
			}
			reasoning, _ := cfg["reasoning"].(map[string]interface{})
			if reasoning == nil {
				t.Fatalf("reasoning 块缺失：%#v", cfg)
			}
			variants, _ := reasoning["variants"].([]string)
			if !reflect.DeepEqual(variants, tc.expectVars) {
				t.Fatalf("reasoning.variants = %#v, 期望 %#v", variants, tc.expectVars)
			}
		})
	}
}

// TestCardToCfgReasoningOffUnaffectedBySwitch 是回归保护：关闭思考时 variants 必须
// 收敛为单元素 ["off"]（S1 语义），开关开与关都一样，放宽不得改变它。
func TestCardToCfgReasoningOffUnaffectedBySwitch(t *testing.T) {
	card := ModelCard{
		ModelID: "m", Reasoning: false,
		Variants: []string{"low", "high"},
	}
	for _, allow := range []bool{true, false} {
		setCustomVariantsForTest(t, allow)
		cfg, _, err := CardToCfg(card, "openai-compatible")
		if err != nil {
			t.Fatalf("CardToCfg allow=%v: %v", allow, err)
		}
		reasoning, _ := cfg["reasoning"].(map[string]interface{})
		if reasoning == nil {
			t.Fatalf("reasoning 块缺失 allow=%v：%#v", allow, cfg)
		}
		variants, _ := reasoning["variants"].([]string)
		if !reflect.DeepEqual(variants, []string{"off"}) {
			t.Fatalf("关闭思考 variants allow=%v = %#v, 期望 [off]", allow, variants)
		}
	}
}

func TestDshModelToRawCustomSwitch(t *testing.T) {
	t.Run("打开时自填档位也不落盘（schema 边界）", func(t *testing.T) {
		// 开关只放宽卡片侧与读端：DSH 的 reasoningEfforts 字典键被 schema 限死 7 档，
		// 写一个自填名（very-high）不是「这一档被忽略」，而是整份 llm-pi-ai 配置解析期
		// 抛错、所有模型一起不可用，因此写端宁可丢弃。本用例集合 {off, very-high}
		// 过滤后只剩 off，按 #28 的裁定连 reasoningEfforts 键都不写（留未知）。
		setCustomVariantsForTest(t, true)
		node := dshModelToRaw(ModelCard{
			ModelID: "demo", Name: "demo", Reasoning: true,
			Variants: []string{"off", "very-high"}, DefaultVariant: "very-high",
			RawCfg: map[string]interface{}{},
		}, "", dshTestAPI)
		if got, exists := node["reasoningEfforts"]; exists {
			t.Fatalf("自填档位不得落盘，reasoningEfforts 应缺省：%#v", got)
		}
		if _, leftover := node["reasoningEffort"]; leftover {
			t.Fatalf("自填默认档位不得残留 reasoningEffort：%#v", node["reasoningEffort"])
		}
		b, err := json.Marshal(node)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(b), "very-high") {
			t.Fatalf("写端产物不得出现自填档位名：%s", b)
		}
	})

	t.Run("关闭时自定义档位被丢弃", func(t *testing.T) {
		setCustomVariantsForTest(t, false)
		node := dshModelToRaw(ModelCard{
			ModelID: "demo", Name: "demo", Reasoning: true,
			Variants: []string{"off", "very-high"}, DefaultVariant: "very-high",
			RawCfg: map[string]interface{}{},
		}, "", dshTestAPI)
		efforts, _ := node["reasoningEfforts"].(map[string]interface{})
		if _, exists := efforts["very-high"]; exists {
			t.Fatalf("关闭时不应保留自定义档位：%#v", efforts)
		}
	})
}

// TestDshWriteKeepsSevenEffortsDropsCustom 锁住 #30② 的写端 schema 边界：DSH 写端
// 永远只写固定 7 档，自填档位名（very-high）不落盘，七档内的档位原样落盘。这不是读/写
// 不对称的疏漏，而是刻意的取舍：写入自填名会让整份 llm-pi-ai 配置解析失败。
func TestDshWriteKeepsSevenEffortsDropsCustom(t *testing.T) {
	setCustomVariantsForTest(t, true)

	node := dshModelToRaw(ModelCard{
		ModelID: "demo", Name: "demo", Reasoning: true,
		Variants: []string{"off", "medium", "very-high", "high"}, DefaultVariant: "medium",
		RawCfg: map[string]interface{}{},
	}, "", dshTestAPI)
	efforts, ok := node["reasoningEfforts"].(map[string]interface{})
	if !ok {
		t.Fatalf("reasoningEfforts 不是字典：%#v", node["reasoningEfforts"])
	}
	if _, exists := efforts["very-high"]; exists {
		t.Fatalf("自填档位 very-high 不得落盘：%#v", efforts)
	}
	for _, want := range []string{"off", "medium", "high"} {
		if _, exists := efforts[want]; !exists {
			t.Fatalf("七档内的 %q 应落盘：%#v", want, efforts)
		}
	}
	if len(efforts) != 3 {
		t.Fatalf("只应落 3 个七档内档位，实际 %#v", efforts)
	}
	if v, exists := efforts["off"]; !exists || v != nil {
		t.Fatalf("off 档应写 null：%#v", efforts)
	}
	if node["reasoningEffort"] != "medium" {
		t.Fatalf("默认档应为 medium，实际 %#v", node["reasoningEffort"])
	}
}

// TestDshCustomEffortReadPreservedWhenOn 锁住读端放宽（与写端刻意不对称）：开关打开时，
// 配置里已有的自填档位名读进卡片必须保留；写端不落盘只是 DSH 的 schema 边界，读端若也
// 丢，用户手改配置在编辑器里就会静默消失。
func TestDshCustomEffortReadPreservedWhenOn(t *testing.T) {
	setCustomVariantsForTest(t, true)

	intermediate := ConvertDeepSeekModel("demo", map[string]interface{}{
		"reasoningEfforts": map[string]interface{}{
			"off":       nil,
			"high":      "high",
			"very-high": "very-high",
		},
	}, "openai-compatible")
	card := CfgToCard("demo", intermediate)
	if !containsStr(card.Variants, "very-high") {
		t.Fatalf("读端应保留自填档位 very-high，实际 %#v", card.Variants)
	}
	if !containsStr(card.Variants, "high") {
		t.Fatalf("内置档位 high 不应丢失，实际 %#v", card.Variants)
	}
}

// TestDshCustomEffortReadDroppedWhenOff 对称行为的反向：开关关闭时读端仍按白名单丢弃
// 未知档位（保证默认行为不变）。
func TestDshCustomEffortReadDroppedWhenOff(t *testing.T) {
	setCustomVariantsForTest(t, false)
	node := map[string]interface{}{
		"reasoningEfforts": map[string]interface{}{
			"off":       nil,
			"high":      "high",
			"very-high": "very-high",
		},
	}
	intermediate := ConvertDeepSeekModel("demo", node, "openai-compatible")
	card := CfgToCard("demo", intermediate)
	if containsStr(card.Variants, "very-high") {
		t.Fatalf("开关关闭时读端不应保留自定义档位：%#v", card.Variants)
	}
	if !containsStr(card.Variants, "high") {
		t.Fatalf("内置档位 high 应保留：%#v", card.Variants)
	}
}

// TestZCodeCustomVariantRoundTripSymmetry 锁住 ZCode 侧读/写对称（阶段 C）：
// 写端（zcodeprovider）不做白名单过滤、读端经 CfgToCard→NormalizeVariants 尊重开关，
// 因此开关打开时自填的档位必须能保存→重读保真。
func TestZCodeCustomVariantRoundTripSymmetry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("USERPROFILE", dir)
	setCustomVariantsForTest(t, true)

	doc := reasoningV2Doc(t, "p", "m", []string{"disabled", "low", "high"}, boolPtr(true))
	card := v2CardOf(t, doc, "p", "m")
	// 模拟用户在高级开关下自填一个档位。
	card.Variants = []string{"off", "very-high"}
	card.DefaultVariant = "very-high"

	updated, err := zcodeV2Store{}.Upsert(doc, "p", payloadOfCard("p", "openai-compatible", card), SaveOptions{})
	if err != nil {
		t.Fatalf("自填档位保存不应报错：%v", err)
	}
	if got := v2ModelValuesOf(t, updated, "p", "m"); !containsStr(got, "very-high") {
		t.Fatalf("写端应保留自填档位 very-high，实际 %v", got)
	}
	// 再读回：自填档位必须还原（对称）。
	reCard := v2CardOf(t, updated, "p", "m")
	if !containsStr(reCard.Variants, "very-high") {
		t.Fatalf("读端应还原自填档位 very-high，实际 %v", reCard.Variants)
	}
}

// TestZCodeCustomVariantDroppedWhenOff 对称反向：开关关闭时自填档位被白名单滤掉。
func TestZCodeCustomVariantDroppedWhenOff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("USERPROFILE", dir)
	setCustomVariantsForTest(t, false)

	doc := reasoningV2Doc(t, "p", "m", []string{"disabled", "low", "high"}, boolPtr(true))
	card := v2CardOf(t, doc, "p", "m")
	card.Variants = []string{"off", "very-high"}
	card.DefaultVariant = "very-high"

	updated, err := zcodeV2Store{}.Upsert(doc, "p", payloadOfCard("p", "openai-compatible", card), SaveOptions{})
	if err != nil {
		t.Fatalf("保存不应报错：%v", err)
	}
	reCard := v2CardOf(t, updated, "p", "m")
	if containsStr(reCard.Variants, "very-high") {
		t.Fatalf("开关关闭时不应保留自填档位：%v", reCard.Variants)
	}
}

// TestCfgToCardSupportedLevelsAndVendor 验证编辑已有 provider 的读卡路径也补齐
// SupportedLevels / Vendor，且不改动配置里已读到的 variants。
func TestCfgToCardSupportedLevelsAndVendor(t *testing.T) {
	// 与策略无关，但显式固定为默认值，避免受同包其它用例残留影响。
	setCustomVariantsForTest(t, false)
	modelCfg := map[string]interface{}{
		"reasoning": map[string]interface{}{
			"enabled":  true,
			"variants": []interface{}{"disabled", "enabled"},
		},
	}
	card := CfgToCard("glm-5.3", modelCfg)

	wantLevels := []string{"low", "high", "max"}
	if !reflect.DeepEqual(card.SupportedLevels, wantLevels) {
		t.Fatalf("SupportedLevels = %#v, 期望 %#v", card.SupportedLevels, wantLevels)
	}
	if containsStr(card.SupportedLevels, "off") {
		t.Fatalf("SupportedLevels 不应含 off：%#v", card.SupportedLevels)
	}
	if card.Vendor != "zhipu" {
		t.Fatalf("Vendor = %q, 期望 zhipu", card.Vendor)
	}
	// 配置里原有的 variants 必须原样保留（此处 disabled 归一为 off + enabled）。
	if !reflect.DeepEqual(card.Variants, []string{"off", "enabled"}) {
		t.Fatalf("card.Variants = %#v, 期望 [off enabled]（不应被 SupportedLevels 覆盖）", card.Variants)
	}
}
