package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ompTestSandbox 把 omp 的路径解析完全约束在临时目录里。
func ompTestSandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("PI_CONFIG_DIR", "")
	t.Setenv("OMP_PROFILE", "")
	t.Setenv("PI_PROFILE", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	return home
}

func TestOMPBackendDescriptor(t *testing.T) {
	home := ompTestSandbox(t)
	desc := ompBackend{}.Descriptor()
	if desc.ID != "omp" || desc.Label != "OMP" {
		t.Fatalf("Descriptor 不符：%#v", desc)
	}
	want := filepath.Join(home, ".omp", "agent", "models.yml")
	if desc.DefaultPath != want {
		t.Fatalf("DefaultPath = %q，期望 %q", desc.DefaultPath, want)
	}
	if desc.Candidates == nil || len(desc.Candidates) != 0 {
		t.Fatalf("Candidates 必须是空切片（避免与 DetectConfigLocationsForAgent 互递归）：%#v", desc.Candidates)
	}
	if backend, err := GetBackend(string(AgentOMP), desc.DefaultPath); err != nil || backend == nil {
		t.Fatalf("omp 应能注册并取到默认 backend：backend=%v err=%v", backend, err)
	}
}

func TestOMPAgentDefaultPath(t *testing.T) {
	home := ompTestSandbox(t)
	if got, want := AgentDefaultPath(AgentOMP), filepath.Join(home, ".omp", "agent", "models.yml"); got != want {
		t.Fatalf("默认 profile：AgentDefaultPath(omp) = %q，期望 %q", got, want)
	}

	agentDir := filepath.Join(home, ".omp", "agent")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	jsonPath := filepath.Join(agentDir, "models.json")
	if err := os.WriteFile(jsonPath, []byte(`{"providers":{}}`), 0o600); err != nil {
		t.Fatalf("写入 models.json 失败：%v", err)
	}
	if got := AgentDefaultPath(AgentOMP); got != jsonPath {
		t.Fatalf("只有 models.json 时 AgentDefaultPath(omp) = %q，期望 %q", got, jsonPath)
	}

	customDir := filepath.Join(home, "custom-agent")
	t.Setenv("PI_CODING_AGENT_DIR", customDir)
	if got, want := AgentDefaultPath(AgentOMP), filepath.Join(customDir, "models.yml"); got != want {
		t.Fatalf("PI_CODING_AGENT_DIR：AgentDefaultPath(omp) = %q，期望 %q", got, want)
	}

	t.Setenv("PI_CODING_AGENT_DIR", "")
	t.Setenv("OMP_PROFILE", "work")
	if got, want := AgentDefaultPath(AgentOMP), filepath.Join(home, ".omp", "profiles", "work", "agent", "models.yml"); got != want {
		t.Fatalf("命名 profile：AgentDefaultPath(omp) = %q，期望 %q", got, want)
	}
}

func TestOMPBackendReadRejectsConfigAndUnknown(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		wantSub string
	}{
		{name: "config.yml", content: "modelRoles:\n  default: p/m\n", wantSub: "config.yml"},
		{name: "random.txt", content: "hello world\n", wantSub: "不是 omp 的模型文件"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name)
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("写入文件失败：%v", err)
			}
			if _, err := (ompBackend{}).Read(path); err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("Read 应拒绝 %s：err=%v", tc.name, err)
			}
		})
	}
}

func TestOMPBackendReadDetectsByContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-name.yml")
	content := "providers:\n  p1:\n    api: anthropic-messages\n    baseUrl: https://p1.example.com\n    models:\n      - id: m1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入文件失败：%v", err)
	}
	doc, fingerprint, err := ompBackend{}.ReadWithFingerprint(path)
	if err != nil {
		t.Fatalf("按内容嗅探读取失败：%v", err)
	}
	if fingerprint == "" {
		t.Fatal("ReadWithFingerprint 应返回非空指纹")
	}
	list := ompStore{}.List(doc)
	if len(list) != 1 || list[0].ID != "p1" || list[0].Kind != "anthropic" {
		t.Fatalf("内容嗅探结果不符：%#v", list)
	}
	if got := ompTestStrings(OMPSelectorsAtPath(path)); got != "p1/m1" {
		t.Fatalf("OMPSelectorsAtPath = %q，期望 p1/m1", got)
	}
	if list := OMPSelectorsAtPath(filepath.Join(dir, "missing.yml")); list == nil || len(list) != 0 {
		t.Fatalf("缺失文件的 selectors 应为空非 nil：%#v", list)
	}
}

func TestOMPBackendWriteRejectsTamperedFingerprint(t *testing.T) {
	doc, path := ompTestReadSample(t)
	before, err := FileFingerprint(path)
	if err != nil {
		t.Fatalf("计算指纹失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(ompTestModelsYAML+"# 外部追加\n"), 0o600); err != nil {
		t.Fatalf("模拟外部修改失败：%v", err)
	}
	if err := (ompBackend{}).Write(path, doc, before); err == nil || !strings.Contains(err.Error(), "其他程序修改") {
		t.Fatalf("指纹不匹配应拒绝写入：err=%v", err)
	}
}
