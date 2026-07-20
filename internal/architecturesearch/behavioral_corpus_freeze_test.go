package architecturesearch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const frozenClosedLoopCorpusSchema = "kicadai.simulation-grounded-closed-loop-corpus.v1"

type frozenClosedLoopManifest struct {
	Schema            string                        `json:"schema"`
	Version           int                           `json:"version"`
	FrozenAt          string                        `json:"frozen_at"`
	RequirementSchema string                        `json:"requirement_schema"`
	Fixtures          []frozenClosedLoopManifestRow `json:"fixtures"`
}

type frozenClosedLoopManifestRow struct {
	ID         string   `json:"id"`
	File       string   `json:"file"`
	Categories []string `json:"categories"`
	Analyses   []string `json:"analyses"`
	SHA256     string   `json:"sha256"`
}

type frozenBehaviorRequirement struct {
	Schema       string                   `json:"schema"`
	Version      int                      `json:"version"`
	Project      frozenBehaviorProject    `json:"project"`
	Requirements frozenBehaviorNeeds      `json:"requirements"`
	Acceptance   frozenBehaviorAcceptance `json:"acceptance"`
}

type frozenBehaviorProject struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type frozenBehaviorNeeds struct {
	Domains                []frozenBehaviorDomain      `json:"domains"`
	Ports                  []frozenBehaviorPort        `json:"ports"`
	Signals                []frozenBehaviorPort        `json:"signals,omitempty"`
	Participants           []frozenBehaviorParticipant `json:"participants,omitempty"`
	Objectives             []frozenBehaviorObjective   `json:"objectives"`
	SystemConstraints      []frozenBehaviorConstraint  `json:"system_constraints,omitempty"`
	OperatingCases         []frozenOperatingCase       `json:"operating_cases"`
	BehavioralRequirements []frozenBehaviorAssertion   `json:"behavioral_requirements"`
	Constraints            frozenBehaviorBoard         `json:"constraints"`
}

type frozenBehaviorDomain struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	MinVoltageV     float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV float64 `json:"nominal_voltage_v,omitempty"`
	MaxVoltageV     float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA     float64 `json:"max_current_a,omitempty"`
	Source          string  `json:"source"`
}

type frozenBehaviorPort struct {
	ID         string                   `json:"id"`
	Kind       string                   `json:"kind"`
	Direction  string                   `json:"direction,omitempty"`
	Domain     string                   `json:"domain"`
	Electrical frozenBehaviorElectrical `json:"electrical,omitempty"`
	Protocol   frozenBehaviorProtocol   `json:"protocol,omitempty"`
}

type frozenBehaviorElectrical struct {
	MinVoltageV          float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV      float64 `json:"nominal_voltage_v,omitempty"`
	MaxVoltageV          float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA          float64 `json:"max_current_a,omitempty"`
	MaxSourceCurrentMA   float64 `json:"max_source_current_ma,omitempty"`
	InputImpedanceMinOhm float64 `json:"input_impedance_min_ohm,omitempty"`
	FrequencyMaxHz       float64 `json:"frequency_max_hz,omitempty"`
	DefaultState         string  `json:"default_state,omitempty"`
}

type frozenBehaviorProtocol struct {
	Name           string  `json:"name,omitempty"`
	Mode           string  `json:"mode,omitempty"`
	MaxFrequencyHz float64 `json:"max_frequency_hz,omitempty"`
}

type frozenBehaviorParticipant struct {
	ID            string                          `json:"id"`
	Capability    string                          `json:"capability"`
	Domain        string                          `json:"domain"`
	RequiredPorts []frozenBehaviorParticipantPort `json:"required_ports"`
	Constraints   []frozenBehaviorConstraint      `json:"constraints,omitempty"`
}

type frozenBehaviorParticipantPort struct {
	ID        string                 `json:"id"`
	Kind      string                 `json:"kind"`
	Direction string                 `json:"direction"`
	Protocol  frozenBehaviorProtocol `json:"protocol,omitempty"`
}

type frozenBehaviorObjective struct {
	ID          string                     `json:"id"`
	Capability  string                     `json:"capability"`
	Bindings    []frozenBehaviorBinding    `json:"bindings"`
	Constraints []frozenBehaviorConstraint `json:"constraints,omitempty"`
}

