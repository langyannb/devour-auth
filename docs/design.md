# Devour Auth v2 设计

> 状态：待实现
> 日期：2026-11-27
> 范围：网络验证服务端、管理端、代理端。不含客户端（协议预留）。

## 1. 定位

Devour Auth 是一套自建的软件授权与卡密验证系统：服务端、管理端、代理端全部自研。

- 协议从零设计，不依赖、不兼容任何现有验证平台。
- 全新数据库，不导入任何历史数据。
- 客户端只与 `net.namikao.com` 通信。

服务的三种角色：运营方（管理端）、代理（代理端）、终端用户（卡密 + 查卡页）。

## 2. 目标与非目标

### 目标

- 服务端权威：授权、设备绑定、吊销、额度全部由服务端判定。
- 链路可信：TLS（`net.namikao.com`），令牌为不透明随机串，库里只存哈希。
- 可运营：卡密、设备、在线、代理、账单、权限、版本、公告、对接，一套后台管完。
- 可审计：所有管理操作写入哈希链，改不了、改了能发现。
- 可本地开发：全部功能在本地 SQLite + HTTP 跑通，服务器只负责 TLS 与进程守护。

### 非目标

支付、自动发卡、在线充值、提现、多租户 SaaS、RBAC 多角色矩阵、多语言 SDK、
工单/聊天室/签到/转盘等运营噱头、客户端实现。

### 反目标

- 不把密码学塞进客户端（客户端可被 hook，防线在服务端）。
- 不为了“功能多”堆一套用不上的表。
- 不做在线支付链路的任何对账逻辑。

## 3. 术语

| 术语 | 含义 |
|---|---|
| 应用（app） | 一个受保护的软件产品。当前只有 `devour`，表结构支持多个 |
| 卡类型（policy） | 一套授权规则：计费方式、设备上限、心跳、离线宽限、权限包 |
| 卡密（card） | 发放给终端用户的一串凭据，引用卡类型，可单独覆盖少量参数 |
| 功能权限（entitlement） | 客户端可按 `code` 判断的功能位 |
| 代理（agent） | 二级分销角色，有折扣与月度额度，独立入口 |
| 会话（session） | 登录后签发的 access/refresh 令牌对 |

## 4. 总体架构

```
                ┌──────────────────────────────┐
  浏览器 ──TLS──▶│ nginx 443                    │
                │  net.namikao.com             │
                │  limit_req / HSTS / CSP      │
                └──────────────┬───────────────┘
                               │ 127.0.0.1:6760
                ┌──────────────▼───────────────┐
                │ devour-authd (Go 单二进制)     │
                │  httpapi / auth / card        │
                │  device / session / agent     │
                │  policy / entitlement         │
                │  release / admin / openapi    │
                │  sqlite (WAL)                 │
                └──────────────────────────────┘
```

- 前端 `web/dist` 通过 `embed.FS` 打进二进制，部署只有一个可执行文件。
- 数据库与备份在 `/var/lib/devour/`、`/var/backups/devour/`。
- 所有业务时间以服务端 UTC 为准，接口返回 `server_time`。

## 5. 角色与权限

| 角色 | 入口 | 能力 |
|---|---|---|
| `admin` | `/admin` | 全部 |
| `operator` | `/admin` | 卡密查看/详情/解绑/封禁/续期、设备、在线、审计只读；不能改价、不能管代理、不能碰密钥与系统账号 |
| 代理 `level=1` | `/agent` | 本人及下级卡密、出卡、额度、账单、下级管理 |
| 代理 `level=2` | `/agent` | 本人卡密、出卡、账单；无下级 |
| 终端用户 | `/query` | 凭卡密 + 验证码查到期、绑定数；按策略自助解绑 |

规则：

- `admins` 与 `agents` 是两张独立的表、两套登录、两套会话域。
- 三端各用独立子域：`admin.namikao.com` / `agent.namikao.com` / `net.namikao.com`，
  cookie 按域天然隔离；服务端 `hostGuard` 再按子域限制可达路径。
- `/admin/*` 只认 admin 会话，`/agent/*` 只认 agent 会话，交叉访问一律 `403`。
- 授权检查写在每个 handler，不依赖前端隐藏菜单。
- 代理查询强制 `agent_id IN (自己, 下级)`，在 SQL 层拼条件。

