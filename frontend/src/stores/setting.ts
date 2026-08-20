import { defineStore } from 'pinia'
import { ref } from 'vue'

type AppTheme = 'light' | 'dark' | 'system'
type Lang = 'zh' | 'en'
export type AgentID = 'zcode' | 'opencode' | 'deepseek'

const THEME_KEY = 'apm:theme'
const LANG_KEY = 'apm:lang'
const ACCENT_KEY = 'apm:accent'
const AGENT_KEY = 'apm:agent'
const LEGACY_THEME_KEY = 'zcode-pm:theme'
const LEGACY_LANG_KEY = 'zcode-pm:lang'
const LEGACY_ACCENT_KEY = 'zcode-pm:accent'

function loadTheme(): AppTheme {
  const v = localStorage.getItem(THEME_KEY) || localStorage.getItem(LEGACY_THEME_KEY)
  return v === 'light' || v === 'dark' || v === 'system' ? v : 'system'
}
function loadLang(): Lang {
  const v = localStorage.getItem(LANG_KEY) || localStorage.getItem(LEGACY_LANG_KEY)
  return v === 'en' ? 'en' : 'zh'
}
function loadAccent(): string {
  return localStorage.getItem(ACCENT_KEY) || localStorage.getItem(LEGACY_ACCENT_KEY) || '#0067C0'
}
function loadAgent(): AgentID {
  const v = localStorage.getItem(AGENT_KEY)
  if (v === 'zcode' || v === 'opencode' || v === 'deepseek') return v
  const legacy = localStorage.getItem('zcode-pm:agent')
  if (legacy === 'zcode' || legacy === 'opencode' || legacy === 'deepseek') return legacy as AgentID
  return 'zcode'
}

export const useSettingStore = defineStore('setting', () => {
  const theme = ref<AppTheme>(loadTheme())
  const lang = ref<Lang>(loadLang())
  const accentColor = ref<string>(loadAccent())
  const agent = ref<AgentID>(loadAgent())
  const systemDark = ref<boolean>(window.matchMedia('(prefers-color-scheme: dark)').matches)

  function initTheme() {
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    systemDark.value = mq.matches
    mq.addEventListener('change', (e) => { systemDark.value = e.matches })
  }
  function setTheme(t: AppTheme) {
    theme.value = t
    try { localStorage.setItem(THEME_KEY, t) } catch { /* ignore */ }
  }
  function setLang(l: Lang) {
    lang.value = l
    try { localStorage.setItem(LANG_KEY, l) } catch { /* ignore */ }
  }
  function setAccentColor(c: string) {
    accentColor.value = c
    try { localStorage.setItem(ACCENT_KEY, c) } catch { /* ignore */ }
  }
  function setAgent(a: AgentID) {
    agent.value = a
    try { localStorage.setItem(AGENT_KEY, a) } catch { /* ignore */ }
  }
  return { theme, lang, accentColor, agent, systemDark, initTheme, setTheme, setLang, setAccentColor, setAgent }
})
