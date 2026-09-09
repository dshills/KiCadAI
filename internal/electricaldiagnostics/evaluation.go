// Package electricaldiagnostics projects existing numerical evidence without
// changing admission, execution, search, or acceptance decisions.
package electricaldiagnostics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kicadai/internal/opentopologysynthesis"
	"kicadai/internal/simmodel"
	"kicadai/internal/simulationadmission"
)

type Stage string

const (
	StageAdmission   Stage = "admission"
	StagePreparation Stage = "pre_simulation"
	StageSolver      Stage = "solver"
	StageAssertion   Stage = "electrical_assertion"
	StageEvidence    Stage = "insufficient_evidence"
	StageResource    Stage = "resource_limit"
	StageCanceled    Stage = "canceled"
)

// Failure identifies recorded evidence, not a claim that every possible circuit
// has the same cause. Timing is intentionally absent from this deterministic type.
type Failure struct {
	Attempt       int                                          `json:"attempt"`
	Stage         Stage                                        `json:"stage"`
	Assertion     string                                       `json:"assertion,omitempty"`
	Analysis      string                                       `json:"analysis,omitempty"`
	Metric        string                                       `json:"metric,omitempty"`
	OperatingCase string                                       `json:"operating_case,omitempty"`
	Corner        string                                       `json:"corner,omitempty"`
	Workflow      string                                       `json:"workflow,omitempty"`
	PlanHash      string                                       `json:"plan_sha256,omitempty"`
	ReportHash    string                                       `json:"report_sha256,omitempty"`
	ModelEvidence []string                                     `json:"model_evidence_sha256"`
	Actual        *float64                                     `json:"actual,omitempty"`
	Minimum       *float64                                     `json:"minimum,omitempty"`
	Maximum       *float64                                     `json:"maximum,omitempty"`
	Diagnostics   []opentopologysynthesis.SimulationDiagnostic `json:"diagnostics"`
}

type Count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type Evaluation struct {
	Schema            string                                           `json:"schema"`
	EvaluationHash    string                                           `json:"evaluation_sha256"`
	RequirementHash   string                                           `json:"requirement_sha256"`
	InventoryHash     string                                           `json:"inventory_sha256"`
	GraphHash         string                                           `json:"graph_sha256"`
	ValueTrialHash    string                                           `json:"value_trial_sha256,omitempty"`
	Status            opentopologysynthesis.SimulationEvaluationStatus `json:"status"`
	Consumption       opentopologysynthesis.Consumption                `json:"consumption"`
	AttemptCount      int                                              `json:"attempt_count"`
	AttemptOutcomes   []Count                                          `json:"attempt_outcomes"`
	FailureCategories []Count                                          `json:"failure_categories"`
	FirstFailure      *Failure                                         `json:"first_failure,omitempty"`
	Diagnoses         []opentopologysynthesis.Diagnosis                `json:"diagnoses"`
	Hash              string                                           `json:"hash"`
}

// ProjectEvaluation authenticates the exact input before omitting large solver
// waveforms. First means the first nonpassing attempt in recorded execution order,
// not the lexicographically first diagnosis or the most frequent failure category.
func ProjectEvaluation(source opentopologysynthesis.SimulationEvaluation) (Evaluation, error) {
	unsigned := source
	unsigned.Hash = ""
	digest, err := hash(unsigned)
	if err != nil || source.Hash == "" || digest != source.Hash {
		return Evaluation{}, fmt.Errorf("evaluation content hash differs")
	}
	if source.Schema != opentopologysynthesis.SimulationEvaluationSchema || source.Version != opentopologysynthesis.SimulationEvaluationVersion {
		return Evaluation{}, fmt.Errorf("unsupported evaluation schema")
	}
	result := Evaluation{
		Schema: "kicadai.electrical-diagnostic-evaluation.v1", EvaluationHash: source.Hash,
		RequirementHash: source.RequirementHash, InventoryHash: source.InventoryHash,
		GraphHash: source.GraphHash, ValueTrialHash: source.ValueTrialHash,
		Status: source.Status, Consumption: source.Consumption, AttemptCount: len(source.Attempts),
		Diagnoses: slices.Clone(source.Diagnoses),
	}
	outcomes, categories := map[string]int{}, map[string]int{}
	lastNumber := 0
	for _, attempt := range source.Attempts {
		if attempt.Number <= lastNumber {
			return Evaluation{}, fmt.Errorf("attempt order is not strictly increasing")
		}
		lastNumber = attempt.Number
		outcomes[string(attempt.Status)]++
		if attempt.Status == opentopologysynthesis.SimulationEvaluationPassed {
			continue
		}
		failure := projectFailure(attempt)
		if assertionDiagnosisMatches(attempt, source.Diagnoses) {
			failure.Stage = StageAssertion
		}
		if result.FirstFailure == nil {
			result.FirstFailure = &failure
		}
		if len(failure.Diagnostics) == 0 {
			categories[string(failure.Stage)+"/unclassified"]++
		} else {
			seen := map[string]bool{}
			for _, diagnostic := range failure.Diagnostics {
				key := string(failure.Stage) + "/" + diagnostic.Code
				if !seen[key] {
					categories[key]++
					seen[key] = true
				}
			}
		}
	}
	if result.FirstFailure == nil && source.Status != opentopologysynthesis.SimulationEvaluationPassed {
		failure := Failure{Stage: StageEvidence, Diagnostics: []opentopologysynthesis.SimulationDiagnostic{}, ModelEvidence: []string{}}
		if source.Status == opentopologysynthesis.SimulationEvaluationCanceled {
			failure.Stage = StageCanceled
		} else if source.Status == opentopologysynthesis.SimulationEvaluationExhausted {
			failure.Stage = StageResource
		}
		for _, issue := range source.Issues {
			failure.Diagnostics = append(failure.Diagnostics, opentopologysynthesis.SimulationDiagnostic{Code: string(issue.Code), Path: issue.Path, Message: issue.Message, Suggestion: issue.Suggestion})
		}
		result.FirstFailure = &failure
	}
	result.AttemptOutcomes = orderedCounts(outcomes)
	result.FailureCategories = orderedCounts(categories)
	result.Hash, err = hash(result)
	return result, err
}

