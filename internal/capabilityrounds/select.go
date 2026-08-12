package capabilityrounds

import (
	"cmp"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/bits"
	"slices"
	"sort"
)

type normalizedCase struct {
	value      Case
	atoms      map[string]Atom
	members    map[string]Member
	memberKeys []string
}

type candidateSeed struct {
	atoms   map[string]Atom
	members map[string]Member
}

type compactSeed struct {
	atoms   []uint64
	members []uint64
}

type seedVocabulary struct {
	atomKeys    []string
	memberKeys  []string
	atomIndex   map[string]int
	memberIndex map[string]int
	atoms       map[string]Atom
	members     map[string]Member
}

type compactSeedSet struct {
	buckets map[uint64][]compactSeed
	entries []compactSeed
}

func Select(cases []Case, state RoundState, policy Policy) (Selection, error) {
	if err := ValidatePolicy(policy); err != nil {
		return Selection{}, err
	}
	if state.Generation < 0 || state.Generation >= policy.MaximumRounds ||
		state.UsedCapabilityAtoms < 0 || state.UsedExactMembers < 0 ||
		state.UsedCapabilityAtoms > policy.MaximumTotalCapabilityAtoms ||
		state.UsedExactMembers > policy.MaximumTotalExactMembers ||
		state.UsedCapabilityAtoms != len(state.PriorAtomKeys) ||
		state.UsedExactMembers < state.UsedCapabilityAtoms ||
		state.UsedCapabilityAtoms > state.Generation*policy.MaximumRoundCapabilityAtoms ||
		state.UsedExactMembers > state.Generation*policy.MaximumRoundExactMembers {
		return Selection{}, fmt.Errorf("%w: invalid round state", ErrInvalidInput)
	}
	priorKeys := stringSet(state.PriorAtomKeys)
	if len(priorKeys) != len(state.PriorAtomKeys) || !slices.IsSorted(state.PriorAtomKeys) ||
		(state.Generation == 0 && len(priorKeys) != 0) || (state.Generation > 0 && len(priorKeys) == 0) {
		return Selection{}, fmt.Errorf("%w: invalid prior atom set", ErrInvalidInput)
	}
	normalized, atomSupport, err := normalizeCases(cases, state, policy)
	if err != nil {
		return Selection{}, err
	}
	vocabulary := buildSeedVocabulary(normalized)
	seeds, err := buildClosure(normalized, vocabulary, policy.MaximumCandidateBundles)
	if err != nil {
		return Selection{}, err
	}
	prior := priorKeys
	eligible := make([]Candidate, 0, len(seeds))
	remainingAtoms := min(policy.MaximumRoundCapabilityAtoms, policy.MaximumTotalCapabilityAtoms-state.UsedCapabilityAtoms)
	remainingMembers := min(policy.MaximumRoundExactMembers, policy.MaximumTotalExactMembers-state.UsedExactMembers)
	for _, compact := range seeds {
		atomCount, memberCount := compactCardinality(compact)
		if atomCount > remainingAtoms || memberCount > remainingMembers {
			continue
		}
		seed := expandSeed(compact, vocabulary)
		candidate, err := buildCandidate(seed, normalized, atomSupport, policy)
		if err != nil {
			return Selection{}, err
		}
		if candidateEligible(candidate, prior, state, policy) {
			eligible = append(eligible, candidate)
		}
	}
	if len(eligible) == 0 {
		return Selection{}, ErrNoEligibleBundle
	}
	sort.Slice(eligible, func(left, right int) bool {
		if comparison := compareSemantics(eligible[left], eligible[right]); comparison != 0 {
			return comparison < 0
		}
		return eligible[left].Key < eligible[right].Key
	})
	coRankOne := []Candidate{eligible[0]}
	for index := 1; index < len(eligible) && compareSemantics(eligible[0], eligible[index]) == 0; index++ {
		coRankOne = append(coRankOne, eligible[index])
	}
	return Selection{
		Generation:         state.Generation,
		CandidateCount:     len(seeds),
		EligibleCandidates: eligible,
		CoRankOne:          coRankOne,
		Selected:           coRankOne[0],
	}, nil
}

func AtomKey(scope, capability string) (string, error) {
	if scope == "" || capability == "" {
		return "", fmt.Errorf("%w: empty atom field", ErrInvalidInput)
	}
	return tuple(scope, capability)
}

