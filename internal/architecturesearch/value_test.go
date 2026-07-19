package architecturesearch

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPreferredValueCandidatesAreBoundedAndDeterministic(t *testing.T) {
	first, issues := PreferredValueCandidates(10500, SeriesE96, 1000, 100000, 5)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	second, issues := PreferredValueCandidates(10500, SeriesE96, 1000, 100000, 5)
	if len(issues) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeat candidates = %#v issues=%#v", second, issues)
	}
	if len(first) != 5 || first[0] != 10500 {
		t.Fatalf("candidates = %#v", first)
	}
	if _, issues := PreferredValueCandidates(10, "invented", 1, 100, 5); !containsIssue(issues, CodeValueInputInvalid, "preferred_values") {
		t.Fatalf("invalid-series issues = %#v", issues)
	}
	if _, issues := PreferredValueCandidates(10, SeriesE24, 10.01, 10.02, 5); !containsIssue(issues, CodeValueUnsolved, "preferred_values") {
		t.Fatalf("empty-range issues = %#v", issues)
	}
}

func TestDividerSolversSelectPreferredValuesAndProveToleranceCorners(t *testing.T) {
	attenuator, issues := SolveDivider(DividerRequest{
		ID: "input_divider", Mode: DividerAttenuator,
		SourceVoltageV: 12, SourceTolerancePercent: 1,
		TargetVoltageV: 5, TargetTolerancePercent: 4,
		LowerResistanceOhm: 10000, LowerTolerancePercent: 1, UpperTolerancePercent: 1,
		UpperSeries: SeriesE96, MinimumUpperOhm: 1000, MaximumUpperOhm: 100000,
	})
	if len(issues) != 0 || !attenuator.Pass {
		t.Fatalf("attenuator = %#v issues=%#v", attenuator, issues)
	}
	if attenuator.FormulaID != FormulaDividerAttenuator || len(attenuator.SelectedValues) != 1 || attenuator.SelectedValues[0].Selected != 14000 || attenuator.CornerEvaluations != 8 || len(attenuator.RejectedCandidates) == 0 {
		t.Fatalf("attenuator evidence = %#v", attenuator)
	}
	assertValidCalculation(t, attenuator)

	feedback, issues := SolveDivider(DividerRequest{
		ID: "regulator_feedback", Mode: DividerFeedback,
		SourceVoltageV: 1.25, SourceTolerancePercent: 1,
		TargetVoltageV: 5, TargetTolerancePercent: 4,
		LowerResistanceOhm: 10000, LowerTolerancePercent: 1, UpperTolerancePercent: 1,
		UpperSeries: SeriesE96, MinimumUpperOhm: 1000, MaximumUpperOhm: 100000,
	})
	if len(issues) != 0 || !feedback.Pass || feedback.FormulaID != FormulaFeedbackDivider {
		t.Fatalf("feedback = %#v issues=%#v", feedback, issues)
	}
	assertValidCalculation(t, feedback)

	failed, issues := SolveDivider(DividerRequest{
		ID: "tight_divider", Mode: DividerAttenuator,
		SourceVoltageV: 12, SourceTolerancePercent: 2,
		TargetVoltageV: 5, TargetTolerancePercent: 0.1,
		LowerResistanceOhm: 10000, LowerTolerancePercent: 5, UpperTolerancePercent: 5,
		UpperSeries: SeriesE24, MinimumUpperOhm: 1000, MaximumUpperOhm: 100000,
	})
	if failed.Pass || !containsIssue(issues, CodeToleranceFailed, "divider") {
		t.Fatalf("tight divider = %#v issues=%#v", failed, issues)
	}
	assertValidCalculation(t, failed)
}

