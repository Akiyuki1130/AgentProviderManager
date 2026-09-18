package core

import "fmt"

// zcodeLegacyBackend 是现有 ZCode config.json（旧版格式）的 backend。
// 方法体照搬既有实现（LoadConfig / LoadConfigWithFingerprint / WriteConfig /
// BuildProviderSummary / ProviderToEdit / MergeProviderIntoConfig / RenameProviderInConfig /
// NormalizeConfigKinds），只做接口化封装，不改变行为，也不做备份与指纹校验（由 app 层负责）。
type zcodeLegacyBackend struct{}

func init() { RegisterBackend(zcodeLegacyBackend{}) }

func (zcodeLegacyBackend) Descriptor() BackendDescriptor {
	// Label 与 AllAgents() 中 AgentZCode 的 Label 一致。
	return BackendDescriptor{
		ID:          string(AgentZCode),
		Label:       "ZCode",
		DefaultPath: AgentDefaultPath(AgentZCode),
		Candidates:  DetectConfigLocationsForAgent(string(AgentZCode)),
	}
}

func (zcodeLegacyBackend) Read(path string) (Doc, error) {
	return LoadConfig(path)
}

func (zcodeLegacyBackend) ReadWithFingerprint(path string) (Doc, string, error) {
	return LoadConfigWithFingerprint(path)
}

func (zcodeLegacyBackend) Write(path string, doc Doc, expectedFingerprint string) error {
	return WriteConfig(path, doc, expectedFingerprint)
}

// Store 返回 legacy 文档视图。文档由各方法的入参提供，
// 这里绑定的 doc 在 legacy 实现中不额外使用（仅满足接口契约）。
func (zcodeLegacyBackend) Store(doc Doc) ProviderStore {
	return zcodeLegacyStore{}
}

type zcodeLegacyStore struct{}

func (zcodeLegacyStore) List(doc Doc) []ProviderSummary {
	return BuildProviderSummary(doc)
}

func (zcodeLegacyStore) Get(doc Doc, providerID string) (*ProviderEdit, error) {
	return ProviderToEdit(doc, providerID)
}

// Upsert 照搬 app.go SaveProvider 中 ZCode 分支的文档变换部分，
// 顺序为：kind 归一化 -> 重命名 -> MergeProviderIntoConfig。
// 注意：这里的 providerCfg 由 BuildProviderCfgEditor 就地构建，
// 应用层的“已存在”“模型列表为空”等预检查仍在 app.go 中、备份之前完成。
func (zcodeLegacyStore) Upsert(doc Doc, providerID string, p ProviderPayload, opt SaveOptions) (Doc, error) {
	newID, providerCfg, err := BuildProviderCfgEditor(p)
	if err != nil {
		return nil, err
	}
	NormalizeConfigKinds(doc)
	providers, _ := doc["provider"].(map[string]interface{})
	if providers == nil {
		providers = map[string]interface{}{}
	}
	if newID != providerID {
		if _, exists := providers[providerID]; exists {
			_, _ = RenameProviderInConfig(doc, providerID, newID)
		} else {
			if _, ok := doc["provider"]; !ok {
				doc["provider"] = map[string]interface{}{}
			}
			doc["provider"].(map[string]interface{})[newID] = map[string]interface{}{}
		}
	}
	return MergeProviderIntoConfig(doc, newID, providerCfg, opt.MergeModels)
}

// Delete 照搬 app.go DeleteProvider 中 ZCode 分支的文档变换部分。
// found=false 表示文档中没有该 provider（调用方据此返回“不存在”），此时返回 nil 文档。
func (zcodeLegacyStore) Delete(doc Doc, providerID string) (Doc, bool, error) {
	providers, _ := doc["provider"].(map[string]interface{})
	if providers == nil || providers[providerID] == nil {
		return nil, false, nil
	}
	delete(providers, providerID)
	return doc, true, nil
}

// DeleteModel 照搬 app.go DeleteModel 中 ZCode 分支的文档变换部分，
// 顺序为：kind 归一化 -> provider 存在性 -> model 存在性 -> 删除。
// provider 不存在时以 error 返回原有文案（bool 无法区分两种“不存在”）；
// model 不存在时返回 (nil, false, nil)。
func (zcodeLegacyStore) DeleteModel(doc Doc, providerID, modelID string) (Doc, bool, error) {
	NormalizeConfigKinds(doc)
	providers, _ := doc["provider"].(map[string]interface{})
	if providers == nil || providers[providerID] == nil {
		return nil, false, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	prov, _ := providers[providerID].(map[string]interface{})
	models, _ := prov["models"].(map[string]interface{})
	if models == nil || models[modelID] == nil {
		return nil, false, nil
	}
	delete(models, modelID)
	return doc, true, nil
}
