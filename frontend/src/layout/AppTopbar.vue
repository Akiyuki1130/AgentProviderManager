<template>
  <header class="app-topbar" style="--wails-draggable: drag">
    <div class="topbar-left topbar-interactive">
      <n-select
        :value="agent"
        :options="agentOptions"
        size="small"
        style="width: 200px"
        @update:value="(v: string) => emit('changeAgent', v)"
      />
      <!-- B6：目标路径状态胶囊。accent 10% 底 + 胶囊圆角；min-width 固定占位，切换 Agent 路径长短变化不抖动；title 保留全路径、文字 selectable 可复制 -->
      <span v-if="targetPath" class="topbar-status" :title="targetPath">
        <n-icon class="topbar-status-icon" :component="ServerOutline" :size="12" />
        <span class="topbar-path selectable">{{ targetPath }}</span>
      </span>
    </div>
    <div class="topbar-drag-space" aria-hidden="true" />
    <div class="topbar-right topbar-interactive">
      <n-button size="small" :aria-label="t('topbar.chooseCfg', lang)" @click="emit('chooseCfg')">{{ t('topbar.chooseCfg', lang) }}</n-button>
      <n-button size="small" :aria-label="t('topbar.openDir', lang)" @click="emit('openDir')">
        <template #icon><n-icon :component="FolderOpenOutline" /></template>
        {{ t('topbar.openDir', lang) }}
      </n-button>
      <n-button size="small" :aria-label="hasBackup ? t('topbar.restore', lang) : t('topbar.noBackup', lang)" :disabled="!hasBackup" @click="emit('restore')">
        <template #icon><n-icon :component="DownloadOutline" /></template>
        {{ hasBackup ? t('topbar.restore', lang) : t('topbar.noBackup', lang) }}
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
import { computed, onMounted, ref } from 'vue'
import { NButton, NIcon, NSelect } from 'naive-ui'
import { CloseOutline, DownloadOutline, FolderOpenOutline, RemoveOutline, ResizeOutline, ServerOutline, SquareOutline } from '@vicons/ionicons5'
import { WindowIsMaximised, WindowMinimise, WindowToggleMaximise, Quit } from '../../wailsjs/runtime/runtime'
import { useSettingStore } from '../stores/setting'
import { t } from '../i18n'

const settingStore = useSettingStore()
defineProps<{
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
const isMaximised = ref(false)

const agentOptions = [
  { label: 'ZCode', value: 'zcode' },
  { label: 'DeepSeek Harness', value: 'deepseek' },
]

async function syncWindowState() {
  try {
    isMaximised.value = await WindowIsMaximised()
  } catch {
    isMaximised.value = false
  }
}

function minimiseWindow() {
  WindowMinimise()
}

function toggleMaximise() {
  WindowToggleMaximise()
  isMaximised.value = !isMaximised.value
}

function closeWindow() {
  Quit()
}

onMounted(() => {
  void syncWindowState()
})
</script>

<style scoped>
.app-topbar {
  display: flex; align-items: center;
  padding: 10px 20px; border-bottom: 1px solid var(--fluent-border);
  background: var(--fluent-card-bg); min-height: 52px; box-sizing: border-box;
  min-width: 0; flex-wrap: wrap; row-gap: 6px;
}
.topbar-left { display: flex; align-items: center; gap: 12px; min-width: 0; flex: 0 1 auto; }
.topbar-interactive { --wails-draggable: no-drag; }
/* B6 状态胶囊：accent 10% 底、999 圆角（badge 例外档）；min-width 让路径长短变化时胶囊宽度稳定 */
.topbar-status {
  display: inline-flex; align-items: center; gap: 6px; min-width: 30ch; max-width: 360px;
  padding: 3px 12px 3px 10px; border-radius: 999px;
  background: color-mix(in srgb, var(--accent, #0078d4) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent, #0078d4) 22%, transparent);
  min-height: 22px; box-sizing: border-box; flex: 0 1 auto;
}
.topbar-status-icon { flex-shrink: 0; color: var(--accent-text-strong, var(--accent, #0078d4)); }
.topbar-path { font-size: 11px; color: var(--fluent-text-soft); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; min-width: 0; }
.topbar-drag-space { flex: 1 1 24px; min-width: 16px; align-self: stretch; }
.topbar-right { display: flex; align-items: center; gap: 8px; flex: 0 1 auto; min-width: 0; flex-wrap: wrap; justify-content: flex-end; margin-right: 10px; }
.window-controls { display: flex; align-items: center; gap: 2px; margin-left: 8px; }
.window-control { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 28px; padding: 0; border: 0; border-radius: 4px; background: transparent; color: var(--fluent-text); cursor: pointer; }
.window-control:first-child, .window-control-close { width: 34px; height: 30px; }
.window-control:hover { background: color-mix(in srgb, var(--accent, #0078d4) 12%, transparent); }
.window-control-close:hover { background: var(--err); color: #fff; }
@media (max-width: 900px) {
  .topbar-right { flex-basis: 100%; }
  .topbar-drag-space { display: none; }
}
</style>
