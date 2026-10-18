package core

import (
	"os"
	"path/filepath"
	"testing"

	"agentprovidermanager/internal/dshprovider"
)

// TestDshReasoningProbeFromZCodeCards 是调查用探针（不影响任何生产路径）：
// 把不同形态的 ZCode 模型卡片经跨 Agent 迁移投到 DSH，打印写出的模型节点。
//
// 调查结论（2026-10-16）：DSH 的「推理等级」只有在模型节点带 reasoningEfforts 时才出现；
// 源卡片没有档位（或档位全被白名单滤掉）时，DSH 侧不会有任何推理信息。
func TestDshReasoningProbeFromZCodeCards(t *testing.T) {
	setupMigrateTest(t)
	srcPath := filepath.Join(t.TempDir(), "provider_config.json")

	writeV2FromLegacy(t, srcPath, map[string]interface{}{"provider": map[string]interface{}{
		"demo": map[string]interface{}{
			"name":    "Demo",
			"kind":    "openai-compatible",
			"options": map[string]interface{}{"baseURL": "https://demo.example/v1", "apiKey": "sk-placeholder"},
			"models": map[string]interface{}{
				// 源配置里完全没有 reasoningLevel（内建规则表提供档位时就是这个形态）。
				"m-no-levels": map[string]interface{}{
					"limit": map[string]interface{}{"context": 128000, "output": 4096},
				},
				"m-off-low-high-max": map[string]interface{}{
					"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"disabled", "low", "high", "max"}},
					"limit":     map[string]interface{}{"context": 1000000, "output": 384000},
				},
				"m-six-levels": map[string]interface{}{
					"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"off", "low", "medium", "high", "xhigh", "max"}},
				},
				"m-enabled-only": map[string]interface{}{
					"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"enabled"}},
				},
				"m-off-only": map[string]interface{}{
					"reasoning": map[string]interface{}{"enabled": false, "variants": []interface{}{"disabled"}},
				},
				"m-none-only": map[string]interface{}{
					"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"none"}},
				},
				"m-no-off": map[string]interface{}{
					"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"minimal", "low", "medium", "high"}},
				},
				// 自填档位词：不在 ZCode 白名单里，读卡时被滤掉（除非打开「允许自定义档位」）。
				"m-custom-words": map[string]interface{}{
					"reasoning": map[string]interface{}{"enabled": true, "variants": []interface{}{"thinking", "ultra"}},
				},
			},
		},
	}})

	targetPath := filepath.Join(t.TempDir(), "cordis.patch.yml")
	if _, err := MigrateExecuteAtPaths("zcode", "deepseek", srcPath, targetPath, []string{"demo"}, "overwrite"); err != nil {
		t.Fatalf("迁移失败：%v", err)
	}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("迁移后的 DSH profile patch：\n%s", string(data))

	patch, err := dshprovider.LoadPatch(data)
	if err != nil {
		t.Fatal(err)
	}
	providers, err := patch.Providers()
	if err != nil {
		t.Fatal(err)
	}
	demo, _ := providers["demo"].(map[string]interface{})
	if demo == nil {
		t.Fatalf("provider 未写入：%#v", providers)
	}
	list, _ := demo["models"].([]interface{})
	for _, e := range list {
		mm, _ := e.(map[string]interface{})
		t.Logf("%v → reasoningEfforts=%#v reasoningEffort=%#v", mm["id"], mm["reasoningEfforts"], mm["reasoningEffort"])
	}
}
