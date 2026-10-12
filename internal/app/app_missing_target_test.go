package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
	"agentprovidermanager/internal/dshprovider"
	"agentprovidermanager/internal/zcodeprovider"
)

// 全新 DSH 0.2 用户：profile patch 还不存在时导入 provider，
// 必须创建一份合法的 0.2 patch（顶层序列 + llm-pi-ai 条目），而不是写出旧版 settings 形状的 YAML。
func TestImportProviderCreatesProfilePatchWhenFileMissing(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))
	target := core.DeepSeekDesktopProfilePatch()
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "newco",
		"provider_name": "NewCo",
		"base_url":      "https://newco.example.invalid/v1",
		"api_key":       "sk-placeholder",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         []core.ModelCard{{ModelID: "n1", Context: 2000}},
	})
	requireSuccess(t, resp, "ImportProvider on missing profile patch")

	raw := readText(t, target)
	if strings.HasPrefix(strings.TrimSpace(raw), "llm-pi-ai:") {
		t.Fatalf("不应写出旧版 settings 形状的 YAML：%s", raw)
	}
	patch, err := dshprovider.LoadPatch([]byte(raw))
	if err != nil {
		t.Fatalf("新建的 patch 无法解析：%v，内容：%s", err, raw)
	}
	if !patch.HasLLMPiAIEntry() {
		t.Fatalf("应创建 llm-pi-ai 条目：%s", raw)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := providers["newco"].(map[string]interface{}); !ok {
		t.Fatalf("应写入 newco：%#v", providers)
	}
	refs, err := dshprovider.LoadCredentialRefs(core.DeepSeekCredentialsPath())
	if err != nil {
		t.Fatal(err)
	}
	if refs["NEWCO_API_KEY"] != "sk-placeholder" {
		t.Fatalf("凭据未写入：%#v", refs)
	}
}

// 全新 ZCode 用户：新版文件还不存在时导入 provider，必须写出一份合法 v2 文档。
func TestImportProviderCreatesV2FileWhenMissing(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = target

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "newco",
		"provider_name": "NewCo",
		"base_url":      "https://newco.example.invalid/v1",
		"api_key":       "sk-placeholder",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         []core.ModelCard{{ModelID: "n1", Context: 2000}},
	})
	requireSuccess(t, resp, "ImportProvider on missing v2 file")

	raw := readText(t, target)
	if _, err := zcodeprovider.Decode([]byte(raw)); err != nil {
		t.Fatalf("新建的 v2 文件必须严格合法：%v，内容：%s", err, raw)
	}
}

// 已存在但内容为空的 provider_config.json 也按新版处理，保存后必须成为合法 v2 文档。
func TestSaveProviderIntoEmptyV2File(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	writeText(t, target, "")
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = target

	resp := a.SaveProvider("acme", map[string]interface{}{
		"id":       "acme",
		"name":     "Acme",
		"kind":     "openai-compatible",
		"base_url": "https://acme.example.invalid/v1",
		"api_key":  "sk-placeholder",
		"cards":    []core.ModelCard{{ModelID: "m1", Context: 2000}},
	})
	requireSuccess(t, resp, "SaveProvider into empty v2 file")

	raw := readText(t, target)
	if _, ok := strings.CutPrefix(strings.TrimSpace(raw), "{"); ok {
		// 合法 v2 也是 JSON 对象，这里只做形状提示，真正的判定交给严格解码。
		_ = ok
	}
	cfg, err := zcodeprovider.Decode([]byte(raw))
	if err != nil {
		t.Fatalf("空文件保存后必须严格合法：%v，内容：%s", err, raw)
	}
	if cfg.ProviderRule("acme") == nil {
		t.Fatalf("应写入 acme：%#v", cfg.ProviderRules())
	}
}

// 目标文件不存在时 DeleteProvider / GetProvider 不应破坏文件，也不应崩溃。
func TestDeleteAndGetOnMissingV2File(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	a.currentAgent = string(core.AgentZCode)
	a.targetConfigPath = target

	if ok, _ := a.DeleteProvider("acme")["success"].(bool); ok {
		t.Fatal("不存在的文件里不应删除成功")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("失败的删除不应创建文件：%v", err)
	}
	getResp := a.GetProvider("acme")
	if ok, _ := getResp["success"].(bool); ok {
		t.Fatalf("不存在的 provider 不应读取成功：%v", getResp)
	}
	list := a.ListProviders()
	requireSuccess(t, list, "ListProviders on missing v2 file")
}

// 已存在但内容为空的 profile patch 也按 0.2 处理，导入后必须成为合法 patch。
func TestImportProviderIntoEmptyProfilePatch(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))
	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, "")
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "newco",
		"provider_name": "NewCo",
		"base_url":      "https://newco.example.invalid/v1",
		"api_key":       "sk-placeholder",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         []core.ModelCard{{ModelID: "n1"}},
	})
	requireSuccess(t, resp, "ImportProvider into empty profile patch")

	patch, err := dshprovider.LoadPatch([]byte(readText(t, target)))
	if err != nil {
		t.Fatalf("空 patch 保存后必须可解析：%v", err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := providers["newco"].(map[string]interface{}); !ok {
		t.Fatalf("应写入 newco：%#v", providers)
	}
}
