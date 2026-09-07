package opentopologysynthesis

import (
	"context"
	"testing"

	"kicadai/internal/simulationadmission"
)

func TestTopologyReviewInitialGraphRespectsMemoryLimit(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	graph := causalV19FeedForwardGraph(t, requirement)
	limits := DefaultTopologyCompletionLimitsV21()
	limits.MaximumGraphBytes = 1
	plan := PlanTopologyCompletionV21(context.Background(), requirement, graph, causalV19Inventory(t), limits)
	if plan.Selected != nil || plan.Status == "complete" || !plan.Consumption.BudgetExhausted {
		t.Fatalf("oversized initial graph admitted: status=%s selected=%t consumption=%+v", plan.Status, plan.Selected != nil, plan.Consumption)
	}
}

func TestTopologyReviewRejectsUnsoundStructuralCertificates(t *testing.T) {
	for _, name := range []string{"contending outputs", "untyped feedback", "wrong port domain", "shorted active supply", "floating reference"} {
		t.Run(name, func(t *testing.T) {
			requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
			graph := causalV19FeedForwardGraph(t, requirement)
			switch name {
			case "contending outputs":
				second := graph.Instances[0]
				second.ID = "another_driver"
				graph.Instances = append(graph.Instances, second)
			case "untyped feedback":
				graph = causalV19FeedbackGraph(t, requirement)
			case "wrong port domain":
				for i := range graph.Nodes {
					if graph.Nodes[i].SemanticID == "signal_out" {
						graph.Nodes[i].Domain = "supply"
					}
				}
			case "shorted active supply":
				for i := range graph.Instances[0].Terminals {
					if graph.Instances[0].Terminals[i].Terminal == "V_MINUS" {
						graph.Instances[0].Terminals[i].Node = "port_vcc"
					}
				}
			case "floating reference":
				graph = causalV19FloatingBufferGraph(t, requirement)
			}
			report := AnalyzeTopologyV21(requirement, graph, causalV19Inventory(t))
			if report.Complete {
				t.Fatal("invalid electrical structure received a completion certificate")
			}
			if !report.Contradictory || len(report.Issues) == 0 {
				t.Fatal("structural rejection must preserve its typed diagnostics")
			}
		})
	}
}

func TestTopologyReviewRepairAuthenticatesInitialEvaluation(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	inventory := causalV19Inventory(t)
	graph := causalV19FeedForwardGraph(t, requirement)
	initial := causalV19BeamEvaluation(t, requirement, inventory, graph, 0, false)
	initial.Status = SimulationEvaluationPassed // Keep the original, now stale digest.
	result := RepairCandidateV21(context.Background(), requirement, graph, initial, inventory, SimulationEnvironment{}, simulationadmission.Environment{}, DefaultPolicy())
	if result.Status != RepairSearchUnsupported || result.Selected != nil {
		t.Fatalf("tampered evaluation admitted: %s", result.Status)
	}
}

func TestTopologyReviewCancellationPrecedesPassedEvaluation(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	inventory := causalV19Inventory(t)
	graph := causalV19FeedForwardGraph(t, requirement)
	initial := causalV19BeamEvaluation(t, requirement, inventory, graph, 0, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := RepairCandidateV21(ctx, requirement, graph, initial, inventory, SimulationEnvironment{}, simulationadmission.Environment{}, DefaultPolicy())
	if result.Status != RepairSearchCanceled || result.Selected != nil {
		t.Fatalf("canceled repair returned a selection: %s", result.Status)
	}
}

func TestTopologyReviewRetainedLimitAppliesBeforeSelection(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	graph, _ := InitialGraph(requirement)
	limits := DefaultTopologyCompletionLimitsV21()
	limits.MaximumRetained = 1 // The initial state consumes the only slot.
	plan := PlanTopologyCompletionV21(context.Background(), requirement, graph, causalV19Inventory(t), limits)
	if plan.Selected != nil || plan.Status != "exhausted" || !plan.Consumption.BudgetExhausted || plan.Consumption.MaximumRetained > 1 {
		t.Fatalf("retained-state limit bypassed: status=%s selected=%t consumption=%+v", plan.Status, plan.Selected != nil, plan.Consumption)
	}
}

func TestTopologyReviewStateHashesAuthenticateFinalOperations(t *testing.T) {
	requirement := causalV19Requirement("voltage_gain", "dc_operating_point")
	graph, _ := InitialGraph(requirement)
	plan := PlanTopologyCompletionV21(context.Background(), requirement, graph, causalV19Inventory(t), DefaultTopologyCompletionLimitsV21())
	if plan.Selected == nil {
		t.Fatal("expected a completed graph to inspect")
	}
	candidates := append(plan.Candidates, *plan.Selected)
	for _, candidate := range candidates {
		want := topologyStateHashV21(topologyStateV21{
			graphHash: candidate.GraphHash, depth: candidate.Depth,
			operations: candidate.Operations, report: candidate.Invariant,
		})
		if candidate.StateHash != want {
			t.Fatalf("state hash does not authenticate published operation: got %s want %s", candidate.StateHash, want)
		}
	}
}

func TestTopologyReviewCannotTradeCriticalObligations(t *testing.T) {
	first := topologyObligationV21(TopologyObligationCausalPathV21, "first", "a", "in", "a", "signal", "", true)
	second := topologyObligationV21(TopologyObligationCausalPathV21, "second", "b", "in", "b", "signal", "", true)
	newFailure := topologyObligationV21(TopologyObligationCausalPathV21, "third", "c", "in", "c", "signal", "", true)
	parent := topologyStateV21{report: TopologyInvariantReportV21{Obligations: []TopologyObligationV21{first, second}}}
	child := topologyStateV21{report: TopologyInvariantReportV21{Obligations: []TopologyObligationV21{newFailure}}}
	if topologyStrictlyImprovesV21(parent, child) {
		t.Fatal("repair may not introduce a previously satisfied critical failure even if the total count falls")
	}
	child.report.Obligations = []TopologyObligationV21{first}
	if !topologyStrictlyImprovesV21(parent, child) {
		t.Fatal("repair resolving a critical obligation without introducing another must remain admissible")
	}
}
