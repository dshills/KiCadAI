package architecturesearch

import (
	"encoding/json"
	"math"
	"testing"

	"kicadai/internal/components"
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

func TestValidatePowerSequenceConstraintProvesMonotonicityAndInrush(t *testing.T) {
	requirement := powerTreeRequirement(false)
	selections := powerSequenceSelectionsWithStartupEvidence(t, .2)
	monotonic := Constraint{Name: "startup_monotonic", Relation: "required", Value: json.RawMessage(`"rail_a_signal"`)}
	check, validation := validatePowerSequenceConstraint(requirement, selections, monotonic, "monotonic")
	if validation != nil || check.Observed == nil || *check.Observed != 1 {
		t.Fatalf("monotonic proof = %#v validation=%#v", check, validation)
	}
	inrush := Constraint{Name: "startup_inrush_current", Relation: "maximum", Unit: "A", Value: json.RawMessage(`{"signal":"rail_a_signal","current_a":0.25}`)}
	check, validation = validatePowerSequenceConstraint(requirement, selections, inrush, "inrush")
	if validation != nil || check.Margin == nil || math.Abs(*check.Margin-.05) > 1e-12 {
		t.Fatalf("inrush proof = %#v validation=%#v", check, validation)
	}

	inrush.Value = json.RawMessage(`{"signal":"rail_a_signal","current_a":0.1}`)
	_, validation = validatePowerSequenceConstraint(requirement, selections, inrush, "inrush")
	if validation == nil || validation.Code != CodePowerSequenceUnproven {
		t.Fatalf("excess inrush validation = %#v", validation)
	}
}

func TestCatalogBehaviorCalculationsExposeTypedRegulatorStartupEvidence(t *testing.T) {
	request := ProviderRequest{Constraints: []Constraint{{Name: "startup_monotonic", Relation: "required", Value: json.RawMessage(`"rail"`)}}}
	parts := []catalogPart{{
		usage: "regulator", selected: SelectedComponent{InstanceID: "regulator"},
		record: components.ComponentRecord{Regulator: &components.RegulatorEvidence{
			StartupTime: &components.EvidenceMeasurement{Value: 2e-3, Unit: "s"}, StartupMonotonicStatus: "proven",
			MaximumInrushCurrent: &components.EvidenceMeasurement{Value: 200, Unit: "mA"},
		}},
	}}
	calculations, unproven, err := catalogBehaviorCalculations(request, parts)
	if err != nil || unproven != 0 {
		t.Fatalf("calculations error=%v unproven=%d", err, unproven)
	}
	want := map[string]float64{"startup_time": .002, "startup_monotonic": 1, "startup_inrush_current": .2}
	for _, calculation := range calculations {
		for _, output := range calculation.NominalOutputs {
			if expected, ok := want[output.Name]; ok && output.Value == expected {
				delete(want, output.Name)
			}
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing startup outputs %#v in %#v", want, calculations)
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

func powerSequenceSelectionsWithStartupEvidence(t *testing.T, inrush float64) []FragmentSelection {
	t.Helper()
	monotonic, err := ObservedCalculation("rail_a_monotonic", NamedQuantity{Name: "startup_monotonic", Value: 1, Unit: "ratio"})
	if err != nil {
		t.Fatal(err)
	}
	boundedInrush, err := ObservedCalculation("rail_a_inrush", NamedQuantity{Name: "startup_inrush_current", Value: inrush, Unit: "A"})
	if err != nil {
		t.Fatal(err)
	}
	return []FragmentSelection{{ObligationPath: "objective:make_a", Calculations: []CalculationEvidence{monotonic, boundedInrush}}}
}
