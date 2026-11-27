<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NAlert, NButton, NCard, NForm, NFormItem, NInput, NSpace, NTag,
} from 'naive-ui'
import QRCode from 'qrcode'
import { api } from '../api'
import { formatTs } from '../utils'

interface Me {
  id: number
  username: string
  role: string
  totp_enabled: boolean
}

interface SetupResp {
  secret: string
  otpauth_url: string
  account: string
  issuer: string
}

interface OpsTask {
  last_at: number
  next_at: number
}

interface OpsStatus {
  anchor: OpsTask
  rollup: OpsTask
  bills: OpsTask
  backup: OpsTask & { configured: boolean }
}

const me = ref<Me | null>(null)
const err = ref('')
const notice = ref('')

const setup = ref<SetupResp | null>(null)
const qr = ref('')
const code = ref('')
const busy = ref(false)

const disablePassword = ref('')
const disableCode = ref('')
const showDisable = ref(false)

const ops = ref<OpsStatus | null>(null)

const opsRows = computed(() => {
  const o = ops.value
  if (!o) return []
  const t = (v: number) => formatTs(v)
  return [
    { name: '审计锚定', last: t(o.anchor.last_at), next: t(o.anchor.next_at), note: '每天 00:05 签名上一日链尾' },
    { name: '日统计', last: t(o.rollup.last_at), next: t(o.rollup.next_at), note: '每天 00:07 汇总前一日数据' },
    { name: '代理账单', last: t(o.bills.last_at), next: t(o.bills.next_at), note: '每月 1 日 01:10 生成上月账单' },
    {
      name: '备份',
      last: o.backup.configured ? t(o.backup.last_at) : '未配置（systemd timer）',
      next: o.backup.configured ? t(o.backup.next_at) : '—',
      note: '每小时整点，GPG 加密',
    },
  ]
})

async function loadOps() {
  try {
    ops.value = await api.get<OpsStatus>('/admin/api/ops/status')
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function loadMe() {
  try {
    me.value = await api.get<Me>('/admin/api/me')
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function startSetup() {
  err.value = ''
  notice.value = ''
  busy.value = true
  try {
    const d = await api.post<SetupResp>('/admin/api/me/totp/setup')
    setup.value = d
    qr.value = await QRCode.toDataURL(d.otpauth_url, { width: 220, margin: 1 })
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function confirmSetup() {
  err.value = ''
  if (!setup.value) return
  busy.value = true
  try {
    await api.post('/admin/api/me/totp/enable', { secret: setup.value.secret, code: code.value })
    notice.value = '动态验证已启用，下次登录需要输入验证码'
    setup.value = null
    code.value = ''
    await loadMe()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function disable() {
  err.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api.post('/admin/api/me/totp/disable', {
      password: disablePassword.value,
      code: disableCode.value,
    })
    notice.value = '动态验证已关闭'
    showDisable.value = false
    disablePassword.value = ''
    disableCode.value = ''
    await loadMe()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

onMounted(() => {
  void loadMe()
  void loadOps()
})
</script>

<template>
  <div style="max-width: 640px">
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-card title="账号" style="margin-bottom: 16px">
      <div v-if="me" style="display: flex; flex-wrap: wrap; gap: 12px 24px">
        <span>用户名：{{ me.username }}</span>
        <span>角色：{{ me.role }}</span>
      </div>
    </n-card>

    <n-card title="定时任务" size="small" style="margin-bottom: 16px">
      <div class="ops-table">
        <div class="ops-row ops-head">
          <span>任务</span>
          <span>上次</span>
          <span>下次</span>
          <span>说明</span>
        </div>
        <div v-if="opsRows.length === 0" style="color: #888; padding: 8px 0">暂无状态</div>
        <div v-for="row in opsRows" :key="row.name" class="ops-row">
          <span>{{ row.name }}</span>
          <span class="ops-time">{{ row.last }}</span>
          <span class="ops-time">{{ row.next }}</span>
          <span class="ops-note">{{ row.note }}</span>
        </div>
      </div>
    </n-card>

    <n-card title="动态验证（TOTP）">
      <template #header-extra>
        <n-tag v-if="me" :type="me.totp_enabled ? 'success' : 'default'" size="small">
          {{ me.totp_enabled ? '已启用' : '未启用' }}
        </n-tag>
      </template>

      <template v-if="setup">
        <p style="margin: 0 0 12px; color: #666">
          用验证器 App 扫码（或手动输入密钥），然后填一次动态码确认。
        </p>
        <div style="display: flex; flex-wrap: wrap; gap: 16px; align-items: flex-start">
          <img :src="qr" alt="TOTP 二维码" style="width: 180px; height: 180px" />
          <div style="min-width: 200px; flex: 1">
            <div style="color: #666; font-size: 12px">发行方 / 账户</div>
            <div style="margin-bottom: 8px">{{ setup.issuer }} / {{ setup.account }}</div>
            <div style="color: #666; font-size: 12px">密钥（手动输入用）</div>
            <div style="font-family: monospace; word-break: break-all; margin-bottom: 12px">{{ setup.secret }}</div>
            <n-form label-placement="top">
              <n-form-item label="6 位动态码">
                <n-input v-model:value="code" placeholder="000000" maxlength="6" style="width: 140px" />
              </n-form-item>
            </n-form>
            <n-button type="primary" :loading="busy" @click="confirmSetup">确认启用</n-button>
          </div>
        </div>
      </template>

      <template v-else-if="!me?.totp_enabled">
        <p style="margin: 0 0 12px; color: #666">
          用验证器 App（Ente Auth、Google Authenticator 等）扫码绑定，登录时需要输入 6 位动态码。
        </p>
        <n-button type="primary" :loading="busy" @click="startSetup">绑定验证器</n-button>
      </template>

      <template v-else>
        <p style="margin: 0 0 12px; color: #666">换手机或换验证器时重新绑定；关闭后登录只需密码，安全性会下降。</p>
        <n-space :wrap="true">
          <n-button v-if="!showDisable" :loading="busy" @click="startSetup">更换验证器</n-button>
          <n-button v-if="!showDisable" @click="showDisable = true">关闭动态验证</n-button>
        </n-space>
        <n-form v-if="showDisable" label-placement="top" style="max-width: 320px; margin-top: 12px">
          <n-form-item label="当前密码">
            <n-input v-model:value="disablePassword" type="password" show-password-on="click" />
          </n-form-item>
          <n-form-item label="当前动态码">
            <n-input v-model:value="disableCode" placeholder="000000" maxlength="6" />
          </n-form-item>
          <n-button type="error" :loading="busy" @click="disable">确认关闭</n-button>
        </n-form>
      </template>
    </n-card>
  </div>
</template>

<style scoped>
.ops-table {
  overflow-x: auto;
}
.ops-row {
  display: grid;
  grid-template-columns: 72px 132px 132px minmax(160px, 1fr);
  gap: 8px;
  align-items: center;
  padding: 8px 0;
  font-size: 13px;
  border-bottom: 1px solid rgba(18, 21, 28, 0.05);
  min-width: 520px;
}
.ops-row:last-child {
  border-bottom: none;
}
.ops-head {
  color: #6b7280;
  font-weight: 500;
}
.ops-time {
  font-variant-numeric: tabular-nums;
}
.ops-note {
  color: #8b93a7;
}
</style>
