package core

import (
	"reflect"
	"strings"
	"testing"

	"agentprovidermanager/internal/ompprovider"
)

// TestOMPWireAPIMapping 锁住 OMP wire api 与 UI kind 的双向映射：只有三种可建模。
func TestOMPWireAPIMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		wire string
		kind string
	}{
		{OMPWireAnthropicMessages, "anthropic"},
		{OMPWireOpenAIResponses, "responses"},
		{OMPWireOpenAICompletions, "openai-compatible"},
		{"google-generative-ai", ""},
		{"openai-chat", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := ompWireAPIToKind(tc.wire); got != tc.kind {
			t.Fatalf("ompWireAPIToKind(%q) = %q，期望 %q", tc.wire, got, tc.kind)
		}
		if got := OMPWireAPIForKind(tc.kind); got != (func() string {
			if tc.kind == "" {
				return ""
			}
			return tc.wire
		}()) {
			t.Fatalf("OMPWireAPIForKind(%q) = %q，期望 %q", tc.kind, got, tc.wire)
		}
	}
}

// TestOMPReadOnlyReason 覆盖计划 §4.7 的只读判定表；返回空串表示可写。
func TestOMPReadOnlyReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     map[string]interface{}
		wantSub string
	}{
		{
			name:    "缺少任何 api",
			raw:     map[string]interface{}{"baseUrl": "https://example.com/v1"},
			wantSub: "无法确定该提供商的协议",
		},
		{
			name:    "模型 api 不一致",
			raw:     map[string]interface{}{"baseUrl": "https://example.com/v1", "models": []interface{}{map[string]interface{}{"id": "a", "api": OMPWireOpenAICompletions}, map[string]interface{}{"id": "b", "api": OMPWireAnthropicMessages}}},
			wantSub: "多种 api",
		},
		{
			name:    "协议不可建模",
			raw:     map[string]interface{}{"api": "google-generative-ai", "baseUrl": "https://example.com/v1"},
			wantSub: "P1 仅支持",
		},
		{
			name:    "缺少 baseUrl",
			raw:     map[string]interface{}{"api": OMPWireAnthropicMessages},
			wantSub: "没有 baseUrl",
		},
		{
			name:    "未知 auth",
			raw:     map[string]interface{}{"api": OMPWireAnthropicMessages, "baseUrl": "https://example.com/v1", "auth": "device-code"},
			wantSub: "auth",
		},
		{
			name:    "只有 modelOverrides",
			raw:     map[string]interface{}{"api": OMPWireAnthropicMessages, "baseUrl": "https://example.com/v1", "modelOverrides": map[string]interface{}{"claude-x": map[string]interface{}{"contextWindow": 1000}}},
			wantSub: "modelOverrides",
		},
		{
			name:    "oauth 且无 key",
			raw:     map[string]interface{}{"api": OMPWireAnthropicMessages, "baseUrl": "https://example.com/v1", "auth": "oauth"},
			wantSub: "oauth",
		},
		{
			name:    "非 chat 模型",
			raw:     map[string]interface{}{"api": OMPWireAnthropicMessages, "baseUrl": "https://example.com/v1", "models": []interface{}{map[string]interface{}{"id": "web", "kind": "search"}}},
			wantSub: "kind",
		},
		{
			name: "oauth 且有 key 可写",
			raw:  map[string]interface{}{"api": OMPWireAnthropicMessages, "baseUrl": "https://example.com/v1", "auth": "oauth", "apiKey": "sk-placeholder"},
		},
		{
			name: "常规 provider 可写",
			raw:  map[string]interface{}{"api": OMPWireOpenAICompletions, "baseUrl": "https://example.com/v1", "models": []interface{}{map[string]interface{}{"id": "m"}}},
		},
		{
			name: "空 models 但无 modelOverrides 可写",
			raw:  map[string]interface{}{"api": OMPWireOpenAIResponses, "baseUrl": "https://example.com/v1", "models": []interface{}{}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := OMPReadOnlyReason(tc.raw)
			if tc.wantSub == "" {
				if got != "" {
					t.Fatalf("期望可写，得到只读原因 %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantSub) {
				t.Fatalf("只读原因 %q 不包含 %q", got, tc.wantSub)
			}
		})
	}

	// 别名 / merge 原因由 backend 传入，优先于其它判定。
	if got := ompReadOnlyReason(map[string]interface{}{"api": "bogus"}, "bogus", "该节点是 YAML 别名"); got != "该节点是 YAML 别名" {
		t.Fatalf("别名原因应直接返回，得到 %q", got)
	}
}

// TestOMPKindOf 锁住 OMPKindOf 的三返回值语义。
func TestOMPKindOf(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		raw      map[string]interface{}
		wantKind string
		wantRO   bool
		wantAPI  string
	}{
		{"provider 级 api", map[string]interface{}{"api": OMPWireOpenAICompletions}, "openai-compatible", false, OMPWireOpenAICompletions},
		{"未知 api 只读", map[string]interface{}{"api": "bogus"}, "", true, "bogus"},
		{"继承模型 api", map[string]interface{}{"models": []interface{}{map[string]interface{}{"id": "a", "api": OMPWireOpenAIResponses}}}, "responses", false, OMPWireOpenAIResponses},
		{"无 api", map[string]interface{}{"baseUrl": "https://example.com"}, "", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kind, readOnly, rawAPI := OMPKindOf(tc.raw)
			if kind != tc.wantKind || readOnly != tc.wantRO || rawAPI != tc.wantAPI {
				t.Fatalf("OMPKindOf = (%q,%v,%q)，期望 (%q,%v,%q)", kind, readOnly, rawAPI, tc.wantKind, tc.wantRO, tc.wantAPI)
			}
		})
	}
}

