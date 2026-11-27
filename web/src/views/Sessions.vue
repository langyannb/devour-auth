<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NPagination, NSpace, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface SessionRow {
  id: number
  card_id: number
  kami: string
  fingerprint: string
  last_heartbeat: number
  ip: string
  ua: string
  created_at: number
}

const rows = ref<SessionRow[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const err = ref('')
const loading = ref(false)

async function load() {
  loading.value = true
  err.value = ''
  try {
    const d = await api.get<{ items: SessionRow[]; total: number }>(
      `/admin/api/sessions?limit=${pageSize}&offset=${(page.value - 1) * pageSize}`,
    )
    rows.value = d.items
    total.value = d.total
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function kill(id: number) {
  try {
    await api.post(`/admin/api/sessions/${id}/revoke`)
    await load()
  } catch (e) {
    err.value = (e as Error).message
  }
}

const columns: DataTableColumns<SessionRow> = [
  { title: '卡 ID', key: 'card_id', width: 80 },
  { title: '卡密', key: 'kami', width: 200 },
  { title: '设备指纹', key: 'fingerprint', width: 190 },
  { title: '心跳', key: 'last_heartbeat', width: 150, render: (r) => formatTs(r.last_heartbeat) },
  { title: 'IP', key: 'ip', width: 130 },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  {
    title: '操作',
    key: 'op',
    width: 90,
    render: (r) => h(NButton, { size: 'tiny', type: 'error', onClick: () => kill(r.id) }, { default: () => '踢下线' }),
  },
]

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px">
      <n-button size="small" @click="load">刷新</n-button>
      <span style="font-size: 13px; color: #888">在线 {{ total }} 个会话</span>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="950" />
    <n-space justify="end" style="margin-top: 16px">
      <n-pagination v-model:page="page" :page-size="pageSize" :item-count="total" @update:page="load" />
    </n-space>
  </div>
</template>
