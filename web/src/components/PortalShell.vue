<script setup lang="ts">
import { ref } from 'vue'
import {
  NButton, NDrawer, NDrawerContent, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider,
  NMenu, type MenuOption,
} from 'naive-ui'
import { useMobile } from '../useMobile'

// 管理端与代理端共用的外壳：品牌区、菜单、页脚用户、页头标题、移动端顶栏。
withDefaults(defineProps<{
  menu: MenuOption[]
  active: string
  title: string
  who: string
  initial: string
  siderWidth?: number
  contentMax?: number
}>(), { siderWidth: 232, contentMax: 1000 })

const emit = defineEmits<{ select: [key: string]; logout: [] }>()

const isMobile = useMobile()
const drawer = ref(false)

function onMenu(key: string) {
  drawer.value = false
  emit('select', key)
}
</script>

<template>
  <n-drawer v-if="isMobile" v-model:show="drawer" placement="left" :width="siderWidth + 16">
    <n-drawer-content closable body-content-style="padding: 0">
      <div class="brand">
        <span class="mark">D</span>
        <span class="brand-text"><b>Devour Auth</b><i><slot name="port" /></i></span>
      </div>
      <n-menu :options="menu" :value="active" @update:value="onMenu" />
    </n-drawer-content>
  </n-drawer>

  <n-layout v-if="!isMobile" has-sider style="height: 100vh">
    <n-layout-sider bordered :width="siderWidth" :content-style="{ display: 'flex', flexDirection: 'column', height: '100%' }">
      <div class="brand">
        <span class="mark">D</span>
        <span class="brand-text"><b>Devour Auth</b><i><slot name="port">管理端</slot></i></span>
      </div>
      <div class="menu-wrap">
        <n-menu :options="menu" :value="active" @update:value="onMenu" />
      </div>
      <div class="sider-foot">
        <span class="avatar">{{ initial }}</span>
        <span class="who">{{ who }}</span>
        <n-button text size="tiny" @click="emit('logout')">退出</n-button>
      </div>
    </n-layout-sider>

    <n-layout class="shell">
      <n-layout-header bordered class="header">
        <span class="page-title">{{ title }}</span>
        <span class="header-right">
          <slot name="header-extra" />
          <span class="who">{{ who }}</span>
        </span>
      </n-layout-header>
      <n-layout-content content-style="padding: 20px 24px 40px">
        <div class="page-wrap" :style="{ maxWidth: contentMax + 'px' }">
          <slot />
        </div>
      </n-layout-content>
    </n-layout>
  </n-layout>

  <div v-else class="m-shell">
    <header class="m-header">
      <button class="burger" aria-label="菜单" @click="drawer = true">☰</button>
      <span class="page-title">{{ title }}</span>
      <span style="flex: 1"></span>
      <slot name="mobile-extra" />
      <span class="who">{{ who }}</span>
      <n-button text size="small" @click="emit('logout')">退出</n-button>
    </header>
    <main class="m-main">
      <slot />
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
.header-right { display: flex; align-items: center; gap: 14px; }

.shell { background: #f6f8f7; }

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
</style>
