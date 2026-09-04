package opentopologysynthesis

import (
	"context"
	"fmt"

	"kicadai/internal/reports"
	"kicadai/internal/simulationadmission"
)

type AdmittedSynthesisResultV21 struct {
	Synthesis SynthesisRun                 `json:"synthesis"`
	Admission simulationadmission.Decision `json:"admission"`
}

func SynthesizeV21(ctx context.Context, requirement Requirement, inventory PrimitiveInventory, environment SimulationEnvironment, policy Policy) SynthesisRun {
	return SynthesizeV21WithLegacy(
		ctx, requirement,
		inventory, environment,
		inventory, environment,
		inventory, environment,
		inventory, environment,
		policy,
	)
}

// SynthesizeV21WithLegacy executes the exact V20 boundary first. Ineligible,
// passing, unsafe, invalid, and canceled results are returned byte-for-byte.
func SynthesizeV21WithLegacy(
	ctx context.Context,
	requirement Requirement,
	v21Inventory PrimitiveInventory,
	v21Simulation SimulationEnvironment,
	v20Inventory PrimitiveInventory,
	v20Simulation SimulationEnvironment,
	v18Inventory PrimitiveInventory,
	v18Simulation SimulationEnvironment,
	legacyInventory PrimitiveInventory,
	legacySimulation SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	v20 := SynthesizeV20WithLegacy(
		ctx, requirement,
		v20Inventory, v20Simulation,
		v18Inventory, v18Simulation,
		legacyInventory, legacySimulation,
		policy,
	)
	return synthesizeTopologyCompletionV21(
		ctx, requirement, v20, v21Inventory, v21Simulation,
		simulationadmission.PrepareEnvironment(bundledAdmissionEnvironmentV20(v21Simulation)), policy,
	)
}

func SynthesizeAdmittedV21(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.Environment,
	policy Policy,
) AdmittedSynthesisResultV21 {
	prepared := simulationadmission.PrepareEnvironment(admission)
	decision := simulationadmission.AdmitPrepared(requirementSimulationAdmissionRequest(Normalize(requirement), inventory), prepared)
	v20 := synthesizeAdmissionPreparedV20(ctx, requirement, inventory, environment, prepared, policy)
	return AdmittedSynthesisResultV21{
		Synthesis: synthesizeTopologyCompletionV21(ctx, requirement, v20, inventory, environment, prepared, policy),
		Admission: decision,
	}
}

