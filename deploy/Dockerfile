# 多阶段构建：前端产物 → 内嵌静态资源的单二进制（N-18）
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY web ./
RUN npm run build

FROM golang:1.25-alpine AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server ./
# 覆盖占位目录，把前端产物编译进二进制
COPY --from=web /src/web/dist ./internal/webui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION:-0.1.0}" -o /out/lanreel ./cmd/lanreel

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata && adduser -D -H -u 10001 lanreel
WORKDIR /app
COPY --from=server /out/lanreel /app/lanreel
RUN mkdir -p /app/data && chown -R lanreel:lanreel /app
USER lanreel
ENV LANREEL_DATA_DIR=/app/data \
    LANREEL_MEDIA_ROOT=/media \
    GOMEMLIMIT=200MiB \
    GOMAXPROCS=2
EXPOSE 8080
VOLUME ["/app/data", "/media"]
ENTRYPOINT ["/app/lanreel"]
CMD ["--config", "/app/config.yaml"]
