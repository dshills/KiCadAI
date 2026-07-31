package opentopologysynthesis

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/architecturesearch"
)

const (
	frozenCorpusSchema      = "kicadai.open-topology-held-out-corpus.v1"
	frozenRequirementSchema = "kicadai.open-topology-requirement.v1"
	frozenBaseCommit        = "8965a304fec2f0aae85ec57b830793204e8455df"
	frozenManifestSHA256    = "90a530d044693daa1a41669d26f867fd8b2a55d9e4e666e1f41a42f891e95835"
	frozenBaselineSchema    = "kicadai.open-topology-baseline.v1"
	frozenBaselineFailure   = "ARCHITECTURE_SCHEMA_INVALID"
	frozenRequiredCaseCount = 8
	frozenMinimumAssertions = 5
	frozenMinimumCaseCount  = 1
)

var frozenStages = []string{
	"integrity",
	"schema",
	"primitive_inventory",
	"topology_search",
	"value_search",
	"simulation",
	"diagnosis_repair",
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

type frozenManifest struct {
	Schema            string               `json:"schema"`
	Version           int                  `json:"version"`
	BaseCommit        string               `json:"base_commit"`
	FrozenAt          string               `json:"frozen_at"`
	RequirementSchema string               `json:"requirement_schema"`
	AuthoringPolicy   string               `json:"authoring_policy"`
	Stages            []string             `json:"stages"`
	Cases             []frozenManifestCase `json:"cases"`
}

type frozenManifestCase struct {
	ID                string   `json:"id"`
	BehaviorFamily    string   `json:"behavior_family"`
	RequirementFile   string   `json:"requirement_file"`
	RequirementSHA256 string   `json:"requirement_sha256"`
	RequiredAnalyses  []string `json:"required_analyses"`
	SafetyCritical    bool     `json:"safety_critical"`
}

type frozenRequirement struct {
	Schema       string             `json:"schema"`
	Version      int                `json:"version"`
	Project      frozenProject      `json:"project"`
	Requirements frozenRequirements `json:"requirements"`
	Acceptance   frozenAcceptance   `json:"acceptance"`
}

type frozenProject struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type frozenRequirements struct {
	Domains                []frozenDomain    `json:"domains"`
	Ports                  []frozenPort      `json:"ports"`
	OperatingCases         []frozenCase      `json:"operating_cases"`
	BehavioralRequirements []frozenAssertion `json:"behavioral_requirements"`
	Constraints            frozenBoardLimits `json:"constraints"`
}

type frozenDomain struct {
	ID              string  `json:"id"`
	Kind            string  `json:"kind"`
	MinVoltageV     float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV float64 `json:"nominal_voltage_v,omitempty"`
	MaxVoltageV     float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA     float64 `json:"max_current_a,omitempty"`
	Source          string  `json:"source"`
}

type frozenPort struct {
	ID         string           `json:"id"`
	Kind       string           `json:"kind"`
	Direction  string           `json:"direction"`
	Domain     string           `json:"domain"`
	Electrical frozenElectrical `json:"electrical,omitempty"`
}

type frozenElectrical struct {
	MinVoltageV          float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV      float64 `json:"nominal_voltage_v,omitempty"`
	MaxVoltageV          float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA          float64 `json:"max_current_a,omitempty"`
	InputImpedanceMinOhm float64 `json:"input_impedance_min_ohm,omitempty"`
	DefaultState         string  `json:"default_state,omitempty"`
}

type frozenCase struct {
	ID         string            `json:"id"`
	Conditions []frozenCondition `json:"conditions"`
	Events     []frozenEvent     `json:"events,omitempty"`
}

type frozenCondition struct {
	Axis   string  `json:"axis"`
	Target string  `json:"target"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Unit   string  `json:"unit"`
}

type frozenEvent struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Target       string  `json:"target"`
	TriggerTimeS float64 `json:"trigger_time_s"`
	Initial      float64 `json:"initial"`
	Applied      float64 `json:"applied"`
	Unit         string  `json:"unit"`
}

type frozenAssertion struct {
	ID             string             `json:"id"`
	Metric         string             `json:"metric"`
	Analysis       string             `json:"analysis"`
	Excitation     *frozenObservation `json:"excitation,omitempty"`
	Observation    frozenObservation  `json:"observation"`
	Min            *float64           `json:"min,omitempty"`
	Max            *float64           `json:"max,omitempty"`
	Unit           string             `json:"unit"`
	FrequencyHz    *float64           `json:"frequency_hz,omitempty"`
	OperatingCases []string           `json:"operating_cases"`
	Critical       bool               `json:"critical,omitempty"`
}

type frozenObservation struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type frozenBoardLimits struct {
	MaxComponents int     `json:"max_components"`
	MaxWidthMM    float64 `json:"max_width_mm"`
	MaxHeightMM   float64 `json:"max_height_mm"`
}

type frozenAcceptance struct {
	RequirePrimitiveOnly       bool `json:"require_primitive_only"`
	RequireTopologySearch      bool `json:"require_topology_search"`
	RequireSimulation          bool `json:"require_simulation"`
	RequireAllCorners          bool `json:"require_all_corners"`
	RequireModelProvenance     bool `json:"require_model_provenance"`
	RequireClosedLoopEvidence  bool `json:"require_closed_loop_evidence"`
	RequireCompleteRouting     bool `json:"require_complete_routing"`
	RequireConnectivity        bool `json:"require_connectivity"`
	RequireWriterCorrectness   bool `json:"require_writer_correctness"`
	RequireRoundTripZeroDiff   bool `json:"require_round_trip_zero_diff"`
	RequireERC                 bool `json:"require_erc"`
	RequireStrictDRC           bool `json:"require_strict_drc"`
	RequireDeterministicReplay bool `json:"require_deterministic_replay"`
	RequireFailClosed          bool `json:"require_fail_closed"`
}

type frozenBaseline struct {
	Schema       string                `json:"schema"`
	Version      int                   `json:"version"`
	BaseCommit   string                `json:"base_commit"`
	ManifestHash string                `json:"manifest_sha256"`
	MeasuredAt   string                `json:"measured_at"`
	Summary      frozenBaselineSummary `json:"summary"`
	Cases        []frozenBaselineCase  `json:"cases"`
}

type frozenBaselineSummary struct {
	Total          int `json:"total"`
	CompletePasses int `json:"complete_passes"`
	FailClosed     int `json:"fail_closed"`
}

type frozenBaselineCase struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	StoppedAt      string `json:"stopped_at"`
	Code           string `json:"code"`
	ProviderBypass bool   `json:"provider_bypass"`
}

func TestHeldOutCorpusIsFrozenBeforeProductionSearch(t *testing.T) {
	root := frozenCorpusRoot()
	manifestBytes := mustRead(t, filepath.Join(root, "manifest.json"))
	if got := frozenHash(manifestBytes); got != frozenManifestSHA256 {
		t.Fatalf("manifest sha256 = %s, want %s", got, frozenManifestSHA256)
	}
	sidecar := strings.TrimSpace(string(mustRead(t, filepath.Join(root, "manifest.sha256"))))
	if sidecar != frozenManifestSHA256+"  manifest.json" {
		t.Fatalf("manifest checksum sidecar = %q", sidecar)
	}

	var manifest frozenManifest
	decodeFrozenStrict(t, manifestBytes, &manifest)
	if manifest.Schema != frozenCorpusSchema || manifest.Version != 1 ||
		manifest.BaseCommit != frozenBaseCommit ||
		manifest.RequirementSchema != frozenRequirementSchema ||
		strings.TrimSpace(manifest.FrozenAt) == "" ||
		strings.TrimSpace(manifest.AuthoringPolicy) == "" {
		t.Fatalf("manifest identity = %#v", manifest)
	}
	if !slices.Equal(manifest.Stages, frozenStages) {
		t.Fatalf("stages = %v, want %v", manifest.Stages, frozenStages)
	}
	if len(manifest.Cases) != frozenRequiredCaseCount {
		t.Fatalf("case count = %d, want %d", len(manifest.Cases), frozenRequiredCaseCount)
	}

	seenFiles := map[string]bool{"manifest.json": true}
	seenFamilies := map[string]bool{}
	previousID := ""
	for _, entry := range manifest.Cases {
		if entry.ID <= previousID {
			t.Fatalf("case IDs are not unique and strictly sorted: %q after %q", entry.ID, previousID)
		}
		previousID = entry.ID
		if strings.TrimSpace(entry.BehaviorFamily) == "" || seenFamilies[entry.BehaviorFamily] {
			t.Fatalf("%s behavior family must be nonempty and unique", entry.ID)
		}
		seenFamilies[entry.BehaviorFamily] = true
		if len(entry.RequiredAnalyses) < 3 || !slices.IsSorted(entry.RequiredAnalyses) {
			t.Fatalf("%s required analyses are incomplete or unsorted: %v", entry.ID, entry.RequiredAnalyses)
		}

		path := filepath.Join(root, entry.RequirementFile)
		if filepath.Base(path) != entry.RequirementFile || seenFiles[entry.RequirementFile] {
			t.Fatalf("%s has unsafe or duplicate requirement file %q", entry.ID, entry.RequirementFile)
		}
		seenFiles[entry.RequirementFile] = true
		data := mustRead(t, path)
		if got := frozenHash(data); got != entry.RequirementSHA256 {
			t.Fatalf("%s sha256 = %s, want %s", entry.ID, got, entry.RequirementSHA256)
		}
		rejectFrozenImplementationDetail(t, entry.ID, data)

		var requirement frozenRequirement
		decodeFrozenStrict(t, data, &requirement)
		validateFrozenRequirement(t, entry, requirement)

		_, issues := architecturesearch.DecodeStrict(bytes.NewReader(data))
		if len(issues) == 0 || issues[0].Code != architecturesearch.CodeSchemaInvalid {
			t.Fatalf("%s provider-backed decoder did not fail closed: %#v", entry.ID, issues)
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

func TestUntouchedBaselineMatchesFrozenCorpus(t *testing.T) {
	var manifest frozenManifest
	decodeFrozenStrict(t, mustRead(t, filepath.Join(frozenCorpusRoot(), "manifest.json")), &manifest)

	var baseline frozenBaseline
	decodeFrozenStrict(t, mustRead(t, filepath.Join("..", "..", "specs", "simulation-guided-open-topology-synthesis", "BASELINE_REPORT.json")), &baseline)
	if baseline.Schema != frozenBaselineSchema || baseline.Version != 1 ||
		baseline.BaseCommit != frozenBaseCommit ||
		baseline.ManifestHash != frozenManifestSHA256 ||
		strings.TrimSpace(baseline.MeasuredAt) == "" {
		t.Fatalf("baseline identity = %#v", baseline)
	}
	if baseline.Summary.Total != frozenRequiredCaseCount ||
		baseline.Summary.CompletePasses != 0 ||
		baseline.Summary.FailClosed != frozenRequiredCaseCount ||
		len(baseline.Cases) != len(manifest.Cases) {
		t.Fatalf("baseline summary = %#v", baseline.Summary)
	}
	for index, result := range baseline.Cases {
		if result.ID != manifest.Cases[index].ID ||
			result.Status != "fail_closed" ||
			result.StoppedAt != "schema" ||
			result.Code != frozenBaselineFailure ||
			result.ProviderBypass {
			t.Errorf("baseline case %d = %#v", index, result)
		}
	}
}

func validateFrozenRequirement(t *testing.T, entry frozenManifestCase, requirement frozenRequirement) {
	t.Helper()
	if requirement.Schema != frozenRequirementSchema || requirement.Version != 1 ||
		requirement.Project.Name != entry.ID ||
		strings.TrimSpace(requirement.Project.Title) == "" ||
		strings.TrimSpace(requirement.Project.Description) == "" {
		t.Fatalf("%s requirement identity = %#v", entry.ID, requirement.Project)
	}
	if len(requirement.Requirements.Domains) < 2 ||
		len(requirement.Requirements.Ports) < 4 ||
		len(requirement.Requirements.OperatingCases) < frozenMinimumCaseCount ||
		len(requirement.Requirements.BehavioralRequirements) < frozenMinimumAssertions {
		t.Fatalf("%s lacks interfaces, cases, or measurable behavior", entry.ID)
	}
	if requirement.Requirements.Constraints.MaxComponents <= 0 ||
		requirement.Requirements.Constraints.MaxWidthMM <= 0 ||
		requirement.Requirements.Constraints.MaxHeightMM <= 0 {
		t.Fatalf("%s has invalid physical bounds", entry.ID)
	}
	if !completeFrozenAcceptance(requirement.Acceptance) {
		t.Fatalf("%s does not require the complete acceptance profile", entry.ID)
	}

	domains := map[string]bool{}
	for _, domain := range requirement.Requirements.Domains {
		if domain.ID == "" || domains[domain.ID] || domain.Source != "external" {
			t.Fatalf("%s invalid domain %#v", entry.ID, domain)
		}
		domains[domain.ID] = true
	}
	ports := map[string]bool{}
	for _, port := range requirement.Requirements.Ports {
		if port.ID == "" || ports[port.ID] || !domains[port.Domain] {
			t.Fatalf("%s invalid port %#v", entry.ID, port)
		}
		ports[port.ID] = true
	}
	cases := map[string]bool{}
	analyses := map[string]bool{}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		if operatingCase.ID == "" || cases[operatingCase.ID] || len(operatingCase.Conditions) == 0 {
			t.Fatalf("%s invalid operating case %#v", entry.ID, operatingCase)
		}
		cases[operatingCase.ID] = true
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.ID == "" || assertion.Metric == "" || assertion.Analysis == "" ||
			assertion.Unit == "" || (assertion.Min == nil && assertion.Max == nil) ||
			len(assertion.OperatingCases) == 0 {
			t.Fatalf("%s invalid behavioral assertion %#v", entry.ID, assertion)
		}
		analyses[assertion.Analysis] = true
		validateFrozenObservation(t, entry.ID, assertion.Observation, ports, domains, true)
		if assertion.Excitation != nil {
			validateFrozenObservation(t, entry.ID, *assertion.Excitation, ports, domains, false)
		}
		for _, caseID := range assertion.OperatingCases {
			if !cases[caseID] {
				t.Fatalf("%s assertion %s refers to unknown case %s", entry.ID, assertion.ID, caseID)
			}
		}
	}
	for _, analysis := range entry.RequiredAnalyses {
		if !analyses[analysis] {
			t.Fatalf("%s manifest analysis %s is not required by the fixture", entry.ID, analysis)
		}
	}
}

func validateFrozenObservation(t *testing.T, caseID string, observation frozenObservation, ports, domains map[string]bool, allowCircuit bool) {
	t.Helper()
	switch observation.Kind {
	case "port":
		if !ports[observation.ID] {
			t.Fatalf("%s observation refers to unknown port %s", caseID, observation.ID)
		}
	case "domain":
		if !domains[observation.ID] {
			t.Fatalf("%s observation refers to unknown domain %s", caseID, observation.ID)
		}
	case "circuit":
		if !allowCircuit || observation.ID == "" {
			t.Fatalf("%s invalid circuit observation %#v", caseID, observation)
		}
	default:
		t.Fatalf("%s invalid observation %#v", caseID, observation)
	}
}

func completeFrozenAcceptance(value frozenAcceptance) bool {
	return value.RequirePrimitiveOnly &&
		value.RequireTopologySearch &&
		value.RequireSimulation &&
		value.RequireAllCorners &&
		value.RequireModelProvenance &&
		value.RequireClosedLoopEvidence &&
		value.RequireCompleteRouting &&
		value.RequireConnectivity &&
		value.RequireWriterCorrectness &&
		value.RequireRoundTripZeroDiff &&
		value.RequireERC &&
		value.RequireStrictDRC &&
		value.RequireDeterministicReplay &&
		value.RequireFailClosed
}

var frozenImplementationText = regexp.MustCompile(`(?i)\b(mpn|manufacturer|catalog|symbol|footprint|pin|pad|net|topology|schematic|provider|expansion|solver|model|formula|coordinate|layer|track|route|via|repair|resistor|capacitor|inductor|diode|transistor|mosfet|bjt|op[- ]?amp|comparator)\b`)

func rejectFrozenImplementationDetail(t *testing.T, id string, data []byte) {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	blockedKeys := map[string]bool{
		"component_id":     true,
		"manufacturer":     true,
		"mpn":              true,
		"catalog_id":       true,
		"symbol":           true,
		"footprint":        true,
		"pin":              true,
		"pad":              true,
		"net":              true,
		"topology":         true,
		"block_family":     true,
		"provider_id":      true,
		"expansion_id":     true,
		"model_id":         true,
		"solver":           true,
		"formula":          true,
		"coordinate":       true,
		"layer":            true,
		"track":            true,
		"route":            true,
		"via":              true,
		"repair_action":    true,
		"expected_part":    true,
		"expected_value":   true,
		"primitive_family": true,
		"objectives":       true,
		"participants":     true,
		"signals":          true,
	}
	var walk func(any, string)
	walk = func(current any, path string) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				lowerKey := strings.ToLower(key)
				if blockedKeys[lowerKey] {
					t.Errorf("%s contains implementation field %s.%s", id, path, key)
				}
				if lowerKey == "schema" {
					continue
				}
				walk(child, path+"."+key)
			}
		case []any:
			for index, child := range typed {
				walk(child, path+"["+frozenIndex(index)+"]")
			}
		case string:
			if match := frozenImplementationText.FindString(typed); match != "" {
				t.Errorf("%s contains implementation text %q at %s", id, match, path)
			}
		}
	}
	walk(value, "$")
}

func decodeFrozenStrict(t *testing.T, data []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("unexpected trailing JSON content: %v", err)
	}
}

func frozenCorpusRoot() string {
	return filepath.Join("testdata", "held_out_corpus")
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func frozenHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func frozenIndex(index int) string {
	const digits = "0123456789"
	if index == 0 {
		return "0"
	}
	result := ""
	for index > 0 {
		result = string(digits[index%10]) + result
		index /= 10
	}
	return result
}
