package core

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"agentprovidermanager/internal/dshprovider"
)

// DeepSeek Harness 文档在不同格式下共用同一套 provider 视图与条目编码，
// 这里集中放文档变换（不做备份、不做指纹校验，写盘由 backend 负责）。

const (
	// docDshPatchKey 承载已解析的 profile patch AST（值类型 *dshprovider.Patch）。
	docDshPatchKey = "_dsh_patch"
	// docDshPathKey 记录文档来源路径。
	docDshPathKey = "_dsh_path"
	// docDshCredentialsKey 承载本次保存需要写入凭据文件的键值对。
	docDshCredentialsKey = "_dsh_credentials"
	// docDshHomeOverrideKey 表示 home 级 patch 已覆盖 llm-pi-ai 配置（值类型 bool）。
	docDshHomeOverrideKey = "_dsh_home_override"
	// docDshHomeProvidersKey 承载 home 级 patch 的 provider 视图（被覆盖时生效）。
	docDshHomeProvidersKey = "_dsh_home_providers"
)

// apiForDeepSeekKind 把 UI 的类型映射成 pi-ai 的协议名（0.2 起 api 是必填的路由协议）。
func apiForDeepSeekKind(uiKind string) string {
	switch uiKind {
	case "anthropic":
		return "anthropic-messages"
	case "responses":
		return "openai-responses"
	default:
		return "openai-completions"
	}
}

// kindFromDeepSeekAPI 把 pi-ai 的 api 取值映射回 UI 类型；无法识别时回退到 baseURL 推断。
func kindFromDeepSeekAPI(api, baseURL string) string {
	low := strings.ToLower(strings.TrimSpace(api))
	switch {
	case low == "":
		return InferKind(baseURL)
	case strings.Contains(low, "anthropic"):
		return "anthropic"
	case strings.Contains(low, "responses"):
		return "responses"
	default:
		return "openai-compatible"
	}
}

// deepSeekProviderKind 判定一条路由的 UI 类型：显式 kind 优先，其次 api，最后按 baseURL 推断。
func deepSeekProviderKind(raw map[string]interface{}) string {
	baseURL, _ := raw["baseURL"].(string)
	if baseURL == "" {
		if opts, ok := raw["options"].(map[string]interface{}); ok {
			baseURL, _ = opts["baseURL"].(string)
		}
	}
	if api, ok := raw["api"].(string); ok && strings.TrimSpace(api) != "" {
		return kindFromDeepSeekAPI(api, baseURL)
	}
	if kind, ok := raw["kind"].(string); ok && strings.TrimSpace(kind) != "" {
		return ConfigKindToUIKind(strings.TrimSpace(kind), baseURL)
	}
	return InferKind(baseURL)
}

// deepSeekModelCount 统计一条路由声明的模型数量（列表与映射两种写法都算）。
func deepSeekModelCount(raw map[string]interface{}) int {
	if arr, ok := raw["models"].([]interface{}); ok {
		return len(arr)
	}
	if mp, ok := raw["models"].(map[string]interface{}); ok {
		return len(mp)
	}
	return 0
}

// deepSeekSummariesFromProviders 由 provider 映射生成 UI 摘要，列表顺序按 ID 排序。
func deepSeekSummariesFromProviders(providers map[string]interface{}) []ProviderSummary {
	out := make([]ProviderSummary, 0, len(providers))
	creds := LoadCredentialsForPreview()
	for pid, value := range providers {
		m, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		baseURL, _ := m["baseURL"].(string)
		apiKey, _ := m["apiKey"].(string)
		apiKeyEnv, _ := m["apiKeyEnv"].(string)
		if opts, ok := m["options"].(map[string]interface{}); ok {
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
		hasKey := strings.TrimSpace(apiKey) != ""
		if !hasKey && strings.TrimSpace(apiKeyEnv) != "" {
			if v, ok := creds[apiKeyEnv]; ok && strings.TrimSpace(v) != "" {
				hasKey = true
			} else if v := os.Getenv(apiKeyEnv); strings.TrimSpace(v) != "" {
				hasKey = true
			}
		}
		name := stringOr(m["displayName"], stringOr(m["name"], pid))
		out = append(out, ProviderSummary{
			ID: pid, Name: name, Kind: deepSeekProviderKind(m),
			BaseURL: baseURL, HasAPIKey: hasKey, ModelCount: deepSeekModelCount(m),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].ID) < strings.ToLower(out[j].ID)
	})
	return out
}

