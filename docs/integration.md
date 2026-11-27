# 客户端与开放接口对接

给接入方（客户端作者、发货系统、代理平台）看的。服务端部署在
`https://net.namikao.com`，管理端 `https://admin.namikao.com`，代理端
`https://agent.namikao.com`。

先分清你要接哪一套：

| 你要做的事 | 用哪套 | 认证方式 |
|---|---|---|
| 你的软件启动时校验卡密、心跳、扣功能点 | **客户端协议** `/v1/*` | 卡密换令牌（Bearer） |
| 你的发货系统批量发卡/查卡/续期/解绑 | **开放 API** `/open/v1/*` | API Key + HMAC 签名 |
| 卖卡、看额度、管下级 | **代理端**（网页，无需对接） | 代理账号登录 |

---

## 0. 管理员：一键开通（推荐）

管理端 → 应用 → **一键开通开发者**，填三样：应用名称、code、管理员用户名（密码留空会自动生成）。

一次建好并返回可复制的开通信息：

- 应用（他的卡密、公告、版本、变量都只在这个应用里）
- 管理员账号（绑定该应用，**不需要动态验证码**）
- 开放接口密钥（`cards:read,cards:write,stats:read`）

密码、恢复码、API secret 只在弹窗里出现一次，点「复制全部」发给对方即可。
对方拿到就能直接登录和对接，不需要自己建任何东西。

> 接口版：`POST /admin/api/tenants`，参数 `{code, name, username, password?, scopes?}`，仅全局管理员可用。

开通之后，对方登录管理端可以在「集成 → 对接信息」里自己查看应用 code、登录地址和
开放接口密钥（secret 可显示/复制），并「一键复制全部」，不需要再来问你。

---

## 0.1 接入前要拿到的四样东西

1. **应用 code**（下文 `APP`）：例如 `friend`。客户端所有请求都要带它，卡密按应用隔离。
2. **管理后台账号**：绑定该应用的管理员，能发卡、看自己的数据。
3. **开放 API Key**（只有你要调发货接口才需要）：`key_id` 与 `secret` 只显示一次，
   `secret` 放环境变量/配置中心，别写进客户端。
4. **域名**：`https://net.namikao.com`（客户端协议与开放 API 同一域名）。

用「一键开通」的话，这四样会在同一个弹窗里一次给到。

---

## 1. 通用约定

- **只走 HTTPS**。客户端里不要内置任何"跳过校验"的开关。
- 请求/响应都是 JSON，`Content-Type: application/json`。
- 成功：`{"ok":true,"data":{...}}`；失败：`{"ok":false,"code":"CARD_EXPIRED","message":"卡已到期"}`。
  **只看 `code` 做分支**，`message` 会改。
- 所有时间都是 Unix 秒。
- 时间偏差：登录、开放 API 的 `ts` 与服务端相差超过 **300 秒**直接拒绝。
  客户端启动时用 `server_time` 校准一次本地偏移。
- `nonce`：至少 16 个字符的随机串（推荐 32 位 hex），5 分钟内不能重复。
- 令牌只放在内存或系统钥匙串里，**不要写日志、不要回传卡密**。

---

## 2. 客户端接入流程

```
启动
 ├─ GET /v1/meta?app=APP           公告/维护/版本/远程变量（免登录）
 ├─ 本地有 refresh_token?
 │    ├─ 是 → POST /v1/auth/refresh（失效就回到登录）
 │    └─ 否 → 让用户输卡密 → POST /v1/auth/login
 ├─ 每 10 分钟 GET /v1/session     心跳：续命、拿最新权限、计时卡扣秒
 ├─ 用户点某个功能 → POST /v1/consume（扣点/扣次）
 └─ 退出/换号 → POST /v1/auth/logout
```

要点：

- `access_token` 只有 **15 分钟**，过期前用 `refresh_token`（**30 天**，每次刷新都换新）换一对新的。
- 旧 `refresh_token` 再次出现会被判定为盗用，**该卡所有会话立刻吊销**（客户端会收到 401 `REVOKED`）。
  所以刷新要串行：拿到新令牌再丢旧的，别并发刷新。
