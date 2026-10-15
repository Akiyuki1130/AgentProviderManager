package core

import (
	"reflect"
	"strings"
	"testing"
)

// 本文件覆盖阶段 E1 卡片层的中立能力位（MODEL-MANAGEMENT-PLAN.md 第 10 节）：
//  1. providerMetaFeatures 从 /models 元数据里读 provider 自报的能力位；
//  2. BuildModelCardsForSite / CfgToCardForSite 把能力位填进 Capabilities /
//     _capabilities_inferred（provider 自报优先、能力表只填空缺；推断值没有已知能力位时不填）；
//  3. CardToCfg 写出内部通道 _apm_capabilities（所有 kind）与 opencode 真实键；
//  4. 内部通道永远不落盘（stripAPMInternalModelKeys）。
//
// 统一口径：只记录为真的位（显式 false 丢弃），未知 = nil，空集合 = nil。
// 全部使用占位数据与 example.invalid 这类保留域名，不读也不写任何真实用户配置目录。

// featureTestOnlyTrueBits 断言内部通道里没有任何 false 值：冻结契约是「只写真」，
// 未知位一律省略键，绝不写 false / null。
func featureTestOnlyTrueBits(t *testing.T, internal map[string]interface{}) {
	t.Helper()
	for key, value := range internal {
		if b, ok := value.(bool); ok && !b {
			t.Errorf("内部通道的 %s 出现了 false：%+v", key, internal)
		}
		if value == nil {
			t.Errorf("内部通道的 %s 出现了 nil：%+v", key, internal)
		}
	}
}

// featureTestNestedMap 取 m[key] 并对类型断言失败直接 t.Fatalf（测试辅助）。
func featureTestNestedMap(t *testing.T, m map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	v, ok := m[key].(map[string]interface{})
	if !ok {
		t.Fatalf("键 %q 应为对象，实际 %#v（整表 %#v）", key, m[key], m)
	}
	return v
}

// featureTestCardCaps 把卡片的能力位取成可比较的值（nil 卡片能力位返回零值 + false）。
func featureTestCardCaps(card ModelCard) (ModelFeatures, bool) {
	if card.Capabilities == nil {
		return ModelFeatures{}, false
	}
	return *card.Capabilities, true
}

