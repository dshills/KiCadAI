package architecturesearch

import (
	"fmt"
	"math"
	"slices"

	"kicadai/internal/reports"
)

// SallenKeyLowPassRequest describes one unity-gain, equal-resistance stage.
// C1 connects the resistor junction to the buffered output; C2 connects the
// non-inverting input to reference.
type SallenKeyLowPassRequest struct {
	ID                          string
	TargetFrequencyHz           float64
	FrequencyTolerancePercent   float64
	TargetQ                     float64
	QTolerancePercent           float64
	ResistanceOhm               float64
	ResistanceTolerancePercent  float64
	CapacitanceTolerancePercent float64
	CapacitanceSeries           PreferredSeries
	MinimumCapacitanceF         float64
	MaximumCapacitanceF         float64
	MaxCandidates               int
}

type evaluatedSallenKeyCandidate struct {
	c1, c2       float64
	nominalF     float64
	nominalQ     float64
	nominalError float64
	worstMargin  float64
	pass         bool
	corners      []CornerEvidence
	bounds       []CalculationBound
}

// ButterworthStageQ returns the ascending Q values for the second-order
// sections of an even-order Butterworth response.
func ButterworthStageQ(order, stage int) (float64, bool) {
	if order < 2 || order%2 != 0 || stage < 0 || stage >= order/2 {
		return 0, false
	}
	return 1 / (2 * math.Cos(float64(2*stage+1)*math.Pi/float64(2*order))), true
}

func SolveSallenKeyLowPass(request SallenKeyLowPassRequest) (CalculationEvidence, []reports.Issue) {
	if request.MaxCandidates == 0 {
		request.MaxCandidates = DefaultMaxValueCandidates
	}
	if request.CapacitanceSeries == "" {
		request.CapacitanceSeries = SeriesE96
	}
	if !finitePositive(request.TargetFrequencyHz) || !finitePositive(request.TargetQ) || !finitePositive(request.ResistanceOhm) ||
		!validPercentage(request.FrequencyTolerancePercent) || !validPercentage(request.QTolerancePercent) ||
		!validPercentage(request.ResistanceTolerancePercent) || !validPercentage(request.CapacitanceTolerancePercent) {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "sallen_key_low_pass", "stage frequency, Q, resistance, or tolerances are invalid")
	}
	productRoot := 1 / (2 * math.Pi * request.TargetFrequencyHz * request.ResistanceOhm)
	idealC1 := productRoot * 2 * request.TargetQ
	idealC2 := productRoot / (2 * request.TargetQ)
	minimumC1, maximumC1 := solverRange(idealC1, request.MinimumCapacitanceF, request.MaximumCapacitanceF)
	minimumC2, maximumC2 := solverRange(idealC2, request.MinimumCapacitanceF, request.MaximumCapacitanceF)
	c1Values, issues := PreferredValueCandidates(idealC1, request.CapacitanceSeries, minimumC1, maximumC1, request.MaxCandidates)
	if len(issues) != 0 {
		return CalculationEvidence{}, issues
	}
	c2Values, issues := PreferredValueCandidates(idealC2, request.CapacitanceSeries, minimumC2, maximumC2, request.MaxCandidates)
	if len(issues) != 0 {
		return CalculationEvidence{}, issues
	}
	evaluated := make([]evaluatedSallenKeyCandidate, 0, len(c1Values)*len(c2Values))
	for _, c1 := range c1Values {
		for _, c2 := range c2Values {
			evaluated = append(evaluated, evaluateSallenKeyCandidate(request, c1, c2))
		}
	}
	selected, ok := selectSallenKeyCandidate(evaluated)
	if !ok {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "sallen_key_low_pass", "stage candidate evaluation produced no finite result")
	}
	evidence := CalculationEvidence{
		ID: request.ID, FormulaID: FormulaSallenKeyLowPass, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "target_frequency", Value: request.TargetFrequencyHz, Unit: "Hz"},
			{Name: "target_q", Value: request.TargetQ, Unit: "ratio"},
			{Name: "resistance", Value: request.ResistanceOhm, Unit: "Ohm"},
		},
		SelectedValues: []SelectedValueEvidence{
			{Name: "capacitance_1", Ideal: quantize(idealC1), Selected: selected.c1, Unit: "F", Series: request.CapacitanceSeries, TolerancePercent: request.CapacitanceTolerancePercent, RelativeError: quantize(math.Abs(selected.c1-idealC1) / idealC1)},
			{Name: "capacitance_2", Ideal: quantize(idealC2), Selected: selected.c2, Unit: "F", Series: request.CapacitanceSeries, TolerancePercent: request.CapacitanceTolerancePercent, RelativeError: quantize(math.Abs(selected.c2-idealC2) / idealC2)},
		},
		NominalOutputs: []NamedQuantity{{Name: "natural_frequency", Value: selected.nominalF, Unit: "Hz"}, {Name: "quality_factor", Value: selected.nominalQ, Unit: "ratio"}},
		Corners:        selected.corners, Bounds: selected.bounds, CornerEvaluations: len(selected.corners),
		WorstMargin: selected.worstMargin, Pass: selected.pass,
	}
	finalized, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "sallen_key_low_pass", "finalize stage evidence: "+err.Error())
	}
	if !finalized.Pass {
		return finalized, calculationIssue(CodeToleranceFailed, "sallen_key_low_pass", "no preferred capacitor pair satisfies all frequency and Q corners")
	}
	return finalized, nil
}