func MemberKey(stage, scope, capability, code string) (string, error) {
	if stage == "" || scope == "" || capability == "" || code == "" {
		return "", fmt.Errorf("%w: empty member field", ErrInvalidInput)
	}
	return tuple(stage, scope, capability, code)
}

func normalizeCases(cases []Case, state RoundState, policy Policy) ([]normalizedCase, map[string]int, error) {
	if len(cases) != policy.ExpectedDiscoveryCaseCount {
		return nil, nil, fmt.Errorf("%w: discovery case count %d, want %d", ErrInvalidInput, len(cases), policy.ExpectedDiscoveryCaseCount)
	}
	prior := stringSet(state.PriorAtomKeys)
	seenCases := map[string]bool{}
	active := make([]normalizedCase, 0, len(cases))
	atomCases := map[string]map[string]bool{}
	for _, input := range cases {
		if input.ID == "" || input.Role != "discovery" || input.ReportingDomain == "" || input.SafetyImpact == "" || input.Outcome == "" || seenCases[input.ID] {
			return nil, nil, fmt.Errorf("%w: invalid or duplicate case %q", ErrInvalidInput, input.ID)
		}
		seenCases[input.ID] = true
		if _, exists := policy.SafetyWeights[input.SafetyImpact]; !exists {
			return nil, nil, fmt.Errorf("%w: unknown safety impact %q", ErrInvalidInput, input.SafetyImpact)
		}
		eligible := slices.Contains(policy.EligibleOutcomes, input.Outcome)
		switch input.Outcome {
		case "pass":
			if len(input.Frontier) != 0 {
				return nil, nil, fmt.Errorf("%w: passing case %q has frontier", ErrInvalidInput, input.ID)
			}
			continue
		case "unsafe", "unsupported", "exhausted":
		default:
			return nil, nil, fmt.Errorf("%w: unknown outcome %q", ErrInvalidInput, input.Outcome)
		}
		if len(input.Frontier) == 0 {
			return nil, nil, fmt.Errorf("%w: active case %q has empty frontier", ErrInvalidInput, input.ID)
		}
		value := input
		value.Frontier = slices.Clone(input.Frontier)
		normalized := normalizedCase{value: value, atoms: map[string]Atom{}, members: map[string]Member{}}
		for _, gap := range value.Frontier {
			if gap.CausalToken == "" || len(gap.RequiredEvidence) == 0 || !slices.IsSorted(gap.RequiredEvidence) {
				return nil, nil, fmt.Errorf("%w: noncanonical gap evidence in case %q", ErrInvalidInput, input.ID)
			}
			for index, evidence := range gap.RequiredEvidence {
				if evidence == "" || (index > 0 && evidence == gap.RequiredEvidence[index-1]) {
					return nil, nil, fmt.Errorf("%w: empty or duplicate gap evidence in case %q", ErrInvalidInput, input.ID)
				}
			}
			stage := gap.Stage
			if alias, exists := policy.StageAliases[stage]; exists {
				stage = alias
			}
			if _, exists := policy.StageOrdinal[stage]; !exists {
				return nil, nil, fmt.Errorf("%w: unknown stage %q", ErrInvalidInput, gap.Stage)
			}
			atomKey, err := AtomKey(gap.Scope, gap.Capability)
			if err != nil {
				return nil, nil, err
			}
			if eligible && prior[atomKey] {
				return nil, nil, fmt.Errorf("%w: prior atom persists in case %q", ErrInvalidInput, input.ID)
			}
			memberKey, err := MemberKey(stage, gap.Scope, gap.Capability, gap.Code)
			if err != nil {
				return nil, nil, err
			}
			if _, duplicate := normalized.members[memberKey]; duplicate {
				return nil, nil, fmt.Errorf("%w: duplicate member in case %q", ErrInvalidInput, input.ID)
			}
			normalized.atoms[atomKey] = Atom{Key: atomKey, Scope: gap.Scope, Capability: gap.Capability}
			normalized.members[memberKey] = Member{Key: memberKey, Stage: stage, Scope: gap.Scope, Capability: gap.Capability, Code: gap.Code}
			normalized.memberKeys = append(normalized.memberKeys, memberKey)
			if !eligible {
				continue
			}
			if atomCases[atomKey] == nil {
				atomCases[atomKey] = map[string]bool{}
			}
			atomCases[atomKey][input.ID] = true
		}
		if !eligible {
			continue
		}
		slices.Sort(normalized.memberKeys)
		active = append(active, normalized)
	}
	if len(active) == 0 {
		return nil, nil, ErrNoEligibleBundle
	}
	sort.Slice(active, func(left, right int) bool { return active[left].value.ID < active[right].value.ID })
	atomSupport := make(map[string]int, len(atomCases))
	for key, caseIDs := range atomCases {
		atomSupport[key] = len(caseIDs)
	}
	return active, atomSupport, nil
}