// TestProviderMetaFeatures 锁定 provider 自报能力位的六类键形态、并集语义、显式 false
// 被丢弃、未知模态词被丢弃、类型不符 fail-closed。所有期望值都不带 Source（读侧不标注来源），
// 除「有值即标 provider」这一条由末尾单独断言。
func TestProviderMetaFeatures(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]interface{}
		want ModelFeatures
	}{
		{
			"modalities.input 与 modalities.output",
			map[string]interface{}{"modalities": map[string]interface{}{
				"input":  []interface{}{"text", "image"},
				"output": []interface{}{"text"},
			}},
			ModelFeatures{InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"}},
		},
		{
			"顶层 input_modalities / output_modalities（规范序重排）",
			map[string]interface{}{
				"input_modalities":  []interface{}{"pdf", "video", "audio"},
				"output_modalities": []string{"audio"},
			},
			ModelFeatures{InputModalities: []string{"video", "audio", "pdf"}, OutputModalities: []string{"audio"}},
		},
		{
			"architecture.input_modalities / architecture.output_modalities",
			map[string]interface{}{"architecture": map[string]interface{}{
				"input_modalities":  []interface{}{"text", "pdf"},
				"output_modalities": []interface{}{"image", "text"},
			}},
			ModelFeatures{InputModalities: []string{"text", "pdf"}, OutputModalities: []string{"text", "image"}},
		},
		{
			"模态字符串按分隔符切分",
			map[string]interface{}{"input_modalities": "text, image"},
			ModelFeatures{InputModalities: []string{"text", "image"}},
		},
		{
			"多个模态键取并集",
			map[string]interface{}{
				"input_modalities": []interface{}{"text"},
				"modalities":       map[string]interface{}{"input": []interface{}{"image"}},
				"architecture":     map[string]interface{}{"input_modalities": []interface{}{"text", "pdf"}},
			},
			ModelFeatures{InputModalities: []string{"text", "image", "pdf"}},
		},
		{
			"六个布尔位的顶层键",
			map[string]interface{}{
				"tool_call":                        true,
				"supports_structured_output":       true,
				"native_web_search":                true,
				"supports_mid_conversation_system": true,
				"attachment":                       true,
				"supports_temperature":             true,
			},
			ModelFeatures{
				ToolCall:         featureBool(true),
				StructuredOutput: featureBool(true),
				NativeWebSearch:  featureBool(true),
				MidConvSystem:    featureBool(true),
				Attachment:       featureBool(true),
				Temperature:      featureBool(true),
			},
		},
		{
			"布尔位别名：supports_tool_call / json_schema / web_search",
			map[string]interface{}{
				"supports_tool_call":   true,
				"json_schema":          true,
				"web_search":           true,
				"supports_attachment":  true,
				"supports_temperature": true,
			},
			ModelFeatures{
				ToolCall:         featureBool(true),
				StructuredOutput: featureBool(true),
				NativeWebSearch:  featureBool(true),
				Attachment:       featureBool(true),
				Temperature:      featureBool(true),
			},
		},
		{
			"capabilities 子对象的同名子键",
			map[string]interface{}{"capabilities": map[string]interface{}{
				"tool_call":         true,
				"structured_output": true,
				"native_web_search": true,
				"attachment":        true,
				"temperature":       true,
			}},
			ModelFeatures{
				ToolCall:         featureBool(true),
				StructuredOutput: featureBool(true),
				NativeWebSearch:  featureBool(true),
				Attachment:       featureBool(true),
				Temperature:      featureBool(true),
			},
		},
		{
			"多键冲突时任一为真即为真",
			map[string]interface{}{
				"tool_call":                  false,
				"supports_tool_call":         true,
				"capabilities":               map[string]interface{}{"tool_call": false},
				"structured_output":          false,
				"supports_structured_output": false,
			},
			ModelFeatures{ToolCall: featureBool(true)},
		},
		{
			"显式 false 被丢弃（不记成 false）",
			map[string]interface{}{
				"tool_call":         false,
				"structured_output": false,
				"temperature":       false,
				"capabilities": map[string]interface{}{
					"native_web_search": false,
					"attachment":        false,
				},
			},
			ModelFeatures{},
		},
		{
			"未知模态词被丢弃",
			map[string]interface{}{
				"input_modalities":  []interface{}{"text", "telepathy"},
				"output_modalities": []interface{}{"hologram"},
			},
			ModelFeatures{InputModalities: []string{"text"}},
		},
		{
			"类型不符 fail-closed",
			map[string]interface{}{
				"tool_call":         "true",
				"structured_output": 1,
				"native_web_search": nil,
				"attachment":        []interface{}{true},
				"temperature":       map[string]interface{}{"enabled": true},
				"input_modalities":  map[string]interface{}{"a": 1},
				"output_modalities": 3,
				"capabilities":      "not-an-object",
			},
			ModelFeatures{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := providerMetaFeatures(c.meta)
			want := c.want
			if want.HasAny() {
				// 有值即标注来源 provider（内部字段 `_source`，绝不落盘）。
				want.Source = "provider"
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("providerMetaFeatures: got %+v，期望 %+v", got, want)
			}
		})
	}
	for _, meta := range []map[string]interface{}{nil, map[string]interface{}{}} {
		if got := providerMetaFeatures(meta); got.HasAny() || got.Source != "" {
			t.Errorf("空 meta 应返回零值，实际 %+v", got)
		}
	}
}

