package simmodel

import (
	"math"
	"slices"
	"testing"
)

func TestRegistryResolvesAndEvaluatesGenericFamilies(t *testing.T) {
	tests := []struct {
		name       string
		intent     Intent
		components []ComponentEvidence
		metric     string
		want       float64
	}{
		{
			name: "resistor divider",
			intent: Intent{ModelID: ModelResistorDividerDCV1,
				Bindings: []Binding{{Role: "upper_resistor", Component: "r1"}, {Role: "lower_resistor", Component: "r2"}},
				Inputs:   []NamedValue{{Name: "input_voltage_v", Value: 5}}, Assertions: []Assertion{{Metric: "output_voltage_v", Min: 2.49, Max: 2.51}}},
			components: []ComponentEvidence{
				{InstanceID: "r1", CatalogID: "resistor.generic.0603", Family: "resistor", ValueSI: 10000, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelResistorDividerDCV1}}},
				{InstanceID: "r2", CatalogID: "resistor.generic.0603", Family: "resistor", ValueSI: 10000, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelResistorDividerDCV1}}},
			}, metric: "output_voltage_v", want: 2.5,
		},
		{
			name: "rc lowpass",
			intent: Intent{ModelID: ModelRCLowpassACV1,
				Bindings: []Binding{{Role: "resistor", Component: "r1"}, {Role: "capacitor", Component: "c1"}},
				Inputs:   []NamedValue{{Name: "frequency_hz", Value: 1000}}, Assertions: []Assertion{{Metric: "cutoff_frequency_hz", Min: 159, Max: 160}}},
			components: []ComponentEvidence{
				{InstanceID: "r1", CatalogID: "resistor.generic.0603", Family: "resistor", ValueSI: 10000, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelRCLowpassACV1}}},
				{InstanceID: "c1", CatalogID: "capacitor.ceramic.0603", Family: "capacitor", ValueSI: 100e-9, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelRCLowpassACV1}}},
			}, metric: "cutoff_frequency_hz", want: 159.15494309189535,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, diagnostics := Resolve(test.intent, "test", "012345", test.components)
			if len(diagnostics) != 0 {
				t.Fatalf("resolve diagnostics: %#v", diagnostics)
			}
			report, diagnostics := Evaluate(plan)
			if len(diagnostics) != 0 || report.Status != "pass" {
				t.Fatalf("evaluation = %#v diagnostics %#v", report, diagnostics)
			}
			for _, measurement := range report.Measurements {
				if measurement.Metric == test.metric && math.Abs(measurement.Value-test.want) < 1e-12 {
					return
				}
			}
			t.Fatalf("measurement %s=%g missing from %#v", test.metric, test.want, report.Measurements)
		})
	}
}

func TestRegistryFailsClosed(t *testing.T) {
	intent := Intent{ModelID: ModelRCLowpassACV1,
		Bindings: []Binding{{Role: "resistor", Component: "r1"}, {Role: "capacitor", Component: "c1"}},
		Inputs:   []NamedValue{{Name: "frequency_hz", Value: 1000}}, Assertions: []Assertion{{Metric: "gain_ratio", Min: 0, Max: 1}}}
	components := []ComponentEvidence{
		{InstanceID: "r1", CatalogID: "r", Family: "resistor", ValueSI: 1000, HasValueSI: true},
		{InstanceID: "c1", CatalogID: "c", Family: "capacitor", ValueSI: 1e-6, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelRCLowpassACV1}}},
	}
	if _, diagnostics := Resolve(intent, "test", "hash", components); len(diagnostics) == 0 || diagnostics[0].Suggestion == "" {
		t.Fatalf("missing actionable fail-closed diagnostic: %#v", diagnostics)
	}
	intent.ModelID = "provider_supplied_model"
	if diagnostics := ValidateIntent(intent, map[string]string{"r1": "resistor", "c1": "capacitor"}); len(diagnostics) == 0 {
		t.Fatal("untrusted model was accepted")
	}
}

func TestRegistryRejectsIncompleteCatalogModelParameters(t *testing.T) {
	diagnostics := ValidateCatalogEvidence("regulator", []CatalogEvidence{{ModelID: ModelLinearRegulatorIdealV1, Parameters: []NamedValue{
		{Name: "output_voltage_v", Value: 3.3},
	}}})
	if len(diagnostics) == 0 {
		t.Fatal("incomplete regulator catalog model evidence was accepted")
	}
}

func TestRegistryRejectsInconsistentOpAmpCatalogParameters(t *testing.T) {
	parameters := []NamedValue{
		{Name: "dc_open_loop_gain", Value: 100000},
		{Name: "gain_bandwidth_hz", Value: 1e6},
		{Name: "supply_min_v", Value: 5.5},
		{Name: "supply_max_v", Value: 2.7},
		{Name: "output_low_margin_v", Value: 3},
		{Name: "output_high_margin_v", Value: 3},
	}
	if diagnostics := ValidateCatalogEvidence("opamp", []CatalogEvidence{{ModelID: PrimitiveOpAmpV1, Parameters: parameters}}); len(diagnostics) < 2 {
		t.Fatalf("inconsistent op-amp catalog model evidence was accepted: %#v", diagnostics)
	}
}

