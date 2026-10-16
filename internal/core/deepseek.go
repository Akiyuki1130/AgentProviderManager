package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agentprovidermanager/internal/dshprovider"
	"gopkg.in/yaml.v3"
)

// dshReasoningEffortOrder 是 DSH（DeepSeek Harness）固定 7 档的顺序，
// 与 DSH 的 THINKING_LEVELS 完全一致。阶段 A 的 A-6 修复后，读端按此顺序排出档位集合。
var dshReasoningEffortOrder = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// dshCanonicalEffort 把配置里出现的档位拼法归一到 DSH 的 7 档之一。
// 返回 ok=false 表示不是 DSH 认识的档位，调用方应直接丢弃。
//
// 语义说明（阶段 A）：
//   - none / disabled / nothink 是跨 Agent 的 off 等价别名，统一折成 DSH 的 "off"；
//   - minimal 在 DSH 里是合法且独立的档位，绝不折成 low（旧实现会折叠，属 A-6 缺陷）；
//   - enabled 是 ZCode 特有的单元素语义，DSH 没有对应档位。这里作兼容映射：视为
//     「开启思考且未指定档位」，落到 DSH 的默认档位 high。这是兼容而非等价，故单独注释。
func dshCanonicalEffort(value string) (string, bool) {
	switch lk := strings.ToLower(strings.TrimSpace(value)); lk {
	case "none", "disabled", "nothink":
		return "off", true
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
		return lk, true
	case "enabled":
		return "high", true
	default:
		return "", false
	}
}

// dshSortEfforts 把从 map 迭代收集来的档位按 DSH 固定 7 档顺序排列。
// map 的迭代顺序随机，不排序会让同一份配置每次读出的档位顺序都不同。
func dshSortEfforts(v []string) []string {
	rank := func(s string) int {
		for i, o := range dshReasoningEffortOrder {
			if o == s {
				return i
			}
		}
		return len(dshReasoningEffortOrder)
	}
	out := make([]string, len(v))
	copy(out, v)
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}

// dshHasNonOffEffort 判断 reasoningEfforts 映射里是否至少有一个非 off 档位。
// DSH 的 zod 约束要求非空 dict 必须声明至少一个非 off 档位。
func dshHasNonOffEffort(efforts map[string]interface{}) bool {
	for k := range efforts {
		if k != "off" {
			return true
		}
	}
	return false
}

// dshEffortForWrite 是写端把卡片档位名落到 DSH 的解析：先走 dshCanonicalEffort
// （别名归一 + 7 档白名单 + enabled→high 兼容）；认不出时，仅在「允许自定义档位」
// 策略打开时放宽为小写去空格的原始名（空串仍丢弃）。
//
// off 语义不受放宽影响：别名（disabled/none/nothink）始终先折成 "off"。
func dshEffortForWrite(value string) (string, bool) {
	if lv, ok := dshCanonicalEffort(value); ok {
		return lv, true
	}
	if !CustomVariantsAllowed() {
		return "", false
	}
	lv := strings.ToLower(strings.TrimSpace(value))
	if lv == "" {
		return "", false
	}
	return lv, true
}

// dshEffortForRead 是读端把配置里的 DSH 档位名解析回卡片的解析。它只是
// dshEffortForWrite 的**语义别名**（二者当前行为完全一致）：之所以单独命名，
// 是为了让读/写两侧的意图在调用点自解释、并在将来某一侧需要分化时不改调用点。
// 语义：先走 dshCanonicalEffort；认不出时仅在「允许自定义档位」策略打开时保留
// 小写去空格的原始名。否则用户自填的档位会在保存→重读后消失，使「自填」不可用。
func dshEffortForRead(value string) (string, bool) {
	return dshEffortForWrite(value)
}

func isYAMLPath(path string) bool {
	low := strings.ToLower(path)
	return strings.HasSuffix(low, ".yaml") || strings.HasSuffix(low, ".yml")
}

func loadDeepSeekRaw(path string) (map[string]interface{}, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if strings.TrimSpace(string(data)) == "" {
		return map[string]interface{}{}, "json", nil
	}
	if isYAMLPath(path) {
		var result map[string]interface{}
		if err := yaml.Unmarshal(data, &result); err != nil {
			return nil, "", &ConfigParseError{Msg: fmt.Sprintf("DeepSeek Harness 配置解析失败（YAML）：%v", err)}
		}
		if result == nil {
			result = map[string]interface{}{}
		}
		return result, "yaml", nil
	}
	cleaned := StripJSONCComments(string(data))
	var raw interface{}
	if err := json.Unmarshal([]byte(cleaned), &raw); err != nil {
		var yResult map[string]interface{}
		if err2 := yaml.Unmarshal(data, &yResult); err2 == nil && yResult != nil {
			return yResult, "yaml", nil
		}
		return nil, "", &ConfigParseError{Msg: fmt.Sprintf("DeepSeek Harness 配置解析失败（JSON）：%v", err)}
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, "", &ConfigParseError{Msg: "配置文件根节点必须是对象 {...}"}
	}
	return m, "json", nil
}

