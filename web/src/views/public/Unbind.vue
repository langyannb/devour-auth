<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NCheckbox, NInput, NTag } from 'naive-ui'
import { formatExpiry, formatTs, cardKindLabel, statusLabels } from '../../utils'
import PublicPage from './PublicPage.vue'
import './public.css'

interface Device {
  id: number
  fingerprint: string
  model: string
  brand: string
  os: string
  first_seen: number
  last_seen: number
}

interface UnbindInfo {
  free_left: number
  free_total: number
  cooldown_days: number
  cost_hours: number
  mode: 'reject' | 'charge'
  next_at: number
}

interface QueryResult {
  status: string
  kind: string
  duration_seconds: number
  expire_at: number
  remaining_seconds: number
  paused: boolean
  device_limit: number
  bound_devices: number
  devices: Device[]
  unbind: UnbindInfo
  unbind_ticket?: string
}

const kami = ref('')
const captchaId = ref('')
const captchaImg = ref('')
const captchaCode = ref('')
const result = ref<QueryResult | null>(null)
const selected = ref<number[]>([])
const err = ref('')
const notice = ref('')
const busy = ref(false)
const unbinding = ref(false)

const statusType = computed(() => {
  switch (result.value?.status) {
    case 'active':
      return 'success'
    case 'banned':
    case 'recycled':
      return 'error'
    case 'expired':
    case 'exhausted':
      return 'warning'
    default:
      return 'default'
  }
})

const cooling = computed(() => {
  const at = result.value?.unbind.next_at ?? 0
  return at > 0 && at * 1000 > Date.now()
})

// 扣时换绑只对按时长计费的卡有意义，后端对其余卡类型直接拒绝。
const chargeable = computed(() => {
  const k = result.value?.kind
  return k === 'timed' || k === 'duration' || k === 'trial'
})

// 解绑被拒的情况：没有 ticket（查询过期）、冷却中、免费次数用完且不允许扣时。
const blockReason = computed(() => {
  const r = result.value
  if (!r) return ''
  if (!r.unbind_ticket) return '查询已过期，请重新查询'
  if (r.bound_devices === 0) return '当前没有已绑定的设备'
  if (cooling.value) return `换绑冷却中，${formatTs(r.unbind.next_at)} 后可再解绑`
  if (r.unbind.mode === 'reject' && r.unbind.free_left <= 0) return '免费换绑次数已用完，请联系客服'
  if (!chargeable.value && r.unbind.free_left <= 0) return '该卡类型不支持自助解绑，请联系客服'
  return ''
})

// 验证码一次有效：失败或成功后都要换新图，下次查询才能用。
async function loadCaptcha() {
  captchaCode.value = ''
  try {
    const res = await fetch('/v1/query/captcha')
    const body = await res.json()
    const d = body.data
    if (d?.id && d?.image) {
      captchaId.value = d.id
      captchaImg.value = d.image
    }
  } catch {
    // 拉取失败时保持旧图，查询会提示验证码错误
  }
}

onMounted(loadCaptcha)

async function query() {
  err.value = ''
  notice.value = ''
  result.value = null
  selected.value = []
  if (!kami.value.trim()) {
    err.value = '请输入卡密'
    return
  }
  if (!captchaCode.value.trim()) {
    err.value = '请输入验证码'
    return
  }
  busy.value = true
  try {
    const res = await fetch('/v1/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        kami: kami.value.trim(),
        captcha_id: captchaId.value,
        captcha_code: captchaCode.value.trim(),
      }),
    })
    const body = await res.json()
    if (!body.ok) {
      err.value = body.message ?? '查询失败'
      return
    }
    result.value = body.data
  } catch {
    err.value = '网络错误，请稍后再试'
  } finally {
    busy.value = false
    await loadCaptcha()
  }
}

function toggleDevice(id: number, on: boolean) {
  if (on) {
    if (!selected.value.includes(id)) selected.value.push(id)
  } else {
    selected.value = selected.value.filter((x) => x !== id)
  }
}

async function unbind() {
  const r = result.value
  if (!r) return
  err.value = ''
  notice.value = ''
  if (selected.value.length === 0) {
    err.value = '请先选择要解绑的设备'
    return
  }
  if (!r.unbind_ticket) {
    err.value = '查询已过期，请重新查询'
    return
  }
  unbinding.value = true
  try {
    const res = await fetch('/v1/query/unbind', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        kami: kami.value.trim(),
        ticket: r.unbind_ticket,
        device_ids: selected.value,
      }),
    })
    const body = await res.json()
    if (!body.ok) {
      err.value = body.message ?? '解绑失败'
      return
    }
    const n = body.data?.unbound ?? 0
    const h = body.data?.charged_hours ?? 0
    result.value = null
    selected.value = []
    notice.value = h > 0 ? `已解绑 ${n} 台，扣时 ${h} 小时` : `已解绑 ${n} 台`
    await loadCaptcha()
  } catch {
    err.value = '网络错误，请稍后再试'
  } finally {
    unbinding.value = false
  }
}
</script>

