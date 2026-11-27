<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NAlert, NButton, NDataTable, NInput, NPagination, NSpace, NTag,
  type DataTableColumns,
} from 'naive-ui'
import { api } from '../api'
import { copyText, formatExpiry, formatTs, kindLabels, statusLabels } from '../utils'

interface Card {
  id: number
  kami: string
  kind: string
  status: string
  points: number
  remain_times: number
  remaining_seconds: number
  device_limit: number
  expire_at: number
  created_at: number
  agent_id: number | null
}

const router = useRouter()
const rows = ref<Card[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const kami = ref('')
const status = ref('')
const loading = ref(false)
const err = ref('')
const notice = ref('')
const selected = ref<number[]>([])
const busy = ref(false)

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
  { type: 'selection' },
  { title: 'ID', key: 'id', width: 64 },
  {
    title: '卡密',
    key: 'kami',
    width: 210,
    render: (r) =>
      h('a', {
        style: 'color:#4f46e5;cursor:pointer;font-family:monospace',
        onClick: () => router.push(`/cards/${r.id}`),
      }, r.kami),
  },
  { title: '类型', key: 'kind', width: 86, render: (r) => kindLabels[r.kind] ?? r.kind },
  {
    title: '状态',
    key: 'status',
    width: 86,
    render: (r) => h(NTag, { type: statusType(r.status), size: 'small' }, { default: () => statusLabels[r.status] ?? r.status }),
  },
  {
    title: '余额',
    key: 'balance',
    width: 100,
    render: (r) => {
      if (r.kind === 'credit') return `${r.points} 点`
      if (r.kind === 'count') return `${r.remain_times} 次`
      if (r.kind === 'timed') return `${Math.round(r.remaining_seconds / 60)} 分`
      return '—'
    },
  },
  { title: '设备上限', key: 'device_limit', width: 86 },
  { title: '到期', key: 'expire_at', width: 140, render: (r) => formatExpiry(r.kind, r.expire_at, r.remaining_seconds) },
  { title: '来源', key: 'agent_id', width: 80, render: (r) => (r.agent_id ? `代理${r.agent_id}` : '平台') },
  {
    title: '操作',
    key: 'op',
    width: 200,
    render: (r) =>
      h(NSpace, { size: 4 }, {
        default: () => [
          h(NButton, { size: 'tiny', onClick: () => copyKami(r.kami) }, { default: () => '复制' }),
          h(NButton, {
            size: 'tiny',
            type: r.status === 'banned' ? 'success' : 'error',
            onClick: () => quick(r, r.status === 'banned' ? 'unban' : 'ban'),
          }, { default: () => (r.status === 'banned' ? '解封' : '封禁') }),
          h(NButton, { size: 'tiny', onClick: () => quick(r, 'unbind') }, { default: () => '解绑' }),
        ],
      }),
  },
])

async function copyKami(kami: string) {
  err.value = ''
  if (await copyText(kami)) notice.value = `已复制 ${kami}`
  else err.value = '复制失败，请手动选中卡密'
}

async function load() {
  loading.value = true
  err.value = ''
  try {
    const q = new URLSearchParams({ limit: String(pageSize), offset: String((page.value - 1) * pageSize) })
    if (kami.value) q.set('kami', kami.value)
    if (status.value) q.set('status', status.value)
    const d = await api.get<{ items: Card[]; total: number }>(`/admin/api/cards?${q.toString()}`)
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

async function quick(c: Card, action: string) {
  notice.value = ''
  busy.value = true
  try {
    await api.post(`/admin/api/cards/${c.id}/${action}`)
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function batch(action: string) {
  if (selected.value.length === 0) return
  notice.value = ''
  err.value = ''
  busy.value = true
  try {
    const d = await api.post<{ ok_count: number; failed: { id: number; error: string }[] }>(
      '/admin/api/cards/batch',
      { ids: selected.value, action },
    )
    notice.value = `成功 ${d.ok_count} 张${d.failed.length ? `，失败 ${d.failed.length} 张（${d.failed[0].error}）` : ''}`
    selected.value = []
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function exportCSV() {
  const q = new URLSearchParams()
  if (status.value) q.set('status', status.value)
  if (kami.value) q.set('kami', kami.value)
  window.open(`/admin/api/cards/export?${q.toString()}`, '_blank')
}

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px" :wrap="true">
      <n-input v-model:value="kami" placeholder="卡密包含" style="width: 200px" clearable @keyup.enter="search" />
      <select v-model="status" style="padding: 0 8px; height: 34px; border-radius: 3px; border: 1px solid #dcdfe6">
        <option value="">全部状态</option>
        <option v-for="(label, key) in statusLabels" :key="key" :value="key">{{ label }}</option>
      </select>
      <n-button type="primary" @click="search">查询</n-button>
      <n-button @click="exportCSV">导出 CSV</n-button>
    </n-space>

    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <div v-if="selected.length > 0" class="batch-bar">
      <span>已选 {{ selected.length }} 张</span>
      <n-button size="small" :loading="busy" @click="batch('ban')">批量封禁</n-button>
      <n-button size="small" :loading="busy" @click="batch('unban')">批量解封</n-button>
      <n-button size="small" :loading="busy" @click="batch('unbind')">批量解绑</n-button>
      <n-button size="small" type="warning" :loading="busy" @click="batch('recycle')">批量回收</n-button>
    </div>

    <n-data-table
      v-model:checked-row-keys="selected"
      :columns="columns"
      :data="rows"
      :loading="loading"
      :bordered="false"
      size="small"
      :scroll-x="1050"
      :row-key="(r: Card) => r.id"
    />

    <n-space justify="end" style="margin-top: 16px">
      <n-pagination v-model:page="page" :page-size="pageSize" :item-count="total" @update:page="load" />
    </n-space>
  </div>
</template>

<style scoped>
.batch-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  padding: 10px 12px;
  margin-bottom: 12px;
  border-radius: 8px;
  background: #eef2ff;
  font-size: 13px;
  color: #4338ca;
}
</style>