## 6. 数据模型

SQLite，开启 `journal_mode=WAL`、`foreign_keys=ON`、`busy_timeout=5000`。
写连接单开（`SetMaxOpenConns(1)`），读连接多开。

```sql
-- 应用
CREATE TABLE apps (
  id            INTEGER PRIMARY KEY,
  code          TEXT NOT NULL UNIQUE,        -- devour
  name          TEXT NOT NULL,
  active        INTEGER NOT NULL DEFAULT 1,
  created_at    INTEGER NOT NULL
);

-- 管理员
CREATE TABLE admins (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  pass_hash     TEXT NOT NULL,               -- argon2id
  role          TEXT NOT NULL,               -- admin | operator
  totp_secret   TEXT,
  ip_allow      TEXT,                        -- 逗号分隔网段，空=不限
  locked_until  INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL
);

CREATE TABLE admin_sessions (
  id            INTEGER PRIMARY KEY,
  admin_id      INTEGER NOT NULL REFERENCES admins(id),
  token_hash    TEXT NOT NULL UNIQUE,
  csrf_token    TEXT NOT NULL,
  ip            TEXT,
  ua            TEXT,
  created_at    INTEGER NOT NULL,
  last_seen     INTEGER NOT NULL,
  expires_at    INTEGER NOT NULL,
  revoked_at    INTEGER
);

-- 代理
CREATE TABLE agents (
  id                   INTEGER PRIMARY KEY,
  name                 TEXT NOT NULL UNIQUE,
  parent_id            INTEGER REFERENCES agents(id),
  level                INTEGER NOT NULL,        -- 1 | 2
  discount_bp          INTEGER NOT NULL,        -- 折扣，基点：7000 = 7.0 折
  rebate_bp            INTEGER NOT NULL DEFAULT 0, -- 给上级的返点，基点
  monthly_quota_cents  INTEGER NOT NULL DEFAULT 0, -- 0 = 不限
  pass_hash            TEXT NOT NULL,
  totp_secret          TEXT,
  status               TEXT NOT NULL,           -- active | frozen
  created_at           INTEGER NOT NULL,
  frozen_at            INTEGER
);

-- 代理流水：出卡、返点、调整；不做充值
CREATE TABLE agent_ledger (
  id            INTEGER PRIMARY KEY,
  agent_id      INTEGER NOT NULL REFERENCES agents(id),
  kind          TEXT NOT NULL,     -- issue | rebate | adjust
  amount_cents  INTEGER NOT NULL,  -- 正=应付，负=抵扣
  ref           TEXT,              -- 卡密批次 / 账单 / 说明
  created_at    INTEGER NOT NULL
);

CREATE TABLE agent_bills (
  id            INTEGER PRIMARY KEY,
  agent_id      INTEGER NOT NULL REFERENCES agents(id),
  month         TEXT NOT NULL,     -- 2026-11
  amount_cents  INTEGER NOT NULL,
  status        TEXT NOT NULL,     -- open | settled
  settled_at    INTEGER,
  note          TEXT,
  UNIQUE(agent_id, month)
);

-- 卡类型
CREATE TABLE policies (
  id                 INTEGER PRIMARY KEY,
  app_id             INTEGER NOT NULL REFERENCES apps(id),
  code               TEXT NOT NULL,
  name               TEXT NOT NULL,
  kind               TEXT NOT NULL,  -- duration|forever|count|credit|timed|trial
  duration_days      INTEGER NOT NULL DEFAULT 0,
  duration_hours     INTEGER NOT NULL DEFAULT 0,
  duration_seconds   INTEGER NOT NULL DEFAULT 0,
  initial_times      INTEGER NOT NULL DEFAULT 0,
  initial_points     INTEGER NOT NULL DEFAULT 0,
  device_limit       INTEGER NOT NULL DEFAULT 1,
  concurrent_limit   INTEGER NOT NULL DEFAULT 1,
  heartbeat_sec      INTEGER NOT NULL DEFAULT 600,
  offline_hours      INTEGER NOT NULL DEFAULT 24,
  offline_starts     INTEGER NOT NULL DEFAULT 20,
  rebind_free        INTEGER NOT NULL DEFAULT 1,
  rebind_cooldown_days INTEGER NOT NULL DEFAULT 30,
  rebind_cost_hours  INTEGER NOT NULL DEFAULT 0,
  renewable          INTEGER NOT NULL DEFAULT 1,
  trial_per_device   INTEGER NOT NULL DEFAULT 1,
  price_cents        INTEGER NOT NULL DEFAULT 0,
  active             INTEGER NOT NULL DEFAULT 1,
  created_at         INTEGER NOT NULL,
  UNIQUE(app_id, code)
);

-- 功能权限
CREATE TABLE entitlements (
  id           INTEGER PRIMARY KEY,
  app_id       INTEGER NOT NULL REFERENCES apps(id),
  code         TEXT NOT NULL,
  name         TEXT NOT NULL,
  cost_points  INTEGER NOT NULL DEFAULT 0,
  cost_times   INTEGER NOT NULL DEFAULT 0,
  active       INTEGER NOT NULL DEFAULT 1,
  created_at   INTEGER NOT NULL,
  UNIQUE(app_id, code)
);

CREATE TABLE policy_entitlements (
  policy_id       INTEGER NOT NULL REFERENCES policies(id),
  entitlement_id  INTEGER NOT NULL REFERENCES entitlements(id),
  PRIMARY KEY (policy_id, entitlement_id)
);

-- 卡密
CREATE TABLE cards (
  id                INTEGER PRIMARY KEY,
  app_id            INTEGER NOT NULL REFERENCES apps(id),
  kami              TEXT NOT NULL UNIQUE,
  policy_id         INTEGER NOT NULL REFERENCES policies(id),
  kind              TEXT NOT NULL,
  status            TEXT NOT NULL,  -- unused|active|expired|exhausted|banned|recycled
  batch_id          INTEGER REFERENCES batches(id),
  agent_id          INTEGER REFERENCES agents(id),
  device_limit      INTEGER NOT NULL,
  expire_at         INTEGER NOT NULL DEFAULT 0,  -- 0=永久
  remain_times      INTEGER NOT NULL DEFAULT 0,
  points            INTEGER NOT NULL DEFAULT 0,
  remaining_seconds INTEGER NOT NULL DEFAULT 0,
  paused            INTEGER NOT NULL DEFAULT 0,
  rebind_used       INTEGER NOT NULL DEFAULT 0,
  snapshot_json     TEXT NOT NULL,                -- 生成时的策略快照
  note              TEXT,
  activated_at      INTEGER,
  banned_at         INTEGER,
  created_at        INTEGER NOT NULL
);

CREATE TABLE batches (
  id          INTEGER PRIMARY KEY,
  app_id      INTEGER NOT NULL,
  policy_id   INTEGER NOT NULL,
  agent_id    INTEGER,
  qty         INTEGER NOT NULL,
  created_by  INTEGER NOT NULL,
  created_at  INTEGER NOT NULL
);

CREATE TABLE card_entitlements (
  card_id         INTEGER NOT NULL REFERENCES cards(id),
  entitlement_id  INTEGER NOT NULL REFERENCES entitlements(id),
  expires_at      INTEGER NOT NULL DEFAULT 0,
  granted_by      INTEGER NOT NULL,
  revoked_at      INTEGER,
  PRIMARY KEY (card_id, entitlement_id)
);

-- 设备与会话
CREATE TABLE devices (
  id           INTEGER PRIMARY KEY,
  card_id      INTEGER NOT NULL REFERENCES cards(id),
  fingerprint  TEXT NOT NULL,
  model        TEXT,
  brand        TEXT,
  os           TEXT,
  app_version  TEXT,
  first_seen   INTEGER NOT NULL,
  last_seen    INTEGER NOT NULL,
  banned       INTEGER NOT NULL DEFAULT 0,
  unbound_at   INTEGER,
  UNIQUE(card_id, fingerprint)
);

CREATE TABLE sessions (
  id             INTEGER PRIMARY KEY,
  card_id        INTEGER NOT NULL REFERENCES cards(id),
  device_id      INTEGER NOT NULL REFERENCES devices(id),
  access_hash    TEXT NOT NULL UNIQUE,
  refresh_hash   TEXT NOT NULL UNIQUE,
  access_exp     INTEGER NOT NULL,
  refresh_exp    INTEGER NOT NULL,
  last_heartbeat INTEGER NOT NULL,
  revoked_at     INTEGER,
  reuse_detected INTEGER NOT NULL DEFAULT 0,
  ip             TEXT,
  ua             TEXT,
  created_at     INTEGER NOT NULL
);

CREATE TABLE entitlement_usage (
  id             INTEGER PRIMARY KEY,
  card_id        INTEGER NOT NULL REFERENCES cards(id),
  code           TEXT NOT NULL,
  cost_points    INTEGER NOT NULL DEFAULT 0,
  cost_times     INTEGER NOT NULL DEFAULT 0,
  balance_after  INTEGER NOT NULL,
  nonce          TEXT NOT NULL UNIQUE,
  created_at     INTEGER NOT NULL
);

CREATE TABLE trial_claims (
  app_id       INTEGER NOT NULL,
  fingerprint  TEXT NOT NULL,
  card_id      INTEGER NOT NULL,
  claimed_at   INTEGER NOT NULL,
  PRIMARY KEY (app_id, fingerprint)
);

-- 黑名单
CREATE TABLE blacklist (
  id         INTEGER PRIMARY KEY,
  app_id     INTEGER NOT NULL DEFAULT 0,  -- 0 = 全局
  kind       TEXT NOT NULL,               -- ip | device | card
  value      TEXT NOT NULL,
  reason     TEXT,
  expires_at INTEGER NOT NULL DEFAULT 0,
  created_by INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  UNIQUE(app_id, kind, value)
);

-- 运营
CREATE TABLE releases (
  id           INTEGER PRIMARY KEY,
  app_id       INTEGER NOT NULL,
  version      TEXT NOT NULL,
  info         TEXT,
  url          TEXT,
  must         INTEGER NOT NULL DEFAULT 0,
  rollout_bp   INTEGER NOT NULL DEFAULT 10000,
  active       INTEGER NOT NULL DEFAULT 1,
  created_at   INTEGER NOT NULL
);

CREATE TABLE notices (
  id         INTEGER PRIMARY KEY,
  app_id     INTEGER NOT NULL,
  kind       TEXT NOT NULL,   -- list | popup | immediate
  title      TEXT,
  body       TEXT NOT NULL,
  active     INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);

CREATE TABLE flags (
  app_id     INTEGER NOT NULL,
  key        TEXT NOT NULL,
  value      TEXT NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (app_id, key)
);

-- 开放对接
CREATE TABLE api_keys (
  id          INTEGER PRIMARY KEY,
  name        TEXT NOT NULL,
  scopes      TEXT NOT NULL,       -- cards:write,cards:read,...
  key_id      TEXT NOT NULL UNIQUE,
  secret_hash TEXT NOT NULL,       -- 只存哈希，明文只在创建时显示一次
  last_used   INTEGER,
  status      TEXT NOT NULL,       -- active | disabled
  created_at  INTEGER NOT NULL
);

-- 审计
CREATE TABLE events (
  id         INTEGER PRIMARY KEY,
  ts         INTEGER NOT NULL,
  kind       TEXT NOT NULL,
  card_id    INTEGER,
  device_id  INTEGER,
  agent_id   INTEGER,
  ip         TEXT,
  ua         TEXT,
  code       TEXT,
  detail     TEXT
);

CREATE TABLE audit_log (
  id         INTEGER PRIMARY KEY,
  ts         INTEGER NOT NULL,
  actor      TEXT NOT NULL,      -- admin:1 / agent:3 / open:key1
  action     TEXT NOT NULL,
  target     TEXT,
  before     TEXT,
  after      TEXT,
  ip         TEXT,
  prev_hash  TEXT NOT NULL,
  hash       TEXT NOT NULL
);

CREATE TABLE audit_anchor (
  day        TEXT PRIMARY KEY,
  last_id    INTEGER NOT NULL,
  last_hash  TEXT NOT NULL,
  signature  TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE daily_stats (
  day           TEXT PRIMARY KEY,
  activations   INTEGER NOT NULL DEFAULT 0,
  logins        INTEGER NOT NULL DEFAULT 0,
  unique_devices INTEGER NOT NULL DEFAULT 0,
  errors        INTEGER NOT NULL DEFAULT 0,
  online_peak   INTEGER NOT NULL DEFAULT 0,
  issued_cents  INTEGER NOT NULL DEFAULT 0
);
```

