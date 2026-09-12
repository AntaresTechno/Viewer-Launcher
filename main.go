// Viewer Launcher hosts Viewer web assets and starts the upstream FastAPI
// backend from a self-contained Python dependency bundle.
package main

import (
	"image"
	"image/color"
	"log"
	"os"
	"strings"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// repository defaults to the official launcher repository. Forks can override
// it with -ldflags or VIEWER_LAUNCHER_REPOSITORY.
var repository = "AntaresTechno/Viewer-Launcher"

var uiColors = struct {
	canvas, surface, surfaceAlt, border, text, muted color.NRGBA
	primary, primarySoft, success, successSoft       color.NRGBA
	warning, warningSoft, danger, dangerSoft, log    color.NRGBA
}{
	canvas:      rgb(246, 247, 251),
	surface:     rgb(255, 255, 255),
	surfaceAlt:  rgb(249, 250, 251),
	border:      rgb(228, 231, 236),
	text:        rgb(29, 41, 57),
	muted:       rgb(102, 112, 133),
	primary:     rgb(79, 70, 229),
	primarySoft: rgb(238, 242, 255),
	success:     rgb(18, 183, 106),
	successSoft: rgb(236, 253, 243),
	warning:     rgb(247, 144, 9),
	warningSoft: rgb(255, 250, 235),
	danger:      rgb(217, 45, 32),
	dangerSoft:  rgb(254, 243, 242),
	log:         rgb(24, 34, 48),
}

var (
	playIcon     = mustIcon(icons.AVPlayArrow)
	launchIcon   = mustIcon(icons.ActionLaunch)
	downloadIcon = mustIcon(icons.FileCloudDownload)
	refreshIcon  = mustIcon(icons.NavigationRefresh)
	stopIcon     = mustIcon(icons.AVStop)
)

type launcherUI struct {
	launcher *Launcher
	list     widget.List

	primary, stop, update, saveNetwork widget.Clickable
	guideOpen, guidePrimary            widget.Clickable
	guideBack, guideSkip               widget.Clickable
	networkToggle, logsToggle          widget.Clickable

	mirror, proxy widget.Editor

	showGuide       bool
	guideStep       int
	networkExpanded bool
	logsExpanded    bool
}

func main() {
	go func() {
		window := new(app.Window)
		window.Option(
			app.Title("Viewer Launcher"),
			app.Size(unit.Dp(900), unit.Dp(780)),
			app.MinSize(unit.Dp(560), unit.Dp(640)),
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
	launcher := NewLauncher(root, repository, window.Invalidate)
	settings := launcher.DownloadSettings()
	ui := launcherUI{
		launcher:  launcher,
		list:      widget.List{List: layout.List{Axis: layout.Vertical}},
		showGuide: !settings.GuideComplete,
	}
	ui.mirror.SingleLine = true
	ui.mirror.SetText(settings.Mirror)
	ui.proxy.SingleLine = true
	ui.proxy.SetText(settings.Proxy)

	th := material.NewTheme()
	th.Shaper = textShaper()
	th.Palette = material.Palette{Bg: uiColors.canvas, Fg: uiColors.text, ContrastBg: uiColors.primary, ContrastFg: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}
	th.TextSize = unit.Sp(16)
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

// System fonts remain enabled so Chinese text uses the platform's native font;
// Go fonts provide a portable Latin fallback.
func textShaper() *text.Shaper {
	return text.NewShaper(text.WithCollection(gofont.Collection()))
}

func (ui *launcherUI) layout(gtx layout.Context, th *material.Theme) {
	state := ui.launcher.Snapshot()
	ui.handleEvents(gtx, state)
	state = ui.launcher.Snapshot()

	paint.Fill(gtx.Ops, uiColors.canvas)
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.header(gtx, th, state) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return ui.content(gtx, th, state) }),
	)
}

func (ui *launcherUI) handleEvents(gtx layout.Context, state State) {
	for ui.primary.Clicked(gtx) {
		if state.Running {
			go ui.launcher.OpenBrowser()
		} else {
			go ui.launcher.EnsureAndStart(false)
		}
	}
	for ui.stop.Clicked(gtx) {
		go ui.launcher.Stop()
	}
	for ui.update.Clicked(gtx) {
		go ui.launcher.EnsureAndStart(true)
	}
	for ui.saveNetwork.Clicked(gtx) {
		_ = ui.launcher.ConfigureDownloads(ui.mirror.Text(), ui.proxy.Text())
	}
	for ui.guideOpen.Clicked(gtx) {
		ui.showGuide = true
		ui.guideStep = 0
	}
	for ui.guideBack.Clicked(gtx) {
		if ui.guideStep > 0 {
			ui.guideStep--
		}
	}
	for ui.guidePrimary.Clicked(gtx) {
		if ui.guideStep < 2 {
			ui.guideStep++
			if ui.guideStep == 1 {
				ui.networkExpanded = true
			}
		} else {
			if ui.launcher.CompleteGuide() == nil {
				ui.showGuide = false
			}
		}
	}
	for ui.guideSkip.Clicked(gtx) {
		if ui.launcher.CompleteGuide() == nil {
			ui.showGuide = false
		}
	}
	for ui.networkToggle.Clicked(gtx) {
		ui.networkExpanded = !ui.networkExpanded
	}
	for ui.logsToggle.Clicked(gtx) {
		ui.logsExpanded = !ui.logsExpanded
	}
}

func (ui *launcherUI) header(gtx layout.Context, th *material.Theme, state State) layout.Dimensions {
	return surface(gtx, uiColors.surface, color.NRGBA{}, 0, layout.Inset{Top: 14, Bottom: 14, Left: 22, Right: 22}, func(gtx layout.Context) layout.Dimensions {
		return contentWidth(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return logo(gtx, th) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return label(gtx, th, "Viewer Launcher", 18, uiColors.text, font.SemiBold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return label(gtx, th, headerSubtitle(state), 12, uiColors.muted, font.Normal)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return compactButton(gtx, th, &ui.guideOpen, "使用指南", uiColors.primarySoft, uiColors.primary)
				}),
			)
		})
	})
}

