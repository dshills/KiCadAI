package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"slices"
)

// synthesisValueTrialV16 deliberately retains only the compact catalog trial.
// The candidate graph is materialized on demand for the current evaluation.
type synthesisValueTrialV16 struct {
	candidateIndex int
	trialIndex     int
	trial          ValueTrial
}

// SynthesizeV16 preserves the frozen bounded synthesis behavior while
// materializing at most one queued value-trial graph at a time and retaining
// only the deterministic best failed graph per topology for repair.
func SynthesizeV16(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	policy = effectiveTopologyPolicy(policy)
	requirement = Normalize(requirement)
	report := Report{
		Schema:        ReportSchema,
		Version:       ReportVersion,
		PolicyVersion: PolicyVersion,
		Policy:        policy,
		Status:        StatusFailed,
		StopReason:    StopNoPassingGraph,
		Candidates:    []CandidateReport{},
		Diagnostics:   []Diagnostic{},
	}
	report.PolicyHash, _ = PolicyHash(policy)
	report.RequirementHash, _ = CanonicalHash(requirement)
	report.PrimitiveInventoryHash = inventory.Hash
	report.CatalogHash = inventory.CatalogHash
	report.ModelRegistryHash = inventory.ModelRegistryHash

	run := SynthesisRun{
		Schema:     SynthesisRunSchema,
		Version:    SynthesisRunVersion,
		Report:     report,
		Candidates: []SynthesisCandidateEvidence{},
	}
	if issues := Validate(requirement); len(issues) != 0 {
		run.Report.Status = StatusInvalid
		run.Report.StopReason = StopRequirementInvalid
		appendSynthesisDiagnostics(&run.Report, issues)
		return finalizeSynthesisRunV16(run)
	}
	if issues := requirementFeasibilityIssues(requirement); len(issues) != 0 {
		run.Report.Status = StatusInfeasible
		run.Report.StopReason = StopRequirementInfeasible
		appendSynthesisDiagnostics(&run.Report, issues)
		return finalizeSynthesisRunV16(run)
	}
	if issues := requirementCapabilityIssues(requirement, inventory); len(issues) != 0 {
		run.Report.Status = StatusUnsupported
		run.Report.StopReason = StopModelUnavailable
		appendSynthesisDiagnostics(&run.Report, issues)
		return finalizeSynthesisRunV16(run)
	}
	if err := ctx.Err(); err != nil {
		run.Report.Status = StatusCanceled
		run.Report.StopReason = StopCanceled
		run.Report.Diagnostics = []Diagnostic{{
			Code: CodeCanceled, Path: "synthesis",
			Message:    "open-topology synthesis canceled",
			Suggestion: "retry with an active context",
		}}
		return finalizeSynthesisRunV16(run)
	}

	run.Search = SearchPrimitiveTopologies(ctx, requirement, inventory, policy)
	addConsumption(&run.Report.Consumption, run.Search.Consumption)
	if len(run.Search.Candidates) == 0 {
		run.Report.Status, run.Report.StopReason = synthesisSearchFailure(run.Search)
		appendSynthesisDiagnostics(&run.Report, run.Search.Issues)
		return finalizeSynthesisRunV16(run)
	}

	valueTrials := make([][]synthesisValueTrialV16, len(run.Search.Candidates))
	initialValueBudget := max(
		len(run.Search.Candidates),
		policy.MaxValueTrials/2,
	)
	initialValueBudget = min(initialValueBudget, policy.MaxValueTrials)
	for candidateIndex, candidate := range run.Search.Candidates {
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		evidence := SynthesisCandidateEvidence{
			Fingerprint:         candidate.Fingerprint,
			TopologyHash:        candidate.TopologyHash,
			ActiveStructureHash: candidate.ActiveStructureHash,
			ValuePlan:           plan,
			Evaluations:         []SimulationEvaluation{},
			Physical:            []PhysicalLoweringResult{},
		}
		run.Candidates = append(run.Candidates, evidence)
		run.Report.Candidates = append(run.Report.Candidates, CandidateReport{
			Fingerprint:         candidate.Fingerprint,
			TopologyHash:        candidate.TopologyHash,
			ActiveStructureHash: candidate.ActiveStructureHash,
			ComponentCount:      len(candidate.Graph.Instances),
			InternalNodes:       internalNodeCount(candidate.Graph),
			Status:              StatusFailed,
			Attempts:            []Attempt{},
		})
		if plan.Status != ValuePlanReady {
			run.Report.Candidates[candidateIndex].Status = statusForValuePlan(plan.Status)
			appendSynthesisDiagnostics(&run.Report, plan.Issues)
			continue
		}
		perCandidate := max(1, initialValueBudget/max(1, len(run.Search.Candidates)))
		enumeration := EnumerateValueTrials(plan, perCandidate)
		for trialIndex := range enumeration.Trials {
			trial := enumeration.Trials[trialIndex]
			// Validate each trial in the frozen deterministic order, but do not
			// retain a cloned graph for every candidate/value pair.
			_, err := ApplyValueTrial(candidate.Graph, trial, inventory)
			if err != nil {
				continue
			}
			valueTrials[candidateIndex] = append(valueTrials[candidateIndex], synthesisValueTrialV16{
				candidateIndex: candidateIndex,
				trialIndex:     trialIndex,
				trial:          trial,
			})
		}
	}

	bestFailures := make([]synthesisFailure, len(run.Search.Candidates))
	hasFailure := make([]bool, len(run.Search.Candidates))
	passes := []synthesisPassingCandidate{}
	maximumTrialCount := 0
	for _, trials := range valueTrials {
		maximumTrialCount = max(maximumTrialCount, len(trials))
	}
	candidateOrder := synthesisCandidateEvaluationOrder(requirement, inventory, run.Search.Candidates)
	initialEvaluationPolicy := synthesisInitialEvaluationPolicy(policy, len(candidateOrder))
	simulationStopped := false
	rankingWindowComplete := false
	postPassEvaluations := 0
	postPassEvaluationBudget := synthesisPostPassEvaluationBudget(policy)
	for trialIndex := 0; trialIndex < maximumTrialCount; trialIndex++ {
		for _, candidateIndex := range candidateOrder {
			if trialIndex >= len(valueTrials[candidateIndex]) {
				continue
			}
			// Once a physically ready pass exists, spend one explicit retained-
			// candidate window looking for a better margin across the round-robin
			// topology order. This preserves deterministic ranking without
			// exhaustively consuming every remaining value trial after success.
			if len(passes) != 0 && postPassEvaluations >= postPassEvaluationBudget {
				rankingWindowComplete = true
				break
			}
			if ctx.Err() != nil {
				run.Report.Status = StatusCanceled
				run.Report.StopReason = StopCanceled
				return finalizeSynthesisRunV16(run)
			}
			if synthesisSimulationBudgetExhausted(run.Report.Consumption, initialEvaluationPolicy) ||
				run.Report.Consumption.ValueTrials >= initialEvaluationPolicy.MaxValueTrials {
				simulationStopped = true
				break
			}
			work := valueTrials[candidateIndex][trialIndex]
			graph, err := ApplyValueTrial(
				run.Search.Candidates[candidateIndex].Graph, work.trial, inventory,
			)
			if err != nil {
				run.Report.Status = StatusFailed
				run.Report.StopReason = StopNoPassingGraph
				run.Report.Diagnostics = append(run.Report.Diagnostics, Diagnostic{
					Code: CodeNoCompleteGraph, Path: "synthesis.value_trials",
					Message:    "validated value trial could not be materialized deterministically",
					Suggestion: "inspect value-trial graph materialization",
				})
				return finalizeSynthesisRunV16(run)
			}
			evaluationPolicy := synthesisCandidateEvaluationPolicy(
				policy, initialEvaluationPolicy, run.Report.Consumption,
			)
			evaluation := EvaluateCandidate(
				ctx, requirement, graph, nil, inventory, environment, evaluationPolicy,
			)
			if len(passes) != 0 {
				postPassEvaluations++
			}
			run.Report.Consumption.ValueTrials++
			addSimulationConsumption(&run.Report.Consumption, evaluation.Consumption)
			run.Candidates[candidateIndex].Evaluations = append(
				run.Candidates[candidateIndex].Evaluations, evaluation,
			)
			run.Report.Candidates[candidateIndex].Attempts = append(
				run.Report.Candidates[candidateIndex].Attempts,
				synthesisAttempt(
					len(run.Report.Candidates[candidateIndex].Attempts)+1,
					graph,
					&work.trial,
					evaluation,
					nil,
				),
			)
			run.Report.Candidates[candidateIndex].Status = statusForSimulation(evaluation.Status)
			switch evaluation.Status {
			case SimulationEvaluationPassed:
				physical := LowerPassingCandidate(
					ctx, requirement, graph, evaluation, inventory, environment,
				)
				run.Candidates[candidateIndex].Physical = append(
					run.Candidates[candidateIndex].Physical, physical,
				)
				if physical.Status == PhysicalLoweringReady {
					passes = append(passes, synthesisPassingCandidate{
						candidateIndex: candidateIndex,
						graph:          graph,
						trial:          &work.trial,
						evaluation:     evaluation,
						physical:       physical,
						margin:         synthesisWorstNormalizedMargin(evaluation),
					})
					continue
				}
				appendSynthesisDiagnostics(&run.Report, physical.Issues)
			case SimulationEvaluationFailed:
				failure := synthesisFailure{
					candidateIndex: candidateIndex,
					graph:          graph,
					evaluation:     evaluation,
					penalty:        simulationEvaluationPenalty(evaluation),
				}
				if !hasFailure[candidateIndex] || compareSynthesisFailureV16(failure, bestFailures[candidateIndex]) < 0 {
					bestFailures[candidateIndex] = failure
					hasFailure[candidateIndex] = true
				}
			case SimulationEvaluationCanceled:
				run.Report.Status = StatusCanceled
				run.Report.StopReason = StopCanceled
				appendSynthesisDiagnostics(&run.Report, evaluation.Issues)
				return finalizeSynthesisRunV16(run)
			default:
				appendSynthesisDiagnostics(&run.Report, evaluation.Issues)
			}
		}
		// A physical pass completes the current round's retained-topology
		// ranking window. Candidate order above still gives every remaining
		// peer topology in this round a bounded opportunity to compete, but
		// later rounds contain only alternate value trials for those same
		// topologies and need not repeat the full corner suite after success.
		if len(passes) != 0 {
			rankingWindowComplete = true
		}
		if simulationStopped || rankingWindowComplete {
			break
		}
	}
	if len(passes) != 0 {
		return selectRankedSynthesisResultV16(run, passes)
	}

	failures := make([]synthesisFailure, 0, len(run.Search.Candidates))
	for candidateIndex := range bestFailures {
		if hasFailure[candidateIndex] {
			failures = append(failures, bestFailures[candidateIndex])
		}
	}
	slices.SortFunc(failures, compareSynthesisFailureV16)
	for _, failure := range failures {
		if ctx.Err() != nil {
			run.Report.Status = StatusCanceled
			run.Report.StopReason = StopCanceled
			return finalizeSynthesisRunV16(run)
		}
		if synthesisRepairBudgetExhausted(run.Report.Consumption, policy) {
			run.Report.Consumption.BudgetExhausted = true
			break
		}
		repairPolicy := remainingSynthesisPolicy(policy, run.Report.Consumption)
		repair := RepairCandidateV16(
			ctx, requirement, failure.graph, failure.evaluation,
			inventory, environment, repairPolicy,
		)
		run.Candidates[failure.candidateIndex].Repair = &repair
		addSimulationConsumption(&run.Report.Consumption, repair.Consumption)
		addRepairConsumption(&run.Report.Consumption, repair.Consumption)
		for _, repairAttempt := range repair.Attempts {
			run.Report.Candidates[failure.candidateIndex].Attempts = append(
				run.Report.Candidates[failure.candidateIndex].Attempts,
				synthesisAttempt(
					len(run.Report.Candidates[failure.candidateIndex].Attempts)+1,
					repairAttemptGraphHash(repairAttempt),
					repairAttempt.ValueTrial,
					repairAttempt.Evaluation,
					&repairAttempt.Repair,
				),
			)
		}
		run.Report.Candidates[failure.candidateIndex].Status = statusForRepair(repair.Status)
		if repair.Status != RepairSearchPassed || repair.Selected == nil {
			appendSynthesisDiagnostics(&run.Report, repair.Issues)
			continue
		}
		selected := repair.Selected
		physical := LowerPassingCandidate(
			ctx, requirement, selected.Graph, selected.Evaluation, inventory, environment,
		)
		run.Candidates[failure.candidateIndex].Physical = append(
			run.Candidates[failure.candidateIndex].Physical, physical,
		)
		if physical.Status == PhysicalLoweringReady {
			passes = append(passes, synthesisPassingCandidate{
				candidateIndex: failure.candidateIndex,
				graph:          selected.Graph,
				trial:          selected.ValueTrial,
				evaluation:     selected.Evaluation,
				repair:         &repair,
				physical:       physical,
				margin:         synthesisWorstNormalizedMargin(selected.Evaluation),
				repairCount:    repair.Consumption.TopologyRepairs,
			})
			continue
		}
		appendSynthesisDiagnostics(&run.Report, physical.Issues)
	}
	if len(passes) != 0 {
		return selectRankedSynthesisResultV16(run, passes)
	}

	run.Report.Status = StatusFailed
	run.Report.StopReason = StopNoPassingGraph
	if ctx.Err() != nil {
		run.Report.Status = StatusCanceled
		run.Report.StopReason = StopCanceled
	} else if run.Report.Consumption.BudgetExhausted {
		run.Report.Status = StatusExhausted
		run.Report.StopReason = StopSearchExhausted
	} else if synthesisValueCapabilityUnavailable(run.Candidates) {
		run.Report.Status = StatusUnsupported
		run.Report.StopReason = StopValueExhausted
	} else if len(failures) != 0 {
		run.Report.StopReason = StopRepairExhausted
	}
	if len(run.Report.Diagnostics) == 0 {
		run.Report.Diagnostics = []Diagnostic{{
			Code: CodeNoPassingGraph, Path: "synthesis",
			Message:    "bounded topology, value, and repair search produced no physically promotable candidate",
			Suggestion: "inspect retained simulation diagnoses or expand explicit count budgets",
		}}
	}
	return finalizeSynthesisRunV16(run)
}

