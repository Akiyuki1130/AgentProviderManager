package core

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// 本文件锁定**中立能力层**（model_features.go）的纯函数语义：模态归一、布尔三态、
// 层间合并（前层胜出）与层内覆盖（后命中覆盖）、空判定。
//
// 全部使用占位值，不触盘、不读任何用户配置与凭据；不依赖任何真实生成表。

// sameFeatures 逐字段比较两个能力位集合。布尔位是指针，reflect.DeepEqual 按「指向的值」
// 比较，因此 nil（未知）与 featureBool(false)（已知为假）会被正确区分开。
func sameFeatures(a, b ModelFeatures) bool {
	return reflect.DeepEqual(a, b)
}

// boolTriState 把布尔三态渲染成 nil / true / false，供断言失败时对照。
func boolTriState(p *bool) string {
	switch {
	case p == nil:
		return "nil"
	case *p:
		return "true"
	default:
		return "false"
	}
}

// describeFeatures 把能力位渲染成一行，供断言失败时对照（%+v 会打出指针地址，读不出来）。
func describeFeatures(f ModelFeatures) string {
	return fmt.Sprintf("in=[%s] out=[%s] tool=%s json=%s web=%s mid=%s att=%s temp=%s source=%q",
		strings.Join(f.InputModalities, ","), strings.Join(f.OutputModalities, ","),
		boolTriState(f.ToolCall), boolTriState(f.StructuredOutput), boolTriState(f.NativeWebSearch),
		boolTriState(f.MidConvSystem), boolTriState(f.Attachment), boolTriState(f.Temperature), f.Source)
}

// TestNormalizeModalities 锁定模态归一：只保留规范词表内的词、按规范序排序去重、
// 折叠已知别名、丢弃未知词，空结果一律返回 nil（而不是空切片）。
//
// 输入与输出两张词表的规范序**不同**（输出是 text, audio, image, video, pdf），
// 因此每条用例都要指明走哪一张表。
func TestNormalizeModalities(t *testing.T) {
	cases := []struct {
		name   string
		output bool
		in     []string
		want   []string
	}{
		{"nil 输入返回 nil", false, nil, nil},
		{"空切片返回 nil", false, []string{}, nil},
		{"全是未知词返回 nil", false, []string{"hologram"}, nil},
		{"空词与纯空白词被丢弃", false, []string{"", "   "}, nil},
		{"未知词与已知词混合时只丢未知词", false, []string{"hologram", "text"}, []string{"text"}},
		{"按输入词表规范序排序（与来源顺序无关）", false, []string{"pdf", "audio", "video", "image", "text"}, []string{"text", "image", "video", "audio", "pdf"}},
		{"去重", false, []string{"text", "text", "image", "image"}, []string{"text", "image"}},
		{"大小写与首尾空白被折叠", false, []string{"  IMAGE ", "Text"}, []string{"text", "image"}},
		{"别名 image_url 折成 image", false, []string{"image_url"}, []string{"image"}},
		{"别名 video_url / audio_url / input_audio 折成 video / audio", false, []string{"video_url", "audio_url", "input_audio"}, []string{"video", "audio"}},
		{"别名 application/pdf 折成 pdf", false, []string{"application/pdf"}, []string{"pdf"}},
		{"别名 text/plain 折成 text", false, []string{"text/plain"}, []string{"text"}},
		{"未登记的 MIME 写法一律丢弃（fail-closed）", false, []string{"image/png"}, nil},
		{"别名折叠后仍去重", false, []string{"input_audio", "audio"}, []string{"audio"}},
		// 输出词表的顺序是 text, audio, image, video, pdf —— 与输入词表不同，必须按输出词表排。
		{"输出模态按输出词表排序", true, []string{"pdf", "video", "image", "audio", "text"}, []string{"text", "audio", "image", "video", "pdf"}},
		{"输出模态同样折叠别名", true, []string{"image_url", "input_audio"}, []string{"audio", "image"}},
		{"输出模态全是未知词返回 nil", true, []string{"embedding"}, nil},
	}
	for _, c := range cases {
		got := NormalizeInputModalities(c.in)
		if c.output {
			got = NormalizeOutputModalities(c.in)
		}
		if !reflect.DeepEqual(got, c.want) {
			table := "输入"
			if c.output {
				table = "输出"
			}
			t.Errorf("%s（%s词表）：got %v，期望 %v", c.name, table, got, c.want)
		}
	}
}

