package architecturesearch

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

type HysteresisRequest struct {
	ID                               string
	TargetCenterV                    float64
	CenterTolerancePercent           float64
	TargetWidthV                     float64
	WidthTolerancePercent            float64
	OutputLowV                       float64
	OutputHighV                      float64
	OutputUncertaintyV               float64
	ReferenceResistanceOhm           float64
	ReferenceTolerancePercent        float64
	FeedbackTolerancePercent         float64
	ReferenceVoltageTolerancePercent float64
	MinimumReferenceVoltageV         float64
	MaximumReferenceVoltageV         float64
	FeedbackSeries                   PreferredSeries
	MinimumFeedbackOhm               float64
	MaximumFeedbackOhm               float64
	MaxCandidates                    int
}

type GateDriveRequest struct {
	ID                           string
	DriveVoltageV                float64
	DriveVoltageTolerancePercent float64
	GateChargeC                  float64
	GateChargeTolerancePercent   float64
	TargetRiseTimeS              float64
	RiseTimeTolerancePercent     float64
	SourceResistanceOhm          float64
	MaximumSourceCurrentA        float64
	ResistorTolerancePercent     float64
	ResistorSeries               PreferredSeries
	MinimumResistorOhm           float64
	MaximumResistorOhm           float64
	MaxCandidates                int
}

type RatingRequirement struct {
	Kind           string
	Required       float64
	Rated          float64
	DeratingFactor float64
	Unit           string
	Evidence       ContractEvidence
}

func SolveHysteresis(request HysteresisRequest) (CalculationEvidence, []reports.Issue) {
	if request.MaxCandidates == 0 {
		request.MaxCandidates = DefaultMaxValueCandidates
	}
	if request.FeedbackSeries == "" {
		request.FeedbackSeries = SeriesE96
	}
	if !finitePositive(request.TargetWidthV) || !finitePositive(request.ReferenceResistanceOhm) || request.OutputHighV <= request.OutputLowV || !finiteNumbers(request.TargetCenterV, request.OutputLowV, request.OutputHighV, request.OutputUncertaintyV) || request.OutputUncertaintyV < 0 || !validPercentage(request.CenterTolerancePercent) || !validPercentage(request.WidthTolerancePercent) || !validPercentage(request.ReferenceTolerancePercent) || !validPercentage(request.FeedbackTolerancePercent) || !validPercentage(request.ReferenceVoltageTolerancePercent) {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "hysteresis", "hysteresis targets, output levels, resistances, or tolerances are invalid")
	}
	alpha := request.TargetWidthV / (request.OutputHighV - request.OutputLowV)
	if alpha <= 0 || alpha >= 1 {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "hysteresis.target_width_v", "hysteresis width must be below the available output swing")
	}
	idealFeedback := request.ReferenceResistanceOhm * (1 - alpha) / alpha
	minimum, maximum := solverRange(idealFeedback, request.MinimumFeedbackOhm, request.MaximumFeedbackOhm)
	candidates, issues := PreferredValueCandidates(idealFeedback, request.FeedbackSeries, minimum, maximum, request.MaxCandidates)
	if len(issues) != 0 {
		return CalculationEvidence{}, issues
	}
	evaluated := make([]evaluatedValueCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		evaluated = append(evaluated, evaluateHysteresisCandidate(request, candidate))
	}
	selected, ok := selectEvaluatedCandidate(evaluated)
	if !ok {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "hysteresis", "hysteresis candidate evaluation produced no finite result")
	}
	selectedAlpha := request.ReferenceResistanceOhm / (request.ReferenceResistanceOhm + selected.value)
	referenceVoltage := requiredHysteresisReference(request, selectedAlpha)
	thresholdLow := hysteresisThreshold(referenceVoltage, request.OutputLowV, selectedAlpha)
	thresholdHigh := hysteresisThreshold(referenceVoltage, request.OutputHighV, selectedAlpha)
	evidence := CalculationEvidence{
		ID: request.ID, FormulaID: FormulaHysteresis, FormulaRevision: FormulaRevision,
		Inputs: []NamedQuantity{
			{Name: "target_center", Value: request.TargetCenterV, Unit: "V"},
			{Name: "target_width", Value: request.TargetWidthV, Unit: "V"},
			{Name: "output_low", Value: request.OutputLowV, Unit: "V"},
			{Name: "output_high", Value: request.OutputHighV, Unit: "V"},
			{Name: "reference_resistance", Value: request.ReferenceResistanceOhm, Unit: "Ohm"},
		},
		SelectedValues: []SelectedValueEvidence{{
			Name: "feedback_resistance", Ideal: quantize(idealFeedback), Selected: selected.value, Unit: "Ohm",
			Series: request.FeedbackSeries, TolerancePercent: request.FeedbackTolerancePercent,
			RelativeError: quantize(math.Abs(selected.value-idealFeedback) / idealFeedback),
		}},
		NominalOutputs: []NamedQuantity{
			{Name: "reference_voltage", Value: referenceVoltage, Unit: "V"},
			{Name: "threshold_low", Value: thresholdLow, Unit: "V"},
			{Name: "threshold_high", Value: thresholdHigh, Unit: "V"},
			{Name: "center_voltage", Value: (thresholdLow + thresholdHigh) / 2, Unit: "V"},
			{Name: "hysteresis_width", Value: thresholdHigh - thresholdLow, Unit: "V"},
		},
		Corners: selected.corners, Bounds: selected.bounds, CornerEvaluations: len(selected.corners),
		WorstMargin: selected.worstMargin, Pass: selected.pass,
		RejectedCandidates: rejectedValueCandidates(evaluated, selected.value, "Ohm"),
	}
	evidence, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "hysteresis", "finalize hysteresis evidence: "+err.Error())
	}
	if !evidence.Pass {
		return evidence, calculationIssue(CodeToleranceFailed, "hysteresis", "no preferred hysteresis value satisfies all tolerance corners")
	}
	return evidence, nil
}

