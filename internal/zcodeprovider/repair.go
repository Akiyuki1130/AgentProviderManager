package zcodeprovider

import (
	"fmt"
	"sort"
	"strings"
)

// RepairResult 描述一次兼容性修复的结果。
type RepairResult struct {
	// Changed 表示文档确实被改动过。
	Changed bool `json:"changed"`
	// Fixes 是本次做过的修复动作（人类可读，供 UI 展示）。
	Fixes []string `json:"fixes"`
	// Remaining 是修复后仍然存在的问题（正常情况下为空）。
	Remaining []CompatibilityIssue `json:"remaining"`
}

// RepairConfig 就地修复 Config 里的 ZCode 兼容性问题。
func RepairConfig(c *Config) (RepairResult, error) {
	if c == nil || c.doc == nil {
		return RepairResult{}, fmt.Errorf("provider_config.json 修复失败：Config 为 nil")
	}
	return Repair(c.doc)
}

// Repair 就地修复文档里“ZCode 3.14 会拒绝”的问题，让文件重新变得可保存。
//
// 修复动作都选择“尽量不丢用户数据”的方向：
//   - builtinModelIds：合并进 personalModelIds（去重保序），而不是直接删除；
//   - group 家族取值：改成 standard-personal；
//   - 非规范 logo：删除该键（ZCode 只认 {type:"builtin",key}）；
//   - providerId 重复：保留最后一条（与 ZCode 内部按 id 建 Map 的语义一致）；
//   - (providerId, modelId) 跨表重复：保留手动规则、删除自动规则，
//     自动规则由 provider 的模型列表推导得出，可以重新生成，手动规则不能。
func Repair(doc map[string]interface{}) (RepairResult, error) {
	var result RepairResult
	if doc == nil {
		return result, fmt.Errorf("provider_config.json 修复失败：文档为空")
	}
	cfg := NewConfigFromDoc(doc)

	// 1. providerRules：builtinModelIds / group / logo
	rules := cfg.ProviderRules()
	for _, rule := range rules {
		pid := RuleProviderID(rule)
		pcfg := RuleConfig(rule)
		if pcfg == nil {
			continue
		}
		if raw, ok := pcfg["builtinModelIds"]; ok {
			builtin, _ := stringSliceStrict(raw)
			personal := ProviderPersonalModelIDs(pcfg)
			merged := dedupeStrings(append(personal, builtin...))
			delete(pcfg, "builtinModelIds")
			if len(merged) > 0 {
				pcfg["personalModelIds"] = toInterfaceSlice(merged)
			}
			result.Changed = true
			result.Fixes = append(result.Fixes, fmt.Sprintf("%s：builtinModelIds 已合并到 personalModelIds（%d 项）", pid, len(builtin)))
		}
		if group, ok := pcfg["group"]; ok && group != nil {
			if s, isStr := group.(string); isStr && s != GroupStandardPersonal {
				pcfg["group"] = GroupStandardPersonal
				result.Changed = true
				result.Fixes = append(result.Fixes, fmt.Sprintf("%s：group %s → %s", pid, s, GroupStandardPersonal))
			}
		}
		if logo, ok := pcfg["logo"]; ok && logo != nil {
			if !canonicalLogo(logo) {
				delete(pcfg, "logo")
				result.Changed = true
				result.Fixes = append(result.Fixes, fmt.Sprintf("%s：删除了 ZCode 不接受的 logo 形状", pid))
			}
		}
	}

	// 2. providerId 重复：保留最后一条
	if arr := cfg.ProviderRules(); len(arr) > 1 {
		lastIndex := map[string]int{}
		for i, rule := range arr {
			lastIndex[RuleProviderID(rule)] = i
		}
		kept := make([]interface{}, 0, len(arr))
		dropped := 0
		for i, rule := range arr {
			if lastIndex[RuleProviderID(rule)] != i {
				dropped++
				continue
			}
			kept = append(kept, rule)
		}
		if dropped > 0 {
			setProviderRules(doc, kept)
			result.Changed = true
			result.Fixes = append(result.Fixes, fmt.Sprintf("删除了 %d 条重复的 providerRule（同一 providerId 只保留最后一条）", dropped))
		}
	}

	// 3. (providerId, modelId) 跨表重复：保留手动规则
	manualKeys := map[string]bool{}
	for _, rule := range cfg.ManualProviderModelRules() {
		manualKeys[ModelRuleProviderID(rule)+"\x00"+ModelRuleModelID(rule)] = true
	}
	if len(manualKeys) > 0 && len(cfg.ProviderModelRules()) > 0 {
		arr, _ := v2Array(doc, "config", "modelConfigRules", "providerModelRules")
		kept := make([]interface{}, 0, len(arr))
		dropped := 0
		for _, raw := range arr {
			rule, _ := raw.(map[string]interface{})
			if rule == nil {
				kept = append(kept, raw)
				continue
			}
			if manualKeys[ModelRuleProviderID(rule)+"\x00"+ModelRuleModelID(rule)] {
				dropped++
				continue
			}
			kept = append(kept, raw)
		}
		if dropped > 0 {
			setV2Array(doc, kept, "config", "modelConfigRules", "providerModelRules")
			result.Changed = true
			result.Fixes = append(result.Fixes, fmt.Sprintf("删除了 %d 条与手动规则重复的 providerModelRule（保留手动规则）", dropped))
		}
	}

	remaining, err := Inspect(doc)
	if err != nil {
		return result, err
	}
	result.Remaining = remaining
	sort.Strings(result.Fixes)
	return result, nil
}

// canonicalLogo 判断 logo 是否已经是 ZCode 3.14 接受的形状。
func canonicalLogo(logo interface{}) bool {
	obj, ok := logo.(map[string]interface{})
	if !ok {
		return false
	}
	for k := range obj {
		if !logoKeys[k] {
			return false
		}
	}
	t, _ := obj["type"].(string)
	key, _ := obj["key"].(string)
	return t == LogoTypeBuiltin && strings.TrimSpace(key) != ""
}

func dedupeStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func setProviderRules(doc map[string]interface{}, rules []interface{}) {
	setV2Array(doc, rules, "config", "providerConfigRules", "providerRules")
}

func v2Array(doc map[string]interface{}, path ...string) ([]interface{}, bool) {
	cur := doc
	for i, key := range path {
		if i == len(path)-1 {
			arr, ok := cur[key].([]interface{})
			return arr, ok
		}
		next, _ := cur[key].(map[string]interface{})
		if next == nil {
			return nil, false
		}
		cur = next
	}
	return nil, false
}

func setV2Array(doc map[string]interface{}, arr []interface{}, path ...string) {
	cur := doc
	for i, key := range path {
		if i == len(path)-1 {
			cur[key] = arr
			return
		}
		next, _ := cur[key].(map[string]interface{})
		if next == nil {
			next = map[string]interface{}{}
			cur[key] = next
		}
		cur = next
	}
}
