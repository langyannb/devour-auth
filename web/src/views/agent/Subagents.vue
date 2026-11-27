<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import {
  NAlert, NButton, NDataTable, NForm, NFormItem, NInput, NInputNumber, NModal,
  NSelect, NSpace, NSwitch, NTag, type DataTableColumns,
} from 'naive-ui'
import { agentApi } from '../../api'
import { useMobile } from '../../useMobile'
import { copyText, formatYuan } from '../../utils'

interface Perms {
  SetDeviceLimit: boolean
  CustomKami: boolean
  MaxQty: number
}

interface Sub {
  id: number
  name: string
  level: number
  quota_cents: number
  status: string
  plan_id?: number
  plan_name?: string
  permissions?: Perms
  is_self?: boolean
  login_link?: string
  activated?: boolean
}

interface PlanItem { kind: string; name: string; price_cents: number }
interface Plan { id: number; name: string; items: PlanItem[] }

const rows = ref<Sub[]>([])
const plans = ref<Plan[]>([])
const err = ref('')
const notice = ref('')
const loading = ref(false)
const busy = ref(false)
const isMobile = useMobile()

const showCreate = ref(false)
const form = ref({ name: '', password: '' })

const quotaDlg = ref<Sub | null>(null)
const linkDlg = ref<{ name: string; link: string } | null>(null)
const quotaYuan = ref(100)
const planDlg = ref<Sub | null>(null)
const planValue = ref<number | null>(null)
const permDlg = ref<Sub | null>(null)
const permForm = ref<Perms>({ SetDeviceLimit: false, CustomKami: false, MaxQty: 10 })

const statusTag = (s: string) =>
  h(NTag, { size: 'small', type: s === 'active' ? 'success' : 'error' },
    { default: () => (s === 'active' ? '正常' : '冻结') })

const columns: DataTableColumns<Sub> = [
  {
    title: '代理',
    key: 'name',
    render: (r) =>
      h('span', {}, [
        r.name,
        r.is_self ? h(NTag, { size: 'tiny', type: 'info', style: 'margin-left:8px' }, { default: () => '我' }) : null,
        r.level === 2 ? h(NTag, { size: 'tiny', style: 'margin-left:6px' }, { default: () => '下级' }) : null,
        !r.is_self && r.activated === false
          ? h(NTag, { size: 'tiny', type: 'warning', style: 'margin-left:6px' }, { default: () => '待激活' })
          : null,
      ]),
  },
  {
    title: '额度',
    key: 'quota_cents',
    width: 110,
    render: (r) => (r.quota_cents < 0 ? '不限' : `${formatYuan(r.quota_cents)} 元`),
  },
  { title: '模板', key: 'plan_name', width: 120, render: (r) => r.plan_name ?? '未设置' },
  { title: '状态', key: 'status', width: 80, render: (r) => statusTag(r.status) },
  {
    title: '操作',
    key: 'op',
    width: 320,
    render: (r) =>
      r.is_self
        ? '—'
        : h(NSpace, { size: 4 }, {
            default: () => [
              h(NButton, { size: 'tiny', onClick: () => showLink(r) }, { default: () => '登录链接' }),
              h(NButton, { size: 'tiny', type: 'primary', onClick: () => openQuota(r) }, { default: () => '划额度' }),
              h(NButton, { size: 'tiny', onClick: () => openPlan(r) }, { default: () => '设模板' }),
              h(NButton, { size: 'tiny', onClick: () => openPerms(r) }, { default: () => '设权限' }),
              h(NButton, {
                size: 'tiny',
                type: r.status === 'active' ? 'error' : 'success',
                onClick: () => toggle(r),
              }, { default: () => (r.status === 'active' ? '冻结' : '解冻') }),
            ],
          }),
  },
]

