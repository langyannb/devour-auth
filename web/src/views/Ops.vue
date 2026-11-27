<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  NAlert, NButton, NCard, NForm, NFormItem, NInput, NInputNumber, NPopconfirm, NSelect,
  NSpace, NSwitch, NTabPane, NTabs, NTag,
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

const route = useRoute()
// ?tab=notices 可直接打开某一页，方便分享链接
const tab = ref(typeof route.query.tab === 'string' ? route.query.tab : 'policy')

const err = ref('')
const ok = ref('')
const busy = ref(false)

// ---------- 卡密验证 / 解绑策略（原设置页） ----------
interface Policy {
  card_mode: 'paid' | 'free'
  unbind_free: number | null
  unbind_cooldown_days: number | null
  unbind_mode: 'reject' | 'charge'
  unbind_cost_hours: number | null
  verify_device: boolean
  verify_ip: boolean
}
const policy = ref<Policy>({
  card_mode: 'paid',
  unbind_free: 0,
  unbind_cooldown_days: 0,
  unbind_mode: 'reject',
  unbind_cost_hours: 0,
  verify_device: true,
  verify_ip: false,
})
const policyBase = ref<Record<string, string>>({})

function num(v: string | undefined): number {
  const n = Number.parseInt(v ?? '0', 10)
  return Number.isFinite(n) && n > 0 ? n : 0
}

function policyValues(): Record<string, string> {
  const p = policy.value
  const int = (v: number | null) => String(Math.max(0, Math.floor(v ?? 0)))
  return {
    card_mode: p.card_mode === 'free' ? 'free' : 'paid',
    unbind_free: int(p.unbind_free),
    unbind_cooldown_days: int(p.unbind_cooldown_days),
    unbind_mode: p.unbind_mode === 'charge' ? 'charge' : 'reject',
    unbind_cost_hours: int(p.unbind_cost_hours),
    verify_device: p.verify_device ? '1' : '0',
    verify_ip: p.verify_ip ? '1' : '0',
  }
}

