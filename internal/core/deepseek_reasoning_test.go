package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// 本文件覆盖阶段 A 的 A-6 修复：DSH 档位为固定 7 档
// （off,minimal,low,medium,high,xhigh,max），读写两端都必须保真往返。
// 全部使用占位值与临时目录沙箱，不读取任何真实用户配置。

// dshAllEfforts 是 DSH 的固定 7 档全集（顺序即 DSH THINKING_LEVELS 顺序）。
var dshAllEfforts = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// dshProviderRawOf 构造一份含单个模型的最小 pi-ai provider 原始配置。
func dshProviderRawOf(model map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"api":     "openai-completions",
		"baseURL": "https://api.example.com/v1",
		"models":  []interface{}{model},
	}
}

// dshReadEffortsThroughProvider 走读端 ConvertDeepSeekProvider，取出归一化后的档位集合。
func dshReadEffortsThroughProvider(t *testing.T, raw map[string]interface{}) []string {
	t.Helper()
	_, internal, err := ConvertDeepSeekProvider("demo", raw, nil)
	if err != nil {
		t.Fatalf("ConvertDeepSeekProvider: %v", err)
	}
	models, _ := internal["models"].(map[string]interface{})
	model, _ := models["demo"].(map[string]interface{})
	reasoning, _ := model["reasoning"].(map[string]interface{})
	if reasoning == nil {
		return nil
	}
	variants, _ := reasoning["variants"].([]string)
	return variants
}

// dshWriteNode 直接按档位集合构造卡片并走写端 dshModelToRaw。
func dshWriteNode(variants []string, defaultVariant string) map[string]interface{} {
	return dshModelToRaw(ModelCard{
		ModelID: "demo", Name: "demo", Reasoning: true,
		Variants: variants, DefaultVariant: defaultVariant,
		RawCfg: map[string]interface{}{},
	}, "", dshTestAPI)
}

func dshEffortsOf(t *testing.T, node map[string]interface{}) map[string]interface{} {
	t.Helper()
	efforts, ok := node["reasoningEfforts"].(map[string]interface{})
	if !ok {
		t.Fatalf("reasoningEfforts is not a dict: %#v", node["reasoningEfforts"])
	}
	return efforts
}

// TestDshSevenEffortsRoundTrip 验证读端→写端→再读端后，7 档集合与顺序一致。
func TestDshSevenEffortsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_HOME", dir)
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	t.Setenv("USERPROFILE", dir)

	in := map[string]interface{}{}
	for _, level := range dshAllEfforts {
		if level == "off" {
			in["off"] = nil
		} else {
			in[level] = level
		}
	}
	// 打乱输入顺序，证明读端按固定顺序排序。
	raw1 := dshProviderRawOf(map[string]interface{}{
		"id": "demo", "reasoningEfforts": in, "reasoningEffort": "medium",
	})

	first := dshReadEffortsThroughProvider(t, raw1)
	if !reflect.DeepEqual(first, dshAllEfforts) {
		t.Fatalf("读端档位/顺序不对: %#v，期望 %#v", first, dshAllEfforts)
	}

	node := dshWriteNode(first, "medium")
	efforts := dshEffortsOf(t, node)
	if len(efforts) != len(dshAllEfforts) {
		t.Fatalf("写端档位数量=%d，期望 %d：%#v", len(efforts), len(dshAllEfforts), efforts)
	}
	for _, level := range dshAllEfforts {
		if level == "off" {
			if v, ok := efforts["off"]; !ok || v != nil {
				t.Fatalf("off 必须是 null：%#v", efforts["off"])
			}
			continue
		}
		if efforts[level] != level {
			t.Fatalf("档位 %q 的值应为同名拼法，得到 %#v", level, efforts[level])
		}
	}
	if node["reasoningEffort"] != "medium" {
		t.Fatalf("默认档位未保留：%#v", node["reasoningEffort"])
	}

	raw2 := dshProviderRawOf(node)
	second := dshReadEffortsThroughProvider(t, raw2)
	if !reflect.DeepEqual(second, dshAllEfforts) {
		t.Fatalf("再读端档位/顺序不对: %#v，期望 %#v", second, dshAllEfforts)
	}
}

