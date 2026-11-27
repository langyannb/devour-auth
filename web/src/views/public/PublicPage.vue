<script setup lang="ts">
defineProps<{ title: string }>()
</script>

<template>
  <div class="pub">
    <header class="head">
      <a class="brand" href="/"><span class="mark">D</span>Devour<b>Auth</b></a>
      <span class="sep"></span>
      <span class="hint">{{ title }}</span>
      <a class="back" href="/">返回首页</a>
    </header>
    <main class="main">
      <slot />
    </main>
  </div>
</template>

<style scoped>
.pub {
  min-height: 100vh;
  background: radial-gradient(120% 80% at 50% 0%, #eef8f2 0%, #f6f8f7 60%, #f6f8f7 100%);
}
.head {
  position: sticky;
  top: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 24px;
  background: rgba(255, 255, 255, 0.86);
  backdrop-filter: blur(10px);
  border-bottom: 1px solid #e8eeeb;
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 700;
  font-size: 15px;
  letter-spacing: -0.01em;
  text-decoration: none;
  color: #12151c;
}
.brand b { color: #18a058; font-weight: 700; }
.brand .mark {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 800;
  color: #04140c;
  background: linear-gradient(135deg, #4ade9b, #18a058);
}
.sep { width: 1px; height: 14px; background: #dfe7e3; }
.hint { font-size: 13px; color: #6b7280; }
.back { margin-left: auto; font-size: 13px; color: #4a5568; text-decoration: none; }
.back:hover { color: #12151c; }
.main {
  max-width: 560px;
  margin: 28px auto 80px;
  padding: 28px 26px 30px;
  background: #fff;
  border: 1px solid #e8eeeb;
  border-radius: 16px;
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.04), 0 24px 48px -40px rgba(16, 24, 40, 0.5);
}
@media (max-width: 640px) {
  .main { margin: 16px 12px 48px; padding: 22px 18px 24px; }
  .head { padding: 10px 14px; }
}
</style>
