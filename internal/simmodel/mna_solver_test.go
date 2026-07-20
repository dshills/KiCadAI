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
		ModelID:  ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: .001}}}},
		Assertions: []Assertion{
			{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: .999, Max: 1.001},
			{AnalysisID: "operating_point", Component: "source", Quantity: QuantityDeviceCurrentA, Min: .000999, Max: .001001},
			{AnalysisID: "operating_point", Node: "OUT", Component: "source", Quantity: QuantityTransimpedanceOhm, Min: 999, Max: 1001},
		},
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

func TestMNAACDerivedGainAndCutoff(t *testing.T) {
	intent := Intent{
		ModelID:  ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{ID: "response", Kind: AnalysisACSweep, StartFrequencyHz: 10, StopFrequencyHz: 100000, Points: 41, Excitations: []SourceExcitation{{Component: "signal", ACMagnitude: 1}}}},
		Assertions: []Assertion{
			{AnalysisID: "response", Node: "OUT", ReferenceNode: "IN", Quantity: QuantityVoltageGainRatio, FrequencyHz: 1000, Min: .83, Max: .86},
			{AnalysisID: "response", Node: "OUT", ReferenceNode: "IN", Quantity: QuantityCutoffFrequencyHz, Min: 1500, Max: 1700},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "IN", "GND"),
		resistorEvidence("resistor", 1000, "IN", "OUT"),
		{InstanceID: "capacitor", CatalogID: "capacitor", Family: "capacitor", HasValueSI: true, ValueSI: 1e-7, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("AC derived resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("AC derived report = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestTransientDerivedWaveformMeasurements(t *testing.T) {
	result := AnalysisResult{ID: "wave", Kind: AnalysisTransient, Points: []AnalysisPoint{
		{TimeS: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}}, Devices: []DeviceResult{{Component: "load", VoltageV: 0, CurrentA: 0}}},
		{TimeS: 1, Nodes: []NodeResult{{Node: "OUT", Real: 10}}, Devices: []DeviceResult{{Component: "load", VoltageV: 10, CurrentA: .1}}},
		{TimeS: 2, Nodes: []NodeResult{{Node: "OUT", Real: 9.9}}, Devices: []DeviceResult{{Component: "load", VoltageV: 9.9, CurrentA: .099}}},
		{TimeS: 3, Nodes: []NodeResult{{Node: "OUT", Real: 10}}, Devices: []DeviceResult{{Component: "load", VoltageV: 10, CurrentA: .1}}},
	}}
	for _, test := range []struct {
		quantity string
		wantMin  float64
		wantMax  float64
	}{
		{QuantityOutputSwingVPP, 9.99, 10.01},
		{QuantitySettlingTimeS, .99, 1.01},
		{QuantityResponseTimeS, .79, .81},
		{QuantityOutputPowerW, .98, 1.01},
	} {
		assertion := Assertion{AnalysisID: "wave", Node: "OUT", Component: "load", Quantity: test.quantity}
		if test.quantity != QuantityOutputPowerW {
			assertion.Component = ""
		}
		actual, diagnostic := transientDerivedValue(result, assertion)
		if diagnostic != nil || actual < test.wantMin || actual > test.wantMax {
			t.Fatalf("%s = %.12g diagnostic=%#v", test.quantity, actual, diagnostic)
		}
	}
}

func TestMNAWorstCaseRehashesAlteredCorners(t *testing.T) {
	intent := Intent{ModelID: ModelLinearCircuitMNAV1, WorstCase: true,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: .001}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: .79, Max: 1.22}},
	}
	components := []ComponentEvidence{
		{InstanceID: "load.unit_a", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}, Uncertainties: []Uncertainty{{Target: "value_si", Source: "catalog:r:resistance_tolerance", Nominal: 1000, Minimum: 900, Maximum: 1100}}},
		{InstanceID: "source", CatalogID: "i", Family: "current_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "GND"}, {Function: "NEGATIVE", Net: "OUT"}}, Uncertainties: []Uncertainty{{Target: "excitation_dc_value", Source: "reviewed-system-supply", Nominal: .001, Minimum: .0009, Maximum: .0011}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT", Role: "signal"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" || len(report.Corners) != 9 {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestMNAAnalysisDeviceOverridesApplyIndependentCorners(t *testing.T) {
	highResistance := 2000.0
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{
			{ID: "load_high", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: .001}}, DeviceOverrides: []DeviceOverride{{Component: "load", ValueSI: &highResistance}}},
			{ID: "load_nominal", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: .001}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "load_high", Node: "OUT", Quantity: QuantityVoltageV, Min: 1.999, Max: 2.001},
			{AnalysisID: "load_nominal", Node: "OUT", Quantity: QuantityVoltageV, Min: .999, Max: 1.001},
		},
	}
	components := []ComponentEvidence{
		{InstanceID: "load", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "i", Family: "current_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "GND"}, {Function: "NEGATIVE", Net: "OUT"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("override resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("override report = %#v diagnostics=%#v", report, diagnostics)
	}
	if report.Assertions[0].Actual != 2 || report.Assertions[1].Actual != 1 {
		t.Fatalf("override assertions = %#v", report.Assertions)
	}
	if *plan.Devices[0].ValueSI != 1000 {
		t.Fatalf("base resolved device was mutated: %#v", plan.Devices[0])
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

func TestMNAOpAmpDCClampsOpenLoopComparatorToCatalogOutputRange(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{
			{ID: "input_high", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "threshold", DCValue: 2.5}, {Component: "signal", DCValue: 3}}},
			{ID: "input_low", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "threshold", DCValue: 2.5}, {Component: "signal", DCValue: 2}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "input_high", Node: "OUT", Quantity: QuantityVoltageV, Min: 4.899, Max: 4.901},
			{AnalysisID: "input_low", Node: "OUT", Quantity: QuantityVoltageV, Min: .099, Max: .101},
		},
	}
	opAmpParameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 30}, {Name: "supply_min_v", Value: 3},
	}
	components := []ComponentEvidence{
		{InstanceID: "supply", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VP"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "threshold", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "THRESH"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "signal", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "IN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "comparator", CatalogID: "opamp", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: opAmpParameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "THRESH"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "THRESH"}, {Name: "VP"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestMNABidirectionalDCSweepMeasuresThresholdAndHysteresis(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "decision", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "reference", DCValue: 2.5}, {Component: "signal"}},
			DCSweep:     &DCSweep{Component: "signal", StartValue: 0, StopValue: 5, Points: 101, Bidirectional: true},
		}},
		Assertions: []Assertion{
			{AnalysisID: "decision", Node: "OUT", Quantity: QuantityThresholdVoltageV, Min: 2.70, Max: 2.78},
			{AnalysisID: "decision", Node: "OUT", Quantity: QuantityHysteresisVoltageV, Min: .44, Max: .52},
		},
	}
	opAmpParameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 30}, {Name: "supply_min_v", Value: 3},
	}
	components := []ComponentEvidence{
		{InstanceID: "supply", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VP"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "reference", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "REF"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "signal", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "IN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "feedback", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 9000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "THRESH"}}},
		{InstanceID: "reference_resistor", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "REF"}, {Function: "B", Net: "THRESH"}}},
		{InstanceID: "comparator", CatalogID: "opamp", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: opAmpParameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "THRESH"}, {Function: "IN_MINUS", Net: "IN"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "REF"}, {Name: "THRESH"}, {Name: "VP"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	if len(report.Analyses) != 1 || len(report.Analyses[0].Points) != 202 {
		t.Fatalf("DC sweep points = %#v", report.Analyses)
	}
	if report.Assertions[0].Actual == report.Assertions[1].Actual {
		t.Fatalf("threshold and hysteresis were not independently derived: %#v", report.Assertions)
	}
}

