# LanReel

局域网随机视频流媒体系统：部署在 Windows / Linux（含 NAS）主机上，用浏览器以抖音式竖屏滑动方式浏览指定文件夹里的视频与图片，支持注册登录、跨设备收藏同步与管理员后台。

**当前版本：v0.1.0（可运行）** — 前后端已完成并在浏览器实测通过。Go 单二进制内嵌前端，启动即用。

<p>
  <img src="./docs/screenshots/setup-admin.png" width="240" alt="首次部署：创建管理员" />
  <img src="./docs/screenshots/feed-video.png" width="240" alt="浏览页：全屏竖滑 + 视频播放" />
  <img src="./docs/screenshots/feed-favorites.png" width="240" alt="仅看收藏模式" />
  <img src="./docs/screenshots/admin-dashboard.png" width="620" alt="管理后台概览" />
</p>

## 特性

**浏览体验**
- 全屏竖屏滑动（触摸 / 滚轮 / 键盘 ↑↓、空格），视频自动播放、离开即暂停
- 图片按配置时长（默认 5 秒）自动切换，底部有停留进度提示
- 视频支持拖拽进度、HTTP Range 断点续播，缓存头 `public, max-age=86400`
- 「随机」与「仅收藏」两种浏览模式，一屏一媒体，切换零刷新

**内容管控**
- 管理员可对文件夹、单文件分别设置「可播 / 禁用 / 时间窗口（支持跨零点）」
- 策略优先级：文件级覆盖 > 文件夹策略（最长前缀）> 默认允许；根目录策略作为全局默认
- 被限制的内容仍保留在收藏中，以模糊预览 + 原因提示展示，不会静默消失
- 时间判断全部在服务端完成，时区可配置

**账号与收藏**
- 注册（受管理员开关控制）、JWT 登录、令牌自动刷新、登出清缓存
- 收藏以「媒体源 + 相对路径」哈希标识，迁移文件夹不失效；服务端为准，跨设备即时同步
- 前端用 IndexedDB 缓存收藏，断网时展示缓存并标记「离线」

**可诊断性**
- 后端按 13 个分类写入独立的 `logs.db`（异步批量、失败降级文件），后台可按分类 / 级别 / 时间 / 关键字 / 请求 ID 检索
- 前端按分类写入 IndexedDB `app_logs`，设置页可查看、导出（自动脱敏密码与 Token）
- 管理员所有写操作进审计日志，独立保留、不随运行日志清理

**部署与资源**
- 单二进制（26MB，含前端），`CGO_ENABLED=0` 可交叉编译
- 首次部署在浏览器内创建管理员，之后每次启动直接进主页（无浏览器可用 `ADMIN_INIT_PASSWORD` 预置）
- 媒体目录可在后台「媒体目录」增删（内置目录浏览器），保存后立即热重扫，无需改配置或重启
- 不做服务端转码、不缓存媒体本体；空闲内存实测 ~28MB
- `GOMEMLIMIT` / `GOMAXPROCS` 通过环境变量注入，便于与前台游戏共存

## 快速开始

```bash
# 1) 构建（Go 1.25+、Node 20+）
./scripts/build.sh          # Windows 可用 scripts/build.ps1 或 make build

# 2) 配置：先填 media.roots 指向你的媒体目录（也可启动后在后台「媒体目录」里改）
cp server/configs/config.example.yaml config.yaml

# 3) 启动（Windows 为 dist\lanreel.exe）
./dist/lanreel --config config.yaml
```

打开 `http://<本机或局域网IP>:8080`：

- **首次部署**：页面自动进入「创建管理员」，设置用户名与密码后直接进入主页。
  该页面只在实例尚未初始化时出现，之后的每次启动都直接进主页（未登录则到登录页）；
  创建成功后接口即失效，不会出现「谁先打开谁当管理员」。
- **管理员**：用你创建的管理员账号登录，浏览页右上角出现「后台」，或从设置页「管理后台」进入 `/admin`。
- **普通用户**：从登录页「注册」自助注册（管理员可在后台关闭该入口），只能浏览与收藏；
  访问 `/admin` 会进入权限说明页，提示当前账号与切换账号入口。

