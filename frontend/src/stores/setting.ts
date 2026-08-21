import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import * as api from '../api'

type AppTheme = 'light' | 'dark' | 'system'
export type Lang = 'system' | 'zh' | 'en' | 'ja'
export type ResolvedLang = 'zh' | 'en' | 'ja'
export type AgentID = 'zcode' | 'opencode' | 'deepseek'

const THEME_KEY = 'apm:theme'
const LANG_KEY = 'apm:lang'
const ACCENT_KEY = 'apm:accent'
const AGENT_KEY = 'apm:agent'
const HTTP_KEY = 'apm:http'
const AUTOFILL_KEY = 'apm:autofill'
const REASONING_ALL_KEY = 'apm:reasoningAll'
const LEGACY_THEME_KEY = 'zcode-pm:theme'
const LEGACY_LANG_KEY = 'zcode-pm:lang'
const LEGACY_ACCENT_KEY = 'zcode-pm:accent'

function loadTheme(): AppTheme {
  const v = localStorage.getItem(THEME_KEY) || localStorage.getItem(LEGACY_THEME_KEY)
  return v === 'light' || v === 'dark' || v === 'system' ? v : 'system'
}
function loadLang(): Lang {
  const v = localStorage.getItem(LANG_KEY) || localStorage.getItem(LEGACY_LANG_KEY)
  return v === 'system' || v === 'zh' || v === 'en' || v === 'ja' ? v : 'system'
}
const DEFAULT_ACCENT = '#0078D4'
const LEGACY_ACCENT_VALUE = '#0067C0'

function loadAccent(): string {
  const stored = localStorage.getItem(ACCENT_KEY) || localStorage.getItem(LEGACY_ACCENT_KEY)
  if (!stored) return DEFAULT_ACCENT
  // 迁移早期版本的深蓝默认色，改为更浅的主题色
  if (stored.trim().toLowerCase() === LEGACY_ACCENT_VALUE.toLowerCase()) return DEFAULT_ACCENT
  return stored
}
function loadBool(key: string, def: boolean): boolean {
  const v = localStorage.getItem(key)
  if (v === 'true') return true
  if (v === 'false') return false
  return def
}
function loadAgent(): AgentID {
  const v = localStorage.getItem(AGENT_KEY)
  if (v === 'zcode' || v === 'opencode' || v === 'deepseek') return v
  const legacy = localStorage.getItem('zcode-pm:agent')
  if (legacy === 'zcode' || legacy === 'opencode' || legacy === 'deepseek') return legacy as AgentID
  return 'zcode'
}

function detectSystemLang(): ResolvedLang {
  const nl = (navigator.language || navigator.languages?.[0] || 'en').toLowerCase()
  if (nl.startsWith('zh')) return 'zh'
  if (nl.startsWith('ja')) return 'ja'
  return 'en'
}

export const useSettingStore = defineStore('setting', () => {
  const theme = ref<AppTheme>(loadTheme())
  const lang = ref<Lang>(loadLang())
  const accentColor = ref<string>(loadAccent())
  const agent = ref<AgentID>(loadAgent())
  const httpEnabled = ref<boolean>(loadBool(HTTP_KEY, false))
  const autoFillLimits = ref<boolean>(loadBool(AUTOFILL_KEY, true))
  const reasoningAllIntensities = ref<boolean>(loadBool(REASONING_ALL_KEY, false))
  const systemDark = ref<boolean>(window.matchMedia('(prefers-color-scheme: dark)').matches)

  // 语言设为「跟随系统」时按系统语言解析出实际界面语言
  const resolvedLang = computed<ResolvedLang>(() => {
    if (lang.value !== 'system') return lang.value
    return detectSystemLang()
  })

  function initTheme() {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    systemDark.value = mq.matches
    mq.addEventListener('change', (e) => { systemDark.value = e.matches })
  }
  function setTheme(t: AppTheme) {
    theme.value = t
    try { localStorage.setItem(THEME_KEY, t) } catch { /* ignore */ }
    void api.SetTheme(t).catch(() => { /* ignore */ })
  }
  function setLang(l: Lang) {
    lang.value = l
    try { localStorage.setItem(LANG_KEY, l) } catch { /* ignore */ }
    void api.SetLanguage(l).catch(() => { /* ignore */ })
  }
  function setAccentColor(c: string) {
    accentColor.value = c
    try { localStorage.setItem(ACCENT_KEY, c) } catch { /* ignore */ }
    void api.SetAccent(c).catch(() => { /* ignore */ })
  }
  function setHttpEnabled(v: boolean) {
    httpEnabled.value = v
    try { localStorage.setItem(HTTP_KEY, String(v)) } catch { /* ignore */ }
    void api.SetHttpEnabled(v).catch(() => { /* ignore */ })
  }
  function setAutoFillLimits(v: boolean) {
    autoFillLimits.value = v
    try { localStorage.setItem(AUTOFILL_KEY, String(v)) } catch { /* ignore */ }
    void api.SetAutoFillLimits(v).catch(() => { /* ignore */ })
  }
  function setReasoningAllIntensities(v: boolean) {
    reasoningAllIntensities.value = v
    try { localStorage.setItem(REASONING_ALL_KEY, String(v)) } catch { /* ignore */ }
  }
  function setAgent(a: AgentID) {
    agent.value = a
    try { localStorage.setItem(AGENT_KEY, a) } catch { /* ignore */ }
  }
  return {
    theme, lang, accentColor, agent, httpEnabled, autoFillLimits, reasoningAllIntensities, systemDark, resolvedLang,
    initTheme, setTheme, setLang, setAccentColor, setHttpEnabled, setAutoFillLimits, setReasoningAllIntensities, setAgent,
  }
})
