package core

import (
	"strings"
	"testing"
)

// 本文件锁定生成表解析器（capability_features.go）的契约：紧凑编码 `<input>|<output>|<flags>`
// 的结构校验（fail-closed）与容错精确查询。
//
// 生成物 capability_features_gen.go 当前是**占位空表**，因此需要非空输入时一律用合成表注入
// （照 capability_site_test.go 的既有写法）；查询函数读的是包级 map，没有可注入变体。

// withFeatureTable 临时替换生成表，测试结束自动还原（t.Cleanup），避免污染其它用例。
//
// 这是 features 表唯一可行的注入方式：lookupCatalogFeatures 直接读包级
// capabilityFeatureTable，没有 buildCatalogTable 那样的 src 参数版本。
func withFeatureTable(t *testing.T, table map[string]string) {
	t.Helper()
	prev := capabilityFeatureTable
	capabilityFeatureTable = table
	t.Cleanup(func() { capabilityFeatureTable = prev })
}

// TestParseFeatureEncoding 锁定编码解析：
//   - 结构性畸形（段数不为 3、flags 长度不为 6）一律 ok==false 并返回零值；
//   - 内容未知（模态段有未登记的词、flags 段有 '0' 或其它字符）不报错，但**绝不产生
//     「已知为真」的位**——只把 '1' 读成真，这正是「宁可没有能力信息也不猜」的落点。
func TestParseFeatureEncoding(t *testing.T) {
	cases := []struct {
		name   string
		enc    string
		wantOK bool
		want   ModelFeatures
	}{
		{
			name:   "正常：模态 + 命中的位",
			enc:    "text,image|text|1-1---",
			wantOK: true,
			want: ModelFeatures{
				InputModalities:  []string{"text", "image"},
				OutputModalities: []string{"text"},
				ToolCall:         featureBool(true),
				NativeWebSearch:  featureBool(true),
			},
		},
		{
			name:   "六位全真（顺序 t j w m a T）",
			enc:    "text|audio|111111",
			wantOK: true,
			want: ModelFeatures{
				InputModalities:  []string{"text"},
				OutputModalities: []string{"audio"},
				ToolCall:         featureBool(true),
				StructuredOutput: featureBool(true),
				NativeWebSearch:  featureBool(true),
				MidConvSystem:    featureBool(true),
				Attachment:       featureBool(true),
				Temperature:      featureBool(true),
			},
		},
		{
			name:   "六位全 '-'：结构合法但没有任何已知位（IsEmpty）",
			enc:    "||------",
			wantOK: true,
			want:   ModelFeatures{},
		},
		{
			name:   "模态段按规范序归一（乱序输入）",
			enc:    "pdf,video,audio,image,text|text|------",
			wantOK: true,
			want: ModelFeatures{
				InputModalities:  []string{"text", "image", "video", "audio", "pdf"},
				OutputModalities: []string{"text"},
			},
		},
		{
			name:   "输出模态按输出词表排序（与输入词表不同）",
			enc:    "text|pdf,image,audio,text|------",
			wantOK: true,
			want: ModelFeatures{
				InputModalities:  []string{"text"},
				OutputModalities: []string{"text", "audio", "image", "pdf"},
			},
		},
		{
			name:   "模态段折叠别名并丢弃未知词",
			enc:    "image_url,hologram,text/plain|application/pdf|------",
			wantOK: true,
			want: ModelFeatures{
				InputModalities:  []string{"text", "image"},
				OutputModalities: []string{"pdf"},
			},
		},
		{
			name:   "flags 里的 '0' 与 '-' 等价（都不算已知为真）",
			enc:    "text|text|0-----",
			wantOK: true,
			want: ModelFeatures{
				InputModalities:  []string{"text"},
				OutputModalities: []string{"text"},
			},
		},
		{
			name:   "flags 里的未知字符同样留在未知态（不误判为真）",
			enc:    "text|text|X--Y--",
			wantOK: true,
			want: ModelFeatures{
				InputModalities:  []string{"text"},
				OutputModalities: []string{"text"},
			},
		},
		// 以下全是结构性畸形：必须 ok==false 且零值。
		{name: "空串", enc: "", wantOK: false, want: ModelFeatures{}},
		{name: "段数不足（两段）", enc: "text|text", wantOK: false, want: ModelFeatures{}},
		{name: "段数不足（模态段后直接跟 flags）", enc: "text|1-1---", wantOK: false, want: ModelFeatures{}},
		{name: "段数过多（四段）", enc: "text|text|1-1---|extra", wantOK: false, want: ModelFeatures{}},
		{name: "三段齐但 flags 长度为 0", enc: "||", wantOK: false, want: ModelFeatures{}},
		{name: "flags 少一位", enc: "text|text|1----", wantOK: false, want: ModelFeatures{}},
		{name: "flags 多一位", enc: "text|text|1-----x", wantOK: false, want: ModelFeatures{}},
		{name: "段数过多（四个分隔段）", enc: "text|text|1-1---|", wantOK: false, want: ModelFeatures{}},
	}
	for _, c := range cases {
		got, ok := parseFeatureEncoding(c.enc)
		if ok != c.wantOK {
			t.Errorf("%s（%q）：ok=%v，期望 %v（got %s）", c.name, c.enc, ok, c.wantOK, describeFeatures(got))
			continue
		}
		if !sameFeatures(got, c.want) {
			t.Errorf("%s（%q）：got %s，期望 %s", c.name, c.enc, describeFeatures(got), describeFeatures(c.want))
		}
		if !ok && !got.IsEmpty() {
			t.Errorf("%s（%q）：解析失败必须返回零值，实际 %s", c.name, c.enc, describeFeatures(got))
		}
	}
}

