package opentopologysynthesis

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"kicadai/internal/reports"
)

func TestV19BeamComposesThreeIndependentProposals(t *testing.T) {
	requirement, graph, inventory, initial := causalV19BeamFixture(t)
	hooks := causalV19BeamHooks(t, requirement, graph, inventory, 4, 3)
	result := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), hooks)
	if result.Status != RepairSearchPassed || result.Selected == nil {
		t.Fatalf("three-change beam did not pass: status=%s attempts=%d issues=%#v", result.Status, len(result.Attempts), result.Issues)
	}
	if len(result.Selected.Repairs) != 3 {
		t.Fatalf("selected repair path has %d proposals, want 3", len(result.Selected.Repairs))
	}
	for _, repair := range result.Selected.Repairs {
		if repair.BeforeGraphHash == "" || repair.AfterGraphHash == "" || repair.BeforeGraphHash == repair.AfterGraphHash {
			t.Fatalf("non-replayable repair in selected path: %#v", repair)
		}
	}
	if result.Consumption.MaximumFrontier > causalMaximumBeamWidthV19 || result.Consumption.TopologyRepairs > DefaultPolicy().MaxTopologyRepairs {
		t.Fatalf("beam escaped frozen limits: %#v", result.Consumption)
	}
}

func TestV19BeamEvaluatesAtMostFortyEightAndChargesGeneratedDuplicates(t *testing.T) {
	requirement, graph, inventory, initial := causalV19BeamFixture(t)
	hooks := causalV19BeamHooks(t, requirement, graph, inventory, 64, 99)
	result := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), hooks)
	if got := len(result.Attempts); got != causalMaximumEvaluatedCandidatesV19 {
		t.Fatalf("evaluated %d candidates, want %d", got, causalMaximumEvaluatedCandidatesV19)
	}
	if result.Consumption.GeneratedGraphs <= result.Consumption.CandidateSimulations || result.Consumption.GeneratedGraphs < 64 {
		t.Fatalf("generated candidates were not charged independently: %#v", result.Consumption)
	}
	if result.Consumption.MaximumFrontier > causalMaximumBeamWidthV19 {
		t.Fatalf("maximum frontier %d exceeds width %d", result.Consumption.MaximumFrontier, causalMaximumBeamWidthV19)
	}
}

func TestV19BeamFinishesPassingDepthSlice(t *testing.T) {
	requirement, graph, inventory, initial := causalV19BeamFixture(t)
	hooks := causalV19BeamHooks(t, requirement, graph, inventory, 16, 1)
	result := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), hooks)
	if result.Status != RepairSearchPassed {
		t.Fatalf("passing slice status = %s", result.Status)
	}
	if got := len(result.Attempts); got != causalBaseDepthQuotaV19 {
		t.Fatalf("passing depth stopped after %d attempts, want completed slice of %d", got, causalBaseDepthQuotaV19)
	}
}

func TestV19BeamRejectsDepthFiveAndThreeChangeProposal(t *testing.T) {
	requirement, graph, inventory, initial := causalV19BeamFixture(t)
	hooks := causalV19BeamHooks(t, requirement, graph, inventory, 1, 5)
	result := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), hooks)
	if result.Status == RepairSearchPassed || result.Selected != nil {
		t.Fatalf("depth-five pass escaped the depth-four bound: %#v", result.Selected)
	}
	if len(result.Attempts) != causalMaximumDepthV19 {
		t.Fatalf("depth bound evaluated %d attempts, want %d", len(result.Attempts), causalMaximumDepthV19)
	}

	badHooks := causalV19BeamHooks(t, requirement, graph, inventory, 1, 1)
	basePropose := badHooks.propose
	badHooks.propose = func(state causalBeamStateV19, budget CausalOperationBudgetV19) (CausalOperationBatchV19, error) {
		batch, err := basePropose(state, budget)
		if len(batch.Proposals) != 0 {
			proposal := batch.Proposals[0]
			proposal.Operations = append(proposal.Operations, proposal.Operations[0], proposal.Operations[0])
			for index := range proposal.Operations {
				proposal.Operations[index].Number = index + 1
			}
			proposal.LogicalChanges = 3
			proposal.CanonicalKey = causalProposalKeyV19(proposal)
			batch.Proposals = []CausalOperationProposalV19{proposal}
		}
		return batch, err
	}
	bad := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), badHooks)
	if len(bad.Attempts) != 0 || bad.Status != RepairSearchUnsupported {
		t.Fatalf("three-change proposal was not rejected: status=%s attempts=%d", bad.Status, len(bad.Attempts))
	}
}

