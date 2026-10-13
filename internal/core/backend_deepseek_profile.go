package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agentprovidermanager/internal/dshprovider"
)

// errDeepSeekHomeOverride 表示 home 级 patch 覆盖了 llm-pi-ai 配置。
// 这与 DSH 自己的 config-editor 行为一致：被更高层覆盖时拒绝写入，而不是静默无效。
var errDeepSeekHomeOverride = fmt.Errorf("llm-pi-ai 配置被 $DSH_HOME/cordis.patch.yml 覆盖，请直接编辑该 home 级文件")

// deepseekProfileBackend 是 DSH 0.2 profile patch（cordis.patch.yml）的 backend。
//
// 桌面端与 web profile 的 patch 结构相同（同一个 dsh-base + dsh-web-app 栈），
// 差别只在 profile 目录，因此共用这个实现，只注册成不同格式。
// profile 为空表示按目标路径判定自定义 profile。
type deepseekProfileBackend struct{ profile string }

func (b deepseekProfileBackend) label() string {
	switch b.profile {
	case dshprovider.DesktopProfileName:
		return "DeepSeek Harness 桌面端 profile（0.2）"
	case dshprovider.WebProfileName:
		return "DeepSeek Harness web profile（0.2）"
	default:
		return "DeepSeek Harness 自定义 profile（0.2）"
	}
}

func (b deepseekProfileBackend) defaultPath() string {
	switch b.profile {
	case dshprovider.DesktopProfileName:
		return DeepSeekDesktopProfilePatch()
	case dshprovider.WebProfileName:
		return DeepSeekWebProfilePatch()
	default:
		return DeepSeekDefaultPath()
	}
}

func (b deepseekProfileBackend) Descriptor() BackendDescriptor {
	return BackendDescriptor{
		ID:          string(AgentDeepSeek),
		Label:       b.label(),
		DefaultPath: b.defaultPath(),
		Candidates:  DetectConfigLocationsForAgent(string(AgentDeepSeek)),
	}
}