// TestBuildModelCardsForSiteCapabilities 锁定读卡路径的补全语义：
//   - provider 自报的能力位是 API 事实，autoFill 为假也照填进 Capabilities；
//   - 能力表那部分只在 autoFill 为真时补进 Capabilities（且只填空缺，provider 优先）；
//   - _capabilities_inferred 恒为「provider + 能力表」的合并结果，与 autoFill 无关。
func TestBuildModelCardsForSiteCapabilities(t *testing.T) {
	const base = "https://example.invalid/v1"
	const api = "openai-chat-completions"
	// 占位 ID：既有测试也用它代表「任何能力来源都推不出结论」的模型。
	const unknown = "my-local-model"
	if tf := LookupModelCapabilityForSite(unknown, base, api).Features; tf.HasAny() {
		t.Fatalf("前提破坏：占位模型 %s 在能力表里出现了能力位：%+v", unknown, tf)
	}

	provider := map[string]interface{}{
		"input_modalities": []interface{}{"image", "text"},
		"tool_call":        true,
		"temperature":      false, // 显式 false 必须被丢弃
		"attachment":       "yes", // 类型不符必须被忽略
	}
	wantProvider := ModelFeatures{
		InputModalities: []string{"text", "image"},
		ToolCall:        featureBool(true),
		Source:          "provider",
	}

	for _, autoFill := range []bool{true, false} {
		c := cardsForSiteTest(t, unknown, provider, autoFill, base, api)
		got, ok := featureTestCardCaps(c)
		if !ok {
			t.Fatalf("autoFill=%v：provider 自报的能力位应始终填进 Capabilities", autoFill)
		}
		if !reflect.DeepEqual(got, wantProvider) {
			t.Errorf("autoFill=%v：Capabilities=%+v，期望 %+v", autoFill, got, wantProvider)
		}
		if c.CapabilitiesInferred == nil || !reflect.DeepEqual(*c.CapabilitiesInferred, wantProvider) {
			t.Errorf("autoFill=%v：_capabilities_inferred 应等于 provider 自报值，实际 %+v", autoFill, c.CapabilitiesInferred)
		}
	}

	// 没有任何能力来源：两个字段都留 nil（HasAny 为假的推断值不落 DTO）。
	for _, autoFill := range []bool{true, false} {
		c := cardsForSiteTest(t, unknown, map[string]interface{}{}, autoFill, base, api)
		if c.Capabilities != nil {
			t.Errorf("autoFill=%v：无能力来源时 Capabilities 应为 nil，实际 %+v", autoFill, c.Capabilities)
		}
		if c.CapabilitiesInferred != nil {
			t.Errorf("autoFill=%v：无能力来源时 _capabilities_inferred 应为 nil，实际 %+v", autoFill, c.CapabilitiesInferred)
		}
	}
	// 只有未知模态词：同样是「读不出」，不是「空集合」。
	c := cardsForSiteTest(t, unknown, map[string]interface{}{"input_modalities": []interface{}{"telepathy"}}, true, base, api)
	if caps, ok := featureTestCardCaps(c); ok {
		t.Errorf("未知模态词应被丢弃，实际 %+v", caps)
	}

	// 能力表侧的接线：Capabilities 在 autoFill 为真时 = mergeFeatures(provider, 表)，
	// 为假时 = provider。表当前是空表，因此这里逐字对照能力表的实际取值来锁住「开关门」，
	// 表被填充后这条断言会自动变得更有意义（仍不会因为数据变化而变红）。
	const known = "deepseek-v4-flash"
	table := LookupModelCapabilityForSite(known, base, api).Features
	if !table.HasAny() {
		t.Log("提示：能力表当前没有能力位，本用例的表侧断言此刻退化为「等于 provider 自报值」")
	}
	mixed := map[string]interface{}{"tool_call": true}
	if c := cardsForSiteTest(t, known, mixed, true, base, api); !reflect.DeepEqual(featureTestMustCaps(t, c), mergeFeatures(providerMetaFeatures(mixed), table)) {
		t.Errorf("autoFill=true 时 Capabilities 应等于 provider+能力表合并：%+v", featureTestMustCaps(t, c))
	}
	if c := cardsForSiteTest(t, known, mixed, false, base, api); !reflect.DeepEqual(featureTestMustCaps(t, c), providerMetaFeatures(mixed)) {
		t.Errorf("autoFill=false 时 Capabilities 只应含 provider 自报值：%+v", featureTestMustCaps(t, c))
	}
}

