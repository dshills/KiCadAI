package capabilitybaselinepublicationv9

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"kicadai/internal/capabilitybaselinev9"
)

const maximumArtifactBytes = 32 << 20

func Verify(root string) (Verification, error) {
	manifestBytes, err := readRegularFile(root, ManifestFile)
	if err != nil {
		return Verification{}, err
	}
	var manifest Manifest
	if err := decodeStrict(manifestBytes, &manifest); err != nil {
		return Verification{}, fmt.Errorf("decode V9 baseline manifest: %w", err)
	}
	if err := validateManifest(manifest); err != nil {
		return Verification{}, err
	}
	wantManifestBytes, err := canonicalJSON(manifest)
	if err != nil || !bytes.Equal(manifestBytes, wantManifestBytes) {
		return Verification{}, fmt.Errorf("V9 baseline manifest is not canonical")
	}

	reportBytes, err := readRegularFile(root, ReportFile)
	if err != nil {
		return Verification{}, err
	}
	var report capabilitybaselinev9.Report
	if err := decodeStrict(reportBytes, &report); err != nil {
		return Verification{}, fmt.Errorf("decode V9 baseline report: %w", err)
	}
	if err := capabilitybaselinev9.Validate(report); err != nil {
		return Verification{}, fmt.Errorf("validate V9 baseline report: %w", err)
	}
	wantReportBytes, err := capabilitybaselinev9.MarshalJSONStable(report)
	if err != nil {
		return Verification{}, err
	}
	wantReportBytes = withSingleTrailingLF(wantReportBytes)
	if !bytes.Equal(reportBytes, wantReportBytes) || hashBytes(reportBytes) != manifest.ReportSHA256 ||
		report.CorpusManifestSHA256 != manifest.Binding.CorpusManifestSHA256 || report.EnvironmentSHA256 != manifest.Binding.EnvironmentSHA256 ||
		report.EvaluatorManifestSHA256 != manifest.Binding.EvaluatorManifestSHA256 || !slices.Equal(report.OutcomeCounts, manifest.OutcomeCounts) {
		return Verification{}, fmt.Errorf("V9 baseline report does not match its manifest")
	}
	if len(report.Cases) != len(manifest.Cases) {
		return Verification{}, fmt.Errorf("V9 baseline report case count does not match its manifest")
	}

	expectedFiles := map[string][]byte{ManifestFile: manifestBytes, ReportFile: reportBytes}
	for index, reference := range manifest.Cases {
		data, readErr := readRegularFile(root, reference.Path)
		if readErr != nil {
			return Verification{}, readErr
		}
		if hashBytes(data) != reference.SHA256 {
			return Verification{}, fmt.Errorf("V9 case %s hash mismatch", reference.ID)
		}
		var record capabilitybaselinev9.CaseEvidence
		if err := decodeStrict(data, &record); err != nil {
			return Verification{}, fmt.Errorf("decode V9 case %s: %w", reference.ID, err)
		}
		canonical, err := canonicalJSON(record)
		if err != nil || !bytes.Equal(data, canonical) || !reflect.DeepEqual(record, report.Cases[index]) {
			return Verification{}, fmt.Errorf("V9 case %s does not reproduce report evidence", reference.ID)
		}
		expectedFiles[reference.Path] = data
	}
	audit, err := readRegularFile(root, AuditFile)
	if err != nil || !bytes.Equal(audit, auditBytes(manifest, hashBytes(manifestBytes))) {
		return Verification{}, fmt.Errorf("V9 baseline audit does not reproduce")
	}
	expectedFiles[AuditFile] = audit
	checksums, err := readRegularFile(root, ChecksumFile)
	if err != nil || !bytes.Equal(checksums, checksumBytes(expectedFiles)) {
		return Verification{}, fmt.Errorf("V9 baseline checksums do not reproduce")
	}
	if err := verifyExactFiles(root, expectedFiles); err != nil {
		return Verification{}, err
	}
	return Verification{Manifest: manifest, ManifestSHA256: hashBytes(manifestBytes), Report: report}, nil
}

func validateManifest(value Manifest) error {
	if err := validateBinding(value.Binding); err != nil {
		return fmt.Errorf("invalid V9 baseline publication binding: %w", err)
	}
	if value.Schema != ManifestSchema || value.Version != Version || value.CaseCount != ExpectedCases ||
		len(value.Cases) != ExpectedCases || len(value.OutcomeCounts) != len(frozenOutcomeOrder) || !digestPattern.MatchString(value.ReportSHA256) || !digestPattern.MatchString(value.Hash) {
		return fmt.Errorf("invalid V9 baseline publication manifest")
	}
	total := 0
	for index, count := range value.OutcomeCounts {
		if count.Outcome != frozenOutcomeOrder[index] || count.Count < 0 {
			return fmt.Errorf("invalid V9 baseline outcome counts")
		}
		total += count.Count
	}
	if total != ExpectedCases {
		return fmt.Errorf("invalid V9 baseline outcome total")
	}
	for index, reference := range value.Cases {
		wantID := fmt.Sprintf("v9_case_%03d", index+1)
		if reference.ID != wantID || reference.Path != CaseDirectory+"/"+wantID+".json" || !digestPattern.MatchString(reference.SHA256) {
			return fmt.Errorf("invalid V9 baseline case reference %d", index)
		}
	}
	wantHash, err := manifestHash(value)
	if err != nil {
		return fmt.Errorf("invalid V9 baseline manifest commitment: %w", err)
	}
	if wantHash != value.Hash {
		return fmt.Errorf("invalid V9 baseline manifest commitment")
	}
	return nil
}

func readRegularFile(root, relative string) ([]byte, error) {
	if relative == "" || path.IsAbs(relative) || filepath.IsAbs(relative) || path.Clean(relative) != relative ||
		strings.Contains(relative, `\`) || relative == ".." || strings.HasPrefix(relative, "../") {
		return nil, fmt.Errorf("invalid V9 baseline artifact path %q", relative)
	}
	filePath := filepath.Join(root, filepath.FromSlash(relative))
	pathInfo, err := os.Lstat(filePath)
	if err != nil {
		return nil, err
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Size() > maximumArtifactBytes {
		return nil, fmt.Errorf("V9 baseline artifact %s is not a bounded regular file", relative)
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !openInfo.Mode().IsRegular() || openInfo.Size() > maximumArtifactBytes || !os.SameFile(pathInfo, openInfo) {
		return nil, fmt.Errorf("V9 baseline artifact %s changed during verification", relative)
	}
	data, err := io.ReadAll(io.LimitReader(file, maximumArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maximumArtifactBytes {
		return nil, fmt.Errorf("V9 baseline artifact %s exceeds its size bound", relative)
	}
	return data, nil
}

func verifyExactFiles(root string, expected map[string][]byte) error {
	want := make(map[string]bool, len(expected)+1)
	for path := range expected {
		want[path] = false
	}
	want[ChecksumFile] = false
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("V9 baseline root is not a real directory")
	}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative == "." || relative == CaseDirectory {
			if !entry.IsDir() {
				return fmt.Errorf("V9 baseline directory shape is invalid")
			}
			return nil
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("unexpected V9 baseline path %s", relative)
		}
		if _, exists := want[relative]; !exists || want[relative] {
			return fmt.Errorf("unexpected or duplicate V9 baseline file %s", relative)
		}
		want[relative] = true
		return nil
	}); err != nil {
		return err
	}
	for path, seen := range want {
		if !seen {
			return fmt.Errorf("missing V9 baseline file %s", path)
		}
	}
	return nil
}
