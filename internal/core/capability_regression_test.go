package core

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

// 本文件把审查用的一次性探针固化为**常驻契约**。
//
// 分层求值的顺序（见 lookupCapabilityForSite）是
//
//	override → site 站点规则 → zcode 内建正则 → presets 旧表 → catalog 离线目录
//
// 其中 presets 与 catalog 都是「兜底层」，它们的**相对顺序**决定了社区快照能否静默改写
// 已经发布出去的取值（实测社区数据存在 gpt-4 上下文 128000→8192、
// gemini-2.5-flash-tts 1048576→8192 这类明显错误的条目，若排在 presets 之前会改写
// 270 个字段 / 180 个模型）。因此这里对
//
//	modelPresets 的全部键 ∪ catalogEntries 的全部键 ∪ modelOverrides 的 Match ∪ zcode 正则样本 id
//
// 逐个 id 断言一条不变量：**旧两层已经给出的字段，在新五层里逐字段完全相同**。
//
// 口径（严格按审查约定）：
//   - 旧两层在测试里现场构造，直接调用既有函数，不复制它们的实现逻辑：
//     zcode 层 = mergeCatalog(id, modelCatalogRules)、presets 层 = LookupModelLimits(id)；
//   - 新五层走两个公开入口：LookupModelCapabilityForSite（带站点上下文）与
//     LookupModelCapability（无站点上下文），并断言两者在 ctx/out/档位上一致；
//   - 站点层用「不会命中任何真实站点规则、但非空」的 baseURL + apiType 关闭，让两个入口
//     的差异只剩站点层，从而既验证站点层惰性、又不干扰本不变量；
//   - 旧层为空而新层有值 = 允许的新增（只统计计数并 t.Logf，不失败）；
//   - 旧层有值而新层不同 = 回归，失败并打印 id / 字段 / 旧值 / 新值。
//
// 只读既有表，不引入任何测试数据文件。

// 站点层的「惰性」上下文：非空且是合法绝对 URL（否则站点层整层不命中，就测不出惰性），
// 但 model_site_rules_gen.go 里的规则只匹配 opencode.ai / openrouter.ai / api.deepseek.com，
// 因此对探测 id 一律不命中（用例开头有显式前提校验）。
const (
	regressionProbeBaseURL = "https://example.invalid/v1"
	regressionProbeAPIType = "openai-chat-completions"
)

// zcodeRegexSampleIDs 是覆盖「旧两层里的 zcode 正则层」的样本 id：它们要么不在 presets /
// 目录的键集里，要么是大小写混合 / 带日期后缀的变体，用来确保 zcode 层给出的 ctx/out/档位
// 在键集之外的 id 上同样不被兜底层改写。
var zcodeRegexSampleIDs = []string{
	"GLM-5.2",
	"MiniMax-M2.5",
	"gpt-5.6-luna-2026-01-01",
	"gpt-5.4",
	"deepseek-v4-flash-0731",
	"zai-org/GLM-5.3",
	"moonshotai/Kimi-K2.5",
	"o3-mini",
	"claude-opus-4-6-2026-01-20",
	"gemini-3.6-flash-001",
	"my-local-model", // 任何来源都读不出：用来锁住「不得凭空新增字段」
}

