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
