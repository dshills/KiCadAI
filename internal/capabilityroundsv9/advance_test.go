package capabilityroundsv9

import (
	"errors"
	"strings"
	"testing"
)

func TestEvaluateRoundAdmitsUpliftAndAppendOnlySuccessors(t *testing.T) {
	previous, selected := roundFixture(t)
	next := cloneCases(previous)
	next[0].Outcome, next[0].Frontier = "pass", nil
	for index := 1; index < 4; index++ {
		prior := previous[index].Frontier[0]
		stage := "component"
		if prior.Path[0].Stage == "component" {
			stage = "model"
		}
		next[index].Frontier = []Gap{testSuccessor(prior, stage, "NEXT"+leftPad(index))}
	}
	evaluation, err := EvaluateRound(previous, next, selected, initialState(), completeEvidence(), FrozenPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Status != EvaluationPublicAdmitted || evaluation.DiscoveryPassBefore != 20 || evaluation.DiscoveryPassAfter != 21 || evaluation.NewActiveCohortPasses != 1 || len(evaluation.AdvancedCaseIDs) != 4 || len(evaluation.AdvancedReportingDomains) != 4 || len(evaluation.AdvancedCircuitRoles) != 4 || len(evaluation.Successors) != 3 {
		t.Fatalf("unexpected evaluation: %+v", evaluation)
	}
	if evaluation.NextState.Generation != 1 || evaluation.NextState.UsedAtomCount != 2 || evaluation.NextState.UsedMemberCount != 2 || len(evaluation.NextState.PriorAtomKeys) != 2 || len(evaluation.NextState.ActiveCohortIDs) != 4 {
		t.Fatalf("unexpected next state: %+v", evaluation.NextState)
	}
}

func TestEvaluateRoundContinuesOnDiverseProgressWithoutPassUplift(t *testing.T) {
	previous, selected := roundFixture(t)
	next := cloneCases(previous)
	for index := 0; index < 4; index++ {
		prior := previous[index].Frontier[0]
		stage := "component"
		if prior.Path[0].Stage == "component" {
			stage = "model"
		}
		next[index].Frontier = []Gap{testSuccessor(prior, stage, "NEXT"+leftPad(index))}
	}
	evaluation, err := EvaluateRound(previous, next, selected, initialState(), completeEvidence(), FrozenPolicy())
	if err != nil || evaluation.Status != EvaluationContinue || evaluation.DiscoveryPassAfter != evaluation.DiscoveryPassBefore || len(evaluation.Successors) != 4 {
		t.Fatalf("continue evaluation = %+v, %v", evaluation, err)
	}
	fanout := cloneCases(next)
	fanout[0].Frontier = nil
	for index := 0; index < 4; index++ {
		fanout[0].Frontier = append(fanout[0].Frontier, testSuccessor(previous[0].Frontier[0], "component", "OK"+leftPad(index)))
	}
	evaluation, err = EvaluateRound(previous, fanout, selected, initialState(), completeEvidence(), FrozenPolicy())
	if err != nil || len(evaluation.Successors) != 7 {
		t.Fatalf("bounded fanout evaluation = %+v, %v", evaluation, err)
	}
}

func TestEvaluateRoundFailsClosedOnFanoutDriftRegressionAndBudget(t *testing.T) {
	previous, selected := roundFixture(t)
	next := cloneCases(previous)
	for index := 0; index < 4; index++ {
		prior := previous[index].Frontier[0]
		stage := "component"
		if prior.Path[0].Stage == "component" {
			stage = "model"
		}
		next[index].Frontier = []Gap{testSuccessor(prior, stage, "NEXT"+leftPad(index))}
	}
	fanout := cloneCases(next)
	fanout[0].Frontier = nil
	for index := 0; index < 5; index++ {
		fanout[0].Frontier = append(fanout[0].Frontier, testSuccessor(previous[0].Frontier[0], "component", "FAN"+leftPad(index)))
	}
	if _, err := EvaluateRound(previous, fanout, selected, initialState(), completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("fanout error = %v", err)
	}
	rewritten := cloneCases(next)
	rewritten[0].Frontier[0].Path = rewritten[0].Frontier[0].Path[1:]
	if _, err := EvaluateRound(previous, rewritten, selected, initialState(), completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("rewritten path error = %v", err)
	}
	lower := cloneCases(next)
	lower[2].Frontier = []Gap{testSuccessor(previous[2].Frontier[0], "topology", "LOWER")}
	if _, err := EvaluateRound(previous, lower, selected, initialState(), completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("lower-stage successor error = %v", err)
	}

	driftBefore, driftAfter := cloneCases(previous), cloneCases(next)
	extra := testLeaf("verification", "unselected", "unselected", "UNSELECTED")
	driftBefore[4].Outcome = "unsafe"
	driftBefore[4].Frontier = []Gap{{ObligationAnchor: identityDigest("anchor", "case_005"), Path: []Leaf{extra}, Diagnostics: []string{"diagnostic"}}}
	driftAfter[4] = driftBefore[4]
	driftAfter[4].Frontier = []Gap{{ObligationAnchor: driftBefore[4].Frontier[0].ObligationAnchor, Path: []Leaf{extra}, Diagnostics: []string{"changed"}}}
	if _, err := EvaluateRound(driftBefore, driftAfter, selected, initialState(), completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("nonclosure drift error = %v", err)
	}

	regressed := cloneCases(next)
	regressed[5].Outcome = "unsafe"
	if _, err := EvaluateRound(previous, regressed, selected, initialState(), completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("pass regression error = %v", err)
	}

	state := RoundState{Generation: 1, UsedAtomCount: 4, UsedMemberCount: 16, PriorAtomKeys: priorAtomsExceptSelected(t, selected, 4), ActiveCohortIDs: initialState().ActiveCohortIDs}
	if _, err := EvaluateRound(previous, next, selected, state, completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("budget exhaustion error = %v", err)
	}
}

func roundFixture(t *testing.T) ([]Case, Candidate) {
	t.Helper()
	leafA := testLeaf("topology", "shared_a", "cap_a", "A")
	leafB := testLeaf("component", "shared_b", "cap_b", "B")
	cases := testCases(leafA, leafB)
	selection, err := Select(cases, []EffectPlan{testPlan(t, []Leaf{leafA, leafB}, nil)}, initialState(), FrozenPolicy())
	if err != nil {
		t.Fatal(err)
	}
	return cases, selection.Selected
}

func testSuccessor(prior Gap, stage, code string) Gap {
	leaves := append([]Leaf(nil), prior.Path...)
	leaves = append(leaves, Leaf{Stage: stage, Category: stage, Scope: "successor_" + strings.ToLower(code), Capability: "successor", Code: code, RequiredEvidence: []string{"evidence", "stronger"}})
	return Gap{ObligationAnchor: prior.ObligationAnchor, Path: leaves, Diagnostics: []string{"diagnostic", "successor"}}
}

func cloneCases(values []Case) []Case {
	result := make([]Case, len(values))
	for index, value := range values {
		result[index] = value
		result[index].SatisfiedObligations = append([]string(nil), value.SatisfiedObligations...)
		result[index].Frontier = make([]Gap, len(value.Frontier))
		for gapIndex, gap := range value.Frontier {
			result[index].Frontier[gapIndex] = gap
			result[index].Frontier[gapIndex].Diagnostics = append([]string(nil), gap.Diagnostics...)
			result[index].Frontier[gapIndex].Path = make([]Leaf, len(gap.Path))
			for leafIndex, leaf := range gap.Path {
				result[index].Frontier[gapIndex].Path[leafIndex] = leaf
				result[index].Frontier[gapIndex].Path[leafIndex].RequiredEvidence = append([]string(nil), leaf.RequiredEvidence...)
			}
		}
	}
	return result
}

func completeEvidence() RoundEvidence {
	return RoundEvidence{DeterministicReplayComplete: true, PhysicalPromotionComplete: true, SealEnvironmentValid: true, EffectClosureValid: true}
}

func priorAtomsExceptSelected(t *testing.T, selected Candidate, count int) []string {
	t.Helper()
	selectedSet := map[string]bool{}
	for _, atom := range selected.Atoms {
		selectedSet[atom.Key] = true
	}
	result := []string{}
	for index := 0; len(result) < count; index++ {
		key, err := AtomKey("topology", "prior_"+leftPad(index), "prior")
		if err != nil {
			t.Fatal(err)
		}
		if !selectedSet[key] {
			result = append(result, key)
		}
	}
	slicesSort(result)
	return result
}

func slicesSort(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
