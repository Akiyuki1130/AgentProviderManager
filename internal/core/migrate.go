package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MigrateSide holds preview info for one agent side.
type MigrateSide struct {
	Agent     string            `json:"agent"`
	Label     string            `json:"label"`
	Path      string            `json:"path"`
	Exists    bool              `json:"exists"`
	Error     string            `json:"error"`
	Providers []ProviderSummary `json:"providers"`
	RawText   string            `json:"raw_text"`
	Format    string            `json:"format"`
}

// MigratePreview aggregates source and target sides.
type MigratePreview struct {
	Source MigrateSide `json:"source"`
	Target MigrateSide `json:"target"`
}

func agentLabel(id string) string {
	for _, a := range AllAgents() {
		if string(a.ID) == NormalizeAgentID(id).String() {
			return a.Label
		}
	}
	return id
}

func (a AgentID) String() string { return string(a) }

func EnsureDeepSeekDefaultModel(cfg map[string]interface{}) { ensureDeepSeekDefaultModel(cfg) }

func ensureDeepSeekDefaultModel(cfg map[string]interface{}) {
	providers := deepSeekProvidersMap(cfg)
	def, _ := cfg["agent-default-model"].(map[string]interface{})
	if def == nil {
		return
	}
	deepSeekEnsureDefaultInProviders(def, providers)
	if _, drop := def["__dsh_delete__"]; drop {
		delete(cfg, "agent-default-model")
	}
}

// readRawText 读取配置文件原文用于迁移预览。返回的文本一律脱敏：预览会渲染进界面，
// 明文密钥没有必要出现在那里（用户核对的是结构，不是密钥本身），
// 真正的写入路径照旧使用完整配置。
func readRawText(path string) (string, string) {
	text, format := readRawConfigText(path)
	return RedactSensitive(text, nil), format
}

func readRawConfigText(path string) (string, string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	if isYAMLPath(path) {
		// Normalize via yaml round-trip for readable display, but keep original if small
		if len(data) > 200*1024 {
			return string(data[:200*1024]) + "\n... (truncated)", "yaml"
		}
		return string(data), "yaml"
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "{}", "json"
	}
	// Try pretty print JSONC -> JSON
	cleaned := StripJSONCComments(text)
	var v interface{}
	if err := json.Unmarshal([]byte(cleaned), &v); err == nil {
		pretty, _ := json.MarshalIndent(v, "", "  ")
		if len(pretty) > 200*1024 {
			return string(pretty[:200*1024]) + "\n... (truncated)", "json"
		}
		return string(pretty), "json"
	}
	if len(data) > 200*1024 {
		return string(data[:200*1024]) + "\n... (truncated)", "json"
	}
	return string(data), "json"
}

// ---- 路径解析与校验 ----

// resolveMigratePath 把显式路径规整为实际使用的路径；为空时按 Agent 取“优先”默认：
// ZCode 新版优先（ZCodeDefaultPath），DeepSeek Harness 0.2 profile 优先（DeepSeekDefaultPath）。
func resolveMigratePath(agentID, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	switch NormalizeAgentID(agentID) {
	case AgentZCode:
		return ZCodeDefaultPath()
	case AgentDeepSeek:
		return DeepSeekDefaultPath()
	default:
		return AgentDefaultPath(NormalizeAgentID(agentID))
	}
}

func migratePathExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// cleanMigratePath 返回绝对、清理过的路径，用于“源与目标是否同一文件”的比较。
func cleanMigratePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Clean(path)
}

// sameMigratePath 大小写不敏感地比较两个清理过的路径。Windows 磁盘路径本就大小写不敏感，
// 两端都做 Clean + Abs 后 EqualFold，避免同一个文件被当成源和目标。
func sameMigratePath(a, b string) bool {
	ca, cb := cleanMigratePath(a), cleanMigratePath(b)
	if ca == "" || cb == "" {
		return false
	}
	return strings.EqualFold(ca, cb)
}

// ---- backend 选择与形态名 ----

// migrateBackendFor 选择迁移使用的 backend。
//
// 正常情况下完全交给 GetBackend（内容/路径探测）。唯一的补充是：ZCode 的
// provider_config.json 在文件尚不存在时只能退到 legacy backend，而 ZCode 对该文件名
// 使用严格 v2 schema；此时按路径兜底到 v2 backend，保证“目标文件不存在”也能写出
// ZCode 读得懂的文档。
func migrateBackendFor(agentID, path string) (Backend, error) {
	backend, err := GetBackend(agentID, path)
	if err != nil {
		return nil, err
	}
	if NormalizeAgentID(agentID) != AgentZCode || migratePathExists(path) {
		return backend, nil
	}
	if !strings.EqualFold(filepath.Base(path), filepath.Base(ZCodeProviderConfigPath())) {
		return backend, nil
	}
	if v2, ok := formatBackends[string(AgentZCode)][BackendFormatZCodeV2]; ok {
		return v2, nil
	}
	return backend, nil
}

