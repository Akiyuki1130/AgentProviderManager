package ompprovider

import (
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const placeholderBaseURL = "https://api.example.invalid/v1"

// doc 把多行 YAML 拼成带末尾换行的文档。
func doc(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func mustLoadModels(t *testing.T, data string) *ModelsFile {
	t.Helper()
	file, err := LoadModels([]byte(data))
	if err != nil {
		t.Fatalf("LoadModels 失败：%v", err)
	}
	return file
}

func mustBytes(t *testing.T, file *ModelsFile) string {
	t.Helper()
	out, err := file.Bytes()
	if err != nil {
		t.Fatalf("Bytes 失败：%v", err)
	}
	return string(out)
}

func decodeModels(t *testing.T, out string) map[string]map[string]interface{} {
	t.Helper()
	var root map[string]interface{}
	if err := yaml.Unmarshal([]byte(out), &root); err != nil {
		t.Fatalf("输出不是合法 YAML：%v\n%s", err, out)
	}
	providers := make(map[string]map[string]interface{})
	raw, _ := root["providers"].(map[string]interface{})
	for key, value := range raw {
		mapping, _ := value.(map[string]interface{})
		providers[key] = mapping
	}
	return providers
}

func TestLoadModelsRobustness(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		wantErr   error
		wantEmpty bool
	}{
		{name: "空内容", data: "", wantEmpty: true},
		{name: "纯空白", data: "  \n\t\n", wantEmpty: true},
		{name: "纯注释", data: "# 只有注释\n# 第二行\n", wantEmpty: true},
		{name: "null 根", data: "null\n", wantEmpty: true},
		{name: "波浪号根", data: "~\n", wantEmpty: true},
		{name: "BOM 前缀", data: "\xef\xbb\xbf" + doc("providers:", "  p1:", "    apiKey: PLACEHOLDER")},
		{name: "CRLF 行尾", data: "providers:\r\n  p1:\r\n    apiKey: PLACEHOLDER\r\n"},
		{name: "尾部空文档", data: doc("providers:", "  p1:", "    apiKey: PLACEHOLDER", "---")},
		{name: "JSON 内容", data: "{\"providers\":{\"p1\":{\"apiKey\":\"PLACEHOLDER\"}}}"},
		{name: "多文档报错", data: doc("providers: {}", "---", "providers: {}"), wantErr: ErrMultiDocument},
		{name: "重复键报错", data: doc("providers:", "  p1:", "    apiKey: A", "    apiKey: B"), wantErr: ErrDuplicateKey},
		{name: "根不是映射", data: doc("- a", "- b"), wantErr: ErrRootNotMapping},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := LoadModels([]byte(test.data))
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("期望错误 %v，实际 %v", test.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不应报错：%v", err)
			}
			if test.wantEmpty {
				ids := file.ProviderIDs()
				if len(ids) != 0 {
					t.Fatalf("应为空文档，实际 provider=%v", ids)
				}
				if out := mustBytes(t, file); out != "" {
					t.Fatalf("空文档序列化应为空，实际 %q", out)
				}
			}
		})
	}
}

func TestLoadModelsRejectsOversize(t *testing.T) {
	data := make([]byte, maxDocumentBytes+1)
	for i := range data {
		data[i] = ' '
	}
	if _, err := LoadModels(data); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("期望 ErrFileTooLarge，实际 %v", err)
	}
}

func TestProvidersAndIDs(t *testing.T) {
	file := mustLoadModels(t, doc(
		"providers:",
		"  zeta:",
		"    apiKey: PLACEHOLDER",
		"  alpha:",
		"    apiKey: PLACEHOLDER",
		"  middle:",
		"    apiKey: PLACEHOLDER",
	))
	got := file.ProviderIDs()
	want := []string{"zeta", "alpha", "middle"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("ProviderIDs 应保持文件顺序：%v", got)
	}
	providers, err := file.Providers()
	if err != nil {
		t.Fatalf("Providers 失败：%v", err)
	}
	if providers["alpha"]["apiKey"] != "PLACEHOLDER" {
		t.Fatalf("Providers 内容错误：%#v", providers["alpha"])
	}
	if !file.HasProvider("middle") || file.HasProvider("nope") {
		t.Fatal("HasProvider 判定错误")
	}
	providers["alpha"]["apiKey"] = "MUTATED"
	if again, _ := file.Providers(); again["alpha"]["apiKey"] != "PLACEHOLDER" {
		t.Fatal("Providers 不是深拷贝")
	}
}

