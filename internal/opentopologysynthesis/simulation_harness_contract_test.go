package opentopologysynthesis

import (
	"math"
	"testing"

	"kicadai/internal/simmodel"
)

func TestBoundedDigitalDynamicRangeUsesTypedPortContract(t *testing.T) {
	requirement := Requirement{Requirements: Requirements{Ports: []Port{{
		ID: "command", Kind: "digital", Direction: "sink", Domain: "logic",
		Electrical: Electrical{MinVoltageV: harnessFloat(.1), MaxVoltageV: harnessFloat(1.8), DefaultState: "low"},
	}}}}
	tests := []struct {
		name         string
		metric       string
		defaultState string
		wantInitial  float64
		wantApplied  float64
		wantFound    bool
	}{
		{name: "propagation from low", metric: "propagation_delay", defaultState: "low", wantInitial: .1, wantApplied: 1.8, wantFound: true},
		{name: "propagation from high", metric: "propagation_delay", defaultState: "high", wantInitial: 1.8, wantApplied: .1, wantFound: true},
		{name: "rise overrides high default", metric: "rise_time", defaultState: "high", wantInitial: .1, wantApplied: 1.8, wantFound: true},
		{name: "fall overrides low default", metric: "fall_time", defaultState: "low", wantInitial: 1.8, wantApplied: .1, wantFound: true},
		{name: "static metric", metric: "output_high_voltage", defaultState: "low", wantFound: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement.Requirements.Ports[0].Electrical.DefaultState = test.defaultState
			initial, applied, found := boundedDigitalDynamicRange(
				requirement, BehavioralAssertion{Metric: test.metric}, "command",
			)
			if found != test.wantFound || (found && (initial != test.wantInitial || applied != test.wantApplied)) {
				t.Fatalf("range = %.12g..%.12g found=%t", initial, applied, found)
			}
		})
	}

	requirement.Requirements.Ports[0].Kind = "analog_voltage"
	if _, _, found := boundedDigitalDynamicRange(requirement, BehavioralAssertion{Metric: "propagation_delay"}, "command"); found {
		t.Fatal("analog port received an inferred digital pulse")
	}
}

func TestBoundedDigitalPeerDefaultDefersToAuthoredCondition(t *testing.T) {
	requirement := Requirement{Requirements: Requirements{Ports: []Port{{
		ID: "peer", Kind: "digital", Direction: "sink", Domain: "logic",
		Electrical: Electrical{MinVoltageV: harnessFloat(0), MaxVoltageV: harnessFloat(3.3), DefaultState: "high"},
	}}}}
	if value, found := boundedDigitalPeerDefault(requirement, OperatingCase{}, "peer"); !found || value != 3.3 {
		t.Fatalf("peer default = %.12g found=%t", value, found)
	}
	authored := OperatingCase{Conditions: []OperatingCondition{{
		Axis: "input_voltage", Target: "peer", Min: 1.2, Max: 1.2, Unit: "V",
	}}}
	if _, found := boundedDigitalPeerDefault(requirement, authored, "peer"); found {
		t.Fatal("peer default overrode an authored operating condition")
	}
}

func TestFailedSimulationActualRejectsCensoredMeasurementPlaceholder(t *testing.T) {
	report := simmodel.Report{Assertions: []simmodel.AssertionResult{{
		Min: -1e12, Max: 1e-6, Actual: -1.000001e12, Pass: false,
	}}}
	measurementFailure := []simmodel.Diagnostic{{
		Path: "assertions.dynamic.output", Message: "event-response assertion requires a nonconstant solved waveform",
	}}
	if actual, found := failedSimulationActual(report, measurementFailure, 1); found {
		t.Fatalf("censored measurement surfaced as actual %.12g", actual)
	}
	boundFailure := []simmodel.Diagnostic{{
		Code: simmodel.DiagnosticAssertionOutOfBounds,
		Path: "assertions.dynamic.output", Message: "measurement rejected by trusted bounds",
	}}
	report.Assertions[0].Actual = 2
	if actual, found := failedSimulationActual(report, boundFailure, 1); !found || actual != 2 {
		t.Fatalf("measured bound failure actual = %.12g found=%t", actual, found)
	}
	report.Assertions = append(report.Assertions, simmodel.AssertionResult{Actual: 3, Pass: false})
	if actual, found := failedSimulationActual(report, boundFailure, 1); found {
		t.Fatalf("ambiguous multi-assertion report surfaced actual %.12g", actual)
	}
}

func TestExecutableCornersCompactPlanEquivalentAxes(t *testing.T) {
	operatingCase := OperatingCase{ID: "envelope", Conditions: []OperatingCondition{
		{Axis: "supply_voltage", Target: "rail", Min: 3, Max: 5, Unit: "V"},
		{Axis: "ambient_temperature", Target: "assembly", Min: -20, Max: 80, Unit: "degC"},
		{Axis: "model_corner", Target: "output", Min: 0, Max: 1, Unit: "ratio"},
		{Axis: "tolerance_corner", Target: "output", Min: .95, Max: 1.05, Unit: "ratio"},
	}}
	allAnalyses := operatingCaseCorners(operatingCase)
	if len(allAnalyses) != 5 {
		t.Fatalf("generic executable corners = %d, want nominal plus supply/ambient cross-product", len(allAnalyses))
	}
	electrical := operatingCaseCornersForAssertion(
		BehavioralAssertion{Analysis: simmodel.AnalysisDCOperatingPoint}, operatingCase,
	)
	if len(electrical) != 3 {
		t.Fatalf("electrical corners = %d, want nominal plus two supply endpoints", len(electrical))
	}
	for _, corner := range electrical {
		if corner.Values["ambient_temperature\x00assembly"] != 30 ||
			corner.Values["model_corner\x00output"] != .5 ||
			math.Abs(corner.Values["tolerance_corner\x00output"]-1) > 1e-12 {
			t.Fatalf("compacted corner lost deterministic midpoint values: %#v", corner)
		}
	}
	thermal := operatingCaseCornersForAssertion(
		BehavioralAssertion{Analysis: simmodel.AnalysisElectrothermal}, operatingCase,
	)
	if len(thermal) != 5 {
		t.Fatalf("electrothermal corners = %d, want nominal plus supply/ambient cross-product", len(thermal))
	}
}

func harnessFloat(value float64) *float64 { return &value }
