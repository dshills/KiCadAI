package architecturesearch

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/reports"
)

// Keep existing supported multi-function derived-rail contracts valid.
// Mutations are authored offline and never written back to the frozen fixture.
func TestRegulationSourceCoherencePreservesDerivedRailsAndRejectsSubstitution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Requirement)
		reject bool
	}{
		{"supported derived rail", func(r *Requirement) {}, false},
		{"external rail substitution", func(r *Requirement) { r.Requirements.Domains[1].Source = "external" }, true},
		{"source signal of another identity", func(r *Requirement) { r.Requirements.Domains[1].Source = "translated_bus" }, true},
		{"wrong kind producer", func(r *Requirement) { r.Requirements.Signals[0].Kind = "analog_voltage" }, true},
		{"external output laundering", func(r *Requirement) {
			r.Requirements.Ports = append(r.Requirements.Ports, Port{ID: "external_extra", Kind: "power", Direction: "source", Domain: "input_5v"})
			r.Requirements.Objectives[0].Bindings = append(r.Requirements.Objectives[0].Bindings, Binding{Role: "exposed_power", Port: "external_extra"})
		}, true},
		{"derived external output", func(r *Requirement) {
			r.Requirements.Ports = append(r.Requirements.Ports, Port{ID: "rail_access", Kind: "power", Direction: "source", Domain: "sensor_3v3"})
			r.Requirements.Objectives[0].Bindings = append(r.Requirements.Objectives[0].Bindings, Binding{Role: "exposed_power", Port: "rail_access"})
		}, false},
		{"input direction legacy convention", func(r *Requirement) { r.Requirements.Ports[1].Direction = "source" }, false},
		{"non-regulation unaffected", func(r *Requirement) {
			r.Requirements.Objectives[0].Capability = "unknown_power_function"
			r.Requirements.Domains[1].Source = "external"
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := os.ReadFile("testdata/simulation_grounded_closed_loop_corpus/regulated_sensor_interface.json")
			if err != nil {
				t.Fatal(err)
			}
			var r Requirement
			if err = json.Unmarshal(b, &r); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&r)
			for _, reversed := range []bool{false, true} {
				if reversed {
					slices.Reverse(r.Requirements.Domains)
					slices.Reverse(r.Requirements.Objectives)
				}
				issues := Validate(Normalize(r))
				sourceFailure := slices.ContainsFunc(issues, func(i reports.Issue) bool {
					return i.Code == CodeDomainInvalid && strings.Contains(i.Message, "derived supply")
				})
				if sourceFailure != tc.reject {
					t.Fatalf("source failure=%v want=%v: %#v", sourceFailure, tc.reject, issues)
				}
				if !tc.reject && reports.HasBlockingIssue(issues) {
					t.Fatalf("compatible contract rejected: %#v", issues)
				}
			}
		})
	}
}