// TestOMPBuildCardEfforts 覆盖计划 §4.6 的读侧档位口径与模型字段映射。
func TestOMPBuildCardEfforts(t *testing.T) {
	t.Parallel()
	five := []string{"minimal", "low", "medium", "high", "xhigh"}
	four := []string{"minimal", "low", "medium", "high"}

	withOff := func(levels ...string) []string {
		return append([]string{ompOffVariant}, levels...)
	}

	cases := []struct {
		name           string
		raw            map[string]interface{}
		providerAPI    string
		providerCompat map[string]interface{}
		wantVariants   []string
		wantDefault    string
		wantLevels     []string
		wantReasoning  bool
	}{
		{
			name:          "anthropic 无 thinking 按五档推断",
			raw:           map[string]interface{}{"id": "claude-x", "reasoning": true},
			providerAPI:   OMPWireAnthropicMessages,
			wantVariants:  withOff(five...),
			wantDefault:   "high",
			wantLevels:    five,
			wantReasoning: true,
		},
		{
			name:           "completions 命中 compat 才给 xhigh",
			raw:            map[string]interface{}{"id": "m", "reasoning": true},
			providerAPI:    OMPWireOpenAICompletions,
			providerCompat: map[string]interface{}{"thinkingFormat": "openai", "supportsReasoningEffort": true},
			wantVariants:   withOff(five...),
			wantDefault:    "high",
			wantLevels:     five,
			wantReasoning:  true,
		},
		{
			name:          "completions 未命中 compat 止于 high",
			raw:           map[string]interface{}{"id": "m", "reasoning": true},
			providerAPI:   OMPWireOpenAICompletions,
			wantVariants:  withOff(four...),
			wantDefault:   "high",
			wantLevels:    four,
			wantReasoning: true,
		},
		{
			name: "显式 efforts 过滤 off/enabled/自定义并保持规范序",
			raw: map[string]interface{}{
				"id":        "m",
				"reasoning": true,
				"thinking":  map[string]interface{}{"efforts": []interface{}{"max", "off", "minimal", "enabled", "ultra"}, "defaultLevel": "max"},
			},
			wantVariants:  withOff("minimal", "max"),
			wantDefault:   "max",
			wantLevels:    []string{"minimal", "max"},
			wantReasoning: true,
		},
		{
			name: "defaultLevel 不在档位内回退 high",
			raw: map[string]interface{}{
				"id":        "m",
				"reasoning": true,
				"thinking":  map[string]interface{}{"efforts": []interface{}{"low", "medium"}, "defaultLevel": "max"},
			},
			wantVariants:  withOff("low", "medium"),
			wantDefault:   "low",
			wantLevels:    []string{"low", "medium"},
			wantReasoning: true,
		},
		{
			name:          "reasoning false 只给 off",
			raw:           map[string]interface{}{"id": "m"},
			wantVariants:  []string{ompOffVariant},
			wantDefault:   ompOffVariant,
			wantReasoning: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card := ompBuildCard(tc.raw, "p", "https://example.com/v1", tc.providerAPI, tc.providerCompat)
			if card.Reasoning != tc.wantReasoning {
				t.Fatalf("Reasoning = %v，期望 %v", card.Reasoning, tc.wantReasoning)
			}
			if !reflect.DeepEqual(card.Variants, tc.wantVariants) {
				t.Fatalf("Variants = %v，期望 %v", card.Variants, tc.wantVariants)
			}
			if card.DefaultVariant != tc.wantDefault {
				t.Fatalf("DefaultVariant = %q，期望 %q", card.DefaultVariant, tc.wantDefault)
			}
			if !reflect.DeepEqual(card.SupportedLevels, tc.wantLevels) {
				t.Fatalf("SupportedLevels = %v，期望 %v", card.SupportedLevels, tc.wantLevels)
			}
			if card.RawReasoningEnabled != nil && *card.RawReasoningEnabled != tc.wantReasoning {
				t.Fatalf("RawReasoningEnabled = %v，期望 %v", *card.RawReasoningEnabled, tc.wantReasoning)
			}
		})
	}
}

