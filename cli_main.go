//go:build cli

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const cliName = "viewer-launcher-cli"

type cliOptions struct {
	root       string
	repository string
}

func main() {
	os.Exit(runCLI(os.Args[1:]))
}

func runCLI(args []string) int {
	root, err := AppRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 1
	}

	global := flag.NewFlagSet(cliName, flag.ContinueOnError)
	global.SetOutput(os.Stderr)
	options := cliOptions{}
	global.StringVar(&options.root, "root", root, "程序数据目录")
	global.StringVar(&options.repository, "repository", repository, "发布仓库（OWNER/REPOSITORY）")
	global.Usage = printCLIUsage
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	remaining := global.Args()
	if len(remaining) == 0 {
		printCLIUsage()
		return 2
	}

	switch remaining[0] {
	case "start":
		return runCLIStart(options, false, remaining[1:])
	case "update":
		return runCLIStart(options, true, remaining[1:])
	case "status":
		return runCLIStatus(options, remaining[1:])
	case "config":
		return runCLIConfig(options, remaining[1:])
	case "version", "--version", "-version":
		fmt.Printf("%s %s\n", cliName, version)
		return 0
	case "help", "--help", "-h":
		printCLIUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知命令 %q\n\n", remaining[0])
		printCLIUsage()
		return 2
	}
}

func runCLIStart(options cliOptions, refresh bool, args []string) int {
	flags := flag.NewFlagSet("start", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "start/update 不接受位置参数")
		return 2
	}

	var launcher *Launcher
	lastLogs := ""
	var outputMu sync.Mutex
	launcher = NewLauncher(options.root, options.repository, func() {
		outputMu.Lock()
		defer outputMu.Unlock()
		if launcher == nil {
			return
		}
		logs := launcher.Snapshot().Logs
		if strings.HasPrefix(logs, lastLogs) {
			addition := strings.TrimPrefix(logs, lastLogs)
			addition = strings.TrimPrefix(addition, "\n")
			if addition != "" {
				fmt.Println(addition)
			}
		} else if logs != lastLogs {
			fmt.Println(logs)
		}
		lastLogs = logs
	})
	launcher.SetOpenBrowserOnStart(false)

	action := "启动"
	if refresh {
		action = "更新并启动"
	}
	fmt.Printf("Viewer Launcher %s\n数据目录: %s\n操作: %s\n\n", version, options.root, action)
	launcher.EnsureAndStart(refresh)
	state := launcher.Snapshot()
	if state.Err != "" {
		fmt.Fprintln(os.Stderr, "失败:", state.Err)
		return 1
	}
	if !state.Running {
		fmt.Fprintln(os.Stderr, "服务未能启动")
		return 1
	}

	fmt.Printf("\n服务已就绪: %s\n按 Ctrl+C 安全停止。\n", state.URL)
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("\n正在停止服务…")
			launcher.Stop()
			return 0
		case <-ticker.C:
			state = launcher.Snapshot()
			if !state.Running {
				if state.Err != "" {
					fmt.Fprintln(os.Stderr, "服务异常退出:", state.Err)
				}
				return 1
			}
		}
	}
}

func runCLIStatus(options cliOptions, args []string) int {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "status 不接受位置参数")
		return 2
	}

	checks := []struct {
		name string
		path string
	}{
		{"Web 前端", filepath.Join(options.root, "web", "index.html")},
		{"Viewer 后端", filepath.Join(options.root, "backend", "app", "main.py")},
		{"Python 运行时", pythonExecutable(options.root)},
	}
	fmt.Printf("Viewer Launcher %s\n数据目录: %s\n", version, options.root)
	for _, check := range checks {
		status := "未安装"
		if _, err := os.Stat(check.path); err == nil {
			status = "已安装"
		}
		fmt.Printf("%-14s %s\n", check.name+":", status)
	}

	client := &http.Client{Timeout: 1200 * time.Millisecond}
	response, err := client.Get(viewerURL)
	if err == nil {
		_ = response.Body.Close()
	}
	if err == nil && response.StatusCode >= 200 && response.StatusCode < 500 {
		fmt.Printf("%-14s 运行中 (%s)\n", "本地服务:", viewerURL)
		return 0
	}
	fmt.Printf("%-14s 未运行\n", "本地服务:")
	return 0
}

func runCLIConfig(options cliOptions, args []string) int {
	launcher := NewLauncher(options.root, options.repository, nil)
	current := launcher.DownloadSettings()
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	mirror := flags.String("mirror", current.Mirror, "GitHub 镜像前缀；传空字符串可清除")
	proxy := flags.String("proxy", current.Proxy, "HTTP(S)/SOCKS5 代理；传空字符串可清除")
	show := flags.Bool("show", false, "仅显示当前配置")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "config 不接受位置参数")
		return 2
	}

	changed := flagWasSet(flags, "mirror") || flagWasSet(flags, "proxy")
	if *show || !changed {
		printConfig(options.root, current)
		return 0
	}
	if err := launcher.ConfigureDownloads(*mirror, *proxy); err != nil {
		fmt.Fprintln(os.Stderr, "保存配置失败:", err)
		return 1
	}
	printConfig(options.root, launcher.DownloadSettings())
	return 0
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	found := false
	flags.Visit(func(item *flag.Flag) {
		if item.Name == name {
			found = true
		}
	})
	return found
}

func printConfig(root string, settings DownloadSettings) {
	mirror := settings.Mirror
	if mirror == "" {
		mirror = "（直连 GitHub）"
	}
	proxy := redactedProxy(settings.Proxy)
	if settings.Proxy == "" {
		proxy = "（使用系统代理设置）"
	}
	fmt.Printf("配置文件: %s\nGitHub 镜像: %s\n下载代理: %s\n", filepath.Join(root, settingsFile), mirror, proxy)
}

func printCLIUsage() {
	output := flag.CommandLine.Output()
	if output == nil {
		output = os.Stderr
	}
	fmt.Fprintf(output, `%s - Viewer 的无界面启动器

用法:
  %s [全局选项] <命令> [命令选项]

命令:
  start             准备组件并启动服务，Ctrl+C 停止
  update            更新全部组件后启动，Ctrl+C 停止
  status            查看组件安装状态和本地服务状态
  config            查看或修改镜像、代理设置
  version           显示版本

全局选项:
  --root <目录>                 指定程序数据目录
  --repository <OWNER/REPO>    指定发布仓库

示例:
  %s start
  %s update
  %s config --mirror "https://gh-proxy.example/"
  %s config --proxy "socks5://127.0.0.1:1080"
`, cliName, cliName, cliName, cliName, cliName, cliName)
}
