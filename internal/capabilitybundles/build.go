package capabilitybundles

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrNoEligibleBundle = errors.New("no eligible causal-unlock bundle")
	ErrAmbiguousRankOne = errors.New("causal-unlock rank one is tied")
	ErrRankOnePlan      = errors.New("causal-unlock rank-one plan is incomplete")
)

type normalizedCase struct {
	value      Case
	atomKeys   []string
	memberKeys []string
}

type atomAccumulator struct {
	atom     Atom
	cases    map[string]bool
	members  map[string]Member
	evidence map[string]bool
}

func Build(cases []Case, policy Policy) (Result, error) {
	if err := ValidatePolicy(policy); err != nil {
		return Result{}, err
	}
	normalized, atoms, err := normalizeCases(cases, policy)
	if err != nil {
		return Result{}, err
	}

	validSignatures := make([][]string, 0, len(normalized))
	rejections := make([]CaseRejection, 0)
	for _, current := range normalized {
		if !eligibleOutcome(current.value.Outcome, policy) {
			continue
		}
		reasons := make([]string, 0)
		if len(current.atomKeys) > policy.BundleGeneration.MaximumCapabilityAtoms {
			reasons = append(reasons, "maximum_capability_atoms")
		}
		for _, key := range current.atomKeys {
			if len(atoms[key].cases) < policy.BundleGeneration.MinimumAtomCaseSupport {
				reasons = append(reasons, "minimum_atom_case_support:"+key)
			}
		}
		if len(reasons) != 0 {
			slices.Sort(reasons)
			rejections = append(rejections, CaseRejection{CaseID: current.value.ID, Reasons: reasons})
			continue
		}
		validSignatures = append(validSignatures, slices.Clone(current.atomKeys))
	}

	bundleSets, err := buildBundleSets(validSignatures, policy.BundleGeneration)
	if err != nil {
		return Result{}, err
	}
	candidates := make([]Candidate, 0, len(bundleSets))
	for _, atomKeys := range bundleSets {
		candidate, err := buildCandidate(atomKeys, normalized, atoms, policy)
		if err != nil {
			return Result{}, err
		}
		candidates = append(candidates, candidate)
	}
	applyMinimality(candidates, policy)
	sortCandidates(candidates)
	rankEligibleCandidates(candidates)
	slices.SortFunc(rejections, func(left, right CaseRejection) int { return cmp.Compare(left.CaseID, right.CaseID) })
	return Result{Candidates: candidates, CaseRejections: rejections}, nil
}

func SelectRankOne(result Result) (Candidate, error) {
	eligible := make([]Candidate, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		if candidate.Eligible {
			eligible = append(eligible, candidate)
		}
	}
	if len(eligible) == 0 {
		return Candidate{}, ErrNoEligibleBundle
	}
	sortCandidates(eligible)
	if len(eligible) > 1 && compareCandidateSemantics(eligible[0], eligible[1]) == 0 {
		return Candidate{}, ErrAmbiguousRankOne
	}
	return eligible[0], nil
}

func AdmitRankOne(result Result, plans map[string]PlanEvidence) (Candidate, error) {
	candidate, err := SelectRankOne(result)
	if err != nil {
		return Candidate{}, err
	}
	plan, found := plans[candidate.Key]
	if !found || !plan.Executable {
		return Candidate{}, ErrRankOnePlan
	}
	atomKeys := make([]string, len(candidate.Atoms))
	for index := range candidate.Atoms {
		atomKeys[index] = candidate.Atoms[index].Key
	}
	memberKeys := make([]string, len(candidate.Members))
	for index := range candidate.Members {
		memberKeys[index] = candidate.Members[index].Key
	}
	if !canonicalUniqueStrings(plan.AtomKeys) || !canonicalUniqueStrings(plan.MemberKeys) ||
		!slices.Equal(plan.AtomKeys, atomKeys) || !slices.Equal(plan.MemberKeys, memberKeys) {
		return Candidate{}, ErrRankOnePlan
	}
	return candidate, nil
}

