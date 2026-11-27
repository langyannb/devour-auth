# 运维手册

面向部署在 `43.248.187.94` 的 `devour-authd`。所有路径以服务器为准，本地开发见第 9 节。

## 1. 环境

| 项 | 值 |
|---|---|
| 域名 | `net.namikao.com` |
| 服务器 | `43.248.187.94` |
| 监听 | `127.0.0.1:6760` |
| 程序 | `/opt/devour-auth/devour-authd` |
| 数据 | `/var/lib/devour/app.db` |
| 备份 | `/var/backups/devour/` |
| 静态前端 | 已 embed 进二进制，无独立目录 |
| 运行用户 | `devour`（无登录 shell） |

首次部署前确认（2026-11-27 实测）：

| 项 | 结果 |
|---|---|
| 架构 | `x86_64`，交叉编译目标 amd64 |
| 80 / 443 | 已被 nginx 占用，上有 7 个站点；certbot 走 webroot，不能用 standalone |
| 6760 | 空闲 |
| certbot | 已安装；`/etc/letsencrypt/live` 下无 `net.namikao.com`，证书待申请 |
| Go 运行时 | 未安装，服务器只接收交叉编译好的二进制 |

复核命令：

```bash
uname -m
ss -lntp | grep -E ':(80|443|6760)'
certbot certificates
```

## 2. 首次部署

### 2.1 用户与目录

```bash
useradd -r -s /usr/sbin/nologin devour
install -d -o devour -g devour -m 700 /var/lib/devour /var/backups/devour
install -d -m 755 /opt/devour-auth
```

### 2.2 安装二进制

本地构建后上传：

```bash
make build-linux-amd64        # 或 build-linux-arm64
scp bin/devour-authd root@43.248.187.94:/opt/devour-auth/devour-authd
ssh root@43.248.187.94 'chmod 755 /opt/devour-auth/devour-authd'
```

### 2.3 环境变量

`/etc/devour-authd.env`，权限 `600`，属主 `root`：

```
DEVOUR_LISTEN=127.0.0.1:6760
DEVOUR_DB=/var/lib/devour/app.db
DEVOUR_ENV=prod
DEVOUR_AUDIT_KEY=/etc/devour-authd/audit_ed25519.key
```

- `DEVOUR_ENV=prod` 时管理端 cookie 用 `__Host-` 前缀并带 `Secure`，必须走 HTTPS。
- 审计私钥首次启动自动生成（0600，64 字节 Ed25519），也可靠 `DEVOUR_AUDIT_KEY` 指定路径。
- 尚未实现：管理 IP 白名单、日志级别开关。

**密钥与 `.env` 永不入库。** 仓库只保留 `.env.example`。

### 2.4 systemd

`/etc/systemd/system/devour-authd.service`：

```ini
[Unit]
Description=Devour Auth
After=network.target

[Service]
Type=simple
User=devour
Group=devour
EnvironmentFile=/etc/devour-authd.env
ExecStart=/opt/devour-auth/devour-authd
Restart=always
RestartSec=3
LimitNOFILE=65535
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/var/lib/devour /var/backups/devour
ProtectHome=true

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now devour-authd
```

### 2.5 初始管理员

```bash
sudo -u devour /opt/devour-auth/devour-authd init-admin
# 输出一次性密码与 TOTP 秘钥（otpauth 链接），保存后关闭终端
```

不要设置默认密码，不要写进仓库或文档。

### 2.6 nginx

`/etc/nginx/sites-available/devour-auth`：

```nginx
limit_req_zone $binary_remote_addr zone=auth:10m rate=5r/s;

server {
    listen 80;
    server_name net.namikao.com;
    location /.well-known/acme-challenge/ { root /var/www/certbot; }
    location / { return 301 https://$host$request_uri; }
}

server {
    listen 443 ssl http2;
    server_name net.namikao.com;

    ssl_certificate     /etc/letsencrypt/live/net.namikao.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/net.namikao.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;

    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options DENY always;
    add_header X-Content-Type-Options nosniff always;
    add_header Referrer-Policy no-referrer always;

    client_max_body_size 2m;

    location / {
        limit_req zone=auth burst=20 nodelay;
        proxy_pass http://127.0.0.1:6760;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 30s;
    }
}
```