func TestMNAOpenCollectorComparatorUsesPullupAndCatalogSinkLimits(t *testing.T) {
	intent := Intent{
		ModelID: ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{
			{ID: "input_high", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "threshold", DCValue: 2.5}, {Component: "signal", DCValue: 3}}},
			{ID: "input_low", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "threshold", DCValue: 2.5}, {Component: "signal", DCValue: 2}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "input_high", Node: "OUT", Quantity: QuantityVoltageV, Min: 4.99, Max: 5.0},
			{AnalysisID: "input_low", Node: "OUT", Quantity: QuantityVoltageV, Min: .10, Max: .12},
		},
	}
	components := []ComponentEvidence{
		{InstanceID: "supply", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VP"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "threshold", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "THRESH"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "signal", CatalogID: "source.v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "IN"}, {Function: "NEGATIVE", Net: "GND"}}},
		{InstanceID: "pullup", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 10000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VP"}, {Function: "B", Net: "OUT"}}},
		{InstanceID: "comparator", CatalogID: "comparator", Family: "comparator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveComparatorOpenCollectorV1, Parameters: comparatorParameters(560e-9)}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "THRESH"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "THRESH"}, {Name: "VP"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func comparatorParameters(delayS float64) []NamedValue {
	return []NamedValue{
		{Name: "input_offset_v", Value: 0},
		{Name: "max_sink_current_a", Value: .02},
		{Name: "output_off_resistance_ohm", Value: 500e6},
		{Name: "output_on_resistance_ohm", Value: 225},
		{Name: "propagation_delay_s", Value: delayS},
		{Name: "quiescent_current_a", Value: 100e-6},
		{Name: "supply_max_v", Value: 36},
		{Name: "supply_min_v", Value: 2.2},
	}
}

func TestSameOpAmpClampsToleratesMinorRailSolveNoise(t *testing.T) {
	left := map[string]float64{"U1": 4.0000000001, "U2": -3.0000000001}
	right := map[string]float64{"U1": 4.0000000002, "U2": -3.0000000002}
	if !sameOpAmpClamps(left, right) {
		t.Fatal("minor rail solve noise should not prevent active-set convergence")
	}
	right["U1"] = 4.01
	if sameOpAmpClamps(left, right) {
		t.Fatal("material clamp movement must require another active-set iteration")
	}
}

func TestMNANoiseIntegratesTrustedResistorSpectrum(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "output_noise", Kind: AnalysisNoise, StartFrequencyHz: 1, StopFrequencyHz: 100000, Points: 64,
			Excitations: []SourceExcitation{{Component: "signal"}},
		}},
		Assertions: []Assertion{{AnalysisID: "output_noise", Node: "OUT", Quantity: QuantityIntegratedNoiseVRMS, Min: 5e-8, Max: 8e-8}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "IN", "GND"),
		resistorEvidence("resistor", 10000, "IN", "OUT"),
		{InstanceID: "capacitor", CatalogID: "capacitor", Family: "capacitor", HasValueSI: true, ValueSI: 1e-6, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("noise resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("noise report = %#v diagnostics=%#v", report, diagnostics)
	}
	if got := report.Assertions[0].Actual; got < 5e-8 || got > 8e-8 {
		t.Fatalf("integrated output noise = %.12g", got)
	}
}

