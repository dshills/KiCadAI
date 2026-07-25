package architecturesearch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const frozenHierarchicalManifestSHA256 = "a1472fc0a8df044ec7ff46af8fd751292ed568d4528da027ee6853571d365dbd"

type frozenHierarchicalManifest struct {
	Schema            string                          `json:"schema"`
	Version           int                             `json:"version"`
	FrozenAt          string                          `json:"frozen_at"`
	RequirementSchema string                          `json:"requirement_schema"`
	Fixtures          []frozenHierarchicalManifestRow `json:"fixtures"`
}

type frozenHierarchicalManifestRow struct {
	ID                       string   `json:"id"`
	File                     string   `json:"file"`
	Categories               []string `json:"categories"`
	Analyses                 []string `json:"analyses"`
	MinimumFunctionalBlocks  int      `json:"minimum_functional_blocks"`
	MinimumElectricalDomains int      `json:"minimum_electrical_domains"`
	SHA256                   string   `json:"sha256"`
}

type frozenHierarchicalRequirement struct {
	Schema       string                       `json:"schema"`
	Version      int                          `json:"version"`
	Project      frozenBehaviorProject        `json:"project"`
	Requirements frozenHierarchicalNeeds      `json:"requirements"`
	Acceptance   frozenHierarchicalAcceptance `json:"acceptance"`
}

type frozenHierarchicalNeeds struct {
	Domains                []frozenBehaviorDomain          `json:"domains"`
	Ports                  []frozenBehaviorPort            `json:"ports"`
	Signals                []frozenBehaviorPort            `json:"signals,omitempty"`
	Participants           []frozenHierarchicalParticipant `json:"participants,omitempty"`
	Objectives             []frozenHierarchicalObjective   `json:"objectives"`
	SystemConstraints      []frozenHierarchicalConstraint  `json:"system_constraints,omitempty"`
	OperatingCases         []frozenOperatingCase           `json:"operating_cases"`
	BehavioralRequirements []frozenBehaviorAssertion       `json:"behavioral_requirements"`
	Constraints            frozenBehaviorBoard             `json:"constraints"`
}

type frozenHierarchicalParticipant struct {
	ID            string                          `json:"id"`
	Capability    string                          `json:"capability"`
	Domain        string                          `json:"domain"`
	RequiredPorts []frozenBehaviorParticipantPort `json:"required_ports"`
	Constraints   []frozenHierarchicalConstraint  `json:"constraints,omitempty"`
}

type frozenHierarchicalObjective struct {
	ID          string                         `json:"id"`
	Capability  string                         `json:"capability"`
	Bindings    []frozenBehaviorBinding        `json:"bindings"`
	Constraints []frozenHierarchicalConstraint `json:"constraints,omitempty"`
}

type frozenHierarchicalConstraint struct {
	Name             string          `json:"name"`
	Relation         string          `json:"relation"`
	Value            json.RawMessage `json:"value"`
	Unit             string          `json:"unit,omitempty"`
	TolerancePercent float64         `json:"tolerance_percent,omitempty"`
}

type frozenHierarchicalAcceptance struct {
	RequireERC                       bool `json:"require_erc"`
	RequireStrictDRC                 bool `json:"require_strict_drc"`
	RequireCompleteRouting           bool `json:"require_complete_routing"`
	RequireConnectivity              bool `json:"require_connectivity"`
	RequireWriterCorrectness         bool `json:"require_writer_correctness"`
	RequireRoundTripZeroDiff         bool `json:"require_round_trip_zero_diff"`
	RequireDeterministicReplay       bool `json:"require_deterministic_replay"`
	RequireContractComposition       bool `json:"require_contract_composition"`
	RequireGlobalReasoning           bool `json:"require_global_reasoning"`
	RequireCoverageAccounting        bool `json:"require_coverage_accounting"`
	RequireAlternatives              bool `json:"require_alternatives"`
	RequireFailClosed                bool `json:"require_fail_closed"`
	RequireSimulation                bool `json:"require_simulation"`
	RequireAllCorners                bool `json:"require_all_corners"`
	RequireModelProvenance           bool `json:"require_model_provenance"`
	RequireClosedLoopEvidence        bool `json:"require_closed_loop_evidence"`
	RequireHierarchicalDecomposition bool `json:"require_hierarchical_decomposition"`
	RequireInterfaceContracts        bool `json:"require_interface_contracts"`
	RequireSharedResourcePlanning    bool `json:"require_shared_resource_planning"`
	RequireDeterministicBacktracking bool `json:"require_deterministic_backtracking"`
	RequirePhysicalPartitioning      bool `json:"require_physical_partitioning"`
	RequireEndToEndTraceability      bool `json:"require_end_to_end_traceability"`
}

