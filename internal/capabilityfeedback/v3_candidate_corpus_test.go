package capabilityfeedback

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ots "kicadai/internal/opentopologysynthesis"
)

const (
	closedLoopV3CandidateRoot         = "testdata/closed_loop_open_set_v3_candidate"
	closedLoopV3AuthorManifestSchema  = "kicadai.closed-loop-open-set-author-manifest.v3"
	closedLoopV3ValidationEnvironment = "VALIDATE_CLOSED_LOOP_V3_CANDIDATE"
)

// TestClosedLoopV3CandidateQuarantine is intentionally content-blind with
// respect to synthesis and outcomes. It validates only the frozen authoring
// contract, authorship isolation, behavior-schema correctness, diversity, and
// semantic non-overlap with V1/V2.
func TestClosedLoopV3CandidateQuarantine(t *testing.T) {
	if _, err := os.Stat(closedLoopV3CandidateRoot); err != nil {
		if os.IsNotExist(err) && os.Getenv(closedLoopV3ValidationEnvironment) == "" {
			t.Skip("V3 candidate quarantine is absent")
		}
		t.Fatalf("V3 candidate quarantine is unavailable: %v", err)
	}

	packetRoot := filepath.Join("..", "..", "specs", "closed-loop-open-set-capability-expansion", "v3-authoring-packet")
	packetManifest := mustCorpusRead(t, filepath.Join(packetRoot, "AUTHOR_MANIFEST.json"))
	candidateManifest := mustCorpusRead(t, filepath.Join(closedLoopV3CandidateRoot, "AUTHOR_MANIFEST.json"))
	if !bytes.Equal(candidateManifest, packetManifest) {
		t.Fatal("V3 candidate author manifest differs from the frozen packet")
	}
	var manifest closedLoopV2AuthorManifest
	decodeCorpusStrict(t, candidateManifest, &manifest)
	if manifest.Schema != closedLoopV3AuthorManifestSchema || manifest.Version != 3 || len(manifest.Entries) != closedLoopCorpusSize {
		t.Fatalf("V3 candidate manifest header/size is invalid")
	}
	authorship := string(mustCorpusRead(t, filepath.Join(closedLoopV3CandidateRoot, "AUTHORSHIP.md")))
	if strings.Contains(authorship, "[") || strings.Contains(authorship, "]") ||
		!strings.Contains(authorship, "this packet was my only input") {
		t.Fatal("V3 candidate authorship record is incomplete or has unresolved uncertainty")
	}

	seenRaw := map[string]string{}
	seenSemantic := closedLoopV1NeutralSemanticHashes(t)
	for hash, id := range closedLoopV2NeutralSemanticHashes(t) {
		seenSemantic[hash] = id
	}
	diversity := map[CorpusRole]*closedLoopV3Diversity{
		RoleDiscovery: newClosedLoopV3Diversity(),
		RoleHeldOut:   newClosedLoopV3Diversity(),
	}
	counts := map[CorpusRole]map[string]int{RoleDiscovery: {}, RoleHeldOut: {}}
	for index, entry := range manifest.Entries {
		wantRole, wantDirectory := RoleDiscovery, "discovery"
		if index >= closedLoopCorpusSize/2 {
			wantRole, wantDirectory = RoleHeldOut, "held_out"
		}
		wantID := fmt.Sprintf("v3_case_%03d", index+1)
		wantFile := fmt.Sprintf("%s/request_%03d.json", wantDirectory, index+1)
		if entry.ID != wantID || entry.Role != wantRole || entry.RequirementFile != wantFile {
			t.Fatalf("V3 candidate entry %d has invalid identity, role, or path", index+1)
		}
		counts[entry.Role][entry.Domain]++

		data := mustCorpusRead(t, filepath.Join(closedLoopV3CandidateRoot, filepath.FromSlash(entry.RequirementFile)))
		rawHash := corpusHash(data)
		if prior, duplicate := seenRaw[rawHash]; duplicate {
			t.Fatalf("%s duplicates raw requirement bytes from %s", entry.ID, prior)
		}
		seenRaw[rawHash] = entry.ID
		if bytes.Contains(data, []byte(entry.ID)) || bytes.Contains(data, []byte(entry.SourceID)) {
			t.Fatalf("%s leaks manifest identity into requirement bytes", entry.ID)
		}
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("%s violates the public requirement contract (%d issues)", entry.ID, len(issues))
		}
		if closedLoopV3ContainsImplementationLanguage(t, data) {
			t.Fatalf("%s contains prohibited implementation language", entry.ID)
		}
		if len(requirement.Requirements.OperatingCases) < 2 || len(requirement.Requirements.BehavioralRequirements) < 4 {
			t.Fatalf("%s does not meet minimum operating-case/assertion counts", entry.ID)
		}
		analyses := map[string]bool{}
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			analyses[assertion.Analysis] = true
		}
		if len(analyses) < 2 {
			t.Fatalf("%s does not meet the minimum analysis-kind count", entry.ID)
		}
		if !closedLoopV2AllAcceptanceGates(requirement.Acceptance) {
			t.Fatalf("%s does not require all acceptance gates", entry.ID)
		}
		semantic := closedLoopNeutralRequirementHash(t, requirement)
		if prior, duplicate := seenSemantic[semantic]; duplicate {
			t.Fatalf("%s duplicates a normalized requirement from %s", entry.ID, prior)
		}
		seenSemantic[semantic] = entry.ID
		diversity[entry.Role].observe(entry.Domain, requirement)
	}
	for _, role := range []CorpusRole{RoleDiscovery, RoleHeldOut} {
		for _, domain := range []string{"analog", "power", "digital", "mcu", "sensor", "mixed_signal"} {
			if counts[role][domain] != 2 {
				t.Fatalf("V3 %s/%s count = %d, want 2", role, domain, counts[role][domain])
			}
		}
		diversity[role].validate(t, role)
	}
}

