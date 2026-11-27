<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NAlert, NButton, NCard, NForm, NFormItem, NInputNumber, NInput, NSelect, NSpace, NSwitch, NTag,
} from 'naive-ui'
import { api } from '../api'
import { useMobile } from '../useMobile'

const err = ref('')
const notice = ref('')
const busy = ref(false)
const kamis = ref<string[]>([])
const batchId = ref(0)
const isMobile = useMobile()

// 卡类型只保留时间类
const kindOptions = [
  { label: '天卡（1 天 / 份）', value: 'day' },
  { label: '周卡（7 天 / 份）', value: 'week' },
  { label: '月卡（30 天 / 份）', value: 'month' },
  { label: '永久卡', value: 'forever' },
  { label: '自定义时间', value: 'custom' },
]

const BASE: Record<string, number> = { day: 86400, week: 7 * 86400, month: 30 * 86400 }

const form = ref({
  kind: 'week',
  multiplier: 1, // 单码卡密：一张卡的时长倍数
  value: 1, // 自定义时间的数值
  unit: 'day', // 自定义时间的单位
  length: 32,
  prefix: 'DV',
  qty: 1, // 卡密数量（生成几张）
  deviceLimit: 1,
  note: '',
})

const custom = ref(false)
const customText = ref('')
const policyId = ref<number | null>(null)
const policies = ref<Array<{ id: number; name: string; code: string }>>([])

const unitLabel: Record<string, string> = { minute: '分钟', hour: '小时', day: '天' }

// 一张卡的实际时长（秒）
const perCardSeconds = computed(() => {
  const f = form.value
  if (f.kind === 'forever') return 0
  if (f.kind === 'custom') {
    const unit = f.unit === 'minute' ? 60 : f.unit === 'hour' ? 3600 : 86400
    return f.value * unit * f.multiplier
  }
  return (BASE[f.kind] ?? 0) * f.multiplier
})

function human(seconds: number): string {
  if (seconds <= 0) return '永久'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const parts: string[] = []
  if (d) parts.push(`${d} 天`)
  if (h) parts.push(`${h} 小时`)
  if (m) parts.push(`${m} 分钟`)
  if (parts.length === 0) parts.push(`${seconds} 秒`)
  return parts.join(' ')
}

// 单码卡密旁边的人话提示
const multiplierHint = computed(() => {
  const f = form.value
  if (f.kind === 'forever') return '永久卡不受倍数影响'
  if (f.kind === 'custom') {
    return `${f.value} ${unitLabel[f.unit]} × ${f.multiplier} = ${human(perCardSeconds.value)}`
  }
  const base = f.kind === 'day' ? '天' : f.kind === 'week' ? '周' : '月'
  return `${f.multiplier} ${base} = ${human(perCardSeconds.value)}`
})

const customLines = computed(() =>
  customText.value.split('\n').map((s) => s.trim()).filter(Boolean),
)

async function loadPolicies() {
  try {
    const apps = await api.get<Array<{ id: number }>>('/admin/api/apps')
    const appId = apps[0]?.id ?? 1
    policies.value = await api.get<Array<{ id: number; name: string; code: string }>>(
      `/admin/api/policies?app_id=${appId}`,
    )
  } catch {
    // 自定义卡密模式才需要
  }
}

