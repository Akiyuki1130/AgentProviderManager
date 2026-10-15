package core

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// mustRe 是测试用的预编译辅助：把字面模式包成与生成文件一致的锚定 + 大小写不敏感形式。
func mustRe(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile("(?i)^(?:" + pattern + ")$")
	if err != nil {
		t.Fatalf("编译测试正则失败 %q：%v", pattern, err)
	}
	return re
}

// TestLookupModelCapabilityCatalog 用实测值锁定「全表顺序扫描 + 逐字段覆盖」的最终结果。
// 期望值来自对 ZCode 内建 modelRules（revision 31）的逐条仿真，跳过纯兜底 `.*` 后的产物。
func TestLookupModelCapabilityCatalog(t *testing.T) {
	cases := []struct {
		id         string
		wantCtx    int
		wantOut    int
		wantLevels []string
		wantVendor string
	}{
		{"deepseek-v4.1-flash", 1000000, 384000, []string{"off", "low", "high", "max"}, "deepseek"},
		{"deepseek-v4-flash", 1000000, 384000, []string{"off", "low", "high", "max"}, "deepseek"},
		{"deepseek-flash", 1000000, 384000, []string{"off", "low", "high", "max"}, "deepseek"},
		{"glm-5.3-flash", 1000000, 128000, []string{"low", "high", "max"}, "zhipu"},
		{"glm-5.3", 1000000, 128000, []string{"low", "high", "max"}, "zhipu"},
		{"glm-5.2", 1000000, 128000, []string{"off", "high", "max"}, "zhipu"},
		{"glm-4.6", 200000, 131072, []string{"off", "enabled"}, "zhipu"},
		{"kimi-k2.7-code", 262144, 98304, []string{"enabled"}, "moonshot"},
		{"kimi-k2.6", 262144, 98304, []string{"off", "enabled"}, "moonshot"},
		{"gpt-5.6-luna-2026-01-01", 1050000, 128000, []string{"off", "low", "medium", "high", "xhigh", "max"}, "openai"},
		{"qwen3-max", 262144, 65536, nil, "alibaba"},
		{"minimax-m2.5", 204800, 32000, nil, "minimax"},
	}
	for _, c := range cases {
		cap, ok := LookupModelCapability(c.id)
		if !ok {
			t.Errorf("%s: 期望 ok==true，实际 false", c.id)
			continue
		}
		if cap.Context != c.wantCtx {
			t.Errorf("%s: Context=%d，期望 %d", c.id, cap.Context, c.wantCtx)
		}
		if cap.Output != c.wantOut {
			t.Errorf("%s: Output=%d，期望 %d", c.id, cap.Output, c.wantOut)
		}
		if len(c.wantLevels) == 0 {
			if len(cap.ReasoningLevels) != 0 {
				t.Errorf("%s: ReasoningLevels=%v，期望为空(nil)", c.id, cap.ReasoningLevels)
			}
		} else if !reflect.DeepEqual(cap.ReasoningLevels, c.wantLevels) {
			t.Errorf("%s: ReasoningLevels=%v，期望 %v", c.id, cap.ReasoningLevels, c.wantLevels)
		}
		if cap.Vendor != c.wantVendor {
			t.Errorf("%s: Vendor=%q，期望 %q", c.id, cap.Vendor, c.wantVendor)
		}
	}
}

// TestLookupModelCapabilityUnknown 未知模型：三字段皆空（兜底 `.*` 被跳过），但仍返回 ok==true。
// 这是 APM 有意偏离 ZCode 兜底默认值（不把未知模型填成 200000）的核心断言。
func TestLookupModelCapabilityUnknown(t *testing.T) {
	cap, ok := LookupModelCapability("my-local-model")
	if !ok {
		t.Fatal("未知模型也应返回 ok==true")
	}
	if cap.Context != 0 || cap.Output != 0 || len(cap.ReasoningLevels) != 0 {
		t.Fatalf("未知模型三字段应为空，实际 %+v", cap)
	}
	if cap.Vendor != "unknown" {
		t.Errorf("Vendor=%q，期望 unknown", cap.Vendor)
	}
}

// TestLookupModelCapabilityEmptyID 空 modelID 是唯一返回 ok==false 的情形。
func TestLookupModelCapabilityEmptyID(t *testing.T) {
	cap, ok := LookupModelCapability("   ")
	if ok {
		t.Fatal("空 modelID 应返回 ok==false")
	}
	if cap.Vendor != "unknown" || cap.Source != "preset" {
		t.Errorf("空 modelID 的兜底条目不符：%+v", cap)
	}
}

// TestMergeCatalogSkipsCatchAll 直接锁住「跳过纯兜底 `.*` 规则」：未知模型只命中它时不得被填值。
func TestMergeCatalogSkipsCatchAll(t *testing.T) {
	rules := []catalogRule{
		{Pattern: ".*", Re: mustRe(t, `.*`), Context: 200000, Output: 32000, Levels: []string{"disabled", "enabled"}},
	}
	cap := mergeCatalog("anything-at-all", rules)
	if cap.Context != 0 || cap.Output != 0 || cap.ReasoningLevels != nil {
		t.Fatalf("纯兜底规则应被跳过，实际 %+v", cap)
	}
}

