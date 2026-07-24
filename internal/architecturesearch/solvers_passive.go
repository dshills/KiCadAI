package architecturesearch

import (
	"fmt"
	"math"
	"slices"

	"kicadai/internal/reports"
)

type DividerMode string

const (
	DividerAttenuator DividerMode = "attenuator"
	DividerFeedback   DividerMode = "feedback"
)

type DividerRequest struct {
	ID                     string
	Mode                   DividerMode
	SourceVoltageV         float64
	SourceTolerancePercent float64
	TargetVoltageV         float64
	TargetTolerancePercent float64
	LowerResistanceOhm     float64
	LowerTolerancePercent  float64
	UpperTolerancePercent  float64
	UpperSeries            PreferredSeries
	MinimumUpperOhm        float64
	MaximumUpperOhm        float64
	FeedbackBiasCurrentA   float64
	MinimumOutputV         float64
	MaximumOutputV         float64
	MaxCandidates          int
}

type RCPoleRequest struct {
	ID                       string
	TargetFrequencyHz        float64
	TargetTolerancePercent   float64
	FixedResistanceOhm       float64
	FixedCapacitanceF        float64
	FixedTolerancePercent    float64
	SelectedTolerancePercent float64
	SelectedSeries           PreferredSeries
	MinimumSelected          float64
	MaximumSelected          float64
	MaxCandidates            int
}

type evaluatedValueCandidate struct {
	value        float64
	nominal      float64
	nominalError float64
	worstMargin  float64
	pass         bool
	corners      []CornerEvidence
	bounds       []CalculationBound
}

func SolveDivider(request DividerRequest) (CalculationEvidence, []reports.Issue) {
	if request.MaxCandidates == 0 {
		request.MaxCandidates = DefaultMaxValueCandidates
	}
	if request.UpperSeries == "" {
		request.UpperSeries = SeriesE96
	}
	if !finitePositive(request.SourceVoltageV) || !finitePositive(request.TargetVoltageV) || !finitePositive(request.LowerResistanceOhm) || !validPercentage(request.SourceTolerancePercent) || !validPercentage(request.TargetTolerancePercent) || !validPercentage(request.LowerTolerancePercent) || !validPercentage(request.UpperTolerancePercent) ||
		!finiteNumbers(request.FeedbackBiasCurrentA, request.MinimumOutputV, request.MaximumOutputV) || request.FeedbackBiasCurrentA < 0 || request.MinimumOutputV < 0 || request.MaximumOutputV < 0 ||
		(request.MinimumOutputV > 0 && request.MaximumOutputV > 0 && request.MinimumOutputV >= request.MaximumOutputV) {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "divider", "divider voltages, resistance, and tolerances are invalid")
	}
	var formulaID string
	var idealUpper float64
	switch request.Mode {
	case DividerAttenuator:
		if request.FeedbackBiasCurrentA != 0 {
			return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "divider.feedback_bias_current_a", "attenuator dividers cannot declare feedback bias current")
		}
		if request.TargetVoltageV >= request.SourceVoltageV {
			return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "divider.target_voltage_v", "attenuator target must be below source voltage")
		}
		formulaID = FormulaDividerAttenuator
		idealUpper = request.LowerResistanceOhm * (request.SourceVoltageV/request.TargetVoltageV - 1)
	case DividerFeedback:
		if request.TargetVoltageV <= request.SourceVoltageV {
			return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "divider.target_voltage_v", "feedback target must exceed reference voltage")
		}
		formulaID = FormulaFeedbackDivider
		denominator := request.SourceVoltageV/request.LowerResistanceOhm + request.FeedbackBiasCurrentA
		idealUpper = (request.TargetVoltageV - request.SourceVoltageV) / denominator
		if request.FeedbackBiasCurrentA > 0 {
			formulaID = FormulaFeedbackBias
		}
	default:
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "divider.mode", "unsupported divider mode")
	}
	minimum, maximum := solverRange(idealUpper, request.MinimumUpperOhm, request.MaximumUpperOhm)
	candidates, issues := PreferredValueCandidates(idealUpper, request.UpperSeries, minimum, maximum, request.MaxCandidates)
	if len(issues) != 0 {
		return CalculationEvidence{}, issues
	}
	evaluated := make([]evaluatedValueCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		evaluated = append(evaluated, evaluateDividerCandidate(request, candidate))
	}
	selected, ok := selectEvaluatedCandidate(evaluated)
	if !ok {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "divider", "divider candidate evaluation produced no finite result")
	}
	evidence := CalculationEvidence{
		ID: request.ID, FormulaID: formulaID, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "source_voltage", Value: request.SourceVoltageV, Unit: "V"},
			{Name: "target_voltage", Value: request.TargetVoltageV, Unit: "V"},
			{Name: "lower_resistance", Value: request.LowerResistanceOhm, Unit: "Ohm"},
		},
		SelectedValues: []SelectedValueEvidence{{
			Name: "upper_resistance", Ideal: quantize(idealUpper), Selected: selected.value, Unit: "Ohm",
			Series: request.UpperSeries, TolerancePercent: request.UpperTolerancePercent,
			RelativeError: quantize(math.Abs(selected.value-idealUpper) / idealUpper),
		}},
		NominalOutputs: []NamedQuantity{{Name: "output_voltage", Value: selected.nominal, Unit: "V"}},
		Corners:        selected.corners, Bounds: selected.bounds, CornerEvaluations: len(selected.corners),
		WorstMargin: selected.worstMargin, Pass: selected.pass,
		RejectedCandidates: rejectedValueCandidates(evaluated, selected.value, "Ohm"),
	}
	if request.FeedbackBiasCurrentA > 0 {
		evidence.Inputs = append(evidence.Inputs, NamedQuantity{Name: "feedback_bias_current", Value: request.FeedbackBiasCurrentA, Unit: "A"})
	}
	evidence, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "divider", "finalize divider evidence: "+err.Error())
	}
	if !evidence.Pass {
		return evidence, calculationIssue(CodeToleranceFailed, "divider", "no preferred divider value satisfies all tolerance corners")
	}
	return evidence, nil
}