func evaluateHysteresisCandidate(request HysteresisRequest, feedback float64) evaluatedValueCandidate {
	alpha := request.ReferenceResistanceOhm / (request.ReferenceResistanceOhm + feedback)
	referenceVoltage := requiredHysteresisReference(request, alpha)
	thresholdLow := hysteresisThreshold(referenceVoltage, request.OutputLowV, alpha)
	thresholdHigh := hysteresisThreshold(referenceVoltage, request.OutputHighV, alpha)
	nominalCenter := (thresholdLow + thresholdHigh) / 2
	nominalWidth := thresholdHigh - thresholdLow
	centerMinimum, centerMaximum := toleranceRange(request.TargetCenterV, request.CenterTolerancePercent)
	widthMinimum, widthMaximum := toleranceRange(request.TargetWidthV, request.WidthTolerancePercent)
	worstCenterMinimum, worstWidthMinimum := math.Inf(1), math.Inf(1)
	worstCenterMaximum, worstWidthMaximum := math.Inf(-1), math.Inf(-1)
	var corners []CornerEvidence
	cornerIndex := 0
	for _, referenceResistance := range toleranceEndpoints(request.ReferenceResistanceOhm, request.ReferenceTolerancePercent) {
		for _, feedbackResistance := range toleranceEndpoints(feedback, request.FeedbackTolerancePercent) {
			cornerAlpha := referenceResistance / (referenceResistance + feedbackResistance)
			for _, reference := range toleranceEndpoints(referenceVoltage, request.ReferenceVoltageTolerancePercent) {
				for _, outputLow := range absoluteEndpoints(request.OutputLowV, request.OutputUncertaintyV) {
					for _, outputHigh := range absoluteEndpoints(request.OutputHighV, request.OutputUncertaintyV) {
						low := hysteresisThreshold(reference, outputLow, cornerAlpha)
						high := hysteresisThreshold(reference, outputHigh, cornerAlpha)
						center := (low + high) / 2
						width := high - low
						worstCenterMinimum = math.Min(worstCenterMinimum, center)
						worstCenterMaximum = math.Max(worstCenterMaximum, center)
						worstWidthMinimum = math.Min(worstWidthMinimum, width)
						worstWidthMaximum = math.Max(worstWidthMaximum, width)
						corners = append(corners, CornerEvidence{
							ID:      fmt.Sprintf("corner_%03d", cornerIndex),
							Inputs:  []NamedQuantity{{Name: "reference_resistance", Value: referenceResistance, Unit: "Ohm"}, {Name: "feedback_resistance", Value: feedbackResistance, Unit: "Ohm"}, {Name: "reference_voltage", Value: reference, Unit: "V"}, {Name: "output_low", Value: outputLow, Unit: "V"}, {Name: "output_high", Value: outputHigh, Unit: "V"}},
							Outputs: []NamedQuantity{{Name: "threshold_low", Value: low, Unit: "V"}, {Name: "threshold_high", Value: high, Unit: "V"}, {Name: "center_voltage", Value: center, Unit: "V"}, {Name: "hysteresis_width", Value: width, Unit: "V"}},
						})
						cornerIndex++
					}
				}
			}
		}
	}
	bounds := []CalculationBound{
		minimumBound("center_voltage_minimum", centerMinimum, worstCenterMinimum, "V"),
		maximumBound("center_voltage_maximum", centerMaximum, worstCenterMaximum, "V"),
		minimumBound("hysteresis_width_minimum", widthMinimum, worstWidthMinimum, "V"),
		maximumBound("hysteresis_width_maximum", widthMaximum, worstWidthMaximum, "V"),
	}
	if finitePositive(request.MaximumReferenceVoltageV) || request.MinimumReferenceVoltageV != 0 {
		bounds = append(bounds, minimumBound("reference_voltage_minimum", request.MinimumReferenceVoltageV, referenceVoltage, "V"))
	}
	if finitePositive(request.MaximumReferenceVoltageV) {
		bounds = append(bounds, maximumBound("reference_voltage_maximum", request.MaximumReferenceVoltageV, referenceVoltage, "V"))
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	return evaluatedValueCandidate{
		value: feedback, nominal: nominalWidth,
		nominalError: math.Abs(nominalCenter-request.TargetCenterV) + math.Abs(nominalWidth-request.TargetWidthV),
		worstMargin:  worstMargin, pass: pass, corners: corners, bounds: bounds,
	}
}

func requiredHysteresisReference(request HysteresisRequest, alpha float64) float64 {
	return (request.TargetCenterV - alpha*(request.OutputLowV+request.OutputHighV)/2) / (1 - alpha)
}

func hysteresisThreshold(reference, output, alpha float64) float64 {
	return (1-alpha)*reference + alpha*output
}

func SolveGateDrive(request GateDriveRequest) (CalculationEvidence, []reports.Issue) {
	if request.MaxCandidates == 0 {
		request.MaxCandidates = DefaultMaxValueCandidates
	}
	if request.ResistorSeries == "" {
		request.ResistorSeries = SeriesE24
	}
	if !finitePositive(request.DriveVoltageV) || !finitePositive(request.GateChargeC) || !finitePositive(request.TargetRiseTimeS) || request.SourceResistanceOhm < 0 || !finitePositive(request.MaximumSourceCurrentA) || !validPercentage(request.DriveVoltageTolerancePercent) || !validPercentage(request.GateChargeTolerancePercent) || !validPercentage(request.RiseTimeTolerancePercent) || !validPercentage(request.ResistorTolerancePercent) {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "gate_drive", "gate-drive voltage, charge, timing, resistance, current, or tolerances are invalid")
	}
	ideal := request.DriveVoltageV*request.TargetRiseTimeS/request.GateChargeC - request.SourceResistanceOhm
	if !finitePositive(ideal) {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "gate_drive", "target rise time requires a non-positive external resistance")
	}
	minimum, maximum := solverRange(ideal, request.MinimumResistorOhm, request.MaximumResistorOhm)
	candidates, issues := PreferredValueCandidates(ideal, request.ResistorSeries, minimum, maximum, request.MaxCandidates)
	if len(issues) != 0 {
		return CalculationEvidence{}, issues
	}
	evaluated := make([]evaluatedValueCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		evaluated = append(evaluated, evaluateGateDriveCandidate(request, candidate))
	}
	selected, ok := selectEvaluatedCandidate(evaluated)
	if !ok {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "gate_drive", "gate-drive candidate evaluation produced no finite result")
	}
	nominalCurrent := request.DriveVoltageV / (request.SourceResistanceOhm + selected.value)
	nominalRise := request.GateChargeC * (request.SourceResistanceOhm + selected.value) / request.DriveVoltageV
	evidence := CalculationEvidence{
		ID: request.ID, FormulaID: FormulaGateDrive, FormulaRevision: FormulaRevision,
		Inputs:         []NamedQuantity{{Name: "drive_voltage", Value: request.DriveVoltageV, Unit: "V"}, {Name: "gate_charge", Value: request.GateChargeC, Unit: "C"}, {Name: "target_rise_time", Value: request.TargetRiseTimeS, Unit: "s"}, {Name: "source_resistance", Value: request.SourceResistanceOhm, Unit: "Ohm"}, {Name: "maximum_source_current", Value: request.MaximumSourceCurrentA, Unit: "A"}},
		SelectedValues: []SelectedValueEvidence{{Name: "gate_resistance", Ideal: quantize(ideal), Selected: selected.value, Unit: "Ohm", Series: request.ResistorSeries, TolerancePercent: request.ResistorTolerancePercent, RelativeError: quantize(math.Abs(selected.value-ideal) / ideal)}},
		NominalOutputs: []NamedQuantity{{Name: "peak_source_current", Value: nominalCurrent, Unit: "A"}, {Name: "rise_time", Value: nominalRise, Unit: "s"}},
		Corners:        selected.corners, Bounds: selected.bounds, CornerEvaluations: len(selected.corners),
		WorstMargin: selected.worstMargin, Pass: selected.pass,
		RejectedCandidates: rejectedValueCandidates(evaluated, selected.value, "Ohm"),
	}
	evidence, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "gate_drive", "finalize gate-drive evidence: "+err.Error())
	}
	if !evidence.Pass {
		return evidence, calculationIssue(CodeToleranceFailed, "gate_drive", "no preferred gate resistor satisfies current and rise-time corners")
	}
	return evidence, nil
}

