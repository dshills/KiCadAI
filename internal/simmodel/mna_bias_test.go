package simmodel

import (
	"math"
	"strings"
	"testing"
)

func TestSelectCenteredSourceBiasPreservesConnectorPolarity(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		connectorVoltageSourceEvidence("signal", "GND", "IN"),
		opAmpEvidence("buffer", "IN", "OUT", "OUT", "VCC", "GND", .125, .125),
	}
	plan := centeredBiasTestPlan(t, components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VCC"}}, []SourceExcitation{{Component: "signal", DCValue: -1.2}, {Component: "supply", DCValue: 12}})

	selection, diagnostics := SelectCenteredSourceBias(plan, "signal", "IN")
	if len(diagnostics) != 0 {
		t.Fatalf("selection diagnostics = %#v", diagnostics)
	}
	if selection.Source != "signal" || math.Abs(selection.ValueV-(-6)) > 1e-12 {
		t.Fatalf("selection = %#v, want physical source bias -6 V", selection)
	}
}

func TestSelectCenteredSourceBiasDoesNotMutateSmallSignalExcitation(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		connectorVoltageSourceEvidence("signal", "GND", "IN"),
		opAmpEvidence("buffer", "IN", "OUT", "OUT", "VCC", "GND", .125, .125),
	}
	plan := centeredBiasTestPlan(t, components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VCC"}}, []SourceExcitation{{Component: "signal", DCValue: -1.2}, {Component: "supply", DCValue: 12}})
	plan.Analyses[0].Kind = AnalysisACSweep
	plan.Analyses[0].StartFrequencyHz = 10
	plan.Analyses[0].StopFrequencyHz = 1000
	plan.Analyses[0].Points = 8
	plan.Analyses[0].Excitations[0].ACMagnitude = 1

	if _, diagnostics := SelectCenteredSourceBias(plan, "signal", "IN"); len(diagnostics) != 0 {
		t.Fatalf("selection diagnostics = %#v", diagnostics)
	}
	if got := plan.Analyses[0].Excitations[0].ACMagnitude; got != 1 {
		t.Fatalf("input plan AC magnitude mutated to %g", got)
	}
}

func TestSelectCenteredSourceBiasFailsClosedWhenGainChainHasNoSharedHeadroom(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		connectorVoltageSourceEvidence("signal", "GND", "IN"),
		opAmpEvidence("buffer", "IN", "BUF", "BUF", "VCC", "GND", 2, 2),
		opAmpEvidence("gain", "BUF", "FB", "OUT", "VCC", "GND", 2, 2),
		resistorEvidence("feedback_lower", 10000, "FB", "GND"),
		resistorEvidence("feedback_upper", 90000, "OUT", "FB"),
	}
	plan := centeredBiasTestPlan(t, components, []NodeEvidence{{Name: "BUF"}, {Name: "FB"}, {Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VCC"}}, []SourceExcitation{{Component: "signal", DCValue: -1.2}, {Component: "supply", DCValue: 12}})

	_, diagnostics := SelectCenteredSourceBias(plan, "signal", "IN")
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "no feasible centered operating point") {
		t.Fatalf("selection diagnostics = %#v", diagnostics)
	}
}

func TestSelectCenteredSourceBiasRefinesNearRailForHighGainChain(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		connectorVoltageSourceEvidence("signal", "GND", "IN"),
		opAmpEvidence("gain", "IN", "FB", "OUT", "VCC", "GND", .065, .12),
		resistorEvidence("feedback_lower", 10000, "FB", "GND"),
		resistorEvidence("feedback_upper", 990000, "OUT", "FB"),
	}
	plan := centeredBiasTestPlan(t, components, []NodeEvidence{{Name: "FB"}, {Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VCC"}}, []SourceExcitation{{Component: "signal", DCValue: -.5}, {Component: "supply", DCValue: 5}})

	selection, diagnostics := SelectCenteredSourceBias(plan, "signal", "IN")
	if len(diagnostics) != 0 {
		t.Fatalf("selection diagnostics = %#v", diagnostics)
	}
	if semantic := -selection.ValueV; semantic <= 0 || semantic >= .05 {
		t.Fatalf("selection = %#v, want a feasible near-ground high-gain bias", selection)
	}
}

func centeredBiasTestPlan(t *testing.T, components []ComponentEvidence, nodes []NodeEvidence, excitations []SourceExcitation) Plan {
	t.Helper()
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: excitations}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "IN", Quantity: QuantityVoltageV, Min: -100, Max: 100}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %#v", diagnostics)
	}
	return plan
}

func connectorVoltageSourceEvidence(id, pin1, pin2 string) ComponentEvidence {
	return ComponentEvidence{
		InstanceID: id, CatalogID: "connector.source", Family: "connector",
		ModelClaims: []CatalogEvidence{{ModelID: PrimitiveConnectorVoltageSourceV1}},
		Connections: []ConnectionEvidence{{Function: "PIN_1", Net: pin1}, {Function: "PIN_2", Net: pin2}},
	}
}

func opAmpEvidence(id, noninverting, inverting, output, positive, negative string, lowMargin, highMargin float64) ComponentEvidence {
	return ComponentEvidence{
		InstanceID: id, CatalogID: "opamp.reviewed", Family: "opamp",
		ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: []NamedValue{
			{Name: "dc_open_loop_gain", Value: 100000},
			{Name: "gain_bandwidth_hz", Value: 1000000},
			{Name: "output_high_margin_v", Value: highMargin},
			{Name: "output_low_margin_v", Value: lowMargin},
			{Name: "supply_max_v", Value: 36},
			{Name: "supply_min_v", Value: 4.5},
		}}},
		Connections: []ConnectionEvidence{
			{Function: "IN_PLUS", Net: noninverting}, {Function: "IN_MINUS", Net: inverting},
			{Function: "OUT", Net: output}, {Function: "V_PLUS", Net: positive}, {Function: "V_MINUS", Net: negative},
		},
	}
}
