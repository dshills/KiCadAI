package capabilityrounds

import (
	"errors"
	"slices"
	"testing"
)

func TestEvaluateRoundAdmitsNextFrontierAndPublicUplift(t *testing.T) {
	policy := testPolicy(t)
	previous, selected := roundFixture(t, policy)
	next := cloneCases(previous)
	replaceCase(next, "a", "unsupported", testGap("value_search", "simulation", "downstream", "VALUE"))
	replaceCase(next, "b", "unsupported", testGap("value_search", "simulation", "downstream", "VALUE"))
	evaluation, err := EvaluateRound(previous, next, selected, RoundState{}, nil, completeRoundEvidence(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Status != EvaluationContinue || !slices.Equal(evaluation.AdvancedCaseIDs, []string{"a", "b"}) || evaluation.NextState.Generation != 1 {
		t.Fatalf("continuing evaluation = %#v", evaluation)
	}

	passing := cloneCases(previous)
	replaceCase(passing, "a", "pass")
	replaceCase(passing, "b", "pass")
	evaluation, err = EvaluateRound(previous, passing, selected, RoundState{}, nil, completeRoundEvidence(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Status != EvaluationPublicAdmitted || evaluation.DiscoveryPassAfter-evaluation.DiscoveryPassBefore != 2 || evaluation.NewActiveCohortPasses != 2 {
		t.Fatalf("public evaluation = %#v", evaluation)
	}
}

func TestEvaluateRoundRequiresLineageForDisappearingNonselectedGap(t *testing.T) {
	policy := testPolicy(t)
	previous, selected := roundFixture(t, policy)
	shared := previousGap(previous, "a")
	extra := testGap("value_search", "simulation", "sizing", "RANGE")
	replaceCase(previous, "pass_c", "unsupported", shared, extra)
	next := cloneCases(previous)
	successor := extra
	successor.Stage = "simulation"
	successor.Code = "MODEL"
	successor.RequiredEvidence = []string{"evidence", "simulation"}
	replaceCase(next, "a", "unsupported", testGap("value_search", "simulation", "downstream", "VALUE"))
	replaceCase(next, "b", "unsupported", testGap("value_search", "simulation", "downstream", "VALUE"))
	replaceCase(next, "pass_c", "unsupported", successor)
	if _, err := EvaluateRound(previous, next, selected, RoundState{}, nil, completeRoundEvidence(), policy); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("missing lineage error = %v", err)
	}
	edge := LineageEdge{CaseID: "pass_c", From: extra, To: successor}
	evaluation, err := EvaluateRound(previous, next, selected, RoundState{}, []LineageEdge{edge}, completeRoundEvidence(), policy)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Status != EvaluationContinue {
		t.Fatalf("lineage evaluation status = %q", evaluation.Status)
	}

	regressing := edge
	regressing.To.Stage = "requirement"
	replaceCase(next, "pass_c", "unsupported", regressing.To)
	if _, err := EvaluateRound(previous, next, selected, RoundState{}, []LineageEdge{regressing}, completeRoundEvidence(), policy); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("regressing lineage error = %v", err)
	}
}

func TestEvaluateRoundRejectsRegressionUnsafePassAndIncompleteEvidence(t *testing.T) {
	policy := testPolicy(t)
	previous, selected := roundFixture(t, policy)
	next := cloneCases(previous)
	replaceCase(next, "a", "unsupported", testGap("value_search", "simulation", "downstream", "VALUE"))
	replaceCase(next, "b", "unsupported", testGap("value_search", "simulation", "downstream", "VALUE"))
	brokenEvidence := completeRoundEvidence()
	brokenEvidence.PhysicalPromotionComplete = false
	if _, err := EvaluateRound(previous, next, selected, RoundState{}, nil, brokenEvidence, policy); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("incomplete evidence error = %v", err)
	}

	replaceCase(previous, "pass_a", "pass")
	replaceCase(next, "pass_a", "unsupported", testGap("simulation", "simulation", "regression", "FAIL"))
	if _, err := EvaluateRound(previous, next, selected, RoundState{}, nil, completeRoundEvidence(), policy); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("pass regression error = %v", err)
	}

	previous, selected = roundFixture(t, policy)
	next = cloneCases(previous)
	unsafeGap := testGap("drc", "physical", "clearance", "DRC")
	replaceCase(previous, "pass_a", "unsafe", unsafeGap)
	replaceCase(next, "pass_a", "pass")
	replaceCase(next, "a", "unsupported", testGap("value_search", "simulation", "downstream", "VALUE"))
	replaceCase(next, "b", "unsupported", testGap("value_search", "simulation", "downstream", "VALUE"))
	if _, err := EvaluateRound(previous, next, selected, RoundState{}, nil, completeRoundEvidence(), policy); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("unsafe-to-pass error = %v", err)
	}
}

func TestEvaluateRoundRejectsNonExhaustiveCoveredCohort(t *testing.T) {
	policy := testPolicy(t)
	previous, selected := roundFixture(t, policy)
	shared := previousGap(previous, "a")
	replaceCase(previous, "pass_c", "unsupported", shared)
	next := cloneCases(previous)
	replacement := testGap("value_search", "simulation", "downstream", "VALUE")
	replaceCase(next, "a", "unsupported", replacement)
	replaceCase(next, "b", "unsupported", replacement)
	replaceCase(next, "pass_c", "unsupported", replacement)

	if _, err := EvaluateRound(previous, next, selected, RoundState{}, nil, completeRoundEvidence(), policy); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("non-exhaustive cohort error = %v", err)
	}
}

func roundFixture(t *testing.T, policy Policy) ([]Case, Candidate) {
	t.Helper()
	shared := testGap("simulation", "simulation", "shared", "MISSING")
	cases := []Case{
		testCase("a", "analog", "review_required", shared),
		testCase("b", "power", "safety_relevant", shared),
	}
	cases = append(cases, inactiveCases(policy.ExpectedDiscoveryCaseCount-len(cases))...)
	selection, err := Select(cases, RoundState{}, policy)
	if err != nil {
		t.Fatal(err)
	}
	return cases, selection.Selected
}

func cloneCases(cases []Case) []Case {
	result := make([]Case, len(cases))
	for index, value := range cases {
		result[index] = value
		result[index].Frontier = slices.Clone(value.Frontier)
	}
	return result
}

func replaceCase(cases []Case, id, outcome string, gaps ...Gap) {
	for index := range cases {
		if cases[index].ID == id {
			cases[index].Outcome = outcome
			cases[index].Frontier = slices.Clone(gaps)
			return
		}
	}
}

func previousGap(cases []Case, id string) Gap {
	for _, value := range cases {
		if value.ID == id {
			return value.Frontier[0]
		}
	}
	return Gap{}
}

func completeRoundEvidence() RoundEvidence {
	return RoundEvidence{DeterministicReplayComplete: true, PhysicalPromotionComplete: true, SealEnvironmentValid: true}
}

func TestMergeSortedStrings(t *testing.T) {
	got := mergeSortedStrings([]string{"a", "c", "c", "e"}, []string{"b", "c", "d", "f", "f"})
	if !slices.Equal(got, []string{"a", "b", "c", "d", "e", "f"}) {
		t.Fatalf("merged strings = %q", got)
	}
}
