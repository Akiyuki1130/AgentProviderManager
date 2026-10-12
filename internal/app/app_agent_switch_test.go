package app

import (
	"path/filepath"
	"testing"

	"agentprovidermanager/internal/core"
)

// 切换 Agent 时必须落到新版/0.2 的默认文件，而不是 legacy 文件
// （ZCode 的 config.json / DSH 的 settings.yaml）。
func TestSetCurrentAgentUsesModernDefaultPaths(t *testing.T) {
	a := sandboxApp(t)
	home := filepath.Join(t.TempDir(), "dsh-home")
	t.Setenv("DSH_HOME", home)

	zcodeV2 := core.ZCodeProviderConfigPath()
	writeText(t, zcodeV2, v2ImportFixture)
	dshProfile := core.DeepSeekDesktopProfilePatch()
	writeText(t, dshProfile, dshProfileFixture)

	resp := a.SetCurrentAgent(string(core.AgentDeepSeek))
	requireSuccess(t, resp, "SetCurrentAgent(deepseek)")
	if got, _ := resp["path"].(string); got != dshProfile {
		t.Fatalf("DeepSeek 默认目标应为 0.2 profile patch %s，实际 %s", dshProfile, got)
	}
	list := a.ListProviders()
	requireSuccess(t, list, "ListProviders(deepseek)")
	if list["format"] != "profile-desktop" {
		t.Errorf("切换后应报告 profile-desktop，实际 %v", list["format"])
	}

	resp = a.SetCurrentAgent(string(core.AgentZCode))
	requireSuccess(t, resp, "SetCurrentAgent(zcode)")
	if got, _ := resp["path"].(string); got != zcodeV2 {
		t.Fatalf("ZCode 默认目标应为新版 provider_config.json %s，实际 %s", zcodeV2, got)
	}
	list = a.ListProviders()
	requireSuccess(t, list, "ListProviders(zcode)")
	if list["format"] != "v2" {
		t.Errorf("切换后应报告 v2，实际 %v", list["format"])
	}
}

// 启动时还没有 0.2 profile patch、只有旧版 settings.yaml；之后 profile patch 出现时，
// 切换 Agent 也必须落到 0.2 文件，而不是继续编辑 settings.yaml。
func TestSetCurrentAgentUpgradesToLateProfilePatch(t *testing.T) {
	a := sandboxApp(t)
	home := filepath.Join(t.TempDir(), "dsh-home")
	t.Setenv("DSH_HOME", home)

	legacy := core.DeepSeekSettingsPath()
	writeText(t, legacy, "llm-pi-ai:\n  providers: {}\n")
	s := a.loadSettings()
	s["agent"] = string(core.AgentDeepSeek)
	s["agentPaths"] = map[string]interface{}{string(core.AgentDeepSeek): legacy}
	if !a.saveSettings(s) {
		t.Fatal("写入测试设置失败")
	}
	a.loadAgentState()
	if a.targetConfigPath != legacy {
		t.Fatalf("启动时只有 settings.yaml，应先用它，实际 %s", a.targetConfigPath)
	}

	// DSH 0.2 随后创建了 profile patch。
	profile := core.DeepSeekDesktopProfilePatch()
	writeText(t, profile, "- id: tools\n  config: {}\n")

	resp := a.SetCurrentAgent(string(core.AgentDeepSeek))
	requireSuccess(t, resp, "SetCurrentAgent(deepseek)")
	if got, _ := resp["path"].(string); got != profile {
		t.Fatalf("profile patch 出现后切换 Agent 应落到 %s，实际 %s", profile, got)
	}
	if got := a.agentPaths[string(core.AgentDeepSeek)]; got != profile {
		t.Fatalf("agentPaths 也应更新为 %s，实际 %s", profile, got)
	}
}
