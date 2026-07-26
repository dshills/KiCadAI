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

func TestBehavioralProjectionDoesNotCrossSupervisoryControlIntoSiblingRail(t *testing.T) {
	rail3Minimum, rail3Maximum := 3.2, 3.4
	rail5Minimum, rail5Maximum := 4.85, 5.15
	requirement := Requirement{Version: VersionV5, Requirements: Requirements{
		Domains: []Domain{
			{ID: "ground", Kind: "reference"},
			{ID: "rail_3v3", Kind: "supply"},
			{ID: "rail_5v", Kind: "supply"},
		},
		Ports: []Port{
			{ID: "input", Direction: "sink", Domain: "ground"},
			{ID: "output_3v3", Direction: "source", Domain: "rail_3v3"},
			{ID: "output_5v", Direction: "source", Domain: "rail_5v"},
		},
		Signals: []Signal{{ID: "rail_3v3_signal"}, {ID: "rail_5v_signal"}, {ID: "sequence_state"}},
		Objectives: []Objective{
			{ID: "generate_3v3", Bindings: []Binding{{Role: "input", Port: "input"}, {Role: "output", Signal: "rail_3v3_signal", Direction: "source"}}},
			{ID: "generate_5v", Bindings: []Binding{{Role: "input", Port: "input"}, {Role: "output", Signal: "rail_5v_signal", Direction: "source"}}},
			{ID: "sequence", Bindings: []Binding{{Role: "rail_a", Signal: "rail_3v3_signal", Direction: "sink"}, {Role: "rail_b", Signal: "rail_5v_signal", Direction: "sink"}, {Role: "state", Signal: "sequence_state", Direction: "source"}}},
			{ID: "protect_3v3", Bindings: []Binding{{Role: "input", Signal: "rail_3v3_signal", Direction: "sink"}, {Role: "control", Signal: "sequence_state", Direction: "sink"}, {Role: "output", Port: "output_3v3"}}},
			{ID: "protect_5v", Bindings: []Binding{{Role: "input", Signal: "rail_5v_signal", Direction: "sink"}, {Role: "control", Signal: "sequence_state", Direction: "sink"}, {Role: "output", Port: "output_5v"}}},
		},
		BehavioralRequirements: []BehavioralRequirement{
			{ID: "rail_3v3_voltage", Metric: "dc_voltage", Observation: Observation{Kind: "port", ID: "output_3v3"}, Min: &rail3Minimum, Max: &rail3Maximum, Unit: "V"},
			{ID: "rail_5v_voltage", Metric: "dc_voltage", Observation: Observation{Kind: "port", ID: "output_5v"}, Min: &rail5Minimum, Max: &rail5Maximum, Unit: "V"},
		},
	}}

	for index, expected := range map[int]float64{0: 3.3, 1: 5} {
		constraint, ok := constraintByName(effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[index]), "output_voltage")
		if !ok {
			t.Fatalf("generator %d output-voltage constraint is absent", index)
		}
		value, _, ok := projectedNumericValue(constraint)
		if !ok || math.Abs(value-expected) > 1e-12 {
			t.Fatalf("generator %d output voltage = %.12g, want %.12g", index, value, expected)
		}
	}
	if _, ok := constraintByName(effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[2]), "output_voltage"); ok {
		t.Fatal("sequencer inherited a downstream rail output-voltage constraint")
	}
}

func TestEventProjectionPreservesDistinctStartupAndShutdownDelayContracts(t *testing.T) {
	startupMinimum, startupMaximum := .002, .02
	shutdownMinimum, shutdownMaximum := .001, .02
	requirement := Requirement{Version: VersionV5, Requirements: Requirements{
		Objectives: []Objective{{ID: "sequence", Capability: "rail_sequencing"}},
		OperatingCases: []OperatingCase{{
			ID: "normal",
			Events: []OperatingEvent{
				{ID: "power_up", Kind: "startup"},
				{ID: "power_down", Kind: "shutdown"},
			},
		}},
		BehavioralRequirements: []BehavioralRequirement{
			{ID: "startup_sequence", Metric: "sequence_delay", Observation: Observation{Kind: "event", ID: "power_up"}, Min: &startupMinimum, Max: &startupMaximum, Unit: "s"},
			{ID: "shutdown_sequence", Metric: "sequence_delay", Observation: Observation{Kind: "event", ID: "power_down"}, Min: &shutdownMinimum, Max: &shutdownMaximum, Unit: "s"},
		},
	}}
	constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0])
	for name, expectedMinimum := range map[string]float64{
		"startup_delay":  startupMinimum,
		"shutdown_delay": shutdownMinimum,
	} {
		constraint, ok := constraintByName(constraints, name)
		if !ok {
			t.Fatalf("%s is absent from event-scoped constraints: %#v", name, constraints)
		}
		minimum, _, ok := numericConstraintBounds([]Constraint{constraint}, name)
		if !ok || math.Abs(minimum-expectedMinimum) > 1e-12 {
			t.Fatalf("%s minimum = %.12g ok=%t, want %.12g", name, minimum, ok, expectedMinimum)
		}
	}
}