// featureTestMustCaps 取卡片能力位，nil 时直接失败（测试辅助）。
func featureTestMustCaps(t *testing.T, card ModelCard) ModelFeatures {
	t.Helper()
	caps, ok := featureTestCardCaps(card)
	if !ok {
		t.Fatalf("Capabilities 不应为 nil：%+v", card)
	}
	return caps
}

// TestCfgToCardForSiteCapabilities 锁定读配置路径的三种来源与优先级：
// _apm_capabilities > opencode 真实键 > v2 形状的 properties.*（前者优先、后者只填空缺）。
func TestCfgToCardForSiteCapabilities(t *testing.T) {
	const id = "my-local-model"

	t.Run("内部通道 _apm_capabilities", func(t *testing.T) {
		cfg := map[string]interface{}{"_apm_capabilities": map[string]interface{}{
			"input_modalities":  []interface{}{"image", "text"},
			"output_modalities": []interface{}{"text"},
			"tool_call":         true,
			"structured_output": true,
			"native_web_search": true,
			"mid_conv_system":   true,
			"attachment":        true,
			"temperature":       true,
		}}
		got := featureTestMustCaps(t, CfgToCardForSite(id, cfg, "", ""))
		want := ModelFeatures{
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"text"},
			ToolCall:         featureBool(true),
			StructuredOutput: featureBool(true),
			NativeWebSearch:  featureBool(true),
			MidConvSystem:    featureBool(true),
			Attachment:       featureBool(true),
			Temperature:      featureBool(true),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("读回 %+v，期望 %+v", got, want)
		}
	})

	t.Run("内部通道只认真值", func(t *testing.T) {
		cfg := map[string]interface{}{"_apm_capabilities": map[string]interface{}{
			"tool_call":         false,
			"structured_output": "true",
			"native_web_search": nil,
			"input_modalities":  []interface{}{"telepathy"},
		}}
		if caps, ok := featureTestCardCaps(CfgToCardForSite(id, cfg, "", "")); ok {
			t.Errorf("非布尔真应视为未知，实际 %+v", caps)
		}
	})

	t.Run("opencode 真实键", func(t *testing.T) {
		cfg := map[string]interface{}{
			"modalities": map[string]interface{}{
				"input":  []interface{}{"text", "image"},
				"output": []interface{}{"audio", "text"},
			},
			"attachment":  true,
			"tool_call":   true,
			"temperature": true,
		}
		card := CfgToCardForSite(id, cfg, "", "")
		got := featureTestMustCaps(t, card)
		want := ModelFeatures{
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"text", "audio"},
			ToolCall:         featureBool(true),
			Attachment:       featureBool(true),
			Temperature:      featureBool(true),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("读回 %+v，期望 %+v", got, want)
		}
		// 既有行为不变：Attachment 与 Modalities 仍按原样填。
		if !card.Attachment {
			t.Errorf("card.Attachment 应保持既有行为（true），实际 %v", card.Attachment)
		}
		if !reflect.DeepEqual(card.Modalities, cfg["modalities"]) {
			t.Errorf("card.Modalities 应原样深拷贝，实际 %#v", card.Modalities)
		}
	})

	t.Run("v2 形状的 properties", func(t *testing.T) {
		cfg := map[string]interface{}{"properties": map[string]interface{}{
			"inputFormat": map[string]interface{}{
				"supportsText":  true,
				"supportsImage": true,
				"supportsVideo": false, // 显式 false 丢弃
				"supportsPdf":   true,
			},
			"outputFormat":                  map[string]interface{}{"supportsText": true},
			"supportsToolCall":              true,
			"supportsJsonSchemaOutput":      true,
			"supportsNativeWebSearch":       false,
			"supportsMidConversationSystem": true,
		}}
		got := featureTestMustCaps(t, CfgToCardForSite(id, cfg, "", ""))
		want := ModelFeatures{
			InputModalities:  []string{"text", "image", "pdf"},
			OutputModalities: []string{"text"},
			ToolCall:         featureBool(true),
			StructuredOutput: featureBool(true),
			MidConvSystem:    featureBool(true),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("读回 %+v，期望 %+v", got, want)
		}
	})

	t.Run("优先级：内部通道 > opencode > v2，后者只填空缺", func(t *testing.T) {
		cfg := map[string]interface{}{
			"_apm_capabilities": map[string]interface{}{
				"tool_call":        true,
				"input_modalities": []interface{}{"text"},
			},
			"tool_call":  false, // 内部通道已给出真值，false 不参与（且本来也只记真）
			"attachment": true,
			"modalities": map[string]interface{}{"input": []interface{}{"image"}},
			"properties": map[string]interface{}{
				"supportsToolCall":         false, // 不得改写内部通道的 true
				"supportsJsonSchemaOutput": true,  // 前面两层都没给，这里补上
			},
		}
		got := featureTestMustCaps(t, CfgToCardForSite(id, cfg, "", ""))
		want := ModelFeatures{
			InputModalities:  []string{"text"}, // 内部通道胜出，opencode 的 image 不参与输入模态
			ToolCall:         featureBool(true),
			Attachment:       featureBool(true),
			StructuredOutput: featureBool(true),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("读回 %+v，期望 %+v", got, want)
		}
	})

	t.Run("_capabilities_inferred 有已知位才填、空则不填", func(t *testing.T) {
		// 空：能力表对这个占位模型没有任何能力位 → 留 nil，卡片不出现 `{}`。
		if table := LookupModelCapabilityForSite(id, "", "").Features; table.HasAny() {
			t.Fatalf("前提破坏：占位模型 %s 在能力表里出现了能力位：%+v", id, table)
		}
		empty := CfgToCardForSite(id, map[string]interface{}{}, "", "")
		if empty.Capabilities != nil {
			t.Errorf("配置里没有能力数据时 Capabilities 应为 nil，实际 %+v", empty.Capabilities)
		}
		if empty.CapabilitiesInferred != nil {
			t.Errorf("能力表没有已知位时不应填 _capabilities_inferred，实际 %+v", empty.CapabilitiesInferred)
		}

		// 非空：能力表给出能力位的模型必须原样填上（本分支只看「有没有值」，不锁具体取值，
		// 因此内建表按 revision 变动时不会误红）。
		for _, known := range []string{"glm-5.3-flash", "deepseek-v4.1-flash", "gpt-5.6-luna", "kimi-k2.6"} {
			table := LookupModelCapabilityForSite(known, "", "").Features
			if !table.HasAny() {
				continue
			}
			card := CfgToCardForSite(known, map[string]interface{}{}, "", "")
			if card.CapabilitiesInferred == nil || !reflect.DeepEqual(*card.CapabilitiesInferred, table) {
				t.Errorf("%s：应填能力表推断值 %+v，实际 %+v", known, table, card.CapabilitiesInferred)
			}
			return
		}
		t.Error("前提破坏：候选模型里没有一个带能力位，「有值才填」这一半未被覆盖")
	})
}

