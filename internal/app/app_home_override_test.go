package app

import (
	"path/filepath"
	"testing"

	"agentprovidermanager/internal/core"
)

// 被 home 级 patch 覆盖时，写入注定失败：除了必须保持文件不变，
// 也不能留下无用的还原点与同级备份（否则会挤占配额、误导用户以为发生过写入）。
func TestRefusedHomeOverrideWriteLeavesNoArtifacts(t *testing.T) {
	a := sandboxApp(t)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "dsh-home"))
	target := core.DeepSeekDesktopProfilePatch()
	writeText(t, target, dshProfileFixture)
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
	a.currentAgent = string(core.AgentDeepSeek)
	a.targetConfigPath = target
	before := readText(t, target)

	importResp := a.ImportProvider(map[string]interface{}{
		"provider_id":   "newco",
		"provider_name": "NewCo",
		"base_url":      "https://newco.example.invalid/v1",
		"api_key":       "sk-placeholder",
		"kind":          "openai-compatible",
		"merge_models":  true,
		"cards":         []core.ModelCard{{ModelID: "n1"}},
	})
	if ok, _ := importResp["success"].(bool); ok {
		t.Fatalf("被 home 级 patch 覆盖时不应导入成功：%v", importResp)
	}
	if got := readText(t, target); got != before {
		t.Fatal("拒绝时不应改动 profile patch")
	}
	if pts := pointsFor(t, target); len(pts) != 0 {
		t.Fatalf("拒绝的写入不应留下还原点：%+v", pts)
	}
	if baks := core.ListBackups(target); len(baks) != 0 {
		t.Fatalf("拒绝的写入不应留下同级备份：%v", baks)
	}

	saveResp := a.SaveProvider("acme", map[string]interface{}{
		"id":       "acme",
		"name":     "Acme",
		"kind":     "openai-compatible",
		"base_url": "https://acme.example.invalid/v1",
		"api_key":  "sk-placeholder",
		"cards":    []core.ModelCard{{ModelID: "acme-1"}},
	})
	if ok, _ := saveResp["success"].(bool); ok {
		t.Fatalf("被 home 级 patch 覆盖时不应保存成功：%v", saveResp)
	}
	if pts := pointsFor(t, target); len(pts) != 0 {
		t.Fatalf("拒绝的保存不应留下还原点：%+v", pts)
	}
}
