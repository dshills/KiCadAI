package capabilityroundsv8

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

type hashedSuccessor struct {
	value Successor
	hash  string
}

// EvaluateRound validates one committed public V8 round, including exact
// effect-closure confinement and bounded append-only causal successors.
func EvaluateRound(previous, next []Case, selected Candidate, state RoundState, evidence RoundEvidence, policy Policy) (Evaluation, error) {
	if err := validatePolicy(policy); err != nil {
		return Evaluation{}, err
	}
	if !evidence.DeterministicReplayComplete || !evidence.PhysicalPromotionComplete || !evidence.SealEnvironmentValid || !evidence.EffectClosureValid {
		return Evaluation{}, fmt.Errorf("%w: incomplete round evidence", ErrRoundGate)
	}
	if err := validateState(state, policy); err != nil {
		return Evaluation{}, err
	}
	if err := validateSelected(selected, state, policy); err != nil {
		return Evaluation{}, err
	}
	priorValues, priorActive, err := normalizeCases(previous, policy)
	if err != nil {
		return Evaluation{}, err
	}
	if err := validateSelectedAgainstCases(selected, priorValues, policy); err != nil {
		return Evaluation{}, err
	}
	if err := validateStateCases(state, priorValues, priorActive); err != nil {
		return Evaluation{}, err
	}
	nextValues, _, err := normalizeCases(next, policy)
	if err != nil {
		return Evaluation{}, err
	}
	priorByID, nextByID := caseIndex(priorValues), caseIndex(nextValues)
	priorCaseIDs, nextCaseIDs := sortedMapKeys(priorByID), sortedMapKeys(nextByID)
	if !slices.Equal(priorCaseIDs, nextCaseIDs) {
		return Evaluation{}, fmt.Errorf("%w: case set changed", ErrRoundGate)
	}
	effectiveMembers := map[string]bool{}
	for _, member := range selected.Members {
		effectiveMembers[member.Key] = true
	}
	covered := stringSet(selected.CoveredCaseIDs)
	activeBefore := map[string]bool{}
	for _, current := range priorActive {
		activeBefore[current.value.ID] = true
	}
	advanced, advancedDomains, advancedRoles := map[string]bool{}, map[string]bool{}, map[string]bool{}
	successors := []hashedSuccessor{}
	passBefore, passAfter, newActivePasses := 0, 0, 0
	for _, caseID := range priorCaseIDs {
		prior, current := priorByID[caseID], nextByID[caseID]
		if prior.value.Role != current.value.Role || prior.value.ReportingDomain != current.value.ReportingDomain || prior.value.CircuitRole != current.value.CircuitRole || prior.value.SafetyImpact != current.value.SafetyImpact {
			return Evaluation{}, fmt.Errorf("%w: case metadata changed", ErrRoundGate)
		}
		if prior.value.Outcome == "pass" {
			passBefore++
			if current.value.Outcome != "pass" {
				return Evaluation{}, fmt.Errorf("%w: passing case regressed", ErrRoundGate)
			}
		}
		if current.value.Outcome == "pass" {
			passAfter++
			if activeBefore[caseID] {
				newActivePasses++
			}
		}
		if prior.value.Outcome == "unsafe" && current.value.Outcome == "pass" {
			return Evaluation{}, fmt.Errorf("%w: unsafe case became pass", ErrRoundGate)
		}
		caseAdvanced, caseSuccessors, err := evaluateCaseTransition(prior.value, current.value, effectiveMembers, covered[caseID], policy)
		if err != nil {
			return Evaluation{}, err
		}
		if !caseAdvanced && prior.value.Outcome != current.value.Outcome {
			return Evaluation{}, fmt.Errorf("%w: outcome changed outside selected closure", ErrRoundGate)
		}
		if covered[caseID] && !caseAdvanced {
			return Evaluation{}, fmt.Errorf("%w: covered case did not advance", ErrRoundGate)
		}
		if caseAdvanced && activeBefore[caseID] {
			advanced[caseID] = true
			advancedDomains[current.value.ReportingDomain] = true
			advancedRoles[current.value.CircuitRole] = true
		}
		successors = append(successors, caseSuccessors...)
	}
	if len(advanced) < policy.MinimumAdvancedCases || len(advancedDomains) < policy.MinimumDomains || len(advancedRoles) < policy.MinimumRoles {
		return Evaluation{}, fmt.Errorf("%w: insufficient diverse advancement", ErrRoundGate)
	}
	slices.SortFunc(successors, func(left, right hashedSuccessor) int {
		if order := cmp.Compare(left.value.CaseID, right.value.CaseID); order != 0 {
			return order
		}
		if order := cmp.Compare(left.value.PriorPathHash, right.value.PriorPathHash); order != 0 {
			return order
		}
		return cmp.Compare(left.hash, right.hash)
	})
	publishedSuccessors := make([]Successor, len(successors))
	for index := range successors {
		publishedSuccessors[index] = successors[index].value
	}
	priorAtoms := stringSet(state.PriorAtomKeys)
	for _, atom := range selected.Atoms {
		priorAtoms[atom.Key] = true
	}
	nextState := RoundState{Generation: state.Generation + 1, UsedAtomCount: state.UsedAtomCount + len(selected.Atoms), UsedMemberCount: state.UsedMemberCount + len(selected.Members), PriorAtomKeys: sortedSet(priorAtoms), ActiveCohortIDs: append([]string(nil), state.ActiveCohortIDs...)}
	status := EvaluationContinue
	if passAfter > passBefore && newActivePasses > 0 {
		status = EvaluationPublicAdmitted
	} else if nextState.Generation >= policy.MaximumRounds || nextState.UsedAtomCount >= policy.MaximumTotalAtoms || nextState.UsedMemberCount >= policy.MaximumTotalMembers {
		return Evaluation{}, fmt.Errorf("%w: no admission before frozen budget exhaustion", ErrRoundGate)
	}
	return Evaluation{Status: status, DiscoveryPassBefore: passBefore, DiscoveryPassAfter: passAfter, NewActiveCohortPasses: newActivePasses,
		AdvancedCaseIDs: sortedSet(advanced), AdvancedReportingDomains: sortedSet(advancedDomains), AdvancedCircuitRoles: sortedSet(advancedRoles), Successors: publishedSuccessors, NextState: nextState}, nil
}

