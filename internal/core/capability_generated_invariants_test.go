package core

import (
	"strings"
	"testing"
)

// TestGeneratedFeatureTablesAlwaysCarryText 直面「输入模态丢 text」缺陷的根因，把不变量钉在
// 两张生成表上。
//
// ZCode 内建资源的**输入**模态语义是「text 是默认项，其余模态都是在 text 之上追加的增量」：
// 它只把非默认项写出来（84 条 modelRules 里只有兜底规则 `.*` 写了 inputFormat.supportsText），
// 因此生成脚本 scripts/sync-model-catalog.mjs 必须把 text 按「缺键即真、只有显式 false 才算否」
// 抽取。谁把这条规则退回严格的 === true，本测试就会立刻失败：抽出的集合会丢掉 text（例如只剩
// image），而中立层对模态是「前层胜出」的整字段合并，这个残缺集合会把离线目录（models.dev）
// 给出的完整集合整段挡住，最终让 DSH 写出 `input: [image]`——在 DSH 里那是「不接受文本输入」的
// 完整集合语义，且「自动补全」默认开启。
//
// **输出段是刻意不对称的另一半**：输出侧保持严格 `=== true`（ZCode 的输出面只有
// outputFormat.supportsText 一位，无法表达 audio/image 输出，「缺键即默认 text」会压掉下层对
// 非文本输出模型的正确读数，见脚本里 IMPLIED_TRUE_KEYS 的注释）。因此输出段只可能来自显式写了
// supportsText: true 的规则（内建兜底规则 `.*` 就是），「输出段非空必含 text」是**结果**而不是
// 被强制补齐的位；它在这里的作用是钉住「输出段一旦出现就不会是别的词」，真正的非文本输出保护
// 由 TestGeneratedTablesKeepLowerLayerOutputModalities 承担。
//
// 断言口径：遍历两张生成表里所有带非空 Features 的条目，输入段非空必含 text、输出段非空必含
// text（用 parseFeatureEncoding 走与运行时同一条读路径，避免只检查原始字符串而漏掉规范化）。
// 末尾另有前提检查，防止表被清空或全无能力位时测试空转变绿。
func TestGeneratedFeatureTablesAlwaysCarryText(t *testing.T) {
	type featureCase struct {
		table    string
		index    int
		label    string
		features string
	}
	var cases []featureCase
	for i, rule := range modelCatalogRules {
		cases = append(cases, featureCase{"modelCatalogRules", i, rule.Pattern, rule.Features})
	}
	for i, rule := range modelSiteRules {
		label := ""
		if rule.Model != nil {
			label = rule.Model.String()
		}
		cases = append(cases, featureCase{"modelSiteRules", i, label, rule.Features})
	}
	if len(modelCatalogRules) == 0 || len(modelSiteRules) == 0 {
		t.Fatalf("前提破坏：生成表为空（modelCatalogRules=%d，modelSiteRules=%d）",
			len(modelCatalogRules), len(modelSiteRules))
	}

	var withFeatures, withInput, withOutput int
	for _, c := range cases {
		if c.features == "" {
			// 空串 = 这条规则没有任何已知能力位，没有模态段可查。
			continue
		}
		withFeatures++
		segments := strings.Split(c.features, "|")
		if len(segments) != 3 {
			t.Errorf("%s[%d] %s：Features 编码不是三段：%q", c.table, c.index, c.label, c.features)
			continue
		}
		features, ok := parseFeatureEncoding(c.features)
		if !ok {
			t.Errorf("%s[%d] %s：Features 编码无法解析（解析 fail-closed 会把整条规则的能力位丢掉）：%q",
				c.table, c.index, c.label, c.features)
			continue
		}
		if segments[0] != "" {
			withInput++
			if !features.HasInputModality("text") {
				t.Errorf("%s[%d] %s：输入模态段非空（%q）却不含 text——text 是 ZCode 的默认项，"+
					"supportsText 缺键必须按隐含为真抽取", c.table, c.index, c.label, segments[0])
			}
		}
		if segments[1] != "" {
			withOutput++
			if !features.HasOutputModality("text") {
				t.Errorf("%s[%d] %s：输出模态段非空（%q）却不含 text——输出段只由显式写了 "+
					"supportsText: true 的规则产生，因此非空段只能是 text", c.table, c.index, c.label, segments[1])
			}
		}
	}
	if withFeatures == 0 || withInput == 0 || withOutput == 0 {
		t.Fatalf("前提破坏：没有覆盖到足够多的条目（带能力位 %d 条，输入段非空 %d 条，输出段非空 %d 条）",
			withFeatures, withInput, withOutput)
	}
}

