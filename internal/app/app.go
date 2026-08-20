package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"agentprovidermanager/internal/core"
)

type App struct {
	ctx              context.Context
	mu               sync.Mutex
	targetConfigPath string
	currentAgent     string
	agentPaths       map[string]string
	lastDialogPath   string
	settingsPath     string
}

func NewApp() *App {
	migrateLegacyAppData()
	p := core.DefaultConfigPath()
	_ = core.CleanupStaleTmp(p)
	return &App{
		targetConfigPath: p,
		currentAgent:     string(core.AgentZCode),
		agentPaths:       map[string]string{},
		settingsPath:     core.SettingsPath(),
	}
}

func migrateLegacyAppData() {
	newDir := core.AppDataDir()
	if _, err := os.Stat(newDir); err == nil {
		return
	}
	oldDir := core.LegacyAppDataDir()
	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return
	}
	_ = os.MkdirAll(filepath.Dir(newDir), 0755)
	_ = os.Rename(oldDir, newDir)
	if _, err := os.Stat(newDir); os.IsNotExist(err) {
		_ = os.MkdirAll(newDir, 0755)
		entries, _ := os.ReadDir(oldDir)
		for _, e := range entries {
			src := filepath.Join(oldDir, e.Name())
			dst := filepath.Join(newDir, e.Name())
			if _, err := os.Stat(dst); os.IsNotExist(err) {
				data, err := os.ReadFile(src)
				if err == nil {
					_ = os.WriteFile(dst, data, 0644)
				}
			}
		}
	}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.loadAgentState()
}

func (a *App) Shutdown(_ context.Context) {}

func (a *App) DomReady(_ context.Context) {}

func (a *App) loadAgentState() {
	s := a.loadSettings()
	agent := ""
	if v, ok := s["agent"].(string); ok {
		agent = strings.TrimSpace(v)
	}
	if !core.IsValidAgent(agent) {
		agent = string(core.AgentZCode)
	}
	a.currentAgent = agent
	paths := map[string]string{}
	if raw, ok := s["agentPaths"].(map[string]interface{}); ok {
		for k, v := range raw {
			if sv, ok := v.(string); ok && strings.TrimSpace(sv) != "" {
				paths[strings.ToLower(k)] = sv
			}
		}
	}
	a.agentPaths = paths
	if p, ok := paths[a.currentAgent]; ok && strings.TrimSpace(p) != "" {
		a.targetConfigPath = p
	} else {
		a.targetConfigPath = core.AgentDefaultPath(core.NormalizeAgentID(a.currentAgent))
	}
	_ = core.CleanupStaleTmp(a.targetConfigPath)
}

func (a *App) persistAgentStateLocked() {
	s := a.loadSettings()
	s["agent"] = a.currentAgent
	m := map[string]interface{}{}
	for k, v := range a.agentPaths {
		m[k] = v
	}
	s["agentPaths"] = m
	_ = a.saveSettings(s)
}