func TestMNAStabilityDerivesPhaseAndGainMargins(t *testing.T) {
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 30}, {Name: "supply_min_v", Value: 3},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "IN", "GND"), voltageSourceEvidence("positive_supply", "VP", "GND"), voltageSourceEvidence("negative_supply", "VN", "GND"),
		{InstanceID: "opamp", CatalogID: "opamp", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "OUT"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VP"}, {Function: "V_MINUS", Net: "VN"}}},
	}
	intent := Intent{
		ModelID:  ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{ID: "loop_stability", Kind: AnalysisStability, StartFrequencyHz: 1, StopFrequencyHz: 1e8, Points: 64, Excitations: []SourceExcitation{{Component: "negative_supply"}, {Component: "positive_supply"}, {Component: "signal"}}}},
		Assertions: []Assertion{
			{AnalysisID: "loop_stability", Node: "OUT", Quantity: QuantityPhaseMarginDeg, Min: 85, Max: 95},
			{AnalysisID: "loop_stability", Node: "OUT", Quantity: QuantityGainMarginDB, Min: 250, Max: 300},
		},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VN"}, {Name: "VP"}})
	if len(diagnostics) != 0 {
		t.Fatalf("stability resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("stability report = %#v diagnostics=%#v", report, diagnostics)
	}
	if got := report.Assertions[1].Actual; got < 85 || got > 95 {
		t.Fatalf("phase margin = %.12g", got)
	}
}

func TestMNANoiseFailsClosedWithoutActiveDeviceNoiseEvidence(t *testing.T) {
	components := bufferedTwoPoleEvidence()
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "noise", Kind: AnalysisNoise, StartFrequencyHz: 10, StopFrequencyHz: 10000, Points: 8, Excitations: []SourceExcitation{{Component: "signal"}, {Component: "supply"}}}},
		Assertions: []Assertion{{AnalysisID: "noise", Node: "OUT", Quantity: QuantityIntegratedNoiseVRMS, Min: 0, Max: 1}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "5V"}, {Name: "VIN"}, {Name: "N1"}, {Name: "N2"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("noise resolution diagnostics = %#v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "catalog-backed op-amp") {
		t.Fatalf("noise evidence diagnostics = %#v", diagnostics)
	}
}

func TestMNAStartupBeginsFromZeroEnergyAndMeasuresPeak(t *testing.T) {
	intent := Intent{
		ModelID:    ModelTransientCircuitV1,
		Analyses:   []Analysis{{ID: "power_up", Kind: AnalysisStartup, DurationS: .005, TimeStepS: .0001, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}}},
		Assertions: []Assertion{{AnalysisID: "power_up", Node: "OUT", Quantity: QuantityPeakAbsVoltageV, Min: 4.9, Max: 5.01}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		resistorEvidence("resistor", 1000, "VIN", "OUT"),
		{InstanceID: "capacitor", CatalogID: "capacitor", Family: "capacitor", HasValueSI: true, ValueSI: 1e-6, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 10}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}, {Name: "VIN"}})
	if len(diagnostics) != 0 {
		t.Fatalf("startup resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("startup report = %#v diagnostics=%#v", report, diagnostics)
	}
	if got := report.Analyses[0].Points[0].Nodes; nodeReal(got, "VIN") != 0 || nodeReal(got, "OUT") != 0 {
		t.Fatalf("startup initial point is not zero energy: %#v", got)
	}
	if report.Analyses[0].Points[0].Solver == nil || report.Analyses[0].Points[0].Solver.InitialCondition != "all_dynamic_and_algebraic_unknowns_zero" {
		t.Fatalf("startup initial evidence = %#v", report.Analyses[0].Points[0].Solver)
	}
}

