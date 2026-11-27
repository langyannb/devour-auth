<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NDataTable, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs, formatYuan } from '../utils'

interface Ledger { id: number; agent_id: number; kind: string; amount_cents: number; ref: string; created_at: number }
interface Bill { id: number; agent_id: number; month: string; amount_cents: number; status: string; settled_at: number | null }
interface Batch { id: number; policy_id: number; agent_id: number | null; qty: number; created_by: number; created_at: number }

const ledger = ref<Ledger[]>([])
const bills = ref<Bill[]>([])
const batches = ref<Batch[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  err.value = ''
  try {
    const [l, b, bt] = await Promise.all([
      api.get<{ items: Ledger[] }>('/admin/api/finance/ledger?limit=100'),
      api.get<Bill[]>('/admin/api/finance/bills'),
      api.get<Batch[]>('/admin/api/batches'),
    ])
    ledger.value = l.items
    bills.value = b
    batches.value = bt
  } catch (e) { err.value = (e as Error).message } finally { loading.value = false }
}

async function settle(b: Bill) {
  notice.value = ''
  try {
    await api.post(`/admin/api/bills/${b.id}/settle`)
    notice.value = `账单 ${b.month} 已标记结算`
    await load()
  } catch (e) { err.value = (e as Error).message }
}

const kindLabel: Record<string, string> = { issue: '出卡', rebate: '返点', adjust: '调整' }

const ledgerCols: DataTableColumns<Ledger> = [
  { title: '时间', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  { title: '代理', key: 'agent_id', width: 70 },
  { title: '类型', key: 'kind', width: 90, render: (r) => kindLabel[r.kind] ?? r.kind },
  { title: '金额（元）', key: 'amount_cents', width: 120, render: (r) => formatYuan(r.amount_cents) },
  { title: '关联', key: 'ref' },
]

const billCols: DataTableColumns<Bill> = [
  { title: '月份', key: 'month', width: 110 },
  { title: '代理', key: 'agent_id', width: 70 },
  { title: '金额（元）', key: 'amount_cents', width: 130, render: (r) => formatYuan(r.amount_cents) },
  {
    title: '状态', key: 'status', width: 100,
    render: (r) => h(NTag, { size: 'small', type: r.status === 'settled' ? 'success' : 'warning' },
      { default: () => (r.status === 'settled' ? '已结算' : '待结算') }),
  },
  { title: '结算时间', key: 'settled_at', width: 150, render: (r) => (r.settled_at ? formatTs(r.settled_at) : '—') },
  {
    title: '操作', key: 'op', width: 100,
    render: (r) => r.status === 'open'
      ? h(NButton, { size: 'tiny', onClick: () => settle(r) }, { default: () => '标记结算' })
      : '—',
  },
]

const batchCols: DataTableColumns<Batch> = [
  { title: '批次', key: 'id', width: 80 },
  { title: '卡类型', key: 'policy_id', width: 90 },
  { title: '数量', key: 'qty', width: 80 },
  { title: '来源', key: 'agent_id', width: 100, render: (r) => (r.agent_id ? `代理${r.agent_id}` : '平台') },
  { title: '创建', key: 'created_at', render: (r) => formatTs(r.created_at) },
]

onMounted(load)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-card title="代理账单" size="small" style="margin-bottom: 16px">
      <template #header-extra><n-button size="small" @click="load">刷新</n-button></template>
      <n-data-table :columns="billCols" :data="bills" :loading="loading" size="small" :bordered="false" :scroll-x="800" />
    </n-card>

    <n-card title="资金流水（最近 100 条）" size="small" style="margin-bottom: 16px">
      <n-data-table :columns="ledgerCols" :data="ledger" :loading="loading" size="small" :bordered="false" :scroll-x="760" />
    </n-card>

    <n-card title="发卡批次" size="small">
      <n-data-table :columns="batchCols" :data="batches" :loading="loading" size="small" :bordered="false" :scroll-x="700" />
    </n-card>
  </div>
</template>
