package main

import (
	"archive/tar"
	"archive/zip"
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
	"regexp"
	"runtime"
	"strings"
)

// InstallTree downloads a GitHub ref archive over HTTPS, validates every ZIP
// path, and atomically replaces destinationName with sourceName. For the
// Viewer backend, ref is the immutable commit recorded by lib/lib.json.
func InstallTree(ctx context.Context, client *http.Client, settings DownloadSettings, destination, repository, ref, sourceName, destinationName string) error {
	if !validRepository(repository) {
		return fmt.Errorf("invalid GitHub repository %q", repository)
	}
	if !validGitHubRef(ref) {
		return fmt.Errorf("invalid GitHub ref %q", ref)
	}
	downloadURL := settings.rewrite(fmt.Sprintf("https://codeload.github.com/%s/zip/%s", repository, ref))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
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
	return activateDirectory(destination, source, destinationName)
}

const splitBundleFormat = "tar.gz-split-v1"

var (
	sha256Hash = regexp.MustCompile(`^[0-9a-f]{64}$`)
	partName   = regexp.MustCompile(`^bundle\.tar\.gz\.part-[0-9]{3,}$`)
)

func (manifest libManifest) hasBundleMetadata() bool {
	return manifest.BundleFormat != "" || manifest.BundleSHA256 != "" || manifest.BundleSize != 0 || len(manifest.Parts) != 0
}

func (manifest libManifest) usesSplitBundle() bool {
	return manifest.BundleFormat == splitBundleFormat
}

func (manifest libManifest) validateBundle() error {
	if manifest.BundleFormat != splitBundleFormat {
		return fmt.Errorf("unsupported lib bundle format %q", manifest.BundleFormat)
	}
	if manifest.BundleSize <= 0 || !sha256Hash.MatchString(manifest.BundleSHA256) {
		return errors.New("lib.json has an invalid bundle checksum or size")
	}
	if len(manifest.Parts) == 0 {
		return errors.New("lib.json has no bundle parts")
	}
	var total int64
	previous := ""
	for _, part := range manifest.Parts {
		if !partName.MatchString(part.Name) || !sha256Hash.MatchString(part.SHA256) || part.Size <= 0 {
			return errors.New("lib.json contains an invalid bundle part")
		}
		if previous != "" && part.Name <= previous {
			return errors.New("lib.json bundle parts are not in a unique ascending order")
		}
		if total > manifest.BundleSize-part.Size {
			return errors.New("lib.json bundle parts exceed the declared size")
		}
		total += part.Size
		previous = part.Name
	}
	if total != manifest.BundleSize {
		return errors.New("lib.json bundle part sizes do not match the declared size")
	}
	return nil
}

// InstallSplitLib downloads only the current platform's Git-sized archive parts,
// verifies both per-part and complete-bundle checksums, then atomically installs it.
func InstallSplitLib(ctx context.Context, client *http.Client, settings DownloadSettings, destination, repository, platform string, manifest libManifest) error {
	if !validRepository(repository) {
		return fmt.Errorf("invalid GitHub repository %q", repository)
	}
	if err := manifest.validateBundle(); err != nil {
		return err
	}
	work, err := os.MkdirTemp(destination, ".viewer-lib-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	archivePath := filepath.Join(work, "bundle.tar.gz")
	archive, err := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	archiveHash := sha256.New()
	writeArchive := io.MultiWriter(archive, archiveHash)
	for _, part := range manifest.Parts {
		partURL := settings.rewrite("https://raw.githubusercontent.com/" + repository + "/lib/lib/" + platform + "/" + part.Name)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, partURL, nil)
		if err != nil {
			archive.Close()
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			archive.Close()
			return fmt.Errorf("download lib bundle part %s: %w", part.Name, err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			archive.Close()
			return fmt.Errorf("download lib bundle part %s: GitHub returned %s", part.Name, response.Status)
		}
		partHash := sha256.New()
		copied, copyErr := io.Copy(writeArchive, io.TeeReader(io.LimitReader(response.Body, part.Size+1), partHash))
		closeErr := response.Body.Close()
		if copyErr != nil {
			archive.Close()
			return fmt.Errorf("download lib bundle part %s: %w", part.Name, copyErr)
		}
		if closeErr != nil {
			archive.Close()
			return fmt.Errorf("close lib bundle part %s: %w", part.Name, closeErr)
		}
		if copied != part.Size || hex.EncodeToString(partHash.Sum(nil)) != part.SHA256 {
			archive.Close()
			return fmt.Errorf("lib bundle part %s failed its checksum or size check", part.Name)
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if hex.EncodeToString(archiveHash.Sum(nil)) != manifest.BundleSHA256 {
		return errors.New("combined lib bundle failed its checksum check")
	}
	staging := filepath.Join(work, "staging")
	if err := untarBundle(archivePath, staging); err != nil {
		return err
	}
	source := filepath.Join(staging, platform)
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		if err != nil {
			return fmt.Errorf("lib bundle has no %s directory: %w", platform, err)
		}
		return fmt.Errorf("lib bundle path %s is not a directory", platform)
	}
	return activateDirectory(destination, source, "lib")
}

func activateDirectory(destination, source, destinationName string) error {
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
		if entry.Mode()&os.ModeSymlink != 0 {
			if runtime.GOOS == "windows" {
				return fmt.Errorf("symbolic link is not permitted in Windows archive: %q", entry.Name)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			in, err := entry.Open()
			if err != nil {
				return err
			}
			targetBytes, readErr := io.ReadAll(io.LimitReader(in, 4096))
			in.Close()
			if readErr != nil {
				return readErr
			}
			target := string(targetBytes)
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), target))
			inside, relErr := filepath.Rel(destination, resolved)
			if target == "" || strings.Contains(target, "\x00") || filepath.IsAbs(target) || relErr != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
				return fmt.Errorf("unsafe symbolic link %q -> %q", entry.Name, target)
			}
			if err := os.Symlink(target, path); err != nil {
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

func untarBundle(archivePath, destination string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	compressed, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("open lib bundle gzip stream: %w", err)
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read lib bundle: %w", err)
		}
		relative, err := safeArchivePath(header.Name)
		if err != nil {
			return fmt.Errorf("unsafe lib bundle path %q: %w", header.Name, err)
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
		default:
			return fmt.Errorf("unsupported lib bundle entry type for %q", header.Name)
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

var validRef = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
var commitHash = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validGitHubRef(value string) bool {
	return validRef.MatchString(value) && !strings.Contains(value, "..")
}