// TestCardToCfgCapabilitiesInternalChannel 锁定写卡路径的内部通道契约（10.2.4②）：
// 所有 kind 都写 _apm_capabilities、只含已知为真的位、空则连键都不写，且非 opencode
// kind 绝不写 modalities / attachment / tool_call / temperature / options。
func TestCardToCfgCapabilitiesInternalChannel(t *testing.T) {
	base := ModelCard{ModelID: "my-local-model", Name: "my-local-model"}
	base.Capabilities = &ModelCapabilities{
		InputModalities:  []string{"image", "text"},
		OutputModalities: []string{"text"},
		ToolCall:         featureBool(true),
		StructuredOutput: featureBool(true),
		Temperature:      featureBool(false), // 显式 false 必须省略键
	}
	cfg, modelID, err := CardToCfg(base, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg 失败：%v", err)
	}
	if modelID != "my-local-model" {
		t.Fatalf("modelID=%q", modelID)
	}
	internal := featureTestNestedMap(t, cfg, "_apm_capabilities")
	want := map[string]interface{}{
		"input_modalities":  []interface{}{"text", "image"}, // 规范序
		"output_modalities": []interface{}{"text"},
		"tool_call":         true,
		"structured_output": true,
	}
	if !reflect.DeepEqual(internal, want) {
		t.Errorf("_apm_capabilities=%#v，期望 %#v", internal, want)
	}
	featureTestOnlyTrueBits(t, internal)
	// 非 opencode kind：既有 delete 行为不变，真实键一个都不出现。
	for _, key := range []string{"modalities", "attachment", "tool_call", "temperature", "options", "headers"} {
		if _, ok := cfg[key]; ok {
			t.Errorf("非 opencode kind 不应写 %q：%#v", key, cfg)
		}
	}

	empty := []struct {
		name string
		caps *ModelCapabilities
	}{
		{"Capabilities 为 nil", nil},
		{"Capabilities 为空结构", &ModelCapabilities{}},
		{"只有显式 false", &ModelCapabilities{ToolCall: featureBool(false), Temperature: featureBool(false)}},
		{"只有未知模态词", &ModelCapabilities{InputModalities: []string{"telepathy"}}},
	}
	for _, c := range empty {
		card := base
		card.Capabilities = c.caps
		out, _, err := CardToCfg(card, "openai-compatible")
		if err != nil {
			t.Fatalf("%s：CardToCfg 失败：%v", c.name, err)
		}
		if _, ok := out["_apm_capabilities"]; ok {
			t.Errorf("%s：整体为空时连 _apm_capabilities 都不该写：%#v", c.name, out["_apm_capabilities"])
		}
	}

	// RawCfg 里可能残留上一轮的通道：卡片能力被清空后必须删除它，不能把陈旧值透传出去。
	stale := base
	stale.Capabilities = nil
	stale.RawCfg = map[string]interface{}{
		"_apm_capabilities": map[string]interface{}{"tool_call": true},
		"name":              "stale",
	}
	out, _, err := CardToCfg(stale, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg 失败：%v", err)
	}
	if _, ok := out["_apm_capabilities"]; ok {
		t.Errorf("陈旧通道应被删除：%#v", out["_apm_capabilities"])
	}
}

