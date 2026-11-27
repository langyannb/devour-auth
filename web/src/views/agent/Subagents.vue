<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NInputNumber, NModal, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { agentApi } from '../../api'
import { formatYuan } from '../../utils'

interface Sub {
  id: number
  name: string
  parent_id: number | null
  level: number
  discount_bp: number
  rebate_bp: number
  monthly_quota_cents: number
  status: string
  is_self?: boolean
}

const rows = ref<Sub[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)
const showCreate = ref(false)
const busy = ref(false)
const form = ref({ name: '', password: '', discount: 80 })

const columns: DataTableColumns<Sub> = [
  {
    title: '代理',
    key: 'name',
    render: (r) =>
      h('span', {}, [
        r.name,
        r.is_self ? h(NTag, { size: 'tiny', type: 'info', style: 'margin-left:8px' }, { default: () => '我' }) : null,
      ]),
  },
  { title: '层级', key: 'level', width: 80, render: (r) => `L${r.level}` },
  { title: '折扣', key: 'discount_bp', width: 100, render: (r) => `${(r.discount_bp / 1000).toFixed(1)} 折` },
  { title: '返点', key: 'rebate_bp', width: 90, render: (r) => `${(r.rebate_bp / 100).toFixed(1)}%` },
  {
    title: '月额度',
    key: 'monthly_quota_cents',
    width: 120,
    render: (r) => (r.monthly_quota_cents > 0 ? `${formatYuan(r.monthly_quota_cents)} 元` : '不限'),
  },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (r) => h(NTag, { size: 'small', type: r.status === 'active' ? 'success' : 'error' },
      { default: () => (r.status === 'active' ? '正常' : '冻结') }),
  },
]

async function load() {
  loading.value = true
  try {
    rows.value = await agentApi.get<Sub[]>('/agent/api/subagents')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function createSub() {
  err.value = ''
  notice.value = ''
  busy.value = true
  try {
    await agentApi.post('/agent/api/subagents', {
      name: form.value.name,
      password: form.value.password,
      discount_bp: Math.round(form.value.discount * 100),
    })
    notice.value = `已开通下级 ${form.value.name}`
    showCreate.value = false
    form.value = { name: '', password: '', discount: 80 }
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px">
      <n-button type="primary" @click="showCreate = true">开通下级</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" :bordered="false" size="small" :scroll-x="620" />

    <n-modal v-model:show="showCreate" preset="card" title="开通下级代理" style="width: min(420px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="代理名">
          <n-input v-model:value="form.name" />
        </n-form-item>
        <n-form-item label="密码（至少 12 位）">
          <n-input v-model:value="form.password" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item label="折扣（%，不能低于你的成本）">
          <n-input-number v-model:value="form.discount" :min="1" :max="100" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="createSub">开通</n-button>
      </n-form>
    </n-modal>
  </div>
</template>