// stageDeepSeekCredential 暂存一次保存要写入凭据文件的键值对。
// 真正的写盘发生在 backend 的 Write 里，因此 ProviderStore 保持“只做文档变换”。
func stageDeepSeekCredential(doc map[string]interface{}, envName, apiKey string) {
	if strings.TrimSpace(envName) == "" || strings.TrimSpace(apiKey) == "" {
		return
	}
	staged, _ := doc[docDshCredentialsKey].(map[string]string)
	if staged == nil {
		staged = map[string]string{}
	}
	staged[strings.TrimSpace(envName)] = strings.TrimSpace(apiKey)
	doc[docDshCredentialsKey] = staged
}

// flushDeepSeekCredentials 把暂存的凭据写进 $DSH_HOME/.credentials.yaml（version 1 布局）。
// 写盘顺序为“先凭据后配置”：配置写失败最多留下一个未使用的 ref，不会出现“配置引用了不存在的密钥”。
func flushDeepSeekCredentials(doc map[string]interface{}) error {
	staged, _ := doc[docDshCredentialsKey].(map[string]string)
	delete(doc, docDshCredentialsKey)
	if len(staged) == 0 {
		return nil
	}
	names := make([]string, 0, len(staged))
	for name := range staged {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := dshprovider.SetCredentialRef(DeepSeekCredentialsPath(), name, staged[name]); err != nil {
			return err
		}
	}
	return nil
}

// deepSeekRouteExtraKeys 是 pi-ai 路由条目上“本包不单独建模、可原样透传”的字段。
// 取自 @deepseek-ai/dsh-llm-pi-ai 的 profile schema；未列出的键不会写入，
// 避免把其它 Agent 的私有 options（例如 endpoints、zcode 段）塞进 DSH 的 patch。
var deepSeekRouteExtraKeys = []string{
	"headers",
	"compat",
	"modelOverrides",
	"defaultContextWindow",
	"defaultMaxTokens",
	"defaultInput",
	"reasoning",
	"thinkingBudgets",
	"cacheRetention",
	"transport",
	"timeoutMs",
	"websocketConnectTimeoutMs",
	"streamIdleTimeoutMs",
	"maxRequestImageBytes",
	"requestImagePixelBudget",
	"requestImageMaxBytes",
	"retryPolicy",
}

// deepSeekRouteExtras 从内部 provider options 里挑出可透传到 pi-ai 路由的字段。
func deepSeekRouteExtras(opts map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range deepSeekRouteExtraKeys {
		if v, ok := opts[key]; ok {
			out[key] = v
		}
	}
	return out
}

