package core

import (
	"regexp"
	"strings"
)

// catalogRule 是 ZCode 内建模型能力规则（来自 model_catalog_gen.go）的一条。
//
// 字段顺序与名称是与生成文件（另一个 Agent 负责产出）之间的共享契约，不得改动：
// 生成文件只声明 `var modelCatalogRules = []catalogRule{...}`，类型由本文件声明。
//
// 语义（阶段 B 硬事实）：ZCode 的规则表是「全表顺序扫描 + 逐字段覆盖」，不是
// 「取第一条命中」。Context/Output 为 0 表示该规则未给出该字段；Levels 为 nil
// 表示该规则未给出档位。未出现的字段必须保持前面规则带来的值。
type catalogRule struct {
	Pattern string
	Re      *regexp.Regexp
	Context int      // 0 = 该规则未给出
	Output  int      // 0 = 该规则未给出
	Levels  []string // nil = 该规则未给出；原样字符串，未规范化
	// Features 用与生成物 capability_feature 表**同一套**紧凑编码（`<input>|<output>|<flags>`，
	// 契约与 fail-closed 规则见 capability_features.go 的 parseFeatureEncoding）；空串表示
	// 该规则未给出能力位。合并时与 Context/Output/Levels 一样「后命中覆盖」，解析失败按未给出处理。
	Features string
}

// ModelCapability 是从内建能力表推断出的模型能力，仅供 APM 内部使用（展示/分组/
// 档位自适应），绝不写回任何配置。
type ModelCapability struct {
	Context         int      // 0 = 未知
	Output          int      // 0 = 未知
	ReasoningLevels []string // 规范化 + 去重 + 按全集顺序排序；nil 表示未推断出
	// Features 是 harness 中立的能力位（模态 / 工具调用 / 结构化输出…），与 Context/Output/
	// ReasoningLevels 走同一套「前层胜出、后层填空缺」的分层合并（类型与合并语义见
	// model_features.go）。零值 = 无能力信息，不落盘。
	Features ModelFeatures
	Vendor   string // 未命中原样返回 "unknown"
	// Source 记录「贡献了能力字段（ctx/out/levels/features）的最高优先级来源」；全空时为空串
	// （唯一例外：空 modelID 的兜底条目，其 Source 固定为 "preset"）。features 走与 ctx/out
	// 同一套逐字段合并语义，因此「某一层只给出 features」同样算它贡献了能力字段。
	// 取值集合（按优先级降序）：
	//   "provider" —— 调用方直接给出的 API 事实（不在本文件求值，代表卡片补全路径上的最高层）；
	//   "override" —— APM 自维护的覆盖层（model_overrides.go）；
	//   "site"     —— ZCode 内建站点级规则（model_site_rules.go）；
	//   "zcode"    —— ZCode 内建模型名正则表（model_catalog_gen.go）；
	//   "catalog"  —— 离线精确 id 目录（capability_catalog.go）；
	//   "preset"   —— presets.go 的手维护旧表。
	// 供测试/调试；分层求值顺序见 lookupCapabilityForSite。
	Source string
}

// LookupModelCapability 按 override > site > zcode 正则全表覆盖 > presets 旧表 > catalog 离线目录
// 的顺序求值（后四层都只补前面各层留下的空字段，见 lookupCapabilityForSite；本入口不带站点
// 上下文，因此 site 层必然不命中，等价于「override 之后直接进 zcode 层」）。
//
// 返回值语义：除 modelID 为空外**始终返回 ok==true**（与 presets 时代的 false 语义
// 区分开）。调用方用「Context==0 && Output==0 && len(ReasoningLevels)==0」判断「无信息」。
// modelID 为空时返回 (ModelCapability{Vendor:"unknown", Source:"preset"}, false)。
//
// 与前 catalog 时代的行为差异（本次改造的**预期**变化）：此前完全匹配不到的 id 现在可能
// 从离线目录拿到 ctx/out/档位（Source 记为 "catalog"），补全命中率因此提高。反过来，
// 任何**原本已由 override、内建正则或 presets 旧表给出**的字段值都不会被离线目录改写——
// 目录排在它们之后，只填空缺。这一点由 capability_regression_test.go 的
// TestNewLayersNeverChangePreviouslyKnownFields 在全键集上逐 id 固化。
func LookupModelCapability(modelID string) (ModelCapability, bool) {
	if strings.TrimSpace(modelID) == "" {
		return ModelCapability{Vendor: "unknown", Source: "preset"}, false
	}
	return lookupCapability(modelID, modelOverrides), true
}

