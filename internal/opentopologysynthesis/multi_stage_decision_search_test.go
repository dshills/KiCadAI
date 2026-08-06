package opentopologysynthesis

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/simmodel"
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
			if instance.Kind == "resistor" && instance.ValueSI != nil && *instance.ValueSI <= 10 {
				ballastCount++
			}
		}
		if converterCount != 1 || ballastCount < 1 || ballastCount > 2 {
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

func TestWindowedControlledSwitchComposesSharedDecisionAndProtectedPower(t *testing.T) {
	var requirement Requirement
	decodeFrozenStrict(
		t,
		mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "windowed_heating_power_control.json")),
		&requirement,
	)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	envelope, hasWindow := topologyWindowThresholdEnvelope(requirement)
	if !hasWindow || !topologyControlledSwitchRequired(requirement) {
		t.Fatalf(
			"windowed controlled-power obligation missing: envelope=%#v window=%t controlled=%t",
			envelope, hasWindow, topologyControlledSwitchRequired(requirement),
		)
	}
	supplies := topologyNodesByRole(initial, "supply")
	loadSupply, _ := topologyControlledSwitchLoadSupply(requirement, initial, "port_"+envelope.output, supplies)
	regulated, regulatedFound := topologyRegulatedLoadRail(requirement, initial, "port_"+envelope.output, loadSupply, inventory)
	if !regulatedFound || regulated.seriesCount < 1 || regulated.parallelCount < 1 ||
		len(regulated.ballast) != 1 || len(regulated.ballastValueSI) != 1 ||
		regulated.ballastValueSI[0] <= 0 {
		t.Fatalf("windowed load envelope lacks a realizable regulated rail: %#v", regulated)
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
	policy := multiStageOODPromotionPolicy()
	candidates, consumption, rejections := topologyWindowedControlledSwitchRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
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
	for _, candidate := range candidates {
		sharedOutputs := map[string]int{}
		hasPowerSwitch, hasProtection, hasGateClamp := false, false, false
		gateNode, sourceNode := "", ""
		for _, instance := range candidate.Graph.Instances {
			terminals := topologyTerminalNodes(instance)
			switch instance.Kind {
			case "comparator":
				sharedOutputs[terminals["OUT"]]++
			case "p_channel_mosfet", "n_channel_mosfet":
				hasPowerSwitch = terminals["DRAIN"] == "port_heating_output"
				gateNode, sourceNode = terminals["GATE"], terminals["SOURCE"]
			case "diode", "signal_diode":
				hasProtection = terminals["ANODE"] == "port_heating_output" ||
					terminals["CATHODE"] == "port_heating_output"
			case "clamp_diode":
				hasGateClamp = (terminals["ANODE"] == gateNode && terminals["CATHODE"] == sourceNode) ||
					(terminals["CATHODE"] == gateNode && terminals["ANODE"] == sourceNode)
			}
		}
		for _, count := range sharedOutputs {
			if count >= 2 && hasPowerSwitch && hasProtection && hasGateClamp {
				plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
				if plan.Status != ValuePlanReady {
					t.Fatalf("windowed composition lacks a deterministic value plan: %#v", plan)
				}
				enumeration := EnumerateValueTrials(plan, 1)
				if len(enumeration.Trials) != 1 {
					t.Fatalf("windowed first value trial missing: %#v", enumeration)
				}
				valued, err := ApplyValueTrial(candidate.Graph, enumeration.Trials[0], inventory)
				if err != nil {
					t.Fatalf("windowed first value trial cannot be applied: %v", err)
				}
				for _, assertion := range requirement.Requirements.BehavioralRequirements {
					if assertion.ID != "lower_entry" && assertion.ID != "upper_exit" &&
						assertion.ID != "boundary_stability" && assertion.ID != "active_current" &&
						assertion.ID != "window_delay" {
						continue
					}
					for _, operatingCase := range requirement.Requirements.OperatingCases {
						if operatingCase.ID != "window_sweep" {
							continue
						}
						operatingCase.Conditions = simulationHarnessConditions(requirement, assertion, operatingCase)
						attempt, diagnoses := evaluateAssertionCorner(
							requirement, assertion, operatingCase,
							operatingCorner{ID: "nominal", Values: map[string]float64{}},
							valued, inventory, environment,
						)
						if attempt.Status != SimulationEvaluationPassed {
							t.Fatalf("windowed %s nominal decision evidence failed: %#v", assertion.ID, diagnoses)
						}
					}
				}
				return
			}
		}
	}
	t.Fatalf(
		"windowed controlled-switch composition missing: candidates=%d consumption=%#v rejections=%#v",
		len(candidates), consumption, rejections,
	)
}

