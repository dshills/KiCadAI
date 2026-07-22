package circuitgraph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

const frozenAdversarialMultiFunctionManifestSHA256 = "3daf2327b26cfebf13b2975f065b0ffac78033d208a86fcd01a830a450d9588d"

type adversarialMultiFunctionManifest struct {
	Schema        string                                    `json:"schema"`
	Version       int                                       `json:"version"`
	FrozenAt      string                                    `json:"frozen_at"`
	PolicyVersion string                                    `json:"policy_version"`
	Fixtures      []adversarialMultiFunctionManifestFixture `json:"fixtures"`
}

type adversarialMultiFunctionManifestFixture struct {
	ID           string   `json:"id"`
	File         string   `json:"file"`
	Capabilities []string `json:"capabilities"`
	Domains      []string `json:"domains"`
	SHA256       string   `json:"sha256"`
}

type adversarialMultiFunctionDocument struct {
	Schema       string                               `json:"schema"`
	Version      int                                  `json:"version"`
	Project      openSetRequirementProject            `json:"project"`
	Requirements adversarialMultiFunctionRequirements `json:"requirements"`
	Acceptance   adversarialMultiFunctionAcceptance   `json:"acceptance"`
}

type adversarialMultiFunctionRequirements struct {
	Domains           []openSetRequirementDomain            `json:"domains"`
	Ports             []openSetRequirementPort              `json:"ports"`
	Signals           []adversarialMultiFunctionSignal      `json:"signals"`
	Participants      []adversarialMultiFunctionParticipant `json:"participants,omitempty"`
	Objectives        []adversarialMultiFunctionObjective   `json:"objectives"`
	SystemConstraints []openSetRequirementConstraint        `json:"system_constraints"`
	Constraints       openSetRequirementBoardLimits         `json:"constraints"`
}

type adversarialMultiFunctionSignal struct {
	ID         string                        `json:"id"`
	Kind       string                        `json:"kind"`
	Domain     string                        `json:"domain"`
	Electrical *openSetRequirementElectrical `json:"electrical,omitempty"`
	Protocol   *openSetRequirementProtocol   `json:"protocol,omitempty"`
}

type adversarialMultiFunctionParticipant struct {
	ID            string                                    `json:"id"`
	Capability    string                                    `json:"capability"`
	Domain        string                                    `json:"domain"`
	RequiredPorts []adversarialMultiFunctionParticipantPort `json:"required_ports"`
	Constraints   []openSetRequirementConstraint            `json:"constraints"`
}

type adversarialMultiFunctionParticipantPort struct {
	ID        string                      `json:"id"`
	Kind      string                      `json:"kind"`
	Direction string                      `json:"direction"`
	Protocol  *openSetRequirementProtocol `json:"protocol,omitempty"`
}

type adversarialMultiFunctionObjective struct {
	ID          string                            `json:"id"`
	Capability  string                            `json:"capability"`
	Bindings    []adversarialMultiFunctionBinding `json:"bindings"`
	Constraints []openSetRequirementConstraint    `json:"constraints"`
}

type adversarialMultiFunctionBinding struct {
	Role            string `json:"role"`
	Port            string `json:"port,omitempty"`
	Signal          string `json:"signal,omitempty"`
	Direction       string `json:"direction,omitempty"`
	Participant     string `json:"participant,omitempty"`
	ParticipantPort string `json:"participant_port,omitempty"`
}

type adversarialMultiFunctionAcceptance struct {
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
}

