package opentopologysynthesis

import (
	"cmp"
	"context"
	"slices"
)

// SynthesizeV19 keeps one convenience entry point for non-evaluator callers.
// The versioned evaluator uses SynthesizeV19WithLegacy so all three inventory
// and simulation boundaries are bound independently.
func SynthesizeV19(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	return SynthesizeV19WithLegacy(
		ctx, requirement,
		inventory, environment,
		inventory, environment,
		inventory, environment,
		policy,
	)
}

// SynthesizeV19WithLegacy always executes the exact V18 constructor first.
// Unless every frozen eligibility predicate succeeds, it returns those exact
// bytes without rebuilding, rehashing, or annotating the V18 run.
func SynthesizeV19WithLegacy(
	ctx context.Context,
	requirement Requirement,
	v19Inventory PrimitiveInventory,
	v19Simulation SimulationEnvironment,
	v18Inventory PrimitiveInventory,
	v18Simulation SimulationEnvironment,
	legacyInventory PrimitiveInventory,
	legacySimulation SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	v18 := SynthesizeV18WithLegacy(
		ctx, requirement,
		v18Inventory, v18Simulation,
		legacyInventory, legacySimulation,
		policy,
	)
	normalized := Normalize(requirement)
	base, eligible := causalRepairBaseV19(normalized, v18, v18Inventory)
	if !eligible || !causalTopologyRepairFrontierV19(normalized, v18) ||
		len(Validate(normalized)) != 0 || len(requirementFeasibilityIssues(normalized)) != 0 ||
		len(requirementCapabilityIssues(normalized, v19Inventory)) != 0 {
		return v18
	}
	if ctx.Err() != nil {
		return v18
	}

	run := v18
	remaining := remainingSynthesisPolicy(effectiveTopologyPolicy(policy), run.Report.Consumption)
	if remaining.MaxCandidateSimulations <= 0 || remaining.MaxCornerEvaluations <= 0 {
		run.Report.Consumption.BudgetExhausted = true
		run.Report.Status, run.Report.StopReason = StatusExhausted, StopRepairExhausted
		run.Report.Diagnostics = []Diagnostic{{
			Code: CodeRepairExhausted, Path: "synthesis.v19",
			Message:    "the exact V18 run consumed the frozen simulation budget before V19 base evaluation",
			Suggestion: "retain the frozen budget and inspect the V18 causal frontier",
		}}
		return finalizeSynthesisRunV17(run)
	}

	initial := EvaluateCandidateV18(ctx, normalized, base.graph, nil, v19Inventory, v19Simulation, remaining)
	addSimulationConsumption(&run.Report.Consumption, initial.Consumption)
	if base.candidateIndex >= 0 && base.candidateIndex < len(run.Candidates) && base.candidateIndex < len(run.Report.Candidates) {
		run.Candidates[base.candidateIndex].Evaluations = append(run.Candidates[base.candidateIndex].Evaluations, initial)
		run.Report.Candidates[base.candidateIndex].Attempts = append(
			run.Report.Candidates[base.candidateIndex].Attempts,
			synthesisAttempt(len(run.Report.Candidates[base.candidateIndex].Attempts)+1, base.graph, base.trial, initial, nil),
		)
		run.Report.Candidates[base.candidateIndex].Status = statusForSimulation(initial.Status)
	}
	if initial.Status == SimulationEvaluationCanceled || ctx.Err() != nil {
		run.Report.Status, run.Report.StopReason = StatusCanceled, StopCanceled
		appendSynthesisDiagnostics(&run.Report, initial.Issues)
		return finalizeSynthesisRunV17(run)
	}
	if initial.Status != SimulationEvaluationFailed || len(initial.Diagnoses) == 0 {
		run.Report.Status, run.Report.StopReason = statusForV19InitialEvaluation(initial.Status)
		run.Report.Diagnostics = []Diagnostic{{
			Code: CodeRepairUnsupported, Path: "synthesis.v19.base_evaluation",
			Message:    "the replayed V18 base graph did not produce a diagnosed V19 repair seed",
			Suggestion: "preserve the V18 result unless the committed V19 environment can replay its diagnosis",
		}}
		return finalizeSynthesisRunV17(run)
	}

	repairPolicy := remainingSynthesisPolicy(effectiveTopologyPolicy(policy), run.Report.Consumption)
	repair := RepairCandidateV19(ctx, normalized, base.graph, initial, v19Inventory, v19Simulation, repairPolicy)
	if base.candidateIndex >= 0 && base.candidateIndex < len(run.Candidates) && base.candidateIndex < len(run.Report.Candidates) {
		run.Candidates[base.candidateIndex].Repair = &repair
		for _, repairAttempt := range repair.Attempts {
			run.Report.Candidates[base.candidateIndex].Attempts = append(
				run.Report.Candidates[base.candidateIndex].Attempts,
				synthesisAttempt(
					len(run.Report.Candidates[base.candidateIndex].Attempts)+1,
					repairAttemptGraphHash(repairAttempt), repairAttempt.ValueTrial,
					repairAttempt.Evaluation, &repairAttempt.Repair,
				),
			)
		}
		run.Report.Candidates[base.candidateIndex].Status = statusForRepair(repair.Status)
	}
	addSimulationConsumption(&run.Report.Consumption, repair.Consumption)
	addRepairConsumption(&run.Report.Consumption, repair.Consumption)
	if repair.Status == RepairSearchCanceled || ctx.Err() != nil {
		run.Report.Status, run.Report.StopReason = StatusCanceled, StopCanceled
		appendSynthesisDiagnostics(&run.Report, repair.Issues)
		return finalizeSynthesisRunV17(run)
	}
	if repair.Status != RepairSearchPassed || repair.Selected == nil {
		run.Report.Status, run.Report.StopReason = statusForV19Repair(repair.Status)
		run.Report.Diagnostics = []Diagnostic{}
		appendSynthesisDiagnostics(&run.Report, repair.Issues)
		return finalizeSynthesisRunV17(run)
	}

	selected := repair.Selected
	selected.ValueTrial = cloneValueTrial(base.trial)
	physical := LowerPassingCandidate(ctx, normalized, selected.Graph, selected.Evaluation, v19Inventory, v19Simulation)
	if base.candidateIndex >= 0 && base.candidateIndex < len(run.Candidates) {
		run.Candidates[base.candidateIndex].Physical = append(run.Candidates[base.candidateIndex].Physical, physical)
	}
	if physical.Status != PhysicalLoweringReady {
		run.Report.Status, run.Report.StopReason = StatusFailed, StopPhysicalPromotionFailed
		run.Report.Diagnostics = []Diagnostic{}
		appendSynthesisDiagnostics(&run.Report, physical.Issues)
		return finalizeSynthesisRunV17(run)
	}
	repair.Selected = selected
	if base.candidateIndex >= 0 && base.candidateIndex < len(run.Candidates) {
		run.Candidates[base.candidateIndex].Repair = &repair
	}
	return selectRankedSynthesisResultV17(run, []synthesisPassingCandidate{{
		candidateIndex: base.candidateIndex, graph: selected.Graph,
		trial: selected.ValueTrial, evaluation: selected.Evaluation,
		repair: &repair, physical: physical,
		margin:      synthesisWorstNormalizedMargin(selected.Evaluation),
		repairCount: repair.Consumption.TopologyRepairs,
	}})
}

