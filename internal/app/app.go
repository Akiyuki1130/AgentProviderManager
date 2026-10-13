package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"agentprovidermanager/internal/core"
	appupdate "agentprovidermanager/internal/update"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx               context.Context
	mu                sync.Mutex
	updateMu          sync.Mutex
	updateInfo        appupdate.UpdateInfo
	updateDownloading bool
	targetConfigPath  string
	currentAgent      string
	agentPaths        map[string]string
	lastDialogPath    string
	settingsPath      string
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
	core.SetHTTPAllowed(a.httpEnabled())
	core.SetPrivateHostsAllowed(a.privateHostsAllowed())
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
	// 旧版 -> 新版只在这里升级一次：保存的路径仍指向旧版文件（ZCode config.json /
	// DSH settings.yaml）而新版/0.2 文件确实存在时，改为新版路径并持久化。
	//
	// 运行期间用户显式选择的文件（SetConfigPath / ChooseConfigFile 直接写 agentPaths）
	// 不会被覆盖，因此这里也只处理“启动时读到的历史路径”。两个 Agent 都升级，
	// 而不是只升级当前 Agent，否则跨 Agent 迁移仍可能写进 DSH 0.2 根本不读的 settings.yaml。
	updated := false
	for _, agentID := range []core.AgentID{core.AgentZCode, core.AgentDeepSeek} {
		key := agentID.String()
		if preferred := preferModernConfigPath(key, paths[key]); preferred != paths[key] {
			paths[key] = preferred
			updated = true
		}
	}
	if updated {
		a.agentPaths = paths
		a.persistAgentStateLocked()
	}
	if p, ok := paths[a.currentAgent]; ok && strings.TrimSpace(p) != "" {
		a.targetConfigPath = p
	} else {
		// 新版/0.2 优先只在这里生效（app 层）；core 的 AgentDefaultPath 仍是旧版语义。
		a.targetConfigPath = defaultPathForAgent(a.currentAgent)
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
// preferModernConfigPath 在保存的路径仍是旧版文件、而新版/0.2 文件确实存在时返回新版路径。
//
// 三个调用点（启动 loadAgentState、切换 Agent SetCurrentAgent、跨 Agent 迁移）共用它：
// 任何一处漏掉都会让写入落到 DSH 0.2 根本不读的 settings.yaml，或 ZCode 已经不用的 config.json。
func preferModernConfigPath(agentID, stored string) string {
	switch core.NormalizeAgentID(agentID) {
	case core.AgentZCode:
		if !core.IsZCodeV2Path(stored) && core.IsZCodeV2Path(core.ZCodeProviderConfigPath()) {
			return core.ZCodeProviderConfigPath()
		}
	case core.AgentDeepSeek:
		if !core.IsDeepSeekProfilePath(stored) {
			if preferred := core.DeepSeekDefaultPath(); core.IsDeepSeekProfilePath(preferred) {
				return preferred
			}
		}
	}
	return stored
}

// defaultPathForAgent 返回 App 层应使用的默认目标路径。
//
// core.AgentDefaultPath 保持旧版语义（ZCode -> config.json、DSH -> settings.yaml），
// App 层必须优先新版：ZCode 新版 provider_config.json、DeepSeek Harness 0.2 profile patch。
// loadAgentState 与 SetCurrentAgent 共用它，避免“切换 Agent 后回落到旧版文件”。
func defaultPathForAgent(agentID string) string {
	switch core.NormalizeAgentID(agentID) {
	case core.AgentZCode:
		return core.ZCodeDefaultPath()
	case core.AgentDeepSeek:
		return core.DeepSeekDefaultPath()
	default:
		return core.AgentDefaultPath(core.NormalizeAgentID(agentID))
	}
}

// agentTargetPathLocked 返回某个 Agent 当前应使用的目标路径。调用方需持有 a.mu。
//
// 已保存的路径优先；没有保存过时用 App 层的默认路径（新版/0.2 优先）。
// 旧版路径的升级只发生在 loadAgentState（启动时），运行期间用户显式选择的文件不会被改写。
func (a *App) agentTargetPathLocked(agentID string) string {
	norm := core.NormalizeAgentID(agentID)
	if p, ok := a.agentPaths[norm.String()]; ok && strings.TrimSpace(p) != "" {
		return preferModernConfigPath(norm.String(), p)
	}
	return defaultPathForAgent(norm.String())
}
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

func (a *App) updateStatusLocked() map[string]interface{} {
	status := map[string]interface{}{
		"current_version":   core.AppVersion,
		"latest_version":    "",
		"available":         false,
		"downloading":       a.updateDownloading,
		"download_progress": 0,
		"ready":             false,
		"error":             "",
	}
	if a.updateInfo.Version != "" {
		status["latest_version"] = a.updateInfo.Version
		status["available"] = a.updateInfo.IsUpdate
		status["ready"] = a.updateInfo.StagedPath != ""
		if a.updateInfo.StagedPath != "" {
			status["download_progress"] = 100
		}
	}
	return status
}

func updateError(err error) string {
	if err == nil {
		return ""
	}
	return core.ShortText(err.Error(), 500)
}

func (a *App) CheckForUpdate() map[string]interface{} {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	info, err := appupdate.New(nil).CheckLatest(ctx, core.AppVersion)
	if err != nil {
		status := a.updateStatusLocked()
		status["error"] = updateError(err)
		return status
	}
	previous := a.updateInfo
	if previous.StagedPath != "" && previous.Version == info.Version && previous.ExpectedSHA256 == info.ExpectedSHA256 {
		info.StagedPath = previous.StagedPath
		info.ExecutablePath = previous.ExecutablePath
		info.Bytes = previous.Bytes
	} else if previous.StagedPath != "" && previous.StagedPath != info.StagedPath {
		_ = os.Remove(previous.StagedPath)
	}
	a.updateInfo = info
	return a.updateStatusLocked()
}

func (a *App) DownloadUpdate() map[string]interface{} {
	a.updateMu.Lock()
	if a.updateDownloading {
		status := a.updateStatusLocked()
		a.updateMu.Unlock()
		return status
	}
	a.updateDownloading = true
	a.updateMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	path, err := os.Executable()
	if err == nil {
		info, downloadErr := appupdate.New(nil).DownloadLatest(ctx, core.AppVersion, func(downloaded, total int64) {
			progress := 0
			if total > 0 {
				progress = int(float64(downloaded) / float64(total) * 100)
			}
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "update:progress", map[string]interface{}{"progress": progress, "downloaded": downloaded, "total": total, "downloading": true})
			}
		})
		a.updateMu.Lock()
		a.updateDownloading = false
		if downloadErr == nil {
			info.ExecutablePath = path
			a.updateInfo = info
		}
		status := a.updateStatusLocked()
		if downloadErr != nil {
			status["error"] = updateError(downloadErr)
		}
		a.updateMu.Unlock()
		if downloadErr == nil && a.ctx != nil {
			runtime.EventsEmit(a.ctx, "update:progress", map[string]interface{}{"progress": 100, "ready": true, "downloading": false})
		}
		return status
	}
	a.updateMu.Lock()
	a.updateDownloading = false
	status := a.updateStatusLocked()
	status["error"] = updateError(err)
	a.updateMu.Unlock()
	return status
}