// TestLookupCatalogFeaturesExactAndTolerant 锁定容错精确查询：先按小写精确 id 查，
// 未命中且含 "/" 时只取最后一个 "/" 之后一段重查，**只在第二次命中时才采用**。
func TestLookupCatalogFeaturesExactAndTolerant(t *testing.T) {
	const (
		exactID = "qwen3.8-max"
		exactIn = "text,image|text|1-----" // in=[text,image] out=[text] tool=true
	)
	withFeatureTable(t, map[string]string{
		exactID:       exactIn,
		"all-unknown": "||------",  // 结构合法但没有任何已知位 → 视为未命中
		"broken-row":  "text|text", // 畸形行 → 视为未命中
	})

	exactWant := ModelFeatures{
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
		ToolCall:         featureBool(true),
		Source:           "catalog",
	}
	// 精确命中（含大小写与首尾空白折叠）。
	for _, id := range []string{exactID, "  Qwen3.8-Max  "} {
		if got := lookupCatalogFeatures(id); !sameFeatures(got, exactWant) {
			t.Errorf("lookupCatalogFeatures(%q)=%s，期望 %s", id, describeFeatures(got), describeFeatures(exactWant))
		}
	}
	// 去前缀容错：只取最后一个 "/" 之后的一段（"/qwen3.8-max" 的末段就是精确键，因此命中）。
	for _, id := range []string{"Qwen/Qwen3.8-Max", "vendor/team/Qwen3.8-Max", "/qwen3.8-max"} {
		if got := lookupCatalogFeatures(id); !sameFeatures(got, exactWant) {
			t.Errorf("lookupCatalogFeatures(%q)=%s，期望 %s", id, describeFeatures(got), describeFeatures(exactWant))
		}
	}

	// fail-closed：以下情形都必须返回零值（不做子串匹配、不逐段回退、不猜）。
	for _, id := range []string{
		"",
		"   ",
		"qwen3.8-max/extra", // 整串未命中，末段 extra 也未命中
		"qwen3.8-max/",      // 末段为空，不再重查
		"qwen3.8-ma",        // 末段只是子串，不得命中
		"apm-definitely-not-a-real-model-xyz",
		"all-unknown", // 命中了键但编码没有任何已知位
		"broken-row",  // 命中了键但编码畸形
		"Qwen/All-Unknown",
	} {
		got := lookupCatalogFeatures(id)
		if got.HasAny() || got.Source != "" {
			t.Errorf("lookupCatalogFeatures(%q) 应返回零值，实际 %s", id, describeFeatures(got))
		}
	}
}

// TestLookupCatalogFeaturesEmptyTable 表为空时必须返回零值（生成物当前就是占位空表，
// 这条也是「表还没生成好时不得凭空给能力位」的保险）。
func TestLookupCatalogFeaturesEmptyTable(t *testing.T) {
	withFeatureTable(t, map[string]string{})
	for _, id := range []string{"qwen3.8-max", "Qwen/Qwen3.8-Max", "glm-5.3", "", "   "} {
		got := lookupCatalogFeatures(id)
		if got.HasAny() || got.Source != "" {
			t.Errorf("空表下 lookupCatalogFeatures(%q) 应为零值，实际 %s", id, describeFeatures(got))
		}
	}
	// 命中时 Source 记为 catalog，供 ModelCapability.Source 与调试使用。
	withFeatureTable(t, map[string]string{"probe-model": "text|text|------"})
	if got := lookupCatalogFeatures("probe-model"); got.Source != "catalog" {
		t.Errorf("命中时 Source=%q，期望 catalog", got.Source)
	}
}

// TestCapabilityFeatureTableGeneratedSelfCheck 对生成物做自检（不写死任何具体模型）：
// 键必须小写、每条编码都必须能解析且至少给出一个已知位、用键本身查询的结果必须与直接解析
// 一致。生成物当前是**空占位表**（见 capability_features_gen.go 文件头），所以这里不要求
// 非空；一旦脚本开始产出条目，本用例立刻开始把关。
func TestCapabilityFeatureTableGeneratedSelfCheck(t *testing.T) {
	for id, enc := range capabilityFeatureTable {
		if id != strings.ToLower(id) {
			t.Errorf("键 %q 不是小写：生成物必须按小写精确 id 建表", id)
			continue
		}
		parsed, ok := parseFeatureEncoding(enc)
		if !ok {
			t.Errorf("%q: 编码 %q 无法解析（生成物不该有畸形行）", id, enc)
			continue
		}
		if parsed.IsEmpty() {
			t.Errorf("%q: 编码 %q 没有任何已知位（生成物只应收录有能力的 id）", id, enc)
			continue
		}
		want := parsed
		want.Source = "catalog"
		if got := lookupCatalogFeatures(id); !sameFeatures(got, want) {
			t.Errorf("%q: 查询结果 %s 与直接解析 %s 不一致", id, describeFeatures(got), describeFeatures(want))
		}
	}
}
