package core

import (
	"strconv"
	"strings"
	"sync"
)

// 本文件是 scripts/sync-ability-catalog.mjs 生成的紧凑能力表
// （capability_catalog_gen.go 里的 catalogEntries）的解析与查询入口。
//
// 与 model_catalog_gen.go（ZCode 内建 modelRules 正则表）的分工：
//   - modelRules 是「供应商无关的通配规则」，靠正则在运行时扫描模型名；
//   - 本表是「离线精确 id 目录」，把 pi-ai 离线目录与 models.dev 快照合并成一张
//     id → 能力的字典，查询是 O(1) 精确命中，不参与任何配置写入。
const catalogSource = "catalog"

// lookupCatalogExact 按**小写精确 id** 查离线目录，未命中返回零值 ModelCapability
// （各项零值、Source 为空串）。
//
// 命中时 Source 固定为 "catalog"，并且只填来源真的给出过的字段：
//   - Context / Output：仅 > 0 时填（编码里的 0 表示未知）；
//   - ReasoningLevels：编码里的档位段为空、或规范化后全被白名单滤掉时为 nil；
//   - Vendor：本表不含厂商信息，始终留空 ""（厂商判定仍走 detectVendor）。
//
// modelID 先 TrimSpace + ToLower；空串直接返回零值。目录惰性解析并缓存，
// 任何畸形行都只是被跳过，绝不 panic。
//
// 返回值里的 ReasoningLevels 是**只读共享切片**（取自缓存，多次调用共享同一底层数组）：
// 调用方只能读取或整体赋给 DTO，**不得**就地排序、裁剪或追加。
func lookupCatalogExact(modelID string) ModelCapability {
	id := strings.ToLower(strings.TrimSpace(modelID))
	if id == "" {
		return ModelCapability{}
	}
	return catalogTable()[id]
}

var (
	catalogMu     sync.Mutex
	catalogCache  map[string]ModelCapability
	catalogBuilt  bool
	catalogPolicy bool // 构建缓存时的「允许自定义档位」策略
)

// catalogTable 惰性构建 id → ModelCapability 索引。生成物是紧凑字符串表，逐次查询现解析
// 太贵，因此缓存；解析结果只读，可被多 goroutine 并发读取。
//
// 缓存以「允许自定义档位」策略为键：档位规范化（normalizeCapabilityLevels →
// NormalizeVariants）会读该全局开关，若只按 sync.Once 缓存一次，结果就会取决于
// 首次查询发生时的策略——用户在设置页开关这个选项后目录层会与 override / site 层口径
// 不一致。这里在策略变化时重建，代价只是开关切换后的一次重建。
func catalogTable() map[string]ModelCapability {
	policy := CustomVariantsAllowed()
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if !catalogBuilt || catalogPolicy != policy {
		catalogCache = buildCatalogTable(catalogEntries)
		catalogPolicy = policy
		catalogBuilt = true
	}
	return catalogCache
}

// buildCatalogTable 把紧凑编码表整体解析成能力表，畸形条目直接跳过。
// 抽成纯函数是为了让测试能喂合成表，不必依赖生成物的具体内容。
func buildCatalogTable(src map[string]string) map[string]ModelCapability {
	out := make(map[string]ModelCapability, len(src))
	for id, enc := range src {
		cap, ok := parseCatalogEncoding(enc)
		if !ok {
			continue
		}
		out[id] = cap
	}
	return out
}

// parseCatalogEncoding 解析生成物里的 `ctx|out|levels` 三段编码：
//
//	ctx     十进制整数，0 表示未知；
//	out     十进制整数，0 表示未知；
//	levels  逗号分隔的档位名，空串表示来源未给出档位枚举。
//
// 段数不足、数字非法或为负一律返回 ok==false，由调用方跳过该行（生成物不该有畸形行，
// 但解析路径必须对任何字节都安全：不 panic、不中断整张表）。
func parseCatalogEncoding(enc string) (ModelCapability, bool) {
	parts := strings.SplitN(enc, "|", 3)
	if len(parts) != 3 {
		return ModelCapability{}, false
	}
	context, errCtx := strconv.Atoi(strings.TrimSpace(parts[0]))
	output, errOut := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errCtx != nil || errOut != nil || context < 0 || output < 0 {
		return ModelCapability{}, false
	}
	cap := ModelCapability{Source: catalogSource}
	if context > 0 {
		cap.Context = context
	}
	if output > 0 {
		cap.Output = output
	}
	// 档位串原样收集，白名单过滤 / 别名归一 / 排序全部交给 normalizeCapabilityLevels。
	// 空段返回 nil，而不是空切片，以配合 DTO 的 omitempty。
	if levels := parseCatalogLevels(parts[2]); len(levels) > 0 {
		cap.ReasoningLevels = normalizeCapabilityLevels(levels)
	}
	return cap, true
}

// parseCatalogLevels 把逗号分隔的档位段拆成切片；空元素丢弃，全空返回 nil。
func parseCatalogLevels(seg string) []string {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return nil
	}
	var out []string
	for _, raw := range strings.Split(seg, ",") {
		if s := strings.TrimSpace(raw); s != "" {
			out = append(out, s)
		}
	}
	return out
}
