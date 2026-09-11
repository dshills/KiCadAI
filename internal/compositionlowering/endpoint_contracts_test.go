package compositionlowering

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/circuitgraph"
	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/components"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

// These are offline contract integration fixtures, not live-evaluation cases.
// The retained corpus is read only; mutations are local to each test instance.
func endpointIntegrationRequirement(t *testing.T, controller bool) architecturesearch.Requirement {
	t.Helper()
	b, err := os.ReadFile("../architecturesearch/testdata/simulation_grounded_closed_loop_corpus/regulated_sensor_interface.json")
	if err != nil {
		t.Fatal(err)
	}
	var r architecturesearch.Requirement
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	r.Project.Name = "offline_regulated_output"
	r.Requirements.Signals = nil
	r.Requirements.Objectives = r.Requirements.Objectives[:1]
	r.Requirements.Objectives[0].Bindings[1] = architecturesearch.Binding{Role: "output", Port: "regulated_output"}
	r.Requirements.Ports = slices.DeleteFunc(r.Requirements.Ports, func(p architecturesearch.Port) bool { return p.ID != "power" && p.ID != "ground" })
	r.Requirements.Ports = append(r.Requirements.Ports, architecturesearch.Port{ID: "regulated_output", Kind: "power", Direction: "source", Domain: "sensor_3v3"})
	r.Requirements.Domains[1].Source = "port:regulated_output"
	r.Requirements.BehavioralRequirements = slices.DeleteFunc(r.Requirements.BehavioralRequirements, func(b architecturesearch.BehavioralRequirement) bool { return b.ID != "rail" && b.ID != "thermal" })
	if controller {
		r.Project.Name = "offline_controller_adc"
		r.Requirements.Participants = []architecturesearch.Participant{{ID: "controller", Capability: "programmable_controller", Domain: "sensor_3v3", RequiredPorts: []architecturesearch.ParticipantPort{{ID: "adc", Kind: "analog_voltage", Direction: "sink"}}, Constraints: []architecturesearch.Constraint{{Name: "programmable_interface", Relation: "required", Value: json.RawMessage(`true`)}, {Name: "programming_kind", Relation: "equal", Value: json.RawMessage(`"swd"`)}}}}
		r.Requirements.Ports = append(r.Requirements.Ports, architecturesearch.Port{ID: "analog_input", Kind: "analog_voltage", Direction: "sink", Domain: "sensor_3v3"})
		r.Requirements.Objectives = append(r.Requirements.Objectives, architecturesearch.Objective{ID: "condition_adc", Capability: "frequency_filter", Bindings: []architecturesearch.Binding{{Role: "input", Port: "analog_input"}, {Role: "output", Participant: "controller", ParticipantPort: "adc"}, {Role: "power", Port: "regulated_output"}, {Role: "reference", Port: "ground"}}})
		lo, hi := 900.0, 1100.0
		r.Requirements.BehavioralRequirements = append(r.Requirements.BehavioralRequirements, architecturesearch.BehavioralRequirement{ID: "adc_cutoff", Metric: "cutoff_frequency", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "participant_port", ID: "controller.adc"}, Min: &lo, Max: &hi, Unit: "Hz", OperatingCases: []string{"supply_load"}})
	}
	return architecturesearch.Normalize(r)
}