func evaluateSallenKeyCandidate(request SallenKeyLowPassRequest, c1, c2 float64) evaluatedSallenKeyCandidate {
	nominalF, nominalQ := sallenKeyResponse(request.ResistanceOhm, request.ResistanceOhm, c1, c2)
	frequencyMinimum, frequencyMaximum := toleranceRange(request.TargetFrequencyHz, request.FrequencyTolerancePercent)
	qMinimum, qMaximum := toleranceRange(request.TargetQ, request.QTolerancePercent)
	worstFrequencyMinimum, worstQMinimum := math.Inf(1), math.Inf(1)
	worstFrequencyMaximum, worstQMaximum := math.Inf(-1), math.Inf(-1)
	var corners []CornerEvidence
	cornerIndex := 0
	for _, r1 := range toleranceEndpoints(request.ResistanceOhm, request.ResistanceTolerancePercent) {
		for _, r2 := range toleranceEndpoints(request.ResistanceOhm, request.ResistanceTolerancePercent) {
			for _, c1Corner := range toleranceEndpoints(c1, request.CapacitanceTolerancePercent) {
				for _, c2Corner := range toleranceEndpoints(c2, request.CapacitanceTolerancePercent) {
					frequency, quality := sallenKeyResponse(r1, r2, c1Corner, c2Corner)
					worstFrequencyMinimum = math.Min(worstFrequencyMinimum, frequency)
					worstFrequencyMaximum = math.Max(worstFrequencyMaximum, frequency)
					worstQMinimum = math.Min(worstQMinimum, quality)
					worstQMaximum = math.Max(worstQMaximum, quality)
					corners = append(corners, CornerEvidence{ID: fmt.Sprintf("corner_%02d", cornerIndex), Inputs: []NamedQuantity{{Name: "resistance_1", Value: r1, Unit: "Ohm"}, {Name: "resistance_2", Value: r2, Unit: "Ohm"}, {Name: "capacitance_1", Value: c1Corner, Unit: "F"}, {Name: "capacitance_2", Value: c2Corner, Unit: "F"}}, Outputs: []NamedQuantity{{Name: "natural_frequency", Value: frequency, Unit: "Hz"}, {Name: "quality_factor", Value: quality, Unit: "ratio"}}})
					cornerIndex++
				}
			}
		}
	}
	bounds := []CalculationBound{
		minimumBound("natural_frequency_minimum", frequencyMinimum, worstFrequencyMinimum, "Hz"),
		maximumBound("natural_frequency_maximum", frequencyMaximum, worstFrequencyMaximum, "Hz"),
		minimumBound("quality_factor_minimum", qMinimum, worstQMinimum, "ratio"),
		maximumBound("quality_factor_maximum", qMaximum, worstQMaximum, "ratio"),
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	return evaluatedSallenKeyCandidate{c1: c1, c2: c2, nominalF: nominalF, nominalQ: nominalQ, nominalError: math.Abs(nominalF-request.TargetFrequencyHz)/request.TargetFrequencyHz + math.Abs(nominalQ-request.TargetQ)/request.TargetQ, worstMargin: worstMargin, pass: pass, corners: corners, bounds: bounds}
}

func sallenKeyResponse(r1, r2, c1, c2 float64) (float64, float64) {
	root := math.Sqrt(r1 * r2 * c1 * c2)
	return 1 / (2 * math.Pi * root), root / (c2 * (r1 + r2))
}

func selectSallenKeyCandidate(candidates []evaluatedSallenKeyCandidate) (evaluatedSallenKeyCandidate, bool) {
	valid := append([]evaluatedSallenKeyCandidate(nil), candidates...)
	valid = slices.DeleteFunc(valid, func(candidate evaluatedSallenKeyCandidate) bool {
		return !finiteNumbers(candidate.c1, candidate.c2, candidate.nominalF, candidate.nominalQ, candidate.nominalError, candidate.worstMargin)
	})
	if len(valid) == 0 {
		return evaluatedSallenKeyCandidate{}, false
	}
	slices.SortStableFunc(valid, func(left, right evaluatedSallenKeyCandidate) int {
		if left.pass != right.pass {
			if left.pass {
				return -1
			}
			return 1
		}
		if left.worstMargin != right.worstMargin {
			if left.worstMargin > right.worstMargin {
				return -1
			}
			return 1
		}
		if left.nominalError != right.nominalError {
			if left.nominalError < right.nominalError {
				return -1
			}
			return 1
		}
		if left.c1 != right.c1 {
			if left.c1 < right.c1 {
				return -1
			}
			return 1
		}
		if left.c2 < right.c2 {
			return -1
		}
		if left.c2 > right.c2 {
			return 1
		}
		return 0
	})
	return valid[0], true
}
