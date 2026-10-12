package core

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"agentprovidermanager/internal/dshprovider"
	"agentprovidermanager/internal/zcodeprovider"
)

type AgentID string

const (
	AgentZCode    AgentID = "zcode"
	AgentOpenCode AgentID = "opencode"
	AgentDeepSeek AgentID = "deepseek"
)

type AgentDef struct {
	ID          AgentID `json:"id"`
	Label       string  `json:"label"`
	LabelEn     string  `json:"labelEn"`
	DetectLabel string  `json:"detectLabel"`
}

func AllAgents() []AgentDef {
	// OpenCode 入口已在应用内完全隐藏：AllAgents 只返回当前可选的 Agent。
	// AgentOpenCode 常量与 NormalizeAgentID 的识别能力保留，供历史数据与内部兼容使用。
	return []AgentDef{
		{ID: AgentZCode, Label: "ZCode", LabelEn: "ZCode", DetectLabel: "ZCode 全局配置"},
		{ID: AgentDeepSeek, Label: "DeepSeek Harness", LabelEn: "DeepSeek Harness", DetectLabel: "DeepSeek 配置"},
	}
}

// IsValidAgent 只把当前可选的 Agent 视为合法；已雪藏的 OpenCode 返回 false，
// app.go 的 loadAgentState 借此让本地已存的 "opencode" 回落。
func IsValidAgent(id string) bool {
	switch AgentID(strings.ToLower(strings.TrimSpace(id))) {
	case AgentZCode, AgentDeepSeek:
		return true
	}
	return false
}

func NormalizeAgentID(id string) AgentID {
	n := AgentID(strings.ToLower(strings.TrimSpace(id)))
	switch n {
	case AgentZCode, AgentOpenCode, AgentDeepSeek:
		return n
	}
	return AgentZCode
}

// AgentDefaultPath 返回某个 Agent 的“规范”默认配置路径。
//
// AgentZCode 必须返回旧版 config.json：migrate.go 的 buildSide /
// MigrateExecuteAtPaths 与 DetectConfigLocationsForAgent 都以此路径读 provider 键，
// 而新版 provider_config.json 没有该键。想要“新版优先”的路径请用 ZCodeDefaultPath。
func AgentDefaultPath(id AgentID) string {
	switch id {
	case AgentOpenCode:
		return OpencodeConfig()
	case AgentDeepSeek:
		return DeepSeekSettingsPath()
	default:
		return ZCodeConfig()
	}
}

// ZCodeProviderConfigPath 返回 ZCode 新版个人 provider 配置文件路径
// （~/.zcode/v2/provider_config.json）。环境变量 ZCODE_PERSONAL_PROVIDER_CONFIG_FILE
// 可覆盖它（ZCode 自身与该变量同名），测试也依赖这个覆盖点。
func ZCodeProviderConfigPath() string {
	if v := strings.TrimSpace(os.Getenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE")); v != "" {
		return v
	}
	return filepath.Join(ZCodeDir(), "provider_config.json")
}

// ZCodeDefaultPath 优先返回新版路径：新版文件确实存在且是 v2 格式时用它；
// 否则退回仍在使用中的旧版 config.json；两者都没有时指向新版（新装用户即 v2）。
//
// 只允许 app 层调用：core 内的既有语义（migrate / 配置位置探测）依赖 AgentDefaultPath
// 的旧版路径，二者不能混用。
func ZCodeDefaultPath() string {
	v2 := ZCodeProviderConfigPath()
	if DetectZCodeFormatAt(v2) == zcodeprovider.FormatV2 {
		return v2
	}
	if DetectZCodeFormatAt(ZCodeConfig()) == zcodeprovider.FormatLegacy {
		return ZCodeConfig()
	}
	return v2
}

// DetectZCodeFormatAt 读取目标文件内容并判定 ZCode 配置格式。
// 文件不存在、读失败或内容无法判定时返回 FormatUnknown。
func DetectZCodeFormatAt(path string) zcodeprovider.Format {
	if strings.TrimSpace(path) == "" {
		return zcodeprovider.FormatUnknown
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return zcodeprovider.FormatUnknown
	}
	return zcodeprovider.Detect(data)
}

// ZCodeFormatName 把格式判定结果映射为前端使用的字面量。
func ZCodeFormatName(f zcodeprovider.Format) string {
	switch f {
	case zcodeprovider.FormatV2:
		return "v2"
	case zcodeprovider.FormatLegacy:
		return "legacy"
	default:
		return "unknown"
	}
}

// IsZCodeV2Path 表示该路径上的文件确实是新版 provider_config.json。
func IsZCodeV2Path(path string) bool {
	return DetectZCodeFormatAt(path) == zcodeprovider.FormatV2
}

// AgentFormatName 返回目标文件在 UI 上的形态名，用于提示用户当前正在编辑哪种格式。
func AgentFormatName(agentID, path string) string {
	switch NormalizeAgentID(agentID) {
	case AgentZCode:
		return ZCodeFormatName(DetectZCodeFormatAt(path))
	case AgentDeepSeek:
		return DeepSeekFormatName(DetectDeepSeekFormatAt(path))
	}
	return ""
}

