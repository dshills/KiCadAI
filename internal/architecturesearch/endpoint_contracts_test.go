package architecturesearch

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

func TestParticipantPortObservationValidatesExactScalarEndpoint(t *testing.T) {
	participant := Participant{ID: "controller", Domain: "logic", RequiredPorts: []ParticipantPort{
		{ID: "adc", Kind: "analog_voltage", Direction: "sink"},
		{ID: "adc_sibling", Kind: "analog_voltage", Direction: "sink"},
		{ID: "bus", Kind: "digital_bus", Direction: "bidirectional"},
	}}
	for _, tc := range []struct {
		id    string
		valid bool
	}{
		{"controller.adc", true}, {"controller.adc_sibling", true},
		{"controller.unknown", false}, {"missing.adc", false}, {"adc", false},
		{"controller.adc.extra", false}, {"controller.bus", false},
	} {
		t.Run(tc.id, func(t *testing.T) {
			validator := requirementValidator{requirement: Requirement{Version: VersionV3, Requirements: Requirements{Domains: []Domain{{ID: "ground", Kind: "reference"}}, Participants: []Participant{participant}}}, participantsByID: map[string]Participant{"controller": participant}}
			validator.behaviorObservation("observation", Observation{Kind: "participant_port", ID: tc.id})
			if got := len(validator.issues) == 0; got != tc.valid {
				t.Fatalf("valid=%v want=%v: %#v", got, tc.valid, validator.issues)
			}
		})
	}
}

func TestParticipantPortObservationScopesProducerWithoutSiblingLeakage(t *testing.T) {
	r := Requirement{Version: VersionV3, Requirements: Requirements{
		Participants: []Participant{{ID: "controller", Domain: "logic", RequiredPorts: []ParticipantPort{{ID: "adc", Kind: "analog_voltage", Direction: "sink"}, {ID: "other", Kind: "analog_voltage", Direction: "sink"}}}},
		Ports:        []Port{{ID: "sensor", Kind: "analog_voltage", Direction: "sink", Domain: "logic"}},
		Objectives: []Objective{
			{ID: "condition_sensor", Capability: "frequency_filter", Bindings: []Binding{{Role: "input", Port: "sensor"}, {Role: "output", Participant: "controller", ParticipantPort: "adc"}}},
			{ID: "unrelated", Capability: "frequency_filter", Bindings: []Binding{{Role: "output", Participant: "controller", ParticipantPort: "other"}}},
		},
	}}
	observation := Observation{Kind: "participant_port", ID: "controller.adc"}
	for _, reorder := range []bool{false, true} {
		if reorder {
			slices.Reverse(r.Requirements.Objectives)
		}
		for _, cone := range []map[string]bool{upstreamObjectiveCone(r, observation), upstreamBehavioralObjectiveCone(r, observation), hierarchicalObjectiveCone(r, observation)} {
			if !cone["condition_sensor"] || cone["unrelated"] {
				t.Fatalf("incorrect observed cone: %#v", cone)
			}
		}
		for _, objective := range r.Requirements.Objectives {
			roles := objectiveRolesForObservation(r, objective, observation)
			if (len(roles) == 1 && roles[0] == "output") != (objective.ID == "condition_sensor") {
				t.Fatalf("wrong projected roles: %s %#v", objective.ID, roles)
			}
		}
	}
}

// This transforms an existing fixture only in memory. The frozen source file
// and its historical result remain unchanged; this is an offline regression.
func standaloneOutputRequirement(t *testing.T) Requirement {
	t.Helper()
	b, err := os.ReadFile("testdata/simulation_grounded_closed_loop_corpus/regulated_sensor_interface.json")
	if err != nil {
		t.Fatal(err)
	}
	var r Requirement
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	r.Project.Name = "standalone_regulator_contract"
	r.Requirements.Signals = nil
	r.Requirements.Objectives = r.Requirements.Objectives[:1]
	r.Requirements.Objectives[0].Bindings[1] = Binding{Role: "output", Port: "regulated_output"}
	r.Requirements.Ports = slices.DeleteFunc(r.Requirements.Ports, func(p Port) bool { return p.ID != "power" && p.ID != "ground" })
	r.Requirements.Ports = append(r.Requirements.Ports, Port{ID: "regulated_output", Kind: "power", Direction: "source", Domain: "sensor_3v3"})
	r.Requirements.Domains[1].Source = "port:regulated_output"
	r.Requirements.BehavioralRequirements = slices.DeleteFunc(r.Requirements.BehavioralRequirements, func(b BehavioralRequirement) bool { return b.ID != "rail" && b.ID != "thermal" })
	return r
}

