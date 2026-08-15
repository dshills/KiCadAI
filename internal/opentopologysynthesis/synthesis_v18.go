package opentopologysynthesis

import "context"

// SynthesizeV18 preserves the V17 path for requirements outside the bounded
// V18 low-voltage multi-output threshold capability. Eligible requirements use
// the V18 search, value, and evaluation adapters and the existing physical
// promotion boundary.
func SynthesizeV18(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	return SynthesizeV18WithLegacy(
		ctx,
		requirement,
		inventory,
		environment,
		inventory,
		environment,
		policy,
	)
}

// SynthesizeV18WithLegacy keeps the extension inputs separate from the exact
// historical inputs used by V17. Requirements outside the bounded extension
// therefore preserve the historical result even when the V18 catalog contains
// additional reviewed primitives. Eligible requirements use the extension and
// fall back only to the same historical V17 environment.
func SynthesizeV18WithLegacy(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	legacyInventory PrimitiveInventory,
	legacySimulation SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	requirement = Normalize(requirement)
	if !v18RequiresThresholdExtension(requirement) {
		return SynthesizeV17(ctx, requirement, legacyInventory, legacySimulation, policy)
	}
	run := synthesizeThresholdExtensionV18(ctx, requirement, inventory, environment, policy)
	if run.Report.Status == StatusPassed || ctx.Err() != nil {
		return run
	}
	// An eligible requirement may still be covered by a legacy architecture.
	// Preserve that result rather than turning a V17 pass into a V18 regression.
	legacy := SynthesizeV17(ctx, requirement, legacyInventory, legacySimulation, policy)
	if legacy.Report.Status == StatusPassed {
		return legacy
	}
	return run
}

func v18RequiresThresholdExtension(requirement Requirement) bool {
	sourceOutputs := map[string]bool{}
	hasLowerThreshold := false
	hasUpperThreshold := false
	hasHighInputImpedance := false
	for _, port := range requirement.Requirements.Ports {
		if port.Direction == "source" && port.Kind != "power" && port.Kind != "reference" {
			sourceOutputs[port.ID] = true
		}
	}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		switch assertion.Metric {
		case "lower_threshold", "lower_threshold_voltage":
			hasLowerThreshold = assertion.Min != nil || assertion.Max != nil
		case "upper_threshold", "upper_threshold_voltage":
			hasUpperThreshold = assertion.Min != nil || assertion.Max != nil
		case "input_impedance":
			hasHighInputImpedance = assertion.Min != nil && *assertion.Min >= 1e6
		}
	}
	return len(sourceOutputs) >= 2 && hasLowerThreshold && hasUpperThreshold && hasHighInputImpedance
}

