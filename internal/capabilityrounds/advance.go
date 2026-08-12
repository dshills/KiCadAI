package capabilityrounds

import (
	"fmt"
	"slices"
)

type evaluatedCase struct {
	value    Case
	fullGaps map[string]Gap
	members  map[string]bool
	pass     bool
	unsafe   bool
	active   bool
}

func EvaluateRound(previous, next []Case, selected Candidate, state RoundState, edges []LineageEdge, evidence RoundEvidence, policy Policy) (Evaluation, error) {
	if err := ValidatePolicy(policy); err != nil {
		return Evaluation{}, err
	}
	if !evidence.DeterministicReplayComplete || !evidence.PhysicalPromotionComplete || !evidence.SealEnvironmentValid {
		return Evaluation{}, fmt.Errorf("%w: incomplete replay, physical, or seal evidence", ErrRoundGate)
	}
	if err := validateStateForEvaluation(state, policy); err != nil {
		return Evaluation{}, err
	}
	selectedAtoms, selectedMembers, err := validateSelectedCandidate(selected, policy)
	if err != nil {
		return Evaluation{}, err
	}
	before, err := evaluateCaseSet(previous, policy)
	if err != nil {
		return Evaluation{}, err
	}
	after, err := evaluateCaseSet(next, policy)
	if err != nil {
		return Evaluation{}, err
	}
	if !sameKeys(before, after) {
		return Evaluation{}, fmt.Errorf("%w: discovery case set changed", ErrRoundGate)
	}

	covered := stringSet(selected.CoveredCaseIDs)
	if len(covered) != len(selected.CoveredCaseIDs) || !slices.IsSorted(selected.CoveredCaseIDs) || len(covered) < policy.MinimumAdvancedActiveCases {
		return Evaluation{}, fmt.Errorf("%w: invalid covered cohort", ErrRoundGate)
	}
	actualCovered := map[string]bool{}
	for caseID, prior := range before {
		if prior.active && frontierCovered(prior, selectedMembers) {
			actualCovered[caseID] = true
		}
	}
	if len(actualCovered) != len(covered) {
		return Evaluation{}, fmt.Errorf("%w: selected cohort is not exhaustive", ErrRoundGate)
	}
	for caseID := range actualCovered {
		if !covered[caseID] {
			return Evaluation{}, fmt.Errorf("%w: selected cohort is not exhaustive", ErrRoundGate)
		}
	}
	advancedDomains := map[string]bool{}
	selectedSafetyWeight := int64(0)
	for caseID := range covered {
		prior, exists := before[caseID]
		if !exists || !prior.active || !frontierCovered(prior, selectedMembers) {
			return Evaluation{}, fmt.Errorf("%w: selected cohort case %q is not completely covered", ErrRoundGate, caseID)
		}
		advancedDomains[prior.value.ReportingDomain] = true
		selectedSafetyWeight += policy.SafetyWeights[prior.value.SafetyImpact]
	}
	if len(advancedDomains) < policy.MinimumReportingDomains {
		return Evaluation{}, fmt.Errorf("%w: selected cohort lacks domain reuse", ErrRoundGate)
	}
	if !slices.Equal(selected.ReportingDomains, sortedSetKeys(advancedDomains)) || selected.SafetyWeight != selectedSafetyWeight {
		return Evaluation{}, fmt.Errorf("%w: selected cohort aggregate mismatch", ErrRoundGate)
	}

	edgeByFrom, err := validateLineageEdges(edges, before, after, policy)
	if err != nil {
		return Evaluation{}, err
	}
	usedEdges := map[string]bool{}
	passBefore, passAfter, newCohortPasses := 0, 0, 0
	for caseID, prior := range before {
		current := after[caseID]
		if prior.pass {
			passBefore++
			if !current.pass {
				return Evaluation{}, fmt.Errorf("%w: passing case %q regressed", ErrRoundGate, caseID)
			}
		}
		if current.pass {
			passAfter++
		}
		if prior.unsafe && current.pass {
			return Evaluation{}, fmt.Errorf("%w: unsafe case %q became pass", ErrRoundGate, caseID)
		}
		if covered[caseID] && !prior.pass && current.pass {
			newCohortPasses++
		}
		for fullKey, oldGap := range prior.fullGaps {
			memberKey, keyErr := canonicalMemberKey(oldGap, policy)
			if keyErr != nil {
				return Evaluation{}, keyErr
			}
			if selectedMembers[memberKey] {
				if current.members[memberKey] {
					return Evaluation{}, fmt.Errorf("%w: selected member persists in case %q", ErrRoundGate, caseID)
				}
				continue
			}
			if _, persists := current.fullGaps[fullKey]; persists {
				continue
			}
			edgeKey, keyErr := lineageKey(caseID, fullKey)
			if keyErr != nil {
				return Evaluation{}, keyErr
			}
			_, exists := edgeByFrom[edgeKey]
			if !exists {
				return Evaluation{}, fmt.Errorf("%w: nonselected gap disappeared without lineage in case %q", ErrRoundGate, caseID)
			}
			usedEdges[edgeKey] = true
		}
		for memberKey := range current.members {
			if selectedMembers[memberKey] {
				return Evaluation{}, fmt.Errorf("%w: selected member reappears in case %q", ErrRoundGate, caseID)
			}
		}
	}
	if len(usedEdges) != len(edgeByFrom) {
		return Evaluation{}, fmt.Errorf("%w: unused or unrelated lineage edge", ErrRoundGate)
	}

	selectedAtomKeys := sortedSetKeys(selectedAtoms)
	for atomKey := range selectedAtoms {
		if _, found := slices.BinarySearch(state.PriorAtomKeys, atomKey); found {
			return Evaluation{}, fmt.Errorf("%w: selected atom was used previously", ErrRoundGate)
		}
	}
	nextAtomKeys := mergeSortedStrings(state.PriorAtomKeys, selectedAtomKeys)
	nextState := RoundState{
		Generation:          state.Generation + 1,
		UsedCapabilityAtoms: state.UsedCapabilityAtoms + len(selectedAtoms),
		UsedExactMembers:    state.UsedExactMembers + len(selectedMembers),
		PriorAtomKeys:       nextAtomKeys,
	}
	if nextState.UsedCapabilityAtoms > policy.MaximumTotalCapabilityAtoms || nextState.UsedExactMembers > policy.MaximumTotalExactMembers {
		return Evaluation{}, fmt.Errorf("%w: total budget exceeded", ErrRoundGate)
	}
	result := Evaluation{
		Status:                   EvaluationContinue,
		DiscoveryPassBefore:      passBefore,
		DiscoveryPassAfter:       passAfter,
		NewActiveCohortPasses:    newCohortPasses,
		AdvancedCaseIDs:          sortedSetKeys(covered),
		AdvancedReportingDomains: sortedSetKeys(advancedDomains),
		NextState:                nextState,
	}
	if passAfter > passBefore && newCohortPasses > 0 {
		result.Status = EvaluationPublicAdmitted
		return result, nil
	}
	if nextState.Generation >= policy.MaximumRounds {
		return Evaluation{}, fmt.Errorf("%w: round ceiling reached without public uplift", ErrRoundGate)
	}
	return result, nil
}

