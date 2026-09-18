package zcodeprovider

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// sampleV2JSON 是一份 canonical v2 个人文件，覆盖了本包建模与被刻意“不建模”的
// 合法字段（templateId、enabled、logo、visibility、headers、builtinModelIds、
// apiKeyManagementUrl、requiresMfjsToolSchema、optionSpecs.map 等）。
// 所有密钥均为占位值。
const sampleV2JSON = `{
  "schemaVersion": 1,
  "config": {
    "providerOrder": ["acme"],
    "providerConfigRules": {
      "providerRules": [
        {
          "providerId": "acme",
          "providerName": "Acme",
          "templateId": "tpl-placeholder",
          "enabled": true,
          "config": {
            "group": "standard-personal",
            "logo": "https://example.invalid/logo.png",
            "visibility": "visible",
            "access": {
              "type": "api-key",
              "apiKey": "sk-test",
              "apiKeyManagementUrl": "https://example.invalid/keys"
            },
            "api": {
              "type": "openai-chat-completions",
              "baseUrl": "https://api.example.invalid/v1",
              "headers": {"X-Trace": "on"}
            },
            "builtinModelIds": ["acme-builtin"],
            "personalModelIds": ["acme-large", "acme-small"],
            "modelOrder": ["acme-small", "acme-large"]
          }
        }
      ]
    },
    "modelConfigRules": {
      "providerModelRules": [
        {
          "providerId": "acme",
          "modelId": "acme-large",
          "config": {
            "enabled": true,
            "properties": {
              "requiresMfjsToolSchema": false,
              "contextWindow": 200000,
              "inputFormat": {
                "supportsText": true,
                "supportsImage": true,
                "supportsVideo": false,
                "supportsAudio": false,
                "supportsPdf": false
              },
              "outputFormat": {"supportsText": true},
              "supportsToolCall": true,
              "supportsJsonSchemaOutput": true,
              "supportsNativeWebSearch": false,
              "supportsMidConversationSystem": false
            },
            "optionSpecs": {
              "reasoningLevel": {"values": ["disabled", "enabled"]},
              "maxOutputTokens": {"max": 32000}
            }
          }
        },
        {
          "providerId": "acme",
          "modelId": "acme-small",
          "config": {
            "optionSpecs": {
              "reasoningLevel": {"values": ["low", "high", "max"], "map": "map-placeholder"},
              "maxOutputTokens": {"max": 8000, "map": "max-placeholder"}
            }
          }
        }
      ],
      "manualProviderModelRules": [
        {
          "providerId": "acme",
          "modelId": "acme-manual",
          "config": {"enabled": false}
        }
      ]
    }
  }
}`

func decodeJSONDoc(t *testing.T, data []byte) map[string]interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("JSON 解析失败：%v", err)
	}
	m, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("根节点不是对象：%T", v)
	}
	return m
}

func childMap(t *testing.T, m map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("缺少键 %q", key)
	}
	cm, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("键 %q 不是对象：%T", key, v)
	}
	return cm
}

func childSlice(t *testing.T, m map[string]interface{}, key string) []interface{} {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Fatalf("缺少键 %q", key)
	}
	cs, ok := v.([]interface{})
	if !ok {
		t.Fatalf("键 %q 不是数组：%T", key, v)
	}
	return cs
}

func TestDecodeEncodeRoundTrip(t *testing.T) {
	c1, err := Decode([]byte(sampleV2JSON))
	if err != nil {
		t.Fatalf("Decode 失败：%v", err)
	}
	out1, err := Encode(c1)
	if err != nil {
		t.Fatalf("Encode 失败：%v", err)
	}
	c2, err := Decode(out1)
	if err != nil {
		t.Fatalf("二次 Decode 失败：%v", err)
	}
	out2, err := Encode(c2)
	if err != nil {
		t.Fatalf("二次 Encode 失败：%v", err)
	}
	if !bytes.Equal(out1, out2) {
		t.Fatalf("往返后输出不稳定：\n--- 第一次 ---\n%s\n--- 第二次 ---\n%s", out1, out2)
	}
	if got := c1.SchemaVersion(); got != SchemaVersionV1 {
		t.Fatalf("SchemaVersion = %d, want %d", got, SchemaVersionV1)
	}
	if got, want := c1.ProviderOrder(), []string{"acme"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("ProviderOrder = %v, want %v", got, want)
	}
	// 编码输出必须是 2 空格缩进 + 末尾换行。
	if !bytes.HasSuffix(out1, []byte("}\n")) {
		t.Fatalf("编码输出缺少末尾换行：%q", out1[len(out1)-4:])
	}
}