// TestMergeCatalogFieldWiseOverwrite 逐字段覆盖的负向用例：后命中的规则只给 Context 时，
// Output 与 Levels 必须保持前一条规则的值，绝不被清零。
func TestMergeCatalogFieldWiseOverwrite(t *testing.T) {
	rules := []catalogRule{
		// 纯兜底，必须被跳过。
		{Pattern: ".*", Re: mustRe(t, `.*`), Context: 200000, Output: 32000, Levels: []string{"disabled", "enabled"}},
		// 家族规则：三类字段都给。
		{Pattern: `model-x.*`, Re: mustRe(t, `model-x.*`), Context: 1000, Output: 500, Levels: []string{"low", "high"}},
		// 更细规则：只给 Context，不得清空 Output / Levels。
		{Pattern: `.*narrow`, Re: mustRe(t, `.*narrow`), Context: 2000},
	}
	cap := mergeCatalog("model-x-narrow", rules)
	if cap.Context != 2000 {
		t.Errorf("Context=%d，期望 2000（后命中覆盖）", cap.Context)
	}
	if cap.Output != 500 {
		t.Errorf("Output=%d，期望 500（后命中未给出，应保留）", cap.Output)
	}
	if !reflect.DeepEqual(cap.ReasoningLevels, []string{"low", "high"}) {
		t.Errorf("ReasoningLevels=%v，期望 [low high]（后命中未给出，应保留）", cap.ReasoningLevels)
	}
}

// TestLookupCapabilityOverrideWins 覆盖层「显式给出即胜出」：注入的 override 必须盖过正则结果。
func TestLookupCapabilityOverrideWins(t *testing.T) {
	overrides := []modelOverride{
		{Match: "deepseek-v4.1-flash", Context: 123, Output: 456, Levels: []string{"high"}, Vendor: "deepseek"},
	}
	cap := lookupCapability("deepseek-v4.1-flash", overrides)
	if cap.Context != 123 || cap.Output != 456 {
		t.Errorf("override 未覆盖 ctx/out：%+v", cap)
	}
	if !reflect.DeepEqual(cap.ReasoningLevels, []string{"high"}) {
		t.Errorf("override 未覆盖 levels：%v", cap.ReasoningLevels)
	}
	if cap.Source != "override" {
		t.Errorf("Source=%q，期望 override", cap.Source)
	}
}

// TestLookupCapabilityOverridePrefix 覆盖层支持最长前缀；未给出的字段沿用后续来源。
func TestLookupCapabilityOverridePrefix(t *testing.T) {
	overrides := []modelOverride{
		{Match: "glm-5.3", Vendor: "zhipu"}, // 只给厂商
	}
	cap := lookupCapability("glm-5.3-2026-01-01", overrides)
	if cap.Vendor != "zhipu" {
		t.Errorf("Vendor=%q，期望 zhipu（来自 override）", cap.Vendor)
	}
	// ctx/out/levels 由正则补上（override 未给出，不得清零）。
	if cap.Context != 1000000 || cap.Output != 128000 {
		t.Errorf("override 未给出的字段应由正则补齐：%+v", cap)
	}
	if !reflect.DeepEqual(cap.ReasoningLevels, []string{"low", "high", "max"}) {
		t.Errorf("ReasoningLevels=%v，期望 [low high max]", cap.ReasoningLevels)
	}
	// 修 S1：override 只给 Vendor（未贡献能力字段）时，Source 不应误标为 override。
	if cap.Source != "zcode" {
		t.Errorf("Source=%q，期望 zcode（override 未贡献能力字段）", cap.Source)
	}
}

// TestLookupCapabilityOverrideLevelsWin 覆盖层与正则同时给 Levels 时，override 必须胜出。
func TestLookupCapabilityOverrideLevelsWin(t *testing.T) {
	overrides := []modelOverride{
		{Match: "glm-5.3", Levels: []string{"low", "max"}}, // 只给档位
	}
	cap := lookupCapability("glm-5.3", overrides)
	if !reflect.DeepEqual(cap.ReasoningLevels, []string{"low", "max"}) {
		t.Errorf("override 的档位应胜出正则：%v，期望 [low max]", cap.ReasoningLevels)
	}
	// override 未给 ctx/out，应由正则补齐，不得清零。
	if cap.Context != 1000000 || cap.Output != 128000 {
		t.Errorf("override 未给的 ctx/out 应由正则补齐：%+v", cap)
	}
	if cap.Source != "override" {
		t.Errorf("Source=%q，期望 override（override 贡献了档位）", cap.Source)
	}
}

// TestLookupCapabilityOverridePrefixSiblings 记录 S2 已知的前缀语义：
// 前缀键会命中同前缀兄弟模型（这是有意保留的行为，用文档约束「键要足够长」）。
func TestLookupCapabilityOverridePrefixSiblings(t *testing.T) {
	overrides := []modelOverride{
		{Match: "modelbase", Context: 777},
	}
	for _, id := range []string{"modelbase", "modelbase-vision", "modelbase-0731"} {
		cap := lookupCapability(id, overrides)
		if cap.Context != 777 {
			t.Errorf("%s: 前缀键应命中同前缀兄弟，Context=%d，期望 777", id, cap.Context)
		}
	}
}

