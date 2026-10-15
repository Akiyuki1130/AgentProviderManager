package core

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// mustReCS 编译「全串锚定 + 大小写敏感」的正则，与 model_site_rules_gen.go 里 baseUrl /
// apiType 两个字段的生成形式一致（模型名那份带 (?i)，可复用 capability_test.go 的 mustRe）。
func mustReCS(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile("^(?:" + pattern + ")$")
	if err != nil {
		t.Fatalf("编译测试正则失败 %q：%v", pattern, err)
	}
	return re
}

// sameCapability 逐字段比较两个 ModelCapability。ModelCapability 含切片，不能用 ==。
func sameCapability(a, b ModelCapability) bool {
	return a.Context == b.Context &&
		a.Output == b.Output &&
		a.Vendor == b.Vendor &&
		a.Source == b.Source &&
		reflect.DeepEqual(a.ReasoningLevels, b.ReasoningLevels)
}

// TestParseCatalogEncoding 锁定 `ctx|out|levels` 紧凑编码的解析语义，重点是「畸形行返回
// ok==false、不 panic」和「档位段原样收集后交给 normalizeCapabilityLevels」。
func TestParseCatalogEncoding(t *testing.T) {
	cases := []struct {
		name       string
		enc        string
		wantOK     bool
		wantCtx    int
		wantOut    int
		wantLevels []string
	}{
		{"三段完整", "200000|32000|low,high,max", true, 200000, 32000, []string{"low", "high", "max"}},
		{"档位段为空", "1000000|384000|", true, 1000000, 384000, nil},
		{"ctx/out 皆为 0", "0|0|", true, 0, 0, nil},
		{"只有档位", "0|0|enabled", true, 0, 0, []string{"enabled"}},
		{"档位别名折成 off", "200000|32000|disabled,enabled", true, 200000, 32000, []string{"off", "enabled"}},
		// 档位按固定全集顺序排序，与来源顺序无关。
		{"档位排序", "200000|32000|max,low", true, 200000, 32000, []string{"low", "max"}},
		{"单元以内空白被忽略", " 200000 | 32000 | low ", true, 200000, 32000, []string{"low"}},
		{"未知档位被白名单滤掉", "200000|32000|huge", true, 200000, 32000, nil},
		{"空档位元素被丢弃", "200000|32000|low,,high", true, 200000, 32000, []string{"low", "high"}},
		{"段数不足", "200000|32000", false, 0, 0, nil},
		{"空串", "", false, 0, 0, nil},
		{"ctx 非数字", "abc|32000|low", false, 0, 0, nil},
		{"out 非数字", "200000|abc|low", false, 0, 0, nil},
		{"ctx 为负", "-1|32000|low", false, 0, 0, nil},
		{"out 为负", "200000|-5|low", false, 0, 0, nil},
	}
	for _, c := range cases {
		got, ok := parseCatalogEncoding(c.enc)
		if ok != c.wantOK {
			t.Errorf("%s：ok=%v，期望 %v", c.name, ok, c.wantOK)
			continue
		}
		if !ok {
			if !sameCapability(got, ModelCapability{}) {
				t.Errorf("%s：解析失败时必须是零值，实际 %+v", c.name, got)
			}
			continue
		}
		if got.Context != c.wantCtx || got.Output != c.wantOut {
			t.Errorf("%s：ctx/out=%d/%d，期望 %d/%d", c.name, got.Context, got.Output, c.wantCtx, c.wantOut)
		}
		if !reflect.DeepEqual(got.ReasoningLevels, c.wantLevels) {
			t.Errorf("%s：levels=%v，期望 %v", c.name, got.ReasoningLevels, c.wantLevels)
		}
		if got.Source != catalogSource {
			t.Errorf("%s：Source=%q，期望 %q", c.name, got.Source, catalogSource)
		}
		if got.Vendor != "" {
			t.Errorf("%s：离线目录不应提供厂商，Vendor=%q", c.name, got.Vendor)
		}
	}
}