func evaluateCaseSet(cases []Case, policy Policy) (map[string]evaluatedCase, error) {
	if len(cases) != policy.ExpectedDiscoveryCaseCount {
		return nil, fmt.Errorf("%w: discovery case count %d, want %d", ErrRoundGate, len(cases), policy.ExpectedDiscoveryCaseCount)
	}
	result := make(map[string]evaluatedCase, len(cases))
	for _, value := range cases {
		if value.ID == "" || value.Role != "discovery" || value.ReportingDomain == "" || value.SafetyImpact == "" {
			return nil, fmt.Errorf("%w: malformed case %q", ErrRoundGate, value.ID)
		}
		if _, duplicate := result[value.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate case %q", ErrRoundGate, value.ID)
		}
		if _, exists := policy.SafetyWeights[value.SafetyImpact]; !exists {
			return nil, fmt.Errorf("%w: unknown safety impact", ErrRoundGate)
		}
		current := evaluatedCase{value: value, fullGaps: map[string]Gap{}, members: map[string]bool{}}
		switch value.Outcome {
		case "pass":
			current.pass = true
			if len(value.Frontier) != 0 {
				return nil, fmt.Errorf("%w: passing case %q has frontier", ErrRoundGate, value.ID)
			}
		case "unsafe", "unsupported", "exhausted":
			current.unsafe = value.Outcome == "unsafe"
			current.active = slices.Contains(policy.EligibleOutcomes, value.Outcome)
			if len(value.Frontier) == 0 {
				return nil, fmt.Errorf("%w: nonpassing case %q has empty frontier", ErrRoundGate, value.ID)
			}
		default:
			return nil, fmt.Errorf("%w: unknown outcome %q", ErrRoundGate, value.Outcome)
		}
		for _, gap := range value.Frontier {
			fullKey, err := canonicalFullGapKey(gap, policy)
			if err != nil {
				return nil, err
			}
			if _, duplicate := current.fullGaps[fullKey]; duplicate {
				return nil, fmt.Errorf("%w: duplicate full gap in case %q", ErrRoundGate, value.ID)
			}
			memberKey, err := canonicalMemberKey(gap, policy)
			if err != nil {
				return nil, err
			}
			if current.members[memberKey] {
				return nil, fmt.Errorf("%w: duplicate member in case %q", ErrRoundGate, value.ID)
			}
			current.fullGaps[fullKey] = gap
			current.members[memberKey] = true
		}
		result[value.ID] = current
	}
	return result, nil
}