// TestDshMinimalNotFoldedToLow 验证 minimal 作为独立档位往返，不被折成 low。
func TestDshMinimalNotFoldedToLow(t *testing.T) {
	raw := dshProviderRawOf(map[string]interface{}{
		"id":               "demo",
		"reasoningEfforts": map[string]interface{}{"off": nil, "minimal": "minimal", "high": "high"},
		"reasoningEffort":  "minimal",
	})
	got := dshReadEffortsThroughProvider(t, raw)
	want := []string{"off", "minimal", "high"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("读端应保留 minimal：%#v，期望 %#v", got, want)
	}

	node := dshWriteNode(got, "minimal")
	efforts := dshEffortsOf(t, node)
	if efforts["minimal"] != "minimal" {
		t.Fatalf("写端应保留 minimal：%#v", efforts)
	}
	if _, folded := efforts["low"]; folded {
		t.Fatalf("minimal 不得被折成 low：%#v", efforts)
	}
	if node["reasoningEffort"] != "minimal" {
		t.Fatalf("默认档位 minimal 未保留：%#v", node["reasoningEffort"])
	}
}

// TestDshMediumXhighNotDropped 验证 medium/xhigh 在读写两端都不丢。
func TestDshMediumXhighNotDropped(t *testing.T) {
	raw := dshProviderRawOf(map[string]interface{}{
		"id":               "demo",
		"reasoningEfforts": map[string]interface{}{"off": nil, "low": "low", "medium": "medium", "high": "high", "xhigh": "xhigh", "max": "max"},
	})
	got := dshReadEffortsThroughProvider(t, raw)
	want := []string{"off", "low", "medium", "high", "xhigh", "max"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("读端丢档位：%#v，期望 %#v", got, want)
	}

	node := dshWriteNode(got, "high")
	efforts := dshEffortsOf(t, node)
	for _, level := range []string{"medium", "xhigh"} {
		if _, ok := efforts[level]; !ok {
			t.Fatalf("写端丢档位 %q：%#v", level, efforts)
		}
	}
}

// TestDshOffAliasesCollapseToOff 验证跨 Agent 的 off 别名统一折成 DSH 的 "off"。
func TestDshOffAliasesCollapseToOff(t *testing.T) {
	cases := []struct {
		name   string
		rawEff map[string]interface{}
	}{
		{"none", map[string]interface{}{"none": nil, "high": "high"}},
		{"disabled", map[string]interface{}{"disabled": nil, "high": "high"}},
		{"nothink", map[string]interface{}{"nothink": nil, "high": "high"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dshReadEffortsThroughProvider(t, dshProviderRawOf(map[string]interface{}{
				"id": "demo", "reasoningEfforts": tc.rawEff,
			}))
			if !containsStr(got, "off") {
				t.Fatalf("off 别名应折成 off：%#v", got)
			}
			if containsStr(got, "none") || containsStr(got, "disabled") || containsStr(got, "nothink") {
				t.Fatalf("别名不应原样保留：%#v", got)
			}
		})
	}
}

// TestDshEnabledCompatMapsToHigh 验证 DSH 无对应档位的 enabled（ZCode 特有）
// 按「开启思考的默认档位」兼容映射到 high，而不是被静默丢弃。
func TestDshEnabledCompatMapsToHigh(t *testing.T) {
	got := dshReadEffortsThroughProvider(t, dshProviderRawOf(map[string]interface{}{
		"id":               "demo",
		"reasoningEfforts": map[string]interface{}{"enabled": "enabled", "low": "low"},
	}))
	if !containsStr(got, "high") {
		t.Fatalf("enabled 应兼容映射为 high：%#v", got)
	}
	if containsStr(got, "enabled") {
		t.Fatalf("enabled 不是 DSH 合法档位，不应原样出现：%#v", got)
	}
}