// TestNewLayersNeverChangePreviouslyKnownFields 在上述全键集上逐 id 固化「零回归」契约。
func TestNewLayersNeverChangePreviouslyKnownFields(t *testing.T) {
	// 前提：探测用的 baseURL 不被任何真实站点规则命中，否则站点层会给新层加字段，
	// 本用例的「旧层字段不变」就不再只反映 presets/catalog 的顺序。
	probe := normalizeBaseURLForRuleMatch(regressionProbeBaseURL)
	if probe == "" {
		t.Fatalf("前提破坏：探测 baseURL %q 不是合法绝对 URL", regressionProbeBaseURL)
	}
	for i, r := range modelSiteRules {
		if r.BaseURL != nil && r.BaseURL.MatchString(probe) {
			t.Fatalf("前提破坏：第 %d 条站点规则命中探测 baseURL %s", i, probe)
		}
	}
	// 前提：键集非空（表被清空时用例会退化成空断言）。
	if len(modelPresets) == 0 || len(catalogEntries) == 0 {
		t.Fatalf("前提破坏：presets=%d 条、catalog=%d 条，键集不该为空", len(modelPresets), len(catalogEntries))
	}

	ids := regressionProbeIDs(t)
	t.Logf("逐 id 校验 %d 个键（presets %d ∪ catalog %d ∪ override %d ∪ zcode 样本 %d）",
		len(ids), len(modelPresets), len(catalogEntries), len(modelOverrides), len(zcodeRegexSampleIDs))

	newFields := 0
	comparedFields := 0
	zcodeKnown := 0
	for _, id := range ids {
		// 旧两层：直接调用既有函数现场构造，不复制其逻辑。
		oldZcode := mergeCatalog(id, modelCatalogRules)
		oldPreset, oldPresetOK := LookupModelLimits(id)
		if oldZcode.Context > 0 || oldZcode.Output > 0 || oldZcode.ReasoningLevels != nil {
			zcodeKnown++
		}

		// 新五层：两个公开入口都必须给出同一份 ctx/out/档位（站点层已按前提惰性）。
		withSite := LookupModelCapabilityForSite(id, regressionProbeBaseURL, regressionProbeAPIType)
		plain, ok := LookupModelCapability(id)
		if !ok {
			t.Errorf("%s：非空 id 的新层入口应返回 ok==true", id)
			continue
		}
		if withSite.Context != plain.Context || withSite.Output != plain.Output ||
			!reflect.DeepEqual(withSite.ReasoningLevels, plain.ReasoningLevels) {
			t.Errorf("%s：带/不带站点上下文的能力字段应一致，withSite=%+v plain=%+v", id, withSite, plain)
			continue
		}

		// 逐字段比对：优先用 zcode 层的值（它在新旧两套里都排在 presets 之前），
		// zcode 层没给才回落到 presets 层的值。
		for _, f := range []regressionProbeField{
			{
				name:   "Context",
				oldVal: oldZcode.Context, newVal: plain.Context, oldHas: oldZcode.Context > 0,
				presetFn: func(p ModelPreset) int { return p.Context },
			},
			{
				name:   "Output",
				oldVal: oldZcode.Output, newVal: plain.Output, oldHas: oldZcode.Output > 0,
				presetFn: func(p ModelPreset) int { return p.Output },
			},
		} {
			if f.oldHas {
				comparedFields++
				if f.oldVal != f.newVal {
					t.Errorf("%s：%s 被新层改写（zcode 层旧值 %d → 新值 %d）", id, f.name, f.oldVal, f.newVal)
				}
				continue
			}
			if oldPresetOK && f.presetField(oldPreset) > 0 {
				comparedFields++
				if want := f.presetField(oldPreset); want != f.newVal {
					t.Errorf("%s：%s 被新层改写（preset 层旧值 %d → 新值 %d）", id, f.name, want, f.newVal)
				}
				continue
			}
			if f.newVal > 0 {
				newFields++
			}
		}

		if oldZcode.ReasoningLevels != nil {
			comparedFields++
			if !reflect.DeepEqual(oldZcode.ReasoningLevels, plain.ReasoningLevels) {
				t.Errorf("%s：ReasoningLevels 被新层改写（zcode 层旧值 %v → 新值 %v）",
					id, oldZcode.ReasoningLevels, plain.ReasoningLevels)
			}
		} else if plain.ReasoningLevels != nil {
			newFields++
		}
	}
	// 反空断言：必须真的比到了「旧层有值」的字段，否则本用例只是空转。
	if comparedFields == 0 {
		t.Fatal("前提破坏：没有任何 id 带有旧层已给出的字段，用例退化为空断言")
	}
	// 反空断言（第二处）：本用例的区分力主要来自「zcode 层与 presets 层都可能有值」这一交错，
	// 若内建正则表被清空或全部失效，只剩 presets 与 catalog 相比，用例仍会通过却抓不到
	// 「zcode 层整体消失」，因此这里要求 zcode 层确实贡献过值。
	if zcodeKnown == 0 {
		t.Fatal("前提破坏：没有任何 id 命中 ZCode 内建正则表，用例已失去对 zcode 层的覆盖")
	}
	t.Logf("旧两层已给出的字段（逐字段比对）：%d（其中 zcode 层贡献过值的 id：%d）", comparedFields, zcodeKnown)
	t.Logf("新增字段数（旧两层为空、新五层有值）：%d", newFields)
}