func validateLineageEdges(edges []LineageEdge, before, after map[string]evaluatedCase, policy Policy) (map[string]LineageEdge, error) {
	result := make(map[string]LineageEdge, len(edges))
	for _, edge := range edges {
		prior, beforeExists := before[edge.CaseID]
		current, afterExists := after[edge.CaseID]
		if !beforeExists || !afterExists || edge.From.CausalToken == "" || edge.From.CausalToken != edge.To.CausalToken ||
			edge.From.Scope != edge.To.Scope || edge.From.Capability != edge.To.Capability {
			return nil, fmt.Errorf("%w: invalid lineage identity for case %q", ErrRoundGate, edge.CaseID)
		}
		fromKey, err := canonicalFullGapKey(edge.From, policy)
		if err != nil {
			return nil, err
		}
		toKey, err := canonicalFullGapKey(edge.To, policy)
		if err != nil {
			return nil, err
		}
		if _, exists := prior.fullGaps[fromKey]; !exists {
			return nil, fmt.Errorf("%w: lineage source is not a prior gap", ErrRoundGate)
		}
		if _, exists := current.fullGaps[toKey]; !exists {
			return nil, fmt.Errorf("%w: lineage destination is not a current gap", ErrRoundGate)
		}
		fromStage, err := canonicalStage(edge.From.Stage, policy)
		if err != nil {
			return nil, err
		}
		toStage, err := canonicalStage(edge.To.Stage, policy)
		if err != nil {
			return nil, err
		}
		if policy.StageOrdinal[toStage] < policy.StageOrdinal[fromStage] || !stringSubset(edge.From.RequiredEvidence, edge.To.RequiredEvidence) {
			return nil, fmt.Errorf("%w: lineage regresses stage or evidence", ErrRoundGate)
		}
		key, err := lineageKey(edge.CaseID, fromKey)
		if err != nil {
			return nil, err
		}
		if _, duplicate := result[key]; duplicate {
			return nil, fmt.Errorf("%w: duplicate lineage source", ErrRoundGate)
		}
		result[key] = edge
	}
	return result, nil
}

func validateSelectedCandidate(selected Candidate, policy Policy) (map[string]bool, map[string]bool, error) {
	if len(selected.Atoms) == 0 || len(selected.Members) == 0 || len(selected.Atoms) > policy.MaximumRoundCapabilityAtoms || len(selected.Members) > policy.MaximumRoundExactMembers {
		return nil, nil, fmt.Errorf("%w: invalid selected bundle size", ErrRoundGate)
	}
	atoms := map[string]bool{}
	members := map[string]bool{}
	atomMemberCount := map[string]int{}
	seed := candidateSeed{atoms: map[string]Atom{}, members: map[string]Member{}}
	for index, atom := range selected.Atoms {
		key, err := AtomKey(atom.Scope, atom.Capability)
		if err != nil || key != atom.Key || atoms[key] || (index > 0 && selected.Atoms[index-1].Key >= key) {
			return nil, nil, fmt.Errorf("%w: invalid selected atom", ErrRoundGate)
		}
		atoms[key] = true
		seed.atoms[key] = atom
	}
	for index, member := range selected.Members {
		stage, err := canonicalStage(member.Stage, policy)
		if err != nil {
			return nil, nil, err
		}
		key, err := MemberKey(stage, member.Scope, member.Capability, member.Code)
		if err != nil || key != member.Key || members[key] || (index > 0 && selected.Members[index-1].Key >= key) {
			return nil, nil, fmt.Errorf("%w: invalid selected member", ErrRoundGate)
		}
		atomKey, atomErr := AtomKey(member.Scope, member.Capability)
		if atomErr != nil || !atoms[atomKey] {
			return nil, nil, fmt.Errorf("%w: selected member has no selected atom", ErrRoundGate)
		}
		atomMemberCount[atomKey]++
		members[key] = true
		seed.members[key] = member
	}
	for atomKey := range atoms {
		if atomMemberCount[atomKey] == 0 {
			return nil, nil, fmt.Errorf("%w: selected atom has no exact member", ErrRoundGate)
		}
	}
	wantKey, err := candidateKey(seed)
	if err != nil {
		return nil, nil, err
	}
	if selected.Key != wantKey {
		return nil, nil, fmt.Errorf("%w: selected candidate key mismatch", ErrRoundGate)
	}
	return atoms, members, nil
}

