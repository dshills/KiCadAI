package closedloopsynthesis

import (
	"context"
	"fmt"
	"testing"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/modelprovenance"
	"kicadai/internal/simmodel"
)

type dividerSimulationResolver struct{}

func (dividerSimulationResolver) ResolveSimulation(_ context.Context, state CandidateState) (SimulationResolution, error) {
	lower := 10_000.0
	for _, variable := range state.Variables {
		if variable.ID == "lower_resistance" {
			lower = variable.Value
		}
	}
	intent := simmodel.Intent{
		ModelID:    simmodel.ModelResistorDividerDCV1,
		Bindings:   []simmodel.Binding{{Role: "upper_resistor", Component: "r1"}, {Role: "lower_resistor", Component: "r2"}},
		Inputs:     []simmodel.NamedValue{{Name: "input_voltage_v", Value: 5}},
		Assertions: []simmodel.Assertion{{Metric: "output_voltage_v", Min: 2.45, Max: 2.55}},
	}
	components := []simmodel.ComponentEvidence{
		{InstanceID: "r1", CatalogID: "resistor.generic.0603", Family: "resistor", ValueSI: 10_000, HasValueSI: true, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.ModelResistorDividerDCV1}}},
		{InstanceID: "r2", CatalogID: "resistor.generic.0603", Family: "resistor", ValueSI: lower, HasValueSI: true, ModelClaims: []simmodel.CatalogEvidence{{ModelID: simmodel.ModelResistorDividerDCV1}}},
	}
	plan, diagnostics := simmodel.Resolve(intent, "test-catalog", testHash("catalog"), components)
	if len(diagnostics) != 0 {
		return SimulationResolution{}, fmt.Errorf("resolve diagnostics: %#v", diagnostics)
	}
	return SimulationResolution{
		Plan:         plan,
		Measurements: []SimulationMeasurementLink{{RequirementID: "output", OperatingCase: "nominal", Assertion: 0}},
	}, nil
}

func TestSimModelEvaluatorRepairsThroughFreshTrustedResolution(t *testing.T) {
	requirement := closedLoopTestRequirement()
	minimum, maximum := 2.45, 2.55
	requirement.Requirements.OperatingCases[0].ID = "nominal"
	requirement.Requirements.BehavioralRequirements = []architecturesearch.BehavioralRequirement{{
		ID: "output", Metric: "dc_voltage", Analysis: simmodel.AnalysisDCOperatingPoint,
		Observation: architecturesearch.Observation{Kind: "port", ID: "output"}, Min: &minimum, Max: &maximum, Unit: "V", OperatingCases: []string{"nominal"}, Critical: true,
	}}
	input := Input{
		Requirement: requirement, CatalogHash: testHash("catalog"), FormulaLibraryHash: testHash("formula"), ModelRegistryHash: testHash("models"),
		Candidates: []Candidate{{
			Fingerprint: testHash("divider"),
			Variables: []Variable{{
				ID: "lower_resistance", Kind: "passive_value", Value: 5_000, AllowedValues: []float64{5_000, 10_000},
				Effects: []RepairEffect{{Analysis: simmodel.AnalysisDCOperatingPoint, Metric: "dc_voltage", Direction: RepairMetricIncreases}},
			}},
		}},
	}
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model provenance registry diagnostics: %#v", diagnostics)
	}
	evaluator := SimModelEvaluator{Resolver: dividerSimulationResolver{}, ProvenanceRegistry: registry}
	report := Run(context.Background(), input, evaluator, DefaultPolicy())
	if report.Status != "pass" || report.Selected == nil || report.Selected.State.Variables[0].Value != 10_000 {
		t.Fatalf("closed-loop simmodel report=%#v", report)
	}
	if report.Consumption.Evaluations != 2 || report.Consumption.RepairsApplied != 1 {
		t.Fatalf("consumption=%#v", report.Consumption)
	}
	if got := len(report.Candidates[0].Attempts[0].ModelDecisions); got != 2 {
		t.Fatalf("independently derived model decisions = %d, want 2", got)
	}
	replay := Run(context.Background(), input, evaluator, DefaultPolicy())
	if hashJSON(report) != hashJSON(replay) {
		t.Fatal("trusted simmodel closed-loop replay differs")
	}
}

func TestSimModelEvaluatorFailsClosedForInvalidLinksAndStructuralDiagnostics(t *testing.T) {
	evaluator := SimModelEvaluator{Resolver: invalidSimulationResolver{}}
	if _, err := evaluator.Evaluate(context.Background(), CandidateState{Fingerprint: testHash("invalid")}); err == nil {
		t.Fatal("invalid simulation resolution was accepted")
	}
}

func TestSimModelEvaluatorFailsClosedWithoutIndependentProvenanceRegistry(t *testing.T) {
	evaluator := SimModelEvaluator{Resolver: dividerSimulationResolver{}}
	if _, err := evaluator.Evaluate(context.Background(), CandidateState{Fingerprint: testHash("missing-provenance")}); err == nil {
		t.Fatal("simulation without independent model provenance was accepted")
	}
}

