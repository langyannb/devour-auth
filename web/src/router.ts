import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { agentAuthed, authed, tryRestore, tryRestoreAgent } from './api'

// 子域决定端：admin.* → 管理端，agent.* → 代理端，unbind.* → 终端自解绑页，
// 其余 → 官网/查卡。本地开发用 ?role=agent 切到代理端。
const host = location.hostname
const forced = new URLSearchParams(location.search).get('role')
const isUnbind = host.startsWith('unbind.')
const isAgent = host.startsWith('agent.') || forced === 'agent'
// ?role=public 本地看官网首页（默认 127.0.0.1 走管理端）
const isPublic = forced === 'public'
const isAdmin = host.startsWith('admin.') || forced === 'admin' ||
  host === 'localhost' || host === '127.0.0.1' || host === ''

const adminRoutes: RouteRecordRaw[] = [
  { path: '/login', component: () => import('./views/Login.vue') },
  {
    path: '/',
    component: () => import('./views/Layout.vue'),
    children: [
      { path: '', redirect: '/dashboard' },
      { path: 'dashboard', meta: { title: '概览' }, component: () => import('./views/Overview.vue') },
      { path: 'cards', meta: { title: '卡密' }, component: () => import('./views/Cards.vue') },
      { path: 'cards/:id', component: () => import('./views/CardDetail.vue') },
      { path: 'devices', meta: { title: '设备' }, component: () => import('./views/Devices.vue') },
      { path: 'sessions', meta: { title: '在线会话' }, component: () => import('./views/Sessions.vue') },
      { path: 'issue', meta: { title: '发卡' }, component: () => import('./views/Issue.vue') },
      { path: 'policies', meta: { title: '策略' }, component: () => import('./views/Policies.vue') },
      { path: 'entitlements', meta: { title: '权限包' }, component: () => import('./views/Entitlements.vue') },
      { path: 'apps', meta: { title: '应用' }, component: () => import('./views/Apps.vue') },
      { path: 'accounts', meta: { title: '账号' }, component: () => import('./views/Accounts.vue') },
      { path: 'open', meta: { title: '开放接口' }, component: () => import('./views/OpenApi.vue') },
      { path: 'webhooks', meta: { title: '回调' }, component: () => import('./views/Webhooks.vue') },
      { path: 'blacklist', meta: { title: '黑名单' }, component: () => import('./views/Blacklist.vue') },
      { path: 'events', meta: { title: '日志' }, component: () => import('./views/Events.vue') },
      { path: 'finance', meta: { title: '财务' }, component: () => import('./views/Finance.vue') },
      { path: 'agents', meta: { title: '代理' }, component: () => import('./views/Agents.vue') },
      { path: 'plans', meta: { title: '价格模板' }, component: () => import('./views/Plans.vue') },
      { path: 'agents/:id', component: () => import('./views/AgentDetail.vue') },
      { path: 'ops', meta: { title: '运营' }, component: () => import('./views/Ops.vue') },
      { path: 'audit', meta: { title: '审计' }, component: () => import('./views/Audit.vue') },
      { path: 'settings', meta: { title: '设置' }, component: () => import('./views/Settings.vue') },
      { path: 'security', meta: { title: '安全' }, component: () => import('./views/Security.vue') },
    ],
  },
]

const agentRoutes: RouteRecordRaw[] = [
  { path: '/login', component: () => import('./views/agent/Login.vue') },
  // 下级代理专属链接：agent.域名/<slug>
  { path: '/xj/:code', name: 'agent-invite', component: () => import('./views/agent/Invite.vue') },
  { path: '/:code', redirect: (to) => `/xj/${to.params.code}` },
  {
    path: '/',
    component: () => import('./views/agent/Layout.vue'),
    children: [
      { path: '', redirect: '/dashboard' },
      { path: 'dashboard', meta: { title: '概览' }, component: () => import('./views/agent/Dashboard.vue') },
      { path: 'cards', meta: { title: '卡密' }, component: () => import('./views/agent/Cards.vue') },
      { path: 'issue', meta: { title: '发卡' }, component: () => import('./views/agent/Issue.vue') },
      { path: 'subagents', component: () => import('./views/agent/Subagents.vue') },
      { path: 'billing', component: () => import('./views/agent/Billing.vue') },
      { path: 'settings', meta: { title: '设置' }, component: () => import('./views/agent/Settings.vue') },
    ],
  },
]

const publicRoutes: RouteRecordRaw[] = [
  { path: '/', component: () => import('./views/public/Landing.vue') },
  { path: '/query', component: () => import('./views/public/Query.vue') },
]

// 解绑页单独一个域，只挂这一条路由，任何其它路径回首页。
const unbindRoutes: RouteRecordRaw[] = [
  { path: '/', component: () => import('./views/public/Unbind.vue') },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

const router = createRouter({
  history: createWebHistory(),
  routes: isUnbind ? unbindRoutes : isPublic ? publicRoutes : isAgent ? agentRoutes : isAdmin ? adminRoutes : publicRoutes,
})

router.beforeEach(async (to) => {
  if (isUnbind) return true // 解绑页免登录
  if (isPublic) return true // 官网首页免登录
  if (!isAgent && !isAdmin) return true // 官网与查卡页免登录

  if (isAgent) {
    if (to.path === '/login' || to.name === 'agent-invite') return true
    if (agentAuthed.value) return true
    return (await tryRestoreAgent()) ? true : '/login'
  }

  if (to.path === '/login') return true
  if (authed.value) return true
  return (await tryRestore()) ? true : '/login'
})

export default router
