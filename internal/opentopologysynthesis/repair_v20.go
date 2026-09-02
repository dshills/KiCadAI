package opentopologysynthesis

import (
	"cmp"
	"context"
	"slices"

	"kicadai/internal/reports"
	"kicadai/internal/simulationadmission"
)

func RepairCandidateV20(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.Environment,
	policy Policy,
) RepairSearchResult {
	return repairCandidatePreparedV20(
		ctx, requirement, graph, initial, inventory, environment,
		simulationadmission.PrepareEnvironment(admission), policy,
	)
}

func repairCandidatePreparedV20(
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
		Schema:                RepairSearchSchema,
		Version:               RepairSearchVersion,
		PolicyVersion:         PolicyVersion,
		InventoryHash:         inventory.Hash,
		InitialEvaluationHash: initial.Hash,
		Status:                RepairSearchFailed,
		Policy:                policy,
		CausalAnalyses:        []CausalRepairAnalysis{},
		Attempts:              []RepairAttempt{},
		Issues:                []reports.Issue{},
	}
	requirement = Normalize(requirement)
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash repair requirement: "+err.Error(), "")}
		return finalizeRepairSearch(result)
	}
	result.RequirementHash = requirementHash
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "normalize repair graph: "+err.Error(), "")}
		return finalizeRepairSearch(result)
	}
	result.InitialGraphHash, err = GraphHash(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "hash repair graph: "+err.Error(), "")}
		return finalizeRepairSearch(result)
	}
	if initial.GraphHash != result.InitialGraphHash || initial.RequirementHash != requirementHash ||
		initial.InventoryHash != inventory.Hash || initial.Hash == "" {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "initial_evaluation", "repair requires a hash-bound evaluation of the supplied graph, requirement, and inventory", "evaluate the exact candidate before repair")}
		return finalizeRepairSearch(result)
	}
	if initial.Status == SimulationEvaluationPassed {
		result.Status = RepairSearchPassed
		result.Selected = &RepairedCandidate{Graph: graph, Repairs: []Repair{}, Evaluation: initial}
		return finalizeRepairSearch(result)
	}
	if len(initial.Diagnoses) == 0 {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "initial_evaluation.diagnoses", "failed candidate has no stable diagnosis for generic repair", "retain normalized simulation diagnoses")}
		return finalizeRepairSearch(result)
	}
	result.traceDiagnoses = append([]Diagnosis(nil), initial.Diagnoses...)

	type repairState struct {
		graph      CandidateGraph
		evaluation SimulationEvaluation
		repairs    []Repair
		penalty    float64
		hash       string
	}
	frontier := []repairState{{
		graph: graph, evaluation: initial, repairs: []Repair{},
		penalty: simulationEvaluationPenalty(initial), hash: result.InitialGraphHash,
	}}
	seenGraphs := map[string]struct{}{result.InitialGraphHash: {}}
	generatedProposal := false
	for len(frontier) != 0 {
		slices.SortFunc(frontier, func(left, right repairState) int {
			return cmp.Or(cmp.Compare(left.penalty, right.penalty), cmp.Compare(left.hash, right.hash))
		})
		state := frontier[0]
		frontier = frontier[1:]
		causalPolicy := policy
		causalPolicy.MaxCandidateSimulations = max(0, policy.MaxCandidateSimulations-result.Consumption.CandidateSimulations)
		causalPolicy.MaxCornerEvaluations = max(0, policy.MaxCornerEvaluations-result.Consumption.CornerEvaluations)
		causalPolicy.MaxValueTrials = max(0, policy.MaxValueTrials-result.Consumption.ValueTrials)
		causalPolicy.MaxTopologyRepairs = max(0, policy.MaxTopologyRepairs-result.Consumption.TopologyRepairs)
		analysis, causalCandidates := analyzeCausalRepairsV20(
			ctx, requirement, state.graph, state.evaluation,
			inventory, environment, admission, causalPolicy,
		)
		result.CausalAnalyses = append(result.CausalAnalyses, analysis)
		if analysis.Status == "canceled" {
			result.Status = RepairSearchCanceled
			result.Issues = []reports.Issue{graphIssue(CodeCanceled, "repair", "open-topology repair canceled", "retry with an active context")}
			return finalizeRepairSearch(result)
		}
		result.Consumption.CandidateSimulations += analysis.Consumption.CandidateSimulations
		result.Consumption.CornerEvaluations += analysis.Consumption.CornerEvaluations
		result.Consumption.ValueTrials += analysis.Consumption.ValueTrials
		result.Consumption.TopologyRepairs += analysis.Consumption.TopologyTrials
		if result.Consumption.CandidateSimulations >= policy.MaxCandidateSimulations ||
			result.Consumption.CornerEvaluations >= policy.MaxCornerEvaluations ||
			result.Consumption.ValueTrials >= policy.MaxValueTrials ||
			result.Consumption.TopologyRepairs >= policy.MaxTopologyRepairs {
			result.Consumption.BudgetExhausted = true
		}
		if len(analysis.Trials) != 0 {
			generatedProposal = true
		}
		for _, causal := range causalCandidates {
			trial := causal.trial
			candidateHash := trial.GraphHash
			if _, duplicate := seenGraphs[candidateHash]; duplicate {
				continue
			}
			seenGraphs[candidateHash] = struct{}{}
			topologyHash, _ := TopologyHash(causal.graph)
			repair := trial.Repair
			repair.Number = len(result.Attempts) + 1
			attempt := RepairAttempt{
				Number: len(result.Attempts) + 1, Repair: repair,
				GraphHash: candidateHash, TopologyHash: topologyHash,
				Evaluation: trial.Evaluation, Improved: trial.Authorized && trial.Improvement > causalEpsilon,
				Status: RepairSearchFailed,
			}
			switch trial.Evaluation.Status {
			case SimulationEvaluationPassed:
				attempt.Status = RepairSearchPassed
			case SimulationEvaluationCanceled:
				attempt.Status = RepairSearchCanceled
			case SimulationEvaluationUnsupported:
				attempt.Status = RepairSearchUnsupported
			case SimulationEvaluationExhausted:
				attempt.Status = RepairSearchExhausted
			}
			result.Attempts = append(result.Attempts, attempt)
			if trial.Authorized && trial.Evaluation.Status == SimulationEvaluationPassed {
				result.Status = RepairSearchPassed
				repairs := append(append([]Repair(nil), state.repairs...), repair)
				result.Selected = &RepairedCandidate{
					Graph: causal.graph, Repair: repair, Repairs: repairs,
					Evaluation: trial.Evaluation,
				}
				return finalizeRepairSearch(result)
			}
			if trial.Authorized && trial.Improvement > causalEpsilon &&
				trial.Evaluation.Status == SimulationEvaluationFailed {
				repairs := append(append([]Repair(nil), state.repairs...), repair)
				frontier = append(frontier, repairState{
					graph: causal.graph, evaluation: trial.Evaluation, repairs: repairs,
					penalty: simulationEvaluationPenalty(trial.Evaluation), hash: candidateHash,
				})
			}
		}
		if len(frontier) > policy.MaxRetainedCandidates {
			slices.SortFunc(frontier, func(left, right repairState) int {
				return cmp.Or(cmp.Compare(left.penalty, right.penalty), cmp.Compare(left.hash, right.hash))
			})
			frontier = frontier[:policy.MaxRetainedCandidates]
		}
		if result.Consumption.BudgetExhausted {
			break
		}
	}
	result.Status = RepairSearchExhausted
	result.Issues = []reports.Issue{graphIssue(CodeRepairExhausted, "repair", "bounded generic repair did not produce a passing candidate", "inspect the strongest improved attempt or expand count budgets")}
	if (!generatedProposal || len(result.Attempts) == 0) && !result.Consumption.BudgetExhausted {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "repair", "all generated repairs collapsed to repeated or invalid graph states", "expand admissible generic repair operators")}
	}
	return finalizeRepairSearch(result)
}
