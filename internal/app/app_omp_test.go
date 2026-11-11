package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
)

// 这一组测试覆盖 app 层的 OMP 接线：默认模型绑定、configFileAgent 的 §9.1 嗅探、
// omp 默认路径与只读 provider 的保存前置检查。
// 所有触盘操作都沙箱化（USERPROFILE / PI_CONFIG_DIR / PI_CODING_AGENT_DIR）。

// ompTestEnv 把 omp 的配置根目录指向临时目录，并返回默认 profile 的 agent 目录。
func ompTestEnv(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("PI_CONFIG_DIR", "")
	t.Setenv("OMP_PROFILE", "")
	t.Setenv("PI_PROFILE", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	return home, filepath.Join(home, ".omp", "agent")
}

// ompModelsFixture 覆盖 selector 通道的两个关键判定：协议可映射（acme/beta 入选，
// weird 的 google-generative-ai 不可映射）与 models[] 显式声明的顺序无关性。
// weird 带 auth: none：omp 的 provider 只要声明了 models 就必须有 apiKey 或 auth，
// 这份 fixture 因此仍是真实 omp 会接受的合法文件（A 的 Validate 会拒绝整份文件）。
const ompModelsFixture = `providers:
  acme:
    baseUrl: https://acme.example.invalid/v1
    api: openai-completions
    apiKey: sk-placeholder
    models:
      - id: m2
      - id: m1
  beta:
    baseUrl: https://beta.example.invalid/v1
    api: anthropic-messages
    apiKey: sk-placeholder
    models:
      - id: m3
  weird:
    baseUrl: https://weird.example.invalid/v1
    api: google-generative-ai
    auth: none
    models:
      - id: m4
`

func TestGetOMPModelRoles(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	writeText(t, filepath.Join(dir, "models.yml"), ompModelsFixture)
	writeText(t, filepath.Join(dir, "config.yml"), "modelRoles:\n  default: acme/m1\n")
	a.currentAgent = string(core.AgentOMP)

	resp := a.GetOMPModelRoles()
	requireSuccess(t, resp, "GetOMPModelRoles")
	if got, _ := resp["path"].(string); got != filepath.Join(dir, "config.yml") {
		t.Fatalf("path = %q，期望 config.yml", got)
	}
	if exists, _ := resp["exists"].(bool); !exists {
		t.Fatal("config.yml 存在时 exists 应为 true")
	}
	roles, _ := resp["roles"].(map[string]string)
	if roles["default"] != "acme/m1" {
		t.Fatalf("roles = %#v，期望 default=acme/m1", roles)
	}
	selectors, _ := resp["selectors"].([]string)
	want := []string{"acme/m1", "acme/m2", "beta/m3"}
	if !reflect.DeepEqual(selectors, want) {
		t.Fatalf("selectors = %#v，期望 %#v", selectors, want)
	}
	if nm, _ := resp["needs_migration"].(bool); nm {
		t.Fatal("已有 models.yml 时不应提示迁移")
	}
	if me, _ := resp["models_exists"].(bool); !me {
		t.Fatal("models.yml 存在时 models_exists 应为 true")
	}
}

// 全新环境（USERPROFILE 下连 .omp/agent 都没有）不是错误：默认模型卡片应显示空
// 选择器，由前端提示「保存第一个提供商时会创建 models.yml」。之前这里直接把
// os.ReadFile 的 ENOENT 当成 READ_FAILED 抛给界面，首启用户会看到红色报错。
func TestGetOMPModelRolesNoModelsFile(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	a.currentAgent = string(core.AgentOMP)

	resp := a.GetOMPModelRoles()
	requireSuccess(t, resp, "GetOMPModelRoles(no models file)")
	if got, _ := resp["models_path"].(string); got != filepath.Join(dir, "models.yml") {
		t.Fatalf("models_path = %q，期望 models.yml", got)
	}
	if me, _ := resp["models_exists"].(bool); me {
		t.Fatal("模型文件不存在时 models_exists 应为 false")
	}
	if nm, _ := resp["needs_migration"].(bool); nm {
		t.Fatal("两个文件都不存在时不应提示迁移")
	}
	selectors, _ := resp["selectors"].([]string)
	if len(selectors) != 0 {
		t.Fatalf("没有 models.yml 时不应给出选择器：%#v", selectors)
	}
	if roles, _ := resp["roles"].(map[string]string); len(roles) != 0 {
		t.Fatalf("没有 config.yml 时 roles 应为空：%#v", roles)
	}
}

func TestGetOMPModelRolesNeedsMigration(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	writeText(t, filepath.Join(dir, "models.json"), `{"providers":{}}`)
	a.currentAgent = string(core.AgentOMP)

	resp := a.GetOMPModelRoles()
	requireSuccess(t, resp, "GetOMPModelRoles(models.json)")
	if nm, _ := resp["needs_migration"].(bool); !nm {
		t.Fatal("只有 models.json 时应提示需要迁移")
	}
	selectors, _ := resp["selectors"].([]string)
	if len(selectors) != 0 {
		t.Fatalf("迁移前不应给出选择器：%#v", selectors)
	}
}

func TestGetOMPModelRolesWrongAgent(t *testing.T) {
	a := sandboxApp(t)
	a.currentAgent = string(core.AgentZCode)

	resp := a.GetOMPModelRoles()
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("非 omp 不应成功：%v", resp)
	}
	if code, _ := resp["error_code"].(string); code != "WRONG_AGENT" {
		t.Fatalf("error_code = %q，期望 WRONG_AGENT", code)
	}
}

