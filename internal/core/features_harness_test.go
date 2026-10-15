package core

import (
	"reflect"
	"testing"

	"agentprovidermanager/internal/zcodeprovider"
)

// 本文件覆盖阶段 E1 的「中立能力位 → 三个 harness」映射与写盘约束
// （MODEL-MANAGEMENT-PLAN.md 第 10.2.3 / 10.2.4）：
//   - DSH：input 收敛到 {text,image}（含反例），compat 只补两个键且与既有 compat 合并；
//   - opencode：modalities{input,output} / tool_call / temperature / attachment 往返不丢；
//   - ZCode v2：5 个模态位与 4 个布尔位都能被读进内部能力通道，再落成 v2 的 properties。
// 全部使用占位值，不读取任何真实用户配置与凭据。

// TestDshInputModalitiesConvergeToDSHWordList 锁住 DSH 写端的收敛缺陷修复：
// DSH 的模型级 input 只接受 text/image，其余模态必须被收敛掉，交集为空则整键省略。
func TestDshInputModalitiesConvergeToDSHWordList(t *testing.T) {
	write := func(card ModelCard) map[string]interface{} { return dshModelToRaw(card, "openai-compatible") }

	t.Run("video/pdf/audio 被收敛掉", func(t *testing.T) {
		raw := write(ModelCard{ModelID: "demo", RawCfg: map[string]interface{}{
			"modalities": map[string]interface{}{"input": []interface{}{"video", "pdf", "audio", "text", "image"}},
		}})
		if got := apmModalityWords(raw["input"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Fatalf("input = %v, want [text image]（DSH 只接受 text/image）", got)
		}
	})

	t.Run("交集为空时整键省略", func(t *testing.T) {
		raw := write(ModelCard{ModelID: "demo", RawCfg: map[string]interface{}{
			"modalities": map[string]interface{}{"input": []interface{}{"video", "pdf", "audio"}},
		}})
		if got, ok := raw["input"]; ok {
			t.Fatalf("交集为空时必须整键省略（不留空数组），实际 %#v", got)
		}
	})

	t.Run("DSH 原生的非法 input 也会被清掉", func(t *testing.T) {
		raw := write(ModelCard{ModelID: "demo", RawCfg: map[string]interface{}{
			"input": []interface{}{"video"},
		}})
		if got, ok := raw["input"]; ok {
			t.Fatalf("text/image 之外的 input 必须被清掉，实际 %#v", got)
		}
	})

	t.Run("卡片能力位优先于原始 modalities", func(t *testing.T) {
		raw := write(ModelCard{
			ModelID:      "demo",
			Capabilities: &ModelCapabilities{InputModalities: []string{"image"}},
			RawCfg: map[string]interface{}{
				"modalities": map[string]interface{}{"input": []interface{}{"text", "image"}},
			},
		})
		if got := apmModalityWords(raw["input"]); !reflect.DeepEqual(got, []string{"image"}) {
			t.Fatalf("input = %v, want [image]（卡片能力位是唯一数据源）", got)
		}
	})

	t.Run("内部能力键与无法表达的能力位都不落盘", func(t *testing.T) {
		raw := write(ModelCard{
			ModelID: "demo",
			Capabilities: &ModelCapabilities{
				InputModalities: []string{"text"},
				ToolCall:        featureBool(true),
				NativeWebSearch: featureBool(true),
				MidConvSystem:   featureBool(true),
			},
			RawCfg: map[string]interface{}{apmCapabilitiesModelKey: map[string]interface{}{"tool_call": true}},
		})
		if got, ok := raw[apmCapabilitiesModelKey]; ok {
			t.Fatalf("_apm_capabilities 是 APM 内部键，不得写进 DSH 配置：%#v", got)
		}
		// 工具调用 / 原生联网搜索 / 对话中途 system 在 DSH 配置层无法表达：不落任何键。
		for _, key := range []string{"tool_call", "supportsNativeWebSearch", "mid_conv_system"} {
			if _, ok := raw[key]; ok {
				t.Errorf("DSH 不应写 %q", key)
			}
		}
	})
}

// TestDshCompatOnlyAddsTwoKnownCapabilityKeys 锁住 compat 的写入口径：
// 只补 supportsStrictMode / supportsTemperature 两个键（只写真），且与既有 compat 合并。
func TestDshCompatOnlyAddsTwoKnownCapabilityKeys(t *testing.T) {
	t.Run("只补两个键且与既有 compat 合并", func(t *testing.T) {
		raw := dshModelToRaw(ModelCard{
			ModelID: "demo",
			Capabilities: &ModelCapabilities{
				StructuredOutput: featureBool(true),
				Temperature:      featureBool(true),
			},
			RawCfg: map[string]interface{}{"compat": map[string]interface{}{
				"supportsStrictTools": true,
				"supportsToolSearch":  false,
			}},
		}, "openai-compatible")
		compat, ok := raw["compat"].(map[string]interface{})
		if !ok {
			t.Fatalf("compat 缺失：%#v", raw["compat"])
		}
		if compat["supportsStrictMode"] != true || compat["supportsTemperature"] != true {
			t.Fatalf("两个能力键都应为 true：%#v", compat)
		}
		if compat["supportsStrictTools"] != true || compat["supportsToolSearch"] != false {
			t.Fatalf("既有 compat 键与值必须原样保留（合并而非整块替换）：%#v", compat)
		}
		expected := []string{"supportsStrictMode", "supportsTemperature", "supportsStrictTools", "supportsToolSearch"}
		for key := range compat {
			if !containsStr(expected, key) {
				t.Errorf("出现了未预期的 compat 键 %q（catalog-only 键会被 DSH 拒绝）", key)
			}
		}
	})

	t.Run("未知/假一律不写 compat", func(t *testing.T) {
		raw := dshModelToRaw(ModelCard{
			ModelID: "demo",
			Capabilities: &ModelCapabilities{
				StructuredOutput: featureBool(false),
				NativeWebSearch:  featureBool(true),
			},
			RawCfg: map[string]interface{}{},
		}, "openai-compatible")
		if got, ok := raw["compat"]; ok {
			t.Fatalf("没有已知为真的 compat 能力时不应写 compat：%#v", got)
		}
	})
}

// TestConvertDeepSeekModelReadsCapabilities 锁住 DSH 读端：input 与 compat 里可配置的两个
// 能力键都要反向读进内部能力通道（只写真），卡片与写端才有唯一数据源。
func TestConvertDeepSeekModelReadsCapabilities(t *testing.T) {
	node := map[string]interface{}{
		"id":    "demo",
		"input": []interface{}{"text", "image"},
		"compat": map[string]interface{}{
			"supportsStrictMode":  true,
			"supportsTemperature": true,
			"supportsStrictTools": false,
		},
	}
	inter := ConvertDeepSeekModel("demo", node, "openai-compatible")
	caps := apmCapabilitiesChannel(inter)
	if caps == nil {
		t.Fatalf("DSH 读端应写入内部能力通道：%#v", inter)
	}
	if got := apmModalityWords(caps["input_modalities"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
		t.Errorf("input_modalities = %v, want [text image]", got)
	}
	if caps["structured_output"] != true || caps["temperature"] != true {
		t.Errorf("compat 能力位应读回 true：%#v", caps)
	}
	for _, key := range []string{"tool_call", "native_web_search", "mid_conv_system", "attachment"} {
		if _, ok := caps[key]; ok {
			t.Errorf("DSH 配置层无法表达 %s，读端不得凭空补：%#v", key, caps)
		}
	}
	if _, ok := caps["output_modalities"]; ok {
		t.Errorf("DSH 没有输出模态键，读端不得凭空补：%#v", caps)
	}
	for key, value := range caps {
		if value == nil {
			t.Errorf("能力通道不得写 null：%s", key)
		}
		if b, isBool := value.(bool); isBool && !b {
			t.Errorf("能力通道不得写 false：%s", key)
		}
	}

	// 写端：只读卡片的中立能力位也能落出 DSH 的 compat 键（不依赖原始键的透传）。
	writeCaps := dshModelToRaw(ModelCard{
		ModelID:      "demo",
		Capabilities: &ModelCapabilities{StructuredOutput: featureBool(true)},
		RawCfg:       map[string]interface{}{},
	}, "openai-compatible")
	writeCompat, _ := writeCaps["compat"].(map[string]interface{})
	if writeCompat["supportsStrictMode"] != true {
		t.Fatalf("卡片能力位应落成 compat.supportsStrictMode：%#v", writeCaps["compat"])
	}

	// 往返：DSH 节点 -> 内部形态 -> 卡片 -> DSH 节点，input 保真。
	round := dshModelToRaw(CfgToCard("demo", inter), "openai-compatible")
	if got := apmModalityWords(round["input"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
		t.Fatalf("往返后 input = %v, want [text image]", got)
	}
}

// TestOpencodeModelCapabilitiesRoundTrip 锁住 opencode 读写两端的能力键：
// modalities{input,output} / tool_call / temperature / attachment 都不丢，且补全只做加法。
func TestOpencodeModelCapabilitiesRoundTrip(t *testing.T) {
	t.Run("真实键往返不丢", func(t *testing.T) {
		internal := ConvertOpencodeModel("m1", map[string]interface{}{
			"id":          "m1",
			"tool_call":   true,
			"temperature": true,
			"attachment":  true,
			"modalities": map[string]interface{}{
				"input":  []interface{}{"text", "image"},
				"output": []interface{}{"text", "audio"},
			},
		}, "openai-compatible")

		caps := apmCapabilitiesChannel(internal)
		if caps == nil {
			t.Fatalf("opencode 读端应写入内部能力通道：%#v", internal)
		}
		if got := apmModalityWords(caps["input_modalities"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Errorf("input_modalities = %v, want [text image]", got)
		}
		if got := apmModalityWords(caps["output_modalities"]); !reflect.DeepEqual(got, []string{"text", "audio"}) {
			t.Errorf("output_modalities = %v, want [text audio]（output 不得丢）", got)
		}
		for _, key := range []string{"tool_call", "temperature", "attachment"} {
			if caps[key] != true {
				t.Errorf("能力位 %s 应读回 true：%#v", key, caps)
			}
		}

		out := OpenCodeModelFromCfg("m1", internal)
		if got, ok := out[apmCapabilitiesModelKey]; ok {
			t.Fatalf("_apm_capabilities 是 APM 内部键，不得写进 opencode 配置：%#v", got)
		}
		if out["tool_call"] != true || out["temperature"] != true || out["attachment"] != true {
			t.Errorf("opencode 能力键丢失：%#v", out)
		}
		modalities, _ := out["modalities"].(map[string]interface{})
		if got := apmModalityWords(modalities["input"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Errorf("modalities.input = %v, want [text image]", got)
		}
		if got := apmModalityWords(modalities["output"]); !reflect.DeepEqual(got, []string{"text", "audio"}) {
			t.Errorf("modalities.output = %v, want [text audio]", got)
		}
	})

	t.Run("卡片能力位补进 opencode 的真实键", func(t *testing.T) {
		out := OpenCodeModelFromCfg("m2", map[string]interface{}{
			"id": "m2",
			apmCapabilitiesModelKey: map[string]interface{}{
				"input_modalities": []interface{}{"text", "pdf"},
				"tool_call":        true,
				"temperature":      true,
				"attachment":       true,
			},
		})
		if _, ok := out[apmCapabilitiesModelKey]; ok {
			t.Fatal("内部能力键不得写进 opencode 配置")
		}
		if out["tool_call"] != true || out["temperature"] != true || out["attachment"] != true {
			t.Errorf("能力位应补成 opencode 的真实键：%#v", out)
		}
		modalities, _ := out["modalities"].(map[string]interface{})
		if got := apmModalityWords(modalities["input"]); !reflect.DeepEqual(got, []string{"text", "pdf"}) {
			t.Errorf("modalities.input = %v, want [text pdf]", got)
		}
	})

	t.Run("不覆盖既有显式值且绝不写 false", func(t *testing.T) {
		out := OpenCodeModelFromCfg("m3", map[string]interface{}{
			"id":        "m3",
			"tool_call": false,
			apmCapabilitiesModelKey: map[string]interface{}{
				"tool_call":   true,
				"temperature": false,
			},
		})
		if out["tool_call"] != false {
			t.Fatalf("既有的显式 false 不得被推断值覆盖：%#v", out["tool_call"])
		}
		if _, ok := out["temperature"]; ok {
			t.Fatalf("能力位为假时不得写键：%#v", out)
		}
	})
}

// TestV2ReadBackCarriesCapabilitiesAndLandsInProperties 锁住 ZCode v2 的读回与落盘：
// v2ModelCard 的中间形态带走 5 个模态位与 4 个布尔位，再由 zcodeprovider 落成 properties。
func TestV2ReadBackCarriesCapabilitiesAndLandsInProperties(t *testing.T) {
	full := map[string]interface{}{
		"properties": map[string]interface{}{
			"contextWindow": 200000,
			"inputFormat": map[string]interface{}{
				"supportsText": true, "supportsImage": true, "supportsVideo": true,
				"supportsAudio": true, "supportsPdf": true,
			},
			"outputFormat":                  map[string]interface{}{"supportsText": true},
			"supportsToolCall":              true,
			"supportsJsonSchemaOutput":      true,
			"supportsNativeWebSearch":       true,
			"supportsMidConversationSystem": true,
		},
	}
	inter := v2LegacyModelCfg(full)
	caps := apmCapabilitiesChannel(inter)
	if caps == nil {
		t.Fatalf("v2 读回必须带走内部能力通道：%#v", inter)
	}
	if got := apmModalityWords(caps["input_modalities"]); !reflect.DeepEqual(got, []string{"text", "image", "video", "audio", "pdf"}) {
		t.Errorf("input_modalities = %v, want 5 个模态位", got)
	}
	if got := apmModalityWords(caps["output_modalities"]); !reflect.DeepEqual(got, []string{"text"}) {
		t.Errorf("output_modalities = %v, want [text]", got)
	}
	for _, key := range []string{"tool_call", "structured_output", "native_web_search", "mid_conv_system"} {
		if caps[key] != true {
			t.Errorf("能力位 %s 应读回 true：%#v", key, caps)
		}
	}
	for _, key := range []string{"attachment", "temperature"} {
		if _, ok := caps[key]; ok {
			t.Errorf("v2 没有 %s 键，读端不得凭空补：%#v", key, caps)
		}
	}
	// 既有行为不动：中间形态的 modalities 仍只承载 image/video。
	modalities, _ := inter["modalities"].(map[string]interface{})
	if got := apmModalityWords(modalities["input"]); !reflect.DeepEqual(got, []string{"image", "video"}) {
		t.Errorf("modalities.input = %v, want [image video]（既有行为不动）", got)
	}

	// 反例：显式 false 与 v2 不认的键一律不得进能力通道（也不该整个写出该键）。
	partial := map[string]interface{}{
		"properties": map[string]interface{}{
			"requiresMfjsToolSchema":   true,
			"inputFormat":              map[string]interface{}{"supportsPdf": false},
			"outputFormat":             map[string]interface{}{"supportsText": false},
			"supportsToolCall":         false,
			"supportsJsonSchemaOutput": false,
		},
	}
	partialInter := v2LegacyModelCfg(partial)
	if got, ok := partialInter[apmCapabilitiesModelKey]; ok {
		t.Fatalf("没有已知为真的能力位时不得写能力通道：%#v", got)
	}

	// 端到端：中间形态 -> v2 文档，模态位与布尔位都落到 properties。
	converted, dropped, err := zcodeprovider.FromLegacy(map[string]interface{}{
		"provider": map[string]interface{}{
			"p": map[string]interface{}{
				"name":    "P",
				"kind":    "openai-compatible",
				"options": map[string]interface{}{"baseURL": "https://p.example.invalid/v1"},
				"models":  map[string]interface{}{"demo": inter},
			},
		},
	})
	if err != nil {
		t.Fatalf("FromLegacy：%v", err)
	}
	if len(dropped) != 0 {
		t.Fatalf("内部能力通道 / modalities.output 都是已知键，不应报丢失：%v", dropped)
	}
	mcfg := zcodeprovider.ModelRuleConfig(converted.ProviderModelRule("p", "demo"))
	format := zcodeprovider.ModelInputFormat(mcfg)
	for _, key := range []string{"supportsText", "supportsImage", "supportsVideo", "supportsAudio", "supportsPdf"} {
		if format[key] != true {
			t.Errorf("v2 inputFormat.%s = %v, want true", key, format[key])
		}
	}
	if got := zcodeprovider.ModelOutputFormat(mcfg)["supportsText"]; got != true {
		t.Errorf("v2 outputFormat.supportsText = %v, want true", got)
	}
	props := zcodeprovider.ModelProperties(mcfg)
	for _, key := range []string{"supportsToolCall", "supportsJsonSchemaOutput", "supportsNativeWebSearch", "supportsMidConversationSystem"} {
		if props[key] != true {
			t.Errorf("v2 properties.%s = %v, want true", key, props[key])
		}
	}
	if _, ok := props["requiresMfjsToolSchema"]; ok {
		t.Errorf("v2 不得写 requiresMfjsToolSchema：%v", props)
	}
	if _, ok := mcfg["_apm_capabilities"]; ok {
		t.Error("_apm_capabilities 不得落进 v2 模型规则")
	}
}
