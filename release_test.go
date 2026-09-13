package main

import (
	"os"
	"path/filepath"
	"testing"
)

const testCommit = "e147f9b98da5aaedde850557b3d8e55954dea4d9"

func validTestManifest() releaseManifest {
	asset := func(name, kind, platform, installDir string) releaseAsset {
		return releaseAsset{Name: name, Kind: kind, Platform: platform, InstallDir: installDir, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Size: 42}
	}
	return releaseManifest{
		SchemaVersion:      releaseManifestSchema,
		ReleaseTag:         "preview-42",
		SourceRepository:   "owner/launcher",
		SourceCommit:       testCommit,
		UpstreamRepository: "owner/viewer",
		UpstreamCommit:     testCommit,
		Assets: []releaseAsset{
			asset("viewer-web.tar.gz", "web", "all", "web"),
			asset("viewer-backend.tar.gz", "backend", "all", "backend"),
			asset("viewer-runtime-windows-amd64.tar.gz", "runtime", "windows-amd64", "lib"),
		},
	}
}

func TestReleaseManifestSelectsRuntimeSet(t *testing.T) {
	manifest := validTestManifest()
	if err := manifest.validate("preview-42"); err != nil {
		t.Fatal(err)
	}
	assets, err := manifest.runtimeAssets("windows-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 3 || assets[0].InstallDir != "web" || assets[1].InstallDir != "backend" || assets[2].InstallDir != "lib" {
		t.Fatalf("unexpected runtime asset set: %#v", assets)
	}
}

func TestReleaseManifestRejectsDuplicateAsset(t *testing.T) {
	manifest := validTestManifest()
	manifest.Assets = append(manifest.Assets, manifest.Assets[0])
	if err := manifest.validate("preview-42"); err == nil {
		t.Fatal("duplicate asset was accepted")
	}
}

func TestLatestReleaseTagSelectsNewestPublishedPreview(t *testing.T) {
	releases := []githubRelease{
		{TagName: "v1.0.0", Prerelease: false, PublishedAt: "2026-09-13T12:00:00Z"},
		{TagName: "preview-41", Prerelease: true, PublishedAt: "2026-09-13T13:00:00Z"},
		{TagName: "preview-42", Prerelease: true, PublishedAt: "2026-09-13T14:00:00Z"},
		{TagName: "preview-43", Draft: true, Prerelease: true, PublishedAt: "2026-09-13T15:00:00Z"},
	}
	tag, err := latestReleaseTag(releases, "preview")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "preview-42" {
		t.Fatalf("latestReleaseTag() = %q, want preview-42", tag)
	}
}

func TestLatestReleaseTagRejectsMissingChannel(t *testing.T) {
	_, err := latestReleaseTag([]githubRelease{{TagName: "nightly-1", Prerelease: true}}, "preview")
	if err == nil {
		t.Fatal("missing preview channel was accepted")
	}
}

func TestActivateReleaseReplacesComponentsAndKeepsData(t *testing.T) {
	root := t.TempDir()
	staging := filepath.Join(root, "staging")
	assets := validTestManifest().Assets
	for _, asset := range assets {
		oldDir := filepath.Join(root, asset.InstallDir)
		newDir := filepath.Join(staging, asset.InstallDir, asset.InstallDir)
		if err := os.MkdirAll(oldDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(oldDir, "old"), []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(newDir, "new"), []byte("new"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "viewer.db"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := activateRelease(root, staging, assets); err != nil {
		t.Fatal(err)
	}
	for _, asset := range assets {
		if _, err := os.Stat(filepath.Join(root, asset.InstallDir, "new")); err != nil {
			t.Fatalf("new %s was not activated: %v", asset.InstallDir, err)
		}
		if _, err := os.Stat(filepath.Join(root, asset.InstallDir, "old")); !os.IsNotExist(err) {
			t.Fatalf("old %s was not replaced", asset.InstallDir)
		}
	}
	if contents, err := os.ReadFile(filepath.Join(dataDir, "viewer.db")); err != nil || string(contents) != "data" {
		t.Fatalf("data directory changed: %q, %v", contents, err)
	}
}
