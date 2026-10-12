package core

import (
	"fmt"
	"strings"

	"agentprovidermanager/internal/dshprovider"
)

// DeepSeek Harness 的 backend 注册。
//
// 同一个 agent 有两种形态：
//   - 0.2（当前桌面端与 dsh web）：profile patch 的 llm-pi-ai 条目，按 profile 再分
//     desktop / web / 自定义三种，做法相同、文件不同，因此注册成三个格式；
//   - 0.1：settings.yaml / settings.yml / settings.json，作为旧格式保留（DSH 0.2
//     只会把它导入一次）。
//
// 检测不出形态时回退到注册为默认的 legacy backend，行为与本次改造前一致。
func init() {
	RegisterBackendFor(string(AgentDeepSeek), deepseekLegacyBackend{})
	RegisterFormatDetector(string(AgentDeepSeek), detectDeepSeekBackendFormat)
	RegisterBackendForFormat(string(AgentDeepSeek), BackendFormat(dshprovider.FormatProfileDesktop), deepseekProfileBackend{profile: dshprovider.DesktopProfileName})
	RegisterBackendForFormat(string(AgentDeepSeek), BackendFormat(dshprovider.FormatProfileWeb), deepseekProfileBackend{profile: dshprovider.WebProfileName})
	RegisterBackendForFormat(string(AgentDeepSeek), BackendFormat(dshprovider.FormatProfileOther), deepseekProfileBackend{})
}

func detectDeepSeekBackendFormat(path string) BackendFormat {
	switch dshprovider.DetectFile(path) {
	case dshprovider.FormatProfileDesktop:
		return BackendFormat(dshprovider.FormatProfileDesktop)
	case dshprovider.FormatProfileWeb:
		return BackendFormat(dshprovider.FormatProfileWeb)
	case dshprovider.FormatProfileOther:
		return BackendFormat(dshprovider.FormatProfileOther)
	default:
		return BackendFormat(dshprovider.FormatSettingsLegacy)
	}
}

// deepseekLegacyBackend 是 0.1 settings.* 的 backend。
type deepseekLegacyBackend struct{}

func (deepseekLegacyBackend) Descriptor() BackendDescriptor {
	return BackendDescriptor{
		ID:          string(AgentDeepSeek),
		Label:       "DeepSeek Harness（旧版 settings）",
		DefaultPath: DeepSeekSettingsPath(),
		Candidates:  DetectConfigLocationsForAgent(string(AgentDeepSeek)),
	}
}

func (deepseekLegacyBackend) Read(path string) (Doc, error) {
	return DeepSeekLoadConfig(path)
}

func (deepseekLegacyBackend) ReadWithFingerprint(path string) (Doc, string, error) {
	before, _ := FileFingerprint(path)
	doc, err := DeepSeekLoadConfig(path)
	if err != nil {
		return nil, "", err
	}
	after, _ := FileFingerprint(path)
	if before != after {
		return nil, "", fmt.Errorf("配置文件在读取期间被其他程序修改，请重试")
	}
	return doc, after, nil
}

func (deepseekLegacyBackend) Write(path string, doc Doc, expectedFingerprint string) error {
	if err := flushDeepSeekCredentials(doc); err != nil {
		return err
	}
	if err := WriteDeepSeekYAMLIfYAML(path, doc, expectedFingerprint); err != nil {
		return err
	}
	return nil
}

func (deepseekLegacyBackend) Store(_ Doc) ProviderStore { return deepseekLegacyStore{} }

// WriteDeepSeekYAMLIfYAML 按目标后缀选择 YAML 或 JSON 序列化（0.1 与 0.2 的
// settings.* 都可能存在），写盘入口与 legacy 行为一致。
func WriteDeepSeekYAMLIfYAML(path string, cfg map[string]interface{}, fingerprint string) error {
	if isYAMLPath(path) {
		return WriteDeepSeekYAML(path, cfg, fingerprint)
	}
	return WriteConfig(path, cfg, fingerprint)
}

type deepseekLegacyStore struct{}

func (deepseekLegacyStore) List(doc Doc) []ProviderSummary {
	return deepSeekSummariesFromProviders(deepSeekProvidersMap(doc))
}

