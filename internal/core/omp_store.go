package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// OMP 文档的内部键。
//
// 这些键只活在内存里的 Doc 上：ompBackend.Read 填充、ompStore 读写、
// ompBackend.Write 序列化前绝不写回文件。任何写盘出口都必须把 "_" 前缀键剥掉，
// 否则 omp 会把它们当成未知的 provider / 模型配置。
const (
	ompDocFile           = "_omp_models_file"
	ompDocPath           = "_omp_path"
	ompDocFileName       = "_omp_file_name"
	ompDocWritable       = "_omp_writable"
	ompDocNeedsMigration = "_omp_needs_migration"
	ompDocAlias          = "_omp_alias_reasons"
	ompDocRoles          = "_omp_model_roles"
	ompDocPresets        = "_omp_model_presets"
	ompDocConfigErr      = "_omp_config_err"
	ompDocRefs           = "_omp_references"
)

// OMPPreset 是 config.yml 里 modelPresets.<name> 的只读快照。
type OMPPreset struct {
	ModelRoles           map[string]string
	DefaultThinkingLevel string
}

// OMPModelsRead 是 ompStore 依赖的读取能力，*ompprovider.ModelsFile 隐式满足。
//
// 这里用接口而不是具体类型，是为了让 core 的读侧（List/Get）在
// internal/ompprovider 的写方法就绪之前也能编译并通过单元测试；
// 写侧再用 OMPModelsWrite 做运行时断言，等 A 补齐 AST 方法后自动生效。
type OMPModelsRead interface {
	Providers() (map[string]map[string]interface{}, error)
	ProviderIDs() []string
	HasProvider(id string) bool
	Bytes() ([]byte, error)
}

// OMPModelsWrite 是 ompStore 依赖的 AST 变更能力。
// *ompprovider.ModelsFile 补齐 SetProvider / DeleteProvider / DeleteModel / Validate
// 后自动满足本接口，core 侧无需改动。
type OMPModelsWrite interface {
	SetProvider(id string, desired map[string]interface{}, mergeModels bool) error
	DeleteProvider(id string) bool
	DeleteModel(providerID, modelID string) (bool, error)
	Validate() error
}

// ompLoadModelsFile 由 backend_omp.go 的 init 注入真实解析器；
// 默认实现把“解析器未注册”变成明确的中文错误，而不是 nil panic。
var ompLoadModelsFile = func([]byte) (OMPModelsRead, error) {
	return nil, fmt.Errorf("OMP 模型文件解析器尚未注册，请升级 AgentProviderManager")
}

// ompWriteNotReadyError 在 internal/ompprovider 尚未提供写方法时给出明确提示。
func ompWriteNotReadyError() error {
	return fmt.Errorf("OMP 模型文件写入能力尚未就绪，请升级 AgentProviderManager")
}

func ompDocModels(doc Doc) (OMPModelsRead, error) {
	if doc == nil {
		return nil, fmt.Errorf("OMP 文档为空")
	}
	file, ok := doc[ompDocFile].(OMPModelsRead)
	if !ok || file == nil {
		return nil, fmt.Errorf("OMP 文档缺少模型文件句柄")
	}
	return file, nil
}

func ompDocStringValue(doc Doc, key string) string {
	s, _ := doc[key].(string)
	return s
}

func ompDocBoolValue(doc Doc, key string) bool {
	b, _ := doc[key].(bool)
	return b
}

func ompDocAliasReasons(doc Doc) map[string]string {
	reasons, _ := doc[ompDocAlias].(map[string]string)
	return reasons
}

func ompDocReferences(doc Doc) OMPReferences {
	refs, _ := doc[ompDocRefs].(OMPReferences)
	return refs
}

// ompStore 是 omp 的 ProviderStore 实现。它是无状态的：所有数据都在 Doc 里，
// 因此读侧与写侧都不会跨文件残留状态。
type ompStore struct{}

