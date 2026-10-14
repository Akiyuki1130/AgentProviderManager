<template>
  <aside class="app-sidebar" :class="{ collapsed }">
    <button class="sidebar-toggle" type="button" :title="collapsed ? '展开侧栏' : '收起侧栏'" :aria-label="collapsed ? '展开侧栏' : '收起侧栏'" :aria-expanded="!collapsed" @click="collapsed = !collapsed">
      <n-icon :component="collapsed ? ChevronForwardOutline : ChevronBackOutline" :size="18" />
    </button>
    <div class="app-brand">
      <svg class="brand-icon" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M12 3.5v11" />
        <path d="m7.5 10.5 4.5 4.5 4.5-4.5" />
        <path d="M4.5 19.5h15" />
      </svg>
      <span class="brand-text">Agent Provider Manager</span>
    </div>
    <nav class="app-nav" ref="navEl">
      <!-- D9：自绘导航项改为原生 button（获得键盘可达 + 全局 :focus-visible 焦点环），高亮态补 aria-current="page" -->
      <button
        v-for="item in navItems"
        :key="item.key"
        type="button"
        class="nav-item"
        :class="{ active: activeKey === item.key }"
        :aria-current="activeKey === item.key ? 'page' : undefined"
        :title="item.label"
        @click="router.push(item.key)"
      >
        <n-icon :component="item.icon" :size="18" class="nav-icon" />
        <span class="nav-label">{{ item.label }}</span>
      </button>
      <div v-show="indicatorReady" class="nav-indicator" :style="indicatorStyle" />
    </nav>
    <Transition name="fade">
      <div v-if="!collapsed && version" class="app-sidebar-footer">
        <span class="version-text">v{{ version }}</span>
      </div>
    </Transition>
  </aside>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NIcon } from 'naive-ui'
import type { Component } from 'vue'
import {
  SettingsOutline,
  CloudDownloadOutline,
  KeyOutline,
  SwapHorizontalOutline,
  OptionsOutline,
  TimeOutline,
  ChevronBackOutline,
  ChevronForwardOutline,
} from '@vicons/ionicons5'
import { useSettingStore } from '../stores/setting'
import { t } from '../i18n'
import * as api from '../api'

const route = useRoute()
const router = useRouter()
const settingStore = useSettingStore()
const collapsed = ref(false)
// 版本号以后端为唯一来源（VERSION → internal/core/version_gen.go → GetAppInfo），
// 前端不再硬编码；绑定未就绪时留空，不回退到可能过期的字面量。
const version = ref('')
const navEl = ref<HTMLElement | null>(null)
const indicatorTop = ref(0)
const indicatorHeight = ref(0)
const indicatorReady = ref(false)
const indicatorStyle = computed(() => ({ transform: `translateY(${indicatorTop.value}px)`, height: `${indicatorHeight.value}px` }))

interface NavItem { key: string; label: string; icon: Component }
const navItems = computed<NavItem[]>(() => {
  const l = settingStore.resolvedLang
  return [
    { key: '/manage', label: t('tab.manage', l), icon: SettingsOutline },
    { key: '/import', label: t('tab.import', l), icon: CloudDownloadOutline },
    { key: '/migrate', label: t('tab.migrate', l), icon: SwapHorizontalOutline },
    { key: '/keychain', label: t('tab.keychain', l), icon: KeyOutline },
    { key: '/restorepoints', label: t('tab.restorePoints', l), icon: TimeOutline },
    { key: '/options', label: t('tab.options', l), icon: OptionsOutline },
  ]
})
const activeKey = computed(() => route.path)
function updateIndicator() {
  const nav = navEl.value
  if (!nav) return
  const items = Array.from(nav.querySelectorAll<HTMLElement>('.nav-item'))
  const idx = navItems.value.findIndex((i) => i.key === activeKey.value)
  if (idx < 0 || !items[idx]) return
  const navRect = nav.getBoundingClientRect()
  const itemRect = items[idx].getBoundingClientRect()
  indicatorTop.value = itemRect.top - navRect.top + itemRect.height * 0.1
  indicatorHeight.value = itemRect.height * 0.8
  indicatorReady.value = true
}
onMounted(() => { window.addEventListener('resize', updateIndicator); nextTick(updateIndicator); void loadVersion() })
watch([activeKey, collapsed, () => settingStore.resolvedLang], () => { nextTick(updateIndicator) })
onBeforeUnmount(() => { window.removeEventListener('resize', updateIndicator) })

