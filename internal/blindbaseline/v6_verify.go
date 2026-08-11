package blindbaseline

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func VerifyV6(root string) (V6Manifest, error) {
	artifacts, err := readBaselineArtifacts(root)
	if err != nil {
		return V6Manifest{}, err
	}
	if err := verifyChecksums(artifacts[ChecksumFile], artifacts); err != nil {
		return V6Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(artifacts[ManifestFile]))
	decoder.DisallowUnknownFields()
	var manifest V6Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return V6Manifest{}, fmt.Errorf("decode V6 held-out baseline manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return V6Manifest{}, fmt.Errorf("decode V6 held-out baseline manifest trailing data")
	}
	if err := validateManifestV6(manifest); err != nil {
		return V6Manifest{}, err
	}
	ciphertext := artifacts[CipherFile]
	if hashBytes(ciphertext) != manifest.CiphertextSHA256 || !bytes.Equal(artifacts[AuditFile], baselineAuditV6(manifest)) {
		return V6Manifest{}, fmt.Errorf("V6 held-out baseline public commitment is invalid")
	}
	canonicalManifest, err := marshalStable(manifest)
	if err != nil {
		return V6Manifest{}, err
	}
	canonicalArtifacts := map[string][]byte{AuditFile: artifacts[AuditFile], CipherFile: ciphertext, ManifestFile: canonicalManifest}
	if !bytes.Equal(artifacts[ChecksumFile], checksumManifest(canonicalArtifacts)) {
		return V6Manifest{}, fmt.Errorf("V6 held-out baseline checksum manifest is noncanonical")
	}
	return manifest, nil
}

func readBaselineArtifacts(root string) (map[string][]byte, error) {
	want := map[string]bool{AuditFile: true, ChecksumFile: true, CipherFile: true, ManifestFile: true}
	directoryEntries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	if len(directoryEntries) != len(want) {
		return nil, fmt.Errorf("V6 held-out baseline file set is invalid")
	}
	artifacts := make(map[string][]byte, len(want))
	for _, entry := range directoryEntries {
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("V6 held-out baseline contains a non-regular entry")
		}
		name := entry.Name()
		if !want[name] {
			return nil, fmt.Errorf("V6 held-out baseline file set is invalid")
		}
		maximum := int64(1 << 20)
		if name == CipherFile {
			maximum = maximumPayloadSize + 1<<20
		}
		artifacts[name], err = readBounded(filepath.Join(root, name), maximum)
		if err != nil {
			return nil, err
		}
	}
	return artifacts, nil
}