func evaluateCaseTransition(prior, next Case, effectiveMembers map[string]bool, covered bool, policy Policy) (bool, []hashedSuccessor, error) {
	priorPaths, nextPaths := map[string]Gap{}, map[string]Gap{}
	for _, gap := range prior.Frontier {
		hash, err := PathHash(gap)
		if err != nil {
			return false, nil, err
		}
		priorPaths[hash] = gap
	}
	for _, gap := range next.Frontier {
		hash, err := PathHash(gap)
		if err != nil {
			return false, nil, err
		}
		nextPaths[hash] = gap
	}
	satisfied := stringSet(next.SatisfiedObligations)
	knownAnchors := map[string]bool{}
	for _, gap := range prior.Frontier {
		knownAnchors[gap.ObligationAnchor] = true
	}
	for anchor := range satisfied {
		if !knownAnchors[anchor] {
			return false, nil, fmt.Errorf("%w: unknown satisfied obligation", ErrRoundGate)
		}
	}
	for _, gap := range next.Frontier {
		if satisfied[gap.ObligationAnchor] {
			return false, nil, fmt.Errorf("%w: satisfied obligation retained a gap", ErrRoundGate)
		}
	}
	consumedNext := map[string]bool{}
	advanced := false
	successors := []hashedSuccessor{}
	for _, priorHash := range sortedMapKeys(priorPaths) {
		priorGap := priorPaths[priorHash]
		priorLeaf := priorGap.Path[len(priorGap.Path)-1]
		memberKey, err := MemberKey(priorLeaf)
		if err != nil {
			return false, nil, err
		}
		if current, exists := nextPaths[priorHash]; exists {
			consumedNext[priorHash] = true
			if !gapEqual(priorGap, current) {
				return false, nil, fmt.Errorf("%w: unchanged gap bytes changed", ErrRoundGate)
			}
			if covered && effectiveMembers[memberKey] {
				return false, nil, fmt.Errorf("%w: selected leaf persisted", ErrRoundGate)
			}
			continue
		}
		if !effectiveMembers[memberKey] {
			return false, nil, fmt.Errorf("%w: nonselected gap disappeared", ErrRoundGate)
		}
		advanced = true
		matches := []Gap{}
		for nextHash, candidate := range nextPaths {
			if consumedNext[nextHash] || candidate.ObligationAnchor != priorGap.ObligationAnchor || !pathIsSuccessor(priorGap, candidate) {
				continue
			}
			matches = append(matches, candidate)
		}
		if next.Outcome == "pass" || satisfied[priorGap.ObligationAnchor] {
			if len(matches) != 0 {
				return false, nil, fmt.Errorf("%w: satisfied obligation retained successors", ErrRoundGate)
			}
			continue
		}
		if len(matches) == 0 || len(matches) > policy.MaximumSuccessors {
			return false, nil, fmt.Errorf("%w: successor fan-out is outside bounds", ErrRoundGate)
		}
		seenMembers := map[string]bool{}
		for _, successor := range matches {
			nextHash, err := PathHash(successor)
			if err != nil {
				return false, nil, err
			}
			nextLeaf := successor.Path[len(successor.Path)-1]
			nextMember, err := MemberKey(nextLeaf)
			if err != nil {
				return false, nil, err
			}
			if nextMember == memberKey || seenMembers[nextMember] || policy.StageOrdinal[nextLeaf.Stage] < policy.StageOrdinal[priorLeaf.Stage] || !subset(priorLeaf.RequiredEvidence, stringSet(nextLeaf.RequiredEvidence)) {
				return false, nil, fmt.Errorf("%w: invalid causal successor", ErrRoundGate)
			}
			seenMembers[nextMember] = true
			consumedNext[nextHash] = true
			successors = append(successors, hashedSuccessor{value: Successor{CaseID: prior.ID, PriorPathHash: priorHash, Current: successor}, hash: nextHash})
		}
	}
	for nextHash := range nextPaths {
		if !consumedNext[nextHash] {
			return false, nil, fmt.Errorf("%w: unexplained new gap", ErrRoundGate)
		}
	}
	return advanced, successors, nil
}