无浏览器的部署（容器 / 自动化）仍可按配置预置管理员：启动时提供 `ADMIN_INIT_PASSWORD`，
服务会直接创建 `auth.username`（默认 `admin`）并跳过初始化页面。
两者只需选一种：预置了密码就不会再出现创建页面。

忘记管理员密码：带上 `ADMIN_INIT_PASSWORD` 与 `--reset-admin-password` 重启一次即可重置
（不带该参数不会改动已有密码）。若管理员用户名与 `auth.username` 不同，会重置最早创建的那个管理员。

> 只想先看看效果？准备一个文件夹放几段 MP4 和几张图片，`media.roots` 指过去即可；
> 也支持 `LANREEL_MEDIA_ROOT=/path/to/media` 环境变量。

其他命令：

```bash
# 前端开发模式（热更新，/api 代理到 127.0.0.1:8080）
cd server && go run ./cmd/lanreel          # 终端 1
cd web && npm run dev                      # 终端 2 → http://localhost:5173

# 检查
cd server && go vet ./... && go test ./...
cd web && npx vue-tsc --noEmit
```

> Makefile 提供等价目标：`make web` / `make server` / `make build` / `make test` / `make clean`，
> 其中 `make build` 会依次执行前端构建、内嵌资源同步与后端编译。

## 目录结构

```
.
├── server/                    Go 后端（Gin + modernc/sqlite，纯 Go 无 CGO）
│   ├── cmd/lanreel/           入口：配置 → 建库迁移 → 扫描 → HTTP → 日志清理
│   ├── internal/api/          HTTP 层（路由、中间件、处理器，仅做协议适配）
│   ├── internal/service/      业务层（认证、媒体、收藏、管理员）
│   ├── internal/media/        媒体源与扫描、video_key、MP4 时长探测
│   ├── internal/policy/       可见性与时间窗口判定
│   ├── internal/logging/      分类日志 + 异步批量落库 + 检索
│   ├── internal/repository/   数据访问（业务库）
│   ├── internal/database/     连接与版本化迁移（业务库 / 日志库各自独立）
│   ├── internal/auth/         bcrypt + JWT
│   ├── internal/webui/        内嵌前端产物（构建时填充）
│   └── configs/               配置示例
├── web/                       Vue 3 前端（Vite + TS + Tailwind，后台 Naive UI）
│   └── src/
│       ├── views/             浏览页、登录注册、设置诊断、admin/* 后台
│       ├── components/reel/   滑动容器与播放器（二者解耦）
│       ├── api/               统一请求层（鉴权、刷新、错误、日志）
│       ├── stores/            Pinia：auth / feed / favorites
│       └── lib/               IndexedDB、分类日志、格式化
├── docs/
│   ├── requirements/          需求管理（原始 / 版本 / 已完成 / 未完成 / 变更）
│   └── screenshots/           界面截图
├── deploy/                    systemd、Dockerfile、docker-compose、部署说明
├── scripts/                   构建辅助与自签证书脚本
└── Makefile
```

## 架构

```
浏览器 / 手机 / 电视（未来）
        │  HTTPS(可选) + JWT
        ▼
┌──────────────────────────────────────────────┐
│ Go 单二进制 (Gin)                             │
│  middleware: request_id → auth → admin → 访问日志│
│  api  ──► service ──► repository ──► lanreel.db │
│                 │                              │
│                 ├──► media.Scanner（快照 + 定时重扫）
│                 ├──► policy.Evaluator（策略判定）
│                 └──► logging.Logger ──► logs.db │
│  内嵌 web/dist（SPA，history 路由回退）          │
└──────────────────────────────────────────────┘
        │ http.ServeContent（Range / ETag）
        ▼
   本地媒体文件（只读，不转码）
```

分层目的：`handler` 只做协议适配，`service` 承载业务，`repository` 隔离存储——
未来换 PostgreSQL、加 GraphQL/gRPC 网关或远程日志聚合时，上层无需改动。

## 配置

完整示例见 [`server/configs/config.example.yaml`](./server/configs/config.example.yaml)，常用项：

