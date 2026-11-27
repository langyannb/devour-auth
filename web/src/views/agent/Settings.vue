<script setup lang="ts">
import { ref } from 'vue'
import { NAlert, NButton, NCard, NForm, NFormItem, NInput } from 'naive-ui'
import { agentApi } from '../../api'

const oldPass = ref('')
const newPass = ref('')
const confirm = ref('')
const err = ref('')
const notice = ref('')
const busy = ref(false)

async function submit() {
  err.value = ''
  notice.value = ''
  if (newPass.value !== confirm.value) {
    err.value = '两次输入的新密码不一致'
    return
  }
  busy.value = true
  try {
    await agentApi.post('/agent/api/password', { old_password: oldPass.value, new_password: newPass.value })
    notice.value = '密码已修改'
    oldPass.value = ''
    newPass.value = ''
    confirm.value = ''
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div style="max-width: 420px">
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>
    <n-card title="修改密码" size="small">
      <n-form label-placement="top">
        <n-form-item label="原密码">
          <n-input v-model:value="oldPass" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item label="新密码（至少 12 位）">
          <n-input v-model:value="newPass" type="password" show-password-on="click" />
        </n-form-item>
        <n-form-item label="确认新密码">
          <n-input v-model:value="confirm" type="password" show-password-on="click" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="submit">保存</n-button>
      </n-form>
    </n-card>
  </div>
</template>
