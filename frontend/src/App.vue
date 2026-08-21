<template>
  <n-config-provider :theme="theme" :theme-overrides="themeOverrides" :locale="naiveLocale" :date-locale="naiveDateLocale">
    <n-dialog-provider>
      <div class="app-shell" :style="{ '--accent': settingStore.accentColor } as Record<string,string>">
        <div class="app-body">
          <AppSidebar />
          <div class="app-main">
            <AppTopbar
              :target-path="targetPath"
              :has-backup="hasBackup"
              :agent="settingStore.agent"
              @chooseCfg="onChooseCfg"
              @openDir="onOpenDir"
              @restore="onRestore"
              @changeAgent="onChangeAgent"
            />
            <main class="app-content">
              <router-view v-slot="{ Component }">
                <Transition name="fade-slide" mode="out-in">
                  <component :is="Component" :key="settingStore.agent" />
                </Transition>
              </router-view>
            </main>
          </div>
        </div>
      </div>
      <ToastContainer ref="toastRef" />
      <EditContextMenu />
      <n-modal v-model:show="leaveShow" preset="card" title="未保存的更改" style="width: 440px" :mask-closable="false">
        <div style="font-size: 13px; line-height: 1.7">
          当前提供商有未保存的更改，是否保存？
        </div>
        <template #footer>
          <div style="display: flex; justify-content: flex-end; gap: 8px">
            <n-button :disabled="leaveSaving" @click="cancelLeave">取消</n-button>
            <n-button type="error" ghost :disabled="leaveSaving" @click="confirmLeave(false)">不保存</n-button>
            <n-button type="primary" :loading="leaveSaving" @click="confirmLeave(true)">保存</n-button>
          </div>
        </template>
      </n-modal>
      <n-modal v-model:show="showRestoreModal" preset="card" title="恢复备份" style="width: 520px" :mask-closable="false">
        <div style="font-size: 13px; line-height: 1.6">
          为当前 Agent（{{ agentLabel(settingStore.agent) }}）选择要恢复的备份，恢复将覆盖当前配置文件。
        </div>
        <div class="backup-list">
          <div v-for="b in backupList" :key="b" class="backup-item">
            <div class="backup-meta">
              <div class="backup-name">{{ formatBackupLabel(b) }}</div>
              <div class="backup-path">{{ b }}</div>
            </div>
            <n-button size="small" type="primary" :loading="restoring" @click="doRestore(b)">恢复</n-button>
          </div>
        </div>
        <div v-if="backupList.length === 0" style="font-size: 12px; color: var(--fluent-text-soft)">没有可用的备份文件</div>
        <template #footer>
          <div style="display: flex; justify-content: flex-end">
            <n-button @click="showRestoreModal = false">关闭</n-button>
          </div>
        </template>
      </n-modal>
    </n-dialog-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue'
import { NConfigProvider, NDialogProvider, NModal, NButton, zhCN, dateZhCN, enUS, dateEnUS, jaJP, dateJaJP } from 'naive-ui'
import type { GlobalTheme } from 'naive-ui'
import { useSettingStore } from './stores/setting'
import type { AgentID } from './stores/setting'
import { requestLeave, cancelLeave, confirmLeave, leaveShow, leaveSaving } from './stores/leaveGuard'
import { buildThemeOverrides, isDark as isDarkFn, darkTheme, resolveTheme } from './styles/theme'
import AppSidebar from './layout/AppSidebar.vue'
import AppTopbar from './layout/AppTopbar.vue'
import ToastContainer from './components/ToastContainer.vue'
import EditContextMenu from './components/EditContextMenu.vue'
import * as api from './api'

const settingStore = useSettingStore()
const toastRef = ref<InstanceType<typeof ToastContainer>>()
const targetPath = ref('')
const hasBackup = ref(false)
const showRestoreModal = ref(false)
const backupList = ref<string[]>([])
const restoring = ref(false)

