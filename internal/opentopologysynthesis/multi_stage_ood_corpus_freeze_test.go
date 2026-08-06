package opentopologysynthesis

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	multiStageOODCorpusSchema       = "kicadai.multi-stage-ood-corpus.v1"
	multiStageOODCorpusBaseCommit   = "f06a1a621e34f483465b421152c1db905a947c48"
	multiStageOODCorpusManifestHash = "ee3e89939422ac1f0280e57a8ac09994bebac3a8ca083053385778ea0a04edd2"
	multiStageOODDesignCaseCount    = 9
	multiStageOODAdversarialCount   = 4
)

var multiStageOODCorpusStages = []string{
	"integrity",
	"schema",
	"behavior_decomposition",
	"architecture_composition",
	"value_selection",
	"simulation",
	"multi_stage_constraint_propagation",
	"thermal_soa",
	"safety_rejection",
	"diagnosis",
	"cross_stage_repair",
	"ranking",
	"lowering",
	"schematic_readability",
	"placement",
	"routing",
	"connectivity",
	"writer",
	"erc",
	"drc",
	"round_trip",
	"replay",
}

type multiStageOODCorpusManifest struct {
	Schema              string                         `json:"schema"`
	Version             int                            `json:"version"`
	BaseCommit          string                         `json:"base_commit"`
	FrozenAt            string                         `json:"frozen_at"`
	RequirementSchema   string                         `json:"requirement_schema"`
	AuthoringPolicy     string                         `json:"authoring_policy"`
	MinimumDesignPasses int                            `json:"minimum_design_passes"`
	Stages              []string                       `json:"stages"`
	DesignCases         []multiStageOODDesignCase      `json:"design_cases"`
	AdversarialCases    []multiStageOODAdversarialCase `json:"adversarial_cases"`
}

type multiStageOODDesignCase struct {
	ID                  string   `json:"id"`
	BehaviorFamily      string   `json:"behavior_family"`
	FunctionalBehaviors []string `json:"functional_behaviors"`
	RequirementFile     string   `json:"requirement_file"`
	RequirementSHA256   string   `json:"requirement_sha256"`
	RequiredAnalyses    []string `json:"required_analyses"`
	SafetyCritical      bool     `json:"safety_critical"`
}

type multiStageOODAdversarialCase struct {
	ID                  string   `json:"id"`
	BehaviorFamily      string   `json:"behavior_family"`
	FunctionalBehaviors []string `json:"functional_behaviors"`
	ExpectedFailureKind string   `json:"expected_failure_kind"`
	RequirementFile     string   `json:"requirement_file"`
	RequirementSHA256   string   `json:"requirement_sha256"`
	RequiredAnalyses    []string `json:"required_analyses"`
}

