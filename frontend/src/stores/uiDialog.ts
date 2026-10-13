import { ref } from 'vue'

/**
 * 主题化确认 / 输入弹窗（替代原生 confirm()/prompt()）。
 * 用法：`const ok = await uiConfirm({ ... })`；`const name = await uiPrompt({ ... })`。
 * 宿主为 App.vue 内挂载的 <UiDialogHost />，状态在本模块集中管理。
 */

export interface ConfirmOptions {
  title?: string
  content: string
  /** 次要说明行（路径、影响范围等），可选 */
  detail?: string
  positiveText?: string
  negativeText?: string
  /** 危险操作：确认按钮变 error 色 */
  danger?: boolean
}

export interface PromptOptions {
  title: string
  label?: string
  placeholder?: string
  initialValue?: string
  confirmText?: string
  /** 返回错误提示文案；null/undefined = 通过。校验失败时禁提交。 */
  validate?: (value: string) => string | null | undefined
}

export const confirmState = ref<ConfirmOptions | null>(null)
export const promptState = ref<PromptOptions | null>(null)

let confirmResolve: ((v: boolean) => void) | null = null
let promptResolve: ((v: string | null) => void) | null = null

export function uiConfirm(options: ConfirmOptions): Promise<boolean> {
  // 前一枚未结算的弹窗按「取消」结算，避免 Promise 悬挂
  if (promptResolve) { const r = promptResolve; promptResolve = null; promptState.value = null; r(null) }
  if (confirmResolve) { const r = confirmResolve; confirmResolve = null; confirmState.value = null; r(false) }
  return new Promise<boolean>((resolve) => {
    confirmResolve = resolve
    confirmState.value = options
  })
}

/**
 * 与原生 prompt 语义对齐：取消或输入为空（trim 后）解析为 null，
 * 否则解析为 trim 后的字符串。
 */
export function uiPrompt(options: PromptOptions): Promise<string | null> {
  if (promptResolve) { const r = promptResolve; promptResolve = null; promptState.value = null; r(null) }
  if (confirmResolve) { const r = confirmResolve; confirmResolve = null; confirmState.value = null; r(false) }
  return new Promise<string | null>((resolve) => {
    promptResolve = resolve
    promptState.value = options
  })
}

export function settleConfirm(result: boolean): void {
  confirmState.value = null
  const r = confirmResolve
  confirmResolve = null
  r?.(result)
}

export function settlePrompt(value: string | null): void {
  promptState.value = null
  const r = promptResolve
  promptResolve = null
  r?.(value)
}
