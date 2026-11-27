BIN := bin/devour-authd

.PHONY: dev test check fmt build build-linux-amd64 clean web web-build

# 本地起服务（自动迁移）
dev:
	go run ./cmd/devour-authd

# 前端开发服务器（代理到 127.0.0.1:6760）
web:
	cd web && npm run dev

web-build:
	cd web && npm run build

# 本地测试数据：权限、六种卡类型、每种两张卡（可重复执行）
seed:
	go run ./cmd/devour-authd seed

# 建首个管理员（随机密码 + TOTP 秘钥只打印一次）
dev-init:
	go run ./cmd/devour-authd init-admin

test:
	go test ./...

check:
	@test -z "$$(gofmt -l .)" || { echo "以下文件未格式化："; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o $(BIN) ./cmd/devour-authd

# 服务器是 x86_64，本机交叉编译
build-linux-amd64:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o $(BIN)-linux-amd64 ./cmd/devour-authd

clean:
	rm -rf bin