func TestEventProjectionCarriesTransientSOADurationWithoutImplementationIdentity(t *testing.T) {
	minimumMargin := 1.2
	requirement := Requirement{Version: VersionV5, Requirements: Requirements{
		Objectives: []Objective{{
			ID: "protect", Capability: "output_protection",
			Bindings: []Binding{{Role: "output", Port: "load"}},
		}},
		Ports: []Port{{ID: "load", Kind: "power", Direction: "source", Domain: "rail"}},
		OperatingCases: []OperatingCase{
			{ID: "unrelated", Events: []OperatingEvent{{ID: "short", Kind: "short_circuit", DurationS: .5}}},
			{ID: "fault", Events: []OperatingEvent{{ID: "short", Kind: "short_circuit", DurationS: .02}}},
		},
		BehavioralRequirements: []BehavioralRequirement{{
			ID: "soa", Metric: "transient_soa_margin", Analysis: "electrothermal",
			Observation:    Observation{Kind: "event", ID: "short"},
			Min:            &minimumMargin,
			Unit:           "ratio",
			OperatingCases: []string{"fault"},
		}},
	}}
	constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0])
	duration, _, ok := firstNumericConstraint(constraints, "transient_soa_duration")
	if !ok || math.Abs(duration-.02) > 1e-15 {
		t.Fatalf("projected transient SOA duration = %.12g ok=%t; constraints=%#v", duration, ok, constraints)
	}
}

func TestLoadConditionProjectsToSupervisoryRoleOnSharedPowerDomain(t *testing.T) {
	loadMinimum, loadMaximum := .02, 1.0
	requirement := Requirement{Version: VersionV5, Requirements: Requirements{
		Domains: []Domain{
			{ID: "rail_a", Kind: "supply"},
			{ID: "rail_b", Kind: "supply"},
		},
		Ports: []Port{{ID: "output_b", Kind: "power", Direction: "source", Domain: "rail_b"}},
		Signals: []Signal{
			{ID: "rail_a_signal", Kind: "power", Domain: "rail_a"},
			{ID: "rail_b_signal", Kind: "power", Domain: "rail_b"},
		},
		Objectives: []Objective{{
			ID: "sequence", Capability: "rail_sequencing",
			Bindings: []Binding{
				{Role: "rail_a", Signal: "rail_a_signal", Direction: "sink"},
				{Role: "rail_b", Signal: "rail_b_signal", Direction: "sink"},
			},
		}},
		OperatingCases: []OperatingCase{{
			ID: "normal",
			Conditions: []OperatingCondition{{
				Axis: "load_current", Target: "output_b", Min: &loadMinimum, Max: &loadMaximum, Unit: "A",
			}},
		}},
	}}
	constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0])
	constraint, ok := constraintByName(constraints, "rail_b_load_current")
	if !ok {
		t.Fatalf("shared-domain rail load is absent: %#v", constraints)
	}
	minimum, maximum, ok := numericConstraintBounds([]Constraint{constraint}, "rail_b_load_current")
	if !ok || math.Abs(minimum-loadMinimum) > 1e-12 || math.Abs(maximum-loadMaximum) > 1e-12 {
		t.Fatalf("shared-domain rail load = %.12g..%.12g ok=%t, want %.12g..%.12g", minimum, maximum, ok, loadMinimum, loadMaximum)
	}
	if _, ok := constraintByName(constraints, "rail_a_load_current"); ok {
		t.Fatalf("load condition crossed into an unrelated power domain: %#v", constraints)
	}
}

