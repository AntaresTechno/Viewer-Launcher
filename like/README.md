<p align="center">
  <img src="1080.png" alt="Viewer" width="360"/>
</p>

<div align="center">

# Antares · Viewer

**一个自托管、可自扩展的网络阅读与媒体源应用**

[![Vue 3](https://img.shields.io/badge/Vue%203-4FC08D?style=flat-square&logo=vue.js&logoColor=white)](https://vuejs.org/)
[![FastAPI](https://img.shields.io/badge/FastAPI-009688?style=flat-square&logo=fastapi&logoColor=white)](https://fastapi.tiangolo.com/)
[![Python](https://img.shields.io/badge/Python-3776AB?style=flat-square&logo=python&logoColor=white)](https://www.python.org/)
[![SQLite](https://img.shields.io/badge/SQLite-003B57?style=flat-square&logo=sqlite&logoColor=white)](https://www.sqlite.org/)
[![License MIT](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](#许可证)

</div>

Viewer 是一个面向网络阅读与媒体源的自托管 Web 应用。前端使用 Vue 3，后端使用 FastAPI；书源、订阅、媒体、WebDAV、协议处理和规则引擎通过组件注册表装配。

## 主要能力

- 导入与管理 Legado 兼容来源；混合来源文件会自动分流到书源、订阅源、漫画源、音频源或视频源。
- 维护个人书架、阅读进度、本地章节缓存与阅读统计。
- 导入 RSS、Atom 和 Legado 订阅源，浏览文章并管理已读与收藏。
- 管理漫画、音频和视频媒体源与媒体库。
- 使用规则包净化正文，并缓存净化结果。
- 将书架和进度备份到 WebDAV，或向 Legado 提供 `/dav` 同步端点。
- 通过独立的协议桥和协议适配插件处理 `legado://`、`yuedu://` 等系统链接，并在确认后分类导入。
- 安装、启停 ZIP 插件；插件可自带隔离运行的管理界面。
- 使用用户、权限组和插件权限控制功能访问。

## 快速开始

Windows 推荐安装 [uv](https://docs.astral.sh/uv/) 和 Node.js 后直接运行：

```powershell
.\start.bat
```

首次启动会创建 SQLite 数据库和管理员账号：

```text
用户名：admin
密码：view123456
```

登录后请立即修改密码。生产环境还必须固定 `VIEWER_SECRET_KEY`，否则进程重启后已有登录令牌会失效。

开发模式：

```powershell
.\start.bat dev
```

- 生产入口：`http://127.0.0.1:8000`
- 开发前端：`http://127.0.0.1:5173`
- 后端接口文档：`http://127.0.0.1:8000/docs`

macOS、Linux、手动部署和环境变量配置见[安装与启动](docs/getting-started.md)。

## 项目结构

```text
viewer/
├── backend/                 FastAPI、数据库、插件和测试
│   └── app/plugins/         内置组件与外部 ZIP 插件安装位置
├── frontend/                Vue 3 单页应用
├── docs/                    项目文档
├── build.bat                Windows 构建脚本
└── start.bat                Windows 启动脚本
```

## 文档

完整目录见 [docs/README.md](docs/README.md)。

- [安装与启动](docs/getting-started.md)
- [系统架构](docs/architecture.md)
- [配置参考](docs/configuration.md)
- [插件开发](docs/plugins.md)
- [协议桥与 Legado 协议](docs/protocols.md)
- [书源、订阅源与规则引擎](docs/sources-and-engines.md)
- [API、认证与权限](docs/api-and-permissions.md)
- [构建、测试与运维](docs/operations.md)

## 开发验证

```powershell
cd backend
.\.venv\Scripts\python.exe -m pytest -q

cd ..\frontend
npm run build
```

## 许可证

本项目按根目录 [LICENSE](LICENSE) 中的条款发布。
