package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallRelease downloads and verifies the three runtime assets as one
// release. Existing components are only replaced after every archive has been
// downloaded, hashed, and safely extracted.
func InstallRelease(ctx context.Context, client *http.Client, settings DownloadSettings, destination, repository, tag, platform string, manifest releaseManifest) error {
	required, err := manifest.runtimeAssets(platform)
	if err != nil {
		return err
	}
	work, err := os.MkdirTemp(destination, ".viewer-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	staging := filepath.Join(work, "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return err
	}
	for _, asset := range required {
		archivePath := filepath.Join(work, asset.Name)
		if err := downloadReleaseAsset(ctx, client, settings, repository, tag, asset, archivePath); err != nil {
			return err
		}
		assetStage := filepath.Join(staging, asset.InstallDir)
		if err := untarBundle(archivePath, assetStage); err != nil {
			return fmt.Errorf("extract %s: %w", asset.Name, err)
		}
		source := filepath.Join(assetStage, asset.InstallDir)
		if info, err := os.Stat(source); err != nil || !info.IsDir() {
			if err != nil {
				return fmt.Errorf("%s does not contain %s/: %w", asset.Name, asset.InstallDir, err)
			}
			return fmt.Errorf("%s path %s is not a directory", asset.Name, asset.InstallDir)
		}
	}
	return activateRelease(destination, staging, required)
}

func downloadReleaseAsset(ctx context.Context, client *http.Client, settings DownloadSettings, repository, tag string, asset releaseAsset, destination string) error {
	downloadURL := settings.rewrite(fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repository, tag, asset.Name))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: GitHub returned %s", asset.Name, response.Status)
	}

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	digest := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, digest), io.LimitReader(response.Body, asset.Size+1))
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("download %s: %w", asset.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", asset.Name, closeErr)
	}
	if written != asset.Size {
		return fmt.Errorf("%s size mismatch: expected %d, received %d", asset.Name, asset.Size, written)
	}
	if actual := hex.EncodeToString(digest.Sum(nil)); actual != asset.SHA256 {
		return fmt.Errorf("%s SHA-256 mismatch", asset.Name)
	}
	return nil
}

// activateRelease switches web, backend, and lib as a transaction. A failed
// rename restores all previous directories before returning.
func activateRelease(destination, staging string, assets []releaseAsset) error {
	type move struct {
		name, source, final, backup string
		hadPrevious, activated      bool
	}
	moves := make([]move, 0, len(assets))
	for _, asset := range assets {
		moves = append(moves, move{
			name:   asset.InstallDir,
			source: filepath.Join(staging, asset.InstallDir, asset.InstallDir),
			final:  filepath.Join(destination, asset.InstallDir),
			backup: filepath.Join(staging, "previous-"+asset.InstallDir),
		})
	}
	rollback := func() {
		for index := len(moves) - 1; index >= 0; index-- {
			item := &moves[index]
			if item.activated {
				_ = os.RemoveAll(item.final)
			}
			if item.hadPrevious {
				_ = os.Rename(item.backup, item.final)
			}
		}
	}
	for index := range moves {
		item := &moves[index]
		if _, err := os.Stat(item.final); err == nil {
			if err := os.Rename(item.final, item.backup); err != nil {
				rollback()
				return fmt.Errorf("stage previous %s: %w", item.name, err)
			}
			item.hadPrevious = true
		} else if !os.IsNotExist(err) {
			rollback()
			return err
		}
		if err := os.Rename(item.source, item.final); err != nil {
			rollback()
			return fmt.Errorf("activate %s: %w", item.name, err)
		}
		item.activated = true
	}
	return nil
}

func untarBundle(archivePath, destination string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open gzip stream: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		relative, err := safeArchivePath(header.Name)
		if err != nil {
			return fmt.Errorf("unsafe archive path %q: %w", header.Name, err)
		}
		output := filepath.Join(destination, relative)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := ensureArchiveParents(destination, output); err != nil {
				return err
			}
			if info, err := os.Lstat(output); err == nil && !info.IsDir() {
				return fmt.Errorf("directory path is already not a directory: %q", header.Name)
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(output, os.FileMode(header.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := ensureArchiveParents(destination, output); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode)&0o777)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		case tar.TypeSymlink:
			if runtime.GOOS == "windows" {
				return fmt.Errorf("symbolic link is not permitted in Windows archive: %q", header.Name)
			}
			if err := ensureArchiveParents(destination, output); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
				return err
			}
			if err := safeSymlink(destination, output, header.Linkname); err != nil {
				return fmt.Errorf("unsafe symbolic link %q -> %q: %w", header.Name, header.Linkname, err)
			}
		case tar.TypeLink:
			targetRelative, err := safeArchivePath(header.Linkname)
			if err != nil {
				return fmt.Errorf("unsafe hard link %q -> %q: %w", header.Name, header.Linkname, err)
			}
			target := filepath.Join(destination, targetRelative)
			info, err := os.Lstat(target)
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("hard link target is not an existing regular file: %q", header.Linkname)
			}
			if err := ensureArchiveParents(destination, output); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
				return err
			}
			if err := os.Link(target, output); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry type for %q", header.Name)
		}
	}
}

func safeArchivePath(value string) (string, error) {
	if value == "" || strings.Contains(value, "\\") {
		return "", errors.New("empty or backslash path")
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", errors.New("path escapes archive root")
	}
	return filepath.FromSlash(clean), nil
}

func safeSymlink(destination, output, target string) error {
	if target == "" || strings.Contains(target, "\x00") || filepath.IsAbs(target) {
		return errors.New("empty, absolute, or null-containing target")
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(output), filepath.FromSlash(target)))
	inside, err := filepath.Rel(destination, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return errors.New("target escapes archive root")
	}
	return os.Symlink(target, output)
}

func ensureArchiveParents(destination, output string) error {
	relative, err := filepath.Rel(destination, filepath.Dir(output))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return errors.New("archive path escapes destination")
	}
	current := destination
	if relative == "." {
		return nil
	}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("archive parent %s is not a real directory", current)
		}
	}
	return nil
}

func validRepository(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(value, "\\ :?&#")
}
