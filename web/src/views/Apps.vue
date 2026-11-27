<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NModal, NSpace, NSwitch, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface AppItem { id: number; code: string; name: string; active: number; created_at: number }

const rows = ref<AppItem[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)
const showCreate = ref(false)
const busy = ref(false)
const form = ref({ code: '', name: '' })

async function load() {
  loading.value = true
  err.value = ''
  try {
    rows.value = await api.get<AppItem[]>('/admin/api/apps')
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
    await api.post('/admin/api/apps', form.value)
    notice.value = `已创建应用 ${form.value.name}`
    showCreate.value = false
    form.value = { code: '', name: '' }
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function toggle(a: AppItem) {
  try {
    await api.patch(`/admin/api/apps/${a.id}`, { active: a.active ? 0 : 1 })
    await load()
  } catch (e) {
    err.value = (e as Error).message
  }
}

const columns: DataTableColumns<AppItem> = [
  { title: 'ID', key: 'id', width: 64 },
  { title: 'code', key: 'code', width: 160 },
  { title: '名称', key: 'name' },
  {
    title: '状态',
    key: 'active',
    width: 90,
    render: (r) => h(NTag, { size: 'small', type: r.active ? 'success' : 'default' }, { default: () => (r.active ? '启用' : '停用') }),
  },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  {
    title: '操作',
    key: 'op',
    width: 90,
    render: (r) => h(NButton, { size: 'tiny', onClick: () => toggle(r) }, { default: () => (r.active ? '停用' : '启用') }),
  },
]

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px">
      <n-button type="primary" @click="showCreate = true">新建应用</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="700" />

    <n-modal v-model:show="showCreate" preset="card" title="新建应用" style="width: min(420px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="code（小写字母数字，创建后不可改）">
          <n-input v-model:value="form.code" placeholder="mysoftware" />
        </n-form-item>
        <n-form-item label="显示名称">
          <n-input v-model:value="form.name" placeholder="我的软件" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="create">创建</n-button>
      </n-form>
    </n-modal>
  </div>
</template>
