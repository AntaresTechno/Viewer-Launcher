package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	viewerURL  = "http://127.0.0.1:18080"
	backendURL = "http://127.0.0.1:18081"
	upstream   = "AntaresTechno/Viewer"
)

type State struct {
	Root, URL, Message, Err, Logs string
	Running                       bool
}
type libManifest struct {
	UpstreamCommit string `json:"upstream_commit"`
}

type Launcher struct {
	root, repository string
	mu               sync.RWMutex
	state            State
	cmd              *exec.Cmd
	webServer        *http.Server
	listener         net.Listener
	notify           func()
}

func AppRoot() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return filepath.Join(dir, "ViewerLauncher"), nil
}

func NewLauncher(root, compiledRepository string, notify func()) *Launcher {
	repo := os.Getenv("VIEWER_LAUNCHER_REPOSITORY")
	if repo == "" {
		repo = compiledRepository
	}
	return &Launcher{root: root, repository: repo, notify: notify, state: State{Root: root, URL: viewerURL, Message: "准备就绪"}}
}
func (l *Launcher) Repository() string { return l.repository }
func (l *Launcher) Snapshot() State    { l.mu.RLock(); defer l.mu.RUnlock(); return l.state }
func (l *Launcher) notifyUI() {
	if l.notify != nil {
		l.notify()
	}
}

func (l *Launcher) set(message string, err error, running bool) {
	l.mu.Lock()
	l.state.Message, l.state.Running = message, running
	if err != nil {
		l.state.Err, l.state.Message = err.Error(), message+"："+err.Error()
	} else {
		l.state.Err = ""
	}
	l.mu.Unlock()
	l.appendLog(message)
}
func (l *Launcher) appendLog(message string) {
	line := fmt.Sprintf("%s %s", time.Now().Format("15:04:05"), strings.TrimSpace(message))
	l.mu.Lock()
	lines := append(strings.Split(strings.TrimSpace(l.state.Logs), "\n"), line)
	if len(lines) > 300 {
		lines = lines[len(lines)-300:]
	}
	l.state.Logs = strings.Join(lines, "\n")
	l.mu.Unlock()
	logDir := filepath.Join(l.root, "logs")
	if os.MkdirAll(logDir, 0o755) == nil {
		if file, err := os.OpenFile(filepath.Join(logDir, "launcher.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = fmt.Fprintln(file, line)
			_ = file.Close()
		}
	}
	l.notifyUI()
}

func (l *Launcher) EnsureAndStart(refresh bool) {
	if l.Repository() == "" {
		l.set("无法下载发布物", errors.New("未配置 GitHub 发布仓库"), false)
		return
	}
	platform, err := platformKey()
	if err != nil {
		l.set("不支持当前平台", err, false)
		return
	}
	if l.Snapshot().Running && !refresh {
		l.OpenBrowser()
		return
	}
	l.Stop()
	if err := os.MkdirAll(l.root, 0o755); err != nil {
		l.set("创建数据目录失败", err, false)
		return
	}
	// lib pins the commit that was main when its platform wheels were built. The
	// visible installation order is web -> backend -> Python/dependencies.
	l.set("正在读取 lib 依赖清单", nil, false)
	manifest, err := fetchLibManifest(context.Background(), l.Repository(), platform)
	if err != nil {
		l.set("读取 lib 依赖清单失败", err, false)
		return
	}
	if refresh || !webPresent(l.root) {
		l.set("正在拉取 web 前端", nil, false)
		if err := InstallTree(context.Background(), l.root, l.Repository(), "refs/heads/web", "web", "web"); err != nil {
			l.set("安装 web 前端失败", err, false)
			return
		}
	}
	if refresh || !backendPresent(l.root) {
		l.set("正在从 Viewer main 对应提交拉取后端", nil, false)
		if err := InstallTree(context.Background(), l.root, upstream, manifest.UpstreamCommit, "backend", "backend"); err != nil {
			l.set("安装 Viewer 后端失败", err, false)
			return
		}
	}
	if refresh || !libPresent(l.root) {
		l.set("正在拉取嵌入式 Python 与依赖包", nil, false)
		if err := InstallTree(context.Background(), l.root, l.Repository(), "refs/heads/lib", "lib/"+platform, "lib"); err != nil {
			l.set("安装 lib 依赖失败", err, false)
			return
		}
	}
	if err := ready(l.root, manifest.UpstreamCommit); err != nil {
		l.set("安装完整性检查失败", err, false)
		return
	}
	l.set("全部组件已就绪，正在启动后端", nil, false)
	if err := l.startBackend(); err != nil {
		l.set("启动后端失败", err, false)
		return
	}
	if err := l.startWebServer(); err != nil {
		l.Stop()
		l.set("启动 Go HTTP 服务失败", err, false)
		return
	}
	l.set("Viewer 正在运行", nil, true)
	l.OpenBrowser()
}

func fetchLibManifest(ctx context.Context, repository, platform string) (libManifest, error) {
	if !validRepository(repository) {
		return libManifest{}, fmt.Errorf("invalid GitHub repository %q", repository)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/"+repository+"/lib/lib/"+platform+"/lib.json", nil)
	if err != nil {
		return libManifest{}, err
	}
	response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
	if err != nil {
		return libManifest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return libManifest{}, fmt.Errorf("GitHub returned %s", response.Status)
	}
	var manifest libManifest
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&manifest); err != nil {
		return libManifest{}, err
	}
	if !commitHash.MatchString(manifest.UpstreamCommit) {
		return libManifest{}, errors.New("lib.json has no immutable 40-character upstream_commit")
	}
	return manifest, nil
}

func webPresent(root string) bool {
	_, err := os.Stat(filepath.Join(root, "web", "index.html"))
	return err == nil
}
func backendPresent(root string) bool {
	_, err := os.Stat(filepath.Join(root, "backend", "app", "main.py"))
	return err == nil
}
func libPresent(root string) bool { _, err := os.Stat(pythonExecutable(root)); return err == nil }

func platformKey() (string, error) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "windows/amd64", "linux/amd64", "linux/arm64":
		return runtime.GOOS + "-" + runtime.GOARCH, nil
	default:
		return "", fmt.Errorf("当前未提供 %s/%s 的 lib 运行时", runtime.GOOS, runtime.GOARCH)
	}
}

