<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NDescriptions, NDescriptionsItem, NInputNumber, NSpace, NTag } from 'naive-ui'
import { api } from '../api'
import { formatYuan, kindLabels } from '../utils'

interface Agent {
  id: number
  name: string
  level: number
  discount_bp: number
  rebate_bp: number
  monthly_quota_cents: number
  status: string
}

interface PriceRow {
  policy_id: number
  code: string
  name: string
  kind: string
  base_price_cents: number
  discount_price: number
  price_cents: number
  custom: boolean
}

const route = useRoute()
const router = useRouter()
const id = Number(route.params.id)

const agentInfo = ref<Agent | null>(null)
const rows = ref<PriceRow[]>([])
const err = ref('')
const notice = ref('')
const busy = ref(false)

async function load() {
  err.value = ''
  try {
    const [list, prices] = await Promise.all([
      api.get<Agent[]>('/admin/api/agents'),
      api.get<PriceRow[]>(`/admin/api/agents/${id}/prices`),
    ])
    agentInfo.value = list.find((a) => a.id === id) ?? null
    rows.value = prices
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function save() {
  err.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api.put(`/admin/api/agents/${id}/prices`, {
      items: rows.value.map((r) => ({ policy_id: r.policy_id, price_cents: r.price_cents })),
    })
    notice.value = '已保存，代理出卡时按新价计算'
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div style="max-width: 980px">
    <n-space style="margin-bottom: 12px">
      <n-button size="small" @click="router.push('/agents')">返回代理列表</n-button>
      <n-button size="small" @click="load">刷新</n-button>
    </n-space>

    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-card v-if="agentInfo" size="small" style="margin-bottom: 16px">
      <template #header>
        {{ agentInfo.name }}
        <n-tag size="small" :type="agentInfo.status === 'active' ? 'success' : 'error'" style="margin-left: 8px">
          {{ agentInfo.status === 'active' ? '正常' : '冻结' }}
        </n-tag>
      </template>
      <n-descriptions :column="2" size="small" bordered label-placement="left">
        <n-descriptions-item label="层级">L{{ agentInfo.level }}</n-descriptions-item>
        <n-descriptions-item label="折扣">{{ (agentInfo.discount_bp / 1000).toFixed(1) }} 折</n-descriptions-item>
        <n-descriptions-item label="返点">{{ (agentInfo.rebate_bp / 100).toFixed(1) }}%</n-descriptions-item>
        <n-descriptions-item label="月额度">
          {{ agentInfo.monthly_quota_cents > 0 ? formatYuan(agentInfo.monthly_quota_cents) + ' 元' : '不限' }}
        </n-descriptions-item>
      </n-descriptions>
    </n-card>

    <n-card size="small" title="代理价格表">
      <template #header-extra>
        <n-button type="primary" size="small" :loading="busy" @click="save">保存价格</n-button>
      </template>
      <p style="font-size: 12px; color: #8b93a7; margin: 0 0 12px">
        这里填的是这个代理出卡时每张的实际价格（元）。填 0 则恢复成默认的「售价 × 折扣」。
      </p>

      <div v-for="r in rows" :key="r.policy_id" class="price-row">
        <div class="info">
          <b>{{ r.name }}</b>
          <span class="muted">{{ r.code }} · {{ kindLabels[r.kind] ?? r.kind }}</span>
        </div>
        <div class="nums">
          <span class="muted">售价 {{ formatYuan(r.base_price_cents) }}</span>
          <span class="muted">折扣价 {{ formatYuan(r.discount_price) }}</span>
        </div>
        <n-input-number
          v-model:value="r.price_cents"
          :min="0"
          :show-button="false"
          style="width: 130px"
          placeholder="默认价"
        >
          <template #suffix>元</template>
        </n-input-number>
        <n-tag v-if="r.custom" size="tiny" type="info">单独定价</n-tag>
      </div>
    </n-card>
  </div>
</template>

<style scoped>
.price-row {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
  padding: 10px 0;
  border-bottom: 1px solid rgba(18, 21, 28, 0.05);
}
.price-row:last-child {
  border-bottom: none;
}
.info {
  min-width: 200px;
  display: flex;
  flex-direction: column;
}
.nums {
  display: flex;
  gap: 12px;
  min-width: 220px;
}
.muted {
  font-size: 12px;
  color: #8b93a7;
}
</style>
