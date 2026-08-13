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

// VerifyV10 validates the exact bounded public artifact set without reading the
// external key or opening any held-out evidence.
func VerifyV10(root string) (V10Manifest, error) {
	want := []string{AuditFile, ChecksumFile, CipherFileV10, ManifestFile}
	sort.Strings(want)
	entries, err := os.ReadDir(root)
	if err != nil {
		return V10Manifest{}, err
	}
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			return V10Manifest{}, fmt.Errorf("V10 held-out baseline contains a non-regular entry")
		}
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		return V10Manifest{}, fmt.Errorf("V10 held-out baseline file set is invalid")
	}
	artifacts := make(map[string][]byte, len(want))
	for _, name := range want {
		maximum := int64(1 << 20)
		if name == CipherFileV10 {
			maximum = maximumPayloadSize + 2<<20
		}
		artifacts[name], err = readBounded(filepath.Join(root, name), maximum)
		if err != nil {
			return V10Manifest{}, err
		}
	}
	if err := verifyChecksumsV10(artifacts[ChecksumFile], artifacts); err != nil {
		return V10Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(artifacts[ManifestFile]))
	decoder.DisallowUnknownFields()
	var manifest V10Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return V10Manifest{}, fmt.Errorf("decode V10 held-out baseline manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return V10Manifest{}, fmt.Errorf("decode V10 held-out baseline manifest trailing data")
	}
	if err := validateManifestV10(manifest); err != nil {
		return V10Manifest{}, err
	}
	if hashBytes(artifacts[CipherFileV10]) != manifest.CiphertextSHA256 || !bytes.Equal(artifacts[AuditFile], baselineAuditV10(manifest)) {
		return V10Manifest{}, fmt.Errorf("V10 held-out baseline public commitment is invalid")
	}
	canonicalManifest, err := marshalStable(manifest)
	if err != nil {
		return V10Manifest{}, err
	}
	canonicalArtifacts := map[string][]byte{AuditFile: artifacts[AuditFile], CipherFileV10: artifacts[CipherFileV10], ManifestFile: canonicalManifest}
	if !bytes.Equal(artifacts[ChecksumFile], checksumManifest(canonicalArtifacts)) {
		return V10Manifest{}, fmt.Errorf("V10 held-out baseline checksum manifest is noncanonical")
	}
	return manifest, nil
}

func verifyChecksumsV10(data []byte, artifacts map[string][]byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	seen := map[string]bool{}
	for scanner.Scan() {
		digest, path, found := strings.Cut(scanner.Text(), "  ")
		if !found || !hashPattern.MatchString(digest) || path == ChecksumFile || filepath.Base(path) != path || seen[path] {
			return fmt.Errorf("V10 held-out baseline checksum entry is invalid")
		}
		seen[path] = true
		artifact, ok := artifacts[path]
		if !ok || hashBytes(artifact) != digest {
			return fmt.Errorf("V10 held-out baseline checksum mismatch")
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(seen) != 3 || !seen[AuditFile] || !seen[CipherFileV10] || !seen[ManifestFile] {
		return fmt.Errorf("V10 held-out baseline checksums are incomplete")
	}
	return nil
}
