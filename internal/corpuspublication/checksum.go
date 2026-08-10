package corpuspublication

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func VerifyChecksumManifest(baseRoot, manifestPath string) ([]byte, error) {
	baseRoot, err := filepath.Abs(filepath.Clean(baseRoot))
	if err != nil {
		return nil, fmt.Errorf("resolve checksum base: %w", err)
	}
	baseRoot, err = filepath.EvalSymlinks(baseRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve real checksum base: %w", err)
	}
	manifestPath, err = canonicalFuturePath(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("resolve checksum manifest: %w", err)
	}
	if !pathWithin(baseRoot, manifestPath) {
		return nil, fmt.Errorf("checksum manifest is outside its base")
	}
	manifestBytes, err := readRegularFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read checksum manifest: %w", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(manifestBytes)))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	seen := map[string]bool{}
	entries := 0
	for scanner.Scan() {
		line := scanner.Text()
		digest, relative, ok := strings.Cut(line, "  ")
		if !ok || !validSHA256(digest) || !validRelativeArtifactPath(relative) || seen[relative] {
			return nil, fmt.Errorf("checksum manifest contains an invalid entry")
		}
		seen[relative] = true
		entries++
		path := filepath.Join(baseRoot, filepath.FromSlash(relative))
		entryInfo, err := os.Lstat(path)
		if err != nil || entryInfo.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("checksum entry is unavailable or symbolic")
		}
		realPath, err := canonicalFuturePath(path)
		if err != nil || !pathWithin(baseRoot, realPath) {
			return nil, fmt.Errorf("checksum entry escapes its base")
		}
		actual, err := hashRegularFile(realPath, 32<<20)
		if err != nil {
			return nil, err
		}
		if actual != digest {
			return nil, fmt.Errorf("checksum entry does not match its commitment")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan checksum manifest: %w", err)
	}
	if entries == 0 {
		return nil, fmt.Errorf("checksum manifest is empty")
	}
	return manifestBytes, nil
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, fmt.Errorf("file is not a bounded regular file")
	}
	return os.ReadFile(path)
}