func compareSynthesisFailureV16(left, right synthesisFailure) int {
	return cmp.Or(
		cmp.Compare(left.penalty, right.penalty),
		cmp.Compare(left.candidateIndex, right.candidateIndex),
		cmp.Compare(left.evaluation.GraphHash, right.evaluation.GraphHash),
	)
}

func selectRankedSynthesisResultV16(run SynthesisRun, passes []synthesisPassingCandidate) SynthesisRun {
	passes = bestSynthesisPasses(passes)
	slices.SortFunc(passes, compareSynthesisPasses)
	selected := passes[0]
	graphCopy := CloneGraph(selected.graph)
	run.SelectedGraph = &graphCopy
	run.SelectedTrial = cloneValueTrial(selected.trial)
	run.SelectedRepair = selected.repair
	physicalCopy := selected.physical
	run.Physical = &physicalCopy
	run.Report.Status = StatusPassed
	run.Report.StopReason = StopPassed
	run.Report.Diagnostics = []Diagnostic{}
	run.Report.Candidates[selected.candidateIndex].Status = StatusPassed
	alternatives := make([]RankedSelectionCandidate, 0, len(passes))
	for index, candidate := range passes {
		valueHash := candidate.evaluation.ValueTrialHash
		if candidate.trial != nil && candidate.trial.Hash != "" {
			valueHash = candidate.trial.Hash
		}
		disposition := "not_selected"
		reason := synthesisRankingDifference(selected, candidate)
		if index == 0 {
			disposition = "selected"
			reason = "highest deterministic rank among simulation-passing, physically ready architectures"
		}
		alternatives = append(alternatives, RankedSelectionCandidate{
			Rank:                  index + 1,
			Fingerprint:           run.Report.Candidates[candidate.candidateIndex].Fingerprint,
			TopologyHash:          mustTopologyHash(candidate.graph),
			ActiveStructureHash:   mustActiveStructureHash(candidate.graph),
			EvaluationHash:        candidate.evaluation.Hash,
			PhysicalHash:          candidate.physical.Hash,
			ValueHash:             valueHash,
			WorstNormalizedMargin: candidate.margin,
			ComponentCount:        len(candidate.graph.Instances),
			InternalNodes:         internalNodeCount(candidate.graph),
			TopologyRepairs:       candidate.repairCount,
			Selected:              index == 0,
			Disposition:           disposition,
			Reason:                reason,
		})
	}
	run.Report.Selected = &SelectedResult{
		Fingerprint:         run.Report.Candidates[selected.candidateIndex].Fingerprint,
		TopologyHash:        mustTopologyHash(selected.graph),
		ActiveStructureHash: mustActiveStructureHash(selected.graph),
		EvaluationHash:      selected.evaluation.Hash,
		PhysicalHash:        selected.physical.Hash,
		SelectionSummary: fmt.Sprintf(
			"selected rank 1 of %d physically ready architectures: worst normalized requirement margin %.9g, %d topology repairs, %d components, and %d internal nodes; deterministic tie-breakers use topology, evaluation, and value hashes",
			len(passes), selected.margin, selected.repairCount,
			len(selected.graph.Instances), internalNodeCount(selected.graph),
		),
		Ranking: SelectionRanking{
			Policy:       synthesisSelectionRankingPolicy,
			Alternatives: alternatives,
			Rejections:   synthesisSelectionRejections(run, passes),
		},
	}
	return finalizeSynthesisRunV16(run)
}

func finalizeSynthesisRunV16(run SynthesisRun) SynthesisRun {
	if run.Report.Status == StatusPassed &&
		(run.Report.Selected == nil || run.Report.Selected.ActiveStructureHash == "" ||
			run.SelectedGraph == nil || run.Physical == nil ||
			run.Physical.Status != PhysicalLoweringReady) {
		run.Report.Status = StatusFailed
		run.Report.StopReason = StopNoPassingGraph
		run.Report.Diagnostics = append(run.Report.Diagnostics, Diagnostic{
			Code: CodeNoPassingGraph, Path: "synthesis.selection",
			Message:    "passing synthesis result lacks a complete graph and physical selection",
			Suggestion: "inspect selection finalization and rerun bounded synthesis",
		})
	}
	if run.Report.Status != StatusPassed {
		run.SelectedGraph = nil
		run.SelectedTrial = nil
		run.SelectedRepair = nil
		run.Physical = nil
	}
	copy := run
	copy.Hash = ""
	run.Hash = hashJSONV16(copy)
	return run
}