func validateStateForEvaluation(state RoundState, policy Policy) error {
	if state.Generation < 0 || state.Generation >= policy.MaximumRounds || state.UsedCapabilityAtoms != len(state.PriorAtomKeys) ||
		state.UsedCapabilityAtoms < 0 || state.UsedExactMembers < state.UsedCapabilityAtoms ||
		state.UsedCapabilityAtoms > policy.MaximumTotalCapabilityAtoms || state.UsedExactMembers > policy.MaximumTotalExactMembers ||
		len(stringSet(state.PriorAtomKeys)) != len(state.PriorAtomKeys) || !slices.IsSorted(state.PriorAtomKeys) {
		return fmt.Errorf("%w: invalid round state", ErrRoundGate)
	}
	return nil
}

func canonicalStage(stage string, policy Policy) (string, error) {
	if alias, exists := policy.StageAliases[stage]; exists {
		stage = alias
	}
	if _, exists := policy.StageOrdinal[stage]; !exists {
		return "", fmt.Errorf("%w: unknown gap stage %q", ErrRoundGate, stage)
	}
	return stage, nil
}

func canonicalMemberKey(gap Gap, policy Policy) (string, error) {
	stage, err := canonicalStage(gap.Stage, policy)
	if err != nil {
		return "", err
	}
	return MemberKey(stage, gap.Scope, gap.Capability, gap.Code)
}

func canonicalFullGapKey(gap Gap, policy Policy) (string, error) {
	if gap.CausalToken == "" || len(gap.RequiredEvidence) == 0 || !slices.IsSorted(gap.RequiredEvidence) {
		return "", fmt.Errorf("%w: noncanonical gap evidence", ErrRoundGate)
	}
	for index, value := range gap.RequiredEvidence {
		if value == "" || (index > 0 && value == gap.RequiredEvidence[index-1]) {
			return "", fmt.Errorf("%w: empty or duplicate gap evidence", ErrRoundGate)
		}
	}
	memberKey, err := canonicalMemberKey(gap, policy)
	if err != nil {
		return "", err
	}
	fields := append([]string{"member", memberKey, "evidence"}, gap.RequiredEvidence...)
	return tuple(fields...)
}

func frontierCovered(current evaluatedCase, selectedMembers map[string]bool) bool {
	for memberKey := range current.members {
		if !selectedMembers[memberKey] {
			return false
		}
	}
	return true
}

func sameKeys(left, right map[string]evaluatedCase) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, exists := right[key]; !exists {
			return false
		}
	}
	return true
}

func lineageKey(caseID, fullGapKey string) (string, error) {
	return tuple("case", caseID, "gap", fullGapKey)
}

func stringSubset(left, right []string) bool {
	if len(left) > len(right) || !slices.IsSorted(left) || !slices.IsSorted(right) {
		return false
	}
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) && rightIndex < len(right) {
		switch {
		case left[leftIndex] == right[rightIndex]:
			leftIndex++
			rightIndex++
		case left[leftIndex] > right[rightIndex]:
			rightIndex++
		default:
			return false
		}
	}
	return leftIndex == len(left)
}

func mergeSortedStrings(left, right []string) []string {
	result := make([]string, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) || rightIndex < len(right) {
		var next string
		switch {
		case rightIndex >= len(right) || leftIndex < len(left) && left[leftIndex] < right[rightIndex]:
			next = left[leftIndex]
			leftIndex++
		case leftIndex >= len(left) || right[rightIndex] < left[leftIndex]:
			next = right[rightIndex]
			rightIndex++
		default:
			next = left[leftIndex]
			leftIndex++
			rightIndex++
		}
		if len(result) == 0 || result[len(result)-1] != next {
			result = append(result, next)
		}
	}
	return result
}