func (ui *launcherUI) content(gtx layout.Context, th *material.Theme, state State) layout.Dimensions {
	items := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions { return ui.statusCard(gtx, th, state) },
	}
	if ui.showGuide {
		items = append(items, func(gtx layout.Context) layout.Dimensions { return ui.guideCard(gtx, th) })
	}
	items = append(items,
		func(gtx layout.Context) layout.Dimensions { return ui.controlCard(gtx, th, state) },
		func(gtx layout.Context) layout.Dimensions { return ui.networkCard(gtx, th) },
		func(gtx layout.Context) layout.Dimensions { return ui.securityCard(gtx, th) },
		func(gtx layout.Context) layout.Dimensions { return ui.logsCard(gtx, th, state) },
		func(gtx layout.Context) layout.Dimensions { return footer(gtx, th) },
	)
	listStyle := material.List(th, &ui.list)
	listStyle.AnchorStrategy = material.Overlay
	return listStyle.Layout(gtx, len(items), func(gtx layout.Context, index int) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(10), Bottom: unit.Dp(6), Left: unit.Dp(22), Right: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return contentWidth(gtx, items[index])
		})
	})
}

func (ui *launcherUI) statusCard(gtx layout.Context, th *material.Theme, state State) layout.Dimensions {
	status, statusColor, statusSoft := statusPresentation(state)
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return label(gtx, th, "服务状态", 13, uiColors.muted, font.Medium)
							}),
							layout.Rigid(layout.Spacer{Height: unit.Dp(5)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return label(gtx, th, state.Message, 22, uiColors.text, font.SemiBold)
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return statusPill(gtx, th, status, statusColor, statusSoft)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return infoBlock(gtx, th, "访问地址", state.URL) }),
					layout.Rigid(layout.Spacer{Width: unit.Dp(18)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return infoBlock(gtx, th, "程序数据", compactPath(state.Root))
					}),
				)
			}),
		)
	})
}

func (ui *launcherUI) guideCard(gtx layout.Context, th *material.Theme) layout.Dimensions {
	titles := []string{"欢迎使用 Viewer", "检查下载连接", "启动并完成安全设置"}
	bodies := []string{
		"启动器会把程序、数据库、设置和日志统一保存在本地 program 目录。更新组件时不会覆盖你的数据。",
		"默认会直连 GitHub。若下载较慢，可在下方展开的“下载与网络”中填写镜像或代理；网络正常时无需配置。",
		"点击“启动 Viewer”，首次准备会自动下载所需组件并打开浏览器。登录后请立即修改默认管理员密码。",
	}
	return surface(gtx, uiColors.primarySoft, rgb(199, 210, 254), 14, layout.UniformInset(unit.Dp(22)), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return label(gtx, th, "快速上手", 12, uiColors.primary, font.SemiBold)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return guideProgress(gtx, ui.guideStep) }),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return label(gtx, th, titles[ui.guideStep], 20, uiColors.text, font.SemiBold)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(7)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Body2(th, bodies[ui.guideStep])
				l.Color = uiColors.muted
				l.LineHeight = unit.Sp(21)
				return l.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return textButton(gtx, th, &ui.guideSkip, "跳过", uiColors.muted)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{} }),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ui.guideStep == 0 {
							return layout.Dimensions{}
						}
						return textButton(gtx, th, &ui.guideBack, "上一步", uiColors.primary)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						caption := "下一步"
						if ui.guideStep == 2 {
							caption = "完成引导"
						}
						return compactButton(gtx, th, &ui.guidePrimary, caption, uiColors.primary, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
					}),
				)
			}),
		)
	})
}