`audit_log` 建触发器，禁止 `UPDATE` 与 `DELETE`：

```sql
CREATE TRIGGER audit_no_update BEFORE UPDATE ON audit_log
BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;
CREATE TRIGGER audit_no_delete BEFORE DELETE ON audit_log
BEGIN SELECT RAISE(ABORT, 'audit_log is append-only'); END;
```

## 7. 卡类型

卡类型（`policies.kind`）决定余额维度与结算方式。

| kind | 余额字段 | 到期 | 消耗 | 失效 |
|---|---|---|---|---|
| `duration` | `expire_at` | 激活时起算 | 无 | 时间到 |
| `forever` | `expire_at=0` | 无 | 无 | 封禁/回收 |
| `count` | `remain_times` | 可选并存 | 新会话登录 -1 `cost_times` | 次数尽或到期 |
| `credit` | `points` | 可选并存 | 按权限扣点 | 点数尽或到期 |
| `timed` | `remaining_seconds` | 无固定到期 | 心跳按间隔累加扣秒 | 秒数归零 |
| `trial` | 短时 `expire_at` | 领取即起算 | 无 | 时间到或该设备已领 |

规则：

1. **生成快照**：出卡时把 policy 的时长、设备上限、权限集写入 `snapshot_json`。
   之后修改 policy 只影响新卡，老卡按快照执行。