func normalizeCases(cases []Case, policy Policy) ([]normalizedCase, map[string]*atomAccumulator, error) {
	seenCases := map[string]bool{}
	atoms := map[string]*atomAccumulator{}
	normalized := make([]normalizedCase, 0, len(cases))
	for _, current := range cases {
		if current.Role != "discovery" {
			return nil, nil, fmt.Errorf("causal-unlock input contains non-discovery case %q", current.ID)
		}
		if current.ID == "" || current.ReportingDomain == "" || current.SafetyWeight < 0 {
			return nil, nil, fmt.Errorf("causal-unlock case metadata is invalid")
		}
		if policy.BundleGeneration.RejectDuplicateCases && seenCases[current.ID] {
			return nil, nil, fmt.Errorf("causal-unlock case %q is duplicated", current.ID)
		}
		seenCases[current.ID] = true
		if !validOutcome(current.Outcome) {
			return nil, nil, fmt.Errorf("causal-unlock case %q has unknown outcome %q", current.ID, current.Outcome)
		}
		if current.Outcome == "pass" && len(current.Gaps) != 0 {
			return nil, nil, fmt.Errorf("passing causal-unlock case %q contains root gaps", current.ID)
		}
		if eligibleOutcome(current.Outcome, policy) && policy.BundleGeneration.RejectEmptyCaseBlockers && len(current.Gaps) == 0 {
			return nil, nil, fmt.Errorf("causal-unlock case %q has an empty blocker set", current.ID)
		}

		atomKeys := map[string]bool{}
		memberKeys := map[string]bool{}
		for _, gap := range current.Gaps {
			if gap.Stage == "" || gap.Scope == "" || gap.Capability == "" || gap.Code == "" {
				return nil, nil, fmt.Errorf("causal-unlock case %q has an incomplete root member", current.ID)
			}
			member := Member{
				Key:   memberKey(gap.Stage, gap.Scope, gap.Capability, gap.Code),
				Stage: gap.Stage, Scope: gap.Scope, Capability: gap.Capability, Code: gap.Code,
			}
			if policy.BundleGeneration.RejectDuplicateMembers && memberKeys[member.Key] {
				return nil, nil, fmt.Errorf("causal-unlock case %q has duplicate root member %q", current.ID, member.Key)
			}
			memberKeys[member.Key] = true
			atomKey := atomKey(gap.Scope, gap.Capability)
			atomKeys[atomKey] = true
			if !eligibleOutcome(current.Outcome, policy) {
				continue
			}
			accumulator, found := atoms[atomKey]
			if !found {
				accumulator = &atomAccumulator{
					atom:  Atom{Key: atomKey, Scope: gap.Scope, Capability: gap.Capability},
					cases: map[string]bool{}, members: map[string]Member{}, evidence: map[string]bool{},
				}
				atoms[atomKey] = accumulator
			}
			accumulator.cases[current.ID] = true
			accumulator.members[member.Key] = member
			for _, evidence := range gap.RequiredEvidence {
				if evidence == "" {
					return nil, nil, fmt.Errorf("causal-unlock case %q has empty required evidence", current.ID)
				}
				accumulator.evidence[evidence] = true
			}
		}
		normalized = append(normalized, normalizedCase{
			value: current, atomKeys: sortedKeys(atomKeys), memberKeys: sortedKeys(memberKeys),
		})
	}
	slices.SortFunc(normalized, func(left, right normalizedCase) int { return cmp.Compare(left.value.ID, right.value.ID) })
	for _, accumulator := range atoms {
		accumulator.atom.Cases = sortedKeys(accumulator.cases)
		accumulator.atom.CaseSupport = len(accumulator.atom.Cases)
	}
	return normalized, atoms, nil
}

func buildBundleSets(signatures [][]string, policy BundleGeneration) ([][]string, error) {
	sets := map[string][]string{"": {}}
	for _, signature := range signatures {
		existing := make([][]string, 0, len(sets))
		for _, current := range sets {
			existing = append(existing, current)
		}
		for _, current := range existing {
			union := unionStrings(current, signature)
			if len(union) == 0 || len(union) > policy.MaximumCapabilityAtoms {
				continue
			}
			key := tuple(union...)
			if _, exists := sets[key]; exists {
				continue
			}
			if len(sets) > policy.MaximumCandidateBundles {
				return nil, fmt.Errorf("causal-unlock candidate ceiling exceeded")
			}
			sets[key] = union
		}
	}
	delete(sets, "")
	result := make([][]string, 0, len(sets))
	for _, current := range sets {
		result = append(result, current)
	}
	slices.SortFunc(result, func(left, right []string) int { return cmp.Compare(tuple(left...), tuple(right...)) })
	return result, nil
}

func buildCandidate(
	atomKeys []string,
	cases []normalizedCase,
	atoms map[string]*atomAccumulator,
	policy Policy,
) (Candidate, error) {
	candidate := Candidate{Key: tuple(atomKeys...)}
	memberMap := map[string]Member{}
	evidence := map[string]bool{}
	for _, key := range atomKeys {
		accumulator, found := atoms[key]
		if !found {
			return Candidate{}, fmt.Errorf("causal-unlock atom %q has no discovery evidence", key)
		}
		candidate.Atoms = append(candidate.Atoms, accumulator.atom)
		for memberKey, member := range accumulator.members {
			memberMap[memberKey] = member
		}
		for value := range accumulator.evidence {
			evidence[value] = true
		}
	}
	memberKeys := make([]string, 0, len(memberMap))
	for key := range memberMap {
		memberKeys = append(memberKeys, key)
	}
	slices.Sort(memberKeys)
	for _, key := range memberKeys {
		candidate.Members = append(candidate.Members, memberMap[key])
	}

	domains := map[string]bool{}
	for _, current := range cases {
		if !eligibleOutcome(current.value.Outcome, policy) || !isSubset(current.memberKeys, memberKeys) {
			continue
		}
		candidate.UnlockedCases = append(candidate.UnlockedCases, current.value.ID)
		domains[current.value.ReportingDomain] = true
		if current.value.SafetyWeight > math.MaxInt64-candidate.SafetyWeight {
			return Candidate{}, fmt.Errorf("causal-unlock safety weight overflow")
		}
		candidate.SafetyWeight += current.value.SafetyWeight
	}
	candidate.ReportingDomains = sortedKeys(domains)
	candidate.RequiredEvidence = sortedKeys(evidence)
	if len(candidate.Members) > policy.BundleGeneration.MaximumExactMembers {
		candidate.RejectionReasons = append(candidate.RejectionReasons, "maximum_exact_members")
	}
	if len(candidate.UnlockedCases) < policy.Eligibility.MinimumUnlockedDiscoveryCases {
		candidate.RejectionReasons = append(candidate.RejectionReasons, "minimum_unlocked_discovery_cases")
	}
	if len(candidate.ReportingDomains) < policy.Eligibility.MinimumReportingDomains {
		candidate.RejectionReasons = append(candidate.RejectionReasons, "minimum_reporting_domains")
	}
	slices.Sort(candidate.RejectionReasons)
	candidate.Eligible = len(candidate.RejectionReasons) == 0
	return candidate, nil
}