func (ui *launcherUI) controlCard(gtx layout.Context, th *material.Theme, state State) layout.Dimensions {
	primaryText, primaryHint := "启动 Viewer", "自动检查并准备所需组件"
	primaryIcon := playIcon
	if state.Running {
		primaryText, primaryHint = "打开 Viewer", "服务已就绪，在浏览器中继续"
		primaryIcon = launchIcon
	} else if state.Busy {
		primaryText, primaryHint = "正在准备…", "请稍候，可在下方查看实时日志"
		primaryIcon = downloadIcon
	}
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return sectionTitle(gtx, th, "控制中心", "启动、更新或停止本地服务")
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				buttonGTX := gtx
				if state.Busy {
					buttonGTX = gtx.Disabled()
				}
				return primaryAction(buttonGTX, th, &ui.primary, primaryIcon, primaryText, primaryHint)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Spacing: layout.SpaceBetween}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						buttonGTX := gtx
						if state.Busy {
							buttonGTX = gtx.Disabled()
						}
						return secondaryAction(buttonGTX, th, &ui.update, refreshIcon, "检查并更新", "更新全部组件后重启")
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						buttonGTX := gtx
						if !state.Running || state.Busy {
							buttonGTX = gtx.Disabled()
						}
						return secondaryAction(buttonGTX, th, &ui.stop, stopIcon, "停止服务", "安全关闭本地 Viewer")
					}),
				)
			}),
		)
	})
}

func (ui *launcherUI) networkCard(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return sectionTitle(gtx, th, "下载与网络", "可选设置，网络正常时保持为空")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						caption := "展开"
						if ui.networkExpanded {
							caption = "收起"
						}
						return compactButton(gtx, th, &ui.networkToggle, caption, uiColors.surfaceAlt, uiColors.primary)
					}),
				)
			}),
		}
		if ui.networkExpanded {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(18)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return networkEditor(gtx, th, "GitHub 镜像前缀", "留空直连；例如 https://gh-proxy.example/", &ui.mirror)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return networkEditor(gtx, th, "下载代理", "支持 http://、https:// 或 socks5://", &ui.proxy)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return compactButton(gtx, th, &ui.saveNetwork, "保存网络设置", uiColors.primary, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				}),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (ui *launcherUI) securityCard(gtx layout.Context, th *material.Theme) layout.Dimensions {
	message := "首次登录后，请立即修改上游默认管理员密码。服务仅监听本机地址，不会暴露到局域网。"
	if strings.TrimSpace(ui.launcher.Repository()) == "" {
		message = "未配置发布仓库：请通过 -ldflags 或 VIEWER_LAUNCHER_REPOSITORY 设置后再启动。"
	}
	return surface(gtx, uiColors.warningSoft, rgb(254, 223, 137), 12, layout.UniformInset(unit.Dp(18)), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return statusDot(gtx, uiColors.warning, 9) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return label(gtx, th, "安全提醒", 13, rgb(147, 55, 13), font.SemiBold)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return label(gtx, th, message, 13, rgb(181, 71, 8), font.Normal)
					}),
				)
			}),
		)
	})
}

func (ui *launcherUI) logsCard(gtx layout.Context, th *material.Theme, state State) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return sectionTitle(gtx, th, "运行日志", "完整日志保存在 program/logs/launcher.log")
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						caption := "查看"
						if ui.logsExpanded {
							caption = "收起"
						}
						return compactButton(gtx, th, &ui.logsToggle, caption, uiColors.surfaceAlt, uiColors.primary)
					}),
				)
			}),
		}
		if ui.logsExpanded {
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return logPanel(gtx, th, tailLines(state.Logs, 10)) }),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func footer(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(22)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		l := material.Caption(th, "Viewer GPLv3+  ·  CPython PSF  ·  Gio MIT  ·  Python 依赖许可见 lib/licenses")
		l.Color = uiColors.muted
		l.Alignment = text.Middle
		return l.Layout(gtx)
	})
}

