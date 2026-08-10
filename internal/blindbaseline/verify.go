package blindbaseline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

func Verify(root string) (Manifest, error) {
	want := []string{AuditFile, ChecksumFile, CipherFile, ManifestFile}
	entries, err := os.ReadDir(root)
	if err != nil {
		return Manifest{}, err
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			return Manifest{}, fmt.Errorf("held-out baseline contains a non-regular entry")
		}
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		return Manifest{}, fmt.Errorf("held-out baseline file set is invalid")
	}
	artifacts := make(map[string][]byte, len(want))
	for _, name := range want {
		maximum := int64(1 << 20)
		if name == CipherFile {
			maximum = maximumPayloadSize + 1<<20
		}
		artifacts[name], err = readBounded(filepath.Join(root, name), maximum)
		if err != nil {
			return Manifest{}, err
		}
	}
	checksums := artifacts[ChecksumFile]
	if err := verifyChecksums(checksums, artifacts); err != nil {
		return Manifest{}, err
	}
	manifestBytes := artifacts[ManifestFile]
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode held-out baseline manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("decode held-out baseline manifest: trailing JSON value")
		}
		return Manifest{}, fmt.Errorf("decode held-out baseline manifest trailing data: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	ciphertext := artifacts[CipherFile]
	// CHECKSUMS.sha256 detects accidental file drift, while this independently
	// binds ciphertext to the self-hashed authenticated manifest. Neither layer
	// substitutes for the other.
	if hashBytes(ciphertext) != manifest.CiphertextSHA256 {
		return Manifest{}, fmt.Errorf("held-out baseline ciphertext is invalid")
	}
	audit := artifacts[AuditFile]
	if !bytes.Equal(audit, baselineAudit(manifest)) {
		return Manifest{}, fmt.Errorf("held-out baseline audit is invalid")
	}
	manifestCanonical, err := marshalStable(manifest)
	if err != nil {
		return Manifest{}, err
	}
	// Publication is intentionally canonical and exact-byte reproducible. A
	// semantically equivalent JSON rewrite is not an admissible sealed bundle:
	// it must be produced by marshalStable so checksums remain normative.
	canonicalArtifacts := map[string][]byte{AuditFile: audit, CipherFile: ciphertext, ManifestFile: manifestCanonical}
	if !bytes.Equal(checksums, checksumManifest(canonicalArtifacts)) {
		return Manifest{}, fmt.Errorf("held-out baseline checksum manifest is noncanonical")
	}
	return manifest, nil
}

func verifyChecksums(data []byte, artifacts map[string][]byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	seen := map[string]bool{}
	for scanner.Scan() {
		digest, path, found := strings.Cut(scanner.Text(), "  ")
		if !found || !hashPattern.MatchString(digest) || path == ChecksumFile || filepath.Base(path) != path || seen[path] {
			return fmt.Errorf("held-out baseline checksum entry is invalid")
		}
		seen[path] = true
		artifact, ok := artifacts[path]
		if !ok || hashBytes(artifact) != digest {
			return fmt.Errorf("held-out baseline checksum mismatch")
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(seen) != 3 || !seen[AuditFile] || !seen[CipherFile] || !seen[ManifestFile] {
		return fmt.Errorf("held-out baseline checksums are incomplete")
	}
	return nil
}