func TestSetProviderCreatesAndRoundTrips(t *testing.T) {
	file := NewModelsFile()
	err := file.SetProvider("p1", map[string]interface{}{
		"baseUrl": placeholderBaseURL,
		"apiKey":  "PLACEHOLDER_KEY",
		"api":     "openai-completions",
		"headers": map[string]string{"X-Test": "placeholder"},
		"models":  []map[string]interface{}{{"id": "m1", "name": "M1"}},
	}, false)
	if err != nil {
		t.Fatalf("SetProvider 失败：%v", err)
	}
	out := mustBytes(t, file)
	providers := decodeModels(t, out)
	if len(providers) != 1 || providers["p1"]["baseUrl"] != placeholderBaseURL {
		t.Fatalf("写入结果不对：\n%s", out)
	}
	reloaded := mustLoadModels(t, out)
	if again := mustBytes(t, reloaded); again != out {
		t.Fatalf("序列化不幂等：\n%s\n---\n%s", out, again)
	}
}

func TestSetProviderPreservesCommentsUnknownKeysAndTags(t *testing.T) {
	original := doc(
		"# 文件头注释",
		"providers:",
		"  p1:",
		"    # baseUrl 上方注释",
		"    baseUrl: https://old.example.invalid/v1 # 行内注释",
		"    apiKey: !command \"cat placeholder.key\"",
		"    api: openai-completions",
		"    unknownField: keep-me",
		"    headers:",
		"      X-Old: placeholder",
		"    models:",
		"      - id: m1",
		"        name: 旧名字",
		"        customKey: keep",
	)
	file := mustLoadModels(t, original)
	err := file.SetProvider("p1", map[string]interface{}{
		"baseUrl": "https://new.example.invalid/v1",
		"apiKey":  "",
		"models":  []map[string]interface{}{{"id": "m1", "name": "新名字"}},
	}, true)
	if err != nil {
		t.Fatalf("SetProvider 失败：%v", err)
	}
	out := mustBytes(t, file)
	for _, want := range []string{
		"# 文件头注释",
		"# baseUrl 上方注释",
		"# 行内注释",
		"!command",
		"cat placeholder.key",
		"unknownField: keep-me",
		"customKey: keep",
		"https://new.example.invalid/v1",
		"新名字",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("输出缺少 %q：\n%s", want, out)
		}
	}
	if strings.Contains(out, "旧名字") {
		t.Fatalf("旧值应被覆盖：\n%s", out)
	}
}

func TestSetProviderKeepsUnchangedScalarNodeWithTag(t *testing.T) {
	file := mustLoadModels(t, doc(
		"providers:",
		"  p1:",
		"    baseUrl: !command \"echo placeholder\"",
		"    apiKey: PLACEHOLDER",
	))
	// 文本完全相同的 baseUrl 不应被替换成普通标量，标签必须保住。
	if err := file.SetProvider("p1", map[string]interface{}{"baseUrl": "echo placeholder"}, true); err != nil {
		t.Fatalf("SetProvider 失败：%v", err)
	}
	out := mustBytes(t, file)
	if !strings.Contains(out, "!command \"echo placeholder\"") {
		t.Fatalf("未变化节点应保留标签：\n%s", out)
	}
}

func TestSetProviderEnvReferencePreserved(t *testing.T) {
	file := mustLoadModels(t, doc(
		"providers:",
		"  p1:",
		"    baseUrl: https://api.example.invalid/v1",
		"    apiKey: OMP_PLACEHOLDER_ENV",
	))
	// 空 apiKey = 不覆盖：环境变量名原样保留，不做 $VAR 展开。
	if err := file.SetProvider("p1", map[string]interface{}{"apiKey": ""}, true); err != nil {
		t.Fatalf("SetProvider 失败：%v", err)
	}
	out := mustBytes(t, file)
	if !strings.Contains(out, "apiKey: OMP_PLACEHOLDER_ENV") {
		t.Fatalf("环境变量引用应原样保留：\n%s", out)
	}
}

func TestSetProviderNilDeletesKeys(t *testing.T) {
	file := mustLoadModels(t, doc(
		"providers:",
		"  p1:",
		"    baseUrl: https://api.example.invalid/v1",
		"    apiKey: PLACEHOLDER",
		"    authHeader: true",
		"    headers:",
		"      X-A: placeholder",
		"      X-B: placeholder",
	))
	err := file.SetProvider("p1", map[string]interface{}{
		"apiKey":     nil,
		"authHeader": false,
		"headers":    map[string]string{},
	}, true)
	if err != nil {
		t.Fatalf("SetProvider 失败：%v", err)
	}
	providers, _ := file.Providers()
	raw := providers["p1"]
	for _, key := range []string{"apiKey", "authHeader", "headers"} {
		if _, ok := raw[key]; ok {
			t.Fatalf("键 %q 应被删除：%#v", key, raw)
		}
	}
	if raw["baseUrl"] != placeholderBaseURL {
		t.Fatalf("未涉及的键应保留：%#v", raw)
	}
}