| 配置 | 说明 | 默认 |
|------|------|------|
| `media.roots` | 媒体根目录（可多个）；后台「媒体目录」保存后会覆盖此项 | `./media` |
| `media.scan_interval_seconds` | 定时重扫间隔，0 表示仅启动扫描 | `60` |
| `media.image_display_seconds` | 图片停留秒数 | `5` |
| `app.timezone` | 时区（影响时间窗口与日志时间） | `Asia/Shanghai` |
| `database.path` / `logging.path` | 业务库 / 日志库文件 | `./data/lanreel.db`、`./data/logs.db` |
| `server.tls.enabled` | HTTPS 开关 | `false` |
| `auth.access_ttl_minutes` | 访问令牌有效期 | `120` |

环境变量覆盖（容器化 / NAS 常用）：
`LANREEL_PORT`、`LANREEL_MEDIA_ROOT(S)`、`LANREEL_DATA_DIR`、`ADMIN_INIT_PASSWORD`、
`LANREEL_JWT_SECRET`、`LANREEL_LOG_LEVEL`、`LANREEL_TLS_ENABLED/CERT/KEY`、`LANREEL_TIMEZONE`、
`GOMEMLIMIT`、`GOMAXPROCS`。详见 [deploy/README.md](./deploy/README.md)。

## API 一览（`/api/v1`）

### 普通接口

| 分组 | 接口 |
|------|------|
| 初始化 | `GET /setup/status`、`POST /setup/admin`（仅在未初始化时生效，创建第一个管理员） |
| 公开 | `GET /health`、`GET /config/public`、`POST /auth/register`、`POST /auth/login` |
| 认证 | `POST /auth/refresh`、`POST /auth/logout`、`GET /auth/me` |
| 媒体 | `GET /media/random?limit=&exclude=&mode=`、`GET /media/:key/stream`、`GET /media/:key/thumb` |
| 收藏 | `GET /favorites`、`POST /favorites`、`DELETE /favorites/:key` |
| 日志 | `POST /logs/client`（前端日志上报，默认关闭） |

### 管理接口（需 `role=admin`，共 26 个）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/admin/stats` | 概览统计（媒体、用户、收藏、策略、扫描、日志分布） |
| POST | `/admin/scan` | 立即触发媒体重扫 |
| GET | `/admin/folders` | 文件夹列表（含策略生效状态与继承来源） |
| PUT | `/admin/folders` | 设置文件夹策略 |
| DELETE | `/admin/folders?folder_path=` | 删除文件夹策略（回落默认） |
| GET | `/admin/files` | 文件列表（分页、关键字、文件夹过滤） |
| GET | `/admin/files/:key` | 单文件策略与可见性 |
| PUT | `/admin/files/:key` | 设置文件级覆盖 |
| DELETE | `/admin/files/:key` | 清除文件级覆盖 |
| GET | `/admin/users` | 用户列表 |
| PUT | `/admin/users/:id` | 启用/禁用、改角色、重置密码 |
| DELETE | `/admin/users/:id` | 删除用户（级联收藏） |
| GET | `/admin/users/:id/favorites` | 只读查看某用户收藏 |
| POST | `/admin/users/:id/favorites/clean` | 清理该用户的失效收藏 |
| DELETE | `/admin/favorites/:id` | 按 ID 删除收藏条目 |
| GET | `/admin/audit-logs` | 审计日志（对象类型、关键字、时间范围） |
| GET | `/admin/logs` | 运行日志（分类、级别、关键字、请求 ID、时间、分页） |
| GET | `/admin/logs/:id` | 单条日志详情（含脱敏上下文） |
| POST | `/admin/logs/purge` | 按保留策略清理运行日志 |
| GET | `/admin/settings/media` | 读取媒体目录、来源与扫描状态 |
| PUT | `/admin/settings/media` | 保存媒体目录并触发后台重扫（目录须存在） |
| DELETE | `/admin/settings/media` | 清除后台设置，回落到配置文件 `media.roots` |
| GET | `/admin/fs/dirs?path=` | 目录浏览器（`path` 为空列根/盘符，仅返回子目录） |
| GET | `/admin/settings/logging` | 读取日志配置 |
| PUT | `/admin/settings/logging` | 更新保留天数、最大条数、级别、上报开关 |
| PUT | `/admin/settings/registration` | 开关普通用户注册 |