func pathIsSuccessor(prior, next Gap) bool {
	if len(next.Path) != len(prior.Path)+1 {
		return false
	}
	for index := range prior.Path {
		if !leafEqual(prior.Path[index], next.Path[index]) {
			return false
		}
	}
	return true
}

func gapEqual(left, right Gap) bool {
	if left.ObligationAnchor != right.ObligationAnchor || !slices.Equal(left.Diagnostics, right.Diagnostics) || len(left.Path) != len(right.Path) {
		return false
	}
	for index := range left.Path {
		if !leafEqual(left.Path[index], right.Path[index]) {
			return false
		}
	}
	return true
}

func leafEqual(left, right Leaf) bool {
	return left.Stage == right.Stage && left.Category == right.Category && left.Scope == right.Scope && left.Capability == right.Capability && left.Code == right.Code && slices.Equal(left.RequiredEvidence, right.RequiredEvidence)
}

func validateSelected(selected Candidate, state RoundState, policy Policy) error {
	if !digestPattern.MatchString(selected.EffectPlanSHA256) || !strings.HasPrefix(selected.Key, "bundle:") || !digestPattern.MatchString(strings.TrimPrefix(selected.Key, "bundle:")) {
		return fmt.Errorf("%w: selected identity", ErrRoundGate)
	}
	if len(selected.Atoms) == 0 || len(selected.Atoms) > policy.MaximumRoundAtoms || len(selected.Members) == 0 || len(selected.Members) > policy.MaximumRoundMembers || state.UsedAtomCount+len(selected.Atoms) > policy.MaximumTotalAtoms || state.UsedMemberCount+len(selected.Members) > policy.MaximumTotalMembers {
		return fmt.Errorf("%w: selected budget", ErrRoundGate)
	}
	atomKeys, memberKeys := []string{}, []string{}
	prior := stringSet(state.PriorAtomKeys)
	for _, atom := range selected.Atoms {
		key, err := AtomKey(atom.Category, atom.Scope, atom.Capability)
		if err != nil || key != atom.Key || prior[key] {
			return fmt.Errorf("%w: selected atom", ErrRoundGate)
		}
		atomKeys = append(atomKeys, key)
	}
	for _, member := range selected.Members {
		key, err := MemberKey(Leaf{Stage: member.Stage, Category: member.Category, Scope: member.Scope, Capability: member.Capability, Code: member.Code})
		if err != nil || key != member.Key {
			return fmt.Errorf("%w: selected member", ErrRoundGate)
		}
		memberKeys = append(memberKeys, key)
	}
	if _, err := sortedUnique(atomKeys); err != nil {
		return err
	}
	if _, err := sortedUnique(memberKeys); err != nil {
		return err
	}
	if _, err := sortedUnique(selected.DirectAtomKeys); err != nil || !subset(selected.DirectAtomKeys, stringSet(atomKeys)) {
		return fmt.Errorf("%w: direct atom set", ErrRoundGate)
	}
	if _, err := sortedUnique(selected.DirectMemberKeys); err != nil || !subset(selected.DirectMemberKeys, stringSet(memberKeys)) {
		return fmt.Errorf("%w: direct member set", ErrRoundGate)
	}
	if _, err := sortedUnique(selected.CoveredCaseIDs); err != nil || len(selected.CoveredCaseIDs) < policy.MinimumAdvancedCases {
		return fmt.Errorf("%w: covered cases", ErrRoundGate)
	}
	return nil
}

