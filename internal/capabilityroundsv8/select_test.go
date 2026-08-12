package capabilityroundsv8

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSelectCountsClosureAndRanksCaseDomainRoleBreadth(t *testing.T) {
	policy := FrozenPolicy()
	leafA := testLeaf("topology", "shared_a", "cap_a", "A")
	leafB := testLeaf("component", "shared_b", "cap_b", "B")
	closure := testLeaf("model", "shared_model", "cap_model", "M")
	cases := testCases(leafA, leafB)
	plans := []EffectPlan{testPlan(t, []Leaf{leafA}, nil), testPlan(t, []Leaf{leafB}, nil), testPlan(t, []Leaf{leafA, leafB}, []Leaf{closure})}
	selection, err := Select(cases, plans, initialState(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if selection.CandidateCount != 3 || len(selection.Selected.CoveredCaseIDs) != 4 || len(selection.Selected.ReportingDomains) != 4 || len(selection.Selected.CircuitRoles) != 4 || len(selection.Selected.Atoms) != 3 || len(selection.Selected.Members) != 3 {
		t.Fatalf("unexpected selection: %+v", selection)
	}
	if selection.Selected.EffectPlanSHA256 != strings.Repeat("c", 64) || len(selection.CoRankOne) != 1 {
		t.Fatalf("unexpected selected plan: %+v", selection.Selected)
	}
}

func TestSelectPublishesCompleteSemanticTieAndCanonicalFallback(t *testing.T) {
	leafA := testLeaf("topology", "shared_a", "cap_a", "A")
	leafB := testLeaf("component", "shared_b", "cap_b", "B")
	plans := []EffectPlan{testPlan(t, []Leaf{leafA}, nil), testPlan(t, []Leaf{leafB}, nil)}
	selection, err := Select(testCases(leafA, leafB), plans, initialState(), FrozenPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.CoRankOne) != 2 || selection.Selected.Key != selection.CoRankOne[0].Key || selection.CoRankOne[0].Key >= selection.CoRankOne[1].Key {
		t.Fatalf("semantic tie not published canonically: %+v", selection.CoRankOne)
	}
	reversed := testCases(leafA, leafB)
	slices.Reverse(reversed)
	again, err := Select(reversed, slices.Clone(plans), initialState(), FrozenPolicy())
	if err != nil || again.Selected.Key != selection.Selected.Key || !slices.Equal(candidateKeys(again.CoRankOne), candidateKeys(selection.CoRankOne)) {
		t.Fatalf("selection depends on input order: %+v, %v", again, err)
	}
}

func TestSelectRejectsUnprovenClosureBudgetsPriorAtomsAndMalformedPaths(t *testing.T) {
	leafA := testLeaf("topology", "shared_a", "cap_a", "A")
	leafB := testLeaf("component", "shared_b", "cap_b", "B")
	closureA := testLeaf("model", "closure_a", "closure_a", "CA")
	closureB := testLeaf("simulation", "closure_b", "closure_b", "CB")
	plans := []EffectPlan{testPlan(t, []Leaf{leafA, leafB}, []Leaf{closureA, closureB})}
	if _, err := Select(testCases(leafA, leafB), plans, initialState(), FrozenPolicy()); !errors.Is(err, ErrNoEligibleBundle) {
		t.Fatalf("over-budget closure error = %v", err)
	}
	plan := testPlan(t, []Leaf{leafA}, nil)
	plan.MechanicallyProven = false
	if _, err := Select(testCases(leafA, leafB), []EffectPlan{plan}, initialState(), FrozenPolicy()); !errors.Is(err, ErrNoEligibleBundle) {
		t.Fatalf("unproven closure error = %v", err)
	}
	atom, _ := AtomKey(leafA.Category, leafA.Scope, leafA.Capability)
	state := initialState()
	state.Generation, state.UsedAtomCount, state.PriorAtomKeys = 1, 1, []string{atom}
	if _, err := Select(testCases(leafA, leafB), []EffectPlan{testPlan(t, []Leaf{leafA}, nil)}, state, FrozenPolicy()); !errors.Is(err, ErrNoEligibleBundle) {
		t.Fatalf("prior-atom reselection error = %v", err)
	}
	malformed := testCases(leafA, leafB)
	malformed[0].Frontier[0].Path[0].RequiredEvidence = []string{"z", "a"}
	if _, err := Select(malformed, nil, initialState(), FrozenPolicy()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("malformed evidence error = %v", err)
	}
	unknown := testPlan(t, []Leaf{leafA}, nil)
	unknown.DirectAtomKeys = []string{"topology:unknown:capability"}
	if _, err := Select(testCases(leafA, leafB), []EffectPlan{unknown}, initialState(), FrozenPolicy()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown direct atom error = %v", err)
	}
}

func TestCompleteEighteenCaseClosureFitsFrozenCeilingAndPolicyCannotRelax(t *testing.T) {
	domains := []string{"analog_signal_path", "power_energy_conversion", "digital_control", "mixed_signal_data_conversion", "sensing_instrumentation", "protection_power_integrity"}
	roles := []string{"source_bias", "amplification_conditioning", "conversion_regulation", "sensing_measurement", "interface_control", "protection_supervision"}
	cases := make([]Case, 0, 18)
	active := make([]string, 0, 18)
	for index := 1; index <= 18; index++ {
		id := "case_" + leftPad(index)
		leaf := testLeaf("topology", "scope_"+leftPad(index), "capability_"+leftPad(index), "CODE"+leftPad(index))
		cases = append(cases, testCase(id, domains[(index-1)%len(domains)], roles[(index-1)%len(roles)], "non_safety", leaf))
		active = append(active, id)
	}
	if _, err := Select(cases, nil, RoundState{Generation: 0, ActiveCohortIDs: active}, FrozenPolicy()); !errors.Is(err, ErrNoEligibleBundle) {
		t.Fatalf("complete closure error = %v", err)
	}
	relaxed := FrozenPolicy()
	relaxed.MaximumRoundAtoms++
	if _, err := Select(testCases(testLeaf("topology", "a", "a", "A"), testLeaf("component", "b", "b", "B")), nil, initialState(), relaxed); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("relaxed policy error = %v", err)
	}
}

func TestDominanceTransformMatchesExactQuadraticOracle(t *testing.T) {
	policy := FrozenPolicy()
	values := []Candidate{}
	caseIDs := []string{"case_001", "case_002", "case_003", "case_004"}
	for mask := 1; mask < 1<<len(caseIDs); mask++ {
		covered := []string{}
		for bit, caseID := range caseIDs {
			if mask&(1<<bit) != 0 {
				covered = append(covered, caseID)
			}
		}
		for atoms := 1; atoms <= 3; atoms++ {
			for members := atoms; members <= 3; members++ {
				values = append(values, Candidate{Key: strings.Repeat("k", mask+atoms+members), CoveredCaseIDs: covered, Atoms: make([]Atom, atoms), Members: make([]Member, members)})
			}
		}
	}
	got, err := pruneDominated(values, policy)
	if err != nil {
		t.Fatal(err)
	}
	want := []Candidate{}
	for index, candidate := range values {
		dominated := false
		for otherIndex, other := range values {
			if index == otherIndex || !subset(candidate.CoveredCaseIDs, stringSet(other.CoveredCaseIDs)) || len(other.Atoms) > len(candidate.Atoms) || len(other.Members) > len(candidate.Members) {
				continue
			}
			if len(other.CoveredCaseIDs) > len(candidate.CoveredCaseIDs) || len(other.Atoms) < len(candidate.Atoms) || len(other.Members) < len(candidate.Members) {
				dominated = true
				break
			}
		}
		if !dominated {
			want = append(want, candidate)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("dominance transform retained %d candidates, oracle retained %d", len(got), len(want))
	}
	for index := range got {
		if !slices.Equal(got[index].CoveredCaseIDs, want[index].CoveredCaseIDs) || len(got[index].Atoms) != len(want[index].Atoms) || len(got[index].Members) != len(want[index].Members) {
			t.Fatalf("dominance transform mismatch at %d: %+v != %+v", index, got[index], want[index])
		}
	}
}

func testCases(leafA, leafB Leaf) []Case {
	result := []Case{
		testCase("case_001", "analog_signal_path", "source_bias", "review_required", leafA),
		testCase("case_002", "power_energy_conversion", "conversion_regulation", "review_required", leafA),
		testCase("case_003", "digital_control", "interface_control", "review_required", leafB),
		testCase("case_004", "mixed_signal_data_conversion", "sensing_measurement", "review_required", leafB),
	}
	for index := 5; index <= 18; index++ {
		result = append(result, Case{ID: strings.ReplaceAll("case_000", "000", leftPad(index)), Role: "discovery", ReportingDomain: "sensing_instrumentation", CircuitRole: "amplification_conditioning", SafetyImpact: "non_safety", Outcome: "pass"})
	}
	return result
}

func testCase(id, domain, role, safety string, leaf Leaf) Case {
	anchor := identityDigest("anchor", id)
	return Case{ID: id, Role: "discovery", ReportingDomain: domain, CircuitRole: role, SafetyImpact: safety, Outcome: "unsupported",
		Frontier: []Gap{{ObligationAnchor: anchor, Path: []Leaf{leaf}, Diagnostics: []string{"diagnostic"}}}}
}

func testLeaf(stage, scope, capability, code string) Leaf {
	return Leaf{Stage: stage, Category: stage, Scope: scope, Capability: capability, Code: code, RequiredEvidence: []string{"evidence"}}
}

func testPlan(t *testing.T, direct, closure []Leaf) EffectPlan {
	t.Helper()
	directAtoms, directMembers := map[string]bool{}, map[string]bool{}
	closureAtoms, closureMembers := []Atom{}, []Member{}
	allMembers := map[string]bool{}
	for _, leaf := range direct {
		atom, _ := AtomKey(leaf.Category, leaf.Scope, leaf.Capability)
		member, _ := MemberKey(leaf)
		directAtoms[atom], directMembers[member], allMembers[member] = true, true, true
	}
	for _, leaf := range closure {
		atomKey, _ := AtomKey(leaf.Category, leaf.Scope, leaf.Capability)
		memberKey, _ := MemberKey(leaf)
		closureAtoms = append(closureAtoms, Atom{Key: atomKey, Category: leaf.Category, Scope: leaf.Scope, Capability: leaf.Capability})
		closureMembers = append(closureMembers, Member{Key: memberKey, Stage: leaf.Stage, Category: leaf.Category, Scope: leaf.Scope, Capability: leaf.Capability, Code: leaf.Code})
		allMembers[memberKey] = true
	}
	slices.SortFunc(closureAtoms, func(a, b Atom) int { return strings.Compare(a.Key, b.Key) })
	slices.SortFunc(closureMembers, func(a, b Member) int { return strings.Compare(a.Key, b.Key) })
	return EffectPlan{DirectAtomKeys: sortedSet(directAtoms), DirectMemberKeys: sortedSet(directMembers), ClosureAtoms: closureAtoms, ClosureMembers: closureMembers,
		PlannedMemberKeys: sortedSet(allMembers), RequiredEvidence: append([]string(nil), FrozenPolicy().MechanicalEvidence...), Executable: true, MechanicallyProven: true, PlanSHA256: strings.Repeat("c", 64)}
}

func candidateKeys(values []Candidate) []string {
	result := make([]string, len(values))
	for index := range values {
		result[index] = values[index].Key
	}
	return result
}

func initialState() RoundState {
	return RoundState{Generation: 0, ActiveCohortIDs: []string{"case_001", "case_002", "case_003", "case_004"}}
}

func leftPad(value int) string {
	if value < 10 {
		return "00" + string(rune('0'+value))
	}
	return "0" + string(rune('0'+value/10)) + string(rune('0'+value%10))
}
