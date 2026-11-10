package core

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"agentprovidermanager/internal/ompprovider"
)

// 本文件是 OMP（pi 的 models.yml / models.yaml / models.json）读侧的**纯函数**层：
// 只做 map[string]interface{} → DTO 的映射与判定，不读文件、不写文件、不碰 AST。
// 文件 IO 与 AST 变换在 backend_omp.go / omp_store.go；本文件只从
// internal/ompprovider 借用协议词表（如 EffortLevels），不参与其解析与写盘逻辑。
//
// 术语：
//   - wire api：OMP models.yml 里 provider/model 的 api 字段，取值是一个协议名
//     （anthropic-messages / openai-responses / openai-completions …共 13 种）；
//   - UI kind：APM 编辑器里的协议类型，只有 openai-compatible / anthropic / responses。
//     P1 只有这三种 wire api 能映射出 UI kind，其余一律只读（见计划 §4.5 / G16）。

// OMP 的三种可建模 wire api（P1 支持面）。
const (
	OMPWireAnthropicMessages = "anthropic-messages"
	OMPWireOpenAIResponses   = "openai-responses"
	OMPWireOpenAICompletions = "openai-completions"
)

// ompEffortOrder 是 OMP 思考档位的六档规范序（**不含 off**）。
// 单源在 internal/ompprovider.EffortLevels：改档位必须先改那里，core 这里只做
// 包级拷贝（防止外部修改切片污染读侧判定），并在测试里断言两者顺序一致。
var ompEffortOrder = append([]string(nil), ompprovider.EffortLevels...)

// ompOffVariant 是卡片里表示「关闭思考」的档位词。
const ompOffVariant = "off"

// OMPModelsSource 是 OMP models 文档**只读**部分的最小方法集。
//
// 之所以用接口而不是 *ompprovider.ModelsFile：core 层的读侧纯逻辑（selectors）不需要
// 依赖 ompprovider 的完整 AST 能力，也避免 core → ompprovider 的循环依赖。
// internal/ompprovider.ModelsFile 按契约 §2 实现了
// Providers() (map[string]map[string]interface{}, error) 与 ProviderIDs() []string，
// 因此 *ompprovider.ModelsFile **隐式满足**本接口：调用方直接写
//
//	models, err := ompprovider.LoadModels(data)
//	if err != nil { … }
//	selectors := core.OMPSelectors(models)
//
// 即可，不需要任何类型转换或适配器。
type OMPModelsSource interface {
	// Providers 返回 provider ID → provider 节点的深拷贝（调用方可安全改写）。
	Providers() (map[string]map[string]interface{}, error)
	// ProviderIDs 返回全部 provider ID（顺序不保证）。
	ProviderIDs() []string
}

// OMPProviderView 是 OMP 一个 provider 节点的读侧视图（只读，供 List/Get 组 DTO 用）。
type OMPProviderView struct {
	// ID 是 providers 的键，也是 OMP 选择器里 "/" 前半段的原文。
	ID string
	// Raw 是 provider 节点原文（不含任何 APM 内部键）。
	Raw map[string]interface{}
	// WireAPI 是生效的协议名：provider 级 api 优先；provider 未写 api 时，若其全部模型
	// 的 api 一致则取该值；不一致时是常量 "mixed"；彻底没有 api 时为空串。
	WireAPI string
	// Kind 是映射出的 UI kind（openai-compatible / anthropic / responses）；空串 = 不可建模。
	Kind string
	// BaseURL 是 provider 级 baseUrl 原文（不规范化、不补全）。
	BaseURL string
	// APIKeyRaw 是 apiKey 原文：可能是字面量、"!command" 或环境变量名，一律原样保留。
	APIKeyRaw string
	// APIKeyKind 是 apiKey 形态：literal / env / command / none（见 G4）。
	APIKeyKind string
	// Auth 是 auth 原文（"" / apiKey / none / oauth）。
	Auth string
	// APIKeyRequired 是「编辑器是否要显示并允许编辑 API Key」。
	APIKeyRequired bool
	// ReadOnly / ReadOnlyReason 是 P1 只读判定的结果，ReadOnlyReason 为中文原因。
	ReadOnly       bool
	ReadOnlyReason string
	// ModelIDs 是 models[] 里显式声明的模型 ID（保持文件顺序）。
	ModelIDs []string
}

