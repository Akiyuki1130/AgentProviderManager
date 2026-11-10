package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ompConfigSandbox 把 PI_CONFIG_DIR 指到临时目录，屏蔽真实 profile 与环境变量。
func ompConfigSandbox(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("PI_CONFIG_DIR", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("OMP_PROFILE", "")
	t.Setenv("PI_PROFILE", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	return root
}

func writeOMPTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("写入文件失败：%v", err)
	}
}

const ompModelsFixture = `providers:
  alpha:
    baseUrl: https://alpha.example.invalid/v1
    api: openai-completions
    apiKey: PLACEHOLDER
    models:
      - id: m1
        name: Model One
  beta:
    baseUrl: https://beta.example.invalid/v1
    api: anthropic-messages
    apiKey: PLACEHOLDER
    models:
      - id: m2
`

func TestDetectConfigLocationsForAgentOMP(t *testing.T) {
	root := ompConfigSandbox(t)

	// 1) 完全没有文件：给出一个尚不存在的默认 models.yml 候选。
	locs := DetectConfigLocationsForAgent("omp")
	if len(locs) != 1 {
		t.Fatalf("空目录应只列 1 个候选，得到 %#v", locs)
	}
	if want := filepath.Join(root, "agent", "models.yml"); locs[0].Path != want || locs[0].Exists {
		t.Fatalf("默认候选 = %#v，期望不存在路径 %q", locs[0], want)
	}

	// 2) PI_CODING_AGENT_DIR 生效时它就是默认 profile（omp 不读 <home>/agent），
	//    因此只列 override 目录 + 命名 profile；config.yml 不得出现。
	writeOMPTestFile(t, filepath.Join(root, "agent", "models.yml"), ompModelsFixture)
	writeOMPTestFile(t, filepath.Join(root, "agent", "config.yml"), "modelRoles:\n  default: alpha/m1\n")
	override := filepath.Join(root, "override-agent")
	writeOMPTestFile(t, filepath.Join(override, "models.yml"), ompModelsFixture)
	t.Setenv("PI_CODING_AGENT_DIR", override)
	writeOMPTestFile(t, filepath.Join(root, "profiles", "zeta", "agent", "models.yaml"), ompModelsFixture)
	writeOMPTestFile(t, filepath.Join(root, "profiles", "alpha", "agent", "models.yml"), ompModelsFixture)

	locs = DetectConfigLocationsForAgent("omp")
	wantPaths := []string{
		filepath.Join(override, "models.yml"),
		filepath.Join(root, "profiles", "alpha", "agent", "models.yml"),
		filepath.Join(root, "profiles", "zeta", "agent", "models.yaml"),
	}
	if len(locs) != len(wantPaths) {
		t.Fatalf("locations 数量 = %d（%#v），期望 %d", len(locs), locs, len(wantPaths))
	}
	for i, want := range wantPaths {
		if !strings.EqualFold(filepath.Clean(locs[i].Path), filepath.Clean(want)) {
			t.Fatalf("locations[%d].Path = %q，期望 %q", i, locs[i].Path, want)
		}
		if !locs[i].Exists || locs[i].NeedsMigration {
			t.Fatalf("locations[%d] = %#v，期望存在且无需迁移", i, locs[i])
		}
		if strings.HasSuffix(strings.ToLower(locs[i].Path), "config.yml") {
			t.Fatalf("config.yml 不得进入 provider 位置列表（G7）：%q", locs[i].Path)
		}
	}
}

func TestDetectConfigLocationsForAgentOMPJSONOnly(t *testing.T) {
	root := ompConfigSandbox(t)
	jsonPath := filepath.Join(root, "agent", "models.json")
	writeOMPTestFile(t, jsonPath, `{"providers":{"p1":{"baseUrl":"https://p1.example.invalid/v1","api":"openai-completions","apiKey":"PLACEHOLDER","models":{"m1":{}}}}}`)

	locs := DetectConfigLocationsForAgent("omp")
	if len(locs) != 1 {
		t.Fatalf("json-only 应只列 1 个位置，得到 %#v", locs)
	}
	if !strings.EqualFold(filepath.Clean(locs[0].Path), filepath.Clean(jsonPath)) {
		t.Fatalf("json-only 路径 = %q，期望 %q", locs[0].Path, jsonPath)
	}
	if !locs[0].Exists || !locs[0].NeedsMigration {
		t.Fatalf("json-only 必须标记 needs_migration：%#v", locs[0])
	}
}

