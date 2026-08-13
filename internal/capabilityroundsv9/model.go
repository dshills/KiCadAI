// Package capabilityroundsv9 implements the frozen V9 public selection and
// causal-round semantics without accessing corpus source or held-out data.
package capabilityroundsv9

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	ErrInvalidInput      = errors.New("invalid V9 capability-round input")
	ErrCandidateOverflow = errors.New("V9 candidate closure exceeds its frozen ceiling")
	ErrNoEligibleBundle  = errors.New("no eligible V9 capability bundle")
	ErrRoundGate         = errors.New("V9 public round gate failed")
)

type Policy struct {
	ExpectedDiscoveryCases int
	MaximumCandidates      int
	MaximumRounds          int
	MaximumTotalAtoms      int
	MaximumTotalMembers    int
	MaximumRoundAtoms      int
	MaximumRoundMembers    int
	MinimumCaseSupport     int
	MinimumAdvancedCases   int
	MinimumDomains         int
	MinimumRoles           int
	MaximumSuccessors      int
	EligibleOutcomes       map[string]bool
	GapCategories          map[string]bool
	StageOrdinal           map[string]int
	SafetyWeights          map[string]int
	ReportingDomains       map[string]bool
	CircuitRoles           map[string]bool
	MechanicalEvidence     []string
}

func FrozenPolicy() Policy {
	return Policy{
		ExpectedDiscoveryCases: 24, MaximumCandidates: 16777216, MaximumRounds: 2,
		MaximumTotalAtoms: 6, MaximumTotalMembers: 18, MaximumRoundAtoms: 3,
		MaximumRoundMembers: 9, MinimumCaseSupport: 2, MinimumAdvancedCases: 2,
		MinimumDomains: 2, MinimumRoles: 2, MaximumSuccessors: 4,
		EligibleOutcomes: map[string]bool{"unsupported": true, "exhausted": true},
		GapCategories:    map[string]bool{"topology": true, "component": true, "model": true, "simulation": true, "physical_design": true, "verification": true},
		StageOrdinal:     map[string]int{"topology": 1, "component": 2, "model": 3, "simulation": 4, "physical_design": 5, "verification": 6},
		SafetyWeights:    map[string]int{"non_safety": 0, "review_required": 1, "safety_relevant": 3, "safety_critical": 5},
		ReportingDomains: map[string]bool{"analog_signal_path": true, "power_energy_conversion": true, "digital_control": true, "mixed_signal_data_conversion": true, "sensing_instrumentation": true, "protection_power_integrity": true},
		CircuitRoles: map[string]bool{"source_bias": true, "amplification_conditioning": true, "conversion_regulation": true,
			"sensing_measurement": true, "interface_control": true, "protection_supervision": true},
		MechanicalEvidence: []string{"catalog_model_references", "configuration_loader_references", "data_references", "focused_non_corpus_runtime_consumer_trace", "registry_references", "reverse_call_graph"},
	}
}

func validatePolicy(policy Policy) error {
	if !reflect.DeepEqual(policy, FrozenPolicy()) {
		return fmt.Errorf("%w: policy differs from the frozen V9 contract", ErrInvalidInput)
	}
	return nil
}

type Leaf struct {
	Stage            string   `json:"stage"`
	Category         string   `json:"category"`
	Scope            string   `json:"scope"`
	Capability       string   `json:"capability"`
	Code             string   `json:"code"`
	RequiredEvidence []string `json:"required_evidence"`
}

type Gap struct {
	ObligationAnchor string   `json:"obligation_anchor"`
	Path             []Leaf   `json:"path"`
	Diagnostics      []string `json:"diagnostics"`
}

type Case struct {
	ID                   string   `json:"id"`
	Role                 string   `json:"role"`
	ReportingDomain      string   `json:"reporting_domain"`
	CircuitRole          string   `json:"circuit_role"`
	SafetyImpact         string   `json:"safety_impact"`
	Outcome              string   `json:"outcome"`
	Frontier             []Gap    `json:"frontier"`
	SatisfiedObligations []string `json:"satisfied_obligations"`
}

type Atom struct {
	Key        string `json:"key"`
	Category   string `json:"category"`
	Scope      string `json:"scope"`
	Capability string `json:"capability"`
}

type Member struct {
	Key        string `json:"key"`
	Stage      string `json:"stage"`
	Category   string `json:"category"`
	Scope      string `json:"scope"`
	Capability string `json:"capability"`
	Code       string `json:"code"`
}

