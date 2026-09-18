package zcodeprovider

import (
	"bytes"
	"encoding/json"
)

// Format 表示一份 ZCode provider 配置文件的格式。
type Format int

const (
	// FormatUnknown 表示无法判定（空内容、非法 JSON 或非对象根节点）。
	FormatUnknown Format = iota
	// FormatLegacy 表示旧版 config.json（顶层 provider 映射）。
	FormatLegacy
	// FormatV2 表示新版 provider_config.json（顶层 schemaVersion + config）。
	FormatV2
)

// Detect 判定字节流的格式。判定是启发式的，不代替 Decode 的严格校验：
//
//   - 空内容 / 纯空白 / 非法 JSON / 根节点不是对象 -> FormatUnknown
//   - 顶层出现 config 或 schemaVersion -> FormatV2
//   - 其余合法 JSON 对象（含空对象 {} 与顶层 provider）-> FormatLegacy，
//     因为旧版 config.json 的顶层就是一个 provider 映射，缺文件时也等价于 {}。
//
// 为兼容可能带注释的旧版文件，解析失败时会再尝试剥离 //、/* */ 注释与尾随逗号。
func Detect(data []byte) Format {
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	if len(trimmed) == 0 {
		return FormatUnknown
	}
	obj, ok := parseJSONObject(trimmed)
	if !ok {
		return FormatUnknown
	}
	if _, ok := obj["schemaVersion"]; ok {
		return FormatV2
	}
	if _, ok := obj["config"]; ok {
		return FormatV2
	}
	return FormatLegacy
}

func parseJSONObject(data []byte) (map[string]interface{}, bool) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err == nil {
		m, ok := v.(map[string]interface{})
		return m, ok
	}
	var relaxed interface{}
	if err := json.Unmarshal([]byte(stripJSONC(string(data))), &relaxed); err != nil {
		return nil, false
	}
	m, ok := relaxed.(map[string]interface{})
	return m, ok
}

// stripJSONC 剥离 //、/* */ 注释与尾随逗号，仅用于 Detect 的容错解析。
func stripJSONC(text string) string {
	var out []byte
	inString := false
	escaped := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if inString {
			out = append(out, c)
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(text) {
			switch text[i+1] {
			case '/':
				i += 2
				for i < len(text) && text[i] != '\n' && text[i] != '\r' {
					i++
				}
				i--
				continue
			case '*':
				i += 2
				for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
					i++
				}
				i++
				continue
			}
		}
		out = append(out, c)
	}
	return removeTrailingCommas(string(out))
}

func removeTrailingCommas(text string) string {
	var out []byte
	inString := false
	escaped := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if inString {
			out = append(out, c)
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(text) && (text[j] == ' ' || text[j] == '\t' || text[j] == '\n' || text[j] == '\r') {
				j++
			}
			if j < len(text) && (text[j] == '}' || text[j] == ']') {
				continue
			}
		}
		out = append(out, c)
	}
	return string(out)
}
