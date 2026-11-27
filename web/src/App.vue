<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const ready = ref(false)

// 路由守卫里要校验登录态，首次进入可能等一个 API。
// 这段时间显示骨架，避免 Vue 挂载后骨架被替换成白屏。
onMounted(async () => {
  try {
    await router.isReady()
  } finally {
    ready.value = true
  }
})
</script>

<template>
  <div v-if="!ready" class="boot">
    <div class="boot-spin"></div>
    <div>加载中…</div>
  </div>
  <router-view v-else />
</template>

<style>
html,
body,
#app {
  margin: 0;
  height: 100%;
  font-family: system-ui, -apple-system, 'PingFang SC', 'Microsoft YaHei', sans-serif;
}

.boot {
  height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 14px;
  color: #6b7280;
  font-size: 14px;
  background: #f7f8fb;
}

.boot-spin {
  width: 26px;
  height: 26px;
  border: 3px solid #e5e7eb;
  border-top-color: #4f46e5;
  border-radius: 50%;
  animation: boot-rotate 0.8s linear infinite;
}

@keyframes boot-rotate {
  to {
    transform: rotate(360deg);
  }
}
</style>
