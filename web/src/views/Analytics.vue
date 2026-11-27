<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NRadioButton, NRadioGroup } from 'naive-ui'
import BarList from '../components/charts/BarList.vue'
import DonutChart from '../components/charts/DonutChart.vue'
import ProvinceMap from '../components/charts/ProvinceMap.vue'
import TrendChart from '../components/charts/TrendChart.vue'
import { api } from '../api'
import { formatTs, formatYuan, kindLabels, statusLabels } from '../utils'

interface Trend {
  day: string
  logins: number
  activations: number
  issued_cents: number
  unique_devices: number
}

interface Analytics {
  days: number
  trend: Trend[]
  by_kind: { key: string; count: number }[]
  top_agents: { agent_id: number; name: string; count: number }[]
  cards_total: number
  cards_by_status: Record<string, number>
  devices_bound: number
  agents_active: number
  online_sessions: number
  activations_day: number
  active_devices: number
  geo: { province: string; logins: number; ips: number; abroad: boolean }[]
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
    data.value = await api.get<Analytics>(`/admin/api/analytics?days=${days.value}`)
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

// 补齐没有汇总数据的日期，图才连续
const series = computed(() => {
  const byDay = new Map((data.value?.trend ?? []).map((t) => [t.day, t]))
  const out: Trend[] = []
  const today = new Date()
  for (let i = days.value - 1; i >= 0; i--) {
    const d = new Date(today.getFullYear(), today.getMonth(), today.getDate() - i)
    const key = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
    out.push(byDay.get(key) ?? { day: key, logins: 0, activations: 0, issued_cents: 0, unique_devices: 0 })
  }
  return out
})

const labels = computed(() => series.value.map((t) => t.day.slice(5)))
const traffic = computed(() => [
  { name: '登录', color: '#18a058', values: series.value.map((t) => t.logins) },
  { name: '激活', color: '#6366f1', values: series.value.map((t) => t.activations) },
])
const money = computed(() => [
  { name: '出卡金额', color: '#06b6d4', values: series.value.map((t) => Math.round(t.issued_cents / 100)) },
])

const periodMoney = computed(() => {
  const cents = series.value.reduce((n, t) => n + t.issued_cents, 0)
  return `${formatYuan(cents)} 元`
})
const periodActivations = computed(() => series.value.reduce((n, t) => n + t.activations, 0))
const periodLogins = computed(() => series.value.reduce((n, t) => n + t.logins, 0))

const kindItems = computed(() =>
  (data.value?.by_kind ?? []).map((k, i) => ({
    label: kindLabels[k.key] ?? k.key,
    value: k.count,
    color: palette[i % palette.length],
  })),
)

const statusItems = computed(() =>
  Object.entries(data.value?.cards_by_status ?? {})
    .filter(([, n]) => n > 0)
    .sort((a, b) => b[1] - a[1])
    .map(([st, n]) => ({ label: statusLabels[st] ?? st, value: n, color: statusColor[st] ?? '#94a3b8' })),
)

const agentItems = computed(() =>
  (data.value?.top_agents ?? []).map((a) => ({ label: a.name, value: a.count, color: '#6366f1' })),
)

const geoItems = computed(() =>
  (data.value?.geo ?? []).filter((g) => !g.abroad && g.province !== '未知'),
)
const geoTop = computed(() =>
  geoItems.value
    .slice(0, 10)
    .map((g) => ({ label: g.province, value: g.logins, hint: `${g.ips} 个 IP`, color: '#06b6d4' })),
)

const hasTraffic = computed(() => periodLogins.value + periodActivations.value > 0)
const hasMoney = computed(() => series.value.some((t) => t.issued_cents > 0))

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

    <div class="stats">
      <div class="stat">
        <span class="k">卡密总数</span>
        <span class="v">{{ data?.cards_total ?? '—' }}</span>
        <span class="sub">在线 {{ data?.online_sessions ?? 0 }} · 设备 {{ data?.devices_bound ?? 0 }}</span>
      </div>
      <div class="stat">
        <span class="k">{{ days }} 天代理出卡金额</span>
        <span class="v">{{ periodMoney }}</span>
        <span class="sub">仅代理出的卡计入</span>
      </div>
      <div class="stat">
        <span class="k">{{ days }} 天激活</span>
        <span class="v">{{ periodActivations }}</span>
        <span class="sub">今日 {{ data?.activations_day ?? 0 }} 张</span>
      </div>
      <div class="stat">
        <span class="k">{{ days }} 天活跃设备</span>
        <span class="v">{{ data?.active_devices ?? '—' }}</span>
        <span class="sub">其中已绑定 {{ data?.devices_bound ?? 0 }} 台</span>
      </div>
      <div class="stat">
        <span class="k">活跃代理</span>
        <span class="v">{{ data?.agents_active ?? '—' }}</span>
        <span class="sub">平台 + 代理出卡</span>
      </div>
    </div>

    <div class="grid">
      <n-card size="small" class="span-8">
        <template #header><span class="card-title">登录与激活趋势</span></template>
        <div v-if="!hasTraffic" class="empty">这段时间还没有登录/激活记录。</div>
        <TrendChart v-else :labels="labels" :series="traffic" :height="220" />
      </n-card>

      <n-card size="small" class="span-4">
        <template #header><span class="card-title">卡类型分布</span></template>
        <DonutChart v-if="kindItems.length" :items="kindItems" />
        <div v-else class="empty">还没有卡密。</div>
      </n-card>

      <n-card size="small" class="span-8">
        <template #header><span class="card-title">登录来源省份</span></template>
        <div v-if="geoItems.length === 0" class="empty">这段时间还没有带 IP 的登录记录。</div>
        <ProvinceMap v-else :items="data?.geo ?? []" />
      </n-card>

      <n-card size="small" class="span-4">
        <template #header><span class="card-title">省份排行</span></template>
        <BarList :items="geoTop" unit=" 次" />
      </n-card>

      <n-card size="small" class="span-6">
        <template #header><span class="card-title">代理出卡金额（元）</span></template>
        <div v-if="!hasMoney" class="empty">这段时间还没有出卡。</div>
        <TrendChart v-else :labels="labels" :series="money" :height="180" />
      </n-card>

      <n-card size="small" class="span-3">
        <template #header><span class="card-title">卡密状态</span></template>
        <BarList :items="statusItems" unit=" 张" />
      </n-card>

      <n-card size="small" class="span-3">
        <template #header><span class="card-title">代理出卡排行</span></template>
        <BarList :items="agentItems" unit=" 张" />
      </n-card>
    </div>
  </div>
</template>

<style scoped>
.toolbar { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.spacer { flex: 1; }
.hint { font-size: 12px; color: #98a2b3; }

.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 12px; margin-bottom: 12px; }
.stat {
  background: #fff;
  border: 1px solid #e8edeb;
  border-radius: 12px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.stat .k { font-size: 12px; color: #8b93a7; }
.stat .v { font-size: 24px; font-weight: 700; letter-spacing: -0.02em; font-variant-numeric: tabular-nums; }
.stat .sub { font-size: 11px; color: #98a2b3; }

.grid { display: grid; grid-template-columns: repeat(12, minmax(0, 1fr)); gap: 12px; }
.span-8 { grid-column: span 8; }
.span-6 { grid-column: span 6; }
.span-4 { grid-column: span 4; }
.span-3 { grid-column: span 3; }
.card-title { font-size: 14px; font-weight: 600; }
.empty { color: #98a2b3; font-size: 13px; padding: 24px 0; }

@media (max-width: 1000px) {
  .span-8, .span-6, .span-4, .span-3 { grid-column: span 12; }
}
</style>
