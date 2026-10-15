package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"agentprovidermanager/internal/zcodeprovider"
)

// 本文件覆盖阶段 A 修复契约（phase-a-fix-contract.md）的 S1/M1/M2：
//   - S1：「关闭思考」= 档位集合收敛为单元素 off（["off"]），写回经 legacyReasoningVariants
//     兜底成 ["disabled"]，读回由 values 反推 Reasoning=false，自洽且可持久化；
//   - A-8 不回归：本来就只含 off 的集合往返保真；
//   - M1：默认档位 high 优先；
//   - M2：legacy 写盘出口剥离 reasoning.offAlias。
//
// 全部表驱动 / 显式用例，t.TempDir() + t.Setenv 沙箱，只用占位值，绝不读真实用户配置。
// 复用的测助函数（reasoningV2Doc / v2CardOf / payloadOfCard / v2ModelValuesOf / boolPtr）
// 定义在同包的 reasoning_roundtrip_test.go。

// TestS1CloseThinkingPersistsAcrossV2RoundTrip 是 S1 的核心端到端：
// values=["disabled","low","high"] 读成开启 -> 关闭思考 -> 保存 v2 -> 再读回应为关闭且只剩 off。
func TestS1CloseThinkingPersistsAcrossV2RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("USERPROFILE", dir)

	doc := reasoningV2Doc(t, "p", "m", []string{"disabled", "low", "high"}, boolPtr(true))

	// 读入：非 off 档位存在 -> Reasoning=true。
	card := v2CardOf(t, doc, "p", "m")
	if !card.Reasoning {
		t.Fatalf("读入时集合含非 off 档位，Reasoning 应为 true：%+v", card)
	}

	// 模拟 UI 关闭思考：Reasoning=false（集合收敛由 CardToCfg 负责），并明示关闭。
	card.Reasoning = false
	card.RawReasoningEnabled = boolPtr(false)

	updated, err := zcodeV2Store{}.Upsert(doc, "p", payloadOfCard("p", "openai-compatible", card), SaveOptions{})
	if err != nil {
		t.Fatalf("关闭思考后保存不应报错（S1）：%v", err)
	}

	// 写回后的 values 必须收敛为单元素 off 原拼法（该家族 off 词是 disabled）。
	if got := v2ModelValuesOf(t, updated, "p", "m"); !reflect.DeepEqual(got, []string{"disabled"}) {
		t.Fatalf("关闭思考后 values 应收敛为 [disabled]（S1）：%v", got)
	}

	// 再读回：values=["disabled"] -> Reasoning=false，开关不会翻回开启。
	reCard := v2CardOf(t, updated, "p", "m")
	if reCard.Reasoning {
		t.Fatalf("关闭思考写回后读回 Reasoning 应仍为 false（S1 阻断项）：%+v", reCard)
	}
	if !reflect.DeepEqual(reCard.Variants, []string{"off"}) {
		t.Fatalf("读回应得到归一后的 [off]：%v", reCard.Variants)
	}
	if reCard.OffAlias != "disabled" {
		t.Fatalf("off 原拼法应保真为 disabled：%q", reCard.OffAlias)
	}
}

// TestA8SingleOffRoundTripPreserved 覆盖 A-8 不回归：
// 配置本来只有 off（["disabled"]）时，读入 variants=["off"]、写出仍写回 ["disabled"]。
func TestA8SingleOffRoundTripPreserved(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("USERPROFILE", dir)

	doc := reasoningV2Doc(t, "p", "m", []string{"disabled"}, boolPtr(true))

	card := v2CardOf(t, doc, "p", "m")
	if card.Reasoning {
		t.Fatalf("只含单个 off 档位时 Reasoning 应为 false：%+v", card)
	}
	if !reflect.DeepEqual(card.Variants, []string{"off"}) {
		t.Fatalf("单 off 集合应读成 [off]：%v", card.Variants)
	}

	// 原样回写（不改变任何内容）。
	updated, err := zcodeV2Store{}.Upsert(doc, "p", payloadOfCard("p", "openai-compatible", card), SaveOptions{})
	if err != nil {
		t.Fatalf("单 off 集合往返不应报错（A-8）：%v", err)
	}
	if got := v2ModelValuesOf(t, updated, "p", "m"); !reflect.DeepEqual(got, []string{"disabled"}) {
		t.Fatalf("单 off 集合往返必须保真为 [disabled]（A-8）：%v", got)
	}
}