func TestResolveCanonicalizesProviderModelID(t *testing.T) {
	intent := Intent{ModelID: "  " + ModelResistorDividerDCV1 + "  ",
		Bindings: []Binding{{Role: "upper_resistor", Component: "r1"}, {Role: "lower_resistor", Component: "r2"}},
		Inputs:   []NamedValue{{Name: "input_voltage_v", Value: 5}}, Assertions: []Assertion{{Metric: "output_voltage_v", Min: 2, Max: 3}}}
	components := []ComponentEvidence{
		{InstanceID: "r1", CatalogID: "r", Family: "resistor", ValueSI: 10000, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelResistorDividerDCV1}}},
		{InstanceID: "r2", CatalogID: "r", Family: "resistor", ValueSI: 10000, HasValueSI: true, ModelClaims: []CatalogEvidence{{ModelID: ModelResistorDividerDCV1}}},
	}
	plan, diagnostics := Resolve(intent, "test", "hash", components)
	if len(diagnostics) != 0 || plan.ModelID != ModelResistorDividerDCV1 {
		t.Fatalf("canonical plan = %#v diagnostics %#v", plan, diagnostics)
	}
}

func TestClonePlanDoesNotShareMutableEvidence(t *testing.T) {
	value := 1000.0
	source := Plan{Bindings: []ResolvedBinding{{Role: "resistor", ValueSI: &value, ModelParameters: []NamedValue{{Name: "parameter", Value: 1}}}}, Inputs: []NamedValue{{Name: "input", Value: 2}}, Assertions: []Assertion{{Metric: "metric", Min: 0, Max: 3}}}
	clone := ClonePlan(source)
	clone.Bindings[0].Role = "changed"
	*clone.Bindings[0].ValueSI = 2000
	clone.Bindings[0].ModelParameters[0].Value = 4
	clone.Inputs[0].Value = 5
	clone.Assertions[0].Max = 6
	if source.Bindings[0].Role != "resistor" || *source.Bindings[0].ValueSI != 1000 || source.Bindings[0].ModelParameters[0].Value != 1 || source.Inputs[0].Value != 2 || source.Assertions[0].Max != 3 {
		t.Fatalf("source plan mutated through clone: %#v", source)
	}
}

func TestRequiredModelProvenanceFailsClosedAndPreservesTrustedEvidence(t *testing.T) {
	if diagnostics := ValidateRequiredModelProvenance(nil, []string{AnalysisDCOperatingPoint}); len(diagnostics) == 0 {
		t.Fatal("missing model provenance was accepted")
	}
	provenance := &ModelProvenance{
		Source: "manufacturer-datasheet:example", Revision: "rev-a",
		SHA256:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ReviewStatus: "reviewed", AllowedAnalyses: []string{AnalysisACSweep, AnalysisDCOperatingPoint},
	}
	if diagnostics := ValidateRequiredModelProvenance(provenance, []string{AnalysisDCOperatingPoint}); len(diagnostics) != 0 {
		t.Fatalf("reviewed provenance diagnostics: %#v", diagnostics)
	}
	if diagnostics := ValidateRequiredModelProvenance(provenance, []string{AnalysisNoise}); len(diagnostics) == 0 {
		t.Fatal("model provenance without required analysis applicability was accepted")
	}
	provenance.AllowedAnalyses = []string{AnalysisDCOperatingPoint, AnalysisACSweep}
	if diagnostics := ValidateRequiredModelProvenance(provenance, []string{AnalysisDCOperatingPoint}); len(diagnostics) == 0 {
		t.Fatal("noncanonical provenance analysis ordering was accepted")
	}
}

func TestSupportedAnalysisKindsDescribeExecutableRegistryPaths(t *testing.T) {
	tests := []struct {
		model string
		want  []string
	}{
		{ModelLinearRegulatorIdealV1, []string{AnalysisDCOperatingPoint}},
		{ModelResistorDividerDCV1, []string{AnalysisDCOperatingPoint}},
		{ModelRCLowpassACV1, []string{AnalysisACSweep}},
		{ModelLinearCircuitMNAV1, []string{AnalysisACSweep, AnalysisDCOperatingPoint}},
		{ModelNonlinearCircuitDCV1, []string{AnalysisDCOperatingPoint}},
		{ModelTransientCircuitV1, []string{AnalysisTransient}},
	}
	for _, test := range tests {
		if got := SupportedAnalysisKinds(test.model); !slices.Equal(got, test.want) {
			t.Fatalf("SupportedAnalysisKinds(%q) = %#v, want %#v", test.model, got, test.want)
		}
		for _, kind := range test.want {
			if !SupportsAnalysis(test.model, kind) {
				t.Fatalf("SupportsAnalysis(%q, %q) = false", test.model, kind)
			}
		}
	}
	for _, future := range []string{AnalysisNoise, AnalysisStability, AnalysisStartup, AnalysisDistortion, AnalysisThermal} {
		if SupportsAnalysis(ModelLinearCircuitMNAV1, future) {
			t.Fatalf("future analysis %q was reported executable", future)
		}
	}
	if got := SupportedAnalysisKinds("missing"); len(got) != 0 {
		t.Fatalf("unknown model support = %#v", got)
	}
}

func TestModelContentHashIsStableAndModelSpecific(t *testing.T) {
	seen := map[string]string{}
	for _, modelID := range append(ModelIDs(), PrimitiveModelIDs()...) {
		hash, ok := ModelContentHash(modelID)
		if !ok || len(hash) != 64 {
			t.Fatalf("ModelContentHash(%q) = %q, %t", modelID, hash, ok)
		}
		if previous, duplicate := seen[hash]; duplicate {
			t.Fatalf("models %q and %q share content hash %s", previous, modelID, hash)
		}
		seen[hash] = modelID
	}
	if _, ok := ModelContentHash("missing"); ok {
		t.Fatal("unknown model produced a content hash")
	}
}
