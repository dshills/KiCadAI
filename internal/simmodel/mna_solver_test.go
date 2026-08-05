package simmodel

import (
	"encoding/json"
	"math"
	"slices"
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

	intent.Assertions[1].FrequencyHz = 500
	_, diagnostics = ResolveWithTopology(intent, "test", "catalog-hash", bufferedTwoPoleEvidence(), []NodeEvidence{
		{Name: "OUT", Role: "signal"}, {Name: "GND", Role: "ground", VoltageDomain: "0V"}, {Name: "N2", Role: "signal"},
		{Name: "VIN", Role: "signal"}, {Name: "5V", Role: "power_pos"}, {Name: "N1", Role: "signal"},
	})
	if !diagnosticsContain(diagnostics, "absent from the resolved sweep grid") {
		t.Fatalf("off-grid AC assertion diagnostics = %+v", diagnostics)
	}
}

func TestWaveformMeasurementsUseDifferentialReferenceNode(t *testing.T) {
	result := AnalysisResult{
		ID: "transient", Kind: AnalysisTransient,
		Points: []AnalysisPoint{
			{TimeS: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}, {Node: "LOCAL_GROUND", Real: 0}}},
			{TimeS: 1, Nodes: []NodeResult{{Node: "OUT", Real: 10.9}, {Node: "LOCAL_GROUND", Real: 10}}},
			{TimeS: 2, Nodes: []NodeResult{{Node: "OUT", Real: 1.8}, {Node: "LOCAL_GROUND", Real: 0}}},
		},
	}
	assertion := Assertion{AnalysisID: "transient", Node: "OUT", ReferenceNode: "LOCAL_GROUND", Quantity: QuantityRiseTimeS}
	_, values, diagnostic := waveform(result, assertion)
	if diagnostic != nil {
		t.Fatal(diagnostic.Message)
	}
	if len(values) != 3 || values[0] != 0 || math.Abs(values[1]-.9) > 1e-12 || values[2] != 1.8 {
		t.Fatalf("differential waveform = %#v", values)
	}
	actual, diagnostic := transientEdgeTime(result, assertion)
	if diagnostic != nil || math.Abs(actual-1.6) > 1e-12 {
		t.Fatalf("differential rise time = %.12g diagnostic=%#v", actual, diagnostic)
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

func TestMNAResolutionOmitsGraphNodesWithoutModeledDevices(t *testing.T) {
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: 3.3}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "VCC", Quantity: QuantityVoltageV, Min: 3.29, Max: 3.31}},
	}
	components := []ComponentEvidence{
		{InstanceID: "load", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VCC"}, {Function: "B", Net: "GND"}}},
		{InstanceID: "source", CatalogID: "v", Family: "voltage_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "VCC"}, {Function: "NEGATIVE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "FLOATING_GPIO"}, {Name: "GND", Role: "ground"}, {Name: "VCC"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	if slices.Contains(plan.Nodes, "FLOATING_GPIO") {
		t.Fatalf("unmodeled graph node retained in MNA plan: %+v", plan.Nodes)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestMNAStaticDigitalSupplyLoadUsesCanonicalSupplyAliases(t *testing.T) {
	intent := Intent{
		ModelID:  ModelNonlinearCircuitDCV1,
		Analyses: []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: 3.3}}}},
		Assertions: []Assertion{
			{AnalysisID: "operating_point", Node: "V3V3", Quantity: QuantityVoltageV, Min: 3.299, Max: 3.301},
			{AnalysisID: "operating_point", Component: "source", Quantity: QuantityDeviceCurrentA, Min: .013999, Max: .014001},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "V3V3", "GND"),
		{InstanceID: "pullup", CatalogID: "resistor.test", Family: "resistor", HasValueSI: true, ValueSI: 4700, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "V3V3"}, {Function: "B", Net: "SDA"}}},
		{
			InstanceID: "controller", CatalogID: "mcu.test", Family: "mcu",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveMCUStaticSupplyLoadV1, Parameters: []NamedValue{
				{Name: "minimum_supply_voltage_v", Value: 1.8}, {Name: "maximum_supply_voltage_v", Value: 5.5},
				{Name: "maximum_supply_current_a", Value: .014},
			}}},
			Connections: []ConnectionEvidence{
				{Function: "VCC", Net: "V3V3"}, {Function: "AVCC", Net: "V3V3"},
				{Function: "GND", Net: "GND"}, {Function: "AGND", Net: "GND"},
				{Function: "SDA", Net: "SDA"},
			},
		},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "V3V3", Role: "power"}, {Name: "SDA", Role: "signal"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	if got := terminalMap(plan.Devices[0]); got["POWER"] != "V3V3" || got["GROUND"] != "GND" {
		t.Fatalf("canonical load terminals = %+v", got)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestStaticSupplyLoadReleasesUnpoweredRailAndKeepsPoweredWorstCase(t *testing.T) {
	parameters := map[string]float64{
		"minimum_supply_voltage_v": 1.8,
		"maximum_supply_current_a": .014,
	}
	lowCurrent, lowConductance := staticSupplyLoadCurrentAndGradient(.18, parameters)
	if math.Abs(lowCurrent-.0014) > 1e-12 || math.Abs(lowConductance-.014/1.8) > 1e-12 {
		t.Fatalf("low-voltage load = %.12g A, %.12g S", lowCurrent, lowConductance)
	}
	poweredCurrent, poweredConductance := staticSupplyLoadCurrentAndGradient(3.3, parameters)
	if math.Abs(poweredCurrent-.014) > 1e-12 || poweredConductance != 0 {
		t.Fatalf("powered load = %.12g A, %.12g S", poweredCurrent, poweredConductance)
	}
}

func TestMNAStaticDigitalSupplyLoadRejectsMultipleSupplyDomains(t *testing.T) {
	intent := Intent{
		ModelID:    ModelNonlinearCircuitDCV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: 3.3}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "V3V3", Quantity: QuantityVoltageV, Min: 3.2, Max: 3.4}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "V3V3", "GND"),
		{
			InstanceID: "controller", CatalogID: "mcu.test", Family: "mcu",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveMCUStaticSupplyLoadV1, Parameters: []NamedValue{
				{Name: "minimum_supply_voltage_v", Value: 1.8}, {Name: "maximum_supply_voltage_v", Value: 5.5},
				{Name: "maximum_supply_current_a", Value: .014},
			}}},
			Connections: []ConnectionEvidence{
				{Function: "VCC", Net: "V3V3"}, {Function: "AVCC", Net: "AV3V3"}, {Function: "GND", Net: "GND"},
			},
		},
	}
	_, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "V3V3", Role: "power"}, {Name: "AV3V3", Role: "power"}})
	if len(diagnostics) != 1 || diagnostics[0].Path != "topology.devices.controller.terminals.POWER" {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestMNATotalSupplyCurrentSumsResolvedRailMagnitudes(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{
			{Component: "source_a", DCValue: 5}, {Component: "source_b", DCValue: 10},
		}}},
		Assertions: []Assertion{{
			AnalysisID: "operating_point", Components: []string{"source_a", "source_b"}, Quantity: QuantityTotalSupplyCurrentA, Min: .2999, Max: .3001,
		}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source_a", "V5", "GND"),
		voltageSourceEvidence("source_b", "V10", "GND"),
		resistorEvidence("load_a", 50, "V5", "GND"),
		resistorEvidence("load_b", 50, "V10", "GND"),
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "V5"}, {Name: "V10"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" || len(report.Assertions) != 1 || math.Abs(report.Assertions[0].Actual-.3) > 1e-9 {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestMNAFuseClosedStateUsesCatalogResistanceAndFailsAboveRatedCurrent(t *testing.T) {
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 1}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Component: "fuse", Quantity: QuantityDeviceCurrentA, Min: .096, Max: .097}},
	}
	fuseParameters := []NamedValue{
		{Name: "cold_resistance_ohm", Value: .38},
		{Name: "rated_current_a", Value: .5},
		{Name: "max_voltage_v", Value: 75},
		{Name: "nominal_melting_i2t_a2s", Value: .065},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		{InstanceID: "fuse", CatalogID: "fuse", Family: "fuse", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveFuseClosedStateV1, Parameters: fuseParameters}}, Connections: []ConnectionEvidence{{Function: "A", Net: "VIN"}, {Function: "B", Net: "OUT"}}},
		resistorEvidence("load", 10, "OUT", "GND"),
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("fuse resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("fuse report = %#v diagnostics=%#v", report, diagnostics)
	}

	components[1].ModelClaims[0].Parameters[1].Value = .05
	plan, diagnostics = ResolveWithTopology(intent, "catalog", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("limited fuse resolution diagnostics = %#v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if !diagnosticsContain(diagnostics, "fuse current") {
		t.Fatalf("fuse current limit diagnostics = %#v", diagnostics)
	}
}

