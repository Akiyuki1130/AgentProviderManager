package zcodeprovider

import "testing"

const sampleLegacyJSON = `{
  "provider": {
    "acme": {
      "name": "Acme",
      "kind": "openai-compatible",
      "options": {"baseURL": "https://api.example.invalid/v1", "apiKey": "sk-test"},
      "models": {
        "acme-large": {"limit": {"context": 200000, "output": 32000}}
      }
    }
  }
}`

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want Format
	}{
		{"空字节流", []byte(""), FormatUnknown},
		{"纯空白", []byte("  \n\t "), FormatUnknown},
		{"空对象视为 legacy", []byte(`{}`), FormatLegacy},
		{"非法 JSON", []byte(`{"schemaVersion": 1,`), FormatUnknown},
		{"非对象根节点", []byte(`[]`), FormatUnknown},
		{"裸字符串", []byte(`"provider_config"`), FormatUnknown},
		{"legacy", []byte(sampleLegacyJSON), FormatLegacy},
		{"legacy 带注释与尾随逗号", []byte(`{
			// 旧版允许注释
			"provider": {
				"acme": {"name": "Acme", "kind": "openai",},
			},
		}`), FormatLegacy},
		{"v2", []byte(sampleV2JSON), FormatV2},
		{"v2 最小", []byte(minimalV2JSON), FormatV2},
		{"仅 schemaVersion", []byte(`{"schemaVersion": 1}`), FormatV2},
		{"仅 config", []byte(`{"config": {}}`), FormatV2},
		{"带 BOM 的 v2", append([]byte{0xEF, 0xBB, 0xBF}, []byte(minimalV2JSON)...), FormatV2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Detect(tc.data); got != tc.want {
				t.Fatalf("Detect = %v, want %v", got, tc.want)
			}
		})
	}
}
