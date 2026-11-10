<template>
  <header ref="topbarEl" class="app-topbar" @dblclick="onTopbarDblclick">
    <div class="topbar-left">
      <!-- 拖拽修复：整行 header 可拖，no-drag 只收窄到真正交互的控件（select 包裹层与右侧按钮组） -->
      <div class="topbar-select" @dblclick.stop>
        <n-select
          :value="agent"
          :options="agentOptions"
          size="small"
          style="width: 200px"
          @update:value="(v: string) => emit('changeAgent', v)"
        />
      </div>
      <!-- B6：目标路径状态胶囊（属可拖拽区）。中段按层级自适应省略：盘符 + … + 文件名，
           宽度不够时从最左一级目录开始逐级折叠（fitPath 计算），完整路径见 title 悬浮提示 -->
      <span v-if="pathParts" class="topbar-status" :title="targetPath">
        <n-icon class="topbar-status-icon" :component="ServerOutline" :size="12" />
        <span ref="pathEl" class="topbar-path">
          <span v-if="pathParts.drive" class="path-seg path-drive">{{ pathParts.drive }}</span>
          <span v-show="hiddenMid > 0" class="path-seg path-ellip">…\</span>
          <span
            v-for="(s, i) in pathParts.mids"
            :key="i"
            class="path-seg path-mid"
            :style="{ display: i < hiddenMid ? 'none' : undefined }"
          >{{ s }}\</span>
          <span class="path-seg path-file">{{ pathParts.file }}</span>
        </span>
      </span>
    </div>
    <div class="topbar-drag-space" aria-hidden="true" />
    <div class="topbar-right" @dblclick.stop>
      <!-- compact（顶栏 <1080px）时不渲染文字插槽：naive 原生图标按钮自动收成居中方形，
           比 CSS 隐藏 label 更正确（后者会留下文字档的宽盒与偏左图标） -->
      <n-button size="small" :aria-label="t('topbar.chooseCfg', lang)" :title="t('topbar.chooseCfg', lang)" @click="emit('chooseCfg')">
        <template #icon><n-icon :component="DocumentTextOutline" /></template>
        <template v-if="!compact">{{ t('topbar.chooseCfg', lang) }}</template>
      </n-button>
      <n-button size="small" :aria-label="t('topbar.openDir', lang)" :title="t('topbar.openDir', lang)" @click="emit('openDir')">
        <template #icon><n-icon :component="FolderOpenOutline" /></template>
        <template v-if="!compact">{{ t('topbar.openDir', lang) }}</template>
      </n-button>
      <n-button size="small" :aria-label="hasBackup ? t('topbar.restore', lang) : t('topbar.noBackup', lang)" :title="hasBackup ? t('topbar.restore', lang) : t('topbar.noBackup', lang)" :disabled="!hasBackup" @click="emit('restore')">
        <template #icon><n-icon :component="DownloadOutline" /></template>
        <template v-if="!compact">{{ hasBackup ? t('topbar.restore', lang) : t('topbar.noBackup', lang) }}</template>
      </n-button>
      <div class="window-controls" aria-label="窗口控制">
        <button class="window-control" type="button" title="最小化" aria-label="最小化" @click="minimiseWindow">
          <n-icon :component="RemoveOutline" :size="16" />
        </button>
        <button class="window-control" type="button" :title="isMaximised ? '还原' : '最大化'" :aria-label="isMaximised ? '还原' : '最大化'" @click="toggleMaximise">
          <n-icon :component="isMaximised ? ResizeOutline : SquareOutline" :size="14" />
        </button>
        <button class="window-control window-control-close" type="button" title="关闭" aria-label="关闭" @click="closeWindow">
          <n-icon :component="CloseOutline" :size="15" />
        </button>
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NSelect } from 'naive-ui'
import { CloseOutline, DocumentTextOutline, DownloadOutline, FolderOpenOutline, RemoveOutline, ResizeOutline, ServerOutline, SquareOutline } from '@vicons/ionicons5'
import { WindowIsMaximised, WindowMinimise, WindowToggleMaximise, Quit } from '../../wailsjs/runtime/runtime'
import { useSettingStore } from '../stores/setting'
import { leaveDirty, requestLeave } from '../stores/leaveGuard'
import { t } from '../i18n'
import * as api from '../api'

