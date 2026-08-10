package capabilitybundles

const (
	CandidateUnitMinimalCausalUnlockBundle = "minimal_causal_unlock_bundle"
	IdentityEncodingLengthPrefixed         = "utf8-byte-length-prefixed-fields"
	CaseBlockerSourceNormalizedRootGaps    = "complete_normalized_typed_root_gaps_after_causal_suppression"
	UnlockRuleCompleteMemberCoverage       = "case_exact_root_member_set_is_subset_of_bundle_exact_member_set"
	PublicationOrderSemanticThenKey        = "semantic_ranking_then_canonical_bundle_key_asc"
	TieBehaviorFailBeforeIdentity          = "fail_closed_before_identity_order"
	HeldOutInfluenceProhibited             = "prohibited"
	UnsafeUnlockCreditProhibited           = "prohibited"
	SelectedMemberRemovalExact             = "exact_selected_member_identities_only"
	UnexecutableRankOneNoFallback          = "fail_closed_no_fallback"
	HeldOutCausalityCompleteCoverage       = "newly_passing_case_complete_baseline_root_member_set_covered_by_selected_bundle_boolean_only"
)

var (
	atomIdentityFields   = []string{"scope", "capability"}
	memberIdentityFields = []string{"stage", "scope", "capability", "code"}
	semanticRanking      = []string{
		"unlocked_case_count_desc",
		"unlocked_domain_count_desc",
		"unlocked_safety_weight_desc",
		"capability_atom_count_asc",
		"exact_member_count_asc",
	}
)

type Policy struct {
	Schema                   string           `json:"schema"`
	Version                  int              `json:"version"`
	CandidateUnit            string           `json:"candidate_unit"`
	AtomIdentityFields       []string         `json:"atom_identity_fields"`
	MemberIdentityFields     []string         `json:"member_identity_fields"`
	IdentityEncoding         string           `json:"identity_encoding"`
	CaseBlockerSource        string           `json:"case_blocker_source"`
	UnlockRule               string           `json:"unlock_rule"`
	EligibleBaselineOutcomes []string         `json:"eligible_baseline_outcomes"`
	BundleGeneration         BundleGeneration `json:"bundle_generation"`
	Eligibility              Eligibility      `json:"eligibility"`
	Ranking                  []string         `json:"ranking"`
	PublicationOrder         string           `json:"publication_order"`
	TieBehavior              string           `json:"tie_behavior"`
	RankOnePlanAdmission     PlanAdmission    `json:"rank_one_plan_admission"`
	HeldOutInfluence         string           `json:"held_out_influence"`
	UnsafeCaseUnlockCredit   string           `json:"unsafe_case_unlock_credit"`
	SelectedMemberGapRemoval string           `json:"selected_member_gap_removal"`
	PublicAdmission          PublicAdmission  `json:"public_admission"`
	HeldOutSuccessCausality  string           `json:"held_out_success_causality"`
}

type BundleGeneration struct {
	DiscoveryOnly           bool `json:"discovery_only"`
	MaximumCapabilityAtoms  int  `json:"maximum_capability_atoms"`
	MaximumExactMembers     int  `json:"maximum_exact_members"`
	MaximumCandidateBundles int  `json:"maximum_candidate_bundles"`
	MinimumAtomCaseSupport  int  `json:"minimum_atom_case_support"`
	RequireMinimalUnlockSet bool `json:"require_minimal_unlock_set"`
	RejectEmptyCaseBlockers bool `json:"reject_empty_case_blockers"`
	RejectDuplicateAtoms    bool `json:"reject_duplicate_atoms"`
	RejectDuplicateMembers  bool `json:"reject_duplicate_members"`
	RejectDuplicateCases    bool `json:"reject_duplicate_cases"`
}

type Eligibility struct {
	MinimumUnlockedDiscoveryCases int `json:"minimum_unlocked_discovery_cases"`
	MinimumReportingDomains       int `json:"minimum_reporting_domains"`
}

type PlanAdmission struct {
	RequireCompleteGenericPlan bool   `json:"require_complete_generic_plan"`
	RequireAllAtomsPlanned     bool   `json:"require_all_atoms_planned"`
	RequireAllMembersPlanned   bool   `json:"require_all_members_planned"`
	UnexecutableRankOne        string `json:"unexecutable_rank_one"`
}

type PublicAdmission struct {
	RequireTotalPassUplift            bool `json:"require_total_pass_uplift"`
	RequireClaimedUnlockPassUplift    bool `json:"require_claimed_unlock_pass_uplift"`
	MinimumRealizedClaimedUnlocks     int  `json:"minimum_realized_claimed_unlocks"`
	RequireNonselectedGapPreservation bool `json:"require_nonselected_gap_preservation"`
	RequireNoPassRegression           bool `json:"require_no_pass_regression"`
	RequireNoUnsafeToPass             bool `json:"require_no_unsafe_to_pass"`
}

type Gap struct {
	Stage            string   `json:"stage"`
	Scope            string   `json:"scope"`
	Capability       string   `json:"capability"`
	Code             string   `json:"code"`
	RequiredEvidence []string `json:"required_evidence,omitempty"`
}

type Case struct {
	Role            string `json:"role"`
	ID              string `json:"id"`
	ReportingDomain string `json:"reporting_domain"`
	SafetyWeight    int64  `json:"safety_weight"`
	Outcome         string `json:"outcome"`
	Gaps            []Gap  `json:"gaps,omitempty"`
}

type Atom struct {
	Key         string   `json:"key"`
	Scope       string   `json:"scope"`
	Capability  string   `json:"capability"`
	Cases       []string `json:"cases"`
	CaseSupport int      `json:"case_support"`
}

type Member struct {
	Key        string `json:"key"`
	Stage      string `json:"stage"`
	Scope      string `json:"scope"`
	Capability string `json:"capability"`
	Code       string `json:"code"`
}

type Candidate struct {
	Rank             int      `json:"rank"`
	Key              string   `json:"key"`
	Atoms            []Atom   `json:"atoms"`
	Members          []Member `json:"members"`
	UnlockedCases    []string `json:"unlocked_cases"`
	ReportingDomains []string `json:"reporting_domains"`
	SafetyWeight     int64    `json:"safety_weight"`
	RequiredEvidence []string `json:"required_evidence"`
	Eligible         bool     `json:"eligible"`
	RejectionReasons []string `json:"rejection_reasons,omitempty"`
}

type CaseRejection struct {
	CaseID  string   `json:"case_id"`
	Reasons []string `json:"reasons"`
}

type Result struct {
	Candidates     []Candidate     `json:"candidates"`
	CaseRejections []CaseRejection `json:"case_rejections,omitempty"`
}

type PlanEvidence struct {
	Executable bool     `json:"executable"`
	AtomKeys   []string `json:"atom_keys"`
	MemberKeys []string `json:"member_keys"`
}
