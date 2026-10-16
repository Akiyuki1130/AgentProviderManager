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
//
// 离线目录引入后这条契约依然成立的前提是「该 id 确实不在任何来源里」，所以这里先显式锁住
// 这个前提：规则表与离线目录都读不出它。若将来有人在目录里加了同名条目，本用例会立刻
// 因为前提被破坏而失败，而不是悄悄把契约放宽。
func TestLookupModelCapabilityUnknown(t *testing.T) {
	const id = "my-local-model"
	if raw := mergeCatalog(id, modelCatalogRules); raw.Context != 0 || raw.Output != 0 || raw.ReasoningLevels != nil {
		t.Fatalf("前提破坏：%s 已能被内建正则命中：%+v", id, raw)
	}
	if cat := lookupCatalogTolerant(id); !sameCapability(cat, ModelCapability{}) {
		t.Fatalf("前提破坏：%s 已在离线目录里：%+v", id, cat)
	}
	cap, ok := LookupModelCapability(id)
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

// TestMergeCatalogSkipsCatchAllFeatures 纯兜底规则的 features 必须与 ctx/out/levels **一起**
// 被跳过。理由与那三列完全相同，而且在 features 上后果更重：内建 `.*` 规则填满了
// supportsToolCall:true 之类的位，照抄会让这些位占满每一个模型，后面几层的 features
// 永远轮不到补空缺。
func TestMergeCatalogSkipsCatchAllFeatures(t *testing.T) {
	rules := []catalogRule{
		{
			Pattern: ".*", Re: mustRe(t, `.*`),
			Context: 200000, Output: 32000, Levels: []string{"disabled", "enabled"},
			Features: "text|text|111111",
		},
	}
	got := mergeCatalog("model-with-no-family", rules)
	if got.Features.HasAny() {
		t.Errorf("纯兜底规则的 features 必须被跳过，实际 %s", describeFeatures(got.Features))
	}
	if got.Context != 0 || got.Output != 0 || got.ReasoningLevels != nil {
		t.Errorf("纯兜底规则的 ctx/out/levels 也必须被跳过，实际 %+v", got)
	}
	// 真实内建表上的同一条不变量：任何来源都读不出的 id 不得因为兜底规则拿到能力位。
	if real := mergeCatalog("my-local-model", modelCatalogRules); real.Features.HasAny() {
		t.Errorf("未知模型不得被内建兜底规则填上 features：%s", describeFeatures(real.Features))
	}
}

// TestMergeCatalogFeaturesLastMatchWins 锁定 features 在 zcode 层内的「后命中覆盖」：
// 与 ctx/out/levels 同构——后命中的规则只覆盖它真正给出的列，先前的列保持不动；
// 模态段为空表示该规则未给出模态，不得把已有的模态清空。
func TestMergeCatalogFeaturesLastMatchWins(t *testing.T) {
	rules := []catalogRule{
		// 家族规则：给出输入模态 + 工具调用位。
		{Pattern: `model-feat.*`, Re: mustRe(t, `model-feat.*`), Features: "text,image|text|1-----"},
		// 更细规则：只给输出模态与联网位（工具调用仍为真），输入模态段留空 = 未给出。
		{Pattern: `.*narrow`, Re: mustRe(t, `.*narrow`), Features: "|audio|1-1---"},
	}
	got := mergeCatalog("model-feat-narrow", rules)
	want := ModelFeatures{
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"audio"},
		ToolCall:         featureBool(true),
		NativeWebSearch:  featureBool(true),
	}
	if !sameFeatures(got.Features, want) {
		t.Errorf("mergeCatalog features=%s，期望 %s", describeFeatures(got.Features), describeFeatures(want))
	}

	// 反空断言：若后命中规则的「空模态段」被误当成清空，上面的期望就会拿不到输入模态。
	if len(got.Features.InputModalities) == 0 {
		t.Fatal("后命中规则的未给出列把先前的输入模态清空了")
	}
}