// TestLookupModelCapabilityCaseInsensitive 大小写回归：provider 常以小写传模型名，
// 而内建部分规则（GLM-5.2 / MiniMax-M2.5）字面量含大写，必须仍能命中。
func TestLookupModelCapabilityCaseInsensitive(t *testing.T) {
	cases := []struct {
		id      string
		wantCtx int
	}{
		{"glm-5.2", 1000000},
		{"GLM-5.2", 1000000},
		{"minimax-m2.5", 204800},
		{"MiniMax-M2.5", 204800},
	}
	for _, c := range cases {
		cap, _ := LookupModelCapability(c.id)
		if cap.Context != c.wantCtx {
			t.Errorf("%s: Context=%d，期望 %d（大小写不敏感）", c.id, cap.Context, c.wantCtx)
		}
	}
}

// TestLookupModelCapabilityDeepseekChat 回归：deepseek-chat 不命中任何家族规则（纯兜底被跳过），
// 正则层应为空；但它仍能靠 presets 旧表兜底拿到 128000/8000——关键是**不得**被误填成兜底的 200000。
func TestLookupModelCapabilityDeepseekChat(t *testing.T) {
	if raw := mergeCatalog("deepseek-chat", modelCatalogRules); raw.Context != 0 || raw.Output != 0 || raw.ReasoningLevels != nil {
		t.Fatalf("mergeCatalog(deepseek-chat) 应为空（只命中被跳过的兜底），实际 %+v", raw)
	}
	cap, ok := LookupModelCapability("deepseek-chat")
	if !ok {
		t.Fatal("deepseek-chat 应返回 ok==true")
	}
	if cap.Context == 200000 {
		t.Fatal("deepseek-chat 不得被兜底规则误填成 200000")
	}
	if cap.Context != 128000 || cap.Output != 8000 {
		t.Errorf("deepseek-chat 应由 presets 兜底为 128000/8000，实际 ctx=%d out=%d", cap.Context, cap.Output)
	}
}

// TestModelOverridesAreUsable 覆盖层初始条目（若存在）必须自洽可用：值非零且厂商已知。
func TestModelOverridesAreUsable(t *testing.T) {
	for _, ov := range modelOverrides {
		if strings.TrimSpace(ov.Match) == "" {
			t.Errorf("覆盖层条目缺 Match：%+v", ov)
		}
		if ov.Context == 0 && ov.Output == 0 && len(ov.Levels) == 0 && ov.Vendor == "" {
			t.Errorf("覆盖层条目 %q 一个字段都没给，属无效条目", ov.Match)
		}
	}
}

// TestDetectVendor 厂商判定表：只验证规范名与基本命中，不涉及能力表。
func TestDetectVendor(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"claude-opus-4-8", "anthropic"},
		{"gpt-4.1", "openai"},
		{"o3-mini", "openai"},
		{"deepseek-chat", "deepseek"},
		{"glm-4.6", "zhipu"},
		{"qwen3-max", "alibaba"},
		{"kimi-k2.6", "moonshot"},
		{"minimax-m2.5", "minimax"},
		{"grok-4.5", "xai"},
		{"doubao-1-5-pro-32k-250115", "bytedance"},
		{"mimo-v2-flash", "xiaomi"},
		{"some-random-model", "unknown"},
	}
	for _, c := range cases {
		if got := detectVendor(c.id); got != c.want {
			t.Errorf("detectVendor(%q)=%q，期望 %q", c.id, got, c.want)
		}
	}
}

// TestStripAPMInternalModelKeysVendorAndLevels 厂商与档位是 APM 内部字段，绝不能落回配置。
func TestStripAPMInternalModelKeysVendorAndLevels(t *testing.T) {
	providerCfg := map[string]interface{}{
		"models": map[string]interface{}{
			"m": map[string]interface{}{
				"_raw_cfg":          map[string]interface{}{"x": 1},
				"_off_alias":        "disabled",
				"_vendor":           "deepseek",
				"_supported_levels": []string{"off", "high"},
				"_raw_x":            "y",
				"id":                "m",
				"reasoning": map[string]interface{}{
					"enabled":  true,
					"variants": []string{"off", "high"},
				},
			},
		},
	}
	stripAPMInternalModelKeys(providerCfg)
	m := providerCfg["models"].(map[string]interface{})["m"].(map[string]interface{})
	for _, key := range []string{"_raw_cfg", "_off_alias", "_vendor", "_supported_levels", "_raw_x"} {
		if _, ok := m[key]; ok {
			t.Errorf("内部键 %q 应被剥离：%+v", key, m)
		}
	}
	// 正常键必须保留。
	if _, ok := m["reasoning"]; !ok {
		t.Fatalf("reasoning 块不应被误删：%+v", m)
	}
	if _, ok := m["id"]; !ok {
		t.Fatalf("id 不应被误删：%+v", m)
	}
}