func (a *App) InstallUpdate() map[string]interface{} {
	a.updateMu.Lock()
	info := a.updateInfo
	a.updateMu.Unlock()
	if info.StagedPath == "" || info.ExpectedSHA256 == "" {
		return map[string]interface{}{"success": false, "error": "没有已下载的更新"}
	}
	if err := appupdate.StartCurrentProcessHelper(info.StagedPath, info.ExpectedSHA256); err != nil {
		return map[string]interface{}{"success": false, "error": updateError(err)}
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
	return map[string]interface{}{"success": true, "accepted": true}
}

func (a *App) GetUpdateStatus() map[string]interface{} {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	return a.updateStatusLocked()
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
		// 保存的仍是旧版文件（config.json / settings.yaml）时改用新版/0.2 文件：
		// 否则切换回来会编辑一个 Agent 实际已经不读的文件。
		a.targetConfigPath = preferModernConfigPath(norm, p)
	} else {
		a.targetConfigPath = defaultPathForAgent(norm)
	}
	a.agentPaths[norm] = a.targetConfigPath
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
		Filters:          []runtime.FileFilter{{DisplayName: "JSON/YAML 文件", Pattern: "*.json;*.jsonc;*.yaml;*.yml"}, {DisplayName: "所有文件 (*.*)", Pattern: "*.*"}},
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
	if _, err := loadConfigForAgent(result, a.currentAgent); err != nil {
		return map[string]interface{}{"success": false, "path": "", "error": err.Error()}
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
	if _, err := loadConfigForAgent(path, a.currentAgent); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	a.mu.Lock()
	a.targetConfigPath = path
	a.agentPaths[a.currentAgent] = path
	a.persistAgentStateLocked()
	a.mu.Unlock()
	return map[string]interface{}{"success": true, "path": a.targetConfigPath}
}

// GetRestorePoints 返回全部还原点（按时间倒序），以及当前目标与存储目录。
// 每个还原点自带 target_path / format / operation，前端按需分组或过滤。
func (a *App) GetRestorePoints() map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	core.PruneRestorePoints()
	points := core.ListRestorePoints(core.RestorePointFilter{})
	if points == nil {
		points = []core.RestorePoint{}
	}
	// OpenCode 入口已隐藏（AllAgents 不再包含它），但历史还原点仍应显示可读的 Agent 名。
	for i := range points {
		if strings.EqualFold(points[i].AgentID, string(core.AgentOpenCode)) && strings.TrimSpace(points[i].AgentLabel) != "OpenCode" {
			points[i].AgentLabel = "OpenCode"
		}
	}
	return map[string]interface{}{
		"success": true,
		"points":  points,
		"target":  a.targetConfigPath,
		"agent":   a.currentAgent,
		"dir":     core.RestorePointsDir(),
	}
}

