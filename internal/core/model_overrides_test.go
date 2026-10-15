package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"testing"
)

// siteProbeBaseURL / siteProbeAPIType 是覆盖率实测用的站点上下文（占位值，
// commandcode 中转的 baseUrl 与协议类型）。它不参与任何写盘，只用来把站点层一起拉进
// 求值链路，保证断言的是真实分层结果而不是「不带 site」的简化结果。
const (
	siteProbeBaseURL = "https://api.commandcode.ai/provider/v1"
	siteProbeAPIType = "openai-chat-completions"
)

// capabilityAssertion 是一条「某个 id 在带站点上下文下应得的完整能力结果」断言。
//
// Context/Output 里出现的非覆盖值（来自 zcode 正则或离线目录）也一并断言：这样一旦上游
// 数据漂移，测试会立刻指出是「覆盖层条目失效」还是「底层数据变了」。
type capabilityAssertion struct {
	id     string
	ctx    int
	out    int
	levels []string
	source string
	vendor string
}

// assertCapabilities 是表驱动断言的公共实现。
func assertCapabilities(t *testing.T, cases []capabilityAssertion) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			t.Helper()
			cap := LookupModelCapabilityForSite(tc.id, siteProbeBaseURL, siteProbeAPIType)
			if cap.Context != tc.ctx {
				t.Errorf("Context = %d, 期望 %d", cap.Context, tc.ctx)
			}
			if cap.Output != tc.out {
				t.Errorf("Output = %d, 期望 %d", cap.Output, tc.out)
			}
			if !sameLevels(cap.ReasoningLevels, tc.levels) {
				t.Errorf("ReasoningLevels = %v, 期望 %v", cap.ReasoningLevels, tc.levels)
			}
			if cap.Source != tc.source {
				t.Errorf("Source = %q, 期望 %q", cap.Source, tc.source)
			}
			if cap.Vendor != tc.vendor {
				t.Errorf("Vendor = %q, 期望 %q", cap.Vendor, tc.vendor)
			}
		})
	}
}

