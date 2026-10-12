package app

import (
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
	"agentprovidermanager/internal/dshprovider"
	"agentprovidermanager/internal/zcodeprovider"
)

// App 层的跨 Agent 迁移测试：ZCode（v2 新版）<-> DeepSeek Harness（0.2 profile patch）。
// 环境完全沙箱化，不读写真实用户配置。

func emptyV2Target(t *testing.T, providerIDs ...string) string {
	t.Helper()
	cfg := zcodeprovider.NewConfig()
	for _, id := range providerIDs {
		cfg.UpsertProviderRule(zcodeprovider.NewProviderRule(id, id, zcodeprovider.GroupStandardPersonal))
	}
	cfg.SetProviderOrder(providerIDs)
	data, err := zcodeprovider.Encode(cfg)
	if err != nil {
		t.Fatalf("构造 v2 配置失败：%v", err)
	}
	path := core.ZCodeProviderConfigPath()
	writeText(t, path, string(data))
	return path
}

func sandboxMigrateApp(t *testing.T) (*App, string, string) {
	t.Helper()
	a := sandboxApp(t)
	home := filepath.Join(t.TempDir(), "dsh-home")
	t.Setenv("DSH_HOME", home)
	dshPath := core.DeepSeekDesktopProfilePatch()
	writeText(t, dshPath, dshProfileFixture)
	return a, core.ZCodeProviderConfigPath(), dshPath
}

// 预览必须读取“当前选中的目标文件”，而不是各 Agent 的 legacy 默认路径。
func TestMigratePreviewUsesSelectedPaths(t *testing.T) {
	a, zcodePath, dshPath := sandboxMigrateApp(t)
	writeText(t, zcodePath, v2ImportFixture)
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = zcodePath
	a.agentPaths = map[string]string{string(core.AgentZCode): zcodePath, string(core.AgentDeepSeek): dshPath}

	resp := a.MigratePreview(string(core.AgentZCode), string(core.AgentDeepSeek))
	requireSuccess(t, resp, "MigratePreview")
	preview, _ := resp["preview"].(core.MigratePreview)
	if preview.Source.Path != zcodePath {
		t.Errorf("源侧路径应为当前选中的 %s，实际 %s", zcodePath, preview.Source.Path)
	}
	if !preview.Source.Exists || len(preview.Source.Providers) != 1 || preview.Source.Providers[0].ID != "acme" {
		t.Fatalf("源侧应列出 acme：%#v", preview.Source)
	}
	if preview.Target.Path != dshPath {
		t.Errorf("目标侧路径应为当前选中的 %s，实际 %s", dshPath, preview.Target.Path)
	}
	if !preview.Target.Exists || len(preview.Target.Providers) != 1 || preview.Target.Providers[0].ID != "acme" {
		t.Fatalf("目标侧应列出 acme：%#v", preview.Target)
	}
	if preview.Source.Format != "v2" {
		t.Errorf("源侧格式应为 v2，实际 %q", preview.Source.Format)
	}
	if !strings.HasPrefix(preview.Target.Format, "profile") {
		t.Errorf("目标侧格式应为 profile-*，实际 %q", preview.Target.Format)
	}
}