// DeepSeek Harness 0.2 的配置位置。
//
// 桌面端与 CLI 的 web profile 是同一套 bundle 栈（dsh-base + dsh-web-app），
// 因此二者的 patch 结构完全一致，区别只在 profile 目录：桌面端用 Electron
// 保留的 profiles/desktop，CLI 用 profiles/web（dsh web 等价于 dsh --profile web）。
// 自定义 profile（dsh --profile <name>）同样是 profiles/<name>/cordis.patch.yml。
//
//	$DSH_HOME/profiles/<name>/cordis.patch.yml   每个 profile 的用户 patch
//	$DSH_HOME/cordis.patch.yml                   home 级 patch（优先级更高，会覆盖 profile 层）
//	$DSH_HOME/.credentials.yaml                  凭据（version 1 + refs/records）
//	$DSH_HOME/settings.yaml                      0.1 的旧格式，0.2 只导入一次

// DeepSeekProfilesDir 返回 $DSH_HOME/profiles。
func DeepSeekProfilesDir() string {
	return filepath.Join(DshHome(), dshprovider.ProfilesDirName)
}

// DeepSeekProfileDir 返回某个 profile 的目录。
func DeepSeekProfileDir(name string) string {
	return dshprovider.ProfileDir(DshHome(), name)
}

// DeepSeekProfilePatch 返回某个 profile 的用户 patch 路径。
func DeepSeekProfilePatch(name string) string {
	return dshprovider.ProfilePatchPath(DshHome(), name)
}

// DeepSeekDesktopProfilePatch 返回桌面端 profile 的 patch 路径。
func DeepSeekDesktopProfilePatch() string {
	return DeepSeekProfilePatch(dshprovider.DesktopProfileName)
}

// DeepSeekWebProfilePatch 返回 web profile 的 patch 路径。
func DeepSeekWebProfilePatch() string {
	return DeepSeekProfilePatch(dshprovider.WebProfileName)
}

// DeepSeekHomePatch 返回 home 级 patch 路径。
func DeepSeekHomePatch() string {
	return dshprovider.HomePatchPath(DshHome())
}

// DeepSeekProfilePatches 返回已存在的 profile patch（按 profile 名排序，桌面端优先）。
func DeepSeekProfilePatches() []string {
	entries, err := os.ReadDir(DeepSeekProfilesDir())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == dshprovider.DesktopProfileName {
			return true
		}
		if names[j] == dshprovider.DesktopProfileName {
			return false
		}
		if names[i] == dshprovider.WebProfileName {
			return true
		}
		if names[j] == dshprovider.WebProfileName {
			return false
		}
		return names[i] < names[j]
	})
	out := make([]string, 0, len(names))
	for _, name := range names {
		path := DeepSeekProfilePatch(name)
		if _, err := os.Stat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}

// DeepSeekDefaultPath 选择 DSH 的默认目标：
// 已存在的 desktop patch -> web patch -> 其它 profile patch -> home patch ->
// 旧版 settings.* -> 新装用户的 desktop patch。只允许 app 层调用。
func DeepSeekDefaultPath() string {
	for _, path := range []string{DeepSeekDesktopProfilePatch(), DeepSeekWebProfilePatch()} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if patches := DeepSeekProfilePatches(); len(patches) > 0 {
		return patches[0]
	}
	if _, err := os.Stat(DeepSeekHomePatch()); err == nil {
		return DeepSeekHomePatch()
	}
	if _, err := os.Stat(DeepSeekSettingsPath()); err == nil {
		return DeepSeekSettingsPath()
	}
	return DeepSeekDesktopProfilePatch()
}

// IsDeepSeekProfilePath 表示该路径是 DSH 0.2 的 profile patch（cordis.patch.yml）。
func IsDeepSeekProfilePath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	format := DetectDeepSeekFormatAt(path)
	return format == dshprovider.FormatProfileDesktop || format == dshprovider.FormatProfileWeb || format == dshprovider.FormatProfileOther
}

// DetectDeepSeekFormatAt 读取目标文件并判定 DSH 配置形态。
func DetectDeepSeekFormatAt(path string) dshprovider.Format {
	if strings.TrimSpace(path) == "" {
		return dshprovider.FormatUnknown
	}
	return dshprovider.DetectFile(path)
}

// DeepSeekFormatName 把形态映射为前端字面量。
func DeepSeekFormatName(f dshprovider.Format) string {
	switch f {
	case dshprovider.FormatProfileDesktop:
		return "profile-desktop"
	case dshprovider.FormatProfileWeb:
		return "profile-web"
	case dshprovider.FormatProfileOther:
		return "profile-other"
	case dshprovider.FormatSettingsLegacy:
		return "settings-legacy"
	default:
		return "unknown"
	}
}

// DeepSeekFormatLabel 返回给用户看的形态名称。
func DeepSeekFormatLabel(f dshprovider.Format) string {
	switch f {
	case dshprovider.FormatProfileDesktop:
		return "DeepSeek Harness 桌面端 profile（0.2）"
	case dshprovider.FormatProfileWeb:
		return "DeepSeek Harness web profile（0.2）"
	case dshprovider.FormatProfileOther:
		return "DeepSeek Harness 自定义 profile（0.2）"
	case dshprovider.FormatSettingsLegacy:
		return "DeepSeek Harness 旧版 settings（0.1，仅导入）"
	default:
		return "DeepSeek Harness 配置"
	}
}


func DshHome() string {
	if v := strings.TrimSpace(os.Getenv("DSH_HOME")); v != "" {
		return v
	}
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".dsh")
}

func DeepSeekSettingsPath() string {
	home := DshHome()
	candidates := []string{
		filepath.Join(home, "settings.yaml"),
		filepath.Join(home, "settings.yml"),
		filepath.Join(home, "settings.json"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join(home, "settings.yaml")
}

func DeepSeekCredentialsPath() string {
	return filepath.Join(DshHome(), ".credentials.yaml")
}

func DeepSeekEnvCredentialsPath() string {
	return DeepSeekCredentialsPath()
}