// TestHasModality 读取端判断：按规范化后的词精确匹配，大小写与空白折叠，未知词一律 false。
func TestHasModality(t *testing.T) {
	f := ModelFeatures{
		InputModalities:  NormalizeInputModalities([]string{"text", "IMAGE_URL"}),
		OutputModalities: NormalizeOutputModalities([]string{"text"}),
	}
	for _, c := range []struct {
		name string
		got  bool
		want bool
	}{
		{"输入模态命中", f.HasInputModality("image"), true},
		{"输入模态大小写与空白折叠", f.HasInputModality("  TEXT "), true},
		{"输入模态未命中", f.HasInputModality("audio"), false},
		{"输入模态空集合恒 false", ModelFeatures{}.HasInputModality("text"), false},
		{"输出模态命中", f.HasOutputModality("text"), true},
		{"输出模态未命中", f.HasOutputModality("pdf"), false},
		{"输出模态空集合恒 false", ModelFeatures{}.HasOutputModality("text"), false},
	} {
		if c.got != c.want {
			t.Errorf("%s：got %v，期望 %v", c.name, c.got, c.want)
		}
	}
}

// TestFeatureBool 锁定布尔值的构造：返回独立的指针副本，且零值字段是 nil（未知）而不是 false。
func TestFeatureBool(t *testing.T) {
	if tr := featureBool(true); tr == nil || !*tr {
		t.Errorf("featureBool(true)=%v，期望指向 true", tr)
	}
	if fa := featureBool(false); fa == nil || *fa {
		t.Errorf("featureBool(false)=%v，期望指向 false", fa)
	}
	// 两个指针必须互不影响（否则「写盘端只写真」的三态会被事后篡改）。
	a, b := featureBool(true), featureBool(true)
	if a == b {
		t.Fatal("featureBool 每次调用都应返回新的指针")
	}
	*b = false
	if !*a {
		t.Error("修改一个指针不应影响另一个")
	}
	// 三态的关键：零值必须是 nil。false 表示「已知为假」，绝不能靠零值表达。
	var zero ModelFeatures
	for name, p := range map[string]*bool{
		"tool_call": zero.ToolCall, "structured_output": zero.StructuredOutput,
		"native_web_search": zero.NativeWebSearch, "mid_conv_system": zero.MidConvSystem,
		"attachment": zero.Attachment, "temperature": zero.Temperature,
	} {
		if p != nil {
			t.Errorf("零值 %s 必须是 nil（未知），实际 %v", name, p)
		}
	}
}

// TestFeaturesIsEmptyAndHasAny 锁定空判定：只有 Source（内部展示字段）不算任何能力位；
// 而已知为假的布尔位**算**非空——它是「已知」，与未知是两回事。
func TestFeaturesIsEmptyAndHasAny(t *testing.T) {
	cases := []struct {
		name     string
		f        ModelFeatures
		wantEmpt bool
	}{
		{"零值", ModelFeatures{}, true},
		{"只有 Source 仍算空", ModelFeatures{Source: "catalog"}, true},
		{"空切片模态算空", ModelFeatures{InputModalities: []string{}, OutputModalities: nil}, true},
		{"任一输入模态即非空", ModelFeatures{InputModalities: []string{"text"}}, false},
		{"任一输出模态即非空", ModelFeatures{OutputModalities: []string{"audio"}}, false},
		{"布尔位为真即非空", ModelFeatures{ToolCall: featureBool(true)}, false},
		{"布尔位为假也算非空（已知为假 ≠ 未知）", ModelFeatures{Temperature: featureBool(false)}, false},
	}
	for _, c := range cases {
		if got := c.f.IsEmpty(); got != c.wantEmpt {
			t.Errorf("%s：IsEmpty=%v，期望 %v（%s）", c.name, got, c.wantEmpt, describeFeatures(c.f))
		}
		if got := c.f.HasAny(); got == c.wantEmpt {
			t.Errorf("%s：HasAny=%v，应与 IsEmpty=%v 相反", c.name, got, c.wantEmpt)
		}
	}
}

