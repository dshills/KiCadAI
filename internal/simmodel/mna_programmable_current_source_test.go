package simmodel

import (
	"strings"
	"testing"
)

func TestMNAProgrammableCurrentSourceUsesExternalResistorRatio(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "operating_point", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}},
		}},
		Assertions: []Assertion{
			{AnalysisID: "operating_point", Node: "LOAD", Quantity: QuantityVoltageV, Min: 2.999, Max: 3.001},
			{AnalysisID: "operating_point", Component: "regulator", Quantity: QuantityDeviceCurrentA, Min: .09999, Max: .10001},
		},
	}
	components := programmableCurrentSourceEvidence(30)
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, []NodeEvidence{
		{Name: "GND", Role: "ground"},
		{Name: "5V", Role: "power"},
		{Name: "SET", Role: "bias"},
		{Name: "REGULATED", Role: "regulated_current"},
		{Name: "LOAD", Role: "output"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
}

func TestMNAProgrammableCurrentSourceRejectsInsufficientHeadroom(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "operating_point", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "operating_point", Component: "regulator", Quantity: QuantityDeviceCurrentA, Min: .09, Max: .11}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", programmableCurrentSourceEvidence(40), []NodeEvidence{
		{Name: "GND", Role: "ground"},
		{Name: "5V", Role: "power"},
		{Name: "SET", Role: "bias"},
		{Name: "REGULATED", Role: "regulated_current"},
		{Name: "LOAD", Role: "output"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	_, diagnostics = Evaluate(plan)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "headroom") {
		t.Fatalf("diagnostics = %+v; want catalog-backed headroom rejection", diagnostics)
	}
}

func TestMNAProgrammableCurrentSourceStartupIsOpenUntilPowered(t *testing.T) {
	intent := Intent{
		ModelID: ModelTransientCircuitV1,
		Analyses: []Analysis{{
			ID: "startup", Kind: AnalysisStartup, DurationS: 100e-6, TimeStepS: 10e-6,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 5}},
		}},
		Assertions: []Assertion{{AnalysisID: "startup", Node: "LOAD", Quantity: QuantityPeakAbsVoltageV, Min: 2.999, Max: 3.001}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", programmableCurrentSourceEvidence(30), []NodeEvidence{
		{Name: "GND", Role: "ground"},
		{Name: "5V", Role: "power"},
		{Name: "SET", Role: "bias"},
		{Name: "REGULATED", Role: "regulated_current"},
		{Name: "LOAD", Role: "output"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	if got := nodeReal(report.Analyses[0].Points[1].Nodes, "LOAD"); got != 0 {
		t.Fatalf("LOAD at first powered startup step = %.12g, want unpowered current source open", got)
	}
	if got := nodeReal(report.Analyses[0].Points[2].Nodes, "LOAD"); got < 2.999 || got > 3.001 {
		t.Fatalf("LOAD after headroom is established = %.12g, want regulated output", got)
	}
}

func programmableCurrentSourceEvidence(loadResistance float64) []ComponentEvidence {
	value := func(instance, first, second string, resistance float64) ComponentEvidence {
		return ComponentEvidence{
			InstanceID: instance, CatalogID: "resistor.test", Family: "resistor",
			HasValueSI: true, ValueSI: resistance,
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}},
			Connections: []ConnectionEvidence{{Function: "A", Net: first}, {Function: "B", Net: second}},
		}
	}
	return []ComponentEvidence{
		voltageSourceEvidence("supply", "5V", "GND"),
		{
			InstanceID: "regulator", CatalogID: "current-regulator.test", Family: "current_regulator",
			ModelClaims: []CatalogEvidence{{
				ModelID: PrimitiveProgrammableCurrentSourceV1,
				Parameters: []NamedValue{
					{Name: "reference_current_a", Value: 10e-6},
					{Name: "offset_voltage_v", Value: 0},
					{Name: "min_headroom_v", Value: 1.4},
					{Name: "max_output_current_a", Value: .2},
					{Name: "max_input_output_voltage_v", Value: 40},
					{Name: "soft_start_time_s", Value: 0},
					{Name: "max_temperature_c", Value: 125},
					{Name: "junction_to_ambient_c_per_w", Value: 24},
					{Name: "junction_to_case_c_per_w", Value: 15},
				},
			}},
			Connections: []ConnectionEvidence{
				{Function: "IN", Net: "5V"},
				{Function: "SET", Net: "SET"},
				{Function: "OUT", Net: "REGULATED"},
			},
		},
		value("set_resistor", "SET", "LOAD", 20_000),
		value("output_resistor", "REGULATED", "LOAD", 2.00020002),
		value("load", "LOAD", "GND", loadResistance),
	}
}
