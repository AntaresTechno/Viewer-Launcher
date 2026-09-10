# Viewer Launcher

一个 Windows x64 的 Go + Gio 原生启动器。它将 [AntaresTechno/Viewer](https://github.com/AntaresTechno/Viewer)
的 FastAPI 后端运行在隔离的 CPython 3.13 环境中，并从本仓库的发布分支取用预构建前端。
浏览器只访问本机的 `127.0.0.1:18080`；SQLite 数据位于用户配置目录，不会随运行时更新被覆盖。

## 发布结构

| 分支 | 工作流 | 内容 |
| --- | --- | --- |
| `frontend-dist` | `Build Viewer frontend` | 上游 `frontend/dist`、上游提交号和 GPL 文本 |
| `runtime-windows-amd64` | `Build Viewer runtime` | CPython 嵌入式运行时、后端、Windows x64 wheels、依赖许可证报告 |

启动器只接受这两个分支归档的预期目录布局，解压时拒绝绝对路径与 `..` 路径。更新以暂存目录下载并以目录重命名切换，失败不会破坏已安装版本。

## 初次配置

1. 将此仓库推送到 GitHub，并在仓库 Settings → Actions → General 中允许 workflow 对 `GITHUB_TOKEN` 读写。
2. 确认 `frontend-dist` 和 `runtime-windows-amd64` 没有分支保护规则阻止 Actions 推送。
3. 手动运行两个构建工作流；先取得上游的完整 commit SHA，并把同一个 SHA 填入两次 dispatch 表单。
   启动器会读取两侧元数据并拒绝不匹配的组合。
4. 构建启动器时嵌入你的 GitHub 仓库名：

```powershell
go mod tidy
go build -ldflags "-X main.repository=OWNER/REPOSITORY" -o ViewerLauncher.exe .
```

也可以仅为测试设置 `VIEWER_LAUNCHER_REPOSITORY=OWNER/REPOSITORY`。发布工作流会自动注入该值。

首次运行后，请立即修改 Viewer 的默认管理员密码。启动器刻意将 Uvicorn 绑定到回环地址，请不要改为局域网暴露。

## 许可证与供应链

上游 Viewer 是 **GPL-3.0-or-later**；本启动器及其发布组合也应以 GPL-3.0-or-later 传递。工作流会：

- 将上游 LICENSE 和被构建的 commit 写入发布物；
- 固定 CPython 版本，保留其许可证；
- 用 `pip download --only-binary=:all:` 先解析 Windows wheels，再离线安装；
- 解析每个 wheel 的 `METADATA`，把许可证报告和能找到的许可证文件放入运行时；
- 对未知、GPL 或 AGPL Python 依赖失败。新增依赖须先审阅并更新 `licenses-policy.json`。

这是一项工程上的许可证门禁，不是法律意见。发布前仍应由项目维护者复核报告与上游许可义务。
启动器自身的 Gio 依赖声明见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
