#!/usr/bin/env bash
# 一键构建：前端 → 内嵌资源 → 单二进制（Linux / macOS / Windows Git Bash 通用）
#
# 用法：./scripts/build.sh [版本号]
set -euo pipefail

VERSION="${1:-0.1.0}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

command -v node >/dev/null 2>&1 || { echo "未找到 node，请安装 Node.js 20+" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "未找到 go，请安装 Go 1.25+" >&2; exit 1; }

echo "==> [1/4] 安装前端依赖"
( cd web && npm install --no-audit --no-fund )

echo "==> [2/4] 构建前端"
( cd web && npm run build )

echo "==> [3/4] 同步前端产物到内嵌目录"
rm -rf server/internal/webui/dist/assets server/internal/webui/dist/index.html
cp -r web/dist/. server/internal/webui/dist/

echo "==> [4/4] 编译后端（版本 $VERSION）"
mkdir -p dist
BIN="dist/lanreel"
if [ "${OS:-}" = "Windows_NT" ] || [ -n "${WINDIR:-}" ]; then
  BIN="dist/lanreel.exe"
fi
( cd server && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "../$BIN" ./cmd/lanreel )

SIZE=$(du -h "$BIN" | cut -f1)
echo ""
echo "构建完成：$BIN（$SIZE，含内嵌前端）"
echo "启动：ADMIN_INIT_PASSWORD='你的密码' ./$BIN --config config.yaml"