func TestConfigProviderSummaryForAgentOMP(t *testing.T) {
	root := ompConfigSandbox(t)
	modelsPath := filepath.Join(root, "agent", "models.yml")
	fixture := ompModelsFixture + `  gamma:
    baseUrl: https://gamma.example.invalid/v1
    modelOverrides:
      only:
        name: Override Only
`
	writeOMPTestFile(t, modelsPath, fixture)

	preview := ConfigProviderSummaryForAgent(modelsPath, "omp")
	if preview.Error != "" {
		t.Fatalf("omp 摘要不应报错：%q", preview.Error)
	}
	if len(preview.Providers) != 3 {
		t.Fatalf("provider 数量 = %d（%#v），期望 3", len(preview.Providers), preview.Providers)
	}
	ids := make([]string, 0, len(preview.Providers))
	byID := map[string]ProviderSummary{}
	for _, p := range preview.Providers {
		ids = append(ids, p.ID)
		byID[p.ID] = p
	}
	if strings.Join(ids, ",") != "alpha,beta,gamma" {
		t.Fatalf("provider 顺序 = %v，期望按 ID 排序", ids)
	}
	if byID["alpha"].Kind != "openai-compatible" || byID["alpha"].ModelCount != 1 || byID["alpha"].ReadOnly {
		t.Fatalf("alpha 摘要不符：%#v", byID["alpha"])
	}
	if !byID["gamma"].ReadOnly || byID["gamma"].ReadOnly != true {
		t.Fatalf("只有 modelOverrides 的 provider 必须只读：%#v", byID["gamma"])
	}

	// config.yml 是设置文件，不是 provider 文件（G7）。
	cfgPath := filepath.Join(root, "agent", "config.yml")
	writeOMPTestFile(t, cfgPath, "modelRoles:\n  default: alpha/m1\n")
	cfgPreview := ConfigProviderSummaryForAgent(cfgPath, "omp")
	if len(cfgPreview.Providers) != 0 || !strings.Contains(cfgPreview.Error, "设置文件") {
		t.Fatalf("config.yml 应返回 0 个 provider 并提示设置文件：%#v", cfgPreview)
	}
}

