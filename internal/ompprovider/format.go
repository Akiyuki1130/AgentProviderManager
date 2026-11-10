package ompprovider

import (
	"bytes"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Format 表示 APM 能识别的 omp 配置文件种类。
type Format int

const (
	FormatUnknown Format = iota
	FormatModelsYAML
	FormatModelsJSON
	FormatConfigYAML
)

// FormatName 返回格式的展示名，与 omp 的实际文件名保持一致。
func FormatName(f Format) string {
	switch f {
	case FormatModelsYAML:
		return "models.yml"
	case FormatModelsJSON:
		return "models.json"
	case FormatConfigYAML:
		return "config.yml"
	default:
		return "unknown"
	}
}

// Detect 按路径判断格式；后缀不可用（用户自选文件名）时按内容嗅探。
func Detect(path string, data []byte) Format {
	switch strings.ToLower(stripDecoration(filepath.Base(path))) {
	case "models.yml", "models.yaml":
		return FormatModelsYAML
	case "models.json":
		return FormatModelsJSON
	case "config.yml", "config.yaml":
		return FormatConfigYAML
	}
	trimmed := bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))
	if len(trimmed) == 0 {
		return FormatUnknown
	}
	// 按根键嗅探，避免注释或字符串值里的关键字造成误判。
	var root map[string]interface{}
	if err := yaml.Unmarshal(trimmed, &root); err == nil {
		if _, ok := root["modelRoles"]; ok {
			return FormatConfigYAML
		}
		if _, ok := root["providers"]; ok {
			if trimmed[0] == '{' {
				return FormatModelsJSON
			}
			return FormatModelsYAML
		}
	}
	if trimmed[0] == '{' {
		return FormatModelsJSON
	}
	return FormatUnknown
}

// stripDecoration 去掉备份 / 临时文件后缀，让格式判断看原始文件名。
func stripDecoration(name string) string {
	lower := strings.ToLower(name)
	for _, marker := range []string{".bak_", ".snapshot_", ".restore_", ".removed_", ".tmp_", ".apm-update-", ".apm-backup-"} {
		if idx := strings.Index(lower, marker); idx > 0 {
			return name[:idx]
		}
	}
	return name
}
