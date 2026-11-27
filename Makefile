BIN := bin/devour-authd

.PHONY: dev test check fmt build build-linux-amd64 clean

# 本地起服务（自动迁移）
dev:
	go run ./cmd/devour-authd

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
