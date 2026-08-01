package simmodel

import (
	"math"
	"testing"
)

func TestMinimumOnlyBandwidthUsesConservativeSolvedLowerBound(t *testing.T) {
	result := AnalysisResult{Kind: AnalysisACSweep, Points: []AnalysisPoint{
		{FrequencyHz: 1_000, Nodes: []NodeResult{{Node: "IN", Magnitude: 1}, {Node: "OUT", Magnitude: 1}}},
		{FrequencyHz: 10_000, Nodes: []NodeResult{{Node: "IN", Magnitude: 1}, {Node: "OUT", Magnitude: 0.99}}},
		{FrequencyHz: 100_000, Nodes: []NodeResult{{Node: "IN", Magnitude: 1}, {Node: "OUT", Magnitude: 0.98}}},
	}}
	assertion := Assertion{
		Node: "OUT", ReferenceNode: "IN", Quantity: QuantityBandwidthHz,
		Min: 50_000, Max: 1e12,
	}
	actual, diagnostic := acDerivedValue(result, assertion)
	if diagnostic != nil || actual != 100_000 {
		t.Fatalf("minimum-only bandwidth actual=%g diagnostic=%#v", actual, diagnostic)
	}

	assertion.Max = 80_000
	if _, diagnostic := acDerivedValue(result, assertion); diagnostic == nil {
		t.Fatal("finite maximum bandwidth passed without a bracketed -3 dB crossing")
	}
}

func TestACInputImpedanceUsesExcitationSourceCurrent(t *testing.T) {
	result := AnalysisResult{Kind: AnalysisACSweep, Points: []AnalysisPoint{{
		FrequencyHz: 60,
		Nodes: []NodeResult{
			{Node: "GND", Magnitude: 0},
			{Node: "IN", Magnitude: 1},
		},
		Devices: []DeviceResult{{Component: "source_in", CurrentMagnitudeA: 5e-6}},
	}}}
	assertion := Assertion{
		Node: "IN", ReferenceNode: "GND", Component: "source_in",
		Quantity: QuantityInputImpedanceOhm, FrequencyHz: 60,
	}
	actual, diagnostic := acDerivedValue(result, assertion)
	if diagnostic != nil || math.Abs(actual-200_000) > 1e-9 {
		t.Fatalf("input impedance actual=%g diagnostic=%#v", actual, diagnostic)
	}

	result.Points[0].Devices[0].CurrentMagnitudeA = 0
	actual, diagnostic = acDerivedValue(result, assertion)
	if diagnostic != nil || actual != maximumTrustedOpenCircuitImpedanceOhm {
		t.Fatalf("open input impedance actual=%g diagnostic=%#v", actual, diagnostic)
	}
}

func TestACTransimpedanceUsesExcitationSourceCurrent(t *testing.T) {
	result := AnalysisResult{Kind: AnalysisACSweep, Points: []AnalysisPoint{{
		FrequencyHz: 1_000,
		Nodes:       []NodeResult{{Node: "OUT", Magnitude: 2.5}},
		Devices:     []DeviceResult{{Component: "source_in", CurrentMagnitudeA: 25e-6}},
	}}}
	assertion := Assertion{
		Node: "OUT", Component: "source_in",
		Quantity: QuantityTransimpedanceOhm, FrequencyHz: 1_000,
	}
	actual, diagnostic := acDerivedValue(result, assertion)
	if diagnostic != nil || math.Abs(actual-100_000) > 1e-9 {
		t.Fatalf("transimpedance actual=%g diagnostic=%#v", actual, diagnostic)
	}

	result.Points[0].Devices[0].CurrentMagnitudeA = 0
	if _, diagnostic = acDerivedValue(result, assertion); diagnostic == nil {
		t.Fatal("zero excitation-source current produced trusted AC transimpedance")
	}
}

func TestCurrentReferencedBandwidthUsesExcitationSourceCurrent(t *testing.T) {
	result := AnalysisResult{Kind: AnalysisACSweep, Points: []AnalysisPoint{
		{FrequencyHz: 1_000, Nodes: []NodeResult{{Node: "OUT", Magnitude: 1}}, Devices: []DeviceResult{{Component: "source_in", CurrentMagnitudeA: 10e-6}}},
		{FrequencyHz: 10_000, Nodes: []NodeResult{{Node: "OUT", Magnitude: 0.5}}, Devices: []DeviceResult{{Component: "source_in", CurrentMagnitudeA: 10e-6}}},
	}}
	assertion := Assertion{
		Node: "OUT", Component: "source_in",
		Quantity: QuantityBandwidthHz, Min: 1, Max: 1e12,
	}
	actual, diagnostic := acDerivedValue(result, assertion)
	if diagnostic != nil || actual <= 1_000 || actual >= 10_000 {
		t.Fatalf("current-referenced bandwidth actual=%g diagnostic=%#v", actual, diagnostic)
	}
}