func TestSetProviderHeadersMergesAndKeepsTags(t *testing.T) {
	file := mustLoadModels(t, doc(
		"providers:",
		"  p1:",
		"    apiKey: PLACEHOLDER",
		"    headers:",
		"      X-Keep: !command \"cat placeholder\"",
		"      X-Drop: placeholder",
	))
	err := file.SetProvider("p1", map[string]interface{}{
		"headers": map[string]interface{}{"X-Keep": "cat placeholder", "X-New": "placeholder"},
	}, true)
	if err != nil {
		t.Fatalf("SetProvider 失败：%v", err)
	}
	out := mustBytes(t, file)
	if !strings.Contains(out, "X-Keep: !command") || !strings.Contains(out, "X-New") {
		t.Fatalf("headers 写入不正确：\n%s", out)
	}
	if strings.Contains(out, "X-Drop") {
		t.Fatalf("多余 header 应删除：\n%s", out)
	}
}

func TestSetProviderModelsMergeAndPrune(t *testing.T) {
	original := doc(
		"providers:",
		"  p1:",
		"    apiKey: PLACEHOLDER",
		"    models:",
		"      - id: m1",
		"        name: 一",
		"      - id: m2",
		"        name: 二",
	)
	t.Run("合并保留未列出的模型", func(t *testing.T) {
		file := mustLoadModels(t, original)
		err := file.SetProvider("p1", map[string]interface{}{
			"models": []map[string]interface{}{{"id": "m1", "name": "一改"}},
		}, true)
		if err != nil {
			t.Fatalf("SetProvider 失败：%v", err)
		}
		providers, _ := file.Providers()
		models := providers["p1"]["models"].([]interface{})
		if len(models) != 2 {
			t.Fatalf("merge 不应删除未列出的模型：%#v", models)
		}
	})
	t.Run("不合并时删除未列出的模型", func(t *testing.T) {
		file := mustLoadModels(t, original)
		err := file.SetProvider("p1", map[string]interface{}{
			"models": []map[string]interface{}{{"id": "m1", "name": "一改"}},
		}, false)
		if err != nil {
			t.Fatalf("SetProvider 失败：%v", err)
		}
		providers, _ := file.Providers()
		models := providers["p1"]["models"].([]interface{})
		if len(models) != 1 {
			t.Fatalf("应删除未列出的模型：%#v", models)
		}
	})
}

func TestSetProviderAppendsNewModelAndKeepsUnknownModelKeys(t *testing.T) {
	file := mustLoadModels(t, doc(
		"providers:",
		"  p1:",
		"    apiKey: PLACEHOLDER",
		"    models:",
		"      - id: m1",
		"        customKey: keep",
	))
	err := file.SetProvider("p1", map[string]interface{}{
		"models": []map[string]interface{}{{"id": "m2", "contextWindow": 128000, "maxTokens": 8192}},
	}, true)
	if err != nil {
		t.Fatalf("SetProvider 失败：%v", err)
	}
	out := mustBytes(t, file)
	if !strings.Contains(out, "customKey: keep") || !strings.Contains(out, "id: m2") {
		t.Fatalf("模型追加或保留失败：\n%s", out)
	}
	if !strings.Contains(out, "id: m1") {
		t.Fatalf("旧模型不应丢失：\n%s", out)
	}
}

func TestSetProviderRejectsInvalidID(t *testing.T) {
	file := NewModelsFile()
	for _, id := range []string{"", "a/b", "a b", "a:b"} {
		if err := file.SetProvider(id, map[string]interface{}{"baseUrl": placeholderBaseURL}, true); err == nil {
			t.Fatalf("ID %q 应被拒绝", id)
		}
	}
}

func TestSetProviderRejectsAliasProvider(t *testing.T) {
	file := mustLoadModels(t, doc(
		"defaults: &defaults",
		"  apiKey: PLACEHOLDER",
		"providers:",
		"  p1: *defaults",
	))
	err := file.SetProvider("p1", map[string]interface{}{"baseUrl": placeholderBaseURL}, true)
	if !errors.Is(err, ErrReadOnlyNode) {
		t.Fatalf("别名 provider 应报只读，实际 %v", err)
	}
}

