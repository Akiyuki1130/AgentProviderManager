/**
 * 错峰入场时长缓存：按稳定 id 计算 animation-delay，条目换序/重渲染不重播闪烁。
 * 列表条目用 cachedStaggerMs(id)；页面级卡片直接 staggerMs(index)。
 */
const delays = new Map<string, number>()
let assigned = 0

export function staggerMs(index: number, step = 60): number {
  return step * index
}

export function cachedStaggerMs(id: string, base = 60, step = 40): number {
  let d = delays.get(id)
  if (d === undefined) {
    d = base + assigned++ * step
    if (delays.size > 240) delays.clear(), (assigned = 0) // 防长会话无界增长
    delays.set(id, d)
  }
  return d
}
