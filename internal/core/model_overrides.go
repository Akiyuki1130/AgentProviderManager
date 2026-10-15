package core

import "strings"

// modelOverride 是 APM 自维护的能力覆盖层，优先于 ZCode 内建数据（model_catalog_gen.go）。
//
// 这个表是**给人手改的**：当脚本抽不到的家族、内建数据在 ZCode 升级后改动、或 APM 期望
// 与客户端口径不同时，在这里显式补齐/修正。语义是「**显式给出即胜出**」——某个字段填了
// （Context/Output > 0，Levels 非空，Features 有任一已知位，Vendor 非空）就覆盖后续来源
// （zcode 正则、presets 旧表）；留空则直接沿用后续来源，不做「清零」。
//
// Features 列与 ctx/out 的语义完全相同：显式给出即胜出、留空不覆盖（层间是「前层胜出、
// 后层填空缺」，见 mergeFeatures）。**只有可公开核实的位才填**：这个表会被当成事实写进
// 用户配置，编出来的能力位（例如凭空断言某模型支持原生联网搜索）比留空更糟。
//
// 模态词必须落在 model_features.go 的规范词表内（输入 text,image,video,audio,pdf；
// 输出 text,audio,image,video,pdf）：并入前会过一遍 NormalizeInputModalities /
// NormalizeOutputModalities，**规范词表外的词会被直接丢弃**（包括 image/png 这类未登记的
// MIME 写法），而乱序与重复会被自动排序去重，所以只有「写错词」才会丢信息。布尔位一律三态：
// 只写「已知为真」，未知留 nil，绝不写 false（false 意为「已知为假」，会被后续层原样保留）。
//
// Match 的匹配规则与 presets.go 的 LookupModelLimits 一致：小写精确 ID 优先，其次最长前缀
// （覆盖带日期/后缀的快照 id）。**请在填写前确认数值可公开核实，不确定的模型不要编。**
//
// ⚠️ 前缀匹配的坑（修 S2）：`Match` 是**前缀**语义，键太短会静默波及一族兄弟模型——例如
// `Match: "gpt-5"` 会连带命中 `gpt-5.6-luna`、`gpt-5.4-pro` 等。因此：**优先用完整 ID**；
// 确需前缀覆盖整个家族时，键要足够长且该家族各成员的 ctx/out/档位确实一致。
type modelOverride struct {
	Match    string        // 精确 ID（小写）或最长前缀
	Context  int           // 0 = 不覆盖
	Output   int           // 0 = 不覆盖
	Levels   []string      // 原始拼写，交给 NormalizeVariants；nil = 不覆盖
	Vendor   string        // 空 = 不覆盖
	Features ModelFeatures // 零值 = 不覆盖；模态列并入前自动归一，只填可公开核实的位
}

// normalizeOverrideFeatures 把覆盖层手写的模态列收进规范词表：未知词丢弃、按规范序排序去重
// （布尔三态原样保留）。
//
// 为什么要在并入前归一，而不是信任手写值：生成表那条路径已经在 parseFeatureEncoding 内做过
// 同一件事，手写条目若走另一套口径就会出现「同一个模型从两张表读出的模态集合不可比」；
// 归一后「只写了未知词」的条目会整条变空，因而与「未给出」完全等价——调用方据此判定它没有
// 贡献能力位（fail-closed，既不带入未知模态，也不该因它标注来源）。
func normalizeOverrideFeatures(f ModelFeatures) ModelFeatures {
	f.InputModalities = NormalizeInputModalities(f.InputModalities)
	f.OutputModalities = NormalizeOutputModalities(f.OutputModalities)
	return f
}

