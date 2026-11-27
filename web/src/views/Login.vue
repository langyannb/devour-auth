<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput } from 'naive-ui'
import { api, authed, markSignedIn, setCSRF } from '../api'

const router = useRouter()
const username = ref('')
const password = ref('')
const totp = ref('')
const err = ref('')
const busy = ref(false)

interface LoginData {
  csrf_token: string
  admin: { id: number; username: string; role: string }
}

async function submit() {
  err.value = ''
  busy.value = true
  try {
    const d = await api.post<LoginData>('/admin/api/login', {
      username: username.value,
      password: password.value,
      totp: totp.value,
    })
    setCSRF(d.csrf_token)
    sessionStorage.setItem('devour_user', `${d.admin.username} · ${d.admin.role}`)
    authed.value = true
    markSignedIn()
    router.push('/cards')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div style="display: flex; align-items: center; justify-content: center; min-height: 100vh; background: #f5f6f8">
    <n-card title="Devour Auth 管理端" style="width: min(360px, 92vw)">
      <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
      <n-form @keyup.enter="submit">
        <n-form-item label="账号">
          <n-input v-model:value="username" placeholder="admin" autocomplete="username" />
        </n-form-item>
        <n-form-item label="密码">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            autocomplete="current-password"
          />
        </n-form-item>
        <n-form-item label="动态验证码">
          <n-input v-model:value="totp" placeholder="6 位数字" maxlength="6" />
        </n-form-item>
        <n-button type="primary" block :loading="busy" @click="submit">{{ busy ? '登录中…' : '登录' }}</n-button>
      </n-form>
    </n-card>
  </div>
</template>