func projectFailure(attempt opentopologysynthesis.SimulationAttempt) Failure {
	result := Failure{
		Attempt: attempt.Number, Stage: StageEvidence, Assertion: attempt.RequirementID,
		Analysis: attempt.Analysis, Metric: attempt.Metric, OperatingCase: attempt.OperatingCase,
		Corner: attempt.CornerID, Workflow: attempt.WorkflowModel, PlanHash: attempt.PlanHash,
		ReportHash: attempt.ReportHash, ModelEvidence: slices.Clone(attempt.ModelEvidenceSHA256s),
		Actual: cloneFloat(attempt.Actual), Minimum: cloneFloat(attempt.RequiredMin), Maximum: cloneFloat(attempt.RequiredMax),
		Diagnostics: slices.Clone(attempt.Diagnostics),
	}
	if attempt.Status == opentopologysynthesis.SimulationEvaluationCanceled {
		result.Stage = StageCanceled
		return result
	}
	if attempt.Status == opentopologysynthesis.SimulationEvaluationExhausted {
		result.Stage = StageResource
		return result
	}
	for _, diagnostic := range attempt.Diagnostics {
		if admissionCode(diagnostic.Code) {
			result.Stage = StageAdmission
			return result
		}
	}
	if attempt.Report == nil {
		// Resolve can set PlanHash before refusing. A plan is not proof that a
		// solver was called; preserve this distinction even when it has a hash.
		if len(attempt.Diagnostics) != 0 {
			result.Stage = StagePreparation
		}
		return result
	}
	for _, diagnostic := range attempt.Diagnostics {
		if strings.HasPrefix(diagnostic.Path, "simulation.report") {
			return result
		}
		if diagnostic.Code != simmodel.DiagnosticAssertionOutOfBounds {
			result.Stage = StageSolver
			return result
		}
	}
	if attempt.Actual != nil && !attempt.AssertionPass &&
		((attempt.RequiredMin != nil && *attempt.Actual < *attempt.RequiredMin) || (attempt.RequiredMax != nil && *attempt.Actual > *attempt.RequiredMax)) {
		result.Stage = StageAssertion
		return result
	}
	// A report can contain a failed internal assertion while the outer projected
	// quantity is absent. Preserve that solver-produced assertion evidence.
	for _, assertion := range attempt.Report.Assertions {
		if !assertion.Pass {
			result.Stage = StageAssertion
			return result
		}
	}
	return result
}

// Historical attempts normalize lower-level assertion codes into the generic
// simulation_invalid code. A bound, value, and report-bound structured diagnosis
// restores that distinction without guessing from free-text messages.
func assertionDiagnosisMatches(attempt opentopologysynthesis.SimulationAttempt, diagnoses []opentopologysynthesis.Diagnosis) bool {
	if attempt.Report == nil || attempt.ReportHash == "" || attempt.Actual == nil || attempt.Status != opentopologysynthesis.SimulationEvaluationFailed {
		return false
	}
	for _, diagnostic := range attempt.Diagnostics {
		if diagnostic.Code != "simulation_invalid" && diagnostic.Code != simmodel.DiagnosticAssertionOutOfBounds {
			return false
		}
	}
	for _, diagnosis := range diagnoses {
		if (diagnosis.Code != "assertion_below_minimum" && diagnosis.Code != "assertion_above_maximum") ||
			diagnosis.EvidenceHash != attempt.ReportHash || diagnosis.RequirementID != attempt.RequirementID ||
			diagnosis.OperatingCase != attempt.OperatingCase+"/"+attempt.CornerID || diagnosis.Analysis != attempt.Analysis ||
			diagnosis.Metric != attempt.Metric || diagnosis.Actual == nil || *diagnosis.Actual != *attempt.Actual {
			continue
		}
		if diagnosis.Code == "assertion_below_minimum" && diagnosis.RequiredMin != nil && attempt.RequiredMin != nil && *diagnosis.RequiredMin == *attempt.RequiredMin && *attempt.Actual < *attempt.RequiredMin {
			return true
		}
		if diagnosis.Code == "assertion_above_maximum" && diagnosis.RequiredMax != nil && attempt.RequiredMax != nil && *diagnosis.RequiredMax == *attempt.RequiredMax && *attempt.Actual > *attempt.RequiredMax {
			return true
		}
	}
	return false
}

func admissionCode(code string) bool {
	switch simulationadmission.DiagnosticCode(code) {
	case simulationadmission.CodeMissingModel, simulationadmission.CodeIncompatibleModel,
		simulationadmission.CodeMissingAnalysisDefinition, simulationadmission.CodeUnsupportedAnalysis,
		simulationadmission.CodeSolverUnavailable, simulationadmission.CodeSolverModelIncompatible,
		simulationadmission.CodeInvalidModelParameters:
		return true
	default:
		return false
	}
}

func orderedCounts(values map[string]int) []Count {
	result := make([]Count, 0, len(values))
	for key, count := range values {
		result = append(result, Count{Key: key, Count: count})
	}
	slices.SortFunc(result, func(a, b Count) int { return strings.Compare(a.Key, b.Key) })
	return result
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func hash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}
