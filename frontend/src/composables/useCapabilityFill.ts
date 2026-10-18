import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { ModelCapabilities, ModelCard } from '../types'
import { CAP_FLAG_KEYS, CAP_INPUT_MODALITIES, CAP_OUTPUT_MODALITIES } from '../types'
import { useToast } from './useToast'
import { uiConfirm } from '../stores/uiDialog'
import { t, type UiLang } from '../i18n'

/** 两个模态方向：JSON 键 + 规范序 + 位 ID 前缀（字面量类型，写入时无需再断言）。 */
const DIRECTIONS = [
  { key: 'input_modalities', order: CAP_INPUT_MODALITIES, prefix: 'in:' },
  { key: 'output_modalities', order: CAP_OUTPUT_MODALITIES, prefix: 'out:' },
] as const

type Direction = (typeof DIRECTIONS)[number]

/**
 * 能力位 ID 用 `in:` / `out:` / `flag:` 前缀拼成一个扁平命名空间：
 * 输入与输出共用同一套模态词（text / image / video / audio / pdf），
 * 不加前缀会在同一个多选里互相混淆。
 */
function bitLabel(id: string, lang: UiLang): string {
  if (id.startsWith('in:')) return `${t('cap.inTag', lang)} ${t(`cap.modal.${id.slice(3)}`, lang)}`
  if (id.startsWith('out:')) return `${t('cap.outTag', lang)} ${t(`cap.modal.${id.slice(4)}`, lang)}`
  return t(`cap.flag.${id.slice(5)}`, lang)
}

/** 规范序：输入模态 -> 输出模态 -> 布尔位（各自按词表顺序），排序不随系统语言漂移。 */
function bitOrder(id: string): number {
  if (id.startsWith('in:')) return (CAP_INPUT_MODALITIES as readonly string[]).indexOf(id.slice(3))
  if (id.startsWith('out:')) return 100 + (CAP_OUTPUT_MODALITIES as readonly string[]).indexOf(id.slice(4))
  return 200 + (CAP_FLAG_KEYS as readonly string[]).indexOf(id.slice(5))
}

/** 该卡片上「可补全」的位：能力表 / provider 推断为真、用户尚未确认，按规范序生成。 */
function bitIds(card: ModelCard): string[] {
  const ids: string[] = []
  const inf = card._capabilities_inferred
  if (!inf) return ids
  const cur = card.capabilities || {}
  for (const m of CAP_INPUT_MODALITIES) if ((inf.input_modalities || []).includes(m) && !(cur.input_modalities || []).includes(m)) ids.push(`in:${m}`)
  for (const m of CAP_OUTPUT_MODALITIES) if ((inf.output_modalities || []).includes(m) && !(cur.output_modalities || []).includes(m)) ids.push(`out:${m}`)
  for (const f of CAP_FLAG_KEYS) if (inf[f] === true && cur[f] === undefined) ids.push(`flag:${f}`)
  return ids
}

/** 一个方向的推断模态集合：按规范序、只保留词表内成员（词表外的推断值一律丢弃）。 */
function inferredModalities(card: ModelCard, dir: Direction): string[] {
  // 元组自身的 includes/顺序比较只接受字面量联合，先放宽成 readonly string[]（同原实现的写法）。
  const order: readonly string[] = dir.order
  const raw = (card._capabilities_inferred || {})[dir.key] || []
  return order.filter((m) => raw.includes(m))
}

/** 一个方向的用户已确认模态集合：按规范序，非词表成员丢弃（与卡片 `toggleModality` 同口径）。 */
function currentModalities(card: ModelCard, dir: Direction): string[] {
  const order: readonly string[] = dir.order
  const raw = (card.capabilities || {})[dir.key] || []
  return order.filter((m) => raw.includes(m))
}

/**
 * 并入（「补全」方向）对单张卡片的结果：`bits > 0` 才返回，`null` 表示这张卡无可补项。
 *
 * 语义（与档位侧 `fillableLevels` 同为「只增不减」）：
 * - 模态方向：`当前集合 ∪ 推断集合`（按规范序重建）。**关键是并集而不是只补缺位**——
 *   只要该方向有推断值就整组并入，因此绝不会把「推断未列出的模态」伪造成「确认不支持」；
 *   已有值一个也不删，所以用户确认过的模态（哪怕能力表不认可）原样保留。
 * - 布尔位：`推断为真 且 当前未确认` 才写 `true`，**从不写 `false`、从不删键**。
 * - 多选限制：`fillAll` 为真时处理全部可补项；否则只处理与所选位有交集的位
 *   （模态按方向整组判定，见 `cap.batchArrayNote`）。
 */
