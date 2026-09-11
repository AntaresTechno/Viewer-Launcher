// Viewer Launcher hosts Viewer web assets and starts the upstream FastAPI
// backend from a self-contained Python dependency bundle.
package main

import (
	"image/color"
	"log"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// repository defaults to the official launcher repository. Forks can override
// it with -ldflags or VIEWER_LAUNCHER_REPOSITORY.
var repository = "AntaresTechno/Viewer-Launcher"

type launcherUI struct {
	launcher    *Launcher
	start       widget.Clickable
	stop        widget.Clickable
	open        widget.Clickable
	update      widget.Clickable
	saveNetwork widget.Clickable
	mirror      widget.Editor
	proxy       widget.Editor
}

func main() {
	go func() {
		window := new(app.Window)
		window.Option(
			app.Title("Viewer Launcher"),
			app.Size(unit.Dp(720), unit.Dp(760)),
		)
		if err := run(window); err != nil {
			log.Printf("launcher stopped: %v", err)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(window *app.Window) error {
	root, err := AppRoot()
	if err != nil {
		return err
	}
	ui := launcherUI{launcher: NewLauncher(root, repository, window.Invalidate)}
	settings := ui.launcher.DownloadSettings()
	ui.mirror.SingleLine = true
	ui.mirror.SetText(settings.Mirror)
	ui.proxy.SingleLine = true
	ui.proxy.Mask = '•'
	ui.proxy.SetText(settings.Proxy)
	th := material.NewTheme()
	th.Shaper = textShaper()
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			ui.launcher.Stop()
			return event.Err
		case app.FrameEvent:
			gtx := app.NewContext(new(op.Ops), event)
			ui.layout(gtx, th)
			event.Frame(gtx.Ops)
		}
	}
}

// textShaper keeps the binary independent of OS-installed fonts.
func textShaper() *text.Shaper {
	return text.NewShaper(text.WithCollection(gofont.Collection()))
}

func (ui *launcherUI) layout(gtx layout.Context, th *material.Theme) {
	for ui.start.Clicked(gtx) {
		go ui.launcher.EnsureAndStart(false)
	}
	for ui.stop.Clicked(gtx) {
		go ui.launcher.Stop()
	}
	for ui.update.Clicked(gtx) {
		go ui.launcher.EnsureAndStart(true)
	}
	for ui.open.Clicked(gtx) {
		go ui.launcher.OpenBrowser()
	}
	for ui.saveNetwork.Clicked(gtx) {
		_ = ui.launcher.ConfigureDownloads(ui.mirror.Text(), ui.proxy.Text())
	}

	state := ui.launcher.Snapshot()
	statusColor := color.NRGBA{R: 67, G: 160, B: 71, A: 255}
	if state.Err != "" {
		statusColor = color.NRGBA{R: 198, G: 40, B: 40, A: 255}
	} else if !state.Running {
		statusColor = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	}

	margin := layout.UniformInset(unit.Dp(24))
	margin.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(material.H4(th, "Viewer Launcher").Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Body1(th, state.Message)
				label.Color = statusColor
				return label.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(material.Body2(th, "程序目录："+state.Root).Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(material.Body2(th, "服务地址："+state.URL).Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return networkEditor(gtx, th, "GitHub 镜像前缀（留空直连）", "例：https://gh-proxy.example/ 或 https://example/{url}", &ui.mirror)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return networkEditor(gtx, th, "下载代理（留空使用系统代理）", "http://、https:// 或 socks5://", &ui.proxy)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(material.Button(th, &ui.saveNetwork, "保存镜像/代理设置").Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(24)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Rigid(material.Button(th, &ui.start, "启动").Layout),
					layout.Rigid(material.Button(th, &ui.open, "打开网页").Layout),
					layout.Rigid(material.Button(th, &ui.stop, "停止").Layout),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(material.Button(th, &ui.update, "更新 web、后端与 lib 后重启").Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				text := "首次登录请立即修改上游默认管理员密码。"
				if strings.TrimSpace(ui.launcher.Repository()) == "" {
					text = "未配置发布仓库：请用 -ldflags 或 VIEWER_LAUNCHER_REPOSITORY 配置。"
				}
				label := material.Caption(th, text)
				label.Color = color.NRGBA{R: 100, G: 100, B: 100, A: 255}
				return label.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(material.Caption(th, "许可：Viewer GPLv3+ · CPython PSF · Gio MIT；Python 依赖见 lib/licenses。").Layout),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(material.Body2(th, "日志（界面显示最近 8 行；完整日志在 logs/launcher.log）").Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := material.Caption(th, tailLines(state.Logs, 8))
				label.MaxLines = 8
				label.Color = color.NRGBA{R: 75, G: 75, B: 75, A: 255}
				return label.Layout(gtx)
			}),
		)
	})
}

func networkEditor(gtx layout.Context, th *material.Theme, label, hint string, editor *widget.Editor) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(material.Caption(th, label).Layout),
		layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			border := widget.Border{Color: color.NRGBA{R: 170, G: 170, B: 170, A: 255}, CornerRadius: unit.Dp(4), Width: unit.Dp(1)}
			return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, material.Editor(th, editor, hint).Layout)
			})
		}),
	)
}

func tailLines(value string, limit int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n")
}
