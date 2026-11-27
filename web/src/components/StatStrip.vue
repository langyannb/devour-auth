<script setup lang="ts">
defineProps<{ items: { k: string; v: string | number; sub?: string }[] }>()
</script>

<template>
  <div class="stats">
    <div v-for="(s, i) in items" :key="i" class="stat">
      <span class="k">{{ s.k }}</span>
      <span class="v">{{ s.v }}</span>
      <span v-if="s.sub" class="sub">{{ s.sub }}</span>
    </div>
  </div>
</template>

<style scoped>
.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}
.stat {
  background: #fff;
  border: 1px solid #e8edeb;
  border-radius: 12px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.k { font-size: 12px; color: #8b93a7; }
.v { font-size: 22px; font-weight: 700; letter-spacing: -0.02em; font-variant-numeric: tabular-nums; }
.sub { font-size: 11px; color: #98a2b3; }

@media (max-width: 560px) {
  .stats { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
  .v { font-size: 18px; }
}
</style>
