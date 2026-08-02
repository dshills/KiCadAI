package architecturesearch

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestV6ControlTransitionValidatesPhysicalDirection(t *testing.T) {
	for _, test := range []struct {
		name      string
		polarity  string
		direction string
	}{
		{name: "active_high_asserts_rising", polarity: "active_high", direction: "rising"},
		{name: "active_low_asserts_falling", polarity: "active_low", direction: "falling"},
	} {
		t.Run(test.name, func(t *testing.T) {
			requirement := validControlRequirement(test.polarity, test.direction)
			if issues := Validate(requirement); len(issues) != 0 {
				t.Fatalf("validation issues = %#v", issues)
			}
		})
	}
}

func TestV6ControlTransitionRejectsContradictoryDirection(t *testing.T) {
	requirement := validControlRequirement("active_low", "rising")
	issues := Validate(requirement)
	if !containsIssue(issues, CodeControlInvalid, "control_transitions[0].direction") {
		t.Fatalf("validation issues = %#v", issues)
	}
}

func TestV6ControlRoleAndResponseTimingFailClosedWhenUnderconstrained(t *testing.T) {
	requirement := validControlRequirement("active_low", "falling")
	requirement.Requirements.Ports[3].Control = nil
	requirement.Requirements.ControlTransitions = nil
	requirement.Requirements.BehavioralRequirements[0].Transition = ""
	issues := Validate(requirement)
	if !containsIssue(issues, CodeControlInvalid, "bindings") || !containsIssue(issues, CodeControlInvalid, "behavioral_requirements[0].transition") {
		t.Fatalf("validation issues = %#v", issues)
	}
}

func TestV6BehaviorTimingMayNarrowTransitionEnvelope(t *testing.T) {
	requirement := validControlRequirement("active_low", "falling")
	requirement.Requirements.ControlTransitions[0].MinimumDelayS = floatPointer(.001)
	requirement.Requirements.BehavioralRequirements[0].Min = floatPointer(.002)
	requirement.Requirements.BehavioralRequirements[0].Max = floatPointer(.005)
	if issues := Validate(requirement); len(issues) != 0 {
		t.Fatalf("validation issues = %#v", issues)
	}
}

func TestV6BehaviorTimingRejectsBoundsOutsideTransitionEnvelope(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
		edit func(*Requirement)
	}{
		{name: "minimum", path: "behavioral_requirements[0].min", edit: func(requirement *Requirement) {
			requirement.Requirements.ControlTransitions[0].MinimumDelayS = floatPointer(.001)
			requirement.Requirements.BehavioralRequirements[0].Min = floatPointer(.0005)
		}},
		{name: "maximum", path: "behavioral_requirements[0].max", edit: func(requirement *Requirement) {
			requirement.Requirements.BehavioralRequirements[0].Max = floatPointer(.02)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			requirement := validControlRequirement("active_low", "falling")
			test.edit(&requirement)
			issues := Validate(requirement)
			if !containsIssue(issues, CodeControlInvalid, test.path) {
				t.Fatalf("validation issues = %#v", issues)
			}
		})
	}
}

func TestV6ControlNormalizationIsDeterministic(t *testing.T) {
	requirement := validControlRequirement(" ACTIVE_LOW ", " FALLING ")
	requirement.Requirements.Ports[3].Control.Function = " FAULT "
	requirement.Requirements.ControlTransitions[0].Dependencies = []ControlStateDependency{
		{Target: Observation{Kind: " PORT ", ID: " ALERT "}, State: " ASSERTED ", StableForS: .002},
		{Target: Observation{Kind: " PORT ", ID: " ALERT "}, State: " DEASSERTED ", StableForS: .001},
	}
	first := Normalize(requirement)
	second := Normalize(first)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("normalization is not idempotent\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestV6ControlSemanticsProjectGenericPolarityAndAction(t *testing.T) {
	requirement := validControlRequirement("active_low", "falling")
	constraints := effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[0])
	if got := optionalConstraintString(constraints, "output_polarity"); got != "active_low" {
		t.Fatalf("output polarity = %q", got)
	}
	constraints = effectiveObjectiveConstraints(requirement, requirement.Requirements.Objectives[1])
	if got := optionalConstraintString(constraints, "control_active_state"); got != "low_disconnect" {
		t.Fatalf("control active state = %q", got)
	}
}

func TestV6RejectsConnectedStartupAsDeenergizedProof(t *testing.T) {
	requirement := validControlRequirement("active_high", "rising")
	requirement.Requirements.Ports = append(requirement.Requirements.Ports, Port{ID: "load", Kind: "protected_output", Direction: "source", Domain: "vcc"})
	requirement.Requirements.Objectives[1].Bindings = append(requirement.Requirements.Objectives[1].Bindings, Binding{Role: "output", Port: "load"})
	requirement.Requirements.BehavioralRequirements = append(requirement.Requirements.BehavioralRequirements, BehavioralRequirement{
		ID: "startup_safe", Metric: "startup_output_voltage", Analysis: "startup", Observation: Observation{Kind: "port", ID: "load"},
		Max: floatPointer(.5), Unit: "V", OperatingCases: []string{"fault_case"}, Critical: true,
	})
	issues := Validate(requirement)
	if !containsIssue(issues, CodeControlInvalid, "behavioral_requirements[1]") {
		t.Fatalf("connected startup proof issues = %#v", issues)
	}
}

func validControlRequirement(polarity, direction string) Requirement {
	requirement := validRequirement()
	requirement.Schema, requirement.Version = SchemaIDV6, VersionV6
	requirement.Requirements.Ports[3].Control = &ControlSemantics{Function: "fault", Polarity: polarity, StartupState: "deasserted", SafeState: "asserted"}
	requirement.Requirements.Objectives = append(requirement.Requirements.Objectives, Objective{
		ID: "disconnect", Capability: "load_switch",
		Bindings:    []Binding{{Role: "control", Port: "alert"}, {Role: "power", Port: "power"}, {Role: "reference", Port: "ground"}},
		Constraints: []Constraint{{Name: "load_current", Relation: "minimum", Value: json.RawMessage(`0.01`), Unit: "A"}},
	})
	requirement.Requirements.OperatingCases = []OperatingCase{{
		ID:         "fault_case",
		Conditions: []OperatingCondition{{Axis: "supply_voltage", Target: "vcc", Min: floatPointer(4.75), Max: floatPointer(5.25), Unit: "V"}},
	}}
	requirement.Requirements.ControlTransitions = []ControlTransition{{
		ID: "fault_assertion", Target: Observation{Kind: "port", ID: "alert"}, Trigger: Observation{Kind: "port", ID: "sense"},
		From: "deasserted", To: "asserted", Direction: direction, MaximumDelayS: floatPointer(.01),
	}}
	requirement.Requirements.BehavioralRequirements = []BehavioralRequirement{{
		ID: "fault_response", Metric: "response_time", Analysis: "transient", Observation: Observation{Kind: "port", ID: "alert"},
		Max: floatPointer(.01), Unit: "s", OperatingCases: []string{"fault_case"}, Critical: true, Transition: "fault_assertion",
	}}
	requirement.Acceptance.RequireContractComposition = true
	requirement.Acceptance.RequireGlobalReasoning = true
	requirement.Acceptance.RequireCoverageAccounting = true
	requirement.Acceptance.RequireAlternatives = true
	requirement.Acceptance.RequireFailClosed = true
	requirement.Acceptance.RequireSimulation = true
	requirement.Acceptance.RequireAllCorners = true
	requirement.Acceptance.RequireModelProvenance = true
	requirement.Acceptance.RequireClosedLoopEvidence = true
	return requirement
}
