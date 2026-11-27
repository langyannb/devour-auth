<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NModal, NSelect, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Hook { id: number; url: string; events: string; active: number; created_at: number }

const rows = ref<Hook[]>([])
const err = ref('')
const notice = ref('')
const secret = ref('')
const loading = ref(false)
const showCreate = ref(false)
const busy = ref(false)
const form = ref({ url: '', events: ['card.ban'] })

const eventOptions = [
  { label: '卡密封禁 card.ban', value: 'card.ban' },
  { label: '卡密解封 card.unban', value: 'card.unban' },
  { label: '卡密回收 card.recycle', value: 'card.recycle' },
  { label: '续期 card.extend', value: 'card.extend' },
  { label: '解绑 card.unbind', value: 'card.unbind' },
]

async function load() {
  loading.value = true
  err.value = ''
  try {
    rows.value = await api.get<Hook[]>('/admin/api/webhooks')
  } catch (e) { err.value = (e as Error).message } finally { loading.value = false }
}

async function create() {
  err.value = ''
  secret.value = ''
  busy.value = true
  try {
    const d = await api.post<{ secret: string }>('/admin/api/webhooks', {
      url: form.value.url,
      events: form.value.events.join(','),
    })
    secret.value = d.secret
    notice.value = '已创建回调'
    await load()
  } catch (e) { err.value = (e as Error).message } finally { busy.value = false }
}

async function toggle(k: Hook) {
  try { await api.post(`/admin/api/webhooks/${k.id}/toggle`, { active: !k.active }); await load() } catch (e) { err.value = (e as Error).message }
}

async function del(k: Hook) {
  try { await api.delete(`/admin/api/webhooks/${k.id}`); await load() } catch (e) { err.value = (e as Error).message }
}

const cols: DataTableColumns<Hook> = [
  { title: 'URL', key: 'url' },
  { title: '事件', key: 'events', width: 220 },
  {
    title: '状态', key: 'active', width: 80,
    render: (r) => h(NTag, { size: 'small', type: r.active ? 'success' : 'default' }, { default: () => (r.active ? '启用' : '停用') }),
  },
  { title: '创建', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  {
    title: '操作', key: 'op', width: 140,
    render: (r) => h(NSpace, { size: 4 }, {
      default: () => [
        h(NButton, { size: 'tiny', onClick: () => toggle(r) }, { default: () => (r.active ? '停用' : '启用') }),
        h(NButton, { size: 'tiny', type: 'error', onClick: () => del(r) }, { default: () => '删除' }),
      ],
    }),
  },
]

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px">
      <n-button type="primary" @click="showCreate = true">新建回调</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>
    <n-alert v-if="secret" type="warning" style="margin-bottom: 12px">
      <div style="font-size: 12px; color: #888">校验签名用的 secret（只显示这一次）：</div>
      <div style="font-family: monospace; word-break: break-all">{{ secret }}</div>
      <div style="font-size: 12px; color: #888; margin-top: 6px">
        对方收到请求后，用 `hex(hmac_sha256(secret, body))` 与头 `X-Devour-Signature` 比对。
      </div>
    </n-alert>

    <n-data-table :columns="cols" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="820" />

    <n-modal v-model:show="showCreate" preset="card" title="新建回调" style="width: min(480px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="回调 URL">
          <n-input v-model:value="form.url" placeholder="https://your-server/hook" />
        </n-form-item>
        <n-form-item label="订阅事件">
          <n-select v-model:value="form.events" multiple :options="eventOptions" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="create">创建</n-button>
      </n-form>
    </n-modal>
  </div>
</template>
