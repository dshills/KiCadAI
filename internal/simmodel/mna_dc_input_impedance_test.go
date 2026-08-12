package simmodel

import (
	"math"
	"testing"
)

func TestDCInputImpedanceUsesSolvedDifferentialVoltageAndSourceCurrent(t *testing.T) {
	intent := Intent{
		ModelID: ModelLinearCircuitMNAV1,
		Analyses: []Analysis{{
			ID: "operating_point", Kind: AnalysisDCOperatingPoint,
			Excitations: []SourceExcitation{{Component: "source", DCValue: 2}},
		}},
		Assertions: []Assertion{{
			AnalysisID: "operating_point", Node: "INPUT",
			Component: "source", Quantity: QuantityInputImpedanceOhm,
			Min: 999, Max: 1001,
		}},
	}
	components := []ComponentEvidence{
		{
			InstanceID: "load", CatalogID: "resistor", Family: "resistor",
			HasValueSI: true, ValueSI: 1000,
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveResistorV1}},
			Connections: []ConnectionEvidence{{Function: "A", Net: "INPUT"}, {Function: "B", Net: "GND"}},
		},
		{
			InstanceID: "source", CatalogID: "source", Family: "voltage_source",
			ModelClaims: []CatalogEvidence{{ModelID: PrimitiveVoltageSourceV1}},
			Connections: []ConnectionEvidence{{Function: "POSITIVE", Net: "INPUT"}, {Function: "NEGATIVE", Net: "GND"}},
		},
	}
	plan, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{
		{Name: "GND", Role: "ground"}, {Name: "INPUT", Role: "signal"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("resolve diagnostics = %+v", diagnostics)
	}
	if len(plan.Assertions) != 1 || plan.Assertions[0].ReferenceNode != "GND" {
		t.Fatalf("DC input impedance reference = %#v, want canonical ground", plan.Assertions)
	}
	report, diagnostics := Evaluate(plan)
	if len(diagnostics) != 0 || report.Status != "pass" || len(report.Assertions) != 1 {
		t.Fatalf("report=%+v diagnostics=%+v", report, diagnostics)
	}
	if math.Abs(report.Assertions[0].Actual-1000) > 1e-9 {
		t.Fatalf("DC input impedance = %.12g, want 1000", report.Assertions[0].Actual)
	}

	intent.Assertions[0].FrequencyHz = 1
	if _, diagnostics := ResolveWithTopology(intent, "test", "hash", components, []NodeEvidence{
		{Name: "GND", Role: "ground"}, {Name: "INPUT", Role: "signal"},
	}); !diagnosticsContain(diagnostics, "does not accept a frequency") {
		t.Fatalf("DC frequency diagnostics = %+v", diagnostics)
	}
}

func TestDCInputImpedanceHandlesOpenAndIndeterminateMeasurements(t *testing.T) {
	assertion := Assertion{
		AnalysisID: "operating_point", Node: "INPUT", ReferenceNode: "GND",
		Component: "source", Quantity: QuantityInputImpedanceOhm,
	}
	result := AnalysisResult{ID: "operating_point", Kind: AnalysisDCOperatingPoint, Points: []AnalysisPoint{{
		Nodes:   []NodeResult{{Node: "GND", Real: 0}, {Node: "INPUT", Real: 2}},
		Devices: []DeviceResult{{Component: "source"}},
	}}}
	actual, diagnostic := dcInputImpedanceValue(result, assertion)
	if diagnostic != nil || actual != maximumTrustedOpenCircuitImpedanceOhm {
		t.Fatalf("open input impedance actual=%g diagnostic=%#v", actual, diagnostic)
	}

	result.Points[0].Nodes[1].Real = 0
	actual, diagnostic = dcInputImpedanceValue(result, assertion)
	if diagnostic == nil || actual != 0 {
		t.Fatalf("indeterminate input impedance actual=%g diagnostic=%#v", actual, diagnostic)
	}

	result.Points[0].Nodes[1].Real = 10
	result.Points[0].Devices[0].CurrentA = 1.001e-15
	actual, diagnostic = dcInputImpedanceValue(result, assertion)
	if diagnostic != nil || actual != maximumTrustedOpenCircuitImpedanceOhm {
		t.Fatalf("clamped input impedance actual=%g diagnostic=%#v", actual, diagnostic)
	}
}
