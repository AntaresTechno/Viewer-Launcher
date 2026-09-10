package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const viewerURL = "http://127.0.0.1:18080"

type State struct {
	Root    string
	URL     string
	Message string
	Err     string
	Running bool
}

type Launcher struct {
	root       string
	repository string
	mu         sync.RWMutex
	state      State
	cmd        *exec.Cmd
	notify     func()
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
	return &Launcher{root: root, repository: repo, notify: notify, state: State{
		Root: root, URL: viewerURL, Message: "准备就绪",
	}}
}

func (l *Launcher) Repository() string { return l.repository }

func (l *Launcher) Snapshot() State {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.state
}

func (l *Launcher) set(message string, err error, running bool) {
	l.mu.Lock()
	l.state.Message, l.state.Running = message, running
	if err != nil {
		l.state.Err = err.Error()
		l.state.Message = message + "：" + err.Error()
	} else {
		l.state.Err = ""
	}
	l.mu.Unlock()
	if l.notify != nil {
		l.notify()
	}
}

func (l *Launcher) EnsureAndStart(refresh bool) {
	if l.Repository() == "" {
		l.set("无法下载发布物", errors.New("未配置 GitHub 发布仓库"), false)
		return
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		l.set("不支持当前平台", fmt.Errorf("当前仅提供 windows/amd64 运行时，实际为 %s/%s", runtime.GOOS, runtime.GOARCH), false)
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
	if refresh || !runtimePresent(l.root) {
		l.set("正在下载受检运行时", nil, false)
		if err := InstallBranch(context.Background(), l.root, l.Repository(), "runtime-windows-amd64", "runtime"); err != nil {
			l.set("安装运行时失败", err, false)
			return
		}
	}
	if refresh || !frontendPresent(l.root) {
		l.set("正在下载前端构建产物", nil, false)
		if err := InstallBranch(context.Background(), filepath.Join(l.root, "runtime"), l.Repository(), "frontend-dist", "dist"); err != nil {
			l.set("安装前端失败", err, false)
			return
		}
	}
	if err := compatibleBuilds(l.root); err != nil {
		l.set("运行时与前端版本不兼容", err, false)
		return
	}
	if err := l.start(); err != nil {
		l.set("启动后端失败", err, false)
		return
	}
	l.set("Viewer 正在运行", nil, true)
	l.OpenBrowser()
}

func compatibleBuilds(root string) error {
	readCommit := func(path string) (string, error) {
		contents, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		var metadata struct {
			UpstreamCommit string `json:"upstream_commit"`
		}
		if err := json.Unmarshal(contents, &metadata); err != nil {
			return "", err
		}
		if metadata.UpstreamCommit == "" {
			return "", errors.New("release metadata omits upstream_commit")
		}
		return metadata.UpstreamCommit, nil
	}
	runtimeCommit, err := readCommit(filepath.Join(root, "runtime", "runtime.json"))
	if err != nil {
		return fmt.Errorf("read runtime metadata: %w", err)
	}
	frontendCommit, err := readCommit(filepath.Join(root, "runtime", "frontend", "dist", "release.json"))
	if err != nil {
		return fmt.Errorf("read frontend metadata: %w", err)
	}
	if runtimeCommit != frontendCommit {
		return fmt.Errorf("runtime uses %s but frontend uses %s; publish both from the same Viewer commit", runtimeCommit, frontendCommit)
	}
	return nil
}

func runtimePresent(root string) bool {
	_, err := os.Stat(filepath.Join(root, "runtime", "python", "python.exe"))
	return err == nil
}

func frontendPresent(root string) bool {
	_, err := os.Stat(filepath.Join(root, "runtime", "frontend", "dist", "index.html"))
	return err == nil
}

func (l *Launcher) start() error {
	runtimeRoot := filepath.Join(l.root, "runtime")
	python := filepath.Join(runtimeRoot, "python", "python.exe")
	cmd := exec.Command(python, "-m", "uvicorn", "app.main:app", "--host", "127.0.0.1", "--port", "18080")
	cmd.Dir = filepath.Join(runtimeRoot, "backend")
	dataDir := filepath.Join(l.root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	cmd.Env = append(os.Environ(),
		"PYTHONHOME="+filepath.Join(runtimeRoot, "python"),
		"PYTHONPATH="+filepath.Join(runtimeRoot, "backend"),
		"VIEWER_DATABASE_URL=sqlite+aiosqlite:///"+filepath.ToSlash(filepath.Join(dataDir, "viewer.db")),
	)
	if err := cmd.Start(); err != nil {
		return err
	}
	l.mu.Lock()
	l.cmd = cmd
	l.mu.Unlock()
	go func() {
		err := cmd.Wait()
		l.mu.Lock()
		changed := false
		if l.cmd == cmd {
			l.cmd = nil
			l.state.Running = false
			changed = true
			if err != nil {
				l.state.Err = err.Error()
				l.state.Message = "Viewer 已停止：" + err.Error()
			}
		}
		l.mu.Unlock()
		if changed && l.notify != nil {
			l.notify()
		}
	}()
	if err := waitForHealth(viewerURL + "/api/health"); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return nil
}

func waitForHealth(url string) error {
	client := &http.Client{Timeout: time.Second}
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err == nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return errors.New("35 秒内未通过 /api/health 健康检查")
}

func (l *Launcher) Stop() {
	l.mu.Lock()
	cmd := l.cmd
	l.cmd = nil
	l.state.Running = false
	l.mu.Unlock()
	if l.notify != nil {
		l.notify()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func (l *Launcher) OpenBrowser() {
	if l.Snapshot().Running {
		_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", viewerURL).Start()
	}
}
