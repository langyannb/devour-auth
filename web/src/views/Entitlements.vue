<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NInputNumber, NModal, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Ent {
  id: number
  code: string
  name: string
  cost_points: number
  cost_times: number
  active: number
  created_at: number
}

const rows = ref<Ent[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)
const showCreate = ref(false)
const busy = ref(false)
const form = ref({ code: '', name: '', cost_points: 0, cost_times: 0 })

async function load() {
  loading.value = true
  err.value = ''
  try {
    rows.value = await api.get<Ent[]>('/admin/api/entitlements')
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
    await api.post('/admin/api/entitlements', form.value)
    notice.value = `已创建权限 ${form.value.code}`
    showCreate.value = false
    form.value = { code: '', name: '', cost_points: 0, cost_times: 0 }
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

const columns: DataTableColumns<Ent> = [
  { title: 'ID', key: 'id', width: 64 },
  { title: 'code', key: 'code', width: 160 },
  { title: '名称', key: 'name' },
  { title: '扣点', key: 'cost_points', width: 80 },
  { title: '扣次', key: 'cost_times', width: 80 },
  {
    title: '状态',
    key: 'active',
    width: 80,
    render: (r) => h(NTag, { size: 'small', type: r.active ? 'success' : 'default' }, { default: () => (r.active ? '启用' : '停用') }),
  },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
]

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px">
      <n-button type="primary" @click="showCreate = true">新建权限</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="760" />

    <n-modal v-model:show="showCreate" preset="card" title="新建功能权限" style="width: min(440px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="code（客户端用这个判断功能）">
          <n-input v-model:value="form.code" placeholder="aim_assist" />
        </n-form-item>
        <n-form-item label="名称">
          <n-input v-model:value="form.name" placeholder="辅助瞄准" />
        </n-form-item>
        <n-form-item label="每次消耗点数">
          <n-input-number v-model:value="form.cost_points" :min="0" />
        </n-form-item>
        <n-form-item label="每次消耗次数">
          <n-input-number v-model:value="form.cost_times" :min="0" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="create">创建</n-button>
      </n-form>
    </n-modal>
  </div>
</template>
