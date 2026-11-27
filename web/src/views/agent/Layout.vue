<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NButton, NDrawer, NDrawerContent, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NMenu, type MenuOption } from 'naive-ui'
import { agentApi, agentAuthed } from '../../api'
import { useMobile } from '../../useMobile'

const router = useRouter()
const isMobile = useMobile()
const drawer = ref(false)
const who = sessionStorage.getItem('devour_agent_name') ?? ''

const menu = computed<MenuOption[]>(() => [
  { label: '我的卡密', key: '/agent/cards' },
  { label: '出卡', key: '/agent/issue' },
  { label: '账单与流水', key: '/agent/billing' },
])

function onMenu(key: string) {
  drawer.value = false
  router.push(key)
}

async function logout() {
  try {
    await agentApi.post('/agent/api/logout')
  } catch {
    // 已失效也回登录页
  }
  agentAuthed.value = false
  sessionStorage.removeItem('devour_agent_name')
  router.push('/agent/login')
}

const active = computed(() => router.currentRoute.value.path)
</script>

<template>
  <n-drawer v-if="isMobile" v-model:show="drawer" placement="left" :width="220">
    <n-drawer-content closable>
      <n-menu :options="menu" :value="active" @update:value="onMenu" />
    </n-drawer-content>
  </n-drawer>

  <n-layout v-if="!isMobile" has-sider style="height: 100vh">
    <n-layout-sider bordered width="180" content-style="padding-top: 12px" style="background: #f2f7f4">
      <div style="padding: 0 20px 16px; font-weight: 600; color: #0b6b4f">代理端</div>
      <n-menu :options="menu" :value="active" @update:value="onMenu" />
    </n-layout-sider>
    <n-layout>
      <n-layout-header bordered style="height: 52px; display: flex; align-items: center; justify-content: flex-end; gap: 12px; padding: 0 20px">
        <span style="color: #666">{{ who }}</span>
        <n-button size="small" @click="logout">退出</n-button>
      </n-layout-header>
      <n-layout-content content-style="padding: 20px">
        <router-view />
      </n-layout-content>
    </n-layout>
  </n-layout>

  <div v-else style="min-height: 100vh; display: flex; flex-direction: column; background: #f2f7f4">
    <header style="position: sticky; top: 0; z-index: 10; height: 52px; display: flex; align-items: center; gap: 8px; padding: 0 12px; background: #fff; border-bottom: 1px solid #e3ece7">
      <n-button text style="font-size: 20px; line-height: 1" @click="drawer = true">☰</n-button>
      <span style="font-weight: 600; color: #0b6b4f">代理端</span>
      <span style="flex: 1"></span>
      <n-button text size="small" @click="logout">退出</n-button>
    </header>
    <main style="flex: 1; padding: 12px; overflow-x: auto">
      <router-view />
    </main>
  </div>
</template>
