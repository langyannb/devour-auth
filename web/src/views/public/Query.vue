<script setup lang="ts">
import { computed, ref } from 'vue'
import { NAlert, NButton, NCard, NInput, NTag } from 'naive-ui'
import { formatTs, kindLabels, statusLabels } from '../../utils'

interface QueryResult {
  status: string
  kind: string
  expire_at: number
  points: number
  remain_times: number
  remaining_seconds: number
  paused: boolean
  device_limit: number
  bound_devices: number
}

const kami = ref('')
const result = ref<QueryResult | null>(null)
const err = ref('')
const busy = ref(false)

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
  return '不限'
})

async function query() {
  err.value = ''
  result.value = null
  if (!kami.value.trim()) {
    err.value = '请输入卡密'
    return
  }
  busy.value = true
  try {
    const res = await fetch('/v1/query', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ kami: kami.value.trim() }),
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
      <p class="lede">输入卡密即可查看到期时间、剩余余额与已绑定设备数。</p>

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

      <n-alert v-if="err" type="error" style="margin-top: 16px">{{ err }}</n-alert>

      <n-card v-if="result" style="margin-top: 20px">
        <div class="row">
          <span class="label">状态</span>
          <n-tag :type="statusType" size="small">{{ statusLabels[result.status] ?? result.status }}</n-tag>
        </div>
        <div class="row">
          <span class="label">类型</span>
          <span>{{ kindLabels[result.kind] ?? result.kind }}</span>
        </div>
        <div class="row">
          <span class="label">到期</span>
          <span>{{ result.expire_at ? formatTs(result.expire_at) : '永久' }}</span>
        </div>
        <div class="row">
          <span class="label">余额</span>
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