func TestSetOMPModelRoleUpdatesDefault(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	writeText(t, filepath.Join(dir, "models.yml"), ompModelsFixture)
	configPath := filepath.Join(dir, "config.yml")
	writeText(t, configPath, "modelRoles:\n  default: acme/m1\n")
	a.currentAgent = string(core.AgentOMP)

	resp := a.SetOMPModelRole("default", "acme/m2")
	requireSuccess(t, resp, "SetOMPModelRole")
	roles, _ := resp["roles"].(map[string]string)
	if roles["default"] != "acme/m2" {
		t.Fatalf("roles = %#v，期望 default=acme/m2", roles)
	}
	if got := readText(t, configPath); !strings.Contains(got, "acme/m2") {
		t.Fatalf("config.yml 应已写入新默认模型，实际：%s", got)
	}
}

func TestSetOMPModelRoleDeletesWhenEmpty(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	writeText(t, filepath.Join(dir, "models.yml"), ompModelsFixture)
	configPath := filepath.Join(dir, "config.yml")
	writeText(t, configPath, "modelRoles:\n  default: acme/m1\n")
	a.currentAgent = string(core.AgentOMP)

	resp := a.SetOMPModelRole("default", "")
	requireSuccess(t, resp, "SetOMPModelRole(delete)")
	roles, _ := resp["roles"].(map[string]string)
	if _, ok := roles["default"]; ok {
		t.Fatalf("空选择器应删除 default 键：%#v", roles)
	}
	if got := readText(t, configPath); strings.Contains(got, "default") {
		t.Fatalf("删除后不应残留 default 键：%s", got)
	}
}

func TestSetOMPModelRoleRejectsBadSelectors(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	writeText(t, filepath.Join(dir, "models.yml"), ompModelsFixture)
	configPath := filepath.Join(dir, "config.yml")
	writeText(t, configPath, "modelRoles:\n  default: acme/m1\n")
	a.currentAgent = string(core.AgentOMP)
	before := readText(t, configPath)

	for _, tc := range []struct {
		name     string
		selector string
	}{
		{"通配符", "acme/*"},
		{"@ 别名", "@fast"},
		{"不存在的模型", "acme/nope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := a.SetOMPModelRole("default", tc.selector)
			if ok, _ := resp["success"].(bool); ok {
				t.Fatalf("应拒绝选择器 %q：%v", tc.selector, resp)
			}
			if code, _ := resp["error_code"].(string); code != "BAD_SELECTOR" {
				t.Fatalf("error_code = %q，期望 BAD_SELECTOR", code)
			}
		})
	}
	if got := readText(t, configPath); got != before {
		t.Fatalf("被拒绝时不应改动 config.yml：%s", got)
	}
}