func pythonExecutable(root string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(root, "lib", "python", "python.exe")
	}
	return filepath.Join(root, "lib", "python", "bin", "python3")
}

func ready(root, expectedCommit string) error {
	for _, path := range []string{filepath.Join(root, "web", "index.html"), filepath.Join(root, "backend", "app", "main.py"), pythonExecutable(root), filepath.Join(root, "lib", "licenses", "python-packages.json")} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("required file %s: %w", path, err)
		}
	}
	var web, lib libManifest
	for path, destination := range map[string]*libManifest{filepath.Join(root, "web", "release.json"): &web, filepath.Join(root, "lib", "lib.json"): &lib} {
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(contents, destination); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}
	if web.UpstreamCommit != expectedCommit || lib.UpstreamCommit != expectedCommit {
		return fmt.Errorf("web/lib/backend commits differ (expected %s, web %s, lib %s)", expectedCommit, web.UpstreamCommit, lib.UpstreamCommit)
	}
	return nil
}

func (l *Launcher) startBackend() error {
	pythonRoot := filepath.Join(l.root, "lib", "python")
	cmd := exec.Command(pythonExecutable(l.root), "-m", "uvicorn", "app.main:app", "--host", "127.0.0.1", "--port", "18081")
	cmd.Dir = filepath.Join(l.root, "backend")
	dataDir := filepath.Join(l.root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	cmd.Env = append(os.Environ(), "PYTHONHOME="+pythonRoot, "VIEWER_DATABASE_URL=sqlite+aiosqlite:///"+filepath.ToSlash(filepath.Join(dataDir, "viewer.db")))
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	l.mu.Lock()
	l.cmd = cmd
	l.mu.Unlock()
	go l.capture("[python]", stdout)
	go l.capture("[python]", stderr)
	go func() {
		err := cmd.Wait()
		l.mu.Lock()
		changed := l.cmd == cmd
		if changed {
			l.cmd, l.state.Running = nil, false
		}
		l.mu.Unlock()
		if changed {
			l.appendLog(fmt.Sprintf("后端已退出：%v", err))
		}
	}()
	if err := waitForHealth(backendURL + "/api/health"); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return nil
}
func (l *Launcher) capture(prefix string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		l.appendLog(prefix + " " + scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		l.appendLog(prefix + " log read error: " + err.Error())
	}
}

func (l *Launcher) startWebServer() error {
	target, err := url.Parse(backendURL)
	if err != nil {
		return err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorLog = log.New(logWriter{l, "[proxy]"}, "", 0)
	webRoot := filepath.Join(l.root, "web")
	static := http.FileServer(http.Dir(webRoot))
	handler := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		l.appendLog("[go-http] " + request.Method + " " + request.URL.Path)
		if request.URL.Path == "/api" || strings.HasPrefix(request.URL.Path, "/api/") || request.URL.Path == "/dav" || strings.HasPrefix(request.URL.Path, "/dav/") {
			proxy.ServeHTTP(w, request)
			return
		}
		relative := filepath.FromSlash(strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/"))
		candidate := filepath.Join(webRoot, relative)
		if rel, err := filepath.Rel(webRoot, candidate); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				static.ServeHTTP(w, request)
				return
			}
		}
		http.ServeFile(w, request, filepath.Join(webRoot, "index.html"))
	})
	listener, err := net.Listen("tcp", "127.0.0.1:18080")
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ErrorLog: log.New(logWriter{l, "[go-http]"}, "", 0), ReadHeaderTimeout: 10 * time.Second}
	l.mu.Lock()
	l.webServer, l.listener = server, listener
	l.mu.Unlock()
	l.appendLog("[go-http] serving web dist on " + viewerURL)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			l.appendLog("[go-http] server error: " + err.Error())
		}
	}()
	return nil
}

type logWriter struct {
	launcher *Launcher
	prefix   string
}

func (w logWriter) Write(message []byte) (int, error) {
	w.launcher.appendLog(w.prefix + " " + strings.TrimSpace(string(message)))
	return len(message), nil
}

func waitForHealth(endpoint string) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		if response, err := client.Get(endpoint); err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("35 秒内未通过后端 /api/health 健康检查")
}
func (l *Launcher) Stop() {
	l.mu.Lock()
	cmd, server, listener := l.cmd, l.webServer, l.listener
	l.cmd, l.webServer, l.listener = nil, nil, nil
	l.state.Running = false
	l.mu.Unlock()
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = server.Shutdown(ctx)
		cancel()
	} else if listener != nil {
		_ = listener.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	l.appendLog("服务已停止")
}
func (l *Launcher) OpenBrowser() {
	if !l.Snapshot().Running {
		return
	}
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", viewerURL).Start()
	default:
		_ = exec.Command("xdg-open", viewerURL).Start()
	}
}
