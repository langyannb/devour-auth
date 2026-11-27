<script setup lang="ts">
import { ref } from 'vue'
import { NAlert, NButton, NInput, NModal, NSpace } from 'naive-ui'
import { api, reauthState, resolveReauth } from '../api'

const password = ref('')
const totp = ref('')
const err = ref('')
const busy = ref(false)

async function submit() {
  err.value = ''
  busy.value = true
  try {
    await api.post('/admin/api/reauth', { password: password.value, totp: totp.value })
    password.value = ''
    totp.value = ''
    resolveReauth(true)
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function cancel() {
  password.value = ''
  totp.value = ''
  err.value = ''
  resolveReauth(false)
}
</script>

<template>
  <n-modal
    :show="reauthState.show.value"
    :mask-closable="false"
    :closable="false"
    preset="card"
    title="重新验证身份"
    style="width: min(380px, 92vw)"
  >
    <p style="margin: 0 0 12px; color: #666; font-size: 13px">
      {{ reauthState.message.value }}（5 分钟内有效）
    </p>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-space vertical>
      <n-input v-model:value="password" type="password" show-password-on="click" placeholder="当前密码" />
      <n-input v-model:value="totp" placeholder="6 位动态码" maxlength="6" @keyup.enter="submit" />
      <n-space justify="end">
        <n-button @click="cancel">取消</n-button>
        <n-button type="primary" :loading="busy" @click="submit">确认</n-button>
      </n-space>
    </n-space>
  </n-modal>
</template>
