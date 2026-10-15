import { computed, ref, watch, type Ref } from 'vue'
import type { ModelCapabilities, ModelCard } from '../types'
import { CAP_FLAG_KEYS, CAP_INPUT_MODALITIES, CAP_OUTPUT_MODALITIES } from '../types'
import { useToast } from './useToast'
import { t, type UiLang } from '../i18n'

/** 两个模态方向：JSON 键 + 规范序 + 位 ID 前缀（字面量类型，写入时无需再断言）。 */
const DIRECTIONS = [
  { key: 'input_modalities', order: CAP_INPUT_MODALITIES, prefix: 'in:' },
  { key: 'output_modalities', order: CAP_OUTPUT_MODALITIES, prefix: 'out:' },
] as const

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

/**
 * 能力批量补全：管理页与导入页共用同一实现（选项生成 / 计数 / 只补空缺 / 失效选择收敛 / 汇报），
 * 两页只把各自的可见卡片集合与语言传进来。取数方式仿 `useVendorGroup`——选项从当前模型集现算、
 * 带计数，列表变化后把已选但已不存在的位收敛掉（否则下拉会挂无效标签）。
 *
 * 语义（与 MODEL-MANAGEMENT-PLAN 第 10.2.3 的「只写真」一致）：
 * - 未选择任何位 = 补全全部可补项；选中若干位 = 只补这些位。
 * - 只填空缺，已有值一律不覆盖；取消勾选那套「删键 = 未知」由卡片能力区负责，这里从不写 false。
 * - 模态是集合语义（键存在即「已知支持集合」），选中该方向的任一位就整组采纳推断值，不做半填
 *   ——半填会把「未列出的模态」伪造成「确认不支持」。
 * 结果只落在卡片的 `capabilities` 上，由两页既有的卡片载荷负责提交（不新增接口、不直接写盘）。
 */
export function useCapabilityFill(source: () => ModelCard[], lang: Ref<UiLang>) {
  const { push } = useToast()
  /** 已选能力位（空数组 = 补全全部可补项）。 */
  const picked = ref<string[]>([])

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

  function fill() {
    const list = source()
    if (list.length === 0) return
    const chosen = new Set(picked.value)
    const fillAll = chosen.size === 0
    let models = 0
    let bits = 0
    for (const card of list) {
      const inf = card._capabilities_inferred
      if (!inf) continue
      const next: ModelCapabilities = { ...(card.capabilities || {}) }
      let touched = false
      for (const dir of DIRECTIONS) {
        if ((next[dir.key] || []).length > 0) continue
        // 规范序先放宽成 readonly string[] 再比较：DIRECTIONS 是 as const 元组，
        // 元组自身的 includes 只接受字面量联合，直接传 string 会被判为不可赋值。
        const order: readonly string[] = dir.order
        const src = (inf[dir.key] || []).filter((m) => order.includes(m))
        if (src.length === 0) continue
        if (!fillAll && !src.some((m) => chosen.has(dir.prefix + m))) continue
        const filled = order.filter((m) => src.includes(m))
        next[dir.key] = filled
        bits += filled.length
        touched = true
      }
      for (const f of CAP_FLAG_KEYS) {
        if (inf[f] !== true || next[f] !== undefined) continue
        if (!fillAll && !chosen.has(`flag:${f}`)) continue
        next[f] = true
        bits += 1
        touched = true
      }
      if (!touched) continue
      card.capabilities = next
      models += 1
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

  return { picked, options, fill }
}
