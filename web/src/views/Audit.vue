<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NSpace, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface AuditRow {
  id: number
  ts: number
  actor: string
  action: string
  target: string
  ip: string
}

const rows = ref<AuditRow[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 50
const err = ref('')
const loading = ref(false)

const columns: DataTableColumns<AuditRow> = [
  { title: 'ID', key: 'id', width: 80 },
  { title: '时间', key: 'ts', width: 150, render: (r) => formatTs(r.ts) },
  { title: '操作者', key: 'actor', width: 110 },
  { title: '动作', key: 'action', width: 220 },
  { title: '对象', key: 'target', width: 140 },
  { title: 'IP', key: 'ip', width: 130 },
]

async function load() {
  loading.value = true
  try {
    const d = await api.get<{ items: AuditRow[]; total: number }>(
      `/admin/api/audit?limit=${pageSize}&offset=${(page.value - 1) * pageSize}`,
    )
    rows.value = d.items
    total.value = d.total
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

function next() {
  if (page.value * pageSize < total.value) {
    page.value++
    void load()
  }
}

function prev() {
  if (page.value > 1) {
    page.value--
    void load()
  }
}

onMounted(load)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" :bordered="false" size="small" />
    <n-space justify="end" align="center" style="margin-top: 16px">
      <span>共 {{ total }} 条</span>
      <n-button size="small" :disabled="page <= 1" @click="prev">上一页</n-button>
      <span>第 {{ page }} 页</span>
      <n-button size="small" :disabled="page * pageSize >= total" @click="next">下一页</n-button>
    </n-space>
  </div>
</template>
