<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton, NDrawer, NDrawerContent, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider,
  NMenu, NSelect, type MenuOption,
} from 'naive-ui'
import { api, authed, getCurrentApp, setCurrentApp } from '../api'
import { useMobile } from '../useMobile'
import ReauthDialog from './ReauthDialog.vue'

const router = useRouter()
const isMobile = useMobile()
const drawer = ref(false)
const rawUser = sessionStorage.getItem('devour_user') ?? ''
const user = rawUser.split(' · ')[0]
const initial = (user || 'A').slice(0, 1).toUpperCase()

const menu = computed<MenuOption[]>(() => [
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
      { label: '策略', key: '/policies' },
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

function onMenu(key: string) {
  drawer.value = false
  router.push(key)
}

async function logout() {
  try {
    await api.post('/admin/api/logout')
  } catch {
    // 会话已失效也要回到登录页
  }
  authed.value = false
  sessionStorage.removeItem('devour_user')
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
  <n-drawer v-if="isMobile" v-model:show="drawer" placement="left" :width="264">
    <n-drawer-content closable body-content-style="padding: 0">
      <div class="brand">
        <span class="mark">D</span>
        <span class="brand-text"><b>Devour Auth</b><i>管理端</i></span>
      </div>
      <n-menu :options="menu" :value="active" @update:value="onMenu" />
    </n-drawer-content>
  </n-drawer>

  <!-- 桌面：固定侧栏 -->
  <n-layout v-if="!isMobile" has-sider style="height: 100vh">
    <n-layout-sider bordered width="236" :content-style="{ display: 'flex', flexDirection: 'column', height: '100%' }">
      <div class="brand">
        <span class="mark">D</span>
        <span class="brand-text"><b>Devour Auth</b><i>管理端</i></span>
      </div>
      <div class="menu-wrap">
        <n-menu :options="menu" :value="active" @update:value="onMenu" />
      </div>
      <div class="sider-foot">
        <span class="avatar">{{ initial }}</span>
        <span class="who">{{ user }}</span>
        <n-button text size="tiny" @click="logout">退出</n-button>
      </div>
    </n-layout-sider>

    <n-layout class="shell">
      <n-layout-header bordered class="header">
        <span class="page-title">{{ title }}</span>
        <span class="header-right">
          <n-select
            v-if="appOptions.length"
            :value="appId"
            :options="appOptions"
            size="small"
            style="width: 220px"
            :consistent-menu-width="false"
            @update:value="switchApp"
          />
          <span class="who">{{ user }}</span>
        </span>
      </n-layout-header>
      <n-layout-content content-style="padding: 20px 24px 40px">
        <div class="page-wrap">
          <router-view v-slot="{ Component }">
            <transition name="page" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </n-layout-content>
    </n-layout>
  </n-layout>

  <!-- 移动端：顶栏 + 抽屉 -->
  <div v-else class="m-shell">
    <header class="m-header">
      <button class="burger" aria-label="菜单" @click="drawer = true">☰</button>
      <span class="page-title">{{ title }}</span>
      <span style="flex: 1"></span>
      <n-select
        v-if="appOptions.length"
        :value="appId"
        :options="appOptions"
        size="tiny"
        style="width: 118px"
        @update:value="switchApp"
      />
      <n-button text size="small" @click="logout">退出</n-button>
    </header>
    <main class="m-main">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </main>
  </div>
  <ReauthDialog />
</template>

<style scoped>
.brand {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 16px 18px 14px;
  border-bottom: 1px solid #eef1f0;
}
.mark {
  width: 30px;
  height: 30px;
  border-radius: 9px;
  display: grid;
  place-items: center;
  font-weight: 800;
  font-size: 15px;
  color: #fff;
  background: linear-gradient(135deg, #18a058, #0d7a41);
  box-shadow: 0 6px 14px -8px rgba(24, 160, 88, 0.9);
}
.brand-text { display: flex; flex-direction: column; line-height: 1.2; }
.brand-text b { font-size: 14px; letter-spacing: -0.01em; }
.brand-text i { font-size: 11px; font-style: normal; color: #8b93a7; }

.menu-wrap { flex: 1; overflow-y: auto; padding: 10px 0 16px; }
.menu-wrap :deep(.n-menu-item-group-title),
:deep(.n-menu-item-group-title) {
  color: #98a2b3;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding-top: 10px;
}

.sider-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 16px;
  border-top: 1px solid #eef1f0;
}
.avatar {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  display: grid;
  place-items: center;
  font-size: 11px;
  font-weight: 700;
  color: #0d7a41;
  background: #e7f5ed;
}
.who { font-size: 12px; color: #4a5568; }
.sider-foot .who { flex: 1; }

.header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 0 24px;
}
.page-title { font-size: 16px; font-weight: 600; letter-spacing: -0.01em; }
.header-right { display: flex; align-items: center; gap: 14px; }

.shell { background: #f6f8f7; }
.page-wrap { max-width: 1360px; margin: 0 auto; }

.page-enter-active,
.page-leave-active { transition: opacity 0.16s ease, transform 0.16s ease; }
.page-enter-from { opacity: 0; transform: translateY(6px); }
.page-leave-to { opacity: 0; transform: translateY(-4px); }

.m-shell { min-height: 100vh; display: flex; flex-direction: column; background: #f6f8f7; }
.m-header {
  position: sticky;
  top: 0;
  z-index: 10;
  height: 52px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  background: #fff;
  border-bottom: 1px solid #eef1f0;
}
.burger {
  border: none;
  background: none;
  font-size: 18px;
  line-height: 1;
  padding: 4px;
  cursor: pointer;
  color: #12151c;
}
.m-main { flex: 1; padding: 12px; overflow-x: auto; }

@media (prefers-reduced-motion: reduce) {
  .page-enter-active,
  .page-leave-active { transition: none; }
}
</style>
