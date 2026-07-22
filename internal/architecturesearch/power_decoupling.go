package architecturesearch

import (
	"encoding/json"
	"fmt"
	"math"

	"kicadai/internal/components"
	"kicadai/internal/reports"
)

const (
	CodePowerCapacitorStabilityUnproven      reports.Code = "POWER_CAPACITOR_STABILITY_UNPROVEN"
	CodePowerTransientCapacitanceUnavailable reports.Code = "POWER_TRANSIENT_CAPACITANCE_UNAVAILABLE"
	CodePowerDropoutMarginUnavailable        reports.Code = "POWER_DROPOUT_MARGIN_UNAVAILABLE"
)

type powerSynthesisError struct {
	code    reports.Code
	message string
}

func (err *powerSynthesisError) Error() string { return err.message }

func (err *powerSynthesisError) ArchitectureRejectionCode() reports.Code { return err.code }

// regulatorOutputCapacitor derives the smallest deterministic E12 value that
// satisfies both the selected regulator's catalog-backed stability window and
// an optional bounded transient request. It deliberately does not choose a
// package or dielectric: those remain catalog/evidence decisions downstream.
func regulatorOutputCapacitor(request ProviderRequest, record components.ComponentRecord) (string, *CalculationEvidence, error) {
	hasTransientRequest := false
	for _, name := range []string{"transient_load_current", "transient_duration", "maximum_voltage_droop"} {
		if _, ok := namedConstraint(request.Constraints, name); ok {
			hasTransientRequest = true
		}
	}
	if record.Regulator == nil || record.Regulator.OutputCapacitor == nil {
		// Keep legacy regulation requests reproducible. New transient sizing is
		// intentionally fail-closed because it cannot prove compatibility with
		// an unknown regulator stability window.
		if !hasTransientRequest {
			return "1u", nil, nil
		}
		return "", nil, &powerSynthesisError{code: CodePowerCapacitorStabilityUnproven, message: "selected regulator lacks output-capacitor stability evidence"}
	}
	stability := record.Regulator.OutputCapacitor
	minimum, ok := components.ParseEngineeringValue(stability.MinCapacitance)
	if !ok || minimum <= 0 || canonicalUnit(stability.CapacitanceUnit) != "F" {
		return "", nil, &powerSynthesisError{code: CodePowerCapacitorStabilityUnproven, message: "selected regulator has invalid minimum output-capacitance evidence"}
	}
	maximum := 1.0
	if stability.MaxCapacitance != "" {
		maximum, ok = components.ParseEngineeringValue(stability.MaxCapacitance)
		if !ok || maximum < minimum {
			return "", nil, &powerSynthesisError{code: CodePowerCapacitorStabilityUnproven, message: "selected regulator has invalid output-capacitance stability window"}
		}
	}

	load, loadOK, loadErr := powerTransientConstraint(request.Constraints, "transient_load_current", "A")
	duration, durationOK, durationErr := powerTransientConstraint(request.Constraints, "transient_duration", "s")
	droop, droopOK, droopErr := powerTransientConstraint(request.Constraints, "maximum_voltage_droop", "V")
	if loadErr != nil || durationErr != nil || droopErr != nil || (loadOK || durationOK || droopOK) && !(loadOK && durationOK && droopOK) {
		return "", nil, &powerSynthesisError{code: CodePowerTransientCapacitanceUnavailable, message: "transient capacitance requires positive current, duration, and maximum droop constraints with compatible units"}
	}

	ideal := minimum
	if loadOK {
		ideal = math.Max(ideal, load*duration/droop)
	}
	values, issues := PreferredValueCandidates(ideal, SeriesE12, ideal, maximum, 1)
	if len(issues) != 0 || len(values) != 1 {
		code := CodePowerCapacitorStabilityUnproven
		if loadOK {
			code = CodePowerTransientCapacitanceUnavailable
		}
		return "", nil, &powerSynthesisError{code: code, message: "no preferred output-capacitor value satisfies the regulator stability and transient window"}
	}
	selected := values[0]
	if !loadOK {
		return engineeringValue(selected, "F"), nil, nil
	}

	observedDroop := load * duration / selected
	bounds := []CalculationBound{
		minimumBound("regulator_stability_minimum", minimum, selected, "F"),
		maximumBound("transient_voltage_droop", droop, observedDroop, "V"),
	}
	if stability.MaxCapacitance != "" {
		bounds = append(bounds, maximumBound("regulator_stability_maximum", maximum, selected, "F"))
	}
	worst := math.Inf(1)
	for _, bound := range bounds {
		worst = math.Min(worst, normalizedMargin(bound.Margin, math.Max(math.Abs(bound.Required), 1e-15)))
	}
	evidence := CalculationEvidence{
		ID: "regulator_output_capacitance", FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "transient_load_current", Value: load, Unit: "A"},
			{Name: "transient_duration", Value: duration, Unit: "s"},
			{Name: "maximum_voltage_droop", Value: droop, Unit: "V"},
			{Name: "regulator_minimum_capacitance", Value: minimum, Unit: "F"},
		},
		SelectedValues: []SelectedValueEvidence{{Name: "output_capacitance", Ideal: ideal, Selected: selected, Unit: "F", Series: SeriesE12, TolerancePercent: 0, RelativeError: (selected - ideal) / ideal}},
		NominalOutputs: []NamedQuantity{{Name: "transient_voltage_droop", Value: observedDroop, Unit: "V"}, {Name: "output_capacitance", Value: selected, Unit: "F"}},
		Bounds:         bounds, WorstMargin: quantize(worst), Pass: worst >= 0,
	}
	finalized, err := FinalizeCalculation(evidence)
	if err != nil {
		return "", nil, &powerSynthesisError{code: CodePowerTransientCapacitanceUnavailable, message: "could not finalize transient-capacitance evidence: " + err.Error()}
	}
	return engineeringValue(selected, "F"), &finalized, nil
}

func powerTransientConstraint(constraints []Constraint, name, unit string) (float64, bool, error) {
	constraint, ok := namedConstraint(constraints, name)
	if !ok {
		return 0, false, nil
	}
	var value float64
	if constraint.Relation != "maximum" || json.Unmarshal(constraint.Value, &value) != nil || !finitePositive(value) || canonicalUnit(constraint.Unit) != canonicalUnit(unit) {
		return 0, true, fmt.Errorf("invalid %s", name)
	}
	return value, true, nil
}
