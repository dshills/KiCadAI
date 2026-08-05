package opentopologysynthesis

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	nonlinearSwitchingCorpusSchema       = "kicadai.nonlinear-switching-corpus.v1"
	nonlinearSwitchingCorpusBaseCommit   = "ebf2918aad8e305f416c0677cd8a5017656d3a5d"
	nonlinearSwitchingCorpusManifestHash = "e2378e390f17505d3fefaa0a1b3b19c25a1751bc550e06343788ba0e8ca2255a"
	nonlinearSwitchingDesignCaseCount    = 5
	nonlinearSwitchingAdversarialCount   = 3
)

var nonlinearSwitchingCorpusStages = []string{
	"integrity",
	"schema",
	"primitive_inventory",
	"architecture_generation",
	"equation_sizing",
	"simulation",
	"discontinuity_convergence",
	"safety_rejection",
	"ranking",
	"lowering",
	"schematic_readability",
	"switching_physical_constraints",
	"placement",
	"routing",
	"connectivity",
	"writer",
	"erc",
	"drc",
	"round_trip",
	"replay",
}

type nonlinearSwitchingCorpusManifest struct {
	Schema              string                                 `json:"schema"`
	Version             int                                    `json:"version"`
	BaseCommit          string                                 `json:"base_commit"`
	FrozenAt            string                                 `json:"frozen_at"`
	RequirementSchema   string                                 `json:"requirement_schema"`
	AuthoringPolicy     string                                 `json:"authoring_policy"`
	MinimumDesignPasses int                                    `json:"minimum_design_passes"`
	Stages              []string                               `json:"stages"`
	DesignCases         []nonlinearSwitchingDesignManifestCase `json:"design_cases"`
	AdversarialCases    []nonlinearSwitchingAdversarialCase    `json:"adversarial_cases"`
}

type nonlinearSwitchingDesignManifestCase struct {
	ID                string   `json:"id"`
	BehaviorFamily    string   `json:"behavior_family"`
	AcceptanceClaim   string   `json:"acceptance_claim"`
	RequirementFile   string   `json:"requirement_file"`
	RequirementSHA256 string   `json:"requirement_sha256"`
	RequiredAnalyses  []string `json:"required_analyses"`
	SafetyCritical    bool     `json:"safety_critical"`
}

type nonlinearSwitchingAdversarialCase struct {
	ID                  string   `json:"id"`
	BehaviorFamily      string   `json:"behavior_family"`
	ExpectedFailureKind string   `json:"expected_failure_kind"`
	RequirementFile     string   `json:"requirement_file"`
	RequirementSHA256   string   `json:"requirement_sha256"`
	RequiredAnalyses    []string `json:"required_analyses"`
}

