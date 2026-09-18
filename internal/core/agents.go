package core

import (
	"os"
	"path/filepath"
	"strings"

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
	return []AgentDef{
		{ID: AgentZCode, Label: "ZCode", LabelEn: "ZCode", DetectLabel: "ZCode 全局配置"},
		{ID: AgentOpenCode, Label: "OpenCode", LabelEn: "OpenCode", DetectLabel: "OpenCode 配置"},
		{ID: AgentDeepSeek, Label: "DeepSeek Harness", LabelEn: "DeepSeek Harness", DetectLabel: "DeepSeek 配置"},
	}
}

func IsValidAgent(id string) bool {
	switch AgentID(strings.ToLower(strings.TrimSpace(id))) {
	case AgentZCode, AgentOpenCode, AgentDeepSeek:
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
