package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agentprovidermanager/internal/core"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// apm-backup 导出 / 导入绑定。导出与其他写路径一样在 a.mu.Lock 下读当前目标；
// 导入复用 ImportProvider（它自己拿锁），这里绝不能再次加锁。

const maxBackupFileBytes = 4 << 20

func (a *App) ChooseBackupSavePath(suggestedName string) map[string]interface{} {
	if a.ctx == nil {
		return map[string]interface{}{"success": false, "error": "窗口未就绪"}
	}
	initDir := core.ZCodeDir()
	if strings.TrimSpace(a.targetConfigPath) != "" {
		initDir = filepath.Dir(a.targetConfigPath)
	}
	name := strings.TrimSpace(suggestedName)
	if name == "" {
		name = "agent-provider-backup.json"
	}
	result, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultDirectory: initDir,
		DefaultFilename:  name,
		Filters:          []runtime.FileFilter{{DisplayName: "JSON 备份文件", Pattern: "*.json"}},
	})
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	if result == "" {
		return map[string]interface{}{"success": false, "error": "已取消"}
	}
	return map[string]interface{}{"success": true, "path": result}
}

// collectProviderEdits 按 backend 分流规则读取目标文件的完整 ProviderEdit 列表；
// selected 为空表示全部 provider。解析方式与 RefreshProviderModels 一致。
func collectProviderEdits(agent, target string, selected []string) ([]core.ProviderEdit, error) {
	var ids []string
	var get func(id string) (*core.ProviderEdit, error)
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, rerr := backend.Read(target)
		if rerr != nil {
			return nil, rerr
		}
		store := backend.Store(doc)
		for _, s := range store.List(doc) {
			ids = append(ids, s.ID)
		}
		get = func(id string) (*core.ProviderEdit, error) { return store.Get(doc, id) }
	} else {
		cfg, err := core.LoadConfig(target)
		if err != nil {
			return nil, err
		}
		for _, s := range core.BuildProviderSummary(cfg) {
			ids = append(ids, s.ID)
		}
		get = func(id string) (*core.ProviderEdit, error) { return core.ProviderToEdit(cfg, id) }
	}
	if len(selected) > 0 {
		ids = selected
	}
	out := make([]core.ProviderEdit, 0, len(ids))
	for _, id := range ids {
		edit, err := get(id)
		if err != nil {
			return nil, err
		}
		out = append(out, *edit)
	}
	return out, nil
}

func (a *App) ExportBackup(opts map[string]interface{}) map[string]interface{} {
	if opts == nil {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	path := strings.TrimSpace(stringOr(opts["path"], ""))
	if path == "" {
		return map[string]interface{}{"success": false, "error": "请先选择备份保存位置"}
	}
	includeKeys, _ := opts["include_keys"].(bool)
	keyMode := strings.TrimSpace(stringOr(opts["key_mode"], ""))
	if keyMode == "" {
		keyMode = "plain"
	}
	password := stringOr(opts["password"], "")
	if keyMode == "password" && strings.TrimSpace(password) == "" {
		return map[string]interface{}{"success": false, "error": "密码加密备份必须提供密码"}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	var selected []string
	if raw, ok := opts["provider_ids"].([]interface{}); ok {
		for _, e := range raw {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" {
				selected = append(selected, strings.TrimSpace(s))
			}
		}
	}
	providers, err := collectProviderEdits(agent, target, selected)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	if !includeKeys {
		providers = core.StripBackupKeys(providers)
	}
	pkg := core.BackupPackage{
		CreatedAt: time.Now().Format(time.RFC3339), AppVersion: core.AppVersion,
		Agent: agent, IncludeKeys: includeKeys, KeyMode: keyMode,
		ProviderCount: len(providers), Providers: providers,
	}
	data, err := core.SealBackup(pkg, password)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	return map[string]interface{}{"success": true, "path": path, "provider_count": len(providers), "key_mode": keyMode, "include_keys": includeKeys}
}

func (a *App) OpenBackupFile() map[string]interface{} {
	if a.ctx == nil {
		return map[string]interface{}{"success": false, "error": "窗口未就绪"}
	}
	initDir := core.ZCodeDir()
	if strings.TrimSpace(a.targetConfigPath) != "" {
		initDir = filepath.Dir(a.targetConfigPath)
	}
	result, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		DefaultDirectory: initDir,
		Filters:          []runtime.FileFilter{{DisplayName: "JSON 备份文件", Pattern: "*.json"}},
	})
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	if result == "" {
		return map[string]interface{}{"success": false, "error": "已取消"}
	}
	return map[string]interface{}{"success": true, "path": result}
}

