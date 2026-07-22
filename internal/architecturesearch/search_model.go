package architecturesearch

import (
	"context"
	"encoding/json"

	"kicadai/internal/reports"
)

const (
	DefaultMaxExpandedStates        = 256
	DefaultMaxDepth                 = 12
	DefaultMaxSearchComponents      = 64
	DefaultMaxUnresolvedObligations = 32
	DefaultMaxProviderExpansions    = 16
	DefaultMaxCompleteCandidates    = 3
	DefaultMaxRejectionSamples      = 3
)

const (
	CodeProviderInvalid            reports.Code = "ARCHITECTURE_PROVIDER_INVALID"
	CodeProviderDuplicate          reports.Code = "ARCHITECTURE_PROVIDER_DUPLICATE"
	CodeProviderExpansionInvalid   reports.Code = "ARCHITECTURE_PROVIDER_EXPANSION_INVALID"
	CodeProviderExpansionLimit     reports.Code = "ARCHITECTURE_PROVIDER_EXPANSION_LIMIT"
	CodeCapabilityUnsupported      reports.Code = "ARCHITECTURE_CAPABILITY_UNSUPPORTED"
	CodeSearchBudgetExhausted      reports.Code = "ARCHITECTURE_SEARCH_BUDGET_EXHAUSTED"
	CodeSearchNoCandidate          reports.Code = "ARCHITECTURE_SEARCH_NO_CANDIDATE"
	CodeSearchAmbiguous            reports.Code = "ARCHITECTURE_SEARCH_AMBIGUOUS"
	CodeSearchCanceled             reports.Code = "ARCHITECTURE_SEARCH_CANCELED"
	CodePowerCurrentBudgetUnknown  reports.Code = "POWER_CURRENT_BUDGET_UNKNOWN"
	CodePowerCurrentBudgetExceeded reports.Code = "POWER_CURRENT_BUDGET_EXCEEDED"
	// CodeGlobalCurrentUnknown and CodeGlobalCurrentExceeded remain source-level
	// aliases for callers compiled against the original architecture-search API.
	CodeGlobalCurrentUnknown     reports.Code = CodePowerCurrentBudgetUnknown
	CodeGlobalCurrentExceeded    reports.Code = CodePowerCurrentBudgetExceeded
	CodeGlobalConstraintUnproven reports.Code = "ARCHITECTURE_GLOBAL_CONSTRAINT_UNPROVEN"
	CodePowerRailSourceMissing   reports.Code = "POWER_RAIL_SOURCE_MISSING"
	CodePowerRailSourceAmbiguous reports.Code = "POWER_RAIL_SOURCE_AMBIGUOUS"
	CodePowerRailCycle           reports.Code = "POWER_RAIL_CYCLE"
	CodePowerSequenceUnproven    reports.Code = "POWER_SEQUENCE_UNPROVEN"
)

type SearchPolicy struct {
	MaxExpandedStates        int `json:"max_expanded_states"`
	MaxDepth                 int `json:"max_depth"`
	MaxComponents            int `json:"max_components"`
	MaxUnresolvedObligations int `json:"max_unresolved_obligations"`
	MaxProviderExpansions    int `json:"max_provider_expansions"`
	MaxCompleteCandidates    int `json:"max_complete_candidates"`
	MaxRejectionSamples      int `json:"max_rejection_samples"`
}

func DefaultSearchPolicy() SearchPolicy {
	return SearchPolicy{
		MaxExpandedStates: DefaultMaxExpandedStates, MaxDepth: DefaultMaxDepth,
		MaxComponents: DefaultMaxSearchComponents, MaxUnresolvedObligations: DefaultMaxUnresolvedObligations,
		MaxProviderExpansions: DefaultMaxProviderExpansions, MaxCompleteCandidates: DefaultMaxCompleteCandidates,
		MaxRejectionSamples: DefaultMaxRejectionSamples,
	}
}

type ProviderDescriptor struct {
	ID           string           `json:"id"`
	Revision     string           `json:"revision"`
	Capabilities []string         `json:"capabilities"`
	Evidence     ContractEvidence `json:"evidence"`
}