func evaluateDividerCandidate(request DividerRequest, upper float64) evaluatedValueCandidate {
	nominal := dividerOutput(request.Mode, request.SourceVoltageV, upper, request.LowerResistanceOhm, request.FeedbackBiasCurrentA)
	allowedMinimum, allowedMaximum := toleranceRange(request.TargetVoltageV, request.TargetTolerancePercent)
	if request.MinimumOutputV > 0 {
		allowedMinimum = request.MinimumOutputV
	}
	if request.MaximumOutputV > 0 {
		allowedMaximum = request.MaximumOutputV
	}
	sourceValues := toleranceEndpoints(request.SourceVoltageV, request.SourceTolerancePercent)
	upperValues := toleranceEndpoints(upper, request.UpperTolerancePercent)
	lowerValues := toleranceEndpoints(request.LowerResistanceOhm, request.LowerTolerancePercent)
	worstMinimum := math.Inf(1)
	worstMaximum := math.Inf(-1)
	var corners []CornerEvidence
	cornerIndex := 0
	for _, source := range sourceValues {
		for _, upperCorner := range upperValues {
			for _, lower := range lowerValues {
				output := dividerOutput(request.Mode, source, upperCorner, lower, request.FeedbackBiasCurrentA)
				worstMinimum = math.Min(worstMinimum, output)
				worstMaximum = math.Max(worstMaximum, output)
				inputs := []NamedQuantity{{Name: "source_voltage", Value: source, Unit: "V"}, {Name: "upper_resistance", Value: upperCorner, Unit: "Ohm"}, {Name: "lower_resistance", Value: lower, Unit: "Ohm"}}
				if request.FeedbackBiasCurrentA > 0 {
					inputs = append(inputs, NamedQuantity{Name: "feedback_bias_current", Value: request.FeedbackBiasCurrentA, Unit: "A"})
				}
				corners = append(corners, CornerEvidence{
					ID:      fmt.Sprintf("corner_%02d", cornerIndex),
					Inputs:  inputs,
					Outputs: []NamedQuantity{{Name: "output_voltage", Value: output, Unit: "V"}},
				})
				cornerIndex++
			}
		}
	}
	lowerMargin := worstMinimum - allowedMinimum
	upperMargin := allowedMaximum - worstMaximum
	worstMargin := math.Min(normalizedMargin(lowerMargin, allowedMinimum), normalizedMargin(upperMargin, allowedMaximum))
	return evaluatedValueCandidate{
		value: upper, nominal: nominal, nominalError: math.Abs(nominal - request.TargetVoltageV),
		worstMargin: worstMargin, pass: worstMargin >= 0, corners: corners,
		bounds: []CalculationBound{
			{Name: "output_voltage_minimum", Relation: "minimum", Required: allowedMinimum, ObservedWorst: worstMinimum, Margin: lowerMargin, Unit: "V", Pass: lowerMargin >= 0},
			{Name: "output_voltage_maximum", Relation: "maximum", Required: allowedMaximum, ObservedWorst: worstMaximum, Margin: upperMargin, Unit: "V", Pass: upperMargin >= 0},
		},
	}
}

