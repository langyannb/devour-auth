<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import type { MenuOption } from 'naive-ui'
import PortalShell from '../../components/PortalShell.vue'
import { agentApi, getCSRF, markSignedOut, tryRestoreAgent } from '../../api'

const router = useRouter()
const rawWho = sessionStorage.getItem('devour_agent_name') ?? ''
const who = rawWho.split(' · ')[0]
const initial = (who || 'A').slice(0, 1).toUpperCase()
const active = computed(() => router.currentRoute.value.path)
const title = computed(() => (router.currentRoute.value.meta.title as string | undefined) ?? '')

const menu: MenuOption[] = [
  { label: '概览', key: '/dashboard' },
  { label: '数据', key: '/analytics' },
  { label: '我的卡密', key: '/cards' },
  { label: '出卡', key: '/issue' },
  { label: '下级', key: '/subagents' },
  { label: '账单与流水', key: '/billing' },
  { label: '设置', key: '/settings' },
]

function onMenu(key: string) {
  router.push(key)
}

async function logout() {
  try {
    if (!getCSRF('agent')) await tryRestoreAgent()
    await agentApi.post('/agent/api/logout')
  } catch {
    // 已失效也继续走完本地清理
  }
  markSignedOut()
  sessionStorage.removeItem('devour_agent_name')
  sessionStorage.removeItem('devour_agent_csrf')
  router.push('/login')
}
</script>

<template>
  <PortalShell
    :menu="menu"
    :active="active"
    :title="title"
    :who="rawWho || who"
    :initial="initial"
    :sider-width="220"
    :content-max="1180"
    @select="onMenu"
    @logout="logout"
  >
    <template #port>代理端</template>
    <router-view v-slot="{ Component }">
      <transition name="page" mode="out-in">
        <component :is="Component" />
      </transition>
    </router-view>
  </PortalShell>
</template>
