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

func canonicalVariant(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "disabled", "nothink":
		return "off"
	case "minimal":
		return "low"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
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
		if s != "" && ZCodeVariantSet[s] && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
