<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NAlert, NButton, NDrawer, NDrawerContent, NEmpty, NForm, NFormItem, NInput,
  NInputNumber, NModal, NRadioButton, NRadioGroup, NSelect, NSpace, NSwitch, NTag,
} from 'naive-ui'
import { agentApi } from '../../api'
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

const me = computed(() => rows.value.find((r) => r.is_self) ?? null)
const subs = computed(() => rows.value.filter((r) => !r.is_self))
const pendingCount = computed(() => subs.value.filter((s) => s.activated === false).length)
// 只有一级代理能开通下级，二级进来就是个空列表
const canCreate = computed(() => (me.value?.level ?? 1) === 1)
const availableCents = computed(() => (me.value && me.value.quota_cents >= 0 ? me.value.quota_cents : Infinity))

const quotaText = (v?: number) => {
  if (v === undefined) return '—'
  return v < 0 ? '不限' : `${formatYuan(v)} 元`
}

function permText(p?: Perms) {
  if (!p) return '未设置'
  const on: string[] = []
  if (p.SetDeviceLimit) on.push('可设设备数量')
  if (p.CustomKami) on.push('可自定义卡密')
  return `${on.length > 0 ? on.join('、') : '无特殊权限'} · 单批 ${p.MaxQty}`
}

async function load() {
  loading.value = true
  err.value = ''
  try {
    const [subsData, planList] = await Promise.all([
      agentApi.get<Sub[]>('/agent/api/subagents'),
      agentApi.get<Plan[]>('/agent/api/plans'),
    ])
    rows.value = subsData
    plans.value = planList
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

// ---------- 开通下级 ----------
const showCreate = ref(false)
const form = ref({ name: '', password: '' })

async function createSub() {
  err.value = ''
  notice.value = ''
  busy.value = true
  try {
    const d = await agentApi.post<{ id: number; name: string; login_link: string }>('/agent/api/subagents', {
      name: form.value.name,
      password: form.value.password,
    })
    notice.value = `已开通下级 ${form.value.name}`
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

// ---------- 登录链接 ----------
const linkDlg = ref<{ name: string; link: string } | null>(null)

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

// 抽屉里直接复制，不再叠一层弹窗
async function copyCurrentLink() {
  if (!current.value) return
  drawerErr.value = ''
  if (await copyText(current.value.login_link ?? '')) drawerNotice.value = '链接已复制，发给下级即可'
  else drawerErr.value = '复制失败，请手动选中链接'
}

// ---------- 划额度 ----------
const quotaDlg = ref<Sub | null>(null)
const quotaYuan = ref(100)

function openQuota(r: Sub) {
  err.value = ''
  quotaDlg.value = r
  quotaYuan.value = 100
}

const quotaCents = computed(() => Math.round((quotaYuan.value || 0) * 100))
const quotaAfterSub = computed(() => {
  if (!quotaDlg.value) return 0
  if (quotaDlg.value.quota_cents < 0) return -1
  return quotaDlg.value.quota_cents + quotaCents.value
})
const quotaAfterMine = computed(() => {
  if (!me.value) return 0
  if (me.value.quota_cents < 0) return -1
  return Math.max(0, me.value.quota_cents - quotaCents.value)
})

async function submitQuota() {
  if (!quotaDlg.value) return
  err.value = ''
  if (quotaCents.value <= 0) {
    err.value = '金额要大于 0'
    return
  }
  if (quotaCents.value > availableCents.value) {
    err.value = `你的余额不够，最多划 ${formatYuan(availableCents.value)} 元`
    return
  }
  busy.value = true
  try {
    const d = await agentApi.post<{ sub_quota_cents: number; my_quota_cents: number }>(
      `/agent/api/subagents/${quotaDlg.value.id}/quota`,
      { delta_cents: quotaCents.value },
    )
    notice.value = `已划给「${quotaDlg.value.name}」${formatYuan(quotaCents.value)} 元，你的余额 ${quotaText(d.my_quota_cents)}`
    quotaDlg.value = null
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

// ---------- 设置抽屉：模板 / 权限 / 冻结 ----------
const drawer = ref(false)
const current = ref<Sub | null>(null)
const drawerErr = ref('')
const drawerNotice = ref('')

const planMode = ref<'pick' | 'custom'>('pick')
const planValue = ref<number | null>(null)
const customPrices = ref<{ day: number | null; week: number | null; month: number | null; forever: number | null }>({
  day: null, week: null, month: null, forever: null,
})
const permForm = ref<Perms>({ SetDeviceLimit: false, CustomKami: false, MaxQty: 10 })

const pickedPlan = computed(() => plans.value.find((p) => p.id === planValue.value) ?? null)

function openSettings(r: Sub) {
  current.value = r
  err.value = ''
  drawerErr.value = ''
  drawerNotice.value = ''
  planMode.value = 'pick'
  planValue.value = r.plan_id && r.plan_name !== '自定义' ? r.plan_id : plans.value[0]?.id ?? null
  customPrices.value = { day: null, week: null, month: null, forever: null }
  permForm.value = {
    SetDeviceLimit: r.permissions?.SetDeviceLimit ?? false,
    CustomKami: r.permissions?.CustomKami ?? false,
    MaxQty: r.permissions?.MaxQty || 10,
  }
  drawer.value = true
}

async function savePlan() {
  if (!current.value) return
  let body: Record<string, unknown>
  if (planMode.value === 'custom') {
    const items: Record<string, number> = {}
    for (const [k, v] of Object.entries(customPrices.value)) {
      if (v && v > 0) items[k] = Math.round(v * 100)
    }
    if (Object.keys(items).length === 0) {
      drawerErr.value = '至少填一种卡的价格'
      return
    }
    body = { items }
  } else {
    if (!planValue.value) {
      drawerErr.value = '先选一个模板'
      return
    }
    body = { plan_id: planValue.value }
  }
  busy.value = true
  drawerErr.value = ''
  try {
    await agentApi.put(`/agent/api/subagents/${current.value.id}/plan`, body)
    drawerNotice.value = '模板已保存，下级出卡按新价格执行'
    await load()
    current.value = rows.value.find((r) => r.id === current.value?.id) ?? current.value
  } catch (e) {
    drawerErr.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function savePerms() {
  if (!current.value) return
  busy.value = true
  drawerErr.value = ''
  try {
    await agentApi.put(`/agent/api/subagents/${current.value.id}/perms`, {
      set_device_limit: permForm.value.SetDeviceLimit,
      custom_kami: permForm.value.CustomKami,
      max_qty: permForm.value.MaxQty,
    })
    drawerNotice.value = '权限已保存'
    await load()
    current.value = rows.value.find((r) => r.id === current.value?.id) ?? current.value
  } catch (e) {
    drawerErr.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function toggleFrozen() {
  if (!current.value) return
  const target = current.value
  busy.value = true
  drawerErr.value = ''
  try {
    await agentApi.post(`/agent/api/subagents/${target.id}/${target.status === 'active' ? 'freeze' : 'unfreeze'}`)
    drawerNotice.value = target.status === 'active' ? '已冻结' : '已解冻'
    await load()
    current.value = rows.value.find((r) => r.id === target.id) ?? target
  } catch (e) {
    drawerErr.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="page-head">
      <div>
        <h2>下级代理</h2>
        <p>开通下级、划额度、定价和权限都在这里。</p>
      </div>
      <n-button v-if="canCreate" type="primary" @click="showCreate = true">开通下级</n-button>
    </div>

    <n-alert v-if="err" type="error" closable style="margin-bottom: 12px" @close="err = ''">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" closable style="margin-bottom: 12px" @close="notice = ''">{{ notice }}</n-alert>

    <div class="stats">
      <div class="stat">
        <span class="k">我的余额</span>
        <span class="v">{{ quotaText(me?.quota_cents) }}</span>
      </div>
      <div class="stat">
        <span class="k">下级</span>
        <span class="v">{{ subs.length }}<i>人</i></span>
      </div>
      <div class="stat">
        <span class="k">待激活</span>
        <span class="v">{{ pendingCount }}<i>人</i></span>
      </div>
    </div>

    <div v-if="subs.length === 0" class="empty">
      <n-empty :description="canCreate ? '还没有下级，开通一个试试' : '只有一级代理能开通下级'">
        <template v-if="canCreate" #extra>
          <n-button size="small" @click="showCreate = true">开通下级</n-button>
        </template>
      </n-empty>
    </div>

    <article v-for="s in subs" :key="s.id" class="sub" :class="{ off: s.status !== 'active' }">
      <div class="sub-top">
        <div class="sub-id">
          <span class="name">{{ s.name }}</span>
          <n-tag v-if="s.activated === false" size="tiny" type="warning" :bordered="false">待激活</n-tag>
          <n-tag size="tiny" :type="s.status === 'active' ? 'success' : 'error'" :bordered="false">
            {{ s.status === 'active' ? '正常' : '冻结' }}
          </n-tag>
        </div>
        <div class="sub-balance" :class="{ zero: s.quota_cents === 0 }">
          <span class="k">余额</span>
          <span class="v">{{ quotaText(s.quota_cents) }}</span>
        </div>
      </div>

      <div class="sub-meta">
        <span>模板 <b>{{ s.plan_name ?? '未设置' }}</b></span>
        <span>权限 {{ permText(s.permissions) }}</span>
      </div>

      <div class="sub-actions">
        <n-button size="small" type="primary" secondary @click="openQuota(s)">划额度</n-button>
        <n-button v-if="s.activated === false" size="small" @click="showLink(s)">发登录链接</n-button>
        <n-button size="small" @click="openSettings(s)">设置</n-button>
      </div>
    </article>

    <n-modal v-model:show="showCreate" preset="card" title="开通下级" style="width: min(440px, 92vw)">
      <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
      <n-form label-placement="top">
        <n-form-item label="代理名">
          <n-input v-model:value="form.name" placeholder="下级登录名" />
        </n-form-item>
        <n-form-item label="初始密码（可留空）">
          <n-input v-model:value="form.password" type="password" show-password-on="click"
            placeholder="留空则由下级通过链接自己设置" />
        </n-form-item>
      </n-form>
      <div class="hint">开通后会给你一个专属登录链接，发给下级即可。</div>
      <n-space justify="end" style="margin-top: 16px">
        <n-button @click="showCreate = false">取消</n-button>
        <n-button type="primary" :loading="busy" @click="createSub">开通</n-button>
      </n-space>
    </n-modal>

    <n-modal :show="linkDlg !== null" preset="card" title="下级登录链接" style="width: min(520px, 92vw)"
      @update:show="(v: boolean) => { if (!v) linkDlg = null }">
      <template v-if="linkDlg">
        <p class="hint" style="margin-top: 0">
          发给「{{ linkDlg.name }}」，他打开后设置密码就能登录代理端。
        </p>
        <n-input :value="linkDlg.link" readonly />
        <n-space justify="end" style="margin-top: 16px">
          <n-button @click="linkDlg = null">关闭</n-button>
          <n-button type="primary" @click="copyLink">复制链接</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal :show="quotaDlg !== null" preset="card" title="划额度给下级" style="width: min(440px, 92vw)"
      @update:show="(v: boolean) => { if (!v) quotaDlg = null }">
      <template v-if="quotaDlg">
        <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
        <n-form label-placement="top">
          <n-form-item label="金额（元）">
            <n-input-number v-model:value="quotaYuan" :min="1" style="width: 100%">
              <template #suffix>元</template>
            </n-input-number>
          </n-form-item>
        </n-form>
        <div class="calc">
          <div><span>我的余额</span><span>{{ quotaText(me?.quota_cents) }} → {{ quotaText(quotaAfterMine) }}</span></div>
          <div><span>「{{ quotaDlg.name }}」余额</span><span>{{ quotaText(quotaDlg.quota_cents) }} → {{ quotaText(quotaAfterSub) }}</span></div>
        </div>
        <n-space justify="end" style="margin-top: 16px">
          <n-button @click="quotaDlg = null">取消</n-button>
          <n-button type="primary" :loading="busy" :disabled="quotaCents > availableCents" @click="submitQuota">划转</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-drawer v-model:show="drawer" width="min(440px, 94vw)" placement="right">
      <n-drawer-content v-if="current" closable>
        <template #header>
          <span class="drawer-title">{{ current.name }}</span>
          <n-tag v-if="current.activated === false" size="tiny" type="warning" :bordered="false" style="margin-left: 8px">待激活</n-tag>
          <n-tag size="tiny" :type="current.status === 'active' ? 'success' : 'error'" :bordered="false" style="margin-left: 4px">
            {{ current.status === 'active' ? '正常' : '冻结' }}
          </n-tag>
        </template>

        <n-alert v-if="drawerErr" type="error" closable style="margin-bottom: 12px" @close="drawerErr = ''">{{ drawerErr }}</n-alert>
        <n-alert v-if="drawerNotice" type="success" closable style="margin-bottom: 12px" @close="drawerNotice = ''">{{ drawerNotice }}</n-alert>

        <section class="block">
          <h3>登录链接</h3>
          <p class="hint">
            {{ current.activated === false ? '他还没设密码，把这个链接发给他。' : '链接一直有效，可用于重新设置密码。' }}
          </p>
          <div class="link-row">
            <n-input :value="current.login_link" readonly size="small" />
            <n-button size="small" @click="copyCurrentLink">复制</n-button>
          </div>
        </section>

        <section class="block">
          <h3>价格模板</h3>
          <p class="hint">当前：{{ current.plan_name ?? '未设置' }}。每一档价格不能低于你的成本。</p>
          <n-radio-group v-model:value="planMode" size="small" style="margin-bottom: 12px">
            <n-radio-button value="pick">用现成模板</n-radio-button>
            <n-radio-button value="custom">自己定价</n-radio-button>
          </n-radio-group>

          <template v-if="planMode === 'pick'">
            <n-select v-model:value="planValue" size="small"
              :options="plans.map((p) => ({ label: p.name, value: p.id }))" placeholder="选择模板" />
            <div v-if="pickedPlan" class="prices">
              <span v-for="i in pickedPlan.items" :key="i.kind">{{ i.name }} {{ formatYuan(i.price_cents) }} 元</span>
            </div>
          </template>

          <template v-else>
            <div class="price-grid">
              <label>天卡<n-input-number v-model:value="customPrices.day" :min="0" size="small" placeholder="不开放" /></label>
              <label>周卡<n-input-number v-model:value="customPrices.week" :min="0" size="small" placeholder="不开放" /></label>
              <label>月卡<n-input-number v-model:value="customPrices.month" :min="0" size="small" placeholder="不开放" /></label>
              <label>永久卡<n-input-number v-model:value="customPrices.forever" :min="0" size="small" placeholder="不开放" /></label>
            </div>
            <p class="hint" style="margin-bottom: 0">单位：元；留空表示不开放这一档。</p>
          </template>

          <n-button size="small" type="primary" :loading="busy" style="margin-top: 12px" @click="savePlan">保存模板</n-button>
        </section>

        <section class="block">
          <h3>下级权限</h3>
          <p class="hint">不能超过你自己的权限，超出时服务端会拒绝。</p>
          <n-form label-placement="left" label-width="130" size="small">
            <n-form-item label="可设设备数量">
              <n-switch v-model:value="permForm.SetDeviceLimit" size="small" />
            </n-form-item>
            <n-form-item label="可自定义卡密">
              <n-switch v-model:value="permForm.CustomKami" size="small" />
            </n-form-item>
            <n-form-item label="单批最多张数">
              <n-input-number v-model:value="permForm.MaxQty" :min="1" :max="500" size="small" style="width: 120px" />
            </n-form-item>
          </n-form>
          <n-button size="small" :loading="busy" @click="savePerms">保存权限</n-button>
        </section>

        <section class="block">
          <h3>状态</h3>
          <p class="hint">{{ current.status === 'active' ? '冻结后下级不能登录，也不能出卡。' : '解冻后下级恢复正常使用。' }}</p>
          <n-button size="small" :type="current.status === 'active' ? 'error' : 'success'" :loading="busy" @click="toggleFrozen">
            {{ current.status === 'active' ? '冻结下级' : '解冻下级' }}
          </n-button>
        </section>
      </n-drawer-content>
    </n-drawer>
  </div>
</template>

<style scoped>
.page { max-width: 880px; margin: 0 auto; }
.page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 16px; }
.page-head h2 { margin: 0 0 4px; font-size: 18px; }
.page-head p { margin: 0; font-size: 12px; color: #8b93a7; }

.stats { display: flex; flex-wrap: wrap; gap: 10px; margin-bottom: 16px; }
.stat {
  box-sizing: border-box; flex: 1 1 150px; display: flex; flex-direction: column; gap: 4px;
  background: #fff; border: 1px solid #e6ebe8; border-radius: 10px; padding: 10px 14px;
}
.stat .k { font-size: 12px; color: #8b93a7; }
.stat .v { font-size: 20px; font-weight: 700; letter-spacing: -0.02em; }
.stat .v i { font-size: 12px; font-weight: 500; color: #8b93a7; font-style: normal; margin-left: 2px; }

.empty { background: #fff; border: 1px solid #e6ebe8; border-radius: 10px; padding: 24px 0; }

.sub {
  background: #fff; border: 1px solid #e6ebe8; border-radius: 10px;
  padding: 12px 14px; margin-bottom: 10px;
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.04);
}
.sub.off { background: #fbfcfc; }
.sub-top { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.sub-id { display: flex; align-items: center; gap: 6px; min-width: 0; }
.sub-id .name { font-size: 15px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sub-balance { display: flex; align-items: baseline; gap: 4px; white-space: nowrap; }
.sub-balance .k { font-size: 12px; color: #8b93a7; }
.sub-balance .v { font-size: 15px; font-weight: 600; }
.sub-balance.zero .v { color: #c2410c; }
.sub-meta { display: flex; flex-wrap: wrap; gap: 4px 14px; margin-top: 6px; font-size: 12px; color: #667085; }
.sub-meta b { font-weight: 600; color: #344054; }
.sub-actions { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }

.hint { font-size: 12px; color: #8b93a7; line-height: 1.5; }
.calc { background: #f7faf8; border-radius: 8px; padding: 10px 12px; font-size: 13px; color: #475467; }
.calc div { display: flex; justify-content: space-between; gap: 12px; padding: 2px 0; }

.link-row { display: flex; gap: 8px; }
.link-row .n-input { flex: 1; }
.drawer-title { font-size: 16px; font-weight: 600; }
.block { padding: 16px 0; border-bottom: 1px solid #eef2f0; }
.block:first-of-type { padding-top: 4px; }
.block:last-of-type { border-bottom: none; }
.block h3 { margin: 0 0 6px; font-size: 14px; font-weight: 600; }
.prices { display: flex; flex-wrap: wrap; gap: 4px 14px; margin-top: 10px; font-size: 13px; color: #475467; }
.price-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px 12px; margin-bottom: 8px; }
.price-grid label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: #667085; }

@media (max-width: 560px) {
  .stat { flex: 1 1 0; padding: 8px 10px; }
  .stat .v { font-size: 16px; }
  .sub-top { align-items: flex-start; }
  .sub-actions :deep(.n-button) { flex: 1; }
}
</style>