func TestRCPoleSolverProvesFrequencyCorners(t *testing.T) {
	evidence, issues := SolveRCPole(RCPoleRequest{
		ID: "filter_pole", TargetFrequencyHz: 2000, TargetTolerancePercent: 5,
		FixedResistanceOhm: 10000, FixedTolerancePercent: 1,
		SelectedTolerancePercent: 1, SelectedSeries: SeriesE96,
		MinimumSelected: 1e-9, MaximumSelected: 100e-9,
	})
	if len(issues) != 0 || !evidence.Pass || evidence.FormulaID != FormulaRCPole || evidence.CornerEvaluations != 4 {
		t.Fatalf("RC evidence = %#v issues=%#v", evidence, issues)
	}
	if selected := evidence.SelectedValues[0].Selected; selected != 7.87e-9 && selected != 8.06e-9 {
		t.Fatalf("selected capacitance = %.12g", selected)
	}
	assertValidCalculation(t, evidence)

	resistorSelected, issues := SolveRCPole(RCPoleRequest{
		ID: "resistor_selected_pole", TargetFrequencyHz: 1000, TargetTolerancePercent: 5,
		FixedCapacitanceF: 10e-9, FixedTolerancePercent: 1,
		SelectedTolerancePercent: 1, SelectedSeries: SeriesE96,
		MinimumSelected: 1000, MaximumSelected: 1000000,
	})
	if len(issues) != 0 || !resistorSelected.Pass || resistorSelected.SelectedValues[0].Name != "resistance" {
		t.Fatalf("resistor-selected evidence = %#v issues=%#v", resistorSelected, issues)
	}
}

func TestHysteresisSolverPublishesThresholdAndReferenceEvidence(t *testing.T) {
	evidence, issues := SolveHysteresis(HysteresisRequest{
		ID: "threshold_hysteresis", TargetCenterV: 1.65, CenterTolerancePercent: 6,
		TargetWidthV: 0.2, WidthTolerancePercent: 20,
		OutputLowV: 0.1, OutputHighV: 4.9, OutputUncertaintyV: 0.03,
		ReferenceResistanceOhm: 10000, ReferenceTolerancePercent: 1,
		FeedbackTolerancePercent: 1, ReferenceVoltageTolerancePercent: 1,
		MinimumReferenceVoltageV: 0, MaximumReferenceVoltageV: 5,
		FeedbackSeries: SeriesE96, MinimumFeedbackOhm: 10000, MaximumFeedbackOhm: 1000000,
	})
	if len(issues) != 0 || !evidence.Pass || evidence.FormulaID != FormulaHysteresis || evidence.CornerEvaluations != 32 {
		t.Fatalf("hysteresis evidence = %#v issues=%#v", evidence, issues)
	}
	if !hasQuantity(evidence.NominalOutputs, "reference_voltage") || !hasQuantity(evidence.NominalOutputs, "threshold_low") || !hasQuantity(evidence.NominalOutputs, "threshold_high") || !hasQuantity(evidence.NominalOutputs, "hysteresis_width") {
		t.Fatalf("hysteresis outputs = %#v", evidence.NominalOutputs)
	}
	assertValidCalculation(t, evidence)
}

func TestGateDriveSolverChecksCurrentAndRiseTimeTogether(t *testing.T) {
	evidence, issues := SolveGateDrive(GateDriveRequest{
		ID: "mosfet_gate_drive", DriveVoltageV: 3.3, DriveVoltageTolerancePercent: 5,
		GateChargeC: 10e-9, GateChargeTolerancePercent: 5,
		TargetRiseTimeS: 1e-6, RiseTimeTolerancePercent: 20,
		SourceResistanceOhm: 10, MaximumSourceCurrentA: 0.012,
		ResistorTolerancePercent: 1, ResistorSeries: SeriesE96,
		MinimumResistorOhm: 10, MaximumResistorOhm: 1000,
	})
	if len(issues) != 0 || !evidence.Pass || evidence.FormulaID != FormulaGateDrive || evidence.CornerEvaluations != 8 {
		t.Fatalf("gate evidence = %#v issues=%#v", evidence, issues)
	}
	assertValidCalculation(t, evidence)

	failed, issues := SolveGateDrive(GateDriveRequest{
		ID: "overloaded_gate_drive", DriveVoltageV: 3.3,
		GateChargeC: 10e-9, TargetRiseTimeS: 1e-6, RiseTimeTolerancePercent: 5,
		SourceResistanceOhm: 10, MaximumSourceCurrentA: 0.005,
		ResistorTolerancePercent: 1, ResistorSeries: SeriesE24,
		MinimumResistorOhm: 10, MaximumResistorOhm: 1000,
	})
	if failed.Pass || !containsIssue(issues, CodeToleranceFailed, "gate_drive") {
		t.Fatalf("overloaded gate evidence = %#v issues=%#v", failed, issues)
	}
	assertValidCalculation(t, failed)
}