func TestSetProviderRejectsMergeKey(t *testing.T) {
	file := mustLoadModels(t, doc(
		"base: &base",
		"  apiKey: PLACEHOLDER",
		"providers:",
		"  p1:",
		"    <<: *base",
	))
	err := file.SetProvider("p1", map[string]interface{}{"baseUrl": placeholderBaseURL}, true)
	if !errors.Is(err, ErrReadOnlyNode) {
		t.Fatalf("merge 键 provider 应报只读，实际 %v", err)
	}
}

func TestDeleteProvider(t *testing.T) {
	file := mustLoadModels(t, doc(
		"providers:",
		"  p1:",
		"    apiKey: PLACEHOLDER",
		"  p2:",
		"    apiKey: PLACEHOLDER",
	))
	if file.DeleteProvider("nope") {
		t.Fatal("不存在的 provider 不应报告删除成功")
	}
	if !file.DeleteProvider("p1") {
		t.Fatal("删除失败")
	}
	ids := file.ProviderIDs()
	if len(ids) != 1 || ids[0] != "p2" {
		t.Fatalf("删除结果不对：%v", ids)
	}
}

func TestDeleteModel(t *testing.T) {
	t.Run("普通删除", func(t *testing.T) {
		file := mustLoadModels(t, doc(
			"providers:",
			"  p1:",
			"    baseUrl: https://api.example.invalid/v1",
			"    models:",
			"      - id: m1",
			"      - id: m2",
		))
		removed, err := file.DeleteModel("p1", "m1")
		if err != nil || !removed {
			t.Fatalf("删除失败：removed=%v err=%v", removed, err)
		}
		providers, _ := file.Providers()
		models := providers["p1"]["models"].([]interface{})
		if len(models) != 1 {
			t.Fatalf("剩余模型数量不对：%#v", models)
		}
		if removed, _ := file.DeleteModel("p1", "nope"); removed {
			t.Fatal("不存在的模型不应报告删除成功")
		}
	})
	t.Run("删除最后一个模型且 provider 仍合法", func(t *testing.T) {
		file := mustLoadModels(t, doc(
			"providers:",
			"  p1:",
			"    baseUrl: https://api.example.invalid/v1",
			"    models:",
			"      - id: m1",
		))
		removed, err := file.DeleteModel("p1", "m1")
		if err != nil || !removed {
			t.Fatalf("删除失败：removed=%v err=%v", removed, err)
		}
		providers, _ := file.Providers()
		if _, ok := providers["p1"]["models"]; ok {
			t.Fatal("空 models 键应被移除")
		}
	})
	t.Run("删除后 provider 不合法则拒绝", func(t *testing.T) {
		file := mustLoadModels(t, doc(
			"providers:",
			"  p1:",
			"    api: openai-completions",
			"    models:",
			"      - id: m1",
		))
		removed, err := file.DeleteModel("p1", "m1")
		if removed || err == nil {
			t.Fatalf("应拒绝并返回原因：removed=%v err=%v", removed, err)
		}
	})
	t.Run("别名模型报只读", func(t *testing.T) {
		file := mustLoadModels(t, doc(
			"model: &m",
			"  id: m1",
			"providers:",
			"  p1:",
			"    baseUrl: https://api.example.invalid/v1",
			"    models:",
			"      - *m",
		))
		if _, err := file.DeleteModel("p1", "m1"); !errors.Is(err, ErrReadOnlyNode) {
			t.Fatalf("别名模型应报只读，实际 %v", err)
		}
	})
}