async function savePolicy() {
  err.value = ''
  ok.value = ''
  busy.value = true
  try {
    const cur = policyValues()
    for (const key of Object.keys(cur)) {
      if (cur[key] === policyBase.value[key]) continue
      await api.put('/admin/api/flags', { key, value: cur[key] })
    }
    policyBase.value = cur
    ok.value = '策略已保存'
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

// ---------- 版本 ----------
const releases = ref<Release[]>([])
const relForm = ref({ version: '', info: '', url: '', must: false, rollout_bp: 10000 })
const editingRel = ref<number | null>(null)

async function submitRelease() {
  err.value = ''
  ok.value = ''
  if (!relForm.value.version.trim()) {
    err.value = '版本号必填'
    return
  }
  busy.value = true
  try {
    const body = { ...relForm.value, version: relForm.value.version.trim() }
    if (editingRel.value) {
      await api.put(`/admin/api/releases/${editingRel.value}`, body)
      ok.value = `版本 ${body.version} 已更新`
    } else {
      await api.post('/admin/api/releases', body)
      ok.value = `版本 ${body.version} 已发布`
    }
    resetRelease()
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function editRelease(r: Release) {
  editingRel.value = r.id
  relForm.value = {
    version: r.version,
    info: r.info,
    url: r.url,
    must: r.must === 1,
    rollout_bp: r.rollout_bp,
  }
}

function resetRelease() {
  editingRel.value = null
  relForm.value = { version: '', info: '', url: '', must: false, rollout_bp: 10000 }
}

async function toggleRelease(r: Release) {
  err.value = ''
  try {
    await api.post(`/admin/api/releases/${r.id}/toggle`, { active: !r.active })
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function removeRelease(r: Release) {
  err.value = ''
  ok.value = ''
  try {
    await api.delete(`/admin/api/releases/${r.id}`)
    if (editingRel.value === r.id) resetRelease()
    ok.value = `版本 ${r.version} 已删除`
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

// ---------- 公告 ----------
const notices = ref<Notice[]>([])
const noticeForm = ref({ kind: 'list', title: '', body: '' })
const editingNotice = ref<number | null>(null)

async function submitNotice() {
  err.value = ''
  ok.value = ''
  if (!noticeForm.value.body.trim()) {
    err.value = '正文必填'
    return
  }
  busy.value = true
  try {
    if (editingNotice.value) {
      await api.put(`/admin/api/notices/${editingNotice.value}`, noticeForm.value)
      ok.value = '公告已更新'
    } else {
      await api.post('/admin/api/notices', noticeForm.value)
      ok.value = '公告已发布'
    }
    resetNotice()
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function editNotice(n: Notice) {
  editingNotice.value = n.id
  noticeForm.value = { kind: n.kind, title: n.title, body: n.body }
}

function resetNotice() {
  editingNotice.value = null
  noticeForm.value = { kind: 'list', title: '', body: '' }
}

async function toggleNotice(n: Notice) {
  err.value = ''
  try {
    await api.post(`/admin/api/notices/${n.id}/toggle`, { active: !n.active })
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function removeNotice(n: Notice) {
  err.value = ''
  ok.value = ''
  try {
    await api.delete(`/admin/api/notices/${n.id}`)
    if (editingNotice.value === n.id) resetNotice()
    ok.value = '公告已删除'
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

// ---------- 远程变量 ----------
const flagRows = ref<{ key: string; value: string }[]>([])
const newFlag = ref({ key: '', value: '' })

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
    ok.value = `变量 ${row.key} 已保存`
    await loadAll()
  } catch (e) {
    err.value = (e as Error).message
  }
}

const kindLabel: Record<string, string> = { list: '列表', popup: '弹窗', immediate: '立即' }

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
    policy.value = {
      card_mode: f.card_mode === 'free' ? 'free' : 'paid',
      unbind_free: num(f.unbind_free),
      unbind_cooldown_days: num(f.unbind_cooldown_days),
      unbind_mode: f.unbind_mode === 'charge' ? 'charge' : 'reject',
      unbind_cost_hours: num(f.unbind_cost_hours),
      verify_device: f.verify_device !== '0',
      verify_ip: f.verify_ip === '1',
    }
    policyBase.value = policyValues()
    flagRows.value = Object.entries(f).map(([key, value]) => ({ key, value }))
  } catch (e) {
    err.value = (e as Error).message
  }
}

onMounted(loadAll)
</script>

<template>
  <div class="ops">
    <n-alert v-if="err" type="error" closable style="margin-bottom: 12px" @close="err = ''">{{ err }}</n-alert>
    <n-alert v-if="ok" type="success" closable style="margin-bottom: 12px" @close="ok = ''">{{ ok }}</n-alert>

    <n-tabs v-model:value="tab" type="line" animated>
      <n-tab-pane name="policy" tab="卡密与解绑">
        <n-card size="small" title="卡密验证" style="margin-bottom: 14px">
          <n-form label-placement="top" :show-feedback="false">
            <div class="grid">
              <n-form-item label="模式">
                <n-select
                  v-model:value="policy.card_mode"
                  :options="[
                    { label: '收费模式（校验卡密）', value: 'paid' },
                    { label: '免费模式（不校验卡密）', value: 'free' },
                  ]"
                />
              </n-form-item>
              <n-form-item label="设备验证">
                <n-switch v-model:value="policy.verify_device" />
              </n-form-item>
              <n-form-item label="IP 验证">
                <n-switch v-model:value="policy.verify_ip" />
              </n-form-item>
            </div>
          </n-form>
          <p class="hint">
            免费模式：服务端不校验卡密，`/v1/meta` 下发 mode: free，客户端自行放行；设备/IP 验证只在收费模式下生效。
          </p>
          <p class="hint">设备验证关闭后不校验设备上限，同时在线也不限；IP 验证开启后换 IP 只记录异常，不拦登录。</p>
        </n-card>

        <n-card size="small" title="解绑策略" style="margin-bottom: 14px">
          <n-form label-placement="top" :show-feedback="false">
            <div class="grid">
              <n-form-item label="免费解绑次数">
                <n-input-number v-model:value="policy.unbind_free" :min="0" style="width: 100%" />
              </n-form-item>
              <n-form-item label="解绑冷却天数">
                <n-input-number v-model:value="policy.unbind_cooldown_days" :min="0" style="width: 100%" />
              </n-form-item>
              <n-form-item label="超出免费次数后">
                <n-select
                  v-model:value="policy.unbind_mode"
                  :options="[
                    { label: '拒绝解绑', value: 'reject' },
                    { label: '扣减时长', value: 'charge' },
                  ]"
                />
              </n-form-item>
              <n-form-item label="扣减时长（小时/次）">
                <n-input-number
                  v-model:value="policy.unbind_cost_hours"
                  :min="0"
                  :disabled="policy.unbind_mode === 'reject'"
                  style="width: 100%"
                />
              </n-form-item>
            </div>
          </n-form>
          <p class="hint">扣时只对时长卡生效，永久卡不支持；拒绝模式下不扣时。每次解绑计一次，勾选多台只算一次。</p>
          <n-button type="primary" :loading="busy" @click="savePolicy">保存策略</n-button>
        </n-card>
      </n-tab-pane>

      <n-tab-pane name="notices" tab="公告">
        <n-card size="small" :title="editingNotice ? '编辑公告' : '新建公告'" style="margin-bottom: 14px">
          <n-form label-placement="top">
            <div class="row">
              <n-form-item label="类型" style="width: 160px">
                <n-select
                  v-model:value="noticeForm.kind"
                  :options="[
                    { label: '列表', value: 'list' },
                    { label: '弹窗', value: 'popup' },
                    { label: '立即', value: 'immediate' },
                  ]"
                />
              </n-form-item>
              <n-form-item label="标题" style="flex: 1; min-width: 220px">
                <n-input v-model:value="noticeForm.title" placeholder="例如：今晚维护" />
              </n-form-item>
            </div>
            <n-form-item label="正文（可以换行，客户端原样显示）">
              <n-input
                v-model:value="noticeForm.body"
                type="textarea"
                :autosize="{ minRows: 8, maxRows: 22 }"
                placeholder="想写多少行都行，比如：&#10;1. 今晚 23:00 起维护&#10;2. 预计 30 分钟&#10;3. 期间无法登录"
              />
            </n-form-item>
          </n-form>
          <n-space justify="end">
            <n-button v-if="editingNotice" @click="resetNotice">取消编辑</n-button>
            <n-button type="primary" :loading="busy" @click="submitNotice">
              {{ editingNotice ? '保存修改' : '发布公告' }}
            </n-button>
          </n-space>
        </n-card>

        <div v-if="notices.length === 0" class="empty">还没有公告。</div>
        <article v-for="n in notices" :key="n.id" class="item" :class="{ off: !n.active }">
          <header class="item-head">
            <n-tag size="small" :bordered="false" :type="n.kind === 'immediate' ? 'error' : n.kind === 'popup' ? 'warning' : 'default'">
              {{ kindLabel[n.kind] ?? n.kind }}
            </n-tag>
            <b class="item-title">{{ n.title || '（无标题）' }}</b>
            <n-tag size="small" :type="n.active ? 'success' : 'default'" :bordered="false">{{ n.active ? '启用' : '停用' }}</n-tag>
            <span class="time">{{ formatTs(n.created_at) }}</span>
          </header>
          <pre class="body">{{ n.body }}</pre>
          <footer class="item-actions">
            <n-button size="tiny" @click="editNotice(n)">编辑</n-button>
            <n-button size="tiny" @click="toggleNotice(n)">{{ n.active ? '停用' : '启用' }}</n-button>
            <n-popconfirm @positive-click="removeNotice(n)">
              <template #trigger>
                <n-button size="tiny" type="error" secondary>删除</n-button>
              </template>
              删除后客户端就看不到了，确定？
            </n-popconfirm>
          </footer>
        </article>
      </n-tab-pane>

      <n-tab-pane name="releases" tab="版本">
        <n-card size="small" :title="editingRel ? '编辑版本' : '发布新版本'" style="margin-bottom: 14px">
          <n-form label-placement="top">
            <div class="row">
              <n-form-item label="版本号" style="width: 150px">
                <n-input v-model:value="relForm.version" placeholder="2.0.1" />
              </n-form-item>
              <n-form-item label="下载地址" style="flex: 1; min-width: 240px">
                <n-input v-model:value="relForm.url" placeholder="https://..." />
              </n-form-item>
              <n-form-item label="灰度（千分比）" style="width: 160px">
                <n-input-number v-model:value="relForm.rollout_bp" :min="1" :max="10000" style="width: 100%" />
              </n-form-item>
              <n-form-item label="强制更新">
                <n-switch v-model:value="relForm.must" />
              </n-form-item>
            </div>
            <n-form-item label="更新说明（可以换行）">
              <n-input
                v-model:value="relForm.info"
                type="textarea"
                :autosize="{ minRows: 5, maxRows: 16 }"
                placeholder="修复：&#10;- 登录偶发失败&#10;新增：&#10;- 设备列表"
              />
            </n-form-item>
          </n-form>
          <n-space justify="end">
            <n-button v-if="editingRel" @click="resetRelease">取消编辑</n-button>
            <n-button type="primary" :loading="busy" @click="submitRelease">
              {{ editingRel ? '保存修改' : '发布' }}
            </n-button>
          </n-space>
        </n-card>

        <div v-if="releases.length === 0" class="empty">还没有发布过版本。</div>
        <article v-for="r in releases" :key="r.id" class="item" :class="{ off: !r.active }">
          <header class="item-head">
            <b class="item-title">{{ r.version }}</b>
            <n-tag v-if="r.must" size="small" type="warning" :bordered="false">强更</n-tag>
            <n-tag size="small" :type="r.active ? 'success' : 'default'" :bordered="false">{{ r.active ? '启用' : '停用' }}</n-tag>
            <span class="time">灰度 {{ (r.rollout_bp / 100).toFixed(0) }}% · {{ formatTs(r.created_at) }}</span>
          </header>
          <pre class="body">{{ r.info || '（没有更新说明）' }}</pre>
          <a v-if="r.url" class="url" :href="r.url" target="_blank" rel="noopener">{{ r.url }}</a>
          <footer class="item-actions">
            <n-button size="tiny" @click="editRelease(r)">编辑</n-button>
            <n-button size="tiny" @click="toggleRelease(r)">{{ r.active ? '停用' : '启用' }}</n-button>
            <n-popconfirm @positive-click="removeRelease(r)">
              <template #trigger>
                <n-button size="tiny" type="error" secondary>删除</n-button>
              </template>
              删掉后客户端拉不到这个版本，确定？
            </n-popconfirm>
          </footer>
        </article>
      </n-tab-pane>

      <n-tab-pane name="flags" tab="远程变量">
        <n-card size="small" title="添加 / 修改变量" style="margin-bottom: 14px">
          <n-space :wrap="true" align="end">
            <n-input v-model:value="newFlag.key" placeholder="key，例如 maintenance" style="width: 220px" />
            <n-input v-model:value="newFlag.value" placeholder="value" style="width: 220px" />
            <n-button type="primary" @click="saveNewFlag">保存</n-button>
          </n-space>
          <p class="hint" style="margin-top: 10px">`maintenance=1` 时客户端 `/v1/meta` 返回维护中。</p>
        </n-card>
        <n-card size="small" title="已有变量">
          <div v-if="flagRows.length === 0" class="empty">还没有变量</div>
          <div v-for="row in flagRows" :key="row.key" class="flag-row">
            <code>{{ row.key }}</code>
            <n-input v-model:value="row.value" size="small" style="width: 240px" />
            <n-button size="tiny" @click="saveFlagRow(row)">保存</n-button>
          </div>
        </n-card>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(190px, 1fr)); gap: 12px 16px; }
.row { display: flex; flex-wrap: wrap; gap: 0 16px; align-items: flex-start; }
.hint { margin: 0 0 10px; color: #8b93a7; font-size: 12px; line-height: 1.6; }

.item {
  background: #fff;
  border: 1px solid #e8edeb;
  border-radius: 12px;
  padding: 14px 16px;
  margin-bottom: 10px;
}
.item.off { background: #fbfcfc; }
.item-head { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; }
.item-title { font-size: 15px; }
.time { margin-left: auto; font-size: 12px; color: #98a2b3; font-variant-numeric: tabular-nums; }
.body {
  margin: 10px 0 0;
  font-family: inherit;
  font-size: 13.5px;
  line-height: 1.75;
  color: #475467;
  white-space: pre-wrap;
  word-break: break-word;
}
.url { display: inline-block; margin-top: 8px; font-size: 12px; color: #148c4f; word-break: break-all; }
.item-actions { display: flex; gap: 8px; margin-top: 12px; }
.empty { color: #98a2b3; font-size: 13px; padding: 8px 0 16px; }
.flag-row { display: flex; align-items: center; gap: 12px; padding: 6px 0; }
.flag-row code { font-family: ui-monospace, monospace; font-size: 13px; min-width: 160px; }

@media (max-width: 640px) {
  .time { margin-left: 0; width: 100%; }
}
</style>