const settingStore = useSettingStore()
const props = defineProps<{
  targetPath: string
  hasBackup: boolean
  agent: string
}>()
const emit = defineEmits<{
  (e: 'chooseCfg'): void
  (e: 'openDir'): void
  (e: 'restore'): void
  (e: 'changeAgent', v: string): void
}>()
const lang = computed(() => settingStore.resolvedLang)
// Wails 不派发窗口状态事件：isMaximised 不做乐观取反，统一经 refreshMaximised 回查真值
const isMaximised = ref(false)
let maximisedTimer: ReturnType<typeof setTimeout> | undefined

// 顶栏自适应：窄于 1080px 时操作按钮收成纯图标（ResizeObserver 盯顶栏自身宽度，
// 顶栏宽度由布局决定、不随按钮收缩回摆，无反馈循环）
const COMPACT_BREAKPOINT = 1080
const topbarEl = ref<HTMLElement | null>(null)
const compact = ref(false)
let topbarRO: ResizeObserver | null = null

// Agent 名称是产品名（不随界面语言变化），但新增 Agent（如 omp）必须能从后端
// core.AllAgents() 动态带出来，不能再在前端写死两份清单——写死正是 omp 一开始选不到的原因。
const AGENT_LABELS: Record<string, string> = {
  zcode: 'ZCode',
  deepseek: 'DeepSeek Harness',
  omp: 'PI Agent (oh-my-pi)',
}
const AGENT_FALLBACK = ['zcode', 'deepseek', 'omp']
const agentOptions = ref<{ label: string; value: string }[]>(
  AGENT_FALLBACK.map((id) => ({ label: AGENT_LABELS[id], value: id })),
)
function agentLabelOf(id: string): string {
  return AGENT_LABELS[id] || id
}
async function loadAgents() {
  try {
    const res = await api.ListAgents()
    const raw = res['agents']
    const list = Array.isArray(raw) ? (raw as Array<Record<string, unknown>>) : []
    const opts = list
      .map((a) => {
        const id = typeof a['id'] === 'string' ? a['id'] : ''
        const label = typeof a['label'] === 'string' && a['label'] ? a['label'] : agentLabelOf(id)
        return { label, value: id }
      })
      .filter((o) => o.value)
    // 后端还没实现 omp 时也不清空下拉：内置三项兜底，避免选择器变空
    if (opts.length > 0) agentOptions.value = opts
  } catch {
    /* 后端未就绪：保留内置三项 */
  }
}

async function refreshMaximised() {
  try {
    isMaximised.value = await WindowIsMaximised()
  } catch {
    isMaximised.value = false
  }
}

function scheduleMaximisedRefresh(delay: number) {
  if (maximisedTimer) clearTimeout(maximisedTimer)
  maximisedTimer = setTimeout(() => {
    maximisedTimer = undefined
    void refreshMaximised()
  }, delay)
}

function onWindowResize() {
  scheduleMaximisedRefresh(120)
  void fitPath()
}

function minimiseWindow() {
  WindowMinimise()
}

function toggleMaximise() {
  WindowToggleMaximise()
  scheduleMaximisedRefresh(80)
}

function onTopbarDblclick() {
  toggleMaximise()
}

function closeWindow() {
  // 退出是「不保存就丢」的操作：先过未保存守卫（自动保存开启则静默落盘后退出，
  // 关闭或写不进去时弹窗询问）。
  if (leaveDirty()) {
    void requestLeave(() => { Quit() })
    return
  }
  Quit()
}

// ---- 路径胶囊：按层级自适应中段省略 ----
const pathEl = ref<HTMLElement | null>(null)
const hiddenMid = ref(0)
let fitToken = 0

const pathParts = computed(() => {
  const raw = (props.targetPath || '').trim()
  if (!raw) return null
  const parts = raw.split(/[\\/]+/).filter(Boolean)
  if (parts.length === 0) return null
  if (parts.length === 1) return { drive: '', mids: [] as string[], file: parts[0] }
  // 首段视为盘符/根（"C:" → "C:\"），末段视为文件名，中间为逐级目录
  return { drive: parts[0] + '\\', mids: parts.slice(1, -1), file: parts[parts.length - 1] }
})

