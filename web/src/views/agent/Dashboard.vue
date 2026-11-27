<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NAlert, NCard, NSpace, NTag } from 'naive-ui'
import { agentApi } from '../../api'
import { formatYuan } from '../../utils'

interface Summary {
  name: string
  level: number
  discount_bp: number
  rebate_bp: number
  quota_cents: number
  month_used_cents: number
  month_remaining: number
  cards: number
  online: number
  subagents: number
}

interface Notice { id: number; kind: string; title: string; body: string }

const data = ref<Summary | null>(null)
const notices = ref<Notice[]>([])
const err = ref('')

async function load() {
  err.value = ''
  try {
    const [s, n] = await Promise.all([
      agentApi.get<Summary>('/agent/api/summary'),
      agentApi.get<Notice[]>('/agent/api/notices'),
    ])
    data.value = s
    notices.value = n
  } catch (e) {
    err.value = (e as Error).message
  }
}

const quotaText = (v?: number) => {
  if (v === undefined) return '—'
  if (v < 0) return '不限'
  return `${formatYuan(v)} 元`
}

onMounted(load)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>

    <n-space :wrap="true" style="margin-bottom: 16px">
      <n-card size="small" style="min-width: 160px">
        <div class="k">本月已出</div>
        <div class="v">{{ data ? formatYuan(data.month_used_cents) + ' 元' : '—' }}</div>
      </n-card>
      <n-card size="small" style="min-width: 160px">
        <div class="k">可用额度</div>
        <div class="v">{{ quotaText(data?.quota_cents) }}</div>
      </n-card>
      <n-card size="small" style="min-width: 130px">
        <div class="k">我的卡密</div>
        <div class="v">{{ data?.cards ?? '—' }}</div>
      </n-card>
      <n-card size="small" style="min-width: 130px">
        <div class="k">在线</div>
        <div class="v">{{ data?.online ?? '—' }}</div>
      </n-card>
      <n-card size="small" style="min-width: 130px">
        <div class="k">下级</div>
        <div class="v">{{ data?.subagents ?? '—' }}</div>
      </n-card>
    </n-space>

    <n-card v-if="data" size="small" style="margin-bottom: 16px">
      <div class="line"><span>代理名</span><span>{{ data.name }}（L{{ data.level }}）</span></div>
      <div class="line"><span>返点</span><span>{{ (data.rebate_bp / 100).toFixed(1) }}%</span></div>
      <div class="line"><span>额度</span><span>{{ quotaText(data.quota_cents) }}</span></div>
    </n-card>

    <n-card size="small" title="公告">
      <div v-if="notices.length === 0" style="color: #888; font-size: 13px">暂无公告</div>
      <div v-for="n in notices" :key="n.id" class="notice">
        <n-tag size="tiny" :type="n.kind === 'immediate' ? 'error' : n.kind === 'popup' ? 'warning' : 'default'">
          {{ n.kind === 'immediate' ? '重要' : n.kind === 'popup' ? '弹窗' : '公告' }}
        </n-tag>
        <b>{{ n.title }}</b>
        <div>{{ n.body }}</div>
      </div>
    </n-card>
  </div>
</template>

<style scoped>
.k {
  font-size: 12px;
  color: #8b93a7;
  margin-bottom: 4px;
}
.v {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.02em;
}
.line {
  display: flex;
  justify-content: space-between;
  padding: 8px 0;
  border-bottom: 1px solid rgba(18, 21, 28, 0.05);
  font-size: 14px;
}
.line:last-child {
  border-bottom: none;
}
.notice {
  padding: 10px 0;
  border-bottom: 1px solid rgba(18, 21, 28, 0.05);
  font-size: 13px;
}
.notice:last-child {
  border-bottom: none;
}
</style>
