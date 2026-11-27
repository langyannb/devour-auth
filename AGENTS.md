# AGENTS.md — Devour Auth

> 给所有接手这个仓库的 AI / 协作者。先读这一页，再读 `docs/design.md`。

## 1. 项目

| 项 | 事实 |
|---|---|
| 仓库 | 公开，AGPL-3.0 |
| 形态 | 网络验证服务端 + 管理端 + 代理端，不含客户端 |
| 语言 | Go 1.26（服务端）、TypeScript（前端） |
| 数据库 | SQLite WAL；换 Postgres 只改 `internal/store` |
| 部署 | 单二进制，systemd，nginx TLS |
| 域名 | `net.namikao.com` |

## 2. 硬约束

**安全**

- 不把密码学防线放在客户端假设上；所有判定以服务端为准。
- 卡密、令牌、设备指纹、密码、API secret 不得进入日志、错误信息、审计 detail。
- 密钥只从环境变量或本地文件读取，不入库、不入仓库。
- 管理端与代理端是两套独立会话域，交叉访问必须 403。
- 敏感管理操作必须 reauth（5 分钟内有效）。
- `audit_log` append-only，任何绕过触发器的写法都不接受。

**数据**

- 时间统一 Unix 秒；金额统一「分」。
- 发卡时写入 `snapshot_json`，修改卡类型不得改变已生成卡的行为。
- 扣点/扣次必须在事务内完成，且不允许扣成负数。
- 删除一律软删（`recycled`），不物理删卡密。

**代码**

- 依赖最小化。新增第三方库前说明必要性、体积、维护成本。
- 分层：`httpapi` 只做路由/鉴权/序列化，业务在 `auth/card/agent/...`，
  SQL 只在 `store`。
- 业务函数不直接读环境变量，配置从 `main` 注入。
- 所有对外错误用 `code` 枚举，不把内部错误透传。
- SQLite 单连接（`SetMaxOpenConns(1)`）：事务内只能用 `store.*Tx` 方法，
  混用 `*sql.DB` 的查询会把连接锁死（事务等连接、连接等事务）。

**前端**

- 管理端与代理端同一工程，按角色路由，接口层强制数据范围。
- 金额显示统一走格式化函数，不在页面里除以 100。
- 敏感操作二次确认只是体验，服务端校验才是防线。

## 3. 命令

```bash
make dev          # 后端 + 迁移
make web          # Vite
make test         # go test + vue-tsc
make check        # fmt + lint + test
make build        # 当前平台
make deploy HOST=root@43.248.187.94
```

## 4. 验证

声称完成前，按顺序拿证据：

1. `make check` 通过。
2. 本地跑通对应接口或页面。
3. `docs/design.md` 第 19 节的验收项，相关条目已通过。
4. 部署到服务器后 `curl /healthz` 正常，日志无新异常。

## 5. Git

- `main` 受保护，只接受 PR。
- 一个任务 = 一个分支 = 一个 PR。
- Conventional Commits，标题 ≤72 字符。
- 每个可验证节点推分支。
- 不提交 `.env`、`data/`、`*.db`、密钥、备份。

## 6. 状态写在哪里

| 内容 | 位置 |
|---|---|
| 设计、数据模型、协议 | `docs/design.md` |
| 部署与故障处理 | `docs/runbook.md` |
| 接口定义 | `docs/openapi.yaml` |
| 进行中的任务 | 会话 todo / PR |
| 已发生的改动 | git log、PR |

一条事实只写一处；能从代码读出的不写。
