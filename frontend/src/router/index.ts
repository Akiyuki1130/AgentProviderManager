import { createRouter, createWebHashHistory } from 'vue-router'
import { leaveDirty, requestLeave } from '../stores/leaveGuard'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/manage' },
    { path: '/manage', name: 'Manage', component: () => import('../pages/ManagePage.vue') },
    { path: '/import', name: 'Import', component: () => import('../pages/ImportPage.vue') },
    { path: '/keychain', name: 'Keychain', component: () => import('../pages/KeychainPage.vue') },
    { path: '/restorepoints', name: 'RestorePoints', component: () => import('../pages/RestorePointPage.vue') },
    { path: '/migrate', name: 'Migrate', component: () => import('../pages/MigratePage.vue') },
    { path: '/options', name: 'Options', component: () => import('../pages/OptionsPage.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/manage' },
  ],
})

// 离开管理页前若有未保存更改，先弹窗询问（保存/不保存/取消）；
// 自动保存开启时 requestLeave 会先尝试静默落盘，写不进去才弹窗。
router.beforeEach(async (to, from) => {
  if (from.name !== 'Manage' || !leaveDirty()) return true
  const target = to.fullPath
  await requestLeave(() => {
    void router.push(target)
  })
  return false
})

export default router
