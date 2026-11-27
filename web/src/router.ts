import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { agentAuthed, authed, tryRestore, tryRestoreAgent } from './api'

// 子域决定端：admin.* → 管理端，agent.* → 代理端，其余 → 官网/查卡。
// 本地开发用 ?role=agent 切到代理端。
const host = location.hostname
const forced = new URLSearchParams(location.search).get('role')
const isAgent = host.startsWith('agent.') || forced === 'agent'
const isAdmin = host.startsWith('admin.') || forced === 'admin' ||
  host === 'localhost' || host === '127.0.0.1' || host === ''

const adminRoutes: RouteRecordRaw[] = [
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
]

const agentRoutes: RouteRecordRaw[] = [
  { path: '/login', component: () => import('./views/agent/Login.vue') },
  {
    path: '/',
    component: () => import('./views/agent/Layout.vue'),
    children: [
      { path: '', redirect: '/cards' },
      { path: 'cards', component: () => import('./views/agent/Cards.vue') },
      { path: 'issue', component: () => import('./views/agent/Issue.vue') },
      { path: 'billing', component: () => import('./views/agent/Billing.vue') },
    ],
  },
]

const publicRoutes: RouteRecordRaw[] = [
  { path: '/', component: () => import('./views/public/Landing.vue') },
  { path: '/query', component: () => import('./views/public/Query.vue') },
]

const router = createRouter({
  history: createWebHistory(),
  routes: isAgent ? agentRoutes : isAdmin ? adminRoutes : publicRoutes,
})

router.beforeEach(async (to) => {
  if (!isAgent && !isAdmin) return true // 官网与查卡页免登录

  if (isAgent) {
    if (to.path === '/login') return true
    if (agentAuthed.value) return true
    return (await tryRestoreAgent()) ? true : '/login'
  }

  if (to.path === '/login') return true
  if (authed.value) return true
  return (await tryRestore()) ? true : '/login'
})

export default router