```bash
ln -s /etc/nginx/sites-available/devour-auth /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
```

### 2.7 证书

```bash
certbot --nginx -d net.namikao.com
systemctl list-timers | grep certbot    # 确认自动续期
```

证书到期前 14 天在后台「安全体检」会告警。

## 3. 日常发布

```bash
make check            # fmt + lint + 单测 + 前端类型检查
make deploy HOST=root@43.248.187.94
```

`deploy.sh` 做的事：

1. 本地交叉编译（按 `TARGET_ARCH`，默认当前机器架构）。
2. `rsync` 到 `/opt/devour-auth/devour-authd.new`。
3. 远端 `mv` 当前二进制为 `devour-authd.prev`。
4. 替换、`systemctl restart`。
5. `curl -fsS http://127.0.0.1:6760/healthz`，失败自动回滚并退出非零。

**不做自动 CD。** 只有本地验证通过才手动部署。

## 4. 回滚

```bash
ssh root@43.248.187.94 '
  systemctl stop devour-authd
  cp /opt/devour-auth/devour-authd.prev /opt/devour-auth/devour-authd
  systemctl start devour-authd
  curl -fsS http://127.0.0.1:6760/healthz
'
```

数据库迁移只向前兼容：新版本加了列/表，回滚旧二进制不会崩，但旧版本不会使用新字段。
涉及删列或改语义的迁移必须拆成两个版本发布。

## 5. 备份与恢复

### 5.1 自动备份

`deploy/backup.sh` 安装在 `/opt/devour-auth/backup.sh`，由 `devour-backup.timer` 每小时执行：

- 走 SQLite 在线备份 API 导出（不直接拷 WAL 中间态）
- GPG 加密，收件人 `devour-backup`；公钥在服务器 `/etc/devour-authd/backup-public.asc`
- 保留 14 天（`find -mtime +14`）

**私钥不在应用服务器上。** 私钥 `devour-backup-private.asc` 只在运维本地（当前
`/root/devour-backup-private.asc`，0600），务必另存到离线介质：丢了就解不开任何备份。

首次部署生成密钥（在一台有 gpg 的机器上）：

```bash
export GNUPGHOME=/root/.gnupg-backup
gpg --batch --passphrase '' --quick-gen-key 'devour-backup <backup@devour.local>' rsa2048 encr 0
gpg --armor --export-secret-keys devour-backup > devour-backup-private.asc   # 带走，不要留在服务器
gpg --armor --export devour-backup > /etc/devour-authd/backup-public.asc
```

### 5.2 恢复

```bash
systemctl stop devour-authd
gpg --batch -d /var/backups/devour/app-2026112716.db.gpg > /var/lib/devour/app.db
chown devour:devour /var/lib/devour/app.db
chmod 600 /var/lib/devour/app.db
systemctl start devour-authd
```

恢复后进入后台跑一次「审计校验」，确认链完整。

### 5.3 异地

建议每日把最新备份同步到另一台机器或对象存储。未实现前，至少每周手工下载一次。

## 6. 审计链

- 进程内每天 00:05 对链尾签名写入 `audit_anchor`；`devour-authd anchor` 可手动补锚（幂等）。
- 私钥默认 `data/audit_ed25519.key`，生产用 `DEVOUR_AUDIT_KEY` 指到
  `/etc/devour-authd/offline-ed25519.key`（0600，首次启动自动生成，不入库不入仓库）。
- 校验：`DEVOUR_DB=... DEVOUR_AUDIT_KEY=... devour-authd verify-audit`
  （重算整链 + 校验每个锚点签名与其指向的链尾记录）。
- 断链处理：立即导出当前 audit_log 与 anchor，停止写入，排查是谁有写权限。
- 私钥单独备份，泄露即轮换。

## 7. 密钥轮换

