<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NCard, NDataTable, NTag, type DataTableColumns } from 'naive-ui'
import { agentApi } from '../../api'
import { formatTs, formatYuan } from '../../utils'

interface Bill {
  id: number
  month: string
  amount_cents: number
  status: string
  settled_at: number | null
}

interface Ledger {
  id: number
  kind: string
  amount_cents: number
  ref: string
  created_at: number
}

const bills = ref<Bill[]>([])
const ledger = ref<Ledger[]>([])
const err = ref('')
const loading = ref(false)

const kindLabels: Record<string, string> = {
  issue: '出卡',
  rebate: '返点',
  adjust: '调整',
}

const billCols: DataTableColumns<Bill> = [
  { title: '月份', key: 'month', width: 110 },
  { title: '金额（元）', key: 'amount_cents', width: 130, render: (r) => formatYuan(r.amount_cents) },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render: (r) => h(NTag, { type: r.status === 'settled' ? 'success' : 'warning', size: 'small' },
      { default: () => (r.status === 'settled' ? '已结算' : '待结算') }),
  },
  { title: '结算时间', key: 'settled_at', render: (r) => (r.settled_at ? formatTs(r.settled_at) : '—') },
]

const ledgerCols: DataTableColumns<Ledger> = [
  { title: '时间', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  { title: '类型', key: 'kind', width: 90, render: (r) => kindLabels[r.kind] ?? r.kind },
  { title: '金额（元）', key: 'amount_cents', width: 120, render: (r) => formatYuan(r.amount_cents) },
  { title: '关联', key: 'ref' },
]

function exportCSV() {
  window.open('/agent/api/ledger/export', '_blank')
}

async function load() {
  loading.value = true
  try {
    const [b, l] = await Promise.all([
      agentApi.get<Bill[]>('/agent/api/bills'),
      agentApi.get<{ items: Ledger[] }>('/agent/api/ledger?limit=100'),
    ])
    bills.value = b
    ledger.value = l.items
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-card title="月度账单" style="margin-bottom: 16px">
      <n-data-table :columns="billCols" :data="bills" :loading="loading" :bordered="false" size="small" :scroll-x="600" />
    </n-card>
    <n-card title="流水">
      <template #header-extra>
        <n-button size="small" @click="exportCSV">导出 CSV</n-button>
      </template>
      <n-data-table :columns="ledgerCols" :data="ledger" :loading="loading" :bordered="false" size="small" :scroll-x="600" />
    </n-card>
  </div>
</template>
