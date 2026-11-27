<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NSelect, type MenuOption } from 'naive-ui'
import PortalShell from '../components/PortalShell.vue'
import { adminAppID, api, getCSRF, getCurrentApp, markSignedOut, setCurrentApp, tryRestore } from '../api'
import ReauthDialog from './ReauthDialog.vue'

const router = useRouter()
const rawUser = sessionStorage.getItem('devour_user') ?? ''
const user = rawUser.split(' · ')[0]
const initial = (user || 'A').slice(0, 1).toUpperCase()

const scoped = computed(() => adminAppID.value > 0)

const fullMenu = computed<MenuOption[]>(() => [
  { label: '概览', key: '/dashboard' },
  { label: '数据', key: '/analytics' },
  {
    type: 'group',
    key: 'g-cards',
    label: '卡务',
    children: [
      { label: '卡密', key: '/cards' },
      { label: '发卡', key: '/issue' },
      { label: '设备', key: '/devices' },
      { label: '在线', key: '/sessions' },
      { label: '黑名单', key: '/blacklist' },
    ],
  },
  {
    type: 'group',
    key: 'g-content',
    label: '权限与内容',
    children: [
      { label: '权限包', key: '/entitlements' },
      { label: '应用', key: '/apps' },
    ],
  },
  {
    type: 'group',
    key: 'g-agents',
    label: '分销',
    children: [
      { label: '代理', key: '/agents' },
      { label: '价格模板', key: '/plans' },
      { label: '财务', key: '/finance' },
    ],
  },
  {
    type: 'group',
    key: 'g-integration',
    label: '集成',
    children: [
      { label: '对接信息', key: '/integration' },
      { label: '开放接口', key: '/open' },
      { label: '回调', key: '/webhooks' },
    ],
  },
  {
    type: 'group',
    key: 'g-ops',
    label: '运维',
    children: [
      { label: '运营', key: '/ops' },
      { label: '日志', key: '/events' },
      { label: '审计', key: '/audit' },
      { label: '安全', key: '/security' },
    ],
  },
  {
    type: 'group',
    key: 'g-system',
    label: '系统',
    children: [
      { label: '账号', key: '/accounts' },
      { label: '设置', key: '/settings' },
    ],
  },
])

// 绑定应用的账号权限一样，只是数据限定在自己的应用；安全体检是实例级的，不给
const menu = computed<MenuOption[]>(() => {
  if (!scoped.value) return fullMenu.value
  return fullMenu.value
    .map((g) =>
      g.type === 'group'
        ? { ...g, children: (g.children ?? []).filter((c) => c.key !== '/security' && c.key !== '/accounts') }
        : g,
    )
    .filter((g) => g.type !== 'group' || (g.children ?? []).length > 0)
})

function onMenu(key: string) {
  router.push(key)
}

async function logout() {
  try {
    // 新标签页里 CSRF 可能还没回填，先取一次再退，否则请求 403、服务端会话还在
    if (!getCSRF()) await tryRestore()
    await api.post('/admin/api/logout')
  } catch {
    // 会话已失效也继续走完本地清理
  }
  markSignedOut()
  sessionStorage.removeItem('devour_user')
  sessionStorage.removeItem('devour_csrf')
  router.push('/login')
}

const active = computed(() => router.currentRoute.value.path)
const title = computed(() => (router.currentRoute.value.meta.title as string | undefined) ?? '')

interface AppItem { id: number; code: string; name: string; active: number }
const apps = ref<AppItem[]>([])
const appId = ref<string>(getCurrentApp())
const appOptions = computed(() => apps.value.map((a) => ({ label: `${a.name}（${a.code}）`, value: String(a.id) })))

async function loadApps() {
  try {
    const list = await api.get<AppItem[]>('/admin/api/apps')
    apps.value = list
    if (!appId.value || !list.some(a => String(a.id) === appId.value)) {
      if (list.length) {
        appId.value = String(list[0].id)
        setCurrentApp(appId.value)
      }
    }
  } catch {
    // 未登录时忽略
  }
}

function switchApp(v: string | null) {
  if (!v || v === appId.value) return
  setCurrentApp(v)
  appId.value = v
  location.reload()
}

onMounted(loadApps)
</script>

<template>
  <PortalShell
    :menu="menu"
    :active="active"
    :title="title"
    :who="user"
    :initial="initial"
    :sider-width="232"
    :content-max="1360"
    @select="onMenu"
    @logout="logout"
  >
    <template #port>管理端</template>
    <template #header-extra>
      <n-select
        v-if="appOptions.length"
        :value="appId"
        :options="appOptions"
        size="small"
        style="width: 220px"
        :consistent-menu-width="false"
        @update:value="switchApp"
      />
    </template>
    <template #mobile-extra>
      <n-select
        v-if="appOptions.length"
        :value="appId"
        :options="appOptions"
        size="tiny"
        style="width: 118px"
        @update:value="switchApp"
      />
    </template>

    <router-view v-slot="{ Component }">
      <transition name="page" mode="out-in">
        <component :is="Component" />
      </transition>
    </router-view>
  </PortalShell>
  <ReauthDialog />
</template>
