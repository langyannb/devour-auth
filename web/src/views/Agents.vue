<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NInputNumber,
  NModal, NSpace, NTag, type DataTableColumns,
} from 'naive-ui'
import { api } from '../api'
import { formatTs, formatYuan } from '../utils'

interface Agent {
  id: number
  name: string
  parent_id: number | null
  level: number
  discount_bp: number
  rebate_bp: number
  monthly_quota_cents: number
  quota_cents: number
  status: string
  created_at: number
}

const router = useRouter()
const rows = ref<Agent[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)

const showCreate = ref(false)
const createForm = ref({ name: '', password: '', quotaYuan: 0 })
const busy = ref(false)

const quotaDlg = ref<{ agent: Agent; mode: 'add' | 'sub' } | null>(null)
const quotaAmount = ref(100)
const quotaNote = ref('')

const statusTag = (s: string) =>
  h(NTag, { size: 'small', type: s === 'active' ? 'success' : 'error' },
    { default: () => (s === 'active' ? '正常' : '冻结') })

const columns: DataTableColumns<Agent> = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '代理名', key: 'name' },
  { title: '层级', key: 'level', width: 70, render: (r) => `L${r.level}` },
  {
    title: '额度',
    key: 'quota_cents',
    width: 130,
    render: (r) =>
      r.quota_cents < 0
        ? '不限'
        : r.quota_cents === 0
          ? '0（不能出卡）'
          : `${formatYuan(r.quota_cents)} 元`,
  },
  { title: '状态', key: 'status', width: 80, render: (r) => statusTag(r.status) },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  {
    title: '操作',
    key: 'op',
    width: 260,
    render: (r) =>
      h(NSpace, { size: 4 }, {
        default: () => [
          h(NButton, { size: 'tiny', onClick: () => router.push(`/agents/${r.id}`) }, { default: () => '价格表' }),
          h(NButton, { size: 'tiny', type: 'primary', onClick: () => openQuota(r, 'add') }, { default: () => '加' }),
          h(NButton, { size: 'tiny', type: 'warning', onClick: () => openQuota(r, 'sub') }, { default: () => '扣' }),
          h(NButton, {
            size: 'tiny',
            type: r.status === 'active' ? 'error' : 'success',
            onClick: () => toggleStatus(r),
          }, { default: () => (r.status === 'active' ? '冻结' : '解冻') }),
        ],
      }),
  },
]

async function load() {
  loading.value = true
  err.value = ''
  try {
    rows.value = await api.get<Agent[]>('/admin/api/agents')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function create() {
  err.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api.post('/admin/api/agents', {
      name: createForm.value.name,
      password: createForm.value.password,
      monthly_quota_cents: Math.round(createForm.value.quotaYuan * 100),
    })
    notice.value = `已创建代理 ${createForm.value.name}`
    showCreate.value = false
    createForm.value = { name: '', password: '', quotaYuan: 0 }
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function openQuota(a: Agent, mode: 'add' | 'sub') {
  quotaDlg.value = { agent: a, mode }
  quotaAmount.value = 100
  quotaNote.value = ''
}

const quotaTitle = computed(() => (quotaDlg.value?.mode === 'sub' ? '扣额度' : '加额度'))

const quotaDelta = computed(() => {
  const amount = Math.round((quotaAmount.value || 0) * 100)
  return quotaDlg.value?.mode === 'sub' ? -amount : amount
})

async function submitQuota() {
  if (!quotaDlg.value) return
  err.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api.post(`/admin/api/agents/${quotaDlg.value.agent.id}/quota`, {
      delta_cents: quotaDelta.value,
      note: quotaNote.value,
    })
    notice.value = quotaDlg.value.mode === 'sub' ? '额度已扣减' : '额度已增加'
    quotaDlg.value = null
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function toggleStatus(a: Agent) {
  const action = a.status === 'active' ? 'freeze' : 'unfreeze'
  try {
    await api.post(`/admin/api/agents/${a.id}/${action}`)
    await load()
  } catch (e) {
    err.value = (e as Error).message
  }
}

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px" :wrap="true">
      <n-button type="primary" @click="showCreate = true">新建代理</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-data-table :columns="columns" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="960" />

    <n-modal v-model:show="showCreate" preset="card" title="新建代理" style="width: min(460px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="代理名">
          <n-input v-model:value="createForm.name" placeholder="agent01" />
        </n-form-item>
        <n-form-item label="密码（至少 12 位）">
          <n-input v-model:value="createForm.password" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item label="初始额度（元）">
          <n-input-number v-model:value="createForm.quotaYuan" :min="-1" />
          <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">0 = 不能出卡，-1 = 不限</span>
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="create">创建</n-button>
      </n-form>
    </n-modal>

    <n-modal :show="!!quotaDlg" preset="card" :title="quotaTitle" style="width: min(420px, 94vw)"
      @update:show="(v: boolean) => { if (!v) quotaDlg = null }">
      <n-form label-placement="top">
        <n-form-item label="金额（元）">
          <n-input-number v-model:value="quotaAmount" :min="0.01" />
        </n-form-item>
        <n-form-item label="备注（可选）">
          <n-input v-model:value="quotaNote" placeholder="例如：微信转账 500" />
        </n-form-item>
        <div style="font-size: 12px; color: #8b93a7; margin-bottom: 12px">
          当前 {{ quotaDlg ? (quotaDlg.agent.quota_cents < 0 ? '不限' : formatYuan(quotaDlg.agent.quota_cents) + ' 元') : '' }}
          → 操作后 {{ quotaDlg ? (quotaDlg.agent.quota_cents < 0 ? '不限' : formatYuan(Math.max(0, quotaDlg.agent.quota_cents + quotaDelta)) + ' 元') : '' }}
        </div>
        <n-button type="primary" :loading="busy" block @click="submitQuota">确认</n-button>
      </n-form>
    </n-modal>

  </div>
</template>
