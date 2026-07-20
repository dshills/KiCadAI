// Package closedloopsynthesis provides bounded deterministic behavioral
// candidate evaluation and repair orchestration.
package closedloopsynthesis

import (
	"context"
	"encoding/json"

	"kicadai/internal/architecturesearch"
	"kicadai/internal/simmodel"
)

const (
	ReportSchema  = "kicadai.closed-loop-synthesis-report.v1"
	PolicyVersion = "closed-loop-synthesis-policy-v1"
)

type StopReason string

const (
	StopPassed              StopReason = "passed"
	StopNoCandidate         StopReason = "no_candidate"
	StopInvalidInput        StopReason = "invalid_input"
	StopEvaluationFailed    StopReason = "evaluation_failed"
	StopModelTrustFailed    StopReason = "model_trust_failed"
	StopAssertionIncomplete StopReason = "assertion_coverage_incomplete"
	StopNoRepairVariables   StopReason = "no_repair_variables"
	StopNonImprovement      StopReason = "non_improvement"
	StopRepeatedState       StopReason = "repeated_state"
	StopBudgetExhausted     StopReason = "budget_exhausted"
	StopCanceled            StopReason = "canceled"
)

type Policy struct {
	MaxCandidates            int `json:"max_candidates"`
	MaxRepairsPerCandidate   int `json:"max_repairs_per_candidate"`
	MaxEvaluations           int `json:"max_evaluations"`
	MaxVariablesPerCandidate int `json:"max_variables_per_candidate"`
	MaxValuesPerVariable     int `json:"max_values_per_variable"`
}

func DefaultPolicy() Policy {
	return Policy{MaxCandidates: 16, MaxRepairsPerCandidate: 8, MaxEvaluations: 128, MaxVariablesPerCandidate: 32, MaxValuesPerVariable: 64}
}

type Input struct {
	Requirement        architecturesearch.Requirement `json:"requirement"`
	CatalogHash        string                         `json:"catalog_hash"`
	FormulaLibraryHash string                         `json:"formula_library_hash"`
	ModelRegistryHash  string                         `json:"model_registry_hash"`
	Candidates         []Candidate                    `json:"candidates"`
}

type Candidate struct {
	Fingerprint string                            `json:"fingerprint"`
	Score       architecturesearch.CandidateScore `json:"score"`
	Variables   []Variable                        `json:"variables,omitempty"`
}

type Variable struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Value         float64   `json:"value"`
	AllowedValues []float64 `json:"allowed_values"`
}

type CandidateState struct {
	Fingerprint string     `json:"fingerprint"`
	Variables   []Variable `json:"variables,omitempty"`
}

// Evaluator is implemented only by trusted adapters. It must re-resolve the
// complete candidate and return every required analysis/corner measurement on
// every call.
type Evaluator interface {
	Evaluate(context.Context, CandidateState) (Evaluation, error)
}

type Evaluation struct {
	EvidenceHash   string          `json:"evidence_hash"`
	Measurements   []Measurement   `json:"measurements"`
	ModelDecisions []ModelDecision `json:"model_decisions"`
}

type Measurement struct {
	RequirementID string  `json:"requirement_id"`
	OperatingCase string  `json:"operating_case"`
	Actual        float64 `json:"actual"`
}

type ModelDecision struct {
	Component        string                    `json:"component"`
	Family           string                    `json:"family"`
	Claim            simmodel.CatalogEvidence  `json:"claim"`
	Provenance       *simmodel.ModelProvenance `json:"provenance,omitempty"`
	Status           string                    `json:"status"`
	Reason           string                    `json:"reason"`
	RequiredAnalyses []string                  `json:"required_analyses,omitempty"`
}