// List 把 models.yml 的 providers 映射成列表项。
// omp 没有 provider 显示名的概念，Name 直接回填 ID（前端名称输入框只读）。
func (ompStore) List(doc Doc) []ProviderSummary {
	file, err := ompDocModels(doc)
	if err != nil {
		return []ProviderSummary{}
	}
	providers, err := file.Providers()
	if err != nil {
		return []ProviderSummary{}
	}
	alias := ompDocAliasReasons(doc)
	// 只有 models.json 时整份文件处于待迁移只读态（G1）：配置位置摘要已经用
	// migrationOnly 标只读，管理页列表必须与之一致，否则用户要等到保存才被拒绝。
	migrationOnly := ompDocBoolValue(doc, ompDocNeedsMigration)
	ids := make([]string, 0, len(providers))
	for id := range providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]ProviderSummary, 0, len(ids))
	for _, id := range ids {
		view := ompReadProvider(id, providers[id], alias[id])
		out = append(out, ProviderSummary{
			ID:         id,
			Name:       id,
			Kind:       view.Kind,
			BaseURL:    view.BaseURL,
			HasAPIKey:  strings.TrimSpace(view.APIKeyRaw) != "",
			ModelCount: len(view.ModelIDs),
			ReadOnly:   view.ReadOnly || migrationOnly,
		})
	}
	return out
}