// RestoreRestorePoint 把某个还原点写回它的原始路径。
// 回滚前会先给当前状态建一个还原点（返回值的 snapshot），因此回滚本身可撤销。
func (a *App) RestoreRestorePoint(id string) map[string]interface{} {
	if strings.TrimSpace(id) == "" {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	outcome, err := core.RestoreRestorePoint(id)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	// 回滚的正是当前 Agent 已知的配置位置时，把目标指回该文件（格式可能由 legacy 变回 v2）。
	switched := false
	if a.restorePointPathKnownLocked(outcome.TargetPath) {
		a.targetConfigPath = outcome.TargetPath
		a.agentPaths[a.currentAgent] = outcome.TargetPath
		a.persistAgentStateLocked()
		switched = true
	}
	return map[string]interface{}{
		"success":         true,
		"id":              outcome.ID,
		"target":          outcome.TargetPath,
		"removed_path":    outcome.RemovedPath,
		"snapshot":        outcome.Snapshot,
		"warning":         outcome.Warning,
		"target_switched": switched,
		"current_target":  a.targetConfigPath,
	}
}

// DeleteRestorePoint 删除一个还原点。
func (a *App) DeleteRestorePoint(id string) map[string]interface{} {
	if strings.TrimSpace(id) == "" {
		return map[string]interface{}{"success": false, "error": "参数格式错误"}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := core.DeleteRestorePoint(id); err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	return map[string]interface{}{"success": true, "id": id}
}

// PruneRestorePoints 手动触发一次保留策略清理。
func (a *App) PruneRestorePoints() map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	removed := core.PruneRestorePoints()
	points := core.ListRestorePoints(core.RestorePointFilter{})
	if points == nil {
		points = []core.RestorePoint{}
	}
	return map[string]interface{}{"success": true, "removed": removed, "points": points}
}

// OpenRestorePointsDir 在文件管理器里打开还原点目录。
func (a *App) OpenRestorePointsDir() map[string]interface{} {
	dir := core.RestorePointsDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	if a.ctx == nil {
		return map[string]interface{}{"success": false, "error": "窗口未就绪"}
	}
	runtime.BrowserOpenURL(a.ctx, "file:///"+filepath.ToSlash(dir))
	return map[string]interface{}{"success": true, "dir": dir}
}

// restorePointPathKnownLocked 判断某路径是否属于当前 Agent 的已探测配置位置。
// 调用方必须已持有 a.mu。
func (a *App) restorePointPathKnownLocked(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, loc := range core.DetectConfigLocationsForAgent(a.currentAgent) {
		p, err := filepath.Abs(loc.Path)
		if err == nil && strings.EqualFold(p, target) {
			return true
		}
	}
	return false
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
	if theme != "light" && theme != "dark" && theme != "system" {
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
	if lang != "zh" && lang != "en" && lang != "ja" && lang != "system" {
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

func (a *App) GetAccent() interface{} {
	return a.loadSettings()["accent"]
}

func (a *App) SetAccent(accent string) map[string]interface{} {
	accent = strings.TrimSpace(accent)
	if accent != "" && !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(accent) {
		return map[string]interface{}{"success": false, "error": "主题颜色格式无效"}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	settings := a.loadSettings()
	if accent == "" {
		delete(settings, "accent")
	} else {
		settings["accent"] = accent
	}
	if !a.saveSettings(settings) {
		return map[string]interface{}{"success": false, "error": "主题颜色保存失败"}
	}
	return map[string]interface{}{"success": true}
}

func (a *App) GetHttpEnabled() interface{} {
	on, _ := a.loadSettings()["http_enabled"].(bool)
	return on
}

func (a *App) SetHttpEnabled(v bool) map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	settings := a.loadSettings()
	settings["http_enabled"] = v
	if !a.saveSettings(settings) {
		return map[string]interface{}{"success": false, "error": "HTTP 支持设置保存失败"}
	}
	core.SetHTTPAllowed(v)
	return map[string]interface{}{"success": true}
}

func (a *App) GetPrivateHostsAllowed() interface{} {
	on, _ := a.loadSettings()["private_hosts_enabled"].(bool)
	return on
}

func (a *App) SetPrivateHostsAllowed(v bool) map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	settings := a.loadSettings()
	settings["private_hosts_enabled"] = v
	if !a.saveSettings(settings) {
		return map[string]interface{}{"success": false, "error": "允许本地与内网地址设置保存失败"}
	}
	core.SetPrivateHostsAllowed(v)
	return map[string]interface{}{"success": true}
}

func (a *App) GetAutoFillLimits() interface{} {
	s := a.loadSettings()
	if v, ok := s["auto_fill_limits"].(bool); ok {
		return v
	}
	return true
}

func (a *App) SetAutoFillLimits(v bool) map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	settings := a.loadSettings()
	settings["auto_fill_limits"] = v
	if !a.saveSettings(settings) {
		return map[string]interface{}{"success": false, "error": "自动补全设置保存失败"}
	}
	return map[string]interface{}{"success": true}
}

// GetOptions returns every persisted user option in one call.
func (a *App) GetOptions() map[string]interface{} {
	s := a.loadSettings()
	theme, _ := s["theme"].(string)
	lang, _ := s["language"].(string)
	accent, _ := s["accent"].(string)
	httpOn, _ := s["http_enabled"].(bool)
	privateOn, _ := s["private_hosts_enabled"].(bool)
	autoFill := true
	if v, ok := s["auto_fill_limits"].(bool); ok {
		autoFill = v
	}
	return map[string]interface{}{
		"theme":                 theme,
		"language":              lang,
		"accent":                accent,
		"http_enabled":          httpOn,
		"private_hosts_enabled": privateOn,
		"auto_fill_limits":      autoFill,
	}
}

// httpEnabled reports the persisted http:// policy (default off).
func (a *App) httpEnabled() bool {
	on, _ := a.loadSettings()["http_enabled"].(bool)
	return on
}

// privateHostsAllowed reports the persisted local/private-address policy
// (default off).
func (a *App) privateHostsAllowed() bool {
	on, _ := a.loadSettings()["private_hosts_enabled"].(bool)
	return on
}

// autoFillLimits reports the persisted auto-fill policy (default on).
func (a *App) autoFillLimits() bool {
	s := a.loadSettings()
	if v, ok := s["auto_fill_limits"].(bool); ok {
		return v
	}
	return true
}

func (a *App) GuessProviderID(baseURL string) interface{} {
	return core.AutoProviderID(baseURL)
}

func (a *App) GuessKind(baseURL string) interface{} {
	return core.InferKind(baseURL)
}

func (a *App) NewProviderID() string { return core.NewProviderID() }

// loadConfigForAgent / loadConfigWithFingerprintForAgent / writeConfigForAgent 是
// app 层读写配置的统一入口：注册了 config backend 的 agent（ZCode、DeepSeek）
// 一律走 backend 分发，未注册的 agent（OpenCode）回落到通用 JSON 路径。
func loadConfigForAgent(path, agent string) (map[string]interface{}, error) {
	if backend, err := core.GetBackend(agent, path); err == nil {
		return backend.Read(path)
	}
	return core.LoadConfig(path)
}

func loadConfigWithFingerprintForAgent(path, agent string) (map[string]interface{}, string, error) {
	if backend, err := core.GetBackend(agent, path); err == nil {
		return backend.ReadWithFingerprint(path)
	}
	return core.LoadConfigWithFingerprint(path)
}

func writeConfigForAgent(path string, cfg map[string]interface{}, fingerprint, agent string) error {
	if backend, err := core.GetBackend(agent, path); err == nil {
		return backend.Write(path, cfg, fingerprint)
	}
	return core.WriteConfig(path, cfg, fingerprint)
}

// prepareChange 是所有写配置路径的统一前置动作：先做同级 .bak_ 备份，再建立还原点。
// 返回的第二个值非 nil 时，调用方直接把它当作错误响应返回即可。
//
// 任何新增的写配置路径都必须调用它，否则会破坏“每次变更都有还原点”的保证。
func (a *App) prepareChange(target, agent, operation, providerID, modelID, note string) (core.ChangeSnapshot, map[string]interface{}) {
	// 被 home 级 patch 覆盖的 profile 层写入注定失败：先拒绝，避免留下无用的
	// 同级备份与还原点（store 层稍后也会以同样的理由拒绝）；home 级 patch 读取或
	// 解析失败时同样在这里拒绝，而不是让它看起来成功后才发现写不进去。
	if shadowed, shadowErr := core.DeepSeekProfileShadowedByHome(target); shadowErr != nil {
		return core.ChangeSnapshot{}, map[string]interface{}{"success": false, "error": core.ShortText(shadowErr.Error(), 300)}
	} else if shadowed {
		return core.ChangeSnapshot{}, map[string]interface{}{"success": false, "error": "llm-pi-ai 配置被 $DSH_HOME/cordis.patch.yml 覆盖，请直接编辑该 home 级文件"}
	}
	snapshot, err := core.PrepareChange(core.ChangeContext{
		Target:     target,
		AgentID:    agent,
		Operation:  operation,
		ProviderID: providerID,
		ModelID:    modelID,
		Note:       note,
	})
	if err != nil {
		return core.ChangeSnapshot{}, map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	return snapshot, nil
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
	// 注册了 backend 的 agent（ZCode / DeepSeek）走统一分发；
	// OpenCode 没有 backend，回落到通用 JSON 视图。
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, err := backend.Read(target)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		summaries := backend.Store(doc).List(doc)
		if summaries == nil {
			summaries = []core.ProviderSummary{}
		}
		return map[string]interface{}{
			"success": true, "providers": summaries, "target": target, "agent": agent,
			"format": core.AgentFormatName(agent, target),
		}
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
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, err := backend.Read(target)
		if err != nil {
			return map[string]interface{}{"success": false, "error": err.Error()}
		}
		provider, gerr := backend.Store(doc).Get(doc, providerID)
		if gerr != nil {
			return map[string]interface{}{"success": false, "error": gerr.Error()}
		}
		return map[string]interface{}{"success": true, "provider": provider, "agent": agent, "format": core.AgentFormatName(agent, target)}
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
	// 注册了 backend 的 agent（ZCode 的两种格式 + DeepSeek Harness 的 profile/settings）
	// 一律走统一分发：文档变换交给 backend，备份与写盘时机仍由本函数控制。
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, fingerprint, rerr := backend.ReadWithFingerprint(target)
		if rerr != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(rerr.Error(), 300))}
		}
		store := backend.Store(doc)
		if existing, gerr := store.Get(doc, newID); gerr == nil && existing != nil && newID != providerID {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」已存在", newID)}
		}
		incomingModels, _ := providerCfg["models"].(map[string]interface{})
		if len(incomingModels) == 0 {
			for _, key := range []string{newID, providerID} {
				existing, gerr := store.Get(doc, key)
				if gerr == nil && existing != nil && len(existing.Cards) > 0 {
					return map[string]interface{}{"success": false, "error": "检测到模型列表为空。为避免误删已有模型，已取消保存；如需清空模型，请逐个删除。"}
				}
			}
		}
		snapshot, errResp := a.prepareChange(target, agent, core.OpSaveProvider, newID, "", "")
		if errResp != nil {
			return errResp
		}
		bak := snapshot.BackupPath
		merged, mergeErr := store.Upsert(doc, providerID, provider, core.SaveOptions{MergeModels: false})
		if mergeErr != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(mergeErr.Error(), 300))}
		}
		if err := backend.Write(target, merged, fingerprint); err != nil {
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
	if agent == string(core.AgentOpenCode) {
		providers, _ := config["provider"].(map[string]interface{})
		if providers == nil {
			providers = map[string]interface{}{}
			config["provider"] = providers
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
			if len(em) > 0 && len(incomingModels) == 0 {
				return map[string]interface{}{"success": false, "error": "检测到模型列表为空。为避免误删已有模型，已取消保存；如需清空模型，请逐个删除。"}
			}
		}
		snapshot, errResp := a.prepareChange(target, agent, core.OpSaveProvider, newID, "", "")
		if errResp != nil {
			return errResp
		}
		bak := snapshot.BackupPath
		if newID != providerID {
			delete(providers, providerID)
		}
		providers[newID] = core.OpenCodeProviderFromCfg(providerCfg)
		if err := core.WriteConfig(target, config, fingerprint); err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		latestBak := bak
		if latestBak == "" {
			latestBak = core.FindLatestBackup(target)
		}
		return map[string]interface{}{"success": true, "count": count, "provider_id": newID, "backup": latestBak, "target": target, "agent": agent}
	}
	// 兜底：既没有注册 backend、也不是 OpenCode 的 agent，沿用通用 JSON 视图。
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
	snapshot, errResp := a.prepareChange(target, agent, core.OpSaveProvider, newID, "", "")
	if errResp != nil {
		return errResp
	}
	bak := snapshot.BackupPath
	merged, mergeErr := core.MergeProviderIntoConfig(config, newID, providerCfg, false)
	if mergeErr != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(mergeErr.Error(), 300))}
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
	// 注册了 backend 的 agent 走统一分发（ZCode 两种格式 + DeepSeek profile/settings）。
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, fingerprint, rerr := backend.ReadWithFingerprint(target)
		if rerr != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除提供商时出错：\n%s", core.ShortText(rerr.Error(), 300))}
		}
		newDoc, found, derr := backend.Store(doc).Delete(doc, providerID)
		if derr != nil {
			return map[string]interface{}{"success": false, "error": derr.Error()}
		}
		if !found {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
		}
		snapshot, errResp := a.prepareChange(target, agent, core.OpDeleteProvider, providerID, "", "")
		if errResp != nil {
			return errResp
		}
		bak := snapshot.BackupPath
		if err := backend.Write(target, newDoc, fingerprint); err != nil {
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
	providers, _ := config["provider"].(map[string]interface{})
	if providers == nil || providers[providerID] == nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
	}
	snapshot, errResp := a.prepareChange(target, agent, core.OpDeleteProvider, providerID, "", "")
	if errResp != nil {
		return errResp
	}
	bak := snapshot.BackupPath
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
	// 注册了 backend 的 agent 走统一分发（ZCode 两种格式 + DeepSeek profile/settings）。
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, fingerprint, rerr := backend.ReadWithFingerprint(target)
		if rerr != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("删除模型时出错：\n%s", core.ShortText(rerr.Error(), 300))}
		}
		newDoc, found, derr := backend.Store(doc).DeleteModel(doc, providerID, modelID)
		if derr != nil {
			return map[string]interface{}{"success": false, "error": derr.Error()}
		}
		if !found {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("模型「%s」不存在", modelID)}
		}
		snapshot, errResp := a.prepareChange(target, agent, core.OpDeleteModel, providerID, modelID, "")
		if errResp != nil {
			return errResp
		}
		bak := snapshot.BackupPath
		if err := backend.Write(target, newDoc, fingerprint); err != nil {
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
	if agent != string(core.AgentOpenCode) {
		core.NormalizeConfigKinds(config)
	}
	providers, _ := config["provider"].(map[string]interface{})
	if providers == nil || providers[providerID] == nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("提供商「%s」不存在", providerID)}
	}
	prov, _ := providers[providerID].(map[string]interface{})
	models, _ := prov["models"].(map[string]interface{})
	if models == nil || models[modelID] == nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("模型「%s」不存在", modelID)}
	}
	snapshot, errResp := a.prepareChange(target, agent, core.OpDeleteModel, providerID, modelID, "")
	if errResp != nil {
		return errResp
	}
	bak := snapshot.BackupPath
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

