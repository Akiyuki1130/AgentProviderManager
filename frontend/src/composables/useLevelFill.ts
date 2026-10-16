import { computed, ref, type ComputedRef, type Ref } from 'vue'
import type { ModelCard } from '../types'
import { REASONING_VARIANTS, canonicalVariant, pickDefaultVariant } from '../constants/variants'
import { useToast } from './useToast'
import { uiConfirm } from '../stores/uiDialog'
import { t, type UiLang } from '../i18n'

/**
 * 档位（推理 / 思考等级）补全与覆盖：与 `useCapabilityFill` **同源但不合并**。
 *
 * 刻意不合并的理由（2026-10-16 按人工验证反馈修订）：两者只是「批量 + 计数 + 汇总 toast」
 * 这套骨架相似，字段形状与可表达的意图完全不同——能力位是十几个彼此独立的位（按方向 / 按位
 * 分别判定），"只补输入模态、先不确认布尔位"是用户能表达的意图；档位是**单个整体集合字段**
 * （`card.variants` 是「完整声明集合」），写入永远是整组集合，"只补某几个档位"根本无法表达。
 * 因此能力区保留「选择要补全的位」多选，档位这一侧**没有多选**——多选只会得到一个
 * "你选的和你得到的不一样"的控件（旧版的 `level.batchArrayNote` 文案就是在解释这件事）。
 * 档位这侧改成一个动作按钮 + 一个方向开关（并入 / 覆盖）。
 *
 * 本文件是档位这组动作的**唯一实现**：两个页面（管理页 / 导入页）的批量按钮与卡片上的单卡
 * 「采纳推断档位」都走这里的 `fillableLevels` / `overwriteLevels` / `applyLevels`，
 * 避免单卡与批量两套判定漂移。
 *
 * 语义（对应 MODEL-MANAGEMENT-PLAN 第 10.10 节的冻结契约）：
 * - `_supported_levels` 是后端推断值，**只读**，从不回写；
 * - **并入（按钮默认）** = **只增不减**：把推断集合里「当前集合还没有」的档位加进去，
 *   **绝不删除**任何已有档位。集合为空（或只有 off）时并入的结果就等于推断集合本身；
 * - **覆盖（勾选复选框）** = 把 `variants` **直接置为推断集合**，会删除推断里没有的档位。
 *   这是唯一能清理「APM 早期盲目写入的档位」（实测用户两个网关 90 个 id 里有 43 个带并不存在的
 *   off / max）的动作，因此是破坏性的：必须过 `uiConfirm`，并在确认文案里列出将被删除的档位名。
 *   为什么并入不能顺手清理：删除档位等于声明"其余档位不支持"的方向是不对称的，
 *   而"推断集合里没有的旧档位"属于**上一次写盘的既成事实**，清理必须是用户的显式动作；
 * - 两个动作的**共同门控**：`reasoning === true`（思考未开是用户的显式决定，而且此时落盘会把
 *   档位集合收敛成单个 off 词，写进去的值会被静默丢弃）；推断集合里必须至少有一个非 off 档
 *   （只有 off 等于「只能关闭思考」，与「思考已开」自相矛盾，写它没有意义）；
 * - 结果只落在卡片的 `variants` / `default_variant` 上，由两页既有载荷负责提交（不新增接口、
 *   不直接写盘）。
 */

/**
 * 档位的规范序：ZCode 档位全集（`REASONING_VARIANTS`）的下标。
 * 不在全集里的档位（例如二值思考模型的 `enabled`）排在最后，且**同序内部保持调用方给定的顺序**
 * （`Array.prototype.sort` 稳定，键来源为 `source()` 的确定遍历序，因此不会随系统语言漂移）。
 */
function levelOrder(level: string): number {
  const idx = (REASONING_VARIANTS as readonly string[]).indexOf(level)
  return idx === -1 ? REASONING_VARIANTS.length : idx
}

/** 按规范序稳定排序（不改入参）。 */
function sortLevels(levels: string[]): string[] {
  return [...levels].sort((a, b) => levelOrder(a) - levelOrder(b))
}