// readBackupPackage 读取并解密备份文件；失败时返回可直接回给前端的响应。
// needs_password 只在“密文是密码模式”时为 true，前端据此决定是否弹密码框。
func readBackupPackage(path, password string) (*core.BackupPackage, map[string]interface{}) {
	if strings.TrimSpace(path) == "" {
		return nil, map[string]interface{}{"success": false, "error": "请选择备份文件"}
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	if info.Size() > maxBackupFileBytes {
		return nil, map[string]interface{}{"success": false, "error": "备份文件超过 4MB 上限"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	pkg, err := core.OpenBackup(data, password)
	if err != nil {
		if errors.Is(err, core.ErrBackupNeedsPassword) {
			return nil, map[string]interface{}{"success": false, "needs_password": true, "error": "该备份已加密，请输入密码"}
		}
		if errors.Is(err, core.ErrBackupWrongPassword) {
			return nil, map[string]interface{}{"success": false, "needs_password": true, "error": core.ShortText(err.Error(), 300)}
		}
		return nil, map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	return pkg, nil
}

func (a *App) InspectBackup(path, password string) map[string]interface{} {
	pkg, fail := readBackupPackage(path, password)
	if pkg == nil {
		return fail
	}
	list := make([]map[string]interface{}, 0, len(pkg.Providers))
	for _, p := range pkg.Providers {
		list = append(list, map[string]interface{}{
			"id": p.ID, "name": p.Name, "kind": p.Kind,
			"model_count": len(p.Cards), "has_api_key": p.APIKey != "",
		})
	}
	return map[string]interface{}{
		"success": true, "agent": pkg.Agent, "agent_label": core.AgentLabelFor(pkg.Agent),
		"created_at": pkg.CreatedAt, "app_version": pkg.AppVersion,
		"include_keys": pkg.IncludeKeys, "key_mode": pkg.KeyMode,
		"provider_count": len(pkg.Providers), "needs_password": false,
		"providers": list,
	}
}

func (a *App) ImportBackup(path, password, conflict string) map[string]interface{} {
	pkg, fail := readBackupPackage(path, password)
	if pkg == nil {
		return fail
	}
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	// 导入一律落到当前选中的 Agent 配置文件；信封里的 agent 仅作展示，不会自动切换。
	// 取到完整 edit 是为了覆盖导入时保留本地 Key；Get 失败只记「存在但无 edit」。
	existingEdits := map[string]*core.ProviderEdit{}
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		if doc, rerr := backend.Read(target); rerr == nil {
			store := backend.Store(doc)
			for _, s := range store.List(doc) {
				edit, gerr := store.Get(doc, s.ID)
				if gerr != nil {
					edit = nil
				}
				existingEdits[s.ID] = edit
			}
		}
	} else if cfg, err := core.LoadConfig(target); err == nil {
		for _, s := range core.BuildProviderSummary(cfg) {
			edit, gerr := core.ProviderToEdit(cfg, s.ID)
			if gerr != nil {
				edit = nil
			}
			existingEdits[s.ID] = edit
		}
	}
	imported := []string{}
	skipped := []string{}
	failed := []map[string]interface{}{}
	for _, p := range pkg.Providers {
		_, isExisting := existingEdits[p.ID]
		if isExisting && conflict == "skip" {
			skipped = append(skipped, p.ID)
			continue
		}
		apiKey := p.APIKey
		if apiKey == "" && isExisting && existingEdits[p.ID] != nil {
			// 无密钥备份覆盖本地已有条目时保留现有 Key，与 MergeConfig 的保护同义，
			// 否则 v2 store 会把空 api_key 解释成「编辑器清空了 Key」而静默删除。
			apiKey = existingEdits[p.ID].APIKey
		}
		payload := map[string]interface{}{
			"provider_id": p.ID, "provider_name": p.Name, "base_url": p.BaseURL,
			"api_key": apiKey, "kind": p.Kind, "cards": p.Cards, "merge_models": true,
		}
		if strings.TrimSpace(p.OptionsJSON) != "" {
			// 与 migratePayload 一致：带上非敏感 options，还原时不丢 headers 等键。
			payload["options_json"] = p.OptionsJSON
		}
		// ImportProvider 内部持锁，这里不能再包一层 a.mu。
		resp := a.ImportProvider(payload)
		if ok, _ := resp["success"].(bool); ok {
			imported = append(imported, p.ID)
		} else {
			failed = append(failed, map[string]interface{}{"id": p.ID, "error": core.ShortText(stringOr(resp["error"], "导入失败"), 300)})
		}
	}
	return map[string]interface{}{"success": true, "imported": imported, "skipped": skipped, "failed": failed}
}
