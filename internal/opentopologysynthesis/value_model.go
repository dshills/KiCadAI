package opentopologysynthesis

import "kicadai/internal/reports"

const (
	ValueSearchPlanSchema  = "kicadai.open-topology-value-search-plan.v1"
	ValueSearchPlanVersion = 1
)

type ValuePlanStatus string

const (
	ValuePlanReady       ValuePlanStatus = "ready"
	ValuePlanExhausted   ValuePlanStatus = "exhausted"
	ValuePlanUnsupported ValuePlanStatus = "unsupported"
	ValuePlanFailed      ValuePlanStatus = "failed"
)

type ValueSearchPlan struct {
	Schema          string                `json:"schema"`
	Version         int                   `json:"version"`
	PolicyVersion   string                `json:"policy_version"`
	RequirementHash string                `json:"requirement_hash"`
	InventoryHash   string                `json:"inventory_hash"`
	GraphHash       string                `json:"graph_hash"`
	Status          ValuePlanStatus       `json:"status"`
	Policy          Policy                `json:"policy"`
	Domains         []InstanceValueDomain `json:"domains"`
	CandidateValues int                   `json:"candidate_values"`
	Rejections      []SearchRejection     `json:"rejections"`
	Issues          []reports.Issue       `json:"issues"`
}

type InstanceValueDomain struct {
	InstanceID           string                    `json:"instance_id"`
	OriginalPrimitiveKey string                    `json:"original_primitive_key"`
	PrimitiveKind        string                    `json:"primitive_kind"`
	Quantity             string                    `json:"quantity,omitempty"`
	Unit                 string                    `json:"unit,omitempty"`
	AnalyticScales       []AnalyticScale           `json:"analytic_scales"`
	Candidates           []ComponentValueCandidate `json:"candidates"`
}

type AnalyticScale struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	ValueSI    float64 `json:"value_si"`
	Unit       string  `json:"unit"`
	Derivation string  `json:"derivation"`
	SourceKind string  `json:"source_kind"`
	SourceID   string  `json:"source_id"`
	Priority   int     `json:"priority"`
}

type ComponentValueCandidate struct {
	Rank                 int              `json:"rank"`
	PrimitiveKey         string           `json:"primitive_key"`
	ValueSI              *float64         `json:"value_si,omitempty"`
	Quantity             string           `json:"quantity,omitempty"`
	Unit                 string           `json:"unit,omitempty"`
	PreferredSeries      string           `json:"preferred_series,omitempty"`
	TolerancePercent     float64          `json:"tolerance_percent,omitempty"`
	ToleranceProven      bool             `json:"tolerance_proven"`
	CornerMinimumSI      *float64         `json:"corner_minimum_si,omitempty"`
	CornerMaximumSI      *float64         `json:"corner_maximum_si,omitempty"`
	IdealSI              *float64         `json:"ideal_si,omitempty"`
	AnalyticPriority     int              `json:"analytic_priority,omitempty"`
	RelativeError        float64          `json:"relative_error,omitempty"`
	Derivation           string           `json:"derivation"`
	CatalogEvidence      string           `json:"catalog_evidence"`
	ModelEvidenceSHA256s []string         `json:"model_evidence_sha256s"`
	RatingEvidence       []PrimitiveBound `json:"rating_evidence"`
	Hash                 string           `json:"hash"`
}

type ValueTrial struct {
	Number     int                   `json:"number"`
	Selections []ValueTrialSelection `json:"selections"`
	Hash       string                `json:"hash"`
}

type ValueTrialSelection struct {
	InstanceID    string   `json:"instance_id"`
	PrimitiveKey  string   `json:"primitive_key"`
	ValueSI       *float64 `json:"value_si,omitempty"`
	CandidateHash string   `json:"candidate_hash"`
}

type ValueTrialEnumeration struct {
	Trials            []ValueTrial `json:"trials"`
	TotalCombinations uint64       `json:"total_combinations"`
	Exhausted         bool         `json:"exhausted"`
}