// TestCardToCfgCapabilitiesOpenCode 锁定 opencode 分支的反向映射（10.2.2/10.2.3）：
// 真实键 modalities{input,output} / attachment / tool_call / temperature 只写真，
// 既有 card.Modalities / card.Attachment 的值一个都不能丢。
func TestCardToCfgCapabilitiesOpenCode(t *testing.T) {
	t.Run("从 Capabilities 写真实键", func(t *testing.T) {
		card := ModelCard{ModelID: "r1", Name: "r1"}
		card.Capabilities = &ModelCapabilities{
			InputModalities:  []string{"image", "text"},
			OutputModalities: []string{"text"},
			ToolCall:         featureBool(true),
			Attachment:       featureBool(true),
		}
		cfg, _, err := CardToCfg(card, "opencode")
		if err != nil {
			t.Fatalf("CardToCfg 失败：%v", err)
		}
		wantModalities := map[string]interface{}{
			"input":  []interface{}{"text", "image"},
			"output": []interface{}{"text"},
		}
		if !reflect.DeepEqual(cfg["modalities"], wantModalities) {
			t.Errorf("modalities=%#v，期望 %#v", cfg["modalities"], wantModalities)
		}
		if cfg["attachment"] != true || cfg["tool_call"] != true {
			t.Errorf("attachment/tool_call 应为 true：%#v", cfg)
		}
		if _, ok := cfg["temperature"]; ok {
			t.Errorf("未知的 temperature 应省略键：%#v", cfg)
		}
		internal := featureTestNestedMap(t, cfg, "_apm_capabilities")
		featureTestOnlyTrueBits(t, internal)
		if internal["tool_call"] != true || internal["attachment"] != true {
			t.Errorf("opencode kind 同样要写内部通道：%#v", internal)
		}
	})

	t.Run("既有 Modalities 取并集、顺序不变", func(t *testing.T) {
		card := ModelCard{ModelID: "r1", Name: "r1"}
		card.Modalities = map[string]interface{}{"input": []interface{}{"video"}}
		card.Capabilities = &ModelCapabilities{
			InputModalities: []string{"text"},
			// 未知模态词不得写进 opencode 的 enum。
			OutputModalities: []string{"hologram"},
		}
		cfg, _, err := CardToCfg(card, "opencode")
		if err != nil {
			t.Fatalf("CardToCfg 失败：%v", err)
		}
		wantModalities := map[string]interface{}{"input": []interface{}{"video", "text"}}
		if !reflect.DeepEqual(cfg["modalities"], wantModalities) {
			t.Errorf("modalities=%#v，期望 %#v", cfg["modalities"], wantModalities)
		}
	})

	t.Run("显式 false 删除陈旧键，未知则沿用 RawCfg", func(t *testing.T) {
		card := ModelCard{ModelID: "r1", Name: "r1", Attachment: true}
		card.RawCfg = map[string]interface{}{
			"tool_call":   true,
			"temperature": true,
			"attachment":  true,
		}
		card.Capabilities = &ModelCapabilities{ToolCall: featureBool(false)}
		cfg, _, err := CardToCfg(card, "opencode")
		if err != nil {
			t.Fatalf("CardToCfg 失败：%v", err)
		}
		if _, ok := cfg["tool_call"]; ok {
			t.Errorf("显式 false 应删除工具调用键（绝不写 false）：%#v", cfg)
		}
		if cfg["temperature"] != true {
			t.Errorf("未知位应沿用 RawCfg 里的值：%#v", cfg["temperature"])
		}
		if cfg["attachment"] != true {
			t.Errorf("cap.Attachment 为 nil 时应沿用 card.Attachment：%#v", cfg["attachment"])
		}
	})

	t.Run("Capabilities 为空时保持既有行为", func(t *testing.T) {
		card := ModelCard{ModelID: "r1", Name: "r1"}
		cfg, _, err := CardToCfg(card, "opencode")
		if err != nil {
			t.Fatalf("CardToCfg 失败：%v", err)
		}
		// 既有行为：attachment 恒以布尔落盘，modalities 不写。
		if cfg["attachment"] != false {
			t.Errorf("attachment 应保持既有行为（false）：%#v", cfg["attachment"])
		}
		if _, ok := cfg["modalities"]; ok {
			t.Errorf("Modalities 为 nil 时不应写 modalities：%#v", cfg)
		}
		if _, ok := cfg["_apm_capabilities"]; ok {
			t.Errorf("能力为空时不应写内部通道：%#v", cfg)
		}
	})
}