func DeepSeekLoadConfig(path string) (map[string]interface{}, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{}, nil
	}
	m, _, err := loadDeepSeekRaw(path)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func DeepSeekProvidersMap(cfg map[string]interface{}) map[string]interface{} {
	return deepSeekProvidersMap(cfg)
}

func deepSeekProvidersMap(cfg map[string]interface{}) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	merged := map[string]interface{}{}
	if llm, ok := cfg["llm-pi-ai"].(map[string]interface{}); ok {
		if prov, ok := llm["providers"].(map[string]interface{}); ok {
			for k, v := range prov {
				merged[k] = v
			}
		}
	}
	if p, ok := cfg["provider"].(map[string]interface{}); ok {
		for k, v := range p {
			if _, exists := merged[k]; !exists {
				merged[k] = v
			}
		}
	}
	if len(merged) > 0 {
		return merged
	}
	return nil
}

func DeepSeekProviderSource(cfg map[string]interface{}, providerID string) (string, map[string]interface{}) {
	return deepSeekProviderSource(cfg, providerID)
}

func deepSeekProviderSource(cfg map[string]interface{}, providerID string) (string, map[string]interface{}) {
	if cfg == nil {
		return "", nil
	}
	if llm, ok := cfg["llm-pi-ai"].(map[string]interface{}); ok {
		if prov, ok := llm["providers"].(map[string]interface{}); ok {
			if v, ok := prov[providerID]; ok {
				if m, ok := v.(map[string]interface{}); ok {
					return "llm", m
				}
			}
		}
	}
	if prov, ok := cfg["provider"].(map[string]interface{}); ok {
		if v, ok := prov[providerID]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				return "provider", m
			}
		}
	}
	return "", nil
}

func isDeepSeekLLMMode(cfg map[string]interface{}) bool {
	if cfg == nil {
		return false
	}
	if llm, ok := cfg["llm-pi-ai"].(map[string]interface{}); ok {
		if prov, ok := llm["providers"].(map[string]interface{}); ok && len(prov) > 0 {
			return true
		}
		if _, ok := llm["providers"].(map[string]interface{}); ok {
			if _, hasProvider := cfg["provider"]; !hasProvider {
				return true
			}
			if p, ok := cfg["provider"].(map[string]interface{}); !ok || len(p) == 0 {
				return true
			}
		}
	}
	return false
}

func LoadCredentialsForPreview() map[string]string {
	return loadCredentials("")
}