// TestOMPBuildCardFields 覆盖 context / output / 能力位 / headers 的读侧映射。
func TestOMPBuildCardFields(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"id":              "m",
		"name":            "显示名",
		"contextWindow":   200000,
		"maxTokens":       8192,
		"input":           []interface{}{"text", "image", "audio"},
		"supportsTools":   true,
		"headers":         map[string]interface{}{"x-a": "1", "x-b": 2},
		"reasoning":       false,
		"compactionModel": "other",
	}
	card := BuildOMPCard(raw, "p", "https://example.com/v1")
	if card.ModelID != "m" || card.Name != "显示名" {
		t.Fatalf("标识字段错误：%#v", card)
	}
	if card.Context != 200000 || card.Output != 8192 {
		t.Fatalf("context/output = %v/%v，期望 200000/8192", card.Context, card.Output)
	}
	if card.Capabilities == nil {
		t.Fatalf("capabilities 应包含文件明确写出的位")
	}
	if !reflect.DeepEqual(card.Capabilities.InputModalities, []string{"text", "image"}) {
		t.Fatalf("输入模态应裁成 text,image：%v", card.Capabilities.InputModalities)
	}
	if card.Capabilities.ToolCall == nil || !*card.Capabilities.ToolCall {
		t.Fatalf("supportsTools=true 应映射成 tool_call=true")
	}
	if !reflect.DeepEqual(card.Headers, map[string]string{"x-a": "1"}) {
		t.Fatalf("headers 只应保留字符串值：%v", card.Headers)
	}
	if _, ok := card.RawCfg["compactionModel"]; !ok {
		t.Fatalf("_raw_cfg 应保留模型节点原文")
	}
	if card.Vendor == "" {
		t.Fatalf("_vendor 必须填写")
	}

	// supportsTools=false 与未知都不写 capabilities（三态：未知不写）。
	card = BuildOMPCard(map[string]interface{}{"id": "m", "supportsTools": false}, "p", "")
	if card.Capabilities != nil {
		t.Fatalf("supportsTools=false 不应产出 capabilities：%#v", card.Capabilities)
	}
}

// ompFakeModels 是 OMPModelsSource 的测试实现。
type ompFakeModels struct {
	providers map[string]map[string]interface{}
}

func (f ompFakeModels) Providers() (map[string]map[string]interface{}, error) {
	return f.providers, nil
}
func (f ompFakeModels) ProviderIDs() []string {
	out := make([]string, 0, len(f.providers))
	for id := range f.providers {
		out = append(out, id)
	}
	return out
}

// TestOMPSelectors 覆盖 G6 的过滤与排序口径。
func TestOMPSelectors(t *testing.T) {
	t.Parallel()
	src := ompFakeModels{providers: map[string]map[string]interface{}{
		"alpha": {"api": OMPWireAnthropicMessages, "models": []interface{}{
			map[string]interface{}{"id": "claude-x"},
			map[string]interface{}{"id": "claude-y", "kind": "search"},
		}},
		"beta": {"models": []interface{}{
			map[string]interface{}{"id": "m2", "api": OMPWireOpenAICompletions},
			map[string]interface{}{"id": "m1", "api": OMPWireOpenAICompletions},
		}},
		"gamma": {"models": []interface{}{
			map[string]interface{}{"id": "m", "api": OMPWireOpenAICompletions},
			map[string]interface{}{"id": "n", "api": "google-generative-ai"},
		}},
		"delta": {"api": "google-generative-ai", "models": []interface{}{
			map[string]interface{}{"id": "g"},
		}},
		"epsilon": {"api": OMPWireOpenAIResponses, "models": []interface{}{
			map[string]interface{}{"id": "r", "api": "google-generative-ai"},
			map[string]interface{}{"id": "ok"},
		}},
		"zeta": {"api": OMPWireOpenAICompletions, "models": []interface{}{
			map[string]interface{}{"id": "z"},
			map[string]interface{}{"id": ""},
		}},
	}}
	want := []string{"alpha/claude-x", "beta/m1", "beta/m2", "epsilon/ok", "zeta/z"}
	if got := OMPSelectors(src); !reflect.DeepEqual(got, want) {
		t.Fatalf("OMPSelectors = %v，期望 %v", got, want)
	}
	if got := OMPSelectors(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil 源应返回空非 nil 切片，得到 %#v", got)
	}
}

