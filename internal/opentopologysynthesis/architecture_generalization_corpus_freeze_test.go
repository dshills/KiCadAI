package opentopologysynthesis

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	generalizationCorpusSchema       = "kicadai.architecture-generalization-corpus.v1"
	generalizationCorpusBaseCommit   = "346fcdd4ffb5c99aa3e3945c76110a0652722428"
	generalizationCorpusManifestHash = "fd23189af2ef0471d4e69f0b4693b3bb05a10081a7d7ed15964ae6f98df08f86"
	generalizationDesignCaseCount    = 6
	generalizationAdversarialCount   = 4
)

var generalizationCorpusStages = []string{
	"integrity",
	"schema",
	"primitive_inventory",
	"architecture_generation",
	"equation_sizing",
	"simulation",
	"safety_rejection",
	"ranking",
	"lowering",
	"schematic",
	"placement",
	"routing",
	"writer",
	"erc",
	"drc",
	"round_trip",
	"replay",
}

type generalizationCorpusManifest struct {
	Schema              string                             `json:"schema"`
	Version             int                                `json:"version"`
	BaseCommit          string                             `json:"base_commit"`
	FrozenAt            string                             `json:"frozen_at"`
	RequirementSchema   string                             `json:"requirement_schema"`
	AuthoringPolicy     string                             `json:"authoring_policy"`
	MinimumDesignPasses int                                `json:"minimum_design_passes"`
	Stages              []string                           `json:"stages"`
	DesignCases         []generalizationDesignManifestCase `json:"design_cases"`
	AdversarialCases    []generalizationAdversarialCase    `json:"adversarial_cases"`
}

type generalizationDesignManifestCase struct {
	ID                string   `json:"id"`
	BehaviorFamily    string   `json:"behavior_family"`
	AcceptanceClaim   string   `json:"acceptance_claim"`
	RequirementFile   string   `json:"requirement_file"`
	RequirementSHA256 string   `json:"requirement_sha256"`
	RequiredAnalyses  []string `json:"required_analyses"`
	SafetyCritical    bool     `json:"safety_critical"`
}

type generalizationAdversarialCase struct {
	ID                  string   `json:"id"`
	BehaviorFamily      string   `json:"behavior_family"`
	ExpectedFailureKind string   `json:"expected_failure_kind"`
	RequirementFile     string   `json:"requirement_file"`
	RequirementSHA256   string   `json:"requirement_sha256"`
	RequiredAnalyses    []string `json:"required_analyses"`
}