async function load() {
  loading.value = true
  err.value = ''
  try {
    const [subs, planList] = await Promise.all([
      agentApi.get<Sub[]>('/agent/api/subagents'),
      agentApi.get<Plan[]>('/agent/api/plans'),
    ])
    rows.value = subs
    plans.value = planList
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
    const d = await agentApi.post<{ id: number; name: string; login_link: string }>('/agent/api/subagents', {
      name: form.value.name,
      password: form.value.password,
    })
    notice.value = `已开通下级 ${form.value.name}，把登录链接发给他`
    showCreate.value = false
    form.value = { name: '', password: '' }
    if (d.login_link) linkDlg.value = { name: d.name, link: d.login_link }
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function showLink(r: Sub) {
  err.value = ''
  let link = r.login_link
  if (!link) {
    try {
      const d = await agentApi.post<{ login_link: string }>(`/agent/api/subagents/${r.id}/link`)
      link = d.login_link
      await load()
    } catch (e) {
      err.value = (e as Error).message
      return
    }
  }
  linkDlg.value = { name: r.name, link }
}

async function copyLink() {
  if (!linkDlg.value) return
  if (await copyText(linkDlg.value.link)) notice.value = '链接已复制，发给下级即可'
  else err.value = '复制失败，请手动选中链接'
}

function openQuota(r: Sub) {
  quotaDlg.value = r
  quotaYuan.value = 100
}

async function submitQuota() {
  if (!quotaDlg.value) return
  busy.value = true
  err.value = ''
  try {
    const d = await agentApi.post<{ sub_quota_cents: number; my_quota_cents: number }>(
      `/agent/api/subagents/${quotaDlg.value.id}/quota`,
      { delta_cents: Math.round((quotaYuan.value || 0) * 100) },
    )
    notice.value = `已划转，下级余额 ${formatYuan(d.sub_quota_cents)} 元，你的余额 ${d.my_quota_cents < 0 ? '不限' : formatYuan(d.my_quota_cents) + ' 元'}`
    quotaDlg.value = null
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function openPlan(r: Sub) {
  planDlg.value = r
  planValue.value = r.plan_id ?? null
}

async function submitPlan() {
  if (!planDlg.value || !planValue.value) return
  busy.value = true
  err.value = ''
  try {
    await agentApi.put(`/agent/api/subagents/${planDlg.value.id}/plan`, { plan_id: planValue.value })
    notice.value = '模板已设置，下级出卡按新价格执行'
    planDlg.value = null
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function openPerms(r: Sub) {
  permDlg.value = r
  permForm.value = {
    SetDeviceLimit: r.permissions?.SetDeviceLimit ?? false,
    CustomKami: r.permissions?.CustomKami ?? false,
    MaxQty: r.permissions?.MaxQty || 10,
  }
}

async function submitPerms() {
  if (!permDlg.value) return
  busy.value = true
  err.value = ''
  try {
    await agentApi.put(`/agent/api/subagents/${permDlg.value.id}/perms`, {
      set_device_limit: permForm.value.SetDeviceLimit,
      custom_kami: permForm.value.CustomKami,
      max_qty: permForm.value.MaxQty,
    })
    notice.value = '下级权限已更新'
    permDlg.value = null
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function toggle(r: Sub) {
  busy.value = true
  err.value = ''
  try {
    const action = r.status === 'active' ? 'freeze' : 'unfreeze'
    await agentApi.post(`/agent/api/subagents/${r.id}/${action}`)
    notice.value = r.status === 'active' ? '已冻结' : '已解冻'
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
    <n-space style="margin-bottom: 12px" justify="space-between">
      <span style="font-size: 13px; color: #666">你自己和下级代理；下级只能由一级代理开通。</span>
      <n-button type="primary" size="small" @click="showCreate = true">开通下级</n-button>
    </n-space>

    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <template v-if="isMobile">
      <n-card v-for="r in rows" :key="r.id" size="small" style="margin-bottom: 10px">
        <div style="display:flex;align-items:center;gap:6px;flex-wrap:wrap">
          <b>{{ r.name }}</b>
          <n-tag v-if="r.is_self" size="tiny" type="info">我</n-tag>
          <n-tag v-if="r.level === 2" size="tiny">下级</n-tag>
          <n-tag v-if="!r.is_self && r.activated === false" size="tiny" type="warning">待激活</n-tag>
          <n-tag size="tiny" :type="r.status === 'active' ? 'success' : 'error'">
            {{ r.status === 'active' ? '正常' : '冻结' }}
          </n-tag>
        </div>
        <div style="font-size:13px;color:#666;margin:8px 0">
          额度 {{ r.quota_cents < 0 ? '不限' : formatYuan(r.quota_cents) + ' 元' }} ·
          模板 {{ r.plan_name ?? '未设置' }}
        </div>
        <n-space v-if="!r.is_self" :wrap="true">
          <n-button size="tiny" @click="showLink(r)">登录链接</n-button>
          <n-button size="tiny" type="primary" @click="openQuota(r)">划额度</n-button>
          <n-button size="tiny" @click="openPlan(r)">设模板</n-button>
          <n-button size="tiny" @click="openPerms(r)">设权限</n-button>
          <n-button size="tiny" :type="r.status === 'active' ? 'error' : 'success'" @click="toggle(r)">
            {{ r.status === 'active' ? '冻结' : '解冻' }}
          </n-button>
        </n-space>
      </n-card>
    </template>
    <n-data-table v-else :columns="columns" :data="rows" :loading="loading" :bordered="false" size="small" :scroll-x="720" />

    <n-modal v-model:show="showCreate" preset="card" title="开通下级" style="width: 420px">
      <n-form label-placement="top">
        <n-form-item label="代理名">
          <n-input v-model:value="form.name" placeholder="下级登录名" />
        </n-form-item>
        <n-form-item label="初始密码（可留空）">
          <n-input v-model:value="form.password" type="password" show-password-on="click" placeholder="留空则由下级通过链接自己设置" />
        </n-form-item>
      </n-form>
      <n-space justify="end">
        <n-button @click="showCreate = false">取消</n-button>
        <n-button type="primary" :loading="busy" @click="createSub">开通</n-button>
      </n-space>
    </n-modal>

    <n-modal :show="linkDlg !== null" @update:show="(v: boolean) => { if (!v) linkDlg = null }" preset="card" title="下级登录链接" style="width: 520px">
      <template v-if="linkDlg">
        <p style="margin-top: 0; font-size: 13px; color: #666">
          把这个链接发给「{{ linkDlg.name }}」，他打开后设置密码就能登录代理端。
        </p>
        <a :href="linkDlg.link" target="_blank" rel="noopener" style="word-break: break-all">{{ linkDlg.link }}</a>
        <n-input :value="linkDlg.link" readonly style="margin-top: 8px" />
      </template>
      <n-space justify="end" style="margin-top: 16px">
        <n-button @click="linkDlg = null">关闭</n-button>
        <n-button type="primary" @click="copyLink">复制链接</n-button>
      </n-space>
    </n-modal>

    <n-modal :show="quotaDlg !== null" @update:show="(v: boolean) => { if (!v) quotaDlg = null }" preset="card" title="划额度给下级" style="width: 420px">
      <template v-if="quotaDlg">
        <p style="margin-top: 0; font-size: 13px; color: #666">
          从你的余额划给「{{ quotaDlg.name }}」，1 元 = 100 分。
        </p>
        <n-input-number v-model:value="quotaYuan" :min="1" :precision="0" style="width: 100%">
          <template #suffix>元</template>
        </n-input-number>
      </template>
      <n-space justify="end" style="margin-top: 16px">
        <n-button @click="quotaDlg = null">取消</n-button>
        <n-button type="primary" :loading="busy" @click="submitQuota">划转</n-button>
      </n-space>
    </n-modal>

    <n-modal :show="planDlg !== null" @update:show="(v: boolean) => { if (!v) planDlg = null }" preset="card" title="设置下级价格模板" style="width: 520px">
      <template v-if="planDlg">
        <p style="margin-top: 0; font-size: 13px; color: #666">
          每一档价格都不能低于你自己的模板；下级只能看到模板里有的卡类型。
        </p>
        <n-select
          v-model:value="planValue"
          :options="plans.map((p) => ({ label: p.name, value: p.id }))"
          placeholder="选择模板"
        />
        <div v-if="planValue" style="margin-top: 12px; font-size: 13px; color: #4a5568">
          <div v-for="p in plans.filter((x) => x.id === planValue)" :key="p.id">
            <span v-for="i in p.items" :key="i.kind" style="margin-right: 14px">
              {{ i.name }} {{ formatYuan(i.price_cents) }} 元
            </span>
          </div>
        </div>
      </template>
      <n-space justify="end" style="margin-top: 16px">
        <n-button @click="planDlg = null">取消</n-button>
        <n-button type="primary" :loading="busy" @click="submitPlan">保存</n-button>
      </n-space>
    </n-modal>

    <n-modal :show="permDlg !== null" @update:show="(v: boolean) => { if (!v) permDlg = null }" preset="card" title="设置下级权限" style="width: 460px">
      <p style="margin-top: 0; font-size: 13px; color: #666">下级权限不能超过你自己。</p>
      <n-form label-placement="left" label-width="150">
        <n-form-item label="可设设备数量">
          <n-switch v-model:value="permForm.SetDeviceLimit" />
        </n-form-item>
        <n-form-item label="可自定义卡密">
          <n-switch v-model:value="permForm.CustomKami" />
        </n-form-item>
        <n-form-item label="单批最多张数">
          <n-input-number v-model:value="permForm.MaxQty" :min="1" :max="500" style="width: 140px" />
        </n-form-item>
      </n-form>
      <n-space justify="end">
        <n-button @click="permDlg = null">取消</n-button>
        <n-button type="primary" :loading="busy" @click="submitPerms">保存</n-button>
      </n-space>
    </n-modal>
  </div>
</template>
