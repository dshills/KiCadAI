package capabilityroundsv8

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"slices"
	"sort"
)

type normalizedCase struct {
	value      Case
	atomKeys   []string
	memberKeys []string
	atoms      map[string]Atom
	members    map[string]Member
	evidence   []string
}

type directSeed struct {
	atoms   map[string]Atom
	members map[string]Member
}

type compactSeed struct {
	atoms   []uint64
	members []uint64
}

type compactKey [sha256.Size]byte

type seedVocabulary struct {
	atomKeys    []string
	memberKeys  []string
	atomIndex   map[string]int
	memberIndex map[string]int
	atoms       map[string]Atom
	members     map[string]Member
}

type indexedEffectPlan struct {
	plan EffectPlan
	seed directSeed
}

func Select(cases []Case, plans []EffectPlan, state RoundState, policy Policy) (Selection, error) {
	if err := validatePolicy(policy); err != nil {
		return Selection{}, err
	}
	normalized, active, err := normalizeCases(cases, policy)
	if err != nil {
		return Selection{}, err
	}
	if len(active) == 0 {
		return Selection{}, fmt.Errorf("%w: active cohort is empty", ErrInvalidInput)
	}
	// V8 freezes exactly 18 discovery cases and complete enumeration. Keep the
	// exponential boundary explicit before the shift; later corpus sizes require
	// a new protocol version rather than an implicit heuristic.
	if len(active) > policy.ExpectedDiscoveryCases {
		return Selection{}, fmt.Errorf("%w: active cohort exceeds frozen complete-closure bound", ErrInvalidInput)
	}
	if err := validateState(state, policy); err != nil {
		return Selection{}, err
	}
	if err := validateStateCases(state, normalized, active); err != nil {
		return Selection{}, err
	}
	vocabulary := buildSeedVocabulary(active)
	planIndex, err := indexPlans(plans, vocabulary)
	if err != nil {
		return Selection{}, err
	}
	compactCases := make([]compactSeed, len(active))
	for index, current := range active {
		compactCases[index] = compactCase(current, vocabulary)
	}
	atomSupport := make(map[string]int, len(vocabulary.atomKeys))
	for _, current := range active {
		for atomKey := range current.atoms {
			atomSupport[atomKey]++
		}
	}
	limit := uint64(1) << uint(len(active))
	seeds := map[compactKey]bool{}
	seedHasher := sha256.New()
	seed := compactSeed{atoms: make([]uint64, wordCount(len(vocabulary.atomKeys))), members: make([]uint64, wordCount(len(vocabulary.memberKeys)))}
	eligible := []Candidate{}
	priorAtoms := stringSet(state.PriorAtomKeys)
	for mask := uint64(1); mask < limit; mask++ {
		clear(seed.atoms)
		clear(seed.members)
		for index := range active {
			if mask&(uint64(1)<<uint(index)) == 0 {
				continue
			}
			unionCompact(seed.atoms, compactCases[index].atoms)
			unionCompact(seed.members, compactCases[index].members)
		}
		seedKey := compactSeedKey(seed, seedHasher)
		if !seeds[seedKey] {
			seeds[seedKey] = true
			if indexed, exists := planIndex[seedKey]; exists {
				candidate, err := buildCandidate(indexed.seed, indexed.plan, normalized, atomSupport, priorAtoms, state, policy)
				if err != nil {
					return Selection{}, err
				}
				if candidate.Key != "" {
					eligible = append(eligible, candidate)
				}
			}
		}
		if len(seeds) > policy.MaximumCandidates {
			return Selection{}, ErrCandidateOverflow
		}
	}
	eligible, err = pruneDominated(eligible, policy)
	if err != nil {
		return Selection{}, err
	}
	if len(eligible) == 0 {
		return Selection{}, ErrNoEligibleBundle
	}
	sort.Slice(eligible, func(i, j int) bool {
		if order := compareSemantics(eligible[i], eligible[j]); order != 0 {
			return order < 0
		}
		return eligible[i].Key < eligible[j].Key
	})
	coRank := []Candidate{}
	for _, candidate := range eligible {
		if compareSemantics(candidate, eligible[0]) != 0 {
			break
		}
		coRank = append(coRank, candidate)
	}
	return Selection{Generation: state.Generation, CandidateCount: len(seeds), EligibleCandidates: eligible, CoRankOne: coRank, Selected: coRank[0]}, nil
}

