package architecturesearch

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const frozenDynamicElectrothermalManifestSHA256 = "6558d500373c74cffe44dffacc062a7ab30f7b1ec603ce1f5923869809d7658d"

type frozenDynamicManifest struct {
	Schema                              string                     `json:"schema"`
	Version                             int                        `json:"version"`
	FrozenAt                            string                     `json:"frozen_at"`
	RequirementSchema                   string                     `json:"requirement_schema"`
	MinimumDynamicAlternativeSelections int                        `json:"minimum_dynamic_alternative_selections"`
	Fixtures                            []frozenDynamicManifestRow `json:"fixtures"`
}

type frozenDynamicManifestRow struct {
	ID                       string   `json:"id"`
	File                     string   `json:"file"`
	Categories               []string `json:"categories"`
	Analyses                 []string `json:"analyses"`
	Events                   []string `json:"events"`
	MinimumFunctionalBlocks  int      `json:"minimum_functional_blocks"`
	MinimumElectricalDomains int      `json:"minimum_electrical_domains"`
	SHA256                   string   `json:"sha256"`
}

type frozenDynamicRequirement struct {
	Schema       string                  `json:"schema"`
	Version      int                     `json:"version"`
	Project      frozenBehaviorProject   `json:"project"`
	Requirements frozenDynamicNeeds      `json:"requirements"`
	Acceptance   frozenDynamicAcceptance `json:"acceptance"`
}

type frozenDynamicNeeds struct {
	Domains                []frozenBehaviorDomain          `json:"domains"`
	Ports                  []frozenBehaviorPort            `json:"ports"`
	Signals                []frozenBehaviorPort            `json:"signals,omitempty"`
	Participants           []frozenHierarchicalParticipant `json:"participants,omitempty"`
	Objectives             []frozenHierarchicalObjective   `json:"objectives"`
	SystemConstraints      []frozenHierarchicalConstraint  `json:"system_constraints,omitempty"`
	OperatingCases         []frozenDynamicOperatingCase    `json:"operating_cases"`
	BehavioralRequirements []frozenBehaviorAssertion       `json:"behavioral_requirements"`
	Constraints            frozenBehaviorBoard             `json:"constraints"`
}

type frozenDynamicOperatingCase struct {
	ID         string                     `json:"id"`
	Conditions []frozenOperatingCondition `json:"conditions"`
	Events     []frozenDynamicEvent       `json:"events"`
}

type frozenDynamicEvent struct {
	ID           string                    `json:"id"`
	Kind         string                    `json:"kind"`
	Target       frozenBehaviorObservation `json:"target"`
	TriggerTimeS float64                   `json:"trigger_time_s"`
	DurationS    float64                   `json:"duration_s"`
	Initial      *float64                  `json:"initial,omitempty"`
	Applied      float64                   `json:"applied"`
	Recovered    *float64                  `json:"recovered,omitempty"`
	Unit         string                    `json:"unit"`
}

type frozenDynamicAcceptance struct {
	RequireERC                           bool `json:"require_erc"`
	RequireStrictDRC                     bool `json:"require_strict_drc"`
	RequireCompleteRouting               bool `json:"require_complete_routing"`
	RequireConnectivity                  bool `json:"require_connectivity"`
	RequireWriterCorrectness             bool `json:"require_writer_correctness"`
	RequireRoundTripZeroDiff             bool `json:"require_round_trip_zero_diff"`
	RequireDeterministicReplay           bool `json:"require_deterministic_replay"`
	RequireContractComposition           bool `json:"require_contract_composition"`
	RequireGlobalReasoning               bool `json:"require_global_reasoning"`
	RequireCoverageAccounting            bool `json:"require_coverage_accounting"`
	RequireAlternatives                  bool `json:"require_alternatives"`
	RequireFailClosed                    bool `json:"require_fail_closed"`
	RequireSimulation                    bool `json:"require_simulation"`
	RequireAllCorners                    bool `json:"require_all_corners"`
	RequireModelProvenance               bool `json:"require_model_provenance"`
	RequireClosedLoopEvidence            bool `json:"require_closed_loop_evidence"`
	RequireHierarchicalDecomposition     bool `json:"require_hierarchical_decomposition"`
	RequireInterfaceContracts            bool `json:"require_interface_contracts"`
	RequireSharedResourcePlanning        bool `json:"require_shared_resource_planning"`
	RequireDeterministicBacktracking     bool `json:"require_deterministic_backtracking"`
	RequirePhysicalPartitioning          bool `json:"require_physical_partitioning"`
	RequireEndToEndTraceability          bool `json:"require_end_to_end_traceability"`
	RequireDynamicModelProvenance        bool `json:"require_dynamic_model_provenance"`
	RequireReturnRatioEvidence           bool `json:"require_return_ratio_evidence"`
	RequireDynamicElectrothermalEvidence bool `json:"require_dynamic_electrothermal_evidence"`
	RequireEventCoverage                 bool `json:"require_event_coverage"`
	RequireDynamicArchitectureSelection  bool `json:"require_dynamic_architecture_selection"`
	RequireBoundedDynamicRepair          bool `json:"require_bounded_dynamic_repair"`
}

