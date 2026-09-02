package opentopologysynthesis

import (
	"cmp"
	"context"
	"slices"

	"kicadai/internal/reports"
	"kicadai/internal/simulationadmission"
)

// synthesizeAdmissionV20 is a version-isolated copy of the current bounded
// production search. Historical synthesis files remain byte-frozen; only this
// successor binds the V20 admission evaluator.
func synthesizeAdmissionV20(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.Environment,
	policy Policy,
) SynthesisRun {
	return synthesizeAdmissionPreparedV20(
		ctx, requirement, inventory, environment,
		simulationadmission.PrepareEnvironment(admission), policy,
	)
}

func synthesizeAdmissionPreparedV20(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.PreparedEnvironment,
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
		return finalizeSynthesisRun(run)
	}
	if issues := requirementFeasibilityIssues(requirement); len(issues) != 0 {
		run.Report.Status = StatusInfeasible
		run.Report.StopReason = StopRequirementInfeasible
		appendSynthesisDiagnostics(&run.Report, issues)
		return finalizeSynthesisRun(run)
	}
	decision := simulationadmission.AdmitPrepared(
		requirementSimulationAdmissionRequest(requirement, inventory),
		admission,
	)
	if decision.Status != simulationadmission.StatusAdmitted {
		run.Report.Status = StatusUnsupported
		run.Report.StopReason = StopModelUnavailable
		for _, diagnostic := range decision.Diagnostics {
			run.Report.Diagnostics = append(run.Report.Diagnostics, Diagnostic{
				Code: reports.Code(diagnostic.Code), Path: diagnostic.Path,
				Message: diagnostic.Message, Suggestion: diagnostic.Suggestion,
			})
		}
		return finalizeSynthesisRun(run)
	}
	if issues := requirementCapabilityIssues(requirement, inventory); len(issues) != 0 {
		run.Report.Status = StatusUnsupported
		run.Report.StopReason = StopModelUnavailable
		appendSynthesisDiagnostics(&run.Report, issues)
		return finalizeSynthesisRun(run)
	}
	if err := ctx.Err(); err != nil {
		run.Report.Status = StatusCanceled
		run.Report.StopReason = StopCanceled
		run.Report.Diagnostics = []Diagnostic{{
			Code: CodeCanceled, Path: "synthesis",
			Message:    "open-topology synthesis canceled",
			Suggestion: "retry with an active context",
		}}
		return finalizeSynthesisRun(run)
	}

	run.Search = SearchPrimitiveTopologies(ctx, requirement, inventory, policy)
	addConsumption(&run.Report.Consumption, run.Search.Consumption)
	if len(run.Search.Candidates) == 0 {
		run.Report.Status, run.Report.StopReason = synthesisSearchFailure(run.Search)
		appendSynthesisDiagnostics(&run.Report, run.Search.Issues)
		return finalizeSynthesisRun(run)
	}

	valueTrials := make([][]synthesisValueTrial, len(run.Search.Candidates))
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
			graph, err := ApplyValueTrial(candidate.Graph, trial, inventory)
			if err != nil {
				continue
			}
			valueTrials[candidateIndex] = append(valueTrials[candidateIndex], synthesisValueTrial{
				candidateIndex: candidateIndex,
				trialIndex:     trialIndex,
				graph:          graph,
				trial:          &trial,
			})
		}
	}

	failures := []synthesisFailure{}
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
				return finalizeSynthesisRun(run)
			}
			if synthesisSimulationBudgetExhausted(run.Report.Consumption, initialEvaluationPolicy) ||
				run.Report.Consumption.ValueTrials >= initialEvaluationPolicy.MaxValueTrials {
				simulationStopped = true
				break
			}
			work := valueTrials[candidateIndex][trialIndex]
			evaluationPolicy := synthesisCandidateEvaluationPolicy(
				policy, initialEvaluationPolicy, run.Report.Consumption,
			)
			evaluation := evaluateCandidatePreparedV20(
				ctx, requirement, work.graph, nil, inventory, environment, admission, evaluationPolicy,
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
					work.graph,
					work.trial,
					evaluation,
					nil,
				),
			)
			run.Report.Candidates[candidateIndex].Status = statusForSimulation(evaluation.Status)
			switch evaluation.Status {
			case SimulationEvaluationPassed:
				physical := LowerPassingCandidate(
					ctx, requirement, work.graph, evaluation, inventory, environment,
				)
				run.Candidates[candidateIndex].Physical = append(
					run.Candidates[candidateIndex].Physical, physical,
				)
				if physical.Status == PhysicalLoweringReady {
					passes = append(passes, synthesisPassingCandidate{
						candidateIndex: candidateIndex,
						graph:          work.graph,
						trial:          work.trial,
						evaluation:     evaluation,
						physical:       physical,
						margin:         synthesisWorstNormalizedMargin(evaluation),
					})
					continue
				}
				appendSynthesisDiagnostics(&run.Report, physical.Issues)
			case SimulationEvaluationFailed:
				failures = append(failures, synthesisFailure{
					candidateIndex: candidateIndex,
					graph:          work.graph,
					evaluation:     evaluation,
					penalty:        simulationEvaluationPenalty(evaluation),
				})
			case SimulationEvaluationCanceled:
				run.Report.Status = StatusCanceled
				run.Report.StopReason = StopCanceled
				appendSynthesisDiagnostics(&run.Report, evaluation.Issues)
				return finalizeSynthesisRun(run)
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
		return selectRankedSynthesisResult(run, passes)
	}

	slices.SortFunc(failures, func(left, right synthesisFailure) int {
		return cmp.Or(
			cmp.Compare(left.penalty, right.penalty),
			cmp.Compare(left.candidateIndex, right.candidateIndex),
			cmp.Compare(left.evaluation.GraphHash, right.evaluation.GraphHash),
		)
	})
	failures = bestSynthesisFailures(failures)
	for _, failure := range failures {
		if ctx.Err() != nil {
			run.Report.Status = StatusCanceled
			run.Report.StopReason = StopCanceled
			return finalizeSynthesisRun(run)
		}
		if synthesisRepairBudgetExhausted(run.Report.Consumption, policy) {
			run.Report.Consumption.BudgetExhausted = true
			break
		}
		repairPolicy := remainingSynthesisPolicy(policy, run.Report.Consumption)
		repair := repairCandidatePreparedV20(
			ctx, requirement, failure.graph, failure.evaluation,
			inventory, environment, admission, repairPolicy,
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
		return selectRankedSynthesisResult(run, passes)
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
	return finalizeSynthesisRun(run)
}