// TestGeneratedTablesKeepLowerLayerOutputModalities 锁住本次精修的**刻意不对称**，用真实生成表
// 走一次分层求值：
//
//   - 输入侧：家族规则命中即断言 text（implied true 生效，这正是丢 text 缺陷的修复）；
//   - 输出侧：家族规则的输出段**不得**把离线目录给出的非文本输出压掉。
//
// 为什么输出侧不能 implied：ZCode 的输出面只有 outputFormat.supportsText 一位，它无法表达
// audio/image 输出，所以「缺 outputFormat = 输出只有 text」不是事实，只是这条规则没提输出；
// 一旦 implied 并按「前层胜出」的整字段合并胜出，就会抹掉下层对非文本输出模型的正确读数。
// 表一回归（例如有人「顺手」把输出侧也改成 implied），下面两个用例立刻失败。
func TestGeneratedTablesKeepLowerLayerOutputModalities(t *testing.T) {
	cases := []struct {
		id       string
		nonText  []string // 离线目录给出的、必须存活到最终结果里的非文本输出模态
		scenario string
	}{
		{
			id:       "mimo-v2.5-tts",
			nonText:  []string{"audio"},
			scenario: "TTS 模型：离线目录给 text|audio，家族规则 `.*mimo-v2\\.5(?:[.\\-:/\\[].*)?` 命中它但整段没写 outputFormat",
		},
		{
			id:       "gpt-6-astra",
			nonText:  []string{"image"},
			scenario: "图文输出模型：离线目录给 text,image,pdf|text,image，家族规则 `.*gpt-6-astra(?:...)?` 同样没写 outputFormat",
		},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			// 对照物：离线目录层（capability_features_gen.go）给出的输出模态。
			catalog := lookupCatalogFeatures(c.id)
			for _, word := range c.nonText {
				if !catalog.HasOutputModality(word) {
					t.Fatalf("前提破坏：离线目录对 %s 应给出 %s 输出（它是「不被压掉」的对照物），实际 %s",
						c.id, word, describeFeatures(catalog))
				}
			}

			got := LookupModelCapabilityForSite(c.id, "", "")
			// 输出侧：非文本输出必须原样存活（严格语义下 ZCode 层在输出段什么都没贡献）。
			for _, word := range c.nonText {
				if !got.Features.HasOutputModality(word) {
					t.Errorf("OutputModalities=%v 丢了 %s：ZCode 层的输出段不得 implied 成 text 压掉下层读数（%s）",
						got.Features.OutputModalities, word, c.scenario)
				}
			}
			// 输入侧：implied true 仍然生效。
			if !got.Features.HasInputModality("text") {
				t.Errorf("InputModalities=%v 缺 text：输入侧的 implied true 必须仍然生效", got.Features.InputModalities)
			}
		})
	}
}

