package capabilityroundsv9

import (
	"errors"
	"slices"
	"testing"
)

func TestSelectCommitsCompleteEffectExposureAndSiblingBurden(t *testing.T) {
	leafA := testLeaf("topology", "shared_a", "cap_a", "A")
	leafB := testLeaf("component", "sibling_b", "cap_b", "B")
	cases := exposureCases(leafA, leafB)
	selection, err := Select(cases, []EffectPlan{testPlan(t, []Leaf{leafA}, nil)}, stateForCases(cases), FrozenPolicy())
	if err != nil {
		t.Fatal(err)
	}
	selected := selection.Selected
	if !slices.Equal(selected.FullyCoveredCaseIDs, []string{"case_001", "case_002"}) ||
		!slices.Equal(selected.EffectExposureCaseIDs, []string{"case_001", "case_002", "case_003"}) ||
		selected.ExposedNoncoveredCaseCount != 1 || selected.NonselectedSiblingPathCount != 1 ||
		len(selected.Exposure) != 3 || len(selected.Exposure[2].SelectedPathHashes) != 1 || len(selected.Exposure[2].NonselectedSiblingPathHashes) != 1 ||
		len(selected.NonExposedCases) != 21 {
		t.Fatalf("incomplete V9 exposure commitment: %+v", selected)
	}
}

func TestSelectCountsClosureTowardCoverage(t *testing.T) {
	leafA := testLeaf("topology", "shared_a", "cap_a", "A")
	leafB := testLeaf("component", "shared_b", "cap_b", "B")
	cases := testCases(leafA, leafB)
	selection, err := Select(cases, []EffectPlan{testPlan(t, []Leaf{leafA}, []Leaf{leafB})}, initialState(), FrozenPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(selection.Selected.FullyCoveredCaseIDs, []string{"case_001", "case_002", "case_003", "case_004"}) {
		t.Fatalf("closure coverage was not counted: %+v", selection.Selected)
	}
}

func TestSelectRanksLowerCollateralExposureBeforeCanonicalIdentity(t *testing.T) {
	leafA := testLeaf("topology", "shared_a", "cap_a", "A")
	leafB := testLeaf("component", "shared_b", "cap_b", "B")
	sibling := testLeaf("verification", "unrelated", "unrelated", "U")
	cases := []Case{
		testCase("case_001", "analog_signal_path", "source_bias", "review_required", leafA),
		testCase("case_002", "power_energy_conversion", "conversion_regulation", "review_required", leafA),
		testCase("case_003", "analog_signal_path", "source_bias", "review_required", leafB),
		testCase("case_004", "power_energy_conversion", "conversion_regulation", "review_required", leafB),
	}
	extra := testCase("case_005", "digital_control", "interface_control", "non_safety", sibling)
	extra.Frontier = append([]Gap{{ObligationAnchor: identityDigest("anchor", "case_005", "selected"), Path: []Leaf{leafA}, Diagnostics: []string{"diagnostic"}}}, extra.Frontier...)
	cases = append(cases, extra)
	for index := 6; index <= 24; index++ {
		cases = append(cases, Case{ID: "case_" + leftPad(index), Role: "discovery", ReportingDomain: "sensing_instrumentation",
			CircuitRole: "amplification_conditioning", SafetyImpact: "non_safety", Outcome: "pass"})
	}
	selection, err := Select(cases, []EffectPlan{testPlan(t, []Leaf{leafA}, nil), testPlan(t, []Leaf{leafB}, nil)}, stateForCases(cases), FrozenPolicy())
	if err != nil {
		t.Fatal(err)
	}
	wantAtom, _ := AtomKey(leafB.Category, leafB.Scope, leafB.Capability)
	if !slices.Contains(selection.Selected.DirectAtomKeys, wantAtom) || selection.Selected.ExposedNoncoveredCaseCount != 0 || selection.Selected.NonselectedSiblingPathCount != 0 {
		t.Fatalf("selection ignored collateral exposure ranking: %+v", selection)
	}
}

func TestEvaluateRoundPreservesCommittedSiblingsAndNonExposedCases(t *testing.T) {
	leafA := testLeaf("topology", "shared_a", "cap_a", "A")
	leafB := testLeaf("component", "sibling_b", "cap_b", "B")
	previous := exposureCases(leafA, leafB)
	previous[0].SatisfiedObligations = []string{identityDigest("already", "satisfied")}
	state := stateForCases(previous)
	selection, err := Select(previous, []EffectPlan{testPlan(t, []Leaf{leafA}, nil)}, state, FrozenPolicy())
	if err != nil {
		t.Fatal(err)
	}
	next := cloneCases(previous)
	for index := 0; index < 3; index++ {
		selectedGap := previous[index].Frontier[0]
		next[index].Frontier[0] = testSuccessor(selectedGap, "component", "NEXT"+leftPad(index))
	}
	if _, err := EvaluateRound(previous, next, selection.Selected, state, completeEvidence(), FrozenPolicy()); err != nil {
		t.Fatalf("valid exposed advancement failed: %v", err)
	}
	satisfactionRegression := cloneCases(next)
	satisfactionRegression[0].SatisfiedObligations = nil
	if _, err := EvaluateRound(previous, satisfactionRegression, selection.Selected, state, completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("satisfied-obligation regression error = %v", err)
	}

	siblingDrift := cloneCases(next)
	siblingDrift[2].Frontier[1].Diagnostics = []string{"changed"}
	if _, err := EvaluateRound(previous, siblingDrift, selection.Selected, state, completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("nonselected sibling drift error = %v", err)
	}
	nonExposedDrift := cloneCases(next)
	nonExposedDrift[3].SafetyImpact = "review_required"
	if _, err := EvaluateRound(previous, nonExposedDrift, selection.Selected, state, completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("non-exposed case drift error = %v", err)
	}
	missingExposure := selection.Selected
	missingExposure.EffectExposureCaseIDs = missingExposure.EffectExposureCaseIDs[:2]
	if _, err := EvaluateRound(previous, next, missingExposure, state, completeEvidence(), FrozenPolicy()); !errors.Is(err, ErrRoundGate) {
		t.Fatalf("exposure omission error = %v", err)
	}
}

func exposureCases(selected, sibling Leaf) []Case {
	selectedAtomVariant := selected
	selectedAtomVariant.Code = selected.Code + "_VARIANT"
	result := []Case{
		testCase("case_001", "analog_signal_path", "source_bias", "review_required", selected),
		testCase("case_002", "power_energy_conversion", "conversion_regulation", "review_required", selected),
		testCase("case_003", "digital_control", "interface_control", "review_required", selectedAtomVariant),
	}
	second := Gap{ObligationAnchor: identityDigest("anchor", "case_003", "sibling"), Path: []Leaf{sibling}, Diagnostics: []string{"diagnostic"}}
	result[2].Frontier = append(result[2].Frontier, second)
	for index := 4; index <= 24; index++ {
		result = append(result, Case{ID: "case_" + leftPad(index), Role: "discovery", ReportingDomain: "sensing_instrumentation",
			CircuitRole: "amplification_conditioning", SafetyImpact: "non_safety", Outcome: "pass"})
	}
	return result
}

func stateForCases(cases []Case) RoundState {
	active := []string{}
	for _, current := range cases {
		if current.Outcome == "unsupported" || current.Outcome == "exhausted" {
			active = append(active, current.ID)
		}
	}
	return RoundState{Generation: 0, ActiveCohortIDs: active}
}
