package core

import "strings"

// inputModalityOrder 是输入模态的规范序，同时也是 ZCode v2 `inputFormat.supports*` 的键序。
var inputModalityOrder = []string{"text", "image", "video", "audio", "pdf"}

// outputModalityOrder 是输出模态的规范序，取自 opencode `modalities.output` 的 enum 顺序。
var outputModalityOrder = []string{"text", "audio", "image", "video", "pdf"}

// modalityAliases 把各数据源里的常见写法折成规范词。未列出的词一律丢弃（fail-closed），
// 避免把上游的未知取值当成支持项写进配置。
var modalityAliases = map[string]string{
	"image_url":       "image",
	"video_url":       "video",
	"audio_url":       "audio",
	"input_audio":     "audio",
	"application/pdf": "pdf",
	"text/plain":      "text",
}

// ModelFeatures 是 **harness 中立**的模型能力位集合：ZCode v2 / DSH / opencode 与后续
// 新增的 harness 都从这一层取值，再各自映射到自己的键名（映射约束见 MODEL-MANAGEMENT-PLAN.md 第 10.2.3）。
//
// 一律三态：指针为 nil 表示**未知**，不是「不支持」。未知绝不落盘——各 harness 的写入端
// 只写真值，省略键让目标 harness 自己的默认值生效（ZCode 的 overlay 语义里省略 = 不覆盖，
// 而 null = 显式清空，所以这里既不能写 null 也不能写 false）。
//
// InputModalities / OutputModalities 存的是「**已知支持**」的正向集合，空 = 未知。
type ModelFeatures struct {
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
	ToolCall         *bool    `json:"tool_call,omitempty"`
	StructuredOutput *bool    `json:"structured_output,omitempty"`
	NativeWebSearch  *bool    `json:"native_web_search,omitempty"`
	MidConvSystem    *bool    `json:"mid_conv_system,omitempty"`
	Attachment       *bool    `json:"attachment,omitempty"`
	Temperature      *bool    `json:"temperature,omitempty"`
	// Source 记录这批能力值来自哪一层（取值同 ModelCapability.Source），仅供展示与调试；
	// 它是 APM 内部字段（`_` 前缀），任何写入路径都不得把它落盘。
	Source string `json:"_source,omitempty"`
}

// ModelCapabilities 是 ModelFeatures 的展示/编辑名：前端用它承载「能力」区，
// 与内部求值类型是同一个结构，避免两份定义漂移。
type ModelCapabilities = ModelFeatures

// NormalizeInputModalities 只保留规范词表内的输入模态，按 inputModalityOrder 排序去重。
// 空结果返回 nil（配合 DTO 的 omitempty，也不让空切片序列化成 null）。
func NormalizeInputModalities(list []string) []string {
	return normalizeModalities(list, inputModalityOrder)
}

// NormalizeOutputModalities 同上，按 outputModalityOrder 排序去重。
func NormalizeOutputModalities(list []string) []string {
	return normalizeModalities(list, outputModalityOrder)
}