// TestPickDefaultVariantHighFirst 覆盖 M1：默认档位 high 优先。
func TestPickDefaultVariantHighFirst(t *testing.T) {
	cases := []struct {
		name     string
		variants []string
		want     string
	}{
		{"off/high/max 取 high", []string{"off", "high", "max"}, "high"},
		{"含 enabled 时回落 enabled", []string{"off", "enabled"}, "enabled"},
		{"无 high 时取 max", []string{"low", "medium", "xhigh", "max"}, "max"},
		{"单元素 max", []string{"max"}, "max"},
		{"只有 off", []string{"off"}, "off"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PickDefaultVariant(tc.variants); got != tc.want {
				t.Fatalf("PickDefaultVariant(%v) = %q，want %q", tc.variants, got, tc.want)
			}
		})
	}
	// M1：三者统一 high 优先——DefaultVariantByKind 与默认兜底也必须落在 high。
	if DefaultVariantByKind["openai-compatible"] != "high" {
		t.Fatalf("DefaultVariantByKind 应为 high：%q", DefaultVariantByKind["openai-compatible"])
	}
	if got := PickDefaultVariant(nil); got != "high" {
		t.Fatalf("空集合兜底应为 high：%q", got)
	}
	// M1：死项 none 已从优先级表删除。
	if containsStr(DefaultVariantPreference, "none") {
		t.Fatalf("DefaultVariantPreference 不应再含死项 none：%v", DefaultVariantPreference)
	}
}

// TestLegacyWriteStripsOffAlias 覆盖 M2：
// legacy 写盘出口必须剥离 reasoning.offAlias，产物里不得出现该内部键。
func TestLegacyWriteStripsOffAlias(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("USERPROFILE", dir)

	// 卡片带 offAlias（模拟 v2 读入后回写 legacy 的情形）。
	card := ModelCard{
		ModelID: "m", Name: "m", Reasoning: true,
		Variants: []string{"off", "high"}, OffAlias: "disabled",
	}
	payload := payloadOfCard("p", "openai-compatible", card)

	doc := Doc{"provider": map[string]interface{}{}}
	merged, err := zcodeLegacyStore{}.Upsert(doc, "p", payload, SaveOptions{})
	if err != nil {
		t.Fatalf("legacy Upsert：%v", err)
	}
	if alias := findOffAliasInDoc(merged); alias != "" {
		t.Fatalf("legacy 产物不得含 reasoning.offAlias（M2）：找到 %q", alias)
	}
	// 断言档位本身仍落盘（stripping 不得误删用户数据）。
	providers, _ := merged["provider"].(map[string]interface{})
	raw, _ := providers["p"].(map[string]interface{})
	models, _ := raw["models"].(map[string]interface{})
	mraw, _ := models["m"].(map[string]interface{})
	reasoning, _ := mraw["reasoning"].(map[string]interface{})
	if reasoning == nil {
		t.Fatalf("legacy 产物应保留 reasoning 块：%+v", mraw)
	}
	// legacy 形态存的是归一后的规范档位拼法（off），不是 v2 的 disabled。
	if !hasStringValue(reasoning["variants"], "off") {
		t.Fatalf("legacy 产物应保留档位：%v", reasoning["variants"])
	}

	// 反证：BuildProviderCfgEditor 的原始出口确实带 offAlias，剥离函数确有必要。
	_, providerCfg, err := BuildProviderCfgEditor(payload)
	if err != nil {
		t.Fatalf("BuildProviderCfgEditor：%v", err)
	}
	if alias := findOffAliasInDoc(providerCfg); alias == "" {
		t.Fatalf("构造的中间形态应带 offAlias，否则本测试失去意义：%+v", providerCfg)
	}
}

