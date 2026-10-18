package core

import (
	"reflect"
	"strings"
	"testing"
)

// 本文件覆盖两块：
//  1. metaLevels / metaReasoning 从 provider 返回的 meta 里读「档位枚举」与「是否支持思考」，
//     要求容错、绝不 panic；
//  2. BuildModelCardsForSite / CfgToCardForSite 的优先级：provider 返回的值 > 能力表。
//
// 全部使用占位数据与 example.invalid 这类保留域名，不读也不写任何真实用户配置目录。

// TestMetaLevels 锁定 metaLevels 的三种形态（thinking_level / reasoning_options /
// supported_efforts）与优先级、白名单过滤、畸形输入容错。
func TestMetaLevels(t *testing.T) {
	// 档位白名单策略固定为默认（关闭自定义档位），避免受同包其它用例影响。
	setCustomVariantsForTest(t, false)
	cases := []struct {
		name string
		meta map[string]interface{}
		want []string
	}{
		{"thinking_level 字符串", map[string]interface{}{"thinking_level": "high"}, []string{"high"}},
		{"thinkingLevel 驼峰", map[string]interface{}{"thinkingLevel": "max"}, []string{"max"}},
		{"reasoning_effort 字符串数组", map[string]interface{}{"reasoning_effort": []interface{}{"low", "high"}}, []string{"low", "high"}},
		{"reasoning_efforts []string", map[string]interface{}{"reasoning_efforts": []string{"medium"}}, []string{"medium"}},
		{"supported_efforts 数组", map[string]interface{}{"supported_efforts": []interface{}{"xhigh", "low"}}, []string{"low", "xhigh"}},
		{"reasoning.efforts", map[string]interface{}{"reasoning": map[string]interface{}{"efforts": []interface{}{"low", "high"}}}, []string{"low", "high"}},
		{"reasoning.supported_efforts 逗号串", map[string]interface{}{"reasoning": map[string]interface{}{"supported_efforts": "low, high"}}, []string{"low", "high"}},
		{"off 别名折成 off", map[string]interface{}{"thinking_level": "disabled"}, []string{"off"}},
		{"第一优先组胜出", map[string]interface{}{"thinking_level": "max", "supported_efforts": []interface{}{"low"}}, []string{"max"}},
		{"白名单滤掉未知档位", map[string]interface{}{"thinking_level": "ultra-9"}, nil},
		{"某一组全被滤掉时看下一组", map[string]interface{}{"thinking_level": "ultra-9", "supported_efforts": []interface{}{"low"}}, []string{"low"}},
		{"capabilities 只声明支持态不产档位", map[string]interface{}{"capabilities": map[string]interface{}{"reasoning": true, "thinking": true}}, nil},
		{
			"reasoning_options 只取 effort 的 values",
			map[string]interface{}{"reasoning_options": []interface{}{
				map[string]interface{}{"type": "toggle", "values": []interface{}{"on", "off"}},
				map[string]interface{}{"type": "budget_tokens", "values": []interface{}{"1024", "2048"}},
				map[string]interface{}{"type": "effort", "values": []interface{}{"low", "high"}},
			}},
			[]string{"low", "high"},
		},
		{"reasoning_options 的 type 大小写不敏感", map[string]interface{}{"reasoning_options": []interface{}{
			map[string]interface{}{"type": "Effort", "values": []interface{}{"max"}},
		}}, []string{"max"}},
		{"无档位来源时为 nil", map[string]interface{}{"context_length": 128000}, nil},
		{"空 meta", map[string]interface{}{}, nil},
		{
			"畸形输入不 panic",
			map[string]interface{}{
				"thinking_level":    42,
				"reasoning_effort":  nil,
				"supported_efforts": map[string]interface{}{"a": 1},
				"reasoning":         "high",
				"reasoning_options": "not-a-list",
				"capabilities":      "x",
				"thinkingLevel":     []interface{}{nil, 7, map[string]interface{}{}},
			},
			nil,
		},
		{
			"reasoning_options 元素畸形",
			map[string]interface{}{"reasoning_options": []interface{}{
				nil, 3, map[string]interface{}{}, map[string]interface{}{"type": "effort"},
			}},
			nil,
		},
	}
	for _, c := range cases {
		got := metaLevels(c.meta)
		if len(c.want) == 0 {
			if got != nil {
				t.Errorf("%s：期望 nil，实际 %v", c.name, got)
			}
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s：got %v，期望 %v", c.name, got, c.want)
		}
	}
	if got := metaLevels(nil); got != nil {
		t.Errorf("metaLevels(nil)=%v，期望 nil", got)
	}
}

