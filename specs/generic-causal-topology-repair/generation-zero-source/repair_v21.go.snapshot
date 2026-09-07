package opentopologysynthesis

import (
	"context"
	"slices"

	"kicadai/internal/reports"
	"kicadai/internal/simulationadmission"
)

func RepairCandidateV21(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.Environment,
	policy Policy,
) RepairSearchResult {
	return repairCandidatePreparedV21(ctx, requirement, graph, initial, inventory, environment, simulationadmission.PrepareEnvironment(admission), policy)
}

func repairCandidatePreparedV21(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.PreparedEnvironment,
	policy Policy,
) RepairSearchResult {
	policy = effectiveTopologyPolicy(policy)
	result := RepairSearchResult{
		Schema: RepairSearchSchema, Version: RepairSearchVersion, PolicyVersion: PolicyVersion,
		InventoryHash: inventory.Hash, InitialEvaluationHash: initial.Hash, Status: RepairSearchFailed,
		Policy: policy, CausalAnalyses: []CausalRepairAnalysis{}, Attempts: []RepairAttempt{}, Issues: []reports.Issue{},
	}
	requirement = Normalize(requirement)
	var err error
	result.RequirementHash, err = CanonicalHash(requirement)
	if err != nil {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeTopologyInvalidRepairV21, "repair.v21.requirement", err.Error(), "supply a canonical requirement")}
		return finalizeRepairSearchV17(result)
	}
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeTopologyInvalidRepairV21, "repair.v21.graph", err.Error(), "supply a canonical candidate graph")}
		return finalizeRepairSearchV17(result)
	}
	result.InitialGraphHash, err = GraphHash(graph)
	if err != nil {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeTopologyInvalidRepairV21, "repair.v21.graph_hash", err.Error(), "supply a canonical candidate graph")}
		return finalizeRepairSearchV17(result)
	}
	if initial.GraphHash != result.InitialGraphHash || initial.RequirementHash != result.RequirementHash ||
		initial.InventoryHash != inventory.Hash || initial.Hash == "" {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "repair.v21.initial_evaluation", "V21 repair requires a hash-bound evaluation of the exact graph, requirement, and inventory", "evaluate the exact selected graph before topology completion")}
		return finalizeRepairSearchV17(result)
	}
	if initial.Status == SimulationEvaluationPassed {
		result.Status = RepairSearchPassed
		result.Selected = &RepairedCandidate{Graph: graph, Repairs: []Repair{}, Evaluation: initial}
		return finalizeRepairSearchV17(result)
	}
	result.traceDiagnoses = slices.Clone(initial.Diagnoses)
	limits := DefaultTopologyCompletionLimitsV21()
	limits.MaximumWork = min(limits.MaximumWork, max(1, policy.MaxGeneratedGraphs))
	limits.MaximumRetained = min(limits.MaximumRetained, max(1, policy.MaxRetainedCandidates))
	plan := PlanTopologyCompletionV21(ctx, requirement, graph, inventory, limits)
	result.TopologyCompletionV21 = &plan
	result.Consumption.ExpandedStates = plan.Consumption.ExpandedStates
	result.Consumption.GeneratedGraphs = plan.Consumption.GeneratedCandidates
	result.Consumption.TopologyRepairs = plan.Consumption.WorkConsumed
	result.Consumption.MaximumFrontier = plan.Consumption.MaximumFrontier
	result.Consumption.BudgetExhausted = plan.Consumption.BudgetExhausted
	if ctx.Err() != nil {
		result.Status = RepairSearchCanceled
		result.Issues = []reports.Issue{graphIssue(CodeCanceled, "repair.v21", "V21 topology completion canceled", "retry with an active context")}
		return finalizeRepairSearchV17(result)
	}
	if plan.Status != "complete" || plan.Selected == nil {
		if plan.Status == "exhausted" {
			result.Status = RepairSearchExhausted
		} else {
			result.Status = RepairSearchUnsupported
		}
		result.Issues = slices.Clone(plan.Issues)
		return finalizeRepairSearchV17(result)
	}
	if len(plan.Selected.Operations) == 0 {
		result.Status = RepairSearchFailed
		result.Issues = []reports.Issue{graphIssue(CodeTopologyCertifiedV21, "repair.v21.invariants", "the selected candidate already satisfies the V21 causal topology invariants", "continue with typed model, solver, value, or behavioral evidence")}
		return finalizeRepairSearchV17(result)
	}

	remaining := policy
	remaining.MaxCandidateSimulations = max(0, policy.MaxCandidateSimulations-result.Consumption.CandidateSimulations)
	remaining.MaxCornerEvaluations = max(0, policy.MaxCornerEvaluations-result.Consumption.CornerEvaluations)
	evaluation := evaluateCandidatePreparedV20(ctx, requirement, plan.Selected.Graph, nil, inventory, environment, admission, remaining)
	result.Consumption.CandidateSimulations += evaluation.Consumption.CandidateSimulations
	result.Consumption.CornerEvaluations += evaluation.Consumption.CornerEvaluations
	repair := topologyRepairEvidenceV21(initial, *plan.Selected)
	topologyHash, err := TopologyHash(plan.Selected.Graph)
	if err != nil {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeTopologyInvalidRepairV21, "repair.v21.topology_hash", err.Error(), "supply a canonical candidate graph")}
		return finalizeRepairSearchV17(result)
	}
	attempt := RepairAttempt{
		Number: 1, Repair: repair, GraphHash: plan.Selected.GraphHash, TopologyHash: topologyHash,
		Evaluation: evaluation, Improved: plan.Selected.Invariant.Complete, Status: statusForV21RepairEvaluation(evaluation.Status),
	}
	result.Attempts = []RepairAttempt{attempt}
	switch evaluation.Status {
	case SimulationEvaluationPassed:
		result.Status = RepairSearchPassed
		result.Selected = &RepairedCandidate{Graph: plan.Selected.Graph, Repair: repair, Repairs: []Repair{repair}, Evaluation: evaluation}
	case SimulationEvaluationCanceled:
		result.Status = RepairSearchCanceled
		result.Issues = slices.Clone(evaluation.Issues)
	case SimulationEvaluationUnsupported:
		result.Status = RepairSearchUnsupported
		result.Issues = slices.Clone(evaluation.Issues)
	case SimulationEvaluationExhausted:
		result.Status = RepairSearchExhausted
		result.Consumption.BudgetExhausted = true
		result.Issues = slices.Clone(evaluation.Issues)
	default:
		result.Status = RepairSearchFailed
		result.Issues = []reports.Issue{graphIssue(CodeTopologyCertifiedV21, "repair.v21.invariants", "V21 completed the causal topology but admitted behavioral evaluation still failed", "continue with the typed downstream diagnosis")}
	}
	return finalizeRepairSearchV17(result)
}

