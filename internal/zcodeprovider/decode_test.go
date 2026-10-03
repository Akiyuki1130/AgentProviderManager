package zcodeprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

const minimalV2JSON = `{
  "schemaVersion": 1,
  "config": {
    "providerConfigRules": {"providerRules": []},
    "modelConfigRules": {"providerModelRules": [], "manualProviderModelRules": []}
  }
}`

func mutateSample(t *testing.T, mutate func(doc map[string]interface{})) []byte {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(sampleV2JSON), &v); err != nil {
		t.Fatalf("样例解析失败：%v", err)
	}
	doc, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("样例根节点不是对象")
	}
	mutate(doc)
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("样例重新序列化失败：%v", err)
	}
	return data
}

func firstProviderRuleConfig(doc map[string]interface{}) map[string]interface{} {
	cfg := doc["config"].(map[string]interface{})
	rules := cfg["providerConfigRules"].(map[string]interface{})["providerRules"].([]interface{})
	return rules[0].(map[string]interface{})["config"].(map[string]interface{})
}

func firstModelRuleConfig(doc map[string]interface{}) map[string]interface{} {
	cfg := doc["config"].(map[string]interface{})
	rules := cfg["modelConfigRules"].(map[string]interface{})["providerModelRules"].([]interface{})
	return rules[0].(map[string]interface{})["config"].(map[string]interface{})
}

func TestDecodeValidDocuments(t *testing.T) {
	for name, raw := range map[string]string{
		"完整样例": sampleV2JSON,
		"最小文档": minimalV2JSON,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode([]byte(raw)); err != nil {
				t.Fatalf("Decode 失败：%v", err)
			}
		})
	}
	t.Run("带 BOM", func(t *testing.T) {
		if _, err := Decode(append([]byte{0xEF, 0xBB, 0xBF}, []byte(minimalV2JSON)...)); err != nil {
			t.Fatalf("Decode 失败：%v", err)
		}
	})
	t.Run("可空字段为 null 时合法", func(t *testing.T) {
		// ZCode 3.14 里 group / api / access / logo / providerName / apiKey / baseUrl
		// 都是 nullable+optional，缺省或 null 都必须能解码。
		cases := []func(doc map[string]interface{}){
			func(doc map[string]interface{}) { firstProviderRuleConfig(doc)["group"] = nil },
			func(doc map[string]interface{}) { delete(firstProviderRuleConfig(doc), "group") },
			func(doc map[string]interface{}) { firstProviderRuleConfig(doc)["api"] = nil },
			func(doc map[string]interface{}) { firstProviderRuleConfig(doc)["access"] = nil },
			func(doc map[string]interface{}) { firstProviderRuleConfig(doc)["logo"] = nil },
			func(doc map[string]interface{}) { firstProviderRuleConfig(doc)["visibility"] = nil },
			func(doc map[string]interface{}) {
				rules := doc["config"].(map[string]interface{})["providerConfigRules"].(map[string]interface{})["providerRules"].([]interface{})
				rules[0].(map[string]interface{})["providerName"] = nil
			},
			func(doc map[string]interface{}) {
				api := firstProviderRuleConfig(doc)["api"].(map[string]interface{})
				api["baseUrl"] = nil
			},
			func(doc map[string]interface{}) {
				delete(firstProviderRuleConfig(doc)["api"].(map[string]interface{}), "baseUrl")
			},
			func(doc map[string]interface{}) {
				firstProviderRuleConfig(doc)["access"].(map[string]interface{})["apiKey"] = nil
			},
		}
		for i, mutate := range cases {
			data := mutateSample(t, mutate)
			if _, err := Decode(data); err != nil {
				t.Fatalf("第 %d 个可空用例 Decode 失败：%v", i+1, err)
			}
		}
	})
}

