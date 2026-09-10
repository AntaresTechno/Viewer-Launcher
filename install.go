package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// InstallTree downloads a GitHub ref archive over HTTPS, validates every ZIP
// path, and atomically replaces destinationName with sourceName. For the
// Viewer backend, ref is the immutable commit recorded by lib/lib.json.
func InstallTree(ctx context.Context, destination, repository, ref, sourceName, destinationName string) error {
	if !validRepository(repository) {
		return fmt.Errorf("invalid GitHub repository %q", repository)
	}
	if !validGitHubRef(ref) {
		return fmt.Errorf("invalid GitHub ref %q", ref)
	}
	url := fmt.Sprintf("https://codeload.github.com/%s/zip/%s", repository, ref)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", ref, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: GitHub returned %s", ref, response.Status)
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
	// The web and lib workflows publish one named directory below GitHub's
	// generated top-level archive directory. Do not accept an arbitrary layout.
	source := filepath.Join(staging, sourceName)
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("ref %s does not contain required %s/ directory", ref, sourceName)
	}
	final := filepath.Join(destination, destinationName)
	backup := final + ".previous"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(final); err == nil {
		if err := os.Rename(final, backup); err != nil {
			return fmt.Errorf("stage previous %s: %w", destinationName, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	if err := os.Rename(source, final); err != nil {
		_ = os.Rename(backup, final)
		return fmt.Errorf("activate %s: %w", destinationName, err)
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

var validRef = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
var commitHash = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validGitHubRef(value string) bool {
	return validRef.MatchString(value) && !strings.Contains(value, "..")
}