/**
 * 该卡片上后端推断的支持档位：读 `_supported_levels`（后端已规范化并按档位全集顺序排过序），
 * 这里只做保险的归一 / 去空 / 去重 / **保序**：档位名走共享的 `canonicalVariant`
 * （大小写与 off 别名折叠，与后端 `tokens.go` 同口径），非字符串元素直接丢弃。
 *
 * 为什么这两种保险是必要的：本文件的判定用 `'off'` 字面量比较，而落盘侧会把 `disabled`
 * 之类的拼法折成 `off`——若推断裂表里出现 `Disabled` 这种拼法，判定会认为「没有 off 档」而
 * 放行，写盘折叠成 `["off"]` 后重读又变成「思考已关」，状态与用户看到的对不上。
 * 无推断值则返回 `[]`。纯只读，绝不回写。
 */
export function inferredLevels(card: ModelCard): string[] {
  const raw = card._supported_levels
  if (!raw || raw.length === 0) return []
  const out: string[] = []
  const seen = new Set<string>()
  for (const item of raw) {
    if (typeof item !== 'string') continue
    const v = canonicalVariant(item)
    if (!v || seen.has(v)) continue
    seen.add(v)
    out.push(v)
  }
  return out
}

/** 该卡片缺失的推断档位（推断集合里当前还没有的）：`[]` 表示没有可并入的。 */
function missingLevels(card: ModelCard): string[] {
  const cur = card.variants || []
  return inferredLevels(card).filter((v) => !cur.includes(v))
}

/**
 * 该卡片**还缺**的档位：可补时返回整组要写入的档位（新数组 = 当前集合 ∪ 缺失档位），
 * 否则 `null`。单卡按钮与批量并入共用这一判定，是唯一实现。依次判定：
 * 1. `reasoning !== true` ⇒ `null`：思考未开是用户的显式决定，不动它
 *    （「开思考」那条路径本来就由卡片的 `toggleReasoning` 用 `_supported_levels` 做种子）；
 * 2. 推断集合里没有非 off 档（空、或只有 off）⇒ `null`：只有 off 等于「只能关闭思考」，
 *    与「思考已开」自相矛盾，补它没有意义；
 * 3. 推断集合的档位当前都已存在 ⇒ `null`：无可补（避免虚报，也让单卡按钮自动隐藏）；
 * 4. 否则返回「当前集合 ∪ 缺失档位」并**按规范序排序**。
 *
 * 注意：判定里**没有**「当前集合已有非 off 档就跳过」这一条——那会让本功能在存量配置上
 * 完全失效（APM 早期保存的卡片档位是写死的 `[off,high,max]`，恰恰是最需要补的一批）。
 */
export function fillableLevels(card: ModelCard): string[] | null {
  if (card.reasoning !== true) return null
  const inferred = inferredLevels(card)
  if (!inferred.some((v) => v !== 'off')) return null
  const missing = missingLevels(card)
  if (missing.length === 0) return null
  return sortLevels([...(card.variants || []), ...missing])
}

/**
 * 该卡片**需要覆盖**时才返回推断集合（按规范序），否则 `null`。与 `fillableLevels` 同源
 * （共用 `inferredLevels` 与同一套门控）但**不是同一个判定**，两处差异都是刻意的：
 *
 * 1. 判据是「推断集合与当前集合**不相等**」，而不是「当前集合缺档位」。反例：当前
 *    `[off,high,max,medium]`、推断 `[high,max]` 时没有缺失档位，但正是覆盖模式最该处理的卡片
 *    （它有两个多余的档位）——若复用 `fillableLevels`，这类卡片一张都不会被处理；
 * 2. 空集合 / 只有 off 的推断仍然跳过（同 `fillableLevels` 第 1、2 条）：否则覆盖会把
 *    `variants` 清空或置成 `["off"]`，那是**静默把用户的思考关掉**，比"少补"危险得多。
 *
 * 相等判定忽略顺序（集合语义）：档位顺序由 `sortLevels` 统一，写盘不依赖调用方顺序。
 */
