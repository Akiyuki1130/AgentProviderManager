package zcodeprovider

import (
	"encoding/json"
	"reflect"
	"testing"
)

// legacyFixtureJSON 是旧版 ZCode config.json 的形状（provider 映射 + options + models）。
// 其中 source、zcode.*、options.apiKeyRequired/timeout、模型级 name/attachment/
// reasoning.enabled/defaultVariant 都无法映射进 v2，用于验证 DroppedFields。
// 所有密钥均为占位值。
const legacyFixtureJSON = `{
  "provider": {
    "acme": {
      "name": "Acme",
      "kind": "openai-compatible",
      "source": "custom",
      "zcode": {"modified": 1, "priority": 5},
      "options": {
        "baseURL": "https://api.example.invalid/v1",
        "apiKey": "sk-test",
        "apiKeyRequired": true,
        "timeout": 30
      },
      "models": {
        "acme-large": {
          "name": "Acme Large",
          "limit": {"context": 200000, "output": 32000},
          "reasoning": {"enabled": true, "variants": ["off", "low", "high"], "defaultVariant": "high"},
          "modalities": {"input": ["text", "image", "video"]},
          "attachment": true
        },
        "acme-small": {"limit": {"context": 8000}}
      }
    },
    "beta": {
      "name": "Beta",
      "kind": "anthropic",
      "options": {"baseURL": "https://api.beta.example.invalid"},
      "models": {"beta-1": {"reasoning": {"variants": ["low", "max"]}}}
    },
    "gamma": {
      "name": "Gamma",
      "kind": "openai",
      "options": {"baseURL": "https://api.gamma.example.invalid/v1"},
      "models": {}
    }
  }
}`

func legacyFixture(t *testing.T) map[string]interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(legacyFixtureJSON), &v); err != nil {
		t.Fatalf("legacy 样例解析失败：%v", err)
	}
	doc, ok := v.(map[string]interface{})
	if !ok {
		t.Fatalf("legacy 样例根节点不是对象")
	}
	return doc
}

func legacyProvider(t *testing.T, id string) map[string]interface{} {
	t.Helper()
	providers, _ := legacyFixture(t)["provider"].(map[string]interface{})
	m, _ := providers[id].(map[string]interface{})
	if m == nil {
		t.Fatalf("legacy 样例缺少 provider %q", id)
	}
	return m
}