async function generate() {
  err.value = ''
  notice.value = ''
  kamis.value = []
  busy.value = true
  try {
    if (custom.value) {
      if (!policyId.value) {
        err.value = '自定义卡密需要先选一个卡类型（决定时长与权限）'
        return
      }
      if (customLines.value.length === 0) {
        err.value = '请每行填一个卡密'
        return
      }
      const d = await api.post<{ batch_id: number; kamis: string[] }>('/admin/api/cards/generate', {
        policy_id: policyId.value,
        kamis: customLines.value,
        qty: customLines.value.length,
        note: form.value.note,
      })
      batchId.value = d.batch_id
      kamis.value = d.kamis
      notice.value = `已生成 ${d.kamis.length} 张自定义卡密`
      return
    }

    const d = await api.post<{ batch_id: number; kamis: string[] }>('/admin/api/cards/quick', {
      kind: form.value.kind,
      multiplier: form.value.multiplier,
      value: form.value.value,
      unit: form.value.unit,
      device_limit: form.value.deviceLimit,
      length: form.value.length,
      prefix: form.value.prefix,
      qty: form.value.qty,
      note: form.value.note,
    })
    batchId.value = d.batch_id
    kamis.value = d.kamis
    notice.value = `已生成 ${d.kamis.length} 张，每张 ${human(perCardSeconds.value)}`
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function onFile(e: Event) {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  const reader = new FileReader()
  reader.onload = () => {
    const text = String(reader.result ?? '')
    const lines = text
      .split(/\r?\n/)
      .map((l) => l.split(',')[0].trim())
      .filter((l) => l && !/^kami$/i.test(l))
    customText.value = lines.join('\n')
  }
  reader.readAsText(f)
}

async function copyAll() {
  await navigator.clipboard.writeText(kamis.value.join('\n'))
  notice.value = '已复制'
}

onMounted(loadPolicies)
</script>

<template>
  <n-card title="发卡">
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-form :label-placement="isMobile ? 'top' : 'left'" label-width="110">
      <n-form-item label="自定义卡密">
        <n-switch v-model:value="custom" />
        <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">
          关闭则按下面的设置批量生成
        </span>
      </n-form-item>

      <template v-if="custom">
        <n-form-item label="卡类型">
          <n-select
            v-model:value="policyId"
            :options="policies.map((p) => ({ label: `${p.name}（${p.code}）`, value: p.id }))"
            placeholder="决定时长与权限"
          />
        </n-form-item>
        <n-form-item label="卡密列表">
          <n-input v-model:value="customText" type="textarea" :rows="8" placeholder="VIP-0001&#10;VIP-0002" />
        </n-form-item>
        <n-form-item label="导入 / 数量">
          <label class="csv-btn">
            选择 CSV
            <input type="file" accept=".csv,text/csv,text/plain" @change="onFile" />
          </label>
          <n-tag size="small" style="margin-left: 10px">{{ customLines.length }} 张</n-tag>
        </n-form-item>
      </template>

      <template v-else>
        <n-form-item label="卡类型">
          <n-select v-model:value="form.kind" :options="kindOptions" />
        </n-form-item>

        <n-form-item v-if="form.kind === 'custom'" label="自定义时长">
          <n-input-number v-model:value="form.value" :min="1" :max="100000" style="width: 140px" />
          <n-select
            v-model:value="form.unit"
            :options="[{ label: '分钟', value: 'minute' }, { label: '小时', value: 'hour' }, { label: '天', value: 'day' }]"
            style="width: 110px; margin-left: 8px"
          />
        </n-form-item>

        <n-form-item label="单码卡密">
          <n-input-number v-model:value="form.multiplier" :min="1" :max="10000" style="width: 140px" />
          <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">
            {{ multiplierHint }}
          </span>
        </n-form-item>

        <n-form-item label="卡密数量">
          <n-input-number v-model:value="form.qty" :min="1" :max="500" />
          <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">生成几张卡</span>
        </n-form-item>

        <n-form-item label="卡密长度">
          <n-input-number v-model:value="form.length" :min="4" :max="128" />
          <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">随机部分，默认 32</span>
        </n-form-item>

        <n-form-item label="卡密前缀">
          <n-input v-model:value="form.prefix" placeholder="DV" style="width: 160px" />
        </n-form-item>

        <n-form-item label="设备绑定数量">
          <n-input-number v-model:value="form.deviceLimit" :min="1" :max="100" />
        </n-form-item>
      </template>

      <n-form-item label="备注">
        <n-input v-model:value="form.note" placeholder="客服记录，不进审计" />
      </n-form-item>

      <n-button type="primary" :loading="busy" @click="generate">生成</n-button>
    </n-form>

    <template v-if="kamis.length">
      <n-space align="center" style="margin: 20px 0 8px" :wrap="true">
        <n-tag type="success">批次 {{ batchId }}</n-tag>
        <span>共 {{ kamis.length }} 张，每张 {{ human(perCardSeconds) }}</span>
        <n-button size="small" @click="copyAll">复制全部</n-button>
      </n-space>
      <div class="kami-box">
        <div v-for="k in kamis" :key="k">{{ k }}</div>
      </div>
    </template>
  </n-card>
</template>

<style scoped>
.csv-btn {
  font-size: 13px;
  color: #4f46e5;
  cursor: pointer;
}
.csv-btn input {
  display: none;
}
.kami-box {
  max-height: 320px;
  overflow: auto;
  background: #fafafa;
  padding: 12px;
  border-radius: 4px;
  font-family: ui-monospace, monospace;
  font-size: 13px;
  line-height: 1.8;
  word-break: break-all;
}
</style>
