<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NInput, NPagination, NSelect, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Ev { ts: number; kind: string; card_id: number; device_id: number; ip: string; ua: string; code: string; detail: string }

const rows = ref<Ev[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const kind = ref('')
const cardID = ref('')
const err = ref('')
const loading = ref(false)

const kinds = [
  { label: '全部', value: '' },
  { label: '登录', value: 'login' },
  { label: '刷新', value: 'refresh' },
  { label: '消耗', value: 'consume' },
  { label: '登出', value: 'logout' },
  { label: '暂停', value: 'pause' },
  { label: '令牌盗用', value: 'refresh_reuse' },
]

async function load() {
  loading.value = true
  err.value = ''
  try {
    const q = new URLSearchParams({ limit: String(pageSize), offset: String((page.value - 1) * pageSize) })
    if (kind.value) q.set('kind', kind.value)
    if (cardID.value) q.set('card_id', cardID.value)
    const d = await api.get<{ items: Ev[]; total: number }>(`/admin/api/events?${q}`)
    rows.value = d.items
    total.value = d.total
  } catch (e) { err.value = (e as Error).message } finally { loading.value = false }
}

const cols: DataTableColumns<Ev> = [
  { title: '时间', key: 'ts', width: 150, render: (r) => formatTs(r.ts) },
  { title: '类型', key: 'kind', width: 120 },
  { title: '卡', key: 'card_id', width: 70, render: (r) => (r.card_id ? r.card_id : '—') },
  { title: '设备', key: 'device_id', width: 70, render: (r) => (r.device_id ? r.device_id : '—') },
  { title: 'IP', key: 'ip', width: 130 },
  { title: '结果', key: 'code', width: 140, render: (r) => r.code ? r.code : '—' },
  { title: '详情', key: 'detail', render: (r) => r.detail || '—' },
]

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px" :wrap="true">
      <n-select v-model:value="kind" :options="kinds" style="width: 150px" @update:value="() => { page = 1; load() }" />
      <n-input v-model:value="cardID" placeholder="卡 ID" style="width: 120px" clearable @keyup.enter="() => { page = 1; load() }" />
      <n-button type="primary" @click="() => { page = 1; load() }">查询</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-data-table :columns="cols" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="900" />
    <n-space justify="end" style="margin-top: 16px">
      <n-pagination v-model:page="page" :page-size="pageSize" :item-count="total" @update:page="load" />
    </n-space>
  </div>
</template>