type frozenBehaviorBinding struct {
	Role            string `json:"role"`
	Port            string `json:"port,omitempty"`
	Signal          string `json:"signal,omitempty"`
	Direction       string `json:"direction,omitempty"`
	Participant     string `json:"participant,omitempty"`
	ParticipantPort string `json:"participant_port,omitempty"`
}

type frozenBehaviorConstraint struct {
	Name             string  `json:"name"`
	Relation         string  `json:"relation"`
	Value            float64 `json:"value"`
	Unit             string  `json:"unit,omitempty"`
	TolerancePercent float64 `json:"tolerance_percent,omitempty"`
}

type frozenBehaviorBoard struct {
	MaxComponents int     `json:"max_components"`
	MaxWidthMM    float64 `json:"max_width_mm"`
	MaxHeightMM   float64 `json:"max_height_mm"`
}

type frozenOperatingCase struct {
	ID         string                     `json:"id"`
	Conditions []frozenOperatingCondition `json:"conditions"`
}

type frozenOperatingCondition struct {
	Axis      string   `json:"axis"`
	Target    string   `json:"target"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Unit      string   `json:"unit,omitempty"`
	Selection string   `json:"selection,omitempty"`
}

type frozenBehaviorAssertion struct {
	ID             string                    `json:"id"`
	Metric         string                    `json:"metric"`
	Analysis       string                    `json:"analysis"`
	Observation    frozenBehaviorObservation `json:"observation"`
	Min            *float64                  `json:"min,omitempty"`
	Max            *float64                  `json:"max,omitempty"`
	Unit           string                    `json:"unit"`
	OperatingCases []string                  `json:"operating_cases"`
	Critical       bool                      `json:"critical,omitempty"`
}

type frozenBehaviorObservation struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type frozenBehaviorAcceptance struct {
	RequireERC                 bool `json:"require_erc"`
	RequireStrictDRC           bool `json:"require_strict_drc"`
	RequireCompleteRouting     bool `json:"require_complete_routing"`
	RequireConnectivity        bool `json:"require_connectivity"`
	RequireWriterCorrectness   bool `json:"require_writer_correctness"`
	RequireRoundTripZeroDiff   bool `json:"require_round_trip_zero_diff"`
	RequireDeterministicReplay bool `json:"require_deterministic_replay"`
	RequireContractComposition bool `json:"require_contract_composition"`
	RequireGlobalReasoning     bool `json:"require_global_reasoning"`
	RequireCoverageAccounting  bool `json:"require_coverage_accounting"`
	RequireAlternatives        bool `json:"require_alternatives"`
	RequireFailClosed          bool `json:"require_fail_closed"`
	RequireSimulation          bool `json:"require_simulation"`
	RequireAllCorners          bool `json:"require_all_corners"`
	RequireModelProvenance     bool `json:"require_model_provenance"`
	RequireClosedLoopEvidence  bool `json:"require_closed_loop_evidence"`
}

func TestFrozenSimulationGroundedClosedLoopCorpusPrecedesProductionV3(t *testing.T) {
	root := frozenClosedLoopCorpusRoot()
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest frozenClosedLoopManifest
	decodeFrozenClosedLoopStrict(t, manifestBytes, &manifest)
	if manifest.Schema != frozenClosedLoopCorpusSchema || manifest.Version != 1 || manifest.RequirementSchema != "kicadai.open-set-requirement.v3" {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if len(manifest.Fixtures) != 10 {
		t.Fatalf("fixture count = %d, want 10", len(manifest.Fixtures))
	}

	wantAnalyses := []string{"ac_sweep", "dc_operating_point", "distortion", "noise", "stability", "startup", "thermal", "transient"}
	seenAnalyses := map[string]bool{}
	seenCategories := map[string]bool{}
	seenFiles := map[string]bool{"manifest.json": true}
	previousID := ""
	for _, entry := range manifest.Fixtures {
		if entry.ID <= previousID {
			t.Fatalf("manifest IDs are not strictly sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		if entry.File != entry.ID+".json" || filepath.Base(entry.File) != entry.File {
			t.Fatalf("unsafe or noncanonical fixture path %#v", entry)
		}
		if len(entry.SHA256) != 64 {
			t.Fatalf("%s sha256 = %q", entry.ID, entry.SHA256)
		}
		data, readErr := os.ReadFile(filepath.Join(root, entry.File))
		if readErr != nil {
			t.Fatal(readErr)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != entry.SHA256 {
			t.Fatalf("%s sha256 = %s, want %s", entry.ID, got, entry.SHA256)
		}
		seenFiles[entry.File] = true
		for _, analysis := range entry.Analyses {
			seenAnalyses[analysis] = true
		}
		for _, category := range entry.Categories {
			seenCategories[category] = true
		}

		var requirement frozenBehaviorRequirement
		decodeFrozenClosedLoopStrict(t, data, &requirement)
		validateFrozenBehaviorRequirement(t, entry, requirement, data)
	}
	for _, analysis := range wantAnalyses {
		if !seenAnalyses[analysis] {
			t.Errorf("missing representative analysis %q", analysis)
		}
	}
	for _, category := range []string{"amplifier", "class_a", "class_ab", "control", "digital", "filter", "interface", "noise", "power", "protection", "sensor", "thermal"} {
		if !seenCategories[category] {
			t.Errorf("missing representative category %q", category)
		}
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

func TestFrozenSimulationGroundedClosedLoopCorpusDecodesWithProductionV3(t *testing.T) {
	root := frozenClosedLoopCorpusRoot()
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest frozenClosedLoopManifest
	decodeFrozenClosedLoopStrict(t, manifestBytes, &manifest)
	for _, entry := range manifest.Fixtures {
		entry := entry
		t.Run(entry.ID, func(t *testing.T) {
			file, openErr := os.Open(filepath.Join(root, entry.File))
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer file.Close()
			requirement, issues := DecodeStrict(file)
			if len(issues) != 0 {
				t.Fatalf("production v3 decode issues: %#v", issues)
			}
			if requirement.Schema != SchemaIDV3 || requirement.Version != VersionV3 || requirement.Project.Name != entry.ID {
				t.Fatalf("decoded identity = %#v", requirement)
			}
			if len(requirement.Requirements.OperatingCases) == 0 || len(requirement.Requirements.BehavioralRequirements) == 0 {
				t.Fatalf("decoded v3 lost behavior fields: %#v", requirement.Requirements)
			}
			firstHash, hashErr := CanonicalHash(requirement)
			if hashErr != nil {
				t.Fatal(hashErr)
			}
			reversed := cloneRequirement(requirement)
			slices.Reverse(reversed.Requirements.OperatingCases)
			slices.Reverse(reversed.Requirements.BehavioralRequirements)
			for index := range reversed.Requirements.OperatingCases {
				slices.Reverse(reversed.Requirements.OperatingCases[index].Conditions)
			}
			for index := range reversed.Requirements.BehavioralRequirements {
				slices.Reverse(reversed.Requirements.BehavioralRequirements[index].OperatingCases)
			}
			secondHash, hashErr := CanonicalHash(reversed)
			if hashErr != nil {
				t.Fatal(hashErr)
			}
			if firstHash != secondHash {
				t.Fatalf("canonical hash changed under v3 reordering: %s != %s", firstHash, secondHash)
			}
		})
	}
}

func validateFrozenBehaviorRequirement(t *testing.T, entry frozenClosedLoopManifestRow, requirement frozenBehaviorRequirement, raw []byte) {
	t.Helper()
	if requirement.Schema != "kicadai.open-set-requirement.v3" || requirement.Version != 3 || requirement.Project.Name != entry.ID {
		t.Fatalf("%s identity = %#v", entry.ID, requirement)
	}
	if len(requirement.Requirements.Objectives) < 2 || len(requirement.Requirements.Objectives) > 4 {
		t.Fatalf("%s objectives = %d, want 2..4", entry.ID, len(requirement.Requirements.Objectives))
	}
	if len(requirement.Requirements.OperatingCases) == 0 || len(requirement.Requirements.BehavioralRequirements) < 4 {
		t.Fatalf("%s lacks operating cases or behavioral requirements", entry.ID)
	}
	acceptance := requirement.Acceptance
	if !acceptance.RequireERC || !acceptance.RequireStrictDRC || !acceptance.RequireCompleteRouting || !acceptance.RequireConnectivity || !acceptance.RequireWriterCorrectness || !acceptance.RequireRoundTripZeroDiff || !acceptance.RequireDeterministicReplay || !acceptance.RequireContractComposition || !acceptance.RequireGlobalReasoning || !acceptance.RequireCoverageAccounting || !acceptance.RequireAlternatives || !acceptance.RequireFailClosed || !acceptance.RequireSimulation || !acceptance.RequireAllCorners || !acceptance.RequireModelProvenance || !acceptance.RequireClosedLoopEvidence {
		t.Fatalf("%s does not require every promotion gate", entry.ID)
	}
	cases := map[string]bool{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		if operatingCase.ID == "" || cases[operatingCase.ID] || len(operatingCase.Conditions) == 0 {
			t.Fatalf("%s invalid operating case %#v", entry.ID, operatingCase)
		}
		cases[operatingCase.ID] = true
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "" || condition.Target == "" || (condition.Min == nil && condition.Max == nil && condition.Selection == "") {
				t.Fatalf("%s invalid operating condition %#v", entry.ID, condition)
			}
		}
	}
	assertionIDs := map[string]bool{}
	manifestAnalyses := append([]string(nil), entry.Analyses...)
	slices.Sort(manifestAnalyses)
	actualAnalyses := []string{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.ID == "" || assertionIDs[assertion.ID] || assertion.Metric == "" || assertion.Analysis == "" || assertion.Observation.Kind == "" || assertion.Observation.ID == "" || assertion.Unit == "" || (assertion.Min == nil && assertion.Max == nil) {
			t.Fatalf("%s invalid behavioral requirement %#v", entry.ID, assertion)
		}
		assertionIDs[assertion.ID] = true
		for _, bound := range []*float64{assertion.Min, assertion.Max} {
			if bound != nil && (math.IsNaN(*bound) || math.IsInf(*bound, 0)) {
				t.Fatalf("%s non-finite assertion %#v", entry.ID, assertion)
			}
		}
		for _, caseID := range assertion.OperatingCases {
			if !cases[caseID] {
				t.Fatalf("%s assertion %s references unknown case %q", entry.ID, assertion.ID, caseID)
			}
		}
		actualAnalyses = append(actualAnalyses, assertion.Analysis)
	}
	slices.Sort(actualAnalyses)
	actualAnalyses = slices.Compact(actualAnalyses)
	if !slices.Equal(actualAnalyses, manifestAnalyses) {
		t.Fatalf("%s manifest analyses = %v, assertions use %v", entry.ID, manifestAnalyses, actualAnalyses)
	}
	rejectFrozenImplementationDetail(t, entry.ID, raw)
}

func rejectFrozenImplementationDetail(t *testing.T, fixtureID string, raw []byte) {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	prohibited := []string{"component_id", "variant_id", "provider_id", "expansion_id", "footprint", "symbol", "pin", "pad", "coordinate", "x_mm", "y_mm", "layer", "track", "route", "via", "topology", "repair_action", "expected_part", "expected_value"}
	var walk func(any, string)
	walk = func(current any, path string) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				lower := strings.ToLower(key)
				for _, blocked := range prohibited {
					if lower == blocked || strings.Contains(lower, blocked+"_") {
						t.Errorf("%s contains implementation field %s.%s", fixtureID, path, key)
					}
				}
				walk(child, path+"."+key)
			}
		case []any:
			for index, child := range typed {
				walk(child, fmt.Sprintf("%s[%d]", path, index))
			}
		}
	}
	walk(value, "$.")
}

func decodeFrozenClosedLoopStrict(t *testing.T, data []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if decoder.More() {
		t.Fatal("unexpected trailing JSON")
	}
}

func frozenClosedLoopCorpusRoot() string {
	return filepath.Join("testdata", "simulation_grounded_closed_loop_corpus")
}