// TestBuildCatalogTableSkipsMalformedEntries 合成表里混入畸形行时，只跳过畸形行，
// 其余照常解析，绝不 panic。
func TestBuildCatalogTableSkipsMalformedEntries(t *testing.T) {
	src := map[string]string{
		"good-a": "200000|32000|low,high",
		"good-b": "0|0|",
		"bad-1":  "",
		"bad-2":  "200000",
		"bad-3":  "x|y|z",
	}
	got := buildCatalogTable(src)
	if len(got) != 2 {
		t.Fatalf("解析出 %d 条，期望 2 条（畸形行必须被跳过）：%+v", len(got), got)
	}
	if !sameCapability(got["good-a"], ModelCapability{
		Context: 200000, Output: 32000, ReasoningLevels: []string{"low", "high"}, Source: catalogSource,
	}) {
		t.Errorf("good-a 解析结果不符：%+v", got["good-a"])
	}
	if !sameCapability(got["good-b"], ModelCapability{Source: catalogSource}) {
		t.Errorf("good-b 解析结果不符：%+v", got["good-b"])
	}
}

// TestLookupCatalogExactMiss 未命中（含空 id）必须返回零值：Source 空串、三字段皆零。
func TestLookupCatalogExactMiss(t *testing.T) {
	for _, id := range []string{"", "   ", "\t", "apm-definitely-not-a-real-model-xyz"} {
		got := lookupCatalogExact(id)
		if !sameCapability(got, ModelCapability{}) {
			t.Errorf("%q：未命中应为零值，实际 %+v", id, got)
		}
	}
}

// TestLookupCatalogExactGeneratedTable 对生成物做自检，不写死任何具体模型：
//   - 所有键都是小写（查询按 ToLower 精确命中）；
//   - 每条编码都能解析（生成物不该有畸形行）；
//   - 用键本身查询能得到与直接解析一致的结果，大写形式同样命中。
func TestLookupCatalogExactGeneratedTable(t *testing.T) {
	if len(catalogEntries) == 0 {
		t.Fatal("catalogEntries 为空：生成物没有内容")
	}
	if table := buildCatalogTable(catalogEntries); len(table) != len(catalogEntries) {
		t.Fatalf("有 %d 条编码无法解析（生成物含畸形行）", len(catalogEntries)-len(table))
	}
	for id, enc := range catalogEntries {
		if id != strings.ToLower(id) {
			t.Errorf("键 %q 不是小写：生成物必须按小写精确 id 建表", id)
		}
		want, ok := parseCatalogEncoding(enc)
		if !ok {
			t.Errorf("%q: 编码 %q 无法解析", id, enc)
			continue
		}
		got := lookupCatalogExact(id)
		if !sameCapability(got, want) {
			t.Errorf("%q: 查询结果 %+v 与直接解析 %+v 不一致", id, got, want)
		}
		if up := lookupCatalogExact(strings.ToUpper(id)); !sameCapability(up, want) {
			t.Errorf("%q: 大写查询未命中或结果不同：%+v", id, up)
		}
	}
}