func normalizeCases(cases []Case, policy Policy) ([]normalizedCase, []normalizedCase, error) {
	if len(cases) != policy.ExpectedDiscoveryCases {
		return nil, nil, fmt.Errorf("%w: discovery case count", ErrInvalidInput)
	}
	values := append([]Case(nil), cases...)
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	result := make([]normalizedCase, 0, len(values))
	active := []normalizedCase{}
	seenCases := map[string]bool{}
	for _, current := range values {
		if !tokenPattern.MatchString(current.ID) || seenCases[current.ID] || current.Role != "discovery" || !policy.ReportingDomains[current.ReportingDomain] || !policy.CircuitRoles[current.CircuitRole] {
			return nil, nil, fmt.Errorf("%w: case identity", ErrInvalidInput)
		}
		seenCases[current.ID] = true
		if _, exists := policy.SafetyWeights[current.SafetyImpact]; !exists {
			return nil, nil, fmt.Errorf("%w: safety impact", ErrInvalidInput)
		}
		if current.Outcome != "pass" && current.Outcome != "unsafe" && !policy.EligibleOutcomes[current.Outcome] {
			return nil, nil, fmt.Errorf("%w: outcome", ErrInvalidInput)
		}
		if current.Outcome == "pass" && len(current.Frontier) != 0 {
			return nil, nil, fmt.Errorf("%w: passing case has frontier", ErrInvalidInput)
		}
		if policy.EligibleOutcomes[current.Outcome] && len(current.Frontier) == 0 {
			return nil, nil, fmt.Errorf("%w: eligible case has empty frontier", ErrInvalidInput)
		}
		if _, err := sortedUnique(current.SatisfiedObligations); err != nil {
			return nil, nil, err
		}
		for _, anchor := range current.SatisfiedObligations {
			if !digestPattern.MatchString(anchor) {
				return nil, nil, fmt.Errorf("%w: satisfied obligation", ErrInvalidInput)
			}
		}
		normalized := normalizedCase{value: current, atoms: map[string]Atom{}, members: map[string]Member{}}
		pathSeen := map[string]bool{}
		evidence := map[string]bool{}
		for _, gap := range current.Frontier {
			pathHash, err := validateGap(gap, policy)
			if err != nil || pathSeen[pathHash] {
				return nil, nil, fmt.Errorf("%w: duplicate or invalid gap path", ErrInvalidInput)
			}
			pathSeen[pathHash] = true
			leaf := gap.Path[len(gap.Path)-1]
			atomKey, _ := AtomKey(leaf.Category, leaf.Scope, leaf.Capability)
			memberKey, _ := MemberKey(leaf)
			normalized.atoms[atomKey] = Atom{Key: atomKey, Category: leaf.Category, Scope: leaf.Scope, Capability: leaf.Capability}
			normalized.members[memberKey] = Member{Key: memberKey, Stage: leaf.Stage, Category: leaf.Category, Scope: leaf.Scope, Capability: leaf.Capability, Code: leaf.Code}
			for _, item := range leaf.RequiredEvidence {
				evidence[item] = true
			}
		}
		normalized.atomKeys = sortedMapKeys(normalized.atoms)
		normalized.memberKeys = sortedMapKeys(normalized.members)
		normalized.evidence = sortedSet(evidence)
		result = append(result, normalized)
		if policy.EligibleOutcomes[current.Outcome] {
			active = append(active, normalized)
		}
	}
	return result, active, nil
}

