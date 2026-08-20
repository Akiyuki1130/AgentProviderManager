import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/manage' },
    { path: '/manage', name: 'Manage', component: () => import('../pages/ManagePage.vue') },
    { path: '/import', name: 'Import', component: () => import('../pages/ImportPage.vue') },
    { path: '/keychain', name: 'Keychain', component: () => import('../pages/KeychainPage.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/manage' },
  ],
})

export default router