// sameLevels 比较两个档位切片（nil 与空切片视为等价）。
func sameLevels(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestModelOverridesCoverageAdditions 锁定本次覆盖率补齐新增的 7 条覆盖层条目。
//
// 每条断言三件事：
//  1. Match 键命中的是完整 id（大小写按 provider 返回的原拼写，表内统一小写）；
//  2. 我填写的字段（Context / Output / Levels）确实生效，且 Source == "override"；
//  3. 我没有填的字段取到的仍是底层（zcode 正则或离线目录）的值，说明覆盖层只填空不越权。
//
// 断言值为 2026-10 实测的快照：Context/Output 分别标注了来源类别。
func TestModelOverridesCoverageAdditions(t *testing.T) {
	assertCapabilities(t, []capabilityAssertion{
		{
			// 覆盖 Context（上游自报 262144，目录 1048756 相差 4 倍）；
			// Output 未被覆盖，沿用离线目录的 262144。
			id: "meituan/LongCat-2.0", ctx: 262144, out: 262144,
			levels: nil, source: "override", vendor: "unknown",
		},
		{
			// 覆盖 Context（上游自报 524288）+ 档位（同族直取 off,medium,high）；
			// Output 未被覆盖，沿用离线目录的 65536。
			id: "nvidia/nemotron-3-ultra-550b-a55b", ctx: 524288, out: 65536,
			levels: []string{"off", "medium", "high"}, source: "override", vendor: "unknown",
		},
		{
			// 只覆盖档位（同族直取 low,medium,high）；Context/Output 沿用离线目录。
			id: "poolside/laguna-s-2.1-free", ctx: 256000, out: 32768,
			levels: []string{"low", "medium", "high"}, source: "override", vendor: "unknown",
		},
		{
			// 只覆盖 Context（上游自报 1000000，目录 262144 相差 4 倍）；档位仍无来源。
			id: "Qwen/Qwen3.6-Max-Preview", ctx: 1000000, out: 65536,
			levels: nil, source: "override", vendor: "alibaba",
		},
		{
			// 完全无信息的 id 只剩 Context 一个来源（上游自报 256000）；Output/档位留空。
			id: "stealth/glyph-cluster:free", ctx: 256000, out: 0,
			levels: nil, source: "override", vendor: "unknown",
		},
		{
			// 只覆盖档位（同族直取 low,high）；Context 保留目录的 262144
			// （与上游自报 256000 是同一 256K 窗口的两种写法，故不覆盖）。
			id: "stepfun/Step-3.5-Flash", ctx: 262144, out: 65536,
			levels: []string{"low", "high"}, source: "override", vendor: "stepfun",
		},
		{
			// Context（上游自报 256000）、Output（同族直取 128000）与档位（同族直取
			// off,low,high）三列全覆盖：原本完全无信息。
			id: "tencent/hy3-paid", ctx: 256000, out: 128000,
			levels: []string{"off", "low", "high"}, source: "override", vendor: "tencent",
		},
	})
}

// addedOverrideKeys 是本次覆盖率补齐新增的 7 个 Match 键。防前缀误伤测试要用它构造
// 「没有这些条目」的对照表，因此必须与 modelOverrides 里的实际键保持一致。
var addedOverrideKeys = []string{
	"meituan/longcat-2.0",
	"nvidia/nemotron-3-ultra-550b-a55b",
	"poolside/laguna-s-2.1-free",
	"qwen/qwen3.6-max-preview",
	"stealth/glyph-cluster:free",
	"stepfun/step-3.5-flash",
	"tencent/hy3-paid",
}

// overridesWithoutAddedEntries 返回**去掉**本次新增 7 条之后的覆盖层表，作为对照。
func overridesWithoutAddedEntries() []modelOverride {
	drop := map[string]bool{}
	for _, k := range addedOverrideKeys {
		drop[strings.ToLower(strings.TrimSpace(k))] = true
	}
	out := make([]modelOverride, 0, len(modelOverrides))
	for _, ov := range modelOverrides {
		if drop[strings.ToLower(strings.TrimSpace(ov.Match))] {
			continue
		}
		out = append(out, ov)
	}
	return out
}

// siblingGuardIDs 是每条新增条目可能波及的同族基名 / 邻居。它们的结果都不允许被新增条目
// 改变（见 TestModelOverridesSiblingBasenamesUnchanged）。
//
// 注释里的数字是 2026-10 实测的当前值，**仅作说明、不做断言**：写死快照会因为与新增条目
// 无关的原因（底层表更新、分层顺序调整）变红，那属于别的测试的职责。
var siblingGuardIDs = []string{
	// —— 1. nvidia/nemotron-3-ultra-550b-a55b 的邻居（键带 nvidia/ 前缀，不反向命中基名）——
	"nemotron-3-ultra-550b-a55b",   // 基名：1000000|128000|medium,high
	"nvidia/nemotron-3-ultra",      // 262144|128000|—
	"nvidia/nemotron-3-ultra-550b", // 131072|131072|—

	// —— 2. poolside/laguna-s-2.1-free 的邻居 ——
	"laguna-s-2.1-free",     // 基名：256000|32000|low,medium,high
	"poolside/laguna-s-2.1", // 1048576|131072|—

	// —— 3. stepfun/step-3.5-flash 的邻居 ——
	"step-3.5-flash",         // 基名：256000|256000|low,high
	"stepfun/step-3.7-flash", // 256000|230400|low,medium,high（同版本线的兄弟）

	// —— 4. tencent/hy3-paid 的邻居（同族基名与兄弟档位）——
	"hy3",                 // 基名：256000|128000|off,low,high
	"tencent/hy3",         // 262144|128000|off,low,high
	"tencent/hy3-preview", // 262144|235929|off,low,high

	// —— 5. qwen/qwen3.6-max-preview 的邻居（键带 qwen/ 前缀，不反向命中基名）——
	"qwen3.6-max-preview", // 基名：262144|65536|—
	"Qwen/Qwen3.6-Plus",   // 1000000|65536|off,enabled

	// —— 6. meituan/longcat-2.0 的邻居 ——
	"longcat-2.0",                 // 基名：1000000|131072|—
	"meituan/longcat-2.5-preview", // 1000000|131072|—

	// —— 7. 未新增条目、但必须保持目录原值的邻居 ——
	"inclusionai/ling-3.0-flash-sante", // 256000|32000|—
	// xiaomi/mimo-v2.6-* 家族若按 provider 值写 Context 会外溢到 -flex/-free/-rl 兄弟 id，
	// 故本次**没有**为它们新增条目；这里守住它们仍走目录值。
	"xiaomi/mimo-v2.6-pro-flex", // 1048560|393216|off,high
	"xiaomi/mimo-v2.5",          // 1000000|131072|off,enabled（zcode）
	"xiaomi/mimo-v2.5-pro",      // 1000000|131072|off,enabled（zcode）

	// —— 8. 覆盖率实测里的既有覆盖与既有取值（不该被新条目影响）——
	// 注意：既有的覆盖层条目 Match 是 "deepseek-v4.1-flash"，不带命名空间，因此
	// 网关的原始拼写 deepseek/deepseek-v4.1-flash 命中的是 zcode 正则层而**不是**覆盖层
	// （前缀语义不做命名空间剥离，只有离线目录层才做）。差分断言会同时守住这个事实。
	"deepseek/deepseek-v4.1-flash", // 1000000|384000|off,low,high,max（zcode）
	"deepseek/deepseek-v4-flash",   // 1000000|384000|off,low,high,max（zcode）
	"gpt-5.6-sol",                  // 1050000|128000|off,low,medium,high,xhigh,max（zcode）
}

// TestModelOverridesSiblingBasenamesUnchanged 是「防前缀误伤」回归网。
//
// modelOverride.Match 是**前缀**语义，新增条目天然会波及同前缀的兄弟 id。这里对每个可能
// 被波及的同族基名 / 邻居，比较「完整覆盖层」与「去掉本次新增 7 条后的覆盖层」的逐字段结果
// （Context / Output / 档位 / Source / Vendor），两者必须完全相同；不等就说明我的新增条目
// 伸手碰到了这个 id。
//
// 用**差分**而不是写死快照，原因有二：
//  1. 差分断言的正是真正要保证的性质——「新增条目一个字都没影响到这些 id」，与底层表
//     （zcode 正则 / 离线目录 / presets）的具体数值无关；
//  2. 分层顺序正在被另一条工作流调整（catalog 与 presets 互换位置）。实测把 presets 提到
//     catalog 之前后，step-3.5-flash 的 Source 会由 catalog 变 preset、
//     qwen3.6-max-preview 的 ctx/out 会由目录值变 presets 值（262144|65536 → 256000|65000）
//     ——这两处变动与本覆盖层毫无关系，写死快照会因此误报。
func TestModelOverridesSiblingBasenamesUnchanged(t *testing.T) {
	t.Helper()
	baseline := overridesWithoutAddedEntries()
	if len(baseline) >= len(modelOverrides) {
		t.Fatalf("对照表没有去掉任何条目（modelOverrides=%d 条），防前缀误伤断言会退化成空断言", len(modelOverrides))
	}
	for _, id := range siblingGuardIDs {
		t.Run(id, func(t *testing.T) {
			t.Helper()
			got := lookupCapabilityForSite(id, siteProbeBaseURL, siteProbeAPIType, modelOverrides, modelSiteRules)
			want := lookupCapabilityForSite(id, siteProbeBaseURL, siteProbeAPIType, baseline, modelSiteRules)
			if !overrideSameCapability(got, want) {
				t.Errorf("新增覆盖层条目改变了同族 id %q 的结果：\n  含新增条目   = %s\n  去掉新增条目 = %s",
					id, describeCapability(got), describeCapability(want))
			}
		})
	}
}

// overrideSameCapability 比较两个 ModelCapability 的全部字段。
// 名字特意不与 capability_catalog_test.go 的 sameCapability 重名（后者只比 ctx/out/档位）。
func overrideSameCapability(a, b ModelCapability) bool {
	return a.Context == b.Context && a.Output == b.Output &&
		a.Vendor == b.Vendor && a.Source == b.Source &&
		strings.Join(a.ReasoningLevels, ",") == strings.Join(b.ReasoningLevels, ",")
}

// describeCapability 把一次求值结果渲染成一行，供断言失败时对照。
func describeCapability(cap ModelCapability) string {
	return fmt.Sprintf("ctx=%d out=%d levels=[%s] source=%q vendor=%q",
		cap.Context, cap.Output, strings.Join(cap.ReasoningLevels, ","), cap.Source, cap.Vendor)
}

// TestModelOverridesMatchKeysAreCompleteLowercaseIDs 钉住覆盖层表头的填写规则：
// Match 必须是小写完整 id（前缀语义下，键太短会静默波及一族兄弟模型）。
func TestModelOverridesMatchKeysAreCompleteLowercaseIDs(t *testing.T) {
	t.Helper()
	seen := map[string]bool{}
	for _, ov := range modelOverrides {
		key := ov.Match
		if key == "" || strings.TrimSpace(key) != key {
			t.Errorf("Match 键 %q 不能为空或含首尾空白", key)
			continue
		}
		if key != strings.ToLower(key) {
			t.Errorf("Match 键 %q 必须全小写", key)
		}
		if seen[key] {
			t.Errorf("Match 键 %q 重复", key)
		}
		seen[key] = true
	}
}

// selfReportEvidenceFile 是「上游自报」型覆盖层条目的**本地**证据快照：脱敏只读，只有模型 id
// 与自报的 contextLength，不含任何凭据。它**不入库**（见 .gitignore）——它是从开发者自己的
// 网关取来的数据，仓库里没有它。go test 的工作目录是包目录（internal/core），因此按相对包
// 目录的路径读取。
//
// 它不参与能力表生成（sync-ability-catalog.mjs 不读它），也没有任何运行时代码读取它——
// 唯一的消费者是本文件里的这两个测试，而它们在快照缺失时**整体跳过**（跳过而非失败：
// 仓库里本来就没有这个文件，别人的机器上也不会有）。
const (
	selfReportEvidenceFile     = "data/commandcode-self-report.json"
	selfReportEvidenceRepoPath = "internal/core/data/commandcode-self-report.json"
)

// selfReportEvidence 是证据文件的顶层形状（只声明测试用得到的字段）。
type selfReportEvidence struct {
	Source    string `json:"source"`
	FetchedAt string `json:"fetchedAt"`
	Note      string `json:"note"`
	Models    []struct {
		ID            string `json:"id"`
		ContextLength int    `json:"contextLength"`
	} `json:"models"`
}

// selfReportedContextOverrideKeys 是 modelOverrides 里 Context 来源为「上游自报」的条目键。
//
// 每一条都必须满足「覆盖层 Context == 证据文件里同 id 的 contextLength」。漏登记、改了一边
// 忘了另一边，都会让 TestModelOverridesSelfReportedContextMatchesEvidence 立刻失败；
// 将来新增「上游自报」型条目时也必须登记到这里（model_overrides.go 的注释里有同样要求）。
var selfReportedContextOverrideKeys = []string{
	"meituan/longcat-2.0",
	"nvidia/nemotron-3-ultra-550b-a55b",
	"qwen/qwen3.6-max-preview",
	"stealth/glyph-cluster:free",
	"tencent/hy3-paid",
}

// readSelfReportEvidenceRaw 只读地取出证据快照的原始字节（本测试永不写回该文件）。
// 快照不入库，因此缺失时调用方整体跳过；只有「文件在、但读不动或读空了」才算失败。
func readSelfReportEvidenceRaw(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(selfReportEvidenceFile)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skipf("本地证据快照 %s 不存在（素材不入库，见 .gitignore），跳过依赖它的断言",
			selfReportEvidenceRepoPath)
	}
	if err != nil {
		t.Fatalf("读取证据文件 %s 失败：%v", selfReportEvidenceRepoPath, err)
	}
	if len(raw) == 0 {
		t.Fatalf("证据文件 %s 为空", selfReportEvidenceRepoPath)
	}
	return raw
}