type Report struct {
	Schema             string            `json:"schema"`
	PolicyVersion      string            `json:"policy_version"`
	PolicyHash         string            `json:"policy_hash"`
	RequirementHash    string            `json:"requirement_hash"`
	RegistryHash       string            `json:"registry_hash"`
	CatalogHash        string            `json:"catalog_hash"`
	FormulaLibraryHash string            `json:"formula_library_hash"`
	ModelRegistryHash  string            `json:"model_registry_hash"`
	Policy             Policy            `json:"policy"`
	Candidates         []CandidateReport `json:"candidates"`
	Selected           *SelectedResult   `json:"selected,omitempty"`
	Consumption        Consumption       `json:"consumption"`
	StopReason         StopReason        `json:"stop_reason"`
	Status             string            `json:"status"`
	Diagnostics        []Diagnostic      `json:"diagnostics"`
}

// CloneReport returns a deep copy suitable for persistence boundaries.
func CloneReport(source Report) Report {
	data, err := json.Marshal(source)
	if err != nil {
		return source
	}
	var clone Report
	if err := json.Unmarshal(data, &clone); err != nil {
		return source
	}
	return clone
}

type CandidateReport struct {
	Fingerprint string                            `json:"fingerprint"`
	StaticScore architecturesearch.CandidateScore `json:"static_score"`
	Attempts    []Attempt                         `json:"attempts"`
	Repairs     []Repair                          `json:"repairs"`
	FinalState  CandidateState                    `json:"final_state"`
	FinalScore  EvaluationScore                   `json:"final_score"`
	Status      string                            `json:"status"`
	StopReason  StopReason                        `json:"stop_reason"`
}

type Attempt struct {
	Number         int               `json:"number"`
	State          CandidateState    `json:"state"`
	StateHash      string            `json:"state_hash"`
	EvidenceHash   string            `json:"evidence_hash,omitempty"`
	Assertions     []AssertionResult `json:"assertions"`
	ModelDecisions []ModelDecision   `json:"model_decisions"`
	Diagnoses      []Diagnosis       `json:"diagnoses"`
	Score          EvaluationScore   `json:"score"`
	Status         string            `json:"status"`
	Diagnostics    []Diagnostic      `json:"diagnostics"`
}

type AssertionResult struct {
	RequirementID string   `json:"requirement_id"`
	OperatingCase string   `json:"operating_case"`
	Analysis      string   `json:"analysis"`
	Metric        string   `json:"metric"`
	Actual        float64  `json:"actual"`
	Min           *float64 `json:"min,omitempty"`
	Max           *float64 `json:"max,omitempty"`
	Critical      bool     `json:"critical"`
	Margin        float64  `json:"normalized_margin"`
	Pass          bool     `json:"pass"`
}

type EvaluationScore struct {
	CriticalFailures int     `json:"critical_failures"`
	Failures         int     `json:"failures"`
	WorstMargin      float64 `json:"worst_normalized_margin"`
	ModelUses        int     `json:"reviewed_model_uses"`
}

type Diagnosis struct {
	RequirementID string `json:"requirement_id"`
	OperatingCase string `json:"operating_case"`
	Analysis      string `json:"analysis"`
	Metric        string `json:"metric"`
	Direction     string `json:"direction"`
	Critical      bool   `json:"critical"`
	Message       string `json:"message"`
}

type Repair struct {
	Number          int     `json:"number"`
	Variable        string  `json:"variable"`
	Kind            string  `json:"kind"`
	From            float64 `json:"from"`
	To              float64 `json:"to"`
	BeforeHash      string  `json:"before_hash"`
	AfterHash       string  `json:"after_hash"`
	Reason          string  `json:"reason"`
	EvaluatedTrials int     `json:"evaluated_trials"`
}

type SelectedResult struct {
	Fingerprint string          `json:"fingerprint"`
	State       CandidateState  `json:"state"`
	Score       EvaluationScore `json:"score"`
	Repairs     int             `json:"repairs"`
	Rationale   string          `json:"rationale"`
}

type Consumption struct {
	CandidatesEvaluated int  `json:"candidates_evaluated"`
	Evaluations         int  `json:"evaluations"`
	RepairTrials        int  `json:"repair_trials"`
	RepairsApplied      int  `json:"repairs_applied"`
	BudgetExhausted     bool `json:"budget_exhausted"`
}

type Diagnostic struct {
	Path       string `json:"path"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}
