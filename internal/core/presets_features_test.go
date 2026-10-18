package core

import (
	"reflect"
	"testing"
)

// TestApplyModelPresetsFillsCapabilitiesWithoutOverwriting 锁定「自动匹配」按钮的能力位补全语义：
//
//   - ctx/out 仍是覆盖式写入（既有行为），filled 也只统计这两类字段；
//   - capabilities 走**只补空缺**：卡片上已有选择（含已知为假）一律保留，空缺由能力表补；
//   - 能力表对某模型无信息时，卡片的能力位保持原样（含 nil），不凭空生成空对象。
func TestApplyModelPresetsFillsCapabilitiesWithoutOverwriting(t *testing.T) {
	const known = "deepseek-v4-flash"
	tableCap := LookupModelCapabilityForSite(known, "", "")
	if !tableCap.Features.HasAny() {
		t.Fatalf("前提破坏：能力表对 %s 给不出任何能力位，本用例失去意义", known)
	}

	t.Run("已有位不被改写、空缺被补齐", func(t *testing.T) {
		chosen := featureBool(false) // 已知为假也是用户/配置给出的选择
		cards := []ModelCard{{ModelID: known, Capabilities: &ModelCapabilities{ToolCall: chosen}}}
		out, filled := ApplyModelPresetsForSite(cards, "", "")
		if out[0].Capabilities == nil {
			t.Fatal("能力表有信息时应当补出 capabilities")
		}
		if out[0].Capabilities.ToolCall != chosen {
			t.Errorf("已有选择被改写：%v", out[0].Capabilities.ToolCall)
		}
		for _, field := range []struct {
			name string
			got  *bool
			want *bool
		}{
			{"structured_output", out[0].Capabilities.StructuredOutput, tableCap.Features.StructuredOutput},
			{"native_web_search", out[0].Capabilities.NativeWebSearch, tableCap.Features.NativeWebSearch},
			{"mid_conv_system", out[0].Capabilities.MidConvSystem, tableCap.Features.MidConvSystem},
			{"attachment", out[0].Capabilities.Attachment, tableCap.Features.Attachment},
			{"temperature", out[0].Capabilities.Temperature, tableCap.Features.Temperature},
		} {
			if boolState(field.got) != boolState(field.want) {
				t.Errorf("%s：got %v，能力表给的是 %v", field.name, boolState(field.got), boolState(field.want))
			}
		}
		if wantIn, wantOut := tableCap.Features.InputModalities, tableCap.Features.OutputModalities; len(wantIn) > 0 || len(wantOut) > 0 {
			if len(out[0].Capabilities.InputModalities) != len(wantIn) || len(out[0].Capabilities.OutputModalities) != len(wantOut) {
				t.Errorf("模态未被补齐：input=%v output=%v，能力表给的是 input=%v output=%v",
					out[0].Capabilities.InputModalities, out[0].Capabilities.OutputModalities, wantIn, wantOut)
			}
		}
		wantFilled := 0
		if tableCap.Context > 0 {
			wantFilled++
		}
		if tableCap.Output > 0 {
			wantFilled++
		}
		if filled != wantFilled {
			t.Errorf("filled = %d，期望 %d（只统计 ctx/out）", filled, wantFilled)
		}
	})

	t.Run("能力表无信息时不产生空对象", func(t *testing.T) {
		const unknown = "local-model-with-no-catalog-entry"
		if LookupModelCapabilityForSite(unknown, "", "").Features.HasAny() {
			t.Fatalf("前提破坏：%s 竟然有 features", unknown)
		}
		cards := []ModelCard{{ModelID: unknown}}
		out, _ := ApplyModelPresetsForSite(cards, "", "")
		if out[0].Capabilities != nil {
			t.Errorf("无能力信息时不应生成 capabilities：%+v", out[0].Capabilities)
		}
	})
}

// TestApplyModelPresetsFillsMissingLevels 锁定「自动匹配」按钮的档位补全语义（2026-10-17）：
// 思考已开、档位集合还是空的卡片，用它的支持档位（`_supported_levels`，与界面展示同源）填上；
// 非空集合、显式关闭思考、以及没有任何档位信息的情形一律不动。档位不计入 filled。
func TestApplyModelPresetsFillsMissingLevels(t *testing.T) {
	const known = "glm-5.3" // 能力表档位 [low high max]，不含 off
	tableCap := LookupModelCapabilityForSite(known, "", "")
	if !reflect.DeepEqual(tableCap.ReasoningLevels, []string{"low", "high", "max"}) {
		t.Fatalf("前提破坏：%s 的能力表档位变了：%v", known, tableCap.ReasoningLevels)
	}
	want := []string{"low", "high", "max"}

	cases := []struct {
		name       string
		card       ModelCard
		want       []string
		wantFilled int // glm-5.3 的能力表同时给出 ctx/out；filled 只统计它们
	}{
		{"思考已开 + 卡片带支持档位：补齐", ModelCard{ModelID: known, Reasoning: true, SupportedLevels: tableCap.ReasoningLevels}, want, 2},
		{"思考已开 + 卡片没带支持档位：回落能力表", ModelCard{ModelID: known, Reasoning: true}, want, 2},
		{"思考未开：不动", ModelCard{ModelID: known, Reasoning: false, SupportedLevels: tableCap.ReasoningLevels}, nil, 2},
		{"已有档位集合：不删不改", ModelCard{ModelID: known, Reasoning: true, Variants: []string{"medium", "max"}}, []string{"medium", "max"}, 2},
		{"只有 off 的非空集合：不动", ModelCard{ModelID: known, Reasoning: true, Variants: []string{"off"}}, []string{"off"}, 2},
		{"无档位信息：不动", ModelCard{ModelID: "local-model-with-no-catalog-entry", Reasoning: true}, nil, 0},
	}
	for _, c := range cases {
		cards := []ModelCard{c.card}
		out, filled := ApplyModelPresetsForSite(cards, "", "")
		if len(c.want) == 0 {
			if out[0].Variants != nil {
				t.Errorf("%s：Variants=%v，期望 nil", c.name, out[0].Variants)
			}
		} else if !reflect.DeepEqual(out[0].Variants, c.want) {
			t.Errorf("%s：Variants=%v，期望 %v", c.name, out[0].Variants, c.want)
		}
		if filled != c.wantFilled {
			t.Errorf("%s：filled=%d，期望 %d（档位不计入 filled）", c.name, filled, c.wantFilled)
		}
	}

	// 补齐时同时挑一个落在集合内的默认档位，避免写出「默认档位不在可选集合里」的矛盾数据。
	cards := []ModelCard{{ModelID: known, Reasoning: true}}
	out, _ := ApplyModelPresetsForSite(cards, "", "")
	if out[0].DefaultVariant != "high" {
		t.Errorf("DefaultVariant=%q，期望 high", out[0].DefaultVariant)
	}
}

// boolState 把三态布尔压成可比较的字符串，便于在断言消息里区分 nil / false / true。
func boolState(v *bool) string {
	switch {
	case v == nil:
		return "nil"
	case *v:
		return "true"
	default:
		return "false"
	}
}
