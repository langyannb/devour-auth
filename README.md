# Devour Auth

自托管的网络验证服务：卡密、设备绑定、在线心跳、功能权限、代理分销、审计。

替换原有第三方验证，服务端权威、TLS、可吊销、可审计。当前只有服务端与管理端，
客户端协议已预留。

## 状态

I1 + I2 已完成并部署在 `https://net.namikao.com`：卡密/卡类型/权限/设备/会话、审计链与锚定、
代理体系（折扣/额度/返点/账单）与代理端。未开始：开放 API（I3）、运营端（I4）。

## 技术栈

- 服务端：Go 1.26，标准库 `net/http`，SQLite（WAL），前端产物 `embed`
- 管理端 / 代理端：Vue 3 + TypeScript + Vite + Naive UI（图表页再引入 ECharts；状态用模块级 ref，暂不引 Pinia）
- 部署：单二进制 + systemd + nginx（TLS 终止）

## 快速开始

```bash
make dev          # 后端 http://127.0.0.1:6760
make web          # 前端 http://127.0.0.1:5173
make seed         # 测试数据
make test         # 单测 + 类型检查
```

首次运行创建管理员：

```bash
./bin/authd admin create --username admin
```

## 文档

| 文件 | 内容 |
|---|---|
| `docs/design.md` | 完整设计：数据模型、卡类型、权限、代理、协议、安全 |
| `docs/runbook.md` | 部署、发布、回滚、备份、故障处理 |
| `docs/openapi.yaml` | 开放接口定义（待补） |

## 约定

- 所有时间戳为 Unix 秒（UTC）。
- 所有金额以「分」存储与传输。
- 卡密、令牌、设备指纹永不写入日志。
- 密钥只通过环境变量或服务器本地文件提供，不入库。

## 许可

AGPL-3.0，见 `LICENSE`。