func TestEndpointContractsReachRealLoweredNetsAndWriterRequests(t *testing.T) {
	catalog, err := components.LoadCatalog(context.Background(), components.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	registry, issues := architecturesearch.NewCatalogRegistry(catalog)
	if reports.HasBlockingIssue(issues) {
		t.Fatal(issues)
	}
	graphResolver := circuitgraph.NewResolver(circuitgraph.ResolveOptions{Catalog: catalog, CatalogID: "checked-in"})
	provenance, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	for _, tc := range []struct {
		name                        string
		controller, thermalRejected bool
		maxLoad                     float64
	}{
		{name: "standalone_regulator"},
		{name: "regulator_filter_controller_150ma_thermal_rejected", controller: true, thermalRejected: true},
		{name: "regulator_filter_controller_100ma", controller: true, maxLoad: 0.1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			controller := tc.controller
			r := endpointIntegrationRequirement(t, controller)
			if tc.maxLoad > 0 {
				// A separately named offline design variant, not a changed
				// acceptance limit for the retained 150 mA thermal rejection.
				for i := range r.Requirements.OperatingCases {
					for j := range r.Requirements.OperatingCases[i].Conditions {
						condition := &r.Requirements.OperatingCases[i].Conditions[j]
						if condition.Axis == "load_current" {
							condition.Max = &tc.maxLoad
						}
					}
				}
			}
			if issues := architecturesearch.Validate(r); reports.HasBlockingIssue(issues) {
				t.Fatal(issues)
			}
			search := architecturesearch.Search(context.Background(), r, registry, architecturesearch.SearchOptions{CatalogHash: graphResolver.CatalogHash()})
			if search.Status != architecturesearch.SearchSelected || search.Selected == nil {
				t.Fatalf("search=%s issues=%#v rejections=%#v", search.Status, search.Issues, search.Rejections)
			}
			lowered, issues := Lower(r, search)
			if reports.HasBlockingIssue(issues) {
				t.Fatal(issues)
			}
			target := func(kind, id string) string {
				for _, b := range lowered.Evidence.SemanticBindings {
					if b.Kind == kind && b.ID == id {
						return b.Target
					}
				}
				return ""
			}
			if target("port", "regulated_output") == "" || target("port", "regulated_output") != target("domain", "sensor_3v3") {
				t.Fatalf("generated output/domain not the same physical net: %#v", lowered.Evidence.SemanticBindings)
			}
			if controller && (target("participant_port", "controller.adc") == "" || target("participant_port", "controller.adc") == target("port", "analog_input")) {
				t.Fatal("ADC endpoint absent or shorted to raw input")
			}
			resolved, issues := graphResolver.Resolve(context.Background(), lowered.Document)
			if reports.HasBlockingIssue(issues) {
				t.Fatal(issues)
			}
			assertSelectedRoleOnPhysicalNet(t, search, resolved, "objective:regulate", "output", target("port", "regulated_output"))
			if controller {
				assertSelectedRoleOnPhysicalNet(t, search, resolved, "participant:controller", "adc", target("participant_port", "controller.adc"))
				assertSelectedRoleOnPhysicalNet(t, search, resolved, "participant:controller", "power", target("port", "regulated_output"))
			}
			if _, issues := circuitgraph.ToSchematicIR(resolved); reports.HasBlockingIssue(issues) {
				t.Fatal(issues)
			}
			request, issues := circuitgraph.ToDesignRequest(resolved)
			if reports.HasBlockingIssue(issues) || request.ExplicitCircuit == nil {
				t.Fatalf("writer request unavailable: %#v", issues)
			}
			second, issues := Lower(r, search)
			if reports.HasBlockingIssue(issues) {
				t.Fatal(issues)
			}
			a, _ := json.Marshal(lowered)
			b, _ := json.Marshal(second)
			if !bytes.Equal(a, b) {
				t.Fatal("lowering replay changed")
			}
			resolver := ArchitectureSimulationPlanResolver{Requirement: r, Search: search, GraphResolver: graphResolver, ProvenanceRegistry: provenance}
			plans, err := resolver.ResolveSimulationPlans(context.Background(), closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint})
			if err != nil {
				t.Fatal(err)
			}
			if len(plans) == 0 {
				t.Fatal("no executable simulation plans")
			}
			state := closedloopsynthesis.CandidateState{Fingerprint: search.Selected.Fingerprint}
			planSet, err := resolver.ResolveSimulationPlanSet(context.Background(), state)
			if err != nil {
				t.Fatal(err)
			}
			if controller {
				observed := false
				for _, assertion := range planSet.AnalysisPlan.Assertions {
					if assertion.RequirementID == "adc_cutoff" {
						observed = assertion.Target == target("participant_port", "controller.adc")
					}
				}
				if !observed {
					t.Fatal("simulation assertion not bound to exact physical ADC net")
				}
			}
			evaluator := closedloopsynthesis.SimModelEvaluator{Resolver: closedloopsynthesis.PlannedSimulationResolver{Base: resolver}, ProvenanceRegistry: provenance}
			evaluation, err := evaluator.Evaluate(context.Background(), state)
			if tc.thermalRejected {
				if err == nil || !strings.Contains(err.Error(), "exceeds catalog-backed maximum 125 C") {
					t.Fatalf("expected thermal gate rejection, got %v", err)
				}
				t.Logf("retained engineering rejection: %v", err)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if evaluation.EvidenceHash == "" || len(evaluation.Measurements) != len(r.Requirements.BehavioralRequirements) {
				t.Fatalf("incomplete simulation evidence: %#v", evaluation)
			}
			for _, behavior := range r.Requirements.BehavioralRequirements {
				found := false
				for _, measurement := range evaluation.Measurements {
					if measurement.RequirementID != behavior.ID {
						continue
					}
					found = true
					if behavior.Min != nil && measurement.Actual < *behavior.Min || behavior.Max != nil && measurement.Actual > *behavior.Max {
						t.Fatalf("%s failed requested bounds: %g", behavior.ID, measurement.Actual)
					}
				}
				if !found {
					t.Fatalf("missing measurement for %s", behavior.ID)
				}
			}
			replay, err := evaluator.Evaluate(context.Background(), state)
			if err != nil || replay.EvidenceHash != evaluation.EvidenceHash {
				t.Fatalf("simulation replay changed: %v %s != %s", err, replay.EvidenceHash, evaluation.EvidenceHash)
			}
			t.Logf("simulation measurements=%+v evidence=%s", evaluation.Measurements, evaluation.EvidenceHash)
			t.Logf("selected=%s components=%d physical_nets=%d plans=%d exact_adc=%s", search.Selected.Fingerprint, len(resolved.Components), len(resolved.Nets), len(plans), target("participant_port", "controller.adc"))
		})
	}
}

func assertSelectedRoleOnPhysicalNet(t *testing.T, search architecturesearch.SearchResult, resolved circuitgraph.ResolvedDocument, obligation, role, netName string) {
	t.Helper()
	found := false
	for _, selection := range search.Selected.Selections {
		if selection.ObligationPath != obligation {
			continue
		}
		realization, err := architecturesearch.DecodeFragmentRealization(selection.Payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, binding := range realization.PortBindings {
			if binding.Role != role || binding.Lane != "" {
				continue
			}
			instanceID := safeID(safeID(obligation) + "__" + binding.Instance)
			for _, net := range resolved.Nets {
				if net.Intent.Name != netName {
					continue
				}
				for _, endpoint := range net.Endpoints {
					if endpoint.Intent.Component != instanceID || endpoint.Function != binding.Function {
						continue
					}
					for _, physical := range endpoint.Bindings {
						if physical.SymbolPin != "" && physical.Pad != "" {
							found = true
							t.Logf("%s.%s -> %s.%s symbol_pin=%s pad=%s net=%s", obligation, role, instanceID, binding.Function, physical.SymbolPin, physical.Pad, netName)
						}
					}
				}
			}
		}
	}
	if !found {
		t.Fatalf("%s.%s lacks its selected physical pin/pad on %s", obligation, role, netName)
	}
}