func validateGap(gap Gap, policy Policy) (string, error) {
	pathHash, err := PathHash(gap)
	if err != nil {
		return "", err
	}
	if _, err := sortedUnique(gap.Diagnostics); err != nil || len(gap.Diagnostics) == 0 {
		return "", err
	}
	for _, leaf := range gap.Path {
		if policy.StageOrdinal[leaf.Stage] == 0 || !policy.GapCategories[leaf.Category] || leaf.Stage != leaf.Category {
			return "", fmt.Errorf("%w: unknown or inconsistent gap stage", ErrInvalidInput)
		}
		if _, err := MemberKey(leaf); err != nil {
			return "", err
		}
		if _, err := sortedUnique(leaf.RequiredEvidence); err != nil || len(leaf.RequiredEvidence) == 0 {
			return "", fmt.Errorf("%w: required evidence", ErrInvalidInput)
		}
	}
	return pathHash, nil
}

func indexPlans(plans []EffectPlan, vocabulary seedVocabulary) (map[compactKey]indexedEffectPlan, error) {
	result := map[compactKey]indexedEffectPlan{}
	seedHasher := sha256.New()
	for _, plan := range plans {
		if _, err := sortedUnique(plan.DirectAtomKeys); err != nil {
			return nil, err
		}
		if _, err := sortedUnique(plan.DirectMemberKeys); err != nil {
			return nil, err
		}
		seed := directSeed{atoms: map[string]Atom{}, members: map[string]Member{}}
		compact := compactSeed{
			atoms:   make([]uint64, wordCount(len(vocabulary.atomKeys))),
			members: make([]uint64, wordCount(len(vocabulary.memberKeys))),
		}
		for _, atomKey := range plan.DirectAtomKeys {
			index, exists := vocabulary.atomIndex[atomKey]
			if !exists {
				return nil, fmt.Errorf("%w: effect plan references unknown direct atom", ErrInvalidInput)
			}
			seed.atoms[atomKey] = vocabulary.atoms[atomKey]
			setCompactBit(compact.atoms, index)
		}
		for _, memberKey := range plan.DirectMemberKeys {
			index, exists := vocabulary.memberIndex[memberKey]
			if !exists {
				return nil, fmt.Errorf("%w: effect plan references unknown direct member", ErrInvalidInput)
			}
			seed.members[memberKey] = vocabulary.members[memberKey]
			setCompactBit(compact.members, index)
		}
		key := compactSeedKey(compact, seedHasher)
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("%w: duplicate effect plan", ErrInvalidInput)
		}
		result[key] = indexedEffectPlan{plan: plan, seed: seed}
	}
	return result, nil
}

