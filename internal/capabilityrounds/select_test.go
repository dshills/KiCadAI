package capabilityrounds

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestSelectPublishesSemanticTiesAndUsesCanonicalFallback(t *testing.T) {
	policy := testPolicy(t)
	policy.MaximumRoundCapabilityAtoms = 1
	cases := []Case{
		testCase("a", "analog", "review_required", testGap("simulation", "simulation", "alpha", "A")),
		testCase("b", "power", "review_required", testGap("simulation", "simulation", "alpha", "A")),
		testCase("c", "digital", "review_required", testGap("simulation", "simulation", "beta", "B")),
		testCase("d", "sensor", "review_required", testGap("simulation", "simulation", "beta", "B")),
	}
	cases = append(cases, inactiveCases(policy.ExpectedDiscoveryCaseCount-len(cases))...)
	selection, err := Select(cases, RoundState{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.CoRankOne) != 2 {
		t.Fatalf("co-rank one count = %d, want 2", len(selection.CoRankOne))
	}
	if selection.Selected.Key != selection.CoRankOne[0].Key || selection.CoRankOne[0].Key >= selection.CoRankOne[1].Key {
		t.Fatal("semantic tie did not use canonical key fallback")
	}

	slices.Reverse(cases)
	replayed, err := Select(cases, RoundState{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Selected.Key != selection.Selected.Key || !slices.Equal(candidateKeys(replayed.CoRankOne), candidateKeys(selection.CoRankOne)) {
		t.Fatal("selection depends on input order")
	}
}

func TestSelectBuildsCompleteDeduplicatedClosure(t *testing.T) {
	policy := testPolicy(t)
	cases := []Case{
		testCase("a", "analog", "non_safety", testGap("simulation", "simulation", "alpha", "A")),
		testCase("b", "power", "non_safety", testGap("simulation", "simulation", "alpha", "A")),
		testCase("c", "digital", "non_safety", testGap("simulation", "simulation", "beta", "B")),
		testCase("d", "sensor", "non_safety", testGap("simulation", "simulation", "beta", "B")),
		testCase("e", "analog", "non_safety", testGap("simulation", "simulation", "gamma", "C")),
		testCase("f", "mixed_signal", "non_safety", testGap("simulation", "simulation", "gamma", "C")),
	}
	cases = append(cases, inactiveCases(policy.ExpectedDiscoveryCaseCount-len(cases))...)
	selection, err := Select(cases, RoundState{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if selection.CandidateCount != 7 {
		t.Fatalf("candidate closure count = %d, want 7", selection.CandidateCount)
	}
}

func TestSelectRejectsPersistingPriorAtom(t *testing.T) {
	policy := testPolicy(t)
	gap := testGap("simulation", "simulation", "alpha", "A")
	atomKey, err := AtomKey(gap.Scope, gap.Capability)
	if err != nil {
		t.Fatal(err)
	}
	cases := []Case{
		testCase("a", "analog", "non_safety", gap),
		testCase("b", "power", "non_safety", gap),
	}
	cases = append(cases, inactiveCases(policy.ExpectedDiscoveryCaseCount-len(cases))...)
	_, err = Select(cases, RoundState{Generation: 1, PriorAtomKeys: []string{atomKey}, UsedCapabilityAtoms: 1, UsedExactMembers: 1}, policy)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("prior atom error = %v", err)
	}
}

func TestSelectFailsClosedOnCandidateOverflow(t *testing.T) {
	policy := testPolicy(t)
	policy.MaximumCandidateBundles = 2
	cases := []Case{
		testCase("a", "analog", "non_safety", testGap("simulation", "simulation", "alpha", "A")),
		testCase("b", "power", "non_safety", testGap("simulation", "simulation", "alpha", "A")),
		testCase("c", "digital", "non_safety", testGap("simulation", "simulation", "beta", "B")),
		testCase("d", "sensor", "non_safety", testGap("simulation", "simulation", "beta", "B")),
	}
	cases = append(cases, inactiveCases(policy.ExpectedDiscoveryCaseCount-len(cases))...)
	_, err := Select(cases, RoundState{}, policy)
	if !errors.Is(err, ErrCandidateOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestIdentityEncodingIsUnambiguousAndStageAliasesCanonicalize(t *testing.T) {
	left, err := tuple("a", "bc")
	if err != nil {
		t.Fatal(err)
	}
	right, err := tuple("ab", "c")
	if err != nil {
		t.Fatal(err)
	}
	if left == right {
		t.Fatal("length-prefixed tuple encoding collides")
	}
	policy := testPolicy(t)
	aliasCases := []Case{
		testCase("a", "analog", "non_safety", testGap("roundtrip", "verification", "fidelity", "DIFF")),
		testCase("b", "power", "non_safety", testGap("round_trip", "verification", "fidelity", "DIFF")),
	}
	aliasCases = append(aliasCases, inactiveCases(policy.ExpectedDiscoveryCaseCount-len(aliasCases))...)
	selection, err := Select(aliasCases, RoundState{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Selected.Members) != 1 || selection.Selected.Members[0].Stage != "round_trip" {
		t.Fatal("stage alias was not canonicalized before identity construction")
	}
}

func TestSelectValidatesAllOutcomeClassesAndIgnoresUnsafeForRanking(t *testing.T) {
	policy := testPolicy(t)
	shared := testGap("simulation", "simulation", "alpha", "A")
	cases := []Case{
		testCase("a", "analog", "non_safety", shared),
		testCase("b", "power", "non_safety", shared),
		{ID: "unsafe", Role: "discovery", ReportingDomain: "digital", SafetyImpact: "safety_critical", Outcome: "unsafe", Frontier: []Gap{testGap("drc", "physical", "clearance", "DRC")}},
	}
	cases = append(cases, inactiveCases(policy.ExpectedDiscoveryCaseCount-len(cases))...)
	selection, err := Select(cases, RoundState{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(selection.Selected.CoveredCaseIDs, []string{"a", "b"}) {
		t.Fatalf("unsafe case influenced ranking: %q", selection.Selected.CoveredCaseIDs)
	}

	cases[2].Outcome = "unknown"
	if _, err := Select(cases, RoundState{}, policy); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unknown outcome error = %v", err)
	}
}

func TestSelectRejectsDuplicatePriorAtoms(t *testing.T) {
	policy := testPolicy(t)
	key, err := AtomKey("simulation", "prior")
	if err != nil {
		t.Fatal(err)
	}
	cases := inactiveCases(policy.ExpectedDiscoveryCaseCount)
	_, err = Select(cases, RoundState{Generation: 1, UsedCapabilityAtoms: 2, UsedExactMembers: 2, PriorAtomKeys: []string{key, key}}, policy)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate prior atom error = %v", err)
	}
}

func TestCompleteEighteenCaseClosureFitsFrozenCeiling(t *testing.T) {
	if testing.Short() {
		t.Skip("complete 18-case closure is a bounded non-short contract test")
	}
	policy := testPolicy(t)
	cases := make([]Case, 0, policy.ExpectedDiscoveryCaseCount)
	for index := 0; index < policy.ExpectedDiscoveryCaseCount; index++ {
		id := string(rune('a' + index))
		cases = append(cases, testCase(id, "analog", "non_safety", testGap("simulation", "simulation", "capability_"+id, "CODE_"+id)))
	}
	normalized, _, err := normalizeCases(cases, RoundState{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	vocabulary := buildSeedVocabulary(normalized)
	closure, err := buildClosure(normalized, vocabulary, policy.MaximumCandidateBundles)
	if err != nil {
		t.Fatal(err)
	}
	if len(closure) != (1<<policy.ExpectedDiscoveryCaseCount)-1 {
		t.Fatalf("complete closure count = %d, want %d", len(closure), (1<<policy.ExpectedDiscoveryCaseCount)-1)
	}
}

func testPolicy(t *testing.T) Policy {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test path")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "specs", "closed-loop-open-set-capability-expansion", "V7_SELECTION_POLICY.json")
	reader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	policy, err := DecodePolicy(reader)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func testCase(id, domain, safety string, gaps ...Gap) Case {
	return Case{ID: id, Role: "discovery", ReportingDomain: domain, SafetyImpact: safety, Outcome: "unsupported", Frontier: gaps}
}

func testGap(stage, scope, capability, code string) Gap {
	return Gap{Stage: stage, Scope: scope, Capability: capability, Code: code, CausalToken: capability + "_" + code, RequiredEvidence: []string{"evidence"}}
}

func inactiveCases(count int) []Case {
	result := make([]Case, 0, count)
	for index := 0; index < count; index++ {
		result = append(result, Case{
			ID:              "pass_" + string(rune('a'+index)),
			Role:            "discovery",
			ReportingDomain: "mixed_signal",
			SafetyImpact:    "non_safety",
			Outcome:         "pass",
		})
	}
	return result
}

func candidateKeys(candidates []Candidate) []string {
	keys := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		keys = append(keys, candidate.Key)
	}
	return keys
}