// TestDshOffOnlyBecomesFalse 验证只含 off 的集合在写出时退化为 reasoningEfforts: false
// （DSH 的 zod 约束要求非空字典必须声明至少一个非 off 档位，单 off 无法用字典表达）。
func TestDshOffOnlyBecomesFalse(t *testing.T) {
	node := dshWriteNode([]string{"off"}, "off")
	if node["reasoningEfforts"] != false {
		t.Fatalf("只含 off 应写成 false：%#v", node["reasoningEfforts"])
	}
	if _, leftover := node["reasoningEffort"]; leftover {
		t.Fatalf("reasoningEfforts:false 时不得残留 reasoningEffort：%#v", node["reasoningEffort"])
	}
}

// TestDshEffortsConstraintMutualExclusion 联合断言 DSH 的两条约束互斥语义：
//   - reasoningEfforts 为 dict 时，必须非空且至少含一个非 off 档位；
//   - off 档写 null，非 off 档写同名拼法；
//   - 只含 off 的集合 / 显式关闭一律写出 false，绝不写空字典。
func TestDshEffortsConstraintMutualExclusion(t *testing.T) {
	enabledCard := ModelCard{
		ModelID: "demo", Reasoning: true,
		Variants: dshAllEfforts, DefaultVariant: "high",
		RawCfg: map[string]interface{}{},
	}
	node := dshModelToRaw(enabledCard, "", dshTestAPI)
	efforts := dshEffortsOf(t, node)
	if len(efforts) == 0 {
		t.Fatal("非空字典不得为空")
	}
	if !dshHasNonOffEffort(efforts) {
		t.Fatalf("非空字典必须至少含一个非 off 档位：%#v", efforts)
	}
	for key, v := range efforts {
		if key == "off" {
			if v != nil {
				t.Fatalf("off 必须为 null：%#v", v)
			}
			continue
		}
		if v != key {
			t.Fatalf("非 off 档位的值必须与键同名：%q=%#v", key, v)
		}
	}

	// 显式关闭：写 false，且绝不是字典。
	disabled := false
	offCard := ModelCard{
		ModelID: "demo", Reasoning: true, RawReasoningEnabled: &disabled,
		Variants: dshAllEfforts, RawCfg: map[string]interface{}{},
	}
	offNode := dshModelToRaw(offCard, "", dshTestAPI)
	if offNode["reasoningEfforts"] != false {
		t.Fatalf("显式关闭应写 false：%#v", offNode["reasoningEfforts"])
	}
	if _, isDict := offNode["reasoningEfforts"].(map[string]interface{}); isDict {
		t.Fatal("显式关闭不得写成字典")
	}
}

// TestDshApplyExplicitReasoningOffJoint 联合断言显式关闭链路：
// dshApplyExplicitReasoningOff 清掉陈旧推理键后，CfgToCard + dshModelToRaw 必须落到
// reasoningEfforts: false（与 dshApplyExplicitReasoningOff 的出口一致）。
func TestDshApplyExplicitReasoningOffJoint(t *testing.T) {
	providerCfg := map[string]interface{}{
		"models": map[string]interface{}{
			"demo": map[string]interface{}{
				"id":               "demo",
				"reasoningEfforts": map[string]interface{}{"off": nil, "medium": "medium", "high": "high"},
			},
		},
	}
	disabled := false
	payload := ProviderPayload{
		"cards": []ModelCard{{
			ModelID: "demo", RawReasoningEnabled: &disabled,
			Variants: dshAllEfforts, RawCfg: map[string]interface{}{},
		}},
	}
	dshApplyExplicitReasoningOff(providerCfg, payload)

	models, _ := providerCfg["models"].(map[string]interface{})
	entry, _ := models["demo"].(map[string]interface{})
	if _, stale := entry["reasoningEfforts"]; stale {
		t.Fatalf("陈旧 reasoningEfforts 未被清除：%#v", entry)
	}
	card := CfgToCard("demo", entry)
	if card.RawReasoningEnabled == nil || *card.RawReasoningEnabled {
		t.Fatalf("应读出显式关闭：%#v", card.RawReasoningEnabled)
	}
	node := dshModelToRaw(card, "", dshTestAPI)
	if node["reasoningEfforts"] != false {
		t.Fatalf("显式关闭最终应写成 false：%#v", node["reasoningEfforts"])
	}
}