// ② schemaVersion 缺失与超版本（>1）被拒绝。
func TestDecodeSchemaVersion(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"缺失", []byte(strings.Replace(minimalV2JSON, `"schemaVersion": 1,`, "", 1))},
		{"超版本 2", mutateSample(t, func(doc map[string]interface{}) { doc["schemaVersion"] = 2 })},
		{"版本 0", mutateSample(t, func(doc map[string]interface{}) { doc["schemaVersion"] = 0 })},
		{"负数版本", mutateSample(t, func(doc map[string]interface{}) { doc["schemaVersion"] = -1 })},
		{"字符串版本", mutateSample(t, func(doc map[string]interface{}) { doc["schemaVersion"] = "1" })},
		{"小数版本", mutateSample(t, func(doc map[string]interface{}) { doc["schemaVersion"] = 1.5 })},
		{"缺少 config", []byte(`{"schemaVersion": 1}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode(tc.data); err == nil {
				t.Fatal("Decode 应失败，但成功了")
			}
		})
	}
}

func TestDecodeRejectsUnknownKeys(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"顶层未知键", mutateSample(t, func(doc map[string]interface{}) { doc["extra"] = 1 })},
		{"config 未知键", mutateSample(t, func(doc map[string]interface{}) {
			doc["config"].(map[string]interface{})["extra"] = 1
		})},
		{"providerRule 未知键", mutateSample(t, func(doc map[string]interface{}) {
			cfg := doc["config"].(map[string]interface{})
			rules := cfg["providerConfigRules"].(map[string]interface{})["providerRules"].([]interface{})
			rules[0].(map[string]interface{})["extra"] = 1
		})},
		{"provider config 未知键", mutateSample(t, func(doc map[string]interface{}) {
			firstProviderRuleConfig(doc)["extra"] = 1
		})},
		{"api 未知键", mutateSample(t, func(doc map[string]interface{}) {
			firstProviderRuleConfig(doc)["api"].(map[string]interface{})["extra"] = 1
		})},
		{"access api-key 未知键", mutateSample(t, func(doc map[string]interface{}) {
			firstProviderRuleConfig(doc)["access"].(map[string]interface{})["extra"] = 1
		})},
		{"modelRule 未知键", mutateSample(t, func(doc map[string]interface{}) {
			cfg := doc["config"].(map[string]interface{})
			rules := cfg["modelConfigRules"].(map[string]interface{})["providerModelRules"].([]interface{})
			rules[0].(map[string]interface{})["extra"] = 1
		})},
		{"model config 未知键", mutateSample(t, func(doc map[string]interface{}) {
			firstModelRuleConfig(doc)["extra"] = 1
		})},
		{"properties 未知键", mutateSample(t, func(doc map[string]interface{}) {
			firstModelRuleConfig(doc)["properties"].(map[string]interface{})["extra"] = 1
		})},
		{"optionSpecs 未知键", mutateSample(t, func(doc map[string]interface{}) {
			firstModelRuleConfig(doc)["optionSpecs"].(map[string]interface{})["extra"] = 1
		})},
		// 内建文件用 modelConfigRules.modelRules；个人文件不认它，绝不能混用。
		{"内建 modelRules 混入", mutateSample(t, func(doc map[string]interface{}) {
			cfg := doc["config"].(map[string]interface{})
			cfg["modelConfigRules"].(map[string]interface{})["modelRules"] = []interface{}{}
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode(tc.data); err == nil {
				t.Fatal("Decode 应拒绝未知键，但成功了")
			}
		})
	}
}

func TestDecodeRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"空输入", []byte("")},
		{"非法 JSON", []byte(`{"schemaVersion": 1,`)},
		{"根节点是数组", []byte(`[]`)},
		{"多余内容", append([]byte(minimalV2JSON), []byte(`{}`)...)},
		{"providerId 为空", mutateSample(t, func(doc map[string]interface{}) {
			cfg := doc["config"].(map[string]interface{})
			rules := cfg["providerConfigRules"].(map[string]interface{})["providerRules"].([]interface{})
			rules[0].(map[string]interface{})["providerId"] = ""
		})},
		{"group 非枚举值", mutateSample(t, func(doc map[string]interface{}) {
			firstProviderRuleConfig(doc)["group"] = "unknown-group"
		})},
		{"api.type 非枚举值", mutateSample(t, func(doc map[string]interface{}) {
			firstProviderRuleConfig(doc)["api"].(map[string]interface{})["type"] = "openai-completions"
		})},
		{"access.type 非枚举值", mutateSample(t, func(doc map[string]interface{}) {
			firstProviderRuleConfig(doc)["access"].(map[string]interface{})["type"] = "oauth"
		})},
		{"contextWindow 为 0", mutateSample(t, func(doc map[string]interface{}) {
			firstModelRuleConfig(doc)["properties"].(map[string]interface{})["contextWindow"] = 0
		})},
		{"contextWindow 为负", mutateSample(t, func(doc map[string]interface{}) {
			firstModelRuleConfig(doc)["properties"].(map[string]interface{})["contextWindow"] = -1
		})},
		{"reasoningLevel.values 非字符串数组", mutateSample(t, func(doc map[string]interface{}) {
			specs := firstModelRuleConfig(doc)["optionSpecs"].(map[string]interface{})
			specs["reasoningLevel"] = map[string]interface{}{"values": []interface{}{1}}
		})},
		{"maxOutputTokens.max 非法", mutateSample(t, func(doc map[string]interface{}) {
			specs := firstModelRuleConfig(doc)["optionSpecs"].(map[string]interface{})
			specs["maxOutputTokens"] = map[string]interface{}{"max": 0}
		})},
		{"optionSpecs.map 非字符串", mutateSample(t, func(doc map[string]interface{}) {
			specs := firstModelRuleConfig(doc)["optionSpecs"].(map[string]interface{})
			specs["reasoningLevel"] = map[string]interface{}{"values": []interface{}{"low"}, "map": 1}
		})},
		{"缺少 modelConfigRules", mutateSample(t, func(doc map[string]interface{}) {
			delete(doc["config"].(map[string]interface{}), "modelConfigRules")
		})},
		{"缺少 manualProviderModelRules", mutateSample(t, func(doc map[string]interface{}) {
			mcr := doc["config"].(map[string]interface{})["modelConfigRules"].(map[string]interface{})
			delete(mcr, "manualProviderModelRules")
		})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Decode(tc.data); err == nil {
				t.Fatal("Decode 应失败，但成功了")
			}
		})
	}
}

// ⑤ 空 / 非法 JSON 的判定同时验证 Detect 与 Decode 的一致性。
func TestDecodeDetectConsistency(t *testing.T) {
	if got := Detect([]byte(minimalV2JSON)); got != FormatV2 {
		t.Fatalf("Detect(minimal) = %v, want FormatV2", got)
	}
	if _, err := Decode([]byte(minimalV2JSON)); err != nil {
		t.Fatalf("Decode(minimal) 失败：%v", err)
	}
}