func validateSelectedAgainstCases(selected Candidate, cases []normalizedCase, policy Policy) error {
	directMembers := stringSet(selected.DirectMemberKeys)
	covered, domains, roles := []string{}, map[string]bool{}, map[string]bool{}
	safety := 0
	for _, current := range cases {
		if !policy.EligibleOutcomes[current.value.Outcome] || !subset(current.memberKeys, directMembers) {
			continue
		}
		covered = append(covered, current.value.ID)
		domains[current.value.ReportingDomain] = true
		roles[current.value.CircuitRole] = true
		safety += policy.SafetyWeights[current.value.SafetyImpact]
	}
	if !slices.Equal(covered, selected.CoveredCaseIDs) || !slices.Equal(sortedSet(domains), selected.ReportingDomains) || !slices.Equal(sortedSet(roles), selected.CircuitRoles) || safety != selected.SafetyWeight {
		return fmt.Errorf("%w: selected coverage does not reproduce", ErrRoundGate)
	}
	for _, atomKey := range selected.DirectAtomKeys {
		support := 0
		for _, current := range cases {
			if policy.EligibleOutcomes[current.value.Outcome] && slices.Contains(current.atomKeys, atomKey) {
				support++
			}
		}
		if support < policy.MinimumCaseSupport {
			return fmt.Errorf("%w: selected atom support does not reproduce", ErrRoundGate)
		}
	}
	atomKeys, memberKeys, atomSet := []string{}, []string{}, map[string]bool{}
	for _, atom := range selected.Atoms {
		atomKeys = append(atomKeys, atom.Key)
		atomSet[atom.Key] = true
	}
	for _, member := range selected.Members {
		memberKeys = append(memberKeys, member.Key)
		atomKey, _ := AtomKey(member.Category, member.Scope, member.Capability)
		if !atomSet[atomKey] {
			return fmt.Errorf("%w: selected member has no atom", ErrRoundGate)
		}
	}
	wantKey := "bundle:" + identityDigest(signature(atomKeys, memberKeys), selected.EffectPlanSHA256)
	if selected.Key != wantKey {
		return fmt.Errorf("%w: selected key does not reproduce", ErrRoundGate)
	}
	return nil
}

func caseIndex(values []normalizedCase) map[string]normalizedCase {
	result := make(map[string]normalizedCase, len(values))
	for _, value := range values {
		result[value.value.ID] = value
	}
	return result
}
