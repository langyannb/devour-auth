<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCard, NSpace, NTag } from 'naive-ui'
import { api } from '../api'
import { copyText, formatTs } from '../utils'

interface Key {
  id: number
  name: string
  key_id: string
  secret: string
  scopes: string
  status: string
  created_at: number
}
interface Info {
  app: { id: number; code: string; name: string }
  admin: { username: string }
  api_keys: Key[]
}

const data = ref<Info | null>(null)
const err = ref('')
const notice = ref('')
const busy = ref(false)
const shown = ref<Record<number, boolean>>({})

const loginURL = location.hostname.startsWith('admin.')
  ? 'https://admin.namikao.com/login'
  : `${location.origin}/login`

async function load() {
  err.value = ''
  try {
    data.value = await api.get<Info>('/admin/api/integration')
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function copy(text: string, msg: string) {
  if (await copyText(text)) {
    notice.value = msg
    setTimeout(() => { notice.value = '' }, 2500)
  } else {
    err.value = '复制失败，请手动选中'
  }
}

async function newKey() {
  if (!data.value) return
  busy.value = true
  err.value = ''
  try {
    await api.post('/admin/api/open/keys', { name: '对接密钥', scopes: 'cards:read,cards:write,stats:read' })
    notice.value = '已新建密钥，下面的 secret 可以直接复制'
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

// 一次复制全部：发给自己的对接人/贴进项目文档
const allText = computed(() => {
  const d = data.value
  if (!d) return ''
  const keys = d.api_keys.filter((k) => k.status === 'active')
  const lines = [
    '验证服务对接信息',
    '',
    `管理端：${loginURL}`,
    `用户名：${d.admin.username}`,
    '',
    `应用 code：${d.app.code}`,
    `客户端登录请求要带 "app": "${d.app.code}"`,
    '',
  ]
  if (keys.length === 0) {
    lines.push('开放接口：还没有密钥，在这个页面点「新建密钥」')
  } else {
    lines.push('开放接口：')
    for (const k of keys) {
      lines.push(`X-Api-Key: ${k.key_id}`)
      lines.push(`secret: ${k.secret}`)
      lines.push(`权限: ${k.scopes}`)
    }
  }
  lines.push('', '签名规则与客户端协议见 docs/integration.md（第 3 节、第 7 节）')
  return lines.join('\n')
})

onMounted(load)
</script>

<template>
  <div class="page">
    <n-alert v-if="err" type="error" closable style="margin-bottom: 12px" @close="err = ''">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" closable style="margin-bottom: 12px" @close="notice = ''">{{ notice }}</n-alert>

    <template v-if="data">
      <n-card size="small" style="margin-bottom: 14px">
        <div class="row">
          <div>
            <div class="label">应用</div>
            <div class="big">{{ data.app.name }} <span class="code">{{ data.app.code }}</span></div>
          </div>
          <n-button size="small" @click="copy(data.app.code, '应用 code 已复制')">复制 code</n-button>
        </div>
        <p class="hint">客户端所有请求都要带 <code>"app": "{{ data.app.code }}"</code>，卡密按应用隔离。</p>
      </n-card>

      <n-card size="small" style="margin-bottom: 14px">
        <div class="row">
          <div>
            <div class="label">管理端</div>
            <div class="big">{{ data.admin.username }}</div>
          </div>
          <n-button size="small" @click="copy(`${loginURL}\n${data.admin.username}`, '登录地址和用户名已复制')">
            复制登录信息
          </n-button>
        </div>
        <p class="hint">{{ loginURL }} · 忘记密码让主管理员在「账号」里重置。</p>
      </n-card>

      <n-card size="small" style="margin-bottom: 14px">
        <template #header><span class="card-title">开放接口密钥</span></template>
        <template #header-extra>
          <n-button size="tiny" type="primary" :loading="busy" @click="newKey">新建密钥</n-button>
        </template>
        <div v-if="data.api_keys.length === 0" class="hint">还没有密钥。要自动发货就新建一个。</div>
        <div v-for="k in data.api_keys" :key="k.id" class="key">
          <div class="key-top">
            <b>{{ k.name }}</b>
            <n-tag size="tiny" :type="k.status === 'active' ? 'success' : 'default'" :bordered="false">
              {{ k.status === 'active' ? '启用' : '停用' }}
            </n-tag>
            <span class="time">{{ formatTs(k.created_at) }}</span>
          </div>
          <div class="line">
            <span class="k">X-Api-Key</span>
            <code>{{ k.key_id }}</code>
            <n-button size="tiny" text @click="copy(k.key_id, 'key_id 已复制')">复制</n-button>
          </div>
          <div class="line">
            <span class="k">secret</span>
            <code>{{ shown[k.id] ? k.secret : '••••••••••••••••' }}</code>
            <n-button size="tiny" text @click="shown[k.id] = !shown[k.id]">{{ shown[k.id] ? '隐藏' : '显示' }}</n-button>
            <n-button size="tiny" text @click="copy(k.secret, 'secret 已复制')">复制</n-button>
          </div>
          <div class="line"><span class="k">权限</span><span class="scopes">{{ k.scopes }}</span></div>
        </div>
      </n-card>

      <n-card size="small">
        <template #header><span class="card-title">一键复制全部</span></template>
        <p class="hint" style="margin-top: 0">
          生成一份包含应用 code、登录信息、密钥的文本，贴进项目文档或发给同事。
        </p>
        <n-space>
          <n-button type="primary" size="small" @click="copy(allText, '对接信息已复制')">复制全部</n-button>
        </n-space>
      </n-card>
    </template>
    <div v-else class="hint">加载中…</div>
  </div>
</template>

<style scoped>
.page { max-width: 720px; }
.row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.label { font-size: 12px; color: #8b93a7; margin-bottom: 4px; }
.big { font-size: 20px; font-weight: 700; letter-spacing: -0.02em; }
.code { font-family: ui-monospace, monospace; font-size: 14px; color: #148c4f; margin-left: 6px; }
.hint { margin: 8px 0 0; font-size: 12px; color: #8b93a7; line-height: 1.7; }
.card-title { font-size: 14px; font-weight: 600; }
.key { padding: 12px 0; border-bottom: 1px solid #f1f4f3; }
.key:last-child { border-bottom: none; }
.key-top { display: flex; align-items: center; gap: 8px; margin-bottom: 6px; }
.time { margin-left: auto; font-size: 12px; color: #98a2b3; }
.line { display: flex; align-items: center; gap: 8px; font-size: 13px; padding: 2px 0; }
.k { width: 72px; color: #8b93a7; font-size: 12px; flex: 0 0 auto; }
code { font-family: ui-monospace, monospace; word-break: break-all; }
.scopes { font-size: 12px; color: #475467; }
</style>