func topologyRepairEvidenceV21(initial SimulationEvaluation, selected TopologyCandidateEvidenceV21) Repair {
	repair := Repair{Number: 1, Operator: "topology_completion_v21", BeforeGraphHash: initial.GraphHash, AfterGraphHash: selected.GraphHash, Changes: []GraphChange{}}
	if len(initial.Diagnoses) != 0 {
		diagnosis := initial.Diagnoses[0]
		repair.DiagnosisCode = diagnosis.Code
		repair.DiagnosisRequirementID = diagnosis.RequirementID
		repair.DiagnosisEvidenceHash = diagnosis.EvidenceHash
		repair.ExpectedDirection = diagnosis.Direction
	}
	for _, operation := range selected.Operations {
		change := GraphChange{Kind: operation.Kind, Primitive: operation.PrimitiveKey, Terminal: operation.Terminal}
		if operation.Kind == TopologyOperationRedirectTerminalV21 || operation.Kind == TopologyOperationJoinPathsV21 {
			change.FromNode, change.ToNode = operation.Obligation.FromNode, operation.Obligation.ToNode
		}
		repair.Changes = append(repair.Changes, change)
	}
	return repair
}

func statusForV21RepairEvaluation(status SimulationEvaluationStatus) RepairSearchStatus {
	switch status {
	case SimulationEvaluationPassed:
		return RepairSearchPassed
	case SimulationEvaluationCanceled:
		return RepairSearchCanceled
	case SimulationEvaluationUnsupported:
		return RepairSearchUnsupported
	case SimulationEvaluationExhausted:
		return RepairSearchExhausted
	default:
		return RepairSearchFailed
	}
}
