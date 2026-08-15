package opentopologysynthesis

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"kicadai/internal/canonicaljsonstream"
	"kicadai/internal/reports"
)

func RepairCandidateV17(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
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
		return finalizeRepairSearchV17(result)
	}
	result.RequirementHash = requirementHash
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "normalize repair graph: "+err.Error(), "")}
		return finalizeRepairSearchV17(result)
	}
	result.InitialGraphHash, err = GraphHash(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "hash repair graph: "+err.Error(), "")}
		return finalizeRepairSearchV17(result)
	}
	if initial.GraphHash != result.InitialGraphHash || initial.RequirementHash != requirementHash ||
		initial.InventoryHash != inventory.Hash || initial.Hash == "" {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "initial_evaluation", "repair requires a hash-bound evaluation of the supplied graph, requirement, and inventory", "evaluate the exact candidate before repair")}
		return finalizeRepairSearchV17(result)
	}
	if initial.Status == SimulationEvaluationPassed {
		result.Status = RepairSearchPassed
		result.Selected = &RepairedCandidate{Graph: graph, Repairs: []Repair{}, Evaluation: initial}
		return finalizeRepairSearchV17(result)
	}
	if len(initial.Diagnoses) == 0 {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "initial_evaluation.diagnoses", "failed candidate has no stable diagnosis for generic repair", "retain normalized simulation diagnoses")}
		return finalizeRepairSearchV17(result)
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
		analysis, causalCandidates := analyzeCausalRepairsV17(
			ctx, requirement, state.graph, state.evaluation,
			inventory, environment, causalPolicy,
		)
		result.CausalAnalyses = append(result.CausalAnalyses, analysis)
		if analysis.Status == "canceled" {
			result.Status = RepairSearchCanceled
			result.Issues = []reports.Issue{graphIssue(CodeCanceled, "repair", "open-topology repair canceled", "retry with an active context")}
			return finalizeRepairSearchV17(result)
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
				return finalizeRepairSearchV17(result)
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
	return finalizeRepairSearchV17(result)
}

func finalizeRepairSearchV17(result RepairSearchResult) RepairSearchResult {
	result.Trace = electricalRepairTrace(result)
	copy := result
	copy.Hash = ""
	result.Hash = hashJSONV17(copy)
	return result
}

