package core

import (
	"fmt"
	"strings"

	"agentprovidermanager/internal/zcodeprovider"
)

// Doc 是一个 config backend 的文档，即配置文件在内存中的表示。
type Doc map[string]interface{}

// ProviderPayload 是编辑器提交的 provider 原始对象，
// 与 BuildProviderCfgEditor(provider map[string]interface{}) 的入参完全一致。
type ProviderPayload map[string]interface{}

// BackendDescriptor 描述一个 config backend 的静态信息。
// Candidates 由各 backend 自行给出（legacy ZCode 用 DetectConfigLocationsForAgent 探测）。
type BackendDescriptor struct {
	ID          string
	Label       string
	DefaultPath string
	Candidates  []ConfigLocation
}

// SaveOptions 控制 Upsert 的文档变换方式。
type SaveOptions struct {
	MergeModels bool
}

// ProviderStore 在已读取的文档上提供 provider / model 视图。
// 所有方法只做文档变换：不做备份、不做文件 IO，指纹校验与写盘由调用方负责。
type ProviderStore interface {
	List(doc Doc) []ProviderSummary
	Get(doc Doc, providerID string) (*ProviderEdit, error)
	Upsert(doc Doc, providerID string, p ProviderPayload, opt SaveOptions) (Doc, error)
	Delete(doc Doc, providerID string) (Doc, bool, error)
	DeleteModel(doc Doc, providerID, modelID string) (Doc, bool, error)
}

// Backend 是一个 agent 的配置文件后端。
type Backend interface {
	Descriptor() BackendDescriptor
	Read(path string) (Doc, error)
	ReadWithFingerprint(path string) (Doc, string, error)
	Write(path string, doc Doc, expectedFingerprint string) error
	Store(doc Doc) ProviderStore
}

var backends = map[string]Backend{}

// formatBackends 保存“同一个 agent 因目标文件格式不同而不同”的 backend，
// 外层键为 agentID 小写形式，内层键为 zcodeprovider.Format。
// 目前只有 ZCode：新版 provider_config.json 与旧版 config.json 的实现不同。
var formatBackends = map[string]map[zcodeprovider.Format]Backend{}

// RegisterBackend 注册一个 config backend，键为 Descriptor().ID 的小写形式。
func RegisterBackend(b Backend) {
	backends[strings.ToLower(strings.TrimSpace(b.Descriptor().ID))] = b
}

// RegisterBackendForFormat 为指定 agent 注册“按目标文件格式”生效的 backend。
// 同一 agent 可以注册多个格式；未命中格式时回退到 RegisterBackend 的默认实现。
func RegisterBackendForFormat(agentID string, format zcodeprovider.Format, b Backend) {
	key := strings.ToLower(strings.TrimSpace(agentID))
	if formatBackends[key] == nil {
		formatBackends[key] = map[zcodeprovider.Format]Backend{}
	}
	formatBackends[key][format] = b
}

// GetBackend 按 agentID 查找 config backend，未注册的 agent 返回错误。
// ZCode 额外按目标文件内容分流：新版（provider_config.json）走 v2 backend，
// 其余情况（旧版 / 无法判定）走 legacy backend，二者都不遗漏既有的 legacy 行为。
// 其它 agent 不受影响，仍只按 ID 查找。
func GetBackend(agentID, path string) (Backend, error) {
	key := strings.ToLower(strings.TrimSpace(agentID))
	if variants, ok := formatBackends[key]; ok {
		if b, ok := variants[DetectZCodeFormatAt(path)]; ok {
			return b, nil
		}
	}
	if b, ok := backends[key]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("agent「%s」尚未提供 config backend", agentID)
}
