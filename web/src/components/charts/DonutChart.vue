<script setup lang="ts">
import { computed } from 'vue'

interface Item {
  label: string
  value: number
  color: string
}

const props = defineProps<{ items: Item[] }>()

const R = 42
const C = 2 * Math.PI * R
const total = computed(() => props.items.reduce((n, i) => n + i.value, 0))

const segments = computed(() => {
  let acc = 0
  return props.items.map((i) => {
    const part = total.value > 0 ? i.value / total.value : 0
    const seg = { ...i, dash: part * C, offset: acc, pct: Math.round(part * 100) }
    acc += part * C
    return seg
  })
})
</script>

<template>
  <div class="donut">
    <svg viewBox="0 0 120 120" class="ring">
      <circle cx="60" cy="60" :r="R" fill="none" stroke="#eef1f0" stroke-width="14" />
      <circle
        v-for="(s, i) in segments"
        :key="i"
        cx="60"
        cy="60"
        :r="R"
        fill="none"
        :stroke="s.color"
        stroke-width="14"
        :stroke-dasharray="`${s.dash} ${C - s.dash}`"
        :stroke-dashoffset="-s.offset"
        transform="rotate(-90 60 60)"
        class="seg"
      >
        <title>{{ s.label }}：{{ s.value }}（{{ s.pct }}%）</title>
      </circle>
      <text x="60" y="56" text-anchor="middle" class="total">{{ total }}</text>
      <text x="60" y="72" text-anchor="middle" class="cap">总数</text>
    </svg>
    <ul class="legend">
      <li v-for="(s, i) in segments" :key="i">
        <i :style="{ background: s.color }"></i>
        <span class="label">{{ s.label }}</span>
        <span class="val">{{ s.value }}</span>
        <span class="pct">{{ s.pct }}%</span>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.donut { display: flex; align-items: center; gap: 18px; flex-wrap: wrap; }
.ring { width: 132px; height: 132px; flex: 0 0 auto; }
.seg { transition: stroke-dasharray 0.4s ease; }
.total { font-size: 16px; font-weight: 700; fill: #12151c; }
.cap { font-size: 9px; fill: #98a2b3; }
.legend { flex: 1; min-width: 140px; list-style: none; margin: 0; padding: 0; }
.legend li { display: flex; align-items: center; gap: 7px; font-size: 13px; padding: 4px 0; }
.legend i { width: 8px; height: 8px; border-radius: 2px; flex: 0 0 auto; }
.legend .label { color: #475467; }
.legend .val { margin-left: auto; font-weight: 600; font-variant-numeric: tabular-nums; }
.legend .pct { color: #98a2b3; font-size: 12px; width: 38px; text-align: right; }
</style>