2. **次数卡**：在新会话登录时扣 `cost_times`（默认 1）；同一设备 5 分钟内重连不重复扣。
3. **点数卡**：客户端只上报用了哪个 `entitlement_code`，服务端按权限定价扣点，
   事务内 `UPDATE ... WHERE points >= cost`，不允许扣成负数。
4. **计时卡**：服务端累加 `min(now - last_heartbeat, heartbeat_sec * 2)`，
   不采信客户端上报的时长。暂停（`paused=1`）时停止累加，且心跳返回 `PAUSED`，
   客户端必须关闭全部功能；否则等于无限卡。
5. **试用卡**：`trial_claims` 以 `(app_id, fingerprint)` 唯一，一台设备一个应用只能领一次；
   不可续费、不可叠加。
6. **状态**：`unused → active → expired | exhausted`，任一态可进 `banned`、`recycled`。

## 8. 功能权限

生效权限 = `卡类型权限 ∪ 单卡附加 − 单卡撤销`，同 code 时撤销优先。

- 单卡附加可带独立到期（`card_entitlements.expires_at`）。
- 权限定义停用后立即停止下发，历史记录保留。
- 权限的 `cost_points` / `cost_times` 为 0 表示免费，客户端不需要调用 consume。
- 客户端遇到未知 code 忽略，不阻断登录。
- 心跳与登录都下发完整权限列表，权限变更在下一次心跳生效。

