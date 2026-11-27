<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NButton, NDrawer, NDrawerContent, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider,
  NMenu, type MenuOption,
} from 'naive-ui'
import { agentApi, getCSRF, markSignedOut, tryRestoreAgent } from '../../api'
import { useMobile } from '../../useMobile'

const router = useRouter()
const isMobile = useMobile()
const drawer = ref(false)
const rawWho = sessionStorage.getItem('devour_agent_name') ?? ''
const who = rawWho.split(' · ')[0]
const initial = (who || 'A').slice(0, 1).toUpperCase()

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
  drawer.value = false
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

const active = computed(() => router.currentRoute.value.path)
const title = computed(() => (router.currentRoute.value.meta.title as string | undefined) ?? '')
</script>

<template>
  <n-drawer v-if="isMobile" v-model:show="drawer" placement="left" :width="252">
    <n-drawer-content closable body-content-style="padding: 0">
      <div class="brand">
        <span class="mark">D</span>
        <span class="brand-text"><b>Devour Auth</b><i>代理端</i></span>
      </div>
      <n-menu :options="menu" :value="active" @update:value="onMenu" />
    </n-drawer-content>
  </n-drawer>

  <n-layout v-if="!isMobile" has-sider style="height: 100vh">
    <n-layout-sider bordered width="220" :content-style="{ display: 'flex', flexDirection: 'column', height: '100%' }">
      <div class="brand">
        <span class="mark">D</span>
        <span class="brand-text"><b>Devour Auth</b><i>代理端</i></span>
      </div>
      <div class="menu-wrap">
        <n-menu :options="menu" :value="active" @update:value="onMenu" />
      </div>
      <div class="sider-foot">
        <span class="avatar">{{ initial }}</span>
        <span class="who">{{ rawWho || who }}</span>
        <n-button text size="tiny" @click="logout">退出</n-button>
      </div>
    </n-layout-sider>

    <n-layout class="shell">
      <n-layout-header bordered class="header">
        <span class="page-title">{{ title }}</span>
        <span class="who">{{ who }}</span>
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

  <div v-else class="m-shell">
    <header class="m-header">
      <button class="burger" aria-label="菜单" @click="drawer = true">☰</button>
      <span class="page-title">{{ title }}</span>
      <span style="flex: 1"></span>
      <span class="who">{{ who }}</span>
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
.sider-foot .who { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 0 24px;
}
.page-title { font-size: 16px; font-weight: 600; letter-spacing: -0.01em; }

.shell { background: #f4f8f6; }
.page-wrap { max-width: 1000px; margin: 0 auto; }

.page-enter-active,
.page-leave-active { transition: opacity 0.16s ease, transform 0.16s ease; }
.page-enter-from { opacity: 0; transform: translateY(6px); }
.page-leave-to { opacity: 0; transform: translateY(-4px); }

.m-shell { min-height: 100vh; display: flex; flex-direction: column; background: #f4f8f6; }
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