func hashJSONV17(value any) string {
	hasher := sha256.New()
	if err := canonicaljsonstream.Encode(hasher, value); err != nil {
		panic("canonical V17 repair hash encoding failed: " + err.Error())
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func analyzeCausalRepairsV17(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
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

	candidates := generateCausalCandidatesV17(requirement, graph, initial, inventory, policy)
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
		evaluation := EvaluateCandidateV17(ctx, requirement, candidate.graph, nil, inventory, environment, remaining)
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

func generateCausalCandidatesV17(
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	policy Policy,
) []causalCandidate {
	result := causalValueCandidates(requirement, graph, inventory, policy)
	result = append(result, causalPolarityCandidates(graph, inventory, initial.Diagnoses)...)
	for _, proposal := range generateRepairProposals(requirement, graph, initial.Diagnoses, inventory, policy) {
		repair := proposal.repair
		if repair.Operator == "substitute_compatible_primitive" {
			continue
		}
		repair.Operator = causalOperatorForProposal(repair, initial.Diagnoses)
		result = append(result, sizedCausalProposalCandidatesV17(
			requirement, graph, proposal.graph, repair, inventory, policy,
		)...)
	}
	slices.SortFunc(result, compareCausalCandidates)
	return diversifyCausalCandidates(compactCausalCandidates(result))
}

func sizedCausalProposalCandidatesV17(
	requirement Requirement,
	before CandidateGraph,
	proposed CandidateGraph,
	repair Repair,
	inventory PrimitiveInventory,
	policy Policy,
) []causalCandidate {
	maximum := max(1, min(4, policy.MaxValueTrials))
	sized := []repairedValueCandidate{{graph: proposed}}
	visitCausalValueCandidatesV17(requirement, proposed, inventory, policy, func(valueCandidate causalCandidate) bool {
		if len(valueCandidate.perturbations) != 1 {
			return true
		}
		if graphInstanceIndex(before, valueCandidate.perturbations[0].InstanceID) >= 0 {
			return true
		}
		sized = append(sized, repairedValueCandidate{graph: valueCandidate.graph})
		return len(sized) <= maximum
	})
	if len(sized) == 1 {
		sized = append(sized, repairedGraphValueTrials(requirement, proposed, inventory, maximum, policy)...)
	}
	result := make([]causalCandidate, 0, len(sized))
	seen := map[string]struct{}{}
	for _, candidate := range sized {
		candidateHash, err := GraphHash(candidate.graph)
		if err != nil {
			continue
		}
		if _, duplicate := seen[candidateHash]; duplicate {
			continue
		}
		seen[candidateHash] = struct{}{}
		candidateRepair := repair
		valueChanges := causalValueChanges(proposed, candidate.graph)
		candidateRepair.Changes = append(append([]GraphChange(nil), repair.Changes...), valueChanges...)
		candidateRepair.AfterGraphHash, _ = GraphHash(candidate.graph)
		perturbations := causalPerturbationsForChanges(before, repair.Changes)
		perturbations = append(perturbations, causalPerturbationsForChanges(proposed, valueChanges)...)
		if len(perturbations) == 0 || len(perturbations) > causalMaximumChanges {
			continue
		}
		result = append(result, causalCandidate{
			graph: candidate.graph, perturbations: perturbations, repair: candidateRepair,
		})
	}
	return result
}

func visitCausalValueCandidatesV17(
	requirement Requirement,
	graph CandidateGraph,
	inventory PrimitiveInventory,
	policy Policy,
	visit func(causalCandidate) bool,
) {
	plan := BuildValueSearchPlan(requirement, graph, inventory, policy)
	if plan.Status != ValuePlanReady {
		return
	}
	beforeHash, _ := GraphHash(graph)
	for _, domain := range plan.Domains {
		instanceIndex := graphInstanceIndex(graph, domain.InstanceID)
		if instanceIndex < 0 {
			continue
		}
		instance := graph.Instances[instanceIndex]
		for _, candidate := range domain.Candidates {
			if sameCausalValue(instance, candidate) {
				continue
			}
			selection := ValueTrialSelection{
				InstanceID: domain.InstanceID, PrimitiveKey: candidate.PrimitiveKey,
				ValueSI: cloneInventoryFloat(candidate.ValueSI), CandidateHash: candidate.Hash,
			}
			trial := ValueTrial{Number: 1, Selections: []ValueTrialSelection{selection}}
			trial.Hash = valueTrialHash(trial.Selections)
			candidateGraph, err := ApplyValueTrial(graph, trial, inventory)
			if err != nil {
				continue
			}
			kind := "adjust_value"
			operator := "adjust_component_value"
			changeKind := "set_value"
			if candidate.PrimitiveKey != instance.PrimitiveKey {
				changeKind = "substitute_primitive"
			}
			if candidate.PrimitiveKey != instance.PrimitiveKey && domain.Quantity == "" {
				kind = "substitute_rated_device"
				operator = "substitute_compatible_primitive"
			}
			perturbation := newCausalPerturbation(CausalPerturbation{
				Kind: kind, InstanceID: instance.ID,
				FromPrimitiveKey: instance.PrimitiveKey, ToPrimitiveKey: candidate.PrimitiveKey,
				FromValue: cloneInventoryFloat(instance.ValueSI), ToValue: cloneInventoryFloat(candidate.ValueSI),
				Magnitude: causalValueMagnitude(instance.ValueSI, candidate.ValueSI, instance.PrimitiveKey != candidate.PrimitiveKey),
			})
			afterHash, _ := GraphHash(candidateGraph)
			if !visit(causalCandidate{
				graph:         candidateGraph,
				perturbations: []CausalPerturbation{perturbation},
				repair: Repair{
					Operator: operator, BeforeGraphHash: beforeHash, AfterGraphHash: afterHash,
					Changes: []GraphChange{{
						Kind: changeKind, Primitive: instance.ID,
						FromNode: instance.PrimitiveKey, ToNode: candidate.PrimitiveKey,
						FromValue: cloneInventoryFloat(instance.ValueSI), ToValue: cloneInventoryFloat(candidate.ValueSI),
					}},
				},
			}) {
				return
			}
		}
	}
}
