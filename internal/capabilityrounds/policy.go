package capabilityrounds

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

var semanticRanking = []string{
	"covered_active_case_count_desc",
	"covered_active_domain_count_desc",
	"covered_active_safety_weight_desc",
	"capability_atom_count_asc",
	"exact_member_count_asc",
}

type policyDocument struct {
	IdentityEncoding string   `json:"identity_encoding"`
	EligibleOutcomes []string `json:"eligible_outcomes"`
	AdaptiveRounds   struct {
		MaximumRounds               int    `json:"maximum_rounds"`
		MaximumTotalCapabilityAtoms int    `json:"maximum_total_capability_atoms"`
		MaximumTotalExactMembers    int    `json:"maximum_total_exact_members"`
		PriorAtomReselection        string `json:"prior_atom_reselection"`
	} `json:"adaptive_rounds"`
	Cohort struct {
		ExpectedDiscoveryCaseCount int `json:"expected_discovery_case_count"`
	} `json:"cohort"`
	BundleGeneration struct {
		MaximumRoundCapabilityAtoms  int    `json:"maximum_round_capability_atoms"`
		MaximumRoundExactMembers     int    `json:"maximum_round_exact_members"`
		MaximumCandidateBundles      int    `json:"maximum_candidate_bundles"`
		MinimumAtomActiveCaseSupport int    `json:"minimum_atom_active_case_support"`
		CandidateOverflow            string `json:"candidate_overflow"`
	} `json:"bundle_generation"`
	Eligibility struct {
		MinimumAdvancedActiveCases int  `json:"minimum_advanced_active_cases"`
		MinimumReportingDomains    int  `json:"minimum_reporting_domains"`
		AppliesToEveryRound        bool `json:"applies_to_every_round"`
		FinalRoundRelaxation       bool `json:"final_round_relaxation"`
	} `json:"eligibility"`
	Ranking             []string          `json:"ranking"`
	TieBehavior         string            `json:"tie_behavior"`
	UnknownGapStage     string            `json:"unknown_gap_stage"`
	GapStageOrder       []stageGroup      `json:"gap_stage_order"`
	GapStageAliases     map[string]string `json:"gap_stage_aliases"`
	SafetyWeightMapping map[string]int64  `json:"safety_weight_mapping"`
}

type stageGroup struct {
	Ordinal int      `json:"ordinal"`
	Stages  []string `json:"stages"`
}