## 9. 卡密

- 格式：可配前缀 + 12 位随机（`DV-9F2K-7Q8M-X41B`），不使用顺序号。
- 生成：单张、批量（单批 ≤ 500）、CSV 导入；导出按批次。
- 操作：续期、改设备上限、解绑、清空绑定、重置换绑次数、封禁/解封、回收（软删）、
  调整点数/次数/剩余时长、加/撤销权限。全部写审计（含前后值）。
- 换绑：策略定义免费次数、冷却天数、超出后扣时（`rebind_cost_hours`）。
- 备注：自由文本，用于客服记录，不进日志。

## 10. 设备与会话

- 设备指纹由客户端计算：`SHA256(ANDROID_ID + brand + model + 安装盐)` 前 32 hex。
  服务端只比对，不尝试从网络侧识别。
- 绑定：登录时若指纹不在该卡设备列表，且未超 `device_limit`，自动绑定；
  已满则返回 `DEVICE_LIMIT`。
- 并发：同一卡活跃会话数不得超过 `concurrent_limit`，超出时挤掉最旧会话。
- 心跳：默认 10 分钟；连续 3 次失败客户端标离线，服务端 12 分钟无心跳视为离线。
- 吊销：任意时刻可吊销单个会话或整张卡；`REVOKED` 客户端必须立即登出。

### 10.1 卡类型收敛（2026-11-27 起）

- 只发放时长卡（天/周/月/自定义）与永久卡；次数卡、点数卡、计时卡、试用卡不再新增，
  历史卡和策略保留（外键与售后需要），策略置为停用（迁移 011）。