// TestMergeFeaturesFieldWise 锁定层间合并「前层胜出、后层只填空缺」的逐字段语义。
func TestMergeFeaturesFieldWise(t *testing.T) {
	const (
		dstLayer = "zcode"
		srcLayer = "catalog"
	)
	cases := []struct {
		name string
		dst  ModelFeatures
		src  ModelFeatures
		want ModelFeatures
	}{
		{
			name: "dst 全空时整体取 src，Source 跟随 src",
			dst:  ModelFeatures{},
			src:  ModelFeatures{InputModalities: []string{"text", "image"}, ToolCall: featureBool(true), Source: srcLayer},
			want: ModelFeatures{InputModalities: []string{"text", "image"}, ToolCall: featureBool(true), Source: srcLayer},
		},
		{
			name: "dst 已有输入模态时不被 src 改写",
			dst:  ModelFeatures{InputModalities: []string{"text"}},
			src:  ModelFeatures{InputModalities: []string{"video"}},
			want: ModelFeatures{InputModalities: []string{"text"}},
		},
		{
			name: "两边都有模态时各自保留自己的（不做并集）",
			dst:  ModelFeatures{InputModalities: []string{"text"}, OutputModalities: []string{"audio"}},
			src:  ModelFeatures{InputModalities: []string{"image"}, OutputModalities: []string{"video"}},
			want: ModelFeatures{InputModalities: []string{"text"}, OutputModalities: []string{"audio"}},
		},
		{
			name: "dst 缺的字段逐字段由 src 补齐",
			dst:  ModelFeatures{InputModalities: []string{"text"}, StructuredOutput: featureBool(true)},
			src:  ModelFeatures{InputModalities: []string{"video"}, OutputModalities: []string{"text"}, Temperature: featureBool(true)},
			want: ModelFeatures{InputModalities: []string{"text"}, OutputModalities: []string{"text"}, StructuredOutput: featureBool(true), Temperature: featureBool(true)},
		},
		{
			name: "dst 的已知为假不被 src 的已知为真覆盖",
			dst:  ModelFeatures{ToolCall: featureBool(false)},
			src:  ModelFeatures{ToolCall: featureBool(true), NativeWebSearch: featureBool(true)},
			want: ModelFeatures{ToolCall: featureBool(false), NativeWebSearch: featureBool(true)},
		},
		{
			name: "dst 非空时 Source 不跟随 src（Source 只记首个贡献者）",
			dst:  ModelFeatures{ToolCall: featureBool(true)},
			src:  ModelFeatures{NativeWebSearch: featureBool(true), Source: srcLayer},
			want: ModelFeatures{ToolCall: featureBool(true), NativeWebSearch: featureBool(true)},
		},
		{
			name: "dst 只有 Source、两边都没有已知位时保留 dst（此时 Source 无信息）",
			dst:  ModelFeatures{Source: dstLayer},
			src:  ModelFeatures{Source: srcLayer},
			want: ModelFeatures{Source: dstLayer},
		},
		{
			name: "两边都空时原样返回 dst（含其 Source）",
			dst:  ModelFeatures{Source: dstLayer},
			src:  ModelFeatures{},
			want: ModelFeatures{Source: dstLayer},
		},
	}
	for _, c := range cases {
		got := mergeFeatures(c.dst, c.src)
		if !sameFeatures(got, c.want) {
			t.Errorf("%s：got %s，期望 %s", c.name, describeFeatures(got), describeFeatures(c.want))
		}
	}
}

