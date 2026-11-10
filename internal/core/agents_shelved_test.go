package core

import "testing"

// TestOpenCodeShelvedFromAgentEntries 锁定“OpenCode 已从应用入口完全隐藏”的行为：
// AllAgents / IsValidAgent 不再暴露 OpenCode，但 AgentOpenCode 常量与
// NormalizeAgentID 的识别能力保留，供历史数据与内部兼容使用。
func TestOpenCodeShelvedFromAgentEntries(t *testing.T) {
	agents := AllAgents()
	if len(agents) != 3 {
		t.Fatalf("AllAgents() 长度 = %d，期望 3（zcode/deepseek/omp）", len(agents))
	}
	foundOMP := false
	for _, def := range agents {
		if def.ID == AgentOpenCode {
			t.Fatalf("AllAgents() 不应再返回 OpenCode：%#v", def)
		}
		if def.ID == AgentOMP {
			foundOMP = true
			if def.Label != "OMP" || def.DetectLabel == "" {
				t.Fatalf("OMP 条目的标签不完整：%#v", def)
			}
		}
	}
	if !foundOMP {
		t.Fatal("AllAgents() 必须包含 OMP 条目")
	}
	if !IsValidAgent("omp") || !IsValidAgent(" OMP ") {
		t.Fatal("omp 应是合法 Agent（忽略大小写/空白）")
	}
	if got := NormalizeAgentID("OMP"); got != AgentOMP {
		t.Fatalf("NormalizeAgentID(\"OMP\") = %q，期望 %q", got, AgentOMP)
	}
	if got := AgentFormatName(string(AgentOMP), `C:\\x\\models.yml`); got != "models.yml" {
		t.Fatalf("AgentFormatName(omp) = %q，期望 models.yml", got)
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