// ZCode v2 -> DSH 0.2 profile：patch 里必须出现迁移过来的 provider 与模型，且凭据落到凭据文件。
func TestMigrateZCodeV2ToDeepSeekProfile(t *testing.T) {
	a, zcodePath, dshPath := sandboxMigrateApp(t)
	writeText(t, zcodePath, v2ImportFixture)
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = zcodePath
	a.agentPaths = map[string]string{string(core.AgentZCode): zcodePath, string(core.AgentDeepSeek): dshPath}

	resp := a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentZCode),
		"target":       string(core.AgentDeepSeek),
		"selected_ids": []interface{}{"acme"},
		"mode":         "merge",
	})
	requireSuccess(t, resp, "MigrateExecute zcode->deepseek")

	raw := readText(t, dshPath)
	if !strings.Contains(raw, "!!js process.env.DSH_TOOLS_MODE") {
		t.Fatalf("迁移不应破坏 patch 的 !!js 行：%s", raw)
	}
	patch, err := dshprovider.LoadPatch([]byte(raw))
	if err != nil {
		t.Fatalf("迁移后的 patch 无法解析：%v", err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	acme, _ := providers["acme"].(map[string]interface{})
	if acme == nil {
		t.Fatalf("迁移后缺少 acme：%#v", providers)
	}
	// merge 模式：目标原有的 acme-1 保留，源里的 m1 追加进来。
	models, _ := acme["models"].([]interface{})
	if len(models) != 2 {
		t.Fatalf("acme 应合并出 2 个模型，实际 %#v", acme["models"])
	}
	ids := map[string]bool{}
	for _, raw := range models {
		if m, ok := raw.(map[string]interface{}); ok {
			ids[m["id"].(string)] = true
		}
	}
	if !ids["acme-1"] || !ids["m1"] {
		t.Fatalf("合并后的模型应为 acme-1 与 m1，实际 %#v", acme["models"])
	}
	if acme["baseURL"] != "https://acme.example.invalid/v1" {
		t.Errorf("baseURL 未迁移：%#v", acme["baseURL"])
	}
}

// DSH 0.2 profile -> ZCode v2：目标文件必须仍是合法 v2，且 provider 与模型规则齐备。
func TestMigrateDeepSeekProfileToZCodeV2(t *testing.T) {
	a, zcodePath, dshPath := sandboxMigrateApp(t)
	emptyV2Target(t, "other")
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = zcodePath
	a.agentPaths = map[string]string{string(core.AgentZCode): zcodePath, string(core.AgentDeepSeek): dshPath}

	resp := a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentDeepSeek),
		"target":       string(core.AgentZCode),
		"selected_ids": []interface{}{"acme"},
		"mode":         "merge",
	})
	requireSuccess(t, resp, "MigrateExecute deepseek->zcode")

	cfg, err := zcodeprovider.Decode([]byte(readText(t, zcodePath)))
	if err != nil {
		t.Fatalf("迁移后的 v2 配置必须能严格解码：%v", err)
	}
	if cfg.ProviderRule("acme") == nil {
		t.Fatalf("迁移后缺少 acme：%#v", cfg.ProviderRules())
	}
	if countV2ModelRules(cfg, "acme") != 1 {
		t.Errorf("acme 的模型规则数应为 1，实际 %d", countV2ModelRules(cfg, "acme"))
	}
	if cfg.ProviderRule("other") == nil {
		t.Error("merge 模式不应删除目标原有供应商")
	}
}

// 覆盖模式：先清空目标 provider 列表，再写入所选条目。
func TestMigrateOverwriteClearsTargetProviders(t *testing.T) {
	a, zcodePath, dshPath := sandboxMigrateApp(t)
	writeText(t, zcodePath, v2ImportFixture)
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = zcodePath
	a.agentPaths = map[string]string{string(core.AgentZCode): zcodePath, string(core.AgentDeepSeek): dshPath}

	resp := a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentZCode),
		"target":       string(core.AgentDeepSeek),
		"selected_ids": []interface{}{"acme"},
		"mode":         "overwrite",
	})
	requireSuccess(t, resp, "MigrateExecute overwrite")

	patch, err := dshprovider.LoadPatch([]byte(readText(t, dshPath)))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers["acme"] == nil {
		t.Fatalf("覆盖模式应只剩迁移来的 acme：%#v", providers)
	}
	acme, _ := providers["acme"].(map[string]interface{})
	models, _ := acme["models"].([]interface{})
	if len(models) != 1 {
		t.Fatalf("覆盖模式不应保留目标原有模型，实际 %#v", acme["models"])
	}
	if id, _ := models[0].(map[string]interface{})["id"].(string); id != "m1" {
		t.Fatalf("覆盖后模型应为源里的 m1，实际 %#v", models[0])
	}
	if got := pointsFor(t, dshPath); len(got) != 1 || got[0].Operation != core.OpMigrate {
		t.Fatalf("迁移应留下 1 个 migrate 还原点，实际 %+v", got)
	}
}