export function overwriteLevels(card: ModelCard): string[] | null {
  if (card.reasoning !== true) return null
  const inferred = inferredLevels(card)
  if (!inferred.some((v) => v !== 'off')) return null
  const cur = card.variants || []
  const target = sortLevels(inferred)
  if (cur.length === target.length && target.every((v) => cur.includes(v))) return null
  return target
}

/**
 * 把 `levels` 整组写入 `card.variants`（拷贝，不与入参共享引用）。
 * 若 `card.default_variant` 为空或不在 `levels` 内，用 `pickDefaultVariant` 修正
 * ——默认档位规则只此一处，不另写一套（并入只增不减，因此已有且合法的默认档位不会被改动；
 * 覆盖删掉的默认档位则由这里回落，与后端 `CardToCfg` 的回落同口径）。
 * 只改这两个字段，不 emit、不落盘。
 */
export function applyLevels(card: ModelCard, levels: string[]): void {
  const next = [...levels]
  card.variants = next
  if (!card.default_variant || !next.includes(card.default_variant)) {
    card.default_variant = pickDefaultVariant(next)
  }
}

/**
 * 该卡片「本可处理但因思考未开被门控挡下」：推断里有非 off 档（说明能力表确实知道档位），
 * 而思考当前是关的。只用于把"点了没反应"解释成具体原因，不参与写入判定。
 */
function blockedByReasoning(card: ModelCard): boolean {
  return card.reasoning !== true && inferredLevels(card).some((v) => v !== 'off')
}

/**
 * 档位名列表的展示拼接（中/日用顿号，其它语言用逗号）。
 * 确认弹窗的「将删除」清单与卡片悬浮提示共用，避免两处各写一套分隔符约定。
 */
export function joinLevels(items: string[], lang: UiLang): string {
  const sep = lang === 'zh' || lang === 'ja' ? '、' : ', '
  return items.join(sep)
}

/** 覆盖模式的聚合预览（确认弹窗与计数都用它，避免两处各算一遍）。 */
interface OverwritePlan {
  /** 会被覆盖的模型数（= `overwriteLevels` 非空的卡片数）。 */
  models: number
  /** 将被删除的档位名 → 涉及模型数，按规范序排列。 */
  removed: { level: string; count: number }[]
  /** 覆盖后集合不再含 off 的模型数：这些卡片的思考开关会变灰（S1 语义），确认文案要提醒。 */
  loseOff: number
  /** 因思考未开被跳过的模型数（用于说明"为什么一个都没处理"）。 */
  blocked: number
}

function overwritePlan(cards: ModelCard[]): OverwritePlan {
  const removedCounts = new Map<string, number>()
  let models = 0
  let loseOff = 0
  let blocked = 0
  for (const card of cards) {
    if (blockedByReasoning(card)) blocked += 1
    const target = overwriteLevels(card)
    if (!target) continue
    models += 1
    const cur = card.variants || []
    for (const v of cur) if (!target.includes(v)) removedCounts.set(v, (removedCounts.get(v) || 0) + 1)
    if (cur.includes('off') && !target.includes('off')) loseOff += 1
  }
  const removed = [...removedCounts.keys()]
    .sort((a, b) => levelOrder(a) - levelOrder(b))
    .map((level) => ({ level, count: removedCounts.get(level) || 0 }))
  return { models, removed, loseOff, blocked }
}

/**
 * 档位批量动作：管理页与导入页共用同一实现（计数 / 并入 / 覆盖 / 确认 / 汇报），
 * 两页只把各自的可见卡片集合与语言传进来。
 *
 * - `overwrite`：方向开关。`false` = 并入（只增不减），`true` = 覆盖（以推断集合替换）。
 *   **默认 false，且不持久化**——破坏性方向不该被粘住；
 * - `targetCount`：当前方向下会被处理的模型数（0 时页面禁用按钮）；
 * - `summary`：计数提示文案（含"一个都处理不了"的原因），页面直接渲染，逻辑只此一处；
 * - `fill()`：执行动作。覆盖模式下先过 `uiConfirm`（danger + 列出将删除的档位），
 *   用户取消则什么都不做；一处也没处理时明确说原因，不伪装成功。
 */
