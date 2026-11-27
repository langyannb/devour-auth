<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NModal, NSelect, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Acc { id: number; username: string; role: string; totp_enabled: boolean; created_at: number; unused_recovery_codes: number }

const rows = ref<Acc[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)
const showCreate = ref(false)
const busy = ref(false)
const form = ref({ username: '', password: '', role: 'operator' })
const secrets = ref<{ username: string; password: string; codes: string[] } | null>(null)

async function load() {
  loading.value = true
  err.value = ''
  try { rows.value = await api.get<Acc[]>('/admin/api/accounts') } catch (e) { err.value = (e as Error).message } finally { loading.value = false }
}

async function create() {
  err.value = ''
  busy.value = true
  try {
    const d = await api.post<{ username: string; password: string; recovery_codes: string[] }>('/admin/api/accounts', form.value)
    secrets.value = { username: d.username, password: d.password, codes: d.recovery_codes }
    showCreate.value = false
    form.value = { username: '', password: '', role: 'operator' }
    await load()
  } catch (e) { err.value = (e as Error).message } finally { busy.value = false }
}

async function reset(a: Acc) {
  err.value = ''
  try {
    const d = await api.post<{ username: string; password: string; recovery_codes: string[] }>(`/admin/api/accounts/${a.id}/reset-password`)
    secrets.value = { username: d.username, password: d.password, codes: d.recovery_codes }
    await load()
  } catch (e) { err.value = (e as Error).message }
}

const cols: DataTableColumns<Acc> = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '用户名', key: 'username', width: 160 },
  { title: '角色', key: 'role', width: 100 },
  { title: 'TOTP', key: 'totp_enabled', width: 80, render: (r) => (r.totp_enabled ? '已启用' : '未启用') },
  { title: '恢复码剩余', key: 'unused_recovery_codes', width: 110 },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  {
    title: '操作', key: 'op', width: 120,
    render: (r) => h(NButton, { size: 'tiny', onClick: () => reset(r) }, { default: () => '重置密码' }),
  },
]

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px">
      <n-button type="primary" @click="showCreate = true">新建账号</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-alert v-if="secrets" type="warning" style="margin-bottom: 12px">
      <div style="font-size: 13px">账号 <b>{{ secrets.username }}</b> 的凭据（只显示这一次）：</div>
      <div style="font-family: monospace; margin: 6px 0">密码：{{ secrets.password }}</div>
      <div style="font-size: 12px; color: #888">恢复码（TOTP 丢失时登录用，每个只能用一次）：</div>
      <div style="font-family: monospace; display: grid; grid-template-columns: repeat(2, 1fr); gap: 2px 16px; margin-top: 4px">
        <span v-for="c in secrets.codes" :key="c">{{ c }}</span>
      </div>
      <n-button size="small" style="margin-top: 8px" @click="secrets = null">我已保存，关闭</n-button>
    </n-alert>

    <n-data-table :columns="cols" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="800" />

    <n-modal v-model:show="showCreate" preset="card" title="新建管理账号" style="width: min(440px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="用户名">
          <n-input v-model:value="form.username" />
        </n-form-item>
        <n-form-item label="角色">
          <n-select v-model:value="form.role" :options="[{label:'operator（不能改价/发卡/管账号）',value:'operator'},{label:'admin（全部）',value:'admin'}]" />
        </n-form-item>
        <n-form-item label="密码（留空自动生成 16 位）">
          <n-input v-model:value="form.password" placeholder="留空则随机生成" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="create">创建</n-button>
      </n-form>
    </n-modal>
  </div>
</template>