// loadSelfReportEvidence 解析证据文件并检查元数据齐备。
func loadSelfReportEvidence(t *testing.T) selfReportEvidence {
	t.Helper()
	var ev selfReportEvidence
	if err := json.Unmarshal(readSelfReportEvidenceRaw(t), &ev); err != nil {
		t.Fatalf("解析证据文件 %s 失败：%v", selfReportEvidenceRepoPath, err)
	}
	if strings.TrimSpace(ev.Source) == "" || strings.TrimSpace(ev.FetchedAt) == "" || strings.TrimSpace(ev.Note) == "" {
		t.Errorf("证据文件 %s 缺少 source / fetchedAt / note 元数据", selfReportEvidenceRepoPath)
	}
	if len(ev.Models) == 0 {
		t.Fatalf("证据文件 %s 的 models 为空", selfReportEvidenceRepoPath)
	}
	return ev
}

// overrideByExactKey 在覆盖层里按键取条目：大小写不敏感，但要求**整个键相同**，
// 不接受前缀命中——证据链必须系在「同一个 id」上，不能靠邻居条目蒙对。
func overrideByExactKey(key string) (modelOverride, bool) {
	for _, ov := range modelOverrides {
		if strings.EqualFold(strings.TrimSpace(ov.Match), strings.TrimSpace(key)) {
			return ov, true
		}
	}
	return modelOverride{}, false
}

