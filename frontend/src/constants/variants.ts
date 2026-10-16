// 推理档位的「展示子集」（顺序固定，前端胶囊 / 默认档位下拉的选项顺序真源）。
// 这不是后端枚举全集：ZCode 的 optionSpecs.reasoningLevel.values 由每个模型各写各的，
// 取值除下列 7 档外，还可含：
//   - enabled（如二值思考模型 ["disabled","enabled"]、["enabled"]）；
//   - off 别名 disabled / none / nothink，经后端 canonicalVariant 归一为 off。
// 因此含 enabled 的模型会在卡片层把它并入选项（见 ModelCard.vue 的 variantOptions），
// 使 enabled 在前端可见、可编辑。DSH 的 7 档键与此表同集。
// off 为「关闭思考」档——ZCode / DSH 都靠选它表达关闭，无独立思考开关字段；
// minimal 为合法独立档位（不再被折成 low）。
export const REASONING_VARIANTS = ['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'] as const

// 关闭「单模型思考默认全部强度」时写入的基础三档兜底。
export const REASONING_VARIANTS_BASIC = ['off', 'high', 'max'] as const

// 自定义档位名的长度上限，与后端 core.MaxVariantLen 保持一致（后端在
// NormalizeVariants 里对非白名单档位做最终拒绝，这里只用于前端提前限长）。
export const MAX_VARIANT_LEN = 64

/**
 * 档位名归一：与后端 `internal/core/tokens.go` 的 `canonicalVariant` 同一口径——
 * 去空白 + 小写，并把 off 的等价别名 none / disabled / nothink 折成 `off`
 * （minimal 不折 low，A-6）。前端所有「用户输入 / 后端推断值 → 档位名」的比较都走它，
 * 避免同一档位以多个等价拼法并存（那会让落盘折叠与前端字面量比较对不上）。
 */
export function canonicalVariant(raw: string): string {
  const v = raw.trim().toLowerCase()
  return v === 'none' || v === 'disabled' || v === 'nothink' ? 'off' : v
}

/**
 * 在给定档位集合内挑一个合理的默认档位：
 * 优先 high，其次首个非 off 档位，再次首个元素，空集回落空串。
 * 后端已容错（不再因集合不含 high 报错），但前端也应避免制造矛盾数据。
 */
export function pickDefaultVariant(variants: readonly string[]): string {
  if (variants.includes('high')) return 'high'
  const firstNonOff = variants.find((v) => v !== 'off')
  return firstNonOff ?? variants[0] ?? ''
}
