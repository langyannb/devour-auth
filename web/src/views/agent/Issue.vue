<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  NAlert, NButton, NCard, NForm, NFormItem, NInputNumber, NInput, NSelect, NSpace, NSwitch, NTag,
} from 'naive-ui'
import { agentApi } from '../../api'
import { formatYuan } from '../../utils'
import { useMobile } from '../../useMobile'

interface PlanItem { kind: string; name: string; price_cents: number }
interface PlanResp {
  plan: { id: number; name: string; items: PlanItem[] } | null
  permissions: { SetDeviceLimit: boolean; CustomKami: boolean; MaxQty: number }
}

const err = ref('')
const notice = ref('')
const busy = ref(false)
const isMobile = useMobile()

const plan = ref<PlanResp | null>(null)
const form = ref({
  kind: 'week',
  multiplier: 1,
  value: 1,
  unit: 'day',
  qty: 1,
  deviceLimit: 1,
  prefix: 'AG',
})
const customKami = ref(false)
const customText = ref('')
const kamis = ref<string[]>([])
const batchId = ref(0)

const items = computed(() => plan.value?.plan?.items ?? [])
const perms = computed(() => plan.value?.permissions ?? { SetDeviceLimit: false, CustomKami: false, MaxQty: 10 })
const selected = computed(() => items.value.find((i) => i.kind === form.value.kind))

const total = computed(() => {
  const unit = selected.value?.price_cents ?? 0
  return unit * form.value.qty
})

const customLines = computed(() =>
  customText.value.split('\n').map((s) => s.trim()).filter(Boolean),
)

async function load() {
  err.value = ''
  try {
    const d = await agentApi.get<PlanResp>('/agent/api/plan')
    plan.value = d
    if (d.plan && d.plan.items.length > 0) {
      form.value.kind = d.plan.items[0].kind
    }
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function issue() {
  err.value = ''
  notice.value = ''
  kamis.value = []
  if (!selected.value) { err.value = '请选择卡类型'; return }
  if (form.value.qty > (perms.value.MaxQty || 500)) {
    err.value = `单批最多 ${perms.value.MaxQty} 张`
    return
  }
  if (customKami.value && customLines.value.length === 0) {
    err.value = '请每行填一个卡密'
    return
  }
  busy.value = true
  try {
    const payload: Record<string, unknown> = {
      kind: form.value.kind,
      multiplier: form.value.multiplier,
      qty: customKami.value ? customLines.value.length : form.value.qty,
      prefix: form.value.prefix,
    }
    if (form.value.kind === 'custom') {
      payload.value = form.value.value
      payload.unit = form.value.unit
    }
    if (perms.value.SetDeviceLimit) payload.device_limit = form.value.deviceLimit
    if (customKami.value) payload.kamis = customLines.value

    const d = await agentApi.post<{ batch_id: number; kamis: string[]; payable: number }>(
      '/agent/api/issue', payload,
    )
    batchId.value = d.batch_id
    kamis.value = d.kamis
    notice.value = `已生成 ${d.kamis.length} 张，应付 ${formatYuan(d.payable)} 元`
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function copyAll() {
  await navigator.clipboard.writeText(kamis.value.join('\n'))
  notice.value = '已复制'
}

onMounted(load)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-alert v-if="!plan?.plan" type="warning" style="margin-bottom: 12px">
      管理员还没有给你分配价格模板，暂时不能出卡。
    </n-alert>

    <n-card v-else title="出卡">
      <template #header-extra>
        <n-tag size="small">{{ plan.plan.name }}</n-tag>
      </template>

      <n-form :label-placement="isMobile ? 'top' : 'left'" label-width="110">
        <n-form-item label="卡类型">
          <n-select
            v-model:value="form.kind"
            :options="items.map((i) => ({ label: `${i.name} · ${formatYuan(i.price_cents)} 元`, value: i.kind }))"
          />
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
            {{ form.multiplier }} 份 {{ selected?.name ?? '' }}
          </span>
        </n-form-item>

        <n-form-item label="自定义卡密">
          <n-switch v-model:value="customKami" :disabled="!perms.CustomKami" />
          <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">
            {{ perms.CustomKami ? '开启后每行一个' : '管理员未开放' }}
          </span>
        </n-form-item>

        <template v-if="customKami">
          <n-form-item label="卡密列表">
            <n-input v-model:value="customText" type="textarea" :rows="6" placeholder="VIP-0001&#10;VIP-0002" />
          </n-form-item>
          <n-form-item label="将生成">
            <n-tag size="small">{{ customLines.length }} 张</n-tag>
          </n-form-item>
        </template>

        <template v-else>
          <n-form-item label="数量">
            <n-input-number v-model:value="form.qty" :min="1" :max="perms.MaxQty || 500" />
          </n-form-item>
          <n-form-item label="前缀">
            <n-input v-model:value="form.prefix" style="width: 140px" />
          </n-form-item>
        </template>

        <n-form-item v-if="perms.SetDeviceLimit" label="设备绑定数量">
          <n-input-number v-model:value="form.deviceLimit" :min="1" :max="100" />
        </n-form-item>

        <n-form-item label="应付">
          <n-tag type="info">{{ formatYuan(total) }} 元</n-tag>
          <span style="margin-left: 10px; font-size: 12px; color: #8b93a7">按模板单价 × 数量</span>
        </n-form-item>

        <n-button type="primary" :loading="busy" @click="issue">生成</n-button>
      </n-form>
    </n-card>

    <template v-if="kamis.length">
      <n-space align="center" style="margin: 20px 0 8px" :wrap="true">
        <n-tag type="success">批次 {{ batchId }}</n-tag>
        <span>共 {{ kamis.length }} 张</span>
        <n-button size="small" @click="copyAll">复制全部</n-button>
      </n-space>
      <div class="kami-box">
        <div v-for="k in kamis" :key="k">{{ k }}</div>
      </div>
    </template>
  </div>
</template>

<style scoped>
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