function mergePlan(card: ModelCard, chosen: Set<string>, fillAll: boolean): { next: ModelCapabilities; bits: number } | null {
  const inf = card._capabilities_inferred
  if (!inf) return null
  const next: ModelCapabilities = { ...(card.capabilities || {}) }
  let bits = 0
  for (const dir of DIRECTIONS) {
    const src = inferredModalities(card, dir)
    if (src.length === 0) continue
    if (!fillAll && !src.some((m) => chosen.has(dir.prefix + m))) continue
    const cur = currentModalities(card, dir)
    const added = src.filter((m) => !cur.includes(m))
    if (added.length === 0) continue
    const order: readonly string[] = dir.order
    next[dir.key] = order.filter((m) => cur.includes(m) || src.includes(m))
    bits += added.length
  }
  for (const f of CAP_FLAG_KEYS) {
    if (inf[f] !== true || next[f] !== undefined) continue
    if (!fillAll && !chosen.has(`flag:${f}`)) continue
    next[f] = true
    bits += 1
  }
  return bits > 0 ? { next, bits } : null
}

/**
 * 覆盖（破坏性方向）对单张卡片的结果：与推断集合**不相等的方向**才返回，`null` 表示无需覆盖。
 *
 * 与并入共用同一份推断取值，但判据不同（同源不合并）：覆盖的典型对象是「有值但不等于推断」，
 * 并入对它一个字段都不动（没有缺位）。两处刻意的不对称：
 * 1. **推断为空的方向一律跳过**——把方向置空等于删掉用户确认过的整组模态，
 *    而能力表「没说」不是「不支持」，没有正当依据（与档位侧「推断集合只有 off 就不覆盖」同源）；
 * 2. **布尔位不参与覆盖**：三态里「能力表没声明真」≠「假」，删掉用户勾过的布尔键就是抹掉
 *    已确认的能力。要补的布尔位在「补全」方向下补，覆盖只改模态集合。
 */
function overwritePlan(card: ModelCard): { next: ModelCapabilities; removed: string[] } | null {
  const inf = card._capabilities_inferred
  if (!inf) return null
  const next: ModelCapabilities = { ...(card.capabilities || {}) }
  const removed: string[] = []
  let changed = false
  for (const dir of DIRECTIONS) {
    const target = inferredModalities(card, dir)
    if (target.length === 0) continue
    const cur = currentModalities(card, dir)
    if (cur.length === target.length && target.every((m) => cur.includes(m))) continue
    for (const m of cur) if (!target.includes(m)) removed.push(dir.prefix + m)
    next[dir.key] = [...target]
    changed = true
  }
  return changed ? { next, removed } : null
}

/**
 * 删除清单的展示拼接（中/日用顿号，其它语言用逗号）。与 `useLevelFill` 的 `joinLevels`
 * 同一约定，但两者服务的是不同词表（模态 vs 档位名），因此各留一份而不互相 import。
 */
function joinBits(items: string[], lang: UiLang): string {
  const sep = lang === 'zh' || lang === 'ja' ? '、' : ', '
  return items.join(sep)
}

/**
 * 能力批量补全与覆盖：管理页与导入页共用同一实现（选项生成 / 计数 / 并入 / 覆盖 / 确认 / 汇报），
 * 两页只把各自的可见卡片集合与语言传进来。取数方式仿 `useVendorGroup`——选项从当前模型集现算、
 * 带计数，列表变化后把已选但已不存在的位收敛掉（否则下拉会挂无效标签）。
 *
 * 方向模型与档位侧（`useLevelFill`，见 MODEL-MANAGEMENT-PLAN 10.11）对齐，但**不是同一套判定**：
 * 能力是十几个彼此独立的位（按方向 / 按位分别判定），"只补输入模态、先不确认布尔位"是用户能
 * 精确表达的意图，因此**保留**「选择要补全的位」多选（档位侧因字段是单一整体集合而删掉了它）。
 *
 * 语义（与 MODEL-MANAGEMENT-PLAN 第 10.2.3 的「只写真」一致，见 10.14）：
 * - 并入（默认，`overwrite === false`）＝只增不减，已有值一律不删；从不写 `false`。
 *   未选择任何位 = 补全全部可补项；选中若干位 = 只处理与这些位有交集的位。
 * - 覆盖（`overwrite === true`）＝把该方向的模态集合**替换**为推断集合，会删除推断里没有的
 *   模态；布尔位不参与。破坏性动作，必须先过 `uiConfirm`（`danger` + 列出将删除的模态），
 *   用户取消即什么都不做。多选在覆盖方向下不适用（页面同时禁用它），因此按「全部可覆盖项」执行。
 * - 两个方向都不新增接口、不直接写盘：结果只落在卡片的 `capabilities` 上，由两页既有的
 *   卡片载荷负责提交。
 */
