package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"

	"kicadai/internal/reports"
)

type synthesisValueTrial struct {
	candidateIndex int
	trialIndex     int
	graph          CandidateGraph
	trial          *ValueTrial
}

type synthesisFailure struct {
	candidateIndex int
	graph          CandidateGraph
	evaluation     SimulationEvaluation
	penalty        float64
}

type synthesisPassingCandidate struct {
	candidateIndex int
	graph          CandidateGraph
	trial          *ValueTrial
	evaluation     SimulationEvaluation
	repair         *RepairSearchResult
	physical       PhysicalLoweringResult
	margin         float64
	repairCount    int
}

// Synthesize runs the bounded primitive-only production path. Topologies are
// searched without a named block family, value trials are evaluated across
// candidates in round-robin order, physically ready passes are ranked by
// trusted requirement margins and complexity, failed graphs are ranked for
// repair, and only a fully passing graph may enter physical promotion.
func Synthesize(
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
			Fingerprint:  candidate.Fingerprint,
			TopologyHash: candidate.TopologyHash,
			ValuePlan:    plan,
			Evaluations:  []SimulationEvaluation{},
			Physical:     []PhysicalLoweringResult{},
		}
		run.Candidates = append(run.Candidates, evidence)
		run.Report.Candidates = append(run.Report.Candidates, CandidateReport{
			Fingerprint:    candidate.Fingerprint,
			TopologyHash:   candidate.TopologyHash,
			ComponentCount: len(candidate.Graph.Instances),
			InternalNodes:  internalNodeCount(candidate.Graph),
			Status:         StatusFailed,
			Attempts:       []Attempt{},
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
	candidateOrder := synthesisCandidateEvaluationOrder(run.Search.Candidates)
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
			if synthesisSimulationBudgetExhausted(run.Report.Consumption, policy) ||
				run.Report.Consumption.ValueTrials >= policy.MaxValueTrials {
				run.Report.Consumption.BudgetExhausted = true
				simulationStopped = true
				break
			}
			work := valueTrials[candidateIndex][trialIndex]
			evaluationPolicy := remainingSynthesisPolicy(policy, run.Report.Consumption)
			evaluation := EvaluateCandidate(
				ctx, requirement, work.graph, nil, inventory, environment, evaluationPolicy,
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
		repair := RepairCandidate(
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

func synthesisPostPassEvaluationBudget(policy Policy) int {
	return max(1, policy.MaxRetainedCandidates)
}

func synthesisCandidateEvaluationOrder(
	candidates []TopologyCandidate,
) []int {
	order := make([]int, len(candidates))
	for index := range candidates {
		order[index] = index
	}
	slices.SortStableFunc(order, func(left, right int) int {
		if candidates[left].Repairable != candidates[right].Repairable {
			if candidates[left].Repairable {
				return -1
			}
			return 1
		}
		return cmp.Compare(left, right)
	})
	return order
}

const synthesisSelectionRankingPolicy = "worst_normalized_requirement_margin_desc,topology_repairs_asc,component_count_asc,internal_nodes_asc,topology_hash_asc,evaluation_hash_asc,value_hash_asc"

func selectRankedSynthesisResult(run SynthesisRun, passes []synthesisPassingCandidate) SynthesisRun {
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
	run.Report.Candidates[selected.candidateIndex].Status = StatusPassed
	alternatives := make([]RankedSelectionCandidate, 0, len(passes))
	for index, candidate := range passes {
		valueHash := candidate.evaluation.ValueTrialHash
		if candidate.trial != nil && candidate.trial.Hash != "" {
			valueHash = candidate.trial.Hash
		}
		alternatives = append(alternatives, RankedSelectionCandidate{
			Rank:                  index + 1,
			Fingerprint:           run.Report.Candidates[candidate.candidateIndex].Fingerprint,
			TopologyHash:          mustTopologyHash(candidate.graph),
			EvaluationHash:        candidate.evaluation.Hash,
			ValueHash:             valueHash,
			WorstNormalizedMargin: candidate.margin,
			ComponentCount:        len(candidate.graph.Instances),
			InternalNodes:         internalNodeCount(candidate.graph),
			TopologyRepairs:       candidate.repairCount,
			Selected:              index == 0,
		})
	}
	run.Report.Selected = &SelectedResult{
		Fingerprint:    run.Report.Candidates[selected.candidateIndex].Fingerprint,
		TopologyHash:   mustTopologyHash(selected.graph),
		EvaluationHash: selected.evaluation.Hash,
		PhysicalHash:   selected.physical.Hash,
		SelectionSummary: fmt.Sprintf(
			"selected rank 1 of %d physically ready architectures: worst normalized requirement margin %.9g, %d topology repairs, %d components, and %d internal nodes; deterministic tie-breakers use topology, evaluation, and value hashes",
			len(passes), selected.margin, selected.repairCount,
			len(selected.graph.Instances), internalNodeCount(selected.graph),
		),
		Ranking: SelectionRanking{
			Policy:       synthesisSelectionRankingPolicy,
			Alternatives: alternatives,
		},
	}
	return finalizeSynthesisRun(run)
}

func bestSynthesisPasses(source []synthesisPassingCandidate) []synthesisPassingCandidate {
	best := map[string]synthesisPassingCandidate{}
	for _, candidate := range source {
		topology := mustTopologyHash(candidate.graph)
		if current, ok := best[topology]; !ok || compareSynthesisPasses(candidate, current) < 0 {
			best[topology] = candidate
		}
	}
	result := make([]synthesisPassingCandidate, 0, len(best))
	for _, candidate := range best {
		result = append(result, candidate)
	}
	return result
}

func compareSynthesisPasses(left, right synthesisPassingCandidate) int {
	return cmp.Or(
		cmp.Compare(right.margin, left.margin),
		cmp.Compare(left.repairCount, right.repairCount),
		cmp.Compare(len(left.graph.Instances), len(right.graph.Instances)),
		cmp.Compare(internalNodeCount(left.graph), internalNodeCount(right.graph)),
		cmp.Compare(mustTopologyHash(left.graph), mustTopologyHash(right.graph)),
		cmp.Compare(left.evaluation.Hash, right.evaluation.Hash),
		cmp.Compare(synthesisValueHash(left), synthesisValueHash(right)),
	)
}

func synthesisValueHash(candidate synthesisPassingCandidate) string {
	if candidate.trial != nil && candidate.trial.Hash != "" {
		return candidate.trial.Hash
	}
	return candidate.evaluation.ValueTrialHash
}

func synthesisWorstNormalizedMargin(evaluation SimulationEvaluation) float64 {
	const scaleFloor = 1e-15
	worst := math.Inf(1)
	for _, attempt := range evaluation.Attempts {
		if attempt.Actual == nil {
			continue
		}
		actual := *attempt.Actual
		scale := math.Max(scaleFloor, math.Abs(actual))
		if attempt.RequiredMin != nil {
			scale = math.Max(scale, math.Abs(*attempt.RequiredMin))
			worst = math.Min(worst, (actual-*attempt.RequiredMin)/scale)
		}
		if attempt.RequiredMax != nil {
			scale = math.Max(scale, math.Abs(*attempt.RequiredMax))
			worst = math.Min(worst, (*attempt.RequiredMax-actual)/scale)
		}
	}
	if math.IsInf(worst, 1) || math.IsNaN(worst) {
		return 0
	}
	return worst
}

func bestSynthesisFailures(source []synthesisFailure) []synthesisFailure {
	result := make([]synthesisFailure, 0, len(source))
	seenCandidate := map[int]bool{}
	for _, failure := range source {
		if seenCandidate[failure.candidateIndex] {
			continue
		}
		seenCandidate[failure.candidateIndex] = true
		result = append(result, failure)
	}
	return result
}

func synthesisAttempt(
	number int,
	graph any,
	trial *ValueTrial,
	evaluation SimulationEvaluation,
	repair *Repair,
) Attempt {
	graphHash := ""
	switch typed := graph.(type) {
	case CandidateGraph:
		graphHash, _ = GraphHash(typed)
	case string:
		graphHash = typed
	}
	valueHash := ""
	if trial != nil {
		valueHash = trial.Hash
	}
	return Attempt{
		Number:         number,
		GraphHash:      graphHash,
		ValueHash:      valueHash,
		EvaluationHash: evaluation.Hash,
		Diagnoses:      append([]Diagnosis(nil), evaluation.Diagnoses...),
		Repair:         cloneRepair(repair),
		Status:         statusForSimulation(evaluation.Status),
	}
}

func cloneRepair(repair *Repair) *Repair {
	if repair == nil {
		return nil
	}
	copy := *repair
	copy.Changes = append([]GraphChange(nil), repair.Changes...)
	return &copy
}

func cloneValueTrial(trial *ValueTrial) *ValueTrial {
	if trial == nil {
		return nil
	}
	copy := *trial
	copy.Selections = append([]ValueTrialSelection(nil), trial.Selections...)
	return &copy
}

func repairAttemptGraphHash(attempt RepairAttempt) string {
	return attempt.GraphHash
}

func statusForValuePlan(status ValuePlanStatus) Status {
	switch status {
	case ValuePlanUnsupported:
		return StatusUnsupported
	case ValuePlanExhausted:
		return StatusExhausted
	default:
		return StatusFailed
	}
}

func statusForSimulation(status SimulationEvaluationStatus) Status {
	switch status {
	case SimulationEvaluationPassed:
		return StatusPassed
	case SimulationEvaluationUnsupported:
		return StatusUnsupported
	case SimulationEvaluationExhausted:
		return StatusExhausted
	case SimulationEvaluationCanceled:
		return StatusCanceled
	default:
		return StatusFailed
	}
}

func statusForRepair(status RepairSearchStatus) Status {
	switch status {
	case RepairSearchPassed:
		return StatusPassed
	case RepairSearchUnsupported:
		return StatusUnsupported
	case RepairSearchExhausted:
		return StatusExhausted
	case RepairSearchCanceled:
		return StatusCanceled
	default:
		return StatusFailed
	}
}

func synthesisSearchFailure(search TopologySearchResult) (Status, StopReason) {
	switch search.Status {
	case TopologySearchUnsupported:
		return StatusUnsupported, StopPrimitiveUnavailable
	case TopologySearchCanceled:
		return StatusCanceled, StopCanceled
	case TopologySearchExhausted:
		return StatusExhausted, StopNoCompleteGraph
	default:
		return StatusFailed, StopNoCompleteGraph
	}
}

func remainingSynthesisPolicy(policy Policy, consumed Consumption) Policy {
	result := policy
	result.MaxCandidateSimulations = max(0, policy.MaxCandidateSimulations-consumed.CandidateSimulations)
	result.MaxCornerEvaluations = max(0, policy.MaxCornerEvaluations-consumed.CornerEvaluations)
	result.MaxValueTrials = max(0, policy.MaxValueTrials-consumed.ValueTrials)
	result.MaxTopologyRepairs = max(0, policy.MaxTopologyRepairs-consumed.TopologyRepairs)
	return result
}

func synthesisSimulationBudgetExhausted(consumed Consumption, policy Policy) bool {
	return consumed.CandidateSimulations >= policy.MaxCandidateSimulations ||
		consumed.CornerEvaluations >= policy.MaxCornerEvaluations
}

func synthesisRepairBudgetExhausted(consumed Consumption, policy Policy) bool {
	return synthesisSimulationBudgetExhausted(consumed, policy) ||
		consumed.ValueTrials >= policy.MaxValueTrials ||
		consumed.TopologyRepairs >= policy.MaxTopologyRepairs
}

func addConsumption(target *Consumption, value Consumption) {
	target.ExpandedStates += value.ExpandedStates
	target.GeneratedGraphs += value.GeneratedGraphs
	target.CompleteGraphs += value.CompleteGraphs
	addSimulationConsumption(target, value)
	addRepairConsumption(target, value)
	target.MaximumFrontier = max(target.MaximumFrontier, value.MaximumFrontier)
	target.BudgetExhausted = target.BudgetExhausted || value.BudgetExhausted
}

func addSimulationConsumption(target *Consumption, value Consumption) {
	target.CandidateSimulations += value.CandidateSimulations
	target.CornerEvaluations += value.CornerEvaluations
}

func addRepairConsumption(target *Consumption, value Consumption) {
	target.ValueTrials += value.ValueTrials
	target.TopologyRepairs += value.TopologyRepairs
	target.MaximumFrontier = max(target.MaximumFrontier, value.MaximumFrontier)
	target.BudgetExhausted = target.BudgetExhausted || value.BudgetExhausted
}

func appendSynthesisDiagnostics(report *Report, issues []reports.Issue) {
	for _, issue := range issues {
		report.Diagnostics = append(report.Diagnostics, Diagnostic{
			Code: issue.Code, Path: issue.Path, Message: issue.Message, Suggestion: issue.Suggestion,
		})
	}
	slices.SortFunc(report.Diagnostics, func(left, right Diagnostic) int {
		return cmp.Or(
			cmp.Compare(left.Code, right.Code),
			cmp.Compare(left.Path, right.Path),
			cmp.Compare(left.Message, right.Message),
			cmp.Compare(left.Suggestion, right.Suggestion),
		)
	})
	report.Diagnostics = slices.Compact(report.Diagnostics)
}

func mustTopologyHash(graph CandidateGraph) string {
	hash, _ := TopologyHash(graph)
	return hash
}

func finalizeSynthesisRun(run SynthesisRun) SynthesisRun {
	if run.Report.Status == StatusPassed &&
		(run.SelectedGraph == nil || run.Physical == nil ||
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
	run.Hash = hashJSON(copy)
	return run
}
