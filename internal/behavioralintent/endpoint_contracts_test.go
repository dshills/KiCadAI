package behavioralintent

import (
	"reflect"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
)

func TestProviderSchemaExposesQualifiedPinAndGeneratedOutputForms(t *testing.T) {
	_, p := syntheticFilterProposal()
	for _, tc := range []struct {
		kind, id string
		valid    bool
	}{
		{"participant_port", "controller.adc", true}, {"participant_port", "adc", false},
		{"participant_port", "controller.adc.extra", false}, {"circuit", "controller.adc", false},
	} {
		p.Requirement.Requirements.BehavioralRequirements[0].Observation = architecturesearch.Observation{Kind: tc.kind, ID: tc.id}
		if providerSchemaAccepts(t, ProposalSchema(), providerWireValue(reflect.ValueOf(p))) != tc.valid {
			t.Fatalf("wrong schema result for %s/%s", tc.kind, tc.id)
		}
	}
	_, p = syntheticFilterProposal()
	for _, tc := range []struct {
		source string
		valid  bool
	}{{"port:regulated_output", true}, {"port:", false}, {"port:bad.id", false}, {"external", true}, {"generated_rail", true}} {
		p.Requirement.Requirements.Domains[1].Source = tc.source
		if providerSchemaAccepts(t, ProposalSchema(), providerWireValue(reflect.ValueOf(p))) != tc.valid {
			t.Fatalf("wrong schema result for source %s", tc.source)
		}
	}
	for _, reference := range []string{"", "local_return", "bad.id"} {
		p.Requirement.Requirements.Domains[1].ReferenceDomain = reference
		if got := providerSchemaAccepts(t, ProposalSchema(), providerWireValue(reflect.ValueOf(p))); got != (reference != "bad.id") {
			t.Fatalf("reference schema %q accepted=%v", reference, got)
		}
	}
}

func TestCompilerRetainsQualifiedParticipantObservationWithoutPublicSubstitute(t *testing.T) {
	_, p := syntheticFilterProposal()
	r := &p.Requirement.Requirements
	r.Participants = []architecturesearch.Participant{{ID: "controller", Capability: "programmable_controller", Domain: "instrument_supply", RequiredPorts: []architecturesearch.ParticipantPort{{ID: "adc", Kind: "analog_voltage", Direction: "sink"}}}}
	r.Ports = slices.DeleteFunc(r.Ports, func(p architecturesearch.Port) bool { return p.ID == "filtered_output" })
	r.Objectives[0].Bindings[1] = architecturesearch.Binding{Role: "output", Participant: "controller", ParticipantPort: "adc"}
	r.BehavioralRequirements[0].Observation = architecturesearch.Observation{Kind: "participant_port", ID: "controller.adc"}
	p.Coverage[0].References = slices.DeleteFunc(p.Coverage[0].References, func(r Reference) bool { return r.ID == "filtered_output" })
	p.Coverage[0].References = append(p.Coverage[0].References, Reference{Kind: "requirement", ID: "controller"})
	result := Compile("Use a 5.0 to 5.4 V supply and reference return with a programmable controller ADC and analog input filtered at 2400 Hz within four percent, measure 2300 to 2500 Hz cutoff at that ADC over the full supply range, and fit 22 components within 43 by 29 mm without an extra output connector.", p, testCapabilitySHA256)
	if result.Status != StatusReady || result.Requirement == nil {
		t.Fatalf("qualified observation did not compile: %#v", result.Issues)
	}
	if result.Requirement.Requirements.BehavioralRequirements[0].Observation != (architecturesearch.Observation{Kind: "participant_port", ID: "controller.adc"}) {
		t.Fatal("observation identity changed")
	}
	if len(result.Requirement.Requirements.Ports) != 3 {
		t.Fatal("invented external output")
	}
}