// loadSettings reads settings.json.
func (a *App) loadSettings() map[string]interface{} {
	data, err := os.ReadFile(a.settingsPath)
	if err != nil {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		_ = os.Rename(a.settingsPath, a.settingsPath+".bak")
		return map[string]interface{}{}
	}
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

func (a *App) saveSettings(m map[string]interface{}) bool {
	dir := filepath.Dir(a.settingsPath)
	_ = os.MkdirAll(dir, 0755)
	data, _ := json.MarshalIndent(m, "", "  ")
	tmp, err := os.CreateTemp(dir, "settings.tmp_*")
	if err != nil {
		return false
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return false
	}
	_ = tmp.Sync()
	tmp.Close()
	return os.Rename(tmpName, a.settingsPath) == nil
}

// GetAppInfo returns app metadata.
func (a *App) GetAppInfo() map[string]interface{} {
	return map[string]interface{}{
		"name":                core.AppName,
		"version":             core.AppVersion,
		"kinds":               core.Kinds,
		"variants":            core.ZCodeVariantOptions,
		"ui_variants_by_kind": core.UIVariantOptionsByKind,
		"agents":              core.AllAgents(),
		"current_agent":       a.currentAgent,
	}
}

func (a *App) ListAgents() map[string]interface{} {
	return map[string]interface{}{
		"success": true,
		"agents":  core.AllAgents(),
		"current": a.currentAgent,
		"paths":   a.agentPaths,
	}
}

func (a *App) GetCurrentAgent() map[string]interface{} {
	return map[string]interface{}{
		"success": true,
		"agent":   a.currentAgent,
		"path":    a.targetConfigPath,
		"paths":   a.agentPaths,
	}
}

func (a *App) SetCurrentAgent(agentID string) map[string]interface{} {
	agentID = strings.TrimSpace(agentID)
	if !core.IsValidAgent(agentID) {
		return map[string]interface{}{"success": false, "error": "不支持的 Agent 类型"}
	}
	norm := string(core.NormalizeAgentID(agentID))
	a.mu.Lock()
	defer a.mu.Unlock()
	a.currentAgent = norm
	if p, ok := a.agentPaths[norm]; ok && strings.TrimSpace(p) != "" {
		a.targetConfigPath = p
	} else {
		a.targetConfigPath = core.AgentDefaultPath(core.AgentID(norm))
		a.agentPaths[norm] = a.targetConfigPath
	}
	_ = core.CleanupStaleTmp(a.targetConfigPath)
	a.persistAgentStateLocked()
	return map[string]interface{}{"success": true, "agent": a.currentAgent, "path": a.targetConfigPath}
}

func (a *App) GetTargetConfig() map[string]interface{} {
	p := a.targetConfigPath
	backups := core.ListBackups(p)
	latest := ""
	if len(backups) > 0 {
		latest = backups[0]
	}
	_, exists := os.Stat(p)
	return map[string]interface{}{
		"path": p, "exists": !os.IsNotExist(exists),
		"backups": backups, "latest_backup": latest,
		"agent": a.currentAgent,
	}
}

func (a *App) ListConfigLocations() map[string]interface{} {
	return map[string]interface{}{
		"success": true, "locations": core.DetectConfigLocationsForAgent(a.currentAgent), "current": a.targetConfigPath, "agent": a.currentAgent,
	}
}

func (a *App) ChooseConfigFile() map[string]interface{} {
	if a.ctx == nil {
		return map[string]interface{}{"success": false, "path": "", "error": "窗口未就绪"}
	}
	initDir := filepath.Dir(a.targetConfigPath)
	if initDir == "" {
		initDir = core.ZCodeDir()
	}
	result, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		DefaultDirectory: initDir,
		Filters: []runtime.FileFilter{{DisplayName: "JSON/YAML 文件", Pattern: "*.json;*.jsonc;*.yaml;*.yml"}, {DisplayName: "所有文件 (*.*)", Pattern: "*.*"}},
	})
	if err != nil {
		return map[string]interface{}{"success": false, "path": "", "error": core.ShortText(err.Error(), 300)}
	}
	if result == "" {
		return map[string]interface{}{"success": false, "path": "", "cancelled": true}
	}
	if _, err := os.Stat(result); err != nil {
		return map[string]interface{}{"success": false, "path": "", "error": "请选择有效的配置文件"}
	}
	if a.currentAgent == string(core.AgentDeepSeek) {
		if _, err := core.DeepSeekLoadConfig(result); err != nil {
			return map[string]interface{}{"success": false, "path": "", "error": err.Error()}
		}
	} else {
		if _, err := core.LoadConfig(result); err != nil {
			return map[string]interface{}{"success": false, "path": "", "error": err.Error()}
		}
	}
	a.mu.Lock()
	a.targetConfigPath = result
	a.lastDialogPath = result
	a.agentPaths[a.currentAgent] = result
	a.persistAgentStateLocked()
	a.mu.Unlock()
	return map[string]interface{}{"success": true, "path": a.targetConfigPath, "agent": a.currentAgent}
}

func (a *App) OpenConfigDir() map[string]interface{} {
	d := filepath.Dir(a.targetConfigPath)
	if d == "" {
		d = core.ZCodeDir()
	}
	if _, err := os.Stat(d); os.IsNotExist(err) {
		_ = os.MkdirAll(d, 0755)
	}
	runtime.BrowserOpenURL(a.ctx, "file:///"+filepath.ToSlash(d))
	return map[string]interface{}{"success": true}
}

func (a *App) SetConfigPath(path string) map[string]interface{} {
	if path == "" {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{"success": false, "error": "配置文件不存在"}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return map[string]interface{}{"success": false, "error": "配置路径格式错误"}
	}
	norm := strings.ToLower(abs)
	detected := map[string]bool{}
	for _, loc := range core.DetectConfigLocationsForAgent(a.currentAgent) {
		if p, err := filepath.Abs(loc.Path); err == nil {
			detected[strings.ToLower(p)] = true
		}
	}
	allowed := detected[norm]
	if !allowed && a.lastDialogPath != "" {
		if d, err := filepath.Abs(a.lastDialogPath); err == nil && strings.ToLower(d) == norm {
			allowed = true
		}
	}
	if !allowed {
		return map[string]interface{}{"success": false, "error": "该路径不在已探测的配置位置中，请通过「浏览」选择配置文件"}
	}
	if a.currentAgent == string(core.AgentDeepSeek) {
		if _, err := core.DeepSeekLoadConfig(path); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
	} else {
		if _, err := core.LoadConfig(path); err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
	}
	a.mu.Lock()
	a.targetConfigPath = path
	a.agentPaths[a.currentAgent] = path
	a.persistAgentStateLocked()
	a.mu.Unlock()
	return map[string]interface{}{"success": true, "path": a.targetConfigPath}
}

