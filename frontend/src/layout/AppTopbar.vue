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
      <n-button size="small" @click="emit('toggleLang')">{{ lang === 'zh' ? 'EN' : '中文' }}</n-button>
      <n-button size="small" @click="emit('toggleTheme')">{{ isDark ? '☀️ 浅色' : '🌙 深色' }}</n-button>
      <n-button size="small" @click="emit('chooseCfg')">选择配置文件...</n-button>
      <n-button size="small" @click="emit('openDir')">打开配置目录</n-button>
      <n-button size="small" :disabled="!hasBackup" @click="emit('restore')">
        {{ hasBackup ? '恢复最近备份' : '无备份可恢复' }}
      </n-button>
    </div>
  </header>
</template>

<script setup lang="ts">
import { NButton, NSelect } from 'naive-ui'
defineProps<{
  targetPath: string
  hasBackup: boolean
  isDark: boolean
  lang: string
  agent: string
}>()
const emit = defineEmits<{
  (e: 'toggleLang'): void
  (e: 'toggleTheme'): void
  (e: 'chooseCfg'): void
  (e: 'openDir'): void
  (e: 'restore'): void
  (e: 'changeAgent', v: string): void
}>()

const agentOptions = [
  { label: 'ZCode', value: 'zcode' },
  { label: 'OpenCode', value: 'opencode' },
  { label: 'DeepSeek Harnessed', value: 'deepseek' },
]
</script>

<style scoped>
.app-topbar {
  display: flex; justify-content: space-between; align-items: center;
  padding: 10px 20px; border-bottom: 1px solid var(--fluent-border);
  background: var(--fluent-card-bg); min-height: 52px; box-sizing: border-box;
}
.topbar-left { display: flex; align-items: center; gap: 12px; min-width: 0; }
.topbar-title { font-size: 14px; font-weight: 700; white-space: nowrap; }
.topbar-path { font-size: 11px; color: var(--fluent-text-soft); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 320px; }
.topbar-right { display: flex; align-items: center; gap: 8px; flex-shrink: 0; flex-wrap: wrap; }
</style>