// modelOverrides 是覆盖层数据。
//
// 第一条是用户点名的 deepseek-v4.1-flash：它虽已被内建正则（modelRules 的
// `.*deepseek-v4[.-]1-flash`）覆盖，但该家族此前长期落不进旧 presets 表，这里显式固化
// 一份作为「内建数据将来变动时的保险」，同时作为覆盖层可用性的活样例。
//
// 其余条目是 90 个 commandcode 中转 id 的覆盖率实测补齐：只挑「既有各层全都给不出档位」
// 或「完全无信息」的 id。每个字段的来源类别都写在条目上方的注释里，只有两类：
//
//	[同族直取] 该 id 显然是某个已覆盖模型的纯后缀 / 命名空间 / 大小写变体，
//	           字段值直接取自该族已覆盖成员（多来源时取互相一致的那一组）。
//	[上游自报] 中转 /models 返回的 context_length，与离线目录给的值实质冲突
//	           （不是「十进制 1M/256K」与「二进制 1MiB/256KiB」这种同一窗口的两种写法）
//	           时，按 APM 既有契约取 provider 值。这类条目的 Context 一律附上证据读数
//	           （网关 /v1/models 自报，逐条写在条目注释里）。
//	           该自报的**脱敏快照**（只含模型 id 与 contextLength，不含任何凭据）存放在
//	           开发者本地 internal/core/data/commandcode-self-report.json，**本地存档、
//	           不入库**（见 .gitignore）。它不参与能力表生成（sync-ability-catalog.mjs
//	           不读它），也没有任何运行时代码读取它，改动它不会改变任何运行时行为；
//	           唯一的消费者是 model_overrides_test.go——快照在本地时那里逐条断言
//	           「覆盖层 Context == 快照 contextLength」，不在时整条断言跳过。
//
// 没有实证的字段一律留 0 / nil（= 不覆盖，继续沿用后面的层），宁可留空也不编。
var modelOverrides = []modelOverride{
	{Match: "deepseek-v4.1-flash", Context: 1000000, Output: 384000, Levels: []string{"disabled", "low", "high", "max"}, Vendor: "deepseek"},

	// [上游自报] 目录 meituan/longcat-2.0 给 1048756，与中转自报的 262144 差 4 倍，无法调和
	// → 取 provider 值。（目录该行的 output 列恰好也是 262144，同族其它条目则写 1000000，
	// 说明目录这一行本身就不可信，更加支持以 provider 为准。）档位无同族实证，留 nil。
	// 证据：网关自报快照 → "meituan/LongCat-2.0".contextLength
	// （262144。provider 自报的是 context_length，与目录该行的 output 列无关——两者同为
	//   262144 只是巧合，本条 Context 只以 provider 自报为据。）
	{Match: "meituan/longcat-2.0", Context: 262144},

	// [上游自报] 目录 nvidia/nemotron-3-ultra-550b-a55b 给 1000000，中转自报 524288，差 2 倍
	// → 取 provider 值（同族目录值本身互相矛盾：262144 / 256000 / 202800 都有）。
	// [同族直取] 档位取自「同名去命名空间」后的 nemotron-3-ultra-550b-a55b 家族：
	// 基名条目给 medium,high，其 :free 兄弟给 none,medium,high（none 折成 off），
	// 同族 nvidia-nemotron-3-ultra / nemotron-3-ultra-nvfp4 也含 none(off)，
	// 故取并集中被多处证实的 off,medium,high。
	//
	// 证据：网关自报快照 → "nvidia/nemotron-3-ultra-550b-a55b".contextLength
	//       （524288，只用于本条的 Context；档位来自下面的同族直取，与证据文件无关。）
	//
	// 注意：匹配是前缀语义，本键同时命中 nvidia/nemotron-3-ultra-550b-a55b:free
	// （目录 1000000|65536|off,medium,high）。档位取成 off,medium,high 后该兄弟 id 的档位不变，
	// 只有 Context 会被一并改成 524288——这与 provider 对同一网关同一模型的报价一致。
	{Match: "nvidia/nemotron-3-ultra-550b-a55b", Context: 524288, Levels: []string{"off", "medium", "high"}},

	// [同族直取] poolside/laguna-s-2.1-free 去掉命名空间后与目录 laguna-s-2.1-free
	// （256000|32000|low,medium,high）同名，档位直取 low,medium,high。
	// Context 已被目录给成 256000（与中转自报的 256000 一致），Output 目录本行 32768 与
	// 同族 32000 是同一「32K」窗口的两种写法，故两列都不覆盖。
	{Match: "poolside/laguna-s-2.1-free", Levels: []string{"low", "medium", "high"}},

	// [上游自报] 目录 qwen3.6-max-preview 给 262144|65536（旧 presets 表另给 256000|65000），
	// 中转自报 1000000，差 4 倍且无第三来源可仲裁 → 按契约取 provider 值。
	// 本条目是本次补齐里最可疑的一条（两处离线表都说 ~256K），档位无任何来源，留 nil。
	// 证据：网关自报快照 → "Qwen/Qwen3.6-Max-Preview".contextLength
	{Match: "qwen/qwen3.6-max-preview", Context: 1000000},

	// [上游自报] 离线目录里没有任何 glyph / glyph-cluster 条目，内建正则与 presets 也都不命中，
	// 唯一可用事实就是中转自报的 256000。Output 与档位无来源，留 0 / nil。
	// 证据：网关自报快照 → "stealth/glyph-cluster:free".contextLength
	{Match: "stealth/glyph-cluster:free", Context: 256000},

	// [同族直取] stepfun/step-3.5-flash 去掉命名空间后与目录 step-3.5-flash 同名，
	// 后者（及 step-3.5-flash-2603）一致给出 256000|256000|low,high → 档位直取 low,high。
	// Context 目录本行 262144 与中转自报 256000 是同一「256K」窗口的两种写法，不覆盖。
	{Match: "stepfun/step-3.5-flash", Levels: []string{"low", "high"}},

	// [同族直取] tencent/hy3-paid 是 hy3 家族的付费档（-paid 后缀），
	// 目录 hy3（256000|128000|off,low,high）与 tencent/hy3（262144|128000|off,low,high）
	// 两处一致给出 Output=128000 与档位 off,low,high → 直取该族这两列。
	// [上游自报] Context 取 provider 的 256000（目录 hy3 也给 256000；tencent/hy3 给 262144，
	// 属同一「256K」窗口的两种写法，取 provider 值即可）。
	// 证据：网关自报快照 → "tencent/hy3-paid".contextLength
	{Match: "tencent/hy3-paid", Context: 256000, Output: 128000, Levels: []string{"off", "low", "high"}},
}

// matchOverride 在覆盖层里查 modelID：精确 ID（小写）优先，其次最长前缀。命中返回该条目。
func matchOverride(modelID string, overrides []modelOverride) (modelOverride, bool) {
	low := strings.ToLower(strings.TrimSpace(modelID))
	if low == "" {
		return modelOverride{}, false
	}
	for _, ov := range overrides {
		if strings.ToLower(strings.TrimSpace(ov.Match)) == low {
			return ov, true
		}
	}
	best := modelOverride{}
	bestLen := 0
	for _, ov := range overrides {
		key := strings.ToLower(strings.TrimSpace(ov.Match))
		if key == "" {
			continue
		}
		if len(key) > bestLen && strings.HasPrefix(low, key) {
			best, bestLen = ov, len(key)
		}
	}
	if bestLen == 0 {
		return modelOverride{}, false
	}
	return best, true
}