function toast(type: string, title: string, msg?: string) {
  const w = window as unknown as Record<string, unknown>
  const fn = w['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

function agentLabel(agent: string): string {
  if (agent === 'zcode') return 'ZCode'
  if (agent === 'opencode') return 'OpenCode'
  if (agent === 'deepseek') return 'DeepSeek Harness'
  return agent
}

const dark = computed(() => isDarkFn(settingStore.theme, settingStore.systemDark))
const theme = computed<GlobalTheme | null>(() => (dark.value ? darkTheme : null))
const themeOverrides = computed(() => buildThemeOverrides(settingStore.accentColor, dark.value))

// Naive UI 内置组件文案（弹窗确认/取消等）跟随界面语言
const naiveLocale = computed(() => {
  const l = settingStore.resolvedLang
  if (l === 'zh') return zhCN
  if (l === 'ja') return jaJP
  return enUS
})
const naiveDateLocale = computed(() => {
  const l = settingStore.resolvedLang
  if (l === 'zh') return dateZhCN
  if (l === 'ja') return dateJaJP
  return dateEnUS
})

function onChangeAgent(agent: string) {
  requestLeave(() => { void doChangeAgent(agent) })
}

async function doChangeAgent(agent: string) {
  const res = await api.SetCurrentAgent(agent) as Record<string, unknown>
  if (res['success']) {
    settingStore.setAgent(agent as AgentID)
    targetPath.value = (res['path'] as string) || targetPath.value
    await refreshBackup()
    window.dispatchEvent(new CustomEvent('agent-changed', { detail: agent }))
    toast('success', '已切换 Agent', `${agentLabel(agent)} → ${targetPath.value}`)
  } else {
    toast('error', '切换失败', res['error'] as string)
  }
}

async function onChooseCfg() {
  const res = await api.ChooseConfigFile() as Record<string, unknown>
  if (res['success']) { targetPath.value = res['path'] as string; await refreshBackup() }
  else if (!res['cancelled']) { toast('error', '选择失败', res['error'] as string) }
}
async function onOpenDir() { await api.OpenConfigDir() }
async function onRestore() {
  const info = await api.GetBackupInfo() as Record<string, unknown>
  const list = (info['backups'] as string[]) || []
  if (list.length === 0) { toast('info', '提示', '没有可用的备份文件'); return }
  backupList.value = list
  showRestoreModal.value = true
}
async function doRestore(bak: string) {
  restoring.value = true
  try {
    const res = await api.RestoreLastBackup(bak, targetPath.value) as Record<string, unknown>
    if (res['success']) {
      showRestoreModal.value = false
      toast('success', '恢复成功', `已从备份恢复：${formatBackupLabel(bak)}`)
      await refreshBackup()
    } else toast('error', '恢复失败', res['error'] as string)
  } finally { restoring.value = false }
}
function formatBackupLabel(path: string): string {
  const m = path.match(/\.bak_(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})/)
  if (m) return `${m[1]}-${m[2]}-${m[3]} ${m[4]}:${m[5]}:${m[6]}`
  const base = path.split(/[\\/]/).pop() || path
  return base
}
async function refreshBackup() {
  try {
    const info = await api.GetBackupInfo() as Record<string, unknown>
    hasBackup.value = !!info['has_backup']
    if (info['target']) targetPath.value = info['target'] as string
  } catch { /* ignore */ }
}

onMounted(async () => {
  settingStore.initTheme()
  document.documentElement.setAttribute('data-theme', dark.value ? 'dark' : 'light')
  try {
    const opts = await api.GetOptions() as Record<string, unknown>
    const themeVal = opts['theme'] as string | undefined
    if (themeVal === 'light' || themeVal === 'dark' || themeVal === 'system') {
      settingStore.setTheme(themeVal)
      document.documentElement.setAttribute('data-theme', resolveTheme(themeVal, settingStore.systemDark))
    }
    const langVal = opts['language'] as string | undefined
    if (langVal === 'zh' || langVal === 'en' || langVal === 'ja' || langVal === 'system') {
      settingStore.setLang(langVal as 'zh' | 'en' | 'ja' | 'system')
    }
    const accentVal = opts['accent'] as string | undefined
    if (typeof accentVal === 'string' && accentVal) settingStore.setAccentColor(accentVal)
    if (typeof opts['http_enabled'] === 'boolean') settingStore.setHttpEnabled(opts['http_enabled'] as boolean)
    if (typeof opts['auto_fill_limits'] === 'boolean') settingStore.setAutoFillLimits(opts['auto_fill_limits'] as boolean)
  } catch { /* ignore */ }
  try {
    const ag = await api.GetCurrentAgent() as Record<string, unknown>
    const cur = ag['agent'] as string
    if (cur === 'zcode' || cur === 'opencode' || cur === 'deepseek') {
      settingStore.setAgent(cur as AgentID)
    }
  } catch { /* ignore */ }
  try {
    const info = await api.GetTargetConfig() as Record<string, unknown>
    targetPath.value = (info['path'] as string) || ''
    if (info['agent'] && typeof info['agent'] === 'string') {
      const ag = info['agent'] as string
      if (ag === 'zcode' || ag === 'opencode' || ag === 'deepseek') settingStore.setAgent(ag as AgentID)
    }
    await refreshBackup()
  } catch { /* ignore */ }
})

watch(dark, (d) => { document.documentElement.setAttribute('data-theme', d ? 'dark' : 'light') })
watch(() => settingStore.accentColor, (c) => { document.documentElement.style.setProperty('--accent', c) }, { immediate: true })
</script>

<style scoped>
.app-shell { display: flex; flex-direction: column; height: 100vh; width: 100vw; background: var(--fluent-bg); }
.app-body { flex: 1; display: flex; min-height: 0; }
.app-main { flex: 1; display: flex; flex-direction: column; min-width: 0; }
.app-content { flex: 1; overflow: auto; padding: 20px; box-sizing: border-box; }
.backup-list { display: flex; flex-direction: column; gap: 8px; margin-top: 12px; }
.backup-item { display: flex; align-items: center; gap: 12px; border: 1px solid var(--fluent-border); border-radius: 8px; padding: 10px 12px; }
.backup-meta { flex: 1; min-width: 0; }
.backup-name { font-size: 13px; font-weight: 600; }
.backup-path { font-size: 11px; color: var(--fluent-text-soft); word-break: break-all; margin-top: 2px; }
</style>
