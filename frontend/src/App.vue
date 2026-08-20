<template>
  <n-config-provider :theme="theme" :theme-overrides="themeOverrides">
    <n-dialog-provider>
      <div class="app-shell" :style="{ '--accent': settingStore.accentColor } as Record<string,string>">
        <div class="app-body">
          <AppSidebar />
          <div class="app-main">
            <AppTopbar
              :target-path="targetPath"
              :has-backup="hasBackup"
              :is-dark="isDark"
              :lang="settingStore.lang"
              :agent="settingStore.agent"
              @toggleLang="toggleLang"
              @toggleTheme="toggleTheme"
              @chooseCfg="onChooseCfg"
              @openDir="onOpenDir"
              @restore="onRestore"
              @changeAgent="onChangeAgent"
            />
            <main class="app-content">
              <router-view v-slot="{ Component }">
                <Transition name="fade-slide" mode="out-in">
                  <component :is="Component" />
                </Transition>
              </router-view>
            </main>
          </div>
        </div>
      </div>
      <ToastContainer ref="toastRef" />
    </n-dialog-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue'
import { NConfigProvider, NDialogProvider } from 'naive-ui'
import type { GlobalTheme } from 'naive-ui'
import { useSettingStore } from './stores/setting'
import type { AgentID } from './stores/setting'
import { buildThemeOverrides, isDark as isDarkFn, darkTheme } from './styles/theme'
import AppSidebar from './layout/AppSidebar.vue'
import AppTopbar from './layout/AppTopbar.vue'
import ToastContainer from './components/ToastContainer.vue'
import * as api from './api'

const settingStore = useSettingStore()
const toastRef = ref<InstanceType<typeof ToastContainer>>()
const targetPath = ref('')
const hasBackup = ref(false)

function toast(type: string, title: string, msg?: string) {
  const w = window as unknown as Record<string, unknown>
  const fn = w['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}

const dark = computed(() => isDarkFn(settingStore.theme, settingStore.systemDark))
const isDark = computed(() => dark.value)
const theme = computed<GlobalTheme | null>(() => (dark.value ? darkTheme : null))
const themeOverrides = computed(() => buildThemeOverrides(settingStore.accentColor, dark.value))

function toggleLang() {
  const next = settingStore.lang === 'zh' ? 'en' : 'zh'
  settingStore.setLang(next)
  api.SetLanguage(next)
}
function toggleTheme() {
  const next = dark.value ? 'light' : 'dark'
  settingStore.setTheme(next as 'light' | 'dark')
  api.SetTheme(next)
  document.documentElement.setAttribute('data-theme', next)
}

async function onChangeAgent(agent: string) {
  const res = await api.SetCurrentAgent(agent) as Record<string, unknown>
  if (res['success']) {
    settingStore.setAgent(agent as AgentID)
    targetPath.value = (res['path'] as string) || targetPath.value
    await refreshBackup()
    window.dispatchEvent(new CustomEvent('agent-changed', { detail: agent }))
    toast('success', '已切换 Agent', `${agent} → ${targetPath.value}`)
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
  const backup = info['backup_path'] as string
  if (!backup) { toast('info', '提示', '没有可用的备份文件'); return }
  const res = await api.RestoreLastBackup(backup, targetPath.value) as Record<string, unknown>
  if (res['success']) { toast('success', '恢复成功', `已从备份恢复：${backup}`); await refreshBackup() }
  else toast('error', '恢复失败', res['error'] as string)
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
    const lang = await api.GetLanguage() as string | null
    if (lang === 'zh' || lang === 'en') settingStore.setLang(lang as 'zh' | 'en')
  } catch { /* ignore */ }
  try {
    const t = await api.GetTheme() as string | null
    if (t === 'light' || t === 'dark') {
      document.documentElement.setAttribute('data-theme', t)
      settingStore.setTheme(t as 'light' | 'dark')
    }
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
</style>
