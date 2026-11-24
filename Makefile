# LanReel 构建与运行脚本
#
# 用法：
#   make build        构建前端 + 内嵌资源 + 单二进制（输出到 dist/）
#   make web          仅构建前端
#   make server       仅构建后端（使用已有内嵌资源）
#   make dev          并行启动后端(8080)与前端开发服务器(5173)
#   make clean        清理构建产物
#
# 依赖：Go 1.23+、Node.js 20+

GO ?= go
NPM ?= npm
VERSION ?= 0.2.1
BIN_DIR := dist
WEB_DIR := web
EMBED_DIR := server/internal/webui/dist
LDFLAGS := -s -w -X main.version=$(VERSION)

# Windows 下必须输出 .exe，否则用户双击 dist/lanreel.exe 会命中旧文件
ifeq ($(OS),Windows_NT)
  BIN := $(BIN_DIR)/lanreel.exe
else
  BIN := $(BIN_DIR)/lanreel
endif

.PHONY: all build web embed server dev clean test

all: build

web:
	cd $(WEB_DIR) && $(NPM) install --no-audit --no-fund && $(NPM) run build

embed: web
	rm -rf $(EMBED_DIR)/assets $(EMBED_DIR)/index.html
	cp -r $(WEB_DIR)/dist/. $(EMBED_DIR)/

server: embed
	cd server && $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o ../$(BIN) ./cmd/lanreel

build: server

dev:
	@echo "启动后端 :8080 与前端 :5173（Ctrl+C 结束）"
	@(cd server && $(GO) run ./cmd/lanreel) & cd $(WEB_DIR) && $(NPM) run dev

test:
	cd server && $(GO) test ./...
	cd $(WEB_DIR) && $(NPM) run typecheck

clean:
	rm -rf $(BIN_DIR) $(WEB_DIR)/dist $(EMBED_DIR)/assets $(EMBED_DIR)/index.html
