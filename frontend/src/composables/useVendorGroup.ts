import { computed, ref, watch, type Ref } from 'vue'
import type { ModelCard } from '../types'
import { t, type UiLang } from '../i18n'
import * as api from '../api'

/** 分组开关的持久化键：管理页与导入页共用一个显示偏好（关掉一处即两处都关）。 */
const STORAGE_KEY = 'zcpm.vendorGroup'

export interface VendorGroup {
  vendor: string
  cards: ModelCard[]
}

// 用对象字面量类型别名（而非 interface）：naive-ui 的 SelectOption 带索引签名，
// 只有字面量类型才会被隐式认定为拥有该索引签名，interface 不行（会被判为不可赋给 n-select 的 options）。
export type VendorOption = { label: string; value: string }

/**
 * 卡片所属厂商：`_vendor` 由后端能力表推断（阶段 D），纯展示字段、绝不写盘。
 * 缺失 / 空 / 非字符串一律归入 `unknown`，保证分组与筛选永远不会产生空 key。
 */
export function vendorOf(card: ModelCard | undefined | null): string {
  const raw = card?._vendor
  return typeof raw === 'string' && raw.trim() !== '' ? raw.trim().toLowerCase() : 'unknown'
}

/** unknown 恒排最后，其余按标识符比较（不用 localeCompare，避免随系统语言漂移）。 */
function compareVendor(a: string, b: string): number {
  if (a === b) return 0
  if (a === 'unknown') return 1
  if (b === 'unknown') return -1
  return a < b ? -1 : 1
}

/**
 * 厂商分组 + 筛选的共享状态（管理页、导入页同一实现，不再各写一份）。
 *
 * - `source`：全量卡片 getter，用于生成筛选选项（带计数）；
 * - `lang`：当前语言 ref，用于厂商显示名。
 *
 * 页面侧用法：把 `matches` 串进自己的 `filteredCards`（搜索 + 厂商筛选），
 * 再用 `groupCards(filteredCards)` 生成分组渲染结构；两者都不改批量作用域语义。
 */
export function useVendorGroup(source: () => ModelCard[], lang: Ref<UiLang>) {
  // localStorage 在受限环境下可能抛错（配额 / 隐私模式 / 禁用存储）：
  // 显示偏好丢不起整个页面，读写都降级为「内存态」。
  const groupOn = ref(readStoredGroupOn())
  watch(groupOn, (v) => {
    try {
      localStorage.setItem(STORAGE_KEY, v ? '1' : '0')
    } catch { /* 存储不可用时仅本次会话生效 */ }
  })
  const filter = ref<string[]>([])

  /** 词典缺项时 t() 会原样返回 key，这里回退成厂商标识本身，避免界面出现 `vendor.foo`。 */
  function label(vendor: string): string {
    const key = `vendor.${vendor}`
    const text = t(key, lang.value)
    return text === key ? vendor : text
  }

  /** 厂商筛选判定：未选任何厂商即全部通过（与搜索条件叠加生效）。 */
  function matches(card: ModelCard): boolean {
    const picked = filter.value
    return picked.length === 0 || picked.includes(vendorOf(card))
  }

  const options = computed<VendorOption[]>(() => {
    const counts = new Map<string, number>()
    for (const card of source()) {
      const v = vendorOf(card)
      counts.set(v, (counts.get(v) || 0) + 1)
    }
    return [...counts.entries()]
      .sort((a, b) => compareVendor(a[0], b[0]))
      .map(([v, n]) => ({ label: `${label(v)} (${n})`, value: v }))
  })

  // 列表整体换掉后（重新获取、清空、切换提供商），已选厂商可能已不存在：
  // 留着会让筛选结果恒空且下拉里挂着无效标签，因此按当前实际厂商收敛一次。
  watch(options, (opts) => {
    if (filter.value.length === 0) return
    const alive = new Set(opts.map((o) => o.value))
    const kept = filter.value.filter((v) => alive.has(v))
    if (kept.length !== filter.value.length) filter.value = kept
  })

  /** 按厂商分段，组内保持传入顺序（即搜索结果顺序）。传入的应是已过滤的可见列表。 */
  function groupCards(cards: ModelCard[]): VendorGroup[] {
    const groups = new Map<string, ModelCard[]>()
    for (const card of cards) {
      const v = vendorOf(card)
      const bucket = groups.get(v)
      if (bucket) bucket.push(card)
      else groups.set(v, [card])
    }
    return [...groups.entries()]
      .sort((a, b) => compareVendor(a[0], b[0]))
      .map(([vendor, list]) => ({ vendor, cards: list }))
  }

  return { groupOn, filter, label, matches, options, groupCards }
}

function readStoredGroupOn(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

/** 手动添加的卡片需要补的内部 meta：厂商归属 + 支持档位（两者都只用于展示）。 */
export type VendorMeta = Pick<ModelCard, '_vendor' | '_supported_levels'>

/**
 * 手动添加的模型向能力表要一次内部 meta，使厂商分组与档位弱化对「手动输入」的卡片同样生效
 * （否则它们恒落 未知厂商 / 全档位支持）。
 *
 * 只取 `_` 前缀的推断字段：上下文/输出等业务字段仍保持用户填写的空白，不因手动添加被悄悄填值。
 * 分类只是展示增强，后端不可用或模型未命中时返回 null，由调用方原样添加卡片。
 */
export async function fetchVendorMeta(modelID: string): Promise<VendorMeta | null> {
  try {
    const res = (await api.BuildSingleCard(modelID)) as { success?: boolean; card?: ModelCard } | null
    if (!res || res.success !== true || !res.card) return null
    return { _vendor: res.card._vendor, _supported_levels: res.card._supported_levels }
  } catch {
    return null
  }
}
