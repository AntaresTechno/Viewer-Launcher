# Viewer Launcher  

一个 Windows、Linux 的 Go + Gio 原生启动器。它以 Go HTTP 服务承载 Viewer 前端，反向代理 API 到隔离的 CPython 后端。
浏览器和 Python 均只绑定本机回环地址；程序、SQLite 数据、设置和日志统一位于启动器同级的 `./program`，数据不会随组件更新被覆盖。开发时可通过 `VIEWER_LAUNCHER_ROOT` 覆盖该目录。

## 预发布结构

推送 `main` 或手动运行 `Build preview release` 后，单一工作流会固定一个 Viewer 上游提交，同时构建所有组件，并上传到标签为 `preview` 的滚动 GitHub Pre-release。发布说明由工作流自动生成，使用 Markdown 表格列出每个文件的用途、平台、字节数和 SHA-256。

| 文件 | 作用 |
| --- | --- |
| `release-manifest.json` | 启动器读取的机器清单，包含提交号、平台、文件大小和 SHA-256 |
| `viewer-web.tar.gz` | 已编译的 Viewer 前端及版本元数据 |
| `viewer-backend.tar.gz` | 与前端同一提交的 FastAPI 后端源码 |
| `viewer-runtime-<platform>.tar.gz` | 对应平台的 CPython、Python 包和许可证 |
| `viewer-launchers-windows-amd64.zip` | Windows x64 的 Gio 桌面版与 CLI 启动器 |
| `viewer-launchers-linux-<arch>.tar.gz` | Linux 对应架构的 Gio 桌面版与 CLI 启动器 |

安装逻辑为：读取 `preview/release-manifest.json` → 选择 web、backend 和当前平台 runtime → 逐个下载并校验声明的大小和 SHA-256 → 安全解压到临时目录 → 全部成功后以目录重命名一次性切换三项组件 → 核验三份 `release.json` 的上游提交。任何下载、校验或解压失败都不会替换现有组件，`data`、设置和日志不会参与更新。

归档解压拒绝绝对路径、`..`、越界硬链接和不安全符号链接；Windows 拒绝归档中的符号链接，Linux 仅允许指向安装目录内部的相对符号链接。

启动逻辑为：嵌入式 Python 在 `127.0.0.1:18081` 运行 Uvicorn；Go 在 `127.0.0.1:18080` 提供 `web` 中的 `dist` 文件，将 `/api` 与 `/dav` 反向代理至 Python，并负责 SPA 回退。窗口显示 Go、代理、Python/Uvicorn 日志最近 8 行，完整日志保存在 `./program/logs/launcher.log`。

## 下载镜像与代理

启动器窗口可设置 GitHub 镜像前缀和下载代理，配置保存在 `./program/settings.json`：

- 镜像前缀留空时直连 GitHub；填写 `https://mirror.example/` 时，下载地址会改写为 `https://mirror.example/https://原始地址`。也可使用 `https://mirror.example/{url}` 模板。
- 显式代理支持 `http://`、`https://` 和 `socks5://`。留空时仍遵循系统的 `HTTP_PROXY`、`HTTPS_PROXY` 和 `NO_PROXY` 环境变量。
- 可用 `VIEWER_LAUNCHER_MIRROR`、`VIEWER_LAUNCHER_PROXY` 临时覆盖配置文件。代理账号不会写入日志，但包含口令的代理 URL 会保存在本地设置文件中。

## 初次配置

1. 将此仓库推送到 GitHub，并在仓库 Settings → Actions → General 中允许 workflow 对 `GITHUB_TOKEN` 读写。
2. 推送 `main` 后，`Build preview release` 会创建或更新 `preview` 预发布。无需再维护生成用的 `web`、`lib` 分支。
3. 若需固定特定 Viewer 版本，可手动运行工作流并填写 tag、branch 或完整 commit SHA。该提交会同时用于 web、backend 和三个平台的 Python 依赖构建。
4. Windows 下运行项目根目录的构建脚本。它会依次格式化源码、运行 GUI/CLI 两套测试，并在 `dist` 目录生成两个版本：

```cmd
build.cmd
```

| 产物 | 用途 |
| --- | --- |
| `dist/viewer-launcher.exe` | Gio 桌面界面，适合日常使用 |
| `dist/viewer-launcher-cli.exe` | 无界面终端版本，适合脚本、服务器与故障排查 |

也可以直接使用 Go 构建：

```powershell
go mod tidy
go build -o viewer-launcher.exe .
go build -tags cli -o viewer-launcher-cli.exe .
```

fork 仓库仍可通过 `-ldflags "-X main.repository=OWNER/REPOSITORY"` 或环境变量 `VIEWER_LAUNCHER_REPOSITORY=OWNER/REPOSITORY` 覆盖默认值。发布工作流会自动注入当前仓库名。

支持 `windows/amd64`、`linux/amd64` 与 `linux/arm64`。首次运行后，请立即修改 Viewer 的默认管理员密码。启动器刻意将 Uvicorn 绑定到回环地址，请不要改为局域网暴露。

## CLI 使用

CLI 与桌面版共用同一套配置、安装校验和服务启动逻辑。`start` 与 `update` 会以前台方式运行，按 `Ctrl+C` 可安全停止服务：

```powershell
dist\viewer-launcher-cli.exe status
dist\viewer-launcher-cli.exe start
dist\viewer-launcher-cli.exe update
dist\viewer-launcher-cli.exe config --show
dist\viewer-launcher-cli.exe config --mirror "https://gh-proxy.example/"
dist\viewer-launcher-cli.exe config --proxy "socks5://127.0.0.1:1080"
```

默认数据目录是可执行文件旁的 `program`。也可在命令前使用 `--root <目录>` 或 `--repository <OWNER/REPOSITORY>` 覆盖，例如：

```powershell
dist\viewer-launcher-cli.exe --root D:\ViewerData status
```

## 许可证与供应链

上游 Viewer 是 **GPL-3.0-or-later**；本启动器及其发布组合也应以 GPL-3.0-or-later 传递。工作流会：

- 将上游 LICENSE 和被构建的 commit 写入发布物；
- 固定 CPython 版本，保留其许可证；
- Windows 下载目标平台 wheels；Linux 在对应架构的 manylinux_2_28 环境中构建 wheels，再离线安装；
- 解析每个 wheel 的 `METADATA`，把许可证清单和能找到的许可证文件放入运行时；
- 不对 Python 依赖做许可证策略校验；缺少机器可读许可证字段只会在清单中显示为空，不会阻断构建。

许可证清单用于声明与人工复核，不是法律意见。发布前仍应由项目维护者复核报告与上游许可义务。
启动器自身的 Gio 依赖声明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