// TestNormalizeBaseURLForRuleMatch 锁定 baseUrl 规范化：去尾斜杠、丢弃非法 URL、
// host/scheme 小写、保留 search/hash。
func TestNormalizeBaseURLForRuleMatch(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"not-a-url", ""},                                  // JS 的 new URL 会抛错
		{"api.deepseek.com", ""},                           // 缺 scheme
		{"https://api.deepseek.com", "https://api.deepseek.com"},
		{"https://api.deepseek.com/", "https://api.deepseek.com"},
		{"https://api.deepseek.com///", "https://api.deepseek.com"},
		{"https://api.anthropic.com/v1/", "https://api.anthropic.com/v1"},
		{"https://API.Anthropic.COM/v1", "https://api.anthropic.com/v1"},
		{"https://openrouter.ai/api/v1?x=1", "https://openrouter.ai/api/v1?x=1"},
	}
	for _, c := range cases {
		if got := normalizeBaseURLForRuleMatch(c.in); got != c.want {
			t.Errorf("normalizeBaseURLForRuleMatch(%q)=%q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestMatchSiteRulesSyntheticOverlay 用合成规则验证匹配与覆盖语义：
// 三正则同时命中、apiType 门、顺序扫描 + 逐字段覆盖。
func TestMatchSiteRulesSyntheticOverlay(t *testing.T) {
	// baseUrl 与 apiType 是大小写敏感的（与生成物一致），模型名才带 (?i)。
	base := mustReCS(t, `https://api\.example\.com/v1`)
	rules := []siteRule{
		// 只给 Context：后面命中的规则若没提到 Context，这个值必须保留。
		{Model: mustRe(t, `glm-5\.3`), BaseURL: base, Context: 200000},
		// 只给 Output。
		{Model: mustRe(t, `glm-5\.3`), BaseURL: base, Output: 64000},
		// 覆盖 Context，并给出档位。
		{Model: mustRe(t, `glm-5\.3`), BaseURL: base, Context: 1000000, Levels: []string{"low", "high", "max"}},
		// 模型名不命中：整条被跳过。
		{Model: mustRe(t, `kimi-k3`), BaseURL: base, Context: 1, Output: 1, Levels: []string{"off"}},
		// baseUrl 不命中：整条被跳过。
		{Model: mustRe(t, `glm-5\.3`), BaseURL: mustReCS(t, `https://other\.example\.com`), Output: 7},
	}
	got := matchSiteRulesWith("glm-5.3", "https://api.example.com/v1", "openai-chat-completions", rules)
	want := ModelCapability{Context: 1000000, Output: 64000, ReasoningLevels: []string{"low", "high", "max"}, Source: siteSource}
	if !sameCapability(got, want) {
		t.Errorf("覆盖结果 %+v，期望 %+v", got, want)
	}

	// 尾部斜杠经规范化后仍应命中。
	if got := matchSiteRulesWith("glm-5.3", "https://api.example.com/v1/", "openai-chat-completions", rules); !sameCapability(got, want) {
		t.Errorf("带尾斜杠的 baseUrl 结果 %+v，期望 %+v", got, want)
	}

	// 模型名大小写不敏感（生成物里的正则带 (?i)）。
	if got := matchSiteRulesWith("GLM-5.3", "https://api.example.com/v1", "openai-chat-completions", rules); !sameCapability(got, want) {
		t.Errorf("大写模型名结果 %+v，期望 %+v", got, want)
	}

	// baseUrl 大小写敏感：host 之外的部分大小写不同就不命中。
	if got := matchSiteRulesWith("glm-5.3", "https://api.example.com/V1", "openai-chat-completions", rules); !sameCapability(got, ModelCapability{}) {
		t.Errorf("路径大小写不同不应命中，实际 %+v", got)
	}

	// baseUrl 为空 / 非法：直接返回零值，不匹配任何规则。
	for _, bad := range []string{"", "   ", "not-a-url", "api.example.com"} {
		if got := matchSiteRulesWith("glm-5.3", bad, "openai-chat-completions", rules); !sameCapability(got, ModelCapability{}) {
			t.Errorf("baseUrl=%q 应返回零值，实际 %+v", bad, got)
		}
	}
}

// TestMatchSiteRulesAPITypeGate apiTypeMatch 是可选门（顺序扫描 + 后者覆盖）：
//   - 规则写了 apiTypeMatch 时，apiType 必须非空且命中，否则整条规则跳过；
//   - 规则没写（nil）时不限定 apiType。
func TestMatchSiteRulesAPITypeGate(t *testing.T) {
	base := mustReCS(t, `https://api\.example\.com/v1`)
	rules := []siteRule{
		// 不限定 apiType 的条目在前。
		{Model: mustRe(t, `.*`), BaseURL: base, Context: 222},
		// 限定 anthropic-messages 的条目在后：命中时按「后者覆盖」盖掉前一条的 Context。
		{Model: mustRe(t, `.*`), BaseURL: base, APIType: mustReCS(t, `anthropic-messages`), Context: 111},
	}
	// apiType 命中：门打开，限定条目生效并覆盖兜底条目。
	if got := matchSiteRulesWith("any-model", "https://api.example.com/v1", "anthropic-messages", rules); got.Context != 111 || got.Source != siteSource {
		t.Errorf("apiType 命中时 Context=%d（期望 111）、Source=%q（期望 %q）", got.Context, got.Source, siteSource)
	}
	// apiType 不命中：门关闭，只剩不限定那条。
	if got := matchSiteRulesWith("any-model", "https://api.example.com/v1", "openai-compatible", rules); got.Context != 222 || got.Source != siteSource {
		t.Errorf("apiType 不命中时 Context=%d（期望 222）、Source=%q（期望 %q）", got.Context, got.Source, siteSource)
	}
	// apiType 为空：限定 apiType 的那条必须跳过。
	if got := matchSiteRulesWith("any-model", "https://api.example.com/v1", "", rules); got.Context != 222 {
		t.Errorf("apiType 为空时 Context=%d，期望 222（限定 apiType 的规则应被跳过）", got.Context)
	}
}

// TestMatchSiteRulesAPITypeGateBlocks 单独锁定「门确实挡得住」：表里只有一条限定
// anthropic-messages 的规则时，传任何别的 apiType（含空串与纯空白）都必须完全不命中。
func TestMatchSiteRulesAPITypeGateBlocks(t *testing.T) {
	gated := []siteRule{{
		Model:   mustRe(t, `.*`),
		BaseURL: mustReCS(t, `https://api\.example\.com/v1`),
		APIType: mustReCS(t, `anthropic-messages`),
		Context: 111,
		Output:  222,
		Levels:  []string{"high"},
	}}
	for _, apiType := range []string{"openai-chat-completions", "openai-responses", "", "   "} {
		if got := matchSiteRulesWith("any-model", "https://api.example.com/v1", apiType, gated); !sameCapability(got, ModelCapability{}) {
			t.Errorf("apiType=%q：限定 apiType 的规则不该命中，实际 %+v", apiType, got)
		}
	}
	// 对照组：同一张表在 apiType 命中时必须给出全部字段，证明上面的未命中来自门而不是规则本身。
	got := matchSiteRulesWith("any-model", "https://api.example.com/v1", "anthropic-messages", gated)
	want := ModelCapability{Context: 111, Output: 222, ReasoningLevels: []string{"high"}, Source: siteSource}
	if !sameCapability(got, want) {
		t.Errorf("apiType 命中时 %+v，期望 %+v（对照组的失败说明门之外的匹配也有问题）", got, want)
	}
}

// TestMatchSiteRulesRealTable 用实测值锁定真实站点规则表（ZCode 内建 providerSiteRules，
// revision 31）的匹配结果。期望值来自对 ZCode 内部分配器 resolve 的逐条仿真。
func TestMatchSiteRulesRealTable(t *testing.T) {
	cases := []struct {
		name       string
		modelID    string
		baseURL    string
		apiType    string
		wantCtx    int
		wantOut    int
		wantLevels []string
		wantHit    bool
	}{
		{
			name: "opencode 网关命中", modelID: "glm-5.3-flash",
			baseURL: "https://opencode.ai/zen/go/v1", apiType: "openai-chat-completions",
			wantCtx: 1000000, wantOut: 131072, wantLevels: []string{"low", "high", "max"}, wantHit: true,
		},
		{
			name: "模型名大小写不敏感", modelID: "GLM-5.3-Flash",
			baseURL: "https://opencode.ai/zen/go/v1", apiType: "openai-chat-completions",
			wantCtx: 1000000, wantOut: 131072, wantLevels: []string{"low", "high", "max"}, wantHit: true,
		},
		{
			name: "baseUrl 尾斜杠被规范化", modelID: "glm-5.3-flash",
			baseURL: "https://opencode.ai/zen/go/v1/", apiType: "openai-chat-completions",
			wantCtx: 1000000, wantOut: 131072, wantLevels: []string{"low", "high", "max"}, wantHit: true,
		},
		{
			name: "host 大小写不敏感", modelID: "glm-5.3-flash",
			baseURL: "https://OPENCODE.AI/zen/go/v1", apiType: "openai-chat-completions",
			wantCtx: 1000000, wantOut: 131072, wantLevels: []string{"low", "high", "max"}, wantHit: true,
		},
		{
			name: "apiType 不同则不命中", modelID: "glm-5.3-flash",
			baseURL: "https://opencode.ai/zen/go/v1", apiType: "anthropic-messages",
			wantHit: false,
		},
		{
			name: "apiType 为空则不命中", modelID: "glm-5.3-flash",
			baseURL: "https://opencode.ai/zen/go/v1", apiType: "",
			wantHit: false,
		},
		{
			name: "baseUrl 为空则不命中", modelID: "glm-5.3-flash",
			baseURL: "", apiType: "openai-chat-completions",
			wantHit: false,
		},
		{
			name: "档位别名 disabled 折成 off", modelID: "minimax-m3",
			baseURL: "https://opencode.ai/zen/go/v1", apiType: "anthropic-messages",
			wantCtx: 1000000, wantOut: 131072, wantLevels: []string{"off", "enabled"}, wantHit: true,
		},
		{
			// 这条规则只给了 reasoningLevel.map，没给 values/contextWindow，
			// 所以「命中但能力字段全空」是正确结果。
			name: "命中但无能力字段", modelID: "deepseek-v4-pro",
			baseURL: "https://api.deepseek.com", apiType: "openai-responses",
			wantHit: true,
		},
	}
	for _, c := range cases {
		got := matchSiteRules(c.modelID, c.baseURL, c.apiType)
		if !c.wantHit {
			if !sameCapability(got, ModelCapability{}) {
				t.Errorf("%s：应未命中（零值），实际 %+v", c.name, got)
			}
			continue
		}
		if got.Source != siteSource {
			t.Errorf("%s：Source=%q，期望 %q", c.name, got.Source, siteSource)
			continue
		}
		if got.Context != c.wantCtx || got.Output != c.wantOut {
			t.Errorf("%s：ctx/out=%d/%d，期望 %d/%d", c.name, got.Context, got.Output, c.wantCtx, c.wantOut)
		}
		if !reflect.DeepEqual(got.ReasoningLevels, c.wantLevels) {
			t.Errorf("%s：levels=%v，期望 %v", c.name, got.ReasoningLevels, c.wantLevels)
		}
	}
}

// TestBuildCatalogTableHonorsCustomVariantsPolicy 锁定「目录解析对『允许自定义档位』策略敏感」。
// 这正是 catalogTable 必须以策略为缓存键的原因：档位规范化走 NormalizeVariants，而它读
// CustomVariantsAllowed()；若只按 sync.Once 缓存一次，目录层的档位口径就取决于首次查询
// 发生在用户切换该开关之前还是之后，与每次查询现规范化的 override / site 两层不一致。
func TestBuildCatalogTableHonorsCustomVariantsPolicy(t *testing.T) {
	t.Cleanup(func() { SetCustomVariantsAllowed(false) })
	src := map[string]string{"x-synthetic-model": "100000|2000|ultra-9,high"}

	SetCustomVariantsAllowed(false)
	if got := buildCatalogTable(src)["x-synthetic-model"]; !reflect.DeepEqual(got.ReasoningLevels, []string{"high"}) {
		t.Fatalf("白名单关闭时白名单外档位应被滤掉，实际 %v", got.ReasoningLevels)
	}

	SetCustomVariantsAllowed(true)
	got := buildCatalogTable(src)["x-synthetic-model"]
	if len(got.ReasoningLevels) != 2 || got.ReasoningLevels[0] != "high" || got.ReasoningLevels[1] != "ultra-9" {
		t.Fatalf("白名单打开时应保留自填档位且未知档位排最后，实际 %v", got.ReasoningLevels)
	}
}

// TestCatalogTableFollowsPolicyChange 锁住缓存的行为契约：开关切换后 catalogTable() 的结果
// 必须与「按新策略重建」一致（即缓存确实随策略重建），并且真实生成表里的档位 token 全部
// 落在 ZCodeVariantSet 内——后者成立时，今天缓存是否重建都不改变取值；将来快照若引入
// 白名单外拼写，这条会先红，提示需要复核目录层与其它层的口径。
func TestCatalogTableFollowsPolicyChange(t *testing.T) {
	t.Cleanup(func() { SetCustomVariantsAllowed(false) })

	SetCustomVariantsAllowed(true)
	rebuilt := buildCatalogTable(catalogEntries)
	if len(rebuilt) != len(catalogEntries) {
		t.Fatalf("真实生成表不该有不可解析的行：编码 %d 条，解析出 %d 条", len(catalogEntries), len(rebuilt))
	}
	for id, cap := range rebuilt {
		for _, lv := range cap.ReasoningLevels {
			if _, ok := ZCodeVariantSet[lv]; !ok {
				t.Errorf("生成表条目 %s 的档位 %q 不在 ZCodeVariantSet 内", id, lv)
			}
		}
	}
	if !reflect.DeepEqual(catalogTable(), rebuilt) {
		t.Errorf("策略切换后 catalogTable() 未按新策略重建")
	}
}
