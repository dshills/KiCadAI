package opentopologysynthesis

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"kicadai/internal/simmodel"
)

func TestRegulatedVoltageRelationshipsDeriveCommandProtectionAndDirection(t *testing.T) {
	tests := []struct {
		file          string
		command       string
		output        string
		gain          float64
		currentLimit  float64
		bidirectional bool
	}{
		{"low_noise_adjustable_voltage_output.json", "voltage_command", "quiet_output", 4.15625, 0.25, false},
		{"protected_high_power_voltage_output.json", "output_enable", "power_output", 2.4, 2, false},
		{"bidirectional_midrail_voltage_output.json", "midrail_enable", "midrail_output", 1.2, 0.2, true},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			requirement := testProtectedVoltageOutputRequirement(t, test.file)
			relationships := regulatedVoltageRelationships(requirement)
			if len(relationships) != 1 {
				t.Fatalf("regulated voltage relationships = %#v", relationships)
			}
			got := relationships[0]
			if got.command != test.command || got.output != test.output ||
				math.Abs(got.gain-test.gain) > 1e-12 ||
				math.Abs(got.currentLimitA-test.currentLimit) > 1e-12 ||
				got.bidirectional != test.bidirectional ||
				got.minimumHeadroomV < protectedVoltageCurrentSenseDropV+0.8 {
				t.Fatalf("regulated voltage relationship = %#v", got)
			}
		})
	}
}

func TestProtectedVoltageRelationshipBuildsDerivedFeedbackAndCurrentLimit(t *testing.T) {
	tests := []struct {
		file         string
		minimumNPN   int
		minimumPNP   int
		maximumParts int
	}{
		{"low_noise_adjustable_voltage_output.json", 2, 0, 28},
		{"protected_high_power_voltage_output.json", 2, 0, 32},
		{"bidirectional_midrail_voltage_output.json", 2, 2, 32},
	}
	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			requirement := testProtectedVoltageOutputRequirement(t, test.file)
			inventory, environment := testHeldOutSynthesisEnvironment(t)
			candidates, consumption, rejections := testProtectedVoltageRelationshipCandidates(
				t, requirement, inventory,
			)
			if len(candidates) != 1 {
				t.Fatalf("protected voltage candidates = %d, consumption=%#v rejections=%#v", len(candidates), consumption, rejections)
			}
			candidate := candidates[0]
			counts := map[string]int{}
			for _, instance := range candidate.Graph.Instances {
				counts[instance.Kind]++
			}
			if candidate.Score.BehaviorGap != 0 || counts["opamp"] != 1 ||
				counts["npn_bjt"] < test.minimumNPN || counts["pnp_bjt"] < test.minimumPNP ||
				len(candidate.Graph.Instances) > test.maximumParts {
				t.Fatalf("protected voltage topology score=%#v counts=%v graph=%s", candidate.Score, counts, testGraphTopologySummary(candidate.Graph))
			}
			plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, protectedVoltageOutputSynthesisPolicy())
			if plan.Status != ValuePlanReady {
				t.Fatalf("protected voltage value plan=%s rejections=%#v issues=%#v", plan.Status, plan.Rejections, plan.Issues)
			}
			relationship := regulatedVoltageRelationships(requirement)[0]
			feedbackRatio, found := protectedVoltageFeedbackRatio(candidate.Graph, relationship.output)
			if !found || feedbackRatio < relationship.minimumGain || feedbackRatio > relationship.maximumGain {
				t.Fatalf("feedback gain=%g found=%t envelope=[%g,%g]", feedbackRatio, found, relationship.minimumGain, relationship.maximumGain)
			}
			limitResistance, found := protectedVoltageSourceSenseResistance(candidate.Graph, relationship.output)
			thresholdCurrent := protectedVoltageCurrentSenseDropV / limitResistance
			if !found || thresholdCurrent < relationship.maximumRatedCurrentA*0.95 || thresholdCurrent > relationship.currentLimitA*1.05 {
				t.Fatalf("source current threshold=%g found=%t rated=%g limit=%g graph=%s", thresholdCurrent, found, relationship.maximumRatedCurrentA, relationship.currentLimitA, testGraphTopologySummary(candidate.Graph))
			}
			if os.Getenv("KICADAI_PROTECTED_VOLTAGE_DIAGNOSTIC") == "1" {
				t.Logf("diagnostic graph=%s", testGraphTopologySummary(candidate.Graph))
				trials := EnumerateValueTrials(plan, 1)
				if len(trials.Trials) != 1 {
					t.Fatalf("diagnostic value trials = %#v", trials)
				}
				evaluation := EvaluateCandidate(
					context.Background(), requirement, candidate.Graph, &trials.Trials[0],
					inventory, environment, protectedVoltageOutputSynthesisPolicy(),
				)
				t.Logf("diagnostic evaluation status=%s issues=%#v diagnoses=%#v", evaluation.Status, evaluation.Issues, evaluation.Diagnoses)
				for _, attempt := range evaluation.Attempts {
					if attempt.Status != SimulationEvaluationPassed {
						t.Logf("diagnostic attempt requirement=%s case=%s analysis=%s actual=%v diagnostics=%#v", attempt.RequirementID, attempt.OperatingCase, attempt.Analysis, attempt.Actual, attempt.Diagnostics)
					}
				}
			}
		})
	}
}

