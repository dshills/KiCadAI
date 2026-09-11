package closedloopsynthesis

import (
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

func TestParticipantPinAssertionUsesExactNodeAndLocalReference(t *testing.T) {
	r := architecturesearch.Requirement{Requirements: architecturesearch.Requirements{
		Domains:                []architecturesearch.Domain{{ID: "remote_logic", Kind: "supply"}, {ID: "remote_ground", Kind: "reference"}},
		Participants:           []architecturesearch.Participant{{ID: "controller", Domain: "remote_logic", RequiredPorts: []architecturesearch.ParticipantPort{{ID: "adc", Kind: "analog_voltage", Direction: "sink"}, {ID: "sibling", Kind: "analog_voltage", Direction: "sink"}}}},
		BehavioralRequirements: []architecturesearch.BehavioralRequirement{{ID: "voltage", Metric: "dc_voltage", Observation: architecturesearch.Observation{Kind: "participant_port", ID: "controller.adc"}}},
	}}
	bindings := []SemanticBinding{{Kind: "participant_port", ID: "controller.adc", Target: "ADC_NODE"}, {Kind: "participant_port", ID: "controller.sibling", Target: "OTHER_NODE"}, {Kind: "domain", ID: "remote_ground", Target: "REMOTE_RETURN"}, {Kind: "domain", ID: "host_ground", Target: "HOST_RETURN"}}
	index, diagnostics := validateSemanticBindings(bindings)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	node, exists := observationTarget(r.Requirements.BehavioralRequirements[0].Observation, index)
	if !exists || node != "ADC_NODE" {
		t.Fatalf("wrong pin target: %s", node)
	}
	binding, diagnostic := resolvedAssertionBinding(PlannedAssertion{RequirementID: "voltage", Metric: "dc_voltage", Target: node}, "", nil, nil, simmodel.Plan{Nodes: []string{"ADC_NODE", "OTHER_NODE", "REMOTE_RETURN", "HOST_RETURN"}}, r, bindings)
	if diagnostic != nil {
		t.Fatal(diagnostic.Message)
	}
	if len(binding.Prototypes) != 1 || binding.Prototypes[0].Node != "ADC_NODE" || binding.Prototypes[0].ReferenceNode != "REMOTE_RETURN" {
		t.Fatalf("wrong physical measurement: %#v", binding)
	}
	delete(index, "participant_port\x00controller.adc")
	if _, exists := observationTarget(r.Requirements.BehavioralRequirements[0].Observation, index); exists {
		t.Fatal("missing pin substituted with sibling")
	}
	if _, diagnostic := resolvedAssertionBinding(PlannedAssertion{RequirementID: "voltage", Metric: "dc_voltage", Target: node}, "", nil, nil, simmodel.Plan{Nodes: []string{"ADC_NODE"}}, r, bindings[:2]); diagnostic == nil {
		t.Fatal("missing local reference accepted")
	}
	r.Requirements.Domains = append(r.Requirements.Domains, architecturesearch.Domain{ID: "host_ground", Kind: "reference"})
	if _, diagnostic := resolvedAssertionBinding(PlannedAssertion{RequirementID: "voltage", Metric: "dc_voltage", Target: node}, "", nil, nil, simmodel.Plan{Nodes: []string{"ADC_NODE", "OTHER_NODE", "REMOTE_RETURN", "HOST_RETURN"}}, r, bindings); diagnostic == nil {
		t.Fatal("ambiguous participant reference routing accepted")
	}
	r.Requirements.Domains[0].ReferenceDomain = "host_ground"
	binding, diagnostic = resolvedAssertionBinding(PlannedAssertion{RequirementID: "voltage", Metric: "dc_voltage", Target: node}, "", nil, nil, simmodel.Plan{Nodes: []string{"ADC_NODE", "OTHER_NODE", "REMOTE_RETURN", "HOST_RETURN"}}, r, bindings)
	if diagnostic != nil || len(binding.Prototypes) != 1 || binding.Prototypes[0].ReferenceNode != "HOST_RETURN" {
		t.Fatalf("explicit return did not override misleading remote ground name: %#v %v", binding, diagnostic)
	}
	r.Requirements.Domains[0].ReferenceDomain = "missing"
	if _, diagnostic := resolvedAssertionBinding(PlannedAssertion{RequirementID: "voltage", Metric: "dc_voltage", Target: node}, "", nil, nil, simmodel.Plan{Nodes: []string{"ADC_NODE", "OTHER_NODE", "REMOTE_RETURN", "HOST_RETURN"}}, r, bindings); diagnostic == nil {
		t.Fatal("invalid explicit reference fell back")
	}
}
