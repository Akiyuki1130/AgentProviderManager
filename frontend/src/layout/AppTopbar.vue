<template>
  <header class="app-topbar">
    <div class="topbar-left">
      <span class="topbar-title">Agent Provider Manager</span>
      <n-select
        :value="agent"
        :options="agentOptions"
        size="small"
        style="width: 200px"
        @update:value="(v: string) => emit('changeAgent', v)"
      />
      <span v-if="targetPath" class="topbar-path" :title="targetPath">{{ targetPath }}</span>
    </div>
    <div class="topbar-right">
      <n-button size="small" @click="emit('chooseCfg')">{{ t('topbar.chooseCfg', lang) }}</n-button>
      <n-button size="small" @click="emit('openDir')">{{ t('topbar.openDir', lang) }}</n-button>
      <n-button size="small" :disabled="!hasBackup" @click="emit('restore')">
        {{ hasBackup ? t('topbar.restore', lang) : t('topbar.noBackup', lang) }}
      </n-button>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NSelect } from 'naive-ui'
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

const agentOptions = [
  { label: 'ZCode', value: 'zcode' },
  { label: 'OpenCode', value: 'opencode' },
  { label: 'DeepSeek Harness', value: 'deepseek' },
]
</script>

<style scoped>
.app-topbar {
  display: flex; justify-content: space-between; align-items: center;
  padding: 10px 20px; border-bottom: 1px solid var(--fluent-border);
  background: var(--fluent-card-bg); min-height: 52px; box-sizing: border-box;
  min-width: 0; flex-wrap: wrap; row-gap: 6px;
}
.topbar-left { display: flex; align-items: center; gap: 12px; min-width: 0; flex: 1 1 260px; }
.topbar-title { font-size: 14px; font-weight: 700; white-space: nowrap; }
.topbar-path { font-size: 11px; color: var(--fluent-text-soft); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 320px; min-width: 0; flex: 1 1 auto; }
.topbar-right { display: flex; align-items: center; gap: 8px; flex: 0 1 auto; min-width: 0; flex-wrap: wrap; justify-content: flex-end; }
@media (max-width: 900px) {
  .topbar-right { flex-basis: 100%; }
}
</style>