func TestProtectedVoltageUsesResistiveStaticLoadAndCurrentSweepLoad(t *testing.T) {
	requirement := testProtectedVoltageOutputRequirement(t, "protected_high_power_voltage_output.json")
	condition := OperatingCondition{Axis: "load_current", Target: "power_output"}
	for _, analysis := range []string{simmodel.AnalysisDCOperatingPoint, "dc_sweep"} {
		assertion := BehavioralAssertion{Analysis: analysis, Metric: "output_voltage"}
		resistance, found := dynamicVoltageOutputLoadResistance(requirement, assertion, condition, 1.5)
		if !found || math.Abs(resistance-8) > 1e-12 {
			t.Fatalf("%s static load resistance = %.12g found=%t", analysis, resistance, found)
		}
	}
	if resistance, found := dynamicVoltageOutputLoadResistance(
		requirement,
		BehavioralAssertion{Analysis: simmodel.AnalysisDCOperatingPoint, Metric: "output_voltage"},
		condition,
		1e-15,
	); !found || resistance != maximumHarnessResistanceOhm {
		t.Fatalf("near-open static load resistance = %.12g found=%t", resistance, found)
	}
	if _, found := dynamicVoltageOutputLoadResistance(
		requirement,
		BehavioralAssertion{Analysis: "dc_sweep", Metric: "load_regulation"},
		condition,
		1.5,
	); found {
		t.Fatal("load-regulation sweep replaced its swept current source with a fixed resistor")
	}
}

func testProtectedVoltageOutputRequirement(t *testing.T, file string) Requirement {
	t.Helper()
	requirement, issues := DecodeStrict(bytes.NewReader(mustRead(
		t,
		filepath.Join(protectedVoltageOutputCorpusRoot(), file),
	)))
	if len(issues) != 0 {
		t.Fatalf("requirement decode issues: %#v", issues)
	}
	return requirement
}

func testProtectedVoltageRelationshipCandidates(
	t *testing.T,
	requirement Requirement,
	inventory PrimitiveInventory,
) ([]TopologyCandidate, Consumption, map[string][]string) {
	t.Helper()
	initial, issues := InitialGraph(requirement)
	if len(issues) != 0 {
		t.Fatalf("initial graph issues: %#v", issues)
	}
	hash, err := GraphHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	topology, err := TopologyHash(initial)
	if err != nil {
		t.Fatal(err)
	}
	byKey := primitiveInventoryByKey(inventory)
	state := topologySearchState{
		graph: initial, hash: hash, topology: topology,
		score: scoreTopologyGraph(requirement, initial, byKey, hash),
	}
	policy := protectedVoltageOutputSynthesisPolicy()
	return topologyProtectedVoltageRelationshipSeeds(
		context.Background(), requirement, inventory,
		topologyRepresentatives(requirement, inventory), byKey,
		GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes},
		policy, state,
	)
}

