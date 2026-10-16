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
				// 原始来源只给 text：若卡片不是唯一数据源，结果里就不会有 image。
				"modalities": map[string]interface{}{"input": []interface{}{"text"}},
			},
		})
		// [text,image]：image 只能来自卡片（这正是优先级），text 由 DSH 侧补齐——见
		// dshCapabilityModalities 的「非空交集必含 text」契约。
		if got := apmModalityWords(raw["input"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Fatalf("input = %v, want [text image]（image 来自卡片、text 由 DSH 侧补齐）", got)
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

// TestDshInputModalitiesAlwaysIncludeText 锁住 DSH 侧的**有意不对称**（读端忠实、写端保证）：
//
//   - 写端 helper dshCapabilityModalities：非空交集必含 text——DSH 的模型级 input 是**完整
//     集合**语义（写出 `[image]` 就等于声明「不接受文本输入」，路由级 defaultInput 默认
//     `["text"]`），而中立能力位只有「已知为真」与「未知」两态，缺 text 只说明上游没给出该
//     事实，绝不是「text 为假」。APM 的写入路径绝不产生「声明不支持文本」的集合。
//   - 读端 dshReadInputModalities：`input` 键**存在**时忠实照读（只做合法词过滤与去重），
//     显式写出的 `input: ["image"]` 读回就是只有 image——补 text 等于把用户的明确声明改宽；
//     `input` 键**缺失**时按 DSH 的 defaultInput = ["text"] 记 [text]。
//
// 代价（有意为之、用户可见）：手写的 `input: ["image"]` 条目被 APM 保存一次后会变成
// `["text","image"]`，即「放宽一次」；此后往返稳定（见下方往返用例）。
func TestDshInputModalitiesAlwaysIncludeText(t *testing.T) {
	t.Run("写端 helper：交集非空即含 text，空交集返回 nil", func(t *testing.T) {
		cases := []struct {
			name string
			in   []string
			want []string
		}{
			{"只有 image", []string{"image"}, []string{"text", "image"}},
			{"image 与 pdf", []string{"image", "pdf"}, []string{"text", "image"}},
			{"乱序且带未知词", []string{"pdf", "telepathy", "image"}, []string{"text", "image"}},
			{"本来就是 text", []string{"text"}, []string{"text"}},
			{"text 与 image", []string{"image", "text"}, []string{"text", "image"}},
			{"交集为空（全 audio/video/pdf）", []string{"pdf", "audio", "video"}, nil},
			{"空输入", nil, nil},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				if got := dshCapabilityModalities(c.in); !reflect.DeepEqual(got, c.want) {
					t.Fatalf("dshCapabilityModalities(%v) = %v, want %v", c.in, got, c.want)
				}
			})
		}
	})

	t.Run("写入端：卡片只有 image 时 input 必含 text 且只留 text/image", func(t *testing.T) {
		raw := dshModelToRaw(ModelCard{
			ModelID:      "demo",
			Capabilities: &ModelCapabilities{InputModalities: []string{"image"}},
		}, "openai-compatible")
		got := apmModalityWords(raw["input"])
		if !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Fatalf("input = %v, want [text image]（写 [image] 等于声明不接受文本输入）", got)
		}
	})

	t.Run("写入端：卡片 image+pdf 时收敛掉 pdf 并补上 text", func(t *testing.T) {
		raw := dshModelToRaw(ModelCard{
			ModelID:      "demo",
			Capabilities: &ModelCapabilities{InputModalities: []string{"image", "pdf"}},
		}, "openai-compatible")
		got := apmModalityWords(raw["input"])
		if !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Fatalf("input = %v, want [text image]（DSH 只接受 text/image）", got)
		}
		for _, word := range []string{"pdf", "audio", "video"} {
			if hasModality(got, word) {
				t.Errorf("input 不得含 %s（DSH 的 schema 会拒收）：%v", word, got)
			}
		}
	})

	t.Run("写入端：交集为空时整键省略", func(t *testing.T) {
		raw := dshModelToRaw(ModelCard{
			ModelID:      "demo",
			Capabilities: &ModelCapabilities{InputModalities: []string{"pdf"}},
		}, "openai-compatible")
		if got, ok := raw["input"]; ok {
			t.Fatalf("卡片模态只有 pdf 时交集为空，必须整键省略（不留空数组），实际 %#v", got)
		}
	})

	t.Run("读端：DSH 节点显式只写 image 时忠实读回（不补 text）", func(t *testing.T) {
		caps := dshCapabilitiesFromModel(map[string]interface{}{"input": []interface{}{"image"}})
		if caps == nil {
			t.Fatal("DSH 读端应写入内部能力通道")
		}
		got := apmModalityWords(caps["input_modalities"])
		if !reflect.DeepEqual(got, []string{"image"}) {
			t.Fatalf("input_modalities = %v, want [image]（input 键存在时按原样读，不得补 text）", got)
		}
	})

	t.Run("读端：交集为空时不写 input_modalities", func(t *testing.T) {
		caps := dshCapabilitiesFromModel(map[string]interface{}{"input": []interface{}{"audio"}})
		if caps != nil {
			if _, ok := caps["input_modalities"]; ok {
				t.Fatalf("收敛后一个合法词都不剩时不得写 input_modalities：%#v", caps)
			}
		}
	})

	t.Run("读端：input 键缺失时按 defaultInput 记 [text]", func(t *testing.T) {
		// 键缺失 = DSH 走路由级 defaultInput = ["text"]，读端按「text 可用」记。
		caps := dshCapabilitiesFromModel(map[string]interface{}{})
		if got := apmModalityWords(caps["input_modalities"]); !reflect.DeepEqual(got, []string{"text"}) {
			t.Fatalf("input 键缺失时 input_modalities = %v, want [text]", got)
		}
		// 键存在但读不出合法词（null）：fail-closed，不凭空补 text。
		caps = dshCapabilitiesFromModel(map[string]interface{}{"input": nil})
		if cs, ok := caps["input_modalities"]; ok {
			t.Fatalf("input 键存在但读不出合法词时不得写 input_modalities：%#v", cs)
		}
	})

	t.Run("往返：显式 input:[image] 被放宽一次后稳定", func(t *testing.T) {
		// 第一步：手写的显式声明 —— 读卡片忠实，只有 image（不动用户声明）。
		inter := ConvertDeepSeekModel("demo", map[string]interface{}{
			"id":    "demo",
			"input": []interface{}{"image"},
		}, "openai-compatible")
		card := CfgToCard("demo", inter)
		if card.Capabilities == nil || !reflect.DeepEqual(card.Capabilities.InputModalities, []string{"image"}) {
			t.Fatalf("卡片读回的能力位 = %+v, want input_modalities [image]", card.Capabilities)
		}
		// 第二步：写回 —— 写端保证非空集合必含 text，于是变成 [text,image]（放宽，非收窄）。
		written := dshModelToRaw(card, "openai-compatible")
		if got := apmModalityWords(written["input"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Fatalf("写回 input = %v, want [text image]（这是有意为之的放宽）", got)
		}
		// 第三步：再读一次 —— 此时 input 里已显式含 text，往返稳定，不再变化。
		again := CfgToCard("demo", ConvertDeepSeekModel("demo", written, "openai-compatible"))
		if again.Capabilities == nil || !reflect.DeepEqual(again.Capabilities.InputModalities, []string{"text", "image"}) {
			t.Fatalf("二次读回的能力位 = %+v, want input_modalities [text image]", again.Capabilities)
		}
		if got := apmModalityWords(dshModelToRaw(again, "openai-compatible")["input"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Fatalf("二次写回 input = %v, want [text image]（放宽一次后稳定）", got)
		}
	})

	t.Run("往返：卡片 image 经 DSH 再读回，text 不丢也不会越写越窄", func(t *testing.T) {
		first := dshModelToRaw(ModelCard{
			ModelID:      "demo",
			Capabilities: &ModelCapabilities{InputModalities: []string{"image"}},
		}, "openai-compatible")
		inter := ConvertDeepSeekModel("demo", first, "openai-compatible")
		card := CfgToCard("demo", inter)
		if card.Capabilities == nil || !reflect.DeepEqual(card.Capabilities.InputModalities, []string{"text", "image"}) {
			t.Fatalf("卡片读回的能力位 = %+v, want input_modalities [text image]", card.Capabilities)
		}
		if got := apmModalityWords(dshModelToRaw(card, "openai-compatible")["input"]); !reflect.DeepEqual(got, []string{"text", "image"}) {
			t.Fatalf("二次写入 input = %v, want [text image]（往返必须稳定）", got)
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
	// 中间形态的 modalities.input 现在承载 v2 输入面的**全部 5 位**（读取走唯一的
	// zcodeprovider.InputFormatModalities；早先只搬 image/video 两位，现在一并读全，
	// 5 位全真时 5 个词都在）。
	modalities, _ := inter["modalities"].(map[string]interface{})
	if got := apmModalityWords(modalities["input"]); !reflect.DeepEqual(got, []string{"text", "image", "video", "audio", "pdf"}) {
		t.Errorf("modalities.input = %v, want [text image video audio pdf]", got)
	}

	// 反例：显式 false 与 v2 不认的键一律不得进能力通道（也不该整个写出该键）。
	// inputFormat 显式写了 supportsText: false（只有显式 false 才排除 text），因此这里
	// 读端一位都不该给出——若实现变成「无条件补 text」，本断言会立刻失败。
	partial := map[string]interface{}{
		"properties": map[string]interface{}{
			"requiresMfjsToolSchema":   true,
			"inputFormat":              map[string]interface{}{"supportsText": false, "supportsPdf": false},
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

// TestV2InputFormatImpliedTextOnRead 锁住 v2 读端对 **ZCode 原生 inputFormat.supportsText**
// 的 implied 口径（缺键即真、只有显式 false 才排除），以及它的边界：
//
//   - 该口径只作用于 ZCode 原生的 `properties.inputFormat`（ZCode 的最小增量写入会把与继承值
//     相等的键删掉，所以「被编辑过的个人规则」通常不带 supportsText 键；按严格 === true 读会
//     得到「输入 = image」这类不含 text 的残缺集合，卡片不勾「文本」且整字段前层胜出后补不回来）；
//   - `inputFormat` 整段缺失**不适用**：那是「这条规则没有声明输入面」，与「缺 text 键」不同，
//     此时不该凭空造出 text（否则 v2 → legacy 迁移会把 text 加进每一条没有任何能力信息的模型）；
//   - 内层通道 `_apm_capabilities` **不适用**：它是 APM 自己的三态契约，缺键 = 未知。
//
// 断言同时覆盖三条读路径：中间形态的模态键 `modalities.input`、内部能力通道、以及最终消费方
// 卡片（经真实的 v2 文档 + zcodeV2Store.Get 读回）。
func TestV2InputFormatImpliedTextOnRead(t *testing.T) {
	// probeV2Card 走真实读卡路径：v2 文档 → zcodeV2Store.Get → v2ModelCard → CfgToCardForSite。
	// properties 为 nil 表示这条模型规则完全没有 properties 段。
	probeV2Card := func(t *testing.T, properties map[string]interface{}) ModelCard {
		t.Helper()
		cfg := zcodeprovider.NewConfig()
		cfg.UpsertProviderRule(zcodeprovider.NewProviderRule("p", "P", zcodeprovider.GroupStandardPersonal))
		rule := zcodeprovider.NewProviderModelRule("p", "m")
		mcfg := zcodeprovider.RuleConfig(rule)
		if properties != nil {
			mcfg["properties"] = properties
		}
		cfg.UpsertProviderModelRule(rule)
		cfg.SetProviderOrder([]string{"p"})
		edit, err := zcodeV2Store{}.Get(cfg.Doc(), "p")
		if err != nil {
			t.Fatalf("读取 v2 文档失败：%v", err)
		}
		if len(edit.Cards) != 1 {
			t.Fatalf("应读回 1 张卡片，实际 %d（%+v）", len(edit.Cards), edit.Cards)
		}
		return edit.Cards[0]
	}

	cases := []struct {
		name       string
		properties map[string]interface{}
		want       []string // nil = 不该有任何「已知为真」的输入模态
		why        string
	}{
		{
			name:       "只写 supportsImage（无 supportsText 键）",
			properties: map[string]interface{}{"inputFormat": map[string]interface{}{"supportsImage": true}},
			want:       []string{"text", "image"},
			why:        "supportsText 缺键 = 默认支持文本；这正是 ZCode 最小增量写入的产物",
		},
		{
			name:       "显式 supportsText:false 才排除 text",
			properties: map[string]interface{}{"inputFormat": map[string]interface{}{"supportsText": false, "supportsImage": true}},
			want:       []string{"image"},
			why:        "证明这不是「无条件加 text」：显式 false 是排除文本的唯一方式",
		},
		{
			name:       "显式 supportsText:false 且其余四位都没写",
			properties: map[string]interface{}{"inputFormat": map[string]interface{}{"supportsText": false}},
			want:       nil,
			why:        "其余四位保持严格：缺键不算支持，因此整列为空",
		},
		{
			name:       "空的 inputFormat 对象仍读成 text",
			properties: map[string]interface{}{"inputFormat": map[string]interface{}{}},
			want:       []string{"text"},
			why:        "ZCode 设置页把「与继承值相同」的输入面全删后会留下空对象",
		},
		{
			name: "5 位全真时 5 个词带全",
			properties: map[string]interface{}{"inputFormat": map[string]interface{}{
				"supportsText": true, "supportsImage": true, "supportsVideo": true,
				"supportsAudio": true, "supportsPdf": true,
			}},
			want: []string{"text", "image", "video", "audio", "pdf"},
			why:  "v2LegacyModelCfg 的 modalities.input 承载全部 5 位",
		},
		{
			name:       "没有 properties 段时不凭空造输入面",
			properties: nil,
			want:       nil,
			why:        "implied 只对「存在的 inputFormat 段里的 supportsText 键」生效；整段缺失 = 未声明输入面",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 读路径一：内部能力通道（v2ApmCapabilities）。
			inter := v2LegacyModelCfg(map[string]interface{}{"properties": c.properties})
			caps := apmCapabilitiesChannel(inter)
			var channelWords []string
			if caps != nil {
				channelWords = apmModalityWords(caps["input_modalities"])
			}
			if !reflect.DeepEqual(channelWords, c.want) {
				t.Errorf("能力通道 input_modalities = %v，期望 %v（%s）", channelWords, c.want, c.why)
			}

			// 读路径二：中间形态的真实模态键 modalities.input（同上口径，空则整键省略）。
			var modalityWords []string
			if modalities, ok := inter["modalities"].(map[string]interface{}); ok {
				modalityWords = apmModalityWords(modalities["input"])
			}
			if !reflect.DeepEqual(modalityWords, c.want) {
				t.Errorf("modalities.input = %v，期望 %v", modalityWords, c.want)
			}

			// 读路径三：卡片（最终消费方，经真实 v2 文档读回）。
			card := probeV2Card(t, c.properties)
			var cardWords []string
			if card.Capabilities != nil {
				cardWords = card.Capabilities.InputModalities
			}
			if !reflect.DeepEqual(cardWords, c.want) {
				t.Errorf("卡片 Capabilities.InputModalities = %v，期望 %v（%s）", cardWords, c.want, c.why)
			}
		})
	}

	t.Run("内部通道缺 text 时不得凭空补 text", func(t *testing.T) {
		// 这条通道是 APM 自己的三态契约（缺键 = 未知），读端不得对它套用 ZCode 原生的
		// implied 语义——否则「只知道 image」会被读成「text+image」，在 DSH 里就是放宽用户声明。
		inter := map[string]interface{}{
			apmCapabilitiesModelKey: map[string]interface{}{
				"input_modalities": []interface{}{"image"},
				"tool_call":        true,
			},
		}
		card := CfgToCardForSite("m", inter, "", "")
		if card.Capabilities == nil {
			t.Fatalf("注入的能力通道应被读回：%+v", card)
		}
		if want := []string{"image"}; !reflect.DeepEqual(card.Capabilities.InputModalities, want) {
			t.Errorf("InputModalities = %v，期望 %v（通道缺 text 时不得补 text）", card.Capabilities.InputModalities, want)
		}
	})
}
