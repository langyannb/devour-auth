<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NForm, NFormItem, NInputNumber, NInput, NSelect, NSpace, NTag } from 'naive-ui'
import { agentApi } from '../../api'
import { formatYuan, kindLabels } from '../../utils'
import { useMobile } from '../../useMobile'

interface Policy {
  id: number
  code: string
  name: string
  kind: string
  price_cents: number
  agent_price_cents: number
}

const policies = ref<Policy[]>([])
const policyId = ref<number | null>(null)
const qty = ref(10)
const prefix = ref('AG')
const busy = ref(false)
const err = ref('')
const kamis = ref<string[]>([])
const payable = ref(0)
const isMobile = useMobile()

const options = () =>
  policies.value.map((p) => ({
    label: `${p.name}（代理价 ${formatYuan(p.agent_price_cents)} 元）`,
    value: p.id,
  }))

async function loadPolicies() {
  try {
    policies.value = await agentApi.get<Policy[]>('/agent/api/policies')
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function issue() {
  err.value = ''
  kamis.value = []
  if (!policyId.value) {
    err.value = '请选择卡类型'
    return
  }
  busy.value = true
  try {
    const d = await agentApi.post<{ kamis: string[]; payable: number }>('/agent/api/issue', {
      policy_id: policyId.value,
      qty: qty.value,
      prefix: prefix.value,
    })
    kamis.value = d.kamis
    payable.value = d.payable
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
  <n-card title="出卡">
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-form :label-placement="isMobile ? 'top' : 'left'" label-width="90">
      <n-form-item label="卡类型">
        <n-select v-model:value="policyId" :options="options()" placeholder="选择卡类型" />
      </n-form-item>
      <n-form-item label="数量">
        <n-input-number v-model:value="qty" :min="1" :max="500" />
      </n-form-item>
      <n-form-item label="前缀">
        <n-input v-model:value="prefix" style="width: 160px" />
      </n-form-item>
      <n-button type="primary" :loading="busy" @click="issue">生成</n-button>
    </n-form>

    <template v-if="kamis.length">
      <n-space align="center" style="margin: 20px 0 8px" :wrap="true">
        <n-tag type="success">共 {{ kamis.length }} 张</n-tag>
        <span>应付 {{ formatYuan(payable) }} 元</span>
        <n-button size="small" @click="copyAll">复制全部</n-button>
      </n-space>
      <div style="max-height: 320px; overflow: auto; background: #fafafa; padding: 12px; border-radius: 4px; font-family: monospace">
        <div v-for="k in kamis" :key="k">{{ k }}</div>
      </div>
    </template>
  </n-card>
</template>