func TestMNAFuseI2TClearingOpensAfterCatalogBackedFaultPulse(t *testing.T) {
	recovered := .5
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "fault", Kind: AnalysisTransient, DurationS: .003, TimeStepS: .0001,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: .5}},
			SourceValueEvents: []SourceValueEvent{{
				ID: "short_fault", Component: "supply", TriggerTimeS: .001, DurationS: .001,
				Initial: .5, Applied: 10, Recovered: &recovered,
			}},
		}},
		Assertions: []Assertion{{
			AnalysisID: "fault", Node: "OUT", Quantity: QuantityVoltageV, TimeS: .002,
			Min: -1e-6, Max: 1e-6,
		}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		{
			InstanceID: "fuse", CatalogID: "fuse", Family: "fuse",
			ModelClaims: []CatalogEvidence{{
				ModelID: PrimitiveFuseI2TClearingV1,
				Parameters: []NamedValue{
					{Name: "cold_resistance_ohm", Value: .1},
					{Name: "open_resistance_ohm", Value: 1e9},
					{Name: "rated_current_a", Value: 1},
					{Name: "max_voltage_v", Value: 75},
					{Name: "nominal_melting_i2t_a2s", Value: .01},
				},
			}},
			Connections: []ConnectionEvidence{{Function: "A", Net: "VIN"}, {Function: "B", Net: "OUT"}},
		},
		resistorEvidence("load", 1, "OUT", "GND"),
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("I2t fuse resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" || len(report.Assertions) != 1 {
		t.Fatalf("I2t fuse report = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestMNAFuseClosedStateContributesCatalogResistanceThermalNoise(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "noise", Kind: AnalysisNoise,
			StartFrequencyHz: 100, StopFrequencyHz: 1000, Points: 3,
			Excitations: []SourceExcitation{{Component: "supply"}},
		}},
		Assertions: []Assertion{{
			AnalysisID: "noise", Node: "OUT", Quantity: QuantityIntegratedNoiseVRMS,
			Min: 2e-9, Max: 3e-9,
		}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "SOURCE", "GND"),
		{
			InstanceID: "fuse", CatalogID: "fuse", Family: "fuse",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveFuseClosedStateV1, Parameters: []NamedValue{
				{Name: "cold_resistance_ohm", Value: .38},
				{Name: "rated_current_a", Value: .5},
				{Name: "max_voltage_v", Value: 75},
			}}},
			Connections: []ConnectionEvidence{{Function: "A", Net: "SOURCE"}, {Function: "B", Net: "OUT"}},
		},
		resistorEvidence("load", 1e9, "OUT", "GND"),
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "SOURCE"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("fuse noise resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("fuse noise report = %#v diagnostics=%#v", report, diagnostics)
	}
	if len(report.Analyses) != 1 || len(report.Analyses[0].Points) == 0 {
		t.Fatalf("fuse noise analysis = %#v", report.Analyses)
	}
	for _, point := range report.Analyses[0].Points {
		found := false
		for _, node := range point.Nodes {
			if node.Node == "OUT" {
				found = node.DominantNoiseSource == "fuse"
				break
			}
		}
		if !found {
			t.Fatalf("fuse noise point = %#v", point)
		}
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

func TestMNAZeroCapacitanceOverrideRepresentsAbsentLoad(t *testing.T) {
	zero := 0.0
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "unloaded", Kind: AnalysisACSweep, StartFrequencyHz: 1e6, StopFrequencyHz: 1e6, Points: 2,
			Excitations:     []SourceExcitation{{Component: "source", ACMagnitude: 1}},
			DeviceOverrides: []DeviceOverride{{Component: "load", ValueSI: &zero}},
		}},
		Assertions: []Assertion{{AnalysisID: "unloaded", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 1e6, Min: .999, Max: 1.001}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("source", "IN", "GND"),
		resistorEvidence("series", 1000, "IN", "OUT"),
		{InstanceID: "load", CatalogID: "capacitor", Family: "capacitor", HasValueSI: true, ValueSI: 1e-9, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("zero-capacitance resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("zero-capacitance report = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestMNAACLinearizesNonlinearDeviceAtDCOperatingPoint(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "response", Kind: AnalysisACSweep, StartFrequencyHz: 1000, StopFrequencyHz: 1000, Points: 2,
			Excitations: []SourceExcitation{{Component: "signal", DCValue: 1, ACMagnitude: 1}},
		}},
		Assertions: []Assertion{{AnalysisID: "response", Node: "OUT", Quantity: QuantityVoltageMagnitudeV, FrequencyHz: 1000, Min: .09, Max: .12}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "IN", "GND"),
		resistorEvidence("resistor", 1000, "IN", "OUT"),
		{InstanceID: "diode", CatalogID: "diode", Family: "diode", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDiodeShockleyV1, Parameters: diodeParameters(.02, 5)}}, Connections: []ConnectionEvidence{{Function: "ANODE", Net: "OUT"}, {Function: "CATHODE", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("small-signal nonlinear resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("small-signal nonlinear report = %#v diagnostics=%#v", report, diagnostics)
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

func TestTransientDerivedMeasurementsUseBoundedEventWindow(t *testing.T) {
	result := AnalysisResult{ID: "wave", Kind: AnalysisTransient, Points: []AnalysisPoint{
		{TimeS: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{TimeS: 1, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{TimeS: 2, Nodes: []NodeResult{{Node: "OUT", Real: 10}}},
		{TimeS: 3, Nodes: []NodeResult{{Node: "OUT", Real: 9.9}}},
		{TimeS: 4, Nodes: []NodeResult{{Node: "OUT", Real: 10}}},
		{TimeS: 5, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
	}}
	assertion := Assertion{
		AnalysisID: "wave", Node: "OUT", Quantity: QuantitySettlingTimeS,
		WindowStartS: 1, WindowEndS: 5,
	}
	actual, diagnostic := transientDerivedValue(result, assertion)
	if diagnostic != nil || actual < .99 || actual > 1.01 {
		t.Fatalf("windowed settling time = %.12g diagnostic=%#v", actual, diagnostic)
	}
}

func TestTransientResponseTimeUsesEventToResponseLatencyInBoundedWindow(t *testing.T) {
	result := AnalysisResult{ID: "event", Kind: AnalysisTransient, Points: []AnalysisPoint{
		{TimeS: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{TimeS: 1, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{TimeS: 1.2, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{TimeS: 1.4, Nodes: []NodeResult{{Node: "OUT", Real: 2}}},
		{TimeS: 1.6, Nodes: []NodeResult{{Node: "OUT", Real: 10}}},
		{TimeS: 2, Nodes: []NodeResult{{Node: "OUT", Real: 10}}},
	}}
	assertion := Assertion{
		AnalysisID: "event", Node: "OUT", Quantity: QuantityResponseTimeS,
		WindowStartS: 1, WindowEndS: 2.1,
	}
	actual, diagnostic := transientDerivedValue(result, assertion)
	if diagnostic != nil || math.Abs(actual-.3) > 1e-12 {
		t.Fatalf("event response latency = %.12g, want .3 diagnostic=%#v", actual, diagnostic)
	}
}

func TestTransientResponseTimeIgnoresOppositeDirectionPreResponse(t *testing.T) {
	result := AnalysisResult{ID: "event", Kind: AnalysisTransient, Points: []AnalysisPoint{
		{TimeS: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{TimeS: .1, Nodes: []NodeResult{{Node: "OUT", Real: -20}}},
		{TimeS: .4, Nodes: []NodeResult{{Node: "OUT", Real: -20}}},
		{TimeS: .5, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{TimeS: .6, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{TimeS: 1, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
	}}
	assertion := Assertion{
		AnalysisID: "event", Node: "OUT", Quantity: QuantityResponseTimeS,
		WindowStartS: 0, WindowEndS: 1.1, ResponseDirection: "rising",
	}
	actual, diagnostic := transientDerivedValue(result, assertion)
	if diagnostic != nil || math.Abs(actual-.51) > 1e-12 {
		t.Fatalf("directed event response latency = %.12g, want .51 diagnostic=%#v", actual, diagnostic)
	}
}

func TestTransientResponseTimeRejectsOppositeDirectionGlitch(t *testing.T) {
	result := AnalysisResult{ID: "event", Kind: AnalysisTransient, Points: []AnalysisPoint{
		{TimeS: 0, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{TimeS: .1, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{TimeS: .5, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{TimeS: 1, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
	}}
	assertion := Assertion{
		AnalysisID: "event", Node: "OUT", Quantity: QuantityResponseTimeS,
		WindowStartS: 0, WindowEndS: 1.1, ResponseDirection: "rising",
	}
	if actual, diagnostic := transientDerivedValue(result, assertion); diagnostic == nil {
		t.Fatalf("opposite-direction glitch measured as response %.12g", actual)
	}
}

func TestTransientResponseTimeRejectsPeriodicBaselineMotion(t *testing.T) {
	result := AnalysisResult{
		ID: "event", Kind: AnalysisTransient, FundamentalFrequencyHz: 1,
		Points: []AnalysisPoint{
			{TimeS: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
			{TimeS: .25, Nodes: []NodeResult{{Node: "OUT", Real: 1}}},
			{TimeS: .5, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
			{TimeS: .75, Nodes: []NodeResult{{Node: "OUT", Real: -1}}},
			{TimeS: 1, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
			{TimeS: 1.25, Nodes: []NodeResult{{Node: "OUT", Real: 1}}},
			{TimeS: 1.5, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
			{TimeS: 1.75, Nodes: []NodeResult{{Node: "OUT", Real: -1}}},
			{TimeS: 2, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
			{TimeS: 2.25, Nodes: []NodeResult{{Node: "OUT", Real: 1}}},
			{TimeS: 2.5, Nodes: []NodeResult{{Node: "OUT", Real: 2}}},
			{TimeS: 2.75, Nodes: []NodeResult{{Node: "OUT", Real: -1}}},
			{TimeS: 3, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		},
	}
	assertion := Assertion{
		AnalysisID: "event", Node: "OUT", Quantity: QuantityResponseTimeS,
		WindowStartS: 2, WindowEndS: 3.1,
	}
	actual, diagnostic := transientDerivedValue(result, assertion)
	if diagnostic != nil || math.Abs(actual-.275) > 1e-12 {
		t.Fatalf("periodic event response latency = %.12g, want .275 diagnostic=%#v", actual, diagnostic)
	}
}

func TestTransientDerivedEventStressAndEfficiencyMeasurements(t *testing.T) {
	result := AnalysisResult{ID: "event", Kind: AnalysisTransient, Points: []AnalysisPoint{
		{
			TimeS: 0,
			Nodes: []NodeResult{{Node: "OUT", Real: 0}},
			Devices: []DeviceResult{
				{Component: "load", VoltageV: 0, CurrentA: 0},
				{Component: "supply", VoltageV: 12, CurrentA: 0},
			},
		},
		{
			TimeS: 1,
			Nodes: []NodeResult{{Node: "OUT", Real: 11}},
			Devices: []DeviceResult{
				{Component: "load", VoltageV: 11, CurrentA: .5, CurrentMagnitudeA: .5},
				{Component: "supply", VoltageV: 12, CurrentA: -.5, CurrentMagnitudeA: .5},
			},
		},
		{
			TimeS: 2,
			Nodes: []NodeResult{{Node: "OUT", Real: 10}},
			Devices: []DeviceResult{
				{Component: "load", VoltageV: 10, CurrentA: .5, CurrentMagnitudeA: .5},
				{Component: "supply", VoltageV: 12, CurrentA: -.5, CurrentMagnitudeA: .5},
			},
		},
	}}
	for _, test := range []struct {
		assertion Assertion
		want      float64
	}{
		{Assertion{AnalysisID: "event", Node: "OUT", Quantity: QuantityOvershootVoltageV}, 1},
		{Assertion{AnalysisID: "event", Component: "load", Quantity: QuantityPeakAbsDeviceVoltageV}, 11},
		{Assertion{AnalysisID: "event", Component: "load", Quantity: QuantityPeakAbsDeviceCurrentA}, .5},
		{Assertion{AnalysisID: "event", Component: "load", Components: []string{"supply"}, Quantity: QuantityConversionEfficiencyPct}, 87.5},
	} {
		actual, diagnostic := transientDerivedValue(result, test.assertion)
		if diagnostic != nil || math.Abs(actual-test.want) > 1e-12 {
			t.Fatalf("%s = %.12g, want %.12g diagnostic=%#v", test.assertion.Quantity, actual, test.want, diagnostic)
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

func TestMNANominalResolutionRetainsReviewedUncertaintiesForLaterCornerPlanning(t *testing.T) {
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "source", DCValue: .001}}}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Node: "OUT", Quantity: QuantityVoltageV, Min: .79, Max: 1.22}},
	}
	components := []ComponentEvidence{
		{InstanceID: "load", CatalogID: "r", Family: "resistor", HasValueSI: true, ValueSI: 1000, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}, Uncertainties: []Uncertainty{{Target: "value_si", Source: "catalog:r:resistance_tolerance", Nominal: 1000, Minimum: 900, Maximum: 1100}}},
		{InstanceID: "source", CatalogID: "i", Family: "current_source", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentSourceV1}}, Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "GND"}, {Function: "NEGATIVE", Net: "OUT"}}, Uncertainties: []Uncertainty{{Target: "excitation_dc_value", Source: "reviewed-system-supply", Nominal: .001, Minimum: .0009, Maximum: .0011}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT", Role: "signal"}})
	if len(diagnostics) != 0 || plan.WorstCase || len(plan.Uncertainties) != 2 {
		t.Fatalf("nominal plan did not retain reviewed uncertainties: plan=%#v diagnostics=%#v", plan, diagnostics)
	}
	plan.WorstCase = true
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" || len(report.Corners) != 9 {
		t.Fatalf("later worst-case evaluation = %#v diagnostics=%#v", report, diagnostics)
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

func TestApplyDeviceOverrideDoesNotMutateResolvedTopology(t *testing.T) {
	device := ResolvedDevice{ModelParameters: []NamedValue{
		{Name: "forward_beta", Value: 100},
		{Name: "junction_temperature_k", Value: 298.15},
	}}
	before, err := json.Marshal(device)
	if err != nil {
		t.Fatal(err)
	}
	overridden := applyDeviceOverride(device, DeviceOverride{ModelParameters: []NamedValue{{Name: "junction_temperature_k", Value: 323.15}}})
	after, err := json.Marshal(device)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("device override mutated resolved topology: before=%s after=%s", before, after)
	}
	if len(overridden.ModelParameters) != 2 || overridden.ModelParameters[1].Name != "junction_temperature_k" || overridden.ModelParameters[1].Value != 323.15 {
		t.Fatalf("device override = %#v", overridden.ModelParameters)
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

func TestSmallSignalReferenceOnlyAnchorsDisconnectedUnobservedComponent(t *testing.T) {
	system := mnaSystem{
		matrix: [][]complex128{
			{1, 0, 0},
			{0, 1, -1},
			{0, -1, 1},
		},
		rhs:           []complex128{1, 0, 0},
		unknownLabels: []string{"node:OUT", "node:FLY_A", "node:FLY_B"},
		nodeIndex:     map[string]int{"OUT": 0, "FLY_A": 1, "FLY_B": 2},
		branchIndex:   map[string]int{},
	}
	plan := Plan{Assertions: []Assertion{{AnalysisID: "noise", Node: "OUT", Quantity: QuantityIntegratedNoiseVRMS}}}
	analysis := Analysis{ID: "noise", Kind: AnalysisNoise}
	referenceUnobservedMNAComponents(plan, analysis, &system)
	if system.matrix[0][0] != 1 {
		t.Fatalf("observable graph was loaded: %#v", system.matrix)
	}
	if system.matrix[1][1] != 1+complex(mnaUnobservedReferenceS, 0) {
		t.Fatalf("disconnected graph was not deterministically referenced: %#v", system.matrix)
	}
	solution, diagnostic := solveMNA(system)
	if diagnostic != nil || real(solution[0]) != 1 {
		t.Fatalf("solution=%#v diagnostic=%#v", solution, diagnostic)
	}
}

func TestMNASolverAcceptsConditionedSystemWithBoundedResidual(t *testing.T) {
	const separation = 1e-13
	system := mnaSystem{
		matrix: [][]complex128{
			{1, 1},
			{1, 1 + separation},
		},
		rhs:           []complex128{2, 2 + separation},
		unknownLabels: []string{"node:A", "branch_current:source"},
		nodeIndex:     map[string]int{"A": 0},
		branchIndex:   map[string]int{"source": 1},
	}
	solution, diagnostic := solveMNA(system)
	if diagnostic != nil {
		t.Fatalf("conditioned system diagnostic = %#v", diagnostic)
	}
	for index, value := range solution {
		if math.Abs(real(value)-1) > 1e-3 || math.Abs(imag(value)) > 1e-12 {
			t.Fatalf("solution[%d] = %v; want approximately 1", index, value)
		}
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

func TestMNADCSweepAppliesExcitationScaleButReportsSemanticValue(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "decision", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "signal"}},
			DCSweep:     &DCSweep{Component: "signal", StartValue: 1, StopValue: 2, Points: 3, ExcitationScale: -1},
		}},
		Assertions: []Assertion{{AnalysisID: "decision", Node: "IN", Quantity: QuantityVoltageV, Min: 1, Max: 2}},
	}
	components := []ComponentEvidence{{
		InstanceID: "signal", CatalogID: "connector", Family: "connector",
		ModelClaims: []CatalogEvidence{{ModelID: PrimitiveConnectorVoltageSourceV1}},
		Connections: []ConnectionEvidence{{Function: "PIN_1", Net: "GND"}, {Function: "PIN_2", Net: "IN"}},
	}}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 {
		t.Fatalf("evaluate diagnostics = %+v", diagnostics)
	}
	if got := report.Analyses[0].Points[2].SweepValue; got != 2 {
		t.Fatalf("reported semantic sweep value = %g; want 2", got)
	}
	if got := nodeReal(report.Analyses[0].Points[2].Nodes, "IN"); got != 2 {
		t.Fatalf("scaled physical input voltage = %g; want 2", got)
	}
}

func TestMNADCSweepWithoutTransitionProducesCensoredFailedAssertion(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "decision", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "signal"}},
			DCSweep:     &DCSweep{Component: "signal", StartValue: 0, StopValue: 3, Points: 5},
		}},
		Assertions: []Assertion{{AnalysisID: "decision", Node: "OUT", Quantity: QuantityThresholdVoltageV, Min: 1, Max: 2}},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "OUT", "GND"),
		voltageSourceEvidence("signal", "IN", "GND"),
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 1 || len(report.Assertions) != 1 || report.Assertions[0].Pass || report.Assertions[0].Actual != 0 {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
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

func TestOpenCollectorComparatorRemainsHighImpedanceBelowMinimumSupply(t *testing.T) {
	device := ResolvedDevice{
		PrimitiveModel: PrimitiveComparatorOpenCollectorV1,
		ModelParameters: []NamedValue{
			{Name: "input_offset_v", Value: .005},
			{Name: "supply_min_v", Value: 2.2},
		},
		Terminals: []TerminalBinding{
			{Terminal: "IN_PLUS", Net: "PLUS"},
			{Terminal: "IN_MINUS", Net: "MINUS"},
			{Terminal: "V_PLUS", Net: "VP"},
			{Terminal: "V_MINUS", Net: "GND"},
		},
	}
	system := mnaSystem{nodeIndex: map[string]int{"PLUS": 0, "MINUS": 1, "VP": 2}}
	if comparatorOn(device, system, []complex128{0, 1, 0}) {
		t.Fatal("unpowered open-collector comparator unexpectedly sinks")
	}
	if !comparatorOn(device, system, []complex128{0, 1, 5}) {
		t.Fatal("powered open-collector comparator did not follow its differential input")
	}
}

func TestComparatorOnRejectsIncompleteResolvedContract(t *testing.T) {
	if comparatorOn(ResolvedDevice{}, mnaSystem{}, nil) {
		t.Fatal("incomplete comparator contract was treated as an active sink")
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

func TestMNAAdjustableRegulatorUsesFeedbackAndEnforcesCatalogLimits(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{
			{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}},
			{ID: "thermal", Kind: AnalysisThermal, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}, Conditions: []NamedValue{{Name: "ambient_temperature_c", Value: 70}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "operating_point", Node: "VOUT", Quantity: QuantityVoltageV, Min: 3.327, Max: 3.329},
			{AnalysisID: "thermal", Component: "regulator", Quantity: QuantityJunctionTemperatureC, Min: 80, Max: 90},
		},
	}
	components := adjustableRegulatorEvidence(100)
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "power_pos"}, {Name: "VOUT", Role: "power_pos"}, {Name: "ADJ"}})
	if len(diagnostics) != 0 {
		t.Fatalf("regulator resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("regulator report = %#v diagnostics=%#v", report, diagnostics)
	}

	lowHeadroom := ClonePlan(plan)
	for analysisIndex := range lowHeadroom.Analyses {
		lowHeadroom.Analyses[analysisIndex].Excitations[0].DCValue = 3.5
	}
	if _, diagnostics = Evaluate(lowHeadroom); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "headroom") {
		t.Fatalf("low-headroom regulator was not rejected: %#v", diagnostics)
	}

	overloadedIntent := intent
	overloadedIntent.Analyses = []Analysis{{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}}}
	overloadedIntent.Assertions = []Assertion{{AnalysisID: "operating_point", Node: "VOUT", Quantity: QuantityVoltageV, Min: 3, Max: 3.5}}
	overloaded, diagnostics := ResolveWithTopology(overloadedIntent, "catalog", "catalog-hash", adjustableRegulatorEvidence(5), []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "power_pos"}, {Name: "VOUT", Role: "power_pos"}, {Name: "ADJ"}})
	if len(diagnostics) != 0 {
		t.Fatalf("overload plan resolution diagnostics = %#v", diagnostics)
	}
	if _, diagnostics = Evaluate(overloaded); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "load current") {
		t.Fatalf("overloaded regulator was not rejected: %#v", diagnostics)
	}
}

func TestMNAFloatingAdjustableRegulatorUsesExternalFeedbackForBothPolarities(t *testing.T) {
	for _, test := range []struct {
		name       string
		polarity   float64
		sourceFrom string
		sourceTo   string
		outputMin  float64
		outputMax  float64
	}{
		{name: "positive", polarity: 1, sourceFrom: "VIN", sourceTo: "GND", outputMin: 5.035, outputMax: 5.037},
		{name: "negative", polarity: -1, sourceFrom: "GND", sourceTo: "VIN", outputMin: -5.037, outputMax: -5.035},
	} {
		t.Run(test.name, func(t *testing.T) {
			parameters := []NamedValue{
				{Name: "reference_voltage_v", Value: 1.25}, {Name: "polarity", Value: test.polarity},
				{Name: "min_headroom_v", Value: 3}, {Name: "max_load_current_a", Value: 1.5},
				{Name: "max_input_output_voltage_v", Value: 40}, {Name: "adjustment_pin_current_a", Value: 50e-6},
				{Name: "soft_start_time_s", Value: 0},
			}
			components := []ComponentEvidence{
				voltageSourceEvidence("supply", test.sourceFrom, test.sourceTo),
				{InstanceID: "regulator", CatalogID: "regulator", Family: "regulator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveFloatingAdjustableRegulatorV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "VIN", Net: "VIN"}, {Function: "VOUT", Net: "VOUT"}, {Function: "ADJ", Net: "ADJ"}}},
				resistorEvidence("feedback_reference", 240, "VOUT", "ADJ"),
				resistorEvidence("feedback_ground", 720, "ADJ", "GND"),
				resistorEvidence("load", 1000, "VOUT", "GND"),
			}
			intent := Intent{
				ModelID:    ModelLinearCircuitMNAV1,
				Analyses:   []Analysis{{ID: "dc", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 12}}}},
				Assertions: []Assertion{{AnalysisID: "dc", Node: "VOUT", Quantity: QuantityVoltageV, Min: test.outputMin, Max: test.outputMax}},
			}
			plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "power"}, {Name: "VOUT", Role: "power"}, {Name: "ADJ"}})
			if len(diagnostics) != 0 {
				t.Fatalf("resolve floating regulator: %#v", diagnostics)
			}
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("floating regulator report = %#v diagnostics=%#v", report, diagnostics)
			}
		})
	}
}

func TestMNADualOutputIsolatedConverterDrivesBothRailsAndEnforcesCatalogLimits(t *testing.T) {
	parameters := []NamedValue{
		{Name: "input_min_v", Value: 9}, {Name: "input_max_v", Value: 18},
		{Name: "positive_output_voltage_v", Value: 12}, {Name: "negative_output_voltage_v", Value: 12},
		{Name: "positive_max_output_current_a", Value: .5}, {Name: "negative_max_output_current_a", Value: .5},
		{Name: "soft_start_time_s", Value: 0},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		{InstanceID: "converter", CatalogID: "converter", Family: "isolated_converter", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveDualOutputIsolatedConverterV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "VIN_PLUS", Net: "VIN"}, {Function: "VIN_MINUS", Net: "GND"}, {Function: "COMMON", Net: "GND"}, {Function: "VOUT_PLUS", Net: "VP"}, {Function: "VOUT_MINUS", Net: "VN"}}},
		resistorEvidence("positive_load", 120, "VP", "GND"),
		resistorEvidence("negative_load", 120, "VN", "GND"),
	}
	intent := Intent{
		ModelID:  ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{ID: "dc", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 12}}}},
		Assertions: []Assertion{
			{AnalysisID: "dc", Node: "VP", Quantity: QuantityVoltageV, Min: 11.999, Max: 12.001},
			{AnalysisID: "dc", Node: "VN", Quantity: QuantityVoltageV, Min: -12.001, Max: -11.999},
		},
	}
	nodes := []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "power"}, {Name: "VP", Role: "power"}, {Name: "VN", Role: "power"}}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve dual-output converter: %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("dual-output converter report = %#v diagnostics=%#v", report, diagnostics)
	}

	inputOutOfRange := ClonePlan(plan)
	inputOutOfRange.Analyses[0].Excitations[0].DCValue = 8
	if _, diagnostics = Evaluate(inputOutOfRange); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "input") {
		t.Fatalf("out-of-range converter input was not rejected: %#v", diagnostics)
	}

	overloadedComponents := append([]ComponentEvidence(nil), components...)
	overloadedComponents[2] = resistorEvidence("positive_load", 12, "VP", "GND")
	overloaded, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", overloadedComponents, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve overloaded converter: %#v", diagnostics)
	}
	if _, diagnostics = Evaluate(overloaded); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "positive output current") {
		t.Fatalf("overloaded converter output was not rejected: %#v", diagnostics)
	}
}