- 代理模板只有天/周/月/永久；「自定义时间」只由管理员发。

### 10.2 自助解绑

- 独立域名 `unbind.zhenx.xyz`（`DEVOUR_UNBIND_HOST`），只挂解绑页与 `/v1/query*`。
- 流程：卡密 + 验证码 → 查状态与已绑设备（指纹脱敏）→ 勾选设备解绑。
  查卡返回 5 分钟有效的 `unbind_ticket`，解绑时校验，不必再输一次验证码。
- 策略由管理员在设置页配置（应用开关 `flags`）：
  `unbind_free` 免费次数、`unbind_cooldown_days` 冷却天数、
  `unbind_mode`（reject=超出拒绝 / charge=扣时）、`unbind_cost_hours` 每次扣的小时数。
  未配置时回退到卡快照里的旧字段；扣时只对时长卡生效，永久卡超出即拒绝。
- 每次解绑动作计一次（勾选多台设备只扣一次/一次计数）。

### 10.3 验证开关

- `verify_device=1`（默认）：限制设备数量；关掉后不校验上限。
- `verify_ip=1`（默认关）：换 /24 网段登录时写一条 `ip_anomaly` 事件，只记录不拦登录。

## 11. 代理体系

- 两级：`parent_id` 自关联，`level=1` 或 `2`，最多两级。
- 价格模板：管理员建模板（天卡/周卡/月卡/永久卡/自定义时间各一个固定价），代理绑定模板后
  按模板价出卡；模板里没设置的类型，代理看不到也发不了。
  出卡应付 = 模板单价 × 份数 × 数量；永久卡不受份数影响。
  代理不能发「自定义时间」，模板里也不配这一项，只有管理员能发任意时长。
- 权限：每个代理单独配置「能否自己设设备数量」「能否自定义卡密」「单批最多张数」。
- 额度：`quota_cents` 是总余额，出卡按应付扣减，扣完为止（-1 = 不限，0 = 不能出卡）。
- 返点：下级每次出卡，按上级 `rebate_bp` 生成一条 `agent_ledger`（负数，抵扣）。
- 账单：每月 1 日生成上月 `agent_bills`；管理员线下收款后标记 `settled`。
- **无在线支付、无提现**；额度只由管理员手动加/扣（写 `topup` / `adjust` 流水）。
- 代理端只能看自己与下级，不能看平台成本价或其他代理。
- 代理不可修改自身 `discount_bp`、`quota_cents`、`parent_id`、`status`。

## 12. 认证协议

所有接口 JSON，HTTPS。统一错误：

```json
{ "ok": false, "code": "CARD_EXPIRED", "message": "卡密已到期", "server_time": 1795709714 }
```

`code` 枚举：`BAD_INPUT / CARD_NOT_FOUND / CARD_EXPIRED / CARD_BANNED / CARD_EXHAUSTED /
DEVICE_LIMIT / DEVICE_MISMATCH / UNAUTHORIZED / REVOKED / PAUSED / ENTITLEMENT_MISSING /
RATE_LIMITED / MAINTENANCE / INTERNAL`。

### 登录

`POST /v1/auth/login`

```json
{
  "kami": "DV-...",
  "device": { "fingerprint": "...", "model": "...", "brand": "...", "os": "...", "app_version": "2.0.0" },
  "ts": 1795709714,
  "nonce": "32-hex"
}
```

响应：

```json
{
  "ok": true,
  "data": {
    "access_token": "...", "access_exp": 1795710614,
    "refresh_token": "...", "refresh_exp": 1798301714,
    "card": { "kind": "credit", "expire_at": 0, "points": 500, "remain_times": 0, "remaining_seconds": 0, "paused": false },
    "entitlements": [{ "code": "basic", "cost_points": 0 }, { "code": "aim_assist", "cost_points": 2 }],
    "server_time": 1795709714
  }
}
```

- 令牌 32 字节随机，库里只存 SHA-256。
- access 15 分钟，refresh 30 天；refresh 每次使用都轮换。
- 旧 refresh 再次出现 → 判定盗用，吊销该卡全部会话并记事件。
- 登录请求 `ts` 与服务端偏差 > 300 秒拒绝；`nonce` 5 分钟内去重。