// TestMergeFeaturesClonesSlices 锁定合并时对模态切片做深拷贝：结果不得与被并入的来源
// 共享底层数组，否则调用方事后改一张表的切片会静默改掉已求值的能力位。
func TestMergeFeaturesClonesSlices(t *testing.T) {
	// 路径一：dst 全空，整体取 src。
	src := ModelFeatures{InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"}}
	got := mergeFeatures(ModelFeatures{}, src)
	src.InputModalities[0] = "video"
	src.OutputModalities[0] = "audio"
	if !reflect.DeepEqual(got.InputModalities, []string{"text", "image"}) || !reflect.DeepEqual(got.OutputModalities, []string{"text"}) {
		t.Errorf("dst 全空时未深拷贝：%s", describeFeatures(got))
	}

	// 路径二：dst 已有其它字段，只有模态列被 src 补上。
	src2 := ModelFeatures{OutputModalities: []string{"audio"}}
	got2 := mergeFeatures(ModelFeatures{ToolCall: featureBool(true)}, src2)
	src2.OutputModalities[0] = "video"
	if !reflect.DeepEqual(got2.OutputModalities, []string{"audio"}) {
		t.Errorf("逐字段补空时未深拷贝：%s", describeFeatures(got2))
	}
}

// TestOverlayFeatures 锁定层内「后命中覆盖」：src 里**已知**的字段覆盖 dst 的同名列，
// 未知（nil / 空集合）不动 dst。
func TestOverlayFeatures(t *testing.T) {
	cases := []struct {
		name string
		dst  ModelFeatures
		src  ModelFeatures
		want ModelFeatures
	}{
		{
			name: "src 的模态覆盖 dst 的模态（覆盖方向是后者胜）",
			dst:  ModelFeatures{InputModalities: []string{"text"}, OutputModalities: []string{"text"}},
			src:  ModelFeatures{InputModalities: []string{"video", "image"}, OutputModalities: []string{"audio"}},
			want: ModelFeatures{InputModalities: []string{"video", "image"}, OutputModalities: []string{"audio"}},
		},
		{
			name: "src 未给出的列保持 dst 的值",
			dst:  ModelFeatures{ToolCall: featureBool(true), OutputModalities: []string{"text"}},
			src:  ModelFeatures{InputModalities: []string{"text"}},
			want: ModelFeatures{InputModalities: []string{"text"}, OutputModalities: []string{"text"}, ToolCall: featureBool(true)},
		},
		{
			name: "src 的已知为假也能覆盖 dst 的已知为真（三态由 src 说了算）",
			dst:  ModelFeatures{Temperature: featureBool(true)},
			src:  ModelFeatures{Temperature: featureBool(false)},
			want: ModelFeatures{Temperature: featureBool(false)},
		},
		{
			name: "src 全空时不改动 dst（含 Source）",
			dst:  ModelFeatures{ToolCall: featureBool(true), Source: "zcode"},
			src:  ModelFeatures{},
			want: ModelFeatures{ToolCall: featureBool(true), Source: "zcode"},
		},
		{
			name: "dst 已有 Source 时保留它",
			dst:  ModelFeatures{ToolCall: featureBool(true), Source: "override"},
			src:  ModelFeatures{NativeWebSearch: featureBool(true), Source: "catalog"},
			want: ModelFeatures{ToolCall: featureBool(true), NativeWebSearch: featureBool(true), Source: "override"},
		},
		{
			name: "dst 无 Source 时跟随 src",
			dst:  ModelFeatures{},
			src:  ModelFeatures{NativeWebSearch: featureBool(true), Source: "catalog"},
			want: ModelFeatures{NativeWebSearch: featureBool(true), Source: "catalog"},
		},
	}
	for _, c := range cases {
		got := overlayFeatures(c.dst, c.src)
		if !sameFeatures(got, c.want) {
			t.Errorf("%s：got %s，期望 %s", c.name, describeFeatures(got), describeFeatures(c.want))
		}
	}
}

// TestOverlayFeaturesClonesSlices 与 mergeFeatures 同理：覆盖方向也只搬值不搬底层数组。
func TestOverlayFeaturesClonesSlices(t *testing.T) {
	src := ModelFeatures{InputModalities: []string{"text", "pdf"}}
	got := overlayFeatures(ModelFeatures{}, src)
	src.InputModalities[1] = "video"
	if !reflect.DeepEqual(got.InputModalities, []string{"text", "pdf"}) {
		t.Errorf("overlayFeatures 未深拷贝：%s", describeFeatures(got))
	}
}