func synthesizeThresholdExtensionV18(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	policy = effectiveTopologyPolicy(policy)
	report := Report{
		Schema: ReportSchema, Version: ReportVersion,
		PolicyVersion: PolicyVersion, Policy: policy,
		Status: StatusFailed, StopReason: StopNoPassingGraph,
		Candidates: []CandidateReport{}, Diagnostics: []Diagnostic{},
	}
	report.PolicyHash, _ = PolicyHash(policy)
	report.RequirementHash, _ = CanonicalHash(requirement)
	report.PrimitiveInventoryHash = inventory.Hash
	report.CatalogHash = inventory.CatalogHash
	report.ModelRegistryHash = inventory.ModelRegistryHash
	run := SynthesisRun{
		Schema: SynthesisRunSchema, Version: SynthesisRunVersion,
		Report: report, Candidates: []SynthesisCandidateEvidence{},
	}
	if issues := Validate(requirement); len(issues) != 0 {
		run.Report.Status, run.Report.StopReason = StatusInvalid, StopRequirementInvalid
		appendSynthesisDiagnostics(&run.Report, issues)
		return finalizeSynthesisRunV17(run)
	}
	if issues := requirementFeasibilityIssues(requirement); len(issues) != 0 {
		run.Report.Status, run.Report.StopReason = StatusInfeasible, StopRequirementInfeasible
		appendSynthesisDiagnostics(&run.Report, issues)
		return finalizeSynthesisRunV17(run)
	}
	if issues := requirementCapabilityIssues(requirement, inventory); len(issues) != 0 {
		run.Report.Status, run.Report.StopReason = StatusUnsupported, StopModelUnavailable
		appendSynthesisDiagnostics(&run.Report, issues)
		return finalizeSynthesisRunV17(run)
	}
	if ctx.Err() != nil {
		run.Report.Status, run.Report.StopReason = StatusCanceled, StopCanceled
		return finalizeSynthesisRunV17(run)
	}

	run.Search = SearchPrimitiveTopologiesV18(ctx, requirement, inventory, policy)
	addConsumption(&run.Report.Consumption, run.Search.Consumption)
	if len(run.Search.Candidates) == 0 {
		run.Report.Status, run.Report.StopReason = synthesisSearchFailure(run.Search)
		appendSynthesisDiagnostics(&run.Report, run.Search.Issues)
		return finalizeSynthesisRunV17(run)
	}

	type candidateTrial struct {
		candidateIndex int
		trial          ValueTrial
	}
	trialSets := make([][]candidateTrial, len(run.Search.Candidates))
	valueBudget := min(policy.MaxValueTrials, max(len(run.Search.Candidates), policy.MaxValueTrials/2))
	perCandidate := max(1, valueBudget/max(1, len(run.Search.Candidates)))
	for candidateIndex, candidate := range run.Search.Candidates {
		plan := BuildValueSearchPlan(requirement, candidate.Graph, inventory, policy)
		run.Candidates = append(run.Candidates, SynthesisCandidateEvidence{
			Fingerprint: candidate.Fingerprint, TopologyHash: candidate.TopologyHash,
			ActiveStructureHash: candidate.ActiveStructureHash, ValuePlan: plan,
			Evaluations: []SimulationEvaluation{}, Physical: []PhysicalLoweringResult{},
		})
		run.Report.Candidates = append(run.Report.Candidates, CandidateReport{
			Fingerprint: candidate.Fingerprint, TopologyHash: candidate.TopologyHash,
			ActiveStructureHash: candidate.ActiveStructureHash,
			ComponentCount:      len(candidate.Graph.Instances), InternalNodes: internalNodeCount(candidate.Graph),
			Status: StatusFailed, Attempts: []Attempt{},
		})
		if plan.Status != ValuePlanReady {
			run.Report.Candidates[candidateIndex].Status = statusForValuePlan(plan.Status)
			appendSynthesisDiagnostics(&run.Report, plan.Issues)
			continue
		}
		enumeration := EnumerateValueTrialsV18(plan, requirement, candidate.Graph, inventory, perCandidate)
		for _, trial := range enumeration.Trials {
			if _, err := ApplyValueTrial(candidate.Graph, trial, inventory); err == nil {
				trialSets[candidateIndex] = append(trialSets[candidateIndex], candidateTrial{candidateIndex, trial})
			}
		}
	}

	maximumTrials := 0
	for _, trials := range trialSets {
		maximumTrials = max(maximumTrials, len(trials))
	}
	order := synthesisCandidateEvaluationOrder(requirement, inventory, run.Search.Candidates)
	evaluationPolicy := synthesisInitialEvaluationPolicy(policy, len(order))
	for trialIndex := 0; trialIndex < maximumTrials; trialIndex++ {
		for _, candidateIndex := range order {
			if trialIndex >= len(trialSets[candidateIndex]) {
				continue
			}
			if ctx.Err() != nil {
				run.Report.Status, run.Report.StopReason = StatusCanceled, StopCanceled
				return finalizeSynthesisRunV17(run)
			}
			if synthesisSimulationBudgetExhausted(run.Report.Consumption, evaluationPolicy) ||
				run.Report.Consumption.ValueTrials >= evaluationPolicy.MaxValueTrials {
				run.Report.Consumption.BudgetExhausted = true
				break
			}
			work := trialSets[candidateIndex][trialIndex]
			graph, err := ApplyValueTrial(run.Search.Candidates[candidateIndex].Graph, work.trial, inventory)
			if err != nil {
				continue
			}
			candidatePolicy := synthesisCandidateEvaluationPolicy(policy, evaluationPolicy, run.Report.Consumption)
			evaluation := EvaluateCandidateV18(ctx, requirement, graph, nil, inventory, environment, candidatePolicy)
			run.Report.Consumption.ValueTrials++
			addSimulationConsumption(&run.Report.Consumption, evaluation.Consumption)
			run.Candidates[candidateIndex].Evaluations = append(run.Candidates[candidateIndex].Evaluations, evaluation)
			run.Report.Candidates[candidateIndex].Attempts = append(
				run.Report.Candidates[candidateIndex].Attempts,
				synthesisAttempt(len(run.Report.Candidates[candidateIndex].Attempts)+1, graph, &work.trial, evaluation, nil),
			)
			run.Report.Candidates[candidateIndex].Status = statusForSimulation(evaluation.Status)
			if evaluation.Status != SimulationEvaluationPassed {
				continue
			}
			physical := LowerPassingCandidate(ctx, requirement, graph, evaluation, inventory, environment)
			run.Candidates[candidateIndex].Physical = append(run.Candidates[candidateIndex].Physical, physical)
			if physical.Status != PhysicalLoweringReady {
				appendSynthesisDiagnostics(&run.Report, physical.Issues)
				continue
			}
			return selectRankedSynthesisResultV17(run, []synthesisPassingCandidate{{
				candidateIndex: candidateIndex, graph: graph, trial: &work.trial,
				evaluation: evaluation, physical: physical,
				margin: synthesisWorstNormalizedMargin(evaluation),
			}})
		}
	}
	run.Report.Status = StatusFailed
	if run.Report.Consumption.BudgetExhausted {
		run.Report.Status, run.Report.StopReason = StatusExhausted, StopSearchExhausted
	}
	if len(run.Report.Diagnostics) == 0 {
		run.Report.Diagnostics = []Diagnostic{{
			Code: CodeNoPassingGraph, Path: "synthesis.v18",
			Message:    "bounded V18 topology and value search produced no physically promotable candidate",
			Suggestion: "inspect retained V18 simulation and physical-lowering diagnoses",
		}}
	}
	return finalizeSynthesisRunV17(run)
}