func TestMNADistortionExtractsDeterministicHarmonics(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "audio_thd", Kind: AnalysisDistortion, DurationS: .004, TimeStepS: 0.00003125,
			Excitations: []SourceExcitation{{Component: "signal", SineAmplitude: 1, SineFrequencyHz: 1000}},
		}},
		Assertions: []Assertion{{AnalysisID: "audio_thd", Node: "OUT", Quantity: QuantityTHDPercent, Min: 0, Max: 1e-8}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "OUT", "GND"),
		resistorEvidence("load", 1000, "OUT", "GND"),
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("distortion resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("distortion report = %#v diagnostics=%#v", report, diagnostics)
	}
	if report.Analyses[0].FundamentalFrequencyHz != 1000 || report.Assertions[0].Actual > 1e-8 {
		t.Fatalf("distortion evidence = %#v assertion=%#v", report.Analyses[0], report.Assertions[0])
	}
	replay, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 || replay.Assertions[0].Actual != report.Assertions[0].Actual {
		t.Fatalf("distortion replay = %#v diagnostics=%#v", replay, replayDiagnostics)
	}
}

func TestMNADistortionRejectsNonIntegralSineGrid(t *testing.T) {
	intent := Intent{
		ModelID:    ModelTransientCircuitV1,
		Analyses:   []Analysis{{ID: "thd", Kind: AnalysisDistortion, DurationS: .0041, TimeStepS: 0.00003125, Excitations: []SourceExcitation{{Component: "signal", SineAmplitude: 1, SineFrequencyHz: 1000}}}},
		Assertions: []Assertion{{AnalysisID: "thd", Node: "OUT", Quantity: QuantityTHDPercent, Min: 0, Max: 1}},
	}
	components := []ComponentEvidence{voltageSourceEvidence("signal", "OUT", "GND"), resistorEvidence("load", 1000, "OUT", "GND")}
	_, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "exact integer grid") {
		t.Fatalf("distortion grid diagnostics = %#v", diagnostics)
	}
}

func TestMNAThermalCouplesDissipationToCatalogThermalPath(t *testing.T) {
	intent := Intent{
		ModelID:  ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{ID: "hot", Kind: AnalysisThermal, Conditions: []NamedValue{{Name: "ambient_temperature_c", Value: 25}}, Excitations: []SourceExcitation{{Component: "supply", DCValue: 10}}}},
		Assertions: []Assertion{
			{AnalysisID: "hot", Component: "load", Quantity: QuantityDeviceDissipationW, Min: .999, Max: 1.001},
			{AnalysisID: "hot", Component: "load", Quantity: QuantityJunctionTemperatureC, Min: 74.9, Max: 75.1},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		{InstanceID: "load", CatalogID: "resistor.power", Family: "resistor", HasValueSI: true, ValueSI: 100, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1, Parameters: []NamedValue{{Name: "max_temperature_c", Value: 150}, {Name: "thermal_resistance_c_per_w", Value: 50}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VCC"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC"}})
	if len(diagnostics) != 0 {
		t.Fatalf("thermal resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("thermal report = %#v diagnostics=%#v", report, diagnostics)
	}
	if report.Assertions[0].Component != "load" || report.Assertions[1].Actual != 75 {
		t.Fatalf("thermal assertions = %#v", report.Assertions)
	}
}

func TestMNAThermalFailsClosedWithoutThermalPath(t *testing.T) {
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "hot", Kind: AnalysisThermal, Conditions: []NamedValue{{Name: "ambient_temperature_c", Value: 25}}, Excitations: []SourceExcitation{{Component: "supply", DCValue: 10}}}},
		Assertions: []Assertion{{AnalysisID: "hot", Component: "load", Quantity: QuantityJunctionTemperatureC, Min: 0, Max: 150}},
	}
	components := []ComponentEvidence{voltageSourceEvidence("supply", "VCC", "GND"), resistorEvidence("load", 100, "VCC", "GND")}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC"}})
	if len(diagnostics) != 0 {
		t.Fatalf("thermal resolution diagnostics = %#v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "thermal path") {
		t.Fatalf("thermal path diagnostics = %#v", diagnostics)
	}
}

func nodeReal(nodes []NodeResult, name string) float64 {
	for _, node := range nodes {
		if node.Node == name {
			return node.Real
		}
	}
	return math.NaN()
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