func synthesizeTopologyCompletionV21(
	ctx context.Context,
	requirement Requirement,
	v20 SynthesisRun,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.PreparedEnvironment,
	policy Policy,
) SynthesisRun {
	if ctx.Err() != nil {
		return v20
	}
	normalized := Normalize(requirement)
	if !topologyCompletionStatusFrontierV21(normalized, v20) {
		return v20
	}
	requirement = normalized
	base, eligible := causalRepairBaseV19(requirement, v20, inventory)
	if !eligible {
		return v20
	}
	decision := simulationadmission.AdmitPrepared(requirementSimulationAdmissionRequest(requirement, inventory), admission)
	if decision.Status != simulationadmission.StatusAdmitted {
		run := v20
		run.Report.Status = StatusUnsupported
		run.Report.StopReason = StopRepairUnsupported
		run.Report.Diagnostics = []Diagnostic{}
		for _, diagnostic := range decision.Diagnostics {
			run.Report.Diagnostics = append(run.Report.Diagnostics, Diagnostic{
				Code: reports.Code(diagnostic.Code), Path: "synthesis.v21.admission." + diagnostic.Path,
				Message: diagnostic.Message, Suggestion: diagnostic.Suggestion,
			})
		}
		return finalizeSynthesisRunV17(run)
	}
	initial := evaluateCandidatePreparedV20(ctx, requirement, base.graph, nil, inventory, environment, admission, policy)
	if initial.Status == SimulationEvaluationCanceled || ctx.Err() != nil {
		return v20
	}
	repair := repairCandidatePreparedV21(ctx, requirement, base.graph, initial, inventory, environment, admission, policy)
	run := v20
	run.Report.Consumption.ExpandedStates += repair.Consumption.ExpandedStates
	run.Report.Consumption.GeneratedGraphs += repair.Consumption.GeneratedGraphs
	addSimulationConsumption(&run.Report.Consumption, initial.Consumption)
	addSimulationConsumption(&run.Report.Consumption, repair.Consumption)
	addRepairConsumption(&run.Report.Consumption, repair.Consumption)
	if base.candidateIndex >= 0 && base.candidateIndex < len(run.Candidates) && base.candidateIndex < len(run.Report.Candidates) {
		run.Candidates[base.candidateIndex].Evaluations = append(run.Candidates[base.candidateIndex].Evaluations, initial)
		run.Candidates[base.candidateIndex].Repair = &repair
		run.Report.Candidates[base.candidateIndex].Attempts = append(run.Report.Candidates[base.candidateIndex].Attempts, synthesisAttempt(len(run.Report.Candidates[base.candidateIndex].Attempts)+1, base.graph, base.trial, initial, nil))
		for _, repairAttempt := range repair.Attempts {
			run.Candidates[base.candidateIndex].Evaluations = append(run.Candidates[base.candidateIndex].Evaluations, repairAttempt.Evaluation)
			run.Report.Candidates[base.candidateIndex].Attempts = append(run.Report.Candidates[base.candidateIndex].Attempts, synthesisAttempt(len(run.Report.Candidates[base.candidateIndex].Attempts)+1, repairAttemptGraphHash(repairAttempt), repairAttempt.ValueTrial, repairAttempt.Evaluation, &repairAttempt.Repair))
		}
		run.Report.Candidates[base.candidateIndex].Status = statusForRepair(repair.Status)
	}
	if repair.Status == RepairSearchPassed && repair.Selected != nil {
		selected := repair.Selected
		selected.ValueTrial = cloneValueTrial(base.trial)
		physical := LowerPassingCandidate(ctx, requirement, selected.Graph, selected.Evaluation, inventory, environment)
		if base.candidateIndex >= 0 && base.candidateIndex < len(run.Candidates) {
			run.Candidates[base.candidateIndex].Physical = append(run.Candidates[base.candidateIndex].Physical, physical)
			run.Candidates[base.candidateIndex].Repair = &repair
		}
		if physical.Status != PhysicalLoweringReady {
			run.Report.Status, run.Report.StopReason = StatusFailed, StopPhysicalPromotionFailed
			run.Report.Diagnostics = []Diagnostic{}
			appendSynthesisDiagnostics(&run.Report, physical.Issues)
			return finalizeSynthesisRunV17(run)
		}
		return selectRankedSynthesisResultV17(run, []synthesisPassingCandidate{{
			candidateIndex: base.candidateIndex, graph: selected.Graph, trial: selected.ValueTrial,
			evaluation: selected.Evaluation, repair: &repair, physical: physical,
			margin: synthesisWorstNormalizedMargin(selected.Evaluation), repairCount: repair.Consumption.TopologyRepairs,
		}})
	}
	if repair.Status == RepairSearchCanceled {
		return v20
	}
	if repair.TopologyCompletionV21 != nil && repair.TopologyCompletionV21.Selected != nil && repair.TopologyCompletionV21.Selected.Invariant.Complete &&
		(repair.Status == RepairSearchFailed || repair.Status == RepairSearchUnsupported) {
		run.Report.Status, run.Report.StopReason = StatusFailed, StopNoPassingGraph
		run.Report.Diagnostics = []Diagnostic{{
			Code: CodeNoPassingGraph, Path: "synthesis.v21.topology_certificate",
			Message:    fmt.Sprintf("V21 causal topology certificate %s is complete; the admitted candidate remains behaviorally nonpassing", repair.TopologyCompletionV21.Selected.Invariant.Hash),
			Suggestion: "continue with the typed downstream model, solver, value, or behavioral diagnosis",
		}}
		return finalizeSynthesisRunV17(run)
	}
	run.Report.Status, run.Report.StopReason = statusForRepair(repair.Status), StopRepairExhausted
	run.Report.Diagnostics = []Diagnostic{}
	appendSynthesisDiagnostics(&run.Report, repair.Issues)
	return finalizeSynthesisRunV17(run)
}

func topologyCompletionStatusFrontierV21(requirement Requirement, run SynthesisRun) bool {
	if run.Report.Status == StatusPassed || run.Report.Status == StatusCanceled || run.Report.Status == StatusInvalid || run.Report.Status == StatusInfeasible {
		return false
	}
	if allCandidateFailuresCriticalV19(requirement, run) || universalDiagnosisExistsV19(run) {
		return false
	}
	switch run.Report.StopReason {
	case StopRepairExhausted, StopRepairUnsupported:
		// A repair stop reason can be retained as a downstream diagnostic beneath
		// a stronger simulation, model, solver, or safety frontier. Only a direct,
		// homogeneous V20 topology frontier is eligible for V21 completion.
		return causalTopologyRepairFrontierV19(requirement, run)
	case StopSearchExhausted, StopNoCompleteGraph:
		if len(run.Report.Diagnostics) == 0 {
			return true
		}
		for _, diagnostic := range run.Report.Diagnostics {
			if diagnostic.Code != CodeSearchExhausted && diagnostic.Code != CodeNoCompleteGraph {
				return false
			}
		}
		return true
	default:
		return false
	}
}
