package core

import (
	"reflect"
	"testing"
)

// 本文件锁住 TODO #27 的最小对称修法（裁定见 lead）：读侧白名单过滤不再与写侧分叉，
// 而是共用 tokens.go 的 variantAllowed 门控——
//   - 开关关闭（默认）：读侧仍按 ZCodeVariantSet 丢弃自填名，默认行为一字不变；
//   - 开关打开：读侧保留自填名（仅做归一 / 去重 / 限长 / 禁控制字符），于是
//     「保存 → 重读」不再丢掉整组档位，CfgToCard → CardToCfg → CfgToCard 稳定。
//
// 覆盖三条读卡路径：ZCode legacy variants 布局、ZCode/DSH 通用 reasoning 布局、
// DSH reasoningEfforts 布局，以及 opencode 的 ConvertOpencodeModel。
// 每个用例都用 setCustomVariantsForTest 还原全局开关，互不污染。

// TestCfgToCardReadRespectsCustomSwitch 表驱动覆盖 CfgToCard 的两处读侧门控
// （legacy variants map 与 reasoningEfforts map）以及 reasoning.variants 数组路径。
func TestCfgToCardReadRespectsCustomSwitch(t *testing.T) {
	cases := []struct {
		name     string
		modelCfg map[string]interface{}
	}{
		{
			name: "ZCode legacy variants 布局",
			modelCfg: map[string]interface{}{
				"reasoning": true,
				"variants": map[string]interface{}{
					"disabled":  map[string]interface{}{},
					"low":       map[string]interface{}{},
					"very-high": map[string]interface{}{},
				},
			},
		},
		{
			name: "DSH reasoningEfforts 布局",
			modelCfg: map[string]interface{}{
				"reasoningEfforts": map[string]interface{}{
					"off":       nil,
					"high":      "high",
					"very-high": "very-high",
				},
			},
		},
		{
			name: "reasoning.variants 数组布局",
			modelCfg: map[string]interface{}{
				"reasoning": map[string]interface{}{
					"enabled":  true,
					"variants": []interface{}{"off", "low", "very-high"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("开关关闭时读丢自填名", func(t *testing.T) {
				setCustomVariantsForTest(t, false)
				card := CfgToCard("demo", tc.modelCfg)
				if containsStr(card.Variants, "very-high") {
					t.Fatalf("开关关闭时读端不得保留自填档位，实际 %#v", card.Variants)
				}
				if len(card.Variants) == 0 {
					t.Fatalf("内置档位不应被一起丢掉：%#v", card.Variants)
				}
			})

			t.Run("开关打开时读留自填名且往返稳定", func(t *testing.T) {
				setCustomVariantsForTest(t, true)
				card := CfgToCard("demo", tc.modelCfg)
				if !containsStr(card.Variants, "very-high") {
					t.Fatalf("开关打开时读端应保留自填档位，实际 %#v", card.Variants)
				}

				cfg, _, err := CardToCfg(card, "openai-compatible")
				if err != nil {
					t.Fatalf("CardToCfg: %v", err)
				}
				back := CfgToCard("demo", cfg)
				if !reflect.DeepEqual(card.Variants, back.Variants) {
					t.Fatalf("往返后档位集合漂移：%#v -> %#v", card.Variants, back.Variants)
				}
				// 源配置没有 defaultVariant 时，CardToCfg 会补一个默认档位；只要它落在
				// 集合内即合法，且源里已声明的默认档位不能被改掉。
				if back.DefaultVariant != "" && !containsStr(back.Variants, back.DefaultVariant) {
					t.Fatalf("默认档位不在集合内：%q / %#v", back.DefaultVariant, back.Variants)
				}
				if card.DefaultVariant != "" && card.DefaultVariant != back.DefaultVariant {
					t.Fatalf("往返后默认档位漂移：%q -> %q", card.DefaultVariant, back.DefaultVariant)
				}

				// 第二趟必须幂等：这是「自填档位不再在保存→重读中漂移」的直接证据。
				cfg2, _, err := CardToCfg(back, "openai-compatible")
				if err != nil {
					t.Fatalf("CardToCfg 第二趟: %v", err)
				}
				back2 := CfgToCard("demo", cfg2)
				if !reflect.DeepEqual(back.Variants, back2.Variants) || back.DefaultVariant != back2.DefaultVariant {
					t.Fatalf("第二趟不幂等：%#v/%q -> %#v/%q", back.Variants, back.DefaultVariant, back2.Variants, back2.DefaultVariant)
				}
			})
		})
	}
}

// TestOpencodeReadRespectsCustomSwitch 锁住 ConvertOpencodeModel 的门控：它原本硬按
// ZCodeVariantSet 丢弃未知档位，不受开关控制，是三条读卡路径里唯一的分叉点。
func TestOpencodeReadRespectsCustomSwitch(t *testing.T) {
	modelCfg := map[string]interface{}{
		"id":        "demo",
		"reasoning": true,
		"variants": map[string]interface{}{
			"none":      map[string]interface{}{},
			"low":       map[string]interface{}{},
			"very-high": map[string]interface{}{},
		},
	}

	setCustomVariantsForTest(t, false)
	card := CfgToCard("demo", ConvertOpencodeModel("demo", modelCfg, "openai-compatible"))
	if containsStr(card.Variants, "very-high") {
		t.Fatalf("开关关闭时 opencode 读端不得保留自填档位，实际 %#v", card.Variants)
	}
	if !containsStr(card.Variants, "off") || !containsStr(card.Variants, "low") {
		t.Fatalf("内置档位（含 none→off 归一）不应丢失：%#v", card.Variants)
	}

	setCustomVariantsForTest(t, true)
	card = CfgToCard("demo", ConvertOpencodeModel("demo", modelCfg, "openai-compatible"))
	if !containsStr(card.Variants, "very-high") {
		t.Fatalf("开关打开时 opencode 读端应保留自填档位，实际 %#v", card.Variants)
	}

	// 往返（opencode 的 variants map 迭代顺序随机，这里只断言集合不丢，不断言顺序）。
	cfg, _, err := CardToCfg(card, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg: %v", err)
	}
	back := CfgToCard("demo", cfg)
	if !containsStr(back.Variants, "very-high") {
		t.Fatalf("往返后自填档位丢失：%#v", back.Variants)
	}
}