// deepSeekBuildEntry 生成 pi-ai 路由条目（0.1 的 llm-pi-ai 段与 0.2 的 profile patch 共用）。
// 返回的 envName/apiKey 由调用方交给 stageDeepSeekCredential。
func deepSeekBuildEntry(providerID string, providerCfg map[string]interface{}, existing map[string]interface{}, mergeModels bool) (entry map[string]interface{}, envName, apiKey string) {
	entry = map[string]interface{}{}
	if existing != nil {
		entry = deepCopyMap(existing)
		if entry == nil {
			entry = map[string]interface{}{}
		}
	}
	opts, _ := providerCfg["options"].(map[string]interface{})
	baseURL, _ := opts["baseURL"].(string)
	apiKey, _ = opts["apiKey"].(string)
	modelsMap, _ := providerCfg["models"].(map[string]interface{})
	kind, _ := providerCfg["kind"].(string)
	uiKind := ConfigKindToUIKind(kind, baseURL)

	// 保留原有的模型顺序；新卡片按 ID 排序后追加，保证结果确定。
	ordered := make([]interface{}, 0, len(modelsMap))
	seen := map[string]bool{}
	if old, ok := entry["models"].([]interface{}); ok {
		for _, value := range old {
			mm, _ := value.(map[string]interface{})
			mid := strings.TrimSpace(stringOr(mm["id"], ""))
			if mid == "" || seen[mid] {
				continue
			}
			next, replaced := modelsMap[mid].(map[string]interface{})
			if !replaced {
				seen[mid] = true
				if mergeModels {
					// 未在本次提交里出现且处于合并语义：保留原有节点。
					ordered = append(ordered, value)
				}
				continue
			}
			encoded := dshModelToRaw(CfgToCard(mid, next), uiKind)
			if mergeModels {
				// 合并语义以旧节点为基座叠加本次提交的字段：目标里未被本次提交
				// 涉及的键（目标独有字段）得以保留，与 legacy 的
				// MergeProviderIntoConfig(mergeModels=true) 一致。直接整节点替换会
				// 丢掉这些字段，也会让没有推理信息的模型被写死“关闭推理”。
				ordered = append(ordered, overlayModelNode(mm, encoded))
			} else {
				// 覆盖语义：整节点替换，确保编辑（改名、上下文长度、档位…）落地。
				ordered = append(ordered, encoded)
			}
			seen[mid] = true
		}
	}
	ids := make([]string, 0, len(modelsMap))
	for mid := range modelsMap {
		if !seen[mid] {
			ids = append(ids, mid)
		}
	}
	sort.Strings(ids)
	for _, mid := range ids {
		mm, _ := modelsMap[mid].(map[string]interface{})
		ordered = append(ordered, dshModelToRaw(CfgToCard(mid, mm), uiKind))
	}

	entry["displayName"] = providerCfg["name"]
	entry["baseURL"] = baseURL
	envName, _ = entry["apiKeyEnv"].(string)
	envName = strings.TrimSpace(envName)
	if strings.TrimSpace(apiKey) != "" {
		if envName == "" {
			envName = deepSeekEnvNameFor(providerID)
		}
		entry["apiKeyEnv"] = envName
		delete(entry, "apiKey")
	} else if envName == "" {
		delete(entry, "apiKey")
	}
	entry["api"] = apiForDeepSeekKind(uiKind)
	// 透传 headers 等 pi-ai 路由支持的字段（来自 ZCode 侧 options 或 DSH 原条目）。
	for k, v := range deepSeekRouteExtras(opts) {
		entry[k] = v
	}
	entry["models"] = ordered
	return entry, envName, apiKey
}

// overlayModelNode 把重建的模型节点叠加到旧节点上（合并语义）：
// 以旧节点为基座，用 rebuilt 覆盖其中的键。这样目标里未被本次提交涉及的
// 键（目标独有字段）得以保留，与 legacy 的模型级合并语义一致。
// 仅当旧节点是映射时叠加；否则直接用 rebuilt。
func overlayModelNode(old, rebuilt map[string]interface{}) map[string]interface{} {
	if old == nil {
		return rebuilt
	}
	merged := deepCopyMap(old)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	for k, v := range rebuilt {
		merged[k] = deepCopyValue(v)
	}
	// 模型 ID 必须以本次重建的结果为准。
	if id, ok := rebuilt["id"]; ok {
		merged["id"] = id
	}
	// 重建结果显式关闭推理时，旧节点里可能残留的单数键 reasoningEffort 必须一并清掉，
	// 否则会出现 reasoningEfforts: false 与 reasoningEffort: "high" 并存的歧义状态。
	if re, ok := merged["reasoningEfforts"]; ok && re == false {
		delete(merged, "reasoningEffort")
	}
	return merged
}