type causalRepairBaseSelectionV19 struct {
	candidateIndex int
	graph          CandidateGraph
	trial          *ValueTrial
	evaluation     SimulationEvaluation
	penalty        float64
}

func causalRepairBaseV19(requirement Requirement, run SynthesisRun, inventory PrimitiveInventory) (causalRepairBaseSelectionV19, bool) {
	candidates := []causalRepairBaseSelectionV19{}
	for candidateIndex := range run.Candidates {
		if candidateIndex >= len(run.Search.Candidates) {
			continue
		}
		evidence := run.Candidates[candidateIndex]
		if evidence.Repair == nil || evidence.Repair.InitialGraphHash == "" || evidence.Repair.InitialEvaluationHash == "" ||
			evidence.ValuePlan.Status != ValuePlanReady {
			continue
		}
		enumeration := EnumerateValueTrials(evidence.ValuePlan, max(1, len(evidence.Evaluations)))
		for evaluationIndex, evaluation := range evidence.Evaluations {
			if evaluationIndex >= len(enumeration.Trials) || evaluation.Hash != evidence.Repair.InitialEvaluationHash ||
				evaluation.GraphHash != evidence.Repair.InitialGraphHash || evaluation.Status != SimulationEvaluationFailed || len(evaluation.Diagnoses) == 0 {
				continue
			}
			trial := enumeration.Trials[evaluationIndex]
			graph, err := ApplyValueTrial(run.Search.Candidates[candidateIndex].Graph, trial, inventory)
			if err != nil {
				continue
			}
			graphHash, err := GraphHash(graph)
			if err != nil || graphHash != evaluation.GraphHash {
				continue
			}
			candidates = append(candidates, causalRepairBaseSelectionV19{
				candidateIndex: candidateIndex, graph: graph, trial: cloneValueTrial(&trial),
				evaluation: evaluation, penalty: simulationEvaluationPenalty(evaluation),
			})
		}
	}
	if len(candidates) == 0 {
		return causalRepairBaseSelectionV19{}, false
	}
	slices.SortFunc(candidates, func(left, right causalRepairBaseSelectionV19) int {
		leftHash, _ := GraphHash(left.graph)
		rightHash, _ := GraphHash(right.graph)
		return cmp.Or(cmp.Compare(left.penalty, right.penalty), cmp.Compare(left.candidateIndex, right.candidateIndex), cmp.Compare(leftHash, rightHash))
	})
	return candidates[0], true
}