func (a *App) GetBackupInfo() map[string]interface{} {
	p := a.targetConfigPath
	core.PruneBackups(p)
	backups := core.ListBackups(p)
	return map[string]interface{}{"has_backup": len(backups) > 0, "backups": backups, "backup_path": firstOr(backups, ""), "target": p}
}

func (a *App) RestoreLastBackup(expectedBackup, expectedTarget string) map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.targetConfigPath
	if expectedTarget != "" {
		if abs1, _ := filepath.Abs(expectedTarget); abs1 != "" {
			if abs2, _ := filepath.Abs(target); strings.ToLower(abs1) != strings.ToLower(abs2) {
				return map[string]interface{}{"success": false, "error": "目标配置已发生变化，请重新确认"}
			}
		}
	}
	core.PruneBackups(target)
	backups := core.ListBackups(target)
	if len(backups) == 0 {
		return map[string]interface{}{"success": false, "error": "没有可用的备份文件"}
	}
	bak := backups[0]
	if expectedBackup != "" {
		allowed := map[string]bool{}
		for _, b := range backups {
			if abs, err := filepath.Abs(b); err == nil {
				allowed[strings.ToLower(abs)] = true
			}
		}
		if abs, err := filepath.Abs(expectedBackup); err == nil && !allowed[strings.ToLower(abs)] {
			return map[string]interface{}{"success": false, "error": "备份列表已发生变化，请重新确认"}
		}
		bak = expectedBackup
	}
	if _, err := os.Stat(bak); os.IsNotExist(err) {
		return map[string]interface{}{"success": false, "error": "备份文件已不存在，请重新确认"}
	}
	snap, err := core.RestoreBackup(target, bak)
	if err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("恢复备份时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	return map[string]interface{}{"success": true, "backup": bak, "target": target, "snapshot": snap}
}

func (a *App) GetTheme() interface{} {
	return a.loadSettings()["theme"]
}

func (a *App) SetTheme(theme string) map[string]interface{} {
	if theme != "light" && theme != "dark" {
		theme = ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	settings := a.loadSettings()
	if theme == "" {
		delete(settings, "theme")
	} else {
		settings["theme"] = theme
	}
	if !a.saveSettings(settings) {
		return map[string]interface{}{"success": false, "error": "主题设置保存失败"}
	}
	return map[string]interface{}{"success": true}
}

func (a *App) GetLanguage() interface{} {
	return a.loadSettings()["language"]
}

func (a *App) SetLanguage(lang string) map[string]interface{} {
	if lang != "zh" && lang != "en" {
		lang = ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	settings := a.loadSettings()
	if lang == "" {
		delete(settings, "language")
	} else {
		settings["language"] = lang
	}
	if !a.saveSettings(settings) {
		return map[string]interface{}{"success": false, "error": "语言设置保存失败"}
	}
	return map[string]interface{}{"success": true}
}

func (a *App) GuessProviderID(baseURL string) interface{} {
	return core.AutoProviderID(baseURL)
}

func (a *App) GuessKind(baseURL string) interface{} {
	return core.InferKind(baseURL)
}

func (a *App) NewProviderID() string { return core.NewProviderID() }

func loadConfigForAgent(path, agent string) (map[string]interface{}, error) {
	if agent == string(core.AgentDeepSeek) {
		return core.DeepSeekLoadConfig(path)
	}
	return core.LoadConfig(path)
}

func loadConfigWithFingerprintForAgent(path, agent string) (map[string]interface{}, string, error) {
	if agent == string(core.AgentDeepSeek) {
		before, _ := core.FileFingerprint(path)
		cfg, err := core.DeepSeekLoadConfig(path)
		if err != nil {
			return nil, "", err
		}
		after, _ := core.FileFingerprint(path)
		if before != after {
			return nil, "", fmt.Errorf("配置文件在读取期间被其他程序修改，请重试")
		}
		return cfg, after, nil
	}
	return core.LoadConfigWithFingerprint(path)
}

func writeConfigForAgent(path string, cfg map[string]interface{}, fingerprint, agent string) error {
	if agent == string(core.AgentDeepSeek) {
		return writeDeepSeekConfigCompat(path, cfg, fingerprint)
	}
	return core.WriteConfig(path, cfg, fingerprint)
}

func writeDeepSeekConfigCompat(path string, cfg map[string]interface{}, fingerprint string) error {
	if cfg == nil {
		cfg = map[string]interface{}{}
	}
	format := "json"
	low := strings.ToLower(path)
	if strings.HasSuffix(low, ".yaml") || strings.HasSuffix(low, ".yml") {
		format = "yaml"
	}
	if format == "yaml" {
		return core.WriteDeepSeekYAML(path, cfg, fingerprint)
	}
	return core.WriteConfig(path, cfg, fingerprint)
}

func providersFromConfig(cfg map[string]interface{}, agent string) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	if agent == string(core.AgentDeepSeek) {
		return core.DeepSeekProvidersMap(cfg)
	}
	if p, ok := cfg["provider"].(map[string]interface{}); ok {
		return p
	}
	return nil
}

func (a *App) ListProviders() map[string]interface{} {
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if agent == string(core.AgentDeepSeek) {
		preview := core.DeepSeekImportPreview(target)
		if preview.Error != "" {
			return map[string]interface{}{"success": false, "error": preview.Error}
		}
		return map[string]interface{}{"success": true, "providers": preview.Providers, "target": target, "agent": agent}
	}
	cfg, err := core.LoadConfig(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "providers": core.BuildProviderSummary(cfg), "target": target, "agent": agent}
}

func (a *App) GetProvider(providerID string) map[string]interface{} {
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if agent == string(core.AgentDeepSeek) {
		cfg, err := core.DeepSeekLoadConfig(target)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		rawMap := core.DeepSeekProvidersMap(cfg)
		raw, ok := rawMap[providerID]
		if !ok {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
		}
		m, _ := raw.(map[string]interface{})
		if m == nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」配置格式错误", providerID)}
		}
		pid, normalized, err := core.ConvertDeepSeekProvider(providerID, m, nil)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		fake := map[string]interface{}{"provider": map[string]interface{}{pid: normalized}}
		pe, err := core.ProviderToEdit(fake, pid)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		return map[string]interface{}{"success": true, "provider": pe, "agent": agent}
	}
	cfg, err := core.LoadConfig(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	provider, err := core.ProviderToEdit(cfg, providerID)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "provider": provider, "agent": agent}
}