func card(gtx layout.Context, content layout.Widget) layout.Dimensions {
	return surface(gtx, uiColors.surface, uiColors.border, 14, layout.UniformInset(unit.Dp(22)), content)
}

func surface(gtx layout.Context, background, border color.NRGBA, radius unit.Dp, inset layout.Inset, content layout.Widget) layout.Dimensions {
	borderWidget := widget.Border{Color: border, CornerRadius: radius, Width: unit.Dp(1)}
	if border.A == 0 {
		borderWidget.Width = 0
	}
	return borderWidget.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Background{}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				r := gtx.Dp(radius)
				paint.FillShape(gtx.Ops, background, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, r).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Min}
			},
			func(gtx layout.Context) layout.Dimensions { return inset.Layout(gtx, content) },
		)
	})
}

func logo(gtx layout.Context, th *material.Theme) layout.Dimensions {
	gtx.Constraints.Min = image.Pt(gtx.Dp(38), gtx.Dp(38))
	gtx.Constraints.Max = gtx.Constraints.Min
	return surface(gtx, uiColors.primary, color.NRGBA{}, 11, layout.UniformInset(unit.Dp(7)), func(gtx layout.Context) layout.Dimensions {
		l := material.Label(th, unit.Sp(18), "V")
		l.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		l.Font.Weight = font.Bold
		l.Alignment = text.Middle
		return l.Layout(gtx)
	})
}

func sectionTitle(gtx layout.Context, th *material.Theme, title, subtitle string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return label(gtx, th, title, 17, uiColors.text, font.SemiBold)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(3)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return label(gtx, th, subtitle, 12, uiColors.muted, font.Normal)
		}),
	)
}

func label(gtx layout.Context, th *material.Theme, value string, size unit.Sp, col color.NRGBA, weight font.Weight) layout.Dimensions {
	l := material.Label(th, size, value)
	l.Color = col
	l.Font.Weight = weight
	return l.Layout(gtx)
}

func infoBlock(gtx layout.Context, th *material.Theme, title, value string) layout.Dimensions {
	return surface(gtx, uiColors.surfaceAlt, color.NRGBA{}, 9, layout.UniformInset(unit.Dp(12)), func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return label(gtx, th, title, 11, uiColors.muted, font.Medium)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Label(th, unit.Sp(13), value)
				l.Color = uiColors.text
				l.MaxLines = 1
				return l.Layout(gtx)
			}),
		)
	})
}

func primaryAction(gtx layout.Context, th *material.Theme, click *widget.Clickable, icon *widget.Icon, title, subtitle string) layout.Dimensions {
	style := material.ButtonLayout(th, click)
	style.Background = uiColors.primary
	style.CornerRadius = unit.Dp(11)
	return style.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Top: 15, Bottom: 15, Left: 18, Right: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return icon.Layout(gtx, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(13)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return label(gtx, th, title, 15, color.NRGBA{R: 255, G: 255, B: 255, A: 255}, font.SemiBold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return label(gtx, th, subtitle, 11, rgb(224, 231, 255), font.Normal)
						}),
					)
				}),
			)
		})
	})
}

func secondaryAction(gtx layout.Context, th *material.Theme, click *widget.Clickable, icon *widget.Icon, title, subtitle string) layout.Dimensions {
	style := material.ButtonLayout(th, click)
	style.Background = uiColors.surfaceAlt
	style.CornerRadius = unit.Dp(10)
	return style.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Top: 12, Bottom: 12, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Pt(gtx.Dp(19), gtx.Dp(19))
					return icon.Layout(gtx, uiColors.primary)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return label(gtx, th, title, 13, uiColors.text, font.SemiBold)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return label(gtx, th, subtitle, 10, uiColors.muted, font.Normal)
						}),
					)
				}),
			)
		})
	})
}

func compactButton(gtx layout.Context, th *material.Theme, click *widget.Clickable, caption string, background, foreground color.NRGBA) layout.Dimensions {
	b := material.Button(th, click, caption)
	b.Background = background
	b.Color = foreground
	b.CornerRadius = unit.Dp(9)
	b.TextSize = unit.Sp(13)
	b.Inset = layout.Inset{Top: 8, Bottom: 8, Left: 13, Right: 13}
	return b.Layout(gtx)
}