type closedLoopV3Diversity struct {
	supplyConfigurations  map[string]bool
	observations          map[string]bool
	analyses              map[string]bool
	variations            map[string]bool
	events                map[string]bool
	criticalDomains       map[string]bool
	multiOutput           int
	convergingExcitations int
}

func newClosedLoopV3Diversity() *closedLoopV3Diversity {
	return &closedLoopV3Diversity{
		supplyConfigurations: map[string]bool{}, observations: map[string]bool{},
		analyses: map[string]bool{}, variations: map[string]bool{}, events: map[string]bool{},
		criticalDomains: map[string]bool{},
	}
}

func (diversity *closedLoopV3Diversity) observe(reportingDomain string, requirement ots.Requirement) {
	supplyCount, positive, negative := 0, false, false
	ports := map[string]ots.Port{}
	for _, domain := range requirement.Requirements.Domains {
		if domain.Kind != "supply" {
			continue
		}
		supplyCount++
		if domain.NominalVoltageV != nil && *domain.NominalVoltageV > 0 {
			positive = true
		}
		if domain.NominalVoltageV != nil && *domain.NominalVoltageV < 0 {
			negative = true
		}
	}
	if supplyCount == 1 && positive {
		diversity.supplyConfigurations["single_positive"] = true
	}
	if positive && negative {
		diversity.supplyConfigurations["bipolar"] = true
	}
	if supplyCount >= 2 {
		diversity.supplyConfigurations["multiple"] = true
	}
	for _, port := range requirement.Requirements.Ports {
		ports[port.ID] = port
	}
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		for _, condition := range operatingCase.Conditions {
			switch condition.Axis {
			case "load_capacitance", "load_current", "load_inductance", "load_resistance":
				diversity.variations["load"] = true
			case "model_corner", "tolerance_corner":
				diversity.variations["tolerance_model"] = true
			case "ambient_temperature":
				diversity.variations["temperature"] = true
			case "supply_voltage":
				diversity.variations["supply"] = true
			}
		}
		for _, event := range operatingCase.Events {
			diversity.events[event.Kind] = true
		}
	}
	observedOutputs := map[string]bool{}
	excitationsByObservation := map[string]map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Critical {
			diversity.criticalDomains[reportingDomain] = true
		}
		switch assertion.Analysis {
		case "dc_operating_point", "dc_sweep":
			diversity.analyses["dc"] = true
		case "ac_sweep", "noise", "stability", "distortion":
			diversity.analyses["ac_noise_stability"] = true
		case "transient", "startup":
			diversity.analyses["transient_startup"] = true
		case "thermal", "electrothermal":
			diversity.analyses["thermal"] = true
		}
		switch assertion.Metric {
		case "dc_voltage", "output_voltage", "output_high_voltage", "output_low_voltage", "output_swing", "peak_voltage", "startup_output_voltage":
			diversity.observations["voltage"] = true
		case "dc_current", "output_current", "peak_current", "startup_current", "off_state_current", "quiescent_current":
			diversity.observations["current"] = true
		case "output_power":
			diversity.observations["power"] = true
		}
		if port, ok := ports[assertion.Observation.ID]; ok && port.Direction == "source" {
			observedOutputs[port.ID] = true
			if assertion.Excitation != nil {
				if excitationsByObservation[port.ID] == nil {
					excitationsByObservation[port.ID] = map[string]bool{}
				}
				excitationsByObservation[port.ID][assertion.Excitation.Kind+":"+assertion.Excitation.ID] = true
			}
		}
	}
	if len(observedOutputs) >= 2 {
		diversity.multiOutput++
	}
	for _, excitations := range excitationsByObservation {
		if len(excitations) >= 2 {
			diversity.convergingExcitations++
			break
		}
	}
}