func (a *App) SaveProvider(providerID string, provider map[string]interface{}) map[string]interface{} {
	if providerID == "" || provider == nil {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	newID, providerCfg, err := core.BuildProviderCfgEditor(provider)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	count := 0
	if m, ok := providerCfg["models"].(map[string]interface{}); ok {
		count = len(m)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if agent == string(core.AgentDeepSeek) {
		cfg, fingerprint, err := loadConfigWithFingerprintForAgent(target, agent)
		if err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		providers := providersFromConfig(cfg, agent)
		existsDeepSeek := false
		if providers != nil {
			_, existsDeepSeek = providers[newID]
		}
		if existsDeepSeek && newID != providerID {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」已存在", newID)}
		}
		bak, _ := core.BackupConfig(target)
		if newID != providerID && providers != nil {
			if _, exists := providers[providerID]; exists {
				delete(providers, providerID)
			}
		}
		if err := core.DeepSeekSaveProvider(target, newID, providerCfg, fingerprint); err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		latestBak := bak
		if latestBak == "" {
			latestBak = core.FindLatestBackup(target)
		}
		return map[string]interface{}{"success": true, "count": count, "provider_id": newID, "backup": latestBak, "target": target, "agent": agent}
	}
	config, fingerprint, err := core.LoadConfigWithFingerprint(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	core.NormalizeConfigKinds(config)
	providers, _ := config["provider"].(map[string]interface{})
	if providers == nil {
		providers = map[string]interface{}{}
	}
	if _, exists := providers[newID]; exists && newID != providerID {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」已存在", newID)}
	}
	incomingModels, _ := providerCfg["models"].(map[string]interface{})
	for _, key := range []string{newID, providerID} {
		raw, _ := providers[key].(map[string]interface{})
		if raw == nil {
			continue
		}
		em, _ := raw["models"].(map[string]interface{})
		if em != nil && len(em) > 0 && len(incomingModels) == 0 {
			return map[string]interface{}{"success": false, "error": "检测到模型列表为空。为避免误删已有模型，已取消保存；如需清空模型，请逐个删除。"}
		}
	}
	bak, _ := core.BackupConfig(target)
	if newID != providerID {
		if _, exists := providers[providerID]; exists {
			_, _ = core.RenameProviderInConfig(config, providerID, newID)
		} else {
			if _, ok := config["provider"]; !ok {
				config["provider"] = map[string]interface{}{}
			}
			config["provider"].(map[string]interface{})[newID] = map[string]interface{}{}
		}
	}
	var merged map[string]interface{}
	merged, err = core.MergeProviderIntoConfig(config, newID, providerCfg, false)
	if err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	if err := core.WriteConfig(target, merged, fingerprint); err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	latestBak := bak
	if latestBak == "" {
		latestBak = core.FindLatestBackup(target)
	}
	return map[string]interface{}{"success": true, "count": count, "provider_id": newID, "backup": latestBak, "target": target, "agent": agent}
}

func (a *App) DeleteProvider(providerID string) map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if agent == string(core.AgentDeepSeek) {
		cfg, fingerprint, err := loadConfigWithFingerprintForAgent(target, agent)
		if err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除提供商时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		providers := providersFromConfig(cfg, agent)
		if providers == nil || providers[providerID] == nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
		}
		bak, _ := core.BackupConfig(target)
		delete(providers, providerID)
		if err := writeConfigForAgent(target, cfg, fingerprint, agent); err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除提供商时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		latestBak := bak
		if latestBak == "" {
			latestBak = core.FindLatestBackup(target)
		}
		return map[string]interface{}{"success": true, "provider_id": providerID, "backup": latestBak, "target": target}
	}
	config, fingerprint, err := core.LoadConfigWithFingerprint(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除提供商时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	core.NormalizeConfigKinds(config)
	providers, _ := config["provider"].(map[string]interface{})
	if providers == nil || providers[providerID] == nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
	}
	bak, _ := core.BackupConfig(target)
	delete(providers, providerID)
	if err := core.WriteConfig(target, config, fingerprint); err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除提供商时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	latestBak := bak
	if latestBak == "" {
		latestBak = core.FindLatestBackup(target)
	}
	return map[string]interface{}{"success": true, "provider_id": providerID, "backup": latestBak, "target": target}
}

func (a *App) DeleteModel(providerID, modelID string) map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if agent == string(core.AgentDeepSeek) {
		cfg, fingerprint, err := loadConfigWithFingerprintForAgent(target, agent)
		if err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除模型时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		providers := providersFromConfig(cfg, agent)
		if providers == nil || providers[providerID] == nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
		}
		prov, _ := providers[providerID].(map[string]interface{})
		if arr, ok := prov["models"].([]interface{}); ok {
			newArr := []interface{}{}
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
				return map[string]interface{}{"success": false, "error": fmt.Sprintf("模型「%s」不存在", modelID)}
			}
			bak, _ := core.BackupConfig(target)
			prov["models"] = newArr
			if err := writeConfigForAgent(target, cfg, fingerprint, agent); err != nil {
				return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除模型时出错：\n%s", core.ShortText(err.Error(), 300))}
			}
			latestBak := bak
			if latestBak == "" {
				latestBak = core.FindLatestBackup(target)
			}
			return map[string]interface{}{"success": true, "provider_id": providerID, "model_id": modelID, "backup": latestBak, "target": target}
		}
		models, _ := prov["models"].(map[string]interface{})
		if models == nil || models[modelID] == nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("模型「%s」不存在", modelID)}
		}
		bak, _ := core.BackupConfig(target)
		delete(models, modelID)
		if err := writeConfigForAgent(target, cfg, fingerprint, agent); err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除模型时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		latestBak := bak
		if latestBak == "" {
			latestBak = core.FindLatestBackup(target)
		}
		return map[string]interface{}{"success": true, "provider_id": providerID, "model_id": modelID, "backup": latestBak, "target": target}
	}
	config, fingerprint, err := core.LoadConfigWithFingerprint(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除模型时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	core.NormalizeConfigKinds(config)
	providers, _ := config["provider"].(map[string]interface{})
	if providers == nil || providers[providerID] == nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
	}
	prov, _ := providers[providerID].(map[string]interface{})
	models, _ := prov["models"].(map[string]interface{})
	if models == nil || models[modelID] == nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("模型「%s」不存在", modelID)}
	}
	bak, _ := core.BackupConfig(target)
	delete(models, modelID)
	if err := core.WriteConfig(target, config, fingerprint); err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除模型时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	latestBak := bak
	if latestBak == "" {
		latestBak = core.FindLatestBackup(target)
	}
	return map[string]interface{}{"success": true, "provider_id": providerID, "model_id": modelID, "backup": latestBak, "target": target}
}

