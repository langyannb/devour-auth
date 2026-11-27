<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NFormItem, NInput, NSpin } from 'naive-ui'
import { agentApi, agentAuthed, setCSRF } from '../../api'

const route = useRoute()
const router = useRouter()
const code = String(route.params.code ?? '')

const info = ref<{ name: string; level: number; activated: boolean; status: string } | null>(null)
const password = ref('')
const confirm = ref('')
const err = ref('')
const busy = ref(false)
const loading = ref(true)

async function load() {
  loading.value = true
  err.value = ''
  try {
    info.value = await agentApi.get<{ name: string; level: number; activated: boolean; status: string }>(
      `/agent/api/invite/${code}`,
    )
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function activate() {
  err.value = ''
  if (password.value.length < 12) {
    err.value = '密码至少 12 位'
    return
  }
  if (password.value !== confirm.value) {
    err.value = '两次输入的密码不一致'
    return
  }
  busy.value = true
  try {
    const d = await agentApi.post<{ csrf_token: string; name: string }>(`/agent/api/invite/${code}`, {
      password: password.value,
    })
    setCSRF(d.csrf_token, 'agent')
    agentAuthed.value = true
    sessionStorage.setItem('devour_user', d.name)
    router.push('/dashboard')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div style="min-height: 100vh; display: flex; align-items: center; justify-content: center; background: #f5f6f8; padding: 20px">
    <n-card style="width: 420px; max-width: 100%" title="下级代理开通">
      <n-spin :show="loading">
        <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>

        <template v-if="info">
          <p style="margin-top: 0; color: #333">
            你好，<b>{{ info.name }}</b>（L{{ info.level }}）
          </p>

          <template v-if="info.activated">
            <p style="color: #666; font-size: 13px">这个链接已经设置过密码了，请直接登录。</p>
            <n-button type="primary" block @click="router.push('/login')">去登录</n-button>
          </template>

          <template v-else>
            <p style="color: #666; font-size: 13px">设置登录密码后就能进入代理端；忘记密码找你的上级重置。</p>
            <n-form-item label="登录密码（至少 12 位）">
              <n-input v-model:value="password" type="password" show-password-on="click" />
            </n-form-item>
            <n-form-item label="再输一次">
              <n-input v-model:value="confirm" type="password" show-password-on="click" @keyup.enter="activate" />
            </n-form-item>
            <n-button type="primary" block :loading="busy" @click="activate">设置密码并登录</n-button>
          </template>
        </template>
      </n-spin>
    </n-card>
  </div>
</template>