// OMPReadModel 是 OMP 一个模型条目的读侧视图。
type OMPReadModel struct {
	// ID 是 models[].id；空 ID 的条目属于非法配置，由 ompprovider.Validate 拦截。
	ID string
	// Raw 是模型节点原文。
	Raw map[string]interface{}
	// Card 是映射出的前端卡片。
	Card ModelCard
	// Kind 是模型 kind（缺省 chat；search 等不建模的 kind 会让整个 provider 只读）。
	Kind string
	// WireAPI 是该模型生效的协议名（模型级 api > provider 级 api）。
	WireAPI string
}

// OMPEffortMapping 是 OMP 模型思考档位的读写口径（计划 §4.6）。
//
// 读侧：Explicit 来自 thinking.efforts（按六档规范序过滤）；它为空而 reasoning 为 true 时，
// Levels 是按 G3 fallback ladder 推断出的档位。Default 是 thinking.defaultLevel 的原文，
// 不在可用档位内时由调用方回退（见 ompDefaultVariant）。
type OMPEffortMapping struct {
	// WireAPI 是求值所用的协议名。
	WireAPI string
	// Explicit 是 thinking.efforts 里的六档（规范序、去重、已剔除 off 与未知词）。
	Explicit []string
	// Levels 是 reasoning=true 且没有 thinking.efforts 时，按 G3 推断出的档位。
	Levels []string
	// Default 是 thinking.defaultLevel 原文（可能不在 Explicit/Levels 内）。
	Default string
}

// OMPEffortMappingFor 求一个模型条目的档位映射。compat 传 provider 级合并后的 compat
// （模型级 compat 会覆盖其中同名键；两层都没有时传 nil 即可）。
func OMPEffortMappingFor(raw map[string]interface{}, wireAPI string, compat map[string]interface{}) OMPEffortMapping {
	m := OMPEffortMapping{WireAPI: strings.TrimSpace(wireAPI)}
	thinking := ompMap(raw["thinking"])
	if thinking != nil {
		m.Explicit = ompFilterEfforts(thinking["efforts"])
		m.Default = strings.TrimSpace(stringOr(thinking["defaultLevel"], ""))
	}
	if len(m.Explicit) == 0 {
		m.Levels = ompCapabilityLadder(m.WireAPI, ompMergedCompat(raw, compat))
	}
	return m
}