func (b deepseekProfileBackend) Read(path string) (Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, &ConfigParseError{Msg: fmt.Sprintf("读取配置文件失败：%v", err)}
		}
		data = nil
	}
	patch, perr := dshprovider.LoadPatch(data)
	if perr != nil {
		return nil, &ConfigParseError{Msg: perr.Error()}
	}
	doc := newDeepSeekProfileDoc(path, patch)
	if err := dshHomePatchError(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (b deepseekProfileBackend) ReadWithFingerprint(path string) (Doc, string, error) {
	before, _ := FileFingerprint(path)
	doc, err := b.Read(path)
	if err != nil {
		return nil, "", err
	}
	after, _ := FileFingerprint(path)
	if before != after {
		return nil, "", fmt.Errorf("配置文件在读取期间被其他程序修改，请重试")
	}
	return doc, after, nil
}

// Write 先把暂存的凭据写进 $DSH_HOME/.credentials.yaml，再落盘 patch。
// 顺序刻意是“先凭据后配置”：配置写失败最多留下一个未被引用的 ref。
func (b deepseekProfileBackend) Write(path string, doc Doc, expectedFingerprint string) error {
	patch, err := dshPatchOf(doc)
	if err != nil {
		return err
	}
	data, err := patch.Bytes()
	if err != nil {
		return err
	}
	if err := flushDeepSeekCredentials(doc); err != nil {
		return err
	}
	return WriteConfigBytes(path, data, expectedFingerprint)
}

func (b deepseekProfileBackend) Store(_ Doc) ProviderStore { return deepseekProfileStore{} }

// newDeepSeekProfileDoc 组装文档，并顺带记录 home 级 patch 是否覆盖 llm-pi-ai，
// 使 ProviderStore 的全部方法保持“无文件 IO”。
//
// home 级 patch 存在但读取或解析失败时，无法判定它是否覆盖：把错误记进文档
// （docDshHomePatchErrKey），由 dshHomePatchError 在 Read / dshEffectiveProviders
// 处向上抛出，并按“已被覆盖”拒绝写入。静默当成“没有覆盖”会让一次 DSH 必然忽略的
// 写入看起来成功。
func newDeepSeekProfileDoc(path string, patch *dshprovider.Patch) Doc {
	doc := Doc{docDshPatchKey: patch, docDshPathKey: path}
	homePath := DeepSeekHomePatch()
	if strings.EqualFold(path, homePath) {
		return doc
	}
	data, err := os.ReadFile(homePath)
	if err != nil {
		if os.IsNotExist(err) {
			return doc
		}
		doc[docDshHomePatchErrKey] = fmt.Errorf("%w：读取 home 级 patch %s 失败：%v", errDeepSeekHomeOverride, homePath, err)
		return doc
	}
	patchHome, perr := dshprovider.LoadPatch(data)
	if perr != nil {
		doc[docDshHomePatchErrKey] = fmt.Errorf("%w：解析 home 级 patch %s 失败：%v", errDeepSeekHomeOverride, homePath, perr)
		return doc
	}
	overrides, oerr := dshprovider.DefinesLLMPiAIProviders(data)
	if oerr != nil {
		doc[docDshHomePatchErrKey] = fmt.Errorf("%w：解析 home 级 patch %s 失败：%v", errDeepSeekHomeOverride, homePath, oerr)
		return doc
	}
	if !overrides {
		return doc
	}
	homeProviders, herr := patchHome.Providers()
	if herr != nil {
		doc[docDshHomePatchErrKey] = fmt.Errorf("%w：解析 home 级 patch %s 的 providers 失败：%v", errDeepSeekHomeOverride, homePath, herr)
		return doc
	}
	doc[docDshHomeOverrideKey] = true
	doc[docDshHomeProvidersKey] = homeProviders
	return doc
}

// dshHomePatchError 取出因 home 级 patch 读取/解析失败而记录的错误；无错误时返回 nil。
func dshHomePatchError(doc Doc) error {
	err, _ := doc[docDshHomePatchErrKey].(error)
	return err
}

func dshPatchOf(doc Doc) (*dshprovider.Patch, error) {
	patch, _ := doc[docDshPatchKey].(*dshprovider.Patch)
	if patch == nil {
		return nil, fmt.Errorf("DeepSeek profile patch 文档缺失，请重新选择配置文件")
	}
	return patch, nil
}

func dshDocOverridden(doc Doc) bool {
	if v, _ := doc[docDshHomeOverrideKey].(bool); v {
		return true
	}
	// home 级 patch 读不出或解析失败时按“已被覆盖”处理：宁可拒绝写入，也不要写出
	// DSH 会静默忽略的配置。
	return dshHomePatchError(doc) != nil
}

// DeepSeekProfileShadowedByHome 报告该 profile patch 的 llm-pi-ai 是否被 home 级 patch 覆盖。
//
// 被覆盖时对 profile 层的写入注定无效。调用方（app 层）应在建立同级备份与还原点之前
// 就据此拒绝，否则一次注定失败的写入会留下无用的 .bak_ 与还原点。
//
// home 级 patch 存在但读取或解析失败时无法判定是否覆盖：此时返回包装了
// errDeepSeekHomeOverride 的错误，调用方应据此拒绝写入，而不是当成“没有覆盖”放行。
// 目标本身不是 profile patch、或就是 home 级文件本身时返回 (false, nil)。
func DeepSeekProfileShadowedByHome(path string) (bool, error) {
	if strings.TrimSpace(path) == "" || !IsDeepSeekProfilePath(path) {
		return false, nil
	}
	homePath := DeepSeekHomePatch()
	if strings.EqualFold(filepath.Clean(path), filepath.Clean(homePath)) {
		// 目标就是 home 级文件本身，不存在“被自己覆盖”。
		return false, nil
	}
	data, err := os.ReadFile(homePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("%w：读取 home 级 patch %s 失败：%v", errDeepSeekHomeOverride, homePath, err)
	}
	overrides, oerr := dshprovider.DefinesLLMPiAIProviders(data)
	if oerr != nil {
		return false, fmt.Errorf("%w：解析 home 级 patch %s 失败：%v", errDeepSeekHomeOverride, homePath, oerr)
	}
	if !overrides {
		return false, nil
	}
	patchHome, perr := dshprovider.LoadPatch(data)
	if perr != nil {
		return false, fmt.Errorf("%w：解析 home 级 patch %s 失败：%v", errDeepSeekHomeOverride, homePath, perr)
	}
	_, herr := patchHome.Providers()
	if herr != nil {
		return false, fmt.Errorf("%w：解析 home 级 patch %s 的 providers 失败：%v", errDeepSeekHomeOverride, homePath, herr)
	}
	return true, nil
}

// dshEffectiveProviders 返回生效的 provider 视图：
// home 级 patch 定义 llm-pi-ai 时以它为准（profile 层被覆盖）。
func dshEffectiveProviders(doc Doc) (map[string]interface{}, error) {
	if err := dshHomePatchError(doc); err != nil {
		return nil, err
	}
	if dshDocOverridden(doc) {
		providers, _ := doc[docDshHomeProvidersKey].(map[string]interface{})
		if providers == nil {
			providers = map[string]interface{}{}
		}
		return providers, nil
	}
	patch, err := dshPatchOf(doc)
	if err != nil {
		return nil, err
	}
	return patch.Providers()
}

// dshApplyDefaultModelInPatch 让 profile patch 里的 agent-default-model 条目跟随
// provider 变更；条目不存在时保持 base patch 的默认值不动。
func dshApplyDefaultModelInPatch(patch *dshprovider.Patch, providers map[string]interface{}) {
	def := patch.AgentDefaultModel()
	if def == nil {
		return
	}
	deepSeekEnsureDefaultInProviders(def, providers)
	if _, drop := def["__dsh_delete__"]; drop {
		patch.DeleteRow(dshprovider.AgentDefaultModelEntryID)
		return
	}
	_ = patch.SetAgentDefaultModel(def)
}

// dshRenameDefaultProviderInPatch 让 agent-default-model 跟随 provider 改名。
func dshRenameDefaultProviderInPatch(patch *dshprovider.Patch, oldID, newID string) {
	def := patch.AgentDefaultModel()
	if def == nil {
		return
	}
	if strings.TrimSpace(stringOr(def["provider"], "")) != oldID {
		return
	}
	def["provider"] = newID
	_ = patch.SetAgentDefaultModel(def)
}

type deepseekProfileStore struct{}

func (deepseekProfileStore) List(doc Doc) []ProviderSummary {
	providers, err := dshEffectiveProviders(doc)
	if err != nil {
		return nil
	}
	return deepSeekSummariesFromProviders(providers)
}

func (deepseekProfileStore) Get(doc Doc, providerID string) (*ProviderEdit, error) {
	providers, err := dshEffectiveProviders(doc)
	if err != nil {
		return nil, err
	}
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

func (deepseekProfileStore) Upsert(doc Doc, providerID string, p ProviderPayload, opt SaveOptions) (Doc, error) {
	if dshDocOverridden(doc) {
		return nil, errDeepSeekHomeOverride
	}
	newID, providerCfg, err := BuildProviderCfgEditor(p)
	if err != nil {
		return nil, err
	}
	dshApplyExplicitReasoningOff(providerCfg, p)
	patch, err := dshPatchOf(doc)
	if err != nil {
		return nil, err
	}
	providers, err := patch.Providers()
	if err != nil {
		return nil, err
	}
	existing, _ := providers[providerID].(map[string]interface{})
	if newID != providerID {
		delete(providers, providerID)
	}
	entry, envName, apiKey := deepSeekBuildEntry(newID, providerCfg, existing, opt.MergeModels, deepSeekTakenEnvNames(providers, providerID))
	providers[newID] = entry
	if err := patch.SetProviders(providers); err != nil {
		return nil, err
	}
	if newID != providerID {
		dshRenameDefaultProviderInPatch(patch, providerID, newID)
	}
	dshApplyDefaultModelInPatch(patch, providers)
	stageDeepSeekCredential(doc, envName, apiKey)
	return doc, nil
}

func (deepseekProfileStore) Delete(doc Doc, providerID string) (Doc, bool, error) {
	if dshDocOverridden(doc) {
		return nil, false, errDeepSeekHomeOverride
	}
	patch, err := dshPatchOf(doc)
	if err != nil {
		return nil, false, err
	}
	providers, err := patch.Providers()
	if err != nil {
		return nil, false, err
	}
	if providers[providerID] == nil {
		return nil, false, nil
	}
	delete(providers, providerID)
	if err := patch.SetProviders(providers); err != nil {
		return nil, false, err
	}
	dshApplyDefaultModelInPatch(patch, providers)
	return doc, true, nil
}

func (deepseekProfileStore) DeleteModel(doc Doc, providerID, modelID string) (Doc, bool, error) {
	if dshDocOverridden(doc) {
		return nil, false, errDeepSeekHomeOverride
	}
	patch, err := dshPatchOf(doc)
	if err != nil {
		return nil, false, err
	}
	providers, err := patch.Providers()
	if err != nil {
		return nil, false, err
	}
	raw, _ := providers[providerID].(map[string]interface{})
	if raw == nil {
		return nil, false, fmt.Errorf("提供商「%s」不存在", providerID)
	}
	if !deepSeekRemoveModel(raw, modelID) {
		return nil, false, nil
	}
	if err := patch.SetProviders(providers); err != nil {
		return nil, false, err
	}
	dshApplyDefaultModelInPatch(patch, providers)
	return doc, true, nil
}