约定：成功 `{"data": ...}`，分页附 `meta`，失败 `{"error":{"code","message"}}`。
管理接口未带令牌返回 `401`，普通用户调用返回 `403`；媒体流因 `<video>` 无法带自定义头，支持 `?token=` 传入访问令牌。
`POST /setup/admin` 在已初始化后返回 `409 setup_completed`；`GET /config/public` 与 `/setup/status` 都会返回 `setup_required`。

> 管理接口在前端对应 `/admin` 路由组（概览 / 文件夹策略 / 文件策略 / 用户管理 / 运行日志 / 审计日志 / 系统设置）。
> 入口：用管理员账号登录后，浏览页右上角「后台」或设置页「管理后台」。普通账号访问会进入 `/forbidden` 说明页。

## 日志设计

| 项 | 后端 | 前端 |
|----|------|------|
| 存储 | 独立 SQLite `data/logs.db`（与业务库物理分离） | IndexedDB `app_logs`（与缓存 store 隔离） |
| 写入 | 内存队列 + 后台批量（满 100 条或 1 秒），失败降级 `data/logs/backend-*.log` + stderr | 批量 50 条 / 1 秒，失败降级 console |
| 分类 | `system` `scan` `auth` `access` `business` `policy` `media` `error` `frontend` | `api` `player` `interaction` `performance` `exception` `auth` `app` |
| 容量 | 保留天数 + 最大条数，启动与每日 03:30 清理 | 5000 条上限，按时间淘汰 |
| 脱敏 | 密码 / Token / Cookie 一律 `[redacted]`，上下文白名单 | 同上，导出前再次脱敏 |

审计日志（`admin_operations_log`）在业务库中**独立保留**，不随运行日志清理。

## 安全说明

- 密码 bcrypt 哈希；JWT 密钥默认自动生成并写入 `data/jwt.secret`（首次启动创建，权限 600）
- 管理员账号由启动配置初始化，不存在「谁先注册谁是管理员」
- 媒体接口需鉴权；客户端只提交 `video_key`，实际路径来自扫描快照，避免路径穿越
- 管理员接口额外校验 `role=admin`，越权与写操作均留痕
- 局域网自签 HTTPS 即可；若要暴露公网，请走反向代理 + Let's Encrypt，不要直接开放端口

## 验证情况（v0.1.0）

- `go vet ./...` 无告警；`go test ./...`（media / policy / config）全部通过
- 用 3 个样本媒体（1 视频 2 图片）实测：注册登录、随机流、视频自动播放、图片 5 秒切换、
  滑动切页、收藏增删、仅看收藏、策略禁用与时间窗口拦截（`403` / 随机流过滤 / 收藏标记）、
  管理员绕过预览、后台六个页面渲染、日志分类检索、审计记录
- 浏览器（Edge headless）端到端脚本跑完整流程，控制台零报错；390×844 / 430×932 / 1440×900 三档布局无重叠
- 空闲内存 28MB（低于 N-01 的 50MB 目标）

## 文档

| 文档 | 内容 |
|------|------|
| [REQUIREMENTS.md](./REQUIREMENTS.md) | 原始需求文档 v3.1 |
| [docs/requirements/README.md](./docs/requirements/README.md) | 需求管理规范与索引 |
| [docs/requirements/versions/v0.1.0.md](./docs/requirements/versions/v0.1.0.md) | 本版范围与验收标准 |
| [docs/requirements/completed/v0.1.0.md](./docs/requirements/completed/v0.1.0.md) | 逐条需求完成情况（含实现位置与验证方式） |
| [docs/requirements/backlog/v0.1.0.md](./docs/requirements/backlog/v0.1.0.md) | 遗留项、技术债与下版规划 |
| [docs/requirements/changes/](./docs/requirements/changes/) | 需求变更记录（CR） |
| [deploy/README.md](./deploy/README.md) | 部署、HTTPS、资源限制与排障 |

## 路线图

- **v0.1.0（当前）**：Web 端完整可用，后台可管控，日志可诊断
- **v0.2.0（计划）**：服务端缩略图、文件监听替代轮询、登录限流、PC 端 Tauri 套壳
- **更远**：手机 / 电视端客户端、多实例部署与 PostgreSQL 适配、日志远程聚合

具体条目与前置条件见 [backlog](./docs/requirements/backlog/v0.1.0.md)。

## 许可

本项目以 **GNU General Public License v3.0** 开源，详见 [LICENSE](./LICENSE)。