// TestMetaReasoning 锁定 metaReasoning 的取值顺序与「不猜」语义。
func TestMetaReasoning(t *testing.T) {
	cases := []struct {
		name      string
		meta      map[string]interface{}
		wantValue bool
		wantKnown bool
	}{
		{"reasoning 布尔真", map[string]interface{}{"reasoning": true}, true, true},
		{"reasoning 布尔假", map[string]interface{}{"reasoning": false}, false, true},
		{"thinking 布尔真", map[string]interface{}{"thinking": true}, true, true},
		{"reasoning 对象 enabled", map[string]interface{}{"reasoning": map[string]interface{}{"enabled": true}}, true, true},
		{"capabilities.reasoning", map[string]interface{}{"capabilities": map[string]interface{}{"reasoning": true}}, true, true},
		{"capabilities.thinking 假", map[string]interface{}{"capabilities": map[string]interface{}{"thinking": false}}, false, true},
		{"supported_parameters 含 reasoning", map[string]interface{}{"supported_parameters": []interface{}{"tools", "reasoning"}}, true, true},
		{"supported_parameters 含 include_reasoning", map[string]interface{}{"supported_parameters": []interface{}{"include_reasoning"}}, true, true},
		{"supported_parameters 未列出则不猜", map[string]interface{}{"supported_parameters": []interface{}{"tools", "temperature"}}, false, false},
		{"reasoning 优先于 supported_parameters", map[string]interface{}{"reasoning": false, "supported_parameters": []interface{}{"reasoning"}}, false, true},
		{"只有档位不构成支持态证据", map[string]interface{}{"thinking_level": "high"}, false, false},
		{"空 meta", map[string]interface{}{}, false, false},
		{
			"畸形输入不 panic",
			map[string]interface{}{
				"reasoning":            "yes",
				"thinking":             1,
				"capabilities":         []interface{}{1},
				"supported_parameters": map[string]interface{}{"reasoning": true},
			},
			false, false,
		},
	}
	for _, c := range cases {
		gotValue, gotKnown := metaReasoning(c.meta)
		if gotKnown != c.wantKnown || (c.wantKnown && gotValue != c.wantValue) {
			t.Errorf("%s：got (%v, %v)，期望 (%v, %v)", c.name, gotValue, gotKnown, c.wantValue, c.wantKnown)
		}
	}
	if v, known := metaReasoning(nil); known || v {
		t.Errorf("metaReasoning(nil)=(%v, %v)，期望 (false, false)", v, known)
	}
}

// cardsForSiteTest 是构造卡片的测试辅助：单模型、指定 autoFill 与站点上下文。
func cardsForSiteTest(t *testing.T, id string, meta map[string]interface{}, autoFill bool, baseURL, apiType string) ModelCard {
	t.Helper()
	cards := BuildModelCardsForSite([]struct {
		ID   string
		Meta map[string]interface{}
	}{{ID: id, Meta: meta}}, autoFill, baseURL, apiType)
	if len(cards) != 1 {
		t.Fatalf("期望 1 张卡片，实际 %d", len(cards))
	}
	return cards[0]
}

