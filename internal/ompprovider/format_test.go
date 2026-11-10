package ompprovider

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		path string
		data string
		want Format
	}{
		{name: "models.yml 后缀", path: "models.yml", data: "", want: FormatModelsYAML},
		{name: "models.yaml 后缀", path: "models.yaml", data: "", want: FormatModelsYAML},
		{name: "models.json 后缀", path: "models.json", data: "", want: FormatModelsJSON},
		{name: "config.yml 后缀", path: "config.yml", data: "", want: FormatConfigYAML},
		{name: "config.yaml 后缀", path: "config.yaml", data: "", want: FormatConfigYAML},
		{name: "备份后缀被剥离", path: "models.yml.bak_20260101", data: "", want: FormatModelsYAML},
		{name: "临时后缀被剥离", path: "config.yaml.tmp_abc", data: "", want: FormatConfigYAML},
		{name: "更新器后缀被剥离", path: "models.yml.apm-backup-123", data: "", want: FormatModelsYAML},
		{name: "内容嗅探 config", path: "custom.yml", data: "modelRoles:\n  default: p1/m1\n", want: FormatConfigYAML},
		{name: "内容嗅探 models", path: "custom.yml", data: "providers:\n  p1:\n    apiKey: PLACEHOLDER\n", want: FormatModelsYAML},
		{name: "JSON 内容", path: "custom.dat", data: "{\"providers\":{}}", want: FormatModelsJSON},
		{name: "无法识别", path: "custom.dat", data: "hello: world\n", want: FormatUnknown},
		{name: "空内容", path: "custom.dat", data: "", want: FormatUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Detect(test.path, []byte(test.data)); got != test.want {
				t.Fatalf("Detect=%v，期望 %v", got, test.want)
			}
		})
	}
}

func TestFormatName(t *testing.T) {
	tests := []struct {
		format Format
		want   string
	}{
		{format: FormatModelsYAML, want: "models.yml"},
		{format: FormatModelsJSON, want: "models.json"},
		{format: FormatConfigYAML, want: "config.yml"},
		{format: FormatUnknown, want: "unknown"},
	}
	for _, test := range tests {
		if got := FormatName(test.format); got != test.want {
			t.Fatalf("FormatName(%v)=%q，期望 %q", test.format, got, test.want)
		}
	}
}