// zcodeFormatTargetPath 是格式判定使用的 ZCode 目标文件：
// 当前 Agent 是 ZCode 时用实际目标文件，否则用 ZCode 的默认路径（优先新版）。
func (a *App) zcodeFormatTargetPath() string {
	if a.currentAgent == string(core.AgentZCode) && strings.TrimSpace(a.targetConfigPath) != "" {
		return a.targetConfigPath
	}
	return core.ZCodeDefaultPath()
}

// loadLegacyImportPreview 读取旧版 config.json（不存在时视为空）与新版配置，
// 计算待导入的旧供应商。
func (a *App) loadLegacyImportPreview() (core.LegacyImportPreview, error) {
	legacyDoc, err := core.LoadConfig(core.ZCodeConfig())
	if err != nil {
		return core.LegacyImportPreview{}, err
	}
	v2Doc, err := core.LoadConfig(a.zcodeFormatTargetPath())
	if err != nil {
		return core.LegacyImportPreview{}, err
	}
	return core.PreviewLegacyImport(legacyDoc, v2Doc)
}

// GetZCodeFormat 返回当前 ZCode 配置格式（v2 / legacy / unknown）与判定路径。
// supports_legacy_import 为真表示：当前是 v2、旧版 config.json 存在且尚有未导入的供应商。
func (a *App) GetZCodeFormat() map[string]interface{} {
	path := a.zcodeFormatTargetPath()
	format := core.DetectZCodeFormatAt(path)
	supports := false
	if core.IsZCodeV2Path(path) {
		if _, statErr := os.Stat(core.ZCodeConfig()); statErr == nil {
			if preview, err := a.loadLegacyImportPreview(); err == nil {
				supports = len(preview.Providers) > 0
			}
		}
	}
	return map[string]interface{}{
		"success":                true,
		"format":                 core.ZCodeFormatName(format),
		"path":                   path,
		"supports_legacy_import": supports,
	}
}

