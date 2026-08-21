import { ref } from 'vue'

// 未保存更改守卫：Manage 页注册脏检查/保存/放弃回调，
// 页面跳转、切换 Agent、切换提供商前都会先询问是否保存。

let dirtyCheck: (() => boolean) | null = null
let saveFn: (() => Promise<boolean>) | null = null
let discardFn: (() => void) | null = null
let pending: (() => void) | null = null

export const leaveShow = ref(false)
export const leaveSaving = ref(false)

export function registerManageGuard(
  check: (() => boolean) | null,
  save: (() => Promise<boolean>) | null,
  discard: (() => void) | null,
) {
  dirtyCheck = check
  saveFn = save
  discardFn = discard
}

export function leaveDirty(): boolean {
  return dirtyCheck ? dirtyCheck() : false
}

// 尝试离开：无改动则直接执行 action；有改动则弹窗并把 action 暂存。返回是否被拦截。
export function requestLeave(action: () => void): boolean {
  if (!leaveDirty()) {
    action()
    return false
  }
  pending = action
  leaveShow.value = true
  return true
}

export function cancelLeave() {
  pending = null
  leaveShow.value = false
}

export async function confirmLeave(save: boolean) {
  const act = pending
  pending = null
  leaveShow.value = false
  if (!act) return
  if (save && saveFn) {
    leaveSaving.value = true
    try {
      const ok = await saveFn()
      if (ok) act()
    } finally {
      leaveSaving.value = false
    }
  } else {
    if (discardFn) discardFn()
    act()
  }
}
