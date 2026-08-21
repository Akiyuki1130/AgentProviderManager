import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import './styles/global.css'

const app = createApp(App)

// 全局错误边界：任何未捕获的渲染/事件/异步错误都会记录并提示，
// 避免出错后整个窗口静默白屏而无法获知原因。
function reportError(err: unknown, source: string) {
  const msg = err instanceof Error ? err.message : String(err)
  console.error(`[APM] ${source}:`, err)
  const fn = (window as unknown as Record<string, unknown>)['__toast'] as ((t: string, title: string, msg?: string) => void) | undefined
  if (fn) fn('error', '发生错误', `${source}: ${msg}`)
}
let lastRecover = 0
app.config.errorHandler = (err, _instance, info) => {
  reportError(err, info || '渲染错误')
  // 渲染/挂载期错误会导致页面区域空白，限频重挂载当前路由尝试恢复
  const now = Date.now()
  if (now - lastRecover > 3000) {
    lastRecover = now
    const route = router.currentRoute.value
    void router.replace(route.fullPath)
  }
}
window.addEventListener('error', (e) => { reportError(e.error || e.message, '页面错误') })
window.addEventListener('unhandledrejection', (e) => { reportError(e.reason, '异步错误') })

app.use(createPinia())
app.use(router)
app.mount('#app')
