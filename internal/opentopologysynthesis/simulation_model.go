package opentopologysynthesis

import (
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const (
	SimulationEvaluationSchema  = "kicadai.open-topology-simulation-evaluation.v1"
	SimulationEvaluationVersion = 1
)

type SimulationEvaluationStatus string

const (
	SimulationEvaluationPassed      SimulationEvaluationStatus = "passed"
	SimulationEvaluationFailed      SimulationEvaluationStatus = "failed"
	SimulationEvaluationUnsupported SimulationEvaluationStatus = "unsupported"
	SimulationEvaluationExhausted   SimulationEvaluationStatus = "exhausted"
	SimulationEvaluationCanceled    SimulationEvaluationStatus = "canceled"
)

type SimulationEvaluation struct {
	Schema          string                     `json:"schema"`
	Version         int                        `json:"version"`
	PolicyVersion   string                     `json:"policy_version"`
	RequirementHash string                     `json:"requirement_hash"`
	InventoryHash   string                     `json:"inventory_hash"`
	GraphHash       string                     `json:"graph_hash"`
	ValueTrialHash  string                     `json:"value_trial_hash,omitempty"`
	Status          SimulationEvaluationStatus `json:"status"`
	Policy          Policy                     `json:"policy"`
	Consumption     Consumption                `json:"consumption"`
	Attempts        []SimulationAttempt        `json:"attempts"`
	Diagnoses       []Diagnosis                `json:"diagnoses"`
	Issues          []reports.Issue            `json:"issues"`
	Hash            string                     `json:"hash"`
}

type SimulationAttempt struct {
	Number               int                        `json:"number"`
	RequirementID        string                     `json:"requirement_id"`
	OperatingCase        string                     `json:"operating_case"`
	CornerID             string                     `json:"corner_id"`
	Analysis             string                     `json:"analysis"`
	Metric               string                     `json:"metric"`
	WorkflowModel        string                     `json:"workflow_model"`
	PlanHash             string                     `json:"plan_hash"`
	ModelEvidenceSHA256s []string                   `json:"model_evidence_sha256s"`
	ReportHash           string                     `json:"report_hash,omitempty"`
	Status               SimulationEvaluationStatus `json:"status"`
	Actual               *float64                   `json:"actual,omitempty"`
	RequiredMin          *float64                   `json:"required_min,omitempty"`
	RequiredMax          *float64                   `json:"required_max,omitempty"`
	AssertionPass        bool                       `json:"assertion_pass"`
	Report               *simmodel.Report           `json:"report,omitempty"`
	Diagnostics          []SimulationDiagnostic     `json:"diagnostics"`
}

type SimulationDiagnostic struct {
	Code       string `json:"code"`
	Path       string `json:"path"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}