### 刷新 / 登出 / 心跳

- `POST /v1/auth/refresh { refresh_token }`
- `POST /v1/auth/logout`（Bearer access）
- `GET /v1/session`（Bearer access）：返回卡状态、余额、权限、`server_time`；
  计时卡在此扣秒；`paused` 时返回 `PAUSED`。

### 消耗

`POST /v1/consume`

```json
{ "entitlement_code": "aim_assist", "nonce": "32-hex" }
```

服务端按权限定价扣点/扣次，返回 `balance_after`。nonce 全局唯一，重复请求返回上次结果。

### 暂停

`POST /v1/pause { "paused": true }`，仅 `timed` 卡有效。返回 `paused` 与剩余秒数。

### 元信息

`GET /v1/meta`：公告、最新版本、强更标记、远程变量、维护开关。免登录。

### 离线票据

服务端用 Ed25519 签发：

```json
{ "card": "DV-...", "device": "...", "exp": 1795796114,
  "entitlements": ["basic","macro_pack"], "points": 480 }
```

客户端断网时本地验签，有效期不超过 `offline_hours`。**本期只做签发**，客户端后续接入。

## 13. 开放 API

`/open/v1/*`，供代理发货系统或第三方对接。

请求头：

```
X-Api-Key: key_id
X-Timestamp: 1795709714
X-Nonce: 32-hex
X-Signature: hex(hmac_sha256(secret, method + "\n" + path + "\n" + ts + "\n" + nonce + "\n" + body))
```

- 时间偏差 > 300 秒拒绝；nonce 5 分钟去重。
- `secret` 明文只在创建密钥时返回一次；HMAC 验签需要原文，因此库内保存原文
  （对称密钥无法只存哈希，design 此条已按实现修正）。

| 方法 | 路径 | 权限 |
|---|---|---|
| POST | `/open/v1/cards/generate` | `cards:write` |
| POST | `/open/v1/cards/query` | `cards:read` |
| POST | `/open/v1/cards/extend` | `cards:write` |
| POST | `/open/v1/cards/unbind` | `cards:write` |
| POST | `/open/v1/cards/entitlements/grant` | `cards:write` |
| GET | `/open/v1/stats/online` | `stats:read` |

OpenAPI 3 文档由 `docs/openapi.yaml` 维护，后台 `/open/docs` 内嵌展示。

## 14. 管理端

Vue 3 + TypeScript + Vite + Naive UI + Pinia + ECharts，构建产物 embed 进 Go 二进制。

路由：

| 模块 | 路由 |
|---|---|
| 概览 | `/dashboard` |
| 卡密 | `/cards`、`/cards/:id`、`/cards/batches`、`/cards/types`、`/cards/types/:id`、`/cards/issue` |
| 权限 | `/entitlements` |
| 设备 | `/devices`、`/devices/:id`、`/blacklist` |
| 在线 | `/sessions` |
| 代理 | `/agents`、`/agents/:id`、`/agents/quota`、`/agents/billing` |
| 财务 | `/finance/ledger`、`/finance/settlements` |
| 运营 | `/client/releases`、`/client/notices`、`/client/flags` |
| 对接 | `/open/keys`、`/open/docs`、`/open/webhooks`、`/open/logs` |
| 系统 | `/system/accounts`、`/system/audit`、`/system/security`、`/system/backup` |

交互约定：

- 列表页统一：筛选区 + 批量操作条 + 表格 + 分页。
- 详情页独立路由，不塞抽屉。
- 敏感操作二次验证（密码 + TOTP），服务端校验 5 分钟内有效的 reauth token。
- 所有金额以「分」存储与传输，界面格式化为元。

## 15. 代理端

`/agent` 独立入口、独立登录、独立布局与配色。路由：

`/agent/dashboard`、`/agent/cards`、`/agent/cards/:id`、`/agent/issue`、
`/agent/subagents`、`/agent/billing`、`/agent/settings`。

## 16. 安全

### 16.1 账号

- 密码 argon2id，最少 12 位；admin 与 operator 强制 TOTP。
- 10 个一次性恢复码，只在创建时显示。
- 失败 5 次锁 15 分钟，按账号 + IP 双维度；新 IP 登录额外要求 TOTP。
- 会话空闲 30 分钟、绝对 12 小时；新登录踢旧会话。
- Cookie：`__Host-` 前缀、`HttpOnly`、`Secure`、`SameSite=Strict`，配 CSRF 双提交。

