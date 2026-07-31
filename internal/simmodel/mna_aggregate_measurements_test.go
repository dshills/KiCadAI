package simmodel

import (
	"math"
	"testing"
)

func TestMNAOrderedThresholdMeasurementsAreDirectionSpecific(t *testing.T) {
	points := []AnalysisPoint{
		{Sweep: dcSweepForward, SweepValue: 0, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{Sweep: dcSweepForward, SweepValue: 1, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{Sweep: dcSweepForward, SweepValue: 2, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{Sweep: dcSweepForward, SweepValue: 3, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{Sweep: dcSweepReverse, SweepValue: 3, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{Sweep: dcSweepReverse, SweepValue: 2.5, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{Sweep: dcSweepReverse, SweepValue: 1.5, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{Sweep: dcSweepReverse, SweepValue: 0, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
	}
	result := AnalysisResult{Kind: AnalysisDCOperatingPoint, Points: points}
	lower, diagnostic := dcSweepDerivedValue(result, Assertion{Node: "OUT", Quantity: QuantityLowerThresholdVoltageV})
	if diagnostic != nil || math.Abs(lower-.5) > 1e-12 {
		t.Fatalf("lower threshold = %.12g diagnostic=%#v", lower, diagnostic)
	}
	upper, diagnostic := dcSweepDerivedValue(result, Assertion{Node: "OUT", Quantity: QuantityUpperThresholdVoltageV})
	if diagnostic != nil || math.Abs(upper-2.5) > 1e-12 {
		t.Fatalf("upper threshold = %.12g diagnostic=%#v", upper, diagnostic)
	}

	singleTransition := AnalysisResult{Kind: AnalysisDCOperatingPoint, Points: []AnalysisPoint{
		{Sweep: dcSweepForward, SweepValue: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{Sweep: dcSweepForward, SweepValue: 1, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
		{Sweep: dcSweepForward, SweepValue: 2, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{Sweep: dcSweepReverse, SweepValue: 2, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{Sweep: dcSweepReverse, SweepValue: 1, Nodes: []NodeResult{{Node: "OUT", Real: 5}}},
		{Sweep: dcSweepReverse, SweepValue: 0, Nodes: []NodeResult{{Node: "OUT", Real: 0}}},
	}}
	rising, diagnostic := dcSweepDerivedValue(singleTransition, Assertion{Node: "OUT", Quantity: QuantityRisingThresholdVoltageV})
	if diagnostic != nil || math.Abs(rising-1.5) > 1e-12 {
		t.Fatalf("rising threshold = %.12g diagnostic=%#v", rising, diagnostic)
	}
	falling, diagnostic := dcSweepDerivedValue(singleTransition, Assertion{Node: "OUT", Quantity: QuantityFallingThresholdVoltageV})
	if diagnostic != nil || math.Abs(falling-.5) > 1e-12 {
		t.Fatalf("falling threshold = %.12g diagnostic=%#v", falling, diagnostic)
	}
}

func TestMNASweepSpanAndDeviceSlopeUseSolvedEvidence(t *testing.T) {
	result := AnalysisResult{Kind: AnalysisDCOperatingPoint, Points: []AnalysisPoint{
		{Sweep: dcSweepForward, SweepValue: 0, Nodes: []NodeResult{{Node: "OUT", Real: 5}}, Devices: []DeviceResult{{Component: "load", CurrentMagnitudeA: .01}}},
		{Sweep: dcSweepForward, SweepValue: 1, Nodes: []NodeResult{{Node: "OUT", Real: 4.8}}, Devices: []DeviceResult{{Component: "load", CurrentMagnitudeA: .11}}},
		{Sweep: dcSweepForward, SweepValue: 2, Nodes: []NodeResult{{Node: "OUT", Real: 4.9}}, Devices: []DeviceResult{{Component: "load", CurrentMagnitudeA: .21}}},
	}}
	span, diagnostic := dcSweepSpanOrSlope(result, Assertion{Node: "OUT", Quantity: QuantityDCSweepVoltageSpanV})
	if diagnostic != nil || math.Abs(span-.2) > 1e-12 {
		t.Fatalf("DC sweep span = %.12g diagnostic=%#v", span, diagnostic)
	}
	slope, diagnostic := dcSweepSpanOrSlope(result, Assertion{Component: "load", Quantity: QuantityDCSweepDeviceSlopeAperV})
	if diagnostic != nil || math.Abs(slope-.1) > 1e-12 {
		t.Fatalf("DC sweep device slope = %.12g diagnostic=%#v", slope, diagnostic)
	}
}

func TestMNAAggregateThermalMeasurementsCoverAllDeclaredComponents(t *testing.T) {
	hot := 110.0
	warm := 85.0
	result := AnalysisResult{Kind: AnalysisElectrothermal, Points: []AnalysisPoint{{Devices: []DeviceResult{
		{Component: "q1", JunctionTemperatureC: &warm, TransientSOAEvaluated: true, TransientSOAMargin: 2},
		{Component: "q2", JunctionTemperatureC: &hot, TransientSOAEvaluated: true, TransientSOAMargin: 1.25},
	}}}}
	components := []string{"q1", "q2"}
	maximum, diagnostic := thermalAssertionValue(result, Assertion{Components: components, Quantity: QuantityMaximumJunctionTemperatureC})
	if diagnostic != nil || maximum != hot {
		t.Fatalf("maximum junction temperature = %.12g diagnostic=%#v", maximum, diagnostic)
	}
	minimum, diagnostic := thermalAssertionValue(result, Assertion{Components: components, Quantity: QuantityMinimumTransientSOAMargin})
	if diagnostic != nil || minimum != 1.25 {
		t.Fatalf("minimum SOA margin = %.12g diagnostic=%#v", minimum, diagnostic)
	}
}
