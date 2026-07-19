package circuitgraph

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const frozenOpenSetCompositionManifestSHA256 = "f3a9b341d64186b6063b9f308ff9bdcf66f2421bf12068eba7f9a1a3ac3580b1"

type openSetCompositionManifest struct {
	Schema        string                              `json:"schema"`
	Version       int                                 `json:"version"`
	FrozenAt      string                              `json:"frozen_at"`
	PolicyVersion string                              `json:"policy_version"`
	Fixtures      []openSetCompositionManifestFixture `json:"fixtures"`
}

type openSetCompositionManifestFixture struct {
	ID           string   `json:"id"`
	File         string   `json:"file"`
	Capabilities []string `json:"capabilities"`
	Domains      []string `json:"domains"`
	SHA256       string   `json:"sha256"`
}

type openSetRequirementDocument struct {
	Schema       string                       `json:"schema"`
	Version      int                          `json:"version"`
	Project      openSetRequirementProject    `json:"project"`
	Requirements openSetRequirements          `json:"requirements"`
	Acceptance   openSetRequirementAcceptance `json:"acceptance"`
}

type openSetRequirementProject struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type openSetRequirements struct {
	Domains      []openSetRequirementDomain      `json:"domains"`
	Ports        []openSetRequirementPort        `json:"ports"`
	Participants []openSetRequirementParticipant `json:"participants,omitempty"`
	Objectives   []openSetRequirementObjective   `json:"objectives"`
	Constraints  openSetRequirementBoardLimits   `json:"constraints"`
}

type openSetRequirementDomain struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	MinVoltageV     *float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV float64  `json:"nominal_voltage_v"`
	MaxVoltageV     *float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA     *float64 `json:"max_current_a,omitempty"`
	Source          string   `json:"source"`
}

type openSetRequirementPort struct {
	ID         string                        `json:"id"`
	Kind       string                        `json:"kind"`
	Direction  string                        `json:"direction"`
	Domain     string                        `json:"domain"`
	Electrical *openSetRequirementElectrical `json:"electrical,omitempty"`
	Protocol   *openSetRequirementProtocol   `json:"protocol,omitempty"`
}

type openSetRequirementElectrical struct {
	MinVoltageV          *float64 `json:"min_voltage_v,omitempty"`
	NominalVoltageV      *float64 `json:"nominal_voltage_v,omitempty"`
	MaxVoltageV          *float64 `json:"max_voltage_v,omitempty"`
	MaxCurrentA          *float64 `json:"max_current_a,omitempty"`
	MaxSourceCurrentMA   *float64 `json:"max_source_current_ma,omitempty"`
	InputImpedanceMinOhm *float64 `json:"input_impedance_min_ohm,omitempty"`
	FrequencyMaxHz       *float64 `json:"frequency_max_hz,omitempty"`
	DefaultState         string   `json:"default_state,omitempty"`
}

type openSetRequirementProtocol struct {
	Name           string  `json:"name"`
	Mode           string  `json:"mode"`
	MaxFrequencyHz float64 `json:"max_frequency_hz"`
}

type openSetRequirementParticipant struct {
	ID            string                              `json:"id"`
	Capability    string                              `json:"capability"`
	Domain        string                              `json:"domain"`
	RequiredPorts []openSetRequirementParticipantPort `json:"required_ports"`
	Constraints   []openSetRequirementConstraint      `json:"constraints"`
}

type openSetRequirementParticipantPort struct {
	ID        string                     `json:"id"`
	Kind      string                     `json:"kind"`
	Direction string                     `json:"direction"`
	Protocol  openSetRequirementProtocol `json:"protocol"`
}

type openSetRequirementObjective struct {
	ID          string                         `json:"id"`
	Capability  string                         `json:"capability"`
	Bindings    []openSetRequirementBinding    `json:"bindings"`
	Constraints []openSetRequirementConstraint `json:"constraints"`
}

type openSetRequirementBinding struct {
	Role            string `json:"role"`
	Port            string `json:"port,omitempty"`
	Participant     string `json:"participant,omitempty"`
	ParticipantPort string `json:"participant_port,omitempty"`
}

type openSetRequirementConstraint struct {
	Name             string          `json:"name"`
	Relation         string          `json:"relation"`
	Value            json.RawMessage `json:"value"`
	Unit             string          `json:"unit,omitempty"`
	TolerancePercent *float64        `json:"tolerance_percent,omitempty"`
}

type openSetRequirementBoardLimits struct {
	MaxComponents int     `json:"max_components"`
	MaxWidthMM    float64 `json:"max_width_mm"`
	MaxHeightMM   float64 `json:"max_height_mm"`
}

type openSetRequirementAcceptance struct {
	RequireERC                 bool `json:"require_erc"`
	RequireStrictDRC           bool `json:"require_strict_drc"`
	RequireCompleteRouting     bool `json:"require_complete_routing"`
	RequireConnectivity        bool `json:"require_connectivity"`
	RequireWriterCorrectness   bool `json:"require_writer_correctness"`
	RequireRoundTripZeroDiff   bool `json:"require_round_trip_zero_diff"`
	RequireDeterministicReplay bool `json:"require_deterministic_replay"`
}