func loadCredentials(credsPath string) map[string]string {
	if credsPath == "" {
		credsPath = DeepSeekCredentialsPath()
	}
	data, err := os.ReadFile(credsPath)
	if err != nil {
		return map[string]string{}
	}
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for k, v := range raw {
		// version/refs/records 是 0.2 的布局键（records 是插件的登录记录，不是密钥引用）。
		if k == "version" || k == "refs" || k == "records" {
			continue
		}
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	if refs, ok := raw["refs"].(map[string]interface{}); ok {
		for k, v := range refs {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

func WriteDeepSeekYAML(path string, cfg map[string]interface{}, fingerprint string) error {
	return writeDeepSeekConfig(path, cfg, fingerprint)
}

func ConvertDeepSeekModel(mid string, modelCfg map[string]interface{}, kind string) map[string]interface{} {
	if modelCfg == nil {
		modelCfg = map[string]interface{}{}
	}
	if id, ok := modelCfg["id"].(string); ok && strings.TrimSpace(id) != "" {
		mid = strings.TrimSpace(id)
	}
	out := deepCopyMap(modelCfg)
	if out == nil {
		out = map[string]interface{}{}
	}
	out["_dsh_raw"] = deepCopyMap(modelCfg)
	if name, ok := modelCfg["name"].(string); ok && strings.TrimSpace(name) != "" && strings.TrimSpace(name) != mid {
		n := strings.TrimSpace(name)
		if len(n) > MaxProviderNameLen {
			n = n[:MaxProviderNameLen]
		}
		out["name"] = n
	}
	if re := modelCfg["reasoningEfforts"]; re != nil {
		if re == false {
			// Explicit non-reasoning marker – do not emit reasoning block.
		} else if m, ok := re.(map[string]interface{}); ok && len(m) > 0 {
			// A-6 修复：放开到 DSH 全部 7 档，不再丢 medium/xhigh/enabled，
			// 且 minimal 不再被折成 low。
			variants := []string{}
			seen := map[string]bool{}
			for k, v := range m {
				lk, ok := dshEffortForRead(k)
				if !ok {
					continue
				}
				if lk == "off" {
					if !seen[lk] {
						variants = append(variants, lk)
						seen[lk] = true
					}
					continue
				}
				// DSH 的 null 只允许 off 档；非 off 档位必须是字符串拼法。
				if v == nil {
					continue
				}
				if !seen[lk] {
					variants = append(variants, lk)
					seen[lk] = true
				}
			}
			if len(variants) > 0 {
				variants = dshSortEfforts(variants)
				defaultVariant := ""
				if preferred, ok := modelCfg["reasoningEffort"].(string); ok {
					// 用 DSH 自己的归一化，避免依赖 tokens.go 的 canonicalVariant
					// （它对 minimal 的历史折叠会误判默认档位）。读端同样尊重
					// 「允许自定义档位」，否则自填的默认档位重读后会丢失。
					if pv, ok := dshEffortForRead(preferred); ok && containsStr(variants, pv) {
						defaultVariant = pv
					}
				}
				if defaultVariant == "" {
					for _, pref := range DefaultVariantPreference {
						if containsStr(variants, pref) {
							defaultVariant = pref
							break
						}
					}
				}
				if defaultVariant == "" && len(variants) > 0 {
					defaultVariant = variants[0]
				}

				if !(len(variants) == 1 && variants[0] == "off") {
					out["reasoning"] = map[string]interface{}{
						"enabled":        true,
						"variants":       variants,
						"defaultVariant": defaultVariant,
					}
				}
			}
		}
	}
	if limit, ok := modelCfg["limit"].(map[string]interface{}); ok {
		ctx := ParseTokens(limit["context"])
		outTok := ParseTokens(limit["output"])
		if ctx != nil && outTok != nil {
			out["limit"] = map[string]interface{}{"context": *ctx, "output": *outTok}
		}
	} else {
		var ctxVal, outVal interface{}
		if v, ok := modelCfg["contextWindow"]; ok {
			ctxVal = v
		} else if v, ok := modelCfg["context_window"]; ok {
			ctxVal = v
		}
		if v, ok := modelCfg["maxTokens"]; ok {
			outVal = v
		} else if v, ok := modelCfg["max_tokens"]; ok {
			outVal = v
		}
		if ctxVal != nil || outVal != nil {
			ctx := ParseTokens(ctxVal)
			outTok := ParseTokens(outVal)
			if ctx != nil && outTok != nil {
				out["limit"] = map[string]interface{}{"context": *ctx, "output": *outTok}
			} else if ctx != nil {
				out["limit"] = map[string]interface{}{"context": *ctx}
			} else if outTok != nil {
				out["limit"] = map[string]interface{}{"output": *outTok}
			}
		}
	}
	// 中立能力位（E1）：DSH 模型级能力面只有 input（text/image）与 compat 里可配置的
	// supportsStrictMode / supportsTemperature，把它们反向读进内部能力通道（只写真），
	// 卡片侧的 Capabilities 因此能读回，写入端（dshModelToRaw）也就有了唯一数据源。
	if caps := dshCapabilitiesFromModel(modelCfg); len(caps) > 0 {
		out[apmCapabilitiesModelKey] = caps
	}
	return out
}

// dshModalityOrder 是 DSH 模型级 input 的合法取值范围与规范序
// （见 MODEL-MANAGEMENT-PLAN.md 10.1.3：MODALITIES = {text, image}）。
var dshModalityOrder = []string{"text", "image"}

// dshKnownModalities 把任意来源的词表收敛到 DSH 的合法取值 {text,image}：按 dshModalityOrder
// 排序去重，非法词（video/pdf/audio 等 DSH schema 会拒收的词）一律丢弃；交集为空返回 nil
// （调用方据此整键省略——DSH 的 schema 既拒收空数组，也拒收 text/image 之外的词）。
//
// 它只管「收敛」，不管 text 的补齐：那是读端与写端**刻意不同**的地方，见下面两个函数。
func dshKnownModalities(words []string) []string {
	known := map[string]bool{}
	for _, word := range NormalizeInputModalities(words) {
		known[word] = true
	}
	out := make([]string, 0, len(dshModalityOrder))
	for _, word := range dshModalityOrder {
		if known[word] {
			out = append(out, word)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// dshCapabilityModalities 是**写端**（dshModelToRaw）用的收敛：交集非空时结果必含 text。
//
// 为什么要强制含 text：DSH 模型级的 `input` 是**完整集合**语义——写了 `["image"]` 就等于
// 声明「这个模型不接受文本输入」（路由级 defaultInput 默认 `["text"]`，一旦显式写出就整段
// 取代它）。而中立层的能力位一律三态、只承载「已知为真」的正向事实：它**从来没有「text 为
// 假」的证据**，缺 text 只说明上游这条数据没给出这个事实（例如能力位表里某条只记了
// supportsImage）。把「缺证据」翻译成「不支持文本」正是本层要挡住的错误——APM 绝不写出一个
// 声明「不支持文本」的集合。
//
// 注意与读端的**刻意不对称**（见 dshReadInputModalities）：读端忠实照读配置里显式写出的
// `input`，写端则保证「非空集合必含 text」。代价是：一条手写的 `input: ["image"]` 条目在被
// APM 保存后会变成 `["text","image"]`——这是**放宽而非收窄**、用户可见的变化，也是有意为之
// （宁可把「不接受文本」这种可疑声明放宽，也不让 APM 的写入路径产生它）。
func dshCapabilityModalities(words []string) []string {
	out := dshKnownModalities(words)
	if len(out) == 0 {
		return nil
	}
	// text 是 dshModalityOrder 的首位，因此命中时它必然已经在 out[0]；未命中则前插。
	if out[0] != "text" {
		out = append([]string{"text"}, out...)
	}
	return out
}

// dshReadInputModalities 是**读端**对 DSH 模型节点 `input` 的忠实读取（与写端不对称，见
// dshCapabilityModalities）：
//   - `input` 键**存在**：按原样读——只做合法词过滤与去重（dshKnownModalities），**不补 text**。
//     DSH 的 input 是完整集合语义，显式写出 `["image"]` 就是用户的明确声明，读回时强补 text
//     等于把声明改宽；键存在但读不出任何合法词（null / 空数组 / 全非法词）时不记任何模态
//     （fail-closed，不凭空补 text）；
//   - `input` 键**缺失**：DSH 走路由级 `defaultInput = ["text"]`，即「文本始终可用」是 DSH
//     侧的既定语义，因此按 [text] 记。
func dshReadInputModalities(modelCfg map[string]interface{}) []string {
	raw, present := modelCfg["input"]
	if !present {
		return []string{"text"}
	}
	return dshKnownModalities(apmModalityWords(raw))
}

// dshInputModalities 读卡片已知的输入模态候选，优先级：
//   - 卡片的中立能力位（三个 harness 写入端的唯一数据源）；
//   - 中间形态的真实键 `modalities.input`（opencode kind 与导入数据）；
//   - DSH 原生的 `input` 字段——它本身就是输入模态，回落它才能让没有能力位的往返不丢数据。
func dshInputModalities(card ModelCard) []string {
	if card.Capabilities != nil && len(card.Capabilities.InputModalities) > 0 {
		return card.Capabilities.InputModalities
	}
	if modalities, ok := card.RawCfg["modalities"].(map[string]interface{}); ok {
		if words := apmModalityWords(modalities["input"]); len(words) > 0 {
			return words
		}
	}
	return apmModalityWords(card.RawCfg["input"])
}

// dshCompatCapabilityKeys 是 DSH compat 里**配置侧允许**、且与能力位对应的键。
// 其余 catalog-only 的 compat 键（supportsToolSearch / supportsMidConvoSystemMessages /
// supportsOpenAIGrammarTools / supportsAdditionalTools 等）会被 DSH 的
// assertOfferedCompatFields 拒绝，一个都不能写；工具调用 / 原生联网搜索 / 对话中途 system
// 在 DSH 的配置层无法表达，因此也不落键（见 10.1.3）。
var dshCompatCapabilityKeys = []apmCapabilityKeyMap{
	{"supportsStrictMode", "structured_output"},
	{"supportsTemperature", "temperature"},
}

// dshCapabilitiesFromModel 从 DSH 模型节点反向读出已知为真的能力位：
//   - input（DSH 只接受 text/image）→ input_modalities，走读端口径 dshReadInputModalities：
//     键存在时**忠实照读**（不补 text），键缺失时按 DSH 的 defaultInput=["text"] 记 [text]；
//   - compat.supportsStrictMode → structured_output；
//   - compat.supportsTemperature → temperature。
//
// 只读真值：false / null / 读不出的形状一律不写（false 会被当成「显式不支持」，
// 而 APM 的中立层只有「已知为真」与「未知」两态）。
func dshCapabilitiesFromModel(modelCfg map[string]interface{}) map[string]interface{} {
	compat, _ := modelCfg["compat"].(map[string]interface{})
	flags := apmTrueCapabilityFlags(compat, dshCompatCapabilityKeys)
	return apmCapabilitiesPayload(
		dshReadInputModalities(modelCfg),
		nil,
		flags...,
	)
}

func ConvertDeepSeekProvider(providerID string, raw map[string]interface{}, kind *string) (string, map[string]interface{}, error) {
	pid := strings.TrimSpace(providerID)
	if !ProviderIDValid(pid) {
		return "", nil, fmt.Errorf("Provider ID「%s」只能包含字母、数字、下划线、短横线或冒号", providerID)
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	name := pid
	if n, ok := raw["displayName"].(string); ok && strings.TrimSpace(n) != "" {
		name = strings.TrimSpace(n)
	} else if n, ok := raw["name"].(string); ok && strings.TrimSpace(n) != "" {
		name = strings.TrimSpace(n)
	}
	if len(name) > MaxProviderNameLen {
		name = name[:MaxProviderNameLen]
	}
	baseURL, _ := raw["baseURL"].(string)
	apiKey, _ := raw["apiKey"].(string)
	apiKeyEnv, _ := raw["apiKeyEnv"].(string)
	if opts, ok := raw["options"].(map[string]interface{}); ok {
		if baseURL == "" {
			baseURL, _ = opts["baseURL"].(string)
		}
		if apiKey == "" {
			apiKey, _ = opts["apiKey"].(string)
		}
		if apiKeyEnv == "" {
			apiKeyEnv, _ = opts["apiKeyEnv"].(string)
		}
	}
	if apiKey == "" && apiKeyEnv != "" {
		creds := loadCredentials("")
		if v, ok := creds[apiKeyEnv]; ok {
			apiKey = v
		} else if v := os.Getenv(apiKeyEnv); v != "" {
			apiKey = v
		}
	}
	resolvedKind := ""
	if kind != nil {
		resolvedKind = *kind
	} else {
		resolvedKind = InferKind(baseURL)
		if rk, ok := raw["kind"].(string); ok && strings.TrimSpace(rk) != "" {
			resolvedKind = ConfigKindToUIKind(strings.TrimSpace(rk), baseURL)
		} else if api, ok := raw["api"].(string); ok && strings.TrimSpace(api) != "" {
			// 0.2 的路由用 api 声明协议（openai-completions / openai-responses /
			// anthropic-messages），据此判定 UI 类型。
			resolvedKind = kindFromDeepSeekAPI(api, baseURL)
		}
	}
	kindNorm, err := ValidateKind(resolvedKind)
	if err != nil {
		return "", nil, err
	}
	extra := map[string]interface{}{}
	for k, v := range raw {
		if k != "baseURL" && k != "apiKey" && k != "apiKeyEnv" && k != "models" && k != "displayName" && k != "name" {
			extra[k] = v
		}
	}
	modelsOut := map[string]interface{}{}
	if rawModels, ok := raw["models"].([]interface{}); ok {
		for _, entry := range rawModels {
			mm, _ := entry.(map[string]interface{})
			if mm == nil {
				continue
			}
			mid, _ := mm["id"].(string)
			mid = strings.TrimSpace(mid)
			if mid == "" {
				continue
			}
			if len(mid) > MaxModelIDLength {
				return "", nil, fmt.Errorf("Provider「%s」的模型 ID「%s」过长", pid, ShortText(mid, 40))
			}
			modelsOut[mid] = ConvertDeepSeekModel(mid, mm, kindNorm)
		}
	} else if rawModelsMap, ok := raw["models"].(map[string]interface{}); ok {
		for mid, modelCfg := range rawModelsMap {
			mid = strings.TrimSpace(mid)
			if mid == "" {
				continue
			}
			if len(mid) > MaxModelIDLength {
				return "", nil, fmt.Errorf("Provider「%s」的模型 ID「%s」过长", pid, ShortText(mid, 40))
			}
			mm, _ := modelCfg.(map[string]interface{})
			modelsOut[mid] = ConvertDeepSeekModel(mid, mm, kindNorm)
		}
	}
	if len(modelsOut) > MaxModels {
		return "", nil, fmt.Errorf("Provider「%s」的模型数量不能超过 %d 个", pid, MaxModels)
	}
	optsExtra := map[string]interface{}{}
	for k, v := range extra {
		optsExtra[k] = v
	}
	opts, err := BuildOptions(baseURL, apiKey, strings.TrimSpace(apiKey) != "", optsExtra)
	if err != nil {
		opts = map[string]interface{}{"baseURL": baseURL}
		if strings.TrimSpace(apiKey) != "" {
			opts["apiKey"] = strings.TrimSpace(apiKey)
			opts["apiKeyRequired"] = true
		}
		for k, v := range optsExtra {
			opts[k] = v
		}
	}
	cfg := map[string]interface{}{
		"name":    name,
		"kind":    KindToConfig[kindNorm],
		"source":  "custom",
		"options": opts,
		"models":  modelsOut,
	}
	return pid, cfg, nil
}

func DeepSeekProviderToEdit(providerID string, raw map[string]interface{}) (*ProviderEdit, error) {
	pid, normalized, err := ConvertDeepSeekProvider(providerID, raw, nil)
	if err != nil {
		return nil, err
	}
	opts, _ := normalized["options"].(map[string]interface{})
	baseURL := stringOr(opts["baseURL"], "")
	apiKey := stringOr(opts["apiKey"], "")
	cards := make([]ModelCard, 0)
	models, _ := normalized["models"].(map[string]interface{})
	for mid, value := range models {
		mm, _ := value.(map[string]interface{})
		// 站点上下文只带 baseURL：DSH 侧只有 UI kind（openai-compatible / anthropic /
		// responses），没有 ZCode 的 api.type，拿不到就传空串——限定 apiType 的站点规则
		// 会被跳过（fail-closed），不会误命中。
		card := CfgToCardForSite(mid, mm, baseURL, "")
		if card.Context == "" {
			card.Context = mm["contextWindow"]
		}
		if card.Output == "" {
			card.Output = mm["maxTokens"]
		}
		cards = append(cards, card)
	}
	for i := 0; i < len(cards); i++ {
		for j := i + 1; j < len(cards); j++ {
			if strings.ToLower(cards[j].ModelID) < strings.ToLower(cards[i].ModelID) {
				cards[i], cards[j] = cards[j], cards[i]
			}
		}
	}
	name := stringOr(normalized["name"], pid)
	// 把可透传给 pi-ai 路由（headers、timeoutMs、compat…）的字段交给调用方，
	// 迁移到别处或经编辑器保存时才不会丢。
	optionsJSON := ""
	if extra := deepSeekRouteExtras(opts); len(extra) > 0 {
		if b, merr := json.Marshal(extra); merr == nil {
			optionsJSON = string(b)
		}
	}
	return &ProviderEdit{ID: pid, Name: name, Kind: stringOr(normalized["kind"], "openai-compatible"), BaseURL: baseURL, APIKey: apiKey, APIKeyRequired: apiKey != "", OptionsJSON: optionsJSON, Cards: cards, RawProvider: deepCopyMap(raw)}, nil
}

func DeepSeekImportPreview(path string) ImportPreview {
	if strings.TrimSpace(path) == "" {
		path = DeepSeekSettingsPath()
	}
	item := ImportPreview{Path: path}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return item
	}
	item.Exists = true

	format := DetectDeepSeekFormatAt(path)
	var providers map[string]interface{}
	var err error
	switch format {
	case dshprovider.FormatProfileDesktop, dshprovider.FormatProfileWeb, dshprovider.FormatProfileOther:
		// 0.2：provider 在 profile patch 的 llm-pi-ai 条目里；home 级 patch 覆盖时以它为准。
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			item.Error = fmt.Sprintf("读取配置文件失败：%v", rerr)
			return item
		}
		patch, perr := dshprovider.LoadPatch(data)
		if perr != nil {
			item.Error = perr.Error()
			return item
		}
		providers, err = dshEffectiveProviders(newDeepSeekProfileDoc(path, patch))
	default:
		var cfg map[string]interface{}
		cfg, err = DeepSeekLoadConfig(path)
		if err == nil {
			providers = deepSeekProvidersMap(cfg)
		}
	}
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if len(providers) == 0 {
		item.Error = "该文件中没有可管理的 provider 配置"
		return item
	}
	item.Providers = deepSeekSummariesFromProviders(providers)
	return item
}

// DeepSeekSaveProvider updates a provider in the existing on-disk snapshot.
// It is kept as a compatibility wrapper for callers that only have a path.
func DeepSeekSaveProvider(path string, providerID string, providerCfg map[string]interface{}, fingerprint string) error {
	return DeepSeekSaveProviderWithMerge(path, providerID, providerCfg, fingerprint, false)
}

// DeepSeekSaveProviderWithMerge 是低层写入入口，直接接收已构建好的 providerCfg。
// 它没有 UI 卡片载荷，无法判断“用户显式关闭推理”，因此不会应用
// dshApplyExplicitReasoningOff；生产链路一律走 backend 的 store.Upsert，
// 由后者用 payload 里的卡片补齐该语义。
func DeepSeekSaveProviderWithMerge(path string, providerID string, providerCfg map[string]interface{}, fingerprint string, mergeModels bool) error {
	cfg, err := DeepSeekLoadConfig(path)
	if err != nil {
		return err
	}
	return DeepSeekSaveProviderInConfigWithMerge(path, cfg, providerID, providerCfg, fingerprint, mergeModels)
}

func DeepSeekSaveProviderInConfig(path string, cfg map[string]interface{}, providerID string, providerCfg map[string]interface{}, fingerprint string) error {
	return DeepSeekSaveProviderInConfigWithMerge(path, cfg, providerID, providerCfg, fingerprint, false)
}

func DeepSeekSaveProviderInConfigWithMerge(path string, cfg map[string]interface{}, providerID string, providerCfg map[string]interface{}, fingerprint string, mergeModels bool) error {
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	// 文档变换与 legacy backend 共用同一条路径，避免两处逻辑漂移。
	deepSeekUpsertEntry(cfg, providerID, providerID, providerCfg, mergeModels)
	deepSeekApplyDefaultModel(cfg, deepSeekProvidersMap(cfg))
	if err := flushDeepSeekCredentials(cfg); err != nil {
		return err
	}
	return writeDeepSeekConfig(path, cfg, fingerprint)
}

func dshModelToRaw(card ModelCard, _ string) map[string]interface{} {
	m := deepCopyMap(card.RawCfg)
	if m == nil {
		m = map[string]interface{}{}
	}
	delete(m, "_dsh_raw")
	delete(m, "_opencode_raw")
	// `_apm_capabilities` 是 APM 的内部能力通道（带 `_` 前缀）：下面的 input / compat
	// 映射已经把它的内容消费成 DSH 的键，它自己绝不能写进 DSH 配置——DSH 的 modelFields
	// 是固定集合，多一个键就会被拒。
	delete(m, apmCapabilitiesModelKey)
	delete(m, "reasoning")
	delete(m, "limit")
	delete(m, "modalities")
	delete(m, "zcode")
	delete(m, "source")
	delete(m, "kind")
	// DSH 的模型级 input 只接受 text/image（MODALITIES = {text, image}，见 10.1.3）。
	// 这里把卡片已知的输入模态收敛到两者的交集（dshCapabilityModalities：非空交集必含
	// text，理由与「读端忠实、写端保证」这一有意不对称的代价见该函数注释）：早先的实现把
	// `modalities.input` 原样不过滤地搬进 `input`，一旦含 video/pdf/audio 就会被 DSH 的
	// schema 拒收，连整份配置都会失效。
	// 交集为空则**整键省略**（既不写空数组，也不写 false/null）——不能给 DSH 塞一个
	// 它无法表达的输入模态集合。
	if inputs := dshCapabilityModalities(dshInputModalities(card)); len(inputs) > 0 {
		m["input"] = apmCapabilityStrings(inputs)
	} else {
		delete(m, "input")
	}
	// compat：只补两个 DSH 配置侧允许的能力键，并与既有 compat **合并**（不整块替换，
	// 其它键——包括 headers/timeoutMs 之类无关项——原样保留）。
	// 只写真：未知与假一律不写键，catalog-only 的 compat 键一个都不写（见 10.1.3）。
	// 工具调用 / 原生联网搜索 / 对话中途 system 在 DSH 配置层无法表达，只留在中立层。
	if caps := card.Capabilities; caps != nil {
		addCompat := map[string]interface{}{}
		if caps.StructuredOutput != nil && *caps.StructuredOutput {
			addCompat["supportsStrictMode"] = true
		}
		if caps.Temperature != nil && *caps.Temperature {
			addCompat["supportsTemperature"] = true
		}
		if len(addCompat) > 0 {
			// m 是 card.RawCfg 的深拷贝（deepCopyMap 走 JSON 往返），就地改写不会影响卡片。
			compat, _ := m["compat"].(map[string]interface{})
			if compat == nil {
				compat = map[string]interface{}{}
			}
			for k, v := range addCompat {
				compat[k] = v
			}
			m["compat"] = compat
		}
	}
	delete(m, "id")
	delete(m, "name")
	m["id"] = card.ModelID
	if card.Name != "" && card.Name != card.ModelID {
		m["name"] = card.Name
	}
	// DSH 的档位集合是固定 7 档（off,minimal,low,medium,high,xhigh,max）。
	// 保留源 enabled 状态与所选档位相互独立，跨 Agent 迁移才不会静默开/关推理。
	enabled := card.Reasoning
	explicit := false
	if card.RawReasoningEnabled != nil {
		// The editor updates this field whenever the user toggles reasoning, so
		// it preserves both the source state and an explicit UI change.
		enabled = *card.RawReasoningEnabled
		explicit = true
	}
	switch {
	case explicit && !enabled:
		// 显式关闭：DSH 用 reasoningEfforts: false 表示。
		delete(m, "reasoningEffort")
		m["reasoningEfforts"] = false
	case enabled:
		delete(m, "reasoningEffort")
		// A-6 修复：白名单放开到 DSH 全部 7 档。只写用户集合里的档位，
		// 值取档位同名拼法；off 档写 null（DSH 只允许 off 为 null）。
		//
		// 阶段 C：放宽开关打开时，非 off 的未知档位名（如 very-high）也保留写进
		// reasoningEfforts；off 的语义不受放宽影响（off 一律写 null）。
		re := map[string]interface{}{}
		for _, v := range card.Variants {
			lv, ok := dshEffortForWrite(v)
			if !ok {
				continue
			}
			if lv == "off" {
				re["off"] = nil
			} else {
				re[lv] = lv
			}
		}
		if len(re) == 0 {
			// 卡片声明开启思考但没有可选档位：补 DSH 的合理默认三档。
			re = map[string]interface{}{"off": nil, "high": "high", "max": "max"}
		}
		if !dshHasNonOffEffort(re) {
			// DSH 的 zod 约束要求非空字典必须声明至少一个非 off 档位。
			// 只含 off 的集合无法用字典表达，也不能凭空发明档位，
			// 于是退化为 DSH 的非推理标记——这与 dshApplyExplicitReasoningOff
			// 的出口（reasoningEfforts: false）一致。
			m["reasoningEfforts"] = false
			break
		}
		m["reasoningEfforts"] = re
		// DeepSeek represents the selected default effort in a separate native
		// field. Keep it when the source card has one, including an explicit off.
		// 放宽开关打开时，自定义默认档位同样落回它自身。
		if dv, ok := dshEffortForWrite(card.DefaultVariant); ok {
			if value, exists := re[dv]; exists {
				if dv == "off" {
					m["reasoningEffort"] = "off"
				} else {
					m["reasoningEffort"] = value
				}
			} else {
				delete(m, "reasoningEffort")
			}
		} else {
			delete(m, "reasoningEffort")
		}
	default:
		// 卡片没有任何推理信息（既非显式关闭，也没有档位）：此时不能写
		// reasoningEfforts: false——那是 DSH 的“显式关闭推理”标记，会把原本
		// 没有推理信息的模型静默关掉推理。保持 RawCfg 里已有的推理键不动。
	}
	if card.Context != nil || card.Output != nil {
		ctx := ParseTokens(card.Context)
		out := ParseTokens(card.Output)
		if ctx != nil {
			m["contextWindow"] = *ctx
		}
		if out != nil {
			m["maxTokens"] = *out
		}
	}
	return m
}

func writeDeepSeekConfig(path string, cfg map[string]interface{}, fingerprint string) error {
	format := "json"
	if isYAMLPath(path) {
		format = "yaml"
	}
	dir := filepath.Dir(path)
	if dir != "" {
		_ = os.MkdirAll(dir, 0755)
	}
	var data []byte
	var err error
	if format == "yaml" {
		data, err = yaml.Marshal(cfg)
		if err != nil {
			return err
		}
	} else {
		data, err = json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	}
	d := dir
	if d == "" {
		d = "."
	}
	tmpFile, err := os.CreateTemp(d, filepath.Base(path)+".tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	_ = tmpFile.Sync()
	tmpFile.Close()
	if fingerprint != "" {
		cur, _ := FileFingerprint(path)
		if cur != fingerprint {
			_ = os.Remove(tmpName)
			return fmt.Errorf("配置文件在操作期间被其他程序修改，请重试")
		}
	}
	return os.Rename(tmpName, path)
}

// saveDeepSeekCredential 写入一条凭据 ref。
//
// DSH 0.2 的凭据文件是 version: 1 + refs/records 布局：0.1 的扁平顶层键会被
// DSH 拒绝，因此写盘一律走 dshprovider（内部会把扁平布局升级为 version 1，
// 并保留 records 与注释）。
func saveDeepSeekCredential(envName, apiKey string) error {
	return dshprovider.SetCredentialRef(DeepSeekCredentialsPath(), envName, apiKey)
}