type FragmentProvider interface {
	Descriptor() ProviderDescriptor
	Expand(context.Context, ProviderRequest) ([]ProviderExpansion, error)
}

// ProviderRequest intentionally excludes project identity, corpus identity,
// requirement hashes, descriptions, and extensions. A provider receives only
// the normalized electrical obligation shared by every provider.
type ProviderRequest struct {
	Capability  string         `json:"capability"`
	Ports       []RoleContract `json:"ports"`
	Constraints []Constraint   `json:"constraints"`
	BoardLimits BoardLimits    `json:"board_limits"`
}

type RoleContract struct {
	Role     string       `json:"role"`
	Anchor   string       `json:"anchor"`
	Contract PortContract `json:"contract"`
}

type ChildObligation struct {
	ID          string         `json:"id"`
	Capability  string         `json:"capability"`
	Ports       []RoleContract `json:"ports"`
	Constraints []Constraint   `json:"constraints,omitempty"`
}

type SelectedComponent struct {
	InstanceID string             `json:"instance_id"`
	CatalogID  string             `json:"catalog_id"`
	VariantID  string             `json:"variant_id,omitempty"`
	Evidence   EvidenceConfidence `json:"evidence"`
}

type ExpansionMetrics struct {
	UnprovenNonSafety int      `json:"unproven_non_safety"`
	WorstMargin       *float64 `json:"worst_margin,omitempty"`
	QuiescentPowerW   *float64 `json:"quiescent_power_w,omitempty"`
	AreaMM2           *float64 `json:"area_mm2,omitempty"`
}

type ProviderExpansion struct {
	ID                 string                `json:"id"`
	OfferedPorts       []RoleContract        `json:"offered_ports"`
	Children           []ChildObligation     `json:"children,omitempty"`
	Components         []SelectedComponent   `json:"components,omitempty"`
	Calculations       []CalculationEvidence `json:"calculations,omitempty"`
	Metrics            ExpansionMetrics      `json:"metrics"`
	Evidence           ContractEvidence      `json:"evidence"`
	DecisionClass      string                `json:"decision_class,omitempty"`
	RequiresUserChoice bool                  `json:"requires_user_choice,omitempty"`
	Payload            json.RawMessage       `json:"payload,omitempty"`
}

type FragmentSelection struct {
	ObligationPath     string                `json:"obligation_path"`
	Capability         string                `json:"capability"`
	ProviderID         string                `json:"provider_id"`
	ProviderRevision   string                `json:"provider_revision"`
	ExpansionID        string                `json:"expansion_id"`
	Ports              []RoleContract        `json:"ports"`
	Components         []SelectedComponent   `json:"components,omitempty"`
	Calculations       []CalculationEvidence `json:"calculations,omitempty"`
	Metrics            ExpansionMetrics      `json:"metrics"`
	Evidence           ContractEvidence      `json:"evidence"`
	DecisionClass      string                `json:"decision_class,omitempty"`
	RequiresUserChoice bool                  `json:"requires_user_choice,omitempty"`
	Payload            json.RawMessage       `json:"payload,omitempty"`
}

type CandidateScore struct {
	UnprovenNonSafety int      `json:"unproven_non_safety"`
	WorstMargin       *float64 `json:"worst_margin,omitempty"`
	EvidenceRank      int      `json:"evidence_rank"`
	ComponentCount    int      `json:"component_count"`
	FragmentCount     int      `json:"fragment_count"`
	QuiescentPowerW   *float64 `json:"quiescent_power_w,omitempty"`
	AreaMM2           *float64 `json:"area_mm2,omitempty"`
	Fingerprint       string   `json:"fingerprint"`
}

type CandidateResult struct {
	Fingerprint  string              `json:"fingerprint"`
	Score        CandidateScore      `json:"score"`
	Selections   []FragmentSelection `json:"selections"`
	GlobalChecks []GlobalCheck       `json:"global_checks,omitempty"`
}

