<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NFormItem, NInput, NInputNumber, NModal, NSpace, NTag } from 'naive-ui'
import { api } from '../api'
import { formatYuan } from '../utils'

interface Plan { id: number; name: string; active: number; items: Record<string, number> }

const kinds = [
  { key: 'day', label: '天卡（1 天）' },
  { key: 'week', label: '周卡（7 天）' },
  { key: 'month', label: '月卡（30 天）' },
  { key: 'forever', label: '永久卡' },
]

const plans = ref<Plan[]>([])
const err = ref('')
const notice = ref('')
const busy = ref(false)
const show = ref(false)
const editing = ref<{ id: number; name: string; prices: Record<string, number> }>({ id: 0, name: '', prices: {} })

async function load() {
  err.value = ''
  try {
    plans.value = await api.get<Plan[]>('/admin/api/plans')
  } catch (e) {
    err.value = (e as Error).message
  }
}

function openNew() {
  editing.value = { id: 0, name: '', prices: {} }
  show.value = true
}

function openEdit(p: Plan) {
  const prices: Record<string, number> = {}
  for (const k of kinds) prices[k.key] = (p.items[k.key] ?? 0) / 100
  editing.value = { id: p.id, name: p.name, prices }
  show.value = true
}

async function save() {
  err.value = ''
  notice.value = ''
  if (!editing.value.name) { err.value = '模板名必填'; return }
  busy.value = true
  try {
    const items: Record<string, number> = {}
    for (const k of kinds) {
      const v = editing.value.prices[k.key] ?? 0
      if (v > 0) items[k.key] = Math.round(v * 100)
    }
    await api.post('/admin/api/plans', { id: editing.value.id, name: editing.value.name, items })
    notice.value = '模板已保存'
    show.value = false
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function preview(p: Plan): string {
  return kinds
    .filter((k) => p.items[k.key] > 0)
    .map((k) => `${k.label.split('（')[0]} ${formatYuan(p.items[k.key])}`)
    .join(' · ') || '（空）'
}

onMounted(load)
</script>

<template>
  <div>
    <n-space style="margin-bottom: 12px">
      <n-button type="primary" @click="openNew">新建模板</n-button>
      <n-button @click="load">刷新</n-button>
    </n-space>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <div v-if="plans.length === 0" style="color: #888; font-size: 13px">还没有价格模板</div>

    <n-card v-for="p in plans" :key="p.id" size="small" style="margin-bottom: 12px">
      <template #header>
        {{ p.name }}
        <n-tag v-if="!p.active" size="tiny" type="default" style="margin-left: 8px">停用</n-tag>
      </template>
      <template #header-extra>
        <n-button size="small" @click="openEdit(p)">编辑</n-button>
      </template>
      <div style="font-size: 13px; color: #4a5568">{{ preview(p) }}</div>
    </n-card>

    <n-modal v-model:show="show" preset="card" :title="editing.id ? '编辑模板' : '新建模板'" style="width: min(460px, 94vw)">
      <n-form-item label="模板名">
        <n-input v-model:value="editing.name" placeholder="基础代理价" />
      </n-form-item>
      <p style="font-size: 12px; color: #8b93a7; margin: 0 0 8px">
        填 0 表示模板里不提供这个类型（代理看不到、也发不了）。
      </p>
      <div v-for="k in kinds" :key="k.key" class="row">
        <span>{{ k.label }}</span>
        <n-input-number v-model:value="editing.prices[k.key]" :min="0" :show-button="false" style="width: 140px">
          <template #suffix>元</template>
        </n-input-number>
      </div>
      <n-button type="primary" block :loading="busy" style="margin-top: 12px" @click="save">保存</n-button>
    </n-modal>
  </div>
</template>

<style scoped>
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 0;
  font-size: 14px;
}
</style>
