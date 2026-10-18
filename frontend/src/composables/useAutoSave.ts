import { ref, type Ref } from 'vue'

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
}

export type AutoSaveStatus = 'idle' | 'saving' | 'saved' | 'paused' | 'error'

export interface AutoSave {
  /** 立即把待保存的改动落盘；无改动、已保存或自动保存关闭时返回 true。离开页面/切换提供商/切换 Agent/退出前调用 */
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

// 自动保存调度器：不做后台定时写入，只在 flush() 时落盘一次。
// 编辑过程中一律不碰磁盘，否则填错的参数会立刻覆盖掉磁盘上的旧值，让「放弃更改」
// 无从回滚；写入只发生在「不保存就会阻塞或退出」的时刻（离开页面、切换提供商/Agent、
// 安装更新、退出应用），由调用方的离开守卫触发一次 flush。
export function useAutoSave(opts: UseAutoSaveOptions): AutoSave {
  const enabled = opts.enabled ?? (() => true)

  const saving = ref(false)
  const status = ref<AutoSaveStatus>('idle')
  const pausedReason = ref<string | null>(null)
  const lastError = ref<string | null>(null)
  const lastSavedAt = ref<number | null>(null)

  // 单飞：当前正在进行的保存；flush 先 await 它，避免两次写入并发。
  let inFlight: Promise<boolean> | null = null

  function errText(e: unknown): string {
    if (e instanceof Error) return e.message
    if (typeof e === 'string') return e
    try { return JSON.stringify(e) } catch { return String(e) }
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

  async function flush(): Promise<boolean> {
    if (!opts.isDirty()) return true
    // 自动保存关闭时不做隐式写入，交给显式保存 / 离开守卫弹窗。
    if (!enabled()) return true
    // 已有写入在进行：先等它结束，保证单飞。
    if (inFlight) await inFlight
    if (!opts.isDirty()) return true
    const reason = preflightReason()
    if (reason !== null) {
      // 预检不通过：置暂停态并返回 false，调用方据此弹窗询问。
      pausedReason.value = reason
      status.value = 'paused'
      return false
    }
    pausedReason.value = null
    return runSave()
  }

  return { flush, saving, status, pausedReason, lastError, lastSavedAt }
}
