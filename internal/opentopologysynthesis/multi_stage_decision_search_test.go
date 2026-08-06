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
		onlyCurrentFailures := true
		for _, diagnosis := range evaluation.Diagnoses {
			if diagnosis.RequirementID == "active_current" {
				continue
			}
			onlyCurrentFailures = false
		}
		if !onlyCurrentFailures {
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
		"no composed candidate passed the protected decision operating envelope: candidates=%d consumption=%#v rejections=%#v",
		len(search.Candidates), search.Consumption, search.Rejections,
	)
}

func TestControlledSwitchRelationshipComposesRegulatedLoadRail(t *testing.T) {
	var requirement Requirement
	decodeFrozenStrict(
		t,
		mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "ambient_tracking_airflow_control.json")),
		&requirement,
	)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := DefaultPolicy()
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	for _, candidate := range search.Candidates {
		converterCount, ballastCount := 0, 0
		for _, instance := range candidate.Graph.Instances {
			if instance.Kind == "isolated_converter" {
				converterCount++
			}
			if instance.Kind == "resistor" && instance.ValueSI != nil && *instance.ValueSI < 10 {
				ballastCount++
			}
		}
		if converterCount != 1 || ballastCount != 2 {
			continue
		}
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
		if evaluation.Status == SimulationEvaluationPassed {
			return
		}
		t.Logf(
			"regulated-load-rail candidate values=%s diagnoses=%#v",
			testValueTrialSummary(plan, 0), evaluation.Diagnoses,
		)
	}
	t.Fatalf(
		"no regulated-load-rail candidate passed every electrical and safety assertion: candidates=%d consumption=%#v rejections=%#v",
		len(search.Candidates), search.Consumption, search.Rejections,
	)
}

func TestHighSideUndervoltageDisconnectPassesAbsoluteThresholdCorners(t *testing.T) {
	var requirement Requirement
	decodeFrozenStrict(
		t,
		mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "undervoltage_load_permission.json")),
		&requirement,
	)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	policy := multiStageOODPromotionPolicy()
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, policy)
	for _, candidate := range search.Candidates {
		if !topologyGraphHasAbsoluteDecisionReference(candidate.Graph) {
			continue
		}
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
			context.Background(), requirement, graph, &enumeration.Trials[0],
			inventory, environment, policy,
		)
		if evaluation.Status == SimulationEvaluationPassed {
			return
		}
		t.Logf("undervoltage values=%s diagnoses=%#v", testValueTrialSummary(plan, 0), evaluation.Diagnoses)
	}
	t.Fatalf(
		"no absolute-reference high-side disconnect passed every electrical and safety corner: candidates=%d consumption=%#v rejections=%#v",
		len(search.Candidates), search.Consumption, search.Rejections,
	)
}

func TestAbsoluteDecisionReferenceObligationTracksSweptSupplyEnvelope(t *testing.T) {
	for _, test := range []struct {
		name string
		want bool
	}{
		{name: "undervoltage_load_permission", want: true},
		{name: "ambient_tracking_airflow_control", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requirement Requirement
			decodeFrozenStrict(
				t,
				mustRead(t, filepath.Join(multiStageOODCorpusRoot(), test.name+".json")),
				&requirement,
			)
			if got := topologyRequiresAbsoluteDecisionReference(requirement); got != test.want {
				t.Fatalf("absolute decision reference obligation = %t, want %t", got, test.want)
			}
		})
	}
}

func TestControlledSwitchRelationshipOrientsPowerSourceHighSide(t *testing.T) {
	var requirement Requirement
	decodeFrozenStrict(
		t,
		mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "undervoltage_load_permission.json")),
		&requirement,
	)
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	output := ""
	for _, node := range initial.Nodes {
		if node.SemanticKind == "port" && node.SemanticID == "permitted_output" {
			output = node.ID
			break
		}
	}
	if output == "" || !topologyControlledSwitchHighSideOutput(requirement, initial, output) {
		t.Fatalf("power-source output did not derive high-side orientation: output=%q", output)
	}
	search := SearchPrimitiveTopologies(context.Background(), requirement, inventory, DefaultPolicy())
	for _, candidate := range search.Candidates {
		nodes := map[string]GraphNode{}
		for _, node := range candidate.Graph.Nodes {
			nodes[node.ID] = node
		}
		for _, instance := range candidate.Graph.Instances {
			if instance.Kind != "p_channel_mosfet" {
				continue
			}
			terminals := topologyTerminalNodes(instance)
			if terminals["DRAIN"] == output && nodes[terminals["SOURCE"]].Role == "supply" {
				return
			}
		}
	}
	t.Fatalf("no high-side PMOS candidate was composed: candidates=%d rejections=%#v", len(search.Candidates), search.Rejections)
}
