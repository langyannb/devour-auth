<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  NAlert, NButton, NCard, NDescriptions, NDescriptionsItem, NFormItem,
  NInputNumber, NSelect, NSpace, NSwitch, NTag,
} from 'naive-ui'
import { api } from '../api'
import { formatYuan } from '../utils'

interface Agent {
  id: number
  name: string
  level: number
  monthly_quota_cents: number
  status: string
}
interface Plan { id: number; name: string; items: Record<string, number> }
interface PlanResp {
  plan_id: number
  plan: Plan | null
  permissions: { SetDeviceLimit: boolean; CustomKami: boolean; MaxQty: number }
}

const route = useRoute()
const router = useRouter()
const id = Number(route.params.id)

const agentInfo = ref<Agent | null>(null)
const plans = ref<Plan[]>([])
const planId = ref<number | null>(null)
const perms = ref({ setDeviceLimit: false, customKami: false, maxQty: 10 })
const err = ref('')
const notice = ref('')
const busy = ref(false)

const kindLabel: Record<string, string> = {
  day: '天卡', week: '周卡', month: '月卡', forever: '永久卡', custom: '自定义时间',
}

const currentPlan = computed(() => plans.value.find((p) => p.id === planId.value) ?? null)

async function load() {
  err.value = ''
  try {
    const [list, allPlans, cur] = await Promise.all([
      api.get<Agent[]>('/admin/api/agents'),
      api.get<Plan[]>('/admin/api/plans'),
      api.get<PlanResp>(`/admin/api/agents/${id}/plan`),
    ])
    agentInfo.value = list.find((a) => a.id === id) ?? null
    plans.value = allPlans
    planId.value = cur.plan_id > 0 ? cur.plan_id : null
    perms.value = {
      setDeviceLimit: cur.permissions.SetDeviceLimit,
      customKami: cur.permissions.CustomKami,
      maxQty: cur.permissions.MaxQty || 10,
    }
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function save() {
  err.value = ''
  notice.value = ''
  busy.value = true
  try {
    await api.put(`/admin/api/agents/${id}/plan`, {
      plan_id: planId.value ?? 0,
      set_device_limit: perms.value.setDeviceLimit,
      custom_kami: perms.value.customKami,
      max_qty: perms.value.maxQty,
    })
    notice.value = '已保存，代理出卡时立即按新模板与权限执行'
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div style="max-width: 820px">
    <n-space style="margin-bottom: 12px">
      <n-button size="small" @click="router.push('/agents')">返回代理列表</n-button>
      <n-button size="small" @click="load">刷新</n-button>
      <n-button size="small" @click="router.push('/plans')">管理价格模板</n-button>
    </n-space>

    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-card v-if="agentInfo" size="small" style="margin-bottom: 16px">
      <template #header>
        {{ agentInfo.name }}
        <n-tag size="small" :type="agentInfo.status === 'active' ? 'success' : 'error'" style="margin-left: 8px">
          {{ agentInfo.status === 'active' ? '正常' : '冻结' }}
        </n-tag>
      </template>
      <n-descriptions :column="2" size="small" bordered label-placement="left">
        <n-descriptions-item label="层级">L{{ agentInfo.level }}</n-descriptions-item>
        <n-descriptions-item label="月额度">
          {{ agentInfo.monthly_quota_cents < 0 ? '不限' : agentInfo.monthly_quota_cents === 0 ? '0（不能出卡）' : formatYuan(agentInfo.monthly_quota_cents) + ' 元' }}
        </n-descriptions-item>
      </n-descriptions>
    </n-card>

    <n-card size="small" title="价格模板与权限">
      <template #header-extra>
        <n-button type="primary" size="small" :loading="busy" @click="save">保存</n-button>
      </template>

      <n-form-item label="价格模板">
        <n-select
          v-model:value="planId"
          :options="plans.map((p) => ({ label: p.name, value: p.id }))"
          placeholder="选择模板（不选则不能出卡）"
          clearable
        />
      </n-form-item>

      <div v-if="currentPlan" style="margin: 4px 0 16px; font-size: 13px; color: #4a5568">
        <span style="color: #8b93a7">该模板提供：</span>
        <span v-for="(price, kind) in currentPlan.items" :key="kind" style="margin-right: 12px">
          {{ kindLabel[kind] ?? kind }} {{ formatYuan(price) }} 元
        </span>
      </div>

      <n-form-item label="允许代理设置设备数量">
        <n-switch v-model:value="perms.setDeviceLimit" />
        <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">关闭则固定 1 台</span>
      </n-form-item>

      <n-form-item label="允许自定义卡密">
        <n-switch v-model:value="perms.customKami" />
        <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">关闭则只能随机生成</span>
      </n-form-item>

      <n-form-item label="单批最多张数">
        <n-input-number v-model:value="perms.maxQty" :min="1" :max="500" style="width: 140px" />
      </n-form-item>
    </n-card>
  </div>
</template>
