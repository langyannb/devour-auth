<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NInput, NPagination, NSpace, NSwitch, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Device {
  id: number
  card_id: number
  fingerprint: string
  model: string
  brand: string
  os: string
  app_version: string
  first_seen: number
  last_seen: number
  banned: boolean
  unbound_at: number | null
}

const rows = ref<Device[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const fp = ref('')
const showAll = ref(false)
const err = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  err.value = ''
  try {
    const q = new URLSearchParams({ limit: String(pageSize), offset: String((page.value - 1) * pageSize) })
    if (fp.value) q.set('fingerprint', fp.value)
    if (showAll.value) q.set('all', '1')
    const d = await api.get<{ items: Device[]; total: number }>(`/admin/api/devices?${q}`)
    rows.value = d.items
    total.value = d.total
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function act(row: Device, action: string) {
  try {
    await api.post(`/admin/api/devices/${row.id}/${action}`)
    await load()
  } catch (e) {
    err.value = (e as Error).message
  }
}

const columns: DataTableColumns<Device> = [
  { title: '卡 ID', key: 'card_id', width: 80 },
  { title: '指纹', key: 'fingerprint', width: 200 },
  { title: '型号', key: 'model', render: (r) => `${r.brand} ${r.model}`.trim() || '—' },
  { title: '系统', key: 'os', width: 110 },
  { title: '最近', key: 'last_seen', width: 150, render: (r) => formatTs(r.last_seen) },
  {
    title: '状态',
    key: 'state',
    width: 90,
    render: (r) =>
      h(NTag, { size: 'small', type: r.unbound_at ? 'default' : r.banned ? 'error' : 'success' },
        { default: () => (r.unbound_at ? '已解绑' : r.banned ? '已封禁' : '绑定中') }),
  },
  {
    title: '操作',
    key: 'op',
    width: 170,
    render: (r) =>
      h(NSpace, { size: 4 }, {
        default: () => [
          h(NButton, { size: 'tiny', disabled: !!r.unbound_at, onClick: () => act(r, 'unbind') }, { default: () => '解绑' }),
          h(NButton, {
            size: 'tiny',
            type: r.banned ? 'success' : 'error',
            onClick: () => act(r, r.banned ? 'unban' : 'ban'),
          }, { default: () => (r.banned ? '解封' : '封禁') }),
        ],
      }),
  },
]

function search() {
  page.value = 1
  void load()
}

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px" :wrap="true">
      <n-input v-model:value="fp" placeholder="设备指纹包含" style="width: 220px" clearable @keyup.enter="search" />
      <n-space align="center"><span style="font-size: 13px; color: #666">含已解绑</span><n-switch v-model:value="showAll" @update:value="search" /></n-space>
      <n-button type="primary" @click="search">查询</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="880" />
    <n-space justify="end" style="margin-top: 16px">
      <n-pagination v-model:page="page" :page-size="pageSize" :item-count="total" @update:page="load" />
    </n-space>
  </div>
</template>