export function useCapabilityFill(source: () => ModelCard[], lang: Ref<UiLang>): {
  picked: Ref<string[]>
  options: ComputedRef<{ label: string; value: string }[]>
  overwrite: Ref<boolean>
  summary: ComputedRef<string>
  fill: () => Promise<void>
} {
  const { push } = useToast()
  /** 已选能力位（空数组 = 补全全部可补项）。覆盖方向下不适用（彻底忽略，见 `chosen`）。 */
  const picked = ref<string[]>([])
  /** 动作方向：`false` = 并入（默认，只增不减），`true` = 覆盖（替换模态集合、会删值）。 */
  const overwrite = ref(false)

  /** 可补全位的多选：标签带计数（= 当前可见模型中该位可补全的数量）。 */
  const options = computed(() => {
    const counts = new Map<string, number>()
    for (const card of source()) {
      for (const id of bitIds(card)) counts.set(id, (counts.get(id) || 0) + 1)
    }
    return [...counts.keys()]
      .sort((a, b) => bitOrder(a) - bitOrder(b))
      .map((id) => ({ label: `${bitLabel(id, lang.value)} (${counts.get(id)})`, value: id }))
  })

  watch(options, (opts) => {
    if (picked.value.length === 0) return
    const alive = new Set(opts.map((o) => o.value))
    const kept = picked.value.filter((v) => alive.has(v))
    if (kept.length !== picked.value.length) picked.value = kept
  })

  /** 本次生效的选择：覆盖方向下整体忽略多选（页面同时禁用该多选，避免"选了却不生效"）。 */
  const chosen = computed<Set<string>>(() => new Set(overwrite.value ? [] : picked.value))
  const fillAll = computed(() => chosen.value.size === 0)

  const fillableCount = computed(() => source().filter((c) => mergePlan(c, chosen.value, fillAll.value) !== null).length)
  const overwriteCount = computed(() => source().filter((c) => overwritePlan(c) !== null).length)

  /** 现成文案：可处理时给计数，一个都处理不了时直接说出原因（与当前方向同口径）。 */
  const summary = computed(() => {
    const n = overwrite.value ? overwriteCount.value : fillableCount.value
    if (n > 0) return t(overwrite.value ? 'cap.batchCountOverwrite' : 'cap.batchCount', lang.value).replace('{n}', String(n))
    return t(overwrite.value ? 'cap.batchNoneOverwrite' : 'cap.batchNone', lang.value)
  })

  async function fill(): Promise<void> {
    const list = source()
    if (list.length === 0) return

    if (overwrite.value) {
      const plans = list.map((card) => ({ card, plan: overwritePlan(card) }))
        .filter((x): x is { card: ModelCard; plan: { next: ModelCapabilities; removed: string[] } } => x.plan !== null)
      if (plans.length === 0) {
        push('info', t('cap.batchLabel', lang.value), t('cap.batchNoneOverwrite', lang.value))
        return
      }
      const removedCounts = new Map<string, number>()
      for (const { plan } of plans) for (const id of plan.removed) removedCounts.set(id, (removedCounts.get(id) || 0) + 1)
      const detail = removedCounts.size > 0
        ? t('cap.batchRemoved', lang.value).replace('{list}', joinBits(
            [...removedCounts.keys()]
              .sort((a, b) => bitOrder(a) - bitOrder(b))
              .map((id) => `${bitLabel(id, lang.value)}×${removedCounts.get(id)}`),
            lang.value,
          ))
        : ''
      const ok = await uiConfirm({
        title: t('cap.batchOverwriteTitle', lang.value),
        content: t('cap.batchOverwriteBody', lang.value).replace('{n}', String(plans.length)),
        detail,
        positiveText: t('cap.batchOverwriteConfirm', lang.value),
        danger: true,
      })
      if (!ok) return
      let removedTotal = 0
      for (const { card, plan } of plans) {
        card.capabilities = plan.next
        removedTotal += plan.removed.length
      }
      push(
        'success',
        t('cap.batchDoneTitle', lang.value),
        t('cap.batchDoneBodyOverwrite', lang.value).replace('{n}', String(plans.length)).replace('{m}', String(removedTotal)),
      )
      return
    }

    let models = 0
    let bits = 0
    for (const card of list) {
      const plan = mergePlan(card, chosen.value, fillAll.value)
      if (!plan) continue
      card.capabilities = plan.next
      models += 1
      bits += plan.bits
    }
    // 一处也没补上时明确说「无补项」，不伪装成功
    if (models === 0) {
      push('info', t('cap.batchLabel', lang.value), t('cap.batchNone', lang.value))
      return
    }
    push(
      'success',
      t('cap.batchDoneTitle', lang.value),
      t('cap.batchDoneBody', lang.value).replace('{n}', String(models)).replace('{m}', String(bits)),
    )
  }

  return { picked, options, overwrite, summary, fill }
}
