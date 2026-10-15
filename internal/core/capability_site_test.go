package core

import (
	"reflect"
	"testing"
)

// 本文件锁定「站点上下文分层求值」（LookupModelCapabilityForSite）与离线目录的去命名空间
// 容错查询（lookupCatalogTolerant）。
//
// 所有期望值都先查过生成表 / 覆盖层 / presets 表再写死（capability_catalog_gen.go、
// model_site_rules_gen.go、model_overrides.go、presets.go），**不编造数值**；
// 涉及真实表的用例都在开头显式校验前提，前提一旦失效立刻 Fatal，避免用例退化成空断言。
// 全部使用占位 ID 与占位 URL，不读任何用户配置。

// TestLookupCatalogTolerantNamespaceStrip 去前缀容错：
// 精确层未命中且 id 含 "/" 时，允许对最后一个 "/" 之后的一段再查一次；
// 只在第二次命中时采用，否则返回零值（fail-closed，绝不猜）。
func TestLookupCatalogTolerantNamespaceStrip(t *testing.T) {
	// 前提一：整串（小写后）不是离线目录里的键，因此只能靠「去前缀」命中。
	if got := lookupCatalogExact("qwen/qwen3.8-max"); !sameCapability(got, ModelCapability{}) {
		t.Fatalf("前提破坏：qwen/qwen3.8-max 已在离线目录里：%+v", got)
	}
	// 前提二：最后一段是目录里的真实键（实测 1000000|131072|low,medium,xhigh）。
	want := lookupCatalogExact("qwen3.8-max")
	if want.Source != catalogSource {
		t.Fatalf("前提破坏：qwen3.8-max 不在离线目录里：%+v", want)
	}
	if want.Context != 1000000 || want.Output != 131072 || !reflect.DeepEqual(want.ReasoningLevels, []string{"low", "medium", "xhigh"}) {
		t.Fatalf("前提破坏：qwen3.8-max 的目录条目变了：%+v", want)
	}
	got := lookupCatalogTolerant("Qwen/Qwen3.8-Max")
	if !sameCapability(got, want) {
		t.Errorf("lookupCatalogTolerant(Qwen/Qwen3.8-Max)=%+v，期望 %+v", got, want)
	}

	// fail-closed：最后一段也不在目录里时必须返回零值。
	for _, id := range []string{
		"",
		"   ",
		"acme/",
		"Acme/Definitely-Not-A-Real-Model-Xyz",
		"NoSlash-Definitely-Not-A-Real-Model-Xyz",
	} {
		if got := lookupCatalogTolerant(id); !sameCapability(got, ModelCapability{}) {
			t.Errorf("lookupCatalogTolerant(%q)=%+v，期望零值", id, got)
		}
	}
}

// TestLookupModelCapabilityForSiteTolerantCatalog 端到端：带命名空间的 id 经「去前缀」命中离线目录。
// 选 acme/hy4-preview 是因为目录是它的**唯一**来源（内建正则 / 覆盖层 / presets 都读不出），
// 否则目录只是填空缺，断言就没有区分力。顺带锁住「目录层已不再依赖 baseURL」。
func TestLookupModelCapabilityForSiteTolerantCatalog(t *testing.T) {
	const id = "acme/hy4-preview"
	// 前提一：整串不是目录键，去前缀后的 hy4-preview = 1024000|64000|off,high 才是。
	if got := lookupCatalogExact(id); !sameCapability(got, ModelCapability{}) {
		t.Fatalf("前提破坏：%s 已在离线目录里：%+v", id, got)
	}
	if got := lookupCatalogTolerant(id); got.Context != 1024000 || got.Output != 64000 {
		t.Fatalf("前提破坏：hy4-preview 的目录条目变了：%+v", got)
	}
	// 前提二：内建正则与 presets 对这个 id 都读不出值。
	if raw := mergeCatalog(id, modelCatalogRules); raw.Context != 0 || raw.Output != 0 || raw.ReasoningLevels != nil {
		t.Fatalf("前提破坏：%s 已能被内建正则命中：%+v", id, raw)
	}
	if p, ok := LookupModelLimits(id); ok && (p.Context > 0 || p.Output > 0) {
		t.Fatalf("前提破坏：%s 已能被 presets 命中：%+v", id, p)
	}
	want := ModelCapability{
		Context:         1024000,
		Output:          64000,
		ReasoningLevels: []string{"off", "high"},
		Vendor:          "unknown", // 目录不含厂商，detectVendor 也认不出这个 id
		Source:          catalogSource,
	}
	// 目录层无条件参与：有没有 baseURL / apiType 都拿到同一结果。
	for _, c := range []struct {
		baseURL string
		apiType string
	}{
		{"", ""},
		{"", "anthropic-messages"},
		{"https://example.invalid/v1", ""},
		{"https://example.invalid/v1", "anthropic-messages"},
	} {
		got := LookupModelCapabilityForSite(id, c.baseURL, c.apiType)
		if !sameCapability(got, want) {
			t.Errorf("baseURL=%q apiType=%q：got %+v，期望 %+v", c.baseURL, c.apiType, got, want)
		}
	}
	// 无站点上下文时与 LookupModelCapability 一致。
	if got := LookupModelCapabilityForSite(id, "", ""); !sameCapability(got, lookupCapability(id, modelOverrides)) {
		t.Errorf("带/不带站点入口应一致：%+v", got)
	}
}

