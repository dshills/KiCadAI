package simmodel

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDiscoverControlLoopsIsCanonicalAndReportsReturnRatioEvidence(t *testing.T) {
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000},
		{Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1},
		{Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 30},
		{Name: "supply_min_v", Value: 3},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "IN", "GND"),
		voltageSourceEvidence("positive_supply", "VP", "GND"),
		voltageSourceEvidence("negative_supply", "VN", "GND"),
		{
			InstanceID: "opamp", CatalogID: "opamp", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: "IN"},
				{Function: "IN_MINUS", Net: "OUT"},
				{Function: "OUT", Net: "OUT"},
				{Function: "V_PLUS", Net: "VP"},
				{Function: "V_MINUS", Net: "VN"},
			},
		},
	}
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "stability", Kind: AnalysisStability,
			StartFrequencyHz: 1, StopFrequencyHz: 1e8, Points: 64,
			Excitations: []SourceExcitation{{Component: "negative_supply"}, {Component: "positive_supply"}, {Component: "signal"}},
		}},
		Assertions: []Assertion{{AnalysisID: "stability", Node: "OUT", Quantity: QuantityPhaseMarginDeg, Min: 85, Max: 95}},
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VN"}, {Name: "VP"}}
	firstPlan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve first plan: %#v", diagnostics)
	}
	first, diagnostics := Evaluate(firstPlan)
	if len(diagnostics) != 0 || first.Status != "pass" {
		t.Fatalf("evaluate first plan: report=%#v diagnostics=%#v", first, diagnostics)
	}
	if len(first.Analyses) != 1 || len(first.Analyses[0].ControlLoops) != 1 {
		t.Fatalf("control loops = %#v", first.Analyses)
	}
	loop := first.Analyses[0].ControlLoops[0]
	if loop.ID != "loop:opamp:in_minus" || loop.Polarity != "negative" || !loop.DCPreserved ||
		loop.CrossoverFrequencyHz <= 0 || loop.PhaseMarginDeg < 85 || loop.PhaseMarginDeg > 95 ||
		loop.GainMarginDB != maxReportedMarginDB || len(loop.ReturnRatioSamplesSHA256) != 64 {
		t.Fatalf("loop evidence = %#v", loop)
	}

	reorderedComponents := append([]ComponentEvidence(nil), components...)
	slices.Reverse(reorderedComponents)
	reorderedNodes := append([]NodeEvidence(nil), nodes...)
	slices.Reverse(reorderedNodes)
	secondPlan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", reorderedComponents, reorderedNodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve reordered plan: %#v", diagnostics)
	}
	second, diagnostics := Evaluate(secondPlan)
	if len(diagnostics) != 0 || second.Status != "pass" {
		t.Fatalf("evaluate reordered plan: report=%#v diagnostics=%#v", second, diagnostics)
	}
	firstJSON, _ := json.Marshal(first.Analyses[0].ControlLoops)
	secondJSON, _ := json.Marshal(second.Analyses[0].ControlLoops)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("reordered loop evidence differs\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
}

func TestStabilitySelectsOnlyLoopsObservedByItsAssertions(t *testing.T) {
	loops := []ControlLoop{
		{ID: "loop:buck", ActiveComponent: "buck", ObservationNet: "POWER_OUT"},
		{ID: "loop:servo", ActiveComponent: "servo", ObservationNet: "SERVO_OUT"},
	}
	assertions := []Assertion{
		{AnalysisID: "stability_power", Node: "POWER_OUT", Quantity: QuantityPhaseMarginDeg},
		{AnalysisID: "stability_servo", Node: "SERVO_OUT", Quantity: QuantityPhaseMarginDeg},
	}
	selected := controlLoopsForStabilityAnalysis(loops, assertions, "stability_power")
	if len(selected) != 1 || selected[0].ID != "loop:buck" {
		t.Fatalf("selected stability loops = %#v, want only the asserted power loop", selected)
	}
	if all := controlLoopsForStabilityAnalysis(loops, nil, "stability_power"); len(all) != len(loops) {
		t.Fatalf("unasserted exploratory stability selected %#v, want all loops", all)
	}
}

