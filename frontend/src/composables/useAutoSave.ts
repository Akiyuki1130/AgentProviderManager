import { ref, watch, type Ref } from 'vue'

export interface AutoSaveSaveResult { ok: boolean; error?: string }

export interface UseAutoSaveOptions {
  /** 同步读取“当前是否有未保存改动” */
  isDirty: () => boolean
  /** 执行一次保存；ok=false 时 error 是给用户看的原因（后端原文即可） */
  save: () => Promise<AutoSaveSaveResult>
  /** 预检：非 null 表示本次不宜自动保存，返回值是暂停原因（调用方已 i18n）；null 表示可以保存 */
  preflight?: () => string | null
  /** 是否启用；默认 () => true */
  enabled?: () => boolean
  /** 停止输入后多久触发，默认 1200ms */
  debounceMs?: number
  /** 两次自动保存之间的最小间隔，默认 5000ms（合并连续改动，控制还原点增长） */
  minIntervalMs?: number
}

export type AutoSaveStatus = 'idle' | 'pending' | 'saving' | 'saved' | 'paused' | 'error'

export interface AutoSave {
  /** 组件挂载后调用，开始监视 isDirty 变化 */
  start(): void
  /** 组件卸载时调用，清除定时器与 watch */
  stop(): void
  /** 立即把待保存的改动落盘；无改动或已保存返回 true。守卫/离开页面/切换提供商前调用 */
  flush(): Promise<boolean>
  saving: Ref<boolean>
  status: Ref<AutoSaveStatus>
  /** preflight 返回的暂停原因；无则 null */
  pausedReason: Ref<string | null>
  /** 最近一次自动保存失败的后端原文；无则 null */
  lastError: Ref<string | null>
  /** 最近一次成功自动保存的时间戳（Date.now()）；无则 null */
  lastSavedAt: Ref<number | null>
}

const DEFAULT_DEBOUNCE_MS = 1200
const DEFAULT_MIN_INTERVAL_MS = 5000

// 自动保存调度器：单飞写入 + 合并连续改动 + 静默失败（不向调用方抛错）。
export function useAutoSave(opts: UseAutoSaveOptions): AutoSave {
  const debounceMs = opts.debounceMs ?? DEFAULT_DEBOUNCE_MS
  const minIntervalMs = opts.minIntervalMs ?? DEFAULT_MIN_INTERVAL_MS
  const enabled = opts.enabled ?? (() => true)

  const saving = ref(false)
  const status = ref<AutoSaveStatus>('idle')
  const pausedReason = ref<string | null>(null)
  const lastError = ref<string | null>(null)
  const lastSavedAt = ref<number | null>(null)

  let timer: ReturnType<typeof setTimeout> | null = null
  let stopWatch: (() => void) | null = null
  // 单飞：当前正在进行的保存；flush 先 await 它，避免两次写入并发。
  let inFlight: Promise<boolean> | null = null

  function errText(e: unknown): string {
    if (e instanceof Error) return e.message
    if (typeof e === 'string') return e
    try { return JSON.stringify(e) } catch { return String(e) }
  }
  function clearTimer() { if (timer !== null) { clearTimeout(timer); timer = null } }

  // 排入/重排防抖定时器；每次重排都丢弃上一次，天然实现“最后一次改动才触发”。
  function schedule(delay = debounceMs) {
    clearTimer()
    timer = setTimeout(() => { timer = null; void attempt() }, delay)
  }
  // 预检异常按“暂停”处理，理由取异常信息，保证不把异常抛给调用方。
  function preflightReason(): string | null {
    try { return opts.preflight ? opts.preflight() : null } catch (e) { return errText(e) }
  }

  // 一次真正的写入；所有异常/拒绝都归化成失败，永不 reject。
  function runSave(): Promise<boolean> {
    const p = (async () => {
      saving.value = true
      status.value = 'saving'
      try {
        const res = await opts.save()
        if (!res || res.ok !== true) {
          lastError.value = (res && res.error) ?? ''
          status.value = 'error'
          return false
        }
        lastError.value = null
        lastSavedAt.value = Date.now()
        status.value = 'saved'
        return true
      } catch (e) {
        lastError.value = errText(e)
        status.value = 'error'
        return false
      } finally {
        saving.value = false
      }
    })()
    inFlight = p
    void p.finally(() => { if (inFlight === p) inFlight = null })
    return p
  }

  // 定时器到点：单飞检查 → 改动检查 → minInterval 合并 → 预检 → 写入 → 收尾补排。
  async function attempt() {
    if (!enabled() || saving.value) return
    if (!opts.isDirty()) return
    // 距上次成功保存不足 minIntervalMs：本轮不写，只把定时器重排到剩余时间。
    if (lastSavedAt.value !== null) {
      const wait = minIntervalMs - (Date.now() - lastSavedAt.value)
      if (wait > 0) { schedule(wait); return }
    }
    const reason = preflightReason()
    if (reason !== null) {
      // 预检不通过：只置暂停态，不排重试——等用户下一次改动再触发。
      pausedReason.value = reason
      status.value = 'paused'
      return
    }
    pausedReason.value = null
    const ok = await runSave()
    // 写入期间用户仍在输入：最后状态还需再落一次，重新排队（不并发写）。
    if (ok && opts.isDirty()) schedule()
  }

  function start() {
    if (stopWatch) return // 重复 start 不叠加 watcher / 定时器
    stopWatch = watch(() => opts.isDirty(), (dirty) => {
      if (!dirty) {
        // 回到“不脏”：取消待触发定时器，避免无谓写入。
        clearTimer()
        if (status.value === 'pending') status.value = 'idle'
        return
      }
      if (enabled()) { status.value = 'pending'; schedule() }
    })
  }

  function stop() {
    if (stopWatch) { stopWatch(); stopWatch = null }
    clearTimer()
  }

  async function flush(): Promise<boolean> {
    clearTimer()
    if (!opts.isDirty()) return true
    // 自动保存关闭时不做隐式写入，交给显式保存 / 离开守卫。
    if (!enabled()) return true
    // 已有写入在进行：先等它结束，保证单飞。
    if (inFlight) await inFlight
    if (!opts.isDirty()) return true
    const reason = preflightReason()
    if (reason !== null) {
      pausedReason.value = reason
      status.value = 'paused'
      return false
    }
    pausedReason.value = null
    return runSave() // flush 忽略 minIntervalMs：显式落盘必须立即执行
  }

  return { start, stop, flush, saving, status, pausedReason, lastError, lastSavedAt }
}