// TestBuildModelCardsForSiteProviderValuesWin provider 返回的 ctx/out 永远胜出能力表；
// 新增的键表别名（max_input_tokens / context_size / output_token_limit / max_new_tokens）
// 只在既有 5 个键都读不出时生效。
func TestBuildModelCardsForSiteProviderValuesWin(t *testing.T) {
	const base = "https://example.invalid/v1"
	const api = "openai-chat-completions"
	// 兜底值来自能力表：deepseek-v4-flash 由内建正则给出 1000000/384000
	// （社区目录写的是 393216，但目录排在内建之后，只补空缺，不能改写内建值）。
	if cap := LookupModelCapabilityForSite("deepseek-v4-flash", base, api); cap.Context != 1000000 || cap.Output != 384000 || cap.Source != "zcode" {
		t.Fatalf("前提破坏：deepseek-v4-flash 的能力表值变了：%+v", cap)
	}

	// provider 两个值都给了 → 都胜出。
	c := cardsForSiteTest(t, "deepseek-v4-flash", map[string]interface{}{"context_length": 64000, "max_output_tokens": 8192}, true, base, api)
	if c.Context != 64000 || c.Output != 8192 {
		t.Errorf("provider 值应胜出：ctx=%v out=%v", c.Context, c.Output)
	}
	// 新增上下文别名：命中则胜出，未命中的一侧回落能力表。
	c = cardsForSiteTest(t, "deepseek-v4-flash", map[string]interface{}{"max_input_tokens": 123456}, true, base, api)
	if c.Context != 123456 || c.Output != 384000 {
		t.Errorf("max_input_tokens 应生效：ctx=%v out=%v", c.Context, c.Output)
	}
	c = cardsForSiteTest(t, "deepseek-v4-flash", map[string]interface{}{"context_size": 32000}, true, base, api)
	if c.Context != 32000 || c.Output != 384000 {
		t.Errorf("context_size 应生效：ctx=%v out=%v", c.Context, c.Output)
	}
	// 新增输出别名。
	c = cardsForSiteTest(t, "deepseek-v4-flash", map[string]interface{}{"output_token_limit": 4096}, true, base, api)
	if c.Context != 1000000 || c.Output != 4096 {
		t.Errorf("output_token_limit 应生效：ctx=%v out=%v", c.Context, c.Output)
	}
	c = cardsForSiteTest(t, "deepseek-v4-flash", map[string]interface{}{"max_new_tokens": 2048}, true, base, api)
	if c.Context != 1000000 || c.Output != 2048 {
		t.Errorf("max_new_tokens 应生效：ctx=%v out=%v", c.Context, c.Output)
	}
	// 既有键优先于新增别名。
	c = cardsForSiteTest(t, "deepseek-v4-flash", map[string]interface{}{"context_length": 64000, "context_size": 32000}, true, base, api)
	if c.Context != 64000 {
		t.Errorf("既有键应优先：ctx=%v", c.Context)
	}
	// autoFill 关闭：不补能力表值；provider 值仍然保留。
	c = cardsForSiteTest(t, "deepseek-v4-flash", map[string]interface{}{}, false, base, api)
	if c.Context != "" || c.Output != "" {
		t.Errorf("autoFill 关闭不应补全：ctx=%v out=%v", c.Context, c.Output)
	}
	c = cardsForSiteTest(t, "deepseek-v4-flash", map[string]interface{}{"max_input_tokens": 123456}, false, base, api)
	if c.Context != 123456 || c.Output != "" {
		t.Errorf("autoFill 关闭时 provider 值仍应保留：ctx=%v out=%v", c.Context, c.Output)
	}
}