func TestRatingEvidenceFailsClosedAfterDerating(t *testing.T) {
	verified := ContractEvidence{Confidence: EvidenceVerified, Sources: []string{"datasheet"}}
	evidence, issues := EvaluateRatings("switch_ratings", []RatingRequirement{
		{Kind: "voltage", Required: 13.2, Rated: 30, DeratingFactor: 0.8, Unit: "V", Evidence: verified},
		{Kind: "current", Required: 2, Rated: 4, DeratingFactor: 0.7, Unit: "A", Evidence: verified},
	})
	if len(issues) != 0 || !evidence.Pass || len(evidence.Bounds) != 2 {
		t.Fatalf("rating evidence = %#v issues=%#v", evidence, issues)
	}
	assertValidCalculation(t, evidence)

	failed, issues := EvaluateRatings("weak_switch", []RatingRequirement{{Kind: "current", Required: 2, Rated: 2.2, DeratingFactor: 0.8, Unit: "A", Evidence: verified}})
	if failed.Pass || !containsIssue(issues, CodeRatingFailed, "ratings") {
		t.Fatalf("weak rating evidence = %#v issues=%#v", failed, issues)
	}
	assertValidCalculation(t, failed)

	if _, issues := EvaluateRatings("unknown_rating", []RatingRequirement{{Kind: "current", Required: 1, Rated: 2, DeratingFactor: 0.8, Unit: "A", Evidence: ContractEvidence{Confidence: EvidencePlaceholder}}}); !containsIssue(issues, CodeValueInputInvalid, "ratings") {
		t.Fatalf("placeholder rating issues = %#v", issues)
	}
}

func TestCalculationHashDetectsTamperingAndIsReplayStable(t *testing.T) {
	request := DividerRequest{ID: "stable_divider", Mode: DividerAttenuator, SourceVoltageV: 5, TargetVoltageV: 2.5, TargetTolerancePercent: 3, LowerResistanceOhm: 10000, LowerTolerancePercent: 1, UpperTolerancePercent: 1, UpperSeries: SeriesE96, MinimumUpperOhm: 1000, MaximumUpperOhm: 100000}
	first, issues := SolveDivider(request)
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	second, issues := SolveDivider(request)
	if len(issues) != 0 || !reflect.DeepEqual(first, second) {
		t.Fatalf("replay differs\nfirst=%#v\nsecond=%#v issues=%#v", first, second, issues)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if string(firstJSON) != string(secondJSON) || len(first.Hash) != 64 || len(FormulaLibraryHash()) != 64 {
		t.Fatalf("hash/replay evidence invalid: %s %s", first.Hash, FormulaLibraryHash())
	}
	tampered := first
	tampered.NominalOutputs[0].Value++
	if issues := ValidateCalculation(tampered); !containsIssue(issues, CodeValueInputInvalid, "calculation.hash") {
		t.Fatalf("tamper issues = %#v", issues)
	}
}

func assertValidCalculation(t *testing.T, evidence CalculationEvidence) {
	t.Helper()
	if issues := ValidateCalculation(evidence); len(issues) != 0 {
		t.Fatalf("calculation validation issues = %#v\nevidence=%#v", issues, evidence)
	}
}

func hasQuantity(values []NamedQuantity, name string) bool {
	for _, value := range values {
		if value.Name == name {
			return true
		}
	}
	return false
}
