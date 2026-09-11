# Viewer Launcher

一个 Windows、Linux 的 Go + Gio 原生启动器。它以 Go HTTP 服务承载 Viewer 前端，反向代理 API 到隔离的 CPython 后端。
浏览器和 Python 均只绑定本机回环地址；程序、SQLite 数据、设置和日志统一位于启动器同级的 `./program`，数据不会随组件更新被覆盖。开发时可通过 `VIEWER_LAUNCHER_ROOT` 覆盖该目录。

## 发布结构

| 分支 | 工作流 | 内容 |
| --- | --- | --- |
| `web` | `Build Viewer web branch` | 上游编译后的 `dist`、上游提交号和 GPL 文本 |
| `lib` | `Build Viewer lib` | 默认最新 CPython 3.12 嵌入式包、平台 wheels、许可证清单，以及每个平台运行时的压缩分片 |

安装逻辑为：读取 `lib` 的提交清单 → 拉取 `web` → 从 Viewer `main` 当时对应的不可变 commit 拉取 `backend` → 下载当前平台 `lib` 的压缩分片并逐片校验 SHA-256 → 合并解压运行时 → 核验前端、后端、Python 及三个提交号。归档解压拒绝绝对路径、`..` 和不安全符号链接；Linux 仅允许指向安装目录内的相对符号链接，以保留 CPython 运行时布局。更新以暂存目录下载并以目录重命名切换。

启动逻辑为：嵌入式 Python 在 `127.0.0.1:18081` 运行 Uvicorn；Go 在 `127.0.0.1:18080` 提供 `web` 中的 `dist` 文件，将 `/api` 与 `/dav` 反向代理至 Python，并负责 SPA 回退。窗口显示 Go、代理、Python/Uvicorn 日志最近 8 行，完整日志保存在 `./program/logs/launcher.log`。

## 下载镜像与代理

启动器窗口可设置 GitHub 镜像前缀和下载代理，配置保存在 `./program/settings.json`：

- 镜像前缀留空时直连 GitHub；填写 `https://mirror.example/` 时，下载地址会改写为 `https://mirror.example/https://原始地址`。也可使用 `https://mirror.example/{url}` 模板。
- 显式代理支持 `http://`、`https://` 和 `socks5://`。留空时仍遵循系统的 `HTTP_PROXY`、`HTTPS_PROXY` 和 `NO_PROXY` 环境变量。
- 可用 `VIEWER_LAUNCHER_MIRROR`、`VIEWER_LAUNCHER_PROXY` 临时覆盖配置文件。代理账号不会写入日志，但包含口令的代理 URL 会保存在本地设置文件中。

## 初次配置

1. 将此仓库推送到 GitHub，并在仓库 Settings → Actions → General 中允许 workflow 对 `GITHUB_TOKEN` 读写。
2. 确认 `web` 和 `lib` 没有分支保护规则阻止 Actions 推送。
3. 除自动生成的 `web`、`lib` 分支外，每次 push 都会自动运行 web、`Build Viewer lib` 和启动器构建。`Build Viewer lib` 先固定一个上游 commit，再并行构建 Windows 与 Linux 依赖，最后由一个发布任务写入 `lib` 分支，因此不会产生分支写入竞争。若需固定某个 Viewer 版本，可手动 dispatch 并填入上游 commit SHA。启动器会读取两侧元数据并拒绝不匹配的组合。
4. 直接构建即可使用默认发布仓库 `AntaresTechno/Viewer-Launcher`：

```powershell
go mod tidy
go build -o ViewerLauncher .
```

fork 仓库仍可通过 `-ldflags "-X main.repository=OWNER/REPOSITORY"` 或环境变量 `VIEWER_LAUNCHER_REPOSITORY=OWNER/REPOSITORY` 覆盖默认值。发布工作流会自动注入当前仓库名。

支持 `windows/amd64`、`linux/amd64` 与 `linux/arm64`。首次运行后，请立即修改 Viewer 的默认管理员密码。启动器刻意将 Uvicorn 绑定到回环地址，请不要改为局域网暴露。

## 许可证与供应链

上游 Viewer 是 **GPL-3.0-or-later**；本启动器及其发布组合也应以 GPL-3.0-or-later 传递。工作流会：

- 将上游 LICENSE 和被构建的 commit 写入发布物；
- 固定 CPython 版本，保留其许可证；
- Windows 下载目标平台 wheels；Linux 在对应架构的 manylinux2014 环境中补充构建缺失的 wheels，再离线安装；
- 解析每个 wheel 的 `METADATA`，把许可证清单和能找到的许可证文件放入运行时；
- 不对 Python 依赖做许可证策略校验；缺少机器可读许可证字段只会在清单中显示为空，不会阻断构建。

许可证清单用于声明与人工复核，不是法律意见。发布前仍应由项目维护者复核报告与上游许可义务。
启动器自身的 Gio 依赖声明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