- 服务端 12 分钟没有心跳就把这张卡算离线。
- 免费模式（`/v1/meta` 返回 `mode: "free"`）时服务端不校验卡密，客户端按公告放行即可，不用调登录。

---

## 3. 接口

### 3.1 元信息 `GET /v1/meta?app=APP`

免登录。返回公告、最新版本、维护开关、远程变量、离线票据公钥。

```bash
curl 'https://net.namikao.com/v1/meta?app=friend'
```

```json
{
  "ok": true,
  "data": {
    "app": "friend",
    "server_time": 1795709714,
    "mode": "paid",
    "maintenance": false,
    "notices": [{ "id": 3, "kind": "popup", "title": "维护通知", "body": "今晚 23:00 维护" }],
    "flags": { "maintenance": "0", "card_mode": "paid" },
    "ticket_pubkey": "MCowBQYDK2VwAyEA...",
    "release": { "version": "2.0.1", "info": "修复…", "url": "https://…", "must": false, "rollout_bp": 10000 }
  }
}
```

- `maintenance=true`：客户端提示维护中，不要继续登录。
- `release.must=true`：强制更新。
- 灰度：请求带上 `?fp=设备指纹`，服务端按 `rollout_bp` 决定是否下发这个版本。
- `notices.kind`：`list` 列表展示、`popup` 弹窗、`immediate` 立即打断。

### 3.2 登录 `POST /v1/auth/login`

```json
{
  "app": "friend",
  "kami": "DV-XXXX-XXXX-XXXX",
  "device": {
    "fingerprint": "9f2c…a1",
    "model": "Pixel 7",
    "brand": "google",
    "os": "Android 14",
    "app_version": "2.0.1"
  },
  "ts": 1795709714,
  "nonce": "8f14e45fceea167a5a36dedd4bea2543"
}
```

- `fingerprint` 由客户端生成：同一台设备每次启动必须一致（推荐取硬件序列/ANDROID_ID/主板信息做 SHA-256）。
  换指纹等于换设备，会占设备名额。
- `app` 不传时服务端只认第一个应用的卡；**你接入一定要传自己的 app**。

响应：

```json
{
  "ok": true,
  "data": {
    "access_token": "…", "access_exp": 1795710614,
    "refresh_token": "…", "refresh_exp": 1798301714,
    "card": {
      "kind": "duration", "expire_at": 1798301714,
      "points": 0, "remain_times": 0, "remaining_seconds": 0,
      "paused": false, "device_limit": 2, "bound_devices": 1,
      "heartbeat_sec": 600
    },
    "entitlements": [
      { "code": "basic", "cost_points": 0, "cost_times": 0 },
      { "code": "aim_assist", "cost_points": 2, "cost_times": 0 }
    ],
    "server_time": 1795709714
  }
}
```

- `entitlements` 就是这个用户能用的功能列表；`cost_points`/`cost_times` 是调一次的代价。
- 登录即占用一个设备名额（首次绑定）。超过 `device_limit` 返回 `DEVICE_LIMIT`。
- 限流：**5 次/分/IP + 20 次/时/卡**，超了返回 429 `RATE_LIMITED`。

### 3.3 心跳 `GET /v1/session`

```bash
curl -H 'Authorization: Bearer <access_token>' https://net.namikao.com/v1/session
```

返回结构和登录的 `data` 一样（卡状态、余额、权限、`server_time`）。

- 间隔用登录返回的 `card.heartbeat_sec`（默认 600 秒）；服务端 12 分钟收不到算离线。
- 计时卡（`kind: "timed"`）在这里扣秒，客户端上报的时长服务端不采信。
- 暂停中返回 HTTP 200 + `{"ok":false,"code":"PAUSED","data":{...}}`：客户端要关掉全部功能。
- 会话被吊销/过期返回 401，按 `code` 处理（`REVOKED` 要提示重新登录）。

### 3.4 扣点/扣次 `POST /v1/consume`

