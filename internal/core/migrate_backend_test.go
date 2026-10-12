package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/dshprovider"
	"agentprovidermanager/internal/zcodeprovider"
)

// setupMigrateTest 把 HOME 相关路径全部指向临时目录，
// 确保迁移测试既不读真实用户配置，也不写真实凭据。
func setupMigrateTest(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")
}

// writeV2FromLegacy 用 zcodeprovider.FromLegacy 构造一份合法 v2 配置并落盘。
func writeV2FromLegacy(t *testing.T, path string, legacy map[string]interface{}) {
	t.Helper()
	cfg, _, err := zcodeprovider.FromLegacy(legacy)
	if err != nil {
		t.Fatalf("FromLegacy：%v", err)
	}
	data, err := zcodeprovider.Encode(cfg)
	if err != nil {
		t.Fatalf("Encode：%v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func legacyZCodeProvider(name, baseURL, apiKey, kind string, models ...string) map[string]interface{} {
	modelMap := map[string]interface{}{}
	for _, m := range models {
		modelMap[m] = map[string]interface{}{"limit": map[string]interface{}{"context": 128000, "output": 4096}}
	}
	opts := map[string]interface{}{"baseURL": baseURL}
	if apiKey != "" {
		opts["apiKey"] = apiKey
	}
	return map[string]interface{}{"name": name, "kind": kind, "options": opts, "models": modelMap}
}

func TestMigrateZCodeV2ToDSHDesktopProfileMerge(t *testing.T) {
	setupMigrateTest(t)
	srcPath := filepath.Join(t.TempDir(), "provider_config.json")
	writeV2FromLegacy(t, srcPath, map[string]interface{}{"provider": map[string]interface{}{
		"demo": legacyZCodeProvider("Demo", "https://demo.example/v1", "sk-placeholder", "anthropic", "demo-model"),
	}})

	targetPath := DeepSeekDesktopProfilePatch()
	writeDSHProfile(t, targetPath, testDshDesktopPatch)

	result, err := MigrateExecuteAtPaths("zcode", "deepseek", srcPath, targetPath, []string{"demo"}, "merge")
	if err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	if result["success"] != true || result["migrated"] != 1 {
		t.Fatalf("result=%#v", result)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# Your patch layer for this dsh profile") {
		t.Fatalf("原 patch 注释被破坏：%s", text)
	}
	if !strings.Contains(text, "!!js process.env.DSH_TOOLS_MODE") {
		t.Fatalf("原 patch 的 !!js 行被破坏：%s", text)
	}
	patch, err := dshprovider.LoadPatch(data)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	if providers["acme"] == nil {
		t.Fatalf("merge 不应删除原有 provider：%#v", providers)
	}
	demo, _ := providers["demo"].(map[string]interface{})
	if demo == nil {
		t.Fatalf("新 provider 未写入：%#v", providers)
	}
	if demo["baseURL"] != "https://demo.example/v1" {
		t.Fatalf("baseURL=%#v", demo["baseURL"])
	}
	if demo["api"] != "anthropic-messages" {
		t.Fatalf("api=%#v", demo["api"])
	}
	if demo["apiKeyEnv"] != "DEMO_API_KEY" {
		t.Fatalf("apiKeyEnv=%#v", demo["apiKeyEnv"])
	}
	if _, inline := demo["apiKey"]; inline {
		t.Fatal("明文 Key 不应写进 patch")
	}
	models, _ := demo["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("models=%#v", demo["models"])
	}
	model, _ := models[0].(map[string]interface{})
	if model["id"] != "demo-model" || model["contextWindow"] != 128000 {
		t.Fatalf("model=%#v", model)
	}

	refs, err := dshprovider.LoadCredentialRefs(DeepSeekCredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if refs["DEMO_API_KEY"] != "sk-placeholder" {
		t.Fatalf("凭据引用未写入：%#v", refs)
	}
}

func TestMigrateDSHDesktopProfileToZCodeV2Merge(t *testing.T) {
	setupMigrateTest(t)
	srcPath := DeepSeekDesktopProfilePatch()
	writeDSHProfile(t, srcPath, testDshDesktopPatch)
	if err := dshprovider.SetCredentialRef(DeepSeekCredentialsPath(), "ACME_API_KEY", "sk-placeholder-acme"); err != nil {
		t.Fatal(err)
	}

	tgtPath := filepath.Join(t.TempDir(), "provider_config.json")
	writeV2FromLegacy(t, tgtPath, map[string]interface{}{"provider": map[string]interface{}{
		"keep": legacyZCodeProvider("Keep", "https://keep.example/v1", "", "openai-compatible", "keep-model"),
	}})

	result, err := MigrateExecuteAtPaths("deepseek", "zcode", srcPath, tgtPath, []string{"acme"}, "merge")
	if err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	if result["migrated"] != 1 {
		t.Fatalf("result=%#v", result)
	}

	data, err := os.ReadFile(tgtPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := zcodeprovider.Decode(data)
	if err != nil {
		t.Fatalf("生成的 provider_config.json 严格解码失败：%v", err)
	}
	rule := cfg.ProviderRule("acme")
	if rule == nil {
		t.Fatalf("acme 未写入：%s", data)
	}
	providerCfg := zcodeprovider.RuleConfig(rule)
	if got := zcodeprovider.ProviderAccessAPIKey(providerCfg); got != "sk-placeholder-acme" {
		t.Fatalf("apiKey 未落在 access 里：%q", got)
	}
	if got := zcodeprovider.ProviderAPIBaseURL(providerCfg); got != "https://acme.example/v1" {
		t.Fatalf("baseURL=%q", got)
	}
	foundModel := false
	for _, mr := range cfg.ProviderModelRules() {
		if zcodeprovider.ModelRuleProviderID(mr) == "acme" && zcodeprovider.ModelRuleModelID(mr) == "acme-1" {
			foundModel = true
		}
	}
	if !foundModel {
		t.Fatalf("acme 的 providerModelRules 缺失：%s", data)
	}
	if cfg.ProviderRule("keep") == nil {
		t.Fatalf("merge 不应删除目标已有 provider：%s", data)
	}
}

func TestMigrateDSHWebProfileToZCodeV2Overwrite(t *testing.T) {
	setupMigrateTest(t)
	srcPath := DeepSeekWebProfilePatch()
	writeDSHProfile(t, srcPath, `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      webprov:
        displayName: Web Provider
        baseURL: https://web.example/v1
        api: openai-completions
        models:
          - id: web-model
`)

	tgtPath := filepath.Join(t.TempDir(), "provider_config.json")
	writeV2FromLegacy(t, tgtPath, map[string]interface{}{"provider": map[string]interface{}{
		"old-one": legacyZCodeProvider("Old One", "https://old1.example/v1", "sk-old", "openai-compatible", "old-model"),
		"old-two": legacyZCodeProvider("Old Two", "https://old2.example/v1", "", "openai-compatible", "old-model-2"),
	}})

	result, err := MigrateExecuteAtPaths("deepseek", "zcode", srcPath, tgtPath, []string{"webprov"}, "overwrite")
	if err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	if result["migrated"] != 1 {
		t.Fatalf("result=%#v", result)
	}

	data, err := os.ReadFile(tgtPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := zcodeprovider.Decode(data)
	if err != nil {
		t.Fatalf("解码失败：%v", err)
	}
	ids := make([]string, 0)
	for _, rule := range cfg.ProviderRules() {
		ids = append(ids, zcodeprovider.RuleProviderID(rule))
	}
	if len(ids) != 1 || !strings.EqualFold(ids[0], "webprov") {
		t.Fatalf("overwrite 应只留迁移来的 provider，得到 %v：%s", ids, data)
	}
	for _, mr := range cfg.ProviderModelRules() {
		if pid := zcodeprovider.ModelRuleProviderID(mr); !strings.EqualFold(pid, "webprov") {
			t.Fatalf("overwrite 后仍残留旧模型规则：%v", pid)
		}
	}
}

func TestMigrateLegacyZCodeToDSHSettings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")

	srcPath := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, srcPath, `{"provider":{"demo":{"name":"Demo","kind":"openai-compatible","options":{"baseURL":"https://demo.example/v1","apiKey":"sk-placeholder"},"models":{"m1":{"limit":{"context":64000}}}}}}`)
	tgtPath := filepath.Join(home, ".dsh", "settings.yaml")
	if err := os.MkdirAll(filepath.Dir(tgtPath), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := MigrateExecuteAtPaths("zcode", "deepseek", srcPath, tgtPath, []string{"demo"}, "merge")
	if err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	if result["migrated"] != 1 {
		t.Fatalf("result=%#v", result)
	}
	cfg, err := DeepSeekLoadConfig(tgtPath)
	if err != nil {
		t.Fatal(err)
	}
	if DeepSeekProvidersMap(cfg)["demo"] == nil {
		t.Fatalf("DSH 旧版 settings 未写入 demo：%#v", cfg)
	}
}

func TestMigrateDSHSettingsToLegacyZCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")

	srcPath := filepath.Join(home, ".dsh", "settings.yaml")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, srcPath, `llm-pi-ai:
  providers:
    demo:
      displayName: Demo
      baseURL: https://demo.example/v1
      api: openai-completions
      models:
        - id: m1
`)
	tgtPath := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, tgtPath, `{"provider":{}}`)

	result, err := MigrateExecuteAtPaths("deepseek", "zcode", srcPath, tgtPath, []string{"demo"}, "merge")
	if err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	if result["migrated"] != 1 {
		t.Fatalf("result=%#v", result)
	}
	cfg, err := LoadConfig(tgtPath)
	if err != nil {
		t.Fatal(err)
	}
	providers, _ := cfg["provider"].(map[string]interface{})
	if providers["demo"] == nil {
		t.Fatalf("旧版 config.json 未写入 demo：%#v", cfg)
	}
}

func TestMigratePreviewHandlesMissingTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")

	srcPath := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, srcPath, `{"provider":{"a":{"name":"A"},"b":{"name":"B"}}}`)

	tgtProfile := DeepSeekDesktopProfilePatch()
	preview := MigratePreviewAtPaths("zcode", "deepseek", srcPath, tgtProfile)
	if preview.Source.Error != "" || preview.Target.Error != "" {
		t.Fatalf("缺失目标不应报错：source=%q target=%q", preview.Source.Error, preview.Target.Error)
	}
	if len(preview.Source.Providers) != 2 {
		t.Fatalf("source providers=%#v", preview.Source.Providers)
	}
	if preview.Source.Format != "legacy" {
		t.Fatalf("source format=%q", preview.Source.Format)
	}
	if preview.Target.Exists {
		t.Fatal("目标不应存在")
	}
	if preview.Target.Providers == nil || len(preview.Target.Providers) != 0 {
		t.Fatalf("target providers=%#v", preview.Target.Providers)
	}
	if preview.Target.Format != "profile-desktop" {
		t.Fatalf("target format=%q", preview.Target.Format)
	}

	missingV2 := filepath.Join(home, ".zcode", "v2", "provider_config.json")
	preview2 := MigratePreviewAtPaths("deepseek", "zcode", tgtProfile, missingV2)
	if preview2.Target.Error != "" {
		t.Fatalf("缺失 v2 目标不应报错：%q", preview2.Target.Error)
	}
	if preview2.Target.Exists {
		t.Fatal("目标 v2 不应存在")
	}
	if preview2.Target.Format != "v2" {
		t.Fatalf("缺失 v2 目标 format=%q", preview2.Target.Format)
	}
}

