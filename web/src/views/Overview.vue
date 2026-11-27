<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NTag } from 'naive-ui'
import { api } from '../api'
import { formatTs, statusLabels } from '../utils'

interface Stats {
  cards_total: number
  cards_by_status: Record<string, number>
  devices_bound: number
  agents_active: number
  activations_day: number
  online_sessions: number
  server_time: number
}

interface DailyStat {
  day: string
  activations: number
  logins: number
  unique_devices: number
  issued_cents: number
}

interface Ev { ts: number; kind: string; card_id: number; ip: string }

const router = useRouter()
const stats = ref<Stats | null>(null)
const trend = ref<DailyStat[]>([])
const events = ref<Ev[]>([])
const err = ref('')
const loading = ref(false)

const kindLabels: Record<string, string> = {
  login: '登录',
  refresh: '刷新',
  consume: '消耗',
  logout: '登出',
  pause: '暂停',
  refresh_reuse: '令牌盗用',
}

const statusColor: Record<string, string> = {
  unused: '#94a3b8',
  active: '#18a058',
  expired: '#f59e0b',
  exhausted: '#f97316',
  banned: '#ef4444',
  recycled: '#64748b',
}

async function load() {
  loading.value = true
  err.value = ''
  try {
    const [s, t, e] = await Promise.all([
      api.get<Stats>('/admin/api/stats'),
      api.get<DailyStat[]>('/admin/api/stats/trend?days=14'),
      api.get<{ items: Ev[] }>('/admin/api/events?limit=6'),
    ])
    stats.value = s
    trend.value = t
    events.value = e.items
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

// 折线图：登录数做主序列，激活数做次序列，共用 0 基线
const chart = computed(() => {
  const pts = trend.value
  const w = 600
  const h = 170
  const padX = 8
  const top = 14
  const bottom = 30
  if (pts.length < 2) return null
  const max = Math.max(1, ...pts.map((d) => d.logins), ...pts.map((d) => d.activations))
  const x = (i: number) => padX + (i * (w - padX * 2)) / (pts.length - 1)
  const y = (v: number) => h - bottom - (v / max) * (h - bottom - top)
  const line = (key: 'logins' | 'activations') =>
    pts.map((d, i) => `${i === 0 ? 'M' : 'L'}${x(i).toFixed(1)},${y(d[key]).toFixed(1)}`).join(' ')
  const area = `${line('logins')} L${x(pts.length - 1).toFixed(1)},${h - bottom} L${padX},${h - bottom} Z`
  return {
    w,
    h,
    bottom,
    max,
    lineLogins: line('logins'),
    lineActivations: line('activations'),
    area,
    points: pts.map((d, i) => ({ x: x(i), y: y(d.logins), day: d.day, v: d.logins })),
    ticks: [0, Math.round(max / 2), max],
  }
})

const pct = (n: number) => {
  const total = stats.value?.cards_total ?? 0
  if (!total) return 0
  return Math.round((n / total) * 100)
}

const statusRows = computed(() => {
  const m = stats.value?.cards_by_status ?? {}
  return Object.entries(m)
    .filter(([, n]) => n > 0)
    .sort((a, b) => b[1] - a[1])
    .map(([st, n]) => ({ st, n }))
})

const quick = [
  { label: '发卡', desc: '批量生成卡密', to: '/issue' },
  { label: '卡密', desc: '查询与调整', to: '/cards' },
  { label: '代理', desc: '额度与权限', to: '/agents' },
  { label: '价格模板', desc: '出卡价格', to: '/plans' },
  { label: '审计', desc: '哈希链校验', to: '/audit' },
  { label: '安全', desc: '体检与备份', to: '/security' },
]

onMounted(load)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" closable style="margin-bottom: 12px" @close="err = ''">{{ err }}</n-alert>

    <div class="stats">
      <div class="stat">
        <span class="k">卡密总数</span>
        <span class="v">{{ stats?.cards_total ?? '—' }}</span>
      </div>
      <div class="stat">
        <span class="k">在线会话</span>
        <span class="v">{{ stats?.online_sessions ?? '—' }}</span>
      </div>
      <div class="stat">
        <span class="k">绑定设备</span>
        <span class="v">{{ stats?.devices_bound ?? '—' }}</span>
      </div>
      <div class="stat">
        <span class="k">活跃代理</span>
        <span class="v">{{ stats?.agents_active ?? '—' }}</span>
      </div>
      <div class="stat">
        <span class="k">今日激活</span>
        <span class="v">{{ stats?.activations_day ?? '—' }}</span>
      </div>
    </div>

    <div class="body">
      <n-card size="small" class="chart-card">
        <template #header>
          <span class="card-title">最近 14 天</span>
        </template>
        <template #header-extra>
          <n-button size="tiny" :loading="loading" @click="load">刷新</n-button>
        </template>

        <div v-if="!chart" class="empty">
          历史数据不足（每天 00:07 汇总一次，明天起才有趋势）。<br />
          今天：登录 {{ trend[0]?.logins ?? 0 }} 次、新增激活 {{ trend[0]?.activations ?? 0 }} 张。
        </div>
        <template v-else>
          <svg :viewBox="`0 0 ${chart.w} ${chart.h}`" preserveAspectRatio="none" class="chart">
            <defs>
              <linearGradient id="fill-login" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#18a058" stop-opacity="0.22" />
                <stop offset="100%" stop-color="#18a058" stop-opacity="0" />
              </linearGradient>
            </defs>
            <line
              v-for="t in chart.ticks"
              :key="t"
              :x1="0"
              :x2="chart.w"
              :y1="chart.h - chart.bottom - (t / chart.max) * (chart.h - chart.bottom - 14)"
              :y2="chart.h - chart.bottom - (t / chart.max) * (chart.h - chart.bottom - 14)"
              stroke="#eef1f0"
              stroke-width="1"
            />
            <path :d="chart.area" fill="url(#fill-login)" />
            <path :d="chart.lineLogins" fill="none" stroke="#18a058" stroke-width="2.5" stroke-linecap="round" />
            <path :d="chart.lineActivations" fill="none" stroke="#6366f1" stroke-width="2" stroke-dasharray="4 4" />
            <circle v-for="p in chart.points" :key="p.day" :cx="p.x" :cy="p.y" r="3" fill="#18a058" />
          </svg>
          <div class="legend">
            <span><i style="background: #18a058"></i>登录</span>
            <span><i style="background: #6366f1"></i>新增激活</span>
            <span class="axis">{{ trend[0]?.day }} → {{ trend[trend.length - 1]?.day }}</span>
          </div>
        </template>
      </n-card>

      <n-card size="small">
        <template #header><span class="card-title">卡密状态</span></template>
        <div v-if="statusRows.length === 0" class="empty">还没有卡密，去「发卡」生成第一批。</div>
        <div v-for="r in statusRows" :key="r.st" class="row">
          <div class="row-head">
            <span class="state">
              <i :style="{ background: statusColor[r.st] ?? '#94a3b8' }"></i>
              {{ statusLabels[r.st] ?? r.st }}
            </span>
            <span class="num">{{ r.n }} 张 · {{ pct(r.n) }}%</span>
          </div>
          <div class="bar">
            <div class="fill" :style="{ width: pct(r.n) + '%', background: statusColor[r.st] ?? '#94a3b8' }"></div>
          </div>
        </div>
      </n-card>
    </div>

    <div class="body lower">
      <n-card size="small">
        <template #header><span class="card-title">快捷入口</span></template>
        <div class="quick">
          <button v-for="q in quick" :key="q.to" class="tile" @click="router.push(q.to)">
            <b>{{ q.label }}</b>
            <span>{{ q.desc }}</span>
          </button>
        </div>
      </n-card>

      <n-card size="small">
        <template #header><span class="card-title">最近事件</span></template>
        <template #header-extra>
          <n-button text size="tiny" @click="router.push('/events')">全部</n-button>
        </template>
        <div v-if="events.length === 0" class="empty">暂无事件。</div>
        <div v-for="(e, i) in events" :key="i" class="event">
          <n-tag size="tiny" :bordered="false" :type="e.kind === 'refresh_reuse' ? 'error' : 'default'">
            {{ kindLabels[e.kind] ?? e.kind }}
          </n-tag>
          <span class="event-main">卡 #{{ e.card_id || '—' }} · {{ e.ip || '—' }}</span>
          <span class="event-time">{{ formatTs(e.ts) }}</span>
        </div>
      </n-card>
    </div>

    <p v-if="stats" class="stamp">数据时间 {{ formatTs(stats.server_time) }}</p>
  </div>
</template>

<style scoped>
.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}
.stat {
  position: relative;
  background: #fff;
  border: 1px solid #e8edeb;
  border-radius: 12px;
  padding: 14px 16px 16px;
  overflow: hidden;
}
.stat::after {
  content: '';
  position: absolute;
  inset: 0 0 auto 0;
  height: 2px;
  background: linear-gradient(90deg, #18a058, #06b6d4);
  opacity: 0.7;
}
.stat .k { display: block; font-size: 12px; color: #8b93a7; margin-bottom: 6px; }
.stat .v { font-size: 28px; font-weight: 700; letter-spacing: -0.02em; }

.body {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(0, 1fr);
  gap: 12px;
  margin-bottom: 12px;
}
.body.lower { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
.card-title { font-size: 14px; font-weight: 600; }
.empty { color: #98a2b3; font-size: 13px; line-height: 1.7; padding: 8px 0; }

.chart { width: 100%; height: 190px; display: block; }
.legend {
  display: flex;
  align-items: center;
  gap: 16px;
  font-size: 12px;
  color: #6b7280;
  margin-top: 6px;
}
.legend i { display: inline-block; width: 8px; height: 8px; border-radius: 2px; margin-right: 5px; }
.legend .axis { margin-left: auto; color: #98a2b3; }

.row { margin-bottom: 14px; }
.row-head { display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; }
.state { display: inline-flex; align-items: center; gap: 7px; font-size: 13px; }
.state i { width: 8px; height: 8px; border-radius: 50%; }
.num { font-size: 12px; color: #6b7280; }
.bar { height: 6px; border-radius: 99px; background: #eef1f0; overflow: hidden; }
.fill { height: 100%; border-radius: 99px; transition: width 0.5s ease; }

.quick { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
.tile {
  text-align: left;
  border: 1px solid #e8edeb;
  background: #fbfcfc;
  border-radius: 10px;
  padding: 10px 12px;
  cursor: pointer;
  transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
  font-family: inherit;
}
.tile:hover {
  transform: translateY(-2px);
  border-color: #bfe3cf;
  box-shadow: 0 10px 20px -18px rgba(16, 24, 40, 0.5);
}
.tile b { display: block; font-size: 13px; margin-bottom: 2px; }
.tile span { font-size: 11px; color: #8b93a7; }

.event {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 0;
  border-bottom: 1px solid #f1f4f3;
  font-size: 12px;
}
.event:last-child { border-bottom: none; }
.event-main { flex: 1; color: #475467; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.event-time { color: #98a2b3; font-variant-numeric: tabular-nums; }

.stamp { color: #98a2b3; font-size: 12px; margin-top: 12px; }

@media (max-width: 900px) {
  .body, .body.lower { grid-template-columns: minmax(0, 1fr); }
  .quick { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
</style>