// TestBuildModelCardsForSiteSupportedLevels _supported_levels 的优先级与开关口径：
// provider 明确给出的档位（API 事实）> 能力表档位；**两者都与 autoFill 无关**。
//
// 2026-10-15（阶段 F）本条改写过一次：此前能力表档位被 `autoFill` 门控（「provider 未给
// 档位 + autoFill 为假 ⇒ SupportedLevels 为 nil」），现改为照填。依据：该开关的文案是
// 「导入时自动补全上下文与输出长度」，管的是**会被写进配置的值**；而 _supported_levels
// 从不写盘、也不参与任何自动写值，只驱动展示（档位自适应弱化）、种子（开思考时用支持
// 集合）与「按能力表补全档位」这一显式动作——与同性质的 _capabilities_inferred 一样
// 应当无条件写入（理由见 cards.go 该处注释）。
func TestBuildModelCardsForSiteSupportedLevels(t *testing.T) {
	setCustomVariantsForTest(t, false)
	const base = "https://example.invalid/v1"
	const api = "openai-chat-completions"
	// 能力表里 glm-5.3 的档位是 [low high max]（来自内建正则；社区目录同值）。
	if cap := LookupModelCapabilityForSite("glm-5.3", base, api); !reflect.DeepEqual(cap.ReasoningLevels, []string{"low", "high", "max"}) {
		t.Fatalf("前提破坏：glm-5.3 的能力表档位变了：%+v", cap.ReasoningLevels)
	}

	provider := map[string]interface{}{"thinking_level": "max"}
	cases := []struct {
		name     string
		meta     map[string]interface{}
		autoFill bool
		want     []string
	}{
		{"provider 档位 + autoFill 开：provider 胜出", provider, true, []string{"max"}},
		{"provider 档位 + autoFill 关：provider 仍胜出", provider, false, []string{"max"}},
		{"provider 未给 + autoFill 开：回落能力表", map[string]interface{}{}, true, []string{"low", "high", "max"}},
		{"provider 未给 + autoFill 关：仍回落能力表（阶段 F 修正）", map[string]interface{}{}, false, []string{"low", "high", "max"}},
	}
	for _, c := range cases {
		if got := cardsForSiteTest(t, "glm-5.3", c.meta, c.autoFill, base, api).SupportedLevels; !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s：SupportedLevels=%v，期望 %v", c.name, got, c.want)
		}
	}
	// 放开档位不等于放开 ctx/out 的门控：provider 值照样胜出，缺的那一侧仍留空。
	c := cardsForSiteTest(t, "glm-5.3", map[string]interface{}{"context_length": 64000}, false, base, api)
	if c.Context != 64000 || c.Output != "" {
		t.Errorf("autoFill 关闭时 ctx/out 门控必须保持不变：ctx=%v out=%v", c.Context, c.Output)
	}
	if !reflect.DeepEqual(c.SupportedLevels, []string{"low", "high", "max"}) {
		t.Errorf("autoFill 关闭时档位仍应回落能力表：%v", c.SupportedLevels)
	}
	// 站点层补出的档位同样落到卡片上，且与开关无关：minimax-m2.7 只有站点规则给档位。
	for _, autoFill := range []bool{true, false} {
		c := cardsForSiteTest(t, "minimax-m2.7", map[string]interface{}{}, autoFill, "https://opencode.ai/zen/go/v1", "anthropic-messages")
		if !reflect.DeepEqual(c.SupportedLevels, []string{"enabled"}) {
			t.Errorf("站点层档位应落到卡片（autoFill=%v）：%v", autoFill, c.SupportedLevels)
		}
		if c.Vendor != "minimax" {
			t.Errorf("Vendor=%q，期望 minimax（autoFill=%v）", c.Vendor, autoFill)
		}
	}
}

