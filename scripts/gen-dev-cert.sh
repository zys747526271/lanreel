#!/usr/bin/env bash
# 生成自签名证书（N-13）：局域网首次访问需手动信任，公网部署请改用 Let's Encrypt 反向代理。
#
# 用法：
#   ./scripts/gen-dev-cert.sh [IP 或域名] [输出目录]
# 示例：
#   ./scripts/gen-dev-cert.sh 192.168.1.10 ./data/certs
set -euo pipefail

HOST="${1:-localhost}"
OUT_DIR="${2:-./data/certs}"
DAYS="${DAYS:-825}"

mkdir -p "$OUT_DIR"

if ! command -v openssl >/dev/null 2>&1; then
  echo "未找到 openssl，请先安装（Windows 可用 Git Bash 自带的 openssl）" >&2
  exit 1
fi

openssl req -x509 -newkey rsa:2048 -sha256 -days "$DAYS" -nodes \
  -keyout "$OUT_DIR/lanreel.key" \
  -out "$OUT_DIR/lanreel.crt" \
  -subj "/C=CN/O=LanReel/CN=$HOST" \
  -addext "subjectAltName=DNS:$HOST,IP:$HOST"

chmod 600 "$OUT_DIR/lanreel.key" || true

cat <<EOF
已生成自签名证书：
  证书: $OUT_DIR/lanreel.crt
  私钥: $OUT_DIR/lanreel.key
有效期: $DAYS 天

在 config.yaml 中启用：
  server:
    tls:
      enabled: true
      cert_file: "$OUT_DIR/lanreel.crt"
      key_file: "$OUT_DIR/lanreel.key"

或用环境变量：
  LANREEL_TLS_ENABLED=true LANREEL_TLS_CERT=$OUT_DIR/lanreel.crt LANREEL_TLS_KEY=$OUT_DIR/lanreel.key
EOF
