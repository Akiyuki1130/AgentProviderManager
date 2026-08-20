import { defineStore } from 'pinia'
import { ref } from 'vue'

type AppTheme = 'light' | 'dark' | 'system'
type Lang = 'zh' | 'en'

const THEME_KEY = 'zcode-pm:theme'
const LANG_KEY = 'zcode-pm:lang'
const ACCENT_KEY = 'zcode-pm:accent'

function loadTheme(): AppTheme {
  const v = localStorage.getItem(THEME_KEY)
  return v === 'light' || v === 'dark' || v === 'system' ? v : 'system'
}
function loadLang(): Lang {
  const v = localStorage.getItem(LANG_KEY)
  return v === 'en' ? 'en' : 'zh'
}
function loadAccent(): string {
  return localStorage.getItem(ACCENT_KEY) || '#0067C0'
}

export const useSettingStore = defineStore('setting', () => {
  const theme = ref<AppTheme>(loadTheme())
  const lang = ref<Lang>(loadLang())
  const accentColor = ref<string>(loadAccent())
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
  return { theme, lang, accentColor, systemDark, initTheme, setTheme, setLang, setAccentColor }
})