<template>
  <PublicPage title="设备解绑">

      <h1>解绑已登录设备</h1>
      <p class="lede">输入卡密后勾选需要解绑的设备。解绑占用卡类型的换绑次数或扣减时长。</p>

      <div class="form">
        <n-input
          v-model:value="kami"
          placeholder="DV-XXXX-XXXX-XXXX"
          size="large"
          clearable
          @keyup.enter="query"
        />
        <n-button type="primary" size="large" :loading="busy" @click="query">查询</n-button>
      </div>

      <div class="captcha">
        <n-input
          v-model:value="captchaCode"
          placeholder="图形验证码"
          size="large"
          :maxlength="4"
          @keyup.enter="query"
        />
        <img
          v-if="captchaImg"
          :src="captchaImg"
          class="captcha-img"
          alt="验证码"
          title="点击刷新"
          @click="loadCaptcha"
        />
      </div>

      <n-alert v-if="err" type="error" style="margin-top: 16px">{{ err }}</n-alert>
      <n-alert v-if="notice" type="success" style="margin-top: 16px">{{ notice }}</n-alert>

      <n-card v-if="result" style="margin-top: 20px">
        <div class="row">
          <span class="label">状态</span>
          <n-tag :type="statusType" size="small">{{ statusLabels[result.status] ?? result.status }}</n-tag>
        </div>
        <div class="row">
          <span class="label">类型</span>
          <span>{{ cardKindLabel(result.kind, result.duration_seconds) }}</span>
        </div>
        <div class="row">
          <span class="label">到期</span>
          <span>{{ formatExpiry(result.kind, result.expire_at, result.remaining_seconds) }}</span>
        </div>
        <div class="row">
          <span class="label">设备</span>
          <span>{{ result.bound_devices }} / {{ result.device_limit }}</span>
        </div>
        <div class="row">
          <span class="label">免费换绑</span>
          <span>{{ result.unbind.free_left }} / {{ result.unbind.free_total }} 次</span>
        </div>
        <div v-if="result.unbind.cooldown_days > 0" class="row">
          <span class="label">换绑冷却</span>
          <span :class="{ warn: cooling }">
            {{ cooling ? `冷却至 ${formatTs(result.unbind.next_at)}` : `每次解绑后 ${result.unbind.cooldown_days} 天` }}
          </span>
        </div>
        <div v-if="result.unbind.mode === 'charge' && chargeable" class="row">
          <span class="label">扣时</span>
          <span>每次解绑扣 {{ result.unbind.cost_hours }} 小时</span>
        </div>

        <div v-if="result.devices.length > 0" class="devices">
          <div
            v-for="d in result.devices"
            :key="d.id"
            class="device"
            @click="toggleDevice(d.id, !selected.includes(d.id))"
          >
            <n-checkbox
              :checked="selected.includes(d.id)"
              @click.stop
              @update:checked="(v: boolean) => toggleDevice(d.id, v)"
            />
            <span class="device-body">
              <span class="device-name">{{ [d.brand, d.model].filter(Boolean).join(' ') || d.os || '未知设备' }}</span>
              <span class="device-meta">{{ d.os || '—' }} · {{ d.fingerprint }}</span>
              <span class="device-meta">最近活跃 {{ formatTs(d.last_seen) }}</span>
            </span>
          </div>
        </div>

        <div class="actions">
          <n-button
            type="primary"
            :loading="unbinding"
            :disabled="!!blockReason"
            @click="unbind"
          >
            解绑所选
          </n-button>
          <span v-if="blockReason" class="note">{{ blockReason }}</span>
          <span v-else-if="result.unbind.mode === 'charge' && chargeable" class="note">
            勾选多台也只按一次计算
          </span>
        </div>
      </n-card>

      <p class="foot">解绑后，旧设备需要重新登录。</p>
  </PublicPage>
</template>

<style scoped>
.warn { color: #d97706; }
.devices { margin-top: 14px; display: flex; flex-direction: column; gap: 8px; }
.device {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 10px 12px;
  border: 1px solid #e8eeeb;
  border-radius: 10px;
  background: #fbfcfc;
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}
.device:hover { border-color: #bfe3cf; background: #f3fbf7; }
.device-body { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.device-name { font-size: 14px; font-weight: 600; color: #12151c; }
.device-meta { font-size: 12px; color: #8b93a7; word-break: break-all; }
.actions { margin-top: 16px; display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.note { font-size: 12px; color: #8b93a7; }
</style>
