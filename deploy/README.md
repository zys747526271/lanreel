# 部署说明

LanReel 编译产物是**单二进制 + 内嵌前端**，部署只需要一个可执行文件、一个配置文件和一个媒体目录。

## 1. 通用步骤

```bash
# 1) 构建（需要 Go 1.25+、Node 20+）
make build            # 产出 dist/lanreel（Windows 为 dist/lanreel.exe）

# 2) 准备配置
cp server/configs/config.example.yaml config.yaml
#    修改 media.roots 指向你的媒体目录

# 3) 准备管理员密码（不要写进配置文件并提交到仓库）
export ADMIN_INIT_PASSWORD='你的强密码'

# 4) 启动
./dist/lanreel --config config.yaml

# 5) 首次登录后建议
#    - 在「系统设置」确认注册入口是否需要关闭
#    - 在「系统设置 → 日志策略」按磁盘容量调整保留天数
```

启动后访问 `http://<局域网IP>:8080`。

## 2. 关键环境变量

| 变量 | 说明 | 默认 |
|------|------|------|
| `LANREEL_CONFIG` | 配置文件路径 | `./config.yaml` |
| `LANREEL_HOST` / `LANREEL_PORT` | 监听地址与端口 | `0.0.0.0` / `8080` |
| `LANREEL_MEDIA_ROOT` | 单个媒体根目录 | `./media` |
| `LANREEL_MEDIA_ROOTS` | 多个媒体根目录（逗号或分号分隔） | — |
| `LANREEL_DATA_DIR` | 数据目录（业务库、日志库、密钥） | `./data` |
| `ADMIN_INIT_PASSWORD` | 管理员初始密码 | 无（必须提供） |
| `LANREEL_JWT_SECRET` | JWT 密钥，留空自动生成并持久化 | 自动生成 |
| `LANREEL_LOG_LEVEL` | `debug`/`info`/`warn`/`error` | `info` |
| `LANREEL_TLS_ENABLED` | 是否启用 HTTPS | `false` |
| `LANREEL_TLS_CERT` / `LANREEL_TLS_KEY` | 证书与私钥路径 | — |
| `LANREEL_TIMEZONE` | 时区（影响时间窗口与日志时间） | `Asia/Shanghai` |
| `GOMEMLIMIT` | Go 内存软上限 | 建议 `200MiB` |
| `GOMAXPROCS` | 使用 CPU 核心数 | 建议 `2` |

## 3. Linux / NAS（systemd）

```bash
sudo useradd -r -s /usr/sbin/nologin lanreel
sudo mkdir -p /opt/lanreel && sudo cp dist/lanreel /opt/lanreel/
sudo cp server/configs/config.example.yaml /opt/lanreel/config.yaml
sudo mkdir -p /opt/lanreel/data && sudo chown -R lanreel:lanreel /opt/lanreel

# 管理员密码放环境文件，避免出现在 systemd unit 里
echo "ADMIN_INIT_PASSWORD=你的强密码" | sudo tee /etc/lanreel/env
sudo chmod 600 /etc/lanreel/env

sudo cp deploy/systemd/lanreel.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now lanreel
sudo systemctl status lanreel
```

> NAS 上请把媒体目录挂载为只读，数据目录（`/opt/lanreel/data`）挂载到持久卷，
> 这样业务库与日志库在重启后仍然保留。

## 4. Docker / docker-compose

```bash
cd deploy
# 修改 docker-compose.yml 里的媒体目录与管理员密码
docker compose up -d --build
docker compose logs -f
```

镜像内已设置 `LANREEL_MEDIA_ROOT=/media`、`LANREEL_DATA_DIR=/app/data`，
并以非 root 用户（uid 10001）运行。

## 5. HTTPS（自签名）

```bash
./scripts/gen-dev-cert.sh 192.168.1.10 ./data/certs
# 然后在 config.yaml 中打开 server.tls
```

首次访问浏览器会提示证书不受信任，选择「继续访问」即可。若后续要暴露到公网，
请改用 Nginx/Caddy 反向代理 + Let's Encrypt，LanReel 自身保持 HTTP 仅监听内网。

## 6. 资源占用建议

- **内存**：空闲实测约 28MB；`GOMEMLIMIT=200MiB` 作为软上限即可。
- **CPU**：`GOMAXPROCS=2` 通常足够（无转码，只做文件流输出）。
- **Windows 与游戏共存**：用 Process Lasso 把 `lanreel.exe` 设为「低于正常」优先级，
  并绑定到非游戏使用的核心；也可用任务计划程序在开机时以低优先级启动。
- **磁盘**：日志库默认保留 30 天 / 20 万条，可在后台「系统设置 → 日志策略」调整。

## 7. 备份与排障

| 目标 | 做法 |
|------|------|
| 备份账号与收藏 | 备份 `data/lanreel.db`（WAL 模式下建议先停服或用 `sqlite3 .backup`） |
| 备份运行日志 | 备份 `data/logs.db`，或在后台导出后归档 |
| 排查请求问题 | 后台「运行日志」按 `request_id` 过滤，可串联同一次请求的前后端日志 |
| 媒体不显示 | 检查 `data/logs/backend-*.log` 与「运行日志」中 `scan` 分类的 `skipped`/`errors` |
| 忘记管理员密码 | 带 `--reset-admin-password` 且配置 `ADMIN_INIT_PASSWORD` 重启一次 |