// TestS1CloseWhenOriginalSetHasNoOff 覆盖 S1 边界：
// !Reasoning 且原集合无 off 词（如 ["low","high"]）时，关闭后收敛为 ["off"]，不报错。
func TestS1CloseWhenOriginalSetHasNoOff(t *testing.T) {
	card := ModelCard{
		ModelID: "m", Name: "m", Reasoning: false,
		Variants: []string{"low", "high"},
	}
	cfg, _, err := CardToCfg(card, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg 不应报错（S1 边界）：%v", err)
	}
	reasoning, _ := cfg["reasoning"].(map[string]interface{})
	if reasoning == nil {
		t.Fatalf("关闭思考时应写出 reasoning 块：%+v", cfg)
	}
	variants, _ := reasoning["variants"].([]string)
	if !reflect.DeepEqual(variants, []string{"off"}) {
		t.Fatalf("无 off 词的集合关闭后应收敛为 [off]：%v", variants)
	}

	// 经 FromLegacy 端到端也要能落成单元素 off 拼法（默认 disabled），不报错。
	converted, _, err := zcodeprovider.FromLegacy(map[string]interface{}{
		"provider": map[string]interface{}{"p": map[string]interface{}{
			"name": "p", "kind": "openai-compatible",
			"options": map[string]interface{}{"baseURL": "https://p.example.invalid/v1"},
			"models":  map[string]interface{}{"m": cfg},
		}},
	})
	if err != nil {
		t.Fatalf("FromLegacy：%v", err)
	}
	mcfg := zcodeprovider.ModelRuleConfig(converted.ProviderModelRule("p", "m"))
	if got := zcodeprovider.ModelReasoningLevelValues(mcfg); !reflect.DeepEqual(got, []string{"disabled"}) {
		t.Fatalf("无 off 词集合关闭后应落成 [disabled]：%v", got)
	}
}

// findOffAliasInDoc 在任意嵌套结构里递归查找键名 "offAlias"，返回其字符串值（找不到返回空串）。
//
// 注意（修 M1）：不能只 `case map[string]interface{}`——merged 的静态类型是命名类型
// `Doc`（定义 `type Doc map[string]interface{}`），Go 类型 switch 按**精确动态类型**匹配，
// `Doc` 不匹配 `map[string]interface{}`，会导致在根节点直接返回空串、断言恒真（守护 B1 的
// 测试曾因此空跑）。这里用 reflect 兼容任意 map 类型。
func findOffAliasInDoc(v interface{}) string {
	if s, ok := findKeyString(v, "offAlias"); ok {
		return s
	}
	return ""
}

// findKeyString 递归查找任意 map/slice 里键名 key 对应的字符串值。
func findKeyString(v interface{}, key string) (string, bool) {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return "", false
	}
	switch rv.Kind() {
	case reflect.Map:
		for _, mk := range rv.MapKeys() {
			ks, _ := mk.Interface().(string)
			val := rv.MapIndex(mk).Interface()
			if ks == key {
				if s, ok := val.(string); ok {
					return s, true
				}
				return "(non-string)", true
			}
			if s, ok := findKeyString(val, key); ok {
				return s, true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if s, ok := findKeyString(rv.Index(i).Interface(), key); ok {
				return s, true
			}
		}
	}
	return "", false
}