func (a *App) FetchModels(baseURL, apiKey string) map[string]interface{} {
	raw, err := core.FetchModelsRaw(baseURL, apiKey, 40*1e9)
	if err != nil {
		if mfe, ok := err.(*core.ModelFetchError); ok {
			return map[string]interface{}{"success": false, "error": mfe.Msg, "error_code": mfe.ErrorCode}
		}
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300), "error_code": "OTHER"}
	}
	cards := core.BuildModelCards(raw)
	if len(cards) == 0 {
		return map[string]interface{}{"success": false, "error": "API 返回了空模型列表"}
	}
	return map[string]interface{}{"success": true, "models": cards, "count": len(cards)}
}

func (a *App) RefreshProviderModels(providerID, baseURLOverride, apiKeyOverride string) map[string]interface{} {
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if agent == string(core.AgentDeepSeek) {
		cfg, err := core.DeepSeekLoadConfig(target)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		providers := providersFromConfig(cfg, agent)
		raw, ok := providers[providerID]
		if !ok {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
		}
		m, _ := raw.(map[string]interface{})
		baseURL := strings.TrimSpace(baseURLOverride)
		if baseURL == "" {
			baseURL, _ = m["baseURL"].(string)
		}
		apiKey := strings.TrimSpace(apiKeyOverride)
		if apiKey == "" {
			apiKey, _ = m["apiKey"].(string)
			if apiKey == "" {
				if envName, ok := m["apiKeyEnv"].(string); ok && envName != "" {
					creds := core.LoadCredentialsForPreview()
					if v, ok := creds[envName]; ok {
						apiKey = v
					} else {
						apiKey = os.Getenv(envName)
					}
				}
			}
		}
		if baseURL == "" {
			return map[string]interface{}{"success": false, "error": "该提供商未配置 Base URL，请先在管理页填写"}
		}
		return a.FetchModels(baseURL, apiKey)
	}
	cfg, err := core.LoadConfig(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	provider, err := core.ProviderToEdit(cfg, providerID)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	baseURL := strings.TrimSpace(baseURLOverride)
	if baseURL == "" {
		baseURL = provider.BaseURL
	}
	apiKey := strings.TrimSpace(apiKeyOverride)
	if apiKey == "" {
		apiKey = provider.APIKey
	}
	if baseURL == "" {
		return map[string]interface{}{"success": false, "error": "该提供商未配置 Base URL，请先在管理页填写"}
	}
	return a.FetchModels(baseURL, apiKey)
}

func (a *App) BuildSingleCard(modelID string) map[string]interface{} {
	card, err := core.BuildSingleCard(modelID)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "card": card}
}

