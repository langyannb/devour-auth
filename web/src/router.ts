import { createRouter, createWebHistory } from 'vue-router'
import { authed, tryRestore } from './api'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('./views/Login.vue') },
    {
      path: '/',
      component: () => import('./views/Layout.vue'),
      children: [
        { path: '', redirect: '/cards' },
        { path: 'cards', component: () => import('./views/Cards.vue') },
        { path: 'issue', component: () => import('./views/Issue.vue') },
        { path: 'policies', component: () => import('./views/Policies.vue') },
        { path: 'audit', component: () => import('./views/Audit.vue') },
        { path: 'settings', component: () => import('./views/Settings.vue') },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.path === '/login') return true
  if (authed.value) return true
  return (await tryRestore()) ? true : '/login'
})

export default router
