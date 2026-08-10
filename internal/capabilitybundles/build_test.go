package capabilitybundles

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

func TestBuildRequiresCompleteBlockerCoverage(t *testing.T) {
	policy := testPolicy()
	cases := []Case{
		testCase("power_a", "power", 4, testGap("simulation", "thermal"), testGap("simulation", "dc")),
		testCase("analog_b", "analog", 3, testGap("simulation", "thermal"), testGap("simulation", "dc")),
		testCase("sensor_c", "sensor", 2, testGap("simulation", "thermal"), testGap("simulation", "noise")),
	}
	result, err := Build(cases, policy)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectRankOne(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Atoms) != 2 || len(selected.UnlockedCases) != 2 ||
		!slices.Equal(selected.UnlockedCases, []string{"analog_b", "power_a"}) {
		t.Fatalf("selected causal bundle = %#v", selected)
	}
	for _, candidate := range result.Candidates {
		if len(candidate.Atoms) == 1 && len(candidate.UnlockedCases) != 0 {
			t.Fatalf("partial blocker coverage received unlock credit: %#v", candidate)
		}
	}
	if len(result.CaseRejections) != 1 || result.CaseRejections[0].CaseID != "sensor_c" {
		t.Fatalf("atom reuse rejection = %#v", result.CaseRejections)
	}
}

func TestBuildRejectsRawIncidenceWithoutCausalUnlock(t *testing.T) {
	policy := testPolicy()
	cases := []Case{
		testCase("power_a", "power", 5,
			testGap("simulation", "electrothermal"), testGap("simulation", "dc"), testGap("simulation", "output")),
		testCase("power_b", "power", 4,
			testGap("simulation", "electrothermal"), testGap("simulation", "dc")),
		testCase("analog_c", "analog", 5,
			testGap("simulation", "electrothermal"), testGap("simulation", "bandwidth"),
			testGap("simulation", "noise"), testGap("simulation", "transimpedance")),
	}
	result, err := Build(cases, policy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SelectRankOne(result); !errors.Is(err, ErrNoEligibleBundle) {
		t.Fatalf("raw gap incidence selected a bundle: %v, %#v", err, result.Candidates)
	}
	for _, candidate := range result.Candidates {
		if len(candidate.Atoms) == 1 && candidate.Atoms[0].Capability == "electrothermal" &&
			len(candidate.UnlockedCases) != 0 {
			t.Fatal("shared electrothermal incidence was misclassified as causal unlock")
		}
	}
}

func TestBuildUsesAllExactMembersOfSelectedAtoms(t *testing.T) {
	policy := testPolicy()
	cases := []Case{
		testCase("power", "power", 3, Gap{Stage: "simulation", Scope: "simulation", Capability: "solver", Code: "CODE_A"}),
		testCase("analog", "analog", 2, Gap{Stage: "simulation", Scope: "simulation", Capability: "solver", Code: "CODE_B"}),
	}
	result, err := Build(cases, policy)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := SelectRankOne(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected.Atoms) != 1 || len(selected.Members) != 2 || len(selected.UnlockedCases) != 2 {
		t.Fatalf("exact member union = %#v", selected)
	}
}

func TestBuildIsDeterministicUnderInputPermutation(t *testing.T) {
	policy := testPolicy()
	cases := []Case{
		testCase("a", "power", 3, testGap("simulation", "one"), testGap("simulation", "two")),
		testCase("b", "analog", 2, testGap("simulation", "one"), testGap("simulation", "two")),
		testCase("c", "sensor", 1, testGap("simulation", "one"), testGap("simulation", "three")),
		testCase("d", "mixed_signal", 4, testGap("simulation", "one"), testGap("simulation", "three")),
	}
	first, err := Build(cases, policy)
	if err != nil {
		t.Fatal(err)
	}
	reversed := slices.Clone(cases)
	slices.Reverse(reversed)
	for index := range reversed {
		slices.Reverse(reversed[index].Gaps)
	}
	second, err := Build(reversed, policy)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := json.Marshal(first)
	secondBytes, _ := json.Marshal(second)
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("causal-unlock build depends on case or gap order")
	}
}

func TestSelectRankOneFailsSemanticTieBeforeIdentity(t *testing.T) {
	left := Candidate{Eligible: true, Key: "left", UnlockedCases: []string{"a", "b"}, ReportingDomains: []string{"analog", "power"}, SafetyWeight: 4, Atoms: make([]Atom, 2), Members: make([]Member, 2)}
	right := left
	right.Key = "right"
	result := Result{Candidates: []Candidate{left, right}}
	if _, err := SelectRankOne(result); !errors.Is(err, ErrAmbiguousRankOne) {
		t.Fatalf("semantic rank-one tie error = %v", err)
	}
}

