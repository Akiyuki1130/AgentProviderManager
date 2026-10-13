<template>
  <div class="options-page">
    <!-- C5：页头标题自卡片迁入，不在卡内重复渲染；本页无页面级主操作，actions 留空 -->
    <PageHeader class="enter-stagger" :style="{ animationDelay: staggerMs(0) + 'ms' }" :title="t('options.title', lang)">
      <template #icon><n-icon :component="OptionsOutline" :size="18" /></template>
    </PageHeader>

    <div class="fluent-card card-pad enter-stagger" :style="{ animationDelay: staggerMs(1) + 'ms' }">
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
          <div class="opt-ctrl accent-ctrl">
            <div class="accent-picker">
              <div class="accent-presets">
                <button
                  v-for="c in accentSwatches"
                  :key="c"
                  type="button"
                  class="accent-swatch"
                  :class="{ 'is-selected': isPresetSelected(c) }"
                  :style="{ '--swatch-color': c, '--swatch-tick': accentTextOnAccent(c) }"
                  :title="c"
                  @click="onAccentChange(c)"
                >
                  <n-icon v-if="isPresetSelected(c)" :size="14"><Checkmark /></n-icon>
                </button>
              </div>
              <div class="accent-custom">
                <span class="accent-custom-label">{{ t('options.accentCustom', lang) }}</span>
                <button
                  type="button"
                  class="accent-swatch"
                  :class="{ 'is-selected': isCustomAccent }"
                  :style="{ '--swatch-color': settingStore.accentColor, '--swatch-tick': accentTextOnAccent(settingStore.accentColor) }"
                  :title="settingStore.accentColor"
                  @click="triggerColorInput"
                >
                  <n-icon v-if="isCustomAccent" :size="14"><Checkmark /></n-icon>
                </button>
                <!-- 原生取色器：只绑 @change，避免拖动取色时逐帧触发 IPC 保存 -->
                <input
                  ref="colorInputEl"
                  :value="nativeColorValue"
                  type="color"
                  class="accent-color-input-hidden"
                  @change="onNativeColorChange"
                />
                <span class="accent-value">{{ settingStore.accentColor }}</span>
              </div>
            </div>
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
import { computed, ref } from 'vue'
import { NRadioGroup, NRadioButton, NSwitch, NButton, NIcon } from 'naive-ui'
import { OptionsOutline, Checkmark } from '@vicons/ionicons5'
import { accentTextOnAccent } from '../styles/theme'
import PageHeader from '../components/ui/PageHeader.vue'
import { staggerMs } from '../composables/useStagger'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { useSettingStore } from '../stores/setting'
import { useUpdateStore } from '../stores/update'
import type { Lang } from '../stores/setting'
import { t } from '../i18n'

const settingStore = useSettingStore()
const updateStore = useUpdateStore()
const lang = computed(() => settingStore.resolvedLang)

const currentLangLabel = computed(() => t(`options.${settingStore.resolvedLang}`, settingStore.resolvedLang))
// Win11 经典强调色预设（参考 VrcFrameLimit AppearanceSection）
const accentSwatches = ['#0067C0', '#8764B8', '#038387', '#107C10', '#CA5010', '#D13438', '#515C6B', '#744DA9']
const colorInputEl = ref<HTMLInputElement | null>(null)
// 原生 color input 要求小写 #rrggbb，否则会被浏览器清空为 #000000
const nativeColorValue = computed(() => settingStore.accentColor.toLowerCase())
function isPresetSelected(c: string) {
  return c.toLowerCase() === settingStore.accentColor.toLowerCase()
}
// 当前值不属于任何预设时，"自定义"色板呈选中态
const isCustomAccent = computed(() => !accentSwatches.some((c) => c.toLowerCase() === settingStore.accentColor.toLowerCase()))
function triggerColorInput() {
  colorInputEl.value?.click()
}
function onNativeColorChange(e: Event) {
  void onAccentChange((e.target as HTMLInputElement).value)
}

function openExternal(url: string) {
  BrowserOpenURL(url)
}

