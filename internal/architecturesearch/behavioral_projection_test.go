package architecturesearch

import (
	"encoding/json"
	"math"
	"testing"
)

func TestThresholdProjectionUsesNearestCumulativeGainOnObservedPath(t *testing.T) {
	gainMinimum, gainMaximum := 95.0, 105.0
	thresholdMinimum, thresholdMaximum := .0095, .0105
	requirement := Requirement{
		Version: VersionV3,
		Requirements: Requirements{
			Domains: []Domain{{ID: "ground", Kind: "reference"}},
			Ports: []Port{
				{ID: "sensor", Direction: "sink", Domain: "ground"},
				{ID: "alarm", Direction: "source", Domain: "ground"},
				{ID: "unrelated_input", Direction: "sink", Domain: "ground"},
			},
			Signals: []Signal{{ID: "conditioned"}, {ID: "filtered"}, {ID: "unrelated"}},
			Objectives: []Objective{
				{ID: "amplify", Bindings: []Binding{{Port: "sensor", Direction: "sink"}, {Signal: "conditioned", Direction: "source"}}},
				{ID: "filter", Bindings: []Binding{{Signal: "conditioned", Direction: "sink"}, {Signal: "filtered", Direction: "source"}}},
				{ID: "decide", Bindings: []Binding{{Signal: "filtered", Direction: "sink"}, {Port: "alarm"}}},
				{ID: "unrelated_amplifier", Bindings: []Binding{{Port: "unrelated_input", Direction: "sink"}, {Signal: "unrelated", Direction: "source"}}},
			},
			BehavioralRequirements: []BehavioralRequirement{
				{ID: "gain", Metric: "voltage_gain", Observation: Observation{Kind: "signal", ID: "conditioned"}, Min: &gainMinimum, Max: &gainMaximum, Unit: "ratio"},
				{ID: "unrelated_gain", Metric: "voltage_gain", Observation: Observation{Kind: "signal", ID: "unrelated"}, Min: floatPointer(900), Max: floatPointer(1100), Unit: "ratio"},
				{ID: "threshold", Metric: "threshold_voltage", Observation: Observation{Kind: "port", ID: "alarm"}, Min: &thresholdMinimum, Max: &thresholdMaximum, Unit: "V"},
			},
		},
	}

	constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[2])
	constraint, ok := constraintByName(constraints, "threshold_voltage")
	if !ok {
		t.Fatalf("constraints = %#v", constraints)
	}
	var value float64
	if err := json.Unmarshal(constraint.Value, &value); err != nil {
		t.Fatal(err)
	}
	if math.Abs(value-1) > 1e-12 {
		t.Fatalf("projected local threshold = %.12g, want 1", value)
	}
}

func TestThresholdProjectionWithoutUpstreamGainRemainsInPublicDomain(t *testing.T) {
	minimum, maximum := 2.4, 2.6
	requirement := Requirement{
		Version: VersionV3,
		Requirements: Requirements{
			Domains:                []Domain{{ID: "ground", Kind: "reference"}},
			Ports:                  []Port{{ID: "sense", Direction: "sink", Domain: "ground"}, {ID: "alarm", Direction: "source", Domain: "ground"}},
			Objectives:             []Objective{{ID: "decide", Bindings: []Binding{{Port: "sense", Direction: "sink"}, {Port: "alarm"}}}},
			BehavioralRequirements: []BehavioralRequirement{{ID: "threshold", Metric: "threshold_voltage", Observation: Observation{Kind: "port", ID: "alarm"}, Min: &minimum, Max: &maximum, Unit: "V"}},
		},
	}
	constraint, ok := constraintByName(effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0]), "threshold_voltage")
	if !ok {
		t.Fatal("threshold constraint missing")
	}
	value, _, ok := projectedNumericValue(constraint)
	if !ok || math.Abs(value-2.5) > 1e-12 {
		t.Fatalf("projected threshold = %.12g ok=%t, want 2.5", value, ok)
	}
}