func SolveRCPole(request RCPoleRequest) (CalculationEvidence, []reports.Issue) {
	if request.MaxCandidates == 0 {
		request.MaxCandidates = DefaultMaxValueCandidates
	}
	if request.SelectedSeries == "" {
		request.SelectedSeries = SeriesE96
	}
	fixedResistance := finitePositive(request.FixedResistanceOhm)
	fixedCapacitance := finitePositive(request.FixedCapacitanceF)
	if !finitePositive(request.TargetFrequencyHz) || fixedResistance == fixedCapacitance || !validPercentage(request.TargetTolerancePercent) || !validPercentage(request.FixedTolerancePercent) || !validPercentage(request.SelectedTolerancePercent) {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "rc_pole", "RC pole requires a target, exactly one fixed value, and valid tolerances")
	}
	var ideal, fixed float64
	var selectedName, selectedUnit, fixedName, fixedUnit string
	if fixedResistance {
		fixed = request.FixedResistanceOhm
		ideal = 1 / (2 * math.Pi * fixed * request.TargetFrequencyHz)
		selectedName, selectedUnit = "capacitance", "F"
		fixedName, fixedUnit = "resistance", "Ohm"
	} else {
		fixed = request.FixedCapacitanceF
		ideal = 1 / (2 * math.Pi * fixed * request.TargetFrequencyHz)
		selectedName, selectedUnit = "resistance", "Ohm"
		fixedName, fixedUnit = "capacitance", "F"
	}
	minimum, maximum := solverRange(ideal, request.MinimumSelected, request.MaximumSelected)
	candidates, issues := PreferredValueCandidates(ideal, request.SelectedSeries, minimum, maximum, request.MaxCandidates)
	if len(issues) != 0 {
		return CalculationEvidence{}, issues
	}
	evaluated := make([]evaluatedValueCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		evaluated = append(evaluated, evaluateRCCandidate(request, candidate))
	}
	selected, ok := selectEvaluatedCandidate(evaluated)
	if !ok {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "rc_pole", "RC candidate evaluation produced no finite result")
	}
	evidence := CalculationEvidence{
		ID: request.ID, FormulaID: FormulaRCPole, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{{Name: "target_frequency", Value: request.TargetFrequencyHz, Unit: "Hz"}, {Name: fixedName, Value: fixed, Unit: fixedUnit}},
		SelectedValues: []SelectedValueEvidence{{
			Name: selectedName, Ideal: quantize(ideal), Selected: selected.value, Unit: selectedUnit,
			Series: request.SelectedSeries, TolerancePercent: request.SelectedTolerancePercent,
			RelativeError: quantize(math.Abs(selected.value-ideal) / ideal),
		}},
		NominalOutputs: []NamedQuantity{{Name: "cutoff_frequency", Value: selected.nominal, Unit: "Hz"}},
		Corners:        selected.corners, Bounds: selected.bounds, CornerEvaluations: len(selected.corners),
		WorstMargin: selected.worstMargin, Pass: selected.pass,
		RejectedCandidates: rejectedValueCandidates(evaluated, selected.value, selectedUnit),
	}
	evidence, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "rc_pole", "finalize RC evidence: "+err.Error())
	}
	if !evidence.Pass {
		return evidence, calculationIssue(CodeToleranceFailed, "rc_pole", "no preferred RC value satisfies all tolerance corners")
	}
	return evidence, nil
}

