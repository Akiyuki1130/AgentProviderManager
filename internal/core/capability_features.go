package core

import "strings"

// capabilityFeatureFlagOrder 是生成表 flags 段的固定字符序，与生成脚本
// scripts/sync-ability-catalog.mjs 共享契约，**两侧任何改动都必须同步**：
//
//	t = ToolCall（工具调用）
//	j = StructuredOutput（JSON Schema / 结构化输出）
//	w = NativeWebSearch（原生联网搜索）
//	m = MidConvSystem（对话中途 system）
//	a = Attachment（附件）
//	T = Temperature（支持温度参数）
const capabilityFeatureFlagOrder = "tjwmaT"

// 生成表 capability_features_gen.go 的编码契约（`map[string]string`，键为小写模型 id）：
//
//	<input modalities>|<output modalities>|<flags>
//
// 例如 "text,image|text|1-1---"：
//
//	第 1 段 输入模态，逗号分隔，按 inputModalityOrder 规范序；未知写空
//	第 2 段 输出模态，逗号分隔，按 outputModalityOrder 规范序；未知写空
//	第 3 段 flags，**恰好 6 个字符**，顺序见 capabilityFeatureFlagOrder，
//	      每字符取 '1'（已知为真）/ '-'（未知）；本表只收录「有能力的 id」，
//	      因此 '0' 与 '-' 在解析上等价（都表示未知，不落盘）。
//
// 解析是 fail-closed 的：段数不为 3、flags 长度不为 6、出现未知行为一律返回零值，
// 宁可「没有能力信息」也不猜。这张表与既有的 `ctx|out|levels` 表（capability_catalog.go）
// **彼此独立**：后者要求恰好三段且是 ctx/out 的强契约，这里另开一张表以免动到它。
func parseFeatureEncoding(enc string) (ModelFeatures, bool) {
	segments := strings.Split(enc, "|")
	if len(segments) != 3 {
		return ModelFeatures{}, false
	}
	flags := segments[2]
	if len(flags) != len(capabilityFeatureFlagOrder) {
		return ModelFeatures{}, false
	}
	features := ModelFeatures{
		InputModalities:  NormalizeInputModalities(strings.Split(segments[0], ",")),
		OutputModalities: NormalizeOutputModalities(strings.Split(segments[1], ",")),
	}
	// 只把 '1' 读成「已知为真」；'0' / '-' / 其它字符都留在未知态。
	bit := func(i int) *bool {
		if i >= len(flags) || flags[i] != '1' {
			return nil
		}
		return featureBool(true)
	}
	features.ToolCall = bit(0)
	features.StructuredOutput = bit(1)
	features.NativeWebSearch = bit(2)
	features.MidConvSystem = bit(3)
	features.Attachment = bit(4)
	features.Temperature = bit(5)
	return features, true
}

// lookupCatalogFeaturesExact 按小写精确 id 查生成表；命中即打上 catalog 来源标记。
func lookupCatalogFeaturesExact(modelID string) ModelFeatures {
	enc, ok := capabilityFeatureTable[strings.ToLower(strings.TrimSpace(modelID))]
	if !ok {
		return ModelFeatures{}
	}
	features, ok := parseFeatureEncoding(enc)
	if !ok || features.IsEmpty() {
		return ModelFeatures{}
	}
	features.Source = "catalog"
	return features
}

// lookupCatalogFeatures 与 lookupCatalogTolerant 同构的「容错精确查询」：先按小写精确 id 查，
// 未命中且 id 含 "/" 时再取最后一个 "/" 之后的一段重查一次，**只在第二次命中时采用**
// （fail-closed，不做子串匹配、不逐段回退）。
func lookupCatalogFeatures(modelID string) ModelFeatures {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ModelFeatures{}
	}
	if hit := lookupCatalogFeaturesExact(id); hit.HasAny() {
		return hit
	}
	if i := strings.LastIndex(id, "/"); i >= 0 {
		if tail := strings.TrimSpace(id[i+1:]); tail != "" {
			if hit := lookupCatalogFeaturesExact(tail); hit.HasAny() {
				return hit
			}
		}
	}
	return ModelFeatures{}
}
