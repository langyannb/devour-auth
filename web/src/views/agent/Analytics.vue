<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NRadioButton, NRadioGroup } from 'naive-ui'
import BarList from '../../components/charts/BarList.vue'
import DonutChart from '../../components/charts/DonutChart.vue'
import StatStrip from '../../components/StatStrip.vue'
import TrendChart from '../../components/charts/TrendChart.vue'
import { agentApi } from '../../api'
import { formatTs, formatYuan, kindLabels, statusLabels } from '../../utils'

interface Point { day: string; issued: number; activated: number }
interface Analytics {
  days: number
  trend: Point[]
  by_kind: { key: string; count: number }[]
  by_status: { key: string; count: number }[]
  active_devices: number
  quota_cents: number
  level: number
  server_time: number
}

const palette = ['#18a058', '#06b6d4', '#6366f1', '#f59e0b', '#ef4444', '#94a3b8']
const statusColor: Record<string, string> = {
  unused: '#94a3b8',
  active: '#18a058',
  expired: '#f59e0b',
  exhausted: '#f97316',
  banned: '#ef4444',
  recycled: '#64748b',
}

const data = ref<Analytics | null>(null)
const days = ref(30)
const err = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  err.value = ''
  try {
    data.value = await agentApi.get<Analytics>(`/agent/api/analytics?days=${days.value}`)
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

const series = computed(() => {
  const byDay = new Map((data.value?.trend ?? []).map((t) => [t.day, t]))
  const out: Point[] = []
  const today = new Date()
  for (let i = days.value - 1; i >= 0; i--) {
    const d = new Date(today.getFullYear(), today.getMonth(), today.getDate() - i)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    out.push(byDay.get(key) ?? { day: key, issued: 0, activated: 0 })
  }
  return out
})

const labels = computed(() => series.value.map((t) => t.day.slice(5)))
const trendSeries = computed(() => [
  { name: '出卡', color: '#18a058', values: series.value.map((t) => t.issued) },
  { name: '激活', color: '#6366f1', values: series.value.map((t) => t.activated) },
])

const totalIssued = computed(() => series.value.reduce((n, t) => n + t.issued, 0))
const totalActivated = computed(() => series.value.reduce((n, t) => n + t.activated, 0))
const cardsTotal = computed(() => (data.value?.by_status ?? []).reduce((n, s) => n + s.count, 0))

const kindItems = computed(() =>
  (data.value?.by_kind ?? []).map((k, i) => ({
    label: kindLabels[k.key] ?? k.key,
    value: k.count,
    color: palette[i % palette.length],
  })),
)
const statusItems = computed(() =>
  (data.value?.by_status ?? []).map((s) => ({
    label: statusLabels[s.key] ?? s.key,
    value: s.count,
    color: statusColor[s.key] ?? '#94a3b8',
  })),
)

const quotaText = computed(() => {
  const q = data.value?.quota_cents
  if (q === undefined) return '—'
  return q < 0 ? '不限' : `${formatYuan(q)} 元`
})

const stats = computed(() => [
  { k: '可用余额', v: quotaText.value, sub: '出卡从这里扣' },
  { k: `${days.value} 天出卡`, v: totalIssued.value, sub: '张（含下级）' },
  { k: `${days.value} 天激活`, v: totalActivated.value, sub: '张' },
  { k: '卡密总数', v: cardsTotal.value, sub: '你与下级的全部卡' },
  { k: `${days.value} 天活跃设备`, v: data.value?.active_devices ?? '—', sub: '登录过的设备' },
])

onMounted(load)
</script>

<template>
  <div class="page">
    <n-alert v-if="err" type="error" closable style="margin-bottom: 12px" @close="err = ''">{{ err }}</n-alert>

    <div class="toolbar">
      <span class="hint">{{ data ? `数据时间 ${formatTs(data.server_time)}` : '加载中…' }}</span>
      <span class="spacer"></span>
      <n-radio-group v-model:value="days" size="small" @update:value="load">
        <n-radio-button :value="7">7 天</n-radio-button>
        <n-radio-button :value="30">30 天</n-radio-button>
        <n-radio-button :value="90">90 天</n-radio-button>
      </n-radio-group>
      <n-button size="small" :loading="loading" @click="load">刷新</n-button>
    </div>

    <StatStrip :items="stats" />

    <div class="grid">
      <n-card size="small" class="span-8">
        <template #header><span class="card-title">出卡与激活</span></template>
        <div v-if="totalIssued + totalActivated === 0" class="empty">这段时间还没有出卡记录。</div>
        <TrendChart v-else :labels="labels" :series="trendSeries" :height="220" />
      </n-card>

      <n-card size="small" class="span-4">
        <template #header><span class="card-title">卡类型分布</span></template>
        <DonutChart v-if="kindItems.length" :items="kindItems" />
        <div v-else class="empty">还没有卡密，去「出卡」生成第一批。</div>
      </n-card>

      <n-card size="small" class="span-12">
        <template #header><span class="card-title">卡密状态</span></template>
        <BarList :items="statusItems" unit=" 张" />
      </n-card>
    </div>
  </div>
</template>

<style scoped>
.toolbar { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.spacer { flex: 1; }
.hint { font-size: 12px; color: #98a2b3; }

.grid { display: grid; grid-template-columns: repeat(12, minmax(0, 1fr)); gap: 12px; }
.span-12 { grid-column: span 12; }
.span-8 { grid-column: span 8; }
.span-4 { grid-column: span 4; }
.card-title { font-size: 14px; font-weight: 600; }
.empty { color: #98a2b3; font-size: 13px; padding: 24px 0; }

@media (max-width: 760px) {
  .span-8, .span-4 { grid-column: span 12; }
}
</style>
