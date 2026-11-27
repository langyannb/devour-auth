<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NAlert, NButton, NCard, NDataTable, NDescriptions, NDescriptionsItem,
  NInputNumber, NModal, NSpace, NTag, type DataTableColumns,
} from 'naive-ui'
import { api } from '../api'
import { cardKindLabel, formatExpiry, formatTs, statusLabels } from '../utils'

interface Card {
  id: number
  kami: string
  kind: string
  status: string
  policy_id: number
  device_limit: number
  verify_device: number
  expire_at: number
  remain_times: number
  points: number
  remaining_seconds: number
  paused: number
  rebind_used: number
  note: string
  batch_id: number | null
  agent_id: number | null
  activated_at: number | null
  banned_at: number | null
  created_at: number
  bound_devices: number
  snapshot: any
}

interface Device {
  id: number
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

interface Session {
  id: number
  fingerprint: string
  last_heartbeat: number
  access_exp: number
  ip: string
  ua: string
  created_at: number
}

const route = useRoute()
const router = useRouter()
const id = Number(route.params.id)

const card = ref<Card | null>(null)
const devices = ref<Device[]>([])
const sessions = ref<Session[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)

const showExtend = ref(false)
const showLimit = ref(false)

// 出卡时冻结的时长，详情页据此显示「天卡/周卡/月卡」
const durationSeconds = computed(() => {
  const s = card.value?.snapshot
  if (!s) return 0
  return (s.duration_days ?? 0) * 86400 + (s.duration_hours ?? 0) * 3600 + (s.duration_seconds ?? 0)
})
const limitVal = ref(1)
const ext = ref({ days: 0, points: 0, times: 0, seconds: 0 })
const busy = ref(false)

async function load() {
  loading.value = true
  err.value = ''
  try {
    const [c, d, s] = await Promise.all([
      api.get<Card>(`/admin/api/cards/${id}`),
      api.get<Device[]>(`/admin/api/cards/${id}/devices`),
      api.get<Session[]>(`/admin/api/cards/${id}/sessions`),
    ])
    card.value = c
    devices.value = d
    sessions.value = s
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function act(action: string) {
  notice.value = ''
  err.value = ''
  busy.value = true
  try {
    await api.post(`/admin/api/cards/${id}/${action}`)
    notice.value = '操作成功'
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function toggleVerifyDevice() {
  busy.value = true
  err.value = ''
  notice.value = ''
  try {
    const on = card.value?.verify_device === 0 // 当前关着就打开
    await api.patch(`/admin/api/cards/${id}`, { verify_device: on })
    notice.value = on ? '已开启设备校验' : '已关闭设备校验（不限设备数量与同时在线）'
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function extend() {
  busy.value = true
  err.value = ''
  try {
    await api.post(`/admin/api/cards/${id}/extend`, ext.value)
    notice.value = '已续期'
    showExtend.value = false
    ext.value = { days: 0, points: 0, times: 0, seconds: 0 }
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function openLimit() {
  limitVal.value = card.value?.device_limit ?? 1
  showLimit.value = true
}

async function saveLimit() {
  busy.value = true
  err.value = ''
  try {
    await api.patch(`/admin/api/cards/${id}`, { device_limit: limitVal.value })
    notice.value = '设备上限已更新，立即生效'
    showLimit.value = false
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function killSession(sid: number) {
  try {
    await api.post(`/admin/api/sessions/${sid}/revoke`)
    notice.value = '已踢下线'
    await load()
  } catch (e) {
    err.value = (e as Error).message
  }
}

const deviceCols: DataTableColumns<Device> = [
  { title: '设备指纹', key: 'fingerprint', width: 200 },
  { title: '型号', key: 'model', render: (r) => `${r.brand} ${r.model}`.trim() || '—' },
  { title: '首次', key: 'first_seen', width: 150, render: (r) => formatTs(r.first_seen) },
  { title: '最近', key: 'last_seen', width: 150, render: (r) => formatTs(r.last_seen) },
  {
    title: '状态',
    key: 'state',
    width: 90,
    render: (r) =>
      h(NTag, { size: 'small', type: r.unbound_at ? 'default' : r.banned ? 'error' : 'success' },
        { default: () => (r.unbound_at ? '已解绑' : r.banned ? '已封禁' : '绑定中') }),
  },
]

const sessionCols: DataTableColumns<Session> = [
  { title: '设备指纹', key: 'fingerprint', width: 190 },
  { title: '心跳', key: 'last_heartbeat', width: 150, render: (r) => formatTs(r.last_heartbeat) },
  { title: 'IP', key: 'ip', width: 130 },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  {
    title: '操作',
    key: 'op',
    width: 90,
    render: (r) => h(NButton, { size: 'tiny', onClick: () => killSession(r.id) }, { default: () => '踢下线' }),
  },
]

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px" :wrap="true">
      <n-button size="small" @click="router.push('/cards')">返回列表</n-button>
      <n-button size="small" @click="load">刷新</n-button>
    </n-space>

    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-card v-if="card" size="small" style="margin-bottom: 16px">
      <template #header>
        <span style="font-family: monospace">{{ card.kami }}</span>
        <n-tag size="small" :type="card.status === 'active' ? 'success' : card.status === 'banned' ? 'error' : 'default'" style="margin-left: 10px">
          {{ statusLabels[card.status] ?? card.status }}
        </n-tag>
      </template>

      <n-descriptions :column="2" label-placement="left" size="small" bordered>
        <n-descriptions-item label="ID">{{ card.id }}</n-descriptions-item>
        <n-descriptions-item label="类型">{{ cardKindLabel(card.kind, durationSeconds) }}</n-descriptions-item>
        <n-descriptions-item label="到期">{{ formatExpiry(card.kind, card.expire_at, card.remaining_seconds) }}</n-descriptions-item>
        <n-descriptions-item label="剩余">
          <span v-if="card.kind === 'credit'">{{ card.points }} 点</span>
          <span v-else-if="card.kind === 'count'">{{ card.remain_times }} 次</span>
          <span v-else-if="card.kind === 'timed'">{{ Math.floor(card.remaining_seconds / 60) }} 分钟</span>
          <span v-else>不限</span>
        </n-descriptions-item>
        <n-descriptions-item label="设备">
          {{ card.bound_devices }} / {{ card.device_limit }}
          <n-tag v-if="card.verify_device === 0" size="small" type="warning" style="margin-left: 6px">不校验设备</n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="换绑已用">{{ card.rebind_used }} 次</n-descriptions-item>
        <n-descriptions-item label="激活">{{ card.activated_at ? formatTs(card.activated_at) : '未激活' }}</n-descriptions-item>
        <n-descriptions-item label="创建">{{ formatTs(card.created_at) }}</n-descriptions-item>
        <n-descriptions-item label="批次">{{ card.batch_id ?? '—' }}</n-descriptions-item>
        <n-descriptions-item label="代理">{{ card.agent_id ?? '平台直发' }}</n-descriptions-item>
        <n-descriptions-item label="备注" :span="2">{{ card.note || '—' }}</n-descriptions-item>
      </n-descriptions>

      <n-space style="margin-top: 14px" :wrap="true">
        <n-button size="small" type="primary" :loading="busy" @click="showExtend = true">续期 / 加点</n-button>
        <n-button size="small" :loading="busy" @click="openLimit">改设备上限</n-button>
        <n-button size="small" :loading="busy" @click="toggleVerifyDevice">
          {{ card.verify_device === 0 ? '开启设备校验' : '关闭设备校验' }}
        </n-button>
        <n-button size="small" :loading="busy" @click="act('unbind')">清空设备</n-button>
        <n-button size="small" :loading="busy" @click="act('reset-rebind')">重置换绑次数</n-button>
        <n-button size="small" type="error" :loading="busy" @click="act('ban')">封禁</n-button>
        <n-button size="small" :loading="busy" @click="act('unban')">解封</n-button>
        <n-button size="small" type="warning" :loading="busy" @click="act('recycle')">回收</n-button>
      </n-space>
    </n-card>

    <n-card title="设备" size="small" style="margin-bottom: 16px">
      <n-data-table :columns="deviceCols" :data="devices" size="small" :bordered="false" :scroll-x="700" />
    </n-card>

    <n-card title="会话" size="small">
      <n-data-table :columns="sessionCols" :data="sessions" size="small" :bordered="false" :scroll-x="700" />
    </n-card>

    <n-modal v-model:show="showLimit" preset="card" title="修改设备上限" style="width: min(360px, 94vw)">
      <n-space vertical>
        <span style="font-size: 13px; color: #666">当前 {{ card?.device_limit }} 台，改完立即对所有登录生效。</span>
        <n-input-number v-model:value="limitVal" :min="1" :max="100" />
        <n-button type="primary" :loading="busy" block @click="saveLimit">保存</n-button>
      </n-space>
    </n-modal>

    <n-modal v-model:show="showExtend" preset="card" title="续期 / 加点" style="width: min(420px, 94vw)">
      <n-space vertical>
        <div class="row"><span>加天数</span><n-input-number v-model:value="ext.days" :min="0" /></div>
        <div class="row"><span>加点数</span><n-input-number v-model:value="ext.points" :min="0" /></div>
        <div class="row"><span>加次数</span><n-input-number v-model:value="ext.times" :min="0" /></div>
        <div class="row"><span>加秒数</span><n-input-number v-model:value="ext.seconds" :min="0" /></div>
        <n-button type="primary" :loading="busy" block @click="extend">提交</n-button>
      </n-space>
    </n-modal>
  </div>
</template>

<style scoped>
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
</style>
