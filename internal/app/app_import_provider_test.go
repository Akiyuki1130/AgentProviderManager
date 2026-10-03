package app

import (
	"path/filepath"
	"strings"
	"testing"

	"agentprovidermanager/internal/core"
	"agentprovidermanager/internal/zcodeprovider"
)

// ImportProvider 对 ZCode 新版目标（provider_config.json）必须走 v2 backend。
//
// 回归背景：修复前该路径直接调用 core.MergeProviderIntoConfig + core.WriteConfig，
// 会把旧版的顶层 provider 映射合并进新版文档并原样落盘。ZCode 的 .strict() 校验随即
// 失败（ZodError: unrecognized_keys ["provider"]），ZCode 会把整份个人配置当成空，
// 所有供应商在使用端消失。
const v2ImportFixture = `{
  "schemaVersion": 1,
  "config": {
    "providerOrder": ["acme"],
    "providerConfigRules": {"providerRules": [
      {
        "providerId": "acme",
        "providerName": "Acme",
        "config": {
          "group": "standard-personal",
          "access": {"type": "api-key", "apiKey": "sk-test"},
          "api": {"type": "openai-chat-completions", "baseUrl": "https://acme.example.invalid/v1"},
          "personalModelIds": ["m1"],
          "modelOrder": ["m1"]
        }
      }
    ]},
    "modelConfigRules": {
      "providerModelRules": [
        {"providerId": "acme", "modelId": "m1", "config": {"properties": {"contextWindow": 1000}}}
      ],
      "manualProviderModelRules": []
    }
  }
}`

// 导入新供应商后：文件必须仍能通过 ZCode 的严格校验，且不能出现旧版顶层 provider 键。
func TestImportProviderV2NewProviderKeepsFileStrictlyValid(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	writeText(t, target, v2ImportFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "newco",
		"provider_name": "NewCo",
		"base_url":      "https://newco.example.invalid/v1",
		"api_key":       "sk-test-2",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards": []core.ModelCard{
			{ModelID: "n1", Context: 2000, Output: 300},
			{ModelID: "n2", Context: 4000},
		},
	})
	requireSuccess(t, resp, "ImportProvider")

	raw := readText(t, target)
	cfg, err := zcodeprovider.Decode([]byte(raw))
	if err != nil {
		t.Fatalf("导入后文件必须通过 ZCode 严格校验，实际失败：%v", err)
	}
	if _, ok := cfg.Doc()["provider"]; ok {
		t.Fatalf("新版文档里出现了旧版顶层 provider 映射，ZCode 会因此把整份配置当成空")
	}
	if _, err := zcodeprovider.Encode(cfg); err != nil {
		t.Fatalf("导入后的文档必须可再次编码：%v", err)
	}
	if cfg.ProviderRule("newco") == nil {
		t.Error("导入的供应商未写入 providerConfigRules")
	}
	if n := countV2ModelRules(cfg, "newco"); n != 2 {
		t.Errorf("导入的模型数应为 2，实际 %d", n)
	}
	if n := countV2ModelRules(cfg, "acme"); n != 1 {
		t.Errorf("原有供应商的模型不应受影响，期望 1，实际 %d", n)
	}
	if cfg.ProviderRule("acme") == nil {
		t.Error("原有供应商不应被导入动作删除")
	}

	points := pointsFor(t, target)
	if len(points) != 1 || points[0].Operation != core.OpImportProvider {
		t.Fatalf("导入应留下 1 个 import_provider 还原点，实际 %+v", points)
	}
	if points[0].Format != core.FormatV2 {
		t.Errorf("还原点格式应为 v2，实际 %q", points[0].Format)
	}
}

// merge_models=true：已有模型必须保留，新模型追加（复刻 legacy 合并语义）。
func TestImportProviderV2MergesExistingModels(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	writeText(t, target, v2ImportFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "acme",
		"provider_name": "Acme",
		"base_url":      "https://acme.example.invalid/v1",
		"api_key":       "sk-test",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         []core.ModelCard{{ModelID: "m2", Context: 5000}},
	})
	requireSuccess(t, resp, "ImportProvider")

	raw := readText(t, target)
	cfg, err := zcodeprovider.Decode([]byte(raw))
	if err != nil {
		t.Fatalf("合并导入后文件必须通过严格校验：%v", err)
	}
	if n := countV2ModelRules(cfg, "acme"); n != 2 {
		t.Errorf("merge_models=true 应保留 m1 并追加 m2，期望 2 条模型规则，实际 %d", n)
	}
	if !strings.Contains(raw, "\"m1\"") || !strings.Contains(raw, "\"m2\"") {
		t.Error("m1 与 m2 都应出现在文件中")
	}
}