// LookupModelCapabilityForSite 是带站点上下文的求值入口，分层为
//
//	override → site → zcode 正则 → presets 旧表 → catalog 离线目录
//
// 各层的取舍理由：
//   - override 是 APM 手维护的修正，最高优先；
//   - site 是站点级规则：它要同时满足模型名 + baseUrl（+ apiType）三道门，是 ZCode 自身
//     **最具体**的一层事实——ZCode 的规则数组把 providerSiteRules 拼接在 modelRules /
//     modelApiRules 之后，resolve() 顺序遍历、命中即 overlay，因此站点规则在 ZCode 里
//     真实地覆盖家族正则（由 capability_site_test.go 的回归用例在真实生成表上固定）。
//     它之所以必须排在家族正则之前，还有一层具体后果：输入侧 implied text 让家族层的模态
//     集合恒非空（text 缺键即真），而模态是整字段「前层胜出」，站点层的模态集合就再也无法
//     胜出。实测（目录 2305 个 id）：在 zcode.z.ai/api/v1/highspeed/anthropic（以及同类不限定
//     apiType 的 zcode-plan / api.z.ai / open.bigmodel.cn 规则）下，422 个 id 的输入模态被
//     家族层的 [text] 压掉，站点规则明确写出的 image+video 无法生效；api.deepseek.com 下
//     deepseek-v4-flash / deepseek-v4-flash-vision-exp 同样丢掉站点规则给出的 image；
//   - zcode 内建 modelRules 是「既有正确值」的既有来源，且 APM 写出的配置正是给
//     ZCode / DSH 消费的，ZCode 内建值本身就是那个 harness 的真正口径，因此必须排在
//     社区数据（catalog）之前；
//   - presets 是 APM 手维护的旧表，代表「此前发布物的既有行为」，社区快照同样不该静默
//     改写它，因此排在 catalog 之前；
//   - catalog 是第三方社区补充（models.dev / pi-ai 快照），**最后兜底**：它只逐字段补空，
//     永远无法改写上面任何一层已经给出的值（实测社区数据存在 272000 vs 1050000、
//     262000 vs 262144、128000 vs 8192 这类离群差异，压过任何既有来源都会把原本正确的
//     值改坏）。
//
// 每层都是**逐字段覆盖**：前一层已经给出某个字段（Context/Output > 0、ReasoningLevels
// 非 nil、Features 有任一已知位）时，后面的层不再改写它；该层未给出的字段继续往后找。
// 因为 catalog 被放在最后，所以「零回归」是按构造成立的，而不是靠约定；且覆盖面不损失——
// 它仍能逐字段补空，包括 presets 结构里根本没有的档位列。
//
// **features（模态 / 工具调用 / 结构化输出…）完全走上面这同一套逐字段合并语义**：每层
// 先用 mergeFeatures（前层胜出、后层填空缺）把它合进 cap.Features，层内则用 overlayFeatures
// 做「后命中覆盖」。唯一没有 features 列的是 presets 旧表（ModelPreset 只有 ctx/out），
// 它自然不参与本列。
//
// 站点层的两道门仍由 matchSiteRulesWith 负责且**未放宽**：baseURL 为空或不是合法绝对
// URL 时整层不命中（这是既有契约）；规则写了 apiTypeMatch 而 apiType 为空时该规则被跳过。
// 关于 apiType：Go 侧把空串视作「未知」，因此限定 apiType 的规则一律不参与——这比 ZCode
// 更保守（ZCode 在 apiType == null 时会跳过整层站点规则，而这里仍会应用未限定 apiType 的
// 规则；那几条都不给能力字段，实际影响为零）。
//
// baseURL 为空时的结果与 LookupModelCapability 完全一致（站点层不命中，其余层相同）。
func LookupModelCapabilityForSite(modelID, baseURL, apiType string) ModelCapability {
	if strings.TrimSpace(modelID) == "" {
		// 与 LookupModelCapability 的空 ID 分支保持一致：Source 固定 "preset"。
		return ModelCapability{Vendor: "unknown", Source: "preset"}
	}
	return lookupCapabilityForSite(modelID, baseURL, apiType, modelOverrides, modelSiteRules)
}

