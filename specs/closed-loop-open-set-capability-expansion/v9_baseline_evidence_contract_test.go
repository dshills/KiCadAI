package closedloopopensetcontract

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev9"
)

func TestVersionNineBaselineEvidenceRulesAreFrozenAndOutcomeBlind(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema                 string `json:"schema"`
		Version                int    `json:"version"`
		FreezeCommitParent     string `json:"freeze_commit_parent"`
		EvidenceManifest       string `json:"evidence_manifest"`
		EvidenceManifestSHA256 string `json:"evidence_manifest_sha256"`
		DiscoveryCount         int    `json:"discovery_count"`
		MandatoryGateCount     int    `json:"mandatory_gate_count"`
		PassPromotionCount     int    `json:"pass_promotion_count"`
		RealCorpusEvaluated    bool   `json:"real_corpus_evaluated"`
		HeldOutOpened          bool   `json:"held_out_opened"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V9_BASELINE_EVIDENCE_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-baseline-evidence-freeze.v9" || freeze.Version != 9 {
		t.Fatalf("V9 baseline evidence schema/version = %q/%d", freeze.Schema, freeze.Version)
	}
	if freeze.FreezeCommitParent != "fb59520299831a3c67b126fc4dcef79aa55dbb21" {
		t.Fatalf("V9 baseline evidence parent = %q", freeze.FreezeCommitParent)
	}
	if freeze.EvidenceManifest != "V9_BASELINE_EVIDENCE.sha256" ||
		freeze.EvidenceManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvidenceManifest)) {
		t.Fatal("V9 baseline evidence manifest binding is invalid")
	}
	if freeze.DiscoveryCount != 24 || freeze.MandatoryGateCount != 14 || freeze.PassPromotionCount != 2 {
		t.Fatalf("V9 baseline evidence bounds = %d/%d/%d", freeze.DiscoveryCount, freeze.MandatoryGateCount, freeze.PassPromotionCount)
	}
	if freeze.RealCorpusEvaluated || freeze.HeldOutOpened {
		t.Fatal("V9 baseline preparation claims real evaluation or held-out access")
	}
	if capabilitybaselinev9.Version != 9 || capabilitybaselinev9.CaseEvidenceSchema == "" || capabilitybaselinev9.ReportSchema == "" {
		t.Fatal("V9 baseline runtime schema is invalid")
	}
	v8VerifyManifest(t, directory, freeze.EvidenceManifest)
	assertV9BaselineImportsAreOutcomeBlind(t, directory, freeze.EvidenceManifest)
}

func assertV9BaselineImportsAreOutcomeBlind(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v7ReadFile(t, filepath.Join(directory, manifestName)))
	expectedPaths := map[string]bool{
		"../../internal/capabilitybaselinev9/model.go":         false,
		"../../internal/capabilitybaselinev9/validate.go":      false,
		"../../internal/capabilitybaselinev9/validate_test.go": false,
		"V9_BASELINE_EVIDENCE_ENVELOPE.md":                     false,
	}
	productionSources := 0
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			t.Fatalf("malformed V9 baseline manifest line %q", line)
		}
		relative := fields[1]
		if _, exists := expectedPaths[relative]; !exists {
			t.Fatalf("unexpected V9 baseline manifest path %q", relative)
		}
		expectedPaths[relative] = true
		if !strings.HasPrefix(relative, "../../internal/capabilitybaselinev9/") || !strings.HasSuffix(relative, ".go") || strings.HasSuffix(relative, "_test.go") {
			continue
		}
		productionSources++
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		file, err := parser.ParseFile(token.NewFileSet(), relative, source, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse V9 baseline source %s: %v", relative, err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("decode V9 baseline import in %s: %v", relative, err)
			}
			if strings.HasPrefix(path, "kicadai/") && path != "kicadai/internal/capabilityroundsv9" {
				t.Fatalf("V9 baseline validator %s imports forbidden package %q", relative, path)
			}
		}
	}
	if productionSources != 2 {
		t.Fatalf("V9 baseline manifest names %d production Go sources, want 2", productionSources)
	}
	for path, seen := range expectedPaths {
		if !seen {
			t.Fatalf("V9 baseline manifest is missing %q", path)
		}
	}
}
