# LanReel

局域网随机视频流媒体系统：部署在 Windows / Linux（含 NAS）主机上，通过浏览器或客户端以抖音式竖屏滑动方式浏览指定文件夹中的视频与图片，支持用户注册、跨设备收藏同步与管理后台。

> **当前状态：需求阶段。** 工程代码尚未创建，本仓库先托管需求文档与项目约定。完整需求见 [REQUIREMENTS.md](./REQUIREMENTS.md)（v3.1）。

## 特性概览

- 随机、流畅的竖屏滑动浏览（视频 / 图片），支持 HTTP Range 拖拽播放
- 用户注册 / 登录（JWT）、收藏夹跨设备同步、IndexedDB 离线缓存
- 管理员后台：文件夹策略、文件级覆盖、时间窗口、用户与审计管理
- 服务端低资源占用（Go 单二进制、`GOMEMLIMIT` / `GOMAXPROCS`），不转码
- 多端预留：Web / Tauri PC，未来手机 App 与电视端
- 前后端分类日志，分别落本地日志库（后端 SQLite `logs.db`、前端 IndexedDB `app_logs`）

## 技术栈（规划）

| 层 | 选型 |
|----|------|
| 后端 | Go + Gin + SQLite（`modernc.org/sqlite`） |
| 前端 | Vue 3 + Vite + Tailwind，后台 Naive UI |
| PC 端 | Tauri |
| 缓存 / 日志 | 前端 IndexedDB；后端独立 SQLite 日志库 |

## 仓库结构（规划）

```
.
├── REQUIREMENTS.md   # 需求文档 v3.1（当前唯一交付文件）
├── LICENSE           # GPL-3.0 许可证
└── README.md         # 本文件
```

## 许可

本项目以 **GNU General Public License v3.0** 开源，详见 [LICENSE](./LICENSE)。
