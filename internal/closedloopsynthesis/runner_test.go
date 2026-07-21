package closedloopsynthesis

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

type evaluatorFunc func(context.Context, CandidateState) (Evaluation, error)

func (function evaluatorFunc) Evaluate(ctx context.Context, state CandidateState) (Evaluation, error) {
	return function(ctx, state)
}

func TestClosedLoopRepairsByStrictWholeReportImprovementAndReplays(t *testing.T) {
	requirement := closedLoopTestRequirement()
	candidate := Candidate{Fingerprint: testHash("candidate-a"), Variables: []Variable{{ID: "gain_resistance", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2, 3}, Effects: []RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases}}}}}
	evaluator := closedLoopTestEvaluator(false)
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{candidate}}
	first := Run(context.Background(), input, evaluator, DefaultPolicy())
	if first.Status != "pass" || first.StopReason != StopPassed || first.Selected == nil {
		t.Fatalf("closed loop = %#v", first)
	}
	if len(first.Candidates) != 1 || len(first.Candidates[0].Repairs) != 1 || first.Selected.State.Variables[0].Value != 2 {
		t.Fatalf("repair evidence = %#v", first.Candidates)
	}
	if first.Consumption.Evaluations != 3 || first.Consumption.RepairTrials != 2 || first.Consumption.RepairsApplied != 1 {
		t.Fatalf("consumption = %#v", first.Consumption)
	}
	repair := first.Candidates[0].Repairs[0]
	if repair.RequirementID != "gain" || repair.Analysis != simmodel.AnalysisACSweep || repair.Metric != "voltage_gain" || repair.Direction != "increase" || repair.AllowedMinimum != 1 || repair.AllowedMaximum != 3 {
		t.Fatalf("typed repair authorization evidence = %#v", repair)
	}
	if diagnostics := ValidatePromotionReport(first, input.CatalogHash); len(diagnostics) != 0 {
		t.Fatalf("passing report promotion diagnostics = %#v", diagnostics)
	}
	tampered := CloneReport(first)
	tampered.Selected.State.Variables[0].Value = 3
	if diagnostics := ValidatePromotionReport(tampered, input.CatalogHash); len(diagnostics) == 0 {
		t.Fatal("tampered selected state was accepted for promotion")
	}

	reordered := input
	reordered.Candidates = cloneCandidates(input.Candidates)
	slices.Reverse(reordered.Candidates[0].Variables[0].AllowedValues)
	// Unsorted allowed values are rejected rather than silently normalized.
	invalid := Run(context.Background(), reordered, evaluator, DefaultPolicy())
	if invalid.Status != "blocked" || invalid.StopReason != StopInvalidInput {
		t.Fatalf("noncanonical repair domain was accepted: %#v", invalid)
	}
	second := Run(context.Background(), input, evaluator, DefaultPolicy())
	firstBytes, err := MarshalReport(first)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := MarshalReport(second)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatalf("closed-loop replay differs\nfirst: %s\nsecond: %s", firstBytes, secondBytes)
	}
}

func TestClosedLoopCoordinatesTwoVariablesWhenSinglesRegressWholeReport(t *testing.T) {
	requirement := closedLoopTestRequirement()
	minimumCombined, balanceMinimum, balanceMaximum := 4.0, -0.1, 0.1
	requirement.Requirements.BehavioralRequirements = []architecturesearch.BehavioralRequirement{
		{ID: "gain", Metric: "voltage_gain", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimumCombined, Unit: "ratio", OperatingCases: []string{"rated"}},
		{ID: "bandwidth", Metric: "bandwidth", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimumCombined, Unit: "Hz", OperatingCases: []string{"rated"}},
		{ID: "balance", Metric: "phase_margin", Analysis: simmodel.AnalysisStability, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &balanceMinimum, Max: &balanceMaximum, Unit: "deg", OperatingCases: []string{"rated"}},
	}
	effects := []RepairEffect{
		{Analysis: simmodel.AnalysisACSweep, Metric: "bandwidth", Direction: RepairMetricIncreases},
		{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases},
	}
	candidate := Candidate{Fingerprint: testHash("coordinated"), Variables: []Variable{
		{ID: "collector", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2}, Effects: effects},
		{ID: "emitter", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2}, Effects: effects},
	}}
	evaluator := evaluatorFunc(func(_ context.Context, state CandidateState) (Evaluation, error) {
		x, y := state.Variables[0].Value, state.Variables[1].Value
		simulation := &SimulationEvidence{}
		evidenceHash, _ := HashSimulationEvidence(*simulation)
		return Evaluation{
			EvidenceHash: evidenceHash,
			Measurements: []Measurement{
				{RequirementID: "gain", OperatingCase: "rated", Actual: x + y},
				{RequirementID: "bandwidth", OperatingCase: "rated", Actual: x + y},
				{RequirementID: "balance", OperatingCase: "rated", Actual: x - y},
			},
			Simulation: simulation,
			ModelDecisions: []ModelDecision{{
				Component: "network", Family: "resistor", Status: "used", Reason: "trusted coordinated repair test",
				RequiredAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisStability}, Claim: simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1},
				Provenance: &simmodel.ModelProvenance{Source: "manufacturer-datasheet:test", Revision: "rev-a", SHA256: testHash("coordinated-model"), ReviewStatus: "reviewed", AllowedAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisStability}},
			}},
		}, nil
	})
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{candidate}}
	report := Run(context.Background(), input, evaluator, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || len(report.Candidates[0].Repairs) != 1 {
		t.Fatalf("coordinated closed loop = %#v", report)
	}
	repair := report.Candidates[0].Repairs[0]
	if len(repair.Changes) != 2 || repair.Changes[0].Variable != "collector" || repair.Changes[1].Variable != "emitter" || report.Selected.State.Variables[0].Value != 2 || report.Selected.State.Variables[1].Value != 2 {
		t.Fatalf("coordinated repair evidence = %#v selected=%#v", repair, report.Selected.State)
	}
}