// TestBuildModelCardsForSiteAutoChecksLevels 「导入新供应商」时自动勾选支持的思考强度
// （2026-10-17）：拉取模型产出的卡片直接带着**将要写进配置的档位集合**，因此
//   - 思考已开且档位里有非 off 档时，Variants = 该组档位、DefaultVariant = 规范默认（high 优先）；
//   - 与 ctx/out、capabilities 同口径：受「自动补全」开关门控（它们都会被写进配置）；
//   - provider 显式声明不支持思考、或档位里只有 off、或任何来源都没给出档位时一律不勾。
//
// 最后一组断言锁住真正的收益：写出去的档位是**支持集合**，而不是写死的 {off,high,max}
// ——后者会给 glm-5.3 这类不支持关闭思考的模型凭空写一个 off。
func TestBuildModelCardsForSiteAutoChecksLevels(t *testing.T) {
	setCustomVariantsForTest(t, false)
	const base = "https://example.invalid/v1"
	const api = "openai-chat-completions"
	if cap := LookupModelCapabilityForSite("glm-5.3", base, api); !reflect.DeepEqual(cap.ReasoningLevels, []string{"low", "high", "max"}) {
		t.Fatalf("前提破坏：glm-5.3 的能力表档位变了：%+v", cap.ReasoningLevels)
	}

	cases := []struct {
		name        string
		id          string
		meta        map[string]interface{}
		autoFill    bool
		want        []string
		wantDefault string
	}{
		{"能力表档位 + 开关开：自动勾选", "glm-5.3", map[string]interface{}{}, true, []string{"low", "high", "max"}, "high"},
		{"能力表档位 + 开关关：卡片不动", "glm-5.3", map[string]interface{}{}, false, nil, ""},
		{"provider 自报档位胜出", "glm-5.3", map[string]interface{}{"thinking_level": "max"}, true, []string{"max"}, "max"},
		{"provider 显式关闭思考：不勾", "glm-5.3", map[string]interface{}{"reasoning": false}, true, nil, ""},
		{"档位里只有 off：不勾", "my-local-model", map[string]interface{}{"thinking_level": "disabled"}, true, nil, ""},
		{"任何来源都没有档位：不勾", "my-local-model", map[string]interface{}{}, true, nil, ""},
		{
			"自命名模型 + provider 档位：照勾",
			"my-local-model", map[string]interface{}{"thinking_level": []interface{}{"high", "low"}}, true,
			[]string{"low", "high"}, "high",
		},
	}
	for _, c := range cases {
		card := cardsForSiteTest(t, c.id, c.meta, c.autoFill, base, api)
		if len(c.want) == 0 {
			if card.Variants != nil {
				t.Errorf("%s：Variants=%v，期望 nil", c.name, card.Variants)
			}
			if card.DefaultVariant != "" {
				t.Errorf("%s：DefaultVariant=%q，期望空", c.name, card.DefaultVariant)
			}
			continue
		}
		if !reflect.DeepEqual(card.Variants, c.want) {
			t.Errorf("%s：Variants=%v，期望 %v", c.name, card.Variants, c.want)
		}
		if card.DefaultVariant != c.wantDefault {
			t.Errorf("%s：DefaultVariant=%q，期望 %q", c.name, card.DefaultVariant, c.wantDefault)
		}
	}

	// 收益的落点：导入写出的档位就是支持集合；开关关掉时保持既有行为（卡片无档位，
	// 写盘侧回落 DefaultVariantsFor 的 [off high max]）。
	cfg, _, err := CardToCfg(cardsForSiteTest(t, "glm-5.3", map[string]interface{}{}, true, base, api), "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg 失败：%v", err)
	}
	if got := reasoningVariantsForTest(t, cfg); !reflect.DeepEqual(got, []string{"low", "high", "max"}) {
		t.Errorf("开关开启时写出的档位应为支持集合：%v", got)
	}
	plain, _, err := CardToCfg(cardsForSiteTest(t, "glm-5.3", map[string]interface{}{}, false, base, api), "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg 失败：%v", err)
	}
	if got := reasoningVariantsForTest(t, plain); !reflect.DeepEqual(got, DefaultVariantsFor("openai-compatible")) {
		t.Errorf("开关关闭时应保持既有兜底：%v", got)
	}
}

// reasoningVariantsForTest 取写回配置里 reasoning.variants（测试辅助）。
func reasoningVariantsForTest(t *testing.T, cfg map[string]interface{}) []string {
	t.Helper()
	block, ok := cfg["reasoning"].(map[string]interface{})
	if !ok {
		t.Fatalf("写回的配置里没有 reasoning 块：%+v", cfg)
	}
	got, _ := block["variants"].([]string)
	return got
}