func TestNonlinearSwitchingCorpusIsFrozenBeforeProductionChanges(t *testing.T) {
	root := nonlinearSwitchingCorpusRoot()
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != nonlinearSwitchingCorpusManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, nonlinearSwitchingCorpusManifestHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256")))); sidecar != nonlinearSwitchingCorpusManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest nonlinearSwitchingCorpusManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != nonlinearSwitchingCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != nonlinearSwitchingCorpusBaseCommit ||
		manifest.RequirementSchema != RequirementSchema ||
		manifest.MinimumDesignPasses != nonlinearSwitchingDesignCaseCount ||
		strings.TrimSpace(manifest.FrozenAt) == "" || strings.TrimSpace(manifest.AuthoringPolicy) == "" {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if !slices.Equal(manifest.Stages, nonlinearSwitchingCorpusStages) ||
		len(manifest.DesignCases) != nonlinearSwitchingDesignCaseCount ||
		len(manifest.AdversarialCases) != nonlinearSwitchingAdversarialCount {
		t.Fatalf("manifest coverage stages=%v cases=%d/%d", manifest.Stages, len(manifest.DesignCases), len(manifest.AdversarialCases))
	}

	wantClaims := map[string]bool{
		"diode_limiter":            true,
		"low_power_buck_regulator": true,
		"precision_rectifier":      true,
		"pwm_mosfet_driver":        true,
		"relaxation_oscillator":    true,
	}
	seenClaims := map[string]bool{}
	seenFiles := map[string]bool{"manifest.json": true}
	previousID := ""
	for _, entry := range manifest.DesignCases {
		if entry.ID <= previousID || !wantClaims[entry.AcceptanceClaim] || seenClaims[entry.AcceptanceClaim] {
			t.Fatalf("design case order/claim = %q after %q claim=%q", entry.ID, previousID, entry.AcceptanceClaim)
		}
		previousID = entry.ID
		seenClaims[entry.AcceptanceClaim] = true
		validateNonlinearSwitchingFrozenRequirement(t, root, entry.ID, entry.RequirementFile, entry.RequirementSHA256, entry.RequiredAnalyses, seenFiles)
	}

	wantFailureCounts := map[string]int{"unsafe_thermal_soa": 2, "unsupported_dynamic_envelope": 1}
	seenFailureCounts := map[string]int{}
	previousID = ""
	for _, entry := range manifest.AdversarialCases {
		if entry.ID <= previousID || wantFailureCounts[entry.ExpectedFailureKind] == 0 {
			t.Fatalf("adversarial case order/failure = %q after %q failure=%q", entry.ID, previousID, entry.ExpectedFailureKind)
		}
		previousID = entry.ID
		seenFailureCounts[entry.ExpectedFailureKind]++
		validateNonlinearSwitchingFrozenRequirement(t, root, entry.ID, entry.RequirementFile, entry.RequirementSHA256, entry.RequiredAnalyses, seenFiles)
	}
	if !equalStringIntMap(seenFailureCounts, wantFailureCounts) || len(seenClaims) != len(wantClaims) {
		t.Fatalf("corpus claims=%v failures=%v", seenClaims, seenFailureCounts)
	}
	files, err := filepath.Glob(filepath.Join(root, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if !seenFiles[filepath.Base(path)] {
			t.Errorf("unmanifested corpus file %s", filepath.Base(path))
		}
	}
}

func validateNonlinearSwitchingFrozenRequirement(
	t *testing.T,
	root, id, requirementFile, requirementSHA256 string,
	requiredAnalyses []string,
	seenFiles map[string]bool,
) {
	t.Helper()
	if filepath.Base(requirementFile) != requirementFile || seenFiles[requirementFile] ||
		len(requiredAnalyses) < 2 || !slices.IsSorted(requiredAnalyses) {
		t.Fatalf("%s invalid file/analysis declaration %q/%v", id, requirementFile, requiredAnalyses)
	}
	seenFiles[requirementFile] = true
	data := mustRead(t, filepath.Join(root, requirementFile))
	if got := frozenHash(data); got != requirementSHA256 {
		t.Fatalf("%s sha256 = %s, want %s", id, got, requirementSHA256)
	}
	rejectFrozenImplementationDetail(t, id, data)
	var requirement Requirement
	decodeFrozenStrict(t, data, &requirement)
	if requirement.Schema != RequirementSchema || requirement.Version != RequirementVersion ||
		requirement.Project.Name != id || !nonlinearSwitchingCompleteAcceptance(requirement.Acceptance) {
		t.Fatalf("%s requirement identity/acceptance = %#v", id, requirement)
	}
	analyses := make([]string, 0, len(requirement.Requirements.BehavioralRequirements))
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if !slices.Contains(analyses, assertion.Analysis) {
			analyses = append(analyses, assertion.Analysis)
		}
	}
	slices.Sort(analyses)
	if !slices.Equal(analyses, requiredAnalyses) {
		t.Fatalf("%s analyses = %v, want %v", id, analyses, requiredAnalyses)
	}
}

func nonlinearSwitchingCompleteAcceptance(value Acceptance) bool {
	return value.RequirePrimitiveOnly && value.RequireTopologySearch && value.RequireSimulation &&
		value.RequireAllCorners && value.RequireModelProvenance && value.RequireClosedLoopEvidence &&
		value.RequireCompleteRouting && value.RequireConnectivity && value.RequireWriterCorrectness &&
		value.RequireRoundTripZeroDiff && value.RequireERC && value.RequireStrictDRC &&
		value.RequireDeterministicReplay && value.RequireFailClosed
}

func equalStringIntMap(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func nonlinearSwitchingCorpusRoot() string {
	return filepath.Join("testdata", "nonlinear_switching_corpus")
}
