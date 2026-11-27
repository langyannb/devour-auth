<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NDataTable, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatYuan, kindLabels } from '../utils'

interface Policy {
  id: number
  code: string
  name: string
  kind: string
  device_limit: number
  concurrent_limit: number
  price_cents: number
  active: number
}

const rows = ref<Policy[]>([])
const err = ref('')
const loading = ref(false)

const columns: DataTableColumns<Policy> = [
  { title: 'ID', key: 'id', width: 70 },
  { title: 'code', key: 'code', width: 140 },
  { title: '名称', key: 'name' },
  { title: '类型', key: 'kind', width: 100, render: (r) => kindLabels[r.kind] ?? r.kind },
  { title: '设备上限', key: 'device_limit', width: 90 },
  { title: '并发上限', key: 'concurrent_limit', width: 90 },
  { title: '售价（元）', key: 'price_cents', width: 110, render: (r) => formatYuan(r.price_cents) },
  {
    title: '状态',
    key: 'active',
    width: 90,
    render: (r) => h(NTag, { type: r.active ? 'success' : 'default', size: 'small' }, { default: () => (r.active ? '启用' : '停用') }),
  },
]

async function load() {
  loading.value = true
  try {
    const apps = await api.get<Array<{ id: number }>>('/admin/api/apps')
    rows.value = await api.get<Policy[]>(`/admin/api/policies?app_id=${apps[0]?.id ?? 1}`)
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
    <n-data-table :columns="columns" :data="rows" :loading="loading" :bordered="false" size="small" />
  </div>
</template>