func TestClosedLoopDoesNotTrialVariableWithoutMatchingAuthorizedEffect(t *testing.T) {
	requirement := closedLoopTestRequirement()
	candidate := Candidate{Fingerprint: testHash("candidate"), Variables: []Variable{{
		ID: "temperature_only", Kind: "bias", Value: 1, AllowedValues: []float64{1, 2},
		Effects: []RepairEffect{{Analysis: simmodel.AnalysisThermal, Metric: "junction_temperature", Direction: RepairMetricDecreases}},
	}}}
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{candidate}}
	report := Run(context.Background(), input, closedLoopTestEvaluator(false), DefaultPolicy())
	if report.Status != "blocked" || report.Candidates[0].StopReason != StopUnsupportedDiagnosis || report.Consumption.Evaluations != 1 || report.Consumption.RepairTrials != 0 {
		t.Fatalf("unauthorized repair trial was not rejected before evaluation: %#v", report)
	}
}

func TestClosedLoopRejectsMissingModelTrustAndIncompleteAssertions(t *testing.T) {
	requirement := closedLoopTestRequirement()
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{{Fingerprint: testHash("candidate")}}}
	missingTrust := Run(context.Background(), input, evaluatorFunc(func(_ context.Context, state CandidateState) (Evaluation, error) {
		return Evaluation{EvidenceHash: stateHash(state), Measurements: closedLoopMeasurements(10, 80)}, nil
	}), DefaultPolicy())
	if missingTrust.Status != "blocked" || missingTrust.Candidates[0].StopReason != StopModelTrustFailed {
		t.Fatalf("missing trust did not fail closed: %#v", missingTrust)
	}
	incomplete := Run(context.Background(), input, evaluatorFunc(func(_ context.Context, state CandidateState) (Evaluation, error) {
		evaluation, _ := closedLoopTestEvaluator(false).Evaluate(context.Background(), state)
		evaluation.Measurements = evaluation.Measurements[:1]
		return evaluation, nil
	}), DefaultPolicy())
	if incomplete.Status != "blocked" || incomplete.Candidates[0].StopReason != StopAssertionIncomplete {
		t.Fatalf("incomplete assertion coverage did not fail closed: %#v", incomplete)
	}
}

func TestClosedLoopWillNotTradeCriticalThermalFailureForGainRepair(t *testing.T) {
	requirement := closedLoopTestRequirement()
	candidate := Candidate{Fingerprint: testHash("candidate"), Variables: []Variable{{ID: "gain_resistance", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2, 3}, Effects: []RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases}, {Analysis: simmodel.AnalysisThermal, Metric: "junction_temperature", Direction: RepairMetricIncreases}}}}}
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{candidate}}
	report := Run(context.Background(), input, closedLoopTestEvaluator(true), DefaultPolicy())
	if report.Status != "blocked" || report.Candidates[0].StopReason != StopNonImprovement || len(report.Candidates[0].Repairs) != 0 {
		t.Fatalf("unsafe tradeoff was accepted: %#v", report)
	}
}

