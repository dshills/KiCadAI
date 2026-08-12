package opentopologysynthesis

import (
	"context"
	"reflect"
	"testing"
)

func TestCoordinatedTopologyValueCandidatesAreBoundedAndDeterministic(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	requirement.Requirements.BehavioralRequirements[0].Min = graphFloat(1000)
	requirement.Requirements.BehavioralRequirements[0].Max = graphFloat(10_000)
	graph = seriesPathOnly(t, graph)
	policy := DefaultPolicy()
	initial := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, policy)
	generated := generateCausalCandidates(requirement, graph, initial, inventory, policy)
	evaluated := []causalEvaluatedCandidate{}
	for _, candidate := range generated {
		if len(candidate.perturbations) != 1 {
			continue
		}
		usesTopology := causalChangesUseTopology(candidate.repair.Changes)
		kind := candidate.perturbations[0].Kind
		usesValue := kind == "adjust_value" || kind == "substitute_rated_device"
		if !usesTopology && !usesValue {
			continue
		}
		evaluated = append(evaluated, causalEvaluatedCandidate{
			graph: candidate.graph,
			trial: CausalRepairTrial{
				Hash: candidate.repair.AfterGraphHash, Improvement: 1,
				Authorized: true, Perturbations: candidate.perturbations, Repair: candidate.repair,
			},
		})
	}
	first := coordinatedTopologyValueCandidates(graph, evaluated, 4)
	second := coordinatedTopologyValueCandidates(graph, evaluated, 4)
	if len(first) == 0 || len(first) > 4 || !reflect.DeepEqual(first, second) {
		t.Fatalf("coordinated topology/value candidates first=%#v second=%#v", first, second)
	}
	limits := GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes}
	for _, candidate := range first {
		if !candidate.coordinated || len(candidate.perturbations) != 2 || len(candidate.repair.Changes) > causalMaximumChanges {
			t.Fatalf("unbounded coordinated candidate = %#v", candidate)
		}
		if issues := ValidatePartialGraph(candidate.graph, inventory, limits); len(issues) != 0 {
			t.Fatalf("coordinated graph issues = %#v", issues)
		}
	}
}

func TestCausalRepairProposalCostPrefersFewerChangesAndComponents(t *testing.T) {
	oneValue := Repair{Changes: []GraphChange{{Kind: "set_value"}}}
	oneEdge := Repair{Changes: []GraphChange{{Kind: "add_primitive"}}}
	twoValues := Repair{Changes: []GraphChange{{Kind: "set_value"}, {Kind: "set_value"}}}
	if !(causalRepairProposalCost(oneValue) < causalRepairProposalCost(oneEdge) &&
		causalRepairProposalCost(oneEdge) < causalRepairProposalCost(twoValues)) {
		t.Fatalf("repair costs value=%d edge=%d pair=%d", causalRepairProposalCost(oneValue), causalRepairProposalCost(oneEdge), causalRepairProposalCost(twoValues))
	}
}