// GetZCodeCompat 检查当前 ZCode 新版配置里“ZCode 3.14 会拒绝”的问题（只读）。
// 这些问题会让 ZCode 把整份个人配置当成空，所以要在保存前提示并修复。
func (a *App) GetZCodeCompat() map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	report := core.InspectZCodeCompat(a.zcodeFormatTargetPath())
	return map[string]interface{}{
		"success": true,
		"path":    report.Path,
		"format":  report.Format,
		"issues":  report.Issues,
	}
}

// RepairZCodeCompat 一键修复上述兼容性问题并写回（写盘前照例备份 + 建还原点）。
func (a *App) RepairZCodeCompat() map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.zcodeFormatTargetPath()
	if target == "" {
		return map[string]interface{}{"success": false, "error": "请先选择目标配置文件"}
	}
	report, fixes, snapshot, err := core.RepairZCodeCompatAt(target, a.currentAgent)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	return map[string]interface{}{
		"success":       true,
		"path":          report.Path,
		"fixes":         fixes,
		"issues":        report.Issues,
		"backup":        snapshot.BackupPath,
		"restore_point": snapshot.RestorePoint,
		"target":        target,
	}
}

// PreviewLegacyImport 预览旧版 config.json 中可导入新版的供应商，以及转换会丢掉的字段。
func (a *App) PreviewLegacyImport() map[string]interface{} {
	if !core.IsZCodeV2Path(a.zcodeFormatTargetPath()) {
		return map[string]interface{}{"success": false, "error": "当前不是新版配置格式（provider_config.json），无法导入旧供应商"}
	}
	preview, err := a.loadLegacyImportPreview()
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	return map[string]interface{}{
		"success":   true,
		"providers": preview.Providers,
		"dropped":   preview.Dropped,
	}
}