// dshApplyExplicitReasoningOff 把“用户显式关闭了推理”的模型标记成显式关闭。
//
// CardToCfg/CfgToCard 的往返只保留 reasoningEfforts 映射，无法表达“显式关闭”：卡片关闭
// 后 RawCfg 里陈旧的 reasoningEfforts 会被 CfgToCard 重新推断成“开启”，于是关闭操作
// 无法落盘。这里对 providerCfg 里的模型写入 legacy 形状的 reasoning.enabled=false 并清掉
// 陈旧的推理键，使 CfgToCard 一致地读出“显式关闭”，dshModelToRaw 便会写出
// reasoningEfforts: false。
func dshApplyExplicitReasoningOff(providerCfg map[string]interface{}, payload ProviderPayload) {
	models, _ := providerCfg["models"].(map[string]interface{})
	if len(models) == 0 {
		return
	}
	for _, card := range dshPayloadCards(payload) {
		if card.RawReasoningEnabled == nil || *card.RawReasoningEnabled {
			continue
		}
		mid := strings.TrimSpace(card.ModelID)
		entry, ok := models[mid].(map[string]interface{})
		if !ok {
			continue
		}
		entry["reasoning"] = map[string]interface{}{"enabled": false}
		delete(entry, "reasoningEfforts")
		delete(entry, "reasoningEffort")
	}
}

// dshPayloadCards 取出写入载荷里的原始卡片（[]ModelCard 或前端传来的 []interface{}）。
func dshPayloadCards(payload ProviderPayload) []ModelCard {
	switch v := payload["cards"].(type) {
	case []ModelCard:
		return v
	case []interface{}:
		out := make([]ModelCard, 0, len(v))
		for _, e := range v {
			m, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			b, err := json.Marshal(m)
			if err != nil {
				continue
			}
			var c ModelCard
			if json.Unmarshal(b, &c) == nil {
				out = append(out, c)
			}
		}
		return out
	}
	return nil
}

// deepSeekEnvNameFor 由 provider ID 推导默认的凭据引用名。
func deepSeekEnvNameFor(providerID string) string {
	name := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(providerID), "-", "_"), ":", "_"))
	if name == "" {
		return "DSH_PROVIDER_API_KEY"
	}
	return name + "_API_KEY"
}

// deepSeekEnsureDefaultInProviders 在默认模型条目的 provider/model 失效时，
// 改指第一条可用的模型；没有可用模型时删除该条目（与 0.1 的语义一致）。
func deepSeekEnsureDefaultInProviders(def map[string]interface{}, providers map[string]interface{}) {
	if def == nil {
		return
	}
	provider := strings.TrimSpace(stringOr(def["provider"], ""))
	model := strings.TrimSpace(stringOr(def["model"], ""))
	valid := func(raw interface{}, wanted string) bool {
		p, ok := raw.(map[string]interface{})
		if !ok {
			return false
		}
		if arr, ok := p["models"].([]interface{}); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]interface{}); ok && strings.TrimSpace(stringOr(m["id"], "")) == wanted {
					return true
				}
			}
			return false
		}
		if mp, ok := p["models"].(map[string]interface{}); ok {
			_, exists := mp[wanted]
			return exists
		}
		return false
	}
	if wanted := providers[provider]; wanted != nil && valid(wanted, model) {
		return
	}
	pids := make([]string, 0, len(providers))
	for pid := range providers {
		pids = append(pids, pid)
	}
	sort.Strings(pids)
	for _, pid := range pids {
		p, ok := providers[pid].(map[string]interface{})
		if !ok {
			continue
		}
		if arr, ok := p["models"].([]interface{}); ok {
			for _, v := range arr {
				if m, ok := v.(map[string]interface{}); ok {
					mid := strings.TrimSpace(stringOr(m["id"], ""))
					if mid != "" {
						def["provider"] = pid
						def["model"] = mid
						return
					}
				}
			}
		}
		if mp, ok := p["models"].(map[string]interface{}); ok {
			mids := make([]string, 0, len(mp))
			for mid := range mp {
				mids = append(mids, mid)
			}
			sort.Strings(mids)
			if len(mids) > 0 {
				def["provider"] = pid
				def["model"] = mids[0]
				return
			}
		}
	}
	def["__dsh_delete__"] = true
}

// deepSeekApplyDefaultModel 应用 deepSeekEnsureDefaultInProviders 的结果到文档。
func deepSeekApplyDefaultModel(cfg map[string]interface{}, providers map[string]interface{}) {
	def, _ := cfg["agent-default-model"].(map[string]interface{})
	if def == nil {
		return
	}
	deepSeekEnsureDefaultInProviders(def, providers)
	if _, drop := def["__dsh_delete__"]; drop {
		delete(cfg, "agent-default-model")
	}
}