func TestThresholdProjectionUsesUpstreamGainWhenIndicatorProducesObservedAlarm(t *testing.T) {
	gainMinimum, gainMaximum := 95.0, 105.0
	thresholdMinimum, thresholdMaximum := .0095, .0105
	requirement := Requirement{
		Version: VersionV3,
		Requirements: Requirements{
			Domains: []Domain{{ID: "ground", Kind: "reference"}},
			Ports: []Port{
				{ID: "sensor", Direction: "sink", Domain: "ground"},
				{ID: "alarm", Direction: "source", Domain: "ground"},
			},
			Signals: []Signal{{ID: "conditioned"}, {ID: "filtered"}, {ID: "threshold_state"}},
			Objectives: []Objective{
				{ID: "amplify", Capability: "instrumentation_amplification", Bindings: []Binding{{Port: "sensor", Direction: "sink"}, {Signal: "conditioned", Direction: "source"}}},
				{ID: "filter", Capability: "frequency_filter", Bindings: []Binding{{Signal: "conditioned", Direction: "sink"}, {Signal: "filtered", Direction: "source"}}},
				{ID: "decide", Capability: "threshold_detection", Bindings: []Binding{{Signal: "filtered", Direction: "sink"}, {Signal: "threshold_state", Direction: "source"}}},
				{ID: "indicate", Capability: "fault_indication", Bindings: []Binding{{Signal: "threshold_state", Direction: "sink"}, {Port: "alarm", Direction: "source"}}},
			},
			BehavioralRequirements: []BehavioralRequirement{
				{ID: "gain", Metric: "voltage_gain", Observation: Observation{Kind: "signal", ID: "conditioned"}, Min: &gainMinimum, Max: &gainMaximum, Unit: "ratio"},
				{ID: "threshold", Metric: "threshold_voltage", Observation: Observation{Kind: "port", ID: "alarm"}, Min: &thresholdMinimum, Max: &thresholdMaximum, Unit: "V"},
			},
		},
	}

	constraint, ok := constraintByName(effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[2]), "threshold_voltage")
	if !ok {
		t.Fatal("threshold constraint missing")
	}
	value, _, ok := projectedNumericValue(constraint)
	if !ok || math.Abs(value-1) > 1e-12 {
		t.Fatalf("projected local threshold = %.12g ok=%t, want 1", value, ok)
	}
}

func TestCriticalStartupProjectsFailSafeInterlockThroughLoadObservation(t *testing.T) {
	maximum := .5
	requirement := Requirement{Version: VersionV3, Requirements: Requirements{
		Domains:                []Domain{{ID: "ground", Kind: "reference"}},
		Ports:                  []Port{{ID: "load", Direction: "source", Domain: "ground"}, {ID: "control", Direction: "sink", Domain: "ground"}},
		Objectives:             []Objective{{ID: "switch", Capability: "load_switch", Bindings: []Binding{{Role: "control", Port: "control"}, {Role: "output", Port: "load"}}}},
		BehavioralRequirements: []BehavioralRequirement{{ID: "startup", Metric: "startup_output_voltage", Analysis: "startup", Observation: Observation{Kind: "port", ID: "load"}, Max: &maximum, Critical: true}},
	}}
	constraint, ok := constraintByName(effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0]), "fail_safe_interlock")
	if !ok || constraint.Relation != "required" {
		t.Fatalf("fail-safe startup constraints = %#v", effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0]))
	}
}

func TestV4ProjectsDynamicInterfaceConstraintsToObservedBidirectionalBoundary(t *testing.T) {
	riseTime, loadMinimum, loadMaximum := 1e-6, 1e-12, 400e-12
	requirement := Requirement{Version: VersionV4, Requirements: Requirements{
		Domains: []Domain{
			{ID: "local_3v3", Kind: "supply"},
			{ID: "remote_1v8", Kind: "supply"},
		},
		Ports:   []Port{{ID: "remote_bus", Kind: "digital_bus", Direction: "bidirectional", Domain: "remote_1v8"}},
		Signals: []Signal{{ID: "local_bus", Kind: "digital_bus", Domain: "local_3v3"}},
		Objectives: []Objective{{
			ID: "isolate", Capability: "galvanic_isolation",
			Bindings: []Binding{
				{Role: "side_a", Signal: "local_bus", Direction: "bidirectional"},
				{Role: "side_b", Port: "remote_bus"},
			},
		}},
		OperatingCases: []OperatingCase{{Conditions: []OperatingCondition{{
			Axis: "load_capacitance", Target: "remote_bus", Min: &loadMinimum, Max: &loadMaximum, Unit: "F",
		}}}},
		BehavioralRequirements: []BehavioralRequirement{{
			ID: "rise", Metric: "rise_time", Analysis: "transient",
			Observation: Observation{Kind: "port", ID: "remote_bus"}, Max: &riseTime, Unit: "s",
		}},
	}}
	constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0])
	if _, ok := constraintByName(constraints, "rise_time"); !ok {
		t.Fatalf("rise-time constraint missing: %#v", constraints)
	}
	if _, ok := constraintByName(constraints, "load_capacitance"); !ok {
		t.Fatalf("load-capacitance constraint missing: %#v", constraints)
	}
	if _, ok := constraintByName(constraints, "side_b_rise_time"); !ok {
		t.Fatalf("side-B rise-time constraint missing: %#v", constraints)
	}
	if _, ok := constraintByName(constraints, "side_b_load_capacitance"); !ok {
		t.Fatalf("side-B load-capacitance constraint missing: %#v", constraints)
	}
	if _, ok := constraintByName(constraints, "side_a_load_capacitance"); ok {
		t.Fatalf("load capacitance was projected onto the unobserved side: %#v", constraints)
	}
}

func constraintByName(constraints []Constraint, name string) (Constraint, bool) {
	for _, constraint := range constraints {
		if constraint.Name == name {
			return constraint, true
		}
	}
	return Constraint{}, false
}
