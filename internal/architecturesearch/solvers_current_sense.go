package architecturesearch

import (
	"fmt"
	"math"

	"kicadai/internal/reports"
)

// CurrentSenseRequest describes a fixed-gain shunt measurement chain. All
// tolerances are absolute worst-case percentages, and offset is referred to
// the amplifier input.
type CurrentSenseRequest struct {
	ID                        string
	FullScaleCurrentA         float64
	TargetOutputVoltageV      float64
	OutputTolerancePercent    float64
	MaximumOutputVoltageV     float64
	ShuntResistanceOhm        float64
	ShuntTolerancePercent     float64
	ShuntPowerRatingW         float64
	AmplifierGain             float64
	AmplifierGainErrorPercent float64
	InputOffsetVoltageV       float64
}

// SolveCurrentSense evaluates nominal transfer, both signed error corners,
// output headroom, and shunt dissipation. It fails closed when any guaranteed
// corner exceeds the caller's measurement contract.
func SolveCurrentSense(request CurrentSenseRequest) (CalculationEvidence, []reports.Issue) {
	if !validSemanticID(canonicalIdentifier(request.ID)) || !finitePositive(request.FullScaleCurrentA) || !finitePositive(request.TargetOutputVoltageV) ||
		!finitePositive(request.OutputTolerancePercent) || !finitePositive(request.MaximumOutputVoltageV) || !finitePositive(request.ShuntResistanceOhm) ||
		!finiteNumbers(request.ShuntTolerancePercent, request.AmplifierGainErrorPercent, request.InputOffsetVoltageV) || request.ShuntTolerancePercent < 0 ||
		!finitePositive(request.ShuntPowerRatingW) || !finitePositive(request.AmplifierGain) || request.AmplifierGainErrorPercent < 0 || request.InputOffsetVoltageV < 0 {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "current_sense", "current-sense inputs must be finite and positive")
	}
	shuntLow := request.ShuntResistanceOhm * (1 - request.ShuntTolerancePercent/100)
	shuntHigh := request.ShuntResistanceOhm * (1 + request.ShuntTolerancePercent/100)
	gainLow := request.AmplifierGain * (1 - request.AmplifierGainErrorPercent/100)
	gainHigh := request.AmplifierGain * (1 + request.AmplifierGainErrorPercent/100)
	minimumOutput := request.FullScaleCurrentA*shuntLow*gainLow - request.InputOffsetVoltageV*gainHigh
	maximumOutput := request.FullScaleCurrentA*shuntHigh*gainHigh + request.InputOffsetVoltageV*gainHigh
	nominalOutput := request.FullScaleCurrentA * request.ShuntResistanceOhm * request.AmplifierGain
	allowedError := request.TargetOutputVoltageV * request.OutputTolerancePercent / 100
	minimumAllowed := request.TargetOutputVoltageV - allowedError
	maximumAllowed := request.TargetOutputVoltageV + allowedError
	shuntPower := request.FullScaleCurrentA * request.FullScaleCurrentA * shuntHigh
	bounds := []CalculationBound{
		minimumBound("full_scale_output_minimum", minimumAllowed, minimumOutput, "V"),
		maximumBound("full_scale_output_maximum", maximumAllowed, maximumOutput, "V"),
		maximumBound("measurement_output_headroom", request.MaximumOutputVoltageV, maximumOutput, "V"),
		maximumBound("shunt_power", request.ShuntPowerRatingW, shuntPower, "W"),
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	evidence := CalculationEvidence{
		ID: request.ID, FormulaID: FormulaCurrentSense, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "amplifier_gain", Value: request.AmplifierGain, Unit: "V/V"},
			{Name: "amplifier_gain_error", Value: request.AmplifierGainErrorPercent, Unit: "%"},
			{Name: "full_scale_current", Value: request.FullScaleCurrentA, Unit: "A"},
			{Name: "input_offset_voltage", Value: request.InputOffsetVoltageV, Unit: "V"},
			{Name: "shunt_resistance", Value: request.ShuntResistanceOhm, Unit: "Ohm"},
			{Name: "shunt_tolerance", Value: request.ShuntTolerancePercent, Unit: "%"},
		},
		SelectedValues: []SelectedValueEvidence{{Name: "shunt_resistance", Ideal: request.TargetOutputVoltageV / (request.FullScaleCurrentA * request.AmplifierGain), Selected: request.ShuntResistanceOhm, Unit: "Ohm", TolerancePercent: request.ShuntTolerancePercent, RelativeError: math.Abs(nominalOutput-request.TargetOutputVoltageV) / request.TargetOutputVoltageV}},
		NominalOutputs: []NamedQuantity{{Name: "full_scale_output", Value: nominalOutput, Unit: "V"}, {Name: "shunt_power", Value: request.FullScaleCurrentA * request.FullScaleCurrentA * request.ShuntResistanceOhm, Unit: "W"}},
		Corners: []CornerEvidence{
			{ID: "minimum_output", Outputs: []NamedQuantity{{Name: "full_scale_output", Value: minimumOutput, Unit: "V"}}},
			{ID: "maximum_output", Outputs: []NamedQuantity{{Name: "full_scale_output", Value: maximumOutput, Unit: "V"}, {Name: "shunt_power", Value: shuntPower, Unit: "W"}}},
		},
		Bounds: bounds, CornerEvaluations: 2, WorstMargin: worstMargin, Pass: pass,
	}
	evidence, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "current_sense", "finalize current-sense evidence: "+err.Error())
	}
	if !pass {
		return evidence, calculationIssue(CodeToleranceFailed, "current_sense", fmt.Sprintf("current-sense worst-case output or shunt power is outside the contract (margin %.6g)", worstMargin))
	}
	return evidence, nil
}