// TestBuildModelCardsForSiteReasoning API 明确声明的思考支持态优先于按能力表 / 模型名做的推断；
// 反过来，provider 用**档位枚举**宣告的思考支持同样成立（见函数末尾一段，2026-10-15 阶段 F 补正）。
func TestBuildModelCardsForSiteReasoning(t *testing.T) {
	setCustomVariantsForTest(t, false)
	const base = "https://example.invalid/v1"
	const api = "openai-chat-completions"

	// glm-5.3：能力表档位里有非 off 档，推断为支持思考；API 说 false 时必须以 API 为准。
	if c := cardsForSiteTest(t, "glm-5.3", map[string]interface{}{}, true, base, api); !c.Reasoning {
		t.Fatalf("前提破坏：glm-5.3 的推断结果应为支持思考：%+v", c)
	}
	if c := cardsForSiteTest(t, "glm-5.3", map[string]interface{}{"reasoning": false}, true, base, api); c.Reasoning {
		t.Errorf("API 明确 false 时应以 API 为准：%+v", c.Reasoning)
	}
	// 自命名模型：能力表与关键词都推不出思考，API 说 true 时必须以 API 为准。
	if c := cardsForSiteTest(t, "my-local-model", map[string]interface{}{}, true, base, api); c.Reasoning {
		t.Fatalf("前提破坏：my-local-model 的推断结果应为不支持思考：%+v", c)
	}
	if c := cardsForSiteTest(t, "my-local-model", map[string]interface{}{"capabilities": map[string]interface{}{"reasoning": true}}, true, base, api); !c.Reasoning {
		t.Errorf("API 明确 true 时应以 API 为准：%+v", c.Reasoning)
	}
	// provider 自报**档位枚举** = 支持思考（2026-10-15，阶段 F 补正）：自命名模型的能力表与
	// 关键词都认不出它，但 API 明确列出了可选档位。此前的顺序会得到「卡片列出了档位却声明不
	// 支持思考」，前端据此禁用思考开关，批量的「按能力表补全档位」也会把它当成显式关闭而跳过，
	// 因此必须以档位枚举补正。
	c := cardsForSiteTest(t, "my-local-model", map[string]interface{}{"thinking_level": []interface{}{"low", "high"}}, true, base, api)
	if !c.Reasoning {
		t.Errorf("provider 列出非 off 档位时应判定为支持思考：%+v", c)
	}
	if !reflect.DeepEqual(c.SupportedLevels, []string{"low", "high"}) {
		t.Errorf("SupportedLevels=%v，期望 provider 自报的 [low high]", c.SupportedLevels)
	}
	// 只有关闭档的枚举不算支持思考（与能力表「存在非 off 档才算支持」同口径）。
	if c := cardsForSiteTest(t, "my-local-model", map[string]interface{}{"thinking_level": []interface{}{"disabled"}}, true, base, api); c.Reasoning {
		t.Errorf("档位枚举里只有 off 时不应判定为支持思考：%+v", c)
	}
	// provider **显式**声明过思考支持态时，档位枚举不得把它翻回来。
	if c := cardsForSiteTest(t, "my-local-model", map[string]interface{}{"thinking_level": []interface{}{"high"}, "reasoning": false}, true, base, api); c.Reasoning {
		t.Errorf("provider 显式 false 时不得被档位枚举覆盖：%+v", c)
	}
	// 关掉自动补全不影响这条判断（它看的是 provider 自报事实，不是能力表推断）。
	if c := cardsForSiteTest(t, "my-local-model", map[string]interface{}{"thinking_level": []interface{}{"high"}}, false, base, api); !c.Reasoning {
		t.Errorf("autoFill 关闭时 provider 自报档位仍应判为支持思考：%+v", c)
	}
}

// TestGuessReasoningCatalogOnlyLevels 记录本次改造的一个**预期**行为变化：离线目录带档位后，
// 更多模型会被判定为「支持思考」。
//
// hy4-preview 是纯目录来源：内建正则读不出它，关键词表也认不出（名字里没有
// reason/think/deepseek-r/o1/o3/o4 之类），因此改造前 GuessReasoning 为 false，
// 现在目录给出档位 [off, high]（存在非 off 档）→ true。这正是「只提升自动补全命中率」的
// 那一半；同时「真正没有任何来源的模型仍必须判为不支持」的契约在下方一并锁住。
func TestGuessReasoningCatalogOnlyLevels(t *testing.T) {
	const id = "hy4-preview"
	if raw := mergeCatalog(id, modelCatalogRules); raw.Context != 0 || raw.Output != 0 || raw.ReasoningLevels != nil {
		t.Fatalf("前提破坏：%s 已能被内建正则命中：%+v", id, raw)
	}
	cap, _ := LookupModelCapability(id)
	if cap.Source != catalogSource || !reflect.DeepEqual(cap.ReasoningLevels, []string{"off", "high"}) {
		t.Fatalf("前提破坏：%s 的目录档位变了：%+v", id, cap)
	}
	if !GuessReasoning(id) {
		t.Errorf("目录档位里有非 off 档（high），应判定为支持思考")
	}
	// 契约：真正不在任何来源里的模型必须留空，且不得被判为支持思考。
	if GuessReasoning("my-local-model") {
		t.Errorf("未知模型不应被判定为支持思考")
	}
}

