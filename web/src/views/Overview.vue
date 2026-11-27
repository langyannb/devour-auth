<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NSpace, NTag } from 'naive-ui'
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

const stats = ref<Stats | null>(null)
const trend = ref<DailyStat[]>([])
const err = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  err.value = ''
  try {
    const [s, t] = await Promise.all([
      api.get<Stats>('/admin/api/stats'),
      api.get<DailyStat[]>('/admin/api/stats/trend?days=14'),
    ])
    stats.value = s
    trend.value = t
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

// 迷你折线：14 天登录数与激活数
function spark(values: number[]): string {
  if (values.length === 0) return ''
  const max = Math.max(1, ...values)
  const step = values.length > 1 ? 300 / (values.length - 1) : 300
  return values
    .map((v, i) => `${(i * step).toFixed(1)},${(58 - (v / max) * 52).toFixed(1)}`)
    .join(' ')
}

const loginPoints = computed(() => spark(trend.value.map((d) => d.logins)))
const activationPoints = computed(() => spark(trend.value.map((d) => d.activations)))
const trendMax = computed(() => Math.max(1, ...trend.value.map((d) => d.logins)))

const pct = (n: number) => {
  const total = stats.value?.cards_total ?? 0
  if (!total) return 0
  return Math.round((n / total) * 100)
}

onMounted(load)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>

    <n-space :wrap="true" style="margin-bottom: 16px">
      <n-card size="small" style="min-width: 150px">
        <div class="k">卡密总数</div>
        <div class="v">{{ stats?.cards_total ?? '—' }}</div>
      </n-card>
      <n-card size="small" style="min-width: 150px">
        <div class="k">在线会话</div>
        <div class="v">{{ stats?.online_sessions ?? '—' }}</div>
      </n-card>
      <n-card size="small" style="min-width: 150px">
        <div class="k">绑定设备</div>
        <div class="v">{{ stats?.devices_bound ?? '—' }}</div>
      </n-card>
      <n-card size="small" style="min-width: 150px">
        <div class="k">活跃代理</div>
        <div class="v">{{ stats?.agents_active ?? '—' }}</div>
      </n-card>
      <n-card size="small" style="min-width: 150px">
        <div class="k">今日激活</div>
        <div class="v">{{ stats?.activations_day ?? '—' }}</div>
      </n-card>
    </n-space>

    <n-card title="最近 14 天" size="small" style="margin-bottom: 16px">
      <div v-if="trend.length === 0" style="color: #888; padding: 12px 0">
        还没有历史数据（每天 00:07 自动汇总，也可手动 `rollup` 某天）。
      </div>
      <template v-else>
        <div class="spark-label">
          <span>登录次数（峰值 {{ trendMax }}）</span>
        </div>
        <svg viewBox="0 0 300 60" preserveAspectRatio="none" class="spark">
          <polyline :points="loginPoints" fill="none" stroke="#6366f1" stroke-width="2" />
        </svg>
        <div class="spark-label" style="margin-top: 12px"><span>新增激活</span></div>
        <svg viewBox="0 0 300 60" preserveAspectRatio="none" class="spark">
          <polyline :points="activationPoints" fill="none" stroke="#06b6d4" stroke-width="2" />
        </svg>
        <div class="axis">
          <span>{{ trend[0]?.day }}</span>
          <span>{{ trend[trend.length - 1]?.day }}</span>
        </div>
      </template>
    </n-card>

    <n-card title="卡密状态分布" size="small">
      <div v-if="!stats || stats.cards_total === 0" style="color: #888; padding: 12px 0">
        还没有卡密，去「发卡」生成第一批。
      </div>
      <div v-for="(n, st) in stats?.cards_by_status" :key="st" class="row">
        <div class="row-head">
          <n-tag size="small" :type="st === 'active' ? 'success' : st === 'banned' || st === 'recycled' ? 'error' : st === 'expired' || st === 'exhausted' ? 'warning' : 'default'">
            {{ statusLabels[st] ?? st }}
          </n-tag>
          <span class="num">{{ n }} 张 · {{ pct(n) }}%</span>
        </div>
        <div class="bar"><div class="fill" :style="{ width: pct(n) + '%' }"></div></div>
      </div>
      <template #header-extra>
        <n-button size="small" :loading="loading" @click="load">刷新</n-button>
      </template>
    </n-card>

    <p v-if="stats" style="color: #999; font-size: 12px; margin-top: 12px">
      数据时间 {{ formatTs(stats.server_time) }}
    </p>
  </div>
</template>

<style scoped>
.k {
  font-size: 12px;
  color: #8b93a7;
  margin-bottom: 4px;
}
.v {
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -0.02em;
}
.row {
  margin-bottom: 14px;
}
.row-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}
.num {
  font-size: 12px;
  color: #6b7280;
}
.bar {
  height: 6px;
  border-radius: 99px;
  background: #eef0f5;
  overflow: hidden;
}
.fill {
  height: 100%;
  border-radius: 99px;
  background: linear-gradient(90deg, #6366f1, #06b6d4);
}
.spark {
  width: 100%;
  height: 68px;
  display: block;
}
.spark-label {
  font-size: 12px;
  color: #6b7280;
  margin-bottom: 4px;
}
.axis {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: #9aa3b2;
  margin-top: 4px;
}
</style>