func (a *App) ImportProvider(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	mergeModels := true
	if v, ok := payload["merge_models"].(bool); ok {
		mergeModels = v
	}
	providerID, _ := payload["provider_id"].(string)
	providerName, _ := payload["provider_name"].(string)
	baseURL, _ := payload["base_url"].(string)
	apiKey, _ := payload["api_key"].(string)
	kind, _ := payload["kind"].(string)
	if kind == "" {
		kind = "openai-compatible"
	}
	var cards []core.ModelCard
	if raw, ok := payload["cards"]; ok {
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &cards)
	}
	pid, providerCfg, _, err := core.BuildProviderCfg(providerID, providerName, baseURL, apiKey, cards, kind)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if expectedTarget, ok := payload["target_path"].(string); ok && expectedTarget != "" {
		if abs1, _ := filepath.Abs(expectedTarget); abs1 != "" {
			if abs2, _ := filepath.Abs(target); strings.ToLower(abs1) != strings.ToLower(abs2) {
				return map[string]interface{}{"success": false, "error": "目标配置已发生变化，请确认后重试"}
			}
		}
	}
	if agent == string(core.AgentDeepSeek) {
		cfg, fingerprint, err := loadConfigWithFingerprintForAgent(target, agent)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		if !mergeModels {
			providers := providersFromConfig(cfg, agent)
			if providers != nil {
				if _, exists := providers[pid]; exists {
					if incoming, _ := providerCfg["models"].(map[string]interface{}); len(incoming) == 0 {
						if existingRaw, ok := providers[pid].(map[string]interface{}); ok {
							hasExistingModels := false
							if arr, ok := existingRaw["models"].([]interface{}); ok && len(arr) > 0 {
								hasExistingModels = true
							} else if mp, ok := existingRaw["models"].(map[string]interface{}); ok && len(mp) > 0 {
								hasExistingModels = true
							}
							if hasExistingModels {
								return map[string]interface{}{"success": false, "error": "检测到模型列表为空。为避免误删已有模型，已取消导入；如需清空模型，请逐个删除。"}
							}
						}
					}
				}
			}
		}
		bak, _ := core.BackupConfig(target)
		if err := core.DeepSeekSaveProvider(target, pid, providerCfg, fingerprint); err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		latestBak := bak
		if latestBak == "" {
			latestBak = core.FindLatestBackup(target)
		}
		count := 0
		if m, ok := providerCfg["models"].(map[string]interface{}); ok {
			count = len(m)
		}
		return map[string]interface{}{"success": true, "count": count, "provider_id": pid, "target": target, "backup": latestBak, "latest_backup": latestBak, "agent": agent}
	}
	if !mergeModels {
		if cfg, _ := core.LoadConfig(target); cfg != nil {
			if providers, ok := cfg["provider"].(map[string]interface{}); ok {
				if raw, ok := providers[pid].(map[string]interface{}); ok {
					if em, ok := raw["models"].(map[string]interface{}); ok && len(em) > 0 {
						if incoming, _ := providerCfg["models"].(map[string]interface{}); len(incoming) == 0 {
							return map[string]interface{}{"success": false, "error": "检测到模型列表为空。为避免误删已有模型，已取消导入；如需清空模型，请逐个删除。"}
						}
					}
				}
			}
		}
	}
	existing, fingerprint, err := core.LoadConfigWithFingerprint(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	core.NormalizeConfigKinds(existing)
	bak, _ := core.BackupConfig(target)
	merged, err := core.MergeProviderIntoConfig(existing, pid, providerCfg, mergeModels)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	if err := core.WriteConfig(target, merged, fingerprint); err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	latestBak := bak
	if latestBak == "" {
		latestBak = core.FindLatestBackup(target)
	}
	count := 0
	if m, ok := providerCfg["models"].(map[string]interface{}); ok {
		count = len(m)
	}
	return map[string]interface{}{"success": true, "count": count, "provider_id": pid, "target": target, "backup": latestBak, "latest_backup": latestBak, "agent": agent}
}