// TestModelOverridesSelfReportedContextMatchesEvidence 把「上游自报」型覆盖层的 Context
// 钉死在仓库内的证据文件上，替代只写在注释里的口头依据。
//
// 逐条断言：
//  1. 覆盖层里存在键完全相同的条目（不是靠前缀命中的邻居）；
//  2. matchOverride(key) 解析回同一条目（「全小写完整 id 精确匹配优先」仍然成立）；
//  3. 该条目的 Context == 证据文件里同 id 的 contextLength，且证据值 > 0；
//  4. 该 id 在证据文件里只出现一次（两份读数会让证据自相矛盾）。
func TestModelOverridesSelfReportedContextMatchesEvidence(t *testing.T) {
	t.Helper()
	ev := loadSelfReportEvidence(t)

	counts := map[string]int{}
	lengths := map[string]int{}
	for _, m := range ev.Models {
		id := strings.ToLower(strings.TrimSpace(m.ID))
		if id == "" {
			t.Errorf("证据文件 %s 里存在空的模型 id", selfReportEvidenceRepoPath)
			continue
		}
		counts[id]++
		lengths[id] = m.ContextLength
	}

	for _, key := range selfReportedContextOverrideKeys {
		t.Run(key, func(t *testing.T) {
			t.Helper()
			ov, found := overrideByExactKey(key)
			if !found {
				t.Fatalf("覆盖层里没有键完全等于 %q 的条目，证据链断裂", key)
			}
			if resolved, resolved2 := matchOverride(key, modelOverrides); !resolved2 || !strings.EqualFold(resolved.Match, key) {
				t.Errorf("matchOverride(%q) 解析到 %q，期望解析回同键条目", key, resolved.Match)
			}
			want, seen := lengths[key]
			if !seen {
				t.Fatalf("证据文件 %s 里没有 id %q（Context 的来源无法追溯）", selfReportEvidenceRepoPath, key)
			}
			if counts[key] != 1 {
				t.Fatalf("证据文件里 id %q 出现 %d 次，读数必须唯一", key, counts[key])
			}
			if want <= 0 {
				t.Fatalf("证据文件里 %q 的 contextLength 非法：%d", key, want)
			}
			if ov.Context != want {
				t.Errorf("覆盖层 Context = %d，证据文件 %s → %q.contextLength = %d，两者必须一致",
					ov.Context, selfReportEvidenceRepoPath, key, want)
			}
		})
	}
}

