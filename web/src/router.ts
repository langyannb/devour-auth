import { createRouter, createWebHistory } from 'vue-router'
import { agentAuthed, authed, tryRestore, tryRestoreAgent } from './api'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('./views/Login.vue') },
    { path: '/agent/login', component: () => import('./views/agent/Login.vue') },
    {
      path: '/agent',
      component: () => import('./views/agent/Layout.vue'),
      children: [
        { path: '', redirect: '/agent/cards' },
        { path: 'cards', component: () => import('./views/agent/Cards.vue') },
        { path: 'issue', component: () => import('./views/agent/Issue.vue') },
        { path: 'billing', component: () => import('./views/agent/Billing.vue') },
      ],
    },
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
  if (to.path === '/login' || to.path === '/agent/login') return true
  if (to.path.startsWith('/agent')) {
    if (agentAuthed.value) return true
    return (await tryRestoreAgent()) ? true : '/agent/login'
  }
  if (authed.value) return true
  return (await tryRestore()) ? true : '/login'
})

export default router