// TestStripAPMInternalModelKeysRemovesCapabilities 锁定「内部通道绝不落盘」：
// _apm_capabilities 以 `_` 前缀命中统一剥离规则，两种输入形状都不残留。
func TestStripAPMInternalModelKeysRemovesCapabilities(t *testing.T) {
	card := ModelCard{ModelID: "r1", Name: "r1"}
	card.Capabilities = &ModelCapabilities{ToolCall: featureBool(true)}
	modelCfg, modelID, err := CardToCfg(card, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg 失败：%v", err)
	}
	if _, ok := modelCfg["_apm_capabilities"]; !ok {
		t.Fatalf("前提破坏：CardToCfg 应写出内部通道：%#v", modelCfg)
	}

	// 形状 1：单个 provider 的配置。
	single := map[string]interface{}{"models": map[string]interface{}{modelID: modelCfg}}
	stripAPMInternalModelKeys(single)
	written, _ := single["models"].(map[string]interface{})[modelID].(map[string]interface{})
	if _, ok := written["_apm_capabilities"]; ok {
		t.Errorf("单 provider 形状剥离后不应残留：%#v", written)
	}

	// 形状 2：整份文档。
	modelCfg2, _, err := CardToCfg(card, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg 失败：%v", err)
	}
	doc := map[string]interface{}{"provider": map[string]interface{}{
		"p1": map[string]interface{}{"models": map[string]interface{}{modelID: modelCfg2}},
	}}
	stripAPMInternalModelKeys(doc)
	providers, _ := doc["provider"].(map[string]interface{})
	p1, _ := providers["p1"].(map[string]interface{})
	models, _ := p1["models"].(map[string]interface{})
	written2, _ := models[modelID].(map[string]interface{})
	if _, ok := written2["_apm_capabilities"]; ok {
		t.Errorf("整份文档形状剥离后不应残留：%#v", written2)
	}
	for key := range written2 {
		if strings.HasPrefix(key, "_") {
			t.Errorf("写盘形状里出现内部键 %q：%#v", key, written2)
		}
	}
}