func TestMigrateDSHProfileHomeOverrideError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("DSH_HOME", filepath.Join(home, ".dsh"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("ZCODE_PERSONAL_PROVIDER_CONFIG_FILE", "")

	srcPath := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, srcPath, `{"provider":{"demo":{"name":"Demo","kind":"openai-compatible","options":{"baseURL":"https://demo.example/v1"},"models":{"m1":{}}}}}`)

	targetPath := DeepSeekDesktopProfilePatch()
	writeDSHProfile(t, targetPath, "[]")
	writeDSHProfile(t, DeepSeekHomePatch(), `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      home-route:
        baseURL: https://home.example
        api: openai-completions
        models:
          - id: home-1
`)

	if _, err := MigrateExecuteAtPaths("zcode", "deepseek", srcPath, targetPath, []string{"demo"}, "merge"); !errors.Is(err, errDeepSeekHomeOverride) {
		t.Fatalf("merge 应原样透传 home 覆盖错误，得到 %v", err)
	} else if !strings.Contains(err.Error(), "覆盖") {
		t.Fatalf("错误文案应面向用户：%v", err)
	}
	if _, err := MigrateExecuteAtPaths("zcode", "deepseek", srcPath, targetPath, []string{"demo"}, "overwrite"); !errors.Is(err, errDeepSeekHomeOverride) {
		t.Fatalf("overwrite 也应透传 home 覆盖错误，得到 %v", err)
	}
}

