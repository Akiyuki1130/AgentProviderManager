package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ompTestEmptyOMPDoc 在临时目录里读一份 provider 为空的 models.yml。
func ompTestEmptyOMPDoc(t *testing.T) (Doc, string) {
	t.Helper()
	return ompTestReadDoc(t, t.TempDir(), "models.yml", "providers: {}\n")
}

// 新建的推理模型必须写出 thinking.mode。
//
// deepCopyMap(nil) 返回空 map 而不是 nil，曾经让 ompApplyCardReasoning 跳过
// mode 初始化，写出 {efforts: [...]} 这种缺 mode 的 thinking；omp 校验
// 「thinking.mode 不能为空」会让整次保存（新建 provider / 跨 Agent 迁移）失败。
func TestOMPUpsertNewReasoningModelWritesThinkingMode(t *testing.T) {
	doc, path := ompTestEmptyOMPDoc(t)
	payload := ProviderPayload{
		"id":               "fresh",
		"kind":             "openai-compatible",
		"base_url":         "https://fresh.example.com/v1",
		"api_key":          "sk-placeholder",
		"api_key_required": true,
		"cards": []interface{}{map[string]interface{}{
			"model_id":        "m1",
			"context":         128000,
			"output":          8192,
			"reasoning":       true,
			"variants":        []interface{}{"off", "low", "high"},
			"default_variant": "high",
		}},
	}
	if _, err := (ompStore{}).Upsert(doc, "fresh", payload, SaveOptions{MergeModels: true}); err != nil {
		t.Fatalf("新建推理模型写入失败：%v", err)
	}
	text, reloaded := ompTestSaveAndReload(t, path, doc)
	if !strings.Contains(text, "mode: effort") {
		t.Fatalf("thinking 缺少 mode: effort：\n%s", text)
	}
	edit, err := (ompStore{}).Get(reloaded, "fresh")
	if err != nil {
		t.Fatalf("Get(fresh) 失败：%v", err)
	}
	card := ompTestCardByID(t, edit.Cards, "m1")
	if !card.Reasoning || ompTestStrings(card.Variants) != "off,low,high" || card.DefaultVariant != "high" {
		t.Fatalf("m1 推理字段不符：%#v", card)
	}
}

// anthropic 的档位由 omp 推导：新建模型只写 reasoning:true，不写 thinking（计划 §4.6）。
func TestOMPUpsertNewReasoningModelAnthropicSkipsThinking(t *testing.T) {
	doc, path := ompTestEmptyOMPDoc(t)
	payload := ProviderPayload{
		"id":               "claude",
		"kind":             "anthropic",
		"base_url":         "https://claude.example.com",
		"api_key":          "sk-placeholder",
		"api_key_required": true,
		"cards": []interface{}{map[string]interface{}{
			"model_id":  "c1",
			"reasoning": true,
			"variants":  []interface{}{"low", "high"},
		}},
	}
	if _, err := (ompStore{}).Upsert(doc, "claude", payload, SaveOptions{MergeModels: true}); err != nil {
		t.Fatalf("anthropic 新建推理模型写入失败：%v", err)
	}
	text, _ := ompTestSaveAndReload(t, path, doc)
	if strings.Contains(text, "thinking:") {
		t.Fatalf("anthropic 新建模型不应写 thinking：\n%s", text)
	}
	if !strings.Contains(text, "reasoning: true") {
		t.Fatalf("anthropic 新建模型应写 reasoning: true：\n%s", text)
	}
}

// 文件里已有 thinking 但缺 mode（手工编辑 / 旧版本写坏）时，写回必须补齐 mode，
// 否则保存会被 Validate 拒绝。
func TestOMPUpsertBackfillsMissingThinkingMode(t *testing.T) {
	sample := "providers:\n" +
		"  legacy:\n" +
		"    api: openai-completions\n" +
		"    baseUrl: https://legacy.example.com\n" +
		"    apiKey: sk-placeholder\n" +
		"    models:\n" +
		"      - id: legacy-1\n" +
		"        reasoning: true\n" +
		"        thinking:\n" +
		"          efforts: [low, high]\n"
	doc, path := ompTestReadDoc(t, t.TempDir(), "models.yml", sample)
	payload := ProviderPayload{
		"id":               "legacy",
		"kind":             "openai-compatible",
		"base_url":         "https://legacy.example.com",
		"api_key":          "sk-placeholder",
		"api_key_required": true,
		"cards": []interface{}{map[string]interface{}{
			"model_id":        "legacy-1",
			"reasoning":       true,
			"variants":        []interface{}{"low", "high"},
			"default_variant": "high",
		}},
	}
	if _, err := (ompStore{}).Upsert(doc, "legacy", payload, SaveOptions{MergeModels: true}); err != nil {
		t.Fatalf("缺 mode 的 thinking 写回失败：%v", err)
	}
	text, reloaded := ompTestSaveAndReload(t, path, doc)
	if !strings.Contains(text, "mode: effort") {
		t.Fatalf("写回后仍缺 mode: effort：\n%s", text)
	}
	edit, err := (ompStore{}).Get(reloaded, "legacy")
	if err != nil {
		t.Fatalf("Get(legacy) 失败：%v", err)
	}
	if card := ompTestCardByID(t, edit.Cards, "legacy-1"); !card.Reasoning {
		t.Fatalf("legacy-1 推理状态丢失：%#v", card)
	}
}

// 复现用户报的迁移失败场景：DSH 的推理模型迁到 omp 目标（models.yml 尚不存在）。
func TestMigrateDeepSeekToOMPReasoningModel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	t.Setenv("USERPROFILE", dir)
	t.Setenv("DSH_HOME", filepath.Join(dir, "dsh-home"))

	sourcePath := filepath.Join(dir, "dsh.json")
	targetPath := filepath.Join(dir, "omp", "models.yml")
	source := map[string]interface{}{
		"llm-pi-ai": map[string]interface{}{"providers": map[string]interface{}{
			"new-provider-go": map[string]interface{}{
				"baseURL": "https://api.example.com/v1",
				"models": []interface{}{map[string]interface{}{
					"id":               "reasoner",
					"reasoningEfforts": map[string]interface{}{"off": nil, "low": "low", "high": "high"},
					"reasoningEffort":  "high",
				}},
			},
		}},
	}
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := MigrateExecuteAtPaths("deepseek", "omp", sourcePath, targetPath, []string{"new-provider-go"}, "merge")
	if err != nil {
		t.Fatalf("DSH→omp 迁移失败：%v", err)
	}
	if result["success"] != true || result["migrated"] != 1 {
		t.Fatalf("迁移结果不符：%#v", result)
	}

	out, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("读取目标 models.yml 失败：%v", err)
	}
	if !strings.Contains(string(out), "mode: effort") {
		t.Fatalf("迁移写出的 thinking 缺 mode：\n%s", out)
	}
	reloaded, err := ompBackend{}.Read(targetPath)
	if err != nil {
		t.Fatalf("重新读取目标 models.yml 失败：%v", err)
	}
	edit, err := (ompStore{}).Get(reloaded, "new-provider-go")
	if err != nil {
		t.Fatalf("Get(new-provider-go) 失败：%v", err)
	}
	card := ompTestCardByID(t, edit.Cards, "reasoner")
	if !card.Reasoning || ompTestStrings(card.Variants) != "off,low,high" || card.DefaultVariant != "high" {
		t.Fatalf("迁移后的 reasoner 推理字段不符：%#v", card)
	}
}