### 16.2 授权

- 两张独立的账号表与两套会话域，交叉访问 403。
- 代理敏感字段只读；下级折扣不得低于上级成本。
- 行级过滤在 SQL 层完成。

### 16.3 审计

- `audit_log` append-only（触发器）。
- 每条 `hash = SHA256(prev_hash || ts || actor || action || target || before || after || ip)`。
- 每日 00:05 对当日链尾签名，写入 `audit_anchor`（Ed25519 私钥单独存放）。
- 后台「审计校验」重算整链，断链告警；同时双写 journald。
- 敏感操作（改价、改额度、加/冻代理、批量回收、轮换密钥、改策略）必须 reauth。

### 16.4 限流与防护

| 项 | 默认 |
|---|---|
| 登录限流 | 5 次/分/IP + 20 次/时/卡密，渐进延迟 |
| 同卡并发 | 1 |
| IP 突变 | 标记事件，不自动封 |
| 查卡验证码 | 4 位数字，2 分钟，一次失效；发码 30 次/分/IP |
| 防重放 | 5 分钟 nonce 去重 |
| refresh 轮换 | 开，重用即全量吊销 |
| 离线宽限 | 24 小时且 ≤ 20 次启动 |
| 后台 TOTP | 开 |
| 管理 IP 白名单 | 默认开（配置后生效） |

### 16.5 网络

- TLS 1.2+，HSTS，`X-Frame-Options: DENY`，CSP。
- 管理端与代理端分属不同子域，cookie 不跨域；同一份二进制按 Host 路由到不同前端。
- 6760 只监听 `127.0.0.1`。

### 16.6 边界

客户端运行在用户设备上，root/Magisk 用户可以 hook 掉指纹和全部本地判断。
本设计的防线是「提高成本 + 服务端可吊销」，不承诺客户端不可破解。
服务器 root 被拿下时审计链也可被伪造，只能靠异地锚定与备份降低损失。

## 17. 部署与运维

见 `docs/runbook.md`。要点：

- 域名 `net.namikao.com` → `43.248.187.94`，certbot 签证书。
- systemd 守护，二进制 + SQLite。
- 每小时 `.backup` + GPG，保留 14 天。
- 健康检查 `GET /healthz`。
- 部署由本地脚本执行，CI 只做校验，不做 CD。

## 18. 迭代计划

| 迭代 | 内容 | 人日 |
|---|---|---|
| I1 | 多应用、卡类型、卡密 CRUD、设备绑定、审计链、限流 | 7 |
| I2 | 代理（额度制）、账单、代理端 | 4.5 |
| I3 | 客户端认证、entitlements、心跳、离线票据、开放 API | 5 |
| I4 | 运营端：版本/公告/远程变量/查卡页/通知/安全体检/统计 | 5 |
| I5 | 防护演练、压测、部署、文档 | 4 |
| 合计 | | ~25.5（含卡类型与权限的 4 天已分摊） |

## 19. 验收

1. 六种卡类型的到期、耗尽、续期边界。
2. 10 个并发 consume 扣 500 点，最终不为负。
3. 同一 nonce 重放被拒。
4. 单卡「附加 + 撤销」同一权限，撤销生效。
5. 试用卡第二台设备领取被拒。
6. 计时卡暂停后心跳不扣秒且返回 `PAUSED`。
7. 修改卡类型后已生成卡行为不变。
8. 次数卡同设备 5 分钟内重连不重复扣。
9. operator 会话调 admin 接口 → 403。
10. 代理 token 调 `/admin/*` → 403。
11. 代理 PATCH 自己的 discount → 403。
12. 直接 UPDATE/DELETE audit_log → 触发器拒绝。
13. 旧 refresh 复用 → 全量吊销 + 告警。
14. 伪造 admin cookie 换 IP → 会话失效。
15. 未 reauth 的改价请求 → 403。

## 20. 明确不做

支付与自动发卡、充值、提现、工单、聊天室、签到、转盘、邀请码、多语言 SDK、
独立数据大屏、计时卡的离线暂停、权限阶梯计价、多租户 SaaS。

## 21. 待定

- 代理额度默认值。
