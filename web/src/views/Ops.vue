<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import {
  NAlert, NButton, NCard, NDataTable, NForm, NFormItem, NInput, NInputNumber, NSelect,
  NSpace, NSwitch, NTabPane, NTabs, NTag, type DataTableColumns,
} from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Release {
  id: number
  version: string
  info: string
  url: string
  must: number
  rollout_bp: number
  active: number
  created_at: number
}

interface Notice {
  id: number
  kind: string
  title: string
  body: string
  active: number
  created_at: number
}

const err = ref('')
const noticeOk = ref('')

const releases = ref<Release[]>([])
const notices = ref<Notice[]>([])
const flags = ref<Record<string, string>>({})
const flagRows = ref<{ key: string; value: string }[]>([])

const relForm = ref({ version: '', info: '', url: '', must: false, rollout_bp: 10000 })
const noticeForm = ref({ kind: 'list', title: '', body: '' })
const newFlag = ref({ key: '', value: '' })

const releaseCols: DataTableColumns<Release> = [
  { title: '版本', key: 'version', width: 110 },
  { title: '说明', key: 'info' },
  { title: '强更', key: 'must', width: 70, render: (r) => h(NTag, { size: 'small', type: r.must ? 'warning' : 'default' }, { default: () => (r.must ? '是' : '否') }) },
  { title: '灰度', key: 'rollout_bp', width: 90, render: (r) => `${(r.rollout_bp / 100).toFixed(0)}%` },
  { title: '状态', key: 'active', width: 80, render: (r) => h(NTag, { size: 'small', type: r.active ? 'success' : 'default' }, { default: () => (r.active ? '启用' : '停用') }) },
  { title: '时间', key: 'created_at', width: 150, render: (r) => formatTs(r.created_at) },
  {
    title: '操作',
    key: 'op',
    width: 90,
    render: (r) =>
      h(NButton, { size: 'tiny', onClick: () => toggleRelease(r) }, { default: () => (r.active ? '停用' : '启用') }),
  },
]

const noticeCols: DataTableColumns<Notice> = [
  { title: '类型', key: 'kind', width: 100, render: (r) => ({ list: '列表', popup: '弹窗', immediate: '立即' }[r.kind] ?? r.kind) },
  { title: '标题', key: 'title', width: 160 },
  { title: '正文', key: 'body' },
  { title: '状态', key: 'active', width: 80, render: (r) => h(NTag, { size: 'small', type: r.active ? 'success' : 'default' }, { default: () => (r.active ? '启用' : '停用') }) },
  {
    title: '操作',
    key: 'op',
    width: 90,
    render: (r) => h(NButton, { size: 'tiny', onClick: () => toggleNotice(r) }, { default: () => (r.active ? '停用' : '启用') }),
  },
]

