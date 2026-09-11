package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const settingsFile = "settings.json"

// DownloadSettings controls only launcher downloads. Mirror is a GitHub URL
// prefix (or a template containing {url}); Proxy is an explicit HTTP(S) or
// SOCKS5 proxy. An empty Proxy keeps Go's standard environment-proxy behavior.
type DownloadSettings struct {
	Mirror string `json:"mirror,omitempty"`
	Proxy  string `json:"proxy,omitempty"`
}

func (settings DownloadSettings) normalized() DownloadSettings {
	settings.Mirror = strings.TrimSpace(settings.Mirror)
	settings.Proxy = strings.TrimSpace(settings.Proxy)
	return settings
}

func (settings DownloadSettings) validate() error {
	settings = settings.normalized()
	if settings.Mirror != "" {
		candidate := settings.Mirror
		if strings.Contains(candidate, "{url}") {
			candidate = strings.ReplaceAll(candidate, "{url}", "https://github.com/example/archive.zip")
		} else {
			candidate = strings.TrimRight(candidate, "/") + "/https://github.com/example/archive.zip"
		}
		if err := validateNetworkURL("镜像地址", candidate, "http", "https"); err != nil {
			return err
		}
	}
	if settings.Proxy != "" {
		if err := validateNetworkURL("代理地址", settings.Proxy, "http", "https", "socks5"); err != nil {
			return err
		}
	}
	return nil
}

func validateNetworkURL(label, value string, allowed ...string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%s无效", label)
	}
	for _, scheme := range allowed {
		if strings.EqualFold(parsed.Scheme, scheme) {
			return nil
		}
	}
	return fmt.Errorf("%s不支持协议 %q", label, parsed.Scheme)
}

func (settings DownloadSettings) rewrite(original string) string {
	settings = settings.normalized()
	if settings.Mirror == "" {
		return original
	}
	if strings.Contains(settings.Mirror, "{url}") {
		return strings.ReplaceAll(settings.Mirror, "{url}", original)
	}
	return strings.TrimRight(settings.Mirror, "/") + "/" + original
}

func (settings DownloadSettings) client() (*http.Client, error) {
	settings = settings.normalized()
	if err := settings.validate(); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyFromEnvironment
	if settings.Proxy != "" {
		proxyURL, err := url.Parse(settings.Proxy)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Minute}, nil
}

func loadDownloadSettings(root string) (DownloadSettings, error) {
	contents, err := os.ReadFile(filepath.Join(root, settingsFile))
	if os.IsNotExist(err) {
		return DownloadSettings{}, nil
	}
	if err != nil {
		return DownloadSettings{}, err
	}
	var settings DownloadSettings
	if err := json.Unmarshal(contents, &settings); err != nil {
		return DownloadSettings{}, fmt.Errorf("解析 %s: %w", settingsFile, err)
	}
	settings = settings.normalized()
	if err := settings.validate(); err != nil {
		return DownloadSettings{}, err
	}
	return settings, nil
}

func saveDownloadSettings(root string, settings DownloadSettings) error {
	settings = settings.normalized()
	if err := settings.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	contents = append(contents, '\n')
	return os.WriteFile(filepath.Join(root, settingsFile), contents, 0o600)
}

func redactedProxy(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return "已配置"
	}
	parsed.User = nil
	return parsed.String()
}