type EffectPlan struct {
	DirectAtomKeys         []string `json:"direct_atom_keys"`
	DirectMemberKeys       []string `json:"direct_member_keys"`
	ClosureAtoms           []Atom   `json:"closure_atoms"`
	ClosureMembers         []Member `json:"closure_members"`
	PlannedMemberKeys      []string `json:"planned_member_keys"`
	RequiredEvidence       []string `json:"required_evidence"`
	Executable             bool     `json:"executable"`
	MechanicallyProven     bool     `json:"mechanically_proven"`
	UnboundedDynamicLookup bool     `json:"unbounded_dynamic_lookup"`
	UnmappedConsumers      []string `json:"unmapped_consumers"`
	PlanSHA256             string   `json:"plan_sha256"`
}

type Candidate struct {
	Key                         string           `json:"key"`
	DirectAtomKeys              []string         `json:"direct_atom_keys"`
	DirectMemberKeys            []string         `json:"direct_member_keys"`
	Atoms                       []Atom           `json:"atoms"`
	Members                     []Member         `json:"members"`
	FullyCoveredCaseIDs         []string         `json:"fully_covered_case_ids"`
	EffectExposureCaseIDs       []string         `json:"effect_exposure_case_ids"`
	Exposure                    []CaseExposure   `json:"exposure"`
	NonExposedCases             []CaseCommitment `json:"non_exposed_cases"`
	ReportingDomains            []string         `json:"reporting_domains"`
	CircuitRoles                []string         `json:"circuit_roles"`
	SafetyWeight                int              `json:"safety_weight"`
	ExposedNoncoveredCaseCount  int              `json:"exposed_noncovered_case_count"`
	NonselectedSiblingPathCount int              `json:"nonselected_sibling_path_count"`
	RequiredEvidence            []string         `json:"required_evidence"`
	EffectPlanSHA256            string           `json:"effect_plan_sha256"`
}

type CaseExposure struct {
	CaseID                       string   `json:"case_id"`
	SelectedPathHashes           []string `json:"selected_path_hashes"`
	NonselectedSiblingPathHashes []string `json:"nonselected_sibling_path_hashes"`
}

type CaseCommitment struct {
	CaseID     string `json:"case_id"`
	CaseSHA256 string `json:"case_sha256"`
}

type RoundState struct {
	Generation      int      `json:"generation"`
	UsedAtomCount   int      `json:"used_atom_count"`
	UsedMemberCount int      `json:"used_member_count"`
	PriorAtomKeys   []string `json:"prior_atom_keys"`
	ActiveCohortIDs []string `json:"active_cohort_ids"`
}

type Selection struct {
	Generation         int         `json:"generation"`
	CandidateCount     int         `json:"candidate_count"`
	EligibleCandidates []Candidate `json:"eligible_candidates"`
	CoRankOne          []Candidate `json:"co_rank_one"`
	Selected           Candidate   `json:"selected"`
}

type RoundEvidence struct {
	DeterministicReplayComplete bool `json:"deterministic_replay_complete"`
	PhysicalPromotionComplete   bool `json:"physical_promotion_complete"`
	SealEnvironmentValid        bool `json:"seal_environment_valid"`
	EffectClosureValid          bool `json:"effect_closure_valid"`
}

type Successor struct {
	CaseID        string `json:"case_id"`
	PriorPathHash string `json:"prior_path_hash"`
	Current       Gap    `json:"current"`
}

type EvaluationStatus string

const (
	EvaluationContinue       EvaluationStatus = "continue"
	EvaluationPublicAdmitted EvaluationStatus = "public_admitted"
)

type Evaluation struct {
	Status                   EvaluationStatus `json:"status"`
	DiscoveryPassBefore      int              `json:"discovery_pass_before"`
	DiscoveryPassAfter       int              `json:"discovery_pass_after"`
	NewActiveCohortPasses    int              `json:"new_active_cohort_passes"`
	AdvancedCaseIDs          []string         `json:"advanced_case_ids"`
	AdvancedReportingDomains []string         `json:"advanced_reporting_domains"`
	AdvancedCircuitRoles     []string         `json:"advanced_circuit_roles"`
	Successors               []Successor      `json:"successors"`
	NextState                RoundState       `json:"next_state"`
}