func TestDiscoverControlLoopsTraversesDiodeConnectedBJTTracker(t *testing.T) {
	device := func(component, model string, terminals ...TerminalBinding) ResolvedDevice {
		return ResolvedDevice{Component: component, PrimitiveModel: model, Terminals: terminals}
	}
	plan := Plan{
		GroundNode: "GND",
		Devices: []ResolvedDevice{
			device("opamp", PrimitiveOpAmpV1,
				TerminalBinding{Terminal: "IN_PLUS", Net: "IN"},
				TerminalBinding{Terminal: "IN_MINUS", Net: "FB"},
				TerminalBinding{Terminal: "OUT", Net: "DRIVE"}),
			device("bias_tracker", PrimitiveBJTNPNV1,
				TerminalBinding{Terminal: "BASE", Net: "BIAS"},
				TerminalBinding{Terminal: "COLLECTOR", Net: "BIAS"},
				TerminalBinding{Terminal: "EMITTER", Net: "DRIVE"}),
			device("driver", PrimitiveBJTNPNV1,
				TerminalBinding{Terminal: "BASE", Net: "BIAS"},
				TerminalBinding{Terminal: "COLLECTOR", Net: "VP"},
				TerminalBinding{Terminal: "EMITTER", Net: "POWER_BASE"}),
			device("power", PrimitiveBJTNPNV1,
				TerminalBinding{Terminal: "BASE", Net: "POWER_BASE"},
				TerminalBinding{Terminal: "COLLECTOR", Net: "VP"},
				TerminalBinding{Terminal: "EMITTER", Net: "OUT"}),
			device("feedback", PrimitiveResistorV1,
				TerminalBinding{Terminal: "A", Net: "OUT"},
				TerminalBinding{Terminal: "B", Net: "FB"}),
		},
	}
	loops, diagnostics := DiscoverControlLoops(plan)
	if len(diagnostics) != 0 || len(loops) != 1 {
		t.Fatalf("diode-connected tracker loop discovery: loops=%#v diagnostics=%#v", loops, diagnostics)
	}
	if loops[0].NetPath[0] != "DRIVE" || loops[0].NetPath[len(loops[0].NetPath)-1] != "FB" {
		t.Fatalf("loop path = %#v", loops[0].NetPath)
	}
}

func TestDiscoverControlLoopsFailsClosedForPositiveAndAmbiguousFeedback(t *testing.T) {
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000},
		{Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1},
		{Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 30},
		{Name: "supply_min_v", Value: 3},
	}
	base := []ComponentEvidence{
		voltageSourceEvidence("positive_supply", "VP", "GND"),
		voltageSourceEvidence("negative_supply", "VN", "GND"),
	}
	opamp := func(positive, negative string) ComponentEvidence {
		return ComponentEvidence{
			InstanceID: "opamp", CatalogID: "opamp", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: positive},
				{Function: "IN_MINUS", Net: negative},
				{Function: "OUT", Net: "OUT"},
				{Function: "V_PLUS", Net: "VP"},
				{Function: "V_MINUS", Net: "VN"},
			},
		}
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OPEN"}, {Name: "OUT"}, {Name: "VN"}, {Name: "VP"}}
	resolveIntent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "dc", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "negative_supply", DCValue: -5}, {Component: "positive_supply", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "dc", Node: "OUT", Quantity: QuantityVoltageV, Min: -10, Max: 10}},
	}

	positiveComponents := append(append([]ComponentEvidence(nil), base...), opamp("OUT", "OPEN"))
	positivePlan, diagnostics := ResolveWithTopology(resolveIntent, "catalog", "catalog-hash", positiveComponents, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve positive plan: %#v", diagnostics)
	}
	if _, diagnostics = DiscoverControlLoops(positivePlan); len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "positive-feedback") {
		t.Fatalf("positive feedback diagnostics = %#v", diagnostics)
	}

	ambiguousComponents := append(append([]ComponentEvidence(nil), base...), opamp("OUT", "OUT"))
	ambiguousPlan, diagnostics := ResolveWithTopology(resolveIntent, "catalog", "catalog-hash", ambiguousComponents, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve ambiguous plan: %#v", diagnostics)
	}
	if _, diagnostics = DiscoverControlLoops(ambiguousPlan); len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "simultaneous positive- and negative-feedback") {
		t.Fatalf("ambiguous feedback diagnostics = %#v", diagnostics)
	}
}

func TestDiscoverControlLoopsRetainsDistinctFrequencySelectivePositiveReturnPath(t *testing.T) {
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000},
		{Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1},
		{Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 30},
		{Name: "supply_min_v", Value: 3},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("positive_supply", "VP", "GND"),
		voltageSourceEvidence("negative_supply", "VN", "GND"),
		{
			InstanceID: "opamp", CatalogID: "opamp", Family: "opamp",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "IN_PLUS", Net: "FILTERED"},
				{Function: "IN_MINUS", Net: "OUT"},
				{Function: "OUT", Net: "OUT"},
				{Function: "V_PLUS", Net: "VP"},
				{Function: "V_MINUS", Net: "VN"},
			},
		},
		{
			InstanceID: "feedback_capacitor", CatalogID: "capacitor", Family: "capacitor",
			HasValueSI: true, ValueSI: 1e-9, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}},
			Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "FILTERED"}},
		},
	}
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "stability", Kind: AnalysisStability,
			StartFrequencyHz: 1, StopFrequencyHz: 1e8, Points: 64,
			Excitations: []SourceExcitation{{Component: "negative_supply"}, {Component: "positive_supply"}},
		}},
		Assertions: []Assertion{{AnalysisID: "stability", Node: "OUT", Quantity: QuantityPhaseMarginDeg, Min: 0}},
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "FILTERED"}, {Name: "OUT"}, {Name: "VN"}, {Name: "VP"}}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve plan: %#v", diagnostics)
	}
	loops, diagnostics := DiscoverControlLoops(plan)
	if len(diagnostics) != 0 || len(loops) != 1 {
		t.Fatalf("loops=%#v diagnostics=%#v devices=%#v", loops, diagnostics, plan.Devices)
	}
	if !slices.Contains(loops[0].Members, "feedback_capacitor") {
		t.Fatalf("frequency-selective return path is absent from loop evidence: %#v", loops[0])
	}
}
