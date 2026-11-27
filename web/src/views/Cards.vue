<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NAlert, NButton, NDataTable, NInput, NInputNumber, NModal, NPagination, NSelect, NSpace, NTag,
  type DataTableColumns,
} from 'naive-ui'
import { api } from '../api'
import { cardKindLabel, copyText, formatExpiry, formatTs, statusLabels } from '../utils'

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
  duration_seconds: number
  bound_devices: number
  online_devices: number
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

// 按范围批量操作（不是勾选，是整批）
const bulkScope = ref('active')
const bulkValue = ref(1)
const bulkUnit = ref<'day' | 'hour'>('day')
const bulkBusy = ref(false)
const confirmBox = ref({ show: false, action: 'add', text: '', seconds: 0 })

const scopeLabels: Record<string, string> = {
  active: '已激活', unused: '未使用', expired: '已过期', all: '全部',
}

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
  { title: '类型', key: 'kind', width: 120, render: (r) => cardKindLabel(r.kind, r.duration_seconds) },
  {
    title: '状态',
    key: 'status',
    width: 86,
    render: (r) => h(NTag, { type: statusType(r.status), size: 'small' }, { default: () => statusLabels[r.status] ?? r.status }),
  },
  {
    title: '剩余',
    key: 'balance',
    width: 100,
    render: (r) => {
      if (r.kind === 'credit') return `${r.points} 点`
      if (r.kind === 'count') return `${r.remain_times} 次`
      if (r.kind === 'timed') return `${Math.round(r.remaining_seconds / 60)} 分`
      return '—'
    },
  },
  {
    title: '设备',
    key: 'devices',
    width: 90,
    render: (r) => `${r.bound_devices}/${r.device_limit}`,
  },
  {
    title: '在线',
    key: 'online',
    width: 90,
    render: (r) => h(NTag, { type: r.online_devices > 0 ? 'success' : 'default', size: 'small' },
      { default: () => (r.online_devices > 0 ? `在线 ${r.online_devices}` : '离线') }),
  },
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

function bulkSeconds(): number {
  return bulkValue.value * (bulkUnit.value === 'day' ? 86400 : 3600)
}

async function previewBulk(action: string) {
  err.value = ''
  notice.value = ''
  const scope = action === 'recycle' ? 'expired' : bulkScope.value
  try {
    const d = await api.get<{ total: number; time_cards: number }>(
      `/admin/api/cards/bulk-preview?scope=${scope}`,
    )
    const n = action === 'recycle' ? d.total : d.time_cards
    if (n === 0) {
      notice.value = `「${scopeLabels[scope]}」没有可操作的卡`
      return
    }
    const unit = bulkUnit.value === 'day' ? '天' : '小时'
    const what = action === 'add'
      ? `加时 ${bulkValue.value} ${unit}`
      : action === 'subtract' ? `扣时 ${bulkValue.value} ${unit}` : '清理（置为已回收）'
    confirmBox.value = {
      show: true,
      action,
      seconds: action === 'recycle' ? 0 : bulkSeconds(),
      text: `将对「${scopeLabels[scope]}」的 ${n} 张卡${what}，操作不可逐张撤销。`,
    }
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function doBulk() {
  bulkBusy.value = true
  err.value = ''
  notice.value = ''
  const { action, seconds } = confirmBox.value
  const scope = action === 'recycle' ? 'expired' : bulkScope.value
  try {
    const d = await api.post<{ affected: number }>('/admin/api/cards/bulk', { scope, action, seconds })
    notice.value = `已处理 ${d.affected} 张（${scopeLabels[scope]}）`
    confirmBox.value.show = false
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    bulkBusy.value = false
  }
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

    <div class="bulk-bar">
      <span class="bulk-label">批量调时</span>
      <n-select
        v-model:value="bulkScope"
        :options="Object.entries(scopeLabels).map(([value, label]) => ({ value, label }))"
        size="small"
        style="width: 110px"
      />
      <n-input-number v-model:value="bulkValue" :min="1" :precision="0" size="small" style="width: 100px" />
      <n-select
        v-model:value="bulkUnit"
        :options="[{ label: '天', value: 'day' }, { label: '小时', value: 'hour' }]"
        size="small"
        style="width: 90px"
      />
      <n-button size="small" :loading="bulkBusy" @click="previewBulk('add')">全部加时</n-button>
      <n-button size="small" :loading="bulkBusy" @click="previewBulk('subtract')">全部扣时</n-button>
      <n-button size="small" type="error" ghost :loading="bulkBusy" @click="previewBulk('recycle')">清理已过期</n-button>
    </div>

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
      :scroll-x="1250"
      :row-key="(r: Card) => r.id"
    />

    <n-space justify="end" style="margin-top: 16px">
      <n-pagination v-model:page="page" :page-size="pageSize" :item-count="total" @update:page="load" />
    </n-space>

    <n-modal v-model:show="confirmBox.show" preset="card" title="确认批量操作" style="width: 440px">
      <p style="margin: 0 0 16px">{{ confirmBox.text }}</p>
      <n-space justify="end">
        <n-button @click="confirmBox.show = false">取消</n-button>
        <n-button type="primary" :loading="bulkBusy" @click="doBulk">确认执行</n-button>
      </n-space>
    </n-modal>
  </div>
</template>

<style scoped>
.bulk-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 12px;
  padding: 10px 12px;
  background: #fff;
  border: 1px solid #eee;
  border-radius: 6px;
}
.bulk-label {
  font-size: 13px;
  color: #666;
}
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