func buildCandidate(seed directSeed, plan EffectPlan, cases []normalizedCase, atomSupport map[string]int, priorAtoms map[string]bool, state RoundState, policy Policy) (Candidate, error) {
	directAtomKeys, directMemberKeys := sortedAtomMemberKeys(seed)
	if !slices.Equal(directAtomKeys, plan.DirectAtomKeys) || !slices.Equal(directMemberKeys, plan.DirectMemberKeys) {
		return Candidate{}, fmt.Errorf("%w: effect plan direct set mismatch", ErrInvalidInput)
	}
	if !plan.Executable || !plan.MechanicallyProven || plan.UnboundedDynamicLookup || len(plan.UnmappedConsumers) != 0 || !digestPattern.MatchString(plan.PlanSHA256) {
		return Candidate{}, nil
	}
	if _, err := sortedUnique(plan.UnmappedConsumers); err != nil {
		return Candidate{}, fmt.Errorf("%w: unmapped consumer set", ErrInvalidInput)
	}
	atoms := cloneAtomMap(seed.atoms)
	members := cloneMemberMap(seed.members)
	seenClosureAtoms, seenClosureMembers := map[string]bool{}, map[string]bool{}
	priorClosureAtomKey := ""
	for _, atom := range plan.ClosureAtoms {
		key, err := AtomKey(atom.Category, atom.Scope, atom.Capability)
		if err != nil || atom.Key != key || seenClosureAtoms[key] || priorClosureAtomKey >= key && priorClosureAtomKey != "" {
			return Candidate{}, fmt.Errorf("%w: closure atom", ErrInvalidInput)
		}
		seenClosureAtoms[key] = true
		priorClosureAtomKey = key
		atoms[key] = atom
	}
	priorClosureMemberKey := ""
	for _, member := range plan.ClosureMembers {
		leaf := Leaf{Stage: member.Stage, Category: member.Category, Scope: member.Scope, Capability: member.Capability, Code: member.Code}
		key, err := MemberKey(leaf)
		atomKey, atomErr := AtomKey(member.Category, member.Scope, member.Capability)
		if err != nil || atomErr != nil || member.Key != key || atoms[atomKey].Key == "" || seenClosureMembers[key] || priorClosureMemberKey >= key && priorClosureMemberKey != "" {
			return Candidate{}, fmt.Errorf("%w: closure member", ErrInvalidInput)
		}
		seenClosureMembers[key] = true
		priorClosureMemberKey = key
		members[key] = member
	}
	atomKeys, memberKeys := sortedAtomMemberKeys(directSeed{atoms: atoms, members: members})
	memberAtoms := map[string]bool{}
	for _, member := range members {
		atomKey, _ := AtomKey(member.Category, member.Scope, member.Capability)
		memberAtoms[atomKey] = true
	}
	if !subset(atomKeys, memberAtoms) {
		return Candidate{}, fmt.Errorf("%w: effect atom has no exact member", ErrInvalidInput)
	}
	if _, err := sortedUnique(plan.PlannedMemberKeys); err != nil || !slices.Equal(plan.PlannedMemberKeys, memberKeys) {
		return Candidate{}, fmt.Errorf("%w: incomplete effect plan", ErrInvalidInput)
	}
	if _, err := sortedUnique(plan.RequiredEvidence); err != nil || !subset(policy.MechanicalEvidence, stringSet(plan.RequiredEvidence)) {
		return Candidate{}, fmt.Errorf("%w: effect evidence", ErrInvalidInput)
	}
	if len(atomKeys) > policy.MaximumRoundAtoms || len(memberKeys) > policy.MaximumRoundMembers || state.UsedAtomCount+len(atomKeys) > policy.MaximumTotalAtoms || state.UsedMemberCount+len(memberKeys) > policy.MaximumTotalMembers {
		return Candidate{}, nil
	}
	for _, key := range atomKeys {
		if priorAtoms[key] {
			return Candidate{}, nil
		}
	}
	covered, domains, roles, evidence := []string{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	safety := 0
	for _, current := range cases {
		if !policy.EligibleOutcomes[current.value.Outcome] || !subset(current.memberKeys, stringSet(directMemberKeys)) {
			continue
		}
		covered = append(covered, current.value.ID)
		domains[current.value.ReportingDomain] = true
		roles[current.value.CircuitRole] = true
		safety += policy.SafetyWeights[current.value.SafetyImpact]
		for _, item := range current.evidence {
			evidence[item] = true
		}
	}
	for _, item := range plan.RequiredEvidence {
		evidence[item] = true
	}
	if len(covered) < policy.MinimumCaseSupport || len(domains) < policy.MinimumDomains || len(roles) < policy.MinimumRoles {
		return Candidate{}, nil
	}
	for _, directAtom := range directAtomKeys {
		if atomSupport[directAtom] < policy.MinimumCaseSupport {
			return Candidate{}, nil
		}
	}
	candidate := Candidate{DirectAtomKeys: directAtomKeys, DirectMemberKeys: directMemberKeys, Atoms: atomValues(atoms), Members: memberValues(members), CoveredCaseIDs: covered,
		ReportingDomains: sortedSet(domains), CircuitRoles: sortedSet(roles), SafetyWeight: safety, RequiredEvidence: sortedSet(evidence), EffectPlanSHA256: plan.PlanSHA256}
	candidate.Key = "bundle:" + identityDigest(signature(atomKeys, memberKeys), plan.PlanSHA256)
	return candidate, nil
}

func compareSemantics(left, right Candidate) int {
	for _, order := range []int{
		cmp.Compare(len(right.CoveredCaseIDs), len(left.CoveredCaseIDs)),
		cmp.Compare(len(right.ReportingDomains), len(left.ReportingDomains)),
		cmp.Compare(len(right.CircuitRoles), len(left.CircuitRoles)),
		cmp.Compare(right.SafetyWeight, left.SafetyWeight),
		cmp.Compare(len(left.Atoms), len(right.Atoms)),
		cmp.Compare(len(left.Members), len(right.Members)),
	} {
		if order != 0 {
			return order
		}
	}
	return 0
}

func pruneDominated(values []Candidate, policy Policy) ([]Candidate, error) {
	caseSet := map[string]bool{}
	for _, candidate := range values {
		for _, caseID := range candidate.CoveredCaseIDs {
			caseSet[caseID] = true
		}
	}
	caseIDs := sortedSet(caseSet)
	if len(caseIDs) > policy.ExpectedDiscoveryCases {
		return nil, fmt.Errorf("%w: candidate case universe", ErrInvalidInput)
	}
	caseIndex := make(map[string]int, len(caseIDs))
	for index, caseID := range caseIDs {
		caseIndex[caseID] = index
	}
	limit := 1 << len(caseIDs)
	exactCosts := make([]uint64, limit)
	masks := make([]int, len(values))
	for index, candidate := range values {
		mask := 0
		for _, caseID := range candidate.CoveredCaseIDs {
			mask |= 1 << caseIndex[caseID]
		}
		masks[index] = mask
		costBit, err := dominanceCostBit(len(candidate.Atoms), len(candidate.Members), policy)
		if err != nil {
			return nil, err
		}
		exactCosts[mask] |= costBit
	}
	supersetCosts := append([]uint64(nil), exactCosts...)
	for bit := 0; bit < len(caseIDs); bit++ {
		for mask := 0; mask < limit; mask++ {
			if mask&(1<<bit) == 0 {
				supersetCosts[mask] |= supersetCosts[mask|(1<<bit)]
			}
		}
	}
	strictSupersetCosts := make([]uint64, limit)
	for mask := 0; mask < limit; mask++ {
		for bit := 0; bit < len(caseIDs); bit++ {
			if mask&(1<<bit) == 0 {
				strictSupersetCosts[mask] |= supersetCosts[mask|(1<<bit)]
			}
		}
	}
	result := make([]Candidate, 0, len(values))
	for index, candidate := range values {
		atoms, members := len(candidate.Atoms), len(candidate.Members)
		allAffordable, strictlyCheaper := dominanceCostMasks(atoms, members, policy)
		if supersetCosts[masks[index]]&strictlyCheaper != 0 || strictSupersetCosts[masks[index]]&allAffordable != 0 {
			continue
		}
		result = append(result, candidate)
	}
	return result, nil
}

func dominanceCostBit(atoms, members int, policy Policy) (uint64, error) {
	index := atoms*(policy.MaximumRoundMembers+1) + members
	if atoms < 0 || atoms > policy.MaximumRoundAtoms || members < 0 || members > policy.MaximumRoundMembers || index >= 64 {
		return 0, fmt.Errorf("%w: dominance cost is outside bounds", ErrInvalidInput)
	}
	return uint64(1) << uint(index), nil
}

func dominanceCostMasks(atoms, members int, policy Policy) (all, strict uint64) {
	for atomCost := 0; atomCost <= atoms; atomCost++ {
		for memberCost := 0; memberCost <= members; memberCost++ {
			bit, _ := dominanceCostBit(atomCost, memberCost, policy)
			all |= bit
			if atomCost < atoms || memberCost < members {
				strict |= bit
			}
		}
	}
	return all, strict
}

func validateState(state RoundState, policy Policy) error {
	if state.Generation < 0 || state.Generation >= policy.MaximumRounds || state.UsedAtomCount < 0 || state.UsedMemberCount < 0 || state.UsedAtomCount > policy.MaximumTotalAtoms || state.UsedMemberCount > policy.MaximumTotalMembers {
		return fmt.Errorf("%w: round state", ErrInvalidInput)
	}
	if _, err := sortedUnique(state.PriorAtomKeys); err != nil || len(state.PriorAtomKeys) != state.UsedAtomCount {
		return fmt.Errorf("%w: prior atoms", ErrInvalidInput)
	}
	if _, err := sortedUnique(state.ActiveCohortIDs); err != nil {
		return fmt.Errorf("%w: active cohort", ErrInvalidInput)
	}
	return nil
}

func validateStateCases(state RoundState, cases, active []normalizedCase) error {
	caseIDs, activeIDs := map[string]bool{}, map[string]bool{}
	for _, current := range cases {
		caseIDs[current.value.ID] = true
	}
	for _, current := range active {
		activeIDs[current.value.ID] = true
	}
	if state.Generation == 0 {
		if !slices.Equal(state.ActiveCohortIDs, sortedSet(activeIDs)) {
			return fmt.Errorf("%w: generation-zero active cohort", ErrInvalidInput)
		}
		return nil
	}
	cohort := stringSet(state.ActiveCohortIDs)
	if !subset(sortedSet(activeIDs), cohort) || !subset(state.ActiveCohortIDs, caseIDs) {
		return fmt.Errorf("%w: active cohort does not contain current eligible cases", ErrInvalidInput)
	}
	return nil
}

func sortedAtomMemberKeys(seed directSeed) ([]string, []string) {
	return sortedMapKeys(seed.atoms), sortedMapKeys(seed.members)
}
func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func stringSet(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}
func cloneAtomMap(values map[string]Atom) map[string]Atom {
	result := map[string]Atom{}
	for key, value := range values {
		result[key] = value
	}
	return result
}
func cloneMemberMap(values map[string]Member) map[string]Member {
	result := map[string]Member{}
	for key, value := range values {
		result[key] = value
	}
	return result
}
func atomValues(values map[string]Atom) []Atom {
	keys := sortedMapKeys(values)
	result := make([]Atom, len(keys))
	for i, key := range keys {
		result[i] = values[key]
	}
	return result
}
func memberValues(values map[string]Member) []Member {
	keys := sortedMapKeys(values)
	result := make([]Member, len(keys))
	for i, key := range keys {
		result[i] = values[key]
	}
	return result
}

func buildSeedVocabulary(cases []normalizedCase) seedVocabulary {
	atoms, members := map[string]Atom{}, map[string]Member{}
	for _, current := range cases {
		for key, atom := range current.atoms {
			atoms[key] = atom
		}
		for key, member := range current.members {
			members[key] = member
		}
	}
	result := seedVocabulary{atomKeys: sortedMapKeys(atoms), memberKeys: sortedMapKeys(members), atomIndex: map[string]int{}, memberIndex: map[string]int{}, atoms: atoms, members: members}
	for index, key := range result.atomKeys {
		result.atomIndex[key] = index
	}
	for index, key := range result.memberKeys {
		result.memberIndex[key] = index
	}
	return result
}

func compactCase(current normalizedCase, vocabulary seedVocabulary) compactSeed {
	result := compactSeed{atoms: make([]uint64, wordCount(len(vocabulary.atomKeys))), members: make([]uint64, wordCount(len(vocabulary.memberKeys)))}
	for key := range current.atoms {
		setCompactBit(result.atoms, vocabulary.atomIndex[key])
	}
	for key := range current.members {
		setCompactBit(result.members, vocabulary.memberIndex[key])
	}
	return result
}

func compactSeedKey(seed compactSeed, digest hash.Hash) compactKey {
	digest.Reset()
	var encoded [8]byte
	binary.BigEndian.PutUint32(encoded[:4], uint32(len(seed.atoms)))
	_, _ = digest.Write(encoded[:4])
	for _, word := range seed.atoms {
		binary.BigEndian.PutUint64(encoded[:], word)
		_, _ = digest.Write(encoded[:])
	}
	binary.BigEndian.PutUint32(encoded[:4], uint32(len(seed.members)))
	_, _ = digest.Write(encoded[:4])
	for _, word := range seed.members {
		binary.BigEndian.PutUint64(encoded[:], word)
		_, _ = digest.Write(encoded[:])
	}
	var result compactKey
	digest.Sum(result[:0])
	return result
}

func wordCount(size int) int                  { return (size + 63) / 64 }
func setCompactBit(words []uint64, index int) { words[index/64] |= uint64(1) << uint(index%64) }
func unionCompact(target, source []uint64) {
	for index := range target {
		target[index] |= source[index]
	}
}