func TestEfforts(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]interface{}
		want []string
	}{
		{name: "没有 thinking", raw: map[string]interface{}{}, want: nil},
		{name: "thinking 不是映射", raw: map[string]interface{}{"thinking": "x"}, want: nil},
		{name: "没有 efforts", raw: map[string]interface{}{"thinking": map[string]interface{}{"mode": "level"}}, want: nil},
		{
			name: "六档乱序去重",
			raw: map[string]interface{}{"thinking": map[string]interface{}{"efforts": []interface{}{
				"max", "low", "high", "low", "minimal", "medium", "xhigh",
			}}},
			want: []string{"minimal", "low", "medium", "high", "xhigh", "max"},
		},
		{
			name: "过滤 off 与未知档",
			raw:  map[string]interface{}{"thinking": map[string]interface{}{"efforts": []string{"off", "low", "turbo"}}},
			want: []string{"low"},
		},
		{
			name: "全部非法",
			raw:  map[string]interface{}{"thinking": map[string]interface{}{"efforts": []string{"off"}}},
			want: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Efforts(test.raw)
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Fatalf("Efforts=%v，期望 %v", got, test.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	validProvider := doc(
		"providers:",
		"  p1:",
		"    baseUrl: https://api.example.invalid/v1",
		"    apiKey: PLACEHOLDER",
		"    api: openai-completions",
		"    models:",
		"      - id: m1",
		"        contextWindow: 128000",
		"        maxContextWindow: 256000",
		"        maxTokens: 8192",
		"        cost: {input: 1, output: 2, cacheRead: 0.5, cacheWrite: 0.5}",
		"        thinking:",
		"          mode: level",
		"          efforts: [minimal, low, medium, high, xhigh, max]",
	)
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{name: "合法文档", data: validProvider},
		{name: "空文档", data: ""},
		{
			name:    "models 为空且无 G2 字段",
			data:    doc("providers:", "  p1: {}"),
			wantErr: "models 为空时 provider 必须至少提供",
		},
		{
			name: "models 为空但有 baseUrl 合法",
			data: doc("providers:", "  p1:", "    baseUrl: https://api.example.invalid/v1"),
		},
		{
			name: "缺 baseUrl",
			data: doc(
				"providers:",
				"  p1:",
				"    apiKey: PLACEHOLDER",
				"    models:",
				"      - id: m1",
			),
			wantErr: "必须提供 baseUrl",
		},
		{
			name: "缺 apiKey",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    models:",
				"      - id: m1",
			),
			wantErr: "必须提供 apiKey",
		},
		{
			name: "auth none 可省略 apiKey",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    auth: none",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
			),
		},
		{
			name: "非法 api",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: nope",
				"    models:",
				"      - id: m1",
			),
			wantErr: "不受 omp 支持",
		},
		{
			name: "非法 auth",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    auth: nope",
				"    models:",
				"      - id: m1",
			),
			wantErr: "auth 只能是",
		},
		{
			name: "模型缺 api 且 provider 无 api",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    models:",
				"      - id: m1",
			),
			wantErr: "模型必须自带 api",
		},
		{
			name: "模型 id 重复",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
				"      - id: m1",
			),
			wantErr: "重复",
		},
		{
			name: "contextWindow 非正",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
				"        contextWindow: 0",
			),
			wantErr: "contextWindow 必须是正数",
		},
		{
			name: "maxContextWindow 小于 contextWindow",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
				"        contextWindow: 200000",
				"        maxContextWindow: 100000",
			),
			wantErr: "不能小于 contextWindow",
		},
		{
			name: "cost 缺字段",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
				"        cost: {input: 1}",
			),
			wantErr: "cost 必须包含",
		},
		{
			name: "thinking 缺 mode",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
				"        thinking:",
				"          efforts: [low]",
			),
			wantErr: "mode 不能为空",
		},
		{
			name: "thinking 非法档位",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
				"        thinking:",
				"          mode: level",
				"          efforts: [off]",
			),
			wantErr: "非法档位",
		},
		{
			name: "defaultLevel 不在 efforts 内",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
				"        thinking:",
				"          mode: level",
				"          efforts: [low]",
				"          defaultLevel: high",
			),
			wantErr: "defaultLevel",
		},
		{
			name: "input 非法模态",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    api: openai-completions",
				"    models:",
				"      - id: m1",
				"        input: [audio]",
			),
			wantErr: "只支持 text 与 image",
		},
		{
			name: "discovery 缺 api 且非 proxy",
			data: doc(
				"providers:",
				"  p1:",
				"    baseUrl: https://api.example.invalid/v1",
				"    apiKey: PLACEHOLDER",
				"    models:",
				"      - id: m1",
				"        api: openai-completions",
				"    discovery:",
				"      type: ollama",
			),
			wantErr: "discovery.type 只能是 proxy",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, err := LoadModels([]byte(test.data))
			if err != nil {
				t.Fatalf("加载失败：%v", err)
			}
			err = file.Validate()
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("不应报错：%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("期望包含 %q 的错误，实际 %v", test.wantErr, err)
			}
		})
	}
}

func TestBytesNormalizesLineEndingsAndIndent(t *testing.T) {
	file := mustLoadModels(t, "providers:\r\n  p1:\r\n    apiKey: PLACEHOLDER\r\n")
	out := mustBytes(t, file)
	if strings.Contains(out, "\r") {
		t.Fatalf("输出不应包含 CR：%q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("输出应以换行结尾：%q", out)
	}
	if !strings.Contains(out, "\n  p1:") {
		t.Fatalf("应使用 2 空格缩进：\n%s", out)
	}
}
