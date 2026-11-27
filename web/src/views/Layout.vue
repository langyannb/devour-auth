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
const user = sessionStorage.getItem('devour_user') ?? ''

const menu = computed<MenuOption[]>(() => [
  { label: '概览', key: '/dashboard' },
  { label: '卡密', key: '/cards' },
  { label: '发卡', key: '/issue' },
  { label: '设备', key: '/devices' },
  { label: '在线', key: '/sessions' },
  { label: '黑名单', key: '/blacklist' },
  { label: '卡类型', key: '/policies' },
  { label: '权限', key: '/entitlements' },
  { label: '代理', key: '/agents' },
  { label: '价格模板', key: '/plans' },
  { label: '财务', key: '/finance' },
  { label: '运营', key: '/ops' },
  { label: '对接', key: '/open' },
  { label: '回调', key: '/webhooks' },
  { label: '日志', key: '/events' },
  { label: '审计', key: '/audit' },
  { label: '安全', key: '/security' },
  { label: '账号', key: '/accounts' },
  { label: '应用', key: '/apps' },
  { label: '设置', key: '/settings' },
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

interface AppItem { id: number; code: string; name: string; active: number }
const apps = ref<AppItem[]>([])
const appId = ref<string>(getCurrentApp())

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

function switchApp(v: string) {
  setCurrentApp(v)
  location.reload()
}

onMounted(loadApps)
</script>

<template>
  <n-drawer v-if="isMobile" v-model:show="drawer" placement="left" :width="220">
    <n-drawer-content closable>
      <n-menu :options="menu" :value="active" @update:value="onMenu" />
    </n-drawer-content>
  </n-drawer>

  <!-- 桌面：固定侧栏 -->
  <n-layout v-if="!isMobile" has-sider style="height: 100vh">
    <n-layout-sider bordered width="180" content-style="padding-top: 12px">
      <div style="padding: 0 20px 16px; font-weight: 600">Devour Auth</div>
      <n-menu :options="menu" :value="active" @update:value="onMenu" />
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered style="height: 52px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 0 20px">
        <n-select
          v-model:value="appId"
          :options="apps.map(a => ({ label: `${a.name}（${a.code}）`, value: String(a.id) }))"
          size="small"
          style="width: 200px"
          @update:value="switchApp"
        />
        <span style="display:flex;align-items:center;gap:12px">
        <span style="color: #666">{{ user }}</span>
        <n-button size="small" @click="logout">退出</n-button>
        </span>
      </n-layout-header>
      <n-layout-content content-style="padding: 20px">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>

  <!-- 移动端：顶栏 + 抽屉 -->
  <div v-else style="min-height: 100vh; display: flex; flex-direction: column; background: #f5f6f8">
    <header style="position: sticky; top: 0; z-index: 10; height: 52px; display: flex; align-items: center; gap: 8px; padding: 0 12px; background: #fff; border-bottom: 1px solid #eee">
      <n-button text style="font-size: 20px; line-height: 1" @click="drawer = true">☰</n-button>
      <span style="font-weight: 600">Devour Auth</span>
      <span style="flex: 1"></span>
      <n-select
        v-model:value="appId"
        :options="apps.map(a => ({ label: a.name, value: String(a.id) }))"
        size="tiny"
        style="width: 110px"
        @update:value="switchApp"
      />
      <n-button text size="small" @click="logout">退出</n-button>
    </header>
    <main style="flex: 1; padding: 12px; overflow-x: auto">
      <router-view />
    </main>
  </div>
  <ReauthDialog />
</template>
