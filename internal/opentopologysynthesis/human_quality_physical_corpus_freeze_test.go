package opentopologysynthesis

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	humanQualityPhysicalCorpusSchema       = "kicadai.human-quality-physical-corpus.v1"
	humanQualityPhysicalCorpusBaseCommit   = "47f7a018adb292766f0b2dcf324dc5da40df051e"
	humanQualityPhysicalCorpusManifestHash = "4bae9c407539500ad50ad83a841d5911a1145a05cfaa8028a9eeafe4dff138ee"
)

var humanQualityPhysicalCorpusStages = []string{
	"integrity", "schema", "synthesis", "simulation", "thermal_soa",
	"functional_partition", "hierarchy", "inter_sheet_connectivity",
	"schematic_readability", "stackup", "plane_planning",
	"functional_placement", "thermal_placement", "layer_aware_routing",
	"return_path", "connectivity", "zone_fill", "writer", "erc",
	"strict_drc", "round_trip", "replay",
}

type humanQualityPhysicalCorpusManifest struct {
	Schema            string                       `json:"schema"`
	Version           int                          `json:"version"`
	BaseCommit        string                       `json:"base_commit"`
	FrozenAt          string                       `json:"frozen_at"`
	RequirementSchema string                       `json:"requirement_schema"`
	AuthoringPolicy   string                       `json:"authoring_policy"`
	Stages            []string                     `json:"stages"`
	PhysicalContract  humanQualityPhysicalContract `json:"physical_contract"`
	DesignCases       []humanQualityPhysicalCase   `json:"design_cases"`
}

type humanQualityPhysicalContract struct {
	MinimumSchematicSheets              int      `json:"minimum_schematic_sheets"`
	CopperLayers                        []string `json:"copper_layers"`
	GroundReferenceLayer                string   `json:"ground_reference_layer"`
	PowerDistributionLayer              string   `json:"power_distribution_layer"`
	MinimumFunctionalPlacementRegions   int      `json:"minimum_functional_placement_regions"`
	RequireExplicitInterSheetInterfaces bool     `json:"require_explicit_inter_sheet_interfaces"`
	RequireControlledReturnPaths        bool     `json:"require_controlled_return_paths"`
	RequireLayerTransitionEvidence      bool     `json:"require_layer_transition_evidence"`
	RequireThermalPlacement             bool     `json:"require_thermal_placement"`
	RequireZoneFill                     bool     `json:"require_zone_fill"`
	RequireCompleteRouting              bool     `json:"require_complete_routing"`
	RequireConnectivity                 bool     `json:"require_connectivity"`
	RequireWriterCorrectness            bool     `json:"require_writer_correctness"`
	RequireERC                          bool     `json:"require_erc"`
	RequireStrictDRC                    bool     `json:"require_strict_drc"`
	RequireRoundTripZeroDiff            bool     `json:"require_round_trip_zero_diff"`
	RequireDeterministicReplay          bool     `json:"require_deterministic_replay"`
}

type humanQualityPhysicalCase struct {
	ID                  string   `json:"id"`
	Category            string   `json:"category"`
	FunctionalBehaviors []string `json:"functional_behaviors"`
	RequirementFile     string   `json:"requirement_file"`
	RequirementSHA256   string   `json:"requirement_sha256"`
	RequiredAnalyses    []string `json:"required_analyses"`
}

func TestHumanQualityPhysicalCorpusIsFrozenBeforeProductionChanges(t *testing.T) {
	root := humanQualityPhysicalCorpusRoot()
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != humanQualityPhysicalCorpusManifestHash {
		t.Fatalf("manifest sha256 = %s, want %s", got, humanQualityPhysicalCorpusManifestHash)
	}
	if sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256")))); sidecar != humanQualityPhysicalCorpusManifestHash+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest humanQualityPhysicalCorpusManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != humanQualityPhysicalCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != humanQualityPhysicalCorpusBaseCommit ||
		manifest.RequirementSchema != RequirementSchema || strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" ||
		!slices.Equal(manifest.Stages, humanQualityPhysicalCorpusStages) {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	wantLayers := []string{"F.Cu", "In1.Cu", "In2.Cu", "B.Cu"}
	contract := manifest.PhysicalContract
	if contract.MinimumSchematicSheets < 2 || !slices.Equal(contract.CopperLayers, wantLayers) ||
		contract.GroundReferenceLayer != "In1.Cu" || contract.PowerDistributionLayer != "In2.Cu" ||
		contract.MinimumFunctionalPlacementRegions < 3 ||
		!contract.RequireExplicitInterSheetInterfaces || !contract.RequireControlledReturnPaths ||
		!contract.RequireLayerTransitionEvidence || !contract.RequireThermalPlacement ||
		!contract.RequireZoneFill || !contract.RequireCompleteRouting || !contract.RequireConnectivity ||
		!contract.RequireWriterCorrectness || !contract.RequireERC || !contract.RequireStrictDRC ||
		!contract.RequireRoundTripZeroDiff || !contract.RequireDeterministicReplay {
		t.Fatalf("physical contract = %#v", contract)
	}

	wantCategories := []string{"amplifier", "mixed_signal", "power", "protected_control"}
	categories := make([]string, 0, len(manifest.DesignCases))
	previousID := ""
	for _, entry := range manifest.DesignCases {
		if entry.ID <= previousID || len(entry.FunctionalBehaviors) < 3 ||
			!slices.IsSorted(entry.FunctionalBehaviors) || len(entry.RequiredAnalyses) < 4 ||
			!slices.IsSorted(entry.RequiredAnalyses) {
			t.Fatalf("invalid case after %q: %#v", previousID, entry)
		}
		previousID = entry.ID
		categories = append(categories, entry.Category)
		path := filepath.Clean(filepath.Join(root, entry.RequirementFile))
		data := mustRead(t, path)
		if got := frozenHash(data); got != entry.RequirementSHA256 {
			t.Fatalf("%s requirement sha256 = %s, want %s", entry.ID, got, entry.RequirementSHA256)
		}
		rejectFrozenImplementationDetail(t, entry.ID, data)
		var requirement Requirement
		decodeFrozenStrict(t, data, &requirement)
		if requirement.Schema != RequirementSchema || requirement.Version != RequirementVersion ||
			!nonlinearSwitchingCompleteAcceptance(requirement.Acceptance) {
			t.Fatalf("%s requirement identity/acceptance = %#v", entry.ID, requirement)
		}
		if issues := Validate(requirement); len(issues) != 0 {
			t.Fatalf("%s validation issues = %#v", entry.ID, issues)
		}
		analyses := []string{}
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			if !slices.Contains(analyses, assertion.Analysis) {
				analyses = append(analyses, assertion.Analysis)
			}
		}
		slices.Sort(analyses)
		if !slices.Equal(analyses, entry.RequiredAnalyses) {
			t.Fatalf("%s analyses = %v, want %v", entry.ID, analyses, entry.RequiredAnalyses)
		}
	}
	slices.Sort(categories)
	if !slices.Equal(categories, wantCategories) {
		t.Fatalf("categories = %v, want %v", categories, wantCategories)
	}
}

func humanQualityPhysicalCorpusRoot() string {
	return filepath.Join("testdata", "human_quality_physical_corpus")
}