// normalizeModalities 按给定规范序过滤 + 排序 + 去重；未识别的词直接丢弃。
func normalizeModalities(list []string, order []string) []string {
	if len(list) == 0 {
		return nil
	}
	set := map[string]bool{}
	for _, raw := range list {
		word := strings.ToLower(strings.TrimSpace(raw))
		if alias, ok := modalityAliases[word]; ok {
			word = alias
		}
		for _, known := range order {
			if word == known {
				set[word] = true
				break
			}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for _, known := range order {
		if set[known] {
			out = append(out, known)
		}
	}
	return out
}

// hasModality 判断规范化后的集合里是否含某个模态。
func hasModality(list []string, word string) bool {
	want := strings.ToLower(strings.TrimSpace(word))
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// HasInputModality / HasOutputModality 是读取端的便捷判断（写入端不依赖它们）。
func (f ModelFeatures) HasInputModality(word string) bool {
	return hasModality(f.InputModalities, word)
}

func (f ModelFeatures) HasOutputModality(word string) bool {
	return hasModality(f.OutputModalities, word)
}

// IsEmpty 表示没有任何**已知**能力位（全未知）。Source 不计入。
func (f ModelFeatures) IsEmpty() bool {
	return len(f.InputModalities) == 0 && len(f.OutputModalities) == 0 &&
		f.ToolCall == nil && f.StructuredOutput == nil && f.NativeWebSearch == nil &&
		f.MidConvSystem == nil && f.Attachment == nil && f.Temperature == nil
}

// HasAny 与 IsEmpty 相反，供 UI / 日志判断。
func (f ModelFeatures) HasAny() bool { return !f.IsEmpty() }

// featureBool 造一个布尔三态的指针值；写入端的「只写真」判定都经过它。
func featureBool(v bool) *bool {
	b := v
	return &b
}

// mergeFeatures 按「前层胜出、后层只填空缺」把 src 并入 dst，与 ctx/out 的逐字段覆盖同构。
//
// 逐字段语义：dst 的某字段已有已知值就保持不变；未知（nil / 空集合）时取 src 的值。
// Source 只在 dst 完全没有已知能力位时才跟随 src（即「谁先贡献了这批能力值」）；
// 两边都没有已知位时原样返回 dst——没有能力值时 Source 不含任何信息。
func mergeFeatures(dst, src ModelFeatures) ModelFeatures {
	if dst.IsEmpty() {
		if src.IsEmpty() {
			return dst
		}
		out := src
		out.InputModalities = cloneStrings(src.InputModalities)
		out.OutputModalities = cloneStrings(src.OutputModalities)
		return out
	}
	if len(dst.InputModalities) == 0 && len(src.InputModalities) > 0 {
		dst.InputModalities = cloneStrings(src.InputModalities)
	}
	if len(dst.OutputModalities) == 0 && len(src.OutputModalities) > 0 {
		dst.OutputModalities = cloneStrings(src.OutputModalities)
	}
	if dst.ToolCall == nil {
		dst.ToolCall = src.ToolCall
	}
	if dst.StructuredOutput == nil {
		dst.StructuredOutput = src.StructuredOutput
	}
	if dst.NativeWebSearch == nil {
		dst.NativeWebSearch = src.NativeWebSearch
	}
	if dst.MidConvSystem == nil {
		dst.MidConvSystem = src.MidConvSystem
	}
	if dst.Attachment == nil {
		dst.Attachment = src.Attachment
	}
	if dst.Temperature == nil {
		dst.Temperature = src.Temperature
	}
	return dst
}

// overlayFeatures 让 src 里**已知**的字段覆盖 dst 里同名字段（未知不动），用于「同一层内部
// 按顺序扫描、后命中者覆盖」的场景（例如 ZCode 内建规则表的全表扫描）；跨层的「前层胜出、
// 后层填空缺」请用 mergeFeatures。
func overlayFeatures(dst, src ModelFeatures) ModelFeatures {
	if len(src.InputModalities) > 0 {
		dst.InputModalities = cloneStrings(src.InputModalities)
	}
	if len(src.OutputModalities) > 0 {
		dst.OutputModalities = cloneStrings(src.OutputModalities)
	}
	if src.ToolCall != nil {
		dst.ToolCall = src.ToolCall
	}
	if src.StructuredOutput != nil {
		dst.StructuredOutput = src.StructuredOutput
	}
	if src.NativeWebSearch != nil {
		dst.NativeWebSearch = src.NativeWebSearch
	}
	if src.MidConvSystem != nil {
		dst.MidConvSystem = src.MidConvSystem
	}
	if src.Attachment != nil {
		dst.Attachment = src.Attachment
	}
	if src.Temperature != nil {
		dst.Temperature = src.Temperature
	}
	if dst.Source == "" {
		dst.Source = src.Source
	}
	return dst
}

// cloneStrings 复制字符串切片（保持 nil 语义：nil 进 nil 出）。
func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