// migrateFormatName 返回 MigrateSide.format。优先使用 AgentFormatName 的形态判定
// （v2 / legacy / profile-desktop / profile-web / profile-other / settings-legacy）；
// 文件不存在等无法判定时回退：v2 backend 记 v2，其余记 json。
func migrateFormatName(agentID, path string, backend Backend) string {
	name := AgentFormatName(agentID, path)
	if name != "" && name != "unknown" {
		return name
	}
	if _, ok := backend.(zcodeV2Backend); ok {
		return "v2"
	}
	return "json"
}

// ---- 预览 ----

func buildSide(agentID, path string) MigrateSide {
	norm := string(NormalizeAgentID(agentID))
	side := MigrateSide{
		Agent:     norm,
		Label:     agentLabel(norm),
		Path:      path,
		Providers: []ProviderSummary{},
		Format:    "json",
	}
	if norm == string(AgentOpenCode) {
		side.Error = "OpenCode 暂不支持跨 Agent 迁移"
		return side
	}
	side.Exists = migratePathExists(path)
	raw, _ := readRawText(path)
	side.RawText = raw

	backend, err := migrateBackendFor(norm, path)
	if err != nil {
		side.Error = fmt.Sprintf("读取配置失败：%v", err)
		return side
	}
	side.Format = migrateFormatName(norm, path, backend)
	doc, err := backend.Read(path)
	if err != nil {
		side.Error = fmt.Sprintf("读取配置失败：%v", err)
		return side
	}
	side.Providers = backend.Store(doc).List(doc)
	if side.Providers == nil {
		side.Providers = []ProviderSummary{}
	}
	return side
}

// MigratePreviewAtPaths builds preview for source and target at explicit paths.
// 空路径按 Agent 的优先默认路径解析。
func MigratePreviewAtPaths(sourceAgent, targetAgent, sourcePath, targetPath string) MigratePreview {
	srcNorm := string(NormalizeAgentID(sourceAgent))
	tgtNorm := string(NormalizeAgentID(targetAgent))
	srcPath := resolveMigratePath(srcNorm, sourcePath)
	tgtPath := resolveMigratePath(tgtNorm, targetPath)
	preview := MigratePreview{
		Source: buildSide(srcNorm, srcPath),
		Target: buildSide(tgtNorm, tgtPath),
	}
	if sameMigratePath(srcPath, tgtPath) {
		msg := "源配置路径与目标配置路径相同，无法迁移"
		if preview.Source.Error == "" {
			preview.Source.Error = msg
		} else if preview.Target.Error == "" {
			preview.Target.Error = msg
		}
	}
	return preview
}

// MigratePreviewForAgents builds preview for source and target.
// source 为空默认 zcode；target 为空时在 zcode 与 deepseek 中选与 source 不同的那个。
func MigratePreviewForAgents(sourceAgent, targetAgent string) MigratePreview {
	src := strings.TrimSpace(sourceAgent)
	if src == "" {
		src = string(AgentZCode)
	}
	tgt := strings.TrimSpace(targetAgent)
	if tgt == "" {
		if NormalizeAgentID(src) == AgentZCode {
			tgt = string(AgentDeepSeek)
		} else {
			tgt = string(AgentZCode)
		}
	}
	return MigratePreviewAtPaths(src, tgt, "", "")
}

// ---- 执行 ----

