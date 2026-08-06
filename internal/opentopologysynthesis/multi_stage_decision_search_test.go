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
