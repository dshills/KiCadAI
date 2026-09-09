package electricaldiagnostics

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	ot "kicadai/internal/opentopologysynthesis"
	"kicadai/internal/simmodel"
)

func signedEvaluation(t *testing.T, attempts ...ot.SimulationAttempt) ot.SimulationEvaluation {
	t.Helper()
	evaluation := ot.SimulationEvaluation{
		Schema: ot.SimulationEvaluationSchema, Version: ot.SimulationEvaluationVersion,
		RequirementHash: strings.Repeat("1", 64), InventoryHash: strings.Repeat("2", 64), GraphHash: strings.Repeat("3", 64),
		Status: ot.SimulationEvaluationFailed, Attempts: attempts,
	}
	for i := range evaluation.Attempts {
		evaluation.Attempts[i].Number = i + 1
	}
	sealEvaluation(t, &evaluation)
	return evaluation
}

func sealEvaluation(t *testing.T, evaluation *ot.SimulationEvaluation) {
	t.Helper()
	evaluation.Hash = ""
	var err error
	evaluation.Hash, err = hash(*evaluation)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFirstFailureStage(t *testing.T) {
	value, minimum := 0.5, 1.0
	for _, tc := range []struct {
		name    string
		attempt ot.SimulationAttempt
		want    Stage
	}{
		{"admission", ot.SimulationAttempt{Status: ot.SimulationEvaluationUnsupported, Diagnostics: []ot.SimulationDiagnostic{{Code: "MISSING_MODEL"}}}, StageAdmission},
		{"analysis_admission", ot.SimulationAttempt{Status: ot.SimulationEvaluationUnsupported, Diagnostics: []ot.SimulationDiagnostic{{Code: "MISSING_ANALYSIS_DEFINITION"}}}, StageAdmission},
		{"solver_admission", ot.SimulationAttempt{Status: ot.SimulationEvaluationUnsupported, Diagnostics: []ot.SimulationDiagnostic{{Code: "SOLVER_UNAVAILABLE"}}}, StageAdmission},
		{"preparation_with_plan", ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed, PlanHash: strings.Repeat("4", 64), Diagnostics: []ot.SimulationDiagnostic{{Code: "SIMULATION_INVALID"}}}, StagePreparation},
		{"solver", ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed, Report: &simmodel.Report{Status: "fail"}, Diagnostics: []ot.SimulationDiagnostic{{Code: "NONCONVERGENT"}}}, StageSolver},
		{"partial_value_is_not_assertion_proof", ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed, Actual: &value, RequiredMin: &minimum, Report: &simmodel.Report{Status: "fail"}, Diagnostics: []ot.SimulationDiagnostic{{Code: "NONCONVERGENT"}}}, StageSolver},
		{"assertion", ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed, Actual: &value, RequiredMin: &minimum, Report: &simmodel.Report{Status: "fail"}}, StageAssertion},
		{"internal_assertion", ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed, Report: &simmodel.Report{Status: "fail", Assertions: []simmodel.AssertionResult{{Pass: false}}}, Diagnostics: []ot.SimulationDiagnostic{{Code: simmodel.DiagnosticAssertionOutOfBounds}}}, StageAssertion},
		{"unclassified", ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed}, StageEvidence},
		{"invalid_report", ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed, Report: &simmodel.Report{Status: "fail"}, Diagnostics: []ot.SimulationDiagnostic{{Code: "simulation_invalid", Path: "simulation.report.assertions"}}}, StageEvidence},
		{"resource", ot.SimulationAttempt{Status: ot.SimulationEvaluationExhausted}, StageResource},
		{"canceled", ot.SimulationAttempt{Status: ot.SimulationEvaluationCanceled}, StageCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := signedEvaluation(t, tc.attempt)
			result, err := ProjectEvaluation(source)
			if err != nil {
				t.Fatal(err)
			}
			if result.FirstFailure == nil || result.FirstFailure.Stage != tc.want {
				t.Fatalf("got %+v, want %s", result.FirstFailure, tc.want)
			}
		})
	}
}

func TestProjectionPreservesExecutionOrderAndReplay(t *testing.T) {
	source := signedEvaluation(t,
		ot.SimulationAttempt{Status: ot.SimulationEvaluationPassed, AssertionPass: true},
		ot.SimulationAttempt{Status: ot.SimulationEvaluationUnsupported, RequirementID: "z-first", Diagnostics: []ot.SimulationDiagnostic{{Code: "MISSING_MODEL"}, {Code: "MISSING_MODEL"}}},
		ot.SimulationAttempt{Status: ot.SimulationEvaluationUnsupported, RequirementID: "a-later", Diagnostics: []ot.SimulationDiagnostic{{Code: "SOLVER_UNAVAILABLE"}}},
	)
	before, _ := json.Marshal(source)
	first, err := ProjectEvaluation(source)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectEvaluation(source)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Hash == "" {
		t.Fatal("projection does not replay")
	}
	if first.FirstFailure.Attempt != 2 || first.FirstFailure.Assertion != "z-first" {
		t.Fatal("first failure reordered")
	}
	if !reflect.DeepEqual(first.FailureCategories, []Count{{Key: "admission/MISSING_MODEL", Count: 1}, {Key: "admission/SOLVER_UNAVAILABLE", Count: 1}}) {
		t.Fatalf("counts %+v", first.FailureCategories)
	}
	after, _ := json.Marshal(source)
	if string(before) != string(after) {
		t.Fatal("source mutated")
	}
}

