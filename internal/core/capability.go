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
}

// ModelCapability 是从内建能力表推断出的模型能力，仅供 APM 内部使用（展示/分组/
// 档位自适应），绝不写回任何配置。
type ModelCapability struct {
	Context         int      // 0 = 未知
	Output          int      // 0 = 未知
	ReasoningLevels []string // 规范化 + 去重 + 按全集顺序排序；nil 表示未推断出
	Vendor          string   // 未命中原样返回 "unknown"
	Source          string   // "override" | "zcode" | "preset"；供测试/调试
}

// LookupModelCapability 按 override > zcode 正则全表覆盖 > presets 旧表 的顺序求值。
//
// 返回值语义：除 modelID 为空外**始终返回 ok==true**（与 presets 时代的 false 语义
// 区分开）。调用方用「Context==0 && Output==0 && len(ReasoningLevels)==0」判断「无信息」。
// modelID 为空时返回 (ModelCapability{Vendor:"unknown", Source:"preset"}, false)。
func LookupModelCapability(modelID string) (ModelCapability, bool) {
	if strings.TrimSpace(modelID) == "" {
		return ModelCapability{Vendor: "unknown", Source: "preset"}, false
	}
	return lookupCapability(modelID, modelOverrides), true
}

// lookupCapability 是 LookupModelCapability 的可注入版本（覆盖层来源可替换），
// 便于单测构造临时覆盖条目验证「override 显式给出即胜出」。overrides 传
// modelOverrides 即等价于公开函数。
//
// 求值顺序：
//  1. override 层：精确 ID 优先、最长前缀次之；命中字段直接采用（0/nil 表示不覆盖，
//     留给后续来源补），符合「显式给出即胜出」。
//  2. zcode 正则：全表顺序扫描 + 逐字段覆盖（见 mergeCatalog），只填 override 未给出的字段。
//  3. presets 旧表：逐字段兜底（Context/Output 各自独立补，不因一方已有值就丢弃另一方）。
func lookupCapability(modelID string, overrides []modelOverride) ModelCapability {
	id := strings.TrimSpace(modelID)
	cap := ModelCapability{Vendor: "unknown"}

	// 1. override 层（最高优先，逐字段覆盖）。
	// Source 只在 override **真的贡献了能力字段**（ctx/out/levels）时才标 override；
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
		if v := strings.TrimSpace(ov.Vendor); v != "" {
			cap.Vendor = v
		}
	}

	// 2. zcode 正则全表顺序覆盖：只补 override 未给出的字段。
	cat := mergeCatalog(id, modelCatalogRules)
	if cap.Context == 0 {
		cap.Context = cat.Context
	}
	if cap.Output == 0 {
		cap.Output = cat.Output
	}
	if cap.ReasoningLevels == nil {
		cap.ReasoningLevels = cat.ReasoningLevels
	}
	if cap.Source == "" && (cat.Context > 0 || cat.Output > 0 || cat.ReasoningLevels != nil) {
		cap.Source = "zcode"
	}

	// 3. presets 旧表兜底：逐字段补（两列各自独立）。
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

	// 厂商判定独立于补全来源：即使 ctx/out/levels 全空也要给出厂商（或 unknown）。
	if cap.Vendor == "" || cap.Vendor == "unknown" {
		cap.Vendor = detectVendor(id)
	}
	return cap
}

// mergeCatalog 按 ZCode 内建规则表做「全表顺序扫描 + 逐字段覆盖」，是本阶段匹配语义的
// 唯一实现，公开函数传 modelCatalogRules，单测可传入合成规则验证逐字段覆盖。
//
// 关键偏离：**跳过 Pattern == ".*" 的纯兜底规则**。ZCode 自己用它把任意未知模型填成
// ctx=200000 / out=32000 / levels=["disabled","enabled"]，但 APM 的既有契约是「未知就留空
// 自动匹配」，若照抄会让用户自命名的本地模型被写进猜测值（可能弄坏只支持 32k 的模型）。
// 因此这里有意偏离 ZCode 的兜底默认值，只跳过这一条，其余 `.*xxx` 家族规则照常参与。
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
