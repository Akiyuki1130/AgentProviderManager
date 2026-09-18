package core

import (
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
	RegisterBackendForFormat(string(AgentZCode), zcodeprovider.FormatV2, zcodeV2Backend{})
}

func (zcodeV2Backend) Descriptor() BackendDescriptor {
	return BackendDescriptor{
		ID:          string(AgentZCode),
		Label:       "ZCode",
		DefaultPath: ZCodeProviderConfigPath(),
		Candidates:  DetectConfigLocationsForAgent(string(AgentZCode)),
	}
}

// Read 读取并严格校验新版配置。文件不存在等价于一份空的新版配置（与 LoadConfig
// 对缺失文件返回空 map 的语义对齐）。
func (zcodeV2Backend) Read(path string) (Doc, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return zcodeprovider.NewConfig().Doc(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &ConfigParseError{Msg: fmt.Sprintf("读取配置文件失败：%v", err)}
	}
	cfg, err := zcodeprovider.Decode(data)
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
	cards := make([]ModelCard, 0)
	for _, mr := range cfg.ProviderModelRules() {
		if zcodeprovider.ModelRuleProviderID(mr) != providerID {
			continue
		}
		cards = append(cards, v2ModelCard(zcodeprovider.ModelRuleModelID(mr), mr))
	}
	sort.Slice(cards, func(i, j int) bool {
		return strings.ToLower(cards[i].ModelID) < strings.ToLower(cards[j].ModelID)
	})
	return &ProviderEdit{
		ID:             providerID,
		Name:           name,
		Kind:           uiKindForV2Provider(providerCfg),
		BaseURL:        zcodeprovider.ProviderAPIBaseURL(providerCfg),
		APIKey:         apiKey,
		APIKeyRequired: strings.TrimSpace(apiKey) != "",
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
func (zcodeV2Store) Upsert(doc Doc, providerID string, p ProviderPayload, _ SaveOptions) (Doc, error) {
	newID, providerCfg, err := BuildProviderCfgEditor(p)
	if err != nil {
		return nil, err
	}
	cfg := zcodeprovider.NewConfigFromDoc(doc)
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
// 中间形态复用 legacy 的模型配置形状（limit / reasoning.variants / modalities.input），
// 这样 CfgToCard 的既有语义（含 “off” 与 “disabled” 的对应关系）不需要重写。
func v2ModelCard(modelID string, mr map[string]interface{}) ModelCard {
	mcfg := zcodeprovider.ModelRuleConfig(mr)
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
		for _, v := range values {
			if v == "disabled" {
				v = "off"
			}
			variants = append(variants, v)
		}
		reasoning := map[string]interface{}{"variants": variants}
		if enabled, ok := zcodeprovider.ModelEnabled(mcfg); ok {
			reasoning["enabled"] = enabled
		}
		legacy["reasoning"] = reasoning
	}
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
	return CfgToCard(modelID, legacy)
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

// mergeV2ModelRuleConfig 保留已有模型规则里本包未建模的合法字段：
// config.enabled，以及 optionSpecs 下各选项的 map（取值映射，不能丢）。
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
	for _, key := range []string{"reasoningLevel", "maxOutputTokens"} {
		prevOpt, _ := prevSpecs[key].(map[string]interface{})
		if prevOpt == nil {
			continue
		}
		raw, ok := prevOpt["map"]
		if !ok {
			continue
		}
		specs := zcodeprovider.ModelOptionSpecs(incoming)
		if specs == nil {
			continue
		}
		if cur, _ := specs[key].(map[string]interface{}); cur != nil {
			cur["map"] = raw
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
