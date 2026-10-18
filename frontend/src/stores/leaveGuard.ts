import { ref } from 'vue'

// 未保存更改守卫：Manage 页注册脏检查/保存/放弃回调，
// 页面跳转、切换 Agent、切换提供商前都会先询问是否保存。
//
// 自动保存开启时，离开前会先尝试静默落盘（成功就不打扰用户）；
// 只有确实写不进去（例如新提供商还没填 Base URL）才弹窗询问。
// isUnsavedNew 用于区分「新建但从未落盘」的提供商——丢弃它是真正的数据丢失，
// 弹窗需要据此换成更重的警告文案与按钮。

export interface ManageGuard {
  /** 当前是否有未保存改动 */
  isDirty: () => boolean
  /** 是否是「新建且从未写入配置文件」的提供商 */
  isUnsavedNew?: () => boolean
  /** 显式保存当前表单 */
  save: () => Promise<boolean>
  /** 放弃当前改动 */
  discard: () => void
  /** 自动保存落盘（单飞）：返回是否真的写入成功 */
  flush?: () => Promise<boolean>
  /** 自动保存是否开启：开启时才允许「先静默落盘、不弹窗」这条快路径 */
  autoFlush?: () => boolean
}

let guard: ManageGuard | null = null
let pending: (() => void) | null = null

export const leaveShow = ref(false)
export const leaveSaving = ref(false)
// 当前离开弹窗针对的是「从未落盘的新提供商」——App.vue 据此展示丢弃警告。
export const leaveIsNew = ref(false)

export function registerManageGuard(g: ManageGuard | null) {
  guard = g
}

export function leaveDirty(): boolean {
  return guard ? guard.isDirty() : false
}

export function leaveUnsavedNew(): boolean {
  return guard?.isUnsavedNew ? guard.isUnsavedNew() : false
}

// 尝试离开：能静默落盘或无改动则直接执行 action；否则弹窗并把 action 暂存。
// 返回是否被拦截（弹窗已打开）。异步版本——自动保存开启时会先 await 一次落盘。
export async function requestLeave(action: () => void): Promise<boolean> {
  if (guard && leaveDirty() && guard.autoFlush?.() && guard.flush) {
    // 自动保存开着：先尝试落盘，成功就静默放行，用户切卡片不会被弹窗打断。
    if (await guard.flush()) {
      action()
      return false
    }
  } else if (!leaveDirty()) {
    action()
    return false
  }
  pending = action
  leaveIsNew.value = leaveUnsavedNew()
  leaveShow.value = true
  return true
}

export function cancelLeave() {
  pending = null
  leaveShow.value = false
  leaveIsNew.value = false
}

export async function confirmLeave(save: boolean) {
  const act = pending
  pending = null
  leaveShow.value = false
  if (!act) { leaveIsNew.value = false; return }
  if (save) {
    leaveSaving.value = true
    try {
      // 优先走自动保存的 flush（与调度器共用单飞锁）。但 flush 在「自动保存已关闭」
      // 时不写盘只返回 true，因此只有真正落盘成功才放行，否则退回显式保存，
      // 绝不让点「保存」变成静默丢弃。
      let ok = false
      if (guard?.autoFlush?.() && guard.flush) ok = await guard.flush()
      if (!ok && guard) ok = await guard.save()
      if (ok) {
        leaveIsNew.value = false
        act()
      } else {
        // 保存失败：还原待办动作与弹窗，让用户看到失败原因后可重试或改选丢弃。
        pending = act
        leaveShow.value = true
      }
    } finally {
      leaveSaving.value = false
    }
  } else {
    leaveIsNew.value = false
    if (guard) guard.discard()
    act()
  }
}