func TestAdmitRankOneDoesNotFallBackToEasierPlan(t *testing.T) {
	rankOne := Candidate{Eligible: true, Key: "rank_one", Rank: 1, UnlockedCases: []string{"a", "b", "c"}, ReportingDomains: []string{"analog", "power"}, Atoms: []Atom{{Key: "atom"}}, Members: []Member{{Key: "member"}}}
	rankTwo := Candidate{Eligible: true, Key: "rank_two", Rank: 2, UnlockedCases: []string{"a", "b"}, ReportingDomains: []string{"analog", "power"}, Atoms: []Atom{{Key: "other"}}, Members: []Member{{Key: "other_member"}}}
	result := Result{Candidates: []Candidate{rankOne, rankTwo}}
	plans := map[string]PlanEvidence{
		rankTwo.Key: {Executable: true, AtomKeys: []string{"other"}, MemberKeys: []string{"other_member"}},
	}
	if _, err := AdmitRankOne(result, plans); !errors.Is(err, ErrRankOnePlan) {
		t.Fatalf("missing rank-one plan error = %v", err)
	}
	plans[rankOne.Key] = PlanEvidence{Executable: true, AtomKeys: []string{"atom"}, MemberKeys: []string{"member"}}
	if selected, err := AdmitRankOne(result, plans); err != nil || selected.Key != rankOne.Key {
		t.Fatalf("complete rank-one plan = %#v, %v", selected, err)
	}
	plans[rankOne.Key] = PlanEvidence{Executable: true, AtomKeys: []string{"atom", "atom"}, MemberKeys: []string{"member"}}
	if _, err := AdmitRankOne(result, plans); !errors.Is(err, ErrRankOnePlan) {
		t.Fatalf("duplicate plan coverage error = %v", err)
	}
}

func TestBuildRejectsHeldOutInfluenceAndCandidateOverflow(t *testing.T) {
	policy := testPolicy()
	heldOut := testCase("secret", "analog", 1, testGap("simulation", "solver"))
	heldOut.Role = "held_out"
	if _, err := Build([]Case{heldOut}, policy); err == nil {
		t.Fatal("causal-unlock build accepted held-out evidence")
	}

	policy.BundleGeneration.MaximumCandidateBundles = 1
	cases := []Case{
		testCase("a1", "analog", 1, testGap("simulation", "a")),
		testCase("a2", "power", 1, testGap("simulation", "a")),
		testCase("b1", "sensor", 1, testGap("simulation", "b")),
		testCase("b2", "mixed_signal", 1, testGap("simulation", "b")),
	}
	if _, err := Build(cases, policy); err == nil {
		t.Fatal("causal-unlock build silently truncated its candidate set")
	}
}

func TestTupleUsesUTF8ByteLengths(t *testing.T) {
	if got := tuple("é"); got != "2:é" {
		t.Fatalf("tuple UTF-8 encoding = %q, want %q", got, "2:é")
	}
	if tuple("ab", "c") == tuple("a", "bc") {
		t.Fatal("length-prefixed tuple accepted an ambiguous field boundary")
	}
}

func testPolicy() Policy {
	return Policy{
		Schema: "test.causal-unlock-policy", Version: 1,
		CandidateUnit:      CandidateUnitMinimalCausalUnlockBundle,
		AtomIdentityFields: slices.Clone(atomIdentityFields), MemberIdentityFields: slices.Clone(memberIdentityFields),
		IdentityEncoding: IdentityEncodingLengthPrefixed, CaseBlockerSource: CaseBlockerSourceNormalizedRootGaps,
		UnlockRule: UnlockRuleCompleteMemberCoverage, EligibleBaselineOutcomes: []string{"unsupported", "exhausted"},
		BundleGeneration: BundleGeneration{
			DiscoveryOnly: true, MaximumCapabilityAtoms: 4, MaximumExactMembers: 12,
			MaximumCandidateBundles: 4096, MinimumAtomCaseSupport: 2, RequireMinimalUnlockSet: true,
			RejectEmptyCaseBlockers: true, RejectDuplicateAtoms: true, RejectDuplicateMembers: true, RejectDuplicateCases: true,
		},
		Eligibility: Eligibility{MinimumUnlockedDiscoveryCases: 2, MinimumReportingDomains: 2},
		Ranking:     slices.Clone(semanticRanking), PublicationOrder: PublicationOrderSemanticThenKey,
		TieBehavior: TieBehaviorFailBeforeIdentity,
		RankOnePlanAdmission: PlanAdmission{
			RequireCompleteGenericPlan: true, RequireAllAtomsPlanned: true, RequireAllMembersPlanned: true,
			UnexecutableRankOne: UnexecutableRankOneNoFallback,
		},
		HeldOutInfluence: HeldOutInfluenceProhibited, UnsafeCaseUnlockCredit: UnsafeUnlockCreditProhibited,
		SelectedMemberGapRemoval: SelectedMemberRemovalExact,
		PublicAdmission: PublicAdmission{
			RequireTotalPassUplift: true, RequireClaimedUnlockPassUplift: true, MinimumRealizedClaimedUnlocks: 1,
			RequireNonselectedGapPreservation: true, RequireNoPassRegression: true, RequireNoUnsafeToPass: true,
		},
		HeldOutSuccessCausality: HeldOutCausalityCompleteCoverage,
	}
}

func testCase(id, domain string, safety int64, gaps ...Gap) Case {
	return Case{Role: "discovery", ID: id, ReportingDomain: domain, SafetyWeight: safety, Outcome: "unsupported", Gaps: gaps}
}

func testGap(scope, capability string) Gap {
	return Gap{Stage: "simulation", Scope: scope, Capability: capability, Code: "SIMULATION_INVALID", RequiredEvidence: []string{"simulation"}}
}