func TestFrozenDynamicElectrothermalControlLoopCorpusPrecedesProductionV5(t *testing.T) {
	root := frozenDynamicElectrothermalCorpusRoot()
	manifestData, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifestData)
	if got := hex.EncodeToString(digest[:]); got != frozenDynamicElectrothermalManifestSHA256 {
		t.Fatalf("manifest hash = %s, want %s", got, frozenDynamicElectrothermalManifestSHA256)
	}
	checksum, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if want := frozenDynamicElectrothermalManifestSHA256 + "  manifest.json\n"; string(checksum) != want {
		t.Fatalf("manifest.sha256 = %q, want %q", checksum, want)
	}

	var manifest frozenDynamicManifest
	decodeFrozenClosedLoopStrict(t, manifestData, &manifest)
	if manifest.Schema != "kicadai.dynamic-electrothermal-control-loop-corpus.v1" ||
		manifest.Version != 1 ||
		manifest.FrozenAt != "2026-07-25" ||
		manifest.RequirementSchema != "kicadai.open-set-requirement.v5" ||
		manifest.MinimumDynamicAlternativeSelections < 2 ||
		len(manifest.Fixtures) != 6 {
		t.Fatalf("manifest identity or scope = %#v", manifest)
	}

	requiredCategories := stringSet("amplifier", "analog", "control", "conversion", "electrothermal", "inductive", "power", "protection", "sequencing", "servo", "switching")
	requiredAnalyses := stringSet("ac_sweep", "dc_operating_point", "distortion", "electrothermal", "stability", "transient")
	requiredEvents := stringSet("blocked_airflow", "inductive_turn_off", "input_step", "load_step", "overload", "rail_loss", "short_circuit", "shutdown", "startup")
	seenFiles := map[string]bool{"manifest.json": true}
	previousID := ""
	for _, row := range manifest.Fixtures {
		if row.ID <= previousID || row.File != row.ID+".json" || filepath.Base(row.File) != row.File {
			t.Fatalf("noncanonical fixture row %#v after %q", row, previousID)
		}
		previousID = row.ID
		if row.MinimumFunctionalBlocks < 4 || row.MinimumElectricalDomains < 2 ||
			len(row.Categories) < 3 || !slices.IsSorted(row.Categories) ||
			len(row.Analyses) < 2 || !slices.IsSorted(row.Analyses) ||
			len(row.Events) < 2 || !slices.IsSorted(row.Events) ||
			len(row.SHA256) != sha256.Size*2 {
			t.Fatalf("%s has incomplete manifest metadata: %#v", row.ID, row)
		}
		markCovered(requiredCategories, row.Categories)
		markCovered(requiredAnalyses, row.Analyses)
		markCovered(requiredEvents, row.Events)

		data, readErr := os.ReadFile(filepath.Join(root, row.File))
		if readErr != nil {
			t.Fatal(readErr)
		}
		fileDigest := sha256.Sum256(data)
		if got := hex.EncodeToString(fileDigest[:]); got != row.SHA256 {
			t.Fatalf("%s hash = %s, want %s", row.File, got, row.SHA256)
		}
		seenFiles[row.File] = true

		var requirement frozenDynamicRequirement
		decodeFrozenClosedLoopStrict(t, data, &requirement)
		validateFrozenDynamicRequirement(t, row, requirement, data)
	}
	requireAllCovered(t, "category", requiredCategories)
	requireAllCovered(t, "analysis", requiredAnalyses)
	requireAllCovered(t, "event", requiredEvents)

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

func TestFrozenDynamicElectrothermalControlLoopCorpusDecodesWithProductionV5(t *testing.T) {
	root := frozenDynamicElectrothermalCorpusRoot()
	manifestData, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest frozenDynamicManifest
	decodeFrozenClosedLoopStrict(t, manifestData, &manifest)
	for _, row := range manifest.Fixtures {
		file, openErr := os.Open(filepath.Join(root, row.File))
		if openErr != nil {
			t.Fatal(openErr)
		}
		requirement, issues := DecodeStrict(file)
		closeErr := file.Close()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if len(issues) != 0 {
			t.Fatalf("%s production V5 decode issues = %#v", row.ID, issues)
		}
		if requirement.Schema != SchemaIDV5 || requirement.Version != VersionV5 || requirement.Project.Name != row.ID {
			t.Fatalf("%s decoded identity = %#v", row.ID, requirement)
		}
	}
}

