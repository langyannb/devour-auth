BIN := bin/devour-authd
HOST ?= root@43.248.187.94
REMOTE_DIR ?= /opt/devour-auth

.PHONY: dev test check fmt build build-linux-amd64 deploy web web-assets seed dev-init clean

# 本地起服务（自动迁移）
dev:
	go run ./cmd/devour-authd

# 前端开发服务器（代理到 127.0.0.1:6760）
web:
	cd web && npm run dev

# go:embed 要求 internal/webui/dist 存在；干净检出先补一个占位
web-assets:
	@mkdir -p internal/webui/dist && touch internal/webui/dist/.gitkeep

web-build: web-assets
	cd web && npm run build

# 本地测试数据：权限、六种卡类型、每种两张卡（可重复执行）
seed:
	go run ./cmd/devour-authd seed

# 建首个管理员（随机密码 + TOTP 秘钥只打印一次）
dev-init:
	go run ./cmd/devour-authd init-admin

test: web-assets
	go test ./...

fmt:
	gofmt -w .

check: web-assets
	@test -z "$$(gofmt -l .)" || { echo "以下文件未格式化："; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...
	cd web && npx vue-tsc --noEmit && npx vitest run

build: web-build
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o $(BIN) ./cmd/devour-authd

# 服务器是 x86_64，本机交叉编译
build-linux-amd64: web-build
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o $(BIN)-linux-amd64 ./cmd/devour-authd

# 发布：传新二进制 → 备份旧版 → 替换 → 重启 → 健康检查，失败自动回滚
deploy: build-linux-amd64
	scp $(BIN)-linux-amd64 $(HOST):$(REMOTE_DIR)/devour-authd.new
	ssh $(HOST) 'set -e; cd $(REMOTE_DIR); cp -p devour-authd devour-authd.prev; chmod 755 devour-authd.new; mv devour-authd.new devour-authd; systemctl restart devour-authd; sleep 2; if ! curl -fsS http://127.0.0.1:6760/healthz; then cp -p devour-authd.prev devour-authd; systemctl restart devour-authd; echo "健康检查失败，已回滚"; exit 1; fi; systemctl is-active devour-authd'
	@echo "已发布到 $(HOST)"

clean:
	rm -rf bin