func dedupeMigrateIDs(selectedIDs []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// migratePayload 由源 ProviderEdit 构造目标 store 的写入载荷，键与 BuildProviderCfgEditor 一致。
// options_json 可选：仅在源 provider 带额外 options 时携带，避免丢失 headers 等键。
func migratePayload(edit *ProviderEdit, apiKey string, cards []ModelCard) ProviderPayload {
	if cards == nil {
		cards = []ModelCard{}
	}
	payload := ProviderPayload{
		"id":               edit.ID,
		"name":             edit.Name,
		"kind":             edit.Kind,
		"base_url":         edit.BaseURL,
		"api_key":          apiKey,
		"api_key_required": edit.APIKey != "",
		"cards":            cards,
	}
	if strings.TrimSpace(edit.OptionsJSON) != "" {
		payload["options_json"] = edit.OptionsJSON
	}
	return payload
}

// migrateTargetError 保留被 home 级 patch 覆盖这一特殊错误原文，其余写入错误统一加前缀。
func migrateTargetError(err error) error {
	if errors.Is(err, errDeepSeekHomeOverride) {
		return err
	}
	return fmt.Errorf("写入目标配置失败：%v", err)
}

// MigrateExecute performs migration from source to target.
func MigrateExecute(sourceAgent, targetAgent string, selectedIDs []string, mode string) (map[string]interface{}, error) {
	return MigrateExecuteAtPaths(sourceAgent, targetAgent, "", "", selectedIDs, mode)
}

// MigrateExecuteAtPaths 让源与目标都经过 backend：
// GetBackend -> Read/ReadWithFingerprint -> Store(doc)，目标写盘前统一 PrepareChange。
func MigrateExecuteAtPaths(sourceAgent, targetAgent, sourcePath, targetPath string, selectedIDs []string, mode string) (map[string]interface{}, error) {
	srcNorm := string(NormalizeAgentID(sourceAgent))
	tgtNorm := string(NormalizeAgentID(targetAgent))
	if srcNorm == tgtNorm {
		return nil, fmt.Errorf("源和目标不能是同一个 Agent")
	}
	if srcNorm == string(AgentOpenCode) || tgtNorm == string(AgentOpenCode) {
		return nil, fmt.Errorf("OpenCode 暂不支持跨 Agent 迁移")
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "overwrite" {
		mode = "merge"
	}
	ids := dedupeMigrateIDs(selectedIDs)
	if len(ids) == 0 {
		return nil, fmt.Errorf("请至少选择一个提供商进行迁移")
	}
	srcPath := resolveMigratePath(srcNorm, sourcePath)
	tgtPath := resolveMigratePath(tgtNorm, targetPath)
	if sameMigratePath(srcPath, tgtPath) {
		return nil, fmt.Errorf("源配置路径与目标配置路径相同，无法迁移")
	}

	// 源：只读，按 store.Get 读取每个选中的 provider。
	srcBackend, err := migrateBackendFor(srcNorm, srcPath)
	if err != nil {
		return nil, fmt.Errorf("读取源配置失败：%v", err)
	}
	srcDoc, err := srcBackend.Read(srcPath)
	if err != nil {
		return nil, fmt.Errorf("读取源配置失败：%v", err)
	}
	srcStore := srcBackend.Store(srcDoc)
	edits := make([]*ProviderEdit, 0, len(ids))
	for _, pid := range ids {
		edit, gerr := srcStore.Get(srcDoc, pid)
		if gerr != nil {
			return nil, fmt.Errorf("读取源配置失败：%v", gerr)
		}
		if edit == nil {
			return nil, fmt.Errorf("源配置中不存在提供商「%s」", pid)
		}
		edits = append(edits, edit)
	}

	// 目标：读指纹，文档变换全部走目标 store。
	tgtBackend, err := migrateBackendFor(tgtNorm, tgtPath)
	if err != nil {
		return nil, fmt.Errorf("读取目标配置失败：%v", err)
	}
	tgtDoc, fingerprint, err := tgtBackend.ReadWithFingerprint(tgtPath)
	if err != nil {
		return nil, fmt.Errorf("读取目标配置失败：%v", err)
	}
	if tgtDoc == nil {
		tgtDoc = Doc{}
	}
	tgtStore := tgtBackend.Store(tgtDoc)

	if mode == "overwrite" {
		// 先清空目标里已有的全部 provider（Delete 会同步 agent-default-model）。
		for _, summary := range tgtStore.List(tgtDoc) {
			next, _, derr := tgtStore.Delete(tgtDoc, summary.ID)
			if derr != nil {
				return nil, migrateTargetError(derr)
			}
			if next != nil {
				tgtDoc = next
			}
		}
		for _, edit := range edits {
			payload := migratePayload(edit, edit.APIKey, edit.Cards)
			next, uerr := tgtStore.Upsert(tgtDoc, edit.ID, payload, SaveOptions{MergeModels: false})
			if uerr != nil {
				return nil, migrateTargetError(uerr)
			}
			if next != nil {
				tgtDoc = next
			}
		}
	} else {
		for _, edit := range edits {
			apiKey := edit.APIKey
			cards := edit.Cards
			if existing, gerr := tgtStore.Get(tgtDoc, edit.ID); gerr == nil && existing != nil {
				// 合并模型集合：保留目标已有模型，源同名模型覆盖。
				cards = MergeModelCards(existing.Cards, edit.Cards)
				// 源没有 Key 时沿用目标已有的 Key，避免把目标 Key 抹掉。
				if strings.TrimSpace(apiKey) == "" && strings.TrimSpace(existing.APIKey) != "" {
					apiKey = existing.APIKey
				}
			}
			payload := migratePayload(edit, apiKey, cards)
			next, uerr := tgtStore.Upsert(tgtDoc, edit.ID, payload, SaveOptions{MergeModels: true})
			if uerr != nil {
				return nil, migrateTargetError(uerr)
			}
			if next != nil {
				tgtDoc = next
			}
		}
	}

	// 写盘前统一建立同级备份与还原点；指纹来自 ReadWithFingerprint。
	snapshot, err := PrepareChange(ChangeContext{Target: tgtPath, AgentID: tgtNorm, Operation: OpMigrate, Note: "跨 Agent 迁移目标"})
	if err != nil {
		return nil, err
	}
	if werr := tgtBackend.Write(tgtPath, tgtDoc, fingerprint); werr != nil {
		return nil, fmt.Errorf("写入目标配置失败：%v", werr)
	}
	bak := snapshot.BackupPath
	if bak == "" {
		bak = FindLatestBackup(tgtPath)
	}
	return map[string]interface{}{
		"success": true, "migrated": len(edits), "mode": mode,
		"source": srcNorm, "target": tgtNorm, "target_path": tgtPath,
		"backup": bak,
	}, nil
}
