package closedloopopensetcontract

import (
	"encoding/json"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kicadai/internal/capabilitybaselinev10"
	"kicadai/internal/capabilityroundsv10"
)

const (
	v10BaselineEvidenceFreezeParent = "da0e8c41c5cbdf4fa4b62c5e8c9609e63ca88610"
	v10ExposureFreezeParent         = "f508ae2f25bc2ea4f2c6d468c22a7abd8e05d847"
)

func TestVersionTenBaselineEvidenceRulesAreFrozenAndOutcomeBlind(t *testing.T) {
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
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V10_BASELINE_EVIDENCE_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-baseline-evidence-freeze.v10" || freeze.Version != 10 ||
		freeze.FreezeCommitParent != v10BaselineEvidenceFreezeParent {
		t.Fatalf("V10 baseline freeze boundary = %q/%d/%q", freeze.Schema, freeze.Version, freeze.FreezeCommitParent)
	}
	if freeze.EvidenceManifest != "V10_BASELINE_EVIDENCE.sha256" ||
		freeze.EvidenceManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EvidenceManifest)) {
		t.Fatal("V10 baseline evidence manifest binding is invalid")
	}
	if freeze.DiscoveryCount != 24 || freeze.MandatoryGateCount != 14 || freeze.PassPromotionCount != 2 ||
		freeze.RealCorpusEvaluated || freeze.HeldOutOpened {
		t.Fatalf("V10 baseline evidence bounds/state = %+v", freeze)
	}
	if capabilitybaselinev10.Version != 10 || capabilitybaselinev10.CaseEvidenceSchema == "" || capabilitybaselinev10.ReportSchema == "" {
		t.Fatal("V10 baseline runtime schema is invalid")
	}
	v8VerifyManifest(t, directory, freeze.EvidenceManifest)
	assertV10EvaluationImportsAreBlind(t, directory, freeze.EvidenceManifest)
}

func TestVersionTenEffectExposureEngineIsFrozenAndCorpusBlind(t *testing.T) {
	directory := v7ContractDirectory(t)
	var freeze struct {
		Schema               string `json:"schema"`
		Version              int    `json:"version"`
		FreezeCommitParent   string `json:"freeze_commit_parent"`
		EngineManifest       string `json:"engine_manifest"`
		EngineManifestSHA256 string `json:"engine_manifest_sha256"`
		DiscoveryCount       int    `json:"discovery_count"`
		MaximumRounds        int    `json:"maximum_rounds"`
		MaximumTotalAtoms    int    `json:"maximum_total_atoms"`
		MaximumTotalMembers  int    `json:"maximum_total_members"`
		RealFrontierLoaded   bool   `json:"real_frontier_loaded"`
		HeldOutOpened        bool   `json:"held_out_opened"`
	}
	if err := json.Unmarshal(v7ReadFile(t, filepath.Join(directory, "V10_EFFECT_EXPOSURE_ENGINE_FREEZE.json")), &freeze); err != nil {
		t.Fatal(err)
	}
	if freeze.Schema != "kicadai.closed-loop-open-set-effect-exposure-freeze.v10" || freeze.Version != 10 ||
		freeze.FreezeCommitParent != v10ExposureFreezeParent {
		t.Fatalf("V10 exposure freeze boundary = %q/%d/%q", freeze.Schema, freeze.Version, freeze.FreezeCommitParent)
	}
	if freeze.EngineManifest != "V10_EFFECT_EXPOSURE_ENGINE.sha256" ||
		freeze.EngineManifestSHA256 != v7FileSHA256(t, filepath.Join(directory, freeze.EngineManifest)) {
		t.Fatal("V10 exposure engine manifest binding is invalid")
	}
	if freeze.DiscoveryCount != 24 || freeze.MaximumRounds != 2 || freeze.MaximumTotalAtoms != 6 || freeze.MaximumTotalMembers != 18 ||
		freeze.RealFrontierLoaded || freeze.HeldOutOpened {
		t.Fatalf("V10 exposure bounds/state = %+v", freeze)
	}
	v8VerifyManifest(t, directory, freeze.EngineManifest)
	policy := capabilityroundsv10.FrozenPolicy()
	if policy.ExpectedDiscoveryCases != 24 || policy.MaximumRounds != 2 || policy.MaximumTotalAtoms != 6 || policy.MaximumTotalMembers != 18 ||
		policy.MaximumRoundAtoms != 3 || policy.MaximumRoundMembers != 9 || policy.MinimumCaseSupport != 2 ||
		policy.MinimumAdvancedCases != 2 || policy.MinimumDomains != 2 || policy.MinimumRoles != 2 || policy.MaximumSuccessors != 4 {
		t.Fatalf("V10 exposure runtime policy differs from freeze: %+v", policy)
	}
	assertV10EvaluationImportsAreBlind(t, directory, freeze.EngineManifest)
}

func assertV10EvaluationImportsAreBlind(t *testing.T, directory, manifestName string) {
	t.Helper()
	manifest := string(v7ReadFile(t, filepath.Join(directory, manifestName)))
	productionSources := 0
	for _, line := range strings.Split(strings.TrimSpace(manifest), "\n") {
		if len(line) < 67 {
			t.Fatalf("malformed V10 evaluation manifest line %q", line)
		}
		relative := line[66:]
		if !strings.HasPrefix(relative, "../../internal/") || !strings.HasSuffix(relative, ".go") || strings.HasSuffix(relative, "_test.go") {
			continue
		}
		productionSources++
		source := v7ReadFile(t, filepath.Join(directory, filepath.FromSlash(relative)))
		file, err := parser.ParseFile(token.NewFileSet(), relative, source, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse V10 evaluation source %s: %v", relative, err)
		}
		for _, imported := range file.Imports {
			path, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("decode V10 evaluation import in %s: %v", relative, err)
			}
			for _, forbidden := range []string{"corpuspublication", "corpusfreezev10", "opentopologysynthesis", "closedloopsynthesis", "blindbaseline", "externalkey"} {
				if strings.Contains(path, forbidden) {
					t.Fatalf("V10 evaluation boundary %s imports forbidden package %q", relative, path)
				}
			}
		}
	}
	if productionSources == 0 {
		t.Fatal("V10 evaluation manifest names no production sources")
	}
}
