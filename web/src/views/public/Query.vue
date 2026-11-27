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
}

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

// 自助解绑：只走策略里的免费次数，超出提示联系客服
async function unbind() {
  if (!result.value) return
  err.value = ''
  notice.value = ''
  unbinding.value = true
  try {
    const res = await fetch('/v1/query/unbind', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kami: kami.value.trim() }),
    })
    const body = await res.json()
    if (!body.ok) {
      err.value = body.message ?? '解绑失败'
      return
    }
    // 解绑已成功；重新查卡需要新验证码，这里直接更新本地结果。
    if (result.value) result.value.bound_devices = 0
    notice.value = '已解绑全部设备，下次登录会重新绑定当前设备'
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
      <a class="brand" href="/">Devour<span>Auth</span></a>
      <span class="hint">卡密查询</span>
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
    </main>
  </div>
</template>

<style scoped>
.wrap {
  min-height: 100vh;
  background: #f7f8fb;
}
.head {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 24px;
  border-bottom: 1px solid rgba(18, 21, 28, 0.06);
  background: #fff;
}
.brand {
  font-weight: 800;
  font-size: 17px;
  text-decoration: none;
  color: #12151c;
}
.brand span {
  color: #4f46e5;
}
.hint {
  font-size: 13px;
  color: #6b7280;
}
.main {
  max-width: 520px;
  margin: 0 auto;
  padding: 56px 20px 80px;
}
h1 {
  margin: 0 0 10px;
  font-size: 26px;
  letter-spacing: -0.02em;
}
.lede {
  margin: 0 0 24px;
  color: #5b6478;
  font-size: 14px;
  line-height: 1.7;
}
.form {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.form > :first-child {
  flex: 1;
  min-width: 220px;
}
.captcha {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 10px;
}
.captcha > :first-child {
  flex: 1;
  min-width: 120px;
}
.captcha-img {
  height: 50px;
  width: 160px;
  object-fit: cover;
  border-radius: 6px;
  cursor: pointer;
  border: 1px solid rgba(18, 21, 28, 0.1);
  background: #fff;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid rgba(18, 21, 28, 0.05);
  font-size: 14px;
}
.row:last-child {
  border-bottom: none;
}
.label {
  color: #6b7280;
}
.foot {
  margin-top: 28px;
  font-size: 13px;
  color: #8b93a7;
}
@media (max-width: 640px) {
  .main {
    padding: 32px 16px 60px;
  }
}
</style>