func closedLoopV3ContainsImplementationLanguage(t *testing.T, data []byte) bool {
	t.Helper()
	var value any
	decodeCorpusStrict(t, data, &value)
	var contains func(any) bool
	contains = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if key != "schema" && contains(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if contains(child) {
					return true
				}
			}
		case string:
			return implementationLanguage.MatchString(typed)
		}
		return false
	}
	return contains(value)
}

func (diversity *closedLoopV3Diversity) validate(t *testing.T, role CorpusRole) {
	t.Helper()
	for name, values := range map[string]map[string]bool{
		"supply configuration": diversity.supplyConfigurations,
		"observation kind":     diversity.observations,
		"analysis category":    diversity.analyses,
		"variation category":   diversity.variations,
	} {
		if len(values) < 3 && name != "variation category" || name == "variation category" && len(values) < 4 {
			t.Fatalf("V3 %s %s diversity = %d", role, name, len(values))
		}
	}
	for _, event := range []string{"input_step", "load_step", "power_step", "startup", "rail_loss", "short_circuit"} {
		if !diversity.events[event] {
			t.Fatalf("V3 %s omits required event category %s", role, event)
		}
	}
	if diversity.multiOutput < 3 || diversity.convergingExcitations < 3 || len(diversity.criticalDomains) < 3 {
		t.Fatalf("V3 %s structural diversity is below the frozen minima", role)
	}
}

func closedLoopV2NeutralSemanticHashes(t *testing.T) map[string]string {
	t.Helper()
	data := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, "author_manifest.json"))
	var manifest closedLoopV2AuthorManifest
	decodeCorpusStrict(t, data, &manifest)
	result := make(map[string]string, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		data := mustCorpusRead(t, filepath.Join(closedLoopV2CorpusRoot, filepath.FromSlash(entry.RequirementFile)))
		requirement, issues := ots.DecodeStrict(bytes.NewReader(data))
		if len(issues) != 0 {
			t.Fatalf("frozen V2 %s violates its requirement contract", entry.ID)
		}
		result[closedLoopNeutralRequirementHash(t, requirement)] = entry.ID
	}
	return result
}