export function useLevelFill(source: () => ModelCard[], lang: Ref<UiLang>): {
  overwrite: Ref<boolean>
  targetCount: ComputedRef<number>
  summary: ComputedRef<string>
  fill: () => Promise<void>
} {
  const { push } = useToast()
  /** 覆盖方向开关（默认并入）。 */
  const overwrite = ref(false)

  const fillableCount = computed(() => source().filter((c) => fillableLevels(c) !== null).length)
  const overwriteCount = computed(() => source().filter((c) => overwriteLevels(c) !== null).length)
  const blockedCount = computed(() => source().filter(blockedByReasoning).length)

  const targetCount = computed(() => (overwrite.value ? overwriteCount.value : fillableCount.value))

  const summary = computed(() => {
    if (targetCount.value > 0) {
      const key = overwrite.value ? 'level.batchCountOverwrite' : 'level.batchCount'
      return t(key, lang.value).replace('{n}', String(targetCount.value))
    }
    // 一个都处理不了时把原因说出来：最常见的原因是这批模型思考没开（门控），
    // 而不是"能力表没数据"，两者对用户的下一步动作完全不同。
    if (blockedCount.value > 0) return t('level.batchNoneReasoning', lang.value).replace('{n}', String(blockedCount.value))
    return t('level.batchNone', lang.value)
  })

  async function fill(): Promise<void> {
    const list = source()
    if (list.length === 0) return

    if (overwrite.value) {
      const plan = overwritePlan(list)
      if (plan.models === 0) {
        push('info', t('level.batchLabel', lang.value), plan.blocked > 0
          ? t('level.batchNoneReasoning', lang.value).replace('{n}', String(plan.blocked))
          : t('level.batchNone', lang.value))
        return
      }
      const detailParts: string[] = []
      if (plan.removed.length > 0) {
        const joined = joinLevels(plan.removed.map((r) => `${r.level}×${r.count}`), lang.value)
        detailParts.push(t('level.batchRemoved', lang.value).replace('{list}', joined))
      }
      if (plan.loseOff > 0) {
        detailParts.push(t('level.batchLoseOff', lang.value).replace('{n}', String(plan.loseOff)))
      }
      const ok = await uiConfirm({
        title: t('level.batchOverwriteTitle', lang.value),
        content: t('level.batchOverwriteBody', lang.value).replace('{n}', String(plan.models)),
        detail: detailParts.join(' · '),
        positiveText: t('level.batchOverwriteConfirm', lang.value),
        danger: true,
      })
      if (!ok) return
      let models = 0
      let removedTotal = 0
      for (const card of list) {
        const target = overwriteLevels(card)
        if (!target) continue
        const before = card.variants || []
        const removed = before.filter((v) => !target.includes(v)).length
        applyLevels(card, target)
        models += 1
        removedTotal += removed
      }
      push(
        'success',
        t('level.batchDoneTitle', lang.value),
        t('level.batchDoneBodyOverwrite', lang.value)
          .replace('{n}', String(models))
          .replace('{m}', String(removedTotal)),
      )
      return
    }

    let models = 0
    let tiers = 0
    let blocked = 0
    for (const card of list) {
      if (blockedByReasoning(card)) blocked += 1
      const target = fillableLevels(card)
      if (!target) continue
      // 增量必须在 `applyLevels` **之前**算：`applyLevels` 会把缺失档位并入 `card.variants`，
      // 之后 `missingLevels(card)` 恒为空集，`tiers` 会永远报 0（与上方覆盖路径把
      // `before`/`removed` 算在 `applyLevels` 之前一致）。
      const add = missingLevels(card).length
      applyLevels(card, target)
      models += 1
      tiers += add
    }
    // 一处也没补上时明确说「无补项」并给出最常见的原因，不伪装成功
    if (models === 0) {
      push('info', t('level.batchLabel', lang.value), blocked > 0
        ? t('level.batchNoneReasoning', lang.value).replace('{n}', String(blocked))
        : t('level.batchNone', lang.value))
      return
    }
    push(
      'success',
      t('level.batchDoneTitle', lang.value),
      t('level.batchDoneBody', lang.value).replace('{n}', String(models)).replace('{m}', String(tiers)),
    )
  }

  return { overwrite, targetCount, summary, fill }
}