func TestArchitectureGeneralizationCorpusIsFrozenBeforeProductionChanges(t *testing.T) {
	root := architectureGeneralizationCorpusRoot()
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != generalizationCorpusManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, generalizationCorpusManifestHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256")))); sidecar != generalizationCorpusManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest generalizationCorpusManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != generalizationCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != generalizationCorpusBaseCommit ||
		manifest.RequirementSchema != frozenRequirementSchema ||
		strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" ||
		manifest.MinimumDesignPasses != 5 {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if !slices.Equal(manifest.Stages, generalizationCorpusStages) {
		t.Fatalf("stages = %v, want %v", manifest.Stages, generalizationCorpusStages)
	}
	if len(manifest.DesignCases) != generalizationDesignCaseCount ||
		len(manifest.AdversarialCases) != generalizationAdversarialCount {
		t.Fatalf("case counts = %d/%d", len(manifest.DesignCases), len(manifest.AdversarialCases))
	}

	wantClaims := map[string]bool{
		"active_bandpass_crossover":         true,
		"linear_regulator":                  true,
		"precision_rectifier":               true,
		"protected_constant_current_driver": true,
		"transimpedance_amplifier":          true,
		"window_comparator":                 true,
	}
	wantFailures := map[string]bool{
		"inadequate_soa":     true,
		"instability":        true,
		"invalid_bias":       true,
		"unsafe_dissipation": true,
	}
	seenClaims := map[string]bool{}
	seenFailures := map[string]bool{}
	seenFamilies := map[string]bool{}
	seenFiles := map[string]bool{"manifest.json": true}
	previousID := ""
	for _, entry := range manifest.DesignCases {
		if entry.ID <= previousID {
			t.Fatalf("design case IDs are not strictly sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		if !wantClaims[entry.AcceptanceClaim] || seenClaims[entry.AcceptanceClaim] {
			t.Fatalf("%s acceptance claim = %q", entry.ID, entry.AcceptanceClaim)
		}
		seenClaims[entry.AcceptanceClaim] = true
		validateGeneralizationManifestRequirement(t, root, entry.ID, entry.BehaviorFamily,
			entry.RequirementFile, entry.RequirementSHA256, entry.RequiredAnalyses,
			entry.SafetyCritical, seenFamilies, seenFiles)
	}
	previousID = ""
	for _, entry := range manifest.AdversarialCases {
		if entry.ID <= previousID {
			t.Fatalf("adversarial case IDs are not strictly sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		if !wantFailures[entry.ExpectedFailureKind] || seenFailures[entry.ExpectedFailureKind] {
			t.Fatalf("%s expected failure kind = %q", entry.ID, entry.ExpectedFailureKind)
		}
		seenFailures[entry.ExpectedFailureKind] = true
		validateGeneralizationManifestRequirement(t, root, entry.ID, entry.BehaviorFamily,
			entry.RequirementFile, entry.RequirementSHA256, entry.RequiredAnalyses,
			true, seenFamilies, seenFiles)
	}
	if len(seenClaims) != len(wantClaims) || len(seenFailures) != len(wantFailures) {
		t.Fatalf("coverage claims=%v failures=%v", seenClaims, seenFailures)
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

func validateGeneralizationManifestRequirement(
	t *testing.T,
	root, id, behaviorFamily, requirementFile, requirementSHA256 string,
	requiredAnalyses []string,
	safetyCritical bool,
	seenFamilies, seenFiles map[string]bool,
) {
	t.Helper()
	if strings.TrimSpace(behaviorFamily) == "" || seenFamilies[behaviorFamily] {
		t.Fatalf("%s behavior family = %q", id, behaviorFamily)
	}
	seenFamilies[behaviorFamily] = true
	if len(requiredAnalyses) < 3 || !slices.IsSorted(requiredAnalyses) {
		t.Fatalf("%s required analyses = %v", id, requiredAnalyses)
	}
	path := filepath.Join(root, requirementFile)
	if filepath.Base(path) != requirementFile || seenFiles[requirementFile] {
		t.Fatalf("%s unsafe or duplicate requirement file %q", id, requirementFile)
	}
	seenFiles[requirementFile] = true
	data := mustRead(t, path)
	if got := frozenHash(data); got != requirementSHA256 {
		t.Fatalf("%s sha256 = %s, want %s", id, got, requirementSHA256)
	}
	rejectFrozenImplementationDetail(t, id, data)
	var frozen frozenRequirement
	decodeFrozenStrict(t, data, &frozen)
	validateFrozenRequirement(t, frozenManifestCase{
		ID:                id,
		BehaviorFamily:    behaviorFamily,
		RequirementFile:   requirementFile,
		RequirementSHA256: requirementSHA256,
		RequiredAnalyses:  requiredAnalyses,
		SafetyCritical:    safetyCritical,
	}, frozen)
	decoded, issues := DecodeStrict(bytes.NewReader(data))
	if len(issues) != 0 {
		t.Fatalf("%s open-topology requirement is invalid: %#v", id, issues)
	}
	if decoded.Project.Name != id {
		t.Fatalf("%s decoded project name = %q", id, decoded.Project.Name)
	}
}

func architectureGeneralizationCorpusRoot() string {
	return filepath.Join("testdata", "architecture_generalization_corpus")
}
