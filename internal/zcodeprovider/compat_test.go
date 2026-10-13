package zcodeprovider

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func hasIssue(issues []CompatibilityIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

// 兼容性问题（ZCode 3.14 会拒绝）必须：严格 Decode 失败、宽松 DecodeInspect 收集到、
// Repair 能修好并让文件重新可编码。
func TestCompatIssuesAreCollectedAndRepairable(t *testing.T) {
	manualRule := func(doc map[string]interface{}) map[string]interface{} {
		mcr := doc["config"].(map[string]interface{})["modelConfigRules"].(map[string]interface{})
		return mcr["manualProviderModelRules"].([]interface{})[0].(map[string]interface{})
	}
	cases := []struct {
		name   string
		code   string
		mutate func(doc map[string]interface{})
	}{
		{
			name: "builtinModelIds 出现在个人规则",
			code: IssueBuiltinModelIDs,
			mutate: func(doc map[string]interface{}) {
				firstProviderRuleConfig(doc)["builtinModelIds"] = []interface{}{"acme-builtin"}
			},
		},
		{
			name: "group 使用内建家族取值",
			code: IssueUnsupportedGroup,
			mutate: func(doc map[string]interface{}) {
				firstProviderRuleConfig(doc)["group"] = GroupZAIFamily
			},
		},
		{
			name: "logo 是字符串（旧形状）",
			code: IssueUnsupportedLogo,
			mutate: func(doc map[string]interface{}) {
				firstProviderRuleConfig(doc)["logo"] = "https://example.invalid/logo.png"
			},
		},
		{
			name: "logo 带多余键",
			code: IssueUnsupportedLogo,
			mutate: func(doc map[string]interface{}) {
				firstProviderRuleConfig(doc)["logo"] = map[string]interface{}{"type": LogoTypeBuiltin, "key": "acme", "extra": 1}
			},
		},
		{
			name: "providerId 重复",
			code: IssueDuplicateProvider,
			mutate: func(doc map[string]interface{}) {
				pcr := doc["config"].(map[string]interface{})["providerConfigRules"].(map[string]interface{})
				rules := pcr["providerRules"].([]interface{})
				dup := map[string]interface{}{
					"providerId": "acme",
					"config":     map[string]interface{}{"group": GroupStandardPersonal},
				}
				pcr["providerRules"] = append(rules, dup)
			},
		},
		{
			name: "同一 Provider/Model 同时出现在两张表",
			code: IssueDuplicateModelRule,
			mutate: func(doc map[string]interface{}) {
				rule := manualRule(doc)
				rule["modelId"] = "acme-large"
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := mutateSample(t, tc.mutate)
			if _, err := Decode(data); err == nil {
				t.Fatal("严格 Decode 应拒绝，但成功了")
			}
			cfg, issues, err := DecodeInspect(data)
			if err != nil {
				t.Fatalf("DecodeInspect 失败：%v", err)
			}
			if !hasIssue(issues, tc.code) {
				t.Fatalf("未收集到 %s，实际问题：%+v", tc.code, issues)
			}
			repaired, err := RepairConfig(cfg)
			if err != nil {
				t.Fatalf("RepairConfig 失败：%v", err)
			}
			if !repaired.Changed {
				t.Fatal("RepairConfig 未做任何修复")
			}
			if len(repaired.Remaining) != 0 {
				t.Fatalf("修复后仍有问题：%+v", repaired.Remaining)
			}
			if _, err := Encode(cfg); err != nil {
				t.Fatalf("修复后仍无法编码：%v", err)
			}
		})
	}
}

// 非模板行的 builtinModelIds 修复必须合并进 personalModelIds，而不是丢掉。
func TestRepairMergesBuiltinModelIDs(t *testing.T) {
	data := mutateSample(t, func(doc map[string]interface{}) {
		rule := doc["config"].(map[string]interface{})["providerConfigRules"].(map[string]interface{})["providerRules"].([]interface{})[0].(map[string]interface{})
		delete(rule, "templateId") // 模板行走 TestRepairTemplateRuleOnlyDropsBuiltinModelIDs 的删除路径
		pcfg := firstProviderRuleConfig(doc)
		pcfg["builtinModelIds"] = []interface{}{"acme-builtin", "acme-large"}
	})
	cfg, issues, err := DecodeInspect(data)
	if err != nil {
		t.Fatalf("DecodeInspect 失败：%v", err)
	}
	if !hasIssue(issues, IssueBuiltinModelIDs) {
		t.Fatalf("未收集到 %s", IssueBuiltinModelIDs)
	}
	if _, err := RepairConfig(cfg); err != nil {
		t.Fatalf("RepairConfig 失败：%v", err)
	}
	pcfg := RuleConfig(cfg.ProviderRule("acme"))
	if _, ok := pcfg["builtinModelIds"]; ok {
		t.Fatal("builtinModelIds 未被删除")
	}
	got := ProviderPersonalModelIDs(pcfg)
	want := []string{"acme-large", "acme-small", "acme-builtin"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("personalModelIds = %v, want %v", got, want)
	}
}

// 模板派生行（templateId 非空）的 zod 形态 omit 了 personalModelIds/modelOrder：
// 修复 builtinModelIds 时对这类行只能删除该键，绝不能合并出 personalModelIds。
func TestRepairTemplateRuleOnlyDropsBuiltinModelIDs(t *testing.T) {
	data := mutateSample(t, func(doc map[string]interface{}) {
		rule := doc["config"].(map[string]interface{})["providerConfigRules"].(map[string]interface{})["providerRules"].([]interface{})[0].(map[string]interface{})
		rule["templateId"] = "tpl-1"
		pcfg := firstProviderRuleConfig(doc)
		pcfg["builtinModelIds"] = []interface{}{"acme-builtin"}
		delete(pcfg, "personalModelIds")
	})
	cfg, issues, err := DecodeInspect(data)
	if err != nil {
		t.Fatalf("DecodeInspect 失败：%v", err)
	}
	if !hasIssue(issues, IssueBuiltinModelIDs) {
		t.Fatalf("未收集到 %s", IssueBuiltinModelIDs)
	}
	if _, err := RepairConfig(cfg); err != nil {
		t.Fatalf("RepairConfig 失败：%v", err)
	}
	pcfg := RuleConfig(cfg.ProviderRule("acme"))
	if _, ok := pcfg["builtinModelIds"]; ok {
		t.Error("builtinModelIds 应被删除")
	}
	if _, ok := pcfg["personalModelIds"]; ok {
		t.Error("模板行不得写入 personalModelIds（ZCode 会拒收整份配置）")
	}
	if _, err := Encode(cfg); err != nil {
		t.Fatalf("修复后应可编码：%v", err)
	}
}

// ZCode 3.15.1 的 access.mode 枚举新增 highspeed，必须视为合法值而非兼容问题。
func TestDecodeAcceptsHighspeedAccountMode(t *testing.T) {
	data := mutateSample(t, func(doc map[string]interface{}) {
		firstProviderRuleConfig(doc)["access"] = map[string]interface{}{
			"type":        AccessTypeZhipuAccount,
			"accountType": AccountTypeZAI,
			"mode":        "highspeed",
		}
	})
	if _, err := Decode(data); err != nil {
		t.Fatalf("highspeed 应严格通过 Decode：%v", err)
	}
	_, issues, err := DecodeInspect(data)
	if err != nil {
		t.Fatalf("DecodeInspect 失败：%v", err)
	}
	if hasIssue(issues, IssueUnsupportedAccess) {
		t.Fatalf("highspeed 被误报为不兼容：%+v", issues)
	}
}

// 修复跨表重复时保留手动规则（自动规则可由 provider 模型列表重建）。
func TestRepairKeepsManualModelRule(t *testing.T) {
	data := mutateSample(t, func(doc map[string]interface{}) {
		mcr := doc["config"].(map[string]interface{})["modelConfigRules"].(map[string]interface{})
		rule := mcr["manualProviderModelRules"].([]interface{})[0].(map[string]interface{})
		rule["modelId"] = "acme-large"
	})
	cfg, _, err := DecodeInspect(data)
	if err != nil {
		t.Fatalf("DecodeInspect 失败：%v", err)
	}
	if _, err := RepairConfig(cfg); err != nil {
		t.Fatalf("RepairConfig 失败：%v", err)
	}
	if cfg.ProviderModelRule("acme", "acme-large") != nil {
		t.Fatal("自动规则未被删除")
	}
	if len(cfg.ManualProviderModelRules()) != 1 {
		t.Fatal("手动规则不该被删除")
	}
}

// collectObjectKeyOrders 按出现顺序返回文档里每个对象的键序列（用于校验键序）。
func collectObjectKeyOrders(t *testing.T, data []byte) [][]string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	var out [][]string
	var parseValue func()
	parseValue = func() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("解析 token 失败：%v", err)
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return
		}
		switch delim {
		case '{':
			var order []string
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					t.Fatalf("解析键名失败：%v", err)
				}
				key, _ := keyTok.(string)
				order = append(order, key)
				parseValue()
			}
			if _, err := dec.Token(); err != nil {
				t.Fatalf("解析结尾失败：%v", err)
			}
			out = append(out, order)
		case '[':
			for dec.More() {
				parseValue()
			}
			if _, err := dec.Token(); err != nil {
				t.Fatalf("解析结尾失败：%v", err)
			}
		}
	}
	parseValue()
	return out
}