// TestLookupModelCapabilityForSiteEmptyBaseURLMatchesLegacy baseURL 为空时 site 层必然不命中；
// 其余各层（含无条件参与的离线目录）走的是同一段实现，因此结果与 LookupModelCapability
// 逐字段（含 Source）一致。
func TestLookupModelCapabilityForSiteEmptyBaseURLMatchesLegacy(t *testing.T) {
	ids := []string{
		"glm-5.3",
		"kimi-k2.7-code",
		"minimax-m2.7",
		"gpt-5.6-luna",
		"deepseek-v4.1-flash", // 覆盖层
		"qwen3-max",
		"my-local-model", // 无任何来源
		"Qwen/Qwen3.8-Max",
		"x/mimo-v2.5",
		"acme/hy4-preview", // 只靠离线目录（含去前缀）
	}
	for _, id := range ids {
		want, ok := LookupModelCapability(id)
		if !ok {
			t.Fatalf("%s：期望 ok==true", id)
		}
		for _, apiType := range []string{"", "anthropic-messages", "openai-chat-completions"} {
			got := LookupModelCapabilityForSite(id, "", apiType)
			if !sameCapability(got, want) {
				t.Errorf("%s（apiType=%q）：got %+v，期望与 LookupModelCapability 一致 %+v", id, apiType, got, want)
			}
		}
	}
	// 空 modelID：与 LookupModelCapability 的 false 分支完全一致（含 Source="preset"）。
	want, ok := LookupModelCapability("   ")
	if ok {
		t.Fatal("前提破坏：空 modelID 应返回 ok==false")
	}
	if got := LookupModelCapabilityForSite("   ", "", ""); !sameCapability(got, want) {
		t.Errorf("空 modelID：got %+v，期望 %+v", got, want)
	}
}

