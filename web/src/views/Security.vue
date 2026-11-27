<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NTag } from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Check {
  audit_chain_ok: boolean
  audit_entries: number
  audit_error?: string
  anchors: number
  anchors_error?: string
  last_anchor_day?: string
  last_anchor_id?: number
  backups: number
  last_backup_file?: string
  last_backup_at?: number
  last_backup_size?: number
  db_size?: number
  admin_host?: string
  agent_host?: string
  server_time: number
}

const data = ref<Check | null>(null)
const err = ref('')
const loading = ref(false)

const mb = (n?: number) => (n ? `${(n / 1024 / 1024).toFixed(2)} MB` : '—')

async function load() {
  loading.value = true
  err.value = ''
  try {
    data.value = await api.get<Check>('/admin/api/health-check')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <div style="max-width: 760px">
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>

    <n-card title="安全体检" size="small" style="margin-bottom: 16px">
      <template #header-extra>
        <n-button size="small" :loading="loading" @click="load">重新检查</n-button>
      </template>

      <div class="row">
        <span class="label">审计哈希链</span>
        <span>
          <n-tag v-if="data" size="small" :type="data.audit_chain_ok ? 'success' : 'error'">
            {{ data.audit_chain_ok ? '完整' : '断链' }}
          </n-tag>
          <span class="v">{{ data?.audit_entries ?? 0 }} 条</span>
        </span>
      </div>
      <div v-if="data?.audit_error" class="err">{{ data.audit_error }}</div>

      <div class="row">
        <span class="label">每日锚点</span>
        <span>
          <n-tag v-if="data" size="small" :type="(data.anchors ?? 0) >= 0 ? 'success' : 'error'">
            {{ (data.anchors ?? 0) >= 0 ? `${data.anchors} 个` : '校验失败' }}
          </n-tag>
          <span class="v">{{ data?.last_anchor_day ?? '—' }}{{ data?.last_anchor_id ? `（到 #${data.last_anchor_id}）` : '' }}</span>
        </span>
      </div>

      <div class="row">
        <span class="label">加密备份</span>
        <span>
          <n-tag v-if="data" size="small" :type="(data.backups ?? 0) > 0 ? 'success' : 'warning'">
            {{ data?.backups ?? 0 }} 份
          </n-tag>
          <span class="v">{{ data?.last_backup_at ? formatTs(data.last_backup_at) : '还没有备份' }} {{ mb(data?.last_backup_size) }}</span>
        </span>
      </div>

      <div class="row">
        <span class="label">数据库大小</span>
        <span class="v">{{ mb(data?.db_size) }}</span>
      </div>

      <div class="row">
        <span class="label">管理端域</span>
        <span class="v">{{ data?.admin_host ?? '—' }}</span>
      </div>
      <div class="row">
        <span class="label">代理端域</span>
        <span class="v">{{ data?.agent_host ?? '—' }}</span>
      </div>
      <div class="row">
        <span class="label">检查时间</span>
        <span class="v">{{ data ? formatTs(data.server_time) : '—' }}</span>
      </div>
    </n-card>

    <n-card title="还能做什么" size="small">
      <ul class="tips">
        <li>备份私钥不在服务器上，丢了就解不开备份——确认离线介质还在。</li>
        <li>管理端 cookie 只属于 admin 子域，代理端拿不到；换 IP 会立即失效。</li>
        <li>敏感操作（改价、建代理、建密钥）需要登录后 5 分钟内的会话，超时会要求重新验证。</li>
      </ul>
    </n-card>
  </div>
</template>

<style scoped>
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid rgba(18, 21, 28, 0.05);
  font-size: 14px;
}
.row:last-child {
  border-bottom: none;
}
.label {
  color: #6b7280;
}
.v {
  color: #4a5568;
  margin-left: 10px;
  font-size: 13px;
}
.err {
  color: #dc2626;
  font-size: 12px;
  padding: 4px 0 10px;
}
.tips {
  margin: 0;
  padding-left: 18px;
  color: #5b6478;
  font-size: 13px;
  line-height: 1.9;
}
</style>
