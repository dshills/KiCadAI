package promotiontoolchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type BootstrapOptions struct {
	OS       string
	Arch     string
	CacheDir string
	Client   *http.Client
}

func BootstrapToolchain(ctx context.Context, document Document, options BootstrapOptions) (Evidence, error) {
	osName := options.OS
	if osName == "" {
		osName = runtime.GOOS
	}
	arch := options.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	platform, err := document.Platform(osName, arch)
	if err != nil {
		return Evidence{}, err
	}
	if platform.Bootstrap.Kind != "dmg" || osName != "darwin" {
		return Evidence{}, fmt.Errorf("bootstrap kind %q is not supported on %s", platform.Bootstrap.Kind, osName)
	}
	cacheDir := options.CacheDir
	if cacheDir == "" {
		userCache, cacheErr := os.UserCacheDir()
		if cacheErr != nil {
			return Evidence{}, fmt.Errorf("resolve user cache: %w", cacheErr)
		}
		cacheDir = filepath.Join(userCache, "kicadai", "toolchains")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return Evidence{}, fmt.Errorf("create toolchain cache: %w", err)
	}
	target := filepath.Join(cacheDir, "kicad-"+document.Lock.KiCadVersion+"-"+osName+"-"+arch)
	if evidence, resolveErr := ResolveRoot(ctx, document, target, osName, arch, "bootstrap_cache"); resolveErr == nil {
		return evidence, nil
	}
	unlock, err := acquireFileLock(ctx, target+".lock")
	if err != nil {
		return Evidence{}, err
	}
	defer unlock()
	if evidence, resolveErr := ResolveRoot(ctx, document, target, osName, arch, "bootstrap_cache"); resolveErr == nil {
		return evidence, nil
	}
	if _, statErr := os.Lstat(target); statErr == nil {
		return Evidence{}, fmt.Errorf("bootstrap cache target %q exists but failed locked validation; refusing to overwrite it", target)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Evidence{}, fmt.Errorf("inspect bootstrap cache target: %w", statErr)
	}
	temporary, err := os.MkdirTemp(cacheDir, ".kicad-bootstrap-*")
	if err != nil {
		return Evidence{}, fmt.Errorf("create bootstrap workspace: %w", err)
	}
	defer os.RemoveAll(temporary)
	archivePath := filepath.Join(temporary, "toolchain.dmg")
	if err := DownloadVerified(ctx, options.Client, platform.Bootstrap, archivePath); err != nil {
		return Evidence{}, err
	}
	installRoot := filepath.Join(temporary, "install")
	if err := installDMG(ctx, archivePath, installRoot); err != nil {
		return Evidence{}, err
	}
	if err := os.Rename(installRoot, target); err != nil {
		return Evidence{}, fmt.Errorf("publish bootstrapped toolchain: %w", err)
	}
	return ResolveRoot(ctx, document, target, osName, arch, "bootstrap_cache")
}

func DownloadVerified(ctx context.Context, client *http.Client, distribution Bootstrap, destination string) error {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Minute}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, distribution.URL, nil)
	if err != nil {
		return fmt.Errorf("build bootstrap request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download bootstrap distribution: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download bootstrap distribution: HTTP %s", response.Status)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create bootstrap download: %w", err)
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(output, hash), io.LimitReader(response.Body, distribution.SizeBytes+1))
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("download bootstrap distribution: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close bootstrap distribution: %w", closeErr)
	}
	if written != distribution.SizeBytes {
		return fmt.Errorf("bootstrap distribution size %d does not match lock %d", written, distribution.SizeBytes)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != distribution.SHA256 {
		return fmt.Errorf("bootstrap distribution sha256 %s does not match lock %s", actual, distribution.SHA256)
	}
	return nil
}

func installDMG(ctx context.Context, archivePath, installRoot string) error {
	mountRoot := filepath.Join(filepath.Dir(archivePath), "mount")
	if err := os.Mkdir(mountRoot, 0o700); err != nil {
		return fmt.Errorf("create DMG mount root: %w", err)
	}
	attach := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mountRoot, archivePath)
	if output, err := attach.CombinedOutput(); err != nil {
		return fmt.Errorf("attach KiCad DMG: %w: %s", err, strings.TrimSpace(string(output)))
	}
	defer func() {
		detachContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = exec.CommandContext(detachContext, "hdiutil", "detach", mountRoot).Run()
	}()
	var applications []string
	err := filepath.WalkDir(mountRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == "KiCad.app" {
			applications = append(applications, path)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("inspect KiCad DMG: %w", err)
	}
	if len(applications) == 0 {
		return errors.New("KiCad DMG does not contain KiCad.app")
	}
	if len(applications) != 1 {
		return fmt.Errorf("KiCad DMG contains %d KiCad.app directories", len(applications))
	}
	if err := os.Mkdir(installRoot, 0o755); err != nil {
		return fmt.Errorf("create bootstrap install root: %w", err)
	}
	copyCommand := exec.CommandContext(ctx, "ditto", applications[0], filepath.Join(installRoot, "KiCad.app"))
	if output, err := copyCommand.CombinedOutput(); err != nil {
		return fmt.Errorf("copy KiCad application: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
