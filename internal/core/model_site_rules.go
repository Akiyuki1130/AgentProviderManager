package core

import (
	"net/url"
	"regexp"
	"strings"
)

// siteRule 是 ZCode 内建站点级能力规则（providerSiteRules）的一条。
//
// 字段顺序与名称是与生成文件（model_site_rules_gen.go，另一个 Agent 产出）之间的共享契约，
// 不得改动：生成文件只声明 `var modelSiteRules = []siteRule{...}`，类型由本文件声明。
//
// 语义（与 catalogRule 相同的「全表顺序扫描 + 逐字段覆盖」，但多两道门）：
//   - Model / BaseURL / APIType 三个正则必须同时命中才算命中；
//   - APIType 为 nil 表示该规则不限定 apiType；
//   - Context / Output 为 0 表示该规则未给出该字段，Levels 为 nil 表示未给出；
//     未出现的字段必须保持前面规则带来的值。Features 空串同样表示未给出，
//     且走与这三列完全相同的「后命中覆盖」。
//
// Levels 原样保留 ZCode 的拼写（含 "enabled" / "disabled"），规范化交给运行时
// normalizeCapabilityLevels。
type siteRule struct {
	Model   *regexp.Regexp
	BaseURL *regexp.Regexp
	APIType *regexp.Regexp
	Context int
	Output  int
	Levels  []string
	// Features 与 catalogRule.Features 同格式、同语义：与生成表共用的紧凑编码
	// `<input>|<output>|<flags>`（契约见 capability_features.go 的 parseFeatureEncoding），
	// 空串表示该规则未给出能力位，畸形编码同样按未给出处理（fail-closed）。
	// 层内合并是「后命中覆盖」，与 Context/Output/Levels 一致。
	Features string
}

// siteSource 是站点级规则命中时写入 ModelCapability.Source 的标记。
const siteSource = "site"

// matchSiteRules 按 provider 的 baseUrl（与可选 apiType）匹配站点级能力规则；未命中返回
// 零值 ModelCapability（各项零值、Source 空串）。
//
// 参数语义与 ZCode 内部分配器 ModelConfigRules.resolve 一致（实测结论见生成物文件头）：
//   - modelID：模型名。生成物里的正则已带 (?i) 且是全串锚定，因此大小写不敏感、必须整串匹配；
//   - baseURL：provider 的 baseUrl。匹配前先规范化（见 normalizeBaseURLForRuleMatch），
//     正则全串锚定且大小写敏感；baseURL 为空或不是合法绝对 URL 时不做任何匹配，直接返回零值；
//   - apiType：协议类型。**空串或纯空白等价于「未知」**；只有 APIType 为 nil（规则根本没写
//     apiTypeMatch）才是「不限定 apiType」。规则写了 apiTypeMatch 时，apiType 必须非空且命中，
//     否则该规则跳过（对应 ZCode 里 `t.apiType == null` 就跳过站点规则的写法，见下方实现处的
//     偏离说明）。
//
// 命中至少一条规则时 Source 记为 "site"；每条规则只覆盖它真正给出的字段，因此可能出现
// 「Source 为 site、能力字段全空」的结果（规则只给了 inputFormat 之类的非能力字段），
// 调用方仍应按「三字段皆空即无信息」判断。
func matchSiteRules(modelID, baseURL, apiType string) ModelCapability {
	return matchSiteRulesWith(modelID, baseURL, apiType, modelSiteRules)
}

// matchSiteRulesWith 是 matchSiteRules 的可注入版本（规则表可替换），便于单测用合成规则
// 精确验证「三正则同时命中」「顺序扫描 + 逐字段覆盖」等语义。
func matchSiteRulesWith(modelID, baseURL, apiType string, rules []siteRule) ModelCapability {
	subject := normalizeBaseURLForRuleMatch(baseURL)
	if subject == "" {
		return ModelCapability{}
	}
	id := strings.TrimSpace(modelID)
	if id == "" {
		return ModelCapability{}
	}
	api := strings.TrimSpace(apiType)

	cap := ModelCapability{}
	var rawLevels []string
	haveLevels := false
	hit := false
	for _, rule := range rules {
		if rule.Model == nil || !rule.Model.MatchString(id) {
			continue
		}
		// apiTypeMatch 非 nil 时：apiType 必须非空且命中，否则整条规则不适用。
		//
		// 有意偏离（apiType 为空串的处理）：ZCode 的判定是
		// `apiTypeMatch !== undefined && (api.apiType == null || !match)`——它只在
		// **null/undefined** 时跳过；空串会被当成普通字符串去过正则。而真实表里有 6 条
		// apiTypeMatch 写死为 `.*` 的规则（model_site_rules_gen.go 第 71/74/77/78/81/82 行：
		// deepseek-v4-flash(-vision-exp) @ api.deepseek.com，以及 openrouter.ai 下的
		// deepseek-v4-1-flash / qwen3.7-flash / qwen3.7-plus / qwen/qwen3.8-max-0902 /
		// qwen/qwen3.6-(plus|flash)），在 ZCode 里传空串也会命中。
		// APM 的调用语义里空串表示「未知」，拿未知的 apiType 去命中一条显式限定了协议类型的
		// 规则会给出可能错误的能力值，因此这里保守处理：空串一律跳过限定 apiType 的规则。
		// 后果仅限于上述 6 条 `.*` 规则在 apiType 未知时不生效（它们的兄弟条目通常只差一个
		// apiType，精度损失可接受），其余 48 条行为与 ZCode 一致。
		if rule.APIType != nil && (api == "" || !rule.APIType.MatchString(api)) {
			continue
		}
		if rule.BaseURL == nil || !rule.BaseURL.MatchString(subject) {
			continue
		}
		hit = true
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
		// features 走同一套层内「后命中覆盖」；编码畸形一律当作该规则未给出（fail-closed）。
		if parsed, ok := parseFeatureEncoding(rule.Features); ok {
			cap.Features = overlayFeatures(cap.Features, parsed)
		}
	}
	if !hit {
		return ModelCapability{}
	}
	if haveLevels {
		cap.ReasoningLevels = normalizeCapabilityLevels(rawLevels)
	}
	cap.Source = siteSource
	return cap
}

// normalizeBaseURLForRuleMatch 复刻 ZCode 的 normalizeBaseURLForRuleMatch：
// 解析成绝对 URL，把 search/hash 从尾部摘出来，去掉主干的结尾 `/`，再把尾巴拼回去。
//
// 与 JS 版 `new URL(...)` 对齐的部分：host 与 scheme 一律小写（JS 会规范化），
// 缺 scheme/host（JS 的 new URL 会抛错）时返回空串，表示该 baseUrl 不参与匹配。
func normalizeBaseURLForRuleMatch(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	parsed, err := url.Parse(s)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	suffix := ""
	if parsed.RawQuery != "" {
		suffix += "?" + parsed.RawQuery
	}
	if parsed.Fragment != "" {
		suffix += "#" + parsed.Fragment
	}
	base := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + parsed.EscapedPath()
	return strings.TrimRight(base, "/") + suffix
}