func TestMNAIsolatedConverterUsesIndependentSecondaryReferenceGauge(t *testing.T) {
	parameters := []NamedValue{
		{Name: "input_min_v", Value: 9}, {Name: "input_max_v", Value: 18},
		{Name: "output_voltage_v", Value: 12}, {Name: "max_output_current_a", Value: .5},
		{Name: "soft_start_time_s", Value: 0},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "PRIMARY_GROUND"),
		{
			InstanceID: "converter", CatalogID: "converter", Family: "isolated_converter",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveSingleOutputIsolatedConverterV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "VIN_PLUS", Net: "VIN"}, {Function: "VIN_MINUS", Net: "PRIMARY_GROUND"},
				{Function: "VOUT_PLUS", Net: "VOUT"}, {Function: "VOUT_MINUS", Net: "SECONDARY_GROUND"},
			},
		},
		resistorEvidence("load", 120, "VOUT", "SECONDARY_GROUND"),
	}
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "dc", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 12}},
		}},
		Assertions: []Assertion{{
			AnalysisID: "dc", Node: "VOUT", Quantity: QuantityVoltageV, Min: 11.999, Max: 12.001,
		}},
	}
	nodes := []NodeEvidence{
		{Name: "PRIMARY_GROUND", Role: "ground"},
		{Name: "SECONDARY_GROUND", Role: "ground"},
		{Name: "VIN", Role: "power"},
		{Name: "VOUT", Role: "power"},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve independently referenced isolated converter: %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("independently referenced isolated converter report = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestMNACurrentSenseAmplifierClampsToCatalogOutputRange(t *testing.T) {
	for _, test := range []struct {
		name      string
		inputV    float64
		outputMin float64
		outputMax float64
	}{
		{name: "low", inputV: 0, outputMin: .019999, outputMax: .020001},
		{name: "high", inputV: .1, outputMin: 4.799, outputMax: 4.801},
	} {
		t.Run(test.name, func(t *testing.T) {
			parameters := []NamedValue{
				{Name: "gain_v_per_v", Value: 100}, {Name: "bandwidth_hz", Value: 400000},
				{Name: "input_offset_voltage_v", Value: 0}, {Name: "supply_min_v", Value: 2.7},
				{Name: "supply_max_v", Value: 5.5}, {Name: "common_mode_min_v", Value: -4},
				{Name: "common_mode_max_v", Value: 80}, {Name: "output_low_margin_v", Value: .02},
				{Name: "output_high_margin_v", Value: .2}, {Name: "quiescent_current_a", Value: .0024},
			}
			components := []ComponentEvidence{
				voltageSourceEvidence("supply", "VCC", "GND"),
				voltageSourceEvidence("input", "SENSEP", "GND"),
				{InstanceID: "sensor", CatalogID: "sensor", Family: "current_sensor", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentSenseAmplifierV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "SENSEP"}, {Function: "IN_MINUS", Net: "GND"}, {Function: "REF1", Net: "GND"}, {Function: "REF2", Net: "GND"}, {Function: "OUT", Net: "OUT"}, {Function: "VCC", Net: "VCC"}, {Function: "GND_A", Net: "GND"}, {Function: "GND_B", Net: "GND"}}},
				resistorEvidence("load", 10000, "OUT", "GND"),
			}
			intent := Intent{
				ModelID:    ModelLinearCircuitMNAV1,
				Analyses:   []Analysis{{ID: "dc", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}, {Component: "input", DCValue: test.inputV}}}},
				Assertions: []Assertion{{AnalysisID: "dc", Node: "OUT", Quantity: QuantityVoltageV, Min: test.outputMin, Max: test.outputMax}},
			}
			plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC", Role: "power"}, {Name: "SENSEP"}, {Name: "OUT"}})
			if len(diagnostics) != 0 {
				t.Fatalf("resolve current-sense amplifier: %#v", diagnostics)
			}
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("current-sense report = %#v diagnostics=%#v", report, diagnostics)
			}
		})
	}
}