func causalTopologyRepairFrontierV19(requirement Requirement, run SynthesisRun) bool {
	if run.Report.Status == StatusPassed || run.Report.Status == StatusCanceled || run.Report.Status == StatusInvalid ||
		run.Report.Status == StatusInfeasible || allCandidateFailuresCriticalV19(requirement, run) || universalDiagnosisExistsV19(run) {
		return false
	}
	codes := []reportsCodeV19{}
	for _, diagnostic := range run.Report.Diagnostics {
		codes = append(codes, reportsCodeV19(diagnostic.Code))
	}
	if len(codes) == 0 {
		switch run.Report.StopReason {
		case StopRepairExhausted:
			codes = append(codes, reportsCodeV19(CodeRepairExhausted))
		case StopRepairUnsupported:
			codes = append(codes, reportsCodeV19(CodeRepairUnsupported))
		default:
			return false
		}
	}
	for _, code := range codes {
		if code != reportsCodeV19(CodeRepairExhausted) && code != reportsCodeV19(CodeRepairUnsupported) {
			return false
		}
	}
	return true
}

type reportsCodeV19 string

func universalDiagnosisExistsV19(run SynthesisRun) bool {
	if len(run.Candidates) == 0 {
		return false
	}
	counts := map[string]int{}
	for _, candidate := range run.Candidates {
		seen := map[string]bool{}
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				key := diagnosis.Code + "\x00" + diagnosis.Analysis + "\x00" + diagnosis.Metric
				seen[key] = true
			}
		}
		for key := range seen {
			counts[key]++
		}
	}
	for _, count := range counts {
		if count == len(run.Candidates) {
			return true
		}
	}
	return false
}

func allCandidateFailuresCriticalV19(requirement Requirement, run SynthesisRun) bool {
	critical := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		critical[assertion.ID] = assertion.Critical
	}
	found := false
	for _, candidate := range run.Candidates {
		candidateFound := false
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				if (diagnosis.Code != "assertion_below_minimum" && diagnosis.Code != "assertion_above_maximum") || !critical[diagnosis.RequirementID] {
					return false
				}
				found, candidateFound = true, true
			}
		}
		if !candidateFound {
			return false
		}
	}
	return found
}

func statusForV19InitialEvaluation(status SimulationEvaluationStatus) (Status, StopReason) {
	switch status {
	case SimulationEvaluationCanceled:
		return StatusCanceled, StopCanceled
	case SimulationEvaluationExhausted:
		return StatusExhausted, StopRepairExhausted
	default:
		return StatusUnsupported, StopRepairUnsupported
	}
}

func statusForV19Repair(status RepairSearchStatus) (Status, StopReason) {
	switch status {
	case RepairSearchCanceled:
		return StatusCanceled, StopCanceled
	case RepairSearchExhausted:
		return StatusExhausted, StopRepairExhausted
	case RepairSearchUnsupported:
		return StatusUnsupported, StopRepairUnsupported
	default:
		return StatusFailed, StopRepairExhausted
	}
}
