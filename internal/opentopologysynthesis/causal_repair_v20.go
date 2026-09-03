package opentopologysynthesis

import (
	"cmp"
	"context"
	"slices"

	"kicadai/internal/simulationadmission"
)

func analyzeCausalRepairsV20(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.PreparedEnvironment,
	policy Policy,
) (CausalRepairAnalysis, []causalEvaluatedCandidate) {
	graphHash, _ := GraphHash(graph)
	trialBudget := minPositive(
		minPositive(causalMaximumSingleTrials, policy.MaxValueTrials),
		policy.MaxCandidateSimulations,
	)
	if trialBudget < 0 {
		trialBudget = 0
	}
	coordinatedBudget := min(
		max(0, trialBudget/4),
		max(0, policy.MaxTopologyRepairs/2),
	)
	analysis := CausalRepairAnalysis{
		Schema:                CausalRepairSchema,
		Version:               CausalRepairVersion,
		PolicyVersion:         PolicyVersion,
		RequirementHash:       initial.RequirementHash,
		InventoryHash:         inventory.Hash,
		InitialGraphHash:      graphHash,
		InitialEvaluationHash: initial.Hash,
		Budget: CausalRepairBudget{
			Trials:               trialBudget,
			ValueTrials:          policy.MaxValueTrials,
			TopologyTrials:       policy.MaxTopologyRepairs,
			CoordinatedTrials:    coordinatedBudget,
			MaximumChanges:       causalMaximumChanges,
			CandidateSimulations: policy.MaxCandidateSimulations,
			CornerEvaluations:    policy.MaxCornerEvaluations,
		},
		Trials: []CausalRepairTrial{},
		Status: "no_safe_improvement",
	}
	if trialBudget == 0 || initial.Hash == "" {
		analysis.Consumption.BudgetExhausted = trialBudget == 0
		return finalizeCausalRepairAnalysis(analysis), nil
	}

	candidates := generateCausalCandidates(requirement, graph, initial, inventory, policy)
	evaluated := make([]causalEvaluatedCandidate, 0, min(len(candidates), trialBudget))
	seen := map[string]struct{}{graphHash: {}}
	evaluate := func(candidate causalCandidate) bool {
		if err := ctx.Err(); err != nil {
			analysis.Status = "canceled"
			return false
		}
		singleTrialLimit := max(0, analysis.Budget.Trials-analysis.Budget.CoordinatedTrials)
		if !candidate.coordinated && analysis.Consumption.Trials >= singleTrialLimit {
			return true
		}
		if candidate.coordinated && analysis.Consumption.CoordinatedTrials >= analysis.Budget.CoordinatedTrials {
			return true
		}
		if analysis.Consumption.Trials >= analysis.Budget.Trials ||
			analysis.Consumption.CandidateSimulations >= analysis.Budget.CandidateSimulations ||
			analysis.Consumption.CornerEvaluations >= analysis.Budget.CornerEvaluations {
			analysis.Consumption.BudgetExhausted = true
			return false
		}
		usesTopology := causalCandidateUsesTopology(candidate)
		if usesTopology && analysis.Consumption.TopologyTrials >= analysis.Budget.TopologyTrials {
			return true
		}
		if !usesTopology && analysis.Consumption.ValueTrials >= analysis.Budget.ValueTrials {
			return true
		}
		candidateHash, err := GraphHash(candidate.graph)
		if err != nil {
			return true
		}
		if _, duplicate := seen[candidateHash]; duplicate {
			return true
		}
		seen[candidateHash] = struct{}{}
		remaining := policy
		remaining.MaxCandidateSimulations = max(0, analysis.Budget.CandidateSimulations-analysis.Consumption.CandidateSimulations)
		remaining.MaxCornerEvaluations = max(0, analysis.Budget.CornerEvaluations-analysis.Consumption.CornerEvaluations)
		evaluation := evaluateCandidatePreparedV20(ctx, requirement, candidate.graph, nil, inventory, environment, admission, remaining)
		analysis.Consumption.Trials++
		if usesTopology {
			analysis.Consumption.TopologyTrials++
		} else {
			analysis.Consumption.ValueTrials++
		}
		if candidate.coordinated {
			analysis.Consumption.CoordinatedTrials++
		}
		analysis.Consumption.CandidateSimulations += evaluation.Consumption.CandidateSimulations
		analysis.Consumption.CornerEvaluations += evaluation.Consumption.CornerEvaluations
		trial := causalTrialEvidence(requirement, initial, evaluation, candidate, candidateHash)
		trial.Number = len(analysis.Trials) + 1
		trial.Hash = causalTrialHash(trial)
		analysis.Trials = append(analysis.Trials, trial)
		evaluated = append(evaluated, causalEvaluatedCandidate{graph: candidate.graph, trial: trial})
		return true
	}

	for _, candidate := range candidates {
		if !evaluate(candidate) {
			break
		}
	}

	if analysis.Consumption.Trials < analysis.Budget.Trials &&
		analysis.Consumption.CoordinatedTrials < analysis.Budget.CoordinatedTrials {
		coordinated := coordinatedCausalCandidates(graph, evaluated, analysis.Budget.CoordinatedTrials)
		remainingCoordinated := max(0, analysis.Budget.CoordinatedTrials-len(coordinated))
		coordinated = append(coordinated, coordinatedTopologyValueCandidates(graph, evaluated, remainingCoordinated)...)
		for _, candidate := range coordinated {
			if !evaluate(candidate) {
				break
			}
		}
	}

	rankCausalTrials(analysis.Trials)
	byHash := make(map[string]CausalRepairTrial, len(analysis.Trials))
	for _, trial := range analysis.Trials {
		byHash[trial.Hash] = trial
		if trial.Rank == 1 && trial.Authorized {
			analysis.SelectedTrialHash = trial.Hash
			if trial.Status == SimulationEvaluationPassed {
				analysis.Status = "passing_repair_found"
			} else {
				analysis.Status = "safe_improvement_found"
			}
		}
	}
	for index := range evaluated {
		evaluated[index].trial = byHash[evaluated[index].trial.Hash]
	}
	slices.SortFunc(evaluated, func(left, right causalEvaluatedCandidate) int {
		return cmp.Or(
			cmp.Compare(left.trial.Rank, right.trial.Rank),
			cmp.Compare(left.trial.Hash, right.trial.Hash),
		)
	})
	return finalizeCausalRepairAnalysis(analysis), evaluated
}
