package core

import (
	"fmt"
	"strings"
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

// BackendFormat 是“同一个 agent 因目标文件形态不同而不同”的格式键。
// ZCode 用它区分 provider_config.json 与 config.json；
// DeepSeek Harness 用它区分 profile patch（desktop/web/其它）与旧版 settings.yaml。
type BackendFormat string

// BackendFormatDefault 是未命中格式探测时使用的键。
// 各 agent 的默认 backend 通过 RegisterBackend 注册，因此也能作为兜底。
const BackendFormatDefault BackendFormat = ""

// BackendFormatZCodeV2 是 ZCode 新版 provider_config.json 的格式键。
// zcodeprovider.Format 是整数枚举，不能直接转成字符串键，因此这里显式命名。
const BackendFormatZCodeV2 BackendFormat = "zcode-v2"

// formatDetectors 保存每个 agent 的格式探测函数。
// 探测必须只依赖路径与文件内容，不得有副作用：GetBackend 可能被任意 UI 操作调用。
var formatDetectors = map[string]func(path string) BackendFormat{}

var backends = map[string]Backend{}

// formatBackends 保存“同一个 agent 因目标文件格式不同而不同”的 backend，
// 外层键为 agentID 小写形式，内层键为 BackendFormat。
var formatBackends = map[string]map[BackendFormat]Backend{}

// RegisterBackend 注册一个 config backend，键为 Descriptor().ID 的小写形式。
//
// 注意：它会调用 Descriptor()，而 Descriptor 的 Candidates 需要探测并读取候选文件
// （统计 provider 数量）。包初始化阶段请改用 RegisterBackendFor，避免在用户还没选
// 目标文件时就去读真实配置。
func RegisterBackend(b Backend) {
	RegisterBackendFor(b.Descriptor().ID, b)
}

// RegisterBackendFor 按显式 agentID 注册该 agent 的默认 backend，不调用 Descriptor()。
func RegisterBackendFor(agentID string, b Backend) {
	backends[strings.ToLower(strings.TrimSpace(agentID))] = b
}

// RegisterBackendForFormat 为指定 agent 注册“按目标文件格式”生效的 backend。
// 同一 agent 可以注册多个格式；未命中格式时回退到 RegisterBackend 的默认实现。
func RegisterBackendForFormat(agentID string, format BackendFormat, b Backend) {
	key := strings.ToLower(strings.TrimSpace(agentID))
	if formatBackends[key] == nil {
		formatBackends[key] = map[BackendFormat]Backend{}
	}
	formatBackends[key][format] = b
}

// RegisterFormatDetector 为指定 agent 注册格式探测函数。
// 每个 agent 最多一个；重复注册会覆盖先前的函数。
func RegisterFormatDetector(agentID string, detect func(path string) BackendFormat) {
	formatDetectors[strings.ToLower(strings.TrimSpace(agentID))] = detect
}

// GetBackend 按 agentID 查找 config backend，未注册的 agent 返回错误。
// 注册了格式探测的 agent 先按探测结果分流（ZCode：v2 / legacy；
// DeepSeek：profile patch / settings.yaml），未命中格式时回退到该 agent 的默认 backend。
func GetBackend(agentID, path string) (Backend, error) {
	key := strings.ToLower(strings.TrimSpace(agentID))
	if variants, ok := formatBackends[key]; ok {
		format := BackendFormatDefault
		if detect, ok := formatDetectors[key]; ok && detect != nil {
			format = detect(path)
		}
		if b, ok := variants[format]; ok {
			return b, nil
		}
	}
	if b, ok := backends[key]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("agent「%s」尚未提供 config backend", agentID)
}