// lookupCapability 是 LookupModelCapability 的可注入版本（覆盖层来源可替换），
// 便于单测构造临时覆盖条目验证「override 显式给出即胜出」。overrides 传
// modelOverrides 即等价于公开函数。
//
// 实现上就是「没有站点上下文的分层求值」：baseURL/apiType 皆为空、站点规则表为 nil，
// site 层必然不命中，其余各层与 lookupCapabilityForSite 完全共用同一段实现，
// 不会出现两份逻辑漂移。
func lookupCapability(modelID string, overrides []modelOverride) ModelCapability {
	return lookupCapabilityForSite(modelID, "", "", overrides, nil)
}

// lookupCapabilityForSite 是分层求值的唯一实现（rules 传 nil 即关闭站点层）。
//
// 求值顺序（逐字段覆盖，前层已给出的字段不再被后层改写）：
//  1. override 层：精确 ID 优先、最长前缀次之；命中字段直接采用（0/nil 表示不覆盖，
//     留给后续来源补），符合「显式给出即胜出」。
//  2. site 层：内建站点级规则，模型名 / baseUrl / apiType 三正则同时命中才适用；
//     baseURL 为空时整层不命中。它排在家族正则之前，因为 ZCode 自己就是这么应用的
//     （providerSiteRules 在规则数组里拼在 modelRules 之后，后命中者 overlay 前命中者）。
//  3. zcode 层：内建模型名正则全表顺序扫描 + 逐字段覆盖（见 mergeCatalog）。
//  4. presets 层：presets.go 旧表（APM 手维护、代表已发布行为），Context/Output 两列
//     各自独立补。
//  5. catalog 层：离线精确 id 目录（容错允许去掉命名空间前缀），**最后兜底且无条件参与**，
//     只补前面各层留下的空字段；它是唯一能提供「仅存在于社区数据里」的档位的层。
//
// 每一层的 features 走与 ctx/out/levels 完全相同的逐字段合并语义（层间 mergeFeatures、
// 层内 overlayFeatures），因此顺序结论对 features 一字不差地成立。
func lookupCapabilityForSite(modelID, baseURL, apiType string, overrides []modelOverride, rules []siteRule) ModelCapability {
	id := strings.TrimSpace(modelID)
	cap := ModelCapability{Vendor: "unknown"}

	// 1. override 层（最高优先，逐字段覆盖）。
	// Source 只在 override **真的贡献了能力字段**（ctx/out/levels/features）时才标 override；
	// 仅命中一个只给 Vendor 的条目不算贡献（修 S1：避免把「正则给的值」误标成 override）。
	if ov, ok := matchOverride(id, overrides); ok {
		if ov.Context > 0 {
			cap.Context = ov.Context
			cap.Source = "override"
		}
		if ov.Output > 0 {
			cap.Output = ov.Output
			cap.Source = "override"
		}
		if len(ov.Levels) > 0 {
			cap.ReasoningLevels = normalizeCapabilityLevels(ov.Levels)
			cap.Source = "override"
		}
		// 只给 features 的条目也算贡献（否则「override 显式给出了能力位却标成 zcode」）。
		// 模态列并入前先归一到规范词表（见 normalizeOverrideFeatures）：归一后可能整条变空
		// （条目只写了未知词），那就与「未给出」等价，既不带入任何位也不据此标 override。
		if ovFeatures := normalizeOverrideFeatures(ov.Features); ovFeatures.HasAny() {
			cap.Features = mergeFeatures(cap.Features, ovFeatures)
			cap.Source = "override"
		}
		if v := strings.TrimSpace(ov.Vendor); v != "" {
			cap.Vendor = v
		}
	}

	// 2. site 层：baseURL 为空、或 apiType 不满足规则门时整层不命中。
	// 排在 zcode 家族层之前：站点规则是 ZCode 按 baseUrl/apiType 定向的网关调参，在 ZCode 的
	// 规则数组里拼在 modelRules 之后、由 overlay 覆盖家族值，因此它在 APM 里也必须是比家族
	// 正则更具体的 ZCode 事实。否则家族层恒非空的输入模态会整字段压掉站点层的模态集合。
	site := matchSiteRulesWith(id, baseURL, apiType, rules)
	if cap.Context == 0 {
		cap.Context = site.Context
	}
	if cap.Output == 0 {
		cap.Output = site.Output
	}
	if cap.ReasoningLevels == nil {
		cap.ReasoningLevels = site.ReasoningLevels
	}
	cap.Features = mergeFeatures(cap.Features, site.Features)
	if cap.Source == "" && (site.Context > 0 || site.Output > 0 || site.ReasoningLevels != nil || site.Features.HasAny()) {
		cap.Source = siteSource
	}

	// 3. zcode 正则全表顺序覆盖（既有正确值的既有来源，必须排在社区目录之前，但排在站点层
	// 这个更具体的 ZCode 事实之后）。
	zcode := mergeCatalog(id, modelCatalogRules)
	if cap.Context == 0 {
		cap.Context = zcode.Context
	}
	if cap.Output == 0 {
		cap.Output = zcode.Output
	}
	if cap.ReasoningLevels == nil {
		cap.ReasoningLevels = zcode.ReasoningLevels
	}
	// features 逐字段补空：内建正则给出的位不得被后面的目录层改写，但它也改不动站点层
	// 已经给出的位。
	cap.Features = mergeFeatures(cap.Features, zcode.Features)
	if cap.Source == "" && (zcode.Context > 0 || zcode.Output > 0 || zcode.ReasoningLevels != nil || zcode.Features.HasAny()) {
		cap.Source = "zcode"
	}

	// 4. presets 层：APM 手维护的旧表。它代表「此前发布物的既有行为」，因此必须排在社区
	// 目录之前——社区快照不该静默改写我们已经发布出去的取值。只补前面几层留下的空字段，
	// Context/Output 两列各自独立补（不因一方已有值就丢弃另一方）。
	// 本层不参与 features：ModelPreset 结构里只有 ctx/out 两列，没有能力位可给。
	if cap.Context == 0 || cap.Output == 0 {
		if p, ok := LookupModelLimits(id); ok {
			if cap.Context == 0 && p.Context > 0 {
				cap.Context = p.Context
			}
			if cap.Output == 0 && p.Output > 0 {
				cap.Output = p.Output
			}
			if cap.Source == "" {
				cap.Source = "preset"
			}
		}
	}

	// 5. catalog 层：社区离线目录，**最后兜底**且无条件参与。它对前面各层已经给出的字段
	// 一律不改写，因此社区数据的离群值（实测 272000 vs 1050000、262000 vs 262144 之类）
	// 既压不过 ZCode 内建值，也压不过 APM 手维护的 presets；它的价值在于逐字段补空——
	// 包括 presets 结构里根本不存在的档位列——让此前完全匹配不到的 id 也有值。
	cat := lookupCatalogTolerant(id)
	if cap.Context == 0 {
		cap.Context = cat.Context
	}
	if cap.Output == 0 {
		cap.Output = cat.Output
	}
	if cap.ReasoningLevels == nil {
		cap.ReasoningLevels = cat.ReasoningLevels
	}
	// features 来自与上表彼此独立的 capabilityFeature 表，因此这里单独查一次（同一条
	// 「容错精确查询」口径），而不是从 cat 里取。它对前面各层已给出的位一律不改写。
	catFeatures := lookupCatalogFeatures(id)
	cap.Features = mergeFeatures(cap.Features, catFeatures)
	if cap.Source == "" && (cat.Context > 0 || cat.Output > 0 || cat.ReasoningLevels != nil || catFeatures.HasAny()) {
		cap.Source = catalogSource
	}

	// 厂商判定独立于补全来源：即使 ctx/out/levels 全空也要给出厂商（或 unknown）。
	if cap.Vendor == "" || cap.Vendor == "unknown" {
		cap.Vendor = detectVendor(id)
	}
	return cap
}

