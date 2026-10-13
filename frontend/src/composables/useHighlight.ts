export interface Seg {
  text: string
  hit: boolean
}

/**
 * 搜索命中分段（供模板 <mark v-if="s.hit"> 渲染，禁 v-html）。
 * 大小写不敏感，空查询返回整段未命中。
 */
export function highlightSegments(text: string, query: string): Seg[] {
  const q = query.trim()
  if (!q) return [{ text, hit: false }]
  const lower = text.toLowerCase()
  const ql = q.toLowerCase()
  const segs: Seg[] = []
  let i = 0
  for (;;) {
    const idx = lower.indexOf(ql, i)
    if (idx < 0) {
      segs.push({ text: text.slice(i), hit: false })
      break
    }
    if (idx > i) segs.push({ text: text.slice(i, idx), hit: false })
    segs.push({ text: text.slice(idx, idx + q.length), hit: true })
    i = idx + q.length
  }
  return segs.filter((s) => s.text.length > 0)
}
