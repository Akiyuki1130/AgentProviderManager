package core

import (
	"strings"
	"testing"
)

// anthropic 分支以前的早退只 return nil，节点里写坏的 thinking 会原样留到 Validate，
// 导致整份 models.yml 被拒（连删除 provider 也无法自救）。归一化必须发生在协议分支之前。
func TestOMPUpsertAnthropicDropsBrokenThinking(t *testing.T) {
	cases := []struct {
		name     string
		thinking string
	}{
		{"空映射", "        thinking: {}\n"},
		{"缺 mode", "        thinking:\n          efforts: [low, high]\n"},
		{"标量", "        thinking: high\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sample := "providers:\n" +
				"  claude:\n" +
				"    api: anthropic-messages\n" +
				"    baseUrl: https://claude.example.com\n" +
				"    apiKey: sk-placeholder\n" +
				"    models:\n" +
				"      - id: c1\n" +
				"        reasoning: true\n" +
				tc.thinking
			doc, path := ompTestReadDoc(t, t.TempDir(), "models.yml", sample)
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
				t.Fatalf("anthropic 坏 thinking 写回失败：%v", err)
			}
			text, _ := ompTestSaveAndReload(t, path, doc)
			if strings.Contains(text, "thinking:") {
				t.Fatalf("坏 thinking 应被删键而不是原样保留：\n%s", text)
			}
			if !strings.Contains(text, "reasoning: true") {
				t.Fatalf("应保留 reasoning: true：\n%s", text)
			}
		})
	}
}

// 计划 §4.6：已有 mode 原样保留。mode: levels 的节点不能被混写成 efforts/defaultLevel，
// 否则 omp 会同时看到两套语义，且 defaultLevel 可能不在 levels 里。
func TestOMPUpsertPreservesLevelsThinkingMode(t *testing.T) {
	sample := "providers:\n" +
		"  acme:\n" +
		"    api: openai-completions\n" +
		"    baseUrl: https://acme.example.com\n" +
		"    apiKey: sk-placeholder\n" +
		"    models:\n" +
		"      - id: m1\n" +
		"        reasoning: true\n" +
		"        thinking:\n" +
		"          mode: levels\n" +
		"          levels: [low, high]\n" +
		"          minLevel: low\n" +
		"          maxLevel: high\n"
	doc, path := ompTestReadDoc(t, t.TempDir(), "models.yml", sample)
	payload := ProviderPayload{
		"id":               "acme",
		"kind":             "openai-compatible",
		"base_url":         "https://acme.example.com",
		"api_key":          "sk-placeholder",
		"api_key_required": true,
		"cards": []interface{}{map[string]interface{}{
			"model_id":        "m1",
			"reasoning":       true,
			"variants":        []interface{}{"minimal", "medium"},
			"default_variant": "minimal",
		}},
	}
	if _, err := (ompStore{}).Upsert(doc, "acme", payload, SaveOptions{MergeModels: true}); err != nil {
		t.Fatalf("mode: levels 写入失败：%v", err)
	}
	text, _ := ompTestSaveAndReload(t, path, doc)
	if !strings.Contains(text, "mode: levels") {
		t.Fatalf("原有 mode: levels 被改写：\n%s", text)
	}
	for _, forbidden := range []string{"efforts:", "defaultLevel:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("mode: levels 节点被混写 %s：\n%s", forbidden, text)
		}
	}
}

// reasoning 打开但没给档位时也会早退；此时既有的坏 thinking 同样必须先清掉，
// 否则删除/保存都会被 Validate 挡住。
func TestOMPUpsertReasoningWithoutVariantsDropsBrokenThinking(t *testing.T) {
	sample := "providers:\n" +
		"  acme:\n" +
		"    api: openai-completions\n" +
		"    baseUrl: https://acme.example.com\n" +
		"    apiKey: sk-placeholder\n" +
		"    models:\n" +
		"      - id: m1\n" +
		"        reasoning: true\n" +
		"        thinking:\n" +
		"          efforts: [low]\n"
	doc, path := ompTestReadDoc(t, t.TempDir(), "models.yml", sample)
	payload := ProviderPayload{
		"id":               "acme",
		"kind":             "openai-compatible",
		"base_url":         "https://acme.example.com",
		"api_key":          "sk-placeholder",
		"api_key_required": true,
		"cards": []interface{}{map[string]interface{}{
			"model_id":  "m1",
			"reasoning": true,
		}},
	}
	if _, err := (ompStore{}).Upsert(doc, "acme", payload, SaveOptions{MergeModels: true}); err != nil {
		t.Fatalf("无档位推理模型写回失败：%v", err)
	}
	text, _ := ompTestSaveAndReload(t, path, doc)
	if strings.Contains(text, "thinking:") {
		t.Fatalf("坏 thinking 应被删键：\n%s", text)
	}
}
