<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NInput, NPagination, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { agentApi } from '../../api'
import { formatTs, kindLabels, statusLabels } from '../../utils'

interface Card {
  id: number
  kami: string
  kind: string
  status: string
  agent_id: number | null
  points: number
  remain_times: number
  expire_at: number
  created_at: number
}

const rows = ref<Card[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const kami = ref('')
const loading = ref(false)
const err = ref('')

const statusType = (s: string) => {
  switch (s) {
    case 'active':
      return 'success'
    case 'banned':
    case 'recycled':
      return 'error'
    case 'expired':
    case 'exhausted':
      return 'warning'
    default:
      return 'default'
  }
}

const columns = computed<DataTableColumns<Card>>(() => [
  { title: 'ID', key: 'id', width: 70 },
  { title: '卡密', key: 'kami', width: 210 },
  { title: '类型', key: 'kind', width: 90, render: (r) => kindLabels[r.kind] ?? r.kind },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (r) => h(NTag, { type: statusType(r.status), size: 'small' }, { default: () => statusLabels[r.status] ?? r.status }),
  },
  {
    title: '余额',
    key: 'balance',
    width: 110,
    render: (r) => {
      if (r.kind === 'credit') return `${r.points} 点`
      if (r.kind === 'count') return `${r.remain_times} 次`
      return '—'
    },
  },
  { title: '到期', key: 'expire_at', width: 150, render: (r) => (r.expire_at ? formatTs(r.expire_at) : '永久') },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
])

async function load() {
  loading.value = true
  err.value = ''
  try {
    const q = new URLSearchParams({ limit: String(pageSize), offset: String((page.value - 1) * pageSize) })
    if (kami.value) q.set('kami', kami.value)
    const d = await agentApi.get<{ items: Card[]; total: number }>(`/agent/api/cards?${q.toString()}`)
    rows.value = d.items
    total.value = d.total
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

function search() {
  page.value = 1
  void load()
}

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 16px" :wrap="true">
      <n-input v-model:value="kami" placeholder="卡密包含" style="width: 220px" clearable @keyup.enter="search" />
      <n-button type="primary" @click="search">查询</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" :bordered="false" size="small" :scroll-x="900" />
    <n-space justify="end" style="margin-top: 16px">
      <n-pagination v-model:page="page" :page-size="pageSize" :item-count="total" @update:page="load" />
    </n-space>
  </div>
</template>
