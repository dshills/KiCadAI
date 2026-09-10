package opentopologysynthesis

import (
	"context"
	"testing"
)

// This public two-state fixture also exposes a preexisting catalog boundary:
// the resistor pin-role metadata cannot prove an open-collector pull path.
// Control repair must refuse that incomplete source, not infer missing roles.
func TestControlRebindingV22RefusesComparatorWithoutDeclaredPullRoles(t *testing.T) {
	r, _, inventory, environment, admission := testControlBindingFixtureV22(t)
	r.Project = Project{Name: "independent_state_indicator", Title: "Independent two-state control", Description: "Indicate whether an external signal is above or below an external comparison voltage."}
	r.Requirements.Ports = append(r.Requirements.Ports, Port{ID: "comparison", Kind: "analog_voltage", Direction: "sink", Domain: "common", Electrical: Electrical{MinVoltageV: graphFloat(.5), NominalVoltageV: graphFloat(.5), MaxVoltageV: graphFloat(.5)}})
	for i := range r.Requirements.Ports {
		if r.Requirements.Ports[i].ID == "stimulus" {
			r.Requirements.Ports[i].Electrical = Electrical{MinVoltageV: graphFloat(.25), NominalVoltageV: graphFloat(.75), MaxVoltageV: graphFloat(.75)}
		}
		if r.Requirements.Ports[i].ID == "reading" {
			r.Requirements.Ports[i].Electrical = Electrical{MinVoltageV: graphFloat(0), NominalVoltageV: graphFloat(12), MaxVoltageV: graphFloat(12)}
		}
	}
	r.Requirements.OperatingCases = []OperatingCase{}
	for _, c := range []struct {
		id    string
		input float64
	}{{"below", .25}, {"above", .75}} {
		r.Requirements.OperatingCases = append(r.Requirements.OperatingCases, OperatingCase{ID: c.id, Conditions: []OperatingCondition{{Axis: "supply_voltage", Target: "rail", Min: 12, Max: 12, Unit: "V"}, {Axis: "input_voltage", Target: "stimulus", Min: c.input, Max: c.input, Unit: "V"}, {Axis: "input_voltage", Target: "comparison", Min: .5, Max: .5, Unit: "V"}}})
	}
	r.Requirements.BehavioralRequirements = []BehavioralAssertion{
		{ID: "low", Metric: "output_voltage", Analysis: "dc_operating_point", Observation: Observation{Kind: "port", ID: "reading"}, Max: graphFloat(.3), Unit: "V", OperatingCases: []string{"below"}},
		{ID: "high", Metric: "output_voltage", Analysis: "dc_operating_point", Observation: Observation{Kind: "port", ID: "reading"}, Min: graphFloat(11.8), Unit: "V", OperatingCases: []string{"above"}},
	}
	r = Normalize(r)
	if issues := Validate(r); len(issues) != 0 {
		t.Fatal(issues)
	}
	graph, issues := InitialGraph(r)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	device, found := primitiveByKey(inventory, "comparator.ti.tlv1701aidbvr.sot23_5|sot23_5")
	if !found {
		t.Fatal("independent comparator model unavailable")
	}
	resistor, found := valuePrimitiveAt(inventory, r, requirementAnalysisSet(r), "resistor", 12500)
	if !found {
		t.Fatal("independent pull resource unavailable")
	}
	graph = AddPrimitive(graph, device, nil, []TerminalConnection{{Terminal: "IN_MINUS", Node: "port_stimulus"}, {Terminal: "IN_PLUS", Node: "port_stimulus"}, {Terminal: "OUT", Node: "port_reading"}, {Terminal: "V_MINUS", Node: "port_common"}, {Terminal: "V_PLUS", Node: "port_rail"}})
	graph = AddPrimitive(graph, resistor, graphFloat(12500), []TerminalConnection{{Terminal: "A", Node: "port_reading"}, {Terminal: "B", Node: "port_rail"}})
	graph = AddPrimitive(graph, resistor, graphFloat(12500), []TerminalConnection{{Terminal: "A", Node: "port_comparison"}, {Terminal: "B", Node: "port_common"}})
	graph, err := NormalizeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	initial := EvaluateCandidateV20(context.Background(), r, graph, nil, inventory, environment, admission, DefaultPolicy())
	if initial.Status == SimulationEvaluationPassed {
		t.Fatal("uncontrolled comparison unexpectedly passed both states")
	}
	batch, err := controlRebindingsV22(context.Background(), r, graph, inventory, 32)
	if err == nil || len(batch.candidates) != 0 || batch.work != 0 {
		t.Fatalf("incomplete pull/reference evidence must refuse before search: %+v %v", batch, err)
	}
	structure := AnalyzeTopologyV21(r, graph, inventory)
	if structure.Complete || !structure.Contradictory || len(structure.Issues) == 0 {
		t.Fatal("catalog boundary no longer reproduced; reassess this refusal fixture")
	}
}
