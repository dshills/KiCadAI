package simmodel

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestNonlinearDCDiodeOperatingPointIsDeterministic(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"),
		resistorEvidence("limit", 1000, "5V", "OUT"),
		{InstanceID: "diode", CatalogID: "diode.onsemi.1n4148w.sod_123", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan := resolveNonlinearTestPlan(t, components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}}, []Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: .55, Max: .9}})
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" || report.Analyses[0].Points[0].Solver == nil {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	first, _ := json.Marshal(report)
	replayed, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 {
		t.Fatalf("replay diagnostics=%+v", replayDiagnostics)
	}
	second, _ := json.Marshal(replayed)
	if string(first) != string(second) {
		t.Fatalf("nonlinear replay differs\n%s\n%s", first, second)
	}
}

func TestNonlinearDCNPNAndPNPBias(t *testing.T) {
	for _, test := range []struct {
		name       string
		primitive  string
		components []ComponentEvidence
		nodes      []NodeEvidence
		assertions []Assertion
	}{
		{
			name: "npn", primitive: PrimitiveBJTNPNV1,
			components: []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("base_bias", 470000, "5V", "BASE"), resistorEvidence("collector_load", 1000, "5V", "COLLECTOR")},
			nodes:      []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "BASE"}, {Name: "COLLECTOR"}},
			assertions: []Assertion{{AnalysisID: "bias", Node: "BASE", Quantity: QuantityVoltageV, Min: .5, Max: .9}, {AnalysisID: "bias", Node: "COLLECTOR", Quantity: QuantityVoltageV, Min: 3.5, Max: 4.8}},
		},
		{
			name: "pnp", primitive: PrimitiveBJTPNPV1,
			components: []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("base_bias", 470000, "BASE", "GND"), resistorEvidence("collector_load", 1000, "COLLECTOR", "GND")},
			nodes:      []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "BASE"}, {Name: "COLLECTOR"}},
			assertions: []Assertion{{AnalysisID: "bias", Node: "BASE", Quantity: QuantityVoltageV, Min: 4.1, Max: 4.5}, {AnalysisID: "bias", Node: "COLLECTOR", Quantity: QuantityVoltageV, Min: .2, Max: 1.5}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			emitterNet := "GND"
			if test.primitive == PrimitiveBJTPNPV1 {
				emitterNet = "5V"
			}
			test.components = append(test.components, ComponentEvidence{InstanceID: "q1", CatalogID: "reviewed-bjt", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: test.primitive, Parameters: bjtParameters(.2, 40)}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "COLLECTOR"}, {Function: "EMITTER", Net: emitterNet}}})
			plan := resolveNonlinearTestPlan(t, test.components, test.nodes, test.assertions)
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
			}
		})
	}
}

func TestNonlinearDCRejectsACAmbiguousClaimsAndOperatingLimit(t *testing.T) {
	base := []ComponentEvidence{voltageSourceEvidence("supply", "5V", "GND"), resistorEvidence("limit", 1000, "5V", "OUT"), {InstanceID: "diode", CatalogID: "diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}}}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}}
	ac := Intent{ModelID: ModelNonlinearCircuitDCV1, Analyses: []Analysis{{ID: "ac", Kind: AnalysisACSweep, StartFrequencyHz: 1, StopFrequencyHz: 10, Points: 2, Excitations: []SourceExcitation{{Component: "supply", ACMagnitude: 1}}}}, Assertions: []Assertion{{AnalysisID: "ac", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 1, Min: 0, Max: 10}}}
	if _, diagnostics := ResolveWithTopology(ac, "test", "hash", base, nodes); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "DC operating points only") {
		t.Fatalf("AC diagnostics=%+v", diagnostics)
	}
	ambiguous := append([]ComponentEvidence(nil), base...)
	ambiguous[2].ModelClaims = append(ambiguous[2].ModelClaims, ambiguous[2].ModelClaims[0])
	intent := nonlinearTestIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 5}})
	if _, diagnostics := ResolveWithTopology(intent, "test", "hash", ambiguous, nodes); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "ambiguous") {
		t.Fatalf("ambiguous diagnostics=%+v", diagnostics)
	}
	missing := append([]ComponentEvidence(nil), base...)
	missing[2].ModelClaims = []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)[1:]}}
	if _, diagnostics := ResolveWithTopology(intent, "test", "hash", missing, nodes); len(diagnostics) == 0 || !diagnosticsContain(diagnostics, "missing required parameter saturation_current_a") {
		t.Fatalf("missing-parameter diagnostics=%+v", diagnostics)
	}
	limited := append([]ComponentEvidence(nil), base...)
	limited[2].ModelClaims = []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(1e-6, 100)}}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", limited, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "forward current") || diagnostics[0].Suggestion == "" {
		t.Fatalf("limit diagnostics=%+v", diagnostics)
	}
}