func TestSetOMPModelRoleRejectsBadRole(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	writeText(t, filepath.Join(dir, "models.yml"), ompModelsFixture)
	writeText(t, filepath.Join(dir, "config.yml"), "modelRoles:\n  default: acme/m1\n")
	a.currentAgent = string(core.AgentOMP)

	resp := a.SetOMPModelRole("fast", "acme/m1")
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("P1 只支持 default：%v", resp)
	}
	if code, _ := resp["error_code"].(string); code != "BAD_ROLE" {
		t.Fatalf("error_code = %q，期望 BAD_ROLE", code)
	}
}

// G18：config.yml 不存在但 settings.json 还在时，绝不能新建 config.yml。
func TestSetOMPModelRoleG18NeedsMigration(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	writeText(t, filepath.Join(dir, "models.yml"), ompModelsFixture)
	writeText(t, filepath.Join(dir, "settings.json"), `{"legacy":true}`)
	configPath := filepath.Join(dir, "config.yml")
	a.currentAgent = string(core.AgentOMP)

	resp := a.SetOMPModelRole("default", "acme/m1")
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("旧版设置未迁移时不应成功：%v", resp)
	}
	if code, _ := resp["error_code"].(string); code != "NEEDS_MIGRATION" {
		t.Fatalf("error_code = %q，期望 NEEDS_MIGRATION", code)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatal("APM 不得替 omp 新建 config.yml")
	}
}

// G7 双保险：config.yml 被错误地存成 provider 目标时必须拒绝。
func TestSetOMPModelRoleRejectsConfigAsTarget(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	writeText(t, filepath.Join(dir, "models.yml"), ompModelsFixture)
	configPath := filepath.Join(dir, "config.yml")
	writeText(t, configPath, "modelRoles:\n  default: acme/m1\n")
	a.currentAgent = string(core.AgentOMP)
	a.agentPaths = map[string]string{"omp": configPath}
	a.targetConfigPath = configPath

	resp := a.SetOMPModelRole("default", "acme/m2")
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("config.yml 不能当 provider 目标：%v", resp)
	}
	if code, _ := resp["error_code"].(string); code != "BAD_TARGET" {
		t.Fatalf("error_code = %q，期望 BAD_TARGET", code)
	}
}

func TestSetOMPModelRoleWrongAgent(t *testing.T) {
	a := sandboxApp(t)
	a.currentAgent = string(core.AgentDeepSeek)

	resp := a.SetOMPModelRole("default", "acme/m1")
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("非 omp 不应成功：%v", resp)
	}
	if code, _ := resp["error_code"].(string); code != "WRONG_AGENT" {
		t.Fatalf("error_code = %q，期望 WRONG_AGENT", code)
	}
}

// §9.1 嗅探：路径优先（omp 目录下的文件名），其次内容嗅探。
func TestConfigFileAgentDetectsOMP(t *testing.T) {
	_, dir := ompTestEnv(t)
	modelsPath := filepath.Join(dir, "models.yml")
	writeText(t, modelsPath, ompModelsFixture)
	configPath := filepath.Join(dir, "config.yml")
	writeText(t, configPath, "modelRoles:\n  default: acme/m1\n")

	if agent, format := configFileAgent(modelsPath); agent != string(core.AgentOMP) || format != "omp-models" {
		t.Fatalf("models.yml 应识别为 omp/omp-models，得到 %q/%q", agent, format)
	}
	if agent, _, reason := configFileAgentDetail(configPath); agent != "" || !strings.Contains(reason, "设置文件") {
		t.Fatalf("config.yml 不应作为 provider 文件，得到 agent=%q reason=%q", agent, reason)
	}

	// 不在 omp 目录，但内容形状是 models（YAML 根 mapping 含 providers）。
	loose := filepath.Join(t.TempDir(), "models-copy.yml")
	writeText(t, loose, ompModelsFixture)
	if agent, _ := configFileAgent(loose); agent != string(core.AgentOMP) {
		t.Fatalf("内容嗅探应识别 omp models，得到 %q", agent)
	}

	// ZCode v2 仍然走 core 的探测器。
	zcodePath := filepath.Join(t.TempDir(), "provider_config.json")
	writeText(t, zcodePath, v2ImportFixture)
	if agent, _ := configFileAgent(zcodePath); agent != string(core.AgentZCode) {
		t.Fatalf("ZCode v2 应仍识别为 zcode，得到 %q", agent)
	}
}