// credentialKeyFragments 是疑似凭据字段的键名片段（大小写不敏感）。
var credentialKeyFragments = []string{"key", "token", "secret", "authorization", "password", "credential"}

// credentialValuePatterns 是疑似凭据的取值形态：OpenAI 风格前缀、Bearer 认证头，
// 以及 32 位以上的 base64 / hex 串（典型 API Key 的长度与熵）。
var credentialValuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-`),
	regexp.MustCompile(`(?i)bearer`),
	regexp.MustCompile(`[A-Za-z0-9+/]{32,}`),
	regexp.MustCompile(`[A-Fa-f0-9]{32,}`),
}

// credentialScan 是一次凭据扫描的结果：命中的疑似凭据描述，以及扫描覆盖到的节点数。
type credentialScan struct {
	Findings []string
	Keys     int
	Strings  int
}

// scanForCredentials 递归扫描一个已解析的 JSON 文档，返回疑似凭据的键名与取值。
//
// 抽成纯函数（只吃 interface{}、不碰 testing）是为了让单测能喂合成文档，验证「扫描本身
// 真的会命中」——否则一个永远返回空结果的扫描器也能让证据文件测试通过。
func scanForCredentials(doc interface{}) credentialScan {
	result := credentialScan{}
	var walk func(path string, value interface{})
	walk = func(path string, value interface{}) {
		switch typed := value.(type) {
		case map[string]interface{}:
			for k, child := range typed {
				result.Keys++
				lower := strings.ToLower(k)
				for _, frag := range credentialKeyFragments {
					if strings.Contains(lower, frag) {
						result.Findings = append(result.Findings,
							fmt.Sprintf("键 %s 疑似凭据字段（含 %q）", path+"."+k, frag))
					}
				}
				walk(path+"."+k, child)
			}
		case []interface{}:
			for i, child := range typed {
				walk(fmt.Sprintf("%s[%d]", path, i), child)
			}
		case string:
			result.Strings++
			for _, re := range credentialValuePatterns {
				if m := re.FindString(typed); m != "" {
					result.Findings = append(result.Findings, fmt.Sprintf("值 %s 疑似凭据（匹配 %s）：%q",
						path, re.String(), truncateForTest(m, 12)))
				}
			}
		}
	}
	walk("$", doc)
	return result
}

// TestCommandcodeSelfReportEvidenceHasNoCredentials 把证据文件本身钉进 CI：任意键名与字符串
// 值都不允许出现疑似凭据。这样「往快照里手滑粘一个 API Key」会被测试直接挡住。
//
// 合法内容不会误伤：最长的一段连续 base64 字符是 "thinkingmachines/inkling"（25 位，短于阈值），
// 而 id 里的 "-" / "." / ":" 会切断这类连续段。
func TestCommandcodeSelfReportEvidenceHasNoCredentials(t *testing.T) {
	t.Helper()
	var doc interface{}
	if err := json.Unmarshal(readSelfReportEvidenceRaw(t), &doc); err != nil {
		t.Fatalf("证据文件 %s 不是合法 JSON：%v", selfReportEvidenceRepoPath, err)
	}
	if doc == nil {
		t.Fatalf("证据文件 %s 解析结果为空", selfReportEvidenceRepoPath)
	}

	scan := scanForCredentials(doc)
	for _, finding := range scan.Findings {
		t.Errorf("证据文件 %s %s", selfReportEvidenceRepoPath, finding)
	}
	// 覆盖度自检：文档必须真的被扫到了内容，否则空扫描也会「通过」。
	if scan.Keys == 0 || scan.Strings == 0 {
		t.Errorf("凭据扫描没有覆盖到内容（键 %d 个 / 字符串 %d 个），扫描本身可能失效",
			scan.Keys, scan.Strings)
	}
}

// TestCredentialScanDetectsPlantedSecrets 是上一条测试的「反向自检」：喂一份**合成**文档，
// 里面种下五种典型凭据形态，扫描器必须逐一报出来。它是纯内存测试，不读也不写任何文件。
//
// 没有它的话，把 credentialKeyFragments / credentialValuePatterns 改空也能让证据文件测试通过，
// 而「没有凭据」这条不变量就变成了空断言。
func TestCredentialScanDetectsPlantedSecrets(t *testing.T) {
	t.Helper()
	synthetic := map[string]interface{}{
		"apiKey":               "placeholder-not-a-real-credential",
		"authorization_header": "Bearer placeholder-token-value",
		"nested": map[string]interface{}{
			"client_secret": "placeholder",
			"models": []interface{}{
				"sk-placeholderplaceholderplaceholder",
				"0123456789abcdef0123456789abcdef",
			},
		},
	}
	scan := scanForCredentials(synthetic)
	if scan.Keys == 0 || scan.Strings == 0 {
		t.Fatalf("合成文档没有被扫到（键 %d / 字符串 %d）", scan.Keys, scan.Strings)
	}
	// 期望命中：apiKey、authorization_header、client_secret 三个键名，
	// 以及 sk- 前缀、Bearer、32 位 base64、32 位 hex 四种取值形态。
	if len(scan.Findings) < 7 {
		t.Fatalf("合成文档应当报出至少 7 项疑似凭据，实际 %d 项：%v", len(scan.Findings), scan.Findings)
	}
	for _, want := range []string{
		"apiKey", "authorization_header", "client_secret",
		"sk-", "bearer", "[a-fa-f0-9]{32,}", "[a-za-z0-9+/]{32,}",
	} {
		matched := false
		for _, finding := range scan.Findings {
			if strings.Contains(strings.ToLower(finding), strings.ToLower(want)) {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("扫描结果未包含对 %q 的命中：%v", want, scan.Findings)
		}
	}
}

// truncateForTest 把疑似凭据的匹配片段截短后再打印，避免把可能是真凭据的内容整段写进日志。
func truncateForTest(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