func TestFrozenHierarchicalMultiDomainCorpusPrecedesProductionV4(t *testing.T) {
	root := frozenHierarchicalCorpusRoot()
	manifestData, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifestData)
	if got := hex.EncodeToString(digest[:]); got != frozenHierarchicalManifestSHA256 {
		t.Fatalf("manifest hash = %s, want %s", got, frozenHierarchicalManifestSHA256)
	}
	checksum, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if want := frozenHierarchicalManifestSHA256 + "  manifest.json\n"; string(checksum) != want {
		t.Fatalf("manifest.sha256 = %q, want %q", checksum, want)
	}

	var manifest frozenHierarchicalManifest
	decodeFrozenClosedLoopStrict(t, manifestData, &manifest)
	if manifest.Schema != "kicadai.hierarchical-multi-domain-corpus.v1" ||
		manifest.Version != 1 ||
		manifest.FrozenAt != "2026-07-25" ||
		manifest.RequirementSchema != "kicadai.open-set-requirement.v4" {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if len(manifest.Fixtures) != 6 {
		t.Fatalf("fixture count = %d, want 6", len(manifest.Fixtures))
	}

	requiredCategories := map[string]bool{
		"amplifier":  false,
		"analog":     false,
		"class_ab":   false,
		"digital":    false,
		"interface":  false,
		"isolation":  false,
		"mcu":        false,
		"noise":      false,
		"power":      false,
		"protection": false,
		"sensor":     false,
		"thermal":    false,
	}
	seenFiles := map[string]bool{"manifest.json": true}
	previousID := ""
	for _, row := range manifest.Fixtures {
		if row.ID <= previousID || row.File != row.ID+".json" || filepath.Base(row.File) != row.File {
			t.Fatalf("noncanonical or unsorted fixture row %#v after %q", row, previousID)
		}
		previousID = row.ID
		if row.MinimumFunctionalBlocks < 4 || row.MinimumElectricalDomains < 2 ||
			len(row.Categories) < 2 || !slices.IsSorted(row.Categories) ||
			len(row.Analyses) == 0 || !slices.IsSorted(row.Analyses) ||
			len(row.SHA256) != 64 {
			t.Fatalf("%s has incomplete or noncanonical manifest metadata: %#v", row.ID, row)
		}
		for _, category := range row.Categories {
			if _, required := requiredCategories[category]; required {
				requiredCategories[category] = true
			}
		}

		data, readErr := os.ReadFile(filepath.Join(root, row.File))
		if readErr != nil {
			t.Fatal(readErr)
		}
		fileDigest := sha256.Sum256(data)
		if got := hex.EncodeToString(fileDigest[:]); got != row.SHA256 {
			t.Fatalf("%s hash = %s, want %s", row.File, got, row.SHA256)
		}
		seenFiles[row.File] = true

		var requirement frozenHierarchicalRequirement
		decodeFrozenClosedLoopStrict(t, data, &requirement)
		validateFrozenHierarchicalRequirement(t, row, requirement, data)
	}
	for category, covered := range requiredCategories {
		if !covered {
			t.Errorf("frozen corpus lacks representative category %q", category)
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

func TestFrozenHierarchicalMultiDomainCorpusRecordsUnsupportedV4Baseline(t *testing.T) {
	root := frozenHierarchicalCorpusRoot()
	manifestData, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest frozenHierarchicalManifest
	decodeFrozenClosedLoopStrict(t, manifestData, &manifest)
	for _, row := range manifest.Fixtures {
		row := row
		t.Run(row.ID, func(t *testing.T) {
			file, openErr := os.Open(filepath.Join(root, row.File))
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer file.Close()
			_, issues := DecodeStrict(file)
			if len(issues) != 1 || issues[0].Code != CodeSchemaInvalid {
				t.Fatalf("production V4 baseline issues = %#v, want one %s", issues, CodeSchemaInvalid)
			}
		})
	}
}

func validateFrozenHierarchicalRequirement(t *testing.T, row frozenHierarchicalManifestRow, requirement frozenHierarchicalRequirement, raw []byte) {
	t.Helper()
	if requirement.Schema != "kicadai.open-set-requirement.v4" ||
		requirement.Version != 4 ||
		requirement.Project.Name != row.ID ||
		requirement.Project.Title == "" ||
		requirement.Project.Description == "" {
		t.Fatalf("%s has invalid identity", row.ID)
	}
	needs := requirement.Requirements
	if len(needs.Domains) < row.MinimumElectricalDomains {
		t.Fatalf("%s domains = %d, want at least %d", row.ID, len(needs.Domains), row.MinimumElectricalDomains)
	}
	blocks := len(needs.Objectives) + len(needs.Participants)
	if blocks < row.MinimumFunctionalBlocks {
		t.Fatalf("%s functional blocks = %d, want at least %d", row.ID, blocks, row.MinimumFunctionalBlocks)
	}
	if len(needs.Ports) == 0 || len(needs.OperatingCases) == 0 || len(needs.BehavioralRequirements) < 4 {
		t.Fatalf("%s lacks external interfaces, operating corners, or behavior assertions", row.ID)
	}
	if needs.Constraints.MaxComponents <= 0 || needs.Constraints.MaxWidthMM <= 0 || needs.Constraints.MaxHeightMM <= 0 {
		t.Fatalf("%s has invalid board limits", row.ID)
	}
	if !allHierarchicalAcceptanceGates(requirement.Acceptance) {
		t.Fatalf("%s does not require every promotion gate: %#v", row.ID, requirement.Acceptance)
	}

	domainIDs := map[string]bool{}
	hasReference := false
	hasSupply := false
	for _, domain := range needs.Domains {
		if domain.ID == "" || domain.Kind == "" || domain.Source == "" || domainIDs[domain.ID] {
			t.Fatalf("%s has invalid domain %#v", row.ID, domain)
		}
		domainIDs[domain.ID] = true
		hasReference = hasReference || domain.Kind == "reference"
		hasSupply = hasSupply || domain.Kind == "supply"
	}
	if !hasReference || !hasSupply {
		t.Fatalf("%s must span supply and reference electrical domains", row.ID)
	}

	hasSupplyCorner := false
	hasTemperatureCorner := false
	caseIDs := map[string]bool{}
	for _, operatingCase := range needs.OperatingCases {
		if operatingCase.ID == "" || caseIDs[operatingCase.ID] || len(operatingCase.Conditions) == 0 {
			t.Fatalf("%s has invalid operating case %#v", row.ID, operatingCase)
		}
		caseIDs[operatingCase.ID] = true
		for _, condition := range operatingCase.Conditions {
			if condition.Axis == "" || condition.Target == "" ||
				(condition.Min == nil && condition.Max == nil && condition.Selection == "") {
				t.Fatalf("%s has invalid operating condition %#v", row.ID, condition)
			}
			hasSupplyCorner = hasSupplyCorner || condition.Axis == "supply_voltage"
			hasTemperatureCorner = hasTemperatureCorner || condition.Axis == "ambient_temperature"
		}
	}
	if !hasSupplyCorner || !hasTemperatureCorner {
		t.Fatalf("%s lacks supply or environmental corner coverage", row.ID)
	}

	actualAnalyses := make([]string, 0, len(needs.BehavioralRequirements))
	assertionIDs := map[string]bool{}
	for _, assertion := range needs.BehavioralRequirements {
		if assertion.ID == "" || assertionIDs[assertion.ID] || assertion.Metric == "" ||
			assertion.Analysis == "" || assertion.Observation.Kind == "" ||
			assertion.Observation.ID == "" || assertion.Unit == "" ||
			(assertion.Min == nil && assertion.Max == nil) || len(assertion.OperatingCases) == 0 {
			t.Fatalf("%s has invalid behavioral requirement %#v", row.ID, assertion)
		}
		assertionIDs[assertion.ID] = true
		for _, caseID := range assertion.OperatingCases {
			if !caseIDs[caseID] {
				t.Fatalf("%s assertion %s references unknown case %s", row.ID, assertion.ID, caseID)
			}
		}
		actualAnalyses = append(actualAnalyses, assertion.Analysis)
	}
	slices.Sort(actualAnalyses)
	actualAnalyses = slices.Compact(actualAnalyses)
	if !slices.Equal(actualAnalyses, row.Analyses) {
		t.Fatalf("%s analyses = %v, manifest wants %v", row.ID, actualAnalyses, row.Analyses)
	}

	objectiveIDs := map[string]bool{}
	participantIDs := map[string]bool{}
	for _, participant := range needs.Participants {
		if participant.ID == "" || participant.Capability == "" || participantIDs[participant.ID] {
			t.Fatalf("%s has invalid participant %#v", row.ID, participant)
		}
		participantIDs[participant.ID] = true
	}
	sharedBoundaryUses := map[string]int{}
	for _, objective := range needs.Objectives {
		if objective.ID == "" || objective.Capability == "" || objectiveIDs[objective.ID] || len(objective.Bindings) == 0 {
			t.Fatalf("%s has invalid objective %#v", row.ID, objective)
		}
		objectiveIDs[objective.ID] = true
		for _, binding := range objective.Bindings {
			switch {
			case binding.Signal != "":
				sharedBoundaryUses["signal:"+binding.Signal]++
			case binding.Participant != "":
				sharedBoundaryUses["participant:"+binding.Participant+":"+binding.ParticipantPort]++
			}
		}
	}
	interacting := false
	for _, count := range sharedBoundaryUses {
		interacting = interacting || count >= 2
	}
	if !interacting {
		t.Fatalf("%s does not contain an interacting internal behavioral boundary", row.ID)
	}

	rejectFrozenImplementationDetail(t, row.ID, raw)
	rejectFrozenHierarchyHints(t, row.ID, raw)
}

func allHierarchicalAcceptanceGates(gates frozenHierarchicalAcceptance) bool {
	return gates.RequireERC &&
		gates.RequireStrictDRC &&
		gates.RequireCompleteRouting &&
		gates.RequireConnectivity &&
		gates.RequireWriterCorrectness &&
		gates.RequireRoundTripZeroDiff &&
		gates.RequireDeterministicReplay &&
		gates.RequireContractComposition &&
		gates.RequireGlobalReasoning &&
		gates.RequireCoverageAccounting &&
		gates.RequireAlternatives &&
		gates.RequireFailClosed &&
		gates.RequireSimulation &&
		gates.RequireAllCorners &&
		gates.RequireModelProvenance &&
		gates.RequireClosedLoopEvidence &&
		gates.RequireHierarchicalDecomposition &&
		gates.RequireInterfaceContracts &&
		gates.RequireSharedResourcePlanning &&
		gates.RequireDeterministicBacktracking &&
		gates.RequirePhysicalPartitioning &&
		gates.RequireEndToEndTraceability
}

func rejectFrozenHierarchyHints(t *testing.T, fixtureID string, raw []byte) {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	var requirements any
	if err := json.Unmarshal(document["requirements"], &requirements); err != nil {
		t.Fatal(err)
	}
	prohibited := []string{
		"block_family",
		"children",
		"contract",
		"hierarchy",
		"parent",
		"partition",
		"resource_plan",
		"subsystem",
	}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				lower := strings.ToLower(key)
				for _, blocked := range prohibited {
					if lower == blocked || strings.Contains(lower, blocked+"_") {
						t.Errorf("%s contains forbidden hierarchy hint %q", fixtureID, key)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(requirements)
}

func frozenHierarchicalCorpusRoot() string {
	return filepath.Join("testdata", "hierarchical_multi_domain_corpus")
}
