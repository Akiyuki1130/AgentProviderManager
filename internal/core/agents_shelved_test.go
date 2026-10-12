package core

import "testing"

// TestOpenCodeShelvedFromAgentEntries 锁定“OpenCode 已从应用入口完全隐藏”的行为：
// AllAgents / IsValidAgent 不再暴露 OpenCode，但 AgentOpenCode 常量与
// NormalizeAgentID 的识别能力保留，供历史数据与内部兼容使用。
func TestOpenCodeShelvedFromAgentEntries(t *testing.T) {
	agents := AllAgents()
	if len(agents) != 2 {
		t.Fatalf("AllAgents() 长度 = %d，期望 2（仅 zcode/deepseek）", len(agents))
	}
	for _, def := range agents {
		if def.ID == AgentOpenCode {
			t.Fatalf("AllAgents() 不应再返回 OpenCode：%#v", def)
		}
	}
	if IsValidAgent("opencode") {
		t.Fatal("IsValidAgent(\"opencode\") 应为 false")
	}
	if IsValidAgent(" OPENCODE ") {
		t.Fatal("IsValidAgent 应忽略大小写/空白并拒绝 opencode")
	}
	if !IsValidAgent("zcode") || !IsValidAgent("deepseek") {
		t.Fatal("zcode / deepseek 仍应是合法 Agent")
	}
	if got := NormalizeAgentID("OpenCode"); got != AgentOpenCode {
		t.Fatalf("NormalizeAgentID(\"OpenCode\") = %q，期望 %q（历史兼容）", got, AgentOpenCode)
	}
}
