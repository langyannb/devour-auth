<script setup lang="ts">
import { computed } from 'vue'

interface Series {
  name: string
  color: string
  values: (number | null)[]
}

const props = withDefaults(defineProps<{
  labels: string[]
  series: Series[]
  height?: number
  unit?: string
}>(), { height: 200, unit: '' })

const W = 640
const PAD_L = 34
const PAD_B = 8
const PAD_T = 8

const max = computed(() => {
  const all = props.series.flatMap((s) => s.values.filter((v): v is number => v !== null))
  return Math.max(1, ...all)
})

const x = (i: number) => {
  const n = props.labels.length
  if (n <= 1) return PAD_L
  return PAD_L + (i * (W - PAD_L)) / (n - 1)
}
const y = (v: number) => props.height - PAD_B - (v / max.value) * (props.height - PAD_B - PAD_T)

function path(values: (number | null)[]) {
  let d = ''
  let open = false
  values.forEach((v, i) => {
    if (v === null) {
      open = false
      return
    }
    d += `${open ? 'L' : 'M'}${x(i).toFixed(1)},${y(v).toFixed(1)}`
    open = true
  })
  return d
}

function area(values: (number | null)[]) {
  const pts = values.map((v, i) => ({ v, i })).filter((p) => p.v !== null)
  if (pts.length < 2) return ''
  const line = pts.map((p, k) => `${k === 0 ? 'M' : 'L'}${x(p.i).toFixed(1)},${y(p.v as number).toFixed(1)}`).join(' ')
  return `${line} L${x(pts[pts.length - 1].i).toFixed(1)},${props.height - PAD_B} L${x(pts[0].i).toFixed(1)},${props.height - PAD_B} Z`
}

const gridValues = computed(() => Array.from(new Set([max.value, Math.ceil(max.value / 2), 0])))

// x 轴只标首、中、尾
const xLabels = computed(() => {
  const n = props.labels.length
  if (n === 0) return []
  if (n === 1) return [props.labels[0]]
  return [props.labels[0], props.labels[Math.floor((n - 1) / 2)], props.labels[n - 1]]
})
</script>

<template>
  <div class="chart-wrap">
    <div class="plot" :style="{ height: height + 'px' }">
      <div class="yaxis">
        <span v-for="g in gridValues" :key="g">{{ g }}</span>
      </div>
      <svg :viewBox="`0 0 ${W} ${height}`" preserveAspectRatio="none" class="chart">
        <defs>
          <linearGradient v-for="(s, si) in series" :id="`g-${si}`" :key="s.name" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" :stop-color="s.color" stop-opacity="0.20" />
            <stop offset="100%" :stop-color="s.color" stop-opacity="0" />
          </linearGradient>
        </defs>
        <line
          v-for="g in gridValues"
          :key="g"
          :x1="PAD_L"
          :x2="W"
          :y1="y(g)"
          :y2="y(g)"
          stroke="#eef1f0"
          stroke-width="1"
        />
        <g v-for="(s, si) in series" :key="s.name">
          <path :d="area(s.values)" :fill="`url(#g-${si})`" />
          <path :d="path(s.values)" fill="none" :stroke="s.color" stroke-width="2.2" stroke-linecap="round" />
        </g>
      </svg>
    </div>
    <div class="xaxis">
      <span v-for="(l, i) in xLabels" :key="i">{{ l }}</span>
    </div>
    <div class="legend">
      <span v-for="s in series" :key="s.name"><i :style="{ background: s.color }"></i>{{ s.name }}</span>
      <span v-if="unit" class="unit">单位：{{ unit }}</span>
    </div>
  </div>
</template>

<style scoped>
.chart-wrap { width: 100%; }
.plot { position: relative; padding-left: 34px; }
.yaxis {
  position: absolute;
  left: 0;
  top: 8px;
  bottom: 8px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  font-size: 10px;
  color: #98a2b3;
  text-align: right;
  width: 28px;
  font-variant-numeric: tabular-nums;
}
.chart { width: 100%; height: 100%; display: block; overflow: visible; }
.xaxis {
  display: flex;
  justify-content: space-between;
  font-size: 10px;
  color: #98a2b3;
  padding: 4px 0 0 34px;
  font-variant-numeric: tabular-nums;
}
.legend { display: flex; flex-wrap: wrap; gap: 14px; align-items: center; font-size: 12px; color: #6b7280; margin-top: 6px; }
.legend i { display: inline-block; width: 8px; height: 8px; border-radius: 2px; margin-right: 5px; }
.legend .unit { margin-left: auto; color: #98a2b3; }
</style>