func buildClosure(cases []normalizedCase, vocabulary seedVocabulary, maximum int) ([]compactSeed, error) {
	seeds := compactSeedSet{buckets: map[uint64][]compactSeed{}}
	for _, currentCase := range cases {
		frontier := compactSeed{
			atoms:   make([]uint64, wordCount(len(vocabulary.atomKeys))),
			members: make([]uint64, wordCount(len(vocabulary.memberKeys))),
		}
		for key := range currentCase.atoms {
			setBit(frontier.atoms, vocabulary.atomIndex[key])
		}
		for key := range currentCase.members {
			setBit(frontier.members, vocabulary.memberIndex[key])
		}
		snapshotLength := len(seeds.entries)
		seeds.add(frontier)
		for index := 0; index < snapshotLength; index++ {
			seeds.add(unionCompactSeed(seeds.entries[index], frontier))
			if len(seeds.entries) > maximum {
				return nil, ErrCandidateOverflow
			}
		}
	}
	return seeds.values(), nil
}

func (seeds *compactSeedSet) add(seed compactSeed) {
	hash := compactSeedHash(seed)
	for _, existing := range seeds.buckets[hash] {
		if slices.Equal(existing.atoms, seed.atoms) && slices.Equal(existing.members, seed.members) {
			return
		}
	}
	seeds.buckets[hash] = append(seeds.buckets[hash], seed)
	seeds.entries = append(seeds.entries, seed)
}

func (seeds compactSeedSet) values() []compactSeed {
	return slices.Clone(seeds.entries)
}

func unionCompactSeed(left, right compactSeed) compactSeed {
	result := compactSeed{atoms: make([]uint64, len(left.atoms)), members: make([]uint64, len(left.members))}
	for index := range left.atoms {
		result.atoms[index] = left.atoms[index] | right.atoms[index]
	}
	for index := range left.members {
		result.members[index] = left.members[index] | right.members[index]
	}
	return result
}

func buildSeedVocabulary(cases []normalizedCase) seedVocabulary {
	vocabulary := seedVocabulary{atomIndex: map[string]int{}, memberIndex: map[string]int{}, atoms: map[string]Atom{}, members: map[string]Member{}}
	for _, currentCase := range cases {
		for key, atom := range currentCase.atoms {
			vocabulary.atoms[key] = atom
		}
		for key, member := range currentCase.members {
			vocabulary.members[key] = member
		}
	}
	vocabulary.atomKeys = sortedMapKeys(vocabulary.atoms)
	vocabulary.memberKeys = sortedMapKeys(vocabulary.members)
	for index, key := range vocabulary.atomKeys {
		vocabulary.atomIndex[key] = index
	}
	for index, key := range vocabulary.memberKeys {
		vocabulary.memberIndex[key] = index
	}
	return vocabulary
}

func expandSeed(compact compactSeed, vocabulary seedVocabulary) candidateSeed {
	seed := candidateSeed{atoms: map[string]Atom{}, members: map[string]Member{}}
	for index, key := range vocabulary.atomKeys {
		if bitSet(compact.atoms, index) {
			seed.atoms[key] = vocabulary.atoms[key]
		}
	}
	for index, key := range vocabulary.memberKeys {
		if bitSet(compact.members, index) {
			seed.members[key] = vocabulary.members[key]
		}
	}
	return seed
}

func compactSeedHash(seed compactSeed) uint64 {
	const (
		offset = uint64(14695981039346656037)
		prime  = uint64(1099511628211)
	)
	hash := offset
	mix := func(word uint64) {
		for shift := 0; shift < 64; shift += 8 {
			hash ^= uint64(byte(word >> uint(shift)))
			hash *= prime
		}
	}
	mix(uint64(len(seed.atoms)))
	for _, word := range seed.atoms {
		mix(word)
	}
	mix(uint64(len(seed.members)))
	for _, word := range seed.members {
		mix(word)
	}
	return hash
}