```bash
curl -X POST https://net.namikao.com/v1/consume \
  -H 'Authorization: Bearer <access_token>' -H 'Content-Type: application/json' \
  -d '{"entitlement_code":"aim_assist","nonce":"32-hex"}'
```

```json
{ "code": "aim_assist", "balance_after": 498, "cost_points": 2, "cost_times": 0,
  "remaining_points": 498, "remaining_times": 0, "replayed": false, "server_time": 1795709714 }
```

- `nonce` 至少 16 字符，**按卡幂等**：同一个 nonce 重放返回上次结果（`replayed: true`），
  不会重复扣；拿别的卡用同一个 nonce 会返回 `BAD_INPUT`。
- 余额不够返回 `CARD_EXHAUSTED`。
- 权限不在卡上返回 `ENTITLEMENT_MISSING`。

### 3.5 暂停/继续 `POST /v1/pause`（只有计时卡）

```json
{ "paused": true }
```

返回会话视图（同 3.3）。非计时卡调用会被拒绝。

### 3.6 刷新 `POST /v1/auth/refresh`

```json
{ "refresh_token": "…" }
```

返回新的 `access_token` + `refresh_token`（旧的立刻作废）。返回结构同登录 `data`。

### 3.7 登出 `POST /v1/auth/logout`

```bash
curl -X POST -H 'Authorization: Bearer <access_token>' https://net.namikao.com/v1/auth/logout
```

### 3.8 离线票据 `POST /v1/offline-ticket`

需要 Bearer access。服务端用 Ed25519 签一张票，客户端断网时本地验签放行。

```json
{ "ticket": "base64(json)", "signature": "base64(sig)", "exp": 1795796114,
  "public_key": "base64(ed25519 公钥)", "server_time": 1795709714 }
```

票据内容：

```json
{ "card": "DV-...", "device": "9f2c…a1", "exp": 1795796114,
  "entitlements": ["basic","aim_assist"], "points": 480 }
```

校验顺序：`base64` 解出 JSON → 用 `public_key`（或 `/v1/meta` 的 `ticket_pubkey`）验
`signature` → 检查 `exp`、`device` 与当前设备一致。验签用 Ed25519，别自己实现。

---

## 4. 错误码（只看 code）

| code | HTTP | 客户端该做什么 |
|---|---|---|
| `BAD_INPUT` | 400 | 请求写错了（字段缺失、nonce 太短/重复、时间偏差），不要重试 |
| `CARD_NOT_FOUND` | 404 | 卡密不存在，提示核对 |
| `CARD_EXPIRED` | 403 | 已到期，提示续费 |
| `CARD_BANNED` | 403 | 被封禁，提示联系代理 |
| `CARD_EXHAUSTED` | 403 | 次数/点数用完了 |
| `DEVICE_LIMIT` | 403 | 设备数满了，提示去解绑页换绑 |
| `DEVICE_MISMATCH` | 403 | 设备不匹配（或设备被限制） |
| `UNAUTHORIZED` | 401 | 令牌无效/过期，先试 refresh，失败再回登录 |
| `REVOKED` | 401 | 会话被吊销，直接回登录页 |
| `PAUSED` | 200 / 409 | 计时卡暂停中，关掉功能但不登出（`/v1/session` 回 200，其它接口回 409） |
| `ENTITLEMENT_MISSING` | 403 | 这个功能不在这张卡上 |
| `RATE_LIMITED` | 429 | 退避重试（登录 5 次/分/IP、20 次/时/卡） |
| `MAINTENANCE` | 409 | 维护中 |
| `INTERNAL` | 500 | 服务端问题，退避重试并上报 |

---

## 5. 设备、换绑、免费模式

- **设备名单**：登录时指纹不在名单且没满 `device_limit` 就自动绑定；满了返回 `DEVICE_LIMIT`。
- **换绑**：让用户去 `https://unbind.namikao.com/`，输卡密 + 图形验证码，自己勾选设备解绑。
  规则由卡类型决定：免费次数、冷却天数、超额扣时，客户端不用实现，遇到 `DEVICE_LIMIT` 引导过去即可。