func TestMNACurrentSenseAmplifierDefaultsAbsentReferencesToPrimaryGround(t *testing.T) {
	parameters := []NamedValue{
		{Name: "gain_v_per_v", Value: 10}, {Name: "bandwidth_hz", Value: 80000},
		{Name: "input_offset_voltage_v", Value: 0}, {Name: "supply_min_v", Value: 2.7},
		{Name: "supply_max_v", Value: 60}, {Name: "common_mode_min_v", Value: 2.7},
		{Name: "common_mode_max_v", Value: 60}, {Name: "output_low_margin_v", Value: 0},
		{Name: "output_high_margin_v", Value: 1}, {Name: "quiescent_current_a", Value: .00006},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VCC", "GND"),
		voltageSourceEvidence("input_plus", "SENSEP", "GND"),
		voltageSourceEvidence("input_minus", "SENSEM", "GND"),
		{InstanceID: "sensor", CatalogID: "sensor", Family: "current_sensor", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCurrentSenseAmplifierV1, Parameters: parameters}}, Connections: []ConnectionEvidence{
			{Function: "IN_PLUS", Net: "SENSEP"}, {Function: "IN_MINUS", Net: "SENSEM"},
			{Function: "OUT", Net: "OUT"}, {Function: "VCC", Net: "VCC"}, {Function: "GND_A", Net: "GND"},
		}},
		resistorEvidence("load", 50000, "OUT", "GND"),
	}
	intent := Intent{
		ModelID:    ModelLinearCircuitMNAV1,
		Analyses:   []Analysis{{ID: "dc", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 12}, {Component: "input_plus", DCValue: 12.01}, {Component: "input_minus", DCValue: 12}}}},
		Assertions: []Assertion{{AnalysisID: "dc", Node: "OUT", Quantity: QuantityVoltageV, Min: .099, Max: .101}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VCC", Role: "power"}, {Name: "SENSEP"}, {Name: "SENSEM"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve ground-referenced current-sense amplifier: %#v", diagnostics)
	}
	sensor, found := resolvedDeviceByComponent(plan.Devices, "sensor")
	if !found {
		t.Fatalf("resolved plan omitted current sensor: %#v", plan.Devices)
	}
	terminals := terminalMap(sensor)
	for _, terminal := range []string{"REF1", "REF2", "GND_B"} {
		if terminals[terminal] != "GND" {
			t.Fatalf("%s default net = %q, want GND: %#v", terminal, terminals[terminal], sensor.Terminals)
		}
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("current-sense report = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestCurrentSenseOperatingLimitToleratesNumericalNoiseAtOutputBoundary(t *testing.T) {
	parameters := []NamedValue{
		{Name: "supply_min_v", Value: 2.7}, {Name: "supply_max_v", Value: 5.5},
		{Name: "common_mode_min_v", Value: -4}, {Name: "common_mode_max_v", Value: 80},
		{Name: "output_low_margin_v", Value: .02}, {Name: "output_high_margin_v", Value: .2},
	}
	plan := Plan{Devices: []ResolvedDevice{{
		Component: "sensor", PrimitiveModel: PrimitiveCurrentSenseAmplifierV1, ModelParameters: parameters,
		Terminals: []TerminalBinding{
			{Terminal: "IN_PLUS", Net: "SENSEP"}, {Terminal: "IN_MINUS", Net: "SENSEM"},
			{Terminal: "OUT", Net: "OUT"}, {Terminal: "VCC", Net: "VCC"},
			{Terminal: "GND_A", Net: "GND"}, {Terminal: "GND_B", Net: "GND"},
		},
	}}}
	system := mnaSystem{nodeIndex: map[string]int{"VCC": 0, "SENSEP": 1, "SENSEM": 2, "OUT": 3}}
	solution := []complex128{5, 0, 0, complex(.02-1e-9, 0)}
	if diagnostics := validateResolvedOperatingLimits(plan, system, solution, false); len(diagnostics) != 0 {
		t.Fatalf("boundary solve noise was rejected: %#v", diagnostics)
	}
	solution[3] = complex(.02-1e-4, 0)
	if diagnostics := validateResolvedOperatingLimits(plan, system, solution, false); len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Path, ".output") {
		t.Fatalf("material output-range violation was not rejected: %#v", diagnostics)
	}
}