func compactCardinality(seed compactSeed) (int, int) {
	atoms, members := 0, 0
	for _, word := range seed.atoms {
		atoms += bits.OnesCount64(word)
	}
	for _, word := range seed.members {
		members += bits.OnesCount64(word)
	}
	return atoms, members
}

func wordCount(values int) int {
	return (values + 63) / 64
}

func setBit(words []uint64, index int) {
	words[index/64] |= uint64(1) << uint(index%64)
}

func bitSet(words []uint64, index int) bool {
	return words[index/64]&(uint64(1)<<uint(index%64)) != 0
}

func buildCandidate(seed candidateSeed, cases []normalizedCase, atomSupport map[string]int, policy Policy) (Candidate, error) {
	atomKeys := sortedMapKeys(seed.atoms)
	memberKeys := sortedMapKeys(seed.members)
	key, err := candidateKey(seed)
	if err != nil {
		return Candidate{}, err
	}
	candidate := Candidate{Key: key}
	for _, key := range atomKeys {
		atom := seed.atoms[key]
		atom.CaseCount = atomSupport[key]
		candidate.Atoms = append(candidate.Atoms, atom)
	}
	for _, key := range memberKeys {
		candidate.Members = append(candidate.Members, seed.members[key])
	}
	domains := map[string]bool{}
	for _, currentCase := range cases {
		if keysSubset(currentCase.memberKeys, seed.members) {
			candidate.CoveredCaseIDs = append(candidate.CoveredCaseIDs, currentCase.value.ID)
			domains[currentCase.value.ReportingDomain] = true
			candidate.SafetyWeight += policy.SafetyWeights[currentCase.value.SafetyImpact]
		}
	}
	candidate.ReportingDomains = sortedSetKeys(domains)
	return candidate, nil
}

func candidateEligible(candidate Candidate, prior map[string]bool, state RoundState, policy Policy) bool {
	if len(candidate.Atoms) > policy.MaximumRoundCapabilityAtoms || len(candidate.Members) > policy.MaximumRoundExactMembers ||
		state.UsedCapabilityAtoms+len(candidate.Atoms) > policy.MaximumTotalCapabilityAtoms ||
		state.UsedExactMembers+len(candidate.Members) > policy.MaximumTotalExactMembers ||
		len(candidate.CoveredCaseIDs) < policy.MinimumAdvancedActiveCases ||
		len(candidate.ReportingDomains) < policy.MinimumReportingDomains {
		return false
	}
	for _, atom := range candidate.Atoms {
		if prior[atom.Key] || atom.CaseCount < policy.MinimumAtomActiveCaseSupport {
			return false
		}
	}
	return true
}

func compareSemantics(left, right Candidate) int {
	if comparison := cmp.Compare(len(right.CoveredCaseIDs), len(left.CoveredCaseIDs)); comparison != 0 {
		return comparison
	}
	if comparison := cmp.Compare(len(right.ReportingDomains), len(left.ReportingDomains)); comparison != 0 {
		return comparison
	}
	if comparison := cmp.Compare(right.SafetyWeight, left.SafetyWeight); comparison != 0 {
		return comparison
	}
	if comparison := cmp.Compare(len(left.Atoms), len(right.Atoms)); comparison != 0 {
		return comparison
	}
	return cmp.Compare(len(left.Members), len(right.Members))
}

func candidateKey(seed candidateSeed) (string, error) {
	atomKeys := sortedMapKeys(seed.atoms)
	memberKeys := sortedMapKeys(seed.members)
	return tuple(append(append([]string{"atoms"}, atomKeys...), append([]string{"members"}, memberKeys...)...)...)
}

func tuple(fields ...string) (string, error) {
	length := 0
	for _, field := range fields {
		if uint64(len(field)) > math.MaxUint32 {
			return "", fmt.Errorf("%w: tuple field exceeds u32", ErrInvalidInput)
		}
		length += 4 + len(field)
	}
	encoded := make([]byte, 0, length)
	var prefix [4]byte
	for _, field := range fields {
		binary.BigEndian.PutUint32(prefix[:], uint32(len(field)))
		encoded = append(encoded, prefix[:]...)
		encoded = append(encoded, field...)
	}
	return hex.EncodeToString(encoded), nil
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func sortedSetKeys(values map[string]bool) []string {
	return sortedMapKeys(values)
}

func keysSubset(keys []string, set map[string]Member) bool {
	for _, key := range keys {
		if _, exists := set[key]; !exists {
			return false
		}
	}
	return true
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			result[value] = true
		}
	}
	return result
}
