package simmodel

import "testing"

func TestMNAProtectedIsolatedConvertersSupportSeriesParallelRail(t *testing.T) {
	parameters := []NamedValue{
		{Name: "input_min_v", Value: 18}, {Name: "input_max_v", Value: 36},
		{Name: "input_current_reference_voltage_v", Value: 24},
		{Name: "output_voltage_v", Value: 12}, {Name: "max_output_current_a", Value: .833},
		{Name: "short_circuit_current_a", Value: 1.5827}, {Name: "soft_start_time_s", Value: .06},
		{Name: "maximum_overshoot_ratio", Value: .05}, {Name: "efficiency_ratio", Value: .87},
		{Name: "isolation_working_voltage_v", Value: 1000}, {Name: "isolation_resistance_ohm", Value: 1e9},
		{Name: "max_temperature_c", Value: 105}, {Name: "junction_to_ambient_c_per_w", Value: 26.8},
	}
	converter := func(id, negative, positive string) ComponentEvidence {
		return ComponentEvidence{
			InstanceID: id, CatalogID: id, Family: "isolated_converter",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveProtectedIsolatedConverterV1, Parameters: parameters}},
			Connections: []ConnectionEvidence{
				{Function: "VIN_PLUS", Net: "VIN"}, {Function: "VIN_MINUS", Net: "GND"},
				{Function: "VOUT_PLUS", Net: positive}, {Function: "VOUT_MINUS", Net: negative},
			},
		}
	}
	components := []ComponentEvidence{
		voltageSourceEvidence("supply", "VIN", "GND"),
		converter("converter_a1", "GND", "A1"),
		converter("converter_a2", "A1", "A2"),
		converter("converter_b1", "GND", "B1"),
		converter("converter_b2", "B1", "B2"),
		resistorEvidence("ballast_a1", .22, "A2", "AMID"),
		resistorEvidence("ballast_a2", 10, "AMID", "RAIL"),
		resistorEvidence("ballast_b1", .22, "B2", "BMID"),
		resistorEvidence("ballast_b2", 10, "BMID", "RAIL"),
		resistorEvidence("load", 29, "RAIL", "GND"),
	}
	nodes := []NodeEvidence{
		{Name: "GND", Role: "ground"}, {Name: "VIN", Role: "power"},
		{Name: "A1"}, {Name: "A2"}, {Name: "AMID"},
		{Name: "B1"}, {Name: "B2"}, {Name: "BMID"}, {Name: "RAIL", Role: "power"},
	}
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "dc", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "supply", DCValue: 24}},
		}},
		Assertions: []Assertion{{
			AnalysisID: "dc", Node: "RAIL", Quantity: QuantityVoltageV, Min: 20, Max: 21,
		}},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "catalog-hash", components, nodes)
	if len(diagnostics) != 0 {
		t.Fatalf("resolve series-parallel converter rail: %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" {
		t.Fatalf("series-parallel converter rail report=%#v diagnostics=%#v", report, diagnostics)
	}
}