// ApplyLegacyImport 把选中的旧供应商一次性导入新版配置。
// 写盘前会备份目标新版文件，返回值里带上备份路径。旧版 config.json 不会被修改。
func (a *App) ApplyLegacyImport(ids []string) map[string]interface{} {
	if len(ids) == 0 {
		return map[string]interface{}{"success": false, "error": "请至少选择一个提供商"}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	target := a.zcodeFormatTargetPath()
	// 这不是通用校验，而是防配置损坏：canonical v2 内容一旦写进旧版 config.json，
	// ZCode 会认不出旧版结构，等于毁掉这份配置，所以非 v2 目标一律拒绝。
	if !core.IsZCodeV2Path(target) {
		return map[string]interface{}{"success": false, "error": "当前不是新版配置格式（provider_config.json），无法导入旧供应商"}
	}
	legacyDoc, err := core.LoadConfig(core.ZCodeConfig())
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	v2Doc, err := core.LoadConfig(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	_, report, applyErr := core.ApplyLegacyImport(v2Doc, legacyDoc, ids, target)
	if applyErr != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(applyErr.Error(), 300)}
	}
	return map[string]interface{}{
		"success":  true,
		"imported": report.Imported,
		"skipped":  report.Skipped,
		"backup":   report.Backup,
	}
}

func (a *App) FetchModels(baseURL, apiKey string) map[string]interface{} {
	raw, err := core.FetchModelsRaw(baseURL, apiKey, 40*1e9)
	if err != nil {
		if mfe, ok := err.(*core.ModelFetchError); ok {
			return map[string]interface{}{"success": false, "error": mfe.Msg, "error_code": mfe.ErrorCode}
		}
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300), "error_code": "OTHER"}
	}
	cards := core.BuildModelCards(raw, a.autoFillLimits())
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
	// 注册了 backend 的 agent 走统一分发：用 store 的 Get 取 Base URL 与凭据。
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, rerr := backend.Read(target)
		if rerr != nil {
			return map[string]interface{}{"success": false, "error": rerr.Error()}
		}
		edit, gerr := backend.Store(doc).Get(doc, providerID)
		if gerr != nil {
			return map[string]interface{}{"success": false, "error": gerr.Error()}
		}
		baseURL := strings.TrimSpace(baseURLOverride)
		if baseURL == "" {
			baseURL = edit.BaseURL
		}
		apiKey := strings.TrimSpace(apiKeyOverride)
		if apiKey == "" {
			apiKey = edit.APIKey
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
	card, err := core.BuildSingleCard(modelID, a.autoFillLimits())
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "card": card}
}

// ApplyModelPresets applies the common-model preset limits to the given
// cards, overwriting existing context/output values that the preset covers.
// The UI confirms with the user before overwriting filled values.
func (a *App) ApplyModelPresets(cards []core.ModelCard) map[string]interface{} {
	updated, filled := core.ApplyModelPresets(cards)
	return map[string]interface{}{"success": true, "cards": updated, "filled": filled}
}