// Opencode import
func (a *App) PreviewOpencodeImport(path string) map[string]interface{} {
	if strings.TrimSpace(path) == "" {
		path = core.OpencodeConfig()
	}
	item := core.OpencodeImportPreview(path)
	return map[string]interface{}{"success": true, "path": item.Path, "exists": item.Exists, "error": item.Error, "providers": item.Providers}
}

func (a *App) ChooseOpencodeFile() map[string]interface{} {
	if a.ctx == nil {
		return map[string]interface{}{"success": false, "path": "", "error": "窗口未就绪"}
	}
	result, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "JSON 文件 (*.json;*.jsonc)", Pattern: "*.json;*.jsonc"}, {DisplayName: "所有文件 (*.*)", Pattern: "*.*"}},
	})
	if err != nil {
		return map[string]interface{}{"success": false, "path": "", "error": core.ShortText(err.Error(), 300)}
	}
	if result == "" {
		return map[string]interface{}{"success": false, "path": "", "cancelled": true}
	}
	if _, err := core.LoadConfig(result); err != nil {
		return map[string]interface{}{"success": false, "path": "", "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "path": result}
}

func (a *App) ImportOpencode(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	path, _ := payload["path"].(string)
	if strings.TrimSpace(path) == "" {
		path = core.OpencodeConfig()
	}
	selected, _ := payload["selected_ids"].([]interface{})
	var selectedIDs []string
	for _, v := range selected {
		if s, ok := v.(string); ok {
			selectedIDs = append(selectedIDs, s)
		}
	}
	merge := true
	if v, ok := payload["merge"].(bool); ok {
		merge = v
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{"success": false, "error": "opencode 配置文件不存在"}
	}
	opencodeCfg, err := core.LoadConfig(path)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if expectedTarget, ok := payload["target_path"].(string); ok && expectedTarget != "" {
		if abs1, _ := filepath.Abs(expectedTarget); abs1 != "" {
			if abs2, _ := filepath.Abs(target); strings.ToLower(abs1) != strings.ToLower(abs2) {
				return map[string]interface{}{"success": false, "error": "目标配置已发生变化，请确认后重试"}
			}
		}
	}
	if agent == string(core.AgentDeepSeek) {
		return map[string]interface{}{"success": false, "error": "当前 Agent 为 DeepSeek，请直接在管理页保存提供商"}
	}
	existing, fingerprint, err := core.LoadConfigWithFingerprint(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	core.NormalizeConfigKinds(existing)
	result, imported, merged, err := core.ImportOpencodeProviders(existing, opencodeCfg, selectedIDs, merge)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	bak, _ := core.BackupConfig(target)
	if err := core.WriteConfig(target, result, fingerprint); err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	latestBak := bak
	if latestBak == "" {
		latestBak = core.FindLatestBackup(target)
	}
	return map[string]interface{}{"success": true, "imported": imported, "merged": merged, "target": target, "backup": latestBak, "latest_backup": latestBak}
}

// Config merge
func (a *App) PreviewConfigMerge(path string) map[string]interface{} {
	if strings.TrimSpace(path) == "" {
		return map[string]interface{}{"success": false, "error": "请先选择要合并的配置文件"}
	}
	item := core.ZCodeConfigMergePreview(path)
	return map[string]interface{}{"success": true, "path": item.Path, "exists": item.Exists, "error": item.Error, "providers": item.Providers}
}

func (a *App) ChooseMergeFile() map[string]interface{} {
	if a.ctx == nil {
		return map[string]interface{}{"success": false, "path": "", "error": "窗口未就绪"}
	}
	result, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "JSON 文件 (*.json;*.jsonc)", Pattern: "*.json;*.jsonc"}, {DisplayName: "所有文件 (*.*)", Pattern: "*.*"}},
	})
	if err != nil {
		return map[string]interface{}{"success": false, "path": "", "error": core.ShortText(err.Error(), 300)}
	}
	if result == "" {
		return map[string]interface{}{"success": false, "path": "", "cancelled": true}
	}
	if _, err := core.LoadConfig(result); err != nil {
		return map[string]interface{}{"success": false, "path": "", "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "path": result}
}

func (a *App) MergeConfig(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	path, _ := payload["path"].(string)
	if strings.TrimSpace(path) == "" {
		return map[string]interface{}{"success": false, "error": "请先选择要合并的配置文件"}
	}
	selected, _ := payload["selected_ids"].([]interface{})
	var selectedIDs []string
	for _, v := range selected {
		if s, ok := v.(string); ok {
			selectedIDs = append(selectedIDs, s)
		}
	}
	merge := true
	if v, ok := payload["merge"].(bool); ok {
		merge = v
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{"success": false, "error": "源配置文件不存在"}
	}
	sourceCfg, err := core.LoadConfig(path)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.targetConfigPath
	agent := a.currentAgent
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	if expectedTarget, ok := payload["target_path"].(string); ok && expectedTarget != "" {
		if abs1, _ := filepath.Abs(expectedTarget); abs1 != "" {
			if abs2, _ := filepath.Abs(target); strings.ToLower(abs1) != strings.ToLower(abs2) {
				return map[string]interface{}{"success": false, "error": "目标配置已发生变化，请确认后重试"}
			}
		}
	}
	if agent == string(core.AgentDeepSeek) {
		return map[string]interface{}{"success": false, "error": "当前 Agent 为 DeepSeek，暂不支持合并配置文件"}
	}
	existing, fingerprint, err := core.LoadConfigWithFingerprint(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	core.NormalizeConfigKinds(existing)
	result, imported, merged, err := core.ImportZCodeProviders(existing, sourceCfg, selectedIDs, merge)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	bak, _ := core.BackupConfig(target)
	if err := core.WriteConfig(target, result, fingerprint); err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	latestBak := bak
	if latestBak == "" {
		latestBak = core.FindLatestBackup(target)
	}
	return map[string]interface{}{"success": true, "imported": imported, "merged": merged, "target": target, "backup": latestBak, "latest_backup": latestBak}
}

// Migrate
func (a *App) MigratePreview(sourceAgent, targetAgent string) map[string]interface{} {
	preview := core.MigratePreviewForAgents(sourceAgent, targetAgent)
	return map[string]interface{}{"success": true, "preview": preview}
}

func (a *App) MigrateExecute(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	source, _ := payload["source"].(string)
	target, _ := payload["target"].(string)
	mode, _ := payload["mode"].(string)
	var selectedIDs []string
	if raw, ok := payload["selected_ids"]; ok {
		switch v := raw.(type) {
		case []string:
			selectedIDs = v
		case []interface{}:
			for _, e := range v {
				if s, ok := e.(string); ok {
					selectedIDs = append(selectedIDs, s)
				}
			}
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	result, err := core.MigrateExecute(source, target, selectedIDs, mode)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 400)}
	}
	return result
}

// Keychain
func (a *App) ListKeychain() map[string]interface{} {
	entries, err := core.LoadKeychain("")
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	var meta []map[string]interface{}
	for _, e := range entries {
		hint := "••••••••"
		if len(e.APIKey) > 8 {
			hint = "••••••••" + e.APIKey[len(e.APIKey)-4:]
		}
		meta = append(meta, map[string]interface{}{"id": e.ID, "name": e.Name, "note": e.Note, "created_at": e.CreatedAt, "updated_at": e.UpdatedAt, "key_hint": hint})
	}
	if meta == nil {
		meta = []map[string]interface{}{}
	}
	return map[string]interface{}{"success": true, "entries": meta, "path": core.KeychainPath()}
}

func (a *App) GetKeychainEntry(entryID string) map[string]interface{} {
	entry := core.GetKeychainEntry("", entryID)
	if entry == nil {
		return map[string]interface{}{"success": false, "error": "钥匙串条目不存在，请刷新后重试"}
	}
	return map[string]interface{}{"success": true, "entry": entry}
}

func (a *App) SaveKeychainEntry(entry map[string]interface{}) map[string]interface{} {
	if entry == nil {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	ke := core.KeychainEntry{
		ID:     stringOr(entry["id"], ""),
		Name:   stringOr(entry["name"], ""),
		APIKey: stringOr(entry["api_key"], stringOr(entry["apiKey"], "")),
		Note:   stringOr(entry["note"], ""),
	}
	if ke.APIKey == "" {
		if v, ok := entry["api_key"].(string); ok {
			ke.APIKey = v
		}
	}
	ok, msg, saved := core.UpsertKeychainEntry("", ke)
	if !ok {
		return map[string]interface{}{"success": false, "error": msg}
	}
	return map[string]interface{}{"success": true, "entry": map[string]interface{}{"id": saved.ID, "name": saved.Name, "note": saved.Note, "created_at": saved.CreatedAt, "updated_at": saved.UpdatedAt}}
}

func (a *App) DeleteKeychainEntries(ids []interface{}) map[string]interface{} {
	var strIDs []string
	for _, v := range ids {
		if s, ok := v.(string); ok {
			strIDs = append(strIDs, s)
		}
	}
	ok, msg, deleted, _ := core.DeleteKeychainEntries("", strIDs)
	if !ok {
		return map[string]interface{}{"success": false, "error": msg}
	}
	return map[string]interface{}{"success": true, "deleted": deleted}
}

func firstOr(arr []string, fallback string) string {
	if len(arr) > 0 {
		return arr[0]
	}
	return fallback
}

func stringOr(v interface{}, fallback string) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fallback
}
