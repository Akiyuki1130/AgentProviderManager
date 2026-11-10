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

// variantAllowed 是「档位名能否进入卡片」的唯一门控（读端与 NormalizeVariants 共用）：
//   - 白名单内的档位永远允许；
//   - 「允许自填推理档位」关闭（默认）时，白名单外的名字一律丢弃，行为与阶段 A 一致；
//   - 打开时，白名单外的名字只要非空、不超长、不含控制字符就保留。
//
// 白名单外的档位不受 ZCode/DSH 的 schema 校验，限长与禁控制字符必须在这里挡住——
// 它们会原样落进用户配置（DSH 还会把它当作 YAML 键）。
func variantAllowed(s string) bool {
	if ZCodeVariantSet[s] {
		return true
	}
	if !CustomVariantsAllowed() {
		return false
	}
	return s != "" && len(s) <= MaxVariantLen && !hasControlChar(s)
}

// NormalizeVariants normalizes reasoning variants to deduplicated lowercase list.
//
// 门控委托给 variantAllowed（读端与写端、读卡与规范化共用同一处判定）：
//   - 关闭（默认）：只保留 ZCodeVariantSet 白名单内的档位，行为与阶段 A 完全一致；
//   - 打开：跳过白名单过滤，但仍是「去重 + canonicalVariant 归一别名 + 丢弃空串 +
//     限长/禁控制字符」。
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
	seen := map[string]bool{}
	var out []string
	for _, raw := range items {
		s := canonicalVariant(raw)
		if s == "" || seen[s] {
			continue
		}
		// 唯一门控见 variantAllowed：开关关闭时白名单外的名字整条丢弃，打开时
		// 仍要做限长与禁控制字符校验。
		if !variantAllowed(s) {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