func evaluateGateDriveCandidate(request GateDriveRequest, resistor float64) evaluatedValueCandidate {
	nominalRise := request.GateChargeC * (request.SourceResistanceOhm + resistor) / request.DriveVoltageV
	riseMinimum, riseMaximum := toleranceRange(request.TargetRiseTimeS, request.RiseTimeTolerancePercent)
	worstCurrent := math.Inf(-1)
	worstRiseMinimum, worstRiseMaximum := math.Inf(1), math.Inf(-1)
	var corners []CornerEvidence
	cornerIndex := 0
	for _, voltage := range toleranceEndpoints(request.DriveVoltageV, request.DriveVoltageTolerancePercent) {
		for _, charge := range toleranceEndpoints(request.GateChargeC, request.GateChargeTolerancePercent) {
			for _, resistorCorner := range toleranceEndpoints(resistor, request.ResistorTolerancePercent) {
				current := voltage / (request.SourceResistanceOhm + resistorCorner)
				rise := charge * (request.SourceResistanceOhm + resistorCorner) / voltage
				worstCurrent = math.Max(worstCurrent, current)
				worstRiseMinimum = math.Min(worstRiseMinimum, rise)
				worstRiseMaximum = math.Max(worstRiseMaximum, rise)
				corners = append(corners, CornerEvidence{ID: fmt.Sprintf("corner_%02d", cornerIndex), Inputs: []NamedQuantity{{Name: "drive_voltage", Value: voltage, Unit: "V"}, {Name: "gate_charge", Value: charge, Unit: "C"}, {Name: "gate_resistance", Value: resistorCorner, Unit: "Ohm"}}, Outputs: []NamedQuantity{{Name: "peak_source_current", Value: current, Unit: "A"}, {Name: "rise_time", Value: rise, Unit: "s"}}})
				cornerIndex++
			}
		}
	}
	bounds := []CalculationBound{
		maximumBound("peak_source_current_maximum", request.MaximumSourceCurrentA, worstCurrent, "A"),
		minimumBound("rise_time_minimum", riseMinimum, worstRiseMinimum, "s"),
		maximumBound("rise_time_maximum", riseMaximum, worstRiseMaximum, "s"),
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	return evaluatedValueCandidate{value: resistor, nominal: nominalRise, nominalError: math.Abs(nominalRise - request.TargetRiseTimeS), worstMargin: worstMargin, pass: pass, corners: corners, bounds: bounds}
}

func EvaluateRatings(id string, requirements []RatingRequirement) (CalculationEvidence, []reports.Issue) {
	if len(requirements) == 0 {
		return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, "ratings", "at least one rating requirement is required")
	}
	var inputs []NamedQuantity
	var outputs []NamedQuantity
	var bounds []CalculationBound
	for index, requirement := range requirements {
		if !validSemanticID(canonicalIdentifier(requirement.Kind)) || !finitePositive(requirement.Required) || !finitePositive(requirement.Rated) || !finitePositive(requirement.DeratingFactor) || requirement.DeratingFactor > 1 || !validEvidenceConfidence(requirement.Evidence.Confidence) || confidenceRank(requirement.Evidence.Confidence) < confidenceRank(EvidenceRuleInferred) {
			return CalculationEvidence{}, calculationIssue(CodeValueInputInvalid, fmt.Sprintf("ratings[%d]", index), "rating value, derating factor, unit, kind, or evidence is invalid")
		}
		derated := requirement.Rated * requirement.DeratingFactor
		name := canonicalIdentifier(requirement.Kind)
		inputs = append(inputs, NamedQuantity{Name: name + "_required", Value: requirement.Required, Unit: requirement.Unit}, NamedQuantity{Name: name + "_rated", Value: requirement.Rated, Unit: requirement.Unit})
		outputs = append(outputs, NamedQuantity{Name: name + "_derated", Value: derated, Unit: requirement.Unit})
		bounds = append(bounds, minimumBound(name+"_margin", requirement.Required, derated, requirement.Unit))
	}
	worstMargin, pass := normalizedBoundsMargin(bounds)
	evidence := CalculationEvidence{ID: id, FormulaID: FormulaRatingMargin, FormulaRevision: FormulaRevision, Inputs: inputs, NominalOutputs: outputs, Bounds: bounds, WorstMargin: worstMargin, Pass: pass}
	evidence, err := FinalizeCalculation(evidence)
	if err != nil {
		return CalculationEvidence{}, calculationIssue(CodeValueUnsolved, "ratings", "finalize rating evidence: "+err.Error())
	}
	if !pass {
		return evidence, calculationIssue(CodeRatingFailed, "ratings", "one or more derated component ratings are below the requirement")
	}
	return evidence, nil
}