func TestDefaultPathForAgentOMP(t *testing.T) {
	_, dir := ompTestEnv(t)
	if got, want := defaultPathForAgent("omp"), filepath.Join(dir, "models.yml"); got != want {
		t.Fatalf("defaultPathForAgent(omp) = %q，期望 %q", got, want)
	}
}

// 启动升级只把存量 models.json 换成 models.yml；用户显式选择的其它路径不动。
func TestPreferModernConfigPathOMP(t *testing.T) {
	_, dir := ompTestEnv(t)
	if got, want := preferModernConfigPath("omp", filepath.Join(dir, "models.json")), filepath.Join(dir, "models.yml"); got != want {
		t.Fatalf("存量 models.json 应升级到 models.yml，得到 %q", got)
	}
	outside := filepath.Join(t.TempDir(), "models.json")
	if got := preferModernConfigPath("omp", outside); got != outside {
		t.Fatalf("omp 目录外的路径不应被改写，得到 %q", got)
	}
}

func TestUnderOMPDirsHonorsCodingAgentDir(t *testing.T) {
	ompTestEnv(t)
	agentDir := filepath.Join(t.TempDir(), "custom-agent")
	t.Setenv("PI_CODING_AGENT_DIR", agentDir)
	if !underOMPDirs(filepath.Join(agentDir, "models.yml")) {
		t.Fatal("PI_CODING_AGENT_DIR 下的文件应视为 omp 文件")
	}
	if underOMPDirs(filepath.Join(t.TempDir(), "models.yml")) {
		t.Fatal("无关路径不应视为 omp 文件")
	}
}

func TestFetchOptionsFromMap(t *testing.T) {
	if opt, apiType := fetchOptionsFromMap(nil); apiType != "" || len(opt.Headers) != 0 || opt.Kind != "" || opt.AuthHeader != nil {
		t.Fatalf("nil opts 应得到零值，得到 %#v / apiType=%q", opt, apiType)
	}
	opt, apiType := fetchOptionsFromMap(map[string]interface{}{
		"headers":     map[string]interface{}{"X-Test": "1", "Bad": 2},
		"kind":        "anthropic",
		"auth_header": true,
	})
	if apiType != "anthropic" || opt.Kind != "anthropic" {
		t.Fatalf("kind 应归一为 anthropic，得到 %q / %q", opt.Kind, apiType)
	}
	if len(opt.Headers) != 1 || opt.Headers["X-Test"] != "1" {
		t.Fatalf("headers 只应收字符串值，得到 %#v", opt.Headers)
	}
	if opt.AuthHeader == nil || !*opt.AuthHeader {
		t.Fatalf("auth_header=true 应透传为 *bool，得到 %#v", opt.AuthHeader)
	}
	if _, apiType := fetchOptionsFromMap(map[string]interface{}{"kind": "bogus"}); apiType != "" {
		t.Fatalf("未知 kind 的 apiType 应为空串（fail-closed），得到 %q", apiType)
	}
}

func TestProviderRequestOptions(t *testing.T) {
	edit := &core.ProviderEdit{
		Kind:        "anthropic",
		OptionsJSON: `{"headers":{"X-Test":"1"},"authHeader":true}`,
	}
	ompOpts := providerRequestOptions(edit, string(core.AgentOMP))
	if ompOpts["kind"] != "anthropic" || ompOpts["auth_header"] != true {
		t.Fatalf("omp 应透传 kind 与 auth_header：%#v", ompOpts)
	}
	if headers, _ := ompOpts["headers"].(map[string]interface{}); headers["X-Test"] != "1" {
		t.Fatalf("omp 应透传 headers：%#v", ompOpts)
	}
	zcodeOpts := providerRequestOptions(edit, string(core.AgentZCode))
	if _, ok := zcodeOpts["headers"]; ok {
		t.Fatalf("非 omp 不应透传 headers：%#v", zcodeOpts)
	}
	if _, ok := zcodeOpts["auth_header"]; ok {
		t.Fatalf("非 omp 不应透传 auth_header：%#v", zcodeOpts)
	}
	if providerRequestOptions(nil, string(core.AgentOMP)) != nil {
		t.Fatal("nil edit 应返回 nil opts")
	}
}