type GlobalCheck struct {
	// Code identifies the validation rule that passed. The same stable rule ID
	// is used by rejection summaries when that rule fails.
	Code     reports.Code `json:"code"`
	Path     string       `json:"path"`
	Message  string       `json:"message"`
	Required *float64     `json:"required,omitempty"`
	Observed *float64     `json:"observed,omitempty"`
	Margin   *float64     `json:"margin,omitempty"`
}

type AlternativeComparison struct {
	Fingerprint     string `json:"fingerprint"`
	FirstScoreField string `json:"first_score_field"`
	Reason          string `json:"reason"`
}

type SelectionRationale struct {
	SelectedFingerprint string                  `json:"selected_fingerprint"`
	Summary             string                  `json:"summary"`
	Comparisons         []AlternativeComparison `json:"comparisons,omitempty"`
}

type SearchConsumption struct {
	ExpandedStates      int `json:"expanded_states"`
	GeneratedStates     int `json:"generated_states"`
	CompleteCandidates  int `json:"complete_candidates"`
	RejectedExpansions  int `json:"rejected_expansions"`
	MaximumFrontier     int `json:"maximum_frontier"`
	MaximumDepthReached int `json:"maximum_depth_reached"`
}

type ExpansionRejection struct {
	Code        reports.Code `json:"code"`
	Path        string       `json:"path"`
	ProviderID  string       `json:"provider_id,omitempty"`
	ExpansionID string       `json:"expansion_id,omitempty"`
	Message     string       `json:"message"`
}

type RejectionSummary struct {
	Code    reports.Code         `json:"code"`
	Count   int                  `json:"count"`
	Samples []ExpansionRejection `json:"samples"`
}

type SearchStatus string

const (
	SearchSelected    SearchStatus = "selected"
	SearchUnsupported SearchStatus = "unsupported"
	SearchExhausted   SearchStatus = "exhausted"
	SearchAmbiguous   SearchStatus = "ambiguous"
	SearchFailed      SearchStatus = "failed"
)

type CoverageStatus string

const (
	CoverageSelected        CoverageStatus = "selected"
	CoverageRejected        CoverageStatus = "rejected"
	CoverageUnsupported     CoverageStatus = "unsupported"
	CoverageAmbiguous       CoverageStatus = "ambiguous"
	CoverageBudgetExhausted CoverageStatus = "budget_exhausted"
)

type CapabilityCoverageRecord struct {
	Path       string         `json:"path"`
	Capability string         `json:"capability"`
	Status     CoverageStatus `json:"status"`
}

type CapabilityCoverageMetrics struct {
	Total           int `json:"total"`
	Selected        int `json:"selected"`
	Rejected        int `json:"rejected"`
	Unsupported     int `json:"unsupported"`
	Ambiguous       int `json:"ambiguous"`
	BudgetExhausted int `json:"budget_exhausted"`
}

type CapabilityCoverage struct {
	Metrics CapabilityCoverageMetrics  `json:"metrics"`
	Records []CapabilityCoverageRecord `json:"records"`
}

type SearchResult struct {
	Schema             string              `json:"schema"`
	PolicyVersion      string              `json:"policy_version"`
	Status             SearchStatus        `json:"status"`
	RequirementHash    string              `json:"requirement_hash"`
	RegistryHash       string              `json:"registry_hash"`
	CatalogHash        string              `json:"catalog_hash,omitempty"`
	FormulaLibraryHash string              `json:"formula_library_hash"`
	Policy             SearchPolicy        `json:"policy"`
	Consumption        SearchConsumption   `json:"consumption"`
	Selected           *CandidateResult    `json:"selected,omitempty"`
	Alternatives       []CandidateResult   `json:"alternatives,omitempty"`
	Rationale          *SelectionRationale `json:"rationale,omitempty"`
	Rejections         []RejectionSummary  `json:"rejections,omitempty"`
	Coverage           *CapabilityCoverage `json:"coverage,omitempty"`
	Issues             []reports.Issue     `json:"issues,omitempty"`
}

type SearchOptions struct {
	Policy      SearchPolicy
	CatalogHash string
}