// featureLayer 是分层比对里「某一层给出的 features」。
type featureLayer struct {
	name     string
	features ModelFeatures
}

// featureFieldNames 是参与分层比对的 8 个能力列。Source 不参与：它不是能力位，只记录
// 「谁先贡献了能力值」。
var featureFieldNames = []string{
	"input_modalities", "output_modalities",
	"tool_call", "structured_output", "native_web_search",
	"mid_conv_system", "attachment", "temperature",
}

// knownBoolValue 把布尔三态折成「渲染值 + 是否已知」。nil 是未知；false 是**已知为假**，
// 因此它同样是「已给出」，后续层不得改写——这正是三态语义在分层里最容易出错的地方。
func knownBoolValue(p *bool) (string, bool) {
	if p == nil {
		return "", false
	}
	if *p {
		return "true", true
	}
	return "false", true
}

// featureFieldValue 取某一列的渲染值与「是否已知」；模态列折成逗号连接，空集合算未知。
func featureFieldValue(f ModelFeatures, name string) (string, bool) {
	switch name {
	case "input_modalities":
		if len(f.InputModalities) == 0 {
			return "", false
		}
		return strings.Join(f.InputModalities, ","), true
	case "output_modalities":
		if len(f.OutputModalities) == 0 {
			return "", false
		}
		return strings.Join(f.OutputModalities, ","), true
	case "tool_call":
		return knownBoolValue(f.ToolCall)
	case "structured_output":
		return knownBoolValue(f.StructuredOutput)
	case "native_web_search":
		return knownBoolValue(f.NativeWebSearch)
	case "mid_conv_system":
		return knownBoolValue(f.MidConvSystem)
	case "attachment":
		return knownBoolValue(f.Attachment)
	case "temperature":
		return knownBoolValue(f.Temperature)
	}
	return "", false
}

