package core

import (
	"os"
	"path/filepath"
	"strings"
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
		{ID: AgentDeepSeek, Label: "DeepSeek Harnessed", LabelEn: "DeepSeek Harnessed", DetectLabel: "DeepSeek 配置"},
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