// lookupCatalogTolerant 在离线目录（capability_catalog.go）里做「容错精确查询」：
//  1. 先按小写精确 id 查一次（lookupCatalogExact）；
//  2. 未命中且 id 含 "/" 时，只取最后一个 "/" 之后的一段（丢掉全部命名空间前缀，
//     例如 Qwen/Qwen3.8-Max → qwen3.8-max、meta/muse-spark-1.3 → muse-spark-1.3）再查一次；
//  3. **只在第二次命中时才采用**（fail-closed，绝不猜）；两次都未命中返回零值。
//
// 中间层不做任何猜测（不做子串匹配、不逐段回退），避免把多段路由 id 误配到无关模型。
func lookupCatalogTolerant(modelID string) ModelCapability {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ModelCapability{}
	}
	// Source 非空即表示命中（lookupCatalogExact 命中时一定写入 "catalog"）。
	if hit := lookupCatalogExact(id); hit.Source != "" {
		return hit
	}
	if i := strings.LastIndex(id, "/"); i >= 0 {
		if tail := strings.TrimSpace(id[i+1:]); tail != "" {
			if hit := lookupCatalogExact(tail); hit.Source != "" {
				return hit
			}
		}
	}
	return ModelCapability{}
}

// mergeCatalog 按 ZCode 内建规则表做「全表顺序扫描 + 逐字段覆盖」，是本阶段匹配语义的
// 唯一实现，公开函数传 modelCatalogRules，单测可传入合成规则验证逐字段覆盖。
//
// 它在完整分层里的位置（见 lookupCapabilityForSite）：
//
// provider 返回 > override > site > zcode > presets > catalog
//
//   - provider 返回：调用方从 /models 拿到的 ctx/out 是 API 事实，永远最高；
//   - override：APM 手维护的修正，仅覆盖它显式给出的字段，以便在内建数据落后时立刻纠偏；
//   - site：站点级规则要同时满足模型名 + baseUrl（+ apiType），是 ZCode 自身最具体的一层；
//     ZCode 的规则数组把它拼在家族正则之后由 overlay 覆盖家族值，因此它必须排在下面这层之前；
//   - zcode（本函数）：ZCode 自带的家族正则。APM 写出的配置正是给 ZCode / DSH 消费的，
//     因此这一层就是那个 harness 的真正口径，必须排在社区数据之前；
//   - presets：APM 手维护旧表，代表此前发布物的既有行为，社区快照同样不应静默改写它；
//   - catalog：第三方社区离线目录，最后兜底，只补前面各层留下的空缺，**永不改写**已给出的值
//     （实测社区数据与既有值存在 272000 vs 1050000、262000 vs 262144 这类离群差异）。
//
// 全部采用「逐字段覆盖」：某层未给出某字段时，该字段继续向后找，绝不清零前面的值。
//
// 关键偏离：**跳过 Pattern == ".*" 的纯兜底规则**。ZCode 自己用它把任意未知模型填成
// ctx=200000 / out=32000 / levels=["disabled","enabled"]，但 APM 的既有契约是「未知就留空
// 自动匹配」，若照抄会让用户自命名的本地模型被写进猜测值（可能弄坏只支持 32k 的模型）。
// 因此这里有意偏离 ZCode 的兜底默认值，只跳过这一条，其余 `.*xxx` 家族规则照常参与。
//
// features 必须**跟着一起跳过**：那条兜底规则还填满了 supportsToolCall:true /
// supportsJsonSchemaOutput:false 之类的位，照抄会让「工具调用」这类位占满每一个模型；
// 更糟的是它一旦占位，后面各层（站点规则、离线目录）的 features 就永远轮不到补空缺——
// 与 ctx/out 的理由完全相同，不是两套规则。
func mergeCatalog(modelID string, rules []catalogRule) ModelCapability {
	id := strings.TrimSpace(modelID)
	cap := ModelCapability{}
	var rawLevels []string
	haveLevels := false
	for _, rule := range rules {
		if rule.Pattern == ".*" {
			continue
		}
		if rule.Re == nil || !rule.Re.MatchString(id) {
			continue
		}
		// 逐字段覆盖：未出现的字段保持前面的值。
		if rule.Context > 0 {
			cap.Context = rule.Context
		}
		if rule.Output > 0 {
			cap.Output = rule.Output
		}
		if rule.Levels != nil {
			rawLevels = rule.Levels
			haveLevels = true
		}
		// features 与上面三列同一套「后命中覆盖」；编码畸形一律当作该规则未给出（fail-closed）。
		if parsed, ok := parseFeatureEncoding(rule.Features); ok {
			cap.Features = overlayFeatures(cap.Features, parsed)
		}
	}
	if haveLevels {
		cap.ReasoningLevels = normalizeCapabilityLevels(rawLevels)
	}
	return cap
}