func TestOpenSetCompositionCorpusIsFrozenAndBehaviorOnly(t *testing.T) {
	root := openSetCompositionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256Hex(manifestBytes); got != frozenOpenSetCompositionManifestSHA256 {
		t.Fatalf("manifest hash = %s, want %s; corpus membership is frozen by specification", got, frozenOpenSetCompositionManifestSHA256)
	}
	checksumBytes, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	wantChecksum := frozenOpenSetCompositionManifestSHA256 + "  manifest.json\n"
	if string(checksumBytes) != wantChecksum {
		t.Fatalf("manifest.sha256 = %q, want %q", checksumBytes, wantChecksum)
	}

	var manifest openSetCompositionManifest
	strictOpenSetDecode(t, "manifest.json", manifestBytes, &manifest)
	if manifest.Schema != "kicadai.open-set-composition-corpus.v1" || manifest.Version != 1 || manifest.PolicyVersion != "architecture-search-policy-v1" || manifest.FrozenAt != "2026-07-19" {
		t.Fatalf("invalid corpus manifest header: %#v", manifest)
	}
	if len(manifest.Fixtures) != 5 {
		t.Fatalf("fixture count = %d, want 5", len(manifest.Fixtures))
	}

	requiredCapabilities := map[string]bool{
		"threshold_detection":     false,
		"load_switch":             false,
		"voltage_regulation":      false,
		"frequency_filter":        false,
		"logic_level_translation": false,
	}
	seenIDs := map[string]bool{}
	seenFiles := map[string]bool{}
	previousID := ""
	for index, fixture := range manifest.Fixtures {
		if fixture.ID == "" || fixture.File != fixture.ID+".json" || seenIDs[fixture.ID] || seenFiles[fixture.File] || (previousID != "" && fixture.ID <= previousID) {
			t.Fatalf("fixtures[%d] has invalid, duplicate, or unsorted identity: %#v", index, fixture)
		}
		previousID = fixture.ID
		seenIDs[fixture.ID] = true
		seenFiles[fixture.File] = true
		if !slices.IsSorted(fixture.Capabilities) || !slices.IsSorted(fixture.Domains) || len(fixture.Capabilities) == 0 || len(fixture.Domains) == 0 {
			t.Fatalf("fixtures[%d] classifications must be non-empty and sorted: %#v", index, fixture)
		}

		contents, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(contents); got != fixture.SHA256 {
			t.Fatalf("%s hash = %s, want %s", fixture.File, got, fixture.SHA256)
		}
		var document openSetRequirementDocument
		strictOpenSetDecode(t, fixture.File, contents, &document)
		validateFrozenOpenSetRequirement(t, fixture, document)

		var raw any
		if err := json.Unmarshal(contents, &raw); err != nil {
			t.Fatal(err)
		}
		assertNoOpenSetImplementationDetail(t, fixture.File, raw, "")
		for _, capability := range fixture.Capabilities {
			if _, required := requiredCapabilities[capability]; required {
				requiredCapabilities[capability] = true
			}
		}
	}
	for capability, covered := range requiredCapabilities {
		if !covered {
			t.Fatalf("frozen corpus does not cover %s", capability)
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == "manifest.json" {
			continue
		}
		if !seenFiles[entry.Name()] {
			t.Fatalf("unmanifested corpus fixture %s", entry.Name())
		}
	}
}

func validateFrozenOpenSetRequirement(t *testing.T, fixture openSetCompositionManifestFixture, document openSetRequirementDocument) {
	t.Helper()
	if document.Schema != "kicadai.open-set-requirement.v1" || document.Version != 1 || document.Project.Name != fixture.ID || document.Project.Title == "" || document.Project.Description == "" {
		t.Fatalf("%s has invalid document identity: %#v", fixture.File, document.Project)
	}
	if len(document.Requirements.Domains) == 0 || len(document.Requirements.Ports) == 0 || len(document.Requirements.Objectives) == 0 {
		t.Fatalf("%s has incomplete behavioral requirements", fixture.File)
	}
	if document.Requirements.Constraints.MaxComponents <= 0 || document.Requirements.Constraints.MaxWidthMM <= 0 || document.Requirements.Constraints.MaxHeightMM <= 0 {
		t.Fatalf("%s has invalid board limits", fixture.File)
	}
	if acceptance := document.Acceptance; !acceptance.RequireERC || !acceptance.RequireStrictDRC || !acceptance.RequireCompleteRouting || !acceptance.RequireConnectivity || !acceptance.RequireWriterCorrectness || !acceptance.RequireRoundTripZeroDiff || !acceptance.RequireDeterministicReplay {
		t.Fatalf("%s does not require every milestone gate: %#v", fixture.File, acceptance)
	}

	domains := map[string]bool{}
	for _, domain := range document.Requirements.Domains {
		if domain.ID == "" || domain.Kind == "" || domain.Source == "" || domains[domain.ID] {
			t.Fatalf("%s has invalid domain %#v", fixture.File, domain)
		}
		domains[domain.ID] = true
		if domain.MinVoltageV != nil && domain.MaxVoltageV != nil && *domain.MinVoltageV > *domain.MaxVoltageV {
			t.Fatalf("%s domain %s has contradictory voltage range", fixture.File, domain.ID)
		}
	}

	ports := map[string]bool{}
	for _, port := range document.Requirements.Ports {
		if port.ID == "" || port.Kind == "" || port.Direction == "" || !domains[port.Domain] || ports[port.ID] {
			t.Fatalf("%s has invalid external port %#v", fixture.File, port)
		}
		ports[port.ID] = true
	}

	participantPorts := map[string]map[string]bool{}
	for _, participant := range document.Requirements.Participants {
		if participant.ID == "" || participant.Capability == "" || !domains[participant.Domain] || participantPorts[participant.ID] != nil {
			t.Fatalf("%s has invalid participant %#v", fixture.File, participant)
		}
		participantPorts[participant.ID] = map[string]bool{}
		for _, port := range participant.RequiredPorts {
			if port.ID == "" || port.Kind == "" || port.Direction == "" || port.Protocol.Name == "" || port.Protocol.Mode == "" || port.Protocol.MaxFrequencyHz <= 0 || participantPorts[participant.ID][port.ID] {
				t.Fatalf("%s has invalid participant port %#v", fixture.File, port)
			}
			participantPorts[participant.ID][port.ID] = true
		}
		validateOpenSetConstraints(t, fixture.File, participant.Constraints)
	}

	objectiveIDs := map[string]bool{}
	documentCapabilities := map[string]bool{}
	for _, objective := range document.Requirements.Objectives {
		if objective.ID == "" || objective.Capability == "" || objectiveIDs[objective.ID] || len(objective.Bindings) == 0 {
			t.Fatalf("%s has invalid objective %#v", fixture.File, objective)
		}
		objectiveIDs[objective.ID] = true
		documentCapabilities[objective.Capability] = true
		for _, binding := range objective.Bindings {
			external := binding.Port != "" && binding.Participant == "" && binding.ParticipantPort == ""
			participant := binding.Port == "" && binding.Participant != "" && binding.ParticipantPort != ""
			if binding.Role == "" || (!external && !participant) || (external && !ports[binding.Port]) || (participant && !participantPorts[binding.Participant][binding.ParticipantPort]) {
				t.Fatalf("%s has unresolved or ambiguous binding %#v", fixture.File, binding)
			}
		}
		validateOpenSetConstraints(t, fixture.File, objective.Constraints)
	}
	for _, capability := range fixture.Capabilities {
		if !documentCapabilities[capability] {
			t.Fatalf("%s does not contain manifested capability %s", fixture.File, capability)
		}
	}
}

func validateOpenSetConstraints(t *testing.T, file string, constraints []openSetRequirementConstraint) {
	t.Helper()
	allowedRelations := map[string]bool{"equal": true, "maximum": true, "minimum": true, "one_of": true, "range": true, "required": true, "target": true}
	seen := map[string]bool{}
	for _, constraint := range constraints {
		if constraint.Name == "" || !allowedRelations[constraint.Relation] || len(constraint.Value) == 0 || string(constraint.Value) == "null" || seen[constraint.Name] {
			t.Fatalf("%s has invalid or duplicate constraint %#v", file, constraint)
		}
		seen[constraint.Name] = true
		if constraint.TolerancePercent != nil && (*constraint.TolerancePercent < 0 || *constraint.TolerancePercent > 100) {
			t.Fatalf("%s constraint %s has invalid tolerance", file, constraint.Name)
		}
	}
}

func strictOpenSetDecode(t *testing.T, file string, contents []byte, target any) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("strict decode %s: %v", file, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("strict decode %s trailing data: %v", file, err)
	}
}

