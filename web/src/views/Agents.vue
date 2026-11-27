<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
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

const editTarget = ref<Agent | null>(null)
const editForm = ref({ quotaYuan: 0 })

const statusTag = (s: string) =>
  h(NTag, { size: 'small', type: s === 'active' ? 'success' : 'error' },
    { default: () => (s === 'active' ? '正常' : '冻结') })

const columns: DataTableColumns<Agent> = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '代理名', key: 'name' },
  { title: '层级', key: 'level', width: 70, render: (r) => `L${r.level}` },
  {
    title: '月额度',
    key: 'monthly_quota_cents',
    width: 120,
    render: (r) => (r.monthly_quota_cents > 0 ? `${formatYuan(r.monthly_quota_cents)} 元` : '不限'),
  },
  { title: '状态', key: 'status', width: 80, render: (r) => statusTag(r.status) },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  {
    title: '操作',
    key: 'op',
    width: 190,
    render: (r) =>
      h(NSpace, { size: 4 }, {
        default: () => [
          h(NButton, { size: 'tiny', onClick: () => router.push(`/agents/${r.id}`) }, { default: () => '价格表' }),
          h(NButton, { size: 'tiny', onClick: () => openEdit(r) }, { default: () => '改额度' }),
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

function openEdit(a: Agent) {
  editTarget.value = a
  editForm.value = { quotaYuan: a.monthly_quota_cents / 100 }
}

async function saveEdit() {
  if (!editTarget.value) return
  err.value = ''
  busy.value = true
  try {
    await api.patch(`/admin/api/agents/${editTarget.value.id}`, {
      monthly_quota_cents: Math.round(editForm.value.quotaYuan * 100),
    })
    notice.value = '已更新代理策略'
    editTarget.value = null
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
        <n-form-item label="月额度（元，0 = 不限）">
          <n-input-number v-model:value="createForm.quotaYuan" :min="0" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="create">创建</n-button>
      </n-form>
    </n-modal>

    <n-modal :show="!!editTarget" preset="card" title="调整代理策略" style="width: min(420px, 94vw)"
      @update:show="(v: boolean) => { if (!v) editTarget = null }">
      <n-form label-placement="top">
        <n-form-item label="月额度（元，0 = 不限）">
          <n-input-number v-model:value="editForm.quotaYuan" :min="0" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="saveEdit">保存</n-button>
      </n-form>
    </n-modal>
  </div>
</template>