// TestGeneratedFeatureEncodingsHaveKnownBits 校验生成物的**形状**（与
// internal/core/capability_features.go 的 parseFeatureEncoding 共享契约）：每个条目要么整条省略
// Features 字段（Go 侧零值 ""），要么恰好三段、flags 恰好 6 位，且**至少有一个已知为真的位**。
//
// 「三段全空 / flags 全 '-'」的占位串没有任何信息，按契约必须整条省略（生成脚本的
// checkEncodingShape 也把这一条做成生成期硬失败），因此它不该出现在生成物里：这条断言既挡住
// 「空条目却写了 Features 字段」的噪音 diff 回归，也顺带钉住「空集写空段」的编码形状——
// 输入面没声明（例如整段没有 inputFormat）的条目正是靠空输入段 + 非空 flags 表达的，
// 不是靠一个空串占位。
func TestGeneratedFeatureEncodingsHaveKnownBits(t *testing.T) {
	type entry struct {
		table    string
		index    int
		label    string
		features string
	}
	var entries []entry
	for i, rule := range modelCatalogRules {
		entries = append(entries, entry{"modelCatalogRules", i, rule.Pattern, rule.Features})
	}
	for i, rule := range modelSiteRules {
		label := ""
		if rule.Model != nil {
			label = rule.Model.String()
		}
		entries = append(entries, entry{"modelSiteRules", i, label, rule.Features})
	}
	if len(entries) == 0 {
		t.Fatal("前提破坏：两张生成表都是空的")
	}

	encoded := 0
	for _, e := range entries {
		if e.features == "" {
			continue // 整条省略 Features 字段：合法且是空条目的唯一形状
		}
		encoded++
		segments := strings.Split(e.features, "|")
		if len(segments) != 3 {
			t.Errorf("%s[%d] %s：Features 不是三段：%q", e.table, e.index, e.label, e.features)
			continue
		}
		if len(segments[2]) != len(capabilityFeatureFlagOrder) {
			t.Errorf("%s[%d] %s：flags 段长度应为 %d，实际 %q",
				e.table, e.index, e.label, len(capabilityFeatureFlagOrder), segments[2])
			continue
		}
		if segments[0] == "" && segments[1] == "" && !strings.Contains(segments[2], "1") {
			t.Errorf("%s[%d] %s：Features 只有全空段与全 '-' 的 flags（无任何已知为真的位），"+
				"这种条目应整条省略该字段：%q", e.table, e.index, e.label, e.features)
		}
	}
	if encoded == 0 {
		t.Fatal("前提破坏：没有任何条目带 Features，断言空转")
	}
}

// TestGeneratedSiteEntriesDoNotNarrowInputWithoutInputFormat 用真实生成表 + 真实站点上下文走
// 一次分层求值，锁住「`inputFormat` 整段缺失的站点条目不得声明输入面」这条语义。
//
// 背景：ZCode 的 overlay 是逐键的（省略键 = 不覆盖，整段缺失 = 这一层完全不碰输入面），因此
// 生成期必须把「缺 inputFormat 段」与「段内缺 supportsText 键」分开——后者才是「沿用默认
// text」。这两类条目在生成物里的形状不同：整段缺失 ⇒ **空输入段**（如 `||--11--`），
// 段内缺 text ⇒ 输入段含 text（如 `text,image,video||------`）。本测试断言的是前者的行为
// 后果：它命中后不得把输入面收窄成「只有 text」。
//
// 样本取自内建资源的真实协议变体条目（baseUrl 是网关地址、apiType 为 anthropic-messages 的
// 那批，以及 api.deepseek.com 的 openai-responses 变体）；全部使用占位性事实（内建资源本身）。
func TestGeneratedSiteEntriesDoNotNarrowInputWithoutInputFormat(t *testing.T) {
	cases := []struct {
		id      string
		baseURL string
		apiType string
		wantIn  []string // 站点上下文求值后仍必须存在的输入模态
		note    string
	}{
		{
			id:      "deepseek-flash",
			baseURL: "https://api.deepseek.com",
			apiType: "openai-responses",
			wantIn:  []string{"text", "image"},
			note:    "站点变体条目只写了 supportsJsonSchemaOutput（整段没有 inputFormat），不得把 text,image 收窄成 text",
		},
		{
			id:      "claude-fable-5",
			baseURL: "https://api.z.ai/api/anthropic",
			apiType: "anthropic-messages",
			wantIn:  []string{"text", "image"},
			note:    "z.ai 网关的 anthropic 变体条目整段没有 inputFormat，不得把图片输入收窄成 text",
		},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			// 对照物：不带站点上下文的求值结果（家族正则 + 离线目录）。
			plain := LookupModelCapabilityForSite(c.id, "", "")
			for _, word := range c.wantIn {
				if !plain.Features.HasInputModality(word) {
					t.Fatalf("前提破坏：不带站点上下文时 %s 的输入模态应含 %s，实际 %v",
						c.id, word, plain.Features.InputModalities)
				}
			}
			got := LookupModelCapabilityForSite(c.id, c.baseURL, c.apiType)
			for _, word := range c.wantIn {
				if !got.Features.HasInputModality(word) {
					t.Errorf("站点上下文求值后 %s 的输入模态 %v 丢了 %s（%s）",
						c.id, got.Features.InputModalities, word, c.note)
				}
			}
		})
	}
}