// 覆盖模式：ZCode 新版目标的原有 provider 被清空，且文件仍严格合法。
func TestMigrateOverwriteZCodeV2Target(t *testing.T) {
	a, zcodePath, dshPath := sandboxMigrateApp(t)
	emptyV2Target(t, "other")
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = zcodePath
	a.agentPaths = map[string]string{string(core.AgentZCode): zcodePath, string(core.AgentDeepSeek): dshPath}

	resp := a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentDeepSeek),
		"target":       string(core.AgentZCode),
		"selected_ids": []interface{}{"acme"},
		"mode":         "overwrite",
	})
	requireSuccess(t, resp, "MigrateExecute overwrite zcode")

	cfg, err := zcodeprovider.Decode([]byte(readText(t, zcodePath)))
	if err != nil {
		t.Fatalf("覆盖后的 v2 配置必须能严格解码：%v", err)
	}
	if cfg.ProviderRule("other") != nil {
		t.Error("覆盖模式应清空目标原有 provider")
	}
	if cfg.ProviderRule("acme") == nil {
		t.Fatal("覆盖模式应写入迁移来的 acme")
	}
}

// home 级 patch 覆盖 llm-pi-ai 时，迁移必须明确拒绝并保持文件不变。
func TestMigrateIntoDeepSeekRefusedWhenHomePatchOverrides(t *testing.T) {
	a, zcodePath, dshPath := sandboxMigrateApp(t)
	writeText(t, zcodePath, v2ImportFixture)
	writeText(t, core.DeepSeekHomePatch(), `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      home-route:
        baseURL: https://home.example.invalid
        api: openai-completions
        models:
          - id: home-1
`)
	before := readText(t, dshPath)
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = zcodePath
	a.agentPaths = map[string]string{string(core.AgentZCode): zcodePath, string(core.AgentDeepSeek): dshPath}

	resp := a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentZCode),
		"target":       string(core.AgentDeepSeek),
		"selected_ids": []interface{}{"acme"},
		"mode":         "merge",
	})
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("被 home 级 patch 覆盖时不应迁移成功：%v", resp)
	}
	msg, _ := resp["error"].(string)
	if !strings.Contains(msg, "覆盖") {
		t.Fatalf("错误信息应说明被 home 级 patch 覆盖，实际：%s", msg)
	}
	if got := readText(t, dshPath); got != before {
		t.Fatal("拒绝时不应改动 profile patch")
	}
}

// 保存的仍是旧版 settings.yaml、但 0.2 profile patch 已存在时，迁移必须写进 profile patch，
// 否则会出现“提示成功、DSH 却读不到”的情况。
func TestMigratePrefersNewFormatPathOverLegacySetting(t *testing.T) {
	a, zcodePath, dshPath := sandboxMigrateApp(t)
	writeText(t, zcodePath, v2ImportFixture)
	legacySettings := core.DeepSeekSettingsPath()
	writeText(t, legacySettings, "llm-pi-ai:\n  providers: {}\n")
	before := readText(t, legacySettings)

	// 模拟旧版本保存下来的历史路径：DSH 指向 settings.yaml，而 0.2 profile patch 已存在。
	s := a.loadSettings()
	s["agent"] = string(core.AgentZCode)
	s["agentPaths"] = map[string]interface{}{
		string(core.AgentZCode):     zcodePath,
		string(core.AgentDeepSeek): legacySettings,
	}
	if !a.saveSettings(s) {
		t.Fatal("写入测试设置失败")
	}
	a.loadAgentState()
	if got := a.agentPaths[string(core.AgentDeepSeek)]; got != dshPath {
		t.Fatalf("启动时应把历史 settings 路径升级为 0.2 profile patch，实际 %s", got)
	}

	resp := a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentZCode),
		"target":       string(core.AgentDeepSeek),
		"selected_ids": []interface{}{"acme"},
		"mode":         "merge",
	})
	requireSuccess(t, resp, "MigrateExecute with legacy stored path")
	if got := readText(t, legacySettings); got != before {
		t.Fatal("不应写入已被 0.2 取代的 settings.yaml")
	}
	patch, err := dshprovider.LoadPatch([]byte(readText(t, dshPath)))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	if providers["acme"] == nil {
		t.Fatalf("迁移应写入 profile patch：%#v", providers)
	}
}