func evaluateRCCandidate(request RCPoleRequest, selected float64) evaluatedValueCandidate {
	resistance := request.FixedResistanceOhm
	capacitance := selected
	if request.FixedCapacitanceF > 0 {
		resistance = selected
		capacitance = request.FixedCapacitanceF
	}
	nominal := 1 / (2 * math.Pi * resistance * capacitance)
	allowedMinimum, allowedMaximum := toleranceRange(request.TargetFrequencyHz, request.TargetTolerancePercent)
	resistanceTolerance := request.FixedTolerancePercent
	capacitanceTolerance := request.SelectedTolerancePercent
	if request.FixedCapacitanceF > 0 {
		resistanceTolerance = request.SelectedTolerancePercent
		capacitanceTolerance = request.FixedTolerancePercent
	}
	worstMinimum := math.Inf(1)
	worstMaximum := math.Inf(-1)
	var corners []CornerEvidence
	cornerIndex := 0
	for _, resistanceCorner := range toleranceEndpoints(resistance, resistanceTolerance) {
		for _, capacitanceCorner := range toleranceEndpoints(capacitance, capacitanceTolerance) {
			frequency := 1 / (2 * math.Pi * resistanceCorner * capacitanceCorner)
			worstMinimum = math.Min(worstMinimum, frequency)
			worstMaximum = math.Max(worstMaximum, frequency)
			corners = append(corners, CornerEvidence{
				ID:      fmt.Sprintf("corner_%02d", cornerIndex),
				Inputs:  []NamedQuantity{{Name: "resistance", Value: resistanceCorner, Unit: "Ohm"}, {Name: "capacitance", Value: capacitanceCorner, Unit: "F"}},
				Outputs: []NamedQuantity{{Name: "cutoff_frequency", Value: frequency, Unit: "Hz"}},
			})
			cornerIndex++
		}
	}
	lowerMargin := worstMinimum - allowedMinimum
	upperMargin := allowedMaximum - worstMaximum
	worstMargin := math.Min(normalizedMargin(lowerMargin, allowedMinimum), normalizedMargin(upperMargin, allowedMaximum))
	return evaluatedValueCandidate{
		value: selected, nominal: nominal, nominalError: math.Abs(nominal - request.TargetFrequencyHz),
		worstMargin: worstMargin, pass: worstMargin >= 0, corners: corners,
		bounds: []CalculationBound{
			{Name: "cutoff_frequency_minimum", Relation: "minimum", Required: allowedMinimum, ObservedWorst: worstMinimum, Margin: lowerMargin, Unit: "Hz", Pass: lowerMargin >= 0},
			{Name: "cutoff_frequency_maximum", Relation: "maximum", Required: allowedMaximum, ObservedWorst: worstMaximum, Margin: upperMargin, Unit: "Hz", Pass: upperMargin >= 0},
		},
	}
}

func dividerOutput(mode DividerMode, source, upper, lower, feedbackBiasCurrent float64) float64 {
	if mode == DividerFeedback {
		return source*(1+upper/lower) + feedbackBiasCurrent*upper
	}
	return source * lower / (upper + lower)
}

func selectEvaluatedCandidate(candidates []evaluatedValueCandidate) (evaluatedValueCandidate, bool) {
	valid := append([]evaluatedValueCandidate(nil), candidates...)
	valid = slices.DeleteFunc(valid, func(candidate evaluatedValueCandidate) bool {
		return !finiteNumbers(candidate.value, candidate.nominal, candidate.nominalError, candidate.worstMargin)
	})
	if len(valid) == 0 {
		return evaluatedValueCandidate{}, false
	}
	slices.SortStableFunc(valid, func(left, right evaluatedValueCandidate) int {
		if left.pass != right.pass {
			if left.pass {
				return -1
			}
			return 1
		}
		if left.worstMargin > right.worstMargin {
			return -1
		}
		if left.worstMargin < right.worstMargin {
			return 1
		}
		if left.nominalError < right.nominalError {
			return -1
		}
		if left.nominalError > right.nominalError {
			return 1
		}
		if left.value < right.value {
			return -1
		}
		if left.value > right.value {
			return 1
		}
		return 0
	})
	return valid[0], true
}

func rejectedValueCandidates(candidates []evaluatedValueCandidate, selected float64, unit string) []ValueCandidateRejection {
	var rejected []ValueCandidateRejection
	for _, candidate := range candidates {
		if candidate.value == selected {
			continue
		}
		code := "lower_rank"
		message := "candidate has lower deterministic score than selected value"
		if !candidate.pass {
			code = "tolerance_corner_failed"
			message = "candidate violates at least one worst-case tolerance bound"
		}
		rejected = append(rejected, ValueCandidateRejection{Value: candidate.value, Unit: unit, Code: code, Message: message, Margin: candidate.worstMargin})
	}
	return rejected
}

func solverRange(ideal, minimum, maximum float64) (float64, float64) {
	if !finitePositive(minimum) {
		minimum = ideal / 100
	}
	if !finitePositive(maximum) {
		maximum = ideal * 100
	}
	return minimum, maximum
}

func toleranceRange(nominal, percent float64) (float64, float64) {
	delta := nominal * percent / 100
	return nominal - delta, nominal + delta
}

func toleranceEndpoints(nominal, percent float64) []float64 {
	minimum, maximum := toleranceRange(nominal, percent)
	if minimum == maximum {
		return []float64{quantize(nominal)}
	}
	return []float64{quantize(minimum), quantize(maximum)}
}

func validPercentage(value float64) bool {
	return value >= 0 && value <= 100 && finiteNumbers(value)
}

func normalizedMargin(margin, scale float64) float64 {
	denominator := math.Abs(scale)
	if denominator < 1e-30 {
		denominator = 1
	}
	return quantize(margin / denominator)
}