// 只读 provider 允许整体删除，但保存必须被前置检查拦下并给出中文原因（§4.7）。
func TestSaveProviderRejectsReadOnlyOMPProvider(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	target := filepath.Join(dir, "models.yml")
	writeText(t, target, ompModelsFixture)
	a.currentAgent = string(core.AgentOMP)
	a.targetConfigPath = target
	before := readText(t, target)

	resp := a.SaveProvider("weird", map[string]interface{}{
		"id":       "weird",
		"name":     "Weird Renamed",
		"kind":     "openai-compatible",
		"base_url": "https://weird.example.invalid/v1",
		"api_key":  "sk-placeholder",
		"cards":    []core.ModelCard{{ModelID: "m4"}},
	})
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("只读 provider 不应保存成功：%v", resp)
	}
	if code, _ := resp["error_code"].(string); code != "READ_ONLY" {
		t.Fatalf("error_code = %q，期望 READ_ONLY", code)
	}
	if msg, _ := resp["error"].(string); !strings.Contains(msg, "只读") {
		t.Fatalf("应给出中文只读原因，得到 %q", msg)
	}
	if got := readText(t, target); got != before {
		t.Fatal("被拒绝时不应改动 models.yml")
	}
}

// PreviewConfigMerge 必须能直接吃 omp 的 models.yml：agent 与 format 都要对上，
// 否则合并页会把 omp 当未知文件拒绝。
func TestPreviewConfigMergeOMP(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	modelsPath := filepath.Join(dir, "models.yml")
	writeText(t, modelsPath, ompModelsFixture)

	resp := a.PreviewConfigMerge(modelsPath)
	requireSuccess(t, resp, "PreviewConfigMerge(omp)")
	if agent, _ := resp["agent"].(string); agent != string(core.AgentOMP) {
		t.Fatalf("agent = %q，期望 omp", agent)
	}
	if format, _ := resp["format"].(string); format != "omp-models" {
		t.Fatalf("format = %q，期望 omp-models", format)
	}
	providers, _ := resp["providers"].([]core.ProviderSummary)
	if len(providers) != 3 {
		t.Fatalf("providers = %d，期望 3", len(providers))
	}
}