func (deepseekLegacyStore) Get(doc Doc, providerID string) (*ProviderEdit, error) {
	providers := deepSeekProvidersMap(doc)
	raw, _ := providers[providerID].(map[string]interface{})
	if raw == nil {
		return nil, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	pid, _, err := ConvertDeepSeekProvider(providerID, raw, nil)
	if err != nil {
		return nil, err
	}
	return DeepSeekProviderToEdit(pid, raw)
}

// Upsert 复刻改造前 app.go SaveProvider 的 DeepSeek 分支：
// llm-pi-ai 段走 pi-ai 路由条目编码并暂存凭据；旧式顶层 provider 映射保持原样合并。
func (deepseekLegacyStore) Upsert(doc Doc, providerID string, p ProviderPayload, opt SaveOptions) (Doc, error) {
	newID, providerCfg, err := BuildProviderCfgEditor(p)
	if err != nil {
		return nil, err
	}
	dshApplyExplicitReasoningOff(providerCfg, p)
	deepSeekUpsertLegacy(doc, providerID, newID, providerCfg, opt.MergeModels)
	return doc, nil
}

func (deepseekLegacyStore) Delete(doc Doc, providerID string) (Doc, bool, error) {
	if deepSeekProvidersMap(doc)[providerID] == nil {
		return nil, false, nil
	}
	deepSeekRemoveProvider(doc, providerID)
	deepSeekApplyDefaultModel(doc, deepSeekProvidersMap(doc))
	return doc, true, nil
}

func (deepseekLegacyStore) DeleteModel(doc Doc, providerID, modelID string) (Doc, bool, error) {
	prov, err := deepSeekProviderEntry(doc, providerID)
	if err != nil {
		return nil, false, err
	}
	if !deepSeekRemoveModel(prov, modelID) {
		return nil, false, nil
	}
	deepSeekApplyDefaultModel(doc, deepSeekProvidersMap(doc))
	return doc, true, nil
}

// deepSeekUpsertLegacy 对 settings 文档应用一次 provider 写入（含改名与默认模型修正）。
func deepSeekUpsertLegacy(doc map[string]interface{}, providerID, newID string, providerCfg map[string]interface{}, mergeModels bool) {
	deepSeekUpsertEntry(doc, providerID, newID, providerCfg, mergeModels)
	if newID != providerID {
		deepSeekRenameDefaultProvider(doc, providerID, newID)
	}
	deepSeekApplyDefaultModel(doc, deepSeekProvidersMap(doc))
}

// deepSeekUpsertEntry 只做条目本身的写入：llm-pi-ai 段走 pi-ai 路由条目编码并暂存凭据，
// 旧式顶层 provider 映射保持原样合并。改名与默认模型修正由调用方决定。
func deepSeekUpsertEntry(doc map[string]interface{}, providerID, newID string, providerCfg map[string]interface{}, mergeModels bool) {
	src, _ := deepSeekProviderSource(doc, providerID)
	llmMode := isDeepSeekLLMMode(doc)
	if src == "llm" {
		llmMode = true
	} else if src == "provider" {
		llmMode = false
	}
	if llmMode {
		llm, _ := doc["llm-pi-ai"].(map[string]interface{})
		if llm == nil {
			llm = map[string]interface{}{}
			doc["llm-pi-ai"] = llm
		}
		providers, _ := llm["providers"].(map[string]interface{})
		if providers == nil {
			providers = map[string]interface{}{}
			llm["providers"] = providers
		}
		existing, _ := providers[providerID].(map[string]interface{})
		if newID != providerID {
			delete(providers, providerID)
		}
		entry, envName, apiKey := deepSeekBuildEntry(newID, providerCfg, existing, mergeModels)
		providers[newID] = entry
		stageDeepSeekCredential(doc, envName, apiKey)
	} else {
		providers, _ := doc["provider"].(map[string]interface{})
		if providers == nil {
			providers = map[string]interface{}{}
			doc["provider"] = providers
		}
		var entry map[string]interface{}
		if existing, ok := providers[providerID].(map[string]interface{}); ok {
			entry = deepCopyMap(existing)
		}
		if entry == nil {
			entry = map[string]interface{}{}
		}
		if newID != providerID {
			delete(providers, providerID)
		}
		for k, v := range providerCfg {
			entry[k] = deepCopyValue(v)
		}
		providers[newID] = entry
	}
}

// deepSeekRenameDefaultProvider 让 agent-default-model 跟随 provider 改名。
func deepSeekRenameDefaultProvider(doc map[string]interface{}, oldID, newID string) {
	def, _ := doc["agent-default-model"].(map[string]interface{})
	if def == nil {
		return
	}
	if strings.TrimSpace(stringOr(def["provider"], "")) == oldID {
		def["provider"] = newID
	}
}

// deepSeekRemoveProvider 从文档里删除一条 provider（按来源精确删除，来源不明时两边都删）。
func deepSeekRemoveProvider(doc map[string]interface{}, providerID string) {
	src, _ := deepSeekProviderSource(doc, providerID)
	dropLLM := src == "llm" || src == ""
	dropTop := src == "provider" || src == ""
	if dropLLM {
		if llm, ok := doc["llm-pi-ai"].(map[string]interface{}); ok {
			if providers, ok := llm["providers"].(map[string]interface{}); ok {
				delete(providers, providerID)
			}
		}
	}
	if dropTop {
		if providers, ok := doc["provider"].(map[string]interface{}); ok {
			delete(providers, providerID)
		}
	}
}

// deepSeekProviderEntry 返回文档中某条 provider 的原始映射。
func deepSeekProviderEntry(doc map[string]interface{}, providerID string) (map[string]interface{}, error) {
	src, _ := deepSeekProviderSource(doc, providerID)
	var prov map[string]interface{}
	if src == "llm" {
		if llm, ok := doc["llm-pi-ai"].(map[string]interface{}); ok {
			if providers, ok := llm["providers"].(map[string]interface{}); ok {
				prov, _ = providers[providerID].(map[string]interface{})
			}
		}
	} else if src == "provider" {
		if providers, ok := doc["provider"].(map[string]interface{}); ok {
			prov, _ = providers[providerID].(map[string]interface{})
		}
	}
	if prov == nil {
		all := deepSeekProvidersMap(doc)
		if all == nil || all[providerID] == nil {
			return nil, fmt.Errorf("提供商「%s」不存在", providerID)
		}
		var ok bool
		prov, ok = all[providerID].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("提供商「%s」配置格式错误", providerID)
		}
	}
	return prov, nil
}

// deepSeekRemoveModel 删除路由下的一条模型；返回是否找到并删除。
func deepSeekRemoveModel(prov map[string]interface{}, modelID string) bool {
	if arr, ok := prov["models"].([]interface{}); ok {
		newArr := make([]interface{}, 0, len(arr))
		found := false
		for _, e := range arr {
			mm, _ := e.(map[string]interface{})
			id, _ := mm["id"].(string)
			if id == modelID {
				found = true
				continue
			}
			newArr = append(newArr, e)
		}
		if !found {
			return false
		}
		prov["models"] = newArr
		return true
	}
	models, _ := prov["models"].(map[string]interface{})
	if models == nil || models[modelID] == nil {
		return false
	}
	delete(models, modelID)
	return true
}