// TestMergeCatalogFeaturesMalformedEncodingIgnored 规则里的 features 编码畸形时必须按
// 「该规则未给出」处理（fail-closed），而不是把零值或半解析结果合并进去。
func TestMergeCatalogFeaturesMalformedEncodingIgnored(t *testing.T) {
	for _, enc := range []string{"text|text", "text|text|1----", "", "text|text|1-1---|extra"} {
		rules := []catalogRule{
			{Pattern: `model-ok.*`, Re: mustRe(t, `model-ok.*`), Features: "text|text|1-----"},
			{Pattern: `.*bad`, Re: mustRe(t, `.*bad`), Features: enc},
		}
		got := mergeCatalog("model-ok-bad", rules)
		want := ModelFeatures{InputModalities: []string{"text"}, OutputModalities: []string{"text"}, ToolCall: featureBool(true)}
		if !sameFeatures(got.Features, want) {
			t.Errorf("编码 %q（畸形）：got %s，期望 %s", enc, describeFeatures(got.Features), describeFeatures(want))
		}
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

// TestLookupCapabilityOverrideFeaturesMarkSource 覆盖层只给 features 时也必须标 override
// ——features 与 ctx/out/levels 一样算「贡献了能力字段」；反过来，只给 Vendor 的条目
// 不贡献任何能力位，不得把后面几层贡献的 Source 误标成 override。
func TestLookupCapabilityOverrideFeaturesMarkSource(t *testing.T) {
	// 只给 features：ctx/out 全部来自内建正则（glm-5.3 = 1000000/128000），Source 仍记 override。
	ovFeatures := []modelOverride{
		{Match: "glm-5.3", Features: ModelFeatures{StructuredOutput: featureBool(true)}},
	}
	got := lookupCapability("glm-5.3", ovFeatures)
	if got.Context != 1000000 || got.Output != 128000 {
		t.Errorf("override 未给出的 ctx/out 应由内建正则补齐：%+v", got)
	}
	if got.Source != "override" {
		t.Errorf("Source=%q，期望 override（只给 features 也算贡献）", got.Source)
	}
	if got.Features.StructuredOutput == nil || !*got.Features.StructuredOutput {
		t.Errorf("override 给的 features 未生效：%s", describeFeatures(got.Features))
	}

	// 只给 Vendor：不贡献能力字段，Source 必须仍是内建正则。为了让「不得误标 override」有
	// 区分力，这里注入一条 catalog 记录让它真的给出 features——Source 仍不能变成 override。
	ovVendor := []modelOverride{{Match: "glm-5.3", Vendor: "zhipu"}}
	withFeatureTable(t, map[string]string{"glm-5.3": "text|text|1-1---"})
	plain := lookupCapability("glm-5.3", ovVendor)
	if plain.Source != "zcode" {
		t.Errorf("Source=%q，期望 zcode（override 只给 Vendor，未贡献能力字段）", plain.Source)
	}
	if !plain.Features.HasAny() {
		t.Fatal("前提破坏：注入的 catalog features 未生效")
	}
	if plain.Features.NativeWebSearch == nil || !*plain.Features.NativeWebSearch {
		t.Errorf("catalog 的 features 应逐字段补空：%s", describeFeatures(plain.Features))
	}
}

// TestLookupCapabilityFeaturesFollowsCtxOutLayering 锁定「features 与 ctx/out 走同一套分层」：
// override 先给即胜出、zcode 正则逐字段补空、catalog 只能补最后剩下的空缺且永不改写已在的位。
//
// zcode 层在 lookupCapabilityForSite 内部固定用**真实内建表**，没有可注入变体，因此这里取一个
// 真实 id（内建表对它同时给出 ctx/out 与能力位），期望值全部由 mergeCatalog 现场观察得到，
// 不写死任何数值：生成物重裁后只要前提还成立用例就继续有效，前提失效会立刻以「前提破坏」失败。
// baseURL 为空且 rules 传 nil ⇒ 站点层整体关闭，断言只反映 override / zcode / catalog 三层。
func TestLookupCapabilityFeaturesFollowsCtxOutLayering(t *testing.T) {
	const id = "glm-5.3-flash"
	// 前提：内建正则表 modelCatalogRules 对该 id 给出 ctx/out（用来验证 features 与它们同层序）
	// 且至少给出一个能力位。改内建表或重裁生成物时若这条不再成立，下面的断言会失去区分力。
	zcode := mergeCatalog(id, modelCatalogRules)
	if zcode.Context == 0 || zcode.Output == 0 || !zcode.Features.HasAny() {
		t.Fatalf("前提破坏：内建正则表 modelCatalogRules 对 %s 要求「ctx>0 且 out>0 且至少一个能力位」，实际 ctx=%d out=%d features=%s",
			id, zcode.Context, zcode.Output, describeFeatures(zcode.Features))
	}

	// 情形一：override 给出**全部 8 列**，catalog 给出完全不同的写法 —— override 必须全胜；
	// 它没有给的 ctx/out 仍由 zcode 补齐（features 与 ctx/out 同层序，谁先给就归谁）。
	overrideAll := ModelFeatures{
		InputModalities:  []string{"text", "audio"},
		OutputModalities: []string{"audio"},
		ToolCall:         featureBool(true),
		StructuredOutput: featureBool(true),
		NativeWebSearch:  featureBool(true),
		MidConvSystem:    featureBool(true),
		Attachment:       featureBool(true),
		Temperature:      featureBool(true),
	}
	overrides := []modelOverride{{Match: id, Features: overrideAll}}
	// catalog 层：整个词表的超集 + 六位全真，用来验证它一位都改不动 override 给的值。
	withFeatureTable(t, map[string]string{id: "text,image,video,audio,pdf|text,audio,image,video,pdf|111111"})

	got := lookupCapabilityForSite(id, "", "", overrides, nil)
	if got.Context != zcode.Context || got.Output != zcode.Output {
		t.Errorf("ctx/out=%d/%d，期望 %d/%d（override 未给，应由 zcode 补齐）",
			got.Context, got.Output, zcode.Context, zcode.Output)
	}
	if !sameFeatures(got.Features, overrideAll) {
		t.Errorf("override 全给时 features=%s，期望 %s", describeFeatures(got.Features), describeFeatures(overrideAll))
	}
	if got.Source != "override" {
		t.Errorf("Source=%q，期望 override", got.Source)
	}

	// 情形二：没有 override（站点层关闭）——zcode 已给出的列必须原样保留，catalog 的超集不得并入；
	// zcode 没给出的列才由 catalog 补上。逐列按「第一个给出值的层」取期望，同样不写死具体值。
	got = lookupCapabilityForSite(id, "", "", nil, nil)
	catFeatures := lookupCatalogFeatures(id)
	if !catFeatures.HasAny() {
		t.Fatal("前提破坏：注入的 catalog features 未生效")
	}
	for _, name := range featureFieldNames {
		wantVal, wantKnown := featureFieldValue(zcode.Features, name)
		wantLayer := "zcode"
		if !wantKnown {
			wantVal, wantKnown = featureFieldValue(catFeatures, name)
			wantLayer = "catalog"
		}
		gotVal, gotKnown := featureFieldValue(got.Features, name)
		if wantKnown != gotKnown || (wantKnown && wantVal != gotVal) {
			t.Errorf("%s 得到 %q（known=%v），期望来自 %s 的 %q（known=%v）",
				name, gotVal, gotKnown, wantLayer, wantVal, wantKnown)
		}
	}
	// 直接证据：zcode 的输入模态不得被 catalog 的超集并掉（catalog 只能补空）。
	if !reflect.DeepEqual(got.Features.InputModalities, zcode.Features.InputModalities) {
		t.Errorf("输入模态=%v，期望保留 zcode 的 %v（catalog 只能补空，不得做并集）",
			got.Features.InputModalities, zcode.Features.InputModalities)
	}
	if got.Source != "zcode" {
		t.Errorf("Source=%q，期望 zcode（features 与 ctx 都由内建正则给出）", got.Source)
	}
}

// TestLookupCapabilityOverrideFeaturesNormalized 覆盖层的模态列在并入前必须归一到规范词表：
// 未知词丢弃、乱序与重复自动排序去重。同时锁住两条既有语义不受归一影响：
// 「只给 features 的条目也标 override」与「override 没给的 ctx/out 仍由后续层补齐、不被覆盖」。
func TestLookupCapabilityOverrideFeaturesNormalized(t *testing.T) {
	const id = "glm-5.3"
	// 前提一：内建正则（model_catalog_gen.go 的 modelCatalogRules）给出 ctx/out，且它给出的
	// 输入模态与下面断言的 override 归一结果**不同**——这样「断言通过」本身就证明了这批模态
	// 来自 override 层（mergeFeatures 是前层胜出，内建层给的位不会被后层改写；反之若 override
	// 被忽略，结果里就会留下内建层的 text）。
	zcode := mergeCatalog(id, modelCatalogRules)
	if zcode.Context == 0 || zcode.Output == 0 || !zcode.Features.HasAny() {
		t.Fatalf("前提破坏：%s 的内建正则应给出 ctx/out 与能力位，实际 ctx=%d out=%d features=%s",
			id, zcode.Context, zcode.Output, describeFeatures(zcode.Features))
	}
	if reflect.DeepEqual(zcode.Features.InputModalities, []string{"text", "image", "video", "pdf"}) {
		t.Fatalf("前提破坏：%s 的内建输入模态（%v）与目标集合相同，断言失去区分力",
			id, zcode.Features.InputModalities)
	}
	// 前提二：把离线功能表临时清空，隔离出 override 层。真实表（capability_features_gen.go）
	// 是否收录了这个 id 会随快照重裁变化，因此不做「表里没有它」这种会漂移的前提，直接注入空表。
	withFeatureTable(t, map[string]string{})

	// 情形一：乱序 + 重复 + 别名 + 未知词混在一起，覆盖层必须自行把它们收进规范口径。
	overrides := []modelOverride{{
		Match: id,
		Features: ModelFeatures{
			InputModalities:  []string{"PDF", "image_url", "telepathy", "  text ", "Text", "video"},
			OutputModalities: []string{"pdf", "audio", "hologram", "audio", "  IMAGE "},
			ToolCall:         featureBool(true),
		},
	}}
	got := lookupCapabilityForSite(id, "", "", overrides, nil)
	if want := []string{"text", "image", "video", "pdf"}; !reflect.DeepEqual(got.Features.InputModalities, want) {
		t.Errorf("输入模态=%v，期望 %v（未知词丢弃、别名折叠、按输入规范序排序去重）",
			got.Features.InputModalities, want)
	}
	if want := []string{"audio", "image", "pdf"}; !reflect.DeepEqual(got.Features.OutputModalities, want) {
		t.Errorf("输出模态=%v，期望 %v（输出词表顺序是 text,audio,image,video,pdf）",
			got.Features.OutputModalities, want)
	}
	if got.Features.ToolCall == nil || !*got.Features.ToolCall {
		t.Errorf("布尔三态不应被模态归一影响：%s", describeFeatures(got.Features))
	}
	// 只给 features 的条目：Source 记 override；ctx/out 仍由内建正则给出，绝不被这条 override 覆盖。
	if got.Source != "override" {
		t.Errorf("Source=%q，期望 override（只给 features 也算贡献）", got.Source)
	}
	if got.Context != zcode.Context || got.Output != zcode.Output {
		t.Errorf("ctx/out=%d/%d，期望沿用内建正则的 %d/%d（override 只给 features，不得动它们）",
			got.Context, got.Output, zcode.Context, zcode.Output)
	}

	// 情形二（fail-closed）：条目只写了未知词 ⇒ 归一后整条为空，与「未给出」等价：
	// 不带入任何能力位，也不得据此把 Source 标成 override。
	// 内建层本身有能力位（见前提一），因此「等价」要与**完全没有 override** 的基线逐字段比较。
	ovUnknown := []modelOverride{{Match: id, Features: ModelFeatures{InputModalities: []string{"telepathy"}}}}
	plain := lookupCapabilityForSite(id, "", "", ovUnknown, nil)
	if plain.Source != "zcode" {
		t.Errorf("Source=%q，期望 zcode（override 归一后为空，未贡献任何能力字段）", plain.Source)
	}
	if baseline := lookupCapabilityForSite(id, "", "", nil, nil); !sameFeatures(plain.Features, baseline.Features) {
		t.Errorf("只写未知词的条目必须与「未给出」逐字段等价：实际 %s，期望 %s",
			describeFeatures(plain.Features), describeFeatures(baseline.Features))
	}
}

// TestLookupCapabilityFeaturesCatalogOnlyMarksCatalog catalog 只给出 features（ctx/out/档位
// 全空）时，既要拿到这些位，也要把 Source 记成 catalog——features 是能力字段的一部分。
func TestLookupCapabilityFeaturesCatalogOnlyMarksCatalog(t *testing.T) {
	const id = "feat-probe-catalog-only"
	// 前提：真实各层都读不出这个合成 id。内建正则那一层必须连档位也为空——只要它给出任何一列，
	// Source 就会被 zcode 先占，本用例「只靠 catalog 贡献」的前提即不成立。
	if got := mergeCatalog(id, modelCatalogRules); got.Context != 0 || got.Output != 0 ||
		got.ReasoningLevels != nil || got.Features.HasAny() {
		t.Fatalf("前提破坏：内建正则表 modelCatalogRules 已命中合成 id %s（要求 ctx/out/档位/features 全空），实际 %+v / %s",
			id, got, describeFeatures(got.Features))
	}
	// presets 旧表同样不得命中：它会给 ctx/out 并把 Source 占成 preset。
	if p, ok := LookupModelLimits(id); ok {
		t.Fatalf("前提破坏：presets 表已命中合成 id %s（要求完全未命中），实际 %+v", id, p)
	}
	withFeatureTable(t, map[string]string{id: "text|text|1-1---"})

	got := lookupCapabilityForSite(id, "", "", modelOverrides, nil)
	if got.Context != 0 || got.Output != 0 || got.ReasoningLevels != nil {
		t.Errorf("合成 id 的 ctx/out/档位应为空：%+v", got)
	}
	want := ModelFeatures{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"text"},
		ToolCall:         featureBool(true),
		NativeWebSearch:  featureBool(true),
		Source:           catalogSource,
	}
	if !sameFeatures(got.Features, want) {
		t.Errorf("features=%s，期望 %s", describeFeatures(got.Features), describeFeatures(want))
	}
	if got.Source != catalogSource {
		t.Errorf("Source=%q，期望 %q（只有 features 也算 catalog 贡献）", got.Source, catalogSource)
	}
}

// TestLookupCapabilitySiteFeaturesFillsGaps 站点层的 features 与 ctx/out 同构：
// 只补前面各层留下的空位（override 层已给出的位不得被它改写），且它自己排在家族正则之前
// ——本用例的合成 id 不命中内建正则，因此这里观察到的正是「override 首层 + 站点层补空」。
func TestLookupCapabilitySiteFeaturesFillsGaps(t *testing.T) {
	const id = "feat-probe-site-gap"
	rules := []siteRule{{
		Model:    mustRe(t, `feat-probe-site-gap`),
		BaseURL:  mustReCS(t, `https://feat\.example\.com/v1`),
		Features: "text,video|audio|--1---",
	}}
	// 前提：内建正则表 modelCatalogRules 对这个合成 id 一无所出（只剩 override / site 两层，
	// 否则「站点层补空」的断言会被内建值先填满而失去区分力）。
	if raw := mergeCatalog(id, modelCatalogRules); raw.Context != 0 || raw.Output != 0 ||
		raw.ReasoningLevels != nil || raw.Features.HasAny() {
		t.Fatalf("前提破坏：内建正则表 modelCatalogRules 已命中合成 id %s（要求 ctx/out/档位/features 全空），实际 %+v / %s",
			id, raw, describeFeatures(raw.Features))
	}
	// 覆盖层只给输入模态，站点层应补出输出模态与联网位，并在输入模态上被压制。
	overrides := []modelOverride{{
		Match:    id,
		Features: ModelFeatures{InputModalities: []string{"text"}, ToolCall: featureBool(true)},
	}}
	got := lookupCapabilityForSite(id, "https://feat.example.com/v1", "", overrides, rules)
	// in 取 override 的 [text]（站点层的 [text,video] 不得并入）；out 只有站点层给（[audio]）；
	// tool 由 override 给出，站点层的 flags 不得改写；web 只有站点层给。
	want := ModelFeatures{
		InputModalities:  []string{"text"},
		OutputModalities: []string{"audio"},
		ToolCall:         featureBool(true),
		NativeWebSearch:  featureBool(true),
	}
	if !sameFeatures(got.Features, want) {
		t.Errorf("features=%s，期望 %s", describeFeatures(got.Features), describeFeatures(want))
	}
	if got.Source != "override" {
		t.Errorf("Source=%q，期望 override", got.Source)
	}

	// 站点层的两道门没变：baseURL 不是合法绝对 URL 时整层不命中，features 也一起不生效。
	off := lookupCapabilityForSite(id, "not-a-url", "", overrides, rules)
	if len(off.Features.OutputModalities) != 0 || off.Features.NativeWebSearch != nil {
		t.Errorf("baseURL 非法时站点层不应贡献 features：%s", describeFeatures(off.Features))
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
// 正则层应为空；关键是**不得**被误填成兜底规则的 200000。
//
// 本次顺序调整后的新事实：presets 旧表排在离线目录（catalog）之前，而 presets 里
// deepseek-chat = 128000|8000，因此 Output 取 presets 的 8000（目录写的 8192 不得改写它），
// Context 两处一致（128000），补全来源记 presets。
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
		t.Errorf("deepseek-chat 应由 presets 兜底得到 128000/8000，实际 ctx=%d out=%d", cap.Context, cap.Output)
	}
	if cap.Source != "preset" {
		t.Errorf("Source=%q，期望 %q", cap.Source, "preset")
	}
	if len(cap.ReasoningLevels) != 0 {
		t.Errorf("deepseek-chat 不应有档位，实际 %v", cap.ReasoningLevels)
	}
}

// TestModelOverridesAreUsable 覆盖层初始条目（若存在）必须自洽可用：值非零且厂商已知。
func TestModelOverridesAreUsable(t *testing.T) {
	for _, ov := range modelOverrides {
		if strings.TrimSpace(ov.Match) == "" {
			t.Errorf("覆盖层条目缺 Match：%+v", ov)
		}
		// features 与 ctx/out/levels/vendor 一样算「给出了字段」，因此只给 features 的条目也是有效条目。
		if ov.Context == 0 && ov.Output == 0 && len(ov.Levels) == 0 && ov.Vendor == "" && !ov.Features.HasAny() {
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
