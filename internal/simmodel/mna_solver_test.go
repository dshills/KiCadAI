package simmodel

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestMNAResolvesGraphAndRunsDCAndACSweep(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{
			{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "signal", DCValue: 2}, {Component: "supply", DCValue: 5}}},
			{ID: "frequency_response", Kind: AnalysisACSweep, StartFrequencyHz: 100, StopFrequencyHz: 10000, Points: 3, Excitations: []SourceExcitation{{Component: "signal", ACMagnitude: 1}, {Component: "supply"}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: 1.99, Max: 2.01},
			{AnalysisID: "frequency_response", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 1000, Min: 0.49, Max: 0.52},
		},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", bufferedTwoPoleEvidence(), []NodeEvidence{
		{Name: "OUT", Role: "signal"}, {Name: "GND", Role: "ground", VoltageDomain: "0V"}, {Name: "N2", Role: "signal"},
		{Name: "VIN", Role: "signal"}, {Name: "5V", Role: "power_pos"}, {Name: "N1", Role: "signal"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	if plan.GroundNode != "GND" || len(plan.Devices) != 7 || plan.TopologyHash == "" {
		t.Fatalf("plan = %+v", plan)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("evaluate status=%q diagnostics=%+v report=%+v", report.Status, diagnostics, report)
	}
	if len(report.Analyses) != 2 || len(report.Analyses[0].Points) != 3 || len(report.Analyses[1].Points) != 1 {
		t.Fatalf("analysis results = %+v", report.Analyses)
	}
	if report.Assertions[0].AnalysisID != "frequency_response" || math.Abs(report.Assertions[0].Actual-0.5048) > 0.01 {
		t.Fatalf("AC assertion = %+v", report.Assertions[0])
	}
	first, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	replayed, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 {
		t.Fatalf("replay diagnostics = %+v", replayDiagnostics)
	}
	second, err := json.Marshal(replayed)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("report replay differs\nfirst: %s\nsecond: %s", first, second)
	}
}

func TestMNACurrentSourceStamp(t *testing.T) {
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: .001}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: .999, Max: 1.001}},
	}
	resistance := 1000.0
	components := []ComponentEvidence{
		{InstanceID: "load", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: resistance, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "i", Family: "current_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "GND"}, {Function: "NEGATIVE", Net: "OUT"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT", Role: "signal"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestMNAOpAmpTransferIsGroundReferencedWithSplitSupply(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "positive_supply", DCValue: 5}, {Component: "negative_supply", DCValue: -5}, {Component: "signal", DCValue: 1},
		}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: .99998, Max: 1}},
	}
	opAmpParameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 30}, {Name: "supply_min_v", Value: 3},
	}
	components := []ComponentEvidence{
		{InstanceID: "positive_supply", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VP"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "negative_supply", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "signal", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "IN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "opamp", CatalogID: "opamp", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: opAmpParameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "OUT"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "VN"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VN"}, {Name: "VP"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestMNAFailsClosedOnSingularFloatingNode(t *testing.T) {
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: 1}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "FLOAT", Quantity: QuantityVoltageV, Min: 0, Max: 1}},
	}
	capacitance := 1e-6
	components := []ComponentEvidence{
		{InstanceID: "cap", CatalogID: "c", Family: "capacitor", HasValueSI: true, ValueSI: capacitance, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VIN"}, {Function: "B", Net: "FLOAT"}}},
		{InstanceID: "source", CatalogID: "v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VIN"}, {Function: "NEGATIVE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "FLOAT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "singular") || !strings.Contains(diagnostics[0].Suggestion, "floating") {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestMNAFailsClosedOnUnstablePositiveFeedback(t *testing.T) {
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "signal", DCValue: 2}, {Component: "supply", DCValue: 5}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 5}},
	}
	components := bufferedTwoPoleEvidence()
	for componentIndex := range components {
		if components[componentIndex].InstanceID != "opamp" {
			continue
		}
		for connectionIndex := range components[componentIndex].Connections {
			connection := &components[componentIndex].Connections[connectionIndex]
			switch connection.Function {
			case "IN_PLUS":
				connection.Net = "OUT"
			case "IN_MINUS":
				connection.Net = "N2"
			}
		}
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "VIN"}, {Name: "N1"}, {Name: "N2"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "unstable positive feedback") {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestMNAFailsClosedOnUnsupportedNonlinearDevice(t *testing.T) {
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "signal", DCValue: 2}, {Component: "supply", DCValue: 5}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: 0, Max: 5}},
	}
	components := append(bufferedTwoPoleEvidence(), ComponentEvidence{InstanceID: "diode", CatalogID: "diode", Family: "diode", Connections: []ConnectionEvidence{{Function: "ANODE", Net: "GND"}, {Function: "CATHODE", Net: "OUT"}}})
	_, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "VIN"}, {Name: "N1"}, {Name: "N2"}, {Name: "OUT"}})
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "no trusted linear MNA primitive") {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestMNARejectsProviderTopologyAndTamperedPlan(t *testing.T) {
	intent := Intent{ModelID: ModelLinearCircuitMNAV1, Bindings: []Binding{{Role: "filter", Component: "r1"}}}
	if diagnostics := ValidateIntent(intent, map[string]string{"r1": "resistor"}); len(diagnostics) == 0 || diagnostics[0].Path != "bindings" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
	valid := validTwoPolePlanForTest(t)
	valid.Devices[0].Terminals[0].Net = "tampered"
	if diagnostics := ValidatePlan(valid); len(diagnostics) == 0 {
		t.Fatal("expected tampered topology to fail closed")
	}
}

func validTwoPolePlanForTest(t *testing.T) Plan {
	t.Helper()
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "signal", DCValue: 2}, {Component: "supply", DCValue: 5}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: 1, Max: 3}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", bufferedTwoPoleEvidence(), []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "VIN"}, {Name: "N1"}, {Name: "N2"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	return plan
}

func bufferedTwoPoleEvidence() []ComponentEvidence {
	resistance := 10000.0
	capacitance := 10e-9
	opAmpParameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 5.5}, {Name: "supply_min_v", Value: 2.7},
	}
	return []ComponentEvidence{
		{InstanceID: "supply", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "5V"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "signal", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VIN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "r1", CatalogID: "resistor", Family: "resistor", ValueSI: resistance, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VIN"}, {Function: "B", Net: "N1"}}},
		{InstanceID: "c1", CatalogID: "capacitor", Family: "capacitor", ValueSI: capacitance, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "N1"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "r2", CatalogID: "resistor", Family: "resistor", ValueSI: resistance, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "N1"}, {Function: "B", Net: "N2"}}},
		{InstanceID: "c2", CatalogID: "capacitor", Family: "capacitor", ValueSI: capacitance, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "N2"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "opamp", CatalogID: "opamp", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: opAmpParameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "N2"}, {Function: "IN_MINUS", Net: "OUT"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "5V"}, {Function: "V_MINUS", Net: "GND"}}},
	}
}