func TestReplaySimulationEvidenceRequiresExactDeterministicTranscript(t *testing.T) {
	registry, diagnostics := modelprovenance.LoadDefault()
	if len(diagnostics) != 0 {
		t.Fatalf("model provenance registry diagnostics: %#v", diagnostics)
	}
	evaluation, err := (SimModelEvaluator{
		Resolver: dividerSimulationResolver{}, ProvenanceRegistry: registry,
	}).Evaluate(context.Background(), CandidateState{
		Fingerprint: testHash("divider"),
		Variables:   []Variable{{ID: "lower_resistance", Value: 10_000}},
	})
	if err != nil || evaluation.Simulation == nil {
		t.Fatalf("evaluation = %#v, err = %v", evaluation, err)
	}
	if replayDiagnostics := ReplaySimulationEvidence(*evaluation.Simulation); len(replayDiagnostics) != 0 {
		t.Fatalf("replay diagnostics = %#v", replayDiagnostics)
	}
	evaluation.Simulation.Reports[0].Status = "tampered"
	if replayDiagnostics := ReplaySimulationEvidence(*evaluation.Simulation); len(replayDiagnostics) == 0 {
		t.Fatal("tampered simulation transcript replayed successfully")
	}
}

func TestWorstLinkedAssertionSelectsWorstCornerDeterministically(t *testing.T) {
	plan := simmodel.Plan{Assertions: []simmodel.Assertion{{Min: 4.5, Max: 5.5}, {Min: 4.5, Max: 5.5}, {Min: 4.5, Max: 5.5}}}
	report := simmodel.Report{Assertions: []simmodel.AssertionResult{{Actual: 5}, {Actual: 4.6}, {Actual: 5.4}}}
	worst, err := worstLinkedAssertion(plan, report, []int{0, 1, 2})
	if err != nil || worst.Actual != 4.6 {
		t.Fatalf("worst linked assertion = %#v err=%v", worst, err)
	}
	if diagnostics := validateSimulationResolution(SimulationResolution{Plan: simmodel.Plan{}, Measurements: []SimulationMeasurementLink{{RequirementID: "r", OperatingCase: "c", Assertions: []int{1, 0}}}}); len(diagnostics) == 0 {
		t.Fatal("non-canonical aggregate assertion link was accepted")
	}
}

func TestWorstLinkedMeasurementSpansDeterministicPlanBatches(t *testing.T) {
	plans := []simmodel.Plan{
		{Assertions: []simmodel.Assertion{{Min: 4.5, Max: 5.5}}},
		{Assertions: []simmodel.Assertion{{Min: 4.5, Max: 5.5}}},
	}
	reports := []simmodel.Report{
		{Assertions: []simmodel.AssertionResult{{Min: 4.5, Max: 5.5, Actual: 5}}},
		{Assertions: []simmodel.AssertionResult{{Min: 4.5, Max: 5.5, Actual: 4.6}}},
	}
	link := SimulationMeasurementLink{Evidence: []SimulationAssertionSet{{Plan: 0, Assertions: []int{0}}, {Plan: 1, Assertions: []int{0}}}}
	worst, err := worstLinkedMeasurement(plans, reports, link)
	if err != nil || worst.Actual != 4.6 {
		t.Fatalf("worst batched assertion = %#v err=%v", worst, err)
	}
}

func TestOnlyAssertionFailuresRecognizesWorstCaseAssertionDiagnostics(t *testing.T) {
	report := simmodel.Report{Assertions: []simmodel.AssertionResult{{Pass: false}}}
	if !onlyAssertionFailures(report, []simmodel.Diagnostic{{Path: "assertions.bandwidth", Message: "measured 90000 is outside trusted bounds 100000..1e+12"}}) {
		t.Fatal("nominal measured assertion failure was treated as a model execution failure")
	}
	if !onlyAssertionFailures(report, []simmodel.Diagnostic{{Path: "worst_case.devices.r1.value_si=900", Message: "worst-case corner devices.r1.value_si=900 measured 8.5 outside trusted bounds 9..11"}}) {
		t.Fatal("worst-case assertion failure was treated as a model execution failure")
	}
	if onlyAssertionFailures(report, []simmodel.Diagnostic{{Path: "assertions.bandwidth", Message: "solved AC sweep does not bracket the -3 dB cutoff"}}) {
		t.Fatal("unavailable derived measurement was treated as numeric assertion evidence")
	}
	if onlyAssertionFailures(report, []simmodel.Diagnostic{{Path: "worst_case", Message: "corner could not be evaluated"}}) {
		t.Fatal("worst-case execution failure was treated as an assertion failure")
	}
}

type invalidSimulationResolver struct{}

func (invalidSimulationResolver) ResolveSimulation(context.Context, CandidateState) (SimulationResolution, error) {
	return SimulationResolution{Measurements: []SimulationMeasurementLink{{RequirementID: "x", OperatingCase: "y", Assertion: 7}}}, nil
}