func TestMOSFETGateClampSelectionReservesCatalogRatingMargin(t *testing.T) {
	inventory, _ := testHeldOutSynthesisEnvironment(t)
	mosfet := PrimitiveCandidate{}
	for _, primitive := range inventory.Primitives {
		if primitive.Kind != "p_channel_mosfet" ||
			!primitiveHasModel(primitive, simmodel.PrimitivePMOSSwitchV1) {
			continue
		}
		gateLimit := primitiveModelParameter(
			primitive, simmodel.PrimitivePMOSSwitchV1, "max_gate_source_voltage_v",
		)
		gateOn := primitiveModelParameter(
			primitive, simmodel.PrimitivePMOSSwitchV1, "gate_on_voltage_v",
		)
		if gateLimit == 25 && gateOn == 4.5 {
			mosfet = primitive
			break
		}
	}
	if mosfet.Key == "" {
		t.Fatal("test inventory lacks a reviewed 25 VGS, 4.5 V-drive PMOS")
	}
	if clamp, required := topologyMOSFETGateClampPrimitive(inventory, mosfet, 18.75); required || clamp.Key != "" {
		t.Fatalf("gate clamp required inside the reserved rating envelope: required=%t clamp=%#v", required, clamp)
	}
	clamp, required := topologyMOSFETGateClampPrimitive(inventory, mosfet, 24)
	if !required || clamp.Key == "" {
		t.Fatalf("gate clamp missing beyond the reserved rating envelope: required=%t clamp=%#v", required, clamp)
	}
	breakdown := primitiveModelParameter(
		clamp, simmodel.PrimitiveBidirectionalTVSV1, "breakdown_voltage_v",
	)
	if breakdown < 1.25*4.5 || breakdown > .85*25 {
		t.Fatalf("selected gate clamp does not preserve turn-on and VGS margins: breakdown=%g clamp=%#v", breakdown, clamp)
	}
	repeated, repeatedRequired := topologyMOSFETGateClampPrimitive(inventory, mosfet, 24)
	if !repeatedRequired || repeated.Key != clamp.Key {
		t.Fatalf("gate clamp selection is not deterministic: first=%q repeated=%q", clamp.Key, repeated.Key)
	}
}

func TestWindowedHeatingPowerPassesElectricalAndSafetyCorners(t *testing.T) {
	var requirement Requirement
	decodeFrozenStrict(
		t,
		mustRead(t, filepath.Join(multiStageOODCorpusRoot(), "windowed_heating_power_control.json")),
		&requirement,
	)
	inventory, environment := testHeldOutSynthesisEnvironment(t)
	run := Synthesize(
		context.Background(), requirement, inventory, environment, multiStageOODPromotionPolicy(),
	)
	if run.Report.Status == StatusPassed && run.SelectedGraph != nil && run.SelectedTrial != nil {
		return
	}
	logged := 0
	for _, candidate := range run.Candidates {
		for _, evaluation := range candidate.Evaluations {
			if logged >= 8 {
				break
			}
			first := Diagnosis{}
			if len(evaluation.Diagnoses) != 0 {
				first = evaluation.Diagnoses[0]
			}
			t.Logf(
				"windowed candidate topology=%s plan=%s trial=%s status=%s diagnoses=%d first=%#v",
				candidate.TopologyHash, candidate.ValuePlan.Status, evaluation.ValueTrialHash,
				evaluation.Status, len(evaluation.Diagnoses), first,
			)
			logged++
		}
	}
	t.Fatalf(
		"windowed heating synthesis did not pass: status=%s stop=%s candidates=%d consumption=%#v rejections=%#v diagnostics=%#v",
		run.Report.Status, run.Report.StopReason, len(run.Candidates),
		run.Search.Consumption, run.Search.Rejections, run.Report.Diagnostics,
	)
}