// TestLookupModelCapabilityForSiteRealTables 用实测值锁定真实分层的合成结果。
// 每条都注明是哪一层的值胜出。
func TestLookupModelCapabilityForSiteRealTables(t *testing.T) {
	const opencode = "https://opencode.ai/zen/go/v1"
	cases := []struct {
		name    string
		modelID string
		baseURL string
		apiType string
		want    ModelCapability
	}{
		{
			// 站点层补出一个档位空洞：内建正则（MiniMax-M2 家族）与离线目录（204800|131072|）
			// 都没有档位，opencode + anthropic-messages 的站点规则补出 ["enabled"]。
			// Source 仍记最高贡献者 zcode。
			name:    "站点层补出一个档位空洞",
			modelID: "minimax-m2.7",
			baseURL: opencode,
			apiType: "anthropic-messages",
			want: ModelCapability{
				Context: 204800, Output: 32000, ReasoningLevels: []string{"enabled"},
				Vendor: "minimax", Source: "zcode",
			},
		},
		{
			// zcode 胜 catalog：gpt-5.6 家族内建是 1050000/128000，社区目录写的是
			// 272000/128000（离群值），必须保留内建值——这正是审计要求改动顺序的原因。
			name:    "zcode 胜 catalog（gpt-5.6 家族）",
			modelID: "gpt-5.6-luna",
			baseURL: opencode,
			apiType: "openai-responses",
			want: ModelCapability{
				Context: 1050000, Output: 128000,
				ReasoningLevels: []string{"off", "low", "medium", "high", "xhigh", "max"},
				Vendor:          "openai", Source: "zcode",
			},
		},
		{
			// zcode 胜 catalog：内建 kimi-k2.7-code = 262144|98304|[enabled]，
			// 目录 = 262144|262144|（Output 与档位都不同），内建值必须保留。
			name:    "zcode 胜 catalog（kimi-k2.7-code）",
			modelID: "kimi-k2.7-code",
			baseURL: "https://example.invalid/v1",
			apiType: "",
			want: ModelCapability{
				Context: 262144, Output: 98304, ReasoningLevels: []string{"enabled"},
				Vendor: "moonshot", Source: "zcode",
			},
		},
		{
			// presets 在前：grok-3 内建正则与覆盖层都读不出值，presets 旧表给出 131000/32000
			// 并胜出；目录（131072|131072|）只补 presets 没给的字段（此处两列都有值，无补充）。
			name:    "presets 胜 catalog（grok-3）",
			modelID: "grok-3",
			baseURL: "https://example.invalid/v1",
			apiType: "",
			want: ModelCapability{
				Context: 131000, Output: 32000,
				Vendor: "xai", Source: "preset",
			},
		},
		{
			// override 胜 catalog：覆盖层 deepseek-v4.1-flash 的档位含 off（目录只到 low）。
			name:    "override 胜 catalog",
			modelID: "deepseek-v4.1-flash",
			baseURL: "https://example.invalid/v1",
			apiType: "",
			want: ModelCapability{
				Context: 1000000, Output: 384000,
				ReasoningLevels: []string{"off", "low", "high", "max"},
				Vendor:          "deepseek", Source: "override",
			},
		},
	}
	for _, c := range cases {
		got := LookupModelCapabilityForSite(c.modelID, c.baseURL, c.apiType)
		if !sameCapability(got, c.want) {
			t.Errorf("%s：got %+v，期望 %+v", c.name, got, c.want)
		}
	}
}

// TestLookupModelCapabilityForSiteBaseURLNoMatchIsInert baseURL 不命中任何站点规则时，
// 站点层不贡献任何字段：换一个同样不命中的 baseURL（或 apiType）结果逐字段相同；
// 换成一个命中的 baseURL 时则立刻多出站点规则给的档位。
func TestLookupModelCapabilityForSiteBaseURLNoMatchIsInert(t *testing.T) {
	inert := LookupModelCapabilityForSite("minimax-m2.7", "https://example.invalid/v1", "anthropic-messages")
	if inert.ReasoningLevels != nil {
		t.Errorf("未命中的 baseURL 不应带来档位：%+v", inert)
	}
	if other := LookupModelCapabilityForSite("minimax-m2.7", "https://other.invalid/v2", ""); !sameCapability(inert, other) {
		t.Errorf("两个都不命中的 baseURL 结果应一致：%+v vs %+v", inert, other)
	}
	hit := LookupModelCapabilityForSite("minimax-m2.7", "https://opencode.ai/zen/go/v1", "anthropic-messages")
	if !reflect.DeepEqual(hit.ReasoningLevels, []string{"enabled"}) {
		t.Errorf("命中的 baseURL 应拿到站点规则档位，实际 %v", hit.ReasoningLevels)
	}
}

