// Package capabilitypackages groups typed discovery gaps into reusable,
// deterministically ranked capability packages.
package capabilitypackages

import (
	"kicadai/internal/capabilityexpansion"
)

const (
	PolicySchema  = "kicadai.closed-loop-open-set-selection-policy.v5"
	PolicyVersion = 5
	PlanSchema    = "kicadai.closed-loop-open-set-package-plan.v5"
	PlanVersion   = 5
)

type SelectionPolicy struct {
	Schema                      string               `json:"schema"`
	Version                     int                  `json:"version"`
	PackageIdentityFields       []string             `json:"package_identity_fields"`
	MemberIdentityFields        []string             `json:"member_identity_fields"`
	IdentityEncoding            string               `json:"identity_encoding"`
	AffectedCaseAggregation     string               `json:"affected_case_aggregation"`
	RequiredEvidenceAggregation string               `json:"required_evidence_aggregation"`
	Eligibility                 SelectionEligibility `json:"eligibility"`
	RankOnePlanAdmission        PlanAdmission        `json:"rank_one_plan_admission"`
	Ranking                     []string             `json:"ranking"`
	TieBehavior                 string               `json:"tie_behavior"`
	IneligibleBehavior          string               `json:"ineligible_behavior"`
	NoEligiblePackageBehavior   string               `json:"no_eligible_package_behavior"`
	UnexecutableRankOneBehavior string               `json:"unexecutable_rank_one_behavior"`
	HeldOutInfluence            string               `json:"held_out_influence"`
	SelectedMemberGapRemoval    string               `json:"selected_member_gap_removal"`
	HeldOutSuccessCausality     string               `json:"held_out_success_causality"`
}

type SelectionEligibility struct {
	MinimumAffectedDiscoveryCases int `json:"minimum_affected_discovery_cases"`
	MinimumReportingDomains       int `json:"minimum_reporting_domains"`
}

type PlanAdmission struct {
	RequireCompleteGenericPlan bool `json:"require_complete_generic_plan"`
	RequireAllMembersPlanned   bool `json:"require_all_members_planned"`
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
	Scope            string   `json:"scope"`
	Capability       string   `json:"capability"`
	Cases            []string `json:"cases"`
	Domains          []string `json:"domains"`
	SafetyWeight     int64    `json:"safety_weight"`
	Members          []Member `json:"members"`
	RequiredEvidence []string `json:"required_evidence"`
}

type Observation struct {
	Role             string   `json:"role"`
	CaseID           string   `json:"case_id"`
	ReportingDomain  string   `json:"reporting_domain"`
	SafetyWeight     int64    `json:"safety_weight"`
	Stage            string   `json:"stage"`
	Scope            string   `json:"scope"`
	Capability       string   `json:"capability"`
	Code             string   `json:"code"`
	RequiredEvidence []string `json:"required_evidence"`
}

type MemberBinding struct {
	MemberKey string `json:"member_key"`
	NeedID    string `json:"need_id"`
}

type GenericPlan struct {
	Schema        string                            `json:"schema"`
	Version       int                               `json:"version"`
	PackageKey    string                            `json:"package_key"`
	Members       []Member                          `json:"members"`
	Bindings      []MemberBinding                   `json:"bindings"`
	ExpansionPlan capabilityexpansion.ExpansionPlan `json:"expansion_plan"`
	Hash          string                            `json:"hash"`
}