async function loadVersion() {
  try {
    const info = await api.GetAppInfo()
    const v = info && typeof info['version'] === 'string' ? (info['version'] as string) : ''
    if (v) version.value = v
  } catch {
    // 绑定未就绪（例如浏览器环境）时保持空，不显示可能过期的硬编码版本。
  }
}
</script>

<style scoped>
.app-sidebar {
  width: 216px; flex-shrink: 0;
  background: var(--fluent-card-bg);
  border-right: 1px solid var(--fluent-border);
  display: flex; flex-direction: column;
  transition: width var(--dur-standard-lg) var(--ease-standard), background-color var(--dur-standard) var(--ease-standard), border-color var(--dur-standard) var(--ease-standard);
  position: relative;
}
.app-sidebar.collapsed { width: 64px; }
.sidebar-toggle {
  position: absolute; top: 12px; right: -14px;
  width: 28px; height: 28px; border-radius: 50%;
  border: 1px solid var(--fluent-border);
  background: var(--fluent-card-bg); color: var(--accent);
  display: flex; align-items: center; justify-content: center; cursor: pointer; z-index: 10;
  box-shadow: var(--fluent-shadow-sm);
}
.sidebar-toggle:hover { background: var(--fluent-border); }
.app-brand {
  padding: 16px 14px 16px 20px;
  display: flex; align-items: center; gap: 10px;
  min-height: 56px; overflow: hidden; user-select: none;
}
.brand-icon { color: var(--accent, #0078d4); flex-shrink: 0; display: block; }
.brand-text { font-size: 13px; font-weight: 700; white-space: nowrap; overflow: hidden; transition: opacity var(--dur-standard) var(--ease-standard), max-width var(--dur-standard-lg) var(--ease-standard); max-width: 180px; }
.app-sidebar.collapsed .brand-text { opacity: 0; max-width: 0; }
.app-nav { flex: 1; padding: 8px 10px; display: flex; flex-direction: column; gap: 4px; overflow-y: auto; position: relative; }
.nav-item {
  display: flex; align-items: center; gap: 12px;
  /* 导航项高≈40，圆角按档位表提一档至 10px（不过圆） */
  padding: 10px 14px; border-radius: 10px; cursor: pointer;
  /* 原生 button 外观重置：外观完全沿用原自绘 div 的视觉 */
  border: 0; margin: 0; font: inherit; text-align: left; background: transparent;
  transition: background var(--dur-standard) var(--ease-standard), color var(--dur-standard) var(--ease-standard); color: inherit; user-select: none;
}
.nav-item:hover { background: color-mix(in srgb, var(--accent, #0078d4) 8%, transparent); }
.nav-item.active { background: color-mix(in srgb, var(--accent, #0078d4) 12%, transparent); color: var(--accent-text-strong); }
.nav-indicator {
  position: absolute; left: 10px; top: 0; width: 3px; border-radius: 2px;
  background: var(--accent, #0078d4); pointer-events: none;
  /* 指示条回弹 easing（参考项目同款），高度变化用标准曲线 */
  transition: transform var(--dur-settle) cubic-bezier(0.2, 0.9, 0.25, 1), height var(--dur-standard-lg) var(--ease-standard), background-color var(--dur-standard) var(--ease-standard);
}
.nav-icon { flex-shrink: 0; }
.nav-label { font-size: 13px; font-weight: 500; white-space: nowrap; overflow: hidden; max-width: 200px; transition: opacity var(--dur-standard) var(--ease-standard), max-width var(--dur-standard-lg) var(--ease-standard); }
.nav-item.active .nav-label { font-weight: 600; }
.app-sidebar.collapsed .nav-label { opacity: 0; max-width: 0; }
.app-sidebar-footer { padding: 10px 14px; border-top: 1px solid var(--fluent-border); overflow: hidden; }
.fade-enter-active, .fade-leave-active { transition: opacity 0.15s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
.version-text { font-size: 11px; opacity: 0.5; white-space: nowrap; }
</style>