func TestProjectionRejectsTamperAndInvalidOrder(t *testing.T) {
	source := signedEvaluation(t, ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed})
	source.Attempts[0].Metric = "tampered"
	if _, err := ProjectEvaluation(source); err == nil {
		t.Fatal("tamper accepted")
	}
	source = signedEvaluation(t, ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed}, ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed})
	source.Attempts[1].Number = 1
	sealEvaluation(t, &source)
	if _, err := ProjectEvaluation(source); err == nil {
		t.Fatal("duplicate order accepted")
	}
	source.Schema = "wrong"
	sealEvaluation(t, &source)
	if _, err := ProjectEvaluation(source); err == nil {
		t.Fatal("unknown schema accepted")
	}
}

func TestProjectionAbsentAttemptIsNotSolverEvidence(t *testing.T) {
	for _, status := range []ot.SimulationEvaluationStatus{ot.SimulationEvaluationFailed, ot.SimulationEvaluationCanceled, ot.SimulationEvaluationExhausted, ot.SimulationEvaluationPassed} {
		source := signedEvaluation(t)
		source.Status = status
		sealEvaluation(t, &source)
		result, err := ProjectEvaluation(source)
		if err != nil {
			t.Fatal(err)
		}
		if status == ot.SimulationEvaluationPassed {
			if result.FirstFailure != nil {
				t.Fatal("passing evaluation gained a failure")
			}
		} else if result.FirstFailure == nil || result.FirstFailure.Stage == StageSolver || result.FirstFailure.Stage == StageAssertion {
			t.Fatal("absent attempt claimed electrical evidence")
		}
	}
}

func TestProjectionOmitsWaveforms(t *testing.T) {
	points := make([]simmodel.AnalysisPoint, 10000)
	source := signedEvaluation(t, ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed, Report: &simmodel.Report{Analyses: []simmodel.AnalysisResult{{Points: points}}}, Diagnostics: []ot.SimulationDiagnostic{{Code: "NONCONVERGENT"}}})
	result, err := ProjectEvaluation(source)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 4096 || strings.Contains(string(data), "\"points\"") {
		t.Fatal("compact projection retained waveform points")
	}
}

func TestHistoricalAssertionDiagnosisRequiresExactEvidence(t *testing.T) {
	actual, minimum := 2.0, 2.45
	source := signedEvaluation(t, ot.SimulationAttempt{Status: ot.SimulationEvaluationFailed, RequirementID: "level", Analysis: "dc_operating_point", Metric: "output_voltage", OperatingCase: "nominal", CornerID: "center", ReportHash: strings.Repeat("5", 64), Actual: &actual, RequiredMin: &minimum, Report: &simmodel.Report{Status: "fail"}, Diagnostics: []ot.SimulationDiagnostic{{Code: "simulation_invalid"}}})
	source.Diagnoses = []ot.Diagnosis{{Code: "assertion_below_minimum", RequirementID: "level", Analysis: "dc_operating_point", Metric: "output_voltage", OperatingCase: "nominal/center", EvidenceHash: strings.Repeat("5", 64), Actual: &actual, RequiredMin: &minimum}}
	sealEvaluation(t, &source)
	projection, err := ProjectEvaluation(source)
	if err != nil || projection.FirstFailure.Stage != StageAssertion {
		t.Fatalf("assertion code normalization not resolved: %+v %v", projection.FirstFailure, err)
	}
	source.Diagnoses[0].EvidenceHash = strings.Repeat("6", 64)
	sealEvaluation(t, &source)
	projection, err = ProjectEvaluation(source)
	if err != nil || projection.FirstFailure.Stage != StageSolver {
		t.Fatal("unrelated diagnosis overrode failure")
	}
	source.Diagnoses[0].EvidenceHash = strings.Repeat("5", 64)
	for name, mutate := range map[string]func(*ot.SimulationAttempt, *ot.Diagnosis){
		"missing attempt actual":   func(a *ot.SimulationAttempt, _ *ot.Diagnosis) { a.Actual = nil },
		"missing diagnosis actual": func(_ *ot.SimulationAttempt, d *ot.Diagnosis) { d.Actual = nil },
		"missing attempt bound":    func(a *ot.SimulationAttempt, _ *ot.Diagnosis) { a.RequiredMin = nil },
		"missing diagnosis bound":  func(_ *ot.SimulationAttempt, d *ot.Diagnosis) { d.RequiredMin = nil },
		"different exact value":    func(_ *ot.SimulationAttempt, d *ot.Diagnosis) { value := 2.0 + 1e-12; d.Actual = &value },
		"solver failure present": func(a *ot.SimulationAttempt, _ *ot.Diagnosis) {
			a.Diagnostics = append(a.Diagnostics, ot.SimulationDiagnostic{Code: "simulation_nonconvergent"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			attempt, diagnosis := source.Attempts[0], source.Diagnoses[0]
			mutate(&attempt, &diagnosis)
			if assertionDiagnosisMatches(attempt, []ot.Diagnosis{diagnosis}) {
				t.Fatal("incomplete or contradictory evidence classified as assertion-only")
			}
		})
	}
}