// hasStringValue 判断 variants 字段（string / []string / []interface{}）里是否含目标串。
func hasStringValue(v interface{}, want string) bool {
	switch t := v.(type) {
	case string:
		return t == want
	case []string:
		return containsStr(t, want)
	case []interface{}:
		for _, e := range t {
			if s, ok := e.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// TestStripAPMInternalModelKeysRemovesUnderscoreAndOffAlias 直接锁住剥离函数的边界：
// 下划线内部键与 reasoning.offAlias 被删，legacy 既有字段（enabled/defaultVariant）保留。
func TestStripAPMInternalModelKeysRemovesUnderscoreAndOffAlias(t *testing.T) {
	providerCfg := map[string]interface{}{
		"models": map[string]interface{}{
			"m": map[string]interface{}{
				"_raw_cfg": map[string]interface{}{"x": 1},
				"id":       "m",
				"reasoning": map[string]interface{}{
					"enabled":        false,
					"variants":       []string{"off"},
					"defaultVariant": "off",
					"offAlias":       "disabled",
				},
			},
		},
	}
	stripAPMInternalModelKeys(providerCfg)
	m := providerCfg["models"].(map[string]interface{})["m"].(map[string]interface{})
	if _, ok := m["_raw_cfg"]; ok {
		t.Fatalf("下划线内部键 _raw_cfg 应被剥离：%+v", m)
	}
	reasoning := m["reasoning"].(map[string]interface{})
	if _, ok := reasoning["offAlias"]; ok {
		t.Fatalf("reasoning.offAlias 应被剥离：%+v", reasoning)
	}
	// 既有字段不得误删。
	if _, ok := reasoning["enabled"]; !ok {
		t.Fatalf("reasoning.enabled 属既有行为，不应被剥离：%+v", reasoning)
	}
	if _, ok := reasoning["defaultVariant"]; !ok {
		t.Fatalf("reasoning.defaultVariant 属既有行为，不应被剥离：%+v", reasoning)
	}
	// 序列化后也必须无 offAlias。
	b, _ := json.Marshal(providerCfg)
	if strings.Contains(string(b), "offAlias") {
		t.Fatalf("序列化产物不得含 offAlias：%s", string(b))
	}
}

// TestStripAPMInternalModelKeysWholeDoc 锁住真实调用形状（修 B1）：
// zcodeLegacyStore.Upsert 传给剥离函数的是整份文档 {provider:{"<id>":{models:{...}}}}，
// 不是单 provider 的 {models:{...}}。早期实现只处理后者，导致 legacy 写盘时剥离是空操作。
func TestStripAPMInternalModelKeysWholeDoc(t *testing.T) {
	doc := Doc{
		"provider": map[string]interface{}{
			"p": map[string]interface{}{
				"models": map[string]interface{}{
					"m": map[string]interface{}{
						"_vendor":           "deepseek",
						"_supported_levels": []string{"off", "high"},
						"id":                "m",
						"reasoning": map[string]interface{}{
							"enabled":  true,
							"variants": []string{"disabled", "high"},
							"offAlias": "disabled",
						},
					},
				},
			},
		},
	}
	stripAPMInternalModelKeys(doc)
	providers := doc["provider"].(map[string]interface{})
	m := providers["p"].(map[string]interface{})["models"].(map[string]interface{})["m"].(map[string]interface{})
	for _, key := range []string{"_vendor", "_supported_levels"} {
		if _, ok := m[key]; ok {
			t.Fatalf("整份文档形状下，内部键 %q 应被剥离：%+v", key, m)
		}
	}
	reasoning := m["reasoning"].(map[string]interface{})
	if _, ok := reasoning["offAlias"]; ok {
		t.Fatalf("整份文档形状下，reasoning.offAlias 应被剥离：%+v", reasoning)
	}
	if _, ok := reasoning["variants"]; !ok {
		t.Fatalf("档位数据不得被误删：%+v", reasoning)
	}
	// 序列化后也不得残留任何内部键。
	b, _ := json.Marshal(doc)
	for _, needle := range []string{"offAlias", "_vendor", "_supported_levels"} {
		if strings.Contains(string(b), needle) {
			t.Fatalf("整份文档序列化产物不得含 %q：%s", needle, string(b))
		}
	}
}
