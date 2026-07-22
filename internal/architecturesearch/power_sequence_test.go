package architecturesearch

import (
	"encoding/json"
	"testing"
)

func TestValidatePowerSequenceConstraintUsesProducerStartupEvidence(t *testing.T) {
	requirement := powerTreeRequirement(false)
	selections := powerSequenceSelections(t, 0.001, 0.002)
	constraint := Constraint{Name: "rail_sequence_before", Relation: "required", Value: json.RawMessage(`["rail_a_signal","rail_b_signal"]`)}
	check, validation := validatePowerSequenceConstraint(requirement, selections, constraint, "candidate.system_constraints.rail_sequence_before")
	if validation != nil || check.Margin == nil || *check.Margin != 0.001 {
		t.Fatalf("sequence proof = %#v validation=%#v", check, validation)
	}

	delay := Constraint{Name: "rail_sequence_delay", Relation: "minimum", Unit: "s", Value: json.RawMessage(`{"before":"rail_a_signal","after":"rail_b_signal","seconds":0.0005}`)}
	check, validation = validatePowerSequenceConstraint(requirement, selections, delay, "candidate.system_constraints.rail_sequence_delay")
	if validation != nil || check.Observed == nil || *check.Observed != 0.001 {
		t.Fatalf("delay proof = %#v validation=%#v", check, validation)
	}
}

func TestValidatePowerSequenceConstraintFailsClosed(t *testing.T) {
	requirement := powerTreeRequirement(false)
	constraint := Constraint{Name: "rail_sequence_before", Relation: "required", Value: json.RawMessage(`["rail_a_signal","rail_b_signal"]`)}
	_, validation := validatePowerSequenceConstraint(requirement, powerSequenceSelections(t, 0.003, 0.002), constraint, "sequence")
	if validation == nil || validation.Code != CodePowerSequenceUnproven {
		t.Fatalf("reversed sequence validation = %#v", validation)
	}

	monotonic := Constraint{Name: "startup_monotonic", Relation: "required", Value: json.RawMessage(`"rail_a_signal"`)}
	_, validation = validatePowerSequenceConstraint(requirement, powerSequenceSelections(t, 0.001, 0.002), monotonic, "monotonic")
	if validation == nil || validation.Code != CodePowerSequenceUnproven {
		t.Fatalf("unproven monotonic startup = %#v", validation)
	}
}

func powerSequenceSelections(t *testing.T, first, second float64) []FragmentSelection {
	t.Helper()
	firstCalculation, err := ObservedCalculation("rail_a_startup", NamedQuantity{Name: "startup_time", Value: first, Unit: "s"})
	if err != nil {
		t.Fatal(err)
	}
	secondCalculation, err := ObservedCalculation("rail_b_startup", NamedQuantity{Name: "startup_time", Value: second, Unit: "s"})
	if err != nil {
		t.Fatal(err)
	}
	return []FragmentSelection{
		{ObligationPath: "objective:make_a", Calculations: []CalculationEvidence{firstCalculation}},
		{ObligationPath: "objective:make_b", Calculations: []CalculationEvidence{secondCalculation}},
	}
}