// 逐级隐藏最左侧目录直到内容不再溢出（容器有 max-width 上限才会走到这里）；
// token 防止 resize/路径切换并发触发时旧循环回写
async function fitPath() {
  const token = ++fitToken
  const el = pathEl.value
  hiddenMid.value = 0
  if (!el || !pathParts.value) return
  await nextTick()
  if (token !== fitToken) return
  const total = pathParts.value.mids.length
  for (let h = 0; h < total && el.scrollWidth > el.clientWidth; h++) {
    hiddenMid.value = h + 1
    await nextTick()
    if (token !== fitToken) return
  }
}

watch(() => props.targetPath, () => void fitPath())

onMounted(() => {
  window.addEventListener('resize', onWindowResize)
  void loadAgents()
  void refreshMaximised()
  void nextTick(fitPath)
  const el = topbarEl.value
  if (el) {
    compact.value = el.clientWidth < COMPACT_BREAKPOINT
    topbarRO = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w !== undefined) compact.value = w < COMPACT_BREAKPOINT
    })
    topbarRO.observe(el)
  }
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onWindowResize)
  if (maximisedTimer) clearTimeout(maximisedTimer)
  topbarRO?.disconnect()
  topbarRO = null
})
</script>

<style scoped>
.app-topbar {
  display: flex; align-items: center;
  padding: 10px 20px; border-bottom: 1px solid var(--fluent-border);
  background: var(--fluent-card-bg); min-height: 52px; box-sizing: border-box;
  min-width: 0; flex-wrap: nowrap;
  user-select: none;
  /* 拖拽修复：drag 提升到整条顶栏，仅交互控件精确 no-drag，状态胶囊随顶栏可拖 */
  --wails-draggable: drag;
  /* 自适应按钮：以顶栏自身宽度为容器查询基准，挤了先收文字标签 */
  container-type: inline-size;
}
.topbar-left { display: flex; align-items: center; gap: 12px; min-width: 0; flex: 0 1 auto; }
.topbar-select { --wails-draggable: no-drag; flex-shrink: 0; }
/* B6 状态胶囊：accent 10% 底、999 圆角（badge 例外档）；14ch 下限保证"盘符+…+文件名"始终可读 */
.topbar-status {
  display: inline-flex; align-items: center; gap: 6px; min-width: 14ch; max-width: 360px;
  padding: 3px 12px 3px 10px; border-radius: 999px;
  background: color-mix(in srgb, var(--accent, #0078d4) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent, #0078d4) 22%, transparent);
  min-height: 22px; box-sizing: border-box; flex: 0 1 auto;
}
.topbar-status-icon { flex-shrink: 0; color: var(--accent-text-strong, var(--accent, #0078d4)); }
.topbar-path { display: flex; align-items: center; font-size: 11px; color: var(--fluent-text-soft); min-width: 0; flex: 1 1 auto; overflow: hidden; }
.path-seg { white-space: nowrap; }
.path-file { overflow: hidden; text-overflow: ellipsis; }
.path-ellip { color: var(--fluent-text-soft); }
.topbar-drag-space { flex: 1 1 24px; min-width: 16px; align-self: stretch; }
.topbar-right { display: flex; align-items: center; gap: 8px; flex: 0 0 auto; min-width: 0; justify-content: flex-end; margin-right: 10px; --wails-draggable: no-drag; }
.window-controls { display: flex; align-items: center; gap: 2px; margin-left: 8px; }
.window-control { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 28px; padding: 0; border: 0; border-radius: 4px; background: transparent; color: var(--fluent-text); cursor: pointer; }
.window-control:first-child, .window-control-close { width: 34px; height: 30px; }
.window-control:hover { background: color-mix(in srgb, var(--accent, #0078d4) 12%, transparent); }
.window-control-close:hover { background: var(--err); color: #fff; }

/* 极窄时路径胶囊下限再放宽（按钮收图标由 JS compact 承担，见脚本） */
@container (max-width: 760px) {
  .topbar-status { min-width: 10ch; }
}
</style>
