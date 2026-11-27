<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NAlert, NButton, NCard, NForm, NFormItem, NInputNumber, NInput, NSelect, NSpace, NSwitch, NTag,
} from 'naive-ui'
import { api } from '../api'
import { kindLabels } from '../utils'
import { useMobile } from '../useMobile'

interface Policy {
  id: number
  code: string
  name: string
  kind: string
  device_limit: number
  price_cents: number
}

const policies = ref<Policy[]>([])
const policyId = ref<number | null>(null)
const qty = ref(10)
const prefix = ref('DV')
const note = ref('')
const busy = ref(false)
const err = ref('')
const kamis = ref<string[]>([])
const batchId = ref(0)
const isMobile = useMobile()

const custom = ref(false)
const customText = ref('')

const customLines = computed(() =>
  customText.value
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean),
)

const options = () =>
  policies.value.map((p) => ({
    label: `${p.name}（${p.code} · ${kindLabels[p.kind] ?? p.kind}）`,
    value: p.id,
  }))

async function loadPolicies() {
  try {
    const apps = await api.get<Array<{ id: number }>>('/admin/api/apps')
    const appId = apps[0]?.id ?? 1
    policies.value = await api.get<Policy[]>(`/admin/api/policies?app_id=${appId}`)
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function generate() {
  err.value = ''
  kamis.value = []
  if (!policyId.value) {
    err.value = '请选择卡类型'
    return
  }
  const payload: Record<string, unknown> = {
    policy_id: policyId.value,
    prefix: prefix.value,
    note: note.value,
  }
  if (custom.value) {
    if (customLines.value.length === 0) {
      err.value = '请每行填一个卡密'
      return
    }
    payload.kamis = customLines.value
    payload.qty = customLines.value.length
  } else {
    payload.qty = qty.value
  }

  busy.value = true
  try {
    const d = await api.post<{ batch_id: number; kamis: string[] }>('/admin/api/cards/generate', payload)
    batchId.value = d.batch_id
    kamis.value = d.kamis
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
}

onMounted(loadPolicies)
</script>

<template>
  <n-card title="发卡">
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-form :label-placement="isMobile ? 'top' : 'left'" label-width="110">
      <n-form-item label="卡类型">
        <n-select v-model:value="policyId" :options="options()" placeholder="选择卡类型" />
      </n-form-item>

      <n-form-item label="自定义卡密">
        <n-switch v-model:value="custom" />
        <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">
          关闭则随机生成；开启后每行一个，支持批量粘贴
        </span>
      </n-form-item>

      <template v-if="custom">
        <n-form-item label="卡密列表">
          <n-input
            v-model:value="customText"
            type="textarea"
            :rows="8"
            placeholder="VIP-0001&#10;VIP-0002&#10;VIP-0003"
          />
        </n-form-item>
        <n-form-item label="将生成">
          <n-tag size="small">{{ customLines.length }} 张</n-tag>
          <label class="csv-btn">
            选择 CSV 文件
            <input type="file" accept=".csv,text/csv,text/plain" @change="onFile" />
          </label>
        </n-form-item>
      </template>

<style scoped>
.csv-btn {
  margin-left: 12px;
  font-size: 13px;
  color: #4f46e5;
  cursor: pointer;
}
.csv-btn input {
  display: none;
}
</style>

      <template v-else>
        <n-form-item label="数量">
          <n-input-number v-model:value="qty" :min="1" :max="500" />
        </n-form-item>
        <n-form-item label="前缀">
          <n-input v-model:value="prefix" placeholder="DV" style="width: 160px" />
        </n-form-item>
      </template>

      <n-form-item label="备注">
        <n-input v-model:value="note" placeholder="客服记录，不进审计" />
      </n-form-item>
      <n-button type="primary" :loading="busy" @click="generate">生成</n-button>
    </n-form>

    <template v-if="kamis.length">
      <n-space align="center" style="margin: 20px 0 8px" :wrap="true">
        <n-tag type="success">批次 {{ batchId }}</n-tag>
        <span>共 {{ kamis.length }} 张</span>
        <n-button size="small" @click="copyAll">复制全部</n-button>
      </n-space>
      <div style="max-height: 320px; overflow: auto; background: #fafafa; padding: 12px; border-radius: 4px; font-family: monospace">
        <div v-for="k in kamis" :key="k">{{ k }}</div>
      </div>
    </template>
  </n-card>
</template>
