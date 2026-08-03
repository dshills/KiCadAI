package opentopologysynthesis

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestGenericGraphChangingRepairAddsMissingPassiveAndReplays(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	requirement.Requirements.BehavioralRequirements[0].Min = graphFloat(1000)
	requirement.Requirements.BehavioralRequirements[0].Max = graphFloat(10_000)
	graph = seriesPathOnly(t, graph)
	policy := DefaultPolicy()
	policy.MaxTopologyRepairs = 32
	policy.MaxValueTrials = 64
	policy.MaxCandidateSimulations = 256
	policy.MaxCornerEvaluations = 1024
	initial := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, policy)
	if initial.Status != SimulationEvaluationFailed || len(initial.Diagnoses) == 0 {
		t.Fatalf("unrepaired evaluation = status=%s diagnoses=%#v issues=%#v", initial.Status, initial.Diagnoses, initial.Issues)
	}
	first := RepairCandidate(context.Background(), requirement, graph, initial, inventory, environment, policy)
	if first.Status != RepairSearchPassed || first.Selected == nil || len(first.Attempts) == 0 {
		t.Fatalf("repair = status=%s attempts=%d consumption=%#v issues=%#v", first.Status, len(first.Attempts), first.Consumption, first.Issues)
	}
	hasAddedPrimitive := false
	for _, change := range first.Selected.Repair.Changes {
		hasAddedPrimitive = hasAddedPrimitive || change.Kind == "add_primitive"
	}
	if first.Selected.Repair.Operator != "add_passive_edge" || !hasAddedPrimitive ||
		first.Selected.Evaluation.Status != SimulationEvaluationPassed ||
		first.Selected.Repair.BeforeGraphHash == first.Selected.Repair.AfterGraphHash ||
		!repairGraphDeltaPreserved(graph, first.Selected.Graph, first.Selected.Repair) {
		t.Fatalf("selected repair lacks graph-changing evidence: %#v", first.Selected)
	}

	second := RepairCandidate(context.Background(), requirement, graph, initial, inventory, environment, policy)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repair replay differs:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestRepairDeltaCannotBeUndoneByValueEnumeration(t *testing.T) {
	_, graph, inventory, _ := testSimulationFixture(t)
	instance := graph.Instances[0]
	current, found := primitiveByKey(inventory, instance.PrimitiveKey)
	if !found {
		t.Fatal("fixture primitive absent from inventory")
	}
	replacement := ""
	for _, primitive := range inventory.Primitives {
		if primitive.Key != instance.PrimitiveKey &&
			primitive.Kind == instance.Kind &&
			samePrimitiveTerminalContract(current, primitive) {
			replacement = primitive.Key
			break
		}
	}
	if replacement == "" {
		t.Skip("fixture has no compatible replacement primitive")
	}
	repair := Repair{
		Operator: "substitute_compatible_primitive",
		Changes: []GraphChange{{
			Kind:      "substitute_primitive",
			Primitive: instance.ID,
			FromNode:  instance.PrimitiveKey,
			ToNode:    replacement,
		}},
	}
	if repairGraphDeltaPreserved(graph, graph, repair) {
		t.Fatal("unchanged graph incorrectly preserves substitution repair")
	}
	replaced, err := SubstitutePrimitive(
		graph,
		inventory,
		instance.ID,
		replacement,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !repairGraphDeltaPreserved(graph, replaced, repair) {
		t.Fatal("actual substituted graph did not preserve repair")
	}
}

func TestGenericRepairProposesDeterministicSeriesSplit(t *testing.T) {
	requirement, graph, inventory, _ := testSimulationFixture(t)
	assertion := requirement.Requirements.BehavioralRequirements[0]
	diagnoses := []Diagnosis{{
		Code: "assertion_below_minimum", RequirementID: assertion.ID,
		Analysis: assertion.Analysis, Direction: "below_minimum", EvidenceHash: "test-evidence",
	}}
	proposals := generateRepairProposals(requirement, graph, diagnoses, inventory, DefaultPolicy())
	for _, proposal := range proposals {
		if proposal.repair.Operator != "split_passive_edge" {
			continue
		}
		if len(proposal.repair.Changes) != 1 || proposal.repair.Changes[0].Kind != "split_primitive" {
			t.Fatalf("series split evidence = %#v", proposal.repair.Changes)
		}
		if !repairGraphDeltaPreserved(graph, proposal.graph, proposal.repair) {
			t.Fatal("series split graph delta was not preserved")
		}
		if len(proposal.graph.Nodes) != len(graph.Nodes)+1 || len(proposal.graph.Instances) != len(graph.Instances)+1 {
			t.Fatalf("series split dimensions = %d nodes, %d instances", len(proposal.graph.Nodes), len(proposal.graph.Instances))
		}
		return
	}
	t.Fatal("generic repair did not propose a series split")
}

func TestGenericRepairCancellationAndExhaustionAreStable(t *testing.T) {
	requirement, graph, inventory, environment := testSimulationFixture(t)
	graph = seriesPathOnly(t, graph)
	policy := DefaultPolicy()
	initial := EvaluateCandidate(context.Background(), requirement, graph, nil, inventory, environment, policy)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	canceled := RepairCandidate(ctx, requirement, graph, initial, inventory, environment, policy)
	if canceled.Status != RepairSearchCanceled || len(canceled.Issues) != 1 || canceled.Issues[0].Code != CodeCanceled {
		t.Fatalf("canceled repair = status=%s issues=%#v", canceled.Status, canceled.Issues)
	}

	tiny := policy
	tiny.MaxTopologyRepairs = 1
	tiny.MaxValueTrials = 1
	tiny.MaxCandidateSimulations = 1
	tiny.MaxCornerEvaluations = 1
	exhausted := RepairCandidate(context.Background(), requirement, graph, initial, inventory, environment, tiny)
	if exhausted.Status != RepairSearchExhausted || !exhausted.Consumption.BudgetExhausted ||
		len(exhausted.Issues) != 1 || exhausted.Issues[0].Code != CodeRepairExhausted {
		t.Fatalf("exhausted repair = status=%s consumption=%#v issues=%#v", exhausted.Status, exhausted.Consumption, exhausted.Issues)
	}
}

func seriesPathOnly(t *testing.T, graph CandidateGraph) CandidateGraph {
	t.Helper()
	resistorKey := ""
	for _, instance := range append([]GraphInstance(nil), graph.Instances...) {
		if instance.Kind == "resistor" {
			resistorKey = instance.PrimitiveKey
			continue
		}
		var err error
		graph, err = RemovePrimitive(graph, instance.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if resistorKey == "" {
		t.Fatal("simulation fixture has no resistor")
	}
	primitive := PrimitiveCandidate{}
	for _, instance := range graph.Instances {
		if instance.PrimitiveKey == resistorKey {
			primitive = PrimitiveCandidate{
				Key: resistorKey, Kind: instance.Kind,
				Terminals: []PrimitiveTerminal{{Terminal: "A"}, {Terminal: "B"}},
			}
			break
		}
	}
	graph = AddPrimitive(graph, primitive, graphFloat(10_000), []TerminalConnection{
		{Terminal: "A", Node: "port_output"},
		{Terminal: "B", Node: "port_ground"},
	})
	return graph
}