func TestAdversarialMultiFunctionCompositionCorpusIsFrozenAndBehaviorOnly(t *testing.T) {
	root := adversarialMultiFunctionCorpusRoot(t)
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := sha256Hex(manifestBytes); got != frozenAdversarialMultiFunctionManifestSHA256 {
		t.Fatalf("manifest hash = %s, want %s; corpus membership is frozen by specification", got, frozenAdversarialMultiFunctionManifestSHA256)
	}
	checksumBytes, err := os.ReadFile(filepath.Join(root, "manifest.sha256"))
	if err != nil {
		t.Fatal(err)
	}
	if want := frozenAdversarialMultiFunctionManifestSHA256 + "  manifest.json\n"; string(checksumBytes) != want {
		t.Fatalf("manifest.sha256 = %q, want %q", checksumBytes, want)
	}

	var manifest adversarialMultiFunctionManifest
	strictOpenSetDecode(t, "manifest.json", manifestBytes, &manifest)
	if manifest.Schema != "kicadai.adversarial-multi-function-composition-corpus.v1" || manifest.Version != 1 || manifest.PolicyVersion != "architecture-search-policy-v2" || manifest.FrozenAt != "2026-07-19" {
		t.Fatalf("invalid corpus manifest header: %#v", manifest)
	}
	if len(manifest.Fixtures) != 10 {
		t.Fatalf("fixture count = %d, want 10", len(manifest.Fixtures))
	}

	requiredCapabilities := map[string]bool{
		"class_a_amplification": false, "class_ab_bias_control": false,
		"current_sensing": false, "fault_indication": false,
		"frequency_filter": false, "galvanic_isolation": false,
		"load_switch": false, "logic_level_translation": false,
		"mute_control": false, "output_protection": false,
		"signal_amplification": false, "split_supply_generation": false,
		"threshold_detection": false, "transient_protection": false,
		"voltage_regulation": false,
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
		if len(fixture.Capabilities) < 2 || len(fixture.Capabilities) > 4 || !slices.IsSorted(fixture.Capabilities) || len(fixture.Domains) == 0 || !slices.IsSorted(fixture.Domains) {
			t.Fatalf("fixtures[%d] classifications are incomplete or unsorted: %#v", index, fixture)
		}

		contents, err := os.ReadFile(filepath.Join(root, fixture.File))
		if err != nil {
			t.Fatal(err)
		}
		if got := sha256Hex(contents); got != fixture.SHA256 {
			t.Fatalf("%s hash = %s, want %s", fixture.File, got, fixture.SHA256)
		}
		var document adversarialMultiFunctionDocument
		strictOpenSetDecode(t, fixture.File, contents, &document)
		validateFrozenAdversarialMultiFunctionRequirement(t, fixture, document)

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
			t.Fatalf("frozen corpus does not cover representative capability %s", capability)
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

func validateFrozenAdversarialMultiFunctionRequirement(t *testing.T, fixture adversarialMultiFunctionManifestFixture, document adversarialMultiFunctionDocument) {
	t.Helper()
	if document.Schema != "kicadai.open-set-requirement.v2" || document.Version != 2 || document.Project.Name != fixture.ID || document.Project.Title == "" || document.Project.Description == "" {
		t.Fatalf("%s has invalid document identity", fixture.File)
	}
	if len(document.Requirements.Domains) == 0 || len(document.Requirements.Ports) == 0 || len(document.Requirements.Signals) == 0 || len(document.Requirements.Objectives) < 2 || len(document.Requirements.Objectives) > 4 || len(document.Requirements.SystemConstraints) == 0 {
		t.Fatalf("%s has incomplete multi-function requirements", fixture.File)
	}
	if limits := document.Requirements.Constraints; limits.MaxComponents <= 0 || limits.MaxWidthMM <= 0 || limits.MaxHeightMM <= 0 {
		t.Fatalf("%s has invalid board limits", fixture.File)
	}
	if acceptance := document.Acceptance; !acceptance.RequireERC || !acceptance.RequireStrictDRC || !acceptance.RequireCompleteRouting || !acceptance.RequireConnectivity || !acceptance.RequireWriterCorrectness || !acceptance.RequireRoundTripZeroDiff || !acceptance.RequireDeterministicReplay || !acceptance.RequireContractComposition || !acceptance.RequireGlobalReasoning || !acceptance.RequireCoverageAccounting || !acceptance.RequireAlternatives || !acceptance.RequireFailClosed {
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
		if port.ID == "" || port.Kind == "" || !validFrozenDirection(port.Direction) || !domains[port.Domain] || ports[port.ID] {
			t.Fatalf("%s has invalid external port %#v", fixture.File, port)
		}
		ports[port.ID] = true
	}
	signals := map[string]adversarialMultiFunctionSignal{}
	for _, signal := range document.Requirements.Signals {
		if signal.ID == "" || signal.Kind == "" || !domains[signal.Domain] {
			t.Fatalf("%s has invalid signal %#v", fixture.File, signal)
		}
		if _, duplicate := signals[signal.ID]; duplicate {
			t.Fatalf("%s has duplicate signal %s", fixture.File, signal.ID)
		}
		signals[signal.ID] = signal
	}
	for _, domain := range document.Requirements.Domains {
		if domain.Source == "external" {
			continue
		}
		signal, ok := signals[domain.Source]
		if !ok || signal.Kind != "power" || signal.Domain != domain.ID {
			t.Fatalf("%s domain %s has unresolved or incompatible derived source %s", fixture.File, domain.ID, domain.Source)
		}
	}

	participantPorts := map[string]map[string]bool{}
	for _, participant := range document.Requirements.Participants {
		if participant.ID == "" || participant.Capability == "" || !domains[participant.Domain] || participantPorts[participant.ID] != nil {
			t.Fatalf("%s has invalid participant %#v", fixture.File, participant)
		}
		participantPorts[participant.ID] = map[string]bool{}
		for _, port := range participant.RequiredPorts {
			if port.ID == "" || port.Kind == "" || !validFrozenDirection(port.Direction) || participantPorts[participant.ID][port.ID] {
				t.Fatalf("%s has invalid participant port %#v", fixture.File, port)
			}
			participantPorts[participant.ID][port.ID] = true
		}
		validateOpenSetConstraints(t, fixture.File, participant.Constraints)
	}

	type signalUse struct{ sources, sinks, bidirectional int }
	uses := map[string]signalUse{}
	documentCapabilities := map[string]bool{}
	objectiveIDs := map[string]bool{}
	for _, objective := range document.Requirements.Objectives {
		if objective.ID == "" || objective.Capability == "" || objectiveIDs[objective.ID] || len(objective.Bindings) == 0 {
			t.Fatalf("%s has invalid objective %#v", fixture.File, objective)
		}
		objectiveIDs[objective.ID] = true
		documentCapabilities[objective.Capability] = true
		for _, binding := range objective.Bindings {
			external := binding.Port != "" && binding.Signal == "" && binding.Direction == "" && binding.Participant == "" && binding.ParticipantPort == ""
			participant := binding.Port == "" && binding.Signal == "" && binding.Direction == "" && binding.Participant != "" && binding.ParticipantPort != ""
			signal := binding.Port == "" && binding.Signal != "" && validFrozenDirection(binding.Direction) && binding.Participant == "" && binding.ParticipantPort == ""
			if binding.Role == "" || (!external && !participant && !signal) || (external && !ports[binding.Port]) || (participant && !participantPorts[binding.Participant][binding.ParticipantPort]) {
				t.Fatalf("%s has unresolved or ambiguous binding %#v", fixture.File, binding)
			}
			if signal {
				if _, ok := signals[binding.Signal]; !ok {
					t.Fatalf("%s binding references unknown signal %s", fixture.File, binding.Signal)
				}
				use := uses[binding.Signal]
				switch binding.Direction {
				case "source":
					use.sources++
				case "sink":
					use.sinks++
				case "bidirectional":
					use.bidirectional++
				}
				uses[binding.Signal] = use
			}
		}
		validateOpenSetConstraints(t, fixture.File, objective.Constraints)
	}
	validateOpenSetConstraints(t, fixture.File, document.Requirements.SystemConstraints)
	for signalID := range signals {
		use := uses[signalID]
		unidirectional := use.sources == 1 && use.sinks >= 1 && use.bidirectional == 0
		bidirectional := use.sources == 0 && use.sinks == 0 && use.bidirectional >= 2
		if !unidirectional && !bidirectional {
			t.Fatalf("%s signal %s has invalid endpoint cardinality: %#v", fixture.File, signalID, use)
		}
	}
	if len(documentCapabilities) != len(fixture.Capabilities) {
		t.Fatalf("%s manifested capabilities do not exactly match objectives", fixture.File)
	}
	for _, capability := range fixture.Capabilities {
		if !documentCapabilities[capability] {
			t.Fatalf("%s does not contain manifested capability %s", fixture.File, capability)
		}
	}
}

func validFrozenDirection(direction string) bool {
	return direction == "source" || direction == "sink" || direction == "bidirectional"
}

func adversarialMultiFunctionCorpusRoot(t *testing.T) string {
	t.Helper()
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate adversarial multi-function corpus test source")
	}
	return filepath.Join(filepath.Dir(sourcePath), "testdata", "adversarial_multi_function_composition_corpus")
}