func TestClosedLoopBoundsEvaluationErrorsAndBudget(t *testing.T) {
	requirement := closedLoopTestRequirement()
	input := Input{Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formulas"), ModelRegistryHash: testHash("models"), Candidates: []Candidate{{Fingerprint: testHash("candidate")}}}
	failure := Run(context.Background(), input, evaluatorFunc(func(context.Context, CandidateState) (Evaluation, error) {
		return Evaluation{}, errors.New("solver failed")
	}), DefaultPolicy())
	if failure.Candidates[0].StopReason != StopEvaluationFailed {
		t.Fatalf("evaluation error = %#v", failure)
	}
	policy := DefaultPolicy()
	policy.MaxEvaluations = 1
	input.Candidates[0].Variables = []Variable{{ID: "gain", Kind: "gain", Value: 1, AllowedValues: []float64{1, 2}, Effects: []RepairEffect{{Analysis: simmodel.AnalysisACSweep, Metric: "voltage_gain", Direction: RepairMetricIncreases}}}}
	exhausted := Run(context.Background(), input, closedLoopTestEvaluator(false), policy)
	if exhausted.Candidates[0].StopReason != StopBudgetExhausted || !exhausted.Consumption.BudgetExhausted {
		t.Fatalf("evaluation budget = %#v", exhausted)
	}
}

func closedLoopTestEvaluator(thermalRegression bool) evaluatorFunc {
	return func(_ context.Context, state CandidateState) (Evaluation, error) {
		gain := 10.0
		if len(state.Variables) != 0 {
			gain = state.Variables[0].Value * 5
		}
		temperature := 80.0
		if thermalRegression && gain >= 10 {
			temperature = 130
		}
		simulation := &SimulationEvidence{}
		evidenceHash, _ := HashSimulationEvidence(*simulation)
		return Evaluation{
			EvidenceHash: evidenceHash, Measurements: closedLoopMeasurements(gain, temperature), Simulation: simulation,
			ModelDecisions: []ModelDecision{{
				Component: "r1", Family: "resistor", Status: "used", Reason: "trusted full behavioral evaluation",
				RequiredAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal},
				Claim:            simmodel.CatalogEvidence{ModelID: simmodel.PrimitiveResistorV1}, Provenance: &simmodel.ModelProvenance{
					Source: "manufacturer-datasheet:test", Revision: "rev-a", SHA256: testHash("model"), ReviewStatus: "reviewed",
					AllowedAnalyses: []string{simmodel.AnalysisACSweep, simmodel.AnalysisThermal},
				},
			}},
		}, nil
	}
}

func closedLoopMeasurements(gain, temperature float64) []Measurement {
	return []Measurement{{RequirementID: "gain", OperatingCase: "rated", Actual: gain}, {RequirementID: "thermal", OperatingCase: "rated", Actual: temperature}}
}

func closedLoopTestRequirement() architecturesearch.Requirement {
	minimumGain, maximumGain, maximumTemperature := 9.0, 11.0, 100.0
	conditionMinimum, conditionMaximum := 4.5, 5.5
	return architecturesearch.Requirement{
		Schema: architecturesearch.SchemaIDV3, Version: architecturesearch.VersionV3,
		Project: architecturesearch.Project{Name: "closed_loop_test", Title: "Closed loop test", Description: "Behavioral synthesis test requirement."},
		Requirements: architecturesearch.Requirements{
			Domains:        []architecturesearch.Domain{{ID: "supply", Kind: "supply", NominalVoltageV: 5, Source: "external"}, {ID: "ground", Kind: "reference", NominalVoltageV: 0, Source: "external"}},
			Ports:          []architecturesearch.Port{{ID: "ground", Kind: "reference", Direction: "bidirectional", Domain: "ground"}, {ID: "input", Kind: "analog_voltage", Direction: "sink", Domain: "ground"}, {ID: "output", Kind: "analog_voltage", Direction: "source", Domain: "ground"}, {ID: "power", Kind: "power", Direction: "sink", Domain: "supply"}},
			Objectives:     []architecturesearch.Objective{{ID: "amplify", Capability: "signal_amplification", Bindings: []architecturesearch.Binding{{Role: "input", Port: "input"}, {Role: "output", Port: "output"}, {Role: "power", Port: "power"}, {Role: "reference", Port: "ground"}}, Constraints: []architecturesearch.Constraint{}}},
			OperatingCases: []architecturesearch.OperatingCase{{ID: "rated", Conditions: []architecturesearch.OperatingCondition{{Axis: "supply_voltage", Target: "supply", Min: &conditionMinimum, Max: &conditionMaximum, Unit: "V"}}}},
			BehavioralRequirements: []architecturesearch.BehavioralRequirement{
				{ID: "gain", Metric: "voltage_gain", Analysis: simmodel.AnalysisACSweep, Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimumGain, Max: &maximumGain, Unit: "ratio", OperatingCases: []string{"rated"}},
				{ID: "thermal", Metric: "junction_temperature", Analysis: simmodel.AnalysisThermal, Observation: architecturesearch.Observation{Kind: "circuit", ID: "circuit"}, Max: &maximumTemperature, Unit: "degC", OperatingCases: []string{"rated"}, Critical: true},
			},
			Constraints: architecturesearch.BoardLimits{MaxComponents: 16, MaxWidthMM: 50, MaxHeightMM: 40},
		},
		Acceptance: architecturesearch.Acceptance{RequireERC: true, RequireStrictDRC: true, RequireCompleteRouting: true, RequireConnectivity: true, RequireWriterCorrectness: true, RequireRoundTripZeroDiff: true, RequireDeterministicReplay: true, RequireContractComposition: true, RequireGlobalReasoning: true, RequireCoverageAccounting: true, RequireAlternatives: true, RequireFailClosed: true, RequireSimulation: true, RequireAllCorners: true, RequireModelProvenance: true, RequireClosedLoopEvidence: true},
	}
}

func testHash(value string) string {
	data, _ := json.Marshal(value)
	return hashJSON(data)
}