// TestOMPReferenceGuard 覆盖 G11 的保守删除保护。
func TestOMPReferenceGuard(t *testing.T) {
	t.Parallel()
	refs := OMPReferences{
		Roles:   map[string]string{"default": "alpha/claude-x", "smol": "beta/*"},
		Presets: map[string]map[string]string{"fast": {"default": "*/m1"}},
		InFile:  map[string][]string{"alpha": {"claude-y"}, "beta": {"gamma/g1"}},
		Models:  map[string][]string{"alpha": {"claude-x", "claude-y"}, "beta": {"m1"}, "zeta": {"z"}},
	}
	cases := []struct {
		name       string
		providerID string
		modelID    string
		wantSub    string
	}{
		{name: "角色精确引用阻止删 provider", providerID: "alpha", wantSub: "modelRoles.default"},
		{name: "preset 通配符阻止删 provider", providerID: "beta", wantSub: "modelPresets.fast"},
		{name: "任意 provider 的通配符阻止删除", providerID: "delta", wantSub: "modelPresets.fast"},
		{name: "角色精确引用阻止删模型", providerID: "alpha", modelID: "claude-x", wantSub: "modelRoles.default"},
		{name: "preset 通配符阻止删模型", providerID: "beta", modelID: "m1", wantSub: "modelPresets.fast"},
		{name: "文件内部引用阻止删模型", providerID: "alpha", modelID: "claude-y", wantSub: "compactionModel"},
		{name: "无引用可删模型", providerID: "alpha", modelID: "claude-other"},
		{name: "无引用可删 provider", providerID: "zeta"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := OMPReferenceGuard(refs, tc.providerID, tc.modelID)
			if tc.wantSub == "" {
				if got != "" {
					t.Fatalf("期望可删除，得到 %q", got)
				}
				return
			}
			if !strings.Contains(got, tc.wantSub) {
				t.Fatalf("阻止原因 %q 不包含 %q", got, tc.wantSub)
			}
		})
	}

	// 无法静态判定的选择器：删除任何 provider / 模型都拒绝。
	for _, sel := range []string{"*", "@default"} {
		unknown := OMPReferences{Roles: map[string]string{"default": sel}}
		if got := OMPReferenceGuard(unknown, "any", ""); got == "" {
			t.Fatalf("选择器 %q 应阻止删除", sel)
		}
	}
	// provider/* 形式的角色引用单独覆盖（上一个 case 会先被 "*/m1" 命中）。
	providerWildcard := OMPReferences{Roles: map[string]string{"smol": "beta/*"}, Models: map[string][]string{"beta": {"m1"}}}
	if got := OMPReferenceGuard(providerWildcard, "beta", ""); !strings.Contains(got, "modelRoles.smol") {
		t.Fatalf("provider/* 应阻止删 provider，得到 %q", got)
	}
	// 跨 provider 的文件内引用不阻止删除。
	crossRefs := OMPReferences{InFile: map[string][]string{"beta": {"gamma/g1"}}}
	if got := OMPReferenceGuard(crossRefs, "beta", "g1"); got != "" {
		t.Fatalf("跨 provider 的引用不应阻止删除，得到 %q", got)
	}
}

