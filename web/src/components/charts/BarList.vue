<script setup lang="ts">
import { computed } from 'vue'

interface Item {
  label: string
  value: number
  hint?: string
  color?: string
}

const props = defineProps<{ items: Item[]; unit?: string }>()

const max = computed(() => Math.max(1, ...props.items.map((i) => i.value)))

function pct(v: number) {
  return Math.max(2, Math.round((v / max.value) * 100))
}
</script>

<template>
  <div class="bars">
    <div v-if="items.length === 0" class="empty">暂无数据</div>
    <div v-for="(i, idx) in items" :key="idx" class="row">
      <div class="head">
        <span class="label">{{ i.label }}</span>
        <span class="val">{{ i.value }}<em v-if="unit">{{ unit }}</em><small v-if="i.hint"> · {{ i.hint }}</small></span>
      </div>
      <div class="track">
        <div class="fill" :style="{ width: pct(i.value) + '%', background: i.color ?? '#18a058' }"></div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.bars { display: flex; flex-direction: column; gap: 12px; }
.empty { color: #98a2b3; font-size: 13px; }
.row .head { display: flex; justify-content: space-between; gap: 12px; font-size: 13px; margin-bottom: 5px; }
.label { color: #475467; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.val { font-weight: 600; font-variant-numeric: tabular-nums; flex: 0 0 auto; }
.val em { font-style: normal; font-size: 11px; color: #8b93a7; margin-left: 2px; }
.val small { font-weight: 400; color: #98a2b3; }
.track { height: 8px; border-radius: 99px; background: #eef1f0; overflow: hidden; }
.fill { height: 100%; border-radius: 99px; transition: width 0.45s ease; }
</style>
