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

func constraintByName(constraints []Constraint, name string) (Constraint, bool) {
	for _, constraint := range constraints {
		if constraint.Name == name {
			return constraint, true
		}
	}
	return Constraint{}, false
}