// The immutable-corpus helpers used below are shared with corpus_freeze_test.go
// so every open-topology corpus uses identical hashing, strict decoding, input
// vocabulary, acceptance, and failure-count semantics.
func TestMultiStageOODCorpusIsFrozenBeforeProductionChanges(t *testing.T) {
	root := multiStageOODCorpusRoot()
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != multiStageOODCorpusManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, multiStageOODCorpusManifestHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256")))); sidecar != multiStageOODCorpusManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest multiStageOODCorpusManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != multiStageOODCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != multiStageOODCorpusBaseCommit || manifest.RequirementSchema != RequirementSchema ||
		manifest.MinimumDesignPasses != multiStageOODDesignCaseCount || strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if !slices.Equal(manifest.Stages, multiStageOODCorpusStages) ||
		len(manifest.DesignCases) != multiStageOODDesignCaseCount ||
		len(manifest.AdversarialCases) != multiStageOODAdversarialCount {
		t.Fatalf("manifest coverage stages=%v cases=%d/%d", manifest.Stages, len(manifest.DesignCases), len(manifest.AdversarialCases))
	}

	seenFiles := map[string]bool{"manifest.json": true, "manifest.sha256": true}
	previousID := ""
	for _, entry := range manifest.DesignCases {
		if entry.ID <= previousID || !entry.SafetyCritical {
			t.Fatalf("design case order/safety = %q after %q safety=%t", entry.ID, previousID, entry.SafetyCritical)
		}
		previousID = entry.ID
		validateMultiStageOODFrozenRequirement(t, root, entry.ID, entry.FunctionalBehaviors, entry.RequirementFile, entry.RequirementSHA256, entry.RequiredAnalyses, true, seenFiles)
	}

	wantFailureCounts := map[string]int{
		"unsafe_thermal_soa":             2,
		"unsupported_dynamic_envelope":   1,
		"unsupported_high_energy_domain": 1,
	}
	seenFailureCounts := map[string]int{}
	previousID = ""
	for _, entry := range manifest.AdversarialCases {
		if entry.ID <= previousID || wantFailureCounts[entry.ExpectedFailureKind] == 0 {
			t.Fatalf("adversarial case order/failure = %q after %q failure=%q", entry.ID, previousID, entry.ExpectedFailureKind)
		}
		previousID = entry.ID
		seenFailureCounts[entry.ExpectedFailureKind]++
		validateMultiStageOODFrozenRequirement(t, root, entry.ID, entry.FunctionalBehaviors, entry.RequirementFile, entry.RequirementSHA256, entry.RequiredAnalyses, false, seenFiles)
	}
	if !equalStringIntMap(seenFailureCounts, wantFailureCounts) {
		t.Fatalf("failure coverage = %v, want %v", seenFailureCounts, wantFailureCounts)
	}

	files, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.IsDir() || !seenFiles[file.Name()] {
			t.Errorf("unmanifested corpus entry %s", file.Name())
		}
	}
}

func validateMultiStageOODFrozenRequirement(
	t *testing.T,
	root, id string,
	functionalBehaviors []string,
	requirementFile, requirementSHA256 string,
	requiredAnalyses []string,
	positive bool,
	seenFiles map[string]bool,
) {
	t.Helper()
	if filepath.Base(requirementFile) != requirementFile || seenFiles[requirementFile] {
		t.Fatalf("%s requirement file must be a unique basename: %q", id, requirementFile)
	}
	if len(functionalBehaviors) < 3 {
		t.Fatalf("%s functional behaviors = %v, want at least three", id, functionalBehaviors)
	}
	if !slices.IsSorted(functionalBehaviors) {
		t.Fatalf("%s functional behaviors must be alphabetically sorted: %v", id, functionalBehaviors)
	}
	if len(requiredAnalyses) < 3 || !slices.Contains(requiredAnalyses, "transient") ||
		!slices.Contains(requiredAnalyses, "electrothermal") {
		t.Fatalf("%s analyses = %v, want at least three including transient and electrothermal", id, requiredAnalyses)
	}
	if !slices.IsSorted(requiredAnalyses) {
		t.Fatalf("%s required analyses must be alphabetically sorted: %v", id, requiredAnalyses)
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
	if issues := Validate(requirement); len(issues) != 0 {
		t.Fatalf("%s requirement validation issues = %#v", id, issues)
	}
	if len(requirement.Requirements.Ports) < 4 || len(requirement.Requirements.OperatingCases) < 1 ||
		(positive && len(requirement.Requirements.OperatingCases) < 2) {
		t.Fatalf("%s insufficient external or operating coverage", id)
	}

	analyses := make([]string, 0, len(requirement.Requirements.BehavioralRequirements))
	hasTemperature := false
	hasSOA := false
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if !slices.Contains(analyses, assertion.Analysis) {
			analyses = append(analyses, assertion.Analysis)
		}
		hasTemperature = hasTemperature || assertion.Metric == "junction_temperature"
		hasSOA = hasSOA || assertion.Metric == "soa_margin"
	}
	slices.Sort(analyses)
	if !slices.Equal(analyses, requiredAnalyses) || !hasTemperature || !hasSOA {
		t.Fatalf("%s analyses/safety = %v temperature=%t soa=%t, want %v", id, analyses, hasTemperature, hasSOA, requiredAnalyses)
	}
}

func multiStageOODCorpusRoot() string {
	return filepath.Join("testdata", "multi_stage_ood_corpus")
}
