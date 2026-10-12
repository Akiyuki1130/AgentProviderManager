package app

import (
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
	"agentprovidermanager/internal/dshprovider"
	"agentprovidermanager/internal/zcodeprovider"
)

// “选择一份配置文件合并进当前 Agent”必须支持 ZCode 新版与 DSH 0.2 两个方向。

func TestMergeConfigFromZCodeV2IntoDeepSeekProfile(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))

	src := filepath.Join(t.TempDir(), "provider_config.json")
	writeText(t, src, v2ImportFixture)
	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, dshProfileFixture)
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target

	preview := a.PreviewConfigMerge(src)
	requireSuccess(t, preview, "PreviewConfigMerge")
	if preview["agent"] != string(core.AgentZCode) {
		t.Fatalf("应识别为 ZCode 配置，实际 %#v", preview["agent"])
	}
	providers, _ := preview["providers"].([]core.ProviderSummary)
	if len(providers) != 1 || providers[0].ID != "acme" {
		t.Fatalf("预览应列出 acme：%#v", preview["providers"])
	}

	resp := a.MergeConfig(map[string]interface{}{
		"path":         src,
		"selected_ids": []interface{}{"acme"},
		"merge":        true,
	})
	requireSuccess(t, resp, "MergeConfig")

	raw := readText(t, target)
	if !strings.Contains(raw, "!!js process.env.DSH_TOOLS_MODE") {
		t.Fatalf("合并不应破坏 patch 的 !!js 行：%s", raw)
	}
	patch, err := dshprovider.LoadPatch([]byte(raw))
	if err != nil {
		t.Fatalf("合并后的 patch 无法解析：%v", err)
	}
	merged, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	acme, _ := merged["acme"].(map[string]interface{})
	if acme == nil {
		t.Fatalf("合并后缺少 acme：%#v", merged)
	}
	models, _ := acme["models"].([]interface{})
	if len(models) != 2 {
		t.Fatalf("merge 模式应保留原模型并追加新模型，实际 %#v", acme["models"])
	}
}

func TestMergeConfigFromDeepSeekProfileIntoZCodeV2(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))

	src := core.DeepSeekDesktopProfilePatch()
	writeText(t, src, dshProfileFixture)
	target := emptyV2Target(t, "other")
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = target

	preview := a.PreviewConfigMerge(src)
	requireSuccess(t, preview, "PreviewConfigMerge")
	if preview["agent"] != string(core.AgentDeepSeek) {
		t.Fatalf("应识别为 DeepSeek Harness 配置，实际 %#v", preview["agent"])
	}

	resp := a.MergeConfig(map[string]interface{}{
		"path":         src,
		"selected_ids": []interface{}{"acme"},
		"merge":        true,
	})
	requireSuccess(t, resp, "MergeConfig")

	cfg, err := zcodeprovider.Decode([]byte(readText(t, target)))
	if err != nil {
		t.Fatalf("合并后的 v2 配置必须能严格解码：%v", err)
	}
	if cfg.ProviderRule("acme") == nil {
		t.Fatalf("合并后缺少 acme：%#v", cfg.ProviderRules())
	}
	if countV2ModelRules(cfg, "acme") != 1 {
		t.Errorf("acme 的模型规则数应为 1，实际 %d", countV2ModelRules(cfg, "acme"))
	}
	if cfg.ProviderRule("other") == nil {
		t.Error("merge 模式不应删除目标原有供应商")
	}
}

// MergeConfig 与迁移一样必须携带可透传的额外字段：v2 的 api.headers 合并进 DSH 后
// 应变成 route 的 headers，而不是被静默丢弃。
func TestMergeConfigCarriesV2HeadersIntoDeepSeekProfile(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))

	src := core.ZCodeProviderConfigPath()
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
	writeText(t, src, string(encoded))

	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, dshProfileFixture)
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target

	resp := a.MergeConfig(map[string]interface{}{
		"path":         src,
		"selected_ids": []interface{}{"acme"},
		"merge":        true,
	})
	requireSuccess(t, resp, "MergeConfig")

	patch, err := dshprovider.LoadPatch([]byte(readText(t, target)))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	acme, _ := providers["acme"].(map[string]interface{})
	if acme == nil {
		t.Fatalf("合并后缺少 acme：%#v", providers)
	}
	headers, _ := acme["headers"].(map[string]interface{})
	if headers["X-Custom"] != "v" {
		t.Fatalf("MergeConfig 丢失 v2 headers：%#v", acme)
	}
}

// 合并时不能被“整体重编码”误伤：目标里原本没有推理信息的模型，不能凭空多出
// reasoningEfforts: false —— 那是 DSH 的“显式关闭推理”标记，会把推理关掉。
func TestMergeConfigDoesNotDisableReasoningOnUntouchedModels(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))

	// 目标 patch 里的 acme-1 没有 reasoningEfforts；源与它同名同模型。
	src := filepath.Join(t.TempDir(), "provider_config.json")
	writeText(t, src, v2ImportFixture)
	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, dshProfileFixture)
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target

	resp := a.MergeConfig(map[string]interface{}{
		"path":         src,
		"selected_ids": []interface{}{"acme"},
		"merge":        true,
	})
	requireSuccess(t, resp, "MergeConfig")

	patch, err := dshprovider.LoadPatch([]byte(readText(t, target)))
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	acme, _ := providers["acme"].(map[string]interface{})
	models, _ := acme["models"].([]interface{})
	for _, mv := range models {
		mm, _ := mv.(map[string]interface{})
		if mm["id"] != "acme-1" {
			continue
		}
		if re, has := mm["reasoningEfforts"]; has {
			t.Fatalf("未被改动的模型不应被写入 reasoningEfforts：%#v", re)
		}
		return
	}
	t.Fatalf("合并后缺少 acme-1：%#v", acme["models"])
}