// TestLookupCapabilityForSiteOverrideBeatsCatalog 注入式验证「override 胜 catalog」：
// 目录里 glm-5.1 是 200000|131072|（无档位），注入的覆盖层必须全字段胜出。
func TestLookupCapabilityForSiteOverrideBeatsCatalog(t *testing.T) {
	// 前提：目录条目的数值与档位（无档位）。
	cat := lookupCatalogExact("glm-5.1")
	if cat.Context != 200000 || cat.Output != 131072 || cat.ReasoningLevels != nil {
		t.Fatalf("前提破坏：glm-5.1 的目录条目变了：%+v", cat)
	}
	overrides := []modelOverride{
		{Match: "glm-5.1", Context: 123456, Output: 654321, Levels: []string{"medium"}},
	}
	got := lookupCapabilityForSite("glm-5.1", "https://example.invalid/v1", "", overrides, nil)
	want := ModelCapability{Context: 123456, Output: 654321, ReasoningLevels: []string{"medium"}, Vendor: "zhipu", Source: "override"}
	if !sameCapability(got, want) {
		t.Errorf("got %+v，期望 %+v", got, want)
	}
}

// TestLookupCapabilityForSiteBuiltinBeatsCommunityCatalog 锁住本次顺序调整的核心动机：
// 社区离线目录**不得**改写 ZCode 内建正则已经给出的值。用例取审计实测到的离群证据
// （目录值与内建值差异很大，若顺序反了会立刻暴露）。
func TestLookupCapabilityForSiteBuiltinBeatsCommunityCatalog(t *testing.T) {
	cases := []struct {
		name       string
		modelID    string
		wantCtx    int
		wantOut    int
		wantLevels []string
		catCtx     int // 目录里的值，必须与内建值不同（否则用例没有区分力）
	}{
		{
			// 内建 1050000 vs 目录 272000（gpt-5.4 / gpt-5.6 / gpt-6 家族都是这个离群值）。
			name: "gpt-5.4 家族", modelID: "gpt-5.4",
			wantCtx: 1050000, wantOut: 128000,
			wantLevels: []string{"off", "low", "medium", "high", "xhigh"},
			catCtx:     272000,
		},
		{
			// 内建 1000000 vs 目录 1048576（zai-org 命名空间条目）。
			name: "zai-org/GLM-5.3 命名空间条目", modelID: "zai-org/GLM-5.3",
			wantCtx: 1000000, wantOut: 128000,
			wantLevels: []string{"low", "high", "max"},
			catCtx:     1048576,
		},
		{
			// 内建 262144/98304 vs 目录 262000/262000（moonshotai 命名空间条目）。
			name: "moonshotai/Kimi-K2.5 命名空间条目", modelID: "moonshotai/Kimi-K2.5",
			wantCtx: 262144, wantOut: 98304,
			wantLevels: []string{"off", "enabled"},
			catCtx:     262000,
		},
	}
	for _, c := range cases {
		// 前提一：离线目录（含去前缀）确实给出**不同**的 ctx，说明目录真的被查到了。
		if cat := lookupCatalogTolerant(c.modelID); cat.Context != c.catCtx {
			t.Fatalf("%s：前提破坏，目录 ctx=%d，期望 %d（%+v）", c.name, cat.Context, c.catCtx, cat)
		}
		// 前提二：内建正则单独求值就是期望值（目录不允许参与）。
		if raw := mergeCatalog(c.modelID, modelCatalogRules); raw.Context != c.wantCtx || raw.Output != c.wantOut {
			t.Fatalf("%s：前提破坏，内建正则 ctx/out=%d/%d，期望 %d/%d", c.name, raw.Context, raw.Output, c.wantCtx, c.wantOut)
		}
		want := ModelCapability{
			Context:         c.wantCtx,
			Output:          c.wantOut,
			ReasoningLevels: c.wantLevels,
			Source:          "zcode",
		}
		// 覆盖层与站点层都不能给出值，因此 Source 归内建正则；厂商由 detectVendor 独立判定。
		want.Vendor = detectVendor(c.modelID)
		got := LookupModelCapabilityForSite(c.modelID, "https://example.invalid/v1", "")
		if !sameCapability(got, want) {
			t.Errorf("%s：got %+v，期望 %+v（内建值不得被社区目录改写）", c.name, got, want)
		}
	}
}