// ⑥ 未建模字段在往返后仍存在（防数据丢失）。
func TestUnmodeledFieldsSurviveRoundTrip(t *testing.T) {
	c, err := Decode([]byte(sampleV2JSON))
	if err != nil {
		t.Fatalf("Decode 失败：%v", err)
	}
	encoded, err := Encode(c)
	if err != nil {
		t.Fatalf("Encode 失败：%v", err)
	}
	doc := decodeJSONDoc(t, encoded)

	cfg := childMap(t, doc, "config")
	rules := childSlice(t, childMap(t, cfg, "providerConfigRules"), "providerRules")
	if len(rules) != 1 {
		t.Fatalf("providerRules 长度 = %d, want 1", len(rules))
	}
	rule, ok := rules[0].(map[string]interface{})
	if !ok {
		t.Fatalf("providerRules[0] 不是对象")
	}
	if got := rule["templateId"]; got != "tpl-placeholder" {
		t.Errorf("templateId = %v, want tpl-placeholder", got)
	}
	if got := rule["enabled"]; got != true {
		t.Errorf("enabled = %v, want true", got)
	}
	pcfg := childMap(t, rule, "config")
	if got := pcfg["logo"]; got != "https://example.invalid/logo.png" {
		t.Errorf("logo = %v", got)
	}
	if got := pcfg["visibility"]; got != "visible" {
		t.Errorf("visibility = %v", got)
	}
	access := childMap(t, pcfg, "access")
	if got := access["apiKeyManagementUrl"]; got != "https://example.invalid/keys" {
		t.Errorf("apiKeyManagementUrl = %v", got)
	}
	if got := access["apiKey"]; got != "sk-test" {
		t.Errorf("apiKey = %v, want sk-test", got)
	}
	api := childMap(t, pcfg, "api")
	if got := childMap(t, api, "headers")["X-Trace"]; got != "on" {
		t.Errorf("headers.X-Trace = %v, want on", got)
	}
	if got := childSlice(t, pcfg, "builtinModelIds")[0]; got != "acme-builtin" {
		t.Errorf("builtinModelIds[0] = %v", got)
	}

	modelConfigRules := childMap(t, cfg, "modelConfigRules")
	modelRules := childSlice(t, modelConfigRules, "providerModelRules")
	if len(modelRules) != 2 {
		t.Fatalf("providerModelRules 长度 = %d, want 2", len(modelRules))
	}
	large := childMap(t, modelRules[0].(map[string]interface{}), "config")
	props := childMap(t, large, "properties")
	if got := props["requiresMfjsToolSchema"]; got != false {
		t.Errorf("requiresMfjsToolSchema = %v, want false", got)
	}
	if got := childMap(t, props, "inputFormat")["supportsImage"]; got != true {
		t.Errorf("inputFormat.supportsImage = %v, want true", got)
	}
	if got := childMap(t, props, "outputFormat")["supportsText"]; got != true {
		t.Errorf("outputFormat.supportsText = %v, want true", got)
	}
	small := childMap(t, modelRules[1].(map[string]interface{}), "config")
	specs := childMap(t, small, "optionSpecs")
	if got := childMap(t, specs, "reasoningLevel")["map"]; got != "map-placeholder" {
		t.Errorf("reasoningLevel.map = %v, want map-placeholder", got)
	}
	if got := childMap(t, specs, "maxOutputTokens")["map"]; got != "max-placeholder" {
		t.Errorf("maxOutputTokens.map = %v, want max-placeholder", got)
	}
	manual := childSlice(t, modelConfigRules, "manualProviderModelRules")
	if len(manual) != 1 {
		t.Fatalf("manualProviderModelRules 长度 = %d, want 1", len(manual))
	}
}

// 在临时目录内做一次真实文件往返，避免触碰任何真实用户配置。
func TestEncodeFileRoundTripInTempDir(t *testing.T) {
	c, err := Decode([]byte(sampleV2JSON))
	if err != nil {
		t.Fatalf("Decode 失败：%v", err)
	}
	if !c.SetProviderAPIKeyAccess("acme", "sk-test") {
		t.Fatalf("SetProviderAPIKeyAccess 失败")
	}
	encoded, err := Encode(c)
	if err != nil {
		t.Fatalf("Encode 失败：%v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "provider_config.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("写入临时文件失败：%v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取临时文件失败：%v", err)
	}
	reloaded, err := Decode(data)
	if err != nil {
		t.Fatalf("重新 Decode 失败：%v", err)
	}
	if got := ProviderAccessAPIKey(RuleConfig(reloaded.ProviderRule("acme"))); got != "sk-test" {
		t.Fatalf("apiKey = %q, want sk-test", got)
	}
}

func TestEncodeRejectsNonCanonicalDoc(t *testing.T) {
	c, err := Decode([]byte(sampleV2JSON))
	if err != nil {
		t.Fatalf("Decode 失败：%v", err)
	}
	c.Doc()["unknownKey"] = true
	if _, err := Encode(c); err == nil {
		t.Fatal("Encode 应拒绝含未知键的文档")
	}
}

func TestEffectiveOrders(t *testing.T) {
	c, err := Decode([]byte(sampleV2JSON))
	if err != nil {
		t.Fatalf("Decode 失败：%v", err)
	}
	// providerOrder 缺省等价于 []，存在的是追加到末尾，不存在的是静默丢弃。
	c.Doc()["config"].(map[string]interface{})["providerOrder"] = []interface{}{"missing", "acme"}
	if got := c.EffectiveProviderOrder(); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("EffectiveProviderOrder = %v, want [acme]", got)
	}
	// modelOrder 缺省时由 personalModelIds 推导。
	c.Doc()["config"].(map[string]interface{})["providerOrder"] = []interface{}{"acme"}
	pcfg := RuleConfig(c.ProviderRule("acme"))
	if got := EffectiveModelOrder(pcfg); len(got) != 2 || got[0] != "acme-small" || got[1] != "acme-large" {
		t.Fatalf("EffectiveModelOrder = %v, want [acme-small acme-large]", got)
	}
	delete(pcfg, "modelOrder")
	if got := EffectiveModelOrder(pcfg); len(got) != 2 || got[0] != "acme-large" || got[1] != "acme-small" {
		t.Fatalf("回退 personalModelIds 时 EffectiveModelOrder = %v, want [acme-large acme-small]", got)
	}
}