func TestMigrateSamePathRejected(t *testing.T) {
	setupMigrateTest(t)
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, `{"provider":{"demo":{"name":"Demo"}}}`)

	if _, err := MigrateExecuteAtPaths("zcode", "deepseek", path, path, []string{"demo"}, "merge"); err == nil || !strings.Contains(err.Error(), "相同") {
		t.Fatalf("同一路径应报错，得到 %v", err)
	}
	upper := strings.ToUpper(path)
	if _, err := MigrateExecuteAtPaths("zcode", "deepseek", path, upper, []string{"demo"}, "merge"); err == nil || !strings.Contains(err.Error(), "相同") {
		t.Fatalf("大小写不同但同一路径也应报错，得到 %v", err)
	}
	preview := MigratePreviewAtPaths("zcode", "deepseek", path, upper)
	if preview.Source.Error == "" && preview.Target.Error == "" {
		t.Fatal("预览也应提示源与目标路径相同")
	}
}

func TestMigrateOpenCodeUnsupported(t *testing.T) {
	setupMigrateTest(t)
	src := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, src, `{"provider":{"demo":{"name":"Demo"}}}`)

	if _, err := MigrateExecuteAtPaths("opencode", "zcode", src, src, []string{"demo"}, "merge"); err == nil || !strings.Contains(err.Error(), "OpenCode") {
		t.Fatalf("源为 opencode 应报错，得到 %v", err)
	}
	if _, err := MigrateExecuteAtPaths("zcode", "opencode", src, src, []string{"demo"}, "merge"); err == nil || !strings.Contains(err.Error(), "OpenCode") {
		t.Fatalf("目标为 opencode 应报错，得到 %v", err)
	}
	preview := MigratePreviewAtPaths("opencode", "zcode", src, src)
	if !strings.Contains(preview.Source.Error, "OpenCode") {
		t.Fatalf("预览源错误缺少 OpenCode：%#v", preview.Source)
	}
}