// ompFilterEfforts 把 efforts 原文过滤成六档规范序（剔除 off / enabled / 自定义词）。
func ompFilterEfforts(v interface{}) []string {
	list := ompStringSlice(v)
	if len(list) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, item := range list {
		word := strings.ToLower(strings.TrimSpace(item))
		for _, known := range ompEffortOrder {
			if word == known {
				set[known] = true
				break
			}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for _, known := range ompEffortOrder {
		if set[known] {
			out = append(out, known)
		}
	}
	return out
}

// ompCapabilityLadder 是 G3 的 fallback ladder：reasoning=true 但没写 thinking.efforts 时，
// OMP 会按协议推导档位。这里复刻同一条推导，用于卡片展示与写盘时的默认档位。
//
//   - anthropic-messages / openai-responses：minimal,low,medium,high,xhigh；
//   - openai-completions：仅当 compat.thinkingFormat == "openai" 且
//     compat.supportsReasoningEffort == true 时给到 xhigh，否则止于 high；
//   - 其它 wire api（P1 只读）：止于 high；
//   - 任何情况都**不含 max**。
func ompCapabilityLadder(wireAPI string, compat map[string]interface{}) []string {
	five := []string{"minimal", "low", "medium", "high", "xhigh"}
	four := []string{"minimal", "low", "medium", "high"}
	switch strings.TrimSpace(wireAPI) {
	case OMPWireAnthropicMessages, OMPWireOpenAIResponses:
		return five
	case OMPWireOpenAICompletions:
		format := strings.ToLower(strings.TrimSpace(stringOr(compat["thinkingFormat"], "")))
		supports, _ := ompBoolValue(compat["supportsReasoningEffort"])
		if format == "openai" && supports {
			return five
		}
		return four
	}
	return four
}

// ompMergedCompat 把 provider 级 compat 与模型级 compat 合并（模型级胜出）。
func ompMergedCompat(raw map[string]interface{}, providerCompat map[string]interface{}) map[string]interface{} {
	merged := map[string]interface{}{}
	for k, v := range providerCompat {
		merged[k] = v
	}
	for k, v := range ompMap(raw["compat"]) {
		merged[k] = v
	}
	return merged
}

// ompDefaultVariant 为可用档位挑默认档位：优先 thinking.defaultLevel，其次 high，
// 最后取首个非 off 档位。返回的档位一定在 variants 内。
func ompDefaultVariant(variants []string, preferred string) string {
	if len(variants) == 0 {
		return ""
	}
	if preferred != "" && containsStr(variants, preferred) {
		return preferred
	}
	if containsStr(variants, "high") {
		return "high"
	}
	for _, v := range variants {
		if v != ompOffVariant {
			return v
		}
	}
	return ompOffVariant
}

// ompReadProvider 构造 provider 读侧视图。aliasReason 是该 provider 节点被判定为
// YAML 别名 / "<<" merge 时的中文原因（由 backend_omp.go 的 AST 扫描给出，空串表示没有）。
func ompReadProvider(providerID string, raw map[string]interface{}, aliasReason string) OMPProviderView {
	view := OMPProviderView{
		ID:        providerID,
		Raw:       raw,
		WireAPI:   ompProviderWireAPI(raw),
		BaseURL:   strings.TrimSpace(stringOr(raw["baseUrl"], "")),
		APIKeyRaw: stringOr(raw["apiKey"], ""),
		Auth:      strings.TrimSpace(stringOr(raw["auth"], "")),
	}
	view.Kind = ompWireAPIToKind(view.WireAPI)
	view.APIKeyKind = ompAPIKeyKind(view.APIKeyRaw)
	view.APIKeyRequired = ompAPIKeyRequired(view.Auth)
	for _, m := range ompModelList(raw) {
		id := strings.TrimSpace(stringOr(m["id"], ""))
		if id != "" {
			view.ModelIDs = append(view.ModelIDs, id)
		}
	}
	view.ReadOnlyReason = ompReadOnlyReason(raw, view.WireAPI, aliasReason)
	view.ReadOnly = view.ReadOnlyReason != ""
	return view
}

// OMPKindOf 返回 provider 的 UI kind、只读标记与原始 api 原文。
//
// 只读标记在本函数里只反映「协议不可建模/无法确定」这一条；完整的只读判定见
// OMPReadOnlyReason（本函数的返回值供编辑器在「协议下拉」处直接禁用）。
func OMPKindOf(raw map[string]interface{}) (kind string, readOnly bool, rawAPI string) {
	wire := ompProviderWireAPI(raw)
	kind = ompWireAPIToKind(wire)
	return kind, kind == "", wire
}

// OMPReadOnlyReason 返回 P1 下该 provider 只读的中文原因；空串表示可写。
//
// 判定顺序（同一 provider 命中多条时返回第一条，测试按此顺序断言）：
//  1. 协议名不可确定（provider 与模型都没写 api，或模型 api 不一致）；
//  2. 协议名不属于三种可建模 wire api；
//  3. auth 取值不是 ""/apiKey/none/oauth；
//  4. 缺 baseUrl；
//  5. 没有 models[]、只有 modelOverrides；
//  6. auth 为 oauth 且没有 apiKey（key 由 OAuth 流程管理，P1 不写）；
//  7. models[] 里出现非 chat 的 kind（search 等）。
//
// 注意：YAML 别名 / merge 造成的只读、以及 models.json 未迁移造成的只读不在这里判定
// （前者需要 AST，后者需要文件路径），分别由 backend_omp.go 与 omp_store.go 传入。
func OMPReadOnlyReason(raw map[string]interface{}) string {
	return ompReadOnlyReason(raw, ompProviderWireAPI(raw), "")
}

// ompReadOnlyReason 是 OMPReadOnlyReason 的可注入版本，wire 由调用方传入（避免重复求值），
// aliasReason 由 backend 的 AST 扫描给出（空串表示该 provider 节点不是别名/merge）。
func ompReadOnlyReason(raw map[string]interface{}, wire, aliasReason string) string {
	if aliasReason != "" {
		return aliasReason
	}
	if wire == "" {
		return "无法确定该提供商的协议（provider 与模型的 api 均缺失）"
	}
	if wire == "mixed" {
		return "该提供商的模型使用了多种 api，P1 只按单一协议建模"
	}
	if ompWireAPIToKind(wire) == "" {
		return fmt.Sprintf("该提供商的 api 为「%s」，P1 仅支持 %s、%s 与 %s", wire,
			OMPWireAnthropicMessages, OMPWireOpenAIResponses, OMPWireOpenAICompletions)
	}
	auth := strings.TrimSpace(stringOr(raw["auth"], ""))
	switch auth {
	case "", "apiKey", "none", "oauth":
	default:
		return fmt.Sprintf("该提供商的 auth 为「%s」，P1 无法建模", auth)
	}
	if strings.TrimSpace(stringOr(raw["baseUrl"], "")) == "" {
		return "该提供商没有 baseUrl，P1 无法确定请求地址"
	}
	if auth == "oauth" && strings.TrimSpace(stringOr(raw["apiKey"], "")) == "" {
		return "该提供商的 auth 为 oauth 且未配置 apiKey，密钥由 OAuth 流程管理"
	}
	models := ompModelList(raw)
	if len(models) == 0 {
		if len(ompMap(raw["modelOverrides"])) > 0 {
			return "该提供商只有 modelOverrides 而没有 models，P1 只能只读展示"
		}
		return ""
	}
	for _, m := range models {
		id := strings.TrimSpace(stringOr(m["id"], ""))
		if kind := ompModelKind(m); kind != "chat" {
			if id == "" {
				id = "(未命名模型)"
			}
			return fmt.Sprintf("模型「%s」的 kind 为「%s」，P1 只建模 chat 模型", id, kind)
		}
	}
	return ""
}

// BuildOMPCard 把一个 OMP 模型节点映射为前端卡片。
//
// providerID 只用于错误/调试上下文；baseURL 用于能力表的站点层；协议名优先取模型级
// api，其次用 baseUrl 推断（导出入口拿不到 provider 节点，因此无法继承 provider 级 api，
// 需要精确继承时用 ompBuildCard）。
func BuildOMPCard(raw map[string]interface{}, providerID, baseURL string) ModelCard {
	wire := strings.TrimSpace(stringOr(raw["api"], ""))
	return ompBuildCard(raw, providerID, baseURL, wire, nil)
}

// ompBuildCard 是卡片映射的唯一实现。providerAPI 是 provider 级 api（模型未写 api 时生效），
// providerCompat 是 provider 级 compat（与模型级 compat 合并后参与 G3 ladder 推导）。
func ompBuildCard(raw map[string]interface{}, providerID, baseURL, providerAPI string, providerCompat map[string]interface{}) ModelCard {
	modelID := strings.TrimSpace(stringOr(raw["id"], ""))
	card := ModelCard{
		ModelID:  modelID,
		Name:     strings.TrimSpace(stringOr(raw["name"], "")),
		Variants: make([]string, 0, 1+len(ompEffortOrder)),
	}
	wire := providerAPI
	if own := strings.TrimSpace(stringOr(raw["api"], "")); own != "" {
		wire = own
	}
	// 能力分层表按 ZCode 的 api.type 口径求值（KindToConfig 把 responses 折成 "openai"）。
	capAPI := ""
	if kind := ompWireAPIToKind(wire); kind != "" {
		capAPI = KindToConfig[kind]
	}
	cap := LookupModelCapabilityForSite(modelID, strings.TrimSpace(baseURL), capAPI)

	card.Vendor = cap.Vendor
	if card.Vendor == "" {
		card.Vendor = detectVendor(modelID)
	}

	if n, ok := ompPositiveInt(raw["contextWindow"]); ok {
		card.Context = n
	}
	if n, ok := ompPositiveInt(raw["maxTokens"]); ok {
		card.Output = n
	}

	// 思考档位（计划 §4.6 读侧口径）。
	mapping := OMPEffortMappingFor(raw, wire, providerCompat)
	reasoning := ompReasoningEnabled(raw)
	variants := []string{ompOffVariant}
	if reasoning {
		card.Reasoning = true
		card.RawReasoningEnabled = featureBool(true)
		if len(mapping.Explicit) > 0 {
			variants = append(variants, mapping.Explicit...)
			card.SupportedLevels = cloneStrings(mapping.Explicit)
		} else {
			variants = append(variants, mapping.Levels...)
			card.SupportedLevels = cloneStrings(mapping.Levels)
		}
		card.DefaultVariant = ompDefaultVariant(variants, mapping.Default)
	} else {
		// 关闭思考时档位集合只有 off（写回时删掉 thinking 与 reasoning，不写 false）。
		card.DefaultVariant = ompOffVariant
	}
	card.Variants = variants

	// 文件明确写出的能力位（三态：只有明确 true 才写指针）。
	explicit := ModelFeatures{}
	if mods := ompInputModalities(raw["input"]); len(mods) > 0 {
		explicit.InputModalities = mods
	}
	if toolCall, ok := ompBoolValue(raw["supportsTools"]); ok && toolCall {
		explicit.ToolCall = featureBool(true)
	}
	if explicit.HasAny() {
		card.Capabilities = &explicit
	}
	// 能力表推断（G22）：裁剪成 OMP 能表达的位——输入模态只留 text/image，工具调用照抄。
	if inferred := ompExpressibleFeatures(cap.Features); inferred.HasAny() {
		card.CapabilitiesInferred = &inferred
	}

	if headers := ompHeaderMap(raw["headers"]); len(headers) > 0 {
		card.Headers = headers
	}
	if len(raw) > 0 {
		card.RawCfg = deepCopyMap(raw)
	}
	return card
}

// ompExpressibleFeatures 把能力表推断裁剪为 OMP 可表达的位子集（G22）。
func ompExpressibleFeatures(in ModelFeatures) ModelFeatures {
	out := ModelFeatures{}
	var mods []string
	for _, m := range in.InputModalities {
		if m == "text" || m == "image" {
			mods = append(mods, m)
		}
	}
	if len(mods) > 0 {
		out.InputModalities = NormalizeInputModalities(mods)
	}
	if in.ToolCall != nil && *in.ToolCall {
		out.ToolCall = featureBool(true)
	}
	return out
}

// ompInputModalities 读模型 input 字段，只保留 OMP 支持的 text / image（其它词丢弃）。
func ompInputModalities(v interface{}) []string {
	list := ompStringSlice(v)
	if len(list) == 0 {
		return nil
	}
	kept := make([]string, 0, len(list))
	for _, item := range list {
		word := strings.ToLower(strings.TrimSpace(item))
		if word == "text" || word == "image" {
			kept = append(kept, word)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return NormalizeInputModalities(kept)
}

// OMPSelectors 扫描 models 文档，生成 config.yml modelRoles 可用的选择器列表
// （计划 §5 / G6）：只列 models[] 里**显式声明**的模型；只列协议可映射为三种主流
// wire api 的 provider；按 provider 再 model 排序；与当前 default 值无关。
func OMPSelectors(src OMPModelsSource) []string {
	if src == nil {
		return []string{}
	}
	providers, err := src.Providers()
	if err != nil {
		return []string{}
	}
	out := make([]string, 0, len(providers))
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, pid := range ids {
		raw := providers[pid]
		if raw == nil {
			continue
		}
		wire := ompProviderWireAPI(raw)
		if ompWireAPIToKind(wire) == "" {
			continue
		}
		models := ompModelList(raw)
		modelIDs := make([]string, 0, len(models))
		for _, m := range models {
			mid := strings.TrimSpace(stringOr(m["id"], ""))
			if mid == "" || ompModelKind(m) != "chat" {
				continue
			}
			own := strings.TrimSpace(stringOr(m["api"], ""))
			if own != "" && ompWireAPIToKind(own) == "" {
				continue
			}
			modelIDs = append(modelIDs, mid)
		}
		sort.Strings(modelIDs)
		for _, mid := range modelIDs {
			out = append(out, pid+"/"+mid)
		}
	}
	return out
}

// OMPReferences 汇总所有「可能引用 provider / model」的静态来源，供删除保护使用。
type OMPReferences struct {
	// Roles 是 config.yml 的 modelRoles（role → selector）。
	Roles map[string]string
	// Presets 是 config.yml 的 modelPresets（preset 名 → role → selector）。
	Presets map[string]map[string]string
	// InFile 是 models.yml 内部引用：providerID → 该 provider 模型里
	// compactionModel / contextPromotionTarget 的原文（可能是同 provider 的模型 ID，
	// 也可能是 "provider/model"）。
	InFile map[string][]string
	// Models 是 providerID → 该 provider 已知的模型 ID（models[].id）。
	// 它只用于让 "*/model" 这类通配选择器精确到「真的含该模型的 provider」，
	// 缺省（nil）时按「无法判定」保守拒绝。
	Models map[string][]string
}

// OMPReferenceGuard 返回阻止删除的中文原因；空串表示可以安全删除。
//
// modelID 为空表示要删整个 provider；非空表示只删该 provider 下的一个模型。
// 匹配口径**刻意保守**（计划 §5 的 G11）：
//   - 精确匹配 provider/model；
//   - "provider/*"（可带 :effort）命中该 provider 的任何模型；
//   - "*/model" 命中任何 provider 里的同名模型；
//   - "*" 或 "@role" 无法静态判定引用目标，删除任何 provider/model 都拒绝并提示；
//   - 无法解析的 selector（不含 "/"）忽略，不阻断删除。
func OMPReferenceGuard(refs OMPReferences, providerID, modelID string) string {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	if providerID == "" {
		return ""
	}
	type ref struct {
		where    string
		selector string
	}
	var all []ref
	for role, sel := range refs.Roles {
		all = append(all, ref{where: "modelRoles." + role, selector: sel})
	}
	presetNames := make([]string, 0, len(refs.Presets))
	for name := range refs.Presets {
		presetNames = append(presetNames, name)
	}
	sort.Strings(presetNames)
	for _, name := range presetNames {
		roles := refs.Presets[name]
		roleNames := make([]string, 0, len(roles))
		for role := range roles {
			roleNames = append(roleNames, role)
		}
		sort.Strings(roleNames)
		for _, role := range roleNames {
			all = append(all, ref{where: "modelPresets." + name + ".modelRoles." + role, selector: roles[role]})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].where != all[j].where {
			return all[i].where < all[j].where
		}
		return all[i].selector < all[j].selector
	})

	for _, item := range all {
		sel := strings.TrimSpace(item.selector)
		if sel == "" {
			continue
		}
		if sel == "*" || strings.HasPrefix(sel, "@") {
			return fmt.Sprintf("「%s」使用了无法静态判定的选择器「%s」，P1 不自动修改引用", item.where, sel)
		}
		provPattern, modelPattern, ok := ompParseSelector(sel)
		if !ok {
			continue
		}
		if provPattern != providerID && provPattern != "*" {
			continue
		}
		if modelID == "" {
			// 删除整个 provider：pattern 精确命中，或 "*" 覆盖它，或（"*/model" 时）
			// 该 provider 确实含被引用的模型——后者需要已知模型集合。
			if provPattern == providerID || modelPattern == "*" {
				return fmt.Sprintf("「%s」正在使用该提供商的模型（%s）", item.where, sel)
			}
			known, ok := refs.Models[providerID]
			if !ok {
				return fmt.Sprintf("「%s」使用了跨 provider 的通配选择器「%s」，P1 无法判定该提供商是否含此模型", item.where, sel)
			}
			if containsStr(known, modelPattern) {
				return fmt.Sprintf("「%s」正在使用该提供商的模型（%s）", item.where, sel)
			}
			continue
		}
		if modelPattern == "*" || modelPattern == modelID {
			return fmt.Sprintf("「%s」正在使用模型「%s」", item.where, sel)
		}
	}

	if modelID == "" {
		return ""
	}
	// models.yml 内部的 compactionModel / contextPromotionTarget 引用。
	for _, rawRef := range refs.InFile[providerID] {
		ref := strings.TrimSpace(rawRef)
		if ref == "" {
			continue
		}
		refProvider := providerID
		refModel := ref
		if provPattern, modelPattern, ok := ompParseSelector(ref); ok {
			refProvider, refModel = provPattern, modelPattern
		}
		if refProvider != providerID {
			continue
		}
		if refModel == modelID {
			return fmt.Sprintf("该提供商内有模型通过 compactionModel 或 contextPromotionTarget 引用「%s」", modelID)
		}
	}
	return ""
}

// ompParseSelector 把 OMP 选择器拆成 provider 模式与 model 模式。
// 按第一个 "/" 切分；model 段允许含 "@"，末尾 ":<六档>" 视为档位后缀并剥离。
// 不含 "/" 的字符串返回 ok=false（不是合法的 OMP 选择器）。
func ompParseSelector(sel string) (providerPattern, modelPattern string, ok bool) {
	sel = strings.TrimSpace(sel)
	idx := strings.Index(sel, "/")
	if idx <= 0 || idx == len(sel)-1 {
		return "", "", false
	}
	providerPattern = sel[:idx]
	modelPattern = sel[idx+1:]
	// 末尾 ":<六档>" 是档位后缀，不参与模型匹配。
	if cut := strings.LastIndex(modelPattern, ":"); cut > 0 {
		if containsStr(ompEffortOrder, modelPattern[cut+1:]) {
			modelPattern = modelPattern[:cut]
		}
	}
	if modelPattern == "" {
		return "", "", false
	}
	return providerPattern, modelPattern, true
}

// ompProviderWireAPI 求 provider 生效的协议名：
// provider 级 api 优先；否则若 models[] 各自声明的 api 完全一致就用它；
// 存在多种时返回 "mixed"；全都没有时返回空串（= 无法确定）。
func ompProviderWireAPI(raw map[string]interface{}) string {
	if api := strings.TrimSpace(stringOr(raw["api"], "")); api != "" {
		return api
	}
	seen := map[string]bool{}
	var first string
	for _, m := range ompModelList(raw) {
		api := strings.TrimSpace(stringOr(m["api"], ""))
		if api == "" {
			continue
		}
		if first == "" {
			first = api
		}
		seen[api] = true
	}
	switch len(seen) {
	case 0:
		return ""
	case 1:
		return first
	default:
		return "mixed"
	}
}

// ompWireAPIToKind 把 OMP wire api 映射为 UI kind；不可建模时返回空串（fail-closed）。
func ompWireAPIToKind(wire string) string {
	switch strings.TrimSpace(wire) {
	case OMPWireAnthropicMessages:
		return "anthropic"
	case OMPWireOpenAIResponses:
		return "responses"
	case OMPWireOpenAICompletions:
		return "openai-compatible"
	}
	return ""
}

// OMPWireAPIForKind 是 ompWireAPIToKind 的逆映射，供写盘时确定 model 级 api。
// kind 不合法时返回空串。
func OMPWireAPIForKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "anthropic":
		return OMPWireAnthropicMessages
	case "responses":
		return OMPWireOpenAIResponses
	case "openai-compatible":
		return OMPWireOpenAICompletions
	}
	return ""
}

// ompAPIKeyKind 判定 apiKey 形态（G4）：空 = none；"!" 前缀 = command；
// 其余整串按环境变量名精确查找，命中 = env，否则 = literal。绝不执行 command。
func ompAPIKeyKind(rawKey string) string {
	key := strings.TrimSpace(rawKey)
	if key == "" {
		return "none"
	}
	if strings.HasPrefix(key, "!") {
		return "command"
	}
	if _, ok := os.LookupEnv(key); ok {
		return "env"
	}
	return "literal"
}

// ompAPIKeyRequired 按 auth 口径给出「是否需要在编辑器里显示 API Key 输入」：
// 无 auth / apiKey → true；none / oauth → false。
func ompAPIKeyRequired(auth string) bool {
	switch strings.TrimSpace(auth) {
	case "none", "oauth":
		return false
	}
	return true
}

// ompReasoningEnabled 读模型是否开启思考（reasoning: true）。
func ompReasoningEnabled(raw map[string]interface{}) bool {
	if v, ok := ompBoolValue(raw["reasoning"]); ok {
		return v
	}
	return false
}

// ompModelKind 读模型 kind，缺省 chat。
func ompModelKind(raw map[string]interface{}) string {
	kind := strings.ToLower(strings.TrimSpace(stringOr(raw["kind"], "")))
	if kind == "" {
		return "chat"
	}
	return kind
}

// ompModelList 把 provider 节点的 models[] 拆成 []map[string]interface{}。
// 非 map 的条目直接丢弃（非法配置由 ompprovider.Validate 给出中文错误）。
func ompModelList(provider map[string]interface{}) []map[string]interface{} {
	if provider == nil {
		return nil
	}
	items, ok := provider["models"].([]interface{})
	if !ok {
		if typed, ok := provider["models"].([]map[string]interface{}); ok {
			return typed
		}
		return nil
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok && m != nil {
			out = append(out, m)
		}
	}
	return out
}

// ompMap 把任意值转成 map（YAML 解析产物可能是 map[string]interface{} 或 map[interface{}]interface{}）。
func ompMap(v interface{}) map[string]interface{} {
	switch typed := v.(type) {
	case map[string]interface{}:
		return typed
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(typed))
		for k, val := range typed {
			if ks, ok := k.(string); ok {
				out[ks] = val
			}
		}
		return out
	}
	return nil
}

// ompStringSlice 把 []interface{} / []string 统一成 []string（丢弃非字符串元素）。
func ompStringSlice(v interface{}) []string {
	switch typed := v.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// ompBoolValue 容忍 bool 与 "true"/"false" 字符串两种写法。
func ompBoolValue(v interface{}) (bool, bool) {
	switch typed := v.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}

// ompPositiveInt 读正整数（int / int64 / uint64 / float64 / 字符串都容忍）。
func ompPositiveInt(v interface{}) (int, bool) {
	switch typed := v.(type) {
	case int:
		return typed, typed > 0
	case int64:
		return int(typed), typed > 0
	case uint64:
		return int(typed), typed > 0
	case float64:
		n := int(typed)
		return n, n > 0
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		return n, err == nil && n > 0
	}
	return 0, false
}

// ompHeaderMap 把 headers 收敛成 map[string]string（值必须是字符串；其余丢弃）。
func ompHeaderMap(v interface{}) map[string]string {
	raw := ompMap(v)
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, val := range raw {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