// Get 返回单个 provider 的编辑视图。
//
// 只读 provider 也照常返回 BaseURL / APIKey / APIKeyKind / RawAPI（测试连接与
// 刷新模型对只读 provider 依然可用），只额外带上 ReadOnly 与中文 ReadOnlyReason；
// 写操作由 Upsert / DeleteModel 兜底拒绝。
func (ompStore) Get(doc Doc, providerID string) (*ProviderEdit, error) {
	file, err := ompDocModels(doc)
	if err != nil {
		return nil, err
	}
	providers, err := file.Providers()
	if err != nil {
		return nil, err
	}
	raw := providers[providerID]
	if raw == nil {
		return nil, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	view := ompReadProvider(providerID, raw, ompDocAliasReasons(doc)[providerID])
	edit := &ProviderEdit{
		ID:             providerID,
		Name:           providerID,
		Kind:           view.Kind,
		BaseURL:        view.BaseURL,
		APIKey:         view.APIKeyRaw,
		APIKeyRequired: view.APIKeyRequired,
		ReadOnly:       view.ReadOnly,
		ReadOnlyReason: view.ReadOnlyReason,
		APIKeyKind:     view.APIKeyKind,
		OptionsJSON:    ompOptionsJSON(raw),
		RawProvider:    deepCopyMap(raw),
		Cards:          []ModelCard{},
	}
	if rawAPI := strings.TrimSpace(stringOr(raw["api"], "")); rawAPI != "" {
		edit.RawAPI = rawAPI
	} else if view.WireAPI != "" && view.WireAPI != "mixed" {
		edit.RawAPI = view.WireAPI
	}
	compat := ompMap(raw["compat"])
	cards := make([]ModelCard, 0, len(view.ModelIDs))
	for _, model := range ompModelList(raw) {
		if strings.TrimSpace(stringOr(model["id"], "")) == "" {
			continue
		}
		cards = append(cards, ompBuildCard(model, providerID, view.BaseURL, view.WireAPI, compat))
	}
	sort.SliceStable(cards, func(i, j int) bool {
		return strings.ToLower(cards[i].ModelID) < strings.ToLower(cards[j].ModelID)
	})
	edit.Cards = cards
	return edit, nil
}

// Upsert 把编辑器提交的 payload 写回 models.yml 的 AST。
//
// 语义要点（计划 §4.5）：
//   - provider 级只管理 baseUrl / api / apiKey / auth / headers / authHeader / models；
//     文件里其它未知键原样保留；
//   - apiKey 的空值语义由 payload 内部标记 _api_key_touched 决定：触碰且为空=删除，
//     未触碰且为空=保留目标已有 apiKey（绝不因为 UI 没回填就清掉引用）；
//   - options_json 只接受 headers 与 authHeader；空串=不修改，显式 {"headers":{}}=删全部；
//   - 重命名（payload id 与旧 ID 不同）在引用保护通过后先做节点级改名（保住
//     provider 级未知键与注释），再写入 APM 管理的键；目标 ID 已存在时拒绝。
func (ompStore) Upsert(doc Doc, providerID string, p ProviderPayload, opt SaveOptions) (Doc, error) {
	if !ompDocBoolValue(doc, ompDocWritable) {
		return nil, fmt.Errorf("当前 omp 模型文件只能只读解析（只有 models.json），请先运行一次 omp 完成 models.json 迁移")
	}
	file, err := ompDocModels(doc)
	if err != nil {
		return nil, err
	}
	writer, ok := file.(OMPModelsWrite)
	if !ok {
		return nil, ompWriteNotReadyError()
	}
	newID, err := ompPayloadProviderID(p)
	if err != nil {
		return nil, err
	}
	targetID := providerID
	if newID != "" {
		targetID = newID
	}
	if targetID == "" {
		return nil, fmt.Errorf("Provider ID 不能为空")
	}
	if !ompProviderIDValid(targetID) {
		return nil, fmt.Errorf("Provider ID「%s」非法：只能包含字母、数字、点、下划线或短横线（不能包含「/」与空白）", targetID)
	}
	providers, err := file.Providers()
	if err != nil {
		return nil, err
	}
	var oldRaw map[string]interface{}
	if providerID != "" && file.HasProvider(providerID) {
		oldRaw = providers[providerID]
		if reason := ompDocAliasReasons(doc)[providerID]; reason != "" {
			return nil, fmt.Errorf("提供商「%s」%s", providerID, reason)
		}
		view := ompReadProvider(providerID, oldRaw, "")
		if view.ReadOnly {
			return nil, fmt.Errorf("提供商「%s」为只读：%s", providerID, view.ReadOnlyReason)
		}
	}
	renaming := providerID != "" && targetID != providerID
	if renaming {
		if reason := OMPReferenceGuard(ompDocReferences(doc), providerID, ""); reason != "" {
			return nil, fmt.Errorf("无法重命名提供商「%s」：%s", providerID, reason)
		}
		if file.HasProvider(targetID) {
			return nil, fmt.Errorf("提供商「%s」已存在，不能重命名为此 ID", targetID)
		}
	}
	desired, err := ompDesiredProvider(p, oldRaw)
	if err != nil {
		return nil, err
	}
	if renaming {
		// 先在节点层改名，保住 provider 级未知键与注释，再写入管理键。
		renamed, err := ompRenameProviderFile(file, providerID, targetID)
		if err != nil {
			return nil, err
		}
		rewritten, ok := renamed.(OMPModelsWrite)
		if !ok {
			return nil, ompWriteNotReadyError()
		}
		doc[ompDocFile] = renamed
		file = renamed
		writer = rewritten
	}
	if err := writer.SetProvider(targetID, desired, opt.MergeModels); err != nil {
		return nil, err
	}
	if renaming && file.HasProvider(providerID) {
		writer.DeleteProvider(providerID)
	}
	if err := writer.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

// Delete 删除整个 provider。只读 provider 也允许整体删除（计划 §4.7），
// 但被 modelRoles / modelPresets / 文件内引用指向时一律拒绝。
func (ompStore) Delete(doc Doc, providerID string) (Doc, bool, error) {
	if !ompDocBoolValue(doc, ompDocWritable) {
		return nil, false, fmt.Errorf("当前 omp 模型文件只能只读解析（只有 models.json），请先运行一次 omp 完成 models.json 迁移")
	}
	file, err := ompDocModels(doc)
	if err != nil {
		return nil, false, err
	}
	providers, err := file.Providers()
	if err != nil {
		return nil, false, err
	}
	if providers[providerID] == nil {
		return nil, false, nil
	}
	writer, ok := file.(OMPModelsWrite)
	if !ok {
		return nil, false, ompWriteNotReadyError()
	}
	if reason := OMPReferenceGuard(ompDocReferences(doc), providerID, ""); reason != "" {
		return nil, false, fmt.Errorf("无法删除提供商「%s」：%s", providerID, reason)
	}
	if !writer.DeleteProvider(providerID) {
		return nil, false, nil
	}
	if err := writer.Validate(); err != nil {
		return nil, false, err
	}
	return doc, true, nil
}

// DeleteModel 删除单个模型。只读 provider 不允许部分写（计划 §4.7），
// 模型被 modelRoles / modelPresets / 同文件内引用指向时拒绝。
func (ompStore) DeleteModel(doc Doc, providerID, modelID string) (Doc, bool, error) {
	if !ompDocBoolValue(doc, ompDocWritable) {
		return nil, false, fmt.Errorf("当前 omp 模型文件只能只读解析（只有 models.json），请先运行一次 omp 完成 models.json 迁移")
	}
	file, err := ompDocModels(doc)
	if err != nil {
		return nil, false, err
	}
	providers, err := file.Providers()
	if err != nil {
		return nil, false, err
	}
	raw := providers[providerID]
	if raw == nil {
		return nil, false, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	view := ompReadProvider(providerID, raw, ompDocAliasReasons(doc)[providerID])
	if view.ReadOnly {
		return nil, false, fmt.Errorf("提供商「%s」为只读，不能删除其中的模型：%s", providerID, view.ReadOnlyReason)
	}
	writer, ok := file.(OMPModelsWrite)
	if !ok {
		return nil, false, ompWriteNotReadyError()
	}
	if reason := OMPReferenceGuard(ompDocReferences(doc), providerID, modelID); reason != "" {
		return nil, false, fmt.Errorf("无法删除模型「%s/%s」：%s", providerID, modelID, reason)
	}
	removed, err := writer.DeleteModel(providerID, modelID)
	if err != nil {
		return nil, false, err
	}
	if !removed {
		return nil, false, nil
	}
	if err := writer.Validate(); err != nil {
		return nil, false, err
	}
	return doc, true, nil
}

// ompOptionsJSON 只回显文件里显式声明的东西：
//   - 有 headers（哪怕空 map）→ {"headers":{...}}，只收字符串值；
//   - 显式写了 authHeader: true → {"authHeader":true}（false/缺省不出现，omp 默认就是未设置）；
//   - 两者都没有 → 空串（表示「文件没声明」，不是「清空」）。
func ompOptionsJSON(raw map[string]interface{}) string {
	out := map[string]interface{}{}
	if _, ok := raw["headers"]; ok {
		headers := map[string]interface{}{}
		for k, v := range ompHeaderMap(raw["headers"]) {
			headers[k] = v
		}
		out["headers"] = headers
	}
	if enabled, ok := ompBoolValue(raw["authHeader"]); ok && enabled {
		out["authHeader"] = true
	}
	if len(out) == 0 {
		return ""
	}
	data, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(data)
}

// ompPayloadProviderID 读取并校验 payload 里的 Provider ID；空串表示没改名。
func ompPayloadProviderID(p ProviderPayload) (string, error) {
	raw, _ := p["id"].(string)
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", nil
	}
	if !ompProviderIDValid(id) {
		return "", fmt.Errorf("Provider ID「%s」非法：只能包含字母、数字、点、下划线或短横线（不能包含「/」与空白）", id)
	}
	return id, nil
}

// ompProviderIDValid 实现 G14：新建/改名 ID 限 [a-zA-Z0-9._-]，禁止 "/" 与空白。
func ompProviderIDValid(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// ompDesiredProvider 从 payload 构造 SetProvider 的 desired 映射。
// desired 只包含 APM 管理的键；old 里其余的未知键由 SetProvider 自行保留，
// 但重命名场景下 old 的未知键必须显式并进来（目标节点不存在时 SetProvider 无从保留）。
func ompDesiredProvider(p ProviderPayload, old map[string]interface{}) (map[string]interface{}, error) {
	desired := deepCopyMap(old)
	if desired == nil {
		desired = map[string]interface{}{}
	}
	for k := range desired {
		if strings.HasPrefix(k, "_") {
			delete(desired, k)
		}
	}
	baseURL, _ := p["base_url"].(string)
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("Base URL 不能为空")
	}
	if len(baseURL) > 2048 {
		return nil, fmt.Errorf("Base URL 不能超过 2048 个字符")
	}
	kindRaw, _ := p["kind"].(string)
	kindNorm, err := ValidateKind(kindRaw)
	if err != nil {
		return nil, err
	}
	if kindNorm == "" {
		return nil, fmt.Errorf("请选择提供商协议")
	}
	wire := OMPWireAPIForKind(kindNorm)
	if wire == "" {
		return nil, fmt.Errorf("P1 只支持 openai-compatible / anthropic / responses 三种协议")
	}
	desired["api"] = wire
	desired["baseUrl"] = baseURL

	apiKey, _ := p["api_key"].(string)
	apiKey = strings.TrimSpace(apiKey)
	if len(apiKey) > MaxAPIKeyLength {
		return nil, fmt.Errorf("API Key 不能超过 %d 个字符", MaxAPIKeyLength)
	}
	touched, _ := ompBoolValue(p["_api_key_touched"])
	required, _ := p["api_key_required"].(bool)
	oldAuth := strings.TrimSpace(stringOr(old["auth"], ""))
	oldKey := strings.TrimSpace(stringOr(old["apiKey"], ""))

	switch {
	case required:
		// 「需要 Key」：写 apiKey，不写 auth。
		desired["auth"] = nil
		if apiKey != "" {
			desired["apiKey"] = apiKey
		} else if touched || oldKey == "" {
			desired["apiKey"] = nil
		}
	case oldAuth == "oauth":
		// OAuth 由 omp 管理；G20：用户显式填了 key 仍写入，并保留 auth: oauth。
		if apiKey != "" {
			desired["apiKey"] = apiKey
		} else if touched {
			desired["apiKey"] = nil
		}
	default:
		// 「不需要 Key」：写 auth: none，删除 apiKey。
		if apiKey != "" {
			return nil, fmt.Errorf("不需要 Key 的 provider 不应填写 Key")
		}
		desired["auth"] = "none"
		desired["apiKey"] = nil
	}

	if err := ompApplyOptionsJSON(desired, p["options_json"]); err != nil {
		return nil, err
	}

	oldModels := map[string]map[string]interface{}{}
	for _, model := range ompModelList(old) {
		mid := strings.TrimSpace(stringOr(model["id"], ""))
		if mid != "" {
			oldModels[mid] = model
		}
	}
	if cardsRaw, ok := p["cards"]; ok {
		cards, err := ompDecodeCards(cardsRaw)
		if err != nil {
			return nil, err
		}
		models := make([]interface{}, 0, len(cards))
		seen := map[string]bool{}
		for _, card := range cards {
			mid := strings.TrimSpace(card.ModelID)
			if mid == "" {
				return nil, fmt.Errorf("模型 ID 不能为空")
			}
			if seen[mid] {
				return nil, fmt.Errorf("模型 ID「%s」重复", mid)
			}
			seen[mid] = true
			node, err := ompDesiredModel(card, oldModels[mid], wire)
			if err != nil {
				return nil, err
			}
			models = append(models, node)
		}
		desired["models"] = models
	}
	return desired, nil
}

// ompApplyOptionsJSON 解析 payload 的 options_json 并写回 desired 的 headers / authHeader。
// 只接受这两个键；其它键一律中文拒绝（G9）。
func ompApplyOptionsJSON(desired map[string]interface{}, raw interface{}) error {
	text, _ := raw.(string)
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var opts map[string]interface{}
	if err := json.Unmarshal([]byte(text), &opts); err != nil {
		return fmt.Errorf("options_json 不是合法 JSON：%v", err)
	}
	for key := range opts {
		if key != "headers" && key != "authHeader" {
			return fmt.Errorf("options_json 只支持 headers 与 authHeader 两个键，不支持「%s」", key)
		}
	}
	if hv, ok := opts["headers"]; ok {
		if hv == nil {
			desired["headers"] = nil
		} else {
			rawHeaders := ompMap(hv)
			if rawHeaders == nil {
				return fmt.Errorf("options_json.headers 必须是对象")
			}
			headers := make(map[string]interface{}, len(rawHeaders))
			for k, v := range rawHeaders {
				s, ok := v.(string)
				if !ok {
					return fmt.Errorf("options_json.headers「%s」的值必须是字符串", k)
				}
				headers[k] = s
			}
			if len(headers) == 0 {
				desired["headers"] = nil
			} else {
				desired["headers"] = headers
			}
		}
	}
	if av, ok := opts["authHeader"]; ok {
		enabled, ok := av.(bool)
		if !ok {
			return fmt.Errorf("options_json.authHeader 必须是布尔值")
		}
		if enabled {
			desired["authHeader"] = true
		} else {
			desired["authHeader"] = nil
		}
	}
	return nil
}

// ompDecodeCards 兼容 []ModelCard 与 JSON 解码出来的 []interface{}。
func ompDecodeCards(raw interface{}) ([]ModelCard, error) {
	switch typed := raw.(type) {
	case []ModelCard:
		return typed, nil
	case nil:
		return []ModelCard{}, nil
	case []interface{}:
		cards := make([]ModelCard, 0, len(typed))
		for _, item := range typed {
			m, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("cards 里有无法识别的元素")
			}
			data, err := json.Marshal(m)
			if err != nil {
				return nil, fmt.Errorf("解析模型卡片失败：%v", err)
			}
			var card ModelCard
			if err := json.Unmarshal(data, &card); err != nil {
				return nil, fmt.Errorf("解析模型卡片失败：%v", err)
			}
			cards = append(cards, card)
		}
		return cards, nil
	default:
		return nil, fmt.Errorf("cards 字段类型无法识别")
	}
}

// ompDesiredModel 把一张模型卡片映射回 omp 的 models[] 节点。
// 文件里同名模型节点的未知键（headers / compat / tokenizer / cost 等）原样保留。
func ompDesiredModel(card ModelCard, old map[string]interface{}, providerWire string) (map[string]interface{}, error) {
	// isNew 必须在深拷贝之前判定：deepCopyMap(nil) 返回空 map 而非 nil。
	isNew := old == nil
	node := deepCopyMap(old)
	for k := range node {
		if strings.HasPrefix(k, "_") {
			delete(node, k)
		}
	}
	node["id"] = card.ModelID
	if name := strings.TrimSpace(card.Name); name != "" && name != card.ModelID {
		node["name"] = name
	} else {
		node["name"] = nil
	}
	oldMaxContext, _ := ompPositiveInt(node["maxContextWindow"])
	if n, ok := ompPositiveInt(card.Context); ok {
		node["contextWindow"] = n
		if n > oldMaxContext {
			node["maxContextWindow"] = n
		}
	} else {
		node["contextWindow"] = nil
	}
	if n, ok := ompPositiveInt(card.Output); ok {
		node["maxTokens"] = n
	} else {
		node["maxTokens"] = nil
	}
	if err := ompApplyCardReasoning(node, card, providerWire); err != nil {
		return nil, err
	}
	// 能力位：只写文件里明确已知为真的；未知（nil）不碰旧值，更不写 false。
	// omp 的 model.input 只认 text / image（A 的 Validate 会拒绝 audio 等），
	// 卡片能力可能来自能力表推断并含 audio/pdf，写盘前必须裁剪（G22）。
	if card.Capabilities != nil {
		caps := ompExpressibleFeatures(*card.Capabilities)
		if mods := NormalizeInputModalities(caps.InputModalities); len(mods) > 0 {
			node["input"] = mods
		} else if _, had := node["input"]; had || isNew {
			if isNew {
				node["input"] = []string{"text"}
			} else {
				node["input"] = nil
			}
		}
		if toolCall := caps.ToolCall; toolCall != nil && *toolCall {
			node["supportsTools"] = true
		} else if toolCall != nil {
			node["supportsTools"] = nil
		}
	} else if isNew {
		node["input"] = []string{"text"}
	}
	return node, nil
}

// ompNormalizeThinking 归一化节点里已有的 thinking：合法的映射深拷贝返回，
// 其余（标量、空 map、缺 mode）一律返回 nil 表示应当删键。
// 这些坏数据来自手工编辑或旧版本写出，omp 的校验必然拒收；不在这里清掉，
// 整次保存会失败，连「删除 provider / 删除模型」这种自救操作也会被一起拒掉。
func ompNormalizeThinking(value interface{}) map[string]interface{} {
	raw := ompMap(value)
	if raw == nil {
		return nil
	}
	if strings.TrimSpace(stringOr(raw["mode"], "")) == "" {
		return nil
	}
	return deepCopyMap(raw)
}

// ompApplyCardReasoning 实现计划 §4.6 的写侧档位口径。
func ompApplyCardReasoning(node map[string]interface{}, card ModelCard, providerWire string) error {
	levels := ompFilterEfforts(card.Variants)
	thinking := ompNormalizeThinking(node["thinking"])
	if thinking == nil {
		node["thinking"] = nil
	}
	if !card.Reasoning || len(levels) == 0 {
		if card.Reasoning {
			// 打开了推理但没给出具体档位：只写 reasoning，由 omp 自己推导。
			node["reasoning"] = true
			return nil
		}
		node["reasoning"] = nil
		node["thinking"] = nil
		return nil
	}
	node["reasoning"] = true
	if providerWire == OMPWireAnthropicMessages {
		// anthropic 的档位由 omp 推导：P1 只写 reasoning，不写 thinking（计划 §4.6）。
		// 上面的坏 thinking 清理仍然生效，其余既有内容原样保留。
		return nil
	}
	if thinking == nil {
		thinking = map[string]interface{}{"mode": "effort"}
	}
	// 计划 §4.6：已有 mode 原样保留。只有 effort 语义才写 efforts/defaultLevel，
	// 否则会把 levels/minLevel+maxLevel 与 efforts 混成一份自相矛盾的配置。
	if mode := strings.TrimSpace(stringOr(thinking["mode"], "")); mode != "effort" {
		node["thinking"] = thinking
		return nil
	}
	// effort 语义只看 efforts/defaultLevel，清掉可能残留在同一节点里的另一套键。
	delete(thinking, "levels")
	delete(thinking, "minLevel")
	delete(thinking, "maxLevel")
	thinking["efforts"] = levels
	if def := ompDefaultVariant(levels, strings.TrimSpace(card.DefaultVariant)); def != "" && def != ompOffVariant {
		thinking["defaultLevel"] = def
	}
	node["thinking"] = thinking
	return nil
}
