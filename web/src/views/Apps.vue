<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NModal, NSpace, NTag, type DataTableColumns } from 'naive-ui'
import { adminAppID, api } from '../api'
import { copyText, formatTs } from '../utils'

interface AppItem { id: number; code: string; name: string; active: number; created_at: number }

const rows = ref<AppItem[]>([])
const scoped = computed(() => adminAppID.value > 0)
const err = ref('')
const notice = ref('')
const loading = ref(false)
const showCreate = ref(false)
const busy = ref(false)
const form = ref({ code: '', name: '' })

// 一键开通开发者：应用 + 管理员 + 开放接口密钥，结果一次性展示
interface Tenant {
  app: { id: number; code: string; name: string }
  admin: { username: string; password: string; recovery_codes: string[] }
  api_key: { key_id: string; secret: string; scopes: string }
}
const showTenant = ref(false)
const tenantForm = ref({ name: '', code: '', username: '', password: '' })
const tenant = ref<Tenant | null>(null)
const tenantText = ref('')
const copied = ref(false)

const adminLoginURL = location.hostname.startsWith('admin.')
  ? 'https://admin.namikao.com/login'
  : `${location.origin}/login`

function buildTenantText(t: Tenant) {
  const lines = [
    '验证服务已开通',
    '',
    `管理端：${adminLoginURL}`,
    `用户名：${t.admin.username}`,
    `密码：${t.admin.password}`,
    '',
    `应用 code：${t.app.code}`,
    `客户端登录请求要带 "app": "${t.app.code}"`,
    '',
    '开放接口（发货系统用）：',
    `X-Api-Key: ${t.api_key.key_id}`,
    `secret: ${t.api_key.secret}`,
    `签名规则见对接文档（hmac_sha256，method/path/ts/nonce/body）`,
    '',
    '对接文档：代码仓库 docs/integration.md（客户端第 3 节、开放 API 第 7 节）',
    '',
    '恢复码（找回账号用，各只能用一次）：',
    ...t.admin.recovery_codes,
  ]
  return lines.join('\n')
}

async function createTenant() {
  err.value = ''
  busy.value = true
  try {
    const t = await api.post<Tenant>('/admin/api/tenants', tenantForm.value)
    tenant.value = t
    tenantText.value = buildTenantText(t)
    copied.value = false
    showTenant.value = false
    tenantForm.value = { name: '', code: '', username: '', password: '' }
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function copyTenant() {
  if (await copyText(tenantText.value)) copied.value = true
  else err.value = '复制失败，请手动选中'
}

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
      <n-button v-if="!scoped" type="primary" @click="showTenant = true">一键开通开发者</n-button>
      <n-button v-if="!scoped" @click="showCreate = true">新建应用</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" size="small" :bordered="false" :scroll-x="700" />

    <n-modal v-model:show="showTenant" preset="card" title="一键开通开发者" style="width: min(480px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="应用名称">
          <n-input v-model:value="tenantForm.name" placeholder="他的软件" />
        </n-form-item>
        <n-form-item label="应用 code（客户端请求要带它）">
          <n-input v-model:value="tenantForm.code" placeholder="friend" />
        </n-form-item>
        <n-form-item label="管理员用户名">
          <n-input v-model:value="tenantForm.username" placeholder="friend-dev" />
        </n-form-item>
        <n-form-item label="密码（留空自动生成）">
          <n-input v-model:value="tenantForm.password" placeholder="留空则随机生成" />
        </n-form-item>
      </n-form>
      <p style="margin: 0 0 12px; font-size: 12px; color: #8b93a7">
        会一次建好：应用、绑定该应用的管理员（不需要动态验证码）、开放接口密钥。
        密码只显示这一次，密钥随时能在「对接信息」查看。
      </p>
      <n-space justify="end">
        <n-button @click="showTenant = false">取消</n-button>
        <n-button type="primary" :loading="busy" @click="createTenant">开通</n-button>
      </n-space>
    </n-modal>

    <n-modal :show="tenant !== null" preset="card" title="已开通，复制发给对方" style="width: min(620px, 94vw)"
      @update:show="(v: boolean) => { if (!v) tenant = null }">
      <p style="margin: 0 0 10px; font-size: 12px; color: #c2410c">密码和恢复码只显示这一次；API 密钥随时可在「对接信息」查看。</p>
      <pre class="onboard">{{ tenantText }}</pre>
      <n-space justify="end" style="margin-top: 12px">
        <n-button @click="tenant = null">关闭</n-button>
        <n-button type="primary" @click="copyTenant">{{ copied ? '已复制' : '复制全部' }}</n-button>
      </n-space>
    </n-modal>

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

<style scoped>
.onboard {
  margin: 0;
  max-height: 50vh;
  overflow: auto;
  padding: 14px;
  border-radius: 10px;
  background: #f7faf8;
  border: 1px solid #e8eeeb;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12.5px;
  line-height: 1.8;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
