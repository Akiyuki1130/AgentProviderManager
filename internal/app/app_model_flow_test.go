package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
	"agentprovidermanager/internal/dshprovider"
)

// 这一组测试覆盖“获取新模型 -> 导入”链路在 DSH 0.2 profile patch 与 ZCode v2 目标上的行为。

// allowLocalFetch 打开 http 与私有地址开关，便于用 httptest 模拟 /models 接口。
func allowLocalFetch(t *testing.T) {
	t.Helper()
	core.SetHTTPAllowed(true)
	core.SetPrivateHostsAllowed(true)
	t.Cleanup(func() {
		core.SetHTTPAllowed(false)
		core.SetPrivateHostsAllowed(false)
	})
}

func modelsServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "model-b", "name": "Model B"},
				{"id": "model-a"},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func dshProfileHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "dsh-home")
	t.Setenv("DSH_HOME", home)
	return home
}

// 刷新模型：DSH profile 的 provider 只有 apiKeyEnv 引用，凭据在 .credentials.yaml 里，
// RefreshProviderModels 必须能解析出真实 Key 才能请求成功。
func TestRefreshProviderModelsDeepSeekProfileResolvesCredential(t *testing.T) {
	a := sandboxApp(t)
	srv := modelsServer(t)
	allowLocalFetch(t)
	dshProfileHome(t)

	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers:
      acme:
        displayName: Acme
        apiKeyEnv: ACME_API_KEY
        api: openai-completions
        baseURL: `+srv.URL+`
        models:
          - id: acme-1
`)
	if err := dshprovider.SetCredentialRef(core.DeepSeekCredentialsPath(), "ACME_API_KEY", "sk-placeholder"); err != nil {
		t.Fatalf("写入测试凭据失败：%v", err)
	}
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentDeepSeek)

	resp := a.RefreshProviderModels("acme", "", "")
	requireSuccess(t, resp, "RefreshProviderModels")
	if got, _ := resp["count"].(int); got != 2 {
		t.Fatalf("应获取到 2 个模型，实际 %#v", resp["count"])
	}
}

// 获取模型 -> 导入：DSH profile 目标必须写入 patch 的 models 列表与凭据文件。
func TestFetchThenImportIntoDeepSeekProfile(t *testing.T) {
	a := sandboxApp(t)
	srv := modelsServer(t)
	allowLocalFetch(t)
	dshProfileHome(t)

	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, `- id: llm-pi-ai
  name: '@deepseek-ai/dsh-llm-pi-ai'
  config:
    providers: {}
`)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentDeepSeek)

	fetched := a.FetchModels(srv.URL, "sk-placeholder")
	requireSuccess(t, fetched, "FetchModels")
	cards, _ := fetched["models"].([]core.ModelCard)
	if len(cards) != 2 {
		t.Fatalf("获取的模型数应为 2，实际 %d", len(cards))
	}

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "newco",
		"provider_name": "NewCo",
		"base_url":      srv.URL,
		"api_key":       "sk-placeholder",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         cards,
	})
	requireSuccess(t, resp, "ImportProvider")

	patch, err := dshprovider.LoadPatch([]byte(readText(t, target)))
	if err != nil {
		t.Fatalf("导入后的 patch 无法解析：%v", err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	newco, _ := providers["newco"].(map[string]interface{})
	if newco == nil {
		t.Fatalf("导入的 provider 缺失：%#v", providers)
	}
	models, _ := newco["models"].([]interface{})
	if len(models) != 2 {
		t.Fatalf("导入的模型数应为 2，实际 %#v", newco["models"])
	}
	refs, err := dshprovider.LoadCredentialRefs(core.DeepSeekCredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if refs["NEWCO_API_KEY"] != "sk-placeholder" {
		t.Fatalf("导入的 Key 应落到凭据文件：%#v", refs)
	}
}

// 刷新模型：ZCode v2 目标（access.apiKey + api.baseUrl）。
func TestRefreshProviderModelsZCodeV2(t *testing.T) {
	a := sandboxApp(t)
	srv := modelsServer(t)
	allowLocalFetch(t)

	target := core.ZCodeProviderConfigPath()
	writeText(t, target, strings.ReplaceAll(v2ImportFixture, "https://acme.example.invalid/v1", srv.URL))
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	resp := a.RefreshProviderModels("acme", "", "")
	requireSuccess(t, resp, "RefreshProviderModels")
	if got, _ := resp["count"].(int); got != 2 {
		t.Fatalf("应获取到 2 个模型，实际 %#v", resp["count"])
	}
}

// merge_models=true 时，返回的 count 应反映写入后的模型总数（含目标已有模型）。
func TestImportProviderCountReflectsMergedModels(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))
	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, "- id: llm-pi-ai\n  name: '@deepseek-ai/dsh-llm-pi-ai'\n  config:\n    providers:\n      acme:\n        baseURL: https://acme.example.invalid/v1\n        api: openai-completions\n        models:\n          - id: acme-1\n")
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "acme",
		"provider_name": "Acme",
		"base_url":      "https://acme.example.invalid/v1",
		"api_key":       "sk-placeholder",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         []core.ModelCard{{ModelID: "acme-2"}},
	})
	requireSuccess(t, resp, "ImportProvider merge")
	if got, _ := resp["count"].(int); got != 2 {
		t.Fatalf("count 应为合并后的 2，实际 %#v", resp["count"])
	}
}

// merge_models=false 且模型列表为空时，必须给出“避免误删已有模型”的专门提示（不是通用校验错误）。
func TestImportProviderEmptyCardsUsesSpecificMessage(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))
	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, "- id: llm-pi-ai\n  name: '@deepseek-ai/dsh-llm-pi-ai'\n  config:\n    providers:\n      acme:\n        baseURL: https://acme.example.invalid/v1\n        api: openai-completions\n        models:\n          - id: acme-1\n")
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target
	before := readText(t, target)

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "acme",
		"provider_name": "Acme",
		"base_url":      "https://acme.example.invalid/v1",
		"api_key":       "sk-placeholder",
		"kind":          "openai-compatible",
		"merge_models":  false,
		"cards":         []core.ModelCard{},
	})
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("空模型 + 覆盖模式不应导入成功：%v", resp)
	}
	msg, _ := resp["error"].(string)
	if !strings.Contains(msg, "已取消导入") {
		t.Fatalf("应给出避免误删已有模型的专门提示，实际：%s", msg)
	}
	if got := readText(t, target); got != before {
		t.Fatal("拒绝时不应改动目标文件")
	}
}

// 刷新模型：配置里尚不存在的 provider，只要带了 Base URL 覆盖就应能请求成功
// （新建 provider 在内存里即可“自动重新获取”，无需先保存）。
func TestRefreshProviderModelsUnknownProviderWithOverride(t *testing.T) {
	a := sandboxApp(t)
	srv := modelsServer(t)
	allowLocalFetch(t)

	target := core.ZCodeProviderConfigPath()
	writeText(t, target, strings.ReplaceAll(v2ImportFixture, "https://acme.example.invalid/v1", srv.URL))
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	// "brandnew" 不在配置里，但覆盖提供了可用的 Base URL。
	resp := a.RefreshProviderModels("brandnew", srv.URL, "sk-placeholder")
	requireSuccess(t, resp, "RefreshProviderModels(unknown + override)")
	if got, _ := resp["count"].(int); got != 2 {
		t.Fatalf("应获取到 2 个模型，实际 %#v", resp["count"])
	}
}

// 刷新模型：配置里不存在的 provider 且没有 Base URL 覆盖时，仍应报“提供商不存在”，
// 而不是一个误导性的网络/获取错误。
func TestRefreshProviderModelsUnknownProviderWithoutOverrideFails(t *testing.T) {
	a := sandboxApp(t)
	allowLocalFetch(t)

	target := core.ZCodeProviderConfigPath()
	writeText(t, target, v2ImportFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	resp := a.RefreshProviderModels("brandnew", "", "")
	if ok, _ := resp["success"].(bool); ok {
		t.Fatalf("缺少覆盖时不应成功：%v", resp)
	}
	msg, _ := resp["error"].(string)
	if !strings.Contains(msg, "不存在") || !strings.Contains(msg, "brandnew") {
		t.Fatalf("应是“提供商不存在”的解析错误，实际：%s", msg)
	}
}

// 回归：已存在的 provider 在不传覆盖时，仍按原样解析配置里的 Base URL 与凭据。
func TestRefreshProviderModelsExistingProviderUsesStoredConfig(t *testing.T) {
	a := sandboxApp(t)
	srv := modelsServer(t)
	allowLocalFetch(t)

	target := core.ZCodeProviderConfigPath()
	writeText(t, target, strings.ReplaceAll(v2ImportFixture, "https://acme.example.invalid/v1", srv.URL))
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	resp := a.RefreshProviderModels("acme", "", "")
	requireSuccess(t, resp, "RefreshProviderModels(existing)")
	if got, _ := resp["count"].(int); got != 2 {
		t.Fatalf("应获取到 2 个模型，实际 %#v", resp["count"])
	}
}

// SaveProvider 的 _operation 白名单：只有精确的 "auto_save" 才写进还原点，
// 其它值（含伪造的 "hack"）与缺省都保持 save_provider。
func TestSaveProviderAutoSaveOperationIsWhitelisted(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeConfig()
	writeText(t, target, legacyProviderFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	save := func(op interface{}, name string) {
		t.Helper()
		payload := map[string]interface{}{
			"id":       "acme",
			"name":     name,
			"kind":     "openai-compatible",
			"base_url": "https://acme.example.invalid/v1",
			"api_key":  "sk-test",
			"cards":    []core.ModelCard{{ModelID: "m1"}},
		}
		if op != nil {
			payload["_operation"] = op
		}
		requireSuccess(t, a.SaveProvider("acme", payload), "SaveProvider "+name)
	}

	save(core.OpAutoSave, "AutoSave")
	if points := pointsFor(t, target); len(points) == 0 || points[0].Operation != core.OpAutoSave {
		t.Fatalf("_operation=auto_save 应产生 auto_save 还原点：%+v", points)
	}

	save("hack", "Hack")
	if points := pointsFor(t, target); points[0].Operation != core.OpSaveProvider {
		t.Fatalf("伪造的 _operation=hack 应回落到 save_provider：%+v", points[0])
	}

	save(nil, "NoOp")
	if points := pointsFor(t, target); points[0].Operation != core.OpSaveProvider {
		t.Fatalf("缺省 _operation 应为 save_provider：%+v", points[0])
	}
}
