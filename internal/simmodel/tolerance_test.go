package simmodel

import (
	"reflect"
	"strings"
	"testing"
)

func TestEvaluateWorstCaseBlocksNominalOnlyDivider(t *testing.T) {
	plan, diagnostics := Resolve(Intent{ModelID: ModelResistorDividerDCV1,
		Bindings:   []Binding{{Role: "upper_resistor", Component: "r1"}, {Role: "lower_resistor", Component: "r2"}},
		Inputs:     []NamedValue{{Name: "input_voltage_v", Value: 5}},
		Assertions: []Assertion{{Metric: "output_voltage_v", Min: 2.49, Max: 2.51}},
	}, "catalog", "hash", []ComponentEvidence{
		{InstanceID: "r1", CatalogID: "r1", Family: "resistor", ValueSI: 10000, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelResistorDividerDCV1}}},
		{InstanceID: "r2", CatalogID: "r2", Family: "resistor", ValueSI: 10000, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelResistorDividerDCV1}}},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve: %#v", diagnostics)
	}
	plan.Uncertainties = []Uncertainty{
		{Target: "bindings.lower_resistor.value_si", Source: "catalog:r2:tolerance", Nominal: 10000, Minimum: 9900, Maximum: 10100},
		{Target: "bindings.upper_resistor.value_si", Source: "catalog:r1:tolerance", Nominal: 10000, Minimum: 9900, Maximum: 10100},
	}
	plan.WorstCase = true
	report, diagnostics := Evaluate(plan)
	if report.Status != "blocked" || len(diagnostics) == 0 {
		t.Fatalf("worst case report=%#v diagnostics=%#v", report, diagnostics)
	}
	if len(report.Corners) != 9 || len(report.Sensitivity) == 0 {
		t.Fatalf("bounded evidence=%#v", report)
	}
	if !strings.Contains(diagnostics[0].Message, "dominant contributor") {
		t.Fatalf("missing attribution: %#v", diagnostics)
	}
}

func TestEvaluateWorstCaseIsOrderIndependentAndFailsClosed(t *testing.T) {
	base := Plan{RegistryVersion: RegistryVersion, RegistryHash: RegistryHash(), CatalogID: "catalog", CatalogHash: "hash", ModelID: ModelRCLowpassACV1,
		Bindings: []ResolvedBinding{{Role: "capacitor", Component: "c", CatalogID: "c", Family: "capacitor", ValueSI: floatPointer(100e-9)}, {Role: "resistor", Component: "r", CatalogID: "r", Family: "resistor", ValueSI: floatPointer(10000)}},
		Inputs:   []NamedValue{{Name: "frequency_hz", Value: 1000}}, Assertions: []Assertion{{Metric: "gain_ratio", Min: 0.1, Max: 1}},
		Uncertainties: []Uncertainty{{Target: "bindings.capacitor.value_si", Source: "catalog:c:tolerance", Nominal: 100e-9, Minimum: 80e-9, Maximum: 120e-9}, {Target: "bindings.resistor.value_si", Source: "catalog:r:tolerance", Nominal: 10000, Minimum: 9900, Maximum: 10100}},
		WorstCase:     true,
	}
	first, firstDiagnostics := Evaluate(base)
	if len(firstDiagnostics) != 0 || first.Status != "pass" {
		t.Fatalf("first=%#v diagnostics=%#v", first, firstDiagnostics)
	}
	reordered := ClonePlan(base)
	reordered.Uncertainties[0], reordered.Uncertainties[1] = reordered.Uncertainties[1], reordered.Uncertainties[0]
	if _, diagnostics := Evaluate(reordered); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "canonically ordered") {
		t.Fatalf("unordered evidence accepted: %#v", diagnostics)
	}
	if !reflect.DeepEqual(first.Corners, first.Corners) {
		t.Fatal("nondeterministic corners")
	}
	tooMany := ClonePlan(base)
	for len(tooMany.Uncertainties) <= maxWorstCaseUncertainties {
		tooMany.Uncertainties = append(tooMany.Uncertainties, Uncertainty{Target: "bindings.extra.value_si", Source: "catalog", Nominal: 1, Minimum: .9, Maximum: 1.1})
	}
	if _, diagnostics := Evaluate(tooMany); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "at most") {
		t.Fatalf("unbounded evidence accepted: %#v", diagnostics)
	}
}

func TestCatalogModelUncertaintyControlsRegulatorWorstCase(t *testing.T) {
	claim := CatalogEvidence{ModelID: ModelLinearRegulatorIdealV1, Parameters: []NamedValue{{Name: "max_load_current_ma", Value: 500}, {Name: "min_headroom_v", Value: 1}, {Name: "output_voltage_v", Value: 3.3}}, Uncertainties: []Uncertainty{{Target: "model_parameters.output_voltage_v", Source: "datasheet:output-accuracy", Nominal: 3.3, Minimum: 3.2, Maximum: 3.4}}}
	if diagnostics := ValidateCatalogEvidence("regulator", []CatalogEvidence{claim}); len(diagnostics) != 0 {
		t.Fatalf("valid reviewed uncertainty rejected: %#v", diagnostics)
	}
	plan, diagnostics := Resolve(Intent{ModelID: ModelLinearRegulatorIdealV1, WorstCase: true, Bindings: []Binding{{Role: "regulator", Component: "u1"}}, Inputs: []NamedValue{{Name: "input_voltage_v", Value: 5}, {Name: "load_current_ma", Value: 100}}, Assertions: []Assertion{{Metric: "output_voltage_v", Min: 3.25, Max: 3.35}}}, "catalog", "hash", []ComponentEvidence{{InstanceID: "u1", CatalogID: "u1", Family: "regulator", ModelClaims: []CatalogEvidence{claim}, Uncertainties: []Uncertainty{{Target: "model_parameters.output_voltage_v", Source: "datasheet:output-accuracy", Nominal: 3.3, Minimum: 3.2, Maximum: 3.4}}}})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve: %#v", diagnostics)
	}
	report, diagnostics := Evaluate(plan)
	if report.Status != "blocked" || len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "dominant contributor") {
		t.Fatalf("report=%#v diagnostics=%#v", report, diagnostics)
	}
	invalid := claim
	invalid.Uncertainties[0].Target = "model_parameters.unknown"
	if diagnostics := ValidateCatalogEvidence("regulator", []CatalogEvidence{invalid}); len(diagnostics) == 0 {
		t.Fatal("unsupported catalog parameter uncertainty accepted")
	}
}
