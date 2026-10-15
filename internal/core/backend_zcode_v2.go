package core

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"agentprovidermanager/internal/zcodeprovider"
)

// zcodeV2Backend 是 ZCode 新版个人 provider_config.json 的 backend。
//
// 与 legacy backend 一样只做“文档视图 + 编解码”，备份与指纹校验的调用时机仍由 app 层
// 决定（app 层在写盘前调用 BackupConfig，并把读到的指纹传进 Write）。
//
// 关键约束：写盘必须经过 zcodeprovider.Encode。该格式是严格校验的，任何一个未知键
// 都会让 ZCode 把整份配置当成空，因此不能把内存里的 map 直接 json.Marshal 落盘。
type zcodeV2Backend struct{}

func init() {
	RegisterBackendForFormat(string(AgentZCode), BackendFormatZCodeV2, zcodeV2Backend{})
}

func (zcodeV2Backend) Descriptor() BackendDescriptor {
	return BackendDescriptor{
		ID:          string(AgentZCode),
		Label:       "ZCode",
		DefaultPath: ZCodeProviderConfigPath(),
		Candidates:  DetectConfigLocationsForAgent(string(AgentZCode)),
	}
}

// Read 读取并校验新版配置。文件不存在等价于一份空的新版配置（与 LoadConfig
// 对缺失文件返回空 map 的语义对齐）。
//
// 这里用宽松解码：ZCode 3.14 会拒绝、但本包仍能读写的兼容性问题（builtinModelIds、
// 家族 group、旧 logo 形状等）不该让配置打不开——它们由 InspectZCodeCompat
// 单独列出，UI 再提示修复。结构性问题（JSON 形状、必填字段）仍然返回错误。
func (zcodeV2Backend) Read(path string) (Doc, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return zcodeprovider.NewConfig().Doc(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigParseError{Msg: fmt.Sprintf("读取配置文件失败：%v", err)}
	}
	// 空文件（例如刚被创建出来、或上次写入中断留下 0 字节）等价于一份空的新版配置：
	// 否则第一次保存会以“解析失败：EOF”直接拒绝，用户只能手动填内容。
	if strings.TrimSpace(string(data)) == "" {
		return zcodeprovider.NewConfig().Doc(), nil
	}
	cfg, _, err := zcodeprovider.DecodeInspect(data)
	if err != nil {
		return nil, &ConfigParseError{Msg: err.Error()}
	}
	return cfg.Doc(), nil
}

func (zcodeV2Backend) ReadWithFingerprint(path string) (Doc, string, error) {
	before, _ := FileFingerprint(path)
	doc, err := zcodeV2Backend{}.Read(path)
	if err != nil {
		return nil, "", err
	}
	after, _ := FileFingerprint(path)
	if before != after {
		return nil, "", fmt.Errorf("配置文件在读取期间被其他程序修改，请重试")
	}
	return doc, after, nil
}

func (zcodeV2Backend) Write(path string, doc Doc, expectedFingerprint string) error {
	data, err := zcodeprovider.Encode(zcodeprovider.NewConfigFromDoc(doc))
	if err != nil {
		return err
	}
	return WriteConfigBytes(path, data, expectedFingerprint)
}

func (zcodeV2Backend) Store(_ Doc) ProviderStore {
	return zcodeV2Store{}
}

// zcodeV2Store 在新版文档上提供 provider / model 视图。
type zcodeV2Store struct{}

// List 按 providerRule 生成摘要。字段取自 providerRule 本身（不经过 legacy 形态），
// 这样没有 api 的供应商（例如内建系列）也不会让整张列表报错。
func (zcodeV2Store) List(doc Doc) []ProviderSummary {
	cfg := zcodeprovider.NewConfigFromDoc(doc)
	rules := cfg.ProviderRules()
	out := make([]ProviderSummary, 0, len(rules))
	for _, rule := range rules {
		id := zcodeprovider.RuleProviderID(rule)
		if id == "" {
			continue
		}
		providerCfg := zcodeprovider.RuleConfig(rule)
		baseURL := zcodeprovider.ProviderAPIBaseURL(providerCfg)
		name := strings.TrimSpace(zcodeprovider.RuleProviderName(rule))
		if name == "" {
			name = id
		}
		out = append(out, ProviderSummary{
			ID:         id,
			Name:       name,
			Kind:       uiKindForV2Provider(providerCfg),
			BaseURL:    baseURL,
			HasAPIKey:  strings.TrimSpace(zcodeprovider.ProviderAccessAPIKey(providerCfg)) != "",
			ModelCount: v2ModelCount(cfg, id, providerCfg),
		})
	}
	// 与 legacy BuildProviderSummary 一致：按 ID 不区分大小写升序。
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out
}