func TestMigrateMergeKeepsTargetAPIKey(t *testing.T) {
	setupMigrateTest(t)
	srcPath := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, srcPath, `{"provider":{"demo":{"name":"Demo","kind":"openai-compatible","options":{"baseURL":"https://demo.example/v1"},"models":{"m1":{}}}}}`)

	targetPath := DeepSeekDesktopProfilePatch()
	writeDSHProfile(t, targetPath, `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      demo:
        displayName: Demo
        apiKeyEnv: DEMO_API_KEY
        baseURL: https://old.example/v1
        api: openai-completions
        models:
          - id: existing-model
`)
	if err := dshprovider.SetCredentialRef(DeepSeekCredentialsPath(), "DEMO_API_KEY", "sk-target-key"); err != nil {
		t.Fatal(err)
	}

	if _, err := MigrateExecuteAtPaths("zcode", "deepseek", srcPath, targetPath, []string{"demo"}, "merge"); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	refs, err := dshprovider.LoadCredentialRefs(DeepSeekCredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if refs["DEMO_API_KEY"] != "sk-target-key" {
		t.Fatalf("源无 Key 时不应抹掉目标 Key：%#v", refs)
	}
	data, _ := os.ReadFile(targetPath)
	patch, err := dshprovider.LoadPatch(data)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	demo, _ := providers["demo"].(map[string]interface{})
	if demo["apiKeyEnv"] != "DEMO_API_KEY" {
		t.Fatalf("apiKeyEnv=%#v", demo["apiKeyEnv"])
	}
	models, _ := demo["models"].([]interface{})
	if len(models) != 2 {
		t.Fatalf("merge 应保留目标已有模型并追加源模型：%#v", demo["models"])
	}
}

func TestMigratePreviewDefaultTargetNotOpenCode(t *testing.T) {
	setupMigrateTest(t)
	if p := MigratePreviewForAgents("", ""); p.Source.Agent != "zcode" || p.Target.Agent != "deepseek" {
		t.Fatalf("空参数默认应为 zcode -> deepseek，得到 %q -> %q", p.Source.Agent, p.Target.Agent)
	}
	if p := MigratePreviewForAgents("deepseek", ""); p.Target.Agent != "zcode" {
		t.Fatalf("deepseek -> 空 的默认目标应为 zcode，得到 %q", p.Target.Agent)
	}
}
