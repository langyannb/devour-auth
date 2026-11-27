<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NForm, NFormItem, NInputNumber, NInput, NSelect, NSpace, NTag } from 'naive-ui'
import { api } from '../api'
import { kindLabels } from '../utils'

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
  busy.value = true
  try {
    const d = await api.post<{ batch_id: number; kamis: string[] }>('/admin/api/cards/generate', {
      policy_id: policyId.value,
      qty: qty.value,
      prefix: prefix.value,
      note: note.value,
    })
    batchId.value = d.batch_id
    kamis.value = d.kamis
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function copyAll() {
  await navigator.clipboard.writeText(kamis.value.join('\n'))
}

onMounted(loadPolicies)
</script>

<template>
  <n-card title="发卡" style="max-width: 720px">
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-form label-placement="left" label-width="90">
      <n-form-item label="卡类型">
        <n-select v-model:value="policyId" :options="options()" placeholder="选择卡类型" />
      </n-form-item>
      <n-form-item label="数量">
        <n-input-number v-model:value="qty" :min="1" :max="500" />
      </n-form-item>
      <n-form-item label="前缀">
        <n-input v-model:value="prefix" placeholder="DV" style="width: 160px" />
      </n-form-item>
      <n-form-item label="备注">
        <n-input v-model:value="note" placeholder="客服记录，不进审计" />
      </n-form-item>
      <n-button type="primary" :loading="busy" @click="generate">生成</n-button>
    </n-form>

    <template v-if="kamis.length">
      <n-space align="center" style="margin: 20px 0 8px">
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
