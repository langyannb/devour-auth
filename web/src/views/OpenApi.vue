<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import {
  NAlert, NButton, NCard, NDataTable, NForm, NFormItem, NInput, NModal, NSelect,
  NSpace, NTabPane, NTabs, NTag, type DataTableColumns,
} from 'naive-ui'
import { api } from '../api'
import { formatTs } from '../utils'

interface Key {
  id: number
  name: string
  scopes: string
  key_id: string
  status: string
  last_used: number | null
  created_at: number
}

const err = ref('')
const notice = ref('')
const keys = ref<Key[]>([])
const loading = ref(false)

const showCreate = ref(false)
const busy = ref(false)
const form = ref({ name: '', scopes: ['cards:read', 'cards:write'] })
const newSecret = ref('')

const scopeOptions = [
  { label: 'cards:read', value: 'cards:read' },
  { label: 'cards:write', value: 'cards:write' },
  { label: 'stats:read', value: 'stats:read' },
]

async function load() {
  loading.value = true
  err.value = ''
  try {
    keys.value = await api.get<Key[]>('/admin/api/open/keys')
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function create() {
  err.value = ''
  newSecret.value = ''
  busy.value = true
  try {
    const d = await api.post<{ sec: string; key_id: string; secret: string }>('/admin/api/open/keys', {
      name: form.value.name,
      scopes: form.value.scopes.join(','),
    })
    newSecret.value = d.secret
    await load()
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function toggle(k: Key) {
  try {
    await api.post(`/admin/api/open/keys/${k.id}/${k.status === 'active' ? 'disable' : 'enable'}`)
    await load()
  } catch (e) {
    err.value = (e as Error).message
  }
}

async function copyText(t: string) {
  await navigator.clipboard.writeText(t)
  notice.value = '已复制'
}

// ---------- 调试助手 ----------
const endpoints = [
  { label: 'POST 发卡', value: 'POST /open/v1/cards/generate', body: '{"policy_id":2,"qty":1,"prefix":"API"}' },
  { label: 'POST 查卡', value: 'POST /open/v1/cards/query', body: '{"kami":"VIP-8888-0001"}' },
  { label: 'POST 续期', value: 'POST /open/v1/cards/extend', body: '{"kami":"VIP-8888-0001","days":30}' },
  { label: 'POST 解绑', value: 'POST /open/v1/cards/unbind', body: '{"kami":"VIP-8888-0001"}' },
  { label: 'POST 加权限', value: 'POST /open/v1/cards/entitlements/grant', body: '{"kami":"VIP-8888-0001","code":"aim_assist"}' },
  { label: 'GET 在线数', value: 'GET /open/v1/stats/online', body: '' },
]

const debugKey = ref<string>('')
const debugEndpoint = ref(endpoints[0].value)
const debugBody = ref(endpoints[0].body)
const debugResult = ref<{ status: number; body: string; request: string } | null>(null)
const debugging = ref(false)

const keyOptions = computed(() => keys.value.filter(k => k.status === 'active').map(k => ({ label: `${k.name}（${k.key_id}）`, value: k.key_id })))

function onEndpoint(v: string) {
  debugEndpoint.value = v
  const ep = endpoints.find(e => e.value === v)
  if (ep) debugBody.value = ep.body
}

async function runDebug() {
  debugResult.value = null
  err.value = ''
  if (!debugKey.value) {
    err.value = '请选择一个密钥'
    return
  }
  const [method, path] = debugEndpoint.value.split(' ')
  debugging.value = true
  try {
    const d = await api.post<{ status: number; body: string; request: string }>('/admin/api/open/debug', {
      key_id: debugKey.value,
      method,
      path,
      body: method === 'GET' ? '' : debugBody.value,
    })
    debugResult.value = d
  } catch (e) {
    err.value = (e as Error).message
  } finally {
    debugging.value = false
  }
}

// ---------- 示例生成 ----------
const langs = [
  { label: 'Python', value: 'python' },
  { label: 'Node.js', value: 'node' },
  { label: 'C#', value: 'csharp' },
  { label: 'C++', value: 'cpp' },
  { label: 'Java', value: 'java' },
  { label: 'PHP', value: 'php' },
  { label: '易语言', value: 'eyy' },
]
const lang = ref('python')

const sample = computed(() => {
  const keyID = debugKey.value || 'ak_你的key_id'
  const secret = '你的secret'
  const [method, path] = debugEndpoint.value.split(' ')
  const body = method === 'GET' ? '' : debugBody.value
  const p = lang.value
  if (p === 'python') {
    return `import hmac, hashlib, time, secrets, json, requests

KEY_ID = "${keyID}"
SECRET = "${secret}"
METHOD = "${method}"
PATH = "${path}"
BODY = '''${body}'''
ts = str(int(time.time()))
nonce = secrets.token_hex(16)
msg = f"{METHOD}\\n{PATH}\\n{ts}\\n{nonce}\\n{BODY}"
sig = hmac.new(SECRET.encode(), msg.encode(), hashlib.sha256).hexdigest()

r = requests.request(METHOD, "https://net.namikao.com" + PATH, data=BODY, headers={
    "Content-Type": "application/json",
    "X-Api-Key": KEY_ID,
    "X-Timestamp": ts,
    "X-Nonce": nonce,
    "X-Signature": sig,
})
print(r.status_code, r.text)`
  }
  if (p === 'node') {
    return `const crypto = require('crypto')
const KEY_ID = '${keyID}'
const SECRET = '${secret}'
const METHOD = '${method}'
const PATH = '${path}'
const BODY = ${body ? `\`${body}\`` : "''"}
const ts = String(Math.floor(Date.now() / 1000))
const nonce = crypto.randomBytes(16).toString('hex')
const sig = crypto.createHmac('sha256', SECRET).update([METHOD, PATH, ts, nonce, BODY].join('\\n')).digest('hex')

const res = await fetch('https://net.namikao.com' + PATH, {
  method: METHOD,
  body: BODY || undefined,
  headers: { 'Content-Type': 'application/json', 'X-Api-Key': KEY_ID, 'X-Timestamp': ts, 'X-Nonce': nonce, 'X-Signature': sig },
})
console.log(res.status, await res.text())`
  }
  if (p === 'csharp') {
    return `using System.Security.Cryptography;
using System.Text;
using var http = new HttpClient();
var keyId = "${keyID}";
var secret = "${secret}";
var method = "${method}";
var path = "${path}";
var body = @"${body.replace(/"/g, '""')}";
var ts = DateTimeOffset.UtcNow.ToUnixTimeSeconds().ToString();
var nonce = Convert.ToHexString(RandomNumberGenerator.GetBytes(16)).ToLower();
var msg = $"{method}\\n{path}\\n{ts}\\n{nonce}\\n{body}";
using var hmac = new HMACSHA256(Encoding.UTF8.GetBytes(secret));
var sig = Convert.ToHexString(hmac.ComputeHash(Encoding.UTF8.GetBytes(msg))).ToLower();

var req = new HttpRequestMessage(new HttpMethod(method), "https://net.namikao.com" + path);
if (body.Length > 0) req.Content = new StringContent(body, Encoding.UTF8, "application/json");
req.Headers.Add("X-Api-Key", keyId);
req.Headers.Add("X-Timestamp", ts);
req.Headers.Add("X-Nonce", nonce);
req.Headers.Add("X-Signature", sig);
var resp = await http.SendAsync(req);
Console.WriteLine((int)resp.StatusCode + " " + await resp.Content.ReadAsStringAsync());`
  }
  if (p === 'cpp') {
    return `// 依赖 OpenSSL：-lssl -lcrypto
// 用 HMAC-SHA256 对 "method\\npath\\nts\\nnonce\\nbody" 签名，再带上 4 个请求头
std::string msg = method + "\\n" + path + "\\n" + ts + "\\n" + nonce + "\\n" + body;
unsigned char* digest = HMAC(EVP_sha256(), secret.data(), secret.size(),
    (unsigned char*)msg.data(), msg.size(), nullptr, nullptr);
// digest 转 hex 后放 X-Signature
// X-Api-Key=${keyID}  X-Timestamp=ts  X-Nonce=nonce`
  }
  if (p === 'java') {
    return `var keyId = "${keyID}";
var secret = "${secret}";
var method = "${method}";
var path = "${path}";
var body = "${body.replace(/"/g, '\\"')}";
var ts = String(System.currentTimeMillis() / 1000);
var nonce = new BigInteger(128, new SecureRandom()).toString(16);
var msg = method + "\\n" + path + "\\n" + ts + "\\n" + nonce + "\\n" + body;
var mac = Mac.getInstance("HmacSHA256");
mac.init(new SecretKeySpec(secret.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
var sig = HexFormat.of().formatHex(mac.doFinal(msg.getBytes(StandardCharsets.UTF_8)));
// HttpRequest.newBuilder(URI.create("https://net.namikao.com" + path))
//   .header("X-Api-Key", keyId).header("X-Timestamp", ts)
//   .header("X-Nonce", nonce).header("X-Signature", sig)`
  }
  if (p === 'php') {
    return `<?php
$keyId = "${keyID}";
$secret = "${secret}";
$method = "${method}";
$path = "${path}";
$body = '${body}';
$ts = (string)time();
$nonce = bin2hex(random_bytes(16));
$sig = hash_hmac('sha256', "$method\\n$path\\n$ts\\n$nonce\\n$body", $secret);

$ch = curl_init("https://net.namikao.com" . $path);
curl_setopt_array($ch, [
    CURLOPT_CUSTOMREQUEST => $method,
    CURLOPT_POSTFIELDS => $body,
    CURLOPT_HTTPHEADER => [
        "Content-Type: application/json",
        "X-Api-Key: $keyId",
        "X-Timestamp: $ts",
        "X-Nonce: $nonce",
        "X-Signature: $sig",
    ],
    CURLOPT_RETURNTRANSFER => true,
]);
echo curl_exec($ch);`
  }
  return `' 易语言：用「取数据摘要」/ 精易模块的 HmacSHA256 对下面字符串签名
' method + #换行符 + path + #换行符 + ts + #换行符 + nonce + #换行符 + body
' 请求头带 X-Api-Key / X-Timestamp / X-Nonce / X-Signature
' key_id   = ${keyID}
' method   = ${method}
' path     = ${path}
' body     = ${body}`
})

const keyCols: DataTableColumns<Key> = [
  { title: '名称', key: 'name', width: 140 },
  { title: 'key_id', key: 'key_id', width: 260 },
  { title: '权限', key: 'scopes' },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render: (r) => h(NTag, { size: 'small', type: r.status === 'active' ? 'success' : 'default' }, { default: () => (r.status === 'active' ? '启用' : '停用') }),
  },
  { title: '最近调用', key: 'last_used', width: 150, render: (r) => (r.last_used ? formatTs(r.last_used) : '—') },
  {
    title: '操作',
    key: 'op',
    width: 90,
    render: (r) => h(NButton, { size: 'tiny', onClick: () => toggle(r) }, { default: () => (r.status === 'active' ? '停用' : '启用') }),
  },
]

