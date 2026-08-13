package closedloopopensetcontract

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"kicadai/internal/blindbaseline"
)

func TestVersionTenBlindBaselinePublisherIsFrozenBeforeKeyUse(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema                  string `json:"schema"`
		Version                 int    `json:"version"`
		FreezeParentCommit      string `json:"freeze_parent_commit"`
		PublisherManifest       string `json:"publisher_manifest"`
		PublisherManifestSHA256 string `json:"publisher_manifest_sha256"`
		HeldOutCount            int    `json:"held_out_count"`
		RecordEncryption        string `json:"record_encryption"`
		RealCorpusEvaluated     bool   `json:"real_corpus_evaluated"`
		ExternalKeyCreated      bool   `json:"external_key_created"`
		ExternalKeyOpened       bool   `json:"external_key_opened"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V10_BLIND_BASELINE_PUBLISHER_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-blind-baseline-publisher-freeze.v10" || freeze.Version != 10 ||
		freeze.FreezeParentCommit != "ce9457de56de5fb0ad8cacafbcd3db2adf371879" {
		t.Fatalf("invalid V10 blind baseline publisher freeze: %+v", freeze)
	}
	if freeze.PublisherManifest != "V10_BLIND_BASELINE_PUBLISHER.sha256" ||
		freeze.PublisherManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.PublisherManifest)) {
		t.Fatal("V10 blind baseline publisher manifest binding is invalid")
	}
	if freeze.HeldOutCount != 24 || freeze.RecordEncryption != "AES-256-GCM" || freeze.RealCorpusEvaluated ||
		freeze.ExternalKeyCreated || freeze.ExternalKeyOpened {
		t.Fatal("V10 blind baseline publisher freeze crosses its preparation boundary")
	}
	if blindbaseline.ManifestVersionV10 != 10 || !strings.Contains(blindbaseline.ManifestSchemaV10, ".v10") ||
		!strings.Contains(blindbaseline.AlgorithmV10, "random-unique-nonce-per-record") {
		t.Fatal("V10 blind baseline runtime identity is invalid")
	}
	v10VerifyRootManifest(t, directory, freeze.PublisherManifest)
}

func v10VerifyRootManifest(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v7ReadFile(t, filepath.Join(directory, manifestName)))
	previous := ""
	count := 0
	for _, line := range strings.Split(strings.TrimSuffix(manifest, "\n"), "\n") {
		if len(line) < 67 || line[64:66] != "  " {
			t.Fatalf("malformed V10 root manifest line %q", line)
		}
		relative := line[66:]
		if relative <= previous || strings.Contains(relative, "..") || filepath.IsAbs(relative) {
			t.Fatalf("invalid or unordered V10 root manifest path %q", relative)
		}
		if got := v7FileSHA256(t, filepath.Join(directory, "..", "..", filepath.FromSlash(relative))); got != line[:64] {
			t.Fatalf("V10 root manifest hash mismatch for %s", relative)
		}
		previous = relative
		count++
	}
	if count != 22 {
		t.Fatalf("V10 blind baseline publisher manifest entries = %d, want 22", count)
	}
}
