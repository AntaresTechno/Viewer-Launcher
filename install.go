package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InstallBranch downloads a GitHub branch archive over HTTPS, validates every
// ZIP path, and atomically replaces targetName.  Branches are generated only
// by this repository's reviewed GitHub Actions workflows.
func InstallBranch(ctx context.Context, destination, repository, branch, targetName string) error {
	if !validRepository(repository) {
		return fmt.Errorf("invalid GitHub repository %q", repository)
	}
	url := fmt.Sprintf("https://codeload.github.com/%s/zip/refs/heads/%s", repository, branch)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", branch, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: GitHub returned %s", branch, response.Status)
	}
	work, err := os.MkdirTemp(destination, ".viewer-download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	archivePath := filepath.Join(work, "branch.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if _, err = io.Copy(archive, io.LimitReader(response.Body, 2<<30)); err != nil {
		archive.Close()
		return err
	}
	if err = archive.Close(); err != nil {
		return err
	}
	staging := filepath.Join(work, "staging")
	if err := unzipBranch(archivePath, staging); err != nil {
		return err
	}
	// Workflows publish runtime/ and dist/ at the archive root below GitHub's
	// generated top-level directory.  Do not accept an arbitrary layout.
	source := filepath.Join(staging, targetName)
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("branch %s does not contain required %s/ directory", branch, targetName)
	}
	if targetName == "dist" {
		source = filepath.Join(staging, "dist")
		targetName = filepath.Join("frontend", "dist")
	}
	final := filepath.Join(destination, targetName)
	backup := final + ".previous"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, backup); err != nil {
			return fmt.Errorf("stage previous %s: %w", targetName, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	if err := os.Rename(source, final); err != nil {
		_ = os.Rename(backup, final)
		return fmt.Errorf("activate %s: %w", targetName, err)
	}
	_ = os.RemoveAll(backup)
	return nil
}

func unzipBranch(archivePath, destination string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	archiveRoot := ""
	for _, entry := range zr.File {
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link is not permitted in archive: %q", entry.Name)
		}
		parts := strings.Split(filepath.ToSlash(entry.Name), "/")
		if len(parts) < 2 || parts[0] == "" {
			return fmt.Errorf("unexpected archive entry %q", entry.Name)
		}
		if archiveRoot == "" {
			archiveRoot = parts[0]
		} else if parts[0] != archiveRoot {
			return fmt.Errorf("archive has multiple top-level roots (%q and %q)", archiveRoot, parts[0])
		}
		rel := filepath.Join(parts[1:]...)
		if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("unsafe archive path %q", entry.Name)
		}
		path := filepath.Join(destination, rel)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		in, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, entry.Mode())
		if err == nil {
			_, err = io.Copy(out, io.LimitReader(in, 1<<30))
			closeErr := out.Close()
			if err == nil {
				err = closeErr
			}
		}
		in.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func validRepository(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(value, "\\ :?&#")
}