// 旧版 ZCode 目标（config.json）行为不变：仍然写旧版的顶层 provider 映射。
func TestImportProviderLegacyTargetUnchanged(t *testing.T) {
	a := sandboxApp(t)
	target := filepath.Join(core.ZCodeDir(), "config.json")
	writeText(t, target, legacyProviderFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)

	resp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "beta",
		"provider_name": "Beta",
		"base_url":      "https://beta.example.invalid/v1",
		"api_key":       "sk-test",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         []core.ModelCard{{ModelID: "b1", Context: 1000}},
	})
	requireSuccess(t, resp, "ImportProvider")

	cfg, err := core.LoadConfig(target)
	if err != nil {
		t.Fatalf("读取旧版配置失败：%v", err)
	}
	providers, _ := cfg["provider"].(map[string]interface{})
	if providers == nil || providers["beta"] == nil {
		t.Fatal("旧版目标应仍然把供应商写进顶层 provider 映射")
	}
	if providers["acme"] == nil {
		t.Error("既有供应商不应被覆盖丢失")
	}
}

func countV2ModelRules(cfg *zcodeprovider.Config, providerID string) int {
	n := 0
	for _, mr := range cfg.ProviderModelRules() {
		if zcodeprovider.ModelRuleProviderID(mr) == providerID {
			n++
		}
	}
	return n
}
// ImportOpencode 与 MergeConfig 仍是纯旧版实现（只写顶层 provider 映射）。
// 对 v2 目标它们必须在写盘前失败：统一写盘闸门会拒绝写出 ZCode 不接受的文档，
// 而不是像以前那样把新版配置写坏（ZCode 会把整份配置当成空）。
func TestLegacyOnlyFlowsRefuseToCorruptV2Target(t *testing.T) {
	a := sandboxApp(t)
	target := core.ZCodeProviderConfigPath()
	writeText(t, target, v2ImportFixture)
	a.targetConfigPath = target
	a.currentAgent = string(core.AgentZCode)
	before := readText(t, target)

	opencodeSrc := filepath.Join(t.TempDir(), "opencode.json")
	writeText(t, opencodeSrc, `{"provider":{"src1":{"name":"Src","options":{"baseURL":"https://src.example.invalid/v1","apiKey":"sk-x"},"models":{"s1":{"name":"s1"}}}}}`)
	importResp := a.ImportOpencode(map[string]interface{}{
		"path":         opencodeSrc,
		"selected_ids": []interface{}{"src1"},
		"merge":        true,
	})
	if ok, _ := importResp["success"].(bool); ok {
		t.Fatalf("对 v2 目标不应导入成功：%v", importResp)
	}
	if msg, _ := importResp["error"].(string); !strings.Contains(msg, "拒绝写入") {
		t.Errorf("应以写盘闸门的理由失败，实际：%s", msg)
	}
	if got := readText(t, target); got != before {
		t.Fatal("导入失败时不应改动 v2 目标文件")
	}

	legacySrc := filepath.Join(t.TempDir(), "legacy-zcode.json")
	writeText(t, legacySrc, legacyProviderFixture)
	mergeResp := a.MergeConfig(map[string]interface{}{
		"path":         legacySrc,
		"selected_ids": []interface{}{"acme"},
		"merge":        true,
	})
	if ok, _ := mergeResp["success"].(bool); ok {
		t.Fatalf("对 v2 目标不应合并成功：%v", mergeResp)
	}
	if got := readText(t, target); got != before {
		t.Fatal("合并失败时不应改动 v2 目标文件")
	}
	if _, err := zcodeprovider.Decode([]byte(readText(t, target))); err != nil {
		t.Fatalf("v2 目标必须保持严格合法：%v", err)
	}
}
