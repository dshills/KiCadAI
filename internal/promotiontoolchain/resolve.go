package promotiontoolchain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

type ResolveOptions struct {
	OS             string
	Arch           string
	KiCadCLI       string
	SymbolsRoot    string
	FootprintsRoot string
	Getenv         func(string) string
	VersionTimeout time.Duration
}

type candidate struct {
	cli        string
	symbols    string
	footprints string
	resolution string
}

func Resolve(ctx context.Context, document Document, options ResolveOptions) (Evidence, error) {
	osName := strings.TrimSpace(options.OS)
	if osName == "" {
		osName = runtime.GOOS
	}
	arch := strings.TrimSpace(options.Arch)
	if arch == "" {
		arch = runtime.GOARCH
	}
	platform, err := document.Platform(osName, arch)
	if err != nil {
		return Evidence{}, err
	}
	getenv := options.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	candidates, err := resolveCandidates(document, platform, options, getenv)
	if err != nil {
		return Evidence{}, err
	}
	var failures []string
	for _, item := range candidates {
		evidence, candidateErr := validateCandidate(ctx, document, platform, item, options.VersionTimeout)
		if candidateErr == nil {
			return evidence, nil
		}
		failures = append(failures, item.resolution+": "+candidateErr.Error())
	}
	return Evidence{}, fmt.Errorf("no locked KiCad toolchain candidate passed: %s", strings.Join(failures, "; "))
}

func ResolveRoot(ctx context.Context, document Document, root, osName, arch, resolution string) (Evidence, error) {
	platform, err := document.Platform(osName, arch)
	if err != nil {
		return Evidence{}, err
	}
	return validateCandidate(ctx, document, platform, candidate{
		cli: filepath.Join(root, platform.KiCadCLI), symbols: filepath.Join(root, platform.SymbolsRoot),
		footprints: filepath.Join(root, platform.FootprintsRoot), resolution: resolution,
	}, 0)
}

func resolveCandidates(document Document, platform Platform, options ResolveOptions, getenv func(string) string) ([]candidate, error) {
	explicit := []string{options.KiCadCLI, options.SymbolsRoot, options.FootprintsRoot}
	if countNonEmpty(explicit) != 0 {
		if countNonEmpty(explicit) != len(explicit) {
			return nil, errors.New("explicit kicad-cli, symbols root, and footprints root must be provided together")
		}
		return []candidate{{cli: explicit[0], symbols: explicit[1], footprints: explicit[2], resolution: "explicit"}}, nil
	}
	env := []string{
		strings.TrimSpace(getenv(document.Lock.Environment.KiCadCLI)),
		strings.TrimSpace(getenv(document.Lock.Environment.SymbolsRoot)),
		strings.TrimSpace(getenv(document.Lock.Environment.FootprintsRoot)),
	}
	if countNonEmpty(env) != 0 {
		if countNonEmpty(env) != len(env) {
			return nil, errors.New("toolchain environment paths must be provided together")
		}
		return []candidate{{cli: env[0], symbols: env[1], footprints: env[2], resolution: "environment"}}, nil
	}
	result := make([]candidate, 0, len(platform.TrustedRoots))
	for _, root := range platform.TrustedRoots {
		result = append(result, candidate{
			cli: filepath.Join(root, platform.KiCadCLI), symbols: filepath.Join(root, platform.SymbolsRoot),
			footprints: filepath.Join(root, platform.FootprintsRoot), resolution: "trusted_root",
		})
	}
	return result, nil
}

func countNonEmpty(values []string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

func validateCandidate(ctx context.Context, document Document, platform Platform, item candidate, timeout time.Duration) (Evidence, error) {
	cli, err := canonicalExecutable(item.cli)
	if err != nil {
		return Evidence{}, err
	}
	symbols, err := canonicalDirectory(item.symbols)
	if err != nil {
		return Evidence{}, fmt.Errorf("symbols root: %w", err)
	}
	footprints, err := canonicalDirectory(item.footprints)
	if err != nil {
		return Evidence{}, fmt.Errorf("footprints root: %w", err)
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	versionContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := exec.CommandContext(versionContext, cli, "--version").CombinedOutput()
	if err != nil {
		return Evidence{}, fmt.Errorf("run kicad-cli --version: %w", err)
	}
	version := strings.TrimSpace(string(output))
	if version != document.Lock.KiCadVersion {
		return Evidence{}, fmt.Errorf("kicad-cli version %q does not match lock %q", version, document.Lock.KiCadVersion)
	}
	symbolIdentity, err := HashLibrary(symbols)
	if err != nil {
		return Evidence{}, fmt.Errorf("hash symbols root: %w", err)
	}
	footprintIdentity, err := HashLibrary(footprints)
	if err != nil {
		return Evidence{}, fmt.Errorf("hash footprints root: %w", err)
	}
	return Evidence{
		Schema: EvidenceSchema, Version: 1, LockSHA256: document.SHA256,
		OS: platform.OS, Arch: platform.Arch, KiCadVersion: version,
		KiCadCLI: cli, SymbolsRoot: symbols, FootprintsRoot: footprints,
		SymbolsIdentity: symbolIdentity, FootprintsIdentity: footprintIdentity,
		Resolution: item.resolution,
	}, nil
}

func canonicalExecutable(path string) (string, error) {
	resolved, err := canonicalPath(path)
	if err != nil {
		return "", fmt.Errorf("kicad-cli: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("kicad-cli: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("kicad-cli %q is not an executable regular file", resolved)
	}
	return resolved, nil
}

func canonicalDirectory(path string) (string, error) {
	resolved, err := canonicalPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", resolved)
	}
	return resolved, nil
}

func canonicalPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

type libraryFile struct {
	absolute string
	relative string
	size     int64
}

func HashLibrary(root string) (LibraryIdentity, error) {
	files, err := collectLibraryFiles(root)
	if err != nil {
		return LibraryIdentity{}, err
	}
	return hashLibraryFiles(files)
}

func collectLibraryFiles(root string) ([]libraryFile, error) {
	var files []libraryFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("library contains symbolic link %q", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("library contains non-regular file %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, libraryFile{absolute: path, relative: filepath.ToSlash(relative), size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(files, func(left, right libraryFile) int {
		return strings.Compare(left.relative, right.relative)
	})
	return files, nil
}

func hashLibraryFiles(files []libraryFile) (LibraryIdentity, error) {
	aggregate := sha256.New()
	var byteCount int64
	for _, file := range files {
		input, err := os.Open(file.absolute)
		if err != nil {
			return LibraryIdentity{}, err
		}
		contentHash := sha256.New()
		_, copyErr := io.Copy(contentHash, input)
		closeErr := input.Close()
		if copyErr != nil {
			return LibraryIdentity{}, copyErr
		}
		if closeErr != nil {
			return LibraryIdentity{}, closeErr
		}
		_, _ = io.WriteString(aggregate, file.relative)
		_, _ = io.WriteString(aggregate, "\x00"+strconv.FormatInt(file.size, 10)+"\x00")
		_, _ = io.WriteString(aggregate, hex.EncodeToString(contentHash.Sum(nil))+"\n")
		byteCount += file.size
	}
	return LibraryIdentity{
		SHA256: hex.EncodeToString(aggregate.Sum(nil)), FileCount: len(files), ByteCount: byteCount,
	}, nil
}
