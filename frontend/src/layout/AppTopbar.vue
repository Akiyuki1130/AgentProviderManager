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
      <span v-if="targetPath" class="topbar-path" :title="targetPath">{{ targetPath }}</span>
    </div>
    <div class="topbar-drag-space" aria-hidden="true" />
    <div class="topbar-right topbar-interactive">
      <n-button size="small" @click="emit('chooseCfg')">{{ t('topbar.chooseCfg', lang) }}</n-button>
      <n-button size="small" @click="emit('openDir')">{{ t('topbar.openDir', lang) }}</n-button>
      <n-button size="small" :disabled="!hasBackup" @click="emit('restore')">
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
import { CloseOutline, RemoveOutline, ResizeOutline, SquareOutline } from '@vicons/ionicons5'
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
  { label: 'OpenCode', value: 'opencode' },
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
.topbar-path { font-size: 11px; color: var(--fluent-text-soft); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 320px; min-width: 0; flex: 1 1 auto; }
.topbar-drag-space { flex: 1 1 24px; min-width: 16px; align-self: stretch; }
.topbar-right { display: flex; align-items: center; gap: 8px; flex: 0 1 auto; min-width: 0; flex-wrap: wrap; justify-content: flex-end; margin-right: 10px; }
.window-controls { display: flex; align-items: center; gap: 2px; margin-left: 8px; }
.window-control { display: inline-flex; align-items: center; justify-content: center; width: 32px; height: 28px; padding: 0; border: 0; border-radius: 4px; background: transparent; color: var(--fluent-text); cursor: pointer; }
.window-control:first-child, .window-control-close { width: 34px; height: 30px; }
.window-control:hover { background: color-mix(in srgb, var(--accent, #0078d4) 12%, transparent); }
.window-control-close:hover { background: #c42b1c; color: #fff; }
@media (max-width: 900px) {
  .topbar-right { flex-basis: 100%; }
  .topbar-drag-space { display: none; }
}
</style>
