package main

import "testing"

func TestDownloadSettingsRewrite(t *testing.T) {
	original := "https://codeload.github.com/owner/repo/zip/main"
	tests := []struct {
		name, mirror, want string
	}{
		{name: "direct", want: original},
		{name: "prefix", mirror: "https://mirror.example/", want: "https://mirror.example/" + original},
		{name: "template", mirror: "https://mirror.example/fetch?url={url}", want: "https://mirror.example/fetch?url=" + original},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := (DownloadSettings{Mirror: test.mirror}).rewrite(original); got != test.want {
				t.Fatalf("rewrite() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDownloadSettingsPersistence(t *testing.T) {
	root := t.TempDir()
	want := DownloadSettings{Mirror: "https://mirror.example/", Proxy: "socks5://127.0.0.1:1080", GuideComplete: true}
	if err := saveDownloadSettings(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadDownloadSettings(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("loadDownloadSettings() = %#v, want %#v", got, want)
	}
}

func TestGuideCompletionSurvivesNetworkChanges(t *testing.T) {
	launcher := NewLauncher(t.TempDir(), "owner/repository", nil)
	if err := launcher.CompleteGuide(); err != nil {
		t.Fatal(err)
	}
	if err := launcher.ConfigureDownloads("https://mirror.example/", ""); err != nil {
		t.Fatal(err)
	}
	settings := launcher.DownloadSettings()
	if !settings.GuideComplete {
		t.Fatal("network settings cleared the completed guide state")
	}
}

func TestDownloadSettingsRejectInvalidEndpoints(t *testing.T) {
	for _, settings := range []DownloadSettings{
		{Mirror: "file:///tmp/mirror"},
		{Proxy: "ssh://127.0.0.1:22"},
		{Proxy: "://broken"},
	} {
		if err := settings.validate(); err == nil {
			t.Fatalf("validate() accepted %#v", settings)
		}
	}
}