// TestOMPParseSelector 覆盖选择器拆分与 :effort / @revision 处理。
func TestOMPParseSelector(t *testing.T) {
	t.Parallel()
	cases := []struct {
		sel        string
		wantProv   string
		wantModel  string
		wantParsed bool
	}{
		{"a/b", "a", "b", true},
		{"a/b:high", "a", "b", true},
		{"a/b:ultra", "a", "b:ultra", true},
		{"a/b@rev", "a", "b@rev", true},
		{"a/b/c", "a", "b/c", true},
		{"abc", "", "", false},
		{"a/", "", "", false},
		{"/b", "", "", false},
	}
	for _, tc := range cases {
		prov, model, ok := ompParseSelector(tc.sel)
		if ok != tc.wantParsed || prov != tc.wantProv || model != tc.wantModel {
			t.Fatalf("ompParseSelector(%q) = (%q,%q,%v)，期望 (%q,%q,%v)", tc.sel, prov, model, ok, tc.wantProv, tc.wantModel, tc.wantParsed)
		}
	}
}

// TestOMPAPIKeyKind 覆盖 G4 的三种 key 形态判定（绝不执行 command）。
func TestOMPAPIKeyKind(t *testing.T) {
	t.Setenv("APM_TEST_OMP_KEY_PLACEHOLDER", "value")
	cases := []struct {
		key  string
		want string
	}{
		{"", "none"},
		{"   ", "none"},
		{"!echo placeholder", "command"},
		{"APM_TEST_OMP_KEY_PLACEHOLDER", "env"},
		{"sk-placeholder", "literal"},
	}
	for _, tc := range cases {
		if got := ompAPIKeyKind(tc.key); got != tc.want {
			t.Fatalf("ompAPIKeyKind(%q) = %q，期望 %q", tc.key, got, tc.want)
		}
	}
}

// TestOMPAPIKeyRequired 覆盖 auth 口径表。
func TestOMPAPIKeyRequired(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{"": true, "apiKey": true, "none": false, "oauth": false}
	for auth, want := range cases {
		if got := ompAPIKeyRequired(auth); got != want {
			t.Fatalf("ompAPIKeyRequired(%q) = %v，期望 %v", auth, got, want)
		}
	}
}

// TestOMPProviderView 覆盖 provider 视图的派生字段。
func TestOMPProviderView(t *testing.T) {
	t.Parallel()
	raw := map[string]interface{}{
		"api":     OMPWireAnthropicMessages,
		"baseUrl": " https://example.com/v1 ",
		"apiKey":  "sk-placeholder",
		"models":  []interface{}{map[string]interface{}{"id": "a"}, map[string]interface{}{"id": "b"}},
	}
	view := ompReadProvider("p", raw, "")
	if view.ID != "p" || view.WireAPI != OMPWireAnthropicMessages || view.Kind != "anthropic" {
		t.Fatalf("协议字段错误：%#v", view)
	}
	if view.BaseURL != "https://example.com/v1" {
		t.Fatalf("BaseURL 应 trim，得到 %q", view.BaseURL)
	}
	if view.APIKeyKind != "literal" || !view.APIKeyRequired || view.ReadOnly {
		t.Fatalf("key/auth/只读字段错误：%#v", view)
	}
	if !reflect.DeepEqual(view.ModelIDs, []string{"a", "b"}) {
		t.Fatalf("ModelIDs = %v", view.ModelIDs)
	}
	if view.ReadOnlyReason != "" {
		t.Fatalf("可写 provider 不应带只读原因：%q", view.ReadOnlyReason)
	}

	ro := ompReadProvider("p", map[string]interface{}{"api": "bogus"}, "别名节点")
	if !ro.ReadOnly || ro.ReadOnlyReason != "别名节点" {
		t.Fatalf("只读标记或原因错误：%#v", ro)
	}
}

// TestOMPEffortOrderSingleSource 锁住思考档位词表的单源性。
//
// 档位写错一档会让整份 models.yml 解析失败（ompprovider.Validate 的硬校验），
// 所以词表只能有一份：单源是 internal/ompprovider.EffortLevels，core 只做拷贝。
// 如果这里失败，说明有人改了 ompprovider 的档位而没同步契约（或反之）。
func TestOMPEffortOrderSingleSource(t *testing.T) {
	t.Parallel()
	if !reflect.DeepEqual(ompEffortOrder, ompprovider.EffortLevels) {
		t.Fatalf("ompEffortOrder 必须与 ompprovider.EffortLevels 完全一致：%v vs %v",
			ompEffortOrder, ompprovider.EffortLevels)
	}
	want := []string{"minimal", "low", "medium", "high", "xhigh", "max"}
	if !reflect.DeepEqual(ompEffortOrder, want) {
		t.Fatalf("omp 思考档位必须是契约的六档：%v", ompEffortOrder)
	}
}