async function loadAll() {
  err.value = ''
  try {
    const [r, n, f] = await Promise.all([
      api.get<Release[]>('/admin/api/releases'),
      api.get<Notice[]>('/admin/api/notices'),
      api.get<Record<string, string>>('/admin/api/flags'),
    ])
    releases.value = r
    notices.value = n
    flags.value = f
    flagRows.value = Object.entries(f).map(([key, value]) => ({ key, value }))
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function createRelease() {
  err.value = ''
  try {
    await api.post('/admin/api/releases', relForm.value)
    relForm.value = { version: '', info: '', url: '', must: false, rollout_bp: 10000 }
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function toggleRelease(r: Release) {
  try {
    await api.post(`/admin/api/releases/${r.id}/toggle`, { active: !r.active })
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function createNotice() {
  err.value = ''
  noticeOk.value = ''
  try {
    await api.post('/admin/api/notices', noticeForm.value)
    noticeOk.value = '公告已创建'
    noticeForm.value = { kind: 'list', title: '', body: '' }
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function toggleNotice(n: Notice) {
  try {
    await api.post(`/admin/api/notices/${n.id}/toggle`, { active: !n.active })
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function saveNewFlag() {
  err.value = ''
  if (!newFlag.value.key) {
    err.value = '请填 key'
    return
  }
  try {
    await api.put('/admin/api/flags', newFlag.value)
    newFlag.value = { key: '', value: '' }
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function saveFlagRow(row: { key: string; value: string }) {
  try {
    await api.put('/admin/api/flags', row)
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

onMounted(loadAll)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>

    <n-tabs type="line" animated>
      <n-tab-pane name="releases" tab="版本">
        <n-card size="small" title="发布新版本" style="margin-bottom: 16px">
          <n-form :show-feedback="false">
            <n-space :wrap="true" align="end">
              <n-form-item label="版本号">
                <n-input v-model:value="relForm.version" placeholder="2.0.1" style="width: 130px" />
              </n-form-item>
              <n-form-item label="说明">
                <n-input v-model:value="relForm.info" placeholder="修复登录" style="width: 200px" />
              </n-form-item>
              <n-form-item label="下载地址">
                <n-input v-model:value="relForm.url" placeholder="https://..." style="width: 240px" />
              </n-form-item>
              <n-form-item label="灰度(千分比)">
                <n-input-number v-model:value="relForm.rollout_bp" :min="1" :max="10000" style="width: 130px" />
              </n-form-item>
              <n-form-item label="强制更新">
                <n-switch v-model:value="relForm.must" />
              </n-form-item>
              <n-button type="primary" @click="createRelease">发布</n-button>
            </n-space>
          </n-form>
        </n-card>
        <n-data-table :columns="releaseCols" :data="releases" size="small" :bordered="false" :scroll-x="820" />
      </n-tab-pane>

      <n-tab-pane name="notices" tab="公告">
        <n-alert v-if="noticeOk" type="success" style="margin-bottom: 12px">{{ noticeOk }}</n-alert>
        <n-card size="small" title="新建公告" style="margin-bottom: 16px">
          <n-form :show-feedback="false">
            <n-space :wrap="true" align="end">
              <n-form-item label="类型">
                <n-select v-model:value="noticeForm.kind" :options="[
                  { label: '列表', value: 'list' },
                  { label: '弹窗', value: 'popup' },
                  { label: '立即', value: 'immediate' },
                ]" style="width: 120px" />
              </n-form-item>
              <n-form-item label="标题">
                <n-input v-model:value="noticeForm.title" style="width: 180px" />
              </n-form-item>
              <n-form-item label="正文">
                <n-input v-model:value="noticeForm.body" style="width: 280px" />
              </n-form-item>
              <n-button type="primary" @click="createNotice">发布</n-button>
            </n-space>
          </n-form>
        </n-card>
        <n-data-table :columns="noticeCols" :data="notices" size="small" :bordered="false" :scroll-x="720" />
      </n-tab-pane>

      <n-tab-pane name="flags" tab="远程变量">
        <n-card size="small" title="添加 / 修改变量" style="margin-bottom: 16px">
          <n-space :wrap="true" align="end">
            <n-input v-model:value="newFlag.key" placeholder="key，例如 maintenance" style="width: 220px" />
            <n-input v-model:value="newFlag.value" placeholder="value" style="width: 220px" />
            <n-button type="primary" @click="saveNewFlag">保存</n-button>
          </n-space>
          <p style="color: #8b93a7; font-size: 12px; margin: 10px 0 0">
            `maintenance=1` 时客户端 `/v1/meta` 返回维护中。
          </p>
        </n-card>
        <n-card size="small" title="已有变量">
          <div v-if="flagRows.length === 0" style="color: #888">还没有变量</div>
          <div v-for="row in flagRows" :key="row.key" class="flag-row">
            <code>{{ row.key }}</code>
            <n-input v-model:value="row.value" size="small" style="width: 220px" />
            <n-button size="tiny" @click="saveFlagRow(row)">保存</n-button>
          </div>
        </n-card>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
.flag-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 6px 0;
}
.flag-row code {
  font-family: ui-monospace, monospace;
  font-size: 13px;
  min-width: 160px;
}
</style>