func (zcodeV2Store) Get(doc Doc, providerID string) (*ProviderEdit, error) {
	cfg := zcodeprovider.NewConfigFromDoc(doc)
	rule := cfg.ProviderRule(providerID)
	if rule == nil {
		return nil, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	providerCfg := zcodeprovider.RuleConfig(rule)
	apiKey := zcodeprovider.ProviderAccessAPIKey(providerCfg)
	name := strings.TrimSpace(zcodeprovider.RuleProviderName(rule))
	if name == "" {
		name = providerID
	}
	// 必须是非 nil 空切片：零模型 provider 不进循环，nil 会被编码成 JSON null，
	// 前端拿到 null 后在 cards.map 上抛错。
	// 站点上下文取 api.baseUrl 与 api.type：v2 是唯一能直接读到 ZCode api.type 的格式，
	// 站点级规则因此在这里可以完整参与（模型名 + baseUrl + apiType 三道门）。
	baseURL := zcodeprovider.ProviderAPIBaseURL(providerCfg)
	apiType := zcodeprovider.ProviderAPIType(providerCfg)
	cards := make([]ModelCard, 0)
	for _, mr := range cfg.ProviderModelRules() {
		if zcodeprovider.ModelRuleProviderID(mr) != providerID {
			continue
		}
		cards = append(cards, v2ModelCard(zcodeprovider.ModelRuleModelID(mr), mr, baseURL, apiType))
	}
	sort.Slice(cards, func(i, j int) bool {
		return strings.ToLower(cards[i].ModelID) < strings.ToLower(cards[j].ModelID)
	})
	// headers 之类“未单独建模但可表达”的字段通过 options_json 暴露：
	// 跨 Agent 迁移与编辑器保存都靠它把这些字段带到目标格式，否则会被静默丢掉。
	optionsJSON := ""
	if headers := zcodeprovider.ProviderAPIHeaders(providerCfg); len(headers) > 0 {
		if b, err := json.Marshal(map[string]interface{}{"headers": headers}); err == nil {
			optionsJSON = string(b)
		}
	}
	return &ProviderEdit{
		ID:             providerID,
		Name:           name,
		Kind:           uiKindForV2Provider(providerCfg),
		BaseURL:        baseURL,
		APIKey:         apiKey,
		APIKeyRequired: strings.TrimSpace(apiKey) != "",
		OptionsJSON:    optionsJSON,
		Cards:          cards,
	}, nil
}

// Upsert 把编辑器提交的 provider 转换并写入新版文档。
//
// 转换语义与 zcodeprovider.FromLegacy 一致（kind -> api.type、options -> api/access、
// models -> providerModelRules），但落点是在**已有文档上就地更新**：
// 与本包无关的合法字段（providerRule.enabled/templateId、config.logo/visibility/
// builtinModelIds/modelOrder、api.headers、access.apiKeyManagementUrl、
// modelConfigRule.config.enabled 与 optionSpecs.*.map）都会保留，避免一次保存把它们抹掉。
// 模型集合整体替换为本次提交的内容，与 legacy MergeProviderIntoConfig(mergeModels=false) 一致。
//
// 带 templateId 的模板派生行会被拒绝编辑：ZCode 的 zod schema 对这类行 omit 了
// personalModelIds/modelOrder，向其写入个人字段会让 ZCode 拒收整份个人配置。
func (zcodeV2Store) Upsert(doc Doc, providerID string, p ProviderPayload, _ SaveOptions) (Doc, error) {
	newID, providerCfg, err := BuildProviderCfgEditor(p)
	if err != nil {
		return nil, err
	}
	// FromLegacy 读取的是 provider 顶层的 headers（旧版 ZCode 形状），而编辑器/迁移载荷把
	// 额外字段放在 options 里；这里把它抬到顶层，否则 headers 在导入与跨 Agent 迁移时会丢。
	if opts, ok := providerCfg["options"].(map[string]interface{}); ok {
		if headers, ok := opts["headers"].(map[string]interface{}); ok && len(headers) > 0 {
			providerCfg["headers"] = headers
		}
	}
	cfg := zcodeprovider.NewConfigFromDoc(doc)
	for _, id := range []string{providerID, newID} {
		if tid, ok := zcodeprovider.RuleTemplateID(cfg.ProviderRule(id)); ok && tid != "" {
			return nil, fmt.Errorf("提供商「%s」由 ZCode 模板派生（templateId=%s），不能编辑：向模板行写入个人字段会让 ZCode 拒绝整份个人配置", id, tid)
		}
	}
	converted, _, err := zcodeprovider.FromLegacy(map[string]interface{}{
		"provider": map[string]interface{}{newID: providerCfg},
	})
	if err != nil {
		return nil, &ConfigParseError{Msg: err.Error()}
	}
	incoming := converted.ProviderRule(newID)
	if incoming == nil {
		return nil, &ConfigParseError{Msg: fmt.Sprintf("提供商「%s」无法转换为新版格式配置", newID)}
	}

	if newID != providerID {
		// 只把 providerId 换个值的“原地重命名”在这里无法表达，改名等于旧 ID 整体迁到新 ID。
		if cfg.ProviderRule(newID) != nil {
			return nil, &ConfigParseError{Msg: fmt.Sprintf("提供商「%s」已存在", newID)}
		}
		// 与 legacy RenameProviderInConfig 一致：整条规则（含本包未建模的字段）搬到新 ID，
		// 模型规则与 providerOrder 条目同步改名。
		if old := cfg.ProviderRule(providerID); old != nil {
			cfg.RemoveProviderRule(providerID)
			old["providerId"] = newID
			cfg.UpsertProviderRule(old)
			for _, mr := range cfg.ProviderModelRules() {
				if zcodeprovider.ModelRuleProviderID(mr) == providerID {
					mr["providerId"] = newID
				}
			}
			replaceV2ProviderOrderID(cfg, providerID, newID)
		}
	}

	existing := map[string]map[string]interface{}{}
	for _, mr := range cfg.ProviderModelRules() {
		if zcodeprovider.ModelRuleProviderID(mr) == newID {
			existing[zcodeprovider.ModelRuleModelID(mr)] = zcodeprovider.ModelRuleConfig(mr)
		}
	}

	if rule := cfg.ProviderRule(newID); rule != nil {
		mergeV2ProviderRule(rule, incoming)
	} else {
		cfg.UpsertProviderRule(incoming)
	}

	removeV2ModelRules(cfg, newID)
	for _, mr := range converted.ProviderModelRules() {
		if prev := existing[zcodeprovider.ModelRuleModelID(mr)]; prev != nil {
			mergeV2ModelRuleConfig(zcodeprovider.ModelRuleConfig(mr), prev)
		}
		cfg.UpsertProviderModelRule(mr)
	}
	appendV2ProviderOrder(cfg, newID)
	return cfg.Doc(), nil
}

// Delete 删除 providerRule、其模型规则与 providerOrder 条目。
// found=false 表示文档中没有该 provider（调用方据此返回“不存在”），此时返回 nil 文档。
func (zcodeV2Store) Delete(doc Doc, providerID string) (Doc, bool, error) {
	cfg := zcodeprovider.NewConfigFromDoc(doc)
	if cfg.ProviderRule(providerID) == nil {
		return nil, false, nil
	}
	cfg.RemoveProviderRule(providerID)
	removeV2ModelRules(cfg, providerID)
	removeV2ProviderOrderID(cfg, providerID)
	return cfg.Doc(), true, nil
}

// DeleteModel 删除某个模型：模型规则与 personalModelIds 里的条目一并去掉，
// 否则该模型仍会出现在 ZCode 的模型列表里。
// provider 不存在时以 error 返回原有文案；model 不存在时返回 (nil, false, nil)。
func (zcodeV2Store) DeleteModel(doc Doc, providerID, modelID string) (Doc, bool, error) {
	cfg := zcodeprovider.NewConfigFromDoc(doc)
	rule := cfg.ProviderRule(providerID)
	if rule == nil {
		return nil, false, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	found := removeV2ModelRule(cfg, providerID, modelID)
	if removeV2PersonalModelID(cfg, providerID, modelID) {
		found = true
	}
	if !found {
		return nil, false, nil
	}
	return cfg.Doc(), true, nil
}

// ---- v2 视图辅助 ----

// uiKindForV2Provider 把 api.type 映射回编辑器使用的 UI kind。
// api 缺失（例如内建系列供应商）时交给 ConfigKindToUIKind 按 baseUrl 推断。
func uiKindForV2Provider(providerCfg map[string]interface{}) string {
	configKind := ""
	switch zcodeprovider.ProviderAPIType(providerCfg) {
	case zcodeprovider.APITypeAnthropicMessages:
		configKind = "anthropic"
	case zcodeprovider.APITypeOpenAIResponses:
		configKind = "openai"
	case zcodeprovider.APITypeOpenAIChatCompletions:
		configKind = "openai-compatible"
	}
	return ConfigKindToUIKind(configKind, zcodeprovider.ProviderAPIBaseURL(providerCfg))
}

func v2ModelCount(cfg *zcodeprovider.Config, providerID string, providerCfg map[string]interface{}) int {
	count := 0
	for _, mr := range cfg.ProviderModelRules() {
		if zcodeprovider.ModelRuleProviderID(mr) == providerID {
			count++
		}
	}
	if count == 0 {
		count = len(zcodeprovider.ProviderPersonalModelIDs(providerCfg))
	}
	return count
}

// v2ModelCard 把 providerModelRule 还原成编辑器卡片。
// 中间形态由 v2LegacyModelCfg 构造（复用 legacy 的模型配置形状：limit /
// reasoning.variants / modalities.input），这样 CfgToCard 的既有语义（含 “off” 与
// “disabled” 的对应关系）不需要重写。
// baseURL / apiType 是该 provider 的 api.baseUrl 与 api.type，透传给能力表以启用
// 离线目录与站点级规则两层推断。
func v2ModelCard(modelID string, mr map[string]interface{}, baseURL, apiType string) ModelCard {
	return CfgToCardForSite(modelID, v2LegacyModelCfg(zcodeprovider.ModelRuleConfig(mr)), baseURL, apiType)
}

// v2LegacyModelCfg 构造 v2 providerModelRule 对应的 legacy 中间形态。
// 独立成函数是为了让「读回」的中间产物可被直接断言（不必经过卡片），
// v2ModelCard 只是它加一层 CfgToCardForSite。
func v2LegacyModelCfg(mcfg map[string]interface{}) map[string]interface{} {
	legacy := map[string]interface{}{}
	limit := map[string]interface{}{}
	if n, ok := zcodeprovider.ModelContextWindow(mcfg); ok {
		limit["context"] = n
	}
	if n, ok := zcodeprovider.ModelMaxOutputTokens(mcfg); ok {
		limit["output"] = n
	}
	if len(limit) > 0 {
		legacy["limit"] = limit
	}
	if values := zcodeprovider.ModelReasoningLevelValues(mcfg); len(values) > 0 {
		variants := make([]interface{}, 0, len(values))
		offAlias := ""
		for _, v := range values {
			// 契约要求：原始串原样放进 variants（不在这里改拼法），并记下 off 词的
			// 原拼法。规范化和 off→off 折叠由 CfgToCard/NormalizeVariants 负责，
			// 这样原始拼法才能一路带到写回侧（修 A-1/A-8）。
			variants = append(variants, v)
			if offAlias == "" {
				switch strings.ToLower(strings.TrimSpace(v)) {
				case "disabled", "none", "nothink":
					offAlias = strings.ToLower(strings.TrimSpace(v))
				}
			}
		}
		reasoning := map[string]interface{}{"variants": variants}
		if offAlias != "" {
			reasoning["offAlias"] = offAlias
		}
		// 注意：绝不把 config.enabled 映射进 reasoning["enabled"]。内建数据实证
		// config.enabled 是「模型是否在模板/列表里默认启用」的启用位，与「是否开启思考」
		// 无关（详见 mergeV2ModelRuleConfig 注释）。映射进去会让 CfgToCard 采信这个布尔
		// 而跳过由 values 反推，导致关闭思考（values=["disabled"]）读回仍是开启，S1 无法
		// 持久化。config.enabled 由 mergeV2ModelRuleConfig 原样保留，不在此处消费。
		legacy["reasoning"] = reasoning
	}
	// 既有行为保持不动：modalities.input 仍只承载 image/video 两位（其余 3 位走
	// `_apm_capabilities`，由下述 v2ApmCapabilities 带出，写入端取并集）。
	if format := zcodeprovider.ModelInputFormat(mcfg); format != nil {
		var inputs []interface{}
		if b, _ := format["supportsImage"].(bool); b {
			inputs = append(inputs, "image")
		}
		if b, _ := format["supportsVideo"].(bool); b {
			inputs = append(inputs, "video")
		}
		if len(inputs) > 0 {
			legacy["modalities"] = map[string]interface{}{"input": inputs}
		}
	}
	// 中立能力位：把 v2 认的全部能力键收进内部通道，供卡片的 Capabilities 与
	// 三个 harness 的写入端共用（见 10.2.4②）。
	if caps := v2ApmCapabilities(mcfg); len(caps) > 0 {
		legacy[apmCapabilitiesModelKey] = caps
	}
	return legacy
}

// ---- 卡片 ↔ 写入端之间的能力通道（E1，冻结契约见 10.2.4②）----

// apmCapabilitiesModelKey 是卡片与各 harness 写入端之间的内部能力通道键。
// 带 `_` 前缀，属 APM 内部字段：写盘出口由 cards.go 的 stripAPMInternalModelKeys 统一剥离，
// 各 harness 的写端（dshModelToRaw / OpenCodeModelFromCfg）也必须显式丢弃它。
const apmCapabilitiesModelKey = "_apm_capabilities"

// apmCapabilityKeyMap 是一条「harness 侧键名 → 中立能力位」的映射。
type apmCapabilityKeyMap struct {
	key        string
	capability string
}

// apmCapabilitiesChannel 读取中间形态/配置节点里的内部能力通道。
func apmCapabilitiesChannel(cfg map[string]interface{}) map[string]interface{} {
	m, _ := cfg[apmCapabilitiesModelKey].(map[string]interface{})
	return m
}

// apmCapabilityTrue 判断能力通道里的某位是否**明确为真**（只认 bool true）。
// 缺失 / nil / false 一律为假：调用方据此省略键，绝不写 false/null。
func apmCapabilityTrue(caps map[string]interface{}, key string) bool {
	v, ok := caps[key].(bool)
	return ok && v
}

// apmCapabilitiesPayload 组装能力通道的值：只收录**已知为真**的位。
// 模态为空则省略该列（不让空切片序列化成 null/[]）；布尔位只在 trueFlags 里出现时写 true。
// 没有任何已知位时返回 nil，调用方据此整个省略该键。
func apmCapabilitiesPayload(inputModalities, outputModalities []string, trueFlags ...string) map[string]interface{} {
	out := map[string]interface{}{}
	if len(inputModalities) > 0 {
		out["input_modalities"] = apmCapabilityStrings(inputModalities)
	}
	if len(outputModalities) > 0 {
		out["output_modalities"] = apmCapabilityStrings(outputModalities)
	}
	for _, flag := range trueFlags {
		if flag != "" {
			out[flag] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// apmCapabilityStrings 把规范词列表转成 []interface{}（元素为 string）。
// 用 []interface{} 而不是 []string：中间形态要经过 JSON 往返（deepCopyMap），
// 下游读端（cards.go 的 metaStringList、zcodeprovider 的 stringSlice）两种都认，
// 而 []interface{} 正是 json.Unmarshal 之后的原生形状。
func apmCapabilityStrings(words []string) []interface{} {
	out := make([]interface{}, 0, len(words))
	for _, word := range words {
		out = append(out, word)
	}
	return out
}

// apmModalityWords 把配置里的模态值读成字符串列表，兼容 []string / []interface{} /
// 单个字符串（其余形状读不出，返回 nil）。
// 这里不做词表过滤：规范词表与规范序统一由 model_features.go 的
// NormalizeInputModalities / NormalizeOutputModalities 负责（未列出的词 fail-closed 丢弃）。
func apmModalityWords(value interface{}) []string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// apmTrueCapabilityFlags 从 vals 里挑出「明确为真」的能力位：键存在且值为 bool true 才收录。
// mapping 是 harness 侧键名 → 中立能力位的映射（顺序即返回顺序）。
func apmTrueCapabilityFlags(vals map[string]interface{}, mapping []apmCapabilityKeyMap) []string {
	var flags []string
	for _, item := range mapping {
		if v, ok := vals[item.key].(bool); ok && v {
			flags = append(flags, item.capability)
		}
	}
	return flags
}

// v2InputFormatModalities 是 v2 `inputFormat.supports*` 键到中立输入模态词的映射，
// 顺序即 ZCode 的 canonical 键序（order.go 的 inputFormatOrder）。
var v2InputFormatModalities = []struct {
	key  string
	word string
}{
	{"supportsText", "text"},
	{"supportsImage", "image"},
	{"supportsVideo", "video"},
	{"supportsAudio", "audio"},
	{"supportsPdf", "pdf"},
}

// v2CapabilityPropertyKeys 是 v2 `properties` 布尔键到中立能力位的映射
// （与 10.2.3 的写盘口径一致）。
var v2CapabilityPropertyKeys = []apmCapabilityKeyMap{
	{"supportsToolCall", "tool_call"},
	{"supportsJsonSchemaOutput", "structured_output"},
	{"supportsNativeWebSearch", "native_web_search"},
	{"supportsMidConversationSystem", "mid_conv_system"},
}

// v2ApmCapabilities 把 v2 modelConfig 里 ZCode 认的能力键收进内部能力通道。
//
// 只收「明确为真」的位：v2 的 overlay 语义里省略键 = 不覆盖，false 是显式关闭，
// null 是显式清空，因此未知一律省略，一个 false/null 都不写。
// requiresMfjsToolSchema 与 optionSpecs.*.map 本轮不做，因此这里也不读它们。
func v2ApmCapabilities(mcfg map[string]interface{}) map[string]interface{} {
	props := zcodeprovider.ModelProperties(mcfg)
	if len(props) == 0 {
		return nil
	}
	var inputs []string
	format := zcodeprovider.ModelInputFormat(mcfg)
	for _, item := range v2InputFormatModalities {
		if b, _ := format[item.key].(bool); b {
			inputs = append(inputs, item.word)
		}
	}
	var outputs []string
	// v2 的 outputFormat 只有 supportsText 一个键，明确为真时等价于 output_modalities=["text"]。
	if b, _ := zcodeprovider.ModelOutputFormat(mcfg)["supportsText"].(bool); b {
		outputs = append(outputs, "text")
	}
	return apmCapabilitiesPayload(inputs, outputs, apmTrueCapabilityFlags(props, v2CapabilityPropertyKeys)...)
}

// mergeV2ProviderRule 把 incoming（FromLegacy 产出的 canonical 规则）合并进已有规则。
// 只覆盖本次编辑真正负责的字段，其它键原样保留。
func mergeV2ProviderRule(rule, incoming map[string]interface{}) {
	for k, v := range incoming {
		if k == "providerId" || k == "config" {
			continue
		}
		rule[k] = v
	}
	dst := zcodeprovider.RuleConfig(rule)
	if dst == nil {
		dst = map[string]interface{}{}
		rule["config"] = dst
	}
	src := zcodeprovider.RuleConfig(incoming)
	if group, ok := src["group"]; ok {
		dst["group"] = group
	}
	if api, ok := src["api"].(map[string]interface{}); ok {
		mergeV2Child(dst, "api", api)
	}
	if access, ok := src["access"].(map[string]interface{}); ok {
		mergeV2Child(dst, "access", access)
	} else if cur, ok := dst["access"].(map[string]interface{}); ok {
		// 编辑器清空了 API Key：与 legacy 的 delete(options.apiKey) 对齐。
		delete(cur, "apiKey")
	}
	if ids, ok := src["personalModelIds"]; ok {
		dst["personalModelIds"] = ids
	}
}

// mergeV2Child 把 src 的键覆盖到 dst[key] 上，保留 dst 中 src 未提及的键。
func mergeV2Child(dst map[string]interface{}, key string, src map[string]interface{}) {
	cur, _ := dst[key].(map[string]interface{})
	if cur == nil {
		cur = map[string]interface{}{}
		dst[key] = cur
	}
	for k, v := range src {
		cur[k] = v
	}
}

// mergeV2ModelRuleConfig 保留已有模型规则里本包未建模、或本次提交无从表达的合法字段：
// config.enabled，以及 optionSpecs 下各选项的 map（取值映射，不能丢）。
//
// 语义提醒（A-7）：config.enabled 是「该模型是否在模板/列表里默认启用」的启用位，
// 与「是否开启思考」无关——内建数据里它只出现在 templateModelRules/builtinProviderModelRules，
// modelRules/modelApiRules 里一处都没有。ZCode 的「关闭思考」只通过
// optionSpecs.reasoningLevel（选中 off 词）表达。因此这里对 enabled 只做
// “prev 有、incoming 未写则保留”的兜底，绝不把它与卡片 Reasoning 语义关联。
func mergeV2ModelRuleConfig(incoming, prev map[string]interface{}) {
	if v, ok := prev["enabled"]; ok {
		if _, exists := incoming["enabled"]; !exists {
			incoming["enabled"] = v
		}
	}
	prevSpecs := zcodeprovider.ModelOptionSpecs(prev)
	if len(prevSpecs) == 0 {
		return
	}
	incomingSpecs := zcodeprovider.ModelOptionSpecs(incoming)
	if incomingSpecs == nil {
		// incoming 完全没写 optionSpecs（例如关闭思考且卡片档位为空）：把 prev 的
		// optionSpecs 整体带过来。这是 A-8 的关键修复——单元素 off 集合（["disabled"]）
		// 读回卡片后可能不再写出任何 optionSpecs，若此处 continue 会让档位与 map
		// 一并被静默丢弃。整体保留即可同时保住 values 与 map。
		incoming["optionSpecs"] = deepCopyMap(prevSpecs)
		return
	}
	for _, key := range []string{"reasoningLevel", "maxOutputTokens"} {
		prevOpt, _ := prevSpecs[key].(map[string]interface{})
		if prevOpt == nil {
			continue
		}
		raw, ok := prevOpt["map"]
		if !ok {
			continue
		}
		if cur, _ := incomingSpecs[key].(map[string]interface{}); cur != nil {
			cur["map"] = raw
		}
	}
	// reasoningLevel 特例：incoming 有 optionSpecs 但没有 reasoningLevel 键时
	// （例如本次只写了 maxOutputTokens），把 prev 的 reasoningLevel 整项带过来。
	// 抉择理由：APM 唯一能表达档位的载体就是 values；incoming 未写它说明本次提交
	// 没有要表达的档位，保留 prev 的 values+map 比清空更符合“不丢数据”的既定不变量，
	// 也正是 A-8「单元素 off 集合丢档位」要修的场景。
	//
	// 刻意取舍（M5）：由此一来，当 prev 已有 reasoningLevel 时，本次提交无法通过
	// “不写档位”来显式清空它（清空会被这里还原）。这是有意为之——保留 map/档位
	// 优先于“可清空”。真正要关闭思考走的是 S1：把集合收敛为 ["off"] 显式写出，
	// 而不是提交空集合。
	if _, ok := incomingSpecs["reasoningLevel"]; !ok {
		if prevRL, ok := prevSpecs["reasoningLevel"]; ok {
			incomingSpecs["reasoningLevel"] = deepCopyValue(prevRL)
		}
	}
}

// ---- v2 文档结构的低层改写 ----
//
// zcodeprovider 只导出了 providerRule 的增删；providerModelRules / providerOrder
// 需要按 (providerId, modelId) 局部删除，这里直接在其文档路径上操作。

func v2ChildObj(doc map[string]interface{}, keys ...string) map[string]interface{} {
	cur := doc
	for _, key := range keys {
		next, _ := cur[key].(map[string]interface{})
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func v2ProviderModelRulesArray(cfg *zcodeprovider.Config) (map[string]interface{}, []interface{}) {
	rules := v2ChildObj(cfg.Doc(), "config", "modelConfigRules")
	if rules == nil {
		return nil, nil
	}
	arr, _ := rules["providerModelRules"].([]interface{})
	return rules, arr
}

func removeV2ModelRule(cfg *zcodeprovider.Config, providerID, modelID string) bool {
	rules, arr := v2ProviderModelRulesArray(cfg)
	if rules == nil {
		return false
	}
	kept := make([]interface{}, 0, len(arr))
	removed := false
	for _, raw := range arr {
		m, _ := raw.(map[string]interface{})
		if !removed && m != nil &&
			zcodeprovider.ModelRuleProviderID(m) == providerID &&
			zcodeprovider.ModelRuleModelID(m) == modelID {
			removed = true
			continue
		}
		kept = append(kept, raw)
	}
	if removed {
		rules["providerModelRules"] = kept
	}
	return removed
}

func removeV2ModelRules(cfg *zcodeprovider.Config, providerID string) {
	rules, arr := v2ProviderModelRulesArray(cfg)
	if rules == nil {
		return
	}
	kept := make([]interface{}, 0, len(arr))
	for _, raw := range arr {
		m, _ := raw.(map[string]interface{})
		if m != nil && zcodeprovider.ModelRuleProviderID(m) == providerID {
			continue
		}
		kept = append(kept, raw)
	}
	rules["providerModelRules"] = kept
}

// removeV2PersonalModelID 从 providerRule.config.personalModelIds 中删掉一个模型。
// 返回是否确实删除过。
func removeV2PersonalModelID(cfg *zcodeprovider.Config, providerID, modelID string) bool {
	rule := cfg.ProviderRule(providerID)
	if rule == nil {
		return false
	}
	ids := zcodeprovider.ProviderPersonalModelIDs(zcodeprovider.RuleConfig(rule))
	if ids == nil {
		return false
	}
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == modelID {
			continue
		}
		kept = append(kept, id)
	}
	if len(kept) == len(ids) {
		return false
	}
	cfg.SetProviderPersonalModelIDs(providerID, kept)
	return true
}

func appendV2ProviderOrder(cfg *zcodeprovider.Config, providerID string) {
	order := cfg.ProviderOrder()
	for _, id := range order {
		if id == providerID {
			return
		}
	}
	cfg.SetProviderOrder(append(order, providerID))
}

func replaceV2ProviderOrderID(cfg *zcodeprovider.Config, oldID, newID string) {
	order := cfg.ProviderOrder()
	if len(order) == 0 {
		return
	}
	for i, id := range order {
		if id == oldID {
			order[i] = newID
		}
	}
	cfg.SetProviderOrder(order)
}

func removeV2ProviderOrderID(cfg *zcodeprovider.Config, providerID string) {
	order := cfg.ProviderOrder()
	if len(order) == 0 {
		return
	}
	kept := make([]string, 0, len(order))
	for _, id := range order {
		if id == providerID {
			continue
		}
		kept = append(kept, id)
	}
	cfg.SetProviderOrder(kept)
}