// TestFeaturesLayeringNeverErasesEarlierFields 是 features 版的「零回归」网：在**合成表**上逐字段
// 断言「更早层已经给出的能力位，绝不被更晚的层改写或清空」，并顺带锁住 override → site →
// zcode → catalog 的层序（presets 旧表没有 features 列，不参与）。
//
// 期望值不是把实现里的合并代码抄一遍，而是现场调用各层函数取回它们**各自**的提案，再按文档
// 层序取第一个「已知」的提案——与同文件既有的「旧两层现场构造」写法同源：被验证的是合并顺序，
// 不是各层自己的取值。三个场景合计有 21 个字段被某层给出过，其中 9 个是「后层也提了值而前层
// 已有值」的交锋位（实测值，见用例末尾的 t.Logf），因此反空断言保证了这张网不是空转。
//
// 注意 zcode 层是**真实内建表**（lookupCapabilityForSite 内部固定用它，没有可注入变体），
// 所以每个场景都先断言真实内建表对合成 id 一无所出；zcode 层与 catalog 层的交锋由
// capability_test.go 里基于真实 id 的用例覆盖。
func TestFeaturesLayeringNeverErasesEarlierFields(t *testing.T) {
	type scenario struct {
		name        string
		id          string
		baseURL     string
		overrides   []modelOverride
		siteRules   []siteRule
		table       map[string]string
		wantSource  string
		checkLimits bool // 该场景是否同时断言 ctx/out 的层序与 features 一致
		wantCtx     int
		wantOut     int
	}
	scenarios := []scenario{
		{
			// override / site / catalog 三层全都给出了能力位，且互相冲突——
			// 最容易暴露「后层改写前层」。zcode 层由真实表观察（见下方前提校验）。
			name:    "三层全部给出且冲突",
			id:      "reg-feat-alpha",
			baseURL: "https://feat.example.com/v1",
			overrides: []modelOverride{{
				Match:    "reg-feat-alpha",
				Features: ModelFeatures{InputModalities: []string{"text"}, ToolCall: featureBool(true)},
			}},
			siteRules: []siteRule{{
				Model:    mustRe(t, `reg-feat-alpha`),
				BaseURL:  mustReCS(t, `https://feat\.example\.com/v1`),
				Context:  300,
				Output:   400,
				Features: "pdf|pdf|111111",
			}},
			table:       map[string]string{"reg-feat-alpha": "pdf,video,audio|video|111111"},
			wantSource:  "override",
			checkLimits: true,
			wantCtx:     300, // override 未给 ⇒ zcode 也不给 ⇒ 由 site 层给出
			wantOut:     400,
		},
		{
			// 只有 catalog 层给值（zcode 规则不命中、站点规则不命中、没有 override）：
			// 既要拿到这些位，也要把 Source 记成 catalog。
			name:    "只有 catalog 给出",
			id:      "reg-feat-beta",
			baseURL: "https://feat.example.com/v1",
			siteRules: []siteRule{{
				Model:    mustRe(t, `reg-feat-zzz`),
				BaseURL:  mustReCS(t, `https://feat\.example\.com/v1`),
				Features: "text|audio|111111",
			}},
			table:      map[string]string{"reg-feat-beta": "text|text|111---"},
			wantSource: catalogSource,
		},
		{
			// 三态交锋：override 说「温度已知为假」，catalog 说「已知为真」。
			// false 是已知值，因此必须保持不变——这条最容易在「用零值代表未知」的实现里被写坏。
			name: "override 的已知为假不被 catalog 的已知为真覆盖",
			id:   "reg-feat-gamma",
			overrides: []modelOverride{{
				Match:    "reg-feat-gamma",
				Features: ModelFeatures{Temperature: featureBool(false)},
			}},
			table:      map[string]string{"reg-feat-gamma": "text|text|111111"},
			wantSource: "override",
		},
	}

	knownFields := 0
	contested := 0
	for _, sc := range scenarios {
		// 每次只注入本场景的合成表；t.Cleanup 会在用例结束时还原。
		withFeatureTable(t, sc.table)

		// zcode 层用**真实内建表**观察——lookupCapabilityForSite 内部固定用它，没有可注入变体。
		// 因此先要求真实内建表（modelCatalogRules）对这些合成 id 一无所出，否则场景不再受控
		// （zcode 会先填掉一些列，本用例「前层胜出」的结论就不再只反映我构造的四个场景）。
		zcodeObserved := mergeCatalog(sc.id, modelCatalogRules)
		if zcodeObserved.Context != 0 || zcodeObserved.Output != 0 ||
			zcodeObserved.ReasoningLevels != nil || zcodeObserved.Features.HasAny() {
			t.Fatalf("前提破坏：场景 %q（id=%s）要求内建正则表 modelCatalogRules 对 ctx/out/档位/features 全部一无所出，实际命中 %+v / %s",
				sc.name, sc.id, zcodeObserved, describeFeatures(zcodeObserved.Features))
		}

		got := lookupCapabilityForSite(sc.id, sc.baseURL, "", sc.overrides, sc.siteRules)

		// 现场取回各层**各自**的提案，按文档层序排列（override → site → zcode → catalog）。
		// 注意：override 层这里取的是条目原值，而实现会先过 normalizeOverrideFeatures（模态归一）。
		// 因此本用例的合成条目必须写规范词（现有条目都是），否则观测值与实现值会不必要地不一致。
		layers := make([]featureLayer, 0, 4)
		if ov, ok := matchOverride(sc.id, sc.overrides); ok {
			layers = append(layers, featureLayer{"override", ov.Features})
		}
		layers = append(layers,
			featureLayer{"site", matchSiteRulesWith(sc.id, sc.baseURL, "", sc.siteRules).Features},
			featureLayer{"zcode", zcodeObserved.Features},
			featureLayer{"catalog", lookupCatalogFeatures(sc.id)},
		)

		for _, name := range featureFieldNames {
			wantVal := ""
			wantLayer := ""
			proposers := 0
			for _, l := range layers {
				v, known := featureFieldValue(l.features, name)
				if !known {
					continue
				}
				proposers++
				if wantLayer == "" {
					wantVal, wantLayer = v, l.name
				}
			}
			gotVal, gotKnown := featureFieldValue(got.Features, name)
			if wantLayer == "" {
				// 没有任何层给出这一列 ⇒ 最终结果里也不该凭空出现。
				if gotKnown {
					t.Errorf("%s（%s）/%s：所有层都未给出，最终却出现 %q", sc.name, sc.id, name, gotVal)
				}
				continue
			}
			knownFields++
			if proposers > 1 {
				contested++
			}
			if !gotKnown {
				t.Errorf("%s（%s）/%s：被后层清空（首层 %s 给 %q，最终为未知）", sc.name, sc.id, name, wantLayer, wantVal)
				continue
			}
			if gotVal != wantVal {
				t.Errorf("%s（%s）/%s：被后层改写（首层 %s 给 %q，最终 %q）", sc.name, sc.id, name, wantLayer, wantVal, gotVal)
			}
		}

		if got.Source != sc.wantSource {
			t.Errorf("%s（%s）：Source=%q，期望 %q（%s）", sc.name, sc.id, got.Source, sc.wantSource, describeFeatures(got.Features))
		}
		if sc.checkLimits && (got.Context != sc.wantCtx || got.Output != sc.wantOut) {
			t.Errorf("%s（%s）：ctx/out=%d/%d，期望 %d/%d（features 与 ctx/out 必须同层序）",
				sc.name, sc.id, got.Context, got.Output, sc.wantCtx, sc.wantOut)
		}
	}

	// 反空断言：既要比到「层给出过值」的字段，也要比到「后层想改写前层」的交锋位，
	// 否则这条网只是空转。
	if knownFields == 0 {
		t.Fatal("前提破坏：合成场景里没有任何层给出能力位，用例退化为空断言")
	}
	if contested == 0 {
		t.Fatal("前提破坏：没有任何字段被两层同时给出，用例测不到「后层不得改写前层」")
	}
	t.Logf("features 逐字段比对：已有值字段 %d 个，其中后层也提过值的交锋位 %d 个", knownFields, contested)
}