// TestDshSevenEffortsPersistThroughStore 在临时目录里走一次真实写盘读回，
// 证明 7 档经 store 往返后仍完整。
func TestDshSevenEffortsPersistThroughStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DSH_HOME", dir)
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	t.Setenv("USERPROFILE", dir)
	path := filepath.Join(dir, "settings.json")

	in := map[string]interface{}{}
	for _, level := range dshAllEfforts {
		if level == "off" {
			in["off"] = nil
		} else {
			in[level] = level
		}
	}
	cfg := map[string]interface{}{
		"llm-pi-ai": map[string]interface{}{"providers": map[string]interface{}{
			"demo": dshProviderRawOf(map[string]interface{}{
				"id": "demo", "reasoningEfforts": in, "reasoningEffort": "xhigh",
			}),
		}},
	}
	if err := writeDeepSeekConfig(path, cfg, ""); err != nil {
		t.Fatal(err)
	}
	loaded, err := DeepSeekLoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	providers := DeepSeekProvidersMap(loaded)
	raw, _ := providers["demo"].(map[string]interface{})
	if raw == nil {
		t.Fatal("provider 未读回")
	}
	got := dshReadEffortsThroughProvider(t, raw)
	if !reflect.DeepEqual(got, dshAllEfforts) {
		t.Fatalf("store 往返丢档位：%#v，期望 %#v", got, dshAllEfforts)
	}

	// 落盘内容本身也必须是合法的 7 档字典。
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("写入为空")
	}
}

// ---- 阶段 G：DSH 写端用能力表兜底推理档位（MODEL-MANAGEMENT-PLAN.md 10.12）----
//
// 背景：DSH 的「推理等级」只在模型节点声明了 reasoningEfforts 时才渲染，而 APM 写出的
// 都是手写路由（拿不到 pi-ai 内置目录），ZCode 的档位又常常只存在于 ZCode 自己的内建
// 规则表里——于是迁移过去的模型一个档位都没有，DSH 里也没有入口能补。兜底在写端用能力表
// 补这个空缺，且**只补空缺**：显式开/关、已有档位、节点里已声明的推理键一律不动。
//
// 每个用例都用 t.Cleanup 把全局开关还原为默认 false，避免污染其它测试。

// setDshLevelFallbackForTest 设置兜底开关并在用例结束时还原为默认 false。
func setDshLevelFallbackForTest(t *testing.T, on bool) {
	t.Helper()
	SetDshLevelFallbackAllowed(on)
	t.Cleanup(func() { SetDshLevelFallbackAllowed(false) })
}

// TestDshEffortsFromCapabilityTranslation 锁住「能力表档位 → reasoningEfforts 字典」的
// 翻译：复用写端的白名单与拼法，且永不出产 DSH 拒绝的形态（空字典 / 只含 off 的字典）。
func TestDshEffortsFromCapabilityTranslation(t *testing.T) {
	cases := []struct {
		name   string
		levels []string
		want   map[string]interface{}
	}{
		{
			name:   "7 档全集",
			levels: dshAllEfforts,
			want: map[string]interface{}{
				"off": nil, "minimal": "minimal", "low": "low", "medium": "medium",
				"high": "high", "xhigh": "xhigh", "max": "max",
			},
		},
		{
			name:   "无 off 的子集原样保留",
			levels: []string{"low", "high", "max"},
			want:   map[string]interface{}{"low": "low", "high": "high", "max": "max"},
		},
		{
			name:   "ZCode 的 enabled 折成 high",
			levels: []string{"off", "enabled"},
			want:   map[string]interface{}{"off": nil, "high": "high"},
		},
		{name: "只有 off 时一个键都不写", levels: []string{"off"}, want: nil},
		{name: "空集合不写", levels: nil, want: nil},
		{name: "白名单外的词被滤光后为空", levels: []string{"thinking", "ultra"}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dshEffortsFromCapability(tc.levels)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("dshEffortsFromCapability(%v) = %#v，期望 %#v", tc.levels, got, tc.want)
			}
		})
	}
}