func TestMNATransimpedanceUsesDCSweepTransferSlope(t *testing.T) {
	result := AnalysisResult{Kind: AnalysisDCOperatingPoint, Points: []AnalysisPoint{
		{Sweep: dcSweepForward, SweepValue: 0, Nodes: []NodeResult{{Node: "OUT", Real: .02}}, Devices: []DeviceResult{{Component: "load", CurrentMagnitudeA: 0}}},
		{Sweep: dcSweepForward, SweepValue: 1, Nodes: []NodeResult{{Node: "OUT", Real: 1.01}}, Devices: []DeviceResult{{Component: "load", CurrentMagnitudeA: 1}}},
		{Sweep: dcSweepForward, SweepValue: 3, Nodes: []NodeResult{{Node: "OUT", Real: 3.01}}, Devices: []DeviceResult{{Component: "load", CurrentMagnitudeA: 3}}},
	}}
	actual, diagnostic := dcDeviceValue(result, Assertion{Node: "OUT", Component: "load", Quantity: QuantityTransimpedanceOhm})
	if diagnostic != nil || math.Abs(actual-.996666666667) > 1e-12 {
		t.Fatalf("sweep transimpedance = %.12g diagnostic=%#v", actual, diagnostic)
	}
}

func TestMNAAdjustableRegulatorStartupUsesCatalogSoftStart(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "startup", Kind: AnalysisStartup, DurationS: 100e-6, TimeStepS: 10e-6,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "startup", Node: "VOUT", Quantity: QuantityPeakAbsVoltageV, Min: 3.327, Max: 3.329}},
	}
	components := adjustableRegulatorEvidence(100)
	components = append(components, ComponentEvidence{
		InstanceID: "output_capacitor", CatalogID: "c", Family: "capacitor", HasValueSI: true, ValueSI: 1e-6,
		ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 6.3}}}},
		Connections: []ConnectionEvidence{{Function: "A", Net: "VOUT"}, {Function: "B", Net: "GND"}},
	})
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "power_pos"}, {Name: "VOUT", Role: "power_pos"}, {Name: "ADJ"}})
	if len(diagnostics) != 0 {
		t.Fatalf("startup regulator resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("startup regulator report = %#v diagnostics=%#v", report, diagnostics)
	}
	if got := nodeReal(report.Analyses[0].Points[2].Nodes, "VOUT"); got < 1.32 || got > 1.34 {
		t.Fatalf("VOUT at 20us = %.12g, want catalog 50us soft-start ramp", got)
	}
}

