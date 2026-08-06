package opentopologysynthesis

import (
	"context"
	"path/filepath"
	"testing"
)

func TestControlledSwitchRelationshipComposesDecisionFeedback(t *testing.T) {
	var requirement Requirement
	decodeFrozenStrict(
		t,
		mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "ambient_tracking_airflow_control.json")),
		&requirement,
	)
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	graphHash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topologyHash, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	policy := DefaultPolicy()
	policy.MaxExpandedStates = 64
	policy.MaxGeneratedGraphs = 512
	candidates, consumption, rejections := topologyControlledSwitchRelationshipSeeds(
		context.Background(),
		requirement,
		inventory,
		topologyRepresentatives(requirement, inventory),
		byKey,
		GraphLimits{
			MaxPrimitiveInstances: minPositive(
				policy.MaxPrimitiveInstances,
				requirement.Requirements.Constraints.MaxComponents,
			),
			MaxInternalNodes: policy.MaxInternalNodes,
		},
		policy,
		topologySearchState{
			graph: initial, hash: graphHash, topology: topologyHash,
			score: scoreTopologyGraph(requirement, initial, byKey, graphHash),
		},
	)
	if len(candidates) == 0 {
		t.Fatalf("hysteretic controlled-power relationships produced no candidates: consumption=%#v rejections=%#v", consumption, rejections)
	}
	for _, candidate := range candidates {
		if candidate.Score.BehaviorGap != 0 || !topologyGraphHasDecisionFeedback(candidate.Graph) {
			t.Fatalf("candidate does not close its decision and power obligations: score=%#v graph=%#v", candidate.Score, candidate.Graph)
		}
	}
	var hysteresis BehavioralAssertion
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.ID == "decision_memory" {
			hysteresis = assertion
			break
		}
	}
	var thresholdSweep OperatingCase
	for _, operatingCase := range requirement.Requirements.OperatingCases {
		if operatingCase.ID == "threshold_sweep" {
			thresholdSweep = operatingCase
			break
		}
	}
	source, start, stop, ok := sweepSourceAndRange(
		requirement,
		hysteresis,
		thresholdSweep,
		operatingCorner{Values: map[string]float64{}},
		candidates[0].Graph,
	)
	if !ok || source == "" || start != 0.4 || stop != 2.6 {
		t.Fatalf("inferred hysteresis sweep = %q %.12g..%.12g found=%t", source, start, stop, ok)
	}
}

func TestControlledSwitchRelationshipDerivesProtectedDecisionOperatingEnvelope(t *testing.T) {
	var requirement Requirement
	decodeFrozenStrict(
		t,
		mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "ambient_tracking_airflow_control.json")),
		&requirement,
	)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	wantPassed := []string{
		"rising_decision",
		"falling_decision",
		"decision_memory",
		"quiet_start",
		"safe_temperature",
		"safe_operating_area",
	}
	for _, candidate := range search.Candidates {
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		if plan.Status != ValuePlanReady {
			continue
		}
		enumeration := EnumerateValueTrials(plan, 1)
		if len(enumeration.Trials) == 0 {
			continue
		}
		graph, err := ApplyValueTrial(candidate.Graph, enumeration.Trials[0], inventory)
		if err != nil {
			t.Fatal(err)
		}
		evaluation := EvaluateCandidate(
			context.Background(), requirement, graph, nil, inventory, environment, policy,
		)
		activeCurrentFailure := false
		onlyCurrentFailures := true
		for _, diagnosis := range evaluation.Diagnoses {
			if diagnosis.RequirementID == "active_current" {
				activeCurrentFailure = true
				continue
			}
			onlyCurrentFailures = false
		}
		if !activeCurrentFailure || !onlyCurrentFailures {
			continue
		}
		passed := map[string]bool{}
		for _, attempt := range evaluation.Attempts {
			if attempt.Status == SimulationEvaluationPassed {
				passed[attempt.RequirementID] = true
			}
		}
		allPassed := true
		for _, requirementID := range wantPassed {
			if !passed[requirementID] {
				allPassed = false
				break
			}
		}
		if allPassed {
			return
		}
	}
	t.Fatalf(
		"no composed candidate isolated the remaining gap to active-current regulation: candidates=%d consumption=%#v rejections=%#v",
		len(search.Candidates), search.Consumption, search.Rejections,
	)
}