func diagnosticsContain(diagnostics []Diagnostic, fragment string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, fragment) {
			return true
		}
	}
	return false
}

func TestNonlinearDCReportsActionableBoundedSolveFailure(t *testing.T) {
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"),
		voltageSourceEvidence("conflict", "5V", "GND"),
		resistorEvidence("limit", 1000, "5V", "OUT"),
		{InstanceID: "diode", CatalogID: "diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.2, 100)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	intent := nonlinearTestIntent([]Assertion{{AnalysisID: "bias", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 5}})
	intent.Analyses[0].Excitations = append(intent.Analyses[0].Excitations, SourceExcitation{Component: "conflict", DCValue: 4})
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "continuation stage") || !strings.Contains(diagnostics[0].Suggestion, "bias path") {
		t.Fatalf("solve diagnostics=%+v", diagnostics)
	}
}

func TestBidirectionalTVSClampsBothPolarities(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "negative", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: -12}}},
			{ID: "positive", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: 12}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "negative", Node: "OUT", Quantity: QuantityVoltageV, Min: -9.52, Max: -9.50},
			{AnalysisID: "positive", Node: "OUT", Quantity: QuantityVoltageV, Min: 9.50, Max: 9.52},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "SOURCE", "GND"),
		resistorEvidence("series", 100, "SOURCE", "OUT"),
		{InstanceID: "clamp", CatalogID: "tvs", Family: "protection", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBidirectionalTVSV1, Parameters: tvsParameters()}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}, {Name: "SOURCE"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func resolveNonlinearTestPlan(t *testing.T, components []ComponentEvidence, nodes []NodeEvidence, assertions []Assertion) Plan {
	t.Helper()
	plan, diagnostics := ResolveWithTopology(nonlinearTestIntent(assertions), "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics=%+v", diagnostics)
	}
	return plan
}

func nonlinearTestIntent(assertions []Assertion) Intent {
	return Intent{ModelID: ModelNonlinearCircuitDCV1, Analyses: []Analysis{{ID: "bias", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}}}, Assertions: assertions}
}

func voltageSourceEvidence(id, positive, negative string) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: "source.voltage", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: positive}, {Function: "NEGATIVE", Net: negative}}}
}

func resistorEvidence(id string, value float64, a, b string) ComponentEvidence {
	return ComponentEvidence{InstanceID: id, CatalogID: "resistor", Family: "resistor", ValueSI: value, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: a}, {Function: "B", Net: b}}}
}

func diodeParameters(maxCurrent, maxReverse float64) []NamedValue {
	return []NamedValue{{Name: "saturation_current_a", Value: 4e-9}, {Name: "emission_coefficient", Value: 1.9}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_forward_current_a", Value: maxCurrent}, {Name: "max_reverse_voltage_v", Value: maxReverse}}
}

func tvsParameters() []NamedValue {
	return []NamedValue{
		{Name: "breakdown_voltage_v", Value: 9.5},
		{Name: "dynamic_resistance_ohm", Value: .5},
		{Name: "max_pulse_current_a", Value: 12},
		{Name: "off_resistance_ohm", Value: 50e6},
	}
}

func bjtParameters(maxCurrent, maxVoltage float64) []NamedValue {
	return []NamedValue{{Name: "saturation_current_a", Value: 1e-14}, {Name: "forward_beta", Value: 100}, {Name: "reverse_beta", Value: 1}, {Name: "emission_coefficient", Value: 1}, {Name: "junction_temperature_k", Value: 300.15}, {Name: "max_collector_current_a", Value: maxCurrent}, {Name: "max_collector_emitter_voltage_v", Value: maxVoltage}}
}

func TestBoundedExponentialIsFinite(t *testing.T) {
	value, derivative := boundedExponential(1e9)
	if math.IsInf(value, 0) || math.IsInf(derivative, 0) || value <= 0 || derivative <= 0 {
		t.Fatalf("value=%g derivative=%g", value, derivative)
	}
}