// ③ FromLegacy 的整表映射（含 off -> disabled）。
func TestFromLegacyMapping(t *testing.T) {
	cfg, dropped, err := FromLegacy(legacyFixture(t))
	if err != nil {
		t.Fatalf("FromLegacy 失败：%v", err)
	}
	if got, want := cfg.ProviderOrder(), []string{"acme", "beta", "gamma"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("providerOrder = %v, want %v", got, want)
	}
	if len(dropped) == 0 {
		t.Fatal("FromLegacy 应报告被丢弃的字段")
	}
	for _, want := range []string{"acme:source", "acme:zcode.modified", "acme:options.apiKeyRequired"} {
		if !containsString(dropped, want) {
			t.Errorf("dropped 缺少 %q（实际 %v）", want, dropped)
		}
	}

	t.Run("acme", func(t *testing.T) {
		rule := cfg.ProviderRule("acme")
		if rule == nil {
			t.Fatal("缺少 providerRule acme")
		}
		if got := RuleProviderName(rule); got != "Acme" {
			t.Errorf("providerName = %q, want Acme", got)
		}
		pcfg := RuleConfig(rule)
		if got := ProviderGroup(pcfg); got != GroupStandardPersonal {
			t.Errorf("group = %q, want %q", got, GroupStandardPersonal)
		}
		if got := ProviderAPIType(pcfg); got != APITypeOpenAIChatCompletions {
			t.Errorf("api.type = %q, want %q", got, APITypeOpenAIChatCompletions)
		}
		if got := ProviderAPIBaseURL(pcfg); got != "https://api.example.invalid/v1" {
			t.Errorf("api.baseUrl = %q", got)
		}
		if got := ProviderAccessType(pcfg); got != AccessTypeAPIKey {
			t.Errorf("access.type = %q, want %q", got, AccessTypeAPIKey)
		}
		if got := ProviderAccessAPIKey(pcfg); got != "sk-test" {
			t.Errorf("access.apiKey = %q, want sk-test", got)
		}
		if got, want := ProviderPersonalModelIDs(pcfg), []string{"acme-large", "acme-small"}; !reflect.DeepEqual(got, want) {
			t.Errorf("personalModelIds = %v, want %v", got, want)
		}
	})

	t.Run("acme-large", func(t *testing.T) {
		mcfg := ModelRuleConfig(cfg.ProviderModelRule("acme", "acme-large"))
		if mcfg == nil {
			t.Fatal("缺少 providerModelRule acme/acme-large")
		}
		if got, ok := ModelContextWindow(mcfg); !ok || got != 200000 {
			t.Errorf("contextWindow = %d, ok = %v, want 200000", got, ok)
		}
		if got, ok := ModelMaxOutputTokens(mcfg); !ok || got != 32000 {
			t.Errorf("maxOutputTokens.max = %d, ok = %v, want 32000", got, ok)
		}
		values := ModelReasoningLevelValues(mcfg)
		if !reflect.DeepEqual(values, []string{"disabled", "low", "high"}) {
			t.Errorf("reasoningLevel.values = %v, want [disabled low high]", values)
		}
		if containsString(values, "off") {
			t.Errorf("off 应改名为 disabled，实际 %v", values)
		}
		format := ModelInputFormat(mcfg)
		if format["supportsImage"] != true {
			t.Errorf("inputFormat.supportsImage = %v, want true", format["supportsImage"])
		}
		if format["supportsVideo"] != true {
			t.Errorf("inputFormat.supportsVideo = %v, want true", format["supportsVideo"])
		}
		if _, ok := format["supportsText"]; ok {
			t.Errorf("不应推断 supportsText，实际 %v", format)
		}
		if _, hasMap := ModelReasoningLevelMap(mcfg); hasMap {
			t.Errorf("不应编造 reasoningLevel.map")
		}
		if _, hasMap := ModelMaxOutputTokensMap(mcfg); hasMap {
			t.Errorf("不应编造 maxOutputTokens.map")
		}
	})

	t.Run("acme-small", func(t *testing.T) {
		mcfg := ModelRuleConfig(cfg.ProviderModelRule("acme", "acme-small"))
		if mcfg == nil {
			t.Fatal("缺少 providerModelRule acme/acme-small")
		}
		if got, ok := ModelContextWindow(mcfg); !ok || got != 8000 {
			t.Errorf("contextWindow = %d, ok = %v, want 8000", got, ok)
		}
		if _, exists := mcfg["optionSpecs"]; exists {
			t.Errorf("没有 legacy 来源时不应写出 optionSpecs：%v", mcfg)
		}
	})

	t.Run("kind 映射", func(t *testing.T) {
		if got := ProviderAPIType(RuleConfig(cfg.ProviderRule("beta"))); got != APITypeAnthropicMessages {
			t.Errorf("anthropic -> %q", got)
		}
		if got := ProviderAPIType(RuleConfig(cfg.ProviderRule("gamma"))); got != APITypeOpenAIResponses {
			t.Errorf("openai -> %q", got)
		}
		// beta 没有 apiKey，不应产生 access。
		if access := ProviderAccess(RuleConfig(cfg.ProviderRule("beta"))); access != nil {
			t.Errorf("无 apiKey 时不应写 access：%v", access)
		}
		// gamma 的 models 为空对象：personalModelIds 应为空。
		if got := ProviderPersonalModelIDs(RuleConfig(cfg.ProviderRule("gamma"))); len(got) != 0 {
			t.Errorf("personalModelIds = %v, want []", got)
		}
	})
}

// FromLegacy 的产物必须能通过严格校验并再次解码（端到端可用）。
func TestFromLegacyOutputIsCanonical(t *testing.T) {
	cfg, _, err := FromLegacy(legacyFixture(t))
	if err != nil {
		t.Fatalf("FromLegacy 失败：%v", err)
	}
	encoded, err := Encode(cfg)
	if err != nil {
		t.Fatalf("Encode 失败：%v", err)
	}
	if _, err := Decode(encoded); err != nil {
		t.Fatalf("FromLegacy 产物无法通过 Decode：%v", err)
	}
}