func TestMNAAdjustableRegulatorTransientStartsFromBiasedOperatingPoint(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "response", Kind: AnalysisTransient, DurationS: 100e-6, TimeStepS: 10e-6,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "response", Node: "VOUT", Quantity: QuantityVoltageV, TimeS: 10e-6, Min: 3.327, Max: 3.329}},
	}
	components := adjustableRegulatorEvidence(100)
	components = append(components, ComponentEvidence{
		InstanceID: "output_capacitor", CatalogID: "c", Family: "capacitor", HasValueSI: true, ValueSI: 1e-6,
		ModelClaims: []CatalogEvidence{{ModelID: PrimitiveCapacitorTransientV1, Parameters: []NamedValue{{Name: "max_voltage_v", Value: 6.3}}}},
		Connections: []ConnectionEvidence{{Function: "A", Net: "VOUT"}, {Function: "B", Net: "GND"}},
	})
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "power_pos"}, {Name: "VOUT", Role: "power_pos"}, {Name: "ADJ"}})
	if len(diagnostics) != 0 {
		t.Fatalf("transient regulator resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("transient regulator report = %#v diagnostics=%#v", report, diagnostics)
	}
	if got := nodeReal(report.Analyses[0].Points[1].Nodes, "VOUT"); got < 3.327 || got > 3.329 {
		t.Fatalf("VOUT at first transient step = %.12g, want prebiased steady-state output", got)
	}
}

