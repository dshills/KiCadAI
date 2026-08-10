package capabilitypackages

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"kicadai/internal/capabilityevaluation"
	"kicadai/internal/capabilityfeedback"
)

type caseContribution struct {
	domain string
	weight int64
}

type accumulator struct {
	scope        capabilityfeedback.GapScope
	capability   string
	cases        map[string]caseContribution
	domains      map[string]bool
	members      map[string]Member
	requirements map[string]bool
}

func DecodePolicy(reader io.Reader) (SelectionPolicy, error) {
	decoder := json.NewDecoder(io.LimitReader(reader, 1<<20))
	decoder.DisallowUnknownFields()
	var policy SelectionPolicy
	if err := decoder.Decode(&policy); err != nil {
		return SelectionPolicy{}, fmt.Errorf("decode selection policy: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return SelectionPolicy{}, fmt.Errorf("selection policy contains trailing data")
	}
	if err := ValidatePolicy(policy); err != nil {
		return SelectionPolicy{}, err
	}
	return policy, nil
}

func ValidatePolicy(policy SelectionPolicy) error {
	checks := []struct {
		name  string
		valid bool
	}{
		{"schema", policy.Schema == PolicySchema && policy.Version == PolicyVersion},
		{"package identity", slices.Equal(policy.PackageIdentityFields, []string{"scope", "capability"})},
		{"member identity", slices.Equal(policy.MemberIdentityFields, []string{"stage", "scope", "capability", "code"})},
		{"identity encoding", policy.IdentityEncoding == "utf8-decimal-byte-length-colon"},
		{"affected-case aggregation", policy.AffectedCaseAggregation == "sorted-unique-union"},
		{"required-evidence aggregation", policy.RequiredEvidenceAggregation == "sorted-unique-union"},
		{"eligibility", policy.Eligibility.MinimumAffectedDiscoveryCases > 0 && policy.Eligibility.MinimumReportingDomains > 0},
		{"plan admission", policy.RankOnePlanAdmission.RequireCompleteGenericPlan && policy.RankOnePlanAdmission.RequireAllMembersPlanned},
		{"tie behavior", policy.TieBehavior == "ranking_tuple_then_canonical_package_key"},
		{"ineligible behavior", policy.IneligibleBehavior == "exclude_before_ranking"},
		{"no eligible behavior", policy.NoEligiblePackageBehavior == "fail_closed"},
		{"unexecutable behavior", policy.UnexecutableRankOneBehavior == "fail_closed_no_fallback"},
		{"held-out influence", policy.HeldOutInfluence == "prohibited"},
		{"selected gap removal", policy.SelectedMemberGapRemoval == "exact_member_identity_only"},
		{"held-out causality", policy.HeldOutSuccessCausality == "at_least_one_new_pass_with_matching_baseline_package"},
	}
	for _, check := range checks {
		if !check.valid {
			return fmt.Errorf("selection policy %s is unsupported", check.name)
		}
	}
	knownRanking := []string{"affected_discovery_cases_desc", "reporting_domains_desc", "safety_impact_weight_desc", "member_cluster_count_asc", "required_evidence_count_asc", "canonical_package_key_asc"}
	if len(policy.Ranking) != len(knownRanking) {
		return fmt.Errorf("selection policy ranking clause count is invalid")
	}
	seen := map[string]bool{}
	for _, clause := range policy.Ranking {
		if seen[clause] || !slices.Contains(knownRanking, clause) {
			return fmt.Errorf("selection policy ranking clause %q is invalid", clause)
		}
		seen[clause] = true
	}
	return nil
}

func Build(report capabilityfeedback.AggregateReport, registry capabilityevaluation.ImpactRegistry, policy SelectionPolicy) ([]Candidate, error) {
	if err := ValidatePolicy(policy); err != nil {
		return nil, err
	}
	if report.CorpusRole != capabilityfeedback.RoleDiscovery {
		return nil, fmt.Errorf("capability packages require discovery-only evidence")
	}
	if err := capabilityfeedback.ValidateAggregateReport(report, registry); err != nil {
		return nil, fmt.Errorf("validate discovery evidence: %w", err)
	}
	cases := make(map[string]caseContribution, len(report.Cases))
	for _, current := range report.Cases {
		cases[current.Case.ID] = caseContribution{domain: string(current.Case.Domain), weight: safetyWeight(current.Case.SafetyImpact)}
	}
	groups := map[string]*accumulator{}
	for _, cluster := range report.Clusters {
		if cluster.Stage == "" || cluster.Scope == "" || cluster.Capability == "" || cluster.Code == "" {
			return nil, fmt.Errorf("discovery cluster has an incomplete member identity")
		}
		key := Tuple(string(cluster.Scope), cluster.Capability)
		group := groups[key]
		if group == nil {
			group = &accumulator{scope: cluster.Scope, capability: cluster.Capability, cases: map[string]caseContribution{}, domains: map[string]bool{}, members: map[string]Member{}, requirements: map[string]bool{}}
			groups[key] = group
		}
		memberKey := Tuple(cluster.Stage, string(cluster.Scope), cluster.Capability, cluster.Code)
		group.members[memberKey] = Member{Key: memberKey, Stage: cluster.Stage, Scope: cluster.Scope, Capability: cluster.Capability, Code: cluster.Code}
		for _, caseID := range cluster.Cases {
			contribution, found := cases[caseID]
			if !found {
				return nil, fmt.Errorf("cluster references unknown discovery case %q", caseID)
			}
			group.cases[caseID] = contribution
			group.domains[contribution.domain] = true
		}
		for _, evidence := range cluster.RequiredEvidence {
			if strings.TrimSpace(evidence) == "" {
				return nil, fmt.Errorf("cluster contains empty required evidence")
			}
			group.requirements[evidence] = true
		}
	}
	candidates := make([]Candidate, 0, len(groups))
	for key, group := range groups {
		if len(group.cases) < policy.Eligibility.MinimumAffectedDiscoveryCases || len(group.domains) < policy.Eligibility.MinimumReportingDomains {
			continue
		}
		candidate := Candidate{Key: key, Scope: group.scope, Capability: group.capability}
		for caseID, contribution := range group.cases {
			if contribution.weight < 0 || candidate.SafetyWeight > math.MaxInt64-contribution.weight {
				return nil, fmt.Errorf("capability package safety weight overflow")
			}
			candidate.SafetyWeight += contribution.weight
			candidate.Cases = append(candidate.Cases, caseID)
		}
		for domain := range group.domains {
			candidate.Domains = append(candidate.Domains, domain)
		}
		for _, member := range group.members {
			candidate.Members = append(candidate.Members, member)
		}
		for evidence := range group.requirements {
			candidate.RequiredEvidence = append(candidate.RequiredEvidence, evidence)
		}
		sort.Strings(candidate.Cases)
		sort.Strings(candidate.Domains)
		slices.SortFunc(candidate.Members, func(left, right Member) int { return cmp.Compare(left.Key, right.Key) })
		sort.Strings(candidate.RequiredEvidence)
		candidates = append(candidates, candidate)
	}
	slices.SortFunc(candidates, func(left, right Candidate) int { return compare(left, right, policy.Ranking) })
	for index := range candidates {
		candidates[index].Rank = index + 1
	}
	return candidates, nil
}

func SelectRankOne(candidates []Candidate, plan GenericPlan, policy SelectionPolicy) (Candidate, error) {
	if err := ValidatePolicy(policy); err != nil {
		return Candidate{}, err
	}
	if len(candidates) == 0 || candidates[0].Rank != 1 {
		return Candidate{}, fmt.Errorf("no eligible package: %s", policy.NoEligiblePackageBehavior)
	}
	selected := candidates[0]
	if err := ValidatePlan(plan); err != nil {
		return Candidate{}, fmt.Errorf("rank one is not executable: %s: %w", policy.UnexecutableRankOneBehavior, err)
	}
	if policy.RankOnePlanAdmission.RequireCompleteGenericPlan && plan.PackageKey != selected.Key {
		return Candidate{}, fmt.Errorf("rank one is not executable: %s", policy.UnexecutableRankOneBehavior)
	}
	if policy.RankOnePlanAdmission.RequireAllMembersPlanned {
		want := make([]string, len(selected.Members))
		got := make([]string, len(plan.Members))
		for index := range selected.Members {
			want[index] = selected.Members[index].Key
		}
		for index := range plan.Members {
			got[index] = plan.Members[index].Key
		}
		sort.Strings(want)
		sort.Strings(got)
		if !slices.Equal(want, got) {
			return Candidate{}, fmt.Errorf("rank one plan does not cover every member: %s", policy.UnexecutableRankOneBehavior)
		}
	}
	return selected, nil
}

func Tuple(values ...string) string {
	var encoded strings.Builder
	for _, value := range values {
		encoded.WriteString(strconv.Itoa(len(value)))
		encoded.WriteByte(':')
		encoded.WriteString(value)
	}
	return encoded.String()
}

func compare(left, right Candidate, ranking []string) int {
	for _, clause := range ranking {
		var order int
		switch clause {
		case "affected_discovery_cases_desc":
			order = cmp.Compare(len(right.Cases), len(left.Cases))
		case "reporting_domains_desc":
			order = cmp.Compare(len(right.Domains), len(left.Domains))
		case "safety_impact_weight_desc":
			order = cmp.Compare(right.SafetyWeight, left.SafetyWeight)
		case "member_cluster_count_asc":
			order = cmp.Compare(len(left.Members), len(right.Members))
		case "required_evidence_count_asc":
			order = cmp.Compare(len(left.RequiredEvidence), len(right.RequiredEvidence))
		case "canonical_package_key_asc":
			order = cmp.Compare(left.Key, right.Key)
		}
		if order != 0 {
			return order
		}
	}
	return 0
}

func safetyWeight(value capabilityevaluation.SafetyImpact) int64 {
	switch value {
	case capabilityevaluation.SafetyReviewRequired:
		return 1
	case capabilityevaluation.SafetyRelevant:
		return 3
	case capabilityevaluation.SafetyCritical:
		return 5
	default:
		return 0
	}
}