// TestOpenCodeCapabilitiesRoundTrip 是回归保护：opencode 卡片 → cfg → 卡片 的往返行为
// 在引入中立能力位后逐字段不变（含 attachment / modalities 的既有语义，其中 attachment
// 对 opencode 恒以布尔落盘是既有契约，不是本次改动引入的）。
func TestOpenCodeCapabilitiesRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		cfg  map[string]interface{}
		want map[string]interface{}
	}{
		{
			"能力齐全",
			map[string]interface{}{
				"modalities": map[string]interface{}{
					"input":  []interface{}{"text", "image"},
					"output": []interface{}{"text", "audio"},
				},
				"attachment":  true,
				"tool_call":   true,
				"temperature": true,
			},
			map[string]interface{}{
				"modalities": map[string]interface{}{
					"input":  []interface{}{"text", "image"},
					"output": []interface{}{"text", "audio"},
				},
				"attachment":  true,
				"tool_call":   true,
				"temperature": true,
			},
		},
		{
			"attachment 为 false 且无 modalities",
			map[string]interface{}{"attachment": false},
			map[string]interface{}{"attachment": false},
		},
		{
			"无任何能力键（attachment 仍按既有行为落盘为 false）",
			map[string]interface{}{},
			map[string]interface{}{"attachment": false},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			card := CfgToCardForSite("r1", deepCopyMap(c.cfg), "", "")
			out, _, err := CardToCfg(card, "opencode")
			if err != nil {
				t.Fatalf("CardToCfg 失败：%v", err)
			}
			for _, key := range []string{"modalities", "attachment", "tool_call", "temperature"} {
				want, wantOK := c.want[key]
				got, gotOK := out[key]
				if wantOK != gotOK {
					t.Errorf("%s 是否落盘变了：期望 %#v，实际 %#v", key, c.want[key], out[key])
					continue
				}
				if wantOK && !reflect.DeepEqual(want, got) {
					t.Errorf("%s 往返后变了：原始 %#v / 往返 %#v", key, want, got)
				}
			}
			// 再读一次卡片：能力位应稳定（第二次往返与第一次完全一致）。
			again, _, err := CardToCfg(CfgToCardForSite("r1", out, "", ""), "opencode")
			if err != nil {
				t.Fatalf("第二次 CardToCfg 失败：%v", err)
			}
			if !reflect.DeepEqual(again, out) {
				t.Errorf("二次往返不稳定：\n第一次 %#v\n第二次 %#v", out, again)
			}
		})
	}
}
