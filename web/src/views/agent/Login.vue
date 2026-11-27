<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput } from 'naive-ui'
import { agentApi, agentAuthed, setCSRF } from '../../api'

const router = useRouter()
const name = ref('')
const password = ref('')
const err = ref('')
const busy = ref(false)

interface LoginData {
  csrf_token: string
  name: string
  level: number
  discount_bp: number
}

async function submit() {
  err.value = ''
  busy.value = true
  try {
    const d = await agentApi.post<LoginData>('/agent/api/login', {
      name: name.value,
      password: password.value,
    })
    setCSRF(d.csrf_token, 'agent')
    sessionStorage.setItem('devour_agent_name', `${d.name} · L${d.level}`)
    agentAuthed.value = true
    router.push('/cards')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div style="display: flex; align-items: center; justify-content: center; min-height: 100vh; background: #f2f7f4">
    <n-card title="Devour 代理端" style="width: min(360px, 92vw)">
      <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
      <n-form @keyup.enter="submit">
        <n-form-item label="代理名">
          <n-input v-model:value="name" placeholder="agent" />
        </n-form-item>
        <n-form-item label="密码">
          <n-input v-model:value="password" type="password" show-password-on="click" />
        </n-form-item>
        <n-button type="primary" block :loading="busy" @click="submit">登录</n-button>
      </n-form>
    </n-card>
  </div>
</template>