// importProviderPayload 把 ImportProvider 的表单字段整理成 backend 的编辑器 payload，
// 键名与 BuildProviderCfgEditor 的入参保持一致。
func importProviderPayload(pid, providerName, baseURL, apiKey, kind string, cards []core.ModelCard) core.ProviderPayload {
	return core.ProviderPayload{
		"id":               pid,
		"name":             providerName,
		"kind":             kind,
		"base_url":         baseURL,
		"api_key":          apiKey,
		"api_key_required": strings.TrimSpace(apiKey) != "",
		"cards":            cards,
	}
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
	if !mergeModels && len(cards) == 0 && a.targetConfigPath != "" {
		// 空模型 + “覆盖”语义时给出的专门提示必须在 BuildProviderCfg 之前判断：
		// 否则用户先看到的是通用的“请至少填写一个有效的模型”，这句保护永远不会触发。
		if backend, berr := core.GetBackend(a.currentAgent, a.targetConfigPath); berr == nil {
			if doc, rerr := backend.Read(a.targetConfigPath); rerr == nil {
				if existing, gerr := backend.Store(doc).Get(doc, strings.TrimSpace(providerID)); gerr == nil && existing != nil && len(existing.Cards) > 0 {
					return map[string]interface{}{"success": false, "error": "检测到模型列表为空。为避免误删已有模型，已取消导入；如需清空模型，请逐个删除。"}
				}
			}
		}
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
	// 注册了 backend 的 agent（ZCode 的两种格式 + DeepSeek Harness 的 profile/settings）
	// 一律走 backend 分发：绝不能沿用下面的 legacy 路径——MergeProviderIntoConfig 会把
	// 旧版的顶层 provider 映射合并进新版文档，ZCode 新版会因此把整份配置当空。
	if backend, berr := core.GetBackend(agent, target); berr == nil {
		doc, fingerprint, rerr := backend.ReadWithFingerprint(target)
		if rerr != nil {
			return map[string]interface{}{"success": false, "error": rerr.Error()}
		}
		store := backend.Store(doc)
		if existingEdit, getErr := store.Get(doc, pid); getErr == nil && existingEdit != nil && len(existingEdit.Cards) > 0 {
			if !mergeModels {
				if len(cards) == 0 {
					return map[string]interface{}{"success": false, "error": "检测到模型列表为空。为避免误删已有模型，已取消导入；如需清空模型，请逐个删除。"}
				}
			} else {
				// backend 的 Upsert 是“模型集合整体替换”，先把已有模型并进来，
				// 复刻 legacy MergeProviderIntoConfig(mergeModels=true) 的合并语义。
				cards = core.MergeModelCards(existingEdit.Cards, cards)
			}
		}
		snapshot, errResp := a.prepareChange(target, agent, core.OpImportProvider, pid, "", "导入供应商")
		if errResp != nil {
			return errResp
		}
		bak := snapshot.BackupPath
		merged, uerr := store.Upsert(doc, pid, importProviderPayload(pid, providerName, baseURL, apiKey, kind, cards), core.SaveOptions{MergeModels: mergeModels})
		if uerr != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(uerr.Error(), 300))}
		}
		if err := backend.Write(target, merged, fingerprint); err != nil {
			return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
		}
		latestBak := bak
		if latestBak == "" {
			latestBak = core.FindLatestBackup(target)
		}
		// count 必须反映写入后的真实模型数：merge 模式会把目标已有模型并进来，
		// 只按本次提交的卡片数回报会让前端提示的“成功导入 N 个模型”偏小。
		count := 0
		if finalEdit, ferr := store.Get(merged, pid); ferr == nil && finalEdit != nil {
			count = len(finalEdit.Cards)
		} else if m, ok := providerCfg["models"].(map[string]interface{}); ok {
			count = len(m)
		}
		return map[string]interface{}{"success": true, "count": count, "provider_id": pid, "target": target, "backup": latestBak, "latest_backup": latestBak, "agent": agent}
	}
	// 其它 Agent（OpenCode 等）没有注册 config backend，保持原有 legacy 路径不变。
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
	snapshot, errResp := a.prepareChange(target, agent, core.OpImportProvider, pid, "", "导入供应商")
	if errResp != nil {
		return errResp
	}
	bak := snapshot.BackupPath
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
	snapshot, errResp := a.prepareChange(target, agent, core.OpImportProvider, "", "", "导入 OpenCode 供应商")
	if errResp != nil {
		return errResp
	}
	bak := snapshot.BackupPath
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

// configFileAgent 判定一份配置文件属于哪个 Agent，并返回其格式名。
// 只按文件内容判定，供“选择一份配置文件合并进当前 Agent”的入口使用。
func configFileAgent(path string) (string, string) {
	if format := core.DeepSeekFormatName(core.DetectDeepSeekFormatAt(path)); format != "unknown" {
		return string(core.AgentDeepSeek), format
	}
	if format := core.AgentFormatName(string(core.AgentZCode), path); format != "unknown" {
		return string(core.AgentZCode), format
	}
	return "", ""
}

// PreviewConfigMerge 预览任意受支持的配置文件里可合并的供应商。
func (a *App) PreviewConfigMerge(path string) map[string]interface{} {
	if strings.TrimSpace(path) == "" {
		return map[string]interface{}{"success": false, "error": "请先选择要合并的配置文件"}
	}
	agent, format := configFileAgent(path)
	if agent == "" {
		return map[string]interface{}{"success": false, "error": "无法识别该配置文件的格式（支持 ZCode 与 DeepSeek Harness 的配置文件）"}
	}
	item := core.ConfigProviderSummaryForAgent(path, agent)
	return map[string]interface{}{"success": true, "path": item.Path, "exists": item.Exists, "error": item.Error, "providers": item.Providers, "agent": agent, "format": format}
}

func (a *App) ChooseMergeFile() map[string]interface{} {
	if a.ctx == nil {
		return map[string]interface{}{"success": false, "path": "", "error": "窗口未就绪"}
	}
	result, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Filters: []runtime.FileFilter{{DisplayName: "JSON/YAML 文件", Pattern: "*.json;*.jsonc;*.yaml;*.yml"}, {DisplayName: "所有文件 (*.*)", Pattern: "*.*"}},
	})
	if err != nil {
		return map[string]interface{}{"success": false, "path": "", "error": core.ShortText(err.Error(), 300)}
	}
	if result == "" {
		return map[string]interface{}{"success": false, "path": "", "cancelled": true}
	}
	if _, osErr := os.Stat(result); osErr != nil {
		return map[string]interface{}{"success": false, "path": "", "error": "请选择有效的配置文件"}
	}
	if agent, _ := configFileAgent(result); agent == "" {
		return map[string]interface{}{"success": false, "path": "", "error": "无法识别该配置文件的格式（支持 ZCode 与 DeepSeek Harness 的配置文件）"}
	}
	return map[string]interface{}{"success": true, "path": result}
}