func assertNoOpenSetImplementationDetail(t *testing.T, file string, value any, path string) {
	t.Helper()
	forbidden := map[string]bool{
		"component": true, "components": true, "component_id": true, "variant_id": true,
		"manufacturer": true, "mpn": true, "reference": true, "symbol": true, "footprint": true,
		"pin": true, "pins": true, "pad": true, "pads": true,
		"fragment": true, "fragments": true, "fragment_id": true, "topology": true, "architecture": true,
		"function": true, "functions": true, "connection": true, "connections": true, "net": true, "nets": true,
		"schematic": true, "pcb": true, "simulation": true,
		"x_mm": true, "y_mm": true, "layer": true, "layers": true,
		"route": true, "routes": true, "track": true, "tracks": true, "via": true, "vias": true,
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if forbidden[strings.ToLower(key)] {
				t.Fatalf("%s contains forbidden implementation detail %s", file, childPath)
			}
			assertNoOpenSetImplementationDetail(t, file, child, childPath)
		}
	case []any:
		for index, child := range typed {
			assertNoOpenSetImplementationDetail(t, file, child, path+"["+strconv.Itoa(index)+"]")
		}
	}
}

func openSetCompositionCorpusRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate open-set composition corpus test source")
	}
	return filepath.Join(filepath.Dir(sourcePath), "testdata", "open_set_composition_corpus")
}
