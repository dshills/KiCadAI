package architecturesearch

import (
	"encoding/json"
	"errors"
	"testing"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

func TestRegulatorOutputCapacitorRoundsTransientRequirementUp(t *testing.T) {
	request := ProviderRequest{Constraints: []Constraint{
		numericConstraint("transient_load_current", "maximum", 0.2, "A", 0),
		numericConstraint("transient_duration", "maximum", 0.001, "s", 0),
		numericConstraint("maximum_voltage_droop", "maximum", 0.1, "V", 0),
	}}
	value, evidence, err := regulatorOutputCapacitor(request, regulatorWithCapWindow("1u", "10m"))
	if err != nil {
		t.Fatal(err)
	}
	if value != "2.2m" || evidence == nil {
		t.Fatalf("derived output capacitor = %q, evidence=%#v", value, evidence)
	}
	selected, ok := calculationSelectedValue(*evidence, "output_capacitance")
	if !ok || selected != 0.0022 || !evidence.Pass || len(ValidateCalculation(*evidence)) != 0 {
		t.Fatalf("transient evidence is not reproducible: %#v", evidence)
	}

	encoded, _ := json.Marshal(request)
	var replay ProviderRequest
	_ = json.Unmarshal(encoded, &replay)
	replayedValue, replayedEvidence, replayedErr := regulatorOutputCapacitor(replay, regulatorWithCapWindow("1u", "10m"))
	if replayedErr != nil || replayedValue != value || replayedEvidence.Hash != evidence.Hash {
		t.Fatalf("replay changed result: value=%q evidence=%#v err=%v", replayedValue, replayedEvidence, replayedErr)
	}
}

func TestRegulatorOutputCapacitorUsesCatalogStabilityMinimumWithoutTransient(t *testing.T) {
	value, evidence, err := regulatorOutputCapacitor(ProviderRequest{}, regulatorWithCapWindow("4.7u", ""))
	if err != nil || value != "4.7u" || evidence != nil {
		t.Fatalf("stability-only capacitor = %q, evidence=%#v, err=%v", value, evidence, err)
	}
}

func TestRegulatorOutputCapacitorRejectsPartialOrImpossibleTransient(t *testing.T) {
	partial := ProviderRequest{Constraints: []Constraint{numericConstraint("transient_load_current", "maximum", 0.2, "A", 0)}}
	_, _, err := regulatorOutputCapacitor(partial, regulatorWithCapWindow("1u", "10m"))
	assertPowerSynthesisCode(t, err, CodePowerTransientCapacitanceUnavailable)

	impossible := ProviderRequest{Constraints: []Constraint{
		numericConstraint("transient_load_current", "maximum", 1, "A", 0),
		numericConstraint("transient_duration", "maximum", 0.01, "s", 0),
		numericConstraint("maximum_voltage_droop", "maximum", 0.1, "V", 0),
	}}
	_, _, err = regulatorOutputCapacitor(impossible, regulatorWithCapWindow("1u", "1m"))
	assertPowerSynthesisCode(t, err, CodePowerTransientCapacitanceUnavailable)
}

func TestRegulatorOutputCapacitorRequiresStabilityEvidence(t *testing.T) {
	request := ProviderRequest{Constraints: []Constraint{
		numericConstraint("transient_load_current", "maximum", 0.2, "A", 0),
		numericConstraint("transient_duration", "maximum", 0.001, "s", 0),
		numericConstraint("maximum_voltage_droop", "maximum", 0.1, "V", 0),
	}}
	_, _, err := regulatorOutputCapacitor(request, components.ComponentRecord{})
	assertPowerSynthesisCode(t, err, CodePowerCapacitorStabilityUnproven)
}

func regulatorWithCapWindow(minimum, maximum string) components.ComponentRecord {
	return components.ComponentRecord{Regulator: &components.RegulatorEvidence{OutputCapacitor: &components.RegulatorCapacitorStability{
		MinCapacitance: minimum, MaxCapacitance: maximum, CapacitanceUnit: "F",
	}}}
}

func assertPowerSynthesisCode(t *testing.T, err error, expected reports.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s", expected)
	}
	var synthesis *powerSynthesisError
	if !errors.As(err, &synthesis) || synthesis.code != expected {
		t.Fatalf("rejection = %#v, want %s", err, expected)
	}
}
