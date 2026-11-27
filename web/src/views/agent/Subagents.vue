<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NDataTable, NTag, type DataTableColumns } from 'naive-ui'
import { agentApi } from '../../api'
import { formatYuan } from '../../utils'

interface Sub {
  id: number
  name: string
  parent_id: number | null
  level: number
  discount_bp: number
  rebate_bp: number
  monthly_quota_cents: number
  status: string
  is_self?: boolean
}

const rows = ref<Sub[]>([])
const err = ref('')
const loading = ref(false)

const columns: DataTableColumns<Sub> = [
  {
    title: '代理',
    key: 'name',
    render: (r) =>
      h('span', {}, [
        r.name,
        r.is_self ? h(NTag, { size: 'tiny', type: 'info', style: 'margin-left:8px' }, { default: () => '我' }) : null,
      ]),
  },
  { title: '层级', key: 'level', width: 80, render: (r) => `L${r.level}` },
  { title: '折扣', key: 'discount_bp', width: 100, render: (r) => `${(r.discount_bp / 1000).toFixed(1)} 折` },
  { title: '返点', key: 'rebate_bp', width: 90, render: (r) => `${(r.rebate_bp / 100).toFixed(1)}%` },
  {
    title: '月额度',
    key: 'monthly_quota_cents',
    width: 120,
    render: (r) => (r.monthly_quota_cents > 0 ? `${formatYuan(r.monthly_quota_cents)} 元` : '不限'),
  },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (r) => h(NTag, { size: 'small', type: r.status === 'active' ? 'success' : 'error' },
      { default: () => (r.status === 'active' ? '正常' : '冻结') }),
  },
]

async function load() {
  loading.value = true
  try {
    rows.value = await agentApi.get<Sub[]>('/agent/api/subagents')
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
    <n-data-table :columns="columns" :data="rows" :loading="loading" :bordered="false" size="small" :scroll-x="620" />
  </div>
</template>