function toast(type: string, title: string, msg?: string) {
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn(type, title, msg)
}
// D10：带后端调用的选项先 await setter 的 Promise，成功才弹成功 toast；
// 失败弹错误 toast，并把受控开关/选项值回滚到操作前的旧值（store 侧已同步回滚 localStorage）。
function errDetail(e: unknown): string {
  if (e instanceof Error) return e.message
  if (typeof e === 'string') return e
  try { return JSON.stringify(e) } catch { return String(e) }
}
async function onLangChange(v: Lang) {
  const prev = settingStore.lang
  const shown = v === 'system' ? settingStore.resolvedLang : v
  try {
    await settingStore.setLang(v)
    toast('success', t('options.lang', shown), t('common.ok', shown))
  } catch (e) {
    settingStore.lang = prev
    toast('error', t('options.lang', prev === 'system' ? settingStore.resolvedLang : prev), errDetail(e))
  }
}
async function onThemeChange(v: 'light' | 'dark' | 'system') {
  const prev = settingStore.theme
  try {
    await settingStore.setTheme(v)
  } catch (e) {
    settingStore.theme = prev
    toast('error', t('options.theme', lang.value), errDetail(e))
  }
}
async function onAccentChange(v: string) {
  if (!v || !v.trim()) return
  const prev = settingStore.accentColor
  try {
    await settingStore.setAccentColor(v.trim())
  } catch (e) {
    settingStore.accentColor = prev
    toast('error', t('options.accent', lang.value), errDetail(e))
  }
}
async function onHttpChange(v: boolean) {
  const prev = settingStore.httpEnabled
  try {
    await settingStore.setHttpEnabled(v)
    toast('success', t('options.http', lang.value), t('common.ok', lang.value))
  } catch (e) {
    settingStore.httpEnabled = prev
    toast('error', t('options.http', lang.value), errDetail(e))
  }
}
async function onPrivateHostsChange(v: boolean) {
  const prev = settingStore.privateHostsAllowed
  try {
    await settingStore.setPrivateHostsAllowed(v)
    toast('success', t('options.privateHosts', lang.value), t('common.ok', lang.value))
  } catch (e) {
    settingStore.privateHostsAllowed = prev
    toast('error', t('options.privateHosts', lang.value), errDetail(e))
  }
}
async function onAutoFillChange(v: boolean) {
  const prev = settingStore.autoFillLimits
  try {
    await settingStore.setAutoFillLimits(v)
    toast('success', t('options.autofill', lang.value), t('common.ok', lang.value))
  } catch (e) {
    settingStore.autoFillLimits = prev
    toast('error', t('options.autofill', lang.value), errDetail(e))
  }
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
/* B10：.fluent-card 用 global.css 规范层定义，此处不再重复 */
.options-page { display: flex; flex-direction: column; gap: 12px; }
.card-pad { padding: 16px; }
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
  /* 圆角按控件档位表归位 8px（≈28 档小控件；accent tint 属数据强调变体，保留） */
  border-radius: 8px; padding: 1px 6px;
}
.opt-desc { font-size: 11px; color: var(--fluent-text-soft); margin-top: 3px; }
.opt-ctrl { flex-shrink: 0; display: flex; align-items: center; }
.opt-links, .opt-update-controls { gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
.update-ready { margin-top: 4px; font-size: 11px; color: var(--accent, #0078d4); }
.update-error { margin-top: 4px; font-size: 11px; color: var(--err); }

/* === 主题色色板（参考 VrcFrameLimit AppearanceSection，圆形 50% 为合法例外） === */
.accent-ctrl { flex-wrap: wrap; justify-content: flex-end; }
.accent-picker { display: flex; flex-direction: column; gap: 10px; }
.accent-presets { display: flex; flex-wrap: wrap; gap: 10px; justify-content: flex-end; }
.accent-swatch {
  width: 28px; height: 28px; padding: 0;
  border: 2px solid transparent; border-radius: 50%;
  background-color: var(--swatch-color);
  box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.12);
  cursor: pointer;
  display: inline-flex; align-items: center; justify-content: center;
  color: var(--swatch-tick, #fff);
  transition: transform 0.15s ease, border-color 0.15s ease, box-shadow 0.15s ease, background-color 0.15s ease;
}
.accent-swatch:hover { transform: scale(1.1); }
.accent-swatch.is-selected {
  border-color: var(--fluent-text, #000);
  box-shadow: 0 0 0 2px var(--fluent-card-bg, #fff), 0 0 0 4px var(--swatch-color);
}
.accent-custom { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; justify-content: flex-end; }
.accent-custom-label { font-size: 12px; color: var(--fluent-text-soft, rgba(128, 128, 128, 0.85)); }
.accent-color-input-hidden {
  position: absolute; width: 0; height: 0; padding: 0; border: none;
  opacity: 0; pointer-events: none;
}
.accent-value {
  font-size: 12px; font-family: 'Cascadia Code', 'Consolas', monospace;
  color: var(--fluent-text-soft, rgba(128, 128, 128, 0.85));
}
</style>