func TestV19BeamFailsClosedBeforeSimulationAtGraphLimit(t *testing.T) {
	requirement, graph, inventory, initial := causalV19BeamFixture(t)
	hooks := causalV19BeamHooks(t, requirement, graph, inventory, 4, 1)
	evaluations := 0
	hooks.evaluate = func(context.Context, CandidateGraph, Policy) SimulationEvaluation {
		evaluations++
		return SimulationEvaluation{}
	}
	hooks.validate = func(candidate CandidateGraph, _ CausalInvariantContextV19) []reports.Issue {
		if len(candidate.Instances) > DefaultPolicy().MaxPrimitiveInstances || internalNodeCount(candidate) > DefaultPolicy().MaxInternalNodes {
			return []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "maximum graph size exceeded", "")}
		}
		return nil
	}
	hooks.propose = func(state causalBeamStateV19, _ CausalOperationBudgetV19) (CausalOperationBatchV19, error) {
		candidate := CloneGraph(state.graph)
		for internalNodeCount(candidate) <= DefaultPolicy().MaxInternalNodes {
			candidate = AddInternalNode(candidate, "limit")
		}
		proposal := causalV19BeamProposal(t, state, candidate, 0)
		return CausalOperationBatchV19{Proposals: []CausalOperationProposalV19{proposal}, Consumption: CausalOperationConsumptionV19{GeneratedGraphs: 1, TopologyRepairs: 1}}, nil
	}
	result := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), hooks)
	if evaluations != 0 || len(result.Attempts) != 0 || result.Status != RepairSearchUnsupported {
		t.Fatalf("oversize graph reached simulation: eval=%d status=%s attempts=%d", evaluations, result.Status, len(result.Attempts))
	}
}

func TestV19BeamPermutationReplayIsByteStable(t *testing.T) {
	requirement, graph, inventory, initial := causalV19BeamFixture(t)
	baselineHooks := causalV19BeamHooks(t, requirement, graph, inventory, 12, 3)
	baseline := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), baselineHooks)
	want, err := json.Marshal(baseline)
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 256; iteration++ {
		hooks := causalV19BeamHooks(t, requirement, graph, inventory, 12, 3)
		basePropose := hooks.propose
		random := rand.New(rand.NewSource(19019 + int64(iteration)))
		hooks.propose = func(state causalBeamStateV19, budget CausalOperationBudgetV19) (CausalOperationBatchV19, error) {
			batch, proposeErr := basePropose(state, budget)
			random.Shuffle(len(batch.Proposals), func(i, j int) { batch.Proposals[i], batch.Proposals[j] = batch.Proposals[j], batch.Proposals[i] })
			return batch, proposeErr
		}
		gotResult := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), hooks)
		got, marshalErr := json.Marshal(gotResult)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if string(got) != string(want) {
			t.Fatalf("permutation %d changed beam bytes", iteration)
		}
	}
}

func TestV19BeamAdmitsAtMostTwoStructuralPlateausPerParent(t *testing.T) {
	requirement, graph, inventory, initial := causalV19BeamFixture(t)
	hooks := causalV19BeamHooks(t, requirement, graph, inventory, 8, 99)
	basePropose := hooks.propose
	hooks.propose = func(state causalBeamStateV19, budget CausalOperationBudgetV19) (CausalOperationBatchV19, error) {
		if state.depth != 0 {
			return CausalOperationBatchV19{}, nil
		}
		return basePropose(state, budget)
	}
	hooks.evaluate = func(_ context.Context, candidate CandidateGraph, _ Policy) SimulationEvaluation {
		return causalV19BeamEvaluation(t, requirement, inventory, candidate, 10, false)
	}
	result := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), hooks)
	if result.Consumption.MaximumFrontier > causalMaximumPlateauPerParentV19 {
		t.Fatalf("plateau frontier = %d, want <= %d", result.Consumption.MaximumFrontier, causalMaximumPlateauPerParentV19)
	}
}

func causalV19BeamFixture(t *testing.T) (Requirement, CandidateGraph, PrimitiveInventory, SimulationEvaluation) {
	t.Helper()
	requirement := causalV19Requirement("voltage_gain", "ac_sweep")
	graph := causalV19FeedForwardGraph(t, requirement)
	inventory := causalV19Inventory(t)
	initial := causalV19BeamEvaluation(t, requirement, inventory, graph, 10, false)
	return requirement, graph, inventory, initial
}

func causalV19BeamHooks(t *testing.T, requirement Requirement, base CandidateGraph, inventory PrimitiveInventory, fanout, passDepth int) causalBeamHooksV19 {
	t.Helper()
	baseNodes := len(base.Nodes)
	return causalBeamHooksV19{
		validate: func(CandidateGraph, CausalInvariantContextV19) []reports.Issue { return nil },
		structural: func(candidate CandidateGraph, _ CausalInvariantContextV19) CausalStructuralVectorV19 {
			depth := max(0, len(candidate.Nodes)-baseNodes)
			return CausalStructuralVectorV19{UnreachableObservations: max(0, passDepth-depth)}
		},
		propose: func(state causalBeamStateV19, budget CausalOperationBudgetV19) (CausalOperationBatchV19, error) {
			count := min(fanout, budget.GeneratedGraphs, budget.TopologyRepairs)
			batch := CausalOperationBatchV19{Proposals: []CausalOperationProposalV19{}, Consumption: CausalOperationConsumptionV19{GeneratedGraphs: count, TopologyRepairs: count}}
			for index := 0; index < count; index++ {
				candidate := AddInternalNode(state.graph, fmt.Sprintf("beam_%02d_%03d", state.depth+1, index))
				normalized, err := NormalizeGraph(candidate)
				if err != nil {
					t.Fatal(err)
				}
				batch.Proposals = append(batch.Proposals, causalV19BeamProposal(t, state, normalized, index))
			}
			return batch, nil
		},
		evaluate: func(_ context.Context, candidate CandidateGraph, _ Policy) SimulationEvaluation {
			depth := max(0, len(candidate.Nodes)-baseNodes)
			actual := float64(max(0, passDepth-depth))
			return causalV19BeamEvaluation(t, requirement, inventory, candidate, actual, depth >= passDepth)
		},
	}
}