// 全新 ZCode 用户：新版文件还不存在时从 DSH 迁移过来，
// 目标必须是一份合法的新版文档（文件名 provider_config.json 时按新版处理）。
func TestMigrateIntoMissingV2File(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))
	dshPath := core.DeepSeekDesktopProfilePatch()
	writeText(t, dshPath, dshProfileFixture)
	zcodePath := core.ZCodeProviderConfigPath()
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = dshPath
	a.agentPaths = map[string]string{string(core.AgentZCode): zcodePath, string(core.AgentDeepSeek): dshPath}

	resp := a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentDeepSeek),
		"target":       string(core.AgentZCode),
		"selected_ids": []interface{}{"acme"},
		"mode":         "merge",
	})
	requireSuccess(t, resp, "MigrateExecute into missing v2 file")

	raw := readText(t, zcodePath)
	if strings.Contains(raw, "\"provider\"") {
		t.Fatalf("不应写出旧版顶层 provider 映射：%s", raw)
	}
	cfg, err := zcodeprovider.Decode([]byte(raw))
	if err != nil {
		t.Fatalf("新建的 v2 文件必须严格合法：%v，内容：%s", err, raw)
	}
	if cfg.ProviderRule("acme") == nil {
		t.Fatalf("应写入 acme：%#v", cfg.ProviderRules())
	}
}

// v2 里那种自定义请求头属于“可表达字段”，跨 Agent 迁移不能把它丢掉。
func TestMigrateCarriesProviderHeadersBothWays(t *testing.T) {
	a, zcodePath, dshPath := sandboxMigrateApp(t)
	t.Setenv("DSH_HOME", filepath.Dir(filepath.Dir(filepath.Dir(dshPath))))

	// 目标 patch 先清空，避免与源同名 provider 合并造成干扰。
	writeText(t, dshPath, "[]\n")
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = zcodePath
	a.agentPaths = map[string]string{string(core.AgentZCode): zcodePath, string(core.AgentDeepSeek): dshPath}

	// 源：v2 provider 带 api.headers。
	cfg := zcodeprovider.NewConfig()
	rule := zcodeprovider.NewProviderRule("acme", "Acme", zcodeprovider.GroupStandardPersonal)
	ruleCfg := zcodeprovider.RuleConfig(rule)
	ruleCfg["api"] = map[string]interface{}{
		"type":    "openai-chat-completions",
		"baseUrl": "https://acme.example.invalid/v1",
		"headers": map[string]interface{}{"X-Custom": "v"},
	}
	ruleCfg["access"] = map[string]interface{}{"type": "api-key", "apiKey": "sk-placeholder"}
	cfg.UpsertProviderRule(rule)
	cfg.SetProviderOrder([]string{"acme"})
	cfg.UpsertProviderModelRule(zcodeprovider.NewProviderModelRule("acme", "m1"))
	encoded, err := zcodeprovider.Encode(cfg)
	if err != nil {
		t.Fatalf("构造带 headers 的 v2 配置失败：%v", err)
	}
	writeText(t, zcodePath, string(encoded))

	resp := a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentZCode),
		"target":       string(core.AgentDeepSeek),
		"selected_ids": []interface{}{"acme"},
		"mode":         "merge",
	})
	requireSuccess(t, resp, "MigrateExecute v2 -> dsh")

	patch, err := dshprovider.LoadPatch([]byte(readText(t, dshPath)))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	acme, _ := providers["acme"].(map[string]interface{})
	if acme == nil {
		t.Fatalf("迁移后缺少 acme：%#v", providers)
	}
	headers, _ := acme["headers"].(map[string]interface{})
	if headers["X-Custom"] != "v" {
		t.Fatalf("v2 -> DSH 迁移丢失 headers：%#v", acme)
	}

	// 反向：把带 headers 的 DSH provider 迁移回 v2。
	back := emptyV2Target(t, "other")
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = back
	a.agentPaths = map[string]string{string(core.AgentZCode): back, string(core.AgentDeepSeek): dshPath}

	resp = a.MigrateExecute(map[string]interface{}{
		"source":       string(core.AgentDeepSeek),
		"target":       string(core.AgentZCode),
		"selected_ids": []interface{}{"acme"},
		"mode":         "merge",
	})
	requireSuccess(t, resp, "MigrateExecute dsh -> v2")

	decoded, err := zcodeprovider.Decode([]byte(readText(t, back)))
	if err != nil {
		t.Fatalf("迁移后的 v2 配置必须严格合法：%v", err)
	}
	backRule := decoded.ProviderRule("acme")
	if backRule == nil {
		t.Fatal("迁移后缺少 acme")
	}
	backHeaders := zcodeprovider.ProviderAPIHeaders(zcodeprovider.RuleConfig(backRule))
	if backHeaders["X-Custom"] != "v" {
		t.Fatalf("DSH -> v2 迁移丢失 headers：%#v", zcodeprovider.RuleConfig(backRule))
	}
}
