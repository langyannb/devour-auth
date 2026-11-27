<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NConfigProvider, dateZhCN, zhCN } from 'naive-ui'

const router = useRouter()
const ready = ref(false)

// 路由守卫里要校验登录态，首次进入可能等一个 API。
// 这段时间显示品牌骨架，避免白屏和干巴巴的「加载中」。
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
    <div class="boot-brand">
      <span class="mark">D</span>
      <span class="name">Devour<b>Auth</b></span>
    </div>
    <div class="boot-bar"><i></i></div>
    <p class="boot-tip">正在校验会话…</p>
  </div>
  <n-config-provider v-else :locale="zhCN" :date-locale="dateZhCN">
    <router-view />
  </n-config-provider>
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
  gap: 18px;
  background: radial-gradient(120% 90% at 50% 0%, #0f2430 0%, #0a121a 55%, #07090d 100%);
}

.boot-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  animation: boot-rise 0.5s cubic-bezier(0.16, 1, 0.3, 1) both;
}

.boot-brand .mark {
  width: 34px;
  height: 34px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  font-size: 16px;
  font-weight: 800;
  color: #04140c;
  background: linear-gradient(135deg, #4ade9b, #18a058);
  box-shadow: 0 10px 24px -12px rgba(24, 160, 88, 0.9);
  animation: boot-pulse 1.6s ease-in-out infinite;
}

.boot-brand .name {
  font-size: 17px;
  font-weight: 700;
  letter-spacing: -0.02em;
  color: #eef4f0;
}
.boot-brand .name b { color: #4ade9b; font-weight: 700; }

.boot-bar {
  width: 132px;
  height: 3px;
  border-radius: 99px;
  background: rgba(255, 255, 255, 0.08);
  overflow: hidden;
  animation: boot-rise 0.5s 0.08s cubic-bezier(0.16, 1, 0.3, 1) both;
}
.boot-bar i {
  display: block;
  width: 40%;
  height: 100%;
  border-radius: 99px;
  background: linear-gradient(90deg, #18a058, #4ade9b);
  animation: boot-run 1.1s ease-in-out infinite;
}

.boot-tip {
  margin: 0;
  font-size: 12px;
  color: #7f918a;
  letter-spacing: 0.02em;
  animation: boot-rise 0.5s 0.16s cubic-bezier(0.16, 1, 0.3, 1) both;
}

@keyframes boot-rise {
  from { opacity: 0; transform: translateY(8px); }
}
@keyframes boot-run {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(250%); }
}
@keyframes boot-pulse {
  50% { transform: scale(1.05); box-shadow: 0 12px 28px -10px rgba(24, 160, 88, 0.7); }
}

/* 路由切换动效：管理端与代理端共用 */
.page-enter-active,
.page-leave-active { transition: opacity 0.16s ease, transform 0.16s ease; }
.page-enter-from { opacity: 0; transform: translateY(6px); }
.page-leave-to { opacity: 0; transform: translateY(-4px); }

@media (prefers-reduced-motion: reduce) {
  .page-enter-active,
  .page-leave-active { transition: none; }
  .boot-brand, .boot-bar, .boot-tip { animation: none; }
  .boot-brand .mark { animation: none; }
  .boot-bar i { animation: none; width: 100%; }
}
</style>