func minimumBound(name string, required, observed float64, unit string) CalculationBound {
	margin := quantize(observed - required)
	return CalculationBound{Name: name, Relation: "minimum", Required: quantize(required), ObservedWorst: quantize(observed), Margin: margin, Unit: unit, Pass: margin >= 0}
}

func maximumBound(name string, required, observed float64, unit string) CalculationBound {
	margin := quantize(required - observed)
	return CalculationBound{Name: name, Relation: "maximum", Required: quantize(required), ObservedWorst: quantize(observed), Margin: margin, Unit: unit, Pass: margin >= 0}
}

func normalizedBoundsMargin(bounds []CalculationBound) (float64, bool) {
	worst := math.Inf(1)
	pass := true
	for _, bound := range bounds {
		worst = math.Min(worst, normalizedMargin(bound.Margin, bound.Required))
		if !bound.Pass {
			pass = false
		}
	}
	if len(bounds) == 0 {
		return 0, false
	}
	return quantize(worst), pass
}

func absoluteEndpoints(nominal, uncertainty float64) []float64 {
	if uncertainty == 0 {
		return []float64{quantize(nominal)}
	}
	return []float64{quantize(nominal - uncertainty), quantize(nominal + uncertainty)}
}

func sortCalculationEvidence(values []CalculationEvidence) {
	slices.SortStableFunc(values, func(left, right CalculationEvidence) int { return strings.Compare(left.ID, right.ID) })
}