- **免费模式**：管理员在「运营 → 卡密与解绑」把模式设成 `free`，`/v1/meta` 下发 `mode:"free"`，
  客户端直接放行，不调登录。
- **IP 验证**打开时，换网段登录只记一条异常事件，**不拦登录**。

---

## 6. 最小客户端（TypeScript，可直接抄）

```ts
const BASE = 'https://net.namikao.com'
const APP = 'friend'

type Resp<T> = { ok: true; data: T } | { ok: false; code: string; message: string }
type Session = {
  access_token: string; access_exp: number
  refresh_token: string; refresh_exp: number
  card: { kind: string; expire_at: number; remaining_seconds: number; points: number; remain_times: number; paused: boolean }
  entitlements: { code: string; cost_points: number; cost_times: number }[]
  server_time: number
}

let session: Session | null = null
let clockSkew = 0

const now = () => Math.floor(Date.now() / 1000) + clockSkew
const nonce = () => crypto.randomUUID().replaceAll('-', '') // 32 位 hex

async function call<T>(path: string, body?: unknown, token?: string): Promise<Resp<T>> {
  const res = await fetch(BASE + path, {
    method: body === undefined ? 'GET' : 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  return res.json()
}

export async function login(kami: string, fingerprint: string) {
  const r = await call<Session>('/v1/auth/login', {
    app: APP, kami,
    device: { fingerprint, model: 'PC', brand: 'custom', os: navigator.platform, app_version: '2.0.1' },
    ts: now(), nonce: nonce(),
  })
  if (!r.ok) throw new Error(r.code)
  session = r.data
  clockSkew = r.data.server_time - Math.floor(Date.now() / 1000)
}

export async function heartbeat() {
  if (!session) return
  if (session.access_exp - now() < 120) await refresh()
  const r = await call<Session>('/v1/session', undefined, session!.access_token)
  if (r.ok) { session = { ...session!, ...r.data }; return }
  if (r.code === 'UNAUTHORIZED') return refresh()
  if (r.code === 'PAUSED') return void onPaused()
  throw new Error(r.code)
}

export async function refresh() {
  if (!session) return
  const r = await call<Session>('/v1/auth/refresh', { refresh_token: session.refresh_token })
  if (!r.ok) { session = null; throw new Error(r.code) }
  session = r.data
}

export async function consume(code: string) {
  const r = await call('/v1/consume', { entitlement_code: code, nonce: nonce() }, session!.access_token)
  if (!r.ok) throw new Error(r.code)
  return r.data
}

function onPaused() { /* 关掉全部功能，不登出 */ }
```

---

## 7. 开放 API（发货系统用）

`/open/v1/*`，同一域名，HMAC 签名。**`secret` 只能放服务端**，绝不能进客户端。

请求头：

```
X-Api-Key:    key_id
X-Timestamp:  1795709714
X-Nonce:      32-hex
X-Signature:  hex(hmac_sha256(secret, method + "\n" + path + "\n" + ts + "\n" + nonce + "\n" + body))
```

- `path` 只算路径（`/open/v1/cards/query`），不含域名和查询串。
- `body` 是原样的请求体；GET 用空字符串。
- 时间偏差 > 300 秒、nonce 5 分钟内重复 → 401。
- 密钥的权限范围（scopes）：`cards:write`、`cards:read`、`stats:read`。

Python 签名示例：

```python
import hashlib, hmac, json, time, secrets, requests

KEY_ID = "ak_xxx"
SECRET = "只放在服务端"

def call(path, body=None, method="POST"):
    body_str = json.dumps(body or {}, separators=(",", ":")) if body is not None else ""
    ts = str(int(time.time()))
    nonce = secrets.token_hex(16)
    payload = f"{method}\n{path}\n{ts}\n{nonce}\n{body_str}"
    sig = hmac.new(SECRET.encode(), payload.encode(), hashlib.sha256).hexdigest()
    r = requests.request(method, "https://net.namikao.com" + path, data=body_str,
        headers={"X-Api-Key": KEY_ID, "X-Timestamp": ts, "X-Nonce": nonce,
                 "X-Signature": sig, "Content-Type": "application/json"})
    return r.json()
```