// MergeConfig 把另一份配置文件（ZCode 或 DeepSeek Harness）里选中的供应商合并进当前 Agent。
//
// 旧实现只认 ZCode 旧版的顶层 provider 映射，并且当前 Agent 是 DeepSeek 时直接拒绝；
// 这里改成与其它写路径一致：源、目标都通过 config backend 读取与写入，因此
// ZCode 新版（provider_config.json）与 DSH 0.2（cordis.patch.yml + 凭据文件）都不会被写坏。
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
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			selectedIDs = append(selectedIDs, strings.TrimSpace(s))
		}
	}
	merge := true
	if v, ok := payload["merge"].(bool); ok {
		merge = v
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{"success": false, "error": "源配置文件不存在"}
	}
	srcAgent, srcFormat := configFileAgent(path)
	if srcAgent == "" {
		return map[string]interface{}{"success": false, "error": "无法识别该配置文件的格式（支持 ZCode 与 DeepSeek Harness 的配置文件）"}
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
	if absSrc, err := filepath.Abs(path); err == nil {
		if absTgt, err2 := filepath.Abs(target); err2 == nil && strings.EqualFold(absSrc, absTgt) {
			return map[string]interface{}{"success": false, "error": "源配置文件与目标配置文件相同，无需合并"}
		}
	}
	srcBackend, err := core.GetBackend(srcAgent, path)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	srcDoc, err := srcBackend.Read(path)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	srcStore := srcBackend.Store(srcDoc)
	tgtBackend, err := core.GetBackend(agent, target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	tgtDoc, fingerprint, err := tgtBackend.ReadWithFingerprint(target)
	if err != nil {
		return map[string]interface{}{"success": false, "error": core.ShortText(err.Error(), 300)}
	}
	tgtStore := tgtBackend.Store(tgtDoc)
	if len(selectedIDs) == 0 {
		for _, s := range srcStore.List(srcDoc) {
			selectedIDs = append(selectedIDs, s.ID)
		}
	}
	if len(selectedIDs) == 0 {
		return map[string]interface{}{"success": false, "error": "源配置文件中没有可合并的供应商"}
	}
	snapshot, errResp := a.prepareChange(target, agent, core.OpImportProvider, "", "", "合并配置文件")
	if errResp != nil {
		return errResp
	}
	imported := []string{}
	merged := []string{}
	for _, pid := range selectedIDs {
		edit, gerr := srcStore.Get(srcDoc, pid)
		if gerr != nil {
			return map[string]interface{}{"success": false, "error": gerr.Error()}
		}
		apiKey := strings.TrimSpace(edit.APIKey)
		cards := edit.Cards
		if existing, eerr := tgtStore.Get(tgtDoc, pid); eerr == nil && existing != nil {
			if merge {
				cards = core.MergeModelCards(existing.Cards, cards)
				merged = append(merged, pid)
			} else {
				imported = append(imported, pid)
			}
			if apiKey == "" {
				apiKey = existing.APIKey
			}
		} else {
			imported = append(imported, pid)
		}
		next, uerr := tgtStore.Upsert(tgtDoc, pid, core.ProviderPayload{
			"id":               pid,
			"name":             edit.Name,
			"kind":             edit.Kind,
			"base_url":         edit.BaseURL,
			"api_key":          apiKey,
			"api_key_required": apiKey != "",
			"cards":            cards,
			// 带上源 provider 可透传的额外字段（headers 等），否则跨 Agent 合并会把它们丢掉。
			"options_json": edit.OptionsJSON,
		}, core.SaveOptions{MergeModels: merge})
		if uerr != nil {
			return map[string]interface{}{"success": false, "error": core.ShortText(uerr.Error(), 300)}
		}
		tgtDoc = next
	}
	if err := tgtBackend.Write(target, tgtDoc, fingerprint); err != nil {
		return map[string]interface{}{"success": false, "error": fmt.Sprintf("写入配置文件时出错：\n%s", core.ShortText(err.Error(), 300))}
	}
	latestBak := snapshot.BackupPath
	if latestBak == "" {
		latestBak = core.FindLatestBackup(target)
	}
	return map[string]interface{}{"success": true, "imported": imported, "merged": merged, "target": target, "backup": latestBak, "latest_backup": latestBak, "source_agent": srcAgent, "source_format": srcFormat}
}

// Migrate
func (a *App) MigratePreview(sourceAgent, targetAgent string) map[string]interface{} {
	a.mu.Lock()
	srcPath := a.agentTargetPathLocked(sourceAgent)
	tgtPath := a.agentTargetPathLocked(targetAgent)
	a.mu.Unlock()
	// 预览基于两个 Agent 当前应使用的文件（已选路径优先，但会跳过被新版取代的旧版文件）。
	preview := core.MigratePreviewAtPaths(sourceAgent, targetAgent, srcPath, tgtPath)
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
	// 与预览一致：使用两个 Agent 当前应使用的文件，避免写进已被新版取代的旧版文件。
	result, err := core.MigrateExecuteAtPaths(source, target, a.agentTargetPathLocked(source), a.agentTargetPathLocked(target), selectedIDs, mode)
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