func TestMNAFixedRegulatorEnforcesOutputAndThermalPath(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{
			{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}},
			{ID: "thermal", Kind: AnalysisThermal, Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}}, Conditions: []NamedValue{{Name: "ambient_temperature_c", Value: 70}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "operating_point", Node: "VOUT", Quantity: QuantityVoltageV, Min: 3.299, Max: 3.301},
			{AnalysisID: "thermal", Component: "regulator", Quantity: QuantityJunctionTemperatureC, Min: 95, Max: 96},
		},
	}
	parameters := []NamedValue{
		{Name: "output_voltage_v", Value: 3.3}, {Name: "min_headroom_v", Value: .3},
		{Name: "max_load_current_a", Value: .3}, {Name: "min_input_voltage_v", Value: 2.5},
		{Name: "max_input_voltage_v", Value: 6}, {Name: "quiescent_current_a", Value: 90e-6},
		{Name: "soft_start_time_s", Value: 50e-6}, {Name: "max_temperature_c", Value: 125},
		{Name: "junction_to_ambient_c_per_w", Value: 100},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		{InstanceID: "regulator", CatalogID: "regulator", Family: "regulator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveFixedLinearRegulatorV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "VIN", Net: "VIN"}, {Function: "VOUT", Net: "VOUT"}, {Function: "GND", Net: "GND"}}},
		resistorEvidence("load", 22, "VOUT", "GND"),
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "power_pos"}, {Name: "VOUT", Role: "power_pos"}})
	if len(diagnostics) != 0 {
		t.Fatalf("fixed regulator resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("fixed regulator report = %#v diagnostics=%#v", report, diagnostics)
	}
}

func TestMNAFixedBuckModulePowersProtectedIsolatedConverter(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{
			{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Excitations: []SourceExcitation{{Component: "supply", DCValue: 24}}},
			{ID: "thermal", Kind: AnalysisThermal, Excitations: []SourceExcitation{{Component: "supply", DCValue: 24}}, Conditions: []NamedValue{{Name: "ambient_temperature_c", Value: 70}}},
		},
		Assertions: []Assertion{
			{AnalysisID: "operating_point", Node: "VOUT", ReferenceNode: "SECONDARY_GROUND", Quantity: QuantityVoltageV, Min: 11.999, Max: 12.001},
			{AnalysisID: "operating_point", Components: []string{"supply"}, Quantity: QuantityTotalSupplyCurrentA, Min: .1529, Max: .1531},
			{AnalysisID: "thermal", Component: "pre_regulator", Quantity: QuantityJunctionTemperatureC, Min: 77.34, Max: 77.35},
			{AnalysisID: "thermal", Component: "isolated_converter", Quantity: QuantityJunctionTemperatureC, Min: 82.01, Max: 82.02},
		},
	}
	buckParameters := []NamedValue{
		{Name: "input_min_v", Value: 15}, {Name: "input_max_v", Value: 36},
		{Name: "input_current_reference_voltage_v", Value: 24},
		{Name: "output_voltage_v", Value: 12}, {Name: "max_output_current_a", Value: 1},
		{Name: "conversion_efficiency_fraction", Value: .95}, {Name: "soft_start_time_s", Value: 0},
		{Name: "max_temperature_c", Value: 85}, {Name: "junction_to_ambient_c_per_w", Value: 40},
	}
	converterParameters := []NamedValue{
		{Name: "input_min_v", Value: 9}, {Name: "input_max_v", Value: 18},
		{Name: "input_current_reference_voltage_v", Value: 12},
		{Name: "output_voltage_v", Value: 12}, {Name: "max_output_current_a", Value: .833},
		{Name: "short_circuit_current_a", Value: 1.5827}, {Name: "soft_start_time_s", Value: .06},
		{Name: "maximum_overshoot_ratio", Value: .05}, {Name: "efficiency_ratio", Value: .86},
		{Name: "isolation_working_voltage_v", Value: 1000}, {Name: "isolation_resistance_ohm", Value: 1e9},
		{Name: "max_temperature_c", Value: 105}, {Name: "junction_to_ambient_c_per_w", Value: 24.6},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "PRIMARY_GROUND"),
		{
			InstanceID: "pre_regulator", CatalogID: "pre_regulator", Family: "regulator",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveFixedBuckModuleV1, Parameters: buckParameters}},
			Connections: []ConnectionEvidence{{Function: "VIN", Net: "VIN"}, {Function: "VOUT", Net: "INTERMEDIATE"}, {Function: "GND", Net: "PRIMARY_GROUND"}},
		},
		{
			InstanceID: "isolated_converter", CatalogID: "isolated_converter", Family: "isolated_converter",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveProtectedIsolatedConverterV1, Parameters: converterParameters}},
			Connections: []ConnectionEvidence{
				{Function: "VIN_PLUS", Net: "INTERMEDIATE"}, {Function: "VIN_MINUS", Net: "PRIMARY_GROUND"},
				{Function: "VOUT_PLUS", Net: "VOUT"}, {Function: "VOUT_MINUS", Net: "SECONDARY_GROUND"},
			},
		},
		resistorEvidence("load", 48, "VOUT", "SECONDARY_GROUND"),
	}
	nodes := []NodeEvidence{
		{Name: "PRIMARY_GROUND", Role: "ground"}, {Name: "VIN", Role: "power_pos"},
		{Name: "INTERMEDIATE", Role: "power_pos"}, {Name: "VOUT", Role: "power_pos"},
		{Name: "SECONDARY_GROUND", Role: "isolated_ground"},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("fixed buck module resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("fixed buck module report = %#v diagnostics=%#v", report, diagnostics)
	}
}

func adjustableRegulatorEvidence(loadResistance float64) []ComponentEvidence {
	parameters := []NamedValue{
		{Name: "reference_voltage_v", Value: .8},
		{Name: "min_headroom_v", Value: .3},
		{Name: "max_load_current_a", Value: .3},
		{Name: "min_input_voltage_v", Value: 2.5},
		{Name: "max_input_voltage_v", Value: 6},
		{Name: "quiescent_current_a", Value: 90e-6},
		{Name: "soft_start_time_s", Value: 50e-6},
		{Name: "max_temperature_c", Value: 125},
		{Name: "junction_to_ambient_c_per_w", Value: 250},
	}
	return []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		{InstanceID: "regulator", CatalogID: "regulator", Family: "regulator", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveAdjustableLinearRegulatorV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "VIN", Net: "VIN"}, {Function: "VOUT", Net: "VOUT"}, {Function: "GND", Net: "GND"}, {Function: "ADJ", Net: "ADJ"}}},
		resistorEvidence("feedback_upper", 31600, "VOUT", "ADJ"),
		resistorEvidence("feedback_lower", 10000, "ADJ", "GND"),
		resistorEvidence("load", loadResistance, "VOUT", "GND"),
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