onMounted(load)
</script>

<template>
  <div>
    <n-alert v-if="err" type="error" style="margin-bottom: 12px">{{ err }}</n-alert>
    <n-alert v-if="notice" type="success" style="margin-bottom: 12px">{{ notice }}</n-alert>

    <n-tabs type="line" animated>
      <n-tab-pane name="keys" tab="API 密钥">
        <n-space style="margin-bottom: 12px">
          <n-button type="primary" @click="showCreate = true">新建密钥</n-button>
          <n-button @click="load">刷新</n-button>
        </n-space>
        <n-alert v-if="newSecret" type="warning" style="margin-bottom: 12px">
          <div style="font-family: monospace; word-break: break-all">{{ newSecret }}</div>
          <n-space style="margin-top: 8px">
            <n-button size="small" @click="copyText(newSecret)">复制 secret</n-button>
            <span style="font-size: 12px; color: #888">只显示这一次，关闭后无法再取</span>
          </n-space>
        </n-alert>
        <n-data-table :columns="keyCols" :data="keys" :loading="loading" size="small" :bordered="false" :scroll-x="860" />
      </n-tab-pane>

      <n-tab-pane name="debug" tab="调试助手">
        <n-card size="small" style="margin-bottom: 12px">
          <n-form :show-feedback="false">
            <n-space :wrap="true" align="end">
              <n-form-item label="密钥">
                <n-select v-model:value="debugKey" :options="keyOptions" style="width: 280px" placeholder="选择密钥" />
              </n-form-item>
              <n-form-item label="接口">
                <n-select v-model:value="debugEndpoint" :options="endpoints" style="width: 280px" @update:value="onEndpoint" />
              </n-form-item>
              <n-button type="primary" :loading="debugging" @click="runDebug">发送</n-button>
            </n-space>
          </n-form>
        </n-card>

        <n-card v-if="debugEndpoint" size="small" title="请求体" style="margin-bottom: 12px">
          <n-input v-model:value="debugBody" type="textarea" :rows="4" placeholder='{"kami":"..."}' />
        </n-card>

        <n-card v-if="debugResult" size="small" title="响应">
          <n-tag :type="debugResult.status < 300 ? 'success' : 'error'" size="small">HTTP {{ debugResult.status }}</n-tag>
          <pre class="code">{{ debugResult.body }}</pre>
          <div style="font-size: 12px; color: #888; margin: 8px 0 4px">实际发出的请求（签名由后端计算）</div>
          <pre class="code">{{ debugResult.request }}</pre>
        </n-card>
      </n-tab-pane>

      <n-tab-pane name="docs" tab="文档与示例">
        <n-card size="small" title="签名规则" style="margin-bottom: 12px">
          <p style="font-size: 13px; color: #4a5568; line-height: 1.9; margin: 0">
            每个请求带四个头：<code>X-Api-Key</code>、<code>X-Timestamp</code>（Unix 秒，偏差 ≤300）、
            <code>X-Nonce</code>（5 分钟内唯一）、<code>X-Signature</code>。
          </p>
          <pre class="code">X-Signature = hex(hmac_sha256(secret, method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + body))</pre>
          <p style="font-size: 13px; color: #4a5568; margin: 0">path 不含域名；GET 请求 body 为空串。</p>
        </n-card>

        <n-card size="small" title="生成调用代码">
          <n-space style="margin-bottom: 12px" :wrap="true">
            <n-select v-model:value="lang" :options="langs" style="width: 160px" />
            <n-select v-model:value="debugKey" :options="keyOptions" style="width: 260px" placeholder="选择密钥（用于填 key_id）" />
            <n-button size="small" @click="copyText(sample)">复制</n-button>
          </n-space>
          <pre class="code">{{ sample }}</pre>
        </n-card>

        <n-card size="small" title="接口清单" style="margin-top: 12px">
          <ul style="margin: 0; padding-left: 18px; font-size: 13px; line-height: 2; color: #4a5568">
            <li><code>POST /open/v1/cards/generate</code>（cards:write）</li>
            <li><code>POST /open/v1/cards/query</code>（cards:read）</li>
            <li><code>POST /open/v1/cards/extend</code>（cards:write）</li>
            <li><code>POST /open/v1/cards/unbind</code>（cards:write）</li>
            <li><code>POST /open/v1/cards/entitlements/grant</code>（cards:write）</li>
            <li><code>GET /open/v1/stats/online</code>（stats:read）</li>
          </ul>
        </n-card>
      </n-tab-pane>
    </n-tabs>

    <n-modal v-model:show="showCreate" preset="card" title="新建 API 密钥" style="width: min(460px, 94vw)">
      <n-form label-placement="top">
        <n-form-item label="名称">
          <n-input v-model:value="form.name" placeholder="发货系统" />
        </n-form-item>
        <n-form-item label="权限">
          <n-select v-model:value="form.scopes" multiple :options="scopeOptions" />
        </n-form-item>
        <n-button type="primary" :loading="busy" block @click="create">创建</n-button>
      </n-form>
    </n-modal>
  </div>
</template>

<style scoped>
.code {
  background: #f6f7fb;
  border-radius: 6px;
  padding: 12px;
  font-size: 12px;
  line-height: 1.7;
  overflow: auto;
  font-family: ui-monospace, monospace;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
