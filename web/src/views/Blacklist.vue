<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NInputNumber, NModal, NSelect, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Entry { id: number; kind: string; value: string; reason: string; expires_at: number; created_at: number }

const rows = ref<Entry[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)
const showAdd = ref(false)
const busy = ref(false)
const form = ref({ kind: 'ip', value: '', reason: '', hours: 0 })

async function load() {
  loading.value = true
  err.value = ''
  try {
    rows.value = await api.get<Entry[]>('/admin/api/blacklist')
  } catch (e) { err.value = (e as Error).message } finally { loading.value = false }
}

async function add() {
  err.value = ''
  busy.value = true
  try {
    const expires = form.value.hours > 0 ? Math.floor(Date.now() / 1000) + form.value.hours * 3600 : 0
    await api.post('/admin/api/blacklist', { kind: form.value.kind, value: form.value.value, reason: form.value.reason, expires_at: expires })
    notice.value = '已加入黑名单'
    showAdd.value = false
    form.value = { kind: 'ip', value: '', reason: '', hours: 0 }
    await load()
  } catch (e) { err.value = (e as Error).message } finally { busy.value = false }
}

async function del(e: Entry) {
  try { await api.delete(`/admin/api/blacklist/${e.id}`); await load() } catch (x) { err.value = (x as Error).message }
}

const cols: DataTableColumns<Entry> = [
  { title: '类型', key: 'kind', width: 90, render: (r) => ({ ip: 'IP', device: '设备', card: '卡密' }[r.kind] ?? r.kind) },
  { title: '值', key: 'value', width: 220 },
  { title: '原因', key: 'reason' },
  { title: '到期', key: 'expires_at', width: 150, render: (r) => (r.expires_at ? formatTs(r.expires_at) : '永久') },
  { title: '添加', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  { title: '操作', key: 'op', width: 90, render: (r) => h(NButton, { size: 'tiny', type: 'error', onClick: () => del(r) }, { default: () => '移除' }) },
]

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px">
      <n-button type="primary" @click="showAdd = true">加入黑名单</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>
    <n-data-table :columns="cols" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="880" />

    <n-modal v-model:show="showAdd" preset="card" title="加入黑名单" style="width: min(440px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="类型">
          <n-select v-model:value="form.kind" :options="[{label:'IP',value:'ip'},{label:'设备指纹',value:'device'},{label:'卡密',value:'card'}]" />
        </n-form-item>
        <n-form-item label="值">
          <n-input v-model:value="form.value" placeholder="1.2.3.4 / 设备指纹 / 卡密" />
        </n-form-item>
        <n-form-item label="原因（可选）">
          <n-input v-model:value="form.reason" />
        </n-form-item>
        <n-form-item label="有效小时（0 = 永久）">
          <n-input-number v-model:value="form.hours" :min="0" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="add">确认</n-button>
      </n-form>
    </n-modal>
  </div>
</template>
