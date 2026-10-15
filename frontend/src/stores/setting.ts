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
const PRIVATE_HOSTS_KEY = 'apm:privateHosts'
const AUTOFILL_KEY = 'apm:autofill'
const CUSTOM_VARIANTS_KEY = 'apm:customVariants'
const REASONING_ALL_KEY = 'apm:reasoningAll'
const AUTO_UPDATE_KEY = 'apm:autoUpdate'
const AUTO_SAVE_KEY = 'apm:autoSave'
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
// OpenCode 入口已隐藏：本地历史值 "opencode" 归一化回落到 zcode，
// 避免应用停在一个不可选的 Agent 上。AgentID 类型保留并集以兼容历史数据。
function loadAgent(): AgentID {
  const v = localStorage.getItem(AGENT_KEY)
  if (v === 'zcode' || v === 'deepseek') return v
  const legacy = localStorage.getItem('zcode-pm:agent')
  if (legacy === 'zcode' || legacy === 'deepseek') return legacy as AgentID
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
  // M7：agent 由 App.vue 在挂载时经后端回读，`loadAgent()` 的首帧默认值是本地缓存的
  // 兜底（可能是 'zcode' 而实际为 'deepseek'）。此标记在回读尝试结束后置真，
  // 供依赖 agent 的提示（如「仅对 DSH 生效」）在首帧按“未知”处理、避免闪现后消失。
  const agentResolved = ref<boolean>(false)
  const httpEnabled = ref<boolean>(loadBool(HTTP_KEY, false))
  const privateHostsAllowed = ref<boolean>(loadBool(PRIVATE_HOSTS_KEY, false))
  const autoFillLimits = ref<boolean>(loadBool(AUTOFILL_KEY, true))
  const reasoningAllIntensities = ref<boolean>(loadBool(REASONING_ALL_KEY, false))
  // C2：允许自填推理档位。与 reasoningAllIntensities 不同——这个开关后端要据此放宽校验，
  // 因此走 trackSetter 持久化到后端，localStorage 仅作冷启动兜底（默认关闭）。
  const customVariants = ref<boolean>(loadBool(CUSTOM_VARIANTS_KEY, false))
  const autoSave = ref<boolean>(loadBool(AUTO_SAVE_KEY, true))
  const autoUpdate = ref<boolean>(loadBool(AUTO_UPDATE_KEY, false))
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
  // D10：以下 setter 带后端调用，不再 fire-and-forget 吞错——
  // 保留乐观更新与 localStorage 写入，返回 Promise；后端失败时先回滚本地值与持久化副本再 reject，
  // 由调用方（OptionsPage）决定提示文案与受控件回滚。
  // 注意：Go 侧 Set* 不抛错，业务失败走 resolve 的 {success:false, error} 载荷，
  // 因此这里要同时把「binding reject」与「success=false」都归一化成 rejection。
  // App.vue 启动同步不 await 这些 setter，因此预先挂一个只吞错不处理的 catch，
  // 让 rejection 仍能传播给 await 的调用方，但不会冒泡成全局 unhandledrejection。
  function persist(key: string, v: string) {
    try { localStorage.setItem(key, v) } catch { /* ignore */ }
  }
  function setterError(res: unknown, fallback: string): Error {
    const r = res as Record<string, unknown> | null
    const msg = (r && typeof r['error'] === 'string' && r['error']) || fallback
    return new Error(msg)
  }
  function trackSetter(run: () => Promise<unknown>, rollback: () => void, fallback: string): Promise<void> {
    const out = run().then(
      (res) => {
        const r = res as Record<string, unknown> | null
        if (r && r['success'] === false) { rollback(); throw setterError(res, fallback) }
      },
      (err) => { rollback(); throw err },
    )
    out.catch(() => { /* 由 await 方处理，这里仅保证未被 await 时不触发全局错误 */ })
    return out
  }
  function setTheme(t: AppTheme) {
    const prev = theme.value
    theme.value = t
    persist(THEME_KEY, t)
    return trackSetter(() => api.SetTheme(t), () => { theme.value = prev; persist(THEME_KEY, prev) }, '主题设置保存失败')
  }
  function setLang(l: Lang) {
    const prev = lang.value
    lang.value = l
    persist(LANG_KEY, l)
    return trackSetter(() => api.SetLanguage(l), () => { lang.value = prev; persist(LANG_KEY, prev) }, '语言设置保存失败')
  }
  function setAccentColor(c: string) {
    const prev = accentColor.value
    accentColor.value = c
    persist(ACCENT_KEY, c)
    return trackSetter(() => api.SetAccent(c), () => { accentColor.value = prev; persist(ACCENT_KEY, prev) }, '主题色保存失败')
  }
  function setHttpEnabled(v: boolean) {
    const prev = httpEnabled.value
    httpEnabled.value = v
    persist(HTTP_KEY, String(v))
    return trackSetter(() => api.SetHttpEnabled(v), () => { httpEnabled.value = prev; persist(HTTP_KEY, String(prev)) }, 'HTTP 选项保存失败')
  }
  function setPrivateHostsAllowed(v: boolean) {
    const prev = privateHostsAllowed.value
    privateHostsAllowed.value = v
    persist(PRIVATE_HOSTS_KEY, String(v))
    return trackSetter(() => api.SetPrivateHostsAllowed(v), () => { privateHostsAllowed.value = prev; persist(PRIVATE_HOSTS_KEY, String(prev)) }, '私网选项保存失败')
  }
  function setAutoFillLimits(v: boolean) {
    const prev = autoFillLimits.value
    autoFillLimits.value = v
    persist(AUTOFILL_KEY, String(v))
    return trackSetter(() => api.SetAutoFillLimits(v), () => { autoFillLimits.value = prev; persist(AUTOFILL_KEY, String(prev)) }, '自动填充选项保存失败')
  }
  function setReasoningAllIntensities(v: boolean) {
    reasoningAllIntensities.value = v
    try { localStorage.setItem(REASONING_ALL_KEY, String(v)) } catch { /* ignore */ }
  }
  // C2：用户主动切换与启动回填共用同一入口——App.vue 启动回读 GetOptions() 时也调用它。
  // 做法与 setAutoFillLimits 完全一致（该 setter 在回填时同样会落后端），保持一致、不另造回读机制。
  function setCustomVariants(v: boolean) {
    const prev = customVariants.value
    customVariants.value = v
    persist(CUSTOM_VARIANTS_KEY, String(v))
    return trackSetter(() => api.SetCustomVariantsEnabled(v), () => { customVariants.value = prev; persist(CUSTOM_VARIANTS_KEY, String(prev)) }, '自填档位选项保存失败')
  }
  // 纯客户端偏好：无后端调用，只写 localStorage
  function setAutoSave(v: boolean) {
    autoSave.value = v
    try { localStorage.setItem(AUTO_SAVE_KEY, String(v)) } catch { /* ignore */ }
  }
  function setAutoUpdate(v: boolean) {
    autoUpdate.value = v
    try { localStorage.setItem(AUTO_UPDATE_KEY, String(v)) } catch { /* ignore */ }
  }
  function setAgent(a: AgentID) {
    agent.value = a
    try { localStorage.setItem(AGENT_KEY, a) } catch { /* ignore */ }
  }
  // App.vue 的 agent 回读（GetCurrentAgent / GetTargetConfig）结束后调用：无论成功与否，
  // 都标记 agent 已解析，让依赖 agent 的提示退出“首帧未知”态。
  function markAgentResolved() { agentResolved.value = true }
  return {
    theme, lang, accentColor, agent, agentResolved, httpEnabled, privateHostsAllowed, autoFillLimits, reasoningAllIntensities, customVariants, autoSave, autoUpdate, systemDark, resolvedLang,
    initTheme, setTheme, setLang, setAccentColor, setHttpEnabled, setPrivateHostsAllowed, setAutoFillLimits, setReasoningAllIntensities, setCustomVariants, setAutoSave, setAutoUpdate, setAgent, markAgentResolved,
  }
})