// regressionProbeField 描述一个待比对的字段：旧层值（zcode）、新层值，以及旧层为空时
// 回落使用的 presets 取值函数（presets 结构里只有 ctx/out 两列，没有档位）。
type regressionProbeField struct {
	name     string
	oldVal   int
	newVal   int
	oldHas   bool
	presetFn func(ModelPreset) int
}

// presetField 取 presets 条目里与当前字段对应的那一列。
func (f regressionProbeField) presetField(p ModelPreset) int {
	if f.presetFn == nil {
		return 0
	}
	return f.presetFn(p)
}

// regressionProbeIDs 汇总四类键：presets 键、目录键、覆盖层 Match、zcode 正则样本 id。
// 全部小写去空白后去重排序，保证输出稳定、失败可复现。
func regressionProbeIDs(t *testing.T) []string {
	t.Helper()
	set := make(map[string]struct{}, len(modelPresets)+len(catalogEntries)+len(modelOverrides)+len(zcodeRegexSampleIDs))
	add := func(raw string) {
		if id := strings.ToLower(strings.TrimSpace(raw)); id != "" {
			set[id] = struct{}{}
		}
	}
	for id := range modelPresets {
		add(id)
	}
	for id := range catalogEntries {
		add(id)
	}
	for _, ov := range modelOverrides {
		add(ov.Match)
	}
	for _, id := range zcodeRegexSampleIDs {
		add(id)
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