// TestDshLevelFallbackGuards 覆盖兜底的门控：只有「卡片完全没有推理信息、模型节点里也没有
// 推理键、且能力表确实给出档位」时才写 reasoningEfforts。
func TestDshLevelFallbackGuards(t *testing.T) {
	disabled, enabled := false, true
	noLevels := map[string]interface{}{"off": nil, "low": "low", "high": "high", "max": "max"}
	cases := []struct {
		name    string
		allowed bool
		card    ModelCard
		// wantEfforts 为 nil 表示节点里不应出现 reasoningEfforts 键。
		wantEfforts interface{}
	}{
		{
			name:        "开关打开且卡片无任何推理信息：按能力表补档位",
			allowed:     true,
			card:        ModelCard{ModelID: "deepseek-v4-flash", RawCfg: map[string]interface{}{}},
			wantEfforts: noLevels,
		},
		{
			name:        "开关关闭：行为与兜底之前逐字节一致",
			allowed:     false,
			card:        ModelCard{ModelID: "deepseek-v4-flash", RawCfg: map[string]interface{}{}},
			wantEfforts: nil,
		},
		{
			name:        "显式关闭仍是 DSH 的 false 标记",
			allowed:     true,
			card:        ModelCard{ModelID: "deepseek-v4-flash", RawReasoningEnabled: &disabled, RawCfg: map[string]interface{}{}},
			wantEfforts: false,
		},
		{
			name:        "显式开启按用户选的档位写，不追加",
			allowed:     true,
			card:        ModelCard{ModelID: "deepseek-v4-flash", Reasoning: true, RawReasoningEnabled: &enabled, Variants: []string{"off", "medium", "high"}, RawCfg: map[string]interface{}{}},
			wantEfforts: map[string]interface{}{"off": nil, "medium": "medium", "high": "high"},
		},
		{
			name:        "卡片已有档位：既不追加也不修正",
			allowed:     true,
			card:        ModelCard{ModelID: "deepseek-v4-flash", Reasoning: true, Variants: []string{"high"}, RawCfg: map[string]interface{}{}},
			wantEfforts: map[string]interface{}{"high": "high"},
		},
		{
			name:        "节点已声明 reasoningEfforts: false：原样保留（不冒充推断）",
			allowed:     true,
			card:        ModelCard{ModelID: "deepseek-v4-flash", RawCfg: map[string]interface{}{"reasoningEfforts": false}},
			wantEfforts: false,
		},
		{
			name:        "节点已有档位字典：原样保留",
			allowed:     true,
			card:        ModelCard{ModelID: "deepseek-v4-flash", RawCfg: map[string]interface{}{"reasoningEfforts": map[string]interface{}{"off": nil, "ultra": "ultra"}}},
			wantEfforts: map[string]interface{}{"off": nil, "ultra": "ultra"},
		},
		{
			name:        "能力表未给出档位：一个键都不写",
			allowed:     true,
			card:        ModelCard{ModelID: "acme-1", RawCfg: map[string]interface{}{}},
			wantEfforts: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setDshLevelFallbackForTest(t, tc.allowed)
			node := dshModelToRaw(tc.card, "https://api.example.com/v1", dshTestAPI)
			got, has := node["reasoningEfforts"]
			if tc.wantEfforts == nil {
				if has {
					t.Fatalf("不应写入 reasoningEfforts：%#v", got)
				}
			} else {
				if !has {
					t.Fatalf("reasoningEfforts 缺失，期望 %#v", tc.wantEfforts)
				}
				if !reflect.DeepEqual(got, tc.wantEfforts) {
					t.Fatalf("reasoningEfforts = %#v，期望 %#v", got, tc.wantEfforts)
				}
			}
			// 兜底不产出单数键：推断不替用户挑默认档位，交给 DSH 的 provider 默认。
			if _, has := node["reasoningEffort"]; has {
				t.Fatalf("兜底不得写单数键 reasoningEffort：%#v", node["reasoningEffort"])
			}
		})
	}
}