func TestMNAOpAmpInputNoiseUsesClosedLoopNoiseGainOnce(t *testing.T) {
	const density = 10e-9
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000}, {Name: "gain_bandwidth_hz", Value: 1000000},
		{Name: "input_voltage_noise_density_v_sqrt_hz", Value: density},
		{Name: "output_high_margin_v", Value: .1}, {Name: "output_low_margin_v", Value: .1},
		{Name: "supply_max_v", Value: 5.5}, {Name: "supply_min_v", Value: 2.7},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "IN", "GND"),
		voltageSourceEvidence("supply", "VCC", "GND"),
		{InstanceID: "opamp", CatalogID: "opamp", Family: "opamp", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "IN_PLUS", Net: "IN"}, {Function: "IN_MINUS", Net: "OUT"}, {Function: "OUT", Net: "OUT"}, {Function: "V_PLUS", Net: "VCC"}, {Function: "V_MINUS", Net: "GND"}}},
	}
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "output_noise", Kind: AnalysisNoise, StartFrequencyHz: 10, StopFrequencyHz: 100000, Points: 64,
			Excitations: []SourceExcitation{{Component: "signal"}, {Component: "supply"}},
		}},
		Assertions: []Assertion{{AnalysisID: "output_noise", Node: "OUT", Quantity: QuantityIntegratedNoiseVRMS, Min: 2.5e-6, Max: 4e-6}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "IN"}, {Name: "OUT"}, {Name: "VCC"}})
	if len(diagnostics) != 0 {
		t.Fatalf("noise resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("noise report = %#v diagnostics=%#v", report, diagnostics)
	}
	for _, point := range report.Analyses[0].Points {
		for _, node := range point.Nodes {
			if node.Node == "OUT" && (node.DominantNoiseSource != "opamp" || node.DominantNoiseDensityVSqrtHz < 9e-9 || node.DominantNoiseDensityVSqrtHz > 11e-9) {
				t.Fatalf("output noise contribution = %#v", node)
			}
		}
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

func TestMNAStabilityDerivesEmitterDegenerationPhaseMargin(t *testing.T) {
	parameters := append(bjtParameters(.2, 40), NamedValue{Name: "transition_frequency_hz", Value: 300e6})
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "BASE", "GND"),
		voltageSourceEvidence("supply", "VCC", "GND"),
		resistorEvidence("collector_resistor", 1000, "VCC", "COLLECTOR"),
		resistorEvidence("emitter_resistor", 100, "EMITTER", "GND"),
		{InstanceID: "transistor", CatalogID: "bjt", Family: "bjt", ModelClaims: []CatalogEvidence{{ModelID: PrimitiveBJTNPNV1, Parameters: parameters}}, Connections: []ConnectionEvidence{{Function: "BASE", Net: "BASE"}, {Function: "COLLECTOR", Net: "COLLECTOR"}, {Function: "EMITTER", Net: "EMITTER"}}},
	}
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "local_loop", Kind: AnalysisStability, StartFrequencyHz: 10, StopFrequencyHz: 3e9, Points: 64,
			Excitations: []SourceExcitation{{Component: "signal", DCValue: .7}, {Component: "supply", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "local_loop", Node: "COLLECTOR", Quantity: QuantityPhaseMarginDeg, Min: 45, Max: 180}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "BASE"}, {Name: "COLLECTOR"}, {Name: "EMITTER"}, {Name: "VCC"}})
	if len(diagnostics) != 0 {
		t.Fatalf("BJT stability resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("BJT stability report = %#v diagnostics=%#v", report, diagnostics)
	}
	if got := report.Assertions[0].Actual; got < 45 || got > 180 {
		t.Fatalf("BJT emitter-degeneration phase margin = %.12g", got)
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
		Assertions: []Assertion{{AnalysisID: "audio_thd", Node: "OUT", Quantity: QuantityTHDPercent, FrequencyHz: 1000, Min: 0, Max: 1e-8}},
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
	for _, point := range report.Analyses[0].Points {
		if len(point.Devices) != 0 {
			t.Fatalf("distortion waveform retained unused per-step device results: %#v", point.Devices)
		}
	}
	replay, replayDiagnostics := Evaluate(ClonePlan(plan))
	if len(replayDiagnostics) != 0 || replay.Assertions[0].Actual != report.Assertions[0].Actual {
		t.Fatalf("distortion replay = %#v diagnostics=%#v", replay, replayDiagnostics)
	}

	mismatched := intent
	mismatched.Assertions = append([]Assertion(nil), intent.Assertions...)
	mismatched.Assertions[0].FrequencyHz = 2000
	if _, mismatchDiagnostics := ResolveWithTopology(
		mismatched,
		"catalog",
		"catalog-hash",
		components,
		[]NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}},
	); len(mismatchDiagnostics) == 0 ||
		!strings.Contains(mismatchDiagnostics[0].Message, "distortion sine fundamental") {
		t.Fatalf("mismatched THD frequency diagnostics = %#v", mismatchDiagnostics)
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

func TestMNAPeriodicThermalAveragesFinalCompleteCycles(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "hot", Kind: AnalysisThermal, DurationS: .004, TimeStepS: .00003125,
			Conditions:  []NamedValue{{Name: "ambient_temperature_c", Value: 25}},
			Excitations: []SourceExcitation{{Component: "signal", SineAmplitude: 1, SineFrequencyHz: 1000}},
		}},
		Assertions: []Assertion{
			{AnalysisID: "hot", Component: "load", Quantity: QuantityDeviceDissipationW, Min: .004999, Max: .005001},
			{AnalysisID: "hot", Component: "load", Quantity: QuantityJunctionTemperatureC, Min: 25.2499, Max: 25.2501},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "OUT", "GND"),
		{InstanceID: "load", CatalogID: "resistor.power", Family: "resistor", HasValueSI: true, ValueSI: 100, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1, Parameters: []NamedValue{{Name: "max_temperature_c", Value: 150}, {Name: "thermal_resistance_c_per_w", Value: 50}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("periodic thermal resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("periodic thermal report = %#v diagnostics=%#v", report, diagnostics)
	}
	if report.Assertions[0].Actual != .005 || report.Assertions[1].Actual != 25.25 {
		t.Fatalf("periodic thermal assertions = %#v", report.Assertions)
	}
}

func TestMNAPeriodicThermalUsesPulseFundamental(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "hot", Kind: AnalysisThermal, DurationS: .004, TimeStepS: .00003125,
			Conditions: []NamedValue{{Name: "ambient_temperature_c", Value: 25}},
			Excitations: []SourceExcitation{{
				Component: "signal", PulseInitialValue: 0, PulseValue: 1,
				PulseWidthS: .0005, PulsePeriodS: .001,
			}},
		}},
		Assertions: []Assertion{
			{AnalysisID: "hot", Component: "load", Quantity: QuantityDeviceDissipationW, Min: .004999, Max: .005001},
			{AnalysisID: "hot", Component: "load", Quantity: QuantityJunctionTemperatureC, Min: 25.2499, Max: 25.2501},
		},
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("signal", "OUT", "GND"),
		{InstanceID: "load", CatalogID: "resistor.power", Family: "resistor", HasValueSI: true, ValueSI: 100, ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1, Parameters: []NamedValue{{Name: "max_temperature_c", Value: 150}, {Name: "thermal_resistance_c_per_w", Value: 50}}}}, Connections: []ConnectionEvidence{{Function: "A", Net: "OUT"}, {Function: "B", Net: "GND"}}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "catalog", "catalog-hash", components, []NodeEvidence{{Name: "GND", Role: "ground"}, {Name: "OUT"}})
	if len(diagnostics) != 0 {
		t.Fatalf("pulse thermal resolution diagnostics = %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("pulse thermal report = %#v diagnostics=%#v", report, diagnostics)
	}
	if math.Abs(report.Assertions[0].Actual-.005) > 1e-15 || report.Assertions[1].Actual != 25.25 {
		t.Fatalf("pulse thermal assertions = %#v", report.Assertions)
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
