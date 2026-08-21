<template>
  <Teleport to="body">
    <div
      v-if="visible"
      ref="menuRef"
      class="edit-context-menu"
      :style="menuStyle"
      role="menu"
      @mousedown.prevent.stop
    >
      <button type="button" role="menuitem" :disabled="!canCut" @click="runAction('cut')">剪切</button>
      <button type="button" role="menuitem" :disabled="!canCopy" @click="runAction('copy')">复制</button>
      <button type="button" role="menuitem" :disabled="!canPaste" @click="runAction('paste')">粘贴</button>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { ClipboardGetText, ClipboardSetText } from '../../wailsjs/runtime/runtime'

type EditableTarget = HTMLInputElement | HTMLTextAreaElement
type EditAction = 'cut' | 'copy' | 'paste'

const visible = ref(false)
const menuRef = ref<HTMLElement | null>(null)
const menuX = ref(0)
const menuY = ref(0)
const activeTarget = ref<EditableTarget | null>(null)
const selectionStart = ref(0)
const selectionEnd = ref(0)
const targetReadOnly = ref(false)
const targetDisabled = ref(false)

const hasSelection = computed(() => selectionEnd.value > selectionStart.value)
const canCut = computed(() => hasSelection.value && !targetReadOnly.value && !targetDisabled.value)
const canCopy = computed(() => hasSelection.value)
const canPaste = computed(() => !targetReadOnly.value && !targetDisabled.value)
const menuStyle = computed(() => ({ left: `${menuX.value}px`, top: `${menuY.value}px` }))

function editableTarget(eventTarget: EventTarget | null): EditableTarget | null {
  if (!(eventTarget instanceof Element)) return null
  const element = eventTarget.closest('input, textarea')
  if (element instanceof HTMLTextAreaElement) return element
  if (!(element instanceof HTMLInputElement)) return null
  const blockedTypes = new Set(['button', 'checkbox', 'color', 'date', 'datetime-local', 'file', 'hidden', 'image', 'month', 'radio', 'range', 'reset', 'submit', 'time', 'week'])
  return blockedTypes.has(element.type.toLowerCase()) ? null : element
}

function readSelection(target: EditableTarget) {
  try {
    return {
      start: target.selectionStart ?? 0,
      end: target.selectionEnd ?? target.selectionStart ?? 0,
    }
  } catch {
    return { start: 0, end: 0 }
  }
}

function restoreSelection(target: EditableTarget) {
  target.focus({ preventScroll: true })
  try { target.setSelectionRange(selectionStart.value, selectionEnd.value) } catch { /* unsupported input types */ }
}

function closeMenu() {
  visible.value = false
  activeTarget.value = null
}

async function showMenu(event: MouseEvent) {
  const target = editableTarget(event.target)
  if (!target) return

  event.preventDefault()
  const selection = readSelection(target)
  activeTarget.value = target
  selectionStart.value = selection.start
  selectionEnd.value = selection.end
  targetReadOnly.value = target.readOnly
  targetDisabled.value = target.disabled
  menuX.value = event.clientX
  menuY.value = event.clientY
  visible.value = true

  await nextTick()
  const menu = menuRef.value
  if (!menu) return
  const margin = 8
  menuX.value = Math.min(Math.max(margin, menuX.value), Math.max(margin, window.innerWidth - menu.offsetWidth - margin))
  menuY.value = Math.min(Math.max(margin, menuY.value), Math.max(margin, window.innerHeight - menu.offsetHeight - margin))
}

async function getClipboardText(): Promise<string> {
  try {
    return await ClipboardGetText()
  } catch {
    try { return await navigator.clipboard.readText() } catch { return '' }
  }
}

async function setClipboardText(text: string): Promise<boolean> {
  try {
    if (await ClipboardSetText(text)) return true
  } catch { /* fall back to the browser clipboard */ }
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}

function dispatchInput(target: EditableTarget) {
  target.dispatchEvent(new Event('input', { bubbles: true }))
}

async function runAction(action: EditAction) {
  const target = activeTarget.value
  if (!target || !target.isConnected) { closeMenu(); return }

  restoreSelection(target)
  const start = selectionStart.value
  const end = selectionEnd.value
  const selected = target.value.slice(start, end)

  if (action === 'copy') {
    await setClipboardText(selected)
  } else if (action === 'cut') {
    if (canCut.value && await setClipboardText(selected)) {
      target.value = target.value.slice(0, start) + target.value.slice(end)
      dispatchInput(target)
      try { target.setSelectionRange(start, start) } catch { /* unsupported input types */ }
    }
  } else if (canPaste.value) {
    const pasted = await getClipboardText()
    const nextValue = target.value.slice(0, start) + pasted + target.value.slice(end)
    target.value = nextValue
    dispatchInput(target)
    const caret = start + pasted.length
    try { target.setSelectionRange(caret, caret) } catch { /* unsupported input types */ }
  }
  closeMenu()
}

function onDocumentMouseDown(event: MouseEvent) {
  if (visible.value && !menuRef.value?.contains(event.target as Node)) closeMenu()
}
function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') closeMenu()
}
function onWindowBlur() { closeMenu() }
function onScroll() { closeMenu() }

onMounted(() => {
  document.addEventListener('contextmenu', showMenu)
  document.addEventListener('mousedown', onDocumentMouseDown)
  document.addEventListener('keydown', onKeydown)
  window.addEventListener('blur', onWindowBlur)
  window.addEventListener('scroll', onScroll, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('contextmenu', showMenu)
  document.removeEventListener('mousedown', onDocumentMouseDown)
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('blur', onWindowBlur)
  window.removeEventListener('scroll', onScroll, true)
})
</script>

<style scoped>
.edit-context-menu {
  position: fixed;
  z-index: 10000;
  min-width: 132px;
  padding: 4px;
  border: 1px solid var(--fluent-border);
  border-radius: 8px;
  background: var(--fluent-card-bg);
  box-shadow: var(--fluent-shadow-lg);
}
.edit-context-menu button {
  display: block;
  width: 100%;
  padding: 7px 12px;
  border: 0;
  border-radius: 5px;
  background: transparent;
  color: var(--fluent-text);
  font: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}
.edit-context-menu button:hover:not(:disabled) {
  background: color-mix(in srgb, var(--accent, #0078d4) 12%, transparent);
}
.edit-context-menu button:disabled {
  color: var(--fluent-text-soft);
  cursor: default;
  opacity: 0.55;
}
</style>