// TestDshLevelFallbackThroughProfilePatch 走真实写盘路径（profile patch 的 Upsert）验证
// 「覆盖所有 DSH 写入路径」：legacy settings 与 profile patch 共用同一个模型编码出口，
// 因此这里锁住 profile 一侧即可；开关打开时补档位，关闭时一个键都不写。
func TestDshLevelFallbackThroughProfilePatch(t *testing.T) {
	const text = `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      demo:
        api: openai-completions
        baseURL: https://api.example.com/v1
        models:
          - id: deepseek-v4-flash
            contextWindow: 1000
`
	cases := []struct {
		name    string
		allowed bool
		want    bool
	}{
		{name: "开关打开：补出推理档位", allowed: true, want: true},
		{name: "开关关闭：一个键都不写", allowed: false, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setDshLevelFallbackForTest(t, tc.allowed)
			home := t.TempDir()
			t.Setenv("DSH_HOME", home)
			path := DeepSeekDesktopProfilePatch()
			writeDSHProfile(t, path, text)
			backend, err := GetBackend(string(AgentDeepSeek), path)
			if err != nil {
				t.Fatal(err)
			}
			doc, fingerprint, err := backend.ReadWithFingerprint(path)
			if err != nil {
				t.Fatal(err)
			}
			payload := ProviderPayload{
				"id":               "demo",
				"name":             "Demo",
				"kind":             "openai-compatible",
				"base_url":         "https://api.example.com/v1",
				"api_key_required": false,
				"cards":            []ModelCard{{ModelID: "deepseek-v4-flash", Context: 1000}},
			}
			merged, err := backend.Store(doc).Upsert(doc, "demo", payload, SaveOptions{MergeModels: true})
			if err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			if err := backend.Write(path, merged, fingerprint); err != nil {
				t.Fatalf("Write: %v", err)
			}

			// 从落盘内容里读回模型节点，避免依赖内存中的文档。
			reread, err := backend.Read(path)
			if err != nil {
				t.Fatal(err)
			}
			providers, err := dshEffectiveProviders(reread)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := providers["demo"].(map[string]interface{})
			if raw == nil {
				t.Fatal("provider 未读回")
			}
			var node map[string]interface{}
			models, _ := raw["models"].([]interface{})
			for _, value := range models {
				mm, _ := value.(map[string]interface{})
				if mm != nil && mm["id"] == "deepseek-v4-flash" {
					node = mm
				}
			}
			if node == nil {
				t.Fatal("模型节点未读回")
			}
			got, has := node["reasoningEfforts"]
			if !tc.want {
				if has {
					t.Fatalf("开关关闭时不应写入 reasoningEfforts：%#v", got)
				}
				return
			}
			// 期望值直接取自能力表：用例锁的是「写端确实按能力表补」，而不是把表里的
			// 具体档位再抄一遍（表是构建期生成的，抄一遍只会让两处一起漂移）。
			want := dshEffortsFromCapability(
				LookupModelCapabilityForSite("deepseek-v4-flash", "https://api.example.com/v1", "").ReasoningLevels,
			)
			if len(want) == 0 {
				t.Fatal("能力表未给出 deepseek-v4-flash 的档位，本用例失去意义")
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("reasoningEfforts = %#v，期望 %#v（能力表推断）", got, want)
			}
		})
	}
}