func applyMinimality(candidates []Candidate, policy Policy) {
	if !policy.BundleGeneration.RequireMinimalUnlockSet {
		return
	}
	for index := range candidates {
		currentAtomKeys := candidateAtomKeys(candidates[index])
		for otherIndex := range candidates {
			if index == otherIndex || len(candidates[otherIndex].Atoms) >= len(candidates[index].Atoms) ||
				!slices.Equal(candidates[otherIndex].UnlockedCases, candidates[index].UnlockedCases) {
				continue
			}
			if isSubset(candidateAtomKeys(candidates[otherIndex]), currentAtomKeys) {
				candidates[index].RejectionReasons = append(candidates[index].RejectionReasons, "nonminimal_unlock_set")
				candidates[index].Eligible = false
				break
			}
		}
		slices.Sort(candidates[index].RejectionReasons)
	}
}

func sortCandidates(candidates []Candidate) {
	slices.SortFunc(candidates, func(left, right Candidate) int {
		if left.Eligible != right.Eligible {
			if left.Eligible {
				return -1
			}
			return 1
		}
		if order := compareCandidateSemantics(left, right); order != 0 {
			return order
		}
		return cmp.Compare(left.Key, right.Key)
	})
}

func rankEligibleCandidates(candidates []Candidate) {
	rank := 0
	eligibleIndex := 0
	var previous Candidate
	for index := range candidates {
		if !candidates[index].Eligible {
			continue
		}
		eligibleIndex++
		if eligibleIndex == 1 || compareCandidateSemantics(previous, candidates[index]) != 0 {
			rank = eligibleIndex
		}
		candidates[index].Rank = rank
		previous = candidates[index]
	}
}

func compareCandidateSemantics(left, right Candidate) int {
	if order := cmp.Compare(len(right.UnlockedCases), len(left.UnlockedCases)); order != 0 {
		return order
	}
	if order := cmp.Compare(len(right.ReportingDomains), len(left.ReportingDomains)); order != 0 {
		return order
	}
	if order := cmp.Compare(right.SafetyWeight, left.SafetyWeight); order != 0 {
		return order
	}
	if order := cmp.Compare(len(left.Atoms), len(right.Atoms)); order != 0 {
		return order
	}
	return cmp.Compare(len(left.Members), len(right.Members))
}

func candidateAtomKeys(candidate Candidate) []string {
	result := make([]string, len(candidate.Atoms))
	for index := range candidate.Atoms {
		result[index] = candidate.Atoms[index].Key
	}
	return result
}

func atomKey(scope, capability string) string {
	return tuple(scope, capability)
}

func memberKey(stage, scope, capability, code string) string {
	return tuple(stage, scope, capability, code)
}

func tuple(values ...string) string {
	var result strings.Builder
	for _, value := range values {
		result.WriteString(strconv.Itoa(len(value)))
		result.WriteByte(':')
		result.WriteString(value)
	}
	return result.String()
}

func eligibleOutcome(outcome string, policy Policy) bool {
	return slices.Contains(policy.EligibleBaselineOutcomes, outcome)
}

func validOutcome(outcome string) bool {
	return outcome == "pass" || outcome == "unsupported" || outcome == "unsafe" || outcome == "exhausted"
}

func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	slices.Sort(result)
	return result
}

func unionStrings(left, right []string) []string {
	result := append(slices.Clone(left), right...)
	slices.Sort(result)
	return slices.Compact(result)
}

func isSubset(subset, superset []string) bool {
	for _, value := range subset {
		index, found := slices.BinarySearch(superset, value)
		if !found || index < 0 {
			return false
		}
	}
	return true
}

func canonicalUniqueStrings(values []string) bool {
	return sort.StringsAreSorted(values) && len(slices.Compact(slices.Clone(values))) == len(values)
}