// TestCfgToCardForSite 读卡路径：站点上下文会把站点层档位补进 _supported_levels，
// 且 CfgToCard 与「传空 site」完全等价、绝不覆盖配置里读到的 variants。
func TestCfgToCardForSite(t *testing.T) {
	setCustomVariantsForTest(t, false)
	cfg := map[string]interface{}{
		"reasoning": map[string]interface{}{"variants": []interface{}{"high"}},
	}
	// minimax-m2.7 的档位只能来自站点规则（离线目录无档位、内建正则给 nil）。
	site := CfgToCardForSite("minimax-m2.7", cfg, "https://opencode.ai/zen/go/v1", "anthropic-messages")
	if !reflect.DeepEqual(site.SupportedLevels, []string{"enabled"}) {
		t.Errorf("SupportedLevels=%v，期望 [enabled]（来自站点规则）", site.SupportedLevels)
	}
	if site.Vendor != "minimax" {
		t.Errorf("Vendor=%q，期望 minimax", site.Vendor)
	}
	if !reflect.DeepEqual(site.Variants, []string{"high"}) {
		t.Errorf("Variants=%v，期望配置里的 [high] 原样保留", site.Variants)
	}
	// 不带站点上下文：拿不到站点层档位。
	plain := CfgToCardForSite("minimax-m2.7", cfg, "", "")
	if plain.SupportedLevels != nil {
		t.Errorf("无站点上下文时不应有档位：%v", plain.SupportedLevels)
	}
	// 转调关系：CfgToCard 完全等价于传空 site。
	if !reflect.DeepEqual(CfgToCard("minimax-m2.7", cfg), plain) {
		t.Errorf("CfgToCard 应等价于 CfgToCardForSite(..., \"\", \"\")")
	}
}

// TestCfgToCardForSiteInternalKeysNeverWritten 站点上下文新增的 _supported_levels / _vendor
// 只存在于卡片上：写回配置的路径（CardToCfg → stripAPMInternalModelKeys）不会把它们落盘。
// 本次改动没有新增任何写入路径，这里用站点路径产出的卡片复查一遍该不变量。
func TestCfgToCardForSiteInternalKeysNeverWritten(t *testing.T) {
	setCustomVariantsForTest(t, false)
	card := CfgToCardForSite("minimax-m2.7", map[string]interface{}{}, "https://opencode.ai/zen/go/v1", "anthropic-messages")
	if len(card.SupportedLevels) == 0 || card.Vendor == "" {
		t.Fatalf("前提破坏：站点路径应给出内部推断字段：%+v", card)
	}
	modelCfg, modelID, err := CardToCfg(card, "openai-compatible")
	if err != nil {
		t.Fatalf("CardToCfg 失败：%v", err)
	}
	for key := range modelCfg {
		if strings.HasPrefix(key, "_") {
			t.Errorf("写回的模型配置里出现内部键 %q：%+v", key, modelCfg)
		}
	}
	providerCfg := map[string]interface{}{"models": map[string]interface{}{modelID: modelCfg}}
	stripAPMInternalModelKeys(providerCfg)
	written, _ := providerCfg["models"].(map[string]interface{})[modelID].(map[string]interface{})
	for _, key := range []string{"_vendor", "_supported_levels", "_raw_cfg", "_off_alias"} {
		if _, ok := written[key]; ok {
			t.Errorf("内部键 %q 不应落回配置：%+v", key, written)
		}
	}
}