func textButton(gtx layout.Context, th *material.Theme, click *widget.Clickable, caption string, foreground color.NRGBA) layout.Dimensions {
	return compactButton(gtx, th, click, caption, color.NRGBA{}, foreground)
}

func networkEditor(gtx layout.Context, th *material.Theme, fieldLabel, hint string, editor *widget.Editor) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return label(gtx, th, fieldLabel, 12, uiColors.text, font.Medium)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			border := widget.Border{Color: uiColors.border, CornerRadius: unit.Dp(9), Width: unit.Dp(1)}
			return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return surface(gtx, uiColors.surfaceAlt, color.NRGBA{}, 9, layout.Inset{Top: 9, Bottom: 9, Left: 11, Right: 11}, func(gtx layout.Context) layout.Dimensions {
					e := material.Editor(th, editor, hint)
					e.TextSize = unit.Sp(13)
					e.Color = uiColors.text
					e.HintColor = rgb(152, 162, 179)
					return e.Layout(gtx)
				})
			})
		}),
	)
}

func logPanel(gtx layout.Context, th *material.Theme, logs string) layout.Dimensions {
	if strings.TrimSpace(logs) == "" {
		logs = "暂无日志。启动 Viewer 后，进度和错误信息会显示在这里。"
	}
	return surface(gtx, uiColors.log, color.NRGBA{}, 10, layout.UniformInset(unit.Dp(14)), func(gtx layout.Context) layout.Dimensions {
		l := material.Label(th, unit.Sp(11), logs)
		l.Color = rgb(208, 213, 221)
		l.Font.Typeface = font.Typeface("monospace")
		l.MaxLines = 10
		l.LineHeight = unit.Sp(17)
		return l.Layout(gtx)
	})
}

func statusPill(gtx layout.Context, th *material.Theme, caption string, foreground, background color.NRGBA) layout.Dimensions {
	return surface(gtx, background, color.NRGBA{}, 20, layout.Inset{Top: 7, Bottom: 7, Left: 10, Right: 11}, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return statusDot(gtx, foreground, 7) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(7)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return label(gtx, th, caption, 12, foreground, font.SemiBold)
			}),
		)
	})
}

func statusDot(gtx layout.Context, col color.NRGBA, diameter unit.Dp) layout.Dimensions {
	size := gtx.Dp(diameter)
	paint.FillShape(gtx.Ops, col, clip.Ellipse(image.Rectangle{Max: image.Pt(size, size)}).Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(size, size)}
}

func guideProgress(gtx layout.Context, active int) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return progressSegment(gtx, active >= 0) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return progressSegment(gtx, active >= 1) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(5)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return progressSegment(gtx, active >= 2) }),
	)
}

func progressSegment(gtx layout.Context, active bool) layout.Dimensions {
	col := rgb(199, 210, 254)
	if active {
		col = uiColors.primary
	}
	size := image.Pt(gtx.Dp(28), gtx.Dp(4))
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rectangle{Max: size}, size.Y/2).Op(gtx.Ops))
	return layout.Dimensions{Size: size}
}

func contentWidth(gtx layout.Context, content layout.Widget) layout.Dimensions {
	maxWidth := gtx.Dp(unit.Dp(900))
	if gtx.Constraints.Max.X <= maxWidth {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return content(gtx)
	}
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = maxWidth
		gtx.Constraints.Max.X = maxWidth
		return content(gtx)
	})
}

func statusPresentation(state State) (string, color.NRGBA, color.NRGBA) {
	switch {
	case state.Err != "":
		return "需要处理", uiColors.danger, uiColors.dangerSoft
	case state.Busy:
		return "正在准备", uiColors.primary, uiColors.primarySoft
	case state.Running:
		return "运行中", uiColors.success, uiColors.successSoft
	default:
		return "已停止", uiColors.muted, uiColors.surfaceAlt
	}
}

func headerSubtitle(state State) string {
	switch {
	case state.Busy:
		return "正在准备本地服务"
	case state.Running:
		return "本地服务运行中"
	default:
		return "安全、独立的本地启动器"
	}
}

func compactPath(value string) string {
	const limit = 48
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return "…" + string(runes[len(runes)-limit+1:])
}

func mustIcon(data []byte) *widget.Icon {
	icon, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return icon
}

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: 255} }

func tailLines(value string, limit int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return strings.Join(lines, "\n")
}