| 密钥 | 轮换方式 | 影响 |
|---|---|---|
| 离线票据 Ed25519 | 生成新钥，旧钥保留 7 天双签 | 无感 |
| 开放 API secret | 后台轮换，旧钥保留 24 小时 | 调用方需更新 |
| 管理员 TOTP | 后台重置，重新扫码 | 该账号需重新绑定 |
| 数据库备份 GPG | 新收件人 + 重新加密历史备份 | 无 |

轮换全部写审计，需要 reauth。

## 8. 常见故障

| 现象 | 排查 |
|---|---|
| 502 | `systemctl status devour-authd`、`journalctl -u devour-authd -n 100` |
| 全部登录失败 | 检查 `DEVOUR_DB` 权限、磁盘满、`sqlite3 app.db 'PRAGMA integrity_check'` |
| `database is locked` | 是否有第二个进程打开 DB；确认 WAL 与 `busy_timeout` |
| 证书过期 | `certbot renew --dry-run`，检查 80 端口 |
| 审计链断 | 见第 6 节 |
| 在线数异常高 | 检查是否有伪造心跳；看 `sessions.last_heartbeat` 分布与 IP |
| 登录被限流误伤 | 检查 nginx `limit_req` 与应用层计数；临时提高阈值需审计 |
| `certbot` 报 `X509_V_FLAG_NOTIFY_POLICY` / `X509Req` | 系统 pyOpenSSL 与 `/usr/local` 下的 cryptography 版本不匹配。用清华源覆盖安装：`python3 -m pip install -U -i https://pypi.tuna.tsinghua.edu.cn/simple certbot`（当前服务器用的是 `/usr/local/bin/certbot` 5.10.0） |
| ACME 验证 `secondary validation timeout` | 偶发。确认 `curl http://net.namikao.com/.well-known/acme-challenge/test` 能取到文件后原样重试即可 |

## 9. 本地开发

```bash
make dev        # 后端 127.0.0.1:6760，SQLite ./data/app.db，自动迁移
make web        # Vite 5173，代理 /api /admin/api /open 到 6760
make seed       # 生成测试数据：卡类型、卡密、代理、权限
make test       # go test ./... + 前端 vue-tsc
```

本地全部走 HTTP，不需要域名或证书。重置环境：`rm -rf data/`。

管理员（随机密码 + TOTP 秘钥只打印一次）：

```bash
make dev-init          # 等价于 go run ./cmd/devour-authd init-admin
```

`make dev-init` 后按输出把 TOTP 秘钥导入验证器（或手动算码），登录时填 6 位动态码。

### 用 Server Browser 看前端

OpenChamber 的 Server Browser 扩展默认拦截本地与私有地址，页面会显示 `Blocked: http://127.0.0.1:5173`。
在扩展目录的 `config.json` 里放行：

```bash
# ~/.config/openchamber/extensions/server-browser/config.json
{
  "chromePath": "/usr/local/bin/chrome-no-sandbox",
  "allowedOrigins": ["http://127.0.0.1:5173", "http://127.0.0.1:6760"]
}
```

改完重启扩展：`pkill -f 'server-browser/service/main.js'`，OpenChamber 会自动拉起。
缺 Chrome 时按 README 把 `chromePath` 指到本机 `chrome` 二进制。

## 10. 监控

- `GET /healthz`：进程存活 + DB 可读。
- `GET /metrics`：Prometheus 文本（在线数、登录成功率、QPS、错误码计数、DB 大小）。
- 日志：`slog` JSON 到 stdout，由 journald 收集。卡密、令牌、设备指纹不落日志。

## 11. 应急

| 场景 | 操作 |
|---|---|
| 被大规模攻击 | 后台开维护模式，nginx 临时封网段 |
| 某张卡被共享 | 后台封卡 + 拉黑设备指纹 |
| 代理违规 | 冻结代理（不能出卡），保留账单 |
| 密钥泄露 | 轮换对应密钥 + 吊销全部会话 |
| 数据库损坏 | 停服务 → 用最近备份恢复 → 校验审计链 |