// 编码输出的键序必须与 ZCode 的 JSON.stringify 一致，否则 ZCode 读到就会重写文件。
func TestCanonicalKeyOrder(t *testing.T) {
	cfg, err := Decode([]byte(sampleV2JSON))
	if err != nil {
		t.Fatalf("Decode 失败：%v", err)
	}
	out, err := Encode(cfg)
	if err != nil {
		t.Fatalf("Encode 失败：%v", err)
	}
	text := string(out)
	orders := collectObjectKeyOrders(t, out)
	hasOrder := func(keys ...string) bool {
		for _, order := range orders {
			pos := 0
			for _, k := range order {
				if pos < len(keys) && k == keys[pos] {
					pos++
				}
			}
			if pos == len(keys) {
				return true
			}
		}
		return false
	}
	checks := []struct {
		name string
		keys []string
	}{
		{"根节点", []string{"schemaVersion", "config"}},
		{"config", []string{"providerConfigRules", "modelConfigRules"}},
		{"providerRule", []string{"providerId", "providerName", "templateId", "enabled", "config"}},
		{"providerRule.config", []string{"group", "logo", "access", "api", "personalModelIds", "modelOrder", "visibility"}},
		{"access", []string{"type", "apiKey", "apiKeyManagementUrl"}},
		{"api", []string{"type", "baseUrl", "headers"}},
		{"modelConfigRules", []string{"providerModelRules", "manualProviderModelRules"}},
		{"modelRule", []string{"providerId", "modelId", "config"}},
		{"properties", []string{"requiresMfjsToolSchema", "contextWindow", "inputFormat", "outputFormat", "supportsToolCall"}},
	}
	for _, check := range checks {
		if !hasOrder(check.keys...) {
			t.Fatalf("%s 的键序不对，期望以 %v 的顺序出现\n%s", check.name, check.keys, text)
		}
	}
	// 末尾换行 + 2 空格缩进。
	if !bytes.HasSuffix(out, []byte("}\n")) {
		t.Fatalf("输出缺少末尾换行")
	}
	if !strings.Contains(text, "\n  \"config\": {") {
		t.Fatalf("缩进不是 2 空格")
	}
	// 键序输出必须仍然能被自己解析回来（往返一致）。
	var v interface{}
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("输出不是合法 JSON：%v", err)
	}
}
