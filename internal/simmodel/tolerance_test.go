package simmodel

import (
	"fmt"
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
	second, secondDiagnostics := Evaluate(base)
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstDiagnostics, secondDiagnostics) {
		t.Fatalf("parallel worst-case evaluation is nondeterministic:\nfirst=%#v\nsecond=%#v\nfirst diagnostics=%#v\nsecond diagnostics=%#v", first, second, firstDiagnostics, secondDiagnostics)
	}
	reordered := ClonePlan(base)
	reordered.Uncertainties[0], reordered.Uncertainties[1] = reordered.Uncertainties[1], reordered.Uncertainties[0]
	if _, diagnostics := Evaluate(reordered); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "canonically ordered") {
		t.Fatalf("unordered evidence accepted: %#v", diagnostics)
	}
	tooMany := ClonePlan(base)
	for len(tooMany.Uncertainties) <= maxWorstCaseUncertainties {
		target := fmt.Sprintf("bindings.zz_extra_%02d.value_si", len(tooMany.Uncertainties))
		tooMany.Uncertainties = append(tooMany.Uncertainties, Uncertainty{Target: target, Source: "catalog", Nominal: 1, Minimum: .9, Maximum: 1.1})
	}
	if _, diagnostics := Evaluate(tooMany); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "at most") {
		t.Fatalf("unbounded evidence accepted: %#v", diagnostics)
	}
}

func TestEightIndependentUncertaintyGroupsRemainDeterministicallyBounded(t *testing.T) {
	const uncertaintyCount = 8
	uncertainties := make([]Uncertainty, uncertaintyCount)
	for index := range uncertainties {
		uncertainties[index] = Uncertainty{
			Target:  fmt.Sprintf("devices.r%02d.value_si", index),
			Source:  fmt.Sprintf("catalog:r%02d:tolerance", index),
			Nominal: 1000,
			Minimum: 990,
			Maximum: 1010,
		}
	}
	if diagnostics := validateUncertainties(uncertainties); len(diagnostics) != 0 {
		t.Fatalf("bounded uncertainty set rejected: %#v", diagnostics)
	}
	corners := deterministicCorners(uncertainties)
	want := 2*uncertaintyCount + 2 + 2*4
	if len(corners) != want {
		t.Fatalf("corners=%d, want %d", len(corners), want)
	}
	unique, resultIndex := uniqueCornerEvaluationPlan(corners)
	if len(unique) != want || len(resultIndex) != want {
		t.Fatalf("unique corners=%d indices=%d, want %d", len(unique), len(resultIndex), want)
	}
	for first := 0; first < uncertaintyCount; first++ {
		for second := first + 1; second < uncertaintyCount; second++ {
			combinations := map[[2]bool]bool{}
			for _, corner := range corners[2*uncertaintyCount:] {
				combinations[[2]bool{
					corner[first].Value == uncertainties[first].Maximum,
					corner[second].Value == uncertainties[second].Maximum,
				}] = true
			}
			if len(combinations) != 4 {
				t.Fatalf("groups %d/%d pair coverage = %#v", first, second, combinations)
			}
		}
	}
}

func TestCatalogJunctionTemperaturesShareOneEnvironmentalCorner(t *testing.T) {
	uncertainties := make([]Uncertainty, 8)
	for index := range uncertainties {
		uncertainties[index] = Uncertainty{
			Target:  fmt.Sprintf("devices.q%02d.model_parameters.junction_temperature_k", index),
			Source:  fmt.Sprintf("catalog:q%02d:temperature", index),
			Nominal: 298.15,
			Minimum: 233.15 + float64(index),
			Maximum: 423.15 - float64(index),
		}
	}
	if diagnostics := validateUncertainties(uncertainties); len(diagnostics) != 0 {
		t.Fatalf("correlated device temperatures rejected: %#v", diagnostics)
	}
	groups := groupedUncertainties(uncertainties)
	if len(groups) != 1 || groups[0].target != "environment.temperature" || len(groups[0].members) != len(uncertainties) || groups[0].minimum != 240.15 || groups[0].maximum != 416.15 {
		t.Fatalf("groups=%#v", groups)
	}
	corners := deterministicCorners(uncertainties)
	if len(corners) != 4 {
		t.Fatalf("corners=%d, want 4", len(corners))
	}
	unique, resultIndex := uniqueCornerEvaluationPlan(corners)
	if len(unique) != 2 || !reflect.DeepEqual(resultIndex, []int{0, 1, 0, 1}) {
		t.Fatalf("unique corner evaluation plan = %d %#v, want two solved endpoints reused by four report entries", len(unique), resultIndex)
	}
	for _, corner := range corners {
		first := corner[0].Value
		for _, assignment := range corner[1:] {
			if assignment.Value != first {
				t.Fatalf("temperature corner is not correlated: %#v", corner)
			}
		}
	}
	nonoverlapping := append([]Uncertainty(nil), uncertainties...)
	nonoverlapping[len(nonoverlapping)-1].Nominal = 421
	nonoverlapping[len(nonoverlapping)-1].Minimum = 420
	nonoverlapping[len(nonoverlapping)-1].Maximum = 422
	if diagnostics := validateUncertainties(nonoverlapping); len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "do not overlap") {
		t.Fatalf("nonoverlapping environmental ranges accepted: %#v", diagnostics)
	}
}

func TestWorstCaseDistinguishesFailedAssertionsFromIncompleteAnalysis(t *testing.T) {
	if hasCornerEvaluationFailure([]Diagnostic{{Path: "assertions.gain", Message: "outside bounds"}}) {
		t.Fatal("a completed failing assertion was classified as an incomplete corner")
	}
	if !hasCornerEvaluationFailure([]Diagnostic{{Path: "analyses.transient.points[4].convergence", Message: "did not converge"}}) {
		t.Fatal("an incomplete analysis was not classified as a corner evaluation failure")
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