func TestFromLegacyEdgeCases(t *testing.T) {
	for name, legacy := range map[string]map[string]interface{}{
		"nil":          nil,
		"无 provider 键": {},
		"空 provider":   {"provider": map[string]interface{}{}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, dropped, err := FromLegacy(legacy)
			if err != nil {
				t.Fatalf("FromLegacy 失败：%v", err)
			}
			if len(dropped) != 0 {
				t.Fatalf("dropped = %v, want 空", dropped)
			}
			if _, err := Encode(cfg); err != nil {
				t.Fatalf("Encode 失败：%v", err)
			}
		})
	}
	t.Run("kind 未知时报错", func(t *testing.T) {
		legacy := map[string]interface{}{"provider": map[string]interface{}{
			"x": map[string]interface{}{"kind": "mystery", "options": map[string]interface{}{}},
		}}
		if _, _, err := FromLegacy(legacy); err == nil {
			t.Fatal("未知 kind 应报错")
		}
	})
	t.Run("responses 别名归一为 openai", func(t *testing.T) {
		legacy := map[string]interface{}{"provider": map[string]interface{}{
			"x": map[string]interface{}{"kind": "responses", "options": map[string]interface{}{"baseURL": "https://x.invalid"}},
		}}
		cfg, _, err := FromLegacy(legacy)
		if err != nil {
			t.Fatalf("FromLegacy 失败：%v", err)
		}
		if got := ProviderAPIType(RuleConfig(cfg.ProviderRule("x"))); got != APITypeOpenAIResponses {
			t.Fatalf("responses -> %q, want %q", got, APITypeOpenAIResponses)
		}
	})
}

// ④ DroppedFields 清单。
func TestDroppedFields(t *testing.T) {
	want := []string{
		"models.acme-large.attachment",
		"models.acme-large.name",
		"models.acme-large.reasoning.defaultVariant",
		"models.acme-large.reasoning.enabled",
		"options.apiKeyRequired",
		"options.timeout",
		"source",
		"zcode.modified",
		"zcode.priority",
	}
	if got := DroppedFields(legacyProvider(t, "acme")); !reflect.DeepEqual(got, want) {
		t.Fatalf("DroppedFields(acme) = %v, want %v", got, want)
	}
	// beta 的 reasoning 只有 variants（可映射），因此不应有任何丢失。
	if got := DroppedFields(legacyProvider(t, "beta")); len(got) != 0 {
		t.Fatalf("DroppedFields(beta) = %v, want 空", got)
	}
	if got := DroppedFields(nil); got != nil {
		t.Fatalf("DroppedFields(nil) = %v, want nil", got)
	}
}

// 新旧互转：legacy -> v2 -> legacy 保留核心字段，并把 disabled 反转为 off。
func TestToLegacyRoundTrip(t *testing.T) {
	cfg, _, err := FromLegacy(legacyFixture(t))
	if err != nil {
		t.Fatalf("FromLegacy 失败：%v", err)
	}
	out, err := ToLegacy(cfg)
	if err != nil {
		t.Fatalf("ToLegacy 失败：%v", err)
	}
	providers, ok := out["provider"].(map[string]interface{})
	if !ok {
		t.Fatalf("ToLegacy 输出缺少 provider 对象：%v", out)
	}
	if len(providers) != 3 {
		t.Fatalf("provider 数量 = %d, want 3", len(providers))
	}

	acme, _ := providers["acme"].(map[string]interface{})
	if acme == nil {
		t.Fatal("缺少 acme")
	}
	if got := acme["kind"]; got != "openai-compatible" {
		t.Errorf("acme.kind = %v, want openai-compatible", got)
	}
	if got := acme["name"]; got != "Acme" {
		t.Errorf("acme.name = %v, want Acme", got)
	}
	options, _ := acme["options"].(map[string]interface{})
	if got := options["baseURL"]; got != "https://api.example.invalid/v1" {
		t.Errorf("acme.options.baseURL = %v", got)
	}
	if got := options["apiKey"]; got != "sk-test" {
		t.Errorf("acme.options.apiKey = %v, want sk-test", got)
	}
	models, _ := acme["models"].(map[string]interface{})
	if len(models) != 2 {
		t.Fatalf("acme.models 数量 = %d, want 2", len(models))
	}
	large, _ := models["acme-large"].(map[string]interface{})
	if large == nil {
		t.Fatal("缺少 acme-large")
	}
	limit, _ := large["limit"].(map[string]interface{})
	if got, ok := toIntValue(limit["context"]); !ok || got != 200000 {
		t.Errorf("acme-large.limit.context = %v, want 200000", limit["context"])
	}
	if got, ok := toIntValue(limit["output"]); !ok || got != 32000 {
		t.Errorf("acme-large.limit.output = %v, want 32000", limit["output"])
	}
	reasoning, _ := large["reasoning"].(map[string]interface{})
	if got := stringSlice(reasoning["variants"]); !reflect.DeepEqual(got, []string{"off", "low", "high"}) {
		t.Errorf("acme-large.reasoning.variants = %v, want [off low high]", got)
	}
	modalities, _ := large["modalities"].(map[string]interface{})
	if got := stringSlice(modalities["input"]); !reflect.DeepEqual(got, []string{"image", "video"}) {
		t.Errorf("acme-large.modalities.input = %v, want [image video]", got)
	}

	if got := providers["beta"].(map[string]interface{})["kind"]; got != "anthropic" {
		t.Errorf("beta.kind = %v, want anthropic", got)
	}
	if got := providers["gamma"].(map[string]interface{})["kind"]; got != "openai" {
		t.Errorf("gamma.kind = %v, want openai", got)
	}
}

