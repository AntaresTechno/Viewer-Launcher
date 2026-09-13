package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const releaseManifestSchema = 1

var (
	commitHash = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Hash = regexp.MustCompile(`^[0-9a-f]{64}$`)
	assetName  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

type releaseManifest struct {
	SchemaVersion      int            `json:"schema_version"`
	ReleaseTag         string         `json:"release_tag"`
	SourceRepository   string         `json:"source_repository"`
	SourceCommit       string         `json:"source_commit"`
	UpstreamRepository string         `json:"upstream_repository"`
	UpstreamCommit     string         `json:"upstream_commit"`
	CreatedAt          string         `json:"created_at"`
	Assets             []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Platform    string `json:"platform"`
	InstallDir  string `json:"install_dir,omitempty"`
	Description string `json:"description"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
}

func fetchLatestReleaseManifest(ctx context.Context, client *http.Client, settings DownloadSettings, repository, channel string) (releaseManifest, string, error) {
	if !validRepository(repository) {
		return releaseManifest{}, "", fmt.Errorf("invalid GitHub repository %q", repository)
	}
	if !assetName.MatchString(channel) {
		return releaseManifest{}, "", fmt.Errorf("invalid release channel %q", channel)
	}
	releasesURL := settings.rewrite(fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", repository))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return releaseManifest{}, "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Viewer-Launcher/"+version)
	response, err := client.Do(request)
	if err != nil {
		return releaseManifest{}, "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return releaseManifest{}, "", fmt.Errorf("GitHub releases API returned %s", response.Status)
	}
	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&releases); err != nil {
		return releaseManifest{}, "", err
	}
	tag, err := latestReleaseTag(releases, channel)
	if err != nil {
		return releaseManifest{}, "", err
	}
	manifest, err := fetchReleaseManifest(ctx, client, settings, repository, tag)
	return manifest, tag, err
}

func latestReleaseTag(releases []githubRelease, channel string) (string, error) {
	prefix := channel + "-"
	var selected githubRelease
	for _, release := range releases {
		if release.Draft || !release.Prerelease || !strings.HasPrefix(release.TagName, prefix) || !assetName.MatchString(release.TagName) {
			continue
		}
		if selected.TagName == "" || release.PublishedAt > selected.PublishedAt {
			selected = release
		}
	}
	if selected.TagName == "" {
		return "", fmt.Errorf("no published %s-* prerelease was found", channel)
	}
	return selected.TagName, nil
}

func fetchReleaseManifest(ctx context.Context, client *http.Client, settings DownloadSettings, repository, tag string) (releaseManifest, error) {
	if !validRepository(repository) {
		return releaseManifest{}, fmt.Errorf("invalid GitHub repository %q", repository)
	}
	if !assetName.MatchString(tag) {
		return releaseManifest{}, fmt.Errorf("invalid release tag %q", tag)
	}
	manifestURL := settings.rewrite(fmt.Sprintf("https://github.com/%s/releases/download/%s/release-manifest.json", repository, tag))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return releaseManifest{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return releaseManifest{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return releaseManifest{}, fmt.Errorf("GitHub returned %s", response.Status)
	}
	var manifest releaseManifest
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&manifest); err != nil {
		return releaseManifest{}, err
	}
	if err := manifest.validate(tag); err != nil {
		return releaseManifest{}, err
	}
	if manifest.SourceRepository != repository {
		return releaseManifest{}, fmt.Errorf("release manifest belongs to %s, expected %s", manifest.SourceRepository, repository)
	}
	return manifest, nil
}

func (manifest releaseManifest) validate(expectedTag string) error {
	if manifest.SchemaVersion != releaseManifestSchema {
		return fmt.Errorf("unsupported release manifest schema %d", manifest.SchemaVersion)
	}
	if manifest.ReleaseTag != expectedTag {
		return fmt.Errorf("release manifest tag is %q, expected %q", manifest.ReleaseTag, expectedTag)
	}
	if !validRepository(manifest.SourceRepository) || !validRepository(manifest.UpstreamRepository) {
		return errors.New("release manifest contains an invalid repository")
	}
	if !commitHash.MatchString(manifest.SourceCommit) || !commitHash.MatchString(manifest.UpstreamCommit) {
		return errors.New("release manifest must pin immutable 40-character commits")
	}
	if len(manifest.Assets) == 0 {
		return errors.New("release manifest has no assets")
	}
	seen := make(map[string]bool, len(manifest.Assets))
	for _, asset := range manifest.Assets {
		if !assetName.MatchString(asset.Name) || asset.Size <= 0 || !sha256Hash.MatchString(asset.SHA256) {
			return fmt.Errorf("release manifest contains invalid asset %q", asset.Name)
		}
		if seen[asset.Name] {
			return fmt.Errorf("release manifest contains duplicate asset %q", asset.Name)
		}
		seen[asset.Name] = true
	}
	return nil
}

func (manifest releaseManifest) runtimeAssets(platform string) ([]releaseAsset, error) {
	wanted := []struct {
		kind, platform, installDir string
	}{
		{"web", "all", "web"},
		{"backend", "all", "backend"},
		{"runtime", platform, "lib"},
	}
	result := make([]releaseAsset, 0, len(wanted))
	for _, target := range wanted {
		var found *releaseAsset
		for index := range manifest.Assets {
			asset := &manifest.Assets[index]
			if asset.Kind == target.kind && asset.Platform == target.platform {
				if found != nil {
					return nil, fmt.Errorf("release manifest has multiple %s assets for %s", target.kind, target.platform)
				}
				found = asset
			}
		}
		if found == nil {
			return nil, fmt.Errorf("release manifest has no %s asset for %s", target.kind, target.platform)
		}
		if found.InstallDir != target.installDir {
			return nil, fmt.Errorf("asset %s has invalid install_dir %q", found.Name, found.InstallDir)
		}
		result = append(result, *found)
	}
	return result, nil
}