func TestStandaloneRegulatorOutputUsesRealPortWithoutInventedConsumer(t *testing.T) {
	r := standaloneOutputRequirement(t)
	if issues := Validate(Normalize(r)); reports.HasBlockingIssue(issues) {
		t.Fatalf("standalone output rejected: %#v", issues)
	}
	if len(r.Requirements.Signals) != 0 || len(r.Requirements.Objectives) != 1 || len(r.Requirements.Participants) != 0 {
		t.Fatal("invented consumer or function")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Requirement)
	}{
		{"external substitution", func(r *Requirement) { r.Requirements.Domains[1].Source = "external" }},
		{"unknown output", func(r *Requirement) { r.Requirements.Domains[1].Source = "port:absent" }},
		{"input masquerades as producer", func(r *Requirement) { r.Requirements.Domains[1].Source = "port:power" }},
		{"wrong domain", func(r *Requirement) { r.Requirements.Ports[2].Domain = "input_5v" }},
		{"wrong kind", func(r *Requirement) { r.Requirements.Ports[2].Kind = "analog_voltage" }},
		{"ambiguous reference routing", func(r *Requirement) {
			r.Requirements.Domains = append(r.Requirements.Domains, Domain{ID: "other_ground", Kind: "reference", Source: "external"})
		}},
		{"missing producer", func(r *Requirement) {
			r.Requirements.Objectives[0].Bindings = r.Requirements.Objectives[0].Bindings[:1]
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := standaloneOutputRequirement(t)
			tc.mutate(&r)
			issues := Validate(Normalize(r))
			if !reports.HasBlockingIssue(issues) || !slices.ContainsFunc(issues, func(i reports.Issue) bool {
				return strings.Contains(i.Path, "source") || strings.Contains(i.Message, "derived supply")
			}) {
				t.Fatalf("invalid provenance accepted or not diagnosed: %#v", issues)
			}
		})
	}
}

func TestGeneratedOutputConsumersAreNotProducers(t *testing.T) {
	r := standaloneOutputRequirement(t)
	for _, role := range []string{"input", "sense", "protected", "power", "positive_power", "negative_power", "power_a", "power_b", "rail_a", "rail_b"} {
		binding := Binding{Role: role, Port: "regulated_output"}
		consumer := Objective{ID: "consumer_" + role, Capability: "frequency_filter", Bindings: []Binding{binding}}
		r.Requirements.Objectives = append(r.Requirements.Objectives, consumer)
		if direction, ok := objectivePortDirection(r, binding); !ok || direction != "sink" {
			t.Fatalf("%s is not a consumer: %s", role, direction)
		}
		contract, issues := contractFromBinding(r, binding, EvidenceRuleInferred)
		if len(issues) != 0 || contract.Direction != "sink" || contract.RequiredCurrentCapacityA != nil || contract.MaximumCurrentDemandA == nil || *contract.MaximumCurrentDemandA != 0.15 {
			t.Fatalf("%s failed to preserve the consumer current budget: %#v %#v", role, contract, issues)
		}
		if objectiveProducesEndpoint(r, consumer, "port:regulated_output") || !slices.Contains(objectiveInputEndpoints(r, consumer), "port:regulated_output") {
			t.Fatalf("%s incorrectly attributed as a producer", role)
		}
	}
	if producer, unique := generatedSupplyProducer(r, Observation{Kind: "port", ID: "regulated_output"}); !unique || producer.ID != r.Requirements.Objectives[0].ID {
		t.Fatalf("real consumers obscure unique producer: %#v", producer)
	}
	if _, ok := objectivePortDirection(r, Binding{Role: "invented", Port: "regulated_output"}); ok {
		t.Fatal("unknown generated output role accepted")
	}
}

