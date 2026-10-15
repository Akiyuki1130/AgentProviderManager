package core

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ParseTokens parses a token value: int, "128k", "1m", etc. Returns nil if invalid.
func ParseTokens(v interface{}) *int {
	if v == nil {
		return nil
	}
	if _, ok := v.(bool); ok {
		return nil
	}
	switch val := v.(type) {
	case int:
		if val > 0 && val <= MaxTokenLimit {
			return &val
		}
		return nil
	case int64:
		iv := int(val)
		if iv > 0 && iv <= MaxTokenLimit {
			return &iv
		}
		return nil
	case float64:
		if val > 0 && val <= MaxTokenLimit && val == float64(int(val)) {
			iv := int(val)
			return &iv
		}
		return nil
	case float32:
		f := float64(val)
		if f > 0 && f <= MaxTokenLimit && f == float64(int(f)) {
			iv := int(f)
			return &iv
		}
		return nil
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return nil
		}
		re := regexp.MustCompile(`(?i)^\s*(\d+)\s*(k|m)?\s*$`)
		m := re.FindStringSubmatch(strings.ToLower(s))
		if m == nil {
			return nil
		}
		num, _ := strconv.Atoi(m[1])
		unit := strings.ToLower(m[2])
		var value int
		switch unit {
		case "k":
			value = num * 1000
		case "m":
			value = num * 1000000
		default:
			value = num
		}
		if value > 0 && value <= MaxTokenLimit {
			return &value
		}
		return nil
	default:
		return nil
	}
}

// canonicalVariant 把配置里出现的档位拼法归一为规范拼法。
//
// 语义（阶段 A）：
//   - none / disabled / nothink 是 off 的等价别名，统一折成 "off"；
//   - 其余档位（含 "minimal"）原样小写去空格。
//
// 特别说明：minimal 在 ZCode（["none","minimal",...]）与 DSH（7 档）里都是合法且独立的
// 档位，绝不折成 low。旧实现曾做 minimal→low 折叠（A-6/A-1 相关缺陷），已取消。
func canonicalVariant(value string) string {
	switch v := strings.ToLower(strings.TrimSpace(value)); v {
	case "none", "disabled", "nothink":
		return "off"
	default:
		return v
	}
}

// hasControlChar 判断字符串是否含 ASCII 控制字符（含 \n \r \t）或 DEL。
// 档位名必须单行，控制字符会破坏 DSH 的 YAML 键或让配置出现不可见的怪值。
func hasControlChar(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// variantOrderIndex 返回规范化档位在固定 UI 顺序中的下标；未知档位排最后。
func variantOrderIndex(v string) int {
	for i, o := range ZCodeVariantOptions {
		if o == v {
			return i
		}
	}
	return len(ZCodeVariantOptions)
}

// sortVariantsCanonical 把从 map 迭代收集来的档位按固定顺序排好。
// map 的迭代顺序是随机的，不排序会让同一份配置每次读出的档位顺序都不同。
func sortVariantsCanonical(v []string) []string {
	sort.SliceStable(v, func(i, j int) bool { return variantOrderIndex(v[i]) < variantOrderIndex(v[j]) })
	return v
}

// NormalizeVariants normalizes reasoning variants to deduplicated lowercase list.
//
// 白名单受「允许自定义档位」策略（CustomVariantsAllowed）控制：
//   - 关闭（默认）：只保留 ZCodeVariantSet 白名单内的档位，行为与阶段 A 完全一致；
//   - 打开：跳过白名单过滤，但仍是「去重 + canonicalVariant 归一别名 + 丢弃空串」。
//
// 注意顺序保持：两条路径都不排序——排序是 sortVariantsCanonical 的职责，只用于能力表。
func NormalizeVariants(value interface{}) []string {
	if value == nil {
		return nil
	}
	var items []string
	switch v := value.(type) {
	case string:
		items = []string{v}
	case []string:
		items = v
	case []interface{}:
		for _, e := range v {
			if s, ok := e.(string); ok {
				items = append(items, s)
			}
		}
	default:
		return nil
	}
	allowCustom := CustomVariantsAllowed()
	seen := map[string]bool{}
	var out []string
	for _, raw := range items {
		s := canonicalVariant(raw)
		if s == "" || seen[s] {
			continue
		}
		// 放宽路径下只需非空 + 未重复；收紧路径下还须落在白名单内。
		if !allowCustom && !ZCodeVariantSet[s] {
			continue
		}
		// 自定义档位（不在白名单内）额外限长与禁控制字符：ZCode/DSH 对档位名无
		// schema 校验，高级开关打开时必须在这里挡住超长/含控制字符的值——它们会
		// 原样落进用户配置（DSH 还会把它当 YAML 键）。
		if !ZCodeVariantSet[s] && (len(s) > MaxVariantLen || hasControlChar(s)) {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