func DecodePolicy(reader io.Reader) (Policy, error) {
	decoder := json.NewDecoder(reader)
	var document policyDocument
	if err := decoder.Decode(&document); err != nil {
		return Policy{}, fmt.Errorf("%w: decode: %v", ErrInvalidPolicy, err)
	}
	if err := requireEOF(decoder); err != nil {
		return Policy{}, err
	}
	policy := Policy{
		EligibleOutcomes:             slices.Clone(document.EligibleOutcomes),
		ExpectedDiscoveryCaseCount:   document.Cohort.ExpectedDiscoveryCaseCount,
		MaximumRounds:                document.AdaptiveRounds.MaximumRounds,
		MaximumTotalCapabilityAtoms:  document.AdaptiveRounds.MaximumTotalCapabilityAtoms,
		MaximumTotalExactMembers:     document.AdaptiveRounds.MaximumTotalExactMembers,
		MaximumRoundCapabilityAtoms:  document.BundleGeneration.MaximumRoundCapabilityAtoms,
		MaximumRoundExactMembers:     document.BundleGeneration.MaximumRoundExactMembers,
		MaximumCandidateBundles:      document.BundleGeneration.MaximumCandidateBundles,
		MinimumAtomActiveCaseSupport: document.BundleGeneration.MinimumAtomActiveCaseSupport,
		MinimumAdvancedActiveCases:   document.Eligibility.MinimumAdvancedActiveCases,
		MinimumReportingDomains:      document.Eligibility.MinimumReportingDomains,
		IdentityEncoding:             document.IdentityEncoding,
		Ranking:                      slices.Clone(document.Ranking),
		TieBehavior:                  document.TieBehavior,
		UnknownGapStage:              document.UnknownGapStage,
		StageOrdinal:                 map[string]int{},
		StageAliases:                 cloneMap(document.GapStageAliases),
		SafetyWeights:                cloneMap(document.SafetyWeightMapping),
	}
	for index, group := range document.GapStageOrder {
		if group.Ordinal != index || len(group.Stages) == 0 {
			return Policy{}, fmt.Errorf("%w: noncontiguous or empty stage group %d", ErrInvalidPolicy, index)
		}
		for _, stage := range group.Stages {
			if stage == "" {
				return Policy{}, fmt.Errorf("%w: empty stage", ErrInvalidPolicy)
			}
			if _, exists := policy.StageOrdinal[stage]; exists {
				return Policy{}, fmt.Errorf("%w: duplicate stage %q", ErrInvalidPolicy, stage)
			}
			policy.StageOrdinal[stage] = index
		}
	}
	if document.AdaptiveRounds.PriorAtomReselection != "prohibited" ||
		document.BundleGeneration.CandidateOverflow != "fail_closed_no_truncation" ||
		!document.Eligibility.AppliesToEveryRound || document.Eligibility.FinalRoundRelaxation {
		return Policy{}, fmt.Errorf("%w: fail-closed round rules", ErrInvalidPolicy)
	}
	if err := ValidatePolicy(policy); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func ValidatePolicy(policy Policy) error {
	if policy.IdentityEncoding != IdentityEncodingU32BigEndian || policy.TieBehavior != TieBehaviorCanonicalFallback ||
		policy.UnknownGapStage != UnknownStageFailClosed || !slices.Equal(policy.Ranking, semanticRanking) {
		return fmt.Errorf("%w: identity, tie, stage, or ranking contract", ErrInvalidPolicy)
	}
	if policy.ExpectedDiscoveryCaseCount <= 0 || policy.MaximumRounds <= 0 ||
		policy.MaximumTotalCapabilityAtoms <= 0 || policy.MaximumTotalExactMembers <= 0 ||
		policy.MaximumRoundCapabilityAtoms <= 0 || policy.MaximumRoundExactMembers <= 0 ||
		policy.MaximumRoundCapabilityAtoms > policy.MaximumTotalCapabilityAtoms ||
		policy.MaximumRoundExactMembers > policy.MaximumTotalExactMembers ||
		policy.MaximumCandidateBundles <= 0 || policy.MinimumAtomActiveCaseSupport < 2 ||
		policy.MinimumAdvancedActiveCases < 2 || policy.MinimumReportingDomains < 2 {
		return fmt.Errorf("%w: nonpositive or non-generic limits", ErrInvalidPolicy)
	}
	if !slices.Equal(policy.EligibleOutcomes, []string{"unsupported", "exhausted"}) || len(policy.StageOrdinal) == 0 || len(policy.SafetyWeights) == 0 {
		return fmt.Errorf("%w: missing outcome, stage, or safety vocabulary", ErrInvalidPolicy)
	}
	seenOutcomes := map[string]bool{}
	for _, outcome := range policy.EligibleOutcomes {
		if outcome == "" || seenOutcomes[outcome] {
			return fmt.Errorf("%w: invalid eligible outcome %q", ErrInvalidPolicy, outcome)
		}
		seenOutcomes[outcome] = true
	}
	for alias, target := range policy.StageAliases {
		if alias == "" || target == "" || alias == target {
			return fmt.Errorf("%w: invalid stage alias %q -> %q", ErrInvalidPolicy, alias, target)
		}
		if _, exists := policy.StageOrdinal[alias]; exists {
			return fmt.Errorf("%w: alias %q is also canonical", ErrInvalidPolicy, alias)
		}
		if _, exists := policy.StageOrdinal[target]; !exists {
			return fmt.Errorf("%w: alias target %q is unknown", ErrInvalidPolicy, target)
		}
	}
	for _, category := range []string{"non_safety", "review_required", "safety_relevant", "safety_critical"} {
		weight, exists := policy.SafetyWeights[category]
		if !exists || weight < 0 {
			return fmt.Errorf("%w: missing or negative safety weight %q", ErrInvalidPolicy, category)
		}
	}
	if len(policy.SafetyWeights) != 4 {
		return fmt.Errorf("%w: unexpected safety category", ErrInvalidPolicy)
	}
	return nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%w: trailing JSON", ErrInvalidPolicy)
		}
		return fmt.Errorf("%w: trailing JSON: %v", ErrInvalidPolicy, err)
	}
	return nil
}

func cloneMap[K comparable, V any](source map[K]V) map[K]V {
	result := make(map[K]V, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
