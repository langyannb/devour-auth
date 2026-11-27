<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton, NDrawer, NDrawerContent, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NMenu,
  type MenuOption,
} from 'naive-ui'
import { api, authed } from '../api'
import { useMobile } from '../useMobile'

const router = useRouter()
const isMobile = useMobile()
const drawer = ref(false)
const user = sessionStorage.getItem('devour_user') ?? ''

const menu = computed<MenuOption[]>(() => [
  { label: '卡密', key: '/cards' },
  { label: '发卡', key: '/issue' },
  { label: '卡类型', key: '/policies' },
  { label: '审计', key: '/audit' },
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
      <n-layout-header bordered style="height: 52px; display: flex; align-items: center; justify-content: flex-end; gap: 12px; padding: 0 20px">
        <span style="color: #666">{{ user }}</span>
        <n-button size="small" @click="logout">退出</n-button>
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
      <n-button text size="small" @click="logout">退出</n-button>
    </header>
    <main style="flex: 1; padding: 12px; overflow-x: auto">
      <router-view />
    </main>
  </div>
</template>
