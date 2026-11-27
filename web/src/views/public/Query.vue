<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NInput, NTag } from 'naive-ui'
import { formatExpiry, cardKindLabel, statusLabels } from '../../utils'

interface QueryResult {
  status: string
  kind: string
  duration_seconds: number
  expire_at: number
  points: number
  remain_times: number
  remaining_seconds: number
  paused: boolean
  device_limit: number
  bound_devices: number
  devices?: { id: number }[]
  unbind_ticket?: string
}

const host = location.hostname
const isLocal = host === 'localhost' || host === '127.0.0.1' || host === ''
const unbindURL = isLocal ? '/?role=unbind' : 'https://unbind.namikao.com/'

const kami = ref('')
const captchaId = ref('')
const captchaImg = ref('')
const captchaCode = ref('')
const result = ref<QueryResult | null>(null)
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

const balance = computed(() => {
  const r = result.value
  if (!r) return '—'
  if (r.kind === 'credit') return `${r.points} 点`
  if (r.kind === 'count') return `${r.remain_times} 次`
  if (r.kind === 'timed') return `${Math.floor(r.remaining_seconds / 60)} 分钟`
  return '—'
})

// 验证码一次有效：失败后必须换一张，成功查询后也换（下次查询要用）。
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
  result.value = null
  notice.value = ''
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

// 整卡换绑：用查询时拿到的 ticket + 全部设备 id，规则由后端按卡类型判定。
async function unbind() {
  if (!result.value) return
  err.value = ''
  notice.value = ''
  const ticket = result.value.unbind_ticket
  const ids = (result.value.devices ?? []).map((d) => d.id)
  if (!ticket || ids.length === 0) {
    err.value = ticket ? '没有已绑定的设备' : '暂不支持自助解绑，请联系客服'
    return
  }
  unbinding.value = true
  try {
    const res = await fetch('/v1/query/unbind', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kami: kami.value.trim(), ticket, device_ids: ids }),
    })
    const body = await res.json()
    if (!body.ok) {
      err.value = body.message ?? '解绑失败'
      return
    }
    // 解绑已成功；重新查卡需要新验证码，这里直接更新本地结果。
    const n = body.data?.unbound ?? ids.length
    if (result.value) {
      result.value.bound_devices = 0
      result.value.devices = []
    }
    notice.value = `已解绑 ${n} 台设备，下次登录会重新绑定当前设备`
  } catch {
    err.value = '网络错误，请稍后再试'
  } finally {
    unbinding.value = false
  }
}
</script>

<template>
  <div class="wrap">
    <header class="head">
    <a class="brand" href="/"><span class="mark">D</span>Devour<b>Auth</b></a>
    <span class="sep"></span>
    <span class="hint">卡密查询</span>
    <a class="back" href="/">返回首页</a>
  </header>

    <main class="main">
      <h1>查询卡密状态</h1>
      <p class="lede">输入卡密即可查看到期时间、剩余次数/点数/时长与已绑定设备数。</p>

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
          <span class="label">剩余</span>
          <span>{{ balance }}</span>
        </div>
        <div class="row">
          <span class="label">设备</span>
          <span>{{ result.bound_devices }} / {{ result.device_limit }}</span>
        </div>
        <div v-if="result.paused" class="row">
          <span class="label">计时</span>
          <span style="color: #d97706">已暂停</span>
        </div>

        <div v-if="result.bound_devices > 0" style="margin-top: 16px">
          <n-button size="small" :loading="unbinding" @click="unbind">解绑全部设备</n-button>
          <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">
            受卡类型的换绑规则限制（免费次数或扣减时长）
          </span>
        </div>
      </n-card>

      <p class="foot">
        查询不到或状态异常？联系发放卡密的代理或客服。
      </p>
      <p class="foot">
        需要单独解绑某台设备，去
        <a :href="unbindURL">设备解绑页</a>。
      </p>
    </main>
  </div>
</template>

<style scoped>
.wrap {
  min-height: 100vh;
  background: radial-gradient(120% 80% at 50% 0%, #eef8f2 0%, #f6f8f7 60%, #f6f8f7 100%);
}
.head {
  position: sticky;
  top: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 24px;
  background: rgba(255, 255, 255, 0.86);
  backdrop-filter: blur(10px);
  border-bottom: 1px solid #e8eeeb;
}
.brand {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 700;
  font-size: 15px;
  letter-spacing: -0.01em;
  text-decoration: none;
  color: #12151c;
}
.brand b { color: #18a058; font-weight: 700; }
.brand .mark {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: grid;
  place-items: center;
  font-size: 13px;
  font-weight: 800;
  color: #04140c;
  background: linear-gradient(135deg, #4ade9b, #18a058);
}
.sep { width: 1px; height: 14px; background: #dfe7e3; }
.hint { font-size: 13px; color: #6b7280; }
.back { margin-left: auto; font-size: 13px; color: #4a5568; text-decoration: none; }
.back:hover { color: #12151c; }
.main {
  max-width: 560px;
  margin: 28px auto 80px;
  padding: 28px 26px 30px;
  background: #fff;
  border: 1px solid #e8eeeb;
  border-radius: 16px;
  box-shadow: 0 1px 2px rgba(16, 24, 40, 0.04), 0 24px 48px -40px rgba(16, 24, 40, 0.5);
}
h1 { margin: 0 0 8px; font-size: 22px; letter-spacing: -0.02em; }
.lede { margin: 0 0 22px; color: #5b6478; font-size: 13.5px; line-height: 1.7; }
.form { display: flex; gap: 10px; flex-wrap: wrap; }
.form > :first-child { flex: 1; min-width: 200px; }
.captcha { display: flex; align-items: center; gap: 10px; margin-top: 10px; }
.captcha > :first-child { flex: 1; min-width: 120px; }
.captcha-img {
  height: 50px;
  width: 160px;
  object-fit: cover;
  border-radius: 8px;
  cursor: pointer;
  border: 1px solid #e8eeeb;
  background: #fff;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 11px 0;
  border-bottom: 1px solid #f1f4f3;
  font-size: 14px;
}
.row:last-child { border-bottom: none; }
.label { color: #8b93a7; font-size: 13px; }
.foot { margin: 28px 0 0; font-size: 13px; color: #8b93a7; line-height: 1.7; }
.foot a { color: #148c4f; text-decoration: none; }
.foot a:hover { text-decoration: underline; }
@media (max-width: 640px) {
  .main { margin: 16px 12px 48px; padding: 22px 18px 24px; }
  .head { padding: 10px 14px; }
}
</style>
