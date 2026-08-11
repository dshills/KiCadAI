package corpuspublication

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const v6ContractMaximumFileSize = 32 << 20

// VerifyV6ContractManifest verifies the frozen V6 contract manifest relative
// to its own directory while confining every resolved entry to repositoryRoot.
// V6 intentionally binds ../../internal paths; the V5 artifact verifier
// correctly remains stricter for publication trees that never need them.
func VerifyV6ContractManifest(repositoryRoot, manifestPath string) ([]byte, error) {
	repositoryRoot, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve V6 contract repository: %w", err)
	}
	repositoryRoot, err = filepath.EvalSymlinks(repositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve real V6 contract repository: %w", err)
	}
	manifestPath, err = canonicalFuturePath(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("resolve V6 contract manifest: %w", err)
	}
	if !pathWithin(repositoryRoot, manifestPath) {
		return nil, fmt.Errorf("V6 contract manifest is outside the repository")
	}
	manifestBytes, err := readRegularFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read V6 contract manifest: %w", err)
	}

	base := filepath.Dir(manifestPath)
	scanner := bufio.NewScanner(bytes.NewReader(manifestBytes))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	seenRelative, seenTarget := map[string]bool{}, map[string]bool{}
	entries := 0
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return nil, fmt.Errorf("V6 contract manifest entry at line %d is invalid: %q", lineNumber, line)
		}
		digest, relative := line[:64], line[66:]
		local := filepath.FromSlash(relative)
		if !validSHA256(digest) || relative == "" || relative == "." || filepath.IsAbs(local) ||
			relative != filepath.ToSlash(filepath.Clean(local)) || seenRelative[relative] {
			return nil, fmt.Errorf("V6 contract manifest entry at line %d is invalid: %q", lineNumber, line)
		}
		seenRelative[relative] = true
		candidate := filepath.Clean(filepath.Join(base, local))
		if !pathWithin(repositoryRoot, candidate) {
			return nil, fmt.Errorf("V6 contract entry %q at line %d escapes the repository", relative, lineNumber)
		}
		info, err := os.Lstat(candidate)
		if err != nil {
			return nil, fmt.Errorf("inspect V6 contract entry %q at line %d: %w", relative, lineNumber, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("V6 contract entry %q at line %d is non-regular or symbolic", relative, lineNumber)
		}
		realPath, err := canonicalFuturePath(candidate)
		if err != nil {
			return nil, fmt.Errorf("resolve V6 contract entry %q at line %d: %w", relative, lineNumber, err)
		}
		if !pathWithin(repositoryRoot, realPath) {
			return nil, fmt.Errorf("V6 contract entry %q at line %d escapes the repository", relative, lineNumber)
		}
		if seenTarget[realPath] {
			return nil, fmt.Errorf("V6 contract entry %q at line %d aliases one target more than once", relative, lineNumber)
		}
		seenTarget[realPath] = true
		actual, err := hashV6ContractFile(candidate, info, v6ContractMaximumFileSize)
		if err != nil {
			return nil, fmt.Errorf("hash V6 contract entry %q at line %d: %w", relative, lineNumber, err)
		}
		if actual != digest {
			return nil, fmt.Errorf("V6 contract entry %q at line %d does not match its commitment", relative, lineNumber)
		}
		entries++
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan V6 contract manifest: %w", err)
	}
	if entries == 0 {
		return nil, fmt.Errorf("V6 contract manifest is empty")
	}
	return manifestBytes, nil
}

func hashV6ContractFile(path string, expected os.FileInfo, maximum int64) (string, error) {
	if expected.Size() < 0 || expected.Size() > maximum {
		return "", fmt.Errorf("checksum source is not a bounded regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open checksum source: %w", err)
	}
	opened, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return "", fmt.Errorf("inspect opened checksum source: %w", statErr)
	}
	if !os.SameFile(expected, opened) || opened.Size() != expected.Size() || opened.ModTime() != expected.ModTime() {
		_ = file.Close()
		return "", fmt.Errorf("checksum source changed before hashing")
	}

	hash := sha256.New()
	written, copyErr := io.Copy(hash, file)
	after, finalStatErr := file.Stat()
	closeErr := file.Close()
	if copyErr != nil {
		return "", fmt.Errorf("read checksum source: %w", copyErr)
	}
	if finalStatErr != nil {
		return "", fmt.Errorf("inspect checksum source after hashing: %w", finalStatErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close checksum source: %w", closeErr)
	}
	if !os.SameFile(opened, after) || written != opened.Size() || after.Size() != opened.Size() || after.ModTime() != opened.ModTime() {
		return "", fmt.Errorf("checksum source changed while hashing")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