func TestOperatingCaseLoadProjectionPreservesCompleteEnvelope(t *testing.T) {
	hotMinimum, hotMaximum := 4.0, 4.0
	reactiveMinimum, reactiveMaximum := 4.0, 8.0
	requirement := Requirement{Version: VersionV5, Requirements: Requirements{
		Domains: []Domain{{ID: "ground", Kind: "reference"}},
		Ports:   []Port{{ID: "output", Direction: "source", Domain: "ground"}},
		Objectives: []Objective{{
			ID: "drive", Capability: "class_ab_output_stage",
			Bindings: []Binding{{Role: "output", Port: "output"}},
		}},
		OperatingCases: []OperatingCase{
			{ID: "hot_fault", Conditions: []OperatingCondition{{
				Axis: "load_resistance", Target: "output", Min: &hotMinimum, Max: &hotMaximum, Unit: "Ohm",
			}}},
			{ID: "reactive_audio_load", Conditions: []OperatingCondition{{
				Axis: "load_resistance", Target: "output", Min: &reactiveMinimum, Max: &reactiveMaximum, Unit: "Ohm",
			}}},
		},
	}}
	constraint, ok := constraintByName(
		effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0]),
		"load_impedance",
	)
	if !ok {
		t.Fatal("load-impedance constraint is absent")
	}
	minimum, maximum, ok := numericConstraintBounds([]Constraint{constraint}, "load_impedance")
	if !ok || minimum > reactiveMinimum || math.Abs(maximum-reactiveMaximum) > 1e-12 {
		t.Fatalf("projected load envelope = %.12g..%.12g ok=%t, want a lower bound at most 4 and maximum 8", minimum, maximum, ok)
	}
}

func TestCurrentEventProjectsTransientStressOntoTargetPowerPath(t *testing.T) {
	initial, applied, recovered := 1.0, 6.0, 1.0
	requirement := Requirement{Version: VersionV5, Requirements: Requirements{
		Domains: []Domain{{ID: "supply", Kind: "supply"}, {ID: "ground", Kind: "reference"}},
		Ports: []Port{
			{ID: "power", Kind: "power", Direction: "sink", Domain: "supply"},
			{ID: "load", Kind: "switched_load", Direction: "source", Domain: "supply"},
		},
		Objectives: []Objective{{
			ID: "switch", Capability: "load_switch",
			Bindings: []Binding{{Role: "input", Port: "power"}, {Role: "output", Port: "load"}},
		}},
		OperatingCases: []OperatingCase{{
			ID: "fault",
			Events: []OperatingEvent{{
				ID: "overload", Kind: "overload", Target: Observation{Kind: "port", ID: "load"},
				Initial: &initial, Applied: &applied, Recovered: &recovered, Unit: "A",
			}},
		}},
	}}

	constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0])
	constraint, ok := constraintByName(constraints, "transient_load_current")
	if !ok {
		t.Fatalf("transient current stress is absent: %#v", constraints)
	}
	minimum, _, ok := numericConstraintBounds([]Constraint{constraint}, "transient_load_current")
	if !ok || minimum != 6 {
		t.Fatalf("transient current stress = %.12g ok=%t, want 6 A", minimum, ok)
	}
}

func TestCurrentEventStressRespectsDeliveredPeakCurrentLimit(t *testing.T) {
	applied, limited := 6.0, 3.5
	requirement := Requirement{Version: VersionV5, Requirements: Requirements{
		Ports: []Port{{ID: "load", Kind: "switched_load", Direction: "source", Domain: "supply"}},
		Objectives: []Objective{{
			ID: "switch", Capability: "load_switch",
			Bindings: []Binding{{Role: "output", Port: "load"}},
		}},
		OperatingCases: []OperatingCase{{
			ID: "fault",
			Events: []OperatingEvent{{
				ID: "overload", Kind: "overload", Target: Observation{Kind: "port", ID: "load"},
				Applied: &applied, Unit: "A",
			}},
		}},
		BehavioralRequirements: []BehavioralRequirement{{
			ID: "current_limit", Metric: "peak_device_current",
			Observation: Observation{Kind: "port", ID: "load"}, Max: &limited, Unit: "A",
			OperatingCases: []string{"fault"},
		}},
	}}

	constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0])
	constraint, ok := constraintByName(constraints, "transient_load_current")
	if !ok {
		t.Fatalf("transient current stress is absent: %#v", constraints)
	}
	minimum, _, ok := numericConstraintBounds([]Constraint{constraint}, "transient_load_current")
	if !ok || minimum != limited {
		t.Fatalf("transient current stress = %.12g ok=%t, want %.12g A", minimum, ok, limited)
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