func TestParticipantObservationRejectsUnroutableMultipleReferences(t *testing.T) {
	r := Requirement{Version: VersionV3, Requirements: Requirements{
		Domains:      []Domain{{ID: "host_ground", Kind: "reference"}, {ID: "remote_ground", Kind: "reference"}},
		Participants: []Participant{{ID: "controller", Domain: "remote_logic", RequiredPorts: []ParticipantPort{{ID: "adc", Kind: "analog_voltage", Direction: "sink"}}}},
	}}
	validator := requirementValidator{requirement: r}
	validator.behaviorObservation("observation", Observation{Kind: "participant_port", ID: "controller.adc"})
	if !reports.HasBlockingIssue(validator.issues) {
		t.Fatal("multiple references accepted without explicit participant routing")
	}
}

func TestStandaloneRegulatorPowerTreeRetainsProducerAndCycleGates(t *testing.T) {
	r := standaloneOutputRequirement(t)
	selection := FragmentSelection{Ports: []RoleContract{{Role: "output", Anchor: "external:regulated_output", Contract: PortContract{Kind: "power", Direction: "source", Domain: "sensor_3v3"}}}}
	if _, failure := validatePowerTreeTopology(r, []FragmentSelection{selection}); failure != nil {
		t.Fatal(failure)
	}
	if _, failure := validatePowerTreeTopology(r, nil); failure == nil || failure.Code != CodePowerRailSourceMissing {
		t.Fatal("missing selected producer accepted")
	}
	if _, failure := validatePowerTreeTopology(r, []FragmentSelection{selection, selection}); failure == nil || failure.Code != CodePowerRailSourceAmbiguous {
		t.Fatal("duplicate selected producers accepted")
	}
	for i := range r.Requirements.Ports {
		if r.Requirements.Ports[i].ID == "power" {
			r.Requirements.Ports[i].Domain = "sensor_3v3"
		}
	}
	if _, failure := validatePowerTreeTopology(r, []FragmentSelection{selection}); failure == nil || failure.Code != CodePowerRailCycle {
		t.Fatalf("self-powered regulator cycle accepted: %#v", failure)
	}
}

func TestStandaloneRegulatorRejectsDuplicateDeclaredProducerAndLegacySyntax(t *testing.T) {
	r := standaloneOutputRequirement(t)
	second := r.Requirements.Objectives[0]
	second.ID = "other_regulator"
	r.Requirements.Objectives = append(r.Requirements.Objectives, second)
	if !reports.HasBlockingIssue(Validate(Normalize(r))) {
		t.Fatal("ambiguous declared producer accepted")
	}
	r = standaloneOutputRequirement(t)
	r.Version = VersionV2
	r.Schema = SchemaIDV2
	if !reports.HasBlockingIssue(Validate(Normalize(r))) {
		t.Fatal("new source syntax silently enabled in legacy v2")
	}
}

func TestGeneratedOutputRejectsCrossRailCycle(t *testing.T) {
	r := standaloneOutputRequirement(t)
	for i := range r.Requirements.Domains {
		if r.Requirements.Domains[i].ID == "input_5v" {
			r.Requirements.Domains[i].Source = "port:upstream_output"
		}
	}
	r.Requirements.Ports = append(r.Requirements.Ports, Port{ID: "upstream_output", Kind: "power", Direction: "source", Domain: "input_5v"})
	r.Requirements.Objectives[0].Bindings[0] = Binding{Role: "input", Port: "upstream_output"}
	r.Requirements.Objectives = append(r.Requirements.Objectives, Objective{ID: "upstream_regulate", Capability: "voltage_regulation", Bindings: []Binding{{Role: "input", Port: "regulated_output"}, {Role: "output", Port: "upstream_output"}}})
	selections := []FragmentSelection{{Ports: []RoleContract{
		{Role: "output", Anchor: "external:regulated_output", Contract: PortContract{Kind: "power", Direction: "source", Domain: "sensor_3v3"}},
		{Role: "output", Anchor: "external:upstream_output", Contract: PortContract{Kind: "power", Direction: "source", Domain: "input_5v"}},
	}}}
	if _, failure := validatePowerTreeTopology(r, selections); failure == nil || failure.Code != CodePowerRailCycle {
		t.Fatalf("cross-rail cycle accepted: %#v", failure)
	}
}