// omp 下 ZCode 专属绑定必须返回 WRONG_AGENT，而不是静默失败（计划 §9）。
func TestZCodeOnlyBindingsRejectOMP(t *testing.T) {
	a := sandboxApp(t)
	a.currentAgent = string(core.AgentOMP)

	cases := []struct {
		name string
		call func() map[string]interface{}
	}{
		{"GetZCodeFormat", a.GetZCodeFormat},
		{"GetZCodeCompat", a.GetZCodeCompat},
		{"RepairZCodeCompat", a.RepairZCodeCompat},
		{"PreviewLegacyImport", a.PreviewLegacyImport},
		{"ApplyLegacyImport", func() map[string]interface{} { return a.ApplyLegacyImport(nil) }},
		{"ImportOpencode", func() map[string]interface{} { return a.ImportOpencode(nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := tc.call()
			if ok, _ := resp["success"].(bool); ok {
				t.Fatalf("omp 下 %s 不应成功：%v", tc.name, resp)
			}
			if code, _ := resp["error_code"].(string); code != "WRONG_AGENT" {
				t.Fatalf("%s error_code = %q，期望 WRONG_AGENT", tc.name, code)
			}
			if msg, _ := resp["error"].(string); !strings.Contains(msg, "omp") {
				t.Fatalf("%s 应给出中文 omp 原因，得到 %q", tc.name, msg)
			}
		})
	}
}

// omp 普通 provider 的保存要真正落到 models.yml（backend 分发链路贯通）。
func TestSaveProviderOMPWritesModels(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	target := filepath.Join(dir, "models.yml")
	writeText(t, target, ompModelsFixture)
	a.currentAgent = string(core.AgentOMP)
	a.targetConfigPath = target

	resp := a.SaveProvider("acme", map[string]interface{}{
		"id":               "acme",
		"name":             "Acme Updated",
		"kind":             "openai-compatible",
		"base_url":         "https://acme-updated.example.invalid/v1",
		"api_key":          "sk-placeholder",
		"api_key_required": true,
		"cards":            []core.ModelCard{{ModelID: "m1"}, {ModelID: "m2"}},
	})
	requireSuccess(t, resp, "SaveProvider(omp)")

	info := a.GetProvider("acme")
	requireSuccess(t, info, "GetProvider(omp)")
	edit, _ := info["provider"].(*core.ProviderEdit)
	if edit == nil || edit.BaseURL != "https://acme-updated.example.invalid/v1" {
		t.Fatalf("保存后 BaseURL 未更新：%#v", info["provider"])
	}
	if got := readText(t, target); !strings.Contains(got, "acme-updated.example.invalid") {
		t.Fatalf("models.yml 未写入新 BaseURL：%s", got)
	}
}

// 首启场景：~/.omp/agent 还不存在时，保存第一个提供商必须能创建 models.yml，
// 之后默认模型卡片立刻能读到新的选择器（对应界面上的「还没有 models.yml」空状态）。
func TestSaveProviderOMPCreatesModelsFile(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	a.currentAgent = string(core.AgentOMP)
	// NewApp 在构造时按当时的 USERPROFILE 解析过默认目标；测试中途换了沙箱目录，
	// 这里把目标对齐到 ompTestEnv 的目录（真实使用中两者本来就是同一条路径）。
	a.targetConfigPath = filepath.Join(dir, "models.yml")

	resp := a.SaveProvider("fresh", map[string]interface{}{
		"id":               "fresh",
		"name":             "Fresh",
		"kind":             "openai-compatible",
		"base_url":         "https://fresh.example.invalid/v1",
		"api_key":          "sk-placeholder",
		"api_key_required": true,
		"cards":            []core.ModelCard{{ModelID: "m9"}},
	})
	requireSuccess(t, resp, "SaveProvider(omp, first provider)")
	if _, err := os.Stat(filepath.Join(dir, "models.yml")); err != nil {
		t.Fatalf("保存后应创建 models.yml：%v", err)
	}

	roles := a.GetOMPModelRoles()
	requireSuccess(t, roles, "GetOMPModelRoles(after first save)")
	selectors, _ := roles["selectors"].([]string)
	if !reflect.DeepEqual(selectors, []string{"fresh/m9"}) {
		t.Fatalf("selectors = %#v，期望 fresh/m9", selectors)
	}
	if me, _ := roles["models_exists"].(bool); !me {
		t.Fatal("保存后 models_exists 应为 true")
	}
}

// ompBackupFixture 覆盖导出投影最关键的两类剔除：协议不可建模（weird）与
// OAuth 托管密钥（oauthp，P1 不写）；acme 是唯一可以跨 Agent 导出的条目。
// 三者都必须满足 omp 自己的 schema，否则写盘前的 Validate 会拒绝整份文件。
const ompBackupFixture = `providers:
  acme:
    baseUrl: https://acme.example.invalid/v1
    api: openai-completions
    apiKey: sk-placeholder
    models:
      - id: m1
  weird:
    baseUrl: https://weird.example.invalid/v1
    api: google-generative-ai
    auth: none
    models:
      - id: m4
  oauthp:
    baseUrl: https://oauthp.example.invalid/v1
    api: openai-completions
    auth: oauth
    models:
      - id: m6
`

// 备份导出必须先做 omp 投影（只留跨 Agent 安全的字段），再统一剥离凭据；
// 只读 provider 不能硬塞成破损条目，要带中文原因回给前端（计划 §7.4 / G12）。
func TestExportBackupOMPProjectsReadOnlySkipped(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	target := filepath.Join(dir, "models.yml")
	writeText(t, target, ompBackupFixture)
	a.currentAgent = string(core.AgentOMP)
	a.targetConfigPath = target

	backupPath := filepath.Join(t.TempDir(), "omp-backup.json")
	resp := a.ExportBackup(map[string]interface{}{"path": backupPath, "include_keys": false})
	requireSuccess(t, resp, "ExportBackup(omp)")

	if got, _ := resp["provider_count"].(int); got != 1 {
		t.Fatalf("provider_count = %v，期望 1（只读条目已剔除）", resp["provider_count"])
	}
	skipped, _ := resp["skipped"].([]core.OMPBackupSkip)
	if len(skipped) != 2 {
		t.Fatalf("skipped = %#v，期望 2 条", skipped)
	}
	seen := map[string]string{}
	for _, s := range skipped {
		if strings.TrimSpace(s.Reason) == "" {
			t.Fatalf("skipped 必须带中文原因：%#v", s)
		}
		if s.ID == "acme" {
			t.Fatalf("可导出的 provider 不应出现在 skipped：%#v", skipped)
		}
		seen[s.ID] = s.Reason
	}
	for _, id := range []string{"weird", "oauthp"} {
		if _, ok := seen[id]; !ok {
			t.Fatalf("skipped 缺少 %s：%#v", id, skipped)
		}
	}
	// 投影后的备份里只剩 acme，且没有被剥离的私有结构。
	pkg, err := core.OpenBackup([]byte(readText(t, backupPath)), "")
	if err != nil {
		t.Fatalf("打开刚导出的备份失败：%v", err)
	}
	if len(pkg.Providers) != 1 || pkg.Providers[0].ID != "acme" {
		t.Fatalf("备份内容 = %#v，期望只含 acme", pkg.Providers)
	}
	if pkg.Providers[0].RawProvider != nil {
		t.Fatal("备份不应包含 omp 私有 provider 原文")
	}
}

// guardedTargetPath 是 ChooseConfigFile / SetConfigPath 共用的 G7 闸门，做成纯函数以便单测。
func TestGuardedTargetPath(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		agent   string
		path    string
		wantErr bool
	}{
		{"omp config.yml", "omp", filepath.Join(dir, "config.yml"), true},
		{"omp config.yaml", string(core.AgentOMP), filepath.Join(dir, "config.yaml"), true},
		{"omp models.yml", "omp", filepath.Join(dir, "models.yml"), false},
		{"omp models.json", "omp", filepath.Join(dir, "models.json"), false},
		{"zcode config.json", string(core.AgentZCode), filepath.Join(dir, "config.json"), false},
		{"未知 agent 的 config.yml", "", filepath.Join(dir, "config.yml"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := guardedTargetPath(tc.agent, tc.path)
			if tc.wantErr {
				if err == nil {
					t.Fatal("应拒绝 omp 的 config.yml")
				}
				if !strings.Contains(err.Error(), "config.yml 不是模型配置文件") {
					t.Fatalf("错误文案应说明 config.yml 不是模型配置文件，得到 %q", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("不应拒绝 %s：%v", tc.path, err)
			}
		})
	}
}

// SetConfigPath 不能把 omp 的 config.yml 设成 targetConfigPath（G7）：即使文件存在，
// lastDialogPath 兜底也不得放行。
func TestSetConfigPathRejectsOMPConfigFile(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	cfgPath := filepath.Join(dir, "config.yml")
	writeText(t, cfgPath, "modelRoles:\n  default: acme/m1\n")
	a.currentAgent = string(core.AgentOMP)
	before := a.targetConfigPath

	resp := a.SetConfigPath(cfgPath)
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("omp 的 config.yml 不应被设为目标：%v", resp)
	}
	if msg, _ := resp["error"].(string); !strings.Contains(msg, "config.yml 不是模型配置文件") {
		t.Fatalf("应给出专用中文提示，得到 %q", msg)
	}
	if a.targetConfigPath != before {
		t.Fatalf("被拒绝后 targetConfigPath 不应改变，得到 %q（原本 %q）", a.targetConfigPath, before)
	}
}

// MigratePreview 直接返回 core 的结构，omp 作为源时 warnings / dropped_fields 必须已经就位
// （app 层不做二次映射）。
func TestMigratePreviewOMPCarriesWarnings(t *testing.T) {
	a := sandboxApp(t)
	_, dir := ompTestEnv(t)
	modelsPath := filepath.Join(dir, "models.yml")
	writeText(t, modelsPath, ompModelsFixture)
	a.currentAgent = string(core.AgentOMP)
	a.agentPaths[string(core.AgentOMP)] = modelsPath

	resp := a.MigratePreview(string(core.AgentOMP), string(core.AgentZCode))
	requireSuccess(t, resp, "MigratePreview(omp→zcode)")
	preview, _ := resp["preview"].(core.MigratePreview)
	if len(preview.Source.Warnings) == 0 {
		t.Fatal("omp 作为迁移源时必须带 warnings")
	}
	if len(preview.Source.DroppedFields) == 0 {
		t.Fatal("omp 作为迁移源时必须带 dropped_fields")
	}
}
