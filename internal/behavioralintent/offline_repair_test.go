package behavioralintent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/reports"
)

// This is a retained failure regression, not a rerun or new evaluation pass.
func TestOfflineRepairRejectsRecordedExternalRegulatorSource(t *testing.T) {
	b, err := os.ReadFile("testdata/interface_v1_i04_counterexample.json")
	if err != nil {
		t.Fatal(err)
	}
	// The repository fixture adds only a conventional final LF to the retained
	// provider JSON. Pin every other byte to the original evidence identity.
	if got := fmt.Sprintf("%x", sha256.Sum256(bytes.TrimSuffix(b, []byte("\n")))); got != "ea3ff80eaed18fec6316a56a61a47046285576d0b56b29de48ec445a87c94b02" {
		t.Fatalf("retained counterexample content changed: %s", got)
	}
	proposal, issues := DecodeProposalStrict(bytes.NewReader(b))
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	for _, reordered := range []bool{false, true} {
		p := proposal
		if reordered {
			r := architecturesearch.Normalize(*proposal.Requirement)
			p.Requirement = &r
			slices.Reverse(r.Requirements.Objectives[0].Bindings)
		}
		result := Compile("Create requirements for a low-voltage supply-regulation board with an external nominal 7 V input ranging from 6.5 to 7.5 V, external ground, and a regulated external output. The output must remain between 2.94 and 3.06 V while sourcing any load current from 0.005 to 0.025 A. Verify that DC output-voltage range across the full input-voltage and load-current ranges at ambient temperatures from 10 to 35 degrees Celsius. Fit at most 18 components within 39 by 26 mm. Do not add a controller, digital bus, current-limit guarantee, fault-recovery timing, or a second output.", p, testCapabilitySHA256)
		if result.Status != StatusInvalid || result.Requirement != nil {
			t.Fatal("externally sourced regulated output accepted")
		}
		if !slices.ContainsFunc(result.Issues, func(i reports.Issue) bool {
			return i.Code == architecturesearch.CodeDomainInvalid && strings.Contains(i.Message, "derived supply")
		}) {
			t.Fatalf("missing source-coherence diagnosis: %#v", result.Issues)
		}
	}
}

func TestOfflineRepairSchemaBindsCircuitObservationIdentity(t *testing.T) {
	_, p := syntheticFilterProposal()
	for _, tc := range []struct {
		kind, id string
		want     bool
	}{
		{"circuit", "circuit", true}, {"circuit", "filtered_output", false}, {"port", "filtered_output", true},
		{"participant", "controller", false}, {"participant_port", "adc_input", false},
	} {
		p.Requirement.Requirements.BehavioralRequirements[0].Observation = architecturesearch.Observation{Kind: tc.kind, ID: tc.id}
		if got := providerSchemaAccepts(t, ProposalSchema(), providerWireValue(reflect.ValueOf(p))); got != tc.want {
			t.Fatalf("%s/%s schema=%v want=%v", tc.kind, tc.id, got, tc.want)
		}
	}
}

func TestOfflineRepairNamesRejectedIdentities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(*Proposal)
		expected string
	}{
		{"project coverage", func(p *Proposal) {
			p.Coverage[0].References = append(p.Coverage[0].References, Reference{Kind: "requirement", ID: "diagnostic_filter"})
		}, "diagnostic_filter"},
		{"constraint coverage", func(p *Proposal) {
			p.Coverage[0].References = append(p.Coverage[0].References, Reference{Kind: "requirement", ID: "cutoff_frequency"})
		}, "cutoff_frequency"},
		{"local participant port coverage", func(p *Proposal) {
			p.Requirement.Requirements.Participants = append(p.Requirement.Requirements.Participants, architecturesearch.Participant{ID: "controller", Capability: "analog_acquisition", Domain: "instrument_supply", RequiredPorts: []architecturesearch.ParticipantPort{{ID: "adc_local", Kind: "analog_voltage", Direction: "sink"}}})
			p.Coverage[0].References = append(p.Coverage[0].References, Reference{Kind: "requirement", ID: "controller"})
			p.Coverage[0].References = append(p.Coverage[0].References, Reference{Kind: "requirement", ID: "adc_local"})
		}, "adc_local"},
		{"objective operating target", func(p *Proposal) {
			p.Requirement.Requirements.OperatingCases[0].Conditions[0].Target = "diagnostic_band"
		}, "diagnostic_band"},
		{"duplicate constraint", func(p *Proposal) {
			p.Requirement.Requirements.Objectives[0].Constraints = append(p.Requirement.Requirements.Objectives[0].Constraints, architecturesearch.Constraint{Name: "cutoff_frequency", Relation: "range", Value: json.RawMessage(`[2300,2500]`), Unit: "Hz"})
		}, "cutoff_frequency"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt, p := syntheticFilterProposal()
			tc.mutate(&p)
			r := Compile(prompt, p, testCapabilitySHA256)
			if r.Status != StatusInvalid || r.Requirement != nil {
				t.Fatal("invalid proposal accepted")
			}
			if !slices.ContainsFunc(r.Issues, func(i reports.Issue) bool { return strings.Contains(i.Message, tc.expected) }) {
				t.Fatalf("diagnostic omits rejected identity %q: %#v", tc.expected, r.Issues)
			}
		})
	}
}

func TestOfflineRepairNormalizedConditionDiagnosticNamesActualTarget(t *testing.T) {
	prompt, p := syntheticFilterProposal()
	f := func(v float64) *float64 { return &v }
	// The valid supply comes first in provider order. Normalization puts the
	// invalid ambient objective target first; the diagnostic must name it.
	p.Requirement.Requirements.OperatingCases[0].Conditions = append(p.Requirement.Requirements.OperatingCases[0].Conditions, architecturesearch.OperatingCondition{Axis: "ambient_temperature", Target: "diagnostic_band", Min: f(10), Max: f(35), Unit: "degC"})
	r := Compile(prompt, p, testCapabilitySHA256)
	if !slices.ContainsFunc(r.Issues, func(i reports.Issue) bool {
		return i.Code == architecturesearch.CodeBindingUnresolved && strings.HasSuffix(i.Path, "conditions[0].target") && strings.Contains(i.Message, "diagnostic_band") && !strings.Contains(i.Message, "instrument_supply")
	}) {
		t.Fatalf("normalized diagnostic does not identify the actual rejected target: %#v", r.Issues)
	}
}