func TestSniffConfigAgent(t *testing.T) {
	root := ompConfigSandbox(t)
	cases := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{name: "YAML providers", path: filepath.Join(t.TempDir(), "any.yml"), content: "providers:\n  p1: {}\n", want: string(AgentOMP)},
		{name: "YAML llm-pi-ai", path: filepath.Join(t.TempDir(), "any.yaml"), content: "llm-pi-ai:\n  providers: {}\n", want: string(AgentDeepSeek)},
		{name: "JSON legacy", path: filepath.Join(t.TempDir(), "config.json"), content: `{"provider":{"p1":{}}}`, want: ""},
		{name: "JSON v2", path: filepath.Join(t.TempDir(), "provider_config.json"), content: `{"schemaVersion":2,"config":{}}`, want: ""},
		{name: "omp config.yml", path: filepath.Join(root, "agent", "config.yml"), content: "modelRoles:\n  default: p/m\n", want: "omp-config"},
		{name: "无关 YAML", path: filepath.Join(t.TempDir(), "other.yml"), content: "foo: bar\n", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeOMPTestFile(t, tc.path, tc.content)
			if got := sniffConfigAgent(tc.path); got != tc.want {
				t.Fatalf("sniffConfigAgent(%s) = %q，期望 %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestDetectConfigLocationsForAgentOMPDefaultProfileOnly 覆盖：没有 PI_CODING_AGENT_DIR 时
// 默认 profile 就是 <home>/agent。
func TestDetectConfigLocationsForAgentOMPDefaultProfileOnly(t *testing.T) {
	root := ompConfigSandbox(t)
	modelsPath := filepath.Join(root, "agent", "models.yml")
	writeOMPTestFile(t, modelsPath, ompModelsFixture)

	locs := DetectConfigLocationsForAgent("omp")
	if len(locs) != 1 {
		t.Fatalf("应只列默认 profile 一个位置，得到 %#v", locs)
	}
	if !strings.EqualFold(filepath.Clean(locs[0].Path), filepath.Clean(modelsPath)) {
		t.Fatalf("默认位置 = %q，期望 %q", locs[0].Path, modelsPath)
	}
	if !locs[0].Exists || locs[0].NeedsMigration {
		t.Fatalf("默认位置应存在且无需迁移：%#v", locs[0])
	}
	if !strings.Contains(locs[0].Label, "默认 profile") {
		t.Fatalf("默认位置 label = %q，期望含「默认 profile」", locs[0].Label)
	}
}

// TestDetectConfigLocationsForAgentOMPCodingAgentDirOverridesDefault 覆盖：
// PI_CODING_AGENT_DIR 生效时 omp 只读该目录，<home>/agent 里再多的文件也不能被列成默认 profile。
func TestDetectConfigLocationsForAgentOMPCodingAgentDirOverridesDefault(t *testing.T) {
	root := ompConfigSandbox(t)
	writeOMPTestFile(t, filepath.Join(root, "agent", "models.yml"), ompModelsFixture)
	override := filepath.Join(t.TempDir(), "alt")
	writeOMPTestFile(t, filepath.Join(override, "models.yml"), ompModelsFixture)
	t.Setenv("PI_CODING_AGENT_DIR", override)

	locs := DetectConfigLocationsForAgent("omp")
	if len(locs) != 1 {
		t.Fatalf("override 生效时应只列 override 一个位置，得到 %#v", locs)
	}
	if want := filepath.Join(override, "models.yml"); !strings.EqualFold(filepath.Clean(locs[0].Path), filepath.Clean(want)) {
		t.Fatalf("override 位置 = %q，期望 %q", locs[0].Path, want)
	}
	if !strings.Contains(locs[0].Label, "PI_CODING_AGENT_DIR") {
		t.Fatalf("override 位置 label = %q，期望含 PI_CODING_AGENT_DIR", locs[0].Label)
	}
	if strings.EqualFold(filepath.Clean(locs[0].Path), filepath.Clean(filepath.Join(root, "agent", "models.yml"))) {
		t.Fatalf("<home>/agent 不应被列出：%#v", locs[0])
	}
}

// TestDetectConfigLocationsForAgentOMPCodingAgentDirHidesHomeAgent 覆盖：
// override 指向一个还没有 models 的目录时，只给该目录下的候选，绝不回落到 <home>/agent 的死文件。
func TestDetectConfigLocationsForAgentOMPCodingAgentDirHidesHomeAgent(t *testing.T) {
	root := ompConfigSandbox(t)
	homeModels := filepath.Join(root, "agent", "models.yml")
	writeOMPTestFile(t, homeModels, ompModelsFixture)
	override := filepath.Join(t.TempDir(), "alt")
	if err := os.MkdirAll(override, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PI_CODING_AGENT_DIR", override)

	locs := DetectConfigLocationsForAgent("omp")
	if len(locs) != 1 {
		t.Fatalf("应只给 override 目录的候选，得到 %#v", locs)
	}
	if want := filepath.Join(override, "models.yml"); !strings.EqualFold(filepath.Clean(locs[0].Path), filepath.Clean(want)) {
		t.Fatalf("候选位置 = %q，期望 %q", locs[0].Path, want)
	}
	if locs[0].Exists {
		t.Fatalf("候选不应被标记为存在：%#v", locs[0])
	}
	if !strings.Contains(locs[0].Label, "PI_CODING_AGENT_DIR") {
		t.Fatalf("候选 label = %q，期望含 PI_CODING_AGENT_DIR", locs[0].Label)
	}
	if strings.EqualFold(filepath.Clean(locs[0].Path), filepath.Clean(homeModels)) {
		t.Fatalf("<home>/agent 不应被列出：%#v", locs[0])
	}
}

// TestDetectConfigLocationsOverviewIncludesOMP 覆盖总览（agentID 为空）也要能看到 omp 的 models 文件。
func TestDetectConfigLocationsOverviewIncludesOMP(t *testing.T) {
	root := ompConfigSandbox(t)
	modelsPath := filepath.Join(root, "agent", "models.yml")
	writeOMPTestFile(t, modelsPath, ompModelsFixture)

	var found *ConfigLocation
	locs := DetectConfigLocationsForAgent("")
	for i := range locs {
		if strings.EqualFold(filepath.Clean(locs[i].Path), filepath.Clean(modelsPath)) {
			found = &locs[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("总览缺少 omp 的 models 文件位置：%#v", locs)
	}
	if !found.Exists || found.ProviderCount != 2 {
		t.Fatalf("omp 总览条目 = %#v，期望存在且含 2 个 provider", found)
	}
}