func TestToLegacyErrors(t *testing.T) {
	if _, err := ToLegacy(nil); err == nil {
		t.Fatal("ToLegacy(nil) 应报错")
	}
	cfg := NewConfigFromDoc(map[string]interface{}{
		"schemaVersion": SchemaVersionV1,
		"config": map[string]interface{}{
			"providerConfigRules": map[string]interface{}{
				"providerRules": []interface{}{
					map[string]interface{}{
						"providerId": "x",
						"config":     map[string]interface{}{"group": GroupStandardPersonal},
					},
				},
			},
			"modelConfigRules": map[string]interface{}{
				"providerModelRules":       []interface{}{},
				"manualProviderModelRules": []interface{}{},
			},
		},
	})
	if _, err := ToLegacy(cfg); err == nil {
		t.Fatal("缺少 api.type 时 ToLegacy 应报错")
	}
}

func containsString(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}

// ZCode 3.14 迁移规则的补充映射：apiFormat/defaultKind 优先、endpoints 兜底、
// headers/apiKeyUrl/enabled 不再丢失、模型级 contextWindow/maxOutputTokens 优先、
// deleted 模型被跳过。
func TestFromLegacyZCode314Mapping(t *testing.T) {
	legacy := map[string]interface{}{"provider": map[string]interface{}{
		"acme": map[string]interface{}{
			"name":        "Acme",
			"kind":        "openai-compatible",
			"apiFormat":   APITypeOpenAIResponses,
			"enabled":     false,
			"apiKeyUrl":   "https://acme.example.invalid/keys",
			"headers":     map[string]interface{}{"X-Test": "1"},
			"options":     map[string]interface{}{},
			"endpoints":   map[string]interface{}{"baseURL": "https://endpoint.example.invalid/v1", "paths": map[string]interface{}{"anthropic": "/anthropic"}},
			"models": map[string]interface{}{
				"m1": map[string]interface{}{
					"limit":           map[string]interface{}{"context": 1000, "output": 100},
					"contextWindow":   200000,
					"maxOutputTokens": 32000,
				},
				"m2": map[string]interface{}{"deleted": true, "limit": map[string]interface{}{"context": 10}},
			},
		},
	}}

	cfg, dropped, err := FromLegacy(legacy)
	if err != nil {
		t.Fatalf("FromLegacy 失败：%v", err)
	}
	rule := cfg.ProviderRule("acme")
	if rule == nil {
		t.Fatal("缺少 providerRule acme")
	}
	if enabled, ok := RuleEnabled(rule); !ok || enabled {
		t.Errorf("enabled 应映射为 false，得到 %v（ok=%v）", enabled, ok)
	}
	pcfg := RuleConfig(rule)
	if got := ProviderAPIType(pcfg); got != APITypeOpenAIResponses {
		t.Errorf("apiFormat 应优先于 kind，得到 %q", got)
	}
	if got := ProviderAPIBaseURL(pcfg); got != "https://endpoint.example.invalid/v1" {
		t.Errorf("baseUrl 应回退到 endpoints.baseURL，得到 %q", got)
	}
	if got := ProviderAPIHeaders(pcfg)["X-Test"]; got != "1" {
		t.Errorf("headers 应映射到 api.headers，得到 %v", got)
	}
	if got := ProviderAccessAPIKeyManagementURL(pcfg); got != "https://acme.example.invalid/keys" {
		t.Errorf("apiKeyUrl 应映射到 access.apiKeyManagementUrl，得到 %q", got)
	}
	if got := ProviderAccessType(pcfg); got != AccessTypeAPIKey {
		t.Errorf("access.type = %q", got)
	}
	// deleted 模型不进模型列表，也不产生模型规则。
	if got := ProviderPersonalModelIDs(pcfg); !reflect.DeepEqual(got, []string{"m1"}) {
		t.Errorf("personalModelIds = %v, want [m1]", got)
	}
	if cfg.ProviderModelRule("acme", "m2") != nil {
		t.Error("deleted 模型不应产生 providerModelRule")
	}
	mcfg := ModelRuleConfig(cfg.ProviderModelRule("acme", "m1"))
	if got, ok := ModelContextWindow(mcfg); !ok || got != 200000 {
		t.Errorf("模型级 contextWindow 应优先，得到 %d（ok=%v）", got, ok)
	}
	if got, ok := ModelMaxOutputTokens(mcfg); !ok || got != 32000 {
		t.Errorf("模型级 maxOutputTokens 应优先，得到 %d（ok=%v）", got, ok)
	}
	// endpoints 的路径后缀在 v2 没有对应位置，必须如实上报。
	if !containsString(dropped, "acme:endpoints.paths.anthropic") {
		t.Errorf("dropped 应包含 endpoints.paths.anthropic，实际 %v", dropped)
	}
	if containsString(dropped, "acme:endpoints.baseURL") {
		t.Errorf("endpoints.baseURL 已被用作 baseUrl，不应上报为丢失：%v", dropped)
	}
	if containsString(dropped, "acme:headers") || containsString(dropped, "acme:enabled") || containsString(dropped, "acme:apiKeyUrl") {
		t.Errorf("已映射的字段不应上报为丢失：%v", dropped)
	}
	if _, err := Encode(cfg); err != nil {
		t.Fatalf("产物应能通过严格校验：%v", err)
	}
}

