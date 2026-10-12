package app

import (
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
	"agentprovidermanager/internal/dshprovider"
)

// App 层的 DeepSeek Harness 0.2 集成测试：目标是一个 profile patch，
// 环境完全沙箱化（LOCALAPPDATA / USERPROFILE / DSH_HOME），不会读写真实用户配置。
const dshProfileFixture = `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        displayName: Acme
        apiKeyEnv: ACME_API_KEY
        api: openai-completions
        baseURL: https://acme.example.invalid/v1
        models:
          - id: acme-1
- id: tools
  config:
    mode: !!js process.env.DSH_TOOLS_MODE
`

func deepSeekProfileTarget(t *testing.T, a *App) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "dsh-home")
	t.Setenv("DSH_HOME", home)
	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, dshProfileFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentDeepSeek)
	return target
}

// 在管理页把已开启推理的模型关掉并保存，必须落成 reasoningEfforts: false，
// 且不能残留单数键 reasoningEffort（走真实的 GetProvider -> SaveProvider 往返）。
func TestSaveProviderTurnsReasoningOffInDeepSeekProfile(t *testing.T) {
	a := sandboxApp(t)
	home := filepath.Join(t.TempDir(), "dsh-home")
	t.Setenv("DSH_HOME", home)
	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        api: openai-completions
        baseURL: https://acme.example.invalid/v1
        models:
          - id: acme-1
            reasoningEfforts: {off: null, high: high, max: max}
            reasoningEffort: high
            contextWindow: 1000
`)
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target

	got := a.GetProvider("acme")
	requireSuccess(t, got, "GetProvider")
	pe, _ := got["provider"].(*core.ProviderEdit)
	if pe == nil || len(pe.Cards) != 1 {
		t.Fatalf("provider edit = %#v", pe)
	}
	card := pe.Cards[0]
	card.Reasoning = false
	off := false
	card.RawReasoningEnabled = &off
	card.Variants = []string{}
	card.DefaultVariant = ""

	resp := a.SaveProvider("acme", map[string]interface{}{
		"id": "acme", "name": pe.Name, "kind": pe.Kind,
		"base_url": pe.BaseURL, "api_key_required": false,
		"cards": []core.ModelCard{card},
	})
	requireSuccess(t, resp, "SaveProvider")

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
	mm, _ := models[0].(map[string]interface{})
	if mm["reasoningEfforts"] != false {
		t.Fatalf("关闭推理未落成 reasoningEfforts: false：%#v", mm)
	}
	if _, has := mm["reasoningEffort"]; has {
		t.Fatalf("显式关闭后不应残留 reasoningEffort：%#v", mm)
	}
}

func TestImportProviderIntoDeepSeekProfilePatch(t *testing.T) {
	a := sandboxApp(t)
	target := deepSeekProfileTarget(t, a)

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "newco",
		"provider_name": "NewCo",
		"base_url":      "https://newco.example.invalid/v1",
		"api_key":       "sk-test-2",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards": []core.ModelCard{
			{ModelID: "n1", Context: 2000, Output: 300},
		},
	})
	requireSuccess(t, resp, "ImportProvider")

	raw := readText(t, target)
	if !strings.Contains(raw, "!!js process.env.DSH_TOOLS_MODE") {
		t.Fatalf("patch 的 !!js 行必须保留：%s", raw)
	}
	patch, err := dshprovider.LoadPatch([]byte(raw))
	if err != nil {
		t.Fatalf("导入后的 patch 无法解析：%v", err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	if providers["acme"] == nil || providers["newco"] == nil {
		t.Fatalf("导入后 provider 不正确：%#v", providers)
	}
	newco, _ := providers["newco"].(map[string]interface{})
	if newco["apiKeyEnv"] != "NEWCO_API_KEY" {
		t.Fatalf("应写入 apiKeyEnv 引用：%#v", newco["apiKeyEnv"])
	}
	if _, inline := newco["apiKey"]; inline {
		t.Fatal("明文 API Key 不应写进 patch")
	}

	refs, err := dshprovider.LoadCredentialRefs(core.DeepSeekCredentialsPath())
	if err != nil {
		t.Fatalf("读取凭据失败：%v", err)
	}
	if refs["NEWCO_API_KEY"] != "sk-test-2" {
		t.Fatalf("凭据未写入 DSH 凭据文件：%#v", refs)
	}

	if len(pointsFor(t, target)) == 0 {
		t.Fatal("profile patch 的写入必须留下还原点")
	}

	list := a.ListProviders()
	requireSuccess(t, list, "ListProviders")
	if list["format"] != "profile-desktop" {
		t.Errorf("ListProviders 应报告 profile-desktop，实际 %v", list["format"])
	}
	summaries, _ := list["providers"].([]core.ProviderSummary)
	if len(summaries) != 2 {
		t.Fatalf("providers = %#v", list["providers"])
	}

	// 删除刚导入的模型与 provider，都必须落在同一个 patch 上。
	requireSuccess(t, a.DeleteModel("newco", "n1"), "DeleteModel")
	requireSuccess(t, a.DeleteProvider("newco"), "DeleteProvider")

	doc, err := core.GetBackend(string(core.AgentDeepSeek), target)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := doc.Read(target)
	if err != nil {
		t.Fatal(err)
	}
	remaining := doc.Store(parsed).List(parsed)
	if len(remaining) != 1 || remaining[0].ID != "acme" {
		t.Fatalf("删除后应只剩 acme：%#v", remaining)
	}
	raw2 := readText(t, target)
	if !strings.Contains(raw2, "!!js process.env.DSH_TOOLS_MODE") {
		t.Fatalf("删除后 !!js 行必须保留：%s", raw2)
	}
}

func TestImportProviderRefusesWhenHomePatchOverridesDeepSeek(t *testing.T) {
	a := sandboxApp(t)
	target := deepSeekProfileTarget(t, a)
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

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "newco",
		"provider_name": "NewCo",
		"base_url":      "https://newco.example.invalid/v1",
		"api_key":       "sk-test-2",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         []core.ModelCard{{ModelID: "n1"}},
	})
	if ok, _ := resp["success"].(bool); ok {
		t.Fatal("被 home 级 patch 覆盖时必须拒绝写入 profile patch")
	}
	errMsg, _ := resp["error"].(string)
	if !strings.Contains(errMsg, "home") {
		t.Fatalf("错误信息应说明 home 级覆盖：%v", resp["error"])
	}
	if strings.Contains(readText(t, target), "newco") {
		t.Fatal("被拒绝时不得改动 profile patch")
	}
}