// normalizeCapabilityLevels 把规则里收集到的原始档位串规范化：先喂给 NormalizeVariants
// （按 ZCodeVariantSet 白名单过滤、去重，并把 disabled/none/nothink 折成 off；enabled 保留），
// 再按固定全集顺序排序。空结果一律返回 nil（而非空切片），以配合 DTO 的 omitempty。
func normalizeCapabilityLevels(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	norm := NormalizeVariants(raw)
	if len(norm) == 0 {
		return nil
	}
	return sortVariantsCanonical(norm)
}

// vendorRules 是厂商判定用的独立小型正则表（**不复用** ZCode 的 config.group——它已被
// mergeV2ProviderRule 强写回 standard-provider，与厂商无关）。
//
// 顺序即优先级，长名/具体名在前，避免互相抢占：例如 openai 的 `^o[1-9]` 需让 o3/o4 命中，
// 又不能被 gpt- 抢走；anthropic 的 claude 必须先于其它。
var vendorRules = []struct {
	vendor string
	re     *regexp.Regexp
}{
	{"anthropic", regexp.MustCompile(`anthropic|claude`)},
	{"openai", regexp.MustCompile(`openai|gpt-|^o[1-9]`)},
	{"google", regexp.MustCompile(`gemini|google`)},
	{"deepseek", regexp.MustCompile(`deepseek`)},
	{"zhipu", regexp.MustCompile(`glm|zhipu`)},
	{"alibaba", regexp.MustCompile(`qwen|qwq|dashscope|alibaba`)},
	{"moonshot", regexp.MustCompile(`kimi|moonshot`)},
	{"minimax", regexp.MustCompile(`minimax`)},
	{"xai", regexp.MustCompile(`grok|xai`)},
	{"meta", regexp.MustCompile(`llama|meta`)},
	{"mistral", regexp.MustCompile(`mistral|ministral|codestral`)},
	{"bytedance", regexp.MustCompile(`doubao|volcengine|bytedance`)},
	{"tencent", regexp.MustCompile(`hunyuan|tencent`)},
	{"stepfun", regexp.MustCompile(`step-`)},
	{"baichuan", regexp.MustCompile(`baichuan`)},
	{"zeroone", regexp.MustCompile(`yi-`)},
	{"iflytek", regexp.MustCompile(`spark|iflytek`)},
	{"cohere", regexp.MustCompile(`command-|cohere`)},
	{"xiaomi", regexp.MustCompile(`mimo|xiaomi`)},
}

// detectVendor 对 modelID 判定厂商：命中即返回规范厂商名（英文小写标识），否则 "unknown"。
func detectVendor(modelID string) string {
	low := strings.ToLower(strings.TrimSpace(modelID))
	if low == "" {
		return "unknown"
	}
	for _, r := range vendorRules {
		if r.re.MatchString(low) {
			return r.vendor
		}
	}
	return "unknown"
}