// endpoints 只在没有任何其它 baseUrl 来源时才算“用掉”。
func TestDroppedFieldsEndpointsBaseURL(t *testing.T) {
	legacy := map[string]interface{}{"provider": map[string]interface{}{
		"x": map[string]interface{}{
			"kind":      "anthropic",
			"options":   map[string]interface{}{"baseURL": "https://x.example.invalid"},
			"endpoints": map[string]interface{}{"baseURL": "https://unused.example.invalid"},
		},
	}}
	_, dropped, err := FromLegacy(legacy)
	if err != nil {
		t.Fatalf("FromLegacy 失败：%v", err)
	}
	if !containsString(dropped, "x:endpoints.baseURL") {
		t.Errorf("未被使用的 endpoints.baseURL 应上报为丢失，实际 %v", dropped)
	}
}

// endpoints.paths 可以作为 api.type 的兜底来源。
func TestFromLegacyAPITypeFromEndpoints(t *testing.T) {
	legacy := map[string]interface{}{"provider": map[string]interface{}{
		"x": map[string]interface{}{
			"options":   map[string]interface{}{"baseURL": "https://x.example.invalid"},
			"endpoints": map[string]interface{}{"paths": map[string]interface{}{"openai-compatible": "/v1"}},
		},
	}}
	cfg, _, err := FromLegacy(legacy)
	if err != nil {
		t.Fatalf("FromLegacy 失败：%v", err)
	}
	if got := ProviderAPIType(RuleConfig(cfg.ProviderRule("x"))); got != APITypeOpenAIChatCompletions {
		t.Errorf("endpoints.paths 兜底 = %q", got)
	}
}