func protectedVoltageFeedbackRatio(graph CandidateGraph, output string) (float64, bool) {
	outputNode := externalRelationshipNode(graph, output)
	referenceNodes := topologyNodesByRole(graph, "reference")
	if outputNode == "" || len(referenceNodes) != 1 {
		return 0, false
	}
	for _, controller := range graph.Instances {
		if controller.Kind != "opamp" {
			continue
		}
		terminals := topologyTerminalNodes(controller)
		for _, feedback := range []string{terminals["IN_MINUS"], terminals["IN_PLUS"]} {
			upperConductance := 0.0
			lowerConductance := 0.0
			for _, instance := range graph.Instances {
				if instance.Kind != "resistor" || instance.ValueSI == nil {
					continue
				}
				nodes := topologyTerminalNodes(instance)
				if (nodes["A"] == feedback && nodes["B"] == outputNode) ||
					(nodes["A"] == outputNode && nodes["B"] == feedback) {
					upperConductance += 1 / *instance.ValueSI
				} else if (nodes["A"] == feedback && nodes["B"] == referenceNodes[0]) ||
					(nodes["A"] == referenceNodes[0] && nodes["B"] == feedback) {
					lowerConductance += 1 / *instance.ValueSI
				}
			}
			if upperConductance > 0 && lowerConductance > 0 {
				return 1 + lowerConductance/upperConductance, true
			}
		}
	}
	return 0, false
}

func protectedVoltageSourceSenseResistance(graph CandidateGraph, output string) (float64, bool) {
	outputNode := externalRelationshipNode(graph, output)
	for _, instance := range graph.Instances {
		if instance.Kind != "npn_bjt" {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		if nodes["EMITTER"] != outputNode {
			continue
		}
		path := topologyResistorPath(graph, nodes["BASE"], outputNode)
		if total, found := protectedVoltagePathResistance(graph, path); found {
			return total, true
		}
	}
	for _, instance := range graph.Instances {
		if instance.Kind != "npn_bjt" {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		path := topologyResistorPath(graph, nodes["EMITTER"], outputNode)
		if len(path) == 0 {
			continue
		}
		if total, found := protectedVoltagePathResistance(graph, path); found {
			return total, true
		}
	}
	return 0, false
}

func protectedVoltagePathResistance(graph CandidateGraph, path []string) (float64, bool) {
	total := 0.0
	seenBranches := map[string]bool{}
	for _, id := range path {
		for _, resistor := range graph.Instances {
			if resistor.ID != id || resistor.ValueSI == nil {
				continue
			}
			nodes := topologyTerminalNodes(resistor)
			left, right := nodes["A"], nodes["B"]
			if right < left {
				left, right = right, left
			}
			branch := left + "\x00" + right
			if seenBranches[branch] {
				break
			}
			seenBranches[branch] = true
			conductance := 0.0
			for _, parallel := range graph.Instances {
				if parallel.Kind != "resistor" || parallel.ValueSI == nil || *parallel.ValueSI <= 0 {
					continue
				}
				parallelNodes := topologyTerminalNodes(parallel)
				if (parallelNodes["A"] == left && parallelNodes["B"] == right) ||
					(parallelNodes["A"] == right && parallelNodes["B"] == left) {
					conductance += 1 / *parallel.ValueSI
				}
			}
			if conductance > 0 {
				total += 1 / conductance
			}
			break
		}
	}
	return total, total > 0
}

func protectedVoltageResistanceBetween(graph CandidateGraph, left, right string) (float64, bool) {
	for _, instance := range graph.Instances {
		if instance.Kind != "resistor" || instance.ValueSI == nil {
			continue
		}
		nodes := topologyTerminalNodes(instance)
		if (nodes["A"] == left && nodes["B"] == right) || (nodes["A"] == right && nodes["B"] == left) {
			return *instance.ValueSI, true
		}
	}
	return 0, false
}
