# Viewer Launcher

一个 Windows、Linux、macOS 的 Go + Gio 原生启动器。它以 Go HTTP 服务承载 Viewer 前端，反向代理 API 到隔离的 CPython 后端。
浏览器和 Python 均只绑定本机回环地址；SQLite 数据位于用户配置目录，不会随更新被覆盖。

## 发布结构

| 分支 | 工作流 | 内容 |
| --- | --- | --- |
| `web` | `Build Viewer web branch` | 上游编译后的 `dist`、上游提交号和 GPL 文本 |
| `lib` | `Build Viewer lib dependency branch`、`Build Viewer lib for Linux and macOS` | 默认最新 CPython 3.12 嵌入式包、平台 wheels、许可证报告 |

安装逻辑为：读取 `lib` 的提交清单 → 拉取 `web` → 从 Viewer `main` 当时对应的不可变 commit 拉取 `backend` → 拉取 `lib` → 核验前端、后端、Python、依赖许可证报告及三个提交号。归档解压拒绝绝对路径、`..` 和不安全符号链接；Linux/macOS 仅允许指向安装目录内的相对符号链接，以保留 CPython 运行时布局。更新以暂存目录下载并以目录重命名切换。

启动逻辑为：嵌入式 Python 在 `127.0.0.1:18081` 运行 Uvicorn；Go 在 `127.0.0.1:18080` 提供 `web` 中的 `dist` 文件，将 `/api` 与 `/dav` 反向代理至 Python，并负责 SPA 回退。窗口显示 Go、代理、Python/Uvicorn 日志最近 8 行，完整日志保存在 `logs/launcher.log`。

## 初次配置

1. 将此仓库推送到 GitHub，并在仓库 Settings → Actions → General 中允许 workflow 对 `GITHUB_TOKEN` 读写。
2. 确认 `web` 和 `lib` 没有分支保护规则阻止 Actions 推送。
3. 除自动生成的 `web`、`lib` 分支外，每次 push 都会自动运行 web、Windows lib、Linux/macOS lib 和启动器构建。两个写入 `lib` 分支的工作流由共享并发锁串行执行，不会覆盖彼此的平台目录。若需固定某个 Viewer 版本，可手动 dispatch 并填入上游 commit SHA；web 与各 lib 发布应使用同一 SHA。
   启动器会读取两侧元数据并拒绝不匹配的组合。
4. 构建启动器时嵌入你的 GitHub 仓库名：

```powershell
go mod tidy
go build -ldflags "-X main.repository=OWNER/REPOSITORY" -o ViewerLauncher .
```

也可以仅为测试设置 `VIEWER_LAUNCHER_REPOSITORY=OWNER/REPOSITORY`。发布工作流会自动注入该值。

支持 `windows/amd64`、`linux/amd64`、`linux/arm64`、`darwin/amd64` 与 `darwin/arm64`。首次运行后，请立即修改 Viewer 的默认管理员密码。启动器刻意将 Uvicorn 绑定到回环地址，请不要改为局域网暴露。

## 许可证与供应链

上游 Viewer 是 **GPL-3.0-or-later**；本启动器及其发布组合也应以 GPL-3.0-or-later 传递。工作流会：

- 将上游 LICENSE 和被构建的 commit 写入发布物；
- 固定 CPython 版本，保留其许可证；
- 用 `pip download --only-binary=:all:` 先解析目标平台 wheels，再离线安装；
- 解析每个 wheel 的 `METADATA`，把许可证报告和能找到的许可证文件放入运行时；
- 对未知、GPL 或 AGPL Python 依赖失败。新增依赖须先审阅并更新 `licenses-policy.json`。

这是一项工程上的许可证门禁，不是法律意见。发布前仍应由项目维护者复核报告与上游许可义务。
启动器自身的 Gio 依赖声明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