接口：

| 接口 | scope | 请求 | 说明 |
|---|---|---|---|
| `POST /open/v1/cards/quick` | cards:write | `{kind, multiplier, qty, device_limit, prefix, note}` | **推荐**：按类型发卡，和后台「发卡」页一致，返回 `kamis` |
| `POST /open/v1/cards/generate` | cards:write | `{policy_id, qty, prefix, note}` | 按已有卡类型发卡，高级用法 |
| `POST /open/v1/cards/query` | cards:read | `{kami}` | 查卡状态 |
| `POST /open/v1/cards/extend` | cards:write | `{kami, days, times, points, seconds}` | 续期/加点 |
| `POST /open/v1/cards/unbind` | cards:write | `{kami}` | 解绑全部设备 |
| `POST /open/v1/cards/entitlements/grant` | cards:write | `{kami, code, expires_at}` | 单独授权 |
| `GET /open/v1/stats/online` | stats:read | — | 当前在线数 |

`kind` 取 `day | week | month | forever | custom`；`custom` 配 `value` + `unit`（`minute|hour|day`）。
`multiplier` 是单码倍数（周卡 + 2 = 2 周），`qty` 是张数。

例：

```python
print(call("/open/v1/cards/quick",
           {"kind": "month", "qty": 10, "device_limit": 2, "prefix": "FZ"}))
# {"ok":true,"data":{"kamis":["FZ-..."],"qty":10,"seconds":2592000,...}}
```

开放接口只能操作**密钥所属应用**的卡，跨应用一律当卡不存在。

---

## 8. 联调清单

```bash
# 1) 元信息（免登录）
curl 'https://net.namikao.com/v1/meta?app=friend'

# 2) 登录（换成你自己的卡密和指纹）
curl -X POST https://net.namikao.com/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"app":"friend","kami":"DV-XXXX","device":{"fingerprint":"dev-0001"},"ts":'"$(date +%s)"',"nonce":"'"$(openssl rand -hex 16)"'"}'

# 3) 心跳（把 <access> 换成上一步的 access_token）
curl -H 'Authorization: Bearer <access>' https://net.namikao.com/v1/session

# 4) 扣一次功能
curl -X POST https://net.namikao.com/v1/consume \
  -H 'Authorization: Bearer <access>' -H 'Content-Type: application/json' \
  -d '{"entitlement_code":"basic","nonce":"'"$(openssl rand -hex 16)"'"}'
```

自检：同一个 `nonce` 再发一次，应该返回 `replayed: true` 而不是重复扣；
把 `ts` 改成 10 分钟前，应该返回 `BAD_INPUT`。

---

## 9. 常见问题

**登录一直 `BAD_INPUT`**
- `ts` 与服务器差超过 300 秒；`nonce` 少于 16 位或 5 分钟内重复过；少了 `fingerprint`。

**同一个用户在两台机器上同时登录**
- 每台设备一个指纹、各占一个名额；名额满了第二台会被 `DEVICE_LIMIT` 拦住。
- 并发上限目前固定 1（同一张卡同一时间一个有效会话），新登录会挤掉旧会话。

**刷新后旧令牌还能用吗**
- 不能，刷新即轮换。如果用旧 `refresh_token` 再刷一次，会被当成盗用，整张卡的会话全被吊销。

**`PAUSED` 为什么是 200**
- 它是状态不是失败。看 `ok:false` 的 `code` 分支即可，别用 HTTP 状态码判断。

**要改卡的价格/权限/公告**
- 在管理端「发卡 / 权限包 / 运营」改；新登录立即生效，已登录用户下一次心跳生效。

**服务端判定为准**
- 客户端里的任何校验都只是体验优化，服务端永远会再判一次。别把"通过"写在客户端里。

---

文档与实现不一致时，以 `docs/openapi.yaml` 和实际响应为准；发现对不上，
把请求和响应贴给管理员。