// TestLookupCapabilityForSiteZcodeBeatsSite 内建正则胜站点规则：注入同模型的站点规则
// （三个字段都给），结果必须一个字段都不被站点层改写；再用同形状的规则证明站点层
// 在「前面几层全空」时确实会生效（区分「被压制」与「实现缺失」）。
func TestLookupCapabilityForSiteZcodeBeatsSite(t *testing.T) {
	const id = "glm-5.3-flash-zzz"
	// 前提：该 id 不在离线目录里（无 "/" 也不走去前缀路径），因此只有覆盖层 / 内建正则 /
	// 站点层会给值。presets 只兜底 ctx/out 两列，而内建正则已经给出 ctx/out（除非该字段为
	// 0，presets 不会覆盖已有值），所以它不影响本用例的期望值。
	if got := lookupCatalogTolerant(id); !sameCapability(got, ModelCapability{}) {
		t.Fatalf("前提破坏：%s 已被离线目录命中：%+v", id, got)
	}
	zcodeOnly, _ := LookupModelCapability(id)
	want := ModelCapability{Context: 1000000, Output: 128000, ReasoningLevels: []string{"low", "high", "max"}, Vendor: "zhipu", Source: "zcode"}
	if !sameCapability(zcodeOnly, want) {
		t.Fatalf("前提破坏：%s 的内建正则值变了：%+v", id, zcodeOnly)
	}

	hostRe := mustReCS(t, `https://api\.example\.com/v1`)
	sameModel := []siteRule{{
		Model:   mustRe(t, `glm-5\.3-flash-zzz`),
		BaseURL: hostRe,
		Context: 111,
		Output:  222,
		Levels:  []string{"medium"},
	}}
	if got := lookupCapabilityForSite(id, "https://api.example.com/v1", "", modelOverrides, sameModel); !sameCapability(got, want) {
		t.Errorf("内建正则应胜站点规则：got %+v，期望 %+v", got, want)
	}

	// 正向对照：内容相同的规则，换成前面几层全空的 id 时全部字段都由站点层给出。
	anyModel := []siteRule{{
		Model:   mustRe(t, `.*`),
		BaseURL: hostRe,
		Context: 111,
		Output:  222,
		Levels:  []string{"medium"},
	}}
	got := lookupCapabilityForSite("zzz-local-model", "https://api.example.com/v1", "", modelOverrides, anyModel)
	wantSite := ModelCapability{Context: 111, Output: 222, ReasoningLevels: []string{"medium"}, Vendor: "unknown", Source: siteSource}
	if !sameCapability(got, wantSite) {
		t.Errorf("站点层未被采用：got %+v，期望 %+v", got, wantSite)
	}
}

// TestLookupCapabilityForSiteBeatsPresets 站点层胜 presets 旧表：presets 里 4.0ultra 是
// 32000/32000，站点层给出的值必须胜出；baseURL 为空时不参与，回落 presets。
func TestLookupCapabilityForSiteBeatsPresets(t *testing.T) {
	const id = "4.0ultra"
	// 前提：presets 命中，且离线目录与内建正则都不管这个 id。
	p, ok := LookupModelLimits(id)
	if !ok || p.Context != 32000 || p.Output != 32000 {
		t.Fatalf("前提破坏：presets 里 %s 变了：%+v ok=%v", id, p, ok)
	}
	if got := lookupCatalogTolerant(id); !sameCapability(got, ModelCapability{}) {
		t.Fatalf("前提破坏：%s 已被离线目录命中：%+v", id, got)
	}
	rules := []siteRule{{
		Model:   mustRe(t, `.*`),
		BaseURL: mustReCS(t, `https://api\.example\.com/v1`),
		Context: 111111,
		Output:  222222,
	}}

	off := lookupCapabilityForSite(id, "", "", modelOverrides, rules)
	if off.Context != 32000 || off.Output != 32000 || off.Source != "preset" {
		t.Errorf("baseURL 为空时应由 presets 兜底：%+v", off)
	}
	on := lookupCapabilityForSite(id, "https://api.example.com/v1", "", modelOverrides, rules)
	if on.Context != 111111 || on.Output != 222222 || on.Source != siteSource {
		t.Errorf("站点层应胜 presets：%+v", on)
	}
}
