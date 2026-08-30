<template>
  <div class="options-page">
    <div class="fluent-card card-pad">
      <div class="card-title">{{ t('options.title', lang) }}</div>
      <div class="opt-list">
        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">
              {{ t('options.lang', lang) }}
              <span class="opt-tag">{{ t('options.langTag', lang) }}</span>
            </div>
            <div class="opt-desc">{{ t('options.langDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-radio-group :value="settingStore.lang" size="small" @update:value="onLangChange">
              <n-radio-button value="system">
                {{ t('options.langSystem', lang) }}<template v-if="settingStore.lang === 'system'">{{ t('options.langSystemHint', lang).replace('{lang}', currentLangLabel) }}</template>
              </n-radio-button>
              <n-radio-button value="zh">{{ t('options.zh', lang) }}</n-radio-button>
              <n-radio-button value="en">{{ t('options.en', lang) }}</n-radio-button>
              <n-radio-button value="ja">{{ t('options.ja', lang) }}</n-radio-button>
            </n-radio-group>
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.theme', lang) }}</div>
            <div class="opt-desc">{{ t('options.themeDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-radio-group :value="settingStore.theme" size="small" @update:value="onThemeChange">
              <n-radio-button value="light">{{ t('options.themeLight', lang) }}</n-radio-button>
              <n-radio-button value="dark">{{ t('options.themeDark', lang) }}</n-radio-button>
              <n-radio-button value="system">{{ t('options.langSystem', lang) }}</n-radio-button>
            </n-radio-group>
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.accent', lang) }}</div>
            <div class="opt-desc">{{ t('options.accentDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-color-picker
              :value="settingStore.accentColor"
              :show-alpha="false"
              :modes="['hex']"
              :swatches="accentSwatches"
              style="width: 150px"
              @update:value="onAccentChange"
            />
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.http', lang) }}</div>
            <div class="opt-desc">{{ t('options.httpDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-switch :value="settingStore.httpEnabled" @update:value="onHttpChange" />
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.privateHosts', lang) }}</div>
            <div class="opt-desc">{{ t('options.privateHostsDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-switch :value="settingStore.privateHostsAllowed" @update:value="onPrivateHostsChange" />
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.autofill', lang) }}</div>
            <div class="opt-desc">{{ t('options.autofillDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-switch :value="settingStore.autoFillLimits" @update:value="onAutoFillChange" />
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.intensity', lang) }}</div>
            <div class="opt-desc">{{ t('options.intensityDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-switch :value="settingStore.reasoningAllIntensities" @update:value="onIntensityChange" />
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.autoupdate', lang) }}</div>
            <div class="opt-desc">{{ t('options.autoupdateDesc', lang) }}</div>
            <div v-if="updateStore.ready" class="update-ready">{{ t('options.updateDownloaded', lang) }}</div>
            <div v-else-if="updateStore.available" class="update-ready">{{ t('options.updateAvailable', lang).replace('{version}', updateStore.latestVersion) }}</div>
            <div v-if="updateStore.error" class="update-error">{{ updateStore.error }}</div>
          </div>
          <div class="opt-ctrl opt-update-controls">
            <n-switch :value="settingStore.autoUpdate" @update:value="onAutoUpdateChange" />
            <n-button size="small" :loading="updateStore.checking" @click="checkUpdate">
              {{ updateStore.checking ? t('options.updateChecking', lang) : t('options.checkUpdate', lang) }}
            </n-button>
            <n-button v-if="updateStore.available && !updateStore.ready" size="small" type="primary" :loading="updateStore.downloading" @click="downloadUpdate">
              {{ updateStore.downloading ? `${updateStore.downloadProgress}%` : t('options.updateAvailable', lang).replace('{version}', updateStore.latestVersion) }}
            </n-button>
            <n-button v-if="updateStore.ready" size="small" type="primary" @click="installUpdate">{{ t('options.updateRestart', lang) }}</n-button>
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.author', lang) }}</div>
            <div class="opt-desc">{{ t('options.authorDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl opt-links">
            <n-button size="small" @click="openExternal('https://github.com/Akiyuki1130')">{{ t('options.github', lang) }}</n-button>
            <n-button size="small" @click="openExternal('https://space.bilibili.com/30607464')">{{ t('options.bilibili', lang) }}</n-button>
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.project', lang) }}</div>
            <div class="opt-desc">{{ t('options.projectDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-button size="small" @click="openExternal('https://github.com/Akiyuki1130/AgentProviderManager')">{{ t('options.open', lang) }}</n-button>
          </div>
        </div>

        <div class="opt-row">
          <div class="opt-info">
            <div class="opt-name">{{ t('options.bug', lang) }}</div>
            <div class="opt-desc">{{ t('options.bugDesc', lang) }}</div>
          </div>
          <div class="opt-ctrl">
            <n-button size="small" @click="openExternal('https://github.com/Akiyuki1130/AgentProviderManager/issues')">{{ t('options.open', lang) }}</n-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NRadioGroup, NRadioButton, NColorPicker, NSwitch, NButton } from 'naive-ui'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { useSettingStore } from '../stores/setting'
import { useUpdateStore } from '../stores/update'
import type { Lang } from '../stores/setting'
import { t } from '../i18n'

const settingStore = useSettingStore()
const updateStore = useUpdateStore()
const lang = computed(() => settingStore.resolvedLang)

const currentLangLabel = computed(() => t(`options.${settingStore.resolvedLang}`, settingStore.resolvedLang))
const accentSwatches = [
  '#0078D4', '#0067C0', '#28a9d6', '#4aa8ff', '#107c10',
  '#ca5010', '#d13438', '#8a5cf6', '#e3008c', '#6b6b6b',
]

function openExternal(url: string) {
  BrowserOpenURL(url)
}

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}
function onLangChange(v: Lang) {
  settingStore.setLang(v)
  toast('success', t('options.lang', v === 'system' ? settingStore.resolvedLang : v), t('common.ok', v === 'system' ? settingStore.resolvedLang : v))
}
function onThemeChange(v: 'light' | 'dark' | 'system') {
  settingStore.setTheme(v)
}
function onAccentChange(v: string) {
  if (v && v.trim()) settingStore.setAccentColor(v.trim())
}
function onHttpChange(v: boolean) {
  settingStore.setHttpEnabled(v)
  toast('success', t('options.http', lang.value), t('common.ok', lang.value))
}
function onPrivateHostsChange(v: boolean) {
  settingStore.setPrivateHostsAllowed(v)
  toast('success', t('options.privateHosts', lang.value), t('common.ok', lang.value))
}
function onAutoFillChange(v: boolean) {
  settingStore.setAutoFillLimits(v)
  toast('success', t('options.autofill', lang.value), t('common.ok', lang.value))
}
function onIntensityChange(v: boolean) {
  settingStore.setReasoningAllIntensities(v)
  toast('success', t('options.intensity', lang.value), t('common.ok', lang.value))
}
function onAutoUpdateChange(v: boolean) {
  settingStore.setAutoUpdate(v)
  if (v) {
    void updateStore.check(false).then(async () => {
      if (updateStore.available && !updateStore.ready) await updateStore.download()
    })
  }
}
async function checkUpdate() {
  await updateStore.check(true)
  if (!updateStore.error && !updateStore.available) toast('success', t('options.checkUpdate', lang.value), t('options.updateNone', lang.value))
}
async function downloadUpdate() {
  await updateStore.download()
  if (updateStore.error) toast('error', t('options.checkUpdate', lang.value), updateStore.error)
}
function installUpdate() {
  window.dispatchEvent(new CustomEvent('update-request-install'))
}
</script>

<style scoped>
.options-page { display: flex; flex-direction: column; gap: 12px; }
.fluent-card { background: var(--fluent-card-bg); border-radius: 10px; box-shadow: var(--fluent-shadow-md); border: 1px solid var(--fluent-border); }
.card-pad { padding: 16px; }
.card-title { font-size: 14px; font-weight: 600; margin-bottom: 4px; }
.opt-list { display: flex; flex-direction: column; }
.opt-row {
  display: flex; align-items: center; justify-content: space-between; gap: 16px;
  padding: 14px 0; border-bottom: 1px solid var(--fluent-border); flex-wrap: wrap;
}
.opt-row:last-child { border-bottom: none; }
.opt-info { min-width: 0; flex: 1 1 320px; }
.opt-name { font-size: 13px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
.opt-tag {
  font-size: 10px; font-weight: 500; letter-spacing: 0.3px;
  color: var(--accent, #0078d4);
  background: color-mix(in srgb, var(--accent, #0078d4) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--accent, #0078d4) 30%, transparent);
  border-radius: 4px; padding: 1px 6px;
}
.opt-desc { font-size: 11px; color: var(--fluent-text-soft); margin-top: 3px; }
.opt-ctrl { flex-shrink: 0; display: flex; align-items: center; }
.opt-links, .opt-update-controls { gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
.update-ready { margin-top: 4px; font-size: 11px; color: var(--accent, #0078d4); }
.update-error { margin-top: 4px; font-size: 11px; color: #c42b1c; }
</style>
