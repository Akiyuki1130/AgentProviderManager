package core

import "strings"

// modelOverride 是 APM 自维护的能力覆盖层，优先于 ZCode 内建数据（model_catalog_gen.go）。
//
// 这个表是**给人手改的**：当脚本抽不到的家族、内建数据在 ZCode 升级后改动、或 APM 期望
// 与客户端口径不同时，在这里显式补齐/修正。语义是「**显式给出即胜出**」——某个字段填了
// （Context/Output > 0，Levels 非空，Vendor 非空）就覆盖后续来源（zcode 正则、presets 旧表）；
// 留空则直接沿用后续来源，不做「清零」。
//
// Match 的匹配规则与 presets.go 的 LookupModelLimits 一致：小写精确 ID 优先，其次最长前缀
// （覆盖带日期/后缀的快照 id）。**请在填写前确认数值可公开核实，不确定的模型不要编。**
//
// ⚠️ 前缀匹配的坑（修 S2）：`Match` 是**前缀**语义，键太短会静默波及一族兄弟模型——例如
// `Match: "gpt-5"` 会连带命中 `gpt-5.6-luna`、`gpt-5.4-pro` 等。因此：**优先用完整 ID**；
// 确需前缀覆盖整个家族时，键要足够长且该家族各成员的 ctx/out/档位确实一致。
type modelOverride struct {
	Match   string   // 精确 ID（小写）或最长前缀
	Context int      // 0 = 不覆盖
	Output  int      // 0 = 不覆盖
	Levels  []string // 原始拼写，交给 NormalizeVariants；nil = 不覆盖
	Vendor  string   // 空 = 不覆盖
}

// modelOverrides 是覆盖层数据。当前只放用户点名的 deepseek-v4.1-flash：它虽已被内建正则
// （modelRules 的 `.*deepseek-v4[.-]1-flash`）覆盖，但该家族此前长期落不进旧 presets 表，
// 这里显式固化一份作为「内建数据将来变动时的保险」，同时作为覆盖层可用性的活样例。
var modelOverrides = []modelOverride{
	{Match: "deepseek-v4.1-flash", Context: 1000000, Output: 384000, Levels: []string{"disabled", "low", "high", "max"}, Vendor: "deepseek"},
}

// matchOverride 在覆盖层里查 modelID：精确 ID（小写）优先，其次最长前缀。命中返回该条目。
func matchOverride(modelID string, overrides []modelOverride) (modelOverride, bool) {
	low := strings.ToLower(strings.TrimSpace(modelID))
	if low == "" {
		return modelOverride{}, false
	}
	for _, ov := range overrides {
		if strings.ToLower(strings.TrimSpace(ov.Match)) == low {
			return ov, true
		}
	}
	best := modelOverride{}
	bestLen := 0
	for _, ov := range overrides {
		key := strings.ToLower(strings.TrimSpace(ov.Match))
		if key == "" {
			continue
		}
		if len(key) > bestLen && strings.HasPrefix(low, key) {
			best, bestLen = ov, len(key)
		}
	}
	if bestLen == 0 {
		return modelOverride{}, false
	}
	return best, true
}
