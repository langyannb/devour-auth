<script setup lang="ts">
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NMenu, type MenuOption } from 'naive-ui'
import { api, authed } from '../api'

const router = useRouter()
const user = sessionStorage.getItem('devour_user') ?? ''

const menu = computed<MenuOption[]>(() => [
  { label: '卡密', key: '/cards' },
  { label: '发卡', key: '/issue' },
  { label: '卡类型', key: '/policies' },
  { label: '审计', key: '/audit' },
])

function onMenu(key: string) {
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
  <n-layout has-sider style="height: 100vh">
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
</template>
