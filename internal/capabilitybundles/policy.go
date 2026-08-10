package capabilitybundles

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
)

func DecodePolicy(reader io.Reader) (Policy, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("decode causal-unlock policy: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Policy{}, fmt.Errorf("decode causal-unlock policy: multiple JSON values")
		}
		return Policy{}, fmt.Errorf("decode causal-unlock policy trailer: %w", err)
	}
	if err := ValidatePolicy(policy); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func ValidatePolicy(policy Policy) error {
	if policy.Schema == "" || policy.Version <= 0 {
		return fmt.Errorf("causal-unlock policy schema and version are required")
	}
	if policy.CandidateUnit != CandidateUnitMinimalCausalUnlockBundle ||
		!slices.Equal(policy.AtomIdentityFields, atomIdentityFields) ||
		!slices.Equal(policy.MemberIdentityFields, memberIdentityFields) ||
		policy.IdentityEncoding != IdentityEncodingLengthPrefixed ||
		policy.CaseBlockerSource != CaseBlockerSourceNormalizedRootGaps ||
		policy.UnlockRule != UnlockRuleCompleteMemberCoverage {
		return fmt.Errorf("causal-unlock identity or coverage semantics are invalid")
	}
	if !slices.Equal(policy.EligibleBaselineOutcomes, []string{"unsupported", "exhausted"}) {
		return fmt.Errorf("causal-unlock eligible outcomes are invalid")
	}
	generation := policy.BundleGeneration
	if !generation.DiscoveryOnly || generation.MaximumCapabilityAtoms <= 0 ||
		generation.MaximumExactMembers <= 0 || generation.MaximumCandidateBundles <= 0 ||
		generation.MinimumAtomCaseSupport <= 0 || !generation.RequireMinimalUnlockSet ||
		!generation.RejectEmptyCaseBlockers || !generation.RejectDuplicateAtoms ||
		!generation.RejectDuplicateMembers || !generation.RejectDuplicateCases {
		return fmt.Errorf("causal-unlock generation policy is invalid")
	}
	if policy.Eligibility.MinimumUnlockedDiscoveryCases <= 0 ||
		policy.Eligibility.MinimumReportingDomains <= 0 {
		return fmt.Errorf("causal-unlock eligibility policy is invalid")
	}
	if !slices.Equal(policy.Ranking, semanticRanking) ||
		policy.PublicationOrder != PublicationOrderSemanticThenKey ||
		policy.TieBehavior != TieBehaviorFailBeforeIdentity {
		return fmt.Errorf("causal-unlock ranking policy is invalid")
	}
	plan := policy.RankOnePlanAdmission
	if !plan.RequireCompleteGenericPlan || !plan.RequireAllAtomsPlanned ||
		!plan.RequireAllMembersPlanned || plan.UnexecutableRankOne != UnexecutableRankOneNoFallback {
		return fmt.Errorf("causal-unlock plan admission is invalid")
	}
	if policy.HeldOutInfluence != HeldOutInfluenceProhibited ||
		policy.UnsafeCaseUnlockCredit != UnsafeUnlockCreditProhibited ||
		policy.SelectedMemberGapRemoval != SelectedMemberRemovalExact ||
		policy.HeldOutSuccessCausality != HeldOutCausalityCompleteCoverage {
		return fmt.Errorf("causal-unlock isolation or preservation policy is invalid")
	}
	admission := policy.PublicAdmission
	if !admission.RequireTotalPassUplift || !admission.RequireClaimedUnlockPassUplift ||
		admission.MinimumRealizedClaimedUnlocks <= 0 ||
		!admission.RequireNonselectedGapPreservation || !admission.RequireNoPassRegression ||
		!admission.RequireNoUnsafeToPass {
		return fmt.Errorf("causal-unlock public admission policy is invalid")
	}
	return nil
}