func causalV19BeamProposal(t *testing.T, state causalBeamStateV19, graph CandidateGraph, index int) CausalOperationProposalV19 {
	t.Helper()
	beforeHash, err := GraphHash(state.graph)
	if err != nil {
		t.Fatal(err)
	}
	afterHash, err := GraphHash(graph)
	if err != nil {
		t.Fatal(err)
	}
	operation := CausalLogicalOperationV19{
		GraphOperation: GraphOperation{
			Number: 1, Kind: CausalOperationInsertRoleCompleteStageV19,
			PrimitiveKey: "buffer", PrimitiveKind: "opamp",
			Connections: []TerminalConnection{{Terminal: "OUT", Node: "port_signal_out"}},
			BeforeHash:  beforeHash, AfterHash: afterHash,
		},
		ObligationID: "transfer_bound", ObservationID: "signal_out",
		UpstreamNode: "port_signal_in", InstanceID: fmt.Sprintf("beam_instance_%03d", index),
		CreatedNodes: []string{}, CanonicalCost: causalOperationLogicalChangeCostWeightV19,
	}
	proposal := CausalOperationProposalV19{
		PlannerKind: CausalOperationInsertRoleCompleteStageV19,
		Graph:       graph, Context: state.context, Operations: []CausalLogicalOperationV19{operation}, LogicalChanges: 1,
	}
	proposal.CanonicalKey = causalProposalKeyV19(proposal)
	return proposal
}

func causalV19BeamEvaluation(t *testing.T, requirement Requirement, inventory PrimitiveInventory, graph CandidateGraph, actual float64, pass bool) SimulationEvaluation {
	t.Helper()
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		t.Fatal(err)
	}
	graphHash, err := GraphHash(graph)
	if err != nil {
		t.Fatal(err)
	}
	maximum := 0.0
	status := SimulationEvaluationFailed
	diagnoses := []Diagnosis{{Code: "assertion_above_maximum", RequirementID: "transfer_bound", OperatingCase: "bounded", Analysis: "ac_sweep", Metric: "voltage_gain", Direction: "decrease", Actual: causalV19Float(actual), RequiredMax: &maximum, EvidenceHash: stringsOfSHA256V19(1)[0]}}
	if pass {
		status = SimulationEvaluationPassed
		diagnoses = []Diagnosis{}
	}
	evaluation := SimulationEvaluation{
		Schema: SimulationEvaluationSchema, Version: SimulationEvaluationVersion,
		PolicyVersion: PolicyVersion, RequirementHash: requirementHash, InventoryHash: inventory.Hash,
		GraphHash: graphHash, Status: status, Policy: DefaultPolicy(),
		Consumption: Consumption{CandidateSimulations: 1, CornerEvaluations: 1},
		Attempts: []SimulationAttempt{{
			Number: 1, RequirementID: "transfer_bound", OperatingCase: "bounded", CornerID: "nominal",
			Analysis: "ac_sweep", Metric: "voltage_gain", WorkflowModel: "beam-test",
			PlanHash: stringsOfSHA256V19(2)[0], ModelEvidenceSHA256s: stringsOfSHA256V19(3),
			Status: status, Actual: causalV19Float(actual), RequiredMax: &maximum, AssertionPass: pass,
		}},
		Diagnoses: diagnoses, Issues: []reports.Issue{},
	}
	return finalizeSimulationEvaluationV17(evaluation)
}

func stringsOfSHA256V19(seed int) []string {
	value := fmt.Sprintf("%064x", seed)
	return []string{value}
}

func TestV19BeamCanonicalOrderDoesNotDependOnProposalInputOrder(t *testing.T) {
	requirement, graph, inventory, initial := causalV19BeamFixture(t)
	hooks := causalV19BeamHooks(t, requirement, graph, inventory, 8, 2)
	basePropose := hooks.propose
	hooks.propose = func(state causalBeamStateV19, budget CausalOperationBudgetV19) (CausalOperationBatchV19, error) {
		batch, err := basePropose(state, budget)
		slices.Reverse(batch.Proposals)
		return batch, err
	}
	result := repairCandidateV19WithHooks(context.Background(), requirement, graph, initial, inventory, DefaultPolicy(), hooks)
	if result.Status != RepairSearchPassed || result.Selected == nil {
		t.Fatalf("reversed proposal input changed admissibility: %s", result.Status)
	}
}