func validateFrozenDynamicRequirement(t *testing.T, row frozenDynamicManifestRow, requirement frozenDynamicRequirement, raw []byte) {
	t.Helper()
	if requirement.Schema != "kicadai.open-set-requirement.v5" ||
		requirement.Version != 5 ||
		requirement.Project.Name != row.ID ||
		requirement.Project.Title == "" ||
		requirement.Project.Description == "" {
		t.Fatalf("%s has invalid identity", row.ID)
	}
	needs := requirement.Requirements
	if len(needs.Domains) < row.MinimumElectricalDomains ||
		len(needs.Objectives)+len(needs.Participants) < row.MinimumFunctionalBlocks ||
		len(needs.Ports) == 0 ||
		len(needs.OperatingCases) == 0 ||
		len(needs.BehavioralRequirements) < 6 {
		t.Fatalf("%s lacks required dynamic behavior scope", row.ID)
	}
	caseIDs := map[string]bool{}
	eventIDs := map[string]bool{}
	for _, operatingCase := range needs.OperatingCases {
		if operatingCase.ID == "" || len(operatingCase.Conditions) == 0 || len(operatingCase.Events) == 0 || caseIDs[operatingCase.ID] {
			t.Fatalf("%s has invalid operating case %#v", row.ID, operatingCase)
		}
		caseIDs[operatingCase.ID] = true
		for _, event := range operatingCase.Events {
			if event.ID == "" || event.Kind == "" || event.Target.Kind == "" || event.Target.ID == "" ||
				event.TriggerTimeS < 0 || event.DurationS <= 0 || event.Unit == "" || eventIDs[event.ID] {
				t.Fatalf("%s has invalid or duplicate event %#v", row.ID, event)
			}
			eventIDs[event.ID] = true
		}
	}
	for _, assertion := range needs.BehavioralRequirements {
		if assertion.ID == "" || assertion.Metric == "" || assertion.Analysis == "" ||
			assertion.Observation.Kind == "" || assertion.Observation.ID == "" ||
			len(assertion.OperatingCases) == 0 || !assertion.Critical {
			t.Fatalf("%s has incomplete dynamic assertion %#v", row.ID, assertion)
		}
		for _, caseID := range assertion.OperatingCases {
			if !caseIDs[caseID] {
				t.Fatalf("%s assertion %s names unknown operating case %s", row.ID, assertion.ID, caseID)
			}
		}
		if assertion.Observation.Kind == "event" && !eventIDs[assertion.Observation.ID] {
			t.Fatalf("%s assertion %s names unknown event %s", row.ID, assertion.ID, assertion.Observation.ID)
		}
	}
	if !allDynamicAcceptanceGates(requirement.Acceptance) {
		t.Fatalf("%s does not require every V5 acceptance gate", row.ID)
	}
	rejectFrozenDynamicImplementationHints(t, row.ID, raw)
}

func allDynamicAcceptanceGates(value frozenDynamicAcceptance) bool {
	return value.RequireERC && value.RequireStrictDRC && value.RequireCompleteRouting &&
		value.RequireConnectivity && value.RequireWriterCorrectness && value.RequireRoundTripZeroDiff &&
		value.RequireDeterministicReplay && value.RequireContractComposition && value.RequireGlobalReasoning &&
		value.RequireCoverageAccounting && value.RequireAlternatives && value.RequireFailClosed &&
		value.RequireSimulation && value.RequireAllCorners && value.RequireModelProvenance &&
		value.RequireClosedLoopEvidence && value.RequireHierarchicalDecomposition &&
		value.RequireInterfaceContracts && value.RequireSharedResourcePlanning &&
		value.RequireDeterministicBacktracking && value.RequirePhysicalPartitioning &&
		value.RequireEndToEndTraceability && value.RequireDynamicModelProvenance &&
		value.RequireReturnRatioEvidence && value.RequireDynamicElectrothermalEvidence &&
		value.RequireEventCoverage && value.RequireDynamicArchitectureSelection &&
		value.RequireBoundedDynamicRepair
}

func rejectFrozenDynamicImplementationHints(t *testing.T, id string, raw []byte) {
	t.Helper()
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{
		`"catalog_id"`, `"component_id"`, `"coordinate"`, `"equation"`, `"expected_result"`,
		`"footprint"`, `"layer"`, `"loop_break"`, `"model_id"`, `"net"`, `"pin"`,
		`"primitive"`, `"provider"`, `"route"`, `"solver"`, `"topology"`,
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("%s contains forbidden implementation hint %s", id, forbidden)
		}
	}
}

func stringSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = false
	}
	return result
}

func markCovered(required map[string]bool, values []string) {
	for _, value := range values {
		if _, ok := required[value]; ok {
			required[value] = true
		}
	}
}

func requireAllCovered(t *testing.T, kind string, required map[string]bool) {
	t.Helper()
	for value, covered := range required {
		if !covered {
			t.Errorf("frozen corpus lacks required %s %q", kind, value)
		}
	}
}

func frozenDynamicElectrothermalCorpusRoot() string {
	return filepath.Join("testdata", "dynamic_electrothermal_control_loop_corpus")
}
