package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/reports"
)

const (
	causalMaximumDepthV19               = 4
	causalMaximumBeamWidthV19           = 8
	causalMaximumEvaluatedCandidatesV19 = 48
	causalBaseDepthQuotaV19             = 12
	causalMaximumPathChangesV19         = 8
	causalMaximumPlateauPerParentV19    = 2
	causalEpsilonV19                    = 1e-12
)

// CausalStructuralVectorV19 is ordered exactly as the frozen V19 monotonic
// structural condition. Smaller values are better in every field.
type CausalStructuralVectorV19 struct {
	MissingBindings         int `json:"missing_bindings"`
	UnallocatedCones        int `json:"unallocated_cones"`
	MissingTypedFeedback    int `json:"missing_typed_feedback"`
	UnreachableObservations int `json:"unreachable_observations"`
}

type causalBeamStateV19 struct {
	graph             CandidateGraph
	context           CausalInvariantContextV19
	evaluation        SimulationEvaluation
	repairs           []Repair
	history           []CausalLogicalOperationV19
	historyKey        string
	graphHash         string
	depth             int
	logicalChanges    int
	addedPrimitives   int
	addedInternal     int
	structural        CausalStructuralVectorV19
	criticalFailures  int
	totalFailures     int
	worstViolation    float64
	diagnosisMargin   float64
	reachableObserved bool
	plateau           bool
}

type causalBeamHooksV19 struct {
	propose    func(causalBeamStateV19, CausalOperationBudgetV19) (CausalOperationBatchV19, error)
	evaluate   func(context.Context, CandidateGraph, Policy) SimulationEvaluation
	validate   func(CandidateGraph, CausalInvariantContextV19) []reports.Issue
	structural func(CandidateGraph, CausalInvariantContextV19) CausalStructuralVectorV19
}

// RepairCandidateV19 executes only the V19 bounded compositional repair. The
// caller is responsible for the exact V18-first eligibility boundary.
func RepairCandidateV19(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) RepairSearchResult {
	requirement = Normalize(requirement)
	limits := GraphLimits{MaxPrimitiveInstances: policy.MaxPrimitiveInstances, MaxInternalNodes: policy.MaxInternalNodes}
	hooks := causalBeamHooksV19{}
	hooks.validate = func(candidate CandidateGraph, candidateContext CausalInvariantContextV19) []reports.Issue {
		return ValidateCausalGraphV19(requirement, candidate, inventory, limits, candidateContext)
	}
	hooks.structural = func(candidate CandidateGraph, candidateContext CausalInvariantContextV19) CausalStructuralVectorV19 {
		return causalStructuralVectorV19(requirement, candidate, inventory, candidateContext)
	}
	hooks.propose = func(state causalBeamStateV19, budget CausalOperationBudgetV19) (CausalOperationBatchV19, error) {
		return causalProposalsV19(requirement, state, inventory, limits, policy, budget)
	}
	hooks.evaluate = func(ctx context.Context, candidate CandidateGraph, remaining Policy) SimulationEvaluation {
		return EvaluateCandidateV18(ctx, requirement, candidate, nil, inventory, environment, remaining)
	}
	return repairCandidateV19WithHooks(ctx, requirement, graph, initial, inventory, policy, hooks)
}

func repairCandidateV19WithHooks(
	ctx context.Context,
	requirement Requirement,
	graph CandidateGraph,
	initial SimulationEvaluation,
	inventory PrimitiveInventory,
	policy Policy,
	hooks causalBeamHooksV19,
) RepairSearchResult {
	policy = effectiveTopologyPolicy(policy)
	result := RepairSearchResult{
		Schema: RepairSearchSchema, Version: RepairSearchVersion,
		PolicyVersion: PolicyVersion, InventoryHash: inventory.Hash,
		InitialEvaluationHash: initial.Hash, Status: RepairSearchFailed,
		Policy: policy, CausalAnalyses: []CausalRepairAnalysis{},
		Attempts: []RepairAttempt{}, Issues: []reports.Issue{},
	}
	requirement = Normalize(requirement)
	requirementHash, err := CanonicalHash(requirement)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeRequirementInvalid, "requirement", "hash V19 repair requirement: "+err.Error(), "")}
		return finalizeRepairSearchV17(result)
	}
	result.RequirementHash = requirementHash
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "normalize V19 repair graph: "+err.Error(), "")}
		return finalizeRepairSearchV17(result)
	}
	result.InitialGraphHash, err = GraphHash(graph)
	if err != nil {
		result.Issues = []reports.Issue{graphIssue(CodeNoCompleteGraph, "graph", "hash V19 repair graph: "+err.Error(), "")}
		return finalizeRepairSearchV17(result)
	}
	if initial.GraphHash != result.InitialGraphHash || initial.RequirementHash != requirementHash ||
		initial.InventoryHash != inventory.Hash || initial.Hash == "" {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "initial_evaluation", "V19 repair requires a hash-bound evaluation of the exact supplied graph, requirement, and inventory", "evaluate the exact V18-selected graph under the V19 environment")}
		return finalizeRepairSearchV17(result)
	}
	if initial.Status == SimulationEvaluationPassed {
		result.Status = RepairSearchPassed
		result.Selected = &RepairedCandidate{Graph: graph, Repairs: []Repair{}, Evaluation: initial}
		return finalizeRepairSearchV17(result)
	}
	if len(initial.Diagnoses) == 0 || hooks.propose == nil || hooks.evaluate == nil || hooks.validate == nil || hooks.structural == nil {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "repair", "V19 repair lacks replayable diagnosis or a complete bounded engine", "retain the diagnosed evaluation and committed V19 engine")}
		return finalizeRepairSearchV17(result)
	}
	if issues := hooks.validate(graph, CausalInvariantContextV19{}); len(issues) != 0 {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "graph", "V18-selected graph fails the V19 pre-simulation invariant boundary", "do not attempt causal repair on an unsafe or incomplete base graph")}
		return finalizeRepairSearchV17(result)
	}
	result.traceDiagnoses = slices.Clone(initial.Diagnoses)
	base := causalNewBeamStateV19(requirement, graph, CausalInvariantContextV19{}, initial, nil, nil, "", 0, 0, 0, 0, hooks.structural)
	frontier := []causalBeamStateV19{base}
	seen := map[string]causalBeamStateV19{base.graphHash: base}
	evaluatedCandidates := 0
	generatedProposal := false
	passes := []causalBeamStateV19{}

	for depth := 1; depth <= causalMaximumDepthV19 && len(frontier) != 0; depth++ {
		if ctx.Err() != nil {
			result.Status = RepairSearchCanceled
			result.Issues = []reports.Issue{graphIssue(CodeCanceled, "repair", "V19 causal repair canceled", "retry with an active context")}
			return finalizeRepairSearchV17(result)
		}
		if causalRepairBudgetExhaustedV19(result.Consumption, policy) || evaluatedCandidates >= causalMaximumEvaluatedCandidatesV19 {
			result.Consumption.BudgetExhausted = true
			break
		}
		slices.SortFunc(frontier, compareCausalBeamStatesV19)
		depthQuota := min(causalMaximumEvaluatedCandidatesV19-evaluatedCandidates, depth*causalBaseDepthQuotaV19-evaluatedCandidates)
		if depthQuota <= 0 {
			continue
		}
		next := []causalBeamStateV19{}
		for _, parent := range frontier {
			if result.Consumption.ExpandedStates >= policy.MaxExpandedStates || depthQuota <= 0 {
				result.Consumption.BudgetExhausted = result.Consumption.ExpandedStates >= policy.MaxExpandedStates
				break
			}
			result.Consumption.ExpandedStates++
			remainingOperations := CausalOperationBudgetV19{
				TopologyRepairs: max(0, policy.MaxTopologyRepairs-result.Consumption.TopologyRepairs),
				GeneratedGraphs: max(0, policy.MaxGeneratedGraphs-result.Consumption.GeneratedGraphs),
			}
			batch, proposalErr := hooks.propose(parent, remainingOperations)
			if proposalErr != nil {
				continue
			}
			result.Consumption.GeneratedGraphs += batch.Consumption.GeneratedGraphs
			generatedProposal = generatedProposal || batch.Consumption.GeneratedGraphs != 0 || len(batch.Proposals) != 0
			if batch.Exhausted || result.Consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs {
				result.Consumption.BudgetExhausted = true
			}
			proposals := slices.Clone(batch.Proposals)
			slices.SortFunc(proposals, func(left, right CausalOperationProposalV19) int {
				return causalCompareProposalsV19(requirement, left, right)
			})
			plateauCount := 0
			parentAnalysis := causalNewAnalysisV19(parent, requirementHash, inventory.Hash, policy, depthQuota)
			for _, proposal := range proposals {
				if depthQuota <= 0 || evaluatedCandidates >= causalMaximumEvaluatedCandidatesV19 {
					break
				}
				if proposal.LogicalChanges < 1 || proposal.LogicalChanges > CausalMaximumLogicalChangesPerProposalV19 ||
					parent.logicalChanges+proposal.LogicalChanges > causalMaximumPathChangesV19 || !causalProposalEvidenceValidV19(proposal) {
					continue
				}
				if issues := hooks.validate(proposal.Graph, proposal.Context); len(issues) != 0 {
					continue
				}
				candidateHash, hashErr := GraphHash(proposal.Graph)
				if hashErr != nil {
					continue
				}
				history := append(causalCloneOperationsV19(parent.history), causalCloneOperationsV19(proposal.Operations)...)
				historyKey := causalHistoryKeyV19(history)
				if prior, duplicate := seen[candidateHash]; duplicate {
					if historyKey < prior.historyKey {
						prior.history = history
						prior.historyKey = historyKey
						prior.repairs = append(slices.Clone(parent.repairs), causalRepairForProposalV19(parent, proposal))
						prior.logicalChanges = parent.logicalChanges + proposal.LogicalChanges
						seen[candidateHash] = prior
						if prior.depth == depth {
							next = causalReplaceBeamStateV19(next, prior)
						}
					}
					continue
				}
				remaining := causalRemainingPolicyV19(policy, result.Consumption)
				evaluation := hooks.evaluate(ctx, proposal.Graph, remaining)
				evaluatedCandidates++
				depthQuota--
				result.Consumption.CandidateSimulations += evaluation.Consumption.CandidateSimulations
				result.Consumption.CornerEvaluations += evaluation.Consumption.CornerEvaluations
				valueChanges, topologyChanges := causalProposalConsumptionV19(proposal)
				result.Consumption.ValueTrials += valueChanges
				result.Consumption.TopologyRepairs += topologyChanges
				if causalRepairBudgetExhaustedV19(result.Consumption, policy) {
					result.Consumption.BudgetExhausted = true
				}
				repair := causalRepairForProposalV19(parent, proposal)
				repair.Number = len(result.Attempts) + 1
				candidate := causalNewBeamStateV19(requirement, proposal.Graph, proposal.Context, evaluation,
					append(slices.Clone(parent.repairs), repair), history, historyKey, depth,
					parent.logicalChanges+proposal.LogicalChanges,
					max(0, len(proposal.Graph.Instances)-len(base.graph.Instances)),
					max(0, internalNodeCount(proposal.Graph)-internalNodeCount(base.graph)), hooks.structural)
				accepted, plateau := causalMonotonicV19(parent, candidate)
				if plateau && plateauCount >= causalMaximumPlateauPerParentV19 {
					accepted = false
				}
				if plateau && accepted {
					plateauCount++
					candidate.plateau = true
				}
				causalCandidate := causalCandidate{graph: proposal.Graph, repair: repair, perturbations: causalPerturbationsForChanges(parent.graph, repair.Changes), coordinated: proposal.LogicalChanges == 2}
				trial := causalTrialEvidence(requirement, parent.evaluation, evaluation, causalCandidate, candidateHash)
				trial.Number = len(parentAnalysis.Trials) + 1
				trial.Authorized = accepted
				if accepted {
					trial.Rejection = ""
				} else if trial.Rejection == "" {
					trial.Rejection = "candidate does not satisfy the frozen V19 monotonic rule"
				}
				trial.Hash = causalTrialHash(trial)
				parentAnalysis.Trials = append(parentAnalysis.Trials, trial)
				attempt := RepairAttempt{
					Number: len(result.Attempts) + 1, Repair: repair, GraphHash: candidateHash,
					TopologyHash: mustTopologyHash(proposal.Graph), Evaluation: evaluation,
					Improved: accepted && !plateau, Status: causalRepairStatusV19(evaluation.Status),
				}
				result.Attempts = append(result.Attempts, attempt)
				if evaluation.Status == SimulationEvaluationCanceled || ctx.Err() != nil {
					parentAnalysis.Status = "canceled"
					result.CausalAnalyses = append(result.CausalAnalyses, finalizeCausalRepairAnalysis(parentAnalysis))
					result.Status = RepairSearchCanceled
					result.Issues = []reports.Issue{graphIssue(CodeCanceled, "repair", "V19 causal repair canceled", "retry with an active context")}
					return finalizeRepairSearchV17(result)
				}
				if !accepted {
					continue
				}
				seen[candidateHash] = candidate
				if evaluation.Status == SimulationEvaluationPassed {
					passes = append(passes, candidate)
				} else if evaluation.Status == SimulationEvaluationFailed {
					next = append(next, candidate)
				}
			}
			causalFinalizeAnalysisV19(&parentAnalysis)
			result.CausalAnalyses = append(result.CausalAnalyses, parentAnalysis)
		}
		if len(passes) != 0 {
			slices.SortFunc(passes, compareCausalBeamStatesV19)
			selected := passes[0]
			result.Status = RepairSearchPassed
			result.Selected = &RepairedCandidate{
				Graph: selected.graph, Repair: selected.repairs[len(selected.repairs)-1],
				Repairs: slices.Clone(selected.repairs), Evaluation: selected.evaluation,
			}
			return finalizeRepairSearchV17(result)
		}
		slices.SortFunc(next, compareCausalBeamStatesV19)
		if len(next) > causalMaximumBeamWidthV19 {
			next = next[:causalMaximumBeamWidthV19]
		}
		result.Consumption.MaximumFrontier = max(result.Consumption.MaximumFrontier, len(next))
		frontier = next
	}

	result.Status = RepairSearchExhausted
	result.Issues = []reports.Issue{graphIssue(CodeRepairExhausted, "repair", "bounded V19 compositional causal repair did not produce a passing candidate", "inspect the strongest retained monotonic path without increasing frozen limits")}
	if !generatedProposal || len(result.Attempts) == 0 {
		result.Status = RepairSearchUnsupported
		result.Issues = []reports.Issue{graphIssue(CodeRepairUnsupported, "repair", "V19 could not derive a unique invariant-valid causal proposal", "expand reviewed generic operation evidence rather than adding a fixture rule")}
	}
	return finalizeRepairSearchV17(result)
}

func causalNewAnalysisV19(state causalBeamStateV19, requirementHash, inventoryHash string, policy Policy, quota int) CausalRepairAnalysis {
	return CausalRepairAnalysis{
		Schema: CausalRepairSchema, Version: CausalRepairVersion, PolicyVersion: PolicyVersion,
		RequirementHash: requirementHash, InventoryHash: inventoryHash,
		InitialGraphHash: state.graphHash, InitialEvaluationHash: state.evaluation.Hash,
		Budget: CausalRepairBudget{
			Trials: quota, ValueTrials: policy.MaxValueTrials, TopologyTrials: policy.MaxTopologyRepairs,
			CoordinatedTrials: quota, MaximumChanges: CausalMaximumLogicalChangesPerProposalV19,
			CandidateSimulations: policy.MaxCandidateSimulations, CornerEvaluations: policy.MaxCornerEvaluations,
		},
		Trials: []CausalRepairTrial{}, Status: "no_safe_improvement",
	}
}

func causalFinalizeAnalysisV19(analysis *CausalRepairAnalysis) {
	analysis.Consumption.Trials = len(analysis.Trials)
	for _, trial := range analysis.Trials {
		analysis.Consumption.CandidateSimulations += trial.Evaluation.Consumption.CandidateSimulations
		analysis.Consumption.CornerEvaluations += trial.Evaluation.Consumption.CornerEvaluations
		if causalChangesUseTopology(trial.Repair.Changes) {
			analysis.Consumption.TopologyTrials++
		} else {
			analysis.Consumption.ValueTrials++
		}
		if trial.Coordinated {
			analysis.Consumption.CoordinatedTrials++
		}
	}
	rankCausalTrials(analysis.Trials)
	for _, trial := range analysis.Trials {
		if trial.Rank != 1 || !trial.Authorized {
			continue
		}
		analysis.SelectedTrialHash = trial.Hash
		analysis.Status = "safe_improvement_found"
		if trial.Status == SimulationEvaluationPassed {
			analysis.Status = "passing_repair_found"
		}
		break
	}
	*analysis = finalizeCausalRepairAnalysis(*analysis)
}

func causalNewBeamStateV19(
	requirement Requirement,
	graph CandidateGraph,
	context CausalInvariantContextV19,
	evaluation SimulationEvaluation,
	repairs []Repair,
	history []CausalLogicalOperationV19,
	historyKey string,
	depth, logicalChanges, addedPrimitives, addedInternal int,
	structural func(CandidateGraph, CausalInvariantContextV19) CausalStructuralVectorV19,
) causalBeamStateV19 {
	graphHash, _ := GraphHash(graph)
	critical, total := causalFailureCountsV19(requirement, evaluation)
	vector := structural(graph, context)
	return causalBeamStateV19{
		graph: CloneGraph(graph), context: causalCloneContextV19(context), evaluation: evaluation,
		repairs: slices.Clone(repairs), history: causalCloneOperationsV19(history), historyKey: historyKey,
		graphHash: graphHash, depth: depth, logicalChanges: logicalChanges,
		addedPrimitives: addedPrimitives, addedInternal: addedInternal, structural: vector,
		criticalFailures: critical, totalFailures: total,
		worstViolation:    causalWorstViolationV19(evaluation),
		diagnosisMargin:   causalDiagnosisMarginV19(evaluation),
		reachableObserved: vector.UnreachableObservations == 0,
	}
}

func causalMonotonicV19(parent, child causalBeamStateV19) (bool, bool) {
	if child.evaluation.Status == SimulationEvaluationUnsupported || child.evaluation.Status == SimulationEvaluationCanceled ||
		causalEvaluationRegressesV19(parent.evaluation, child.evaluation) {
		return false, false
	}
	if child.evaluation.Status == SimulationEvaluationPassed || child.criticalFailures < parent.criticalFailures || child.totalFailures < parent.totalFailures {
		return true, false
	}
	if causalCompareStructuralV19(child.structural, parent.structural) < 0 {
		return true, true
	}
	if child.reachableObserved && !parent.reachableObserved {
		return true, false
	}
	if parent.worstViolation-child.worstViolation > causalEpsilonV19 {
		return true, false
	}
	if child.diagnosisMargin-parent.diagnosisMargin > causalEpsilonV19 {
		return true, false
	}
	return false, false
}

func compareCausalBeamStatesV19(left, right causalBeamStateV19) int {
	if result := cmp.Or(
		cmp.Compare(causalBoolRankV19(left.evaluation.Status != SimulationEvaluationPassed), causalBoolRankV19(right.evaluation.Status != SimulationEvaluationPassed)),
		cmp.Compare(left.criticalFailures, right.criticalFailures),
		cmp.Compare(left.totalFailures, right.totalFailures),
		cmp.Compare(left.structural.UnreachableObservations, right.structural.UnreachableObservations),
		cmp.Compare(left.worstViolation, right.worstViolation),
		cmp.Compare(left.logicalChanges, right.logicalChanges),
		cmp.Compare(left.addedPrimitives, right.addedPrimitives),
		cmp.Compare(left.addedInternal, right.addedInternal),
		cmp.Compare(left.historyKey, right.historyKey),
		cmp.Compare(left.graphHash, right.graphHash),
	); result != 0 {
		return result
	}
	return cmp.Compare(causalCanonicalGraphBytesV19(left.graph), causalCanonicalGraphBytesV19(right.graph))
}

func causalCanonicalGraphBytesV19(graph CandidateGraph) string {
	data, err := CanonicalGraphJSON(graph)
	if err != nil {
		return ""
	}
	return string(data)
}

func causalCompareStructuralV19(left, right CausalStructuralVectorV19) int {
	return cmp.Or(
		cmp.Compare(left.MissingBindings, right.MissingBindings),
		cmp.Compare(left.UnallocatedCones, right.UnallocatedCones),
		cmp.Compare(left.MissingTypedFeedback, right.MissingTypedFeedback),
		cmp.Compare(left.UnreachableObservations, right.UnreachableObservations),
	)
}

func causalRepairBudgetExhaustedV19(consumption Consumption, policy Policy) bool {
	return consumption.ExpandedStates >= policy.MaxExpandedStates ||
		consumption.GeneratedGraphs >= policy.MaxGeneratedGraphs ||
		consumption.CandidateSimulations >= policy.MaxCandidateSimulations ||
		consumption.CornerEvaluations >= policy.MaxCornerEvaluations ||
		consumption.ValueTrials >= policy.MaxValueTrials ||
		consumption.TopologyRepairs >= policy.MaxTopologyRepairs
}

func causalRemainingPolicyV19(policy Policy, consumption Consumption) Policy {
	remaining := policy
	remaining.MaxExpandedStates = max(0, policy.MaxExpandedStates-consumption.ExpandedStates)
	remaining.MaxGeneratedGraphs = max(0, policy.MaxGeneratedGraphs-consumption.GeneratedGraphs)
	remaining.MaxCandidateSimulations = max(0, policy.MaxCandidateSimulations-consumption.CandidateSimulations)
	remaining.MaxCornerEvaluations = max(0, policy.MaxCornerEvaluations-consumption.CornerEvaluations)
	remaining.MaxValueTrials = max(0, policy.MaxValueTrials-consumption.ValueTrials)
	remaining.MaxTopologyRepairs = max(0, policy.MaxTopologyRepairs-consumption.TopologyRepairs)
	remaining.MaxRetainedCandidates = min(causalMaximumBeamWidthV19, policy.MaxRetainedCandidates)
	return remaining
}

func causalRepairStatusV19(status SimulationEvaluationStatus) RepairSearchStatus {
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

func causalProposalConsumptionV19(proposal CausalOperationProposalV19) (int, int) {
	value, topology := 0, 0
	for _, operation := range proposal.Operations {
		switch operation.Kind {
		case "set_value", "substitute_primitive":
			value++
		default:
			topology++
		}
	}
	return value, topology
}

func causalHistoryKeyV19(history []CausalLogicalOperationV19) string {
	fields := []string{}
	for _, operation := range history {
		fields = append(fields, causalOperationOrderKeyV19(operation))
	}
	return causalLengthDelimitedV19(fields)
}

func causalReplaceBeamStateV19(states []causalBeamStateV19, replacement causalBeamStateV19) []causalBeamStateV19 {
	for index := range states {
		if states[index].graphHash == replacement.graphHash {
			states[index] = replacement
			return states
		}
	}
	return states
}

func causalFailureCountsV19(requirement Requirement, evaluation SimulationEvaluation) (int, int) {
	criticalByID := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		criticalByID[assertion.ID] = assertion.Critical
	}
	failed := map[string]bool{}
	for _, attempt := range evaluation.Attempts {
		if !attempt.AssertionPass {
			failed[attempt.RequirementID] = true
		}
	}
	critical := 0
	for id := range failed {
		if criticalByID[id] {
			critical++
		}
	}
	return critical, len(failed)
}

func causalWorstViolationV19(evaluation SimulationEvaluation) float64 {
	worst := 0.0
	for _, attempt := range evaluation.Attempts {
		worst = math.Max(worst, causalAttemptViolation(attempt))
	}
	return worst
}

func causalDiagnosisMarginV19(evaluation SimulationEvaluation) float64 {
	if len(evaluation.Diagnoses) == 0 {
		return math.Inf(1)
	}
	diagnosis := evaluation.Diagnoses[0]
	margin := math.Inf(1)
	found := false
	for _, attempt := range evaluation.Attempts {
		if attempt.RequirementID == diagnosis.RequirementID && attempt.Analysis == diagnosis.Analysis && attempt.Metric == diagnosis.Metric {
			margin = math.Min(margin, causalAttemptMargin(attempt))
			found = true
		}
	}
	if !found {
		return -math.Inf(1)
	}
	return margin
}

func causalEvaluationRegressesV19(parent, child SimulationEvaluation) bool {
	parentAttempts := causalAttemptMap(parent.Attempts)
	childAttempts := causalAttemptMap(child.Attempts)
	for key, before := range parentAttempts {
		after, found := childAttempts[key]
		if before.AssertionPass && (!found || !after.AssertionPass) {
			return true
		}
	}
	return false
}

func causalRepairForProposalV19(parent causalBeamStateV19, proposal CausalOperationProposalV19) Repair {
	diagnosis := firstRepairDiagnosis(parent.evaluation.Diagnoses)
	repair := Repair{
		Operator: proposal.PlannerKind, DiagnosisCode: diagnosis.Code,
		DiagnosisRequirementID: diagnosis.RequirementID, DiagnosisEvidenceHash: diagnosis.EvidenceHash,
		ExpectedDirection: diagnosis.Direction, Changes: []GraphChange{},
	}
	if len(proposal.Operations) != 0 {
		repair.BeforeGraphHash = proposal.Operations[0].BeforeHash
		repair.AfterGraphHash = proposal.Operations[len(proposal.Operations)-1].AfterHash
	}
	for _, operation := range proposal.Operations {
		if causalExistingOperationKindV19(operation.Kind) {
			repair.Changes = append(repair.Changes, causalExistingGraphChangesV19(operation)...)
			continue
		}
		connections := slices.Clone(operation.Connections)
		slices.SortFunc(connections, compareTerminalConnections)
		for _, connection := range connections {
			repair.Changes = append(repair.Changes, GraphChange{
				Kind: operation.Kind, Primitive: operation.InstanceID, Terminal: connection.Terminal,
				FromNode: operation.PrimitiveKey, ToNode: connection.Node,
				ToValue: cloneInventoryFloat(operation.ValueSI),
			})
		}
	}
	return repair
}

func causalExistingGraphChangesV19(operation CausalLogicalOperationV19) []GraphChange {
	switch operation.Kind {
	case "set_value":
		return []GraphChange{{Kind: operation.Kind, Primitive: operation.InstanceID, ToValue: cloneInventoryFloat(operation.ValueSI)}}
	case "substitute_primitive":
		return []GraphChange{{Kind: operation.Kind, Primitive: operation.InstanceID, ToNode: operation.PrimitiveKey, ToValue: cloneInventoryFloat(operation.ValueSI)}}
	case "correct_polarity", "redirect_terminal":
		result := []GraphChange{}
		for _, connection := range operation.Connections {
			result = append(result, GraphChange{Kind: operation.Kind, Primitive: operation.InstanceID, Terminal: connection.Terminal, ToNode: connection.Node})
		}
		return result
	default:
		result := []GraphChange{}
		for _, connection := range operation.Connections {
			result = append(result, GraphChange{Kind: operation.Kind, Primitive: operation.InstanceID, Terminal: connection.Terminal, FromNode: operation.PrimitiveKey, ToNode: connection.Node, ToValue: cloneInventoryFloat(operation.ValueSI)})
		}
		return result
	}
}

func causalCompareProposalsV19(requirement Requirement, left, right CausalOperationProposalV19) int {
	return cmp.Or(
		cmp.Compare(causalProposalOperationRankV19(left), causalProposalOperationRankV19(right)),
		cmp.Compare(causalProposalCriticalRankV19(requirement, left), causalProposalCriticalRankV19(requirement, right)),
		cmp.Compare(causalProposalObservationV19(left), causalProposalObservationV19(right)),
		cmp.Compare(causalProposalUpstreamV19(left), causalProposalUpstreamV19(right)),
		cmp.Compare(left.CanonicalKey, right.CanonicalKey),
		cmp.Compare(causalProposalAfterHashV19(left), causalProposalAfterHashV19(right)),
	)
}

func causalProposalOperationRankV19(proposal CausalOperationProposalV19) int {
	if proposal.PlannerKind == "coordinated_pair" || proposal.LogicalChanges == 2 {
		return 7
	}
	if len(proposal.Operations) == 0 {
		return 8
	}
	switch proposal.Operations[0].Kind {
	case "set_value", "substitute_primitive":
		return 0
	case "correct_polarity":
		return 1
	case "redirect_terminal", CausalOperationRedirectRoleTerminalV19:
		return 2
	case "add_primitive", "split_primitive":
		return 3
	case CausalOperationInsertRoleCompleteStageV19:
		return 4
	case CausalOperationAllocateObservationConeV19:
		return 5
	case CausalOperationInsertTypedFeedbackPathV19:
		return 6
	default:
		return 8
	}
}

func causalProposalCriticalRankV19(requirement Requirement, proposal CausalOperationProposalV19) int {
	critical := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		critical[assertion.ID] = assertion.Critical
	}
	for _, operation := range proposal.Operations {
		if critical[operation.ObligationID] {
			return 0
		}
	}
	return 1
}

func causalProposalObservationV19(proposal CausalOperationProposalV19) string {
	if len(proposal.Operations) == 0 {
		return ""
	}
	return proposal.Operations[0].ObservationID
}

func causalProposalUpstreamV19(proposal CausalOperationProposalV19) string {
	if len(proposal.Operations) == 0 {
		return ""
	}
	return proposal.Operations[0].UpstreamNode
}

func causalProposalAfterHashV19(proposal CausalOperationProposalV19) string {
	if len(proposal.Operations) == 0 {
		return ""
	}
	return proposal.Operations[len(proposal.Operations)-1].AfterHash
}

func causalOperationOrderKeyV19(operation CausalLogicalOperationV19) string {
	return strings.Join([]string{
		fmt.Sprintf("%010d", causalProposalOperationRankV19(CausalOperationProposalV19{Operations: []CausalLogicalOperationV19{operation}, LogicalChanges: 1})),
		operation.ObligationID, operation.ObservationID, operation.UpstreamNode,
		operation.PrimitiveKind, operation.PrimitiveKey,
		causalConnectionsKeyV19(operation.Connections), causalCanonicalValueV19(operation.ValueSI), operation.AfterHash,
	}, "\x1f")
}

func causalStructuralVectorV19(requirement Requirement, graph CandidateGraph, inventory PrimitiveInventory, context CausalInvariantContextV19) CausalStructuralVectorV19 {
	vector := CausalStructuralVectorV19{}
	portNodes := map[string]string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && node.SemanticKind == "port" {
			portNodes[node.SemanticID] = node.ID
		}
	}
	for _, port := range requirement.Requirements.Ports {
		if portNodes[port.ID] == "" {
			vector.MissingBindings++
		}
	}
	drivers, adjacency := causalGraphReachabilityV19(graph, inventory)
	starts := []string{}
	for _, node := range graph.Nodes {
		if node.Scope == "external" && (node.Role == "input" || node.Role == "control" || node.Role == "supply") {
			starts = append(starts, node.ID)
		}
	}
	reachable := causalReachableNodesV19(starts, adjacency)
	feedback := map[string]bool{}
	for _, path := range context.FeedbackPaths {
		feedback[path.ObligationID] = true
	}
	observed := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		if assertion.Observation.Kind != "port" {
			continue
		}
		node := portNodes[assertion.Observation.ID]
		if node == "" || !drivers[node] {
			if !observed[assertion.Observation.ID] {
				vector.UnallocatedCones++
			}
		}
		if node == "" || !drivers[node] || !reachable[node] {
			if !observed[assertion.Observation.ID] {
				vector.UnreachableObservations++
			}
		}
		observed[assertion.Observation.ID] = true
		if causalFeedbackSensitiveMetricV19(assertion.Metric) && !feedback[assertion.ID] {
			vector.MissingTypedFeedback++
		}
	}
	return vector
}

func causalGraphReachabilityV19(graph CandidateGraph, inventory PrimitiveInventory) (map[string]bool, map[string][]string) {
	drivers := map[string]bool{}
	adjacency := map[string][]string{}
	for _, instance := range graph.Instances {
		primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
		if !found {
			continue
		}
		contracts := causalTerminalContractsV19(primitive)
		if causalPurePassiveV19(contracts, instance) {
			for _, left := range instance.Terminals {
				for _, right := range instance.Terminals {
					if left.Node != right.Node {
						adjacency[left.Node] = append(adjacency[left.Node], right.Node)
					}
				}
			}
			continue
		}
		inputs, outputs := []string{}, []string{}
		for _, connection := range instance.Terminals {
			terminal := contracts[connection.Terminal]
			switch causalTerminalRoleV19(terminal) {
			case "input", "power_input":
				inputs = append(inputs, connection.Node)
			case "output", "open_collector", "power_output":
				outputs = append(outputs, connection.Node)
				drivers[connection.Node] = true
			}
		}
		for _, input := range inputs {
			adjacency[input] = append(adjacency[input], outputs...)
		}
	}
	for node := range adjacency {
		slices.Sort(adjacency[node])
		adjacency[node] = slices.Compact(adjacency[node])
	}
	return drivers, adjacency
}

func causalReachableNodesV19(starts []string, adjacency map[string][]string) map[string]bool {
	queue := slices.Clone(starts)
	slices.Sort(queue)
	seen := map[string]bool{}
	for len(queue) != 0 {
		node := queue[0]
		queue = queue[1:]
		if seen[node] {
			continue
		}
		seen[node] = true
		queue = append(queue, adjacency[node]...)
	}
	return seen
}

func causalProposalsV19(
	requirement Requirement,
	state causalBeamStateV19,
	inventory PrimitiveInventory,
	limits GraphLimits,
	policy Policy,
	budget CausalOperationBudgetV19,
) (CausalOperationBatchV19, error) {
	result := CausalOperationBatchV19{}
	appendProposal := func(proposal CausalOperationProposalV19, generated, logical int) bool {
		if generated <= 0 {
			generated = 1
		}
		if logical <= 0 {
			logical = proposal.LogicalChanges
		}
		if result.Consumption.GeneratedGraphs+generated > budget.GeneratedGraphs || result.Consumption.TopologyRepairs+logical > budget.TopologyRepairs {
			result.Exhausted = true
			return false
		}
		result.Consumption.GeneratedGraphs += generated
		result.Consumption.TopologyRepairs += logical
		result.Proposals = append(result.Proposals, proposal)
		return true
	}

	obligations := causalDiagnosedAssertionsV19(requirement, state.evaluation)
	existingCandidates := causalExistingCandidatesV19(requirement, state, inventory, limits, policy, obligations)
	for _, candidate := range existingCandidates {
		if !appendProposal(candidate, 1, candidate.LogicalChanges) {
			break
		}
	}
	if result.Exhausted {
		result.Proposals = causalOrderAndCompactProposalsV19(requirement, result.Proposals)
		return result, nil
	}

	upstream := causalUpstreamNodesV19(requirement, state.graph, obligations)
	for _, assertion := range obligations {
		for _, node := range upstream[assertion.ID] {
			remaining := causalRemainingOperationBudgetV19(budget, result.Consumption)
			batch, err := InsertRoleCompleteStagesV19(requirement, state.graph, inventory, limits, state.context, RoleCompleteStageRequestV19{ObligationID: assertion.ID, UpstreamNode: node, ObservationID: assertion.Observation.ID}, remaining)
			if err != nil {
				continue
			}
			causalAppendOperationBatchV19(&result, batch)
			if result.Exhausted {
				break
			}
		}
		if result.Exhausted {
			break
		}
	}
	if !result.Exhausted {
		for left := 0; left < len(obligations); left++ {
			for right := left + 1; right < len(obligations); right++ {
				if obligations[left].Observation == obligations[right].Observation || len(upstream[obligations[left].ID]) == 0 || len(upstream[obligations[right].ID]) == 0 {
					continue
				}
				remaining := causalRemainingOperationBudgetV19(budget, result.Consumption)
				batch, err := AllocateIndependentObservationConesV19(requirement, state.graph, inventory, limits, state.context, []RoleCompleteStageRequestV19{
					{ObligationID: obligations[left].ID, UpstreamNode: upstream[obligations[left].ID][0], ObservationID: obligations[left].Observation.ID},
					{ObligationID: obligations[right].ID, UpstreamNode: upstream[obligations[right].ID][0], ObservationID: obligations[right].Observation.ID},
				}, remaining)
				if err == nil {
					causalAppendOperationBatchV19(&result, batch)
				}
				if result.Exhausted {
					break
				}
			}
			if result.Exhausted {
				break
			}
		}
	}
	if !result.Exhausted {
		causalAppendRoleRedirectsV19(&result, requirement, state, inventory, limits, obligations, upstream, budget)
	}
	if !result.Exhausted {
		causalAppendFeedbackV19(&result, requirement, state, inventory, limits, obligations, budget)
	}
	if !result.Exhausted {
		causalAppendCoordinatedPairsV19(&result, requirement, state, inventory, limits, obligations, upstream, existingCandidates, budget)
	}
	result.Proposals = causalOrderAndCompactProposalsV19(requirement, result.Proposals)
	return result, nil
}

func causalExistingCandidatesV19(requirement Requirement, state causalBeamStateV19, inventory PrimitiveInventory, limits GraphLimits, policy Policy, obligations []BehavioralAssertion) []CausalOperationProposalV19 {
	if len(obligations) == 0 {
		return nil
	}
	result := []CausalOperationProposalV19{}
	for _, candidate := range generateCausalCandidatesV17(requirement, state.graph, state.evaluation, inventory, policy) {
		operation, ok := causalGraphOperationForExistingV19(state.graph, candidate.graph, inventory)
		if !ok {
			continue
		}
		proposal, err := RecordExistingCausalOperationV19(requirement, state.graph, candidate.graph, inventory, limits, state.context, operation, obligations[0].ID)
		if err == nil {
			result = append(result, proposal)
		}
	}
	return causalOrderAndCompactProposalsV19(requirement, result)
}

func causalGraphOperationForExistingV19(before, after CandidateGraph, inventory PrimitiveInventory) (GraphOperation, bool) {
	kind, instanceID, _, _, ok := causalExistingDeltaV19(before, after, inventory)
	if !ok {
		return GraphOperation{}, false
	}
	beforeHash, beforeErr := GraphHash(before)
	afterHash, afterErr := GraphHash(after)
	if beforeErr != nil || afterErr != nil {
		return GraphOperation{}, false
	}
	operation := GraphOperation{Kind: kind, Node: instanceID, BeforeHash: beforeHash, AfterHash: afterHash}
	prior, priorFound := causalGraphInstanceV19(before, instanceID)
	current, currentFound := causalGraphInstanceV19(after, instanceID)
	switch kind {
	case "set_value":
		if !priorFound || !currentFound {
			return GraphOperation{}, false
		}
		operation.ValueSI = cloneInventoryFloat(current.ValueSI)
	case "substitute_primitive":
		if !priorFound || !currentFound {
			return GraphOperation{}, false
		}
		operation.PrimitiveKey, operation.PrimitiveKind = current.PrimitiveKey, current.Kind
		operation.ValueSI = cloneInventoryFloat(current.ValueSI)
	case "correct_polarity", "redirect_terminal":
		if !priorFound || !currentFound {
			return GraphOperation{}, false
		}
		operation.Connections = causalChangedTerminalConnectionsV19(prior, current)
	case "add_primitive", "split_primitive":
		if !currentFound {
			return GraphOperation{}, false
		}
		operation.PrimitiveKey, operation.PrimitiveKind = current.PrimitiveKey, current.Kind
		operation.Connections = slices.Clone(current.Terminals)
		operation.ValueSI = cloneInventoryFloat(current.ValueSI)
	default:
		return GraphOperation{}, false
	}
	return operation, true
}

func causalDiagnosedAssertionsV19(requirement Requirement, evaluation SimulationEvaluation) []BehavioralAssertion {
	byID := map[string]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		byID[assertion.ID] = assertion
	}
	result := []BehavioralAssertion{}
	seen := map[string]bool{}
	for _, diagnosis := range evaluation.Diagnoses {
		assertion, found := byID[diagnosis.RequirementID]
		if !found || seen[assertion.ID] || assertion.Observation.Kind != "port" {
			continue
		}
		seen[assertion.ID] = true
		result = append(result, assertion)
	}
	if len(result) == 0 {
		for _, assertion := range requirement.Requirements.BehavioralRequirements {
			if assertion.Observation.Kind == "port" {
				result = append(result, assertion)
			}
		}
	}
	slices.SortFunc(result, func(left, right BehavioralAssertion) int {
		return cmp.Or(cmp.Compare(causalBoolRankV19(!left.Critical), causalBoolRankV19(!right.Critical)), cmp.Compare(left.Observation.ID, right.Observation.ID), cmp.Compare(left.ID, right.ID))
	})
	return result
}

func causalUpstreamNodesV19(requirement Requirement, graph CandidateGraph, obligations []BehavioralAssertion) map[string][]string {
	portNode := map[string]string{}
	defaultNodes := []string{}
	for _, node := range graph.Nodes {
		if node.Scope != "external" {
			continue
		}
		if node.SemanticKind == "port" {
			portNode[node.SemanticID] = node.ID
		}
		if node.Role == "input" || node.Role == "control" || node.Role == "supply" {
			defaultNodes = append(defaultNodes, node.ID)
		}
	}
	slices.Sort(defaultNodes)
	result := map[string][]string{}
	for _, assertion := range obligations {
		nodes := []string{}
		if assertion.Excitation != nil && assertion.Excitation.Kind == "port" && portNode[assertion.Excitation.ID] != "" {
			nodes = append(nodes, portNode[assertion.Excitation.ID])
		}
		nodes = append(nodes, defaultNodes...)
		slices.Sort(nodes)
		result[assertion.ID] = slices.Compact(nodes)
	}
	return result
}

func causalAppendRoleRedirectsV19(result *CausalOperationBatchV19, requirement Requirement, state causalBeamStateV19, inventory PrimitiveInventory, limits GraphLimits, obligations []BehavioralAssertion, upstream map[string][]string, budget CausalOperationBudgetV19) {
	for _, assertion := range obligations {
		observationNode, found := ExternalNodeForObservation(state.graph, requirement, assertion.Observation)
		if !found {
			continue
		}
		for _, instance := range state.graph.Instances {
			primitive, found := primitiveByKey(inventory, instance.PrimitiveKey)
			if !found {
				continue
			}
			contracts := causalTerminalContractsV19(primitive)
			for _, connection := range instance.Terminals {
				var targets []string
				switch causalTerminalRoleV19(contracts[connection.Terminal]) {
				case "output", "open_collector", "power_output":
					targets = []string{observationNode}
				case "input":
					targets = upstream[assertion.ID]
				default:
					continue
				}
				for _, target := range targets {
					if target == connection.Node {
						continue
					}
					remaining := causalRemainingOperationBudgetV19(budget, result.Consumption)
					batch, err := RedirectRoleTerminalV19(requirement, state.graph, inventory, limits, state.context, RoleTerminalRedirectRequestV19{ObligationID: assertion.ID, InstanceID: instance.ID, Terminal: connection.Terminal, Node: target}, remaining)
					if err == nil {
						causalAppendOperationBatchV19(result, batch)
					}
					if result.Exhausted {
						return
					}
				}
			}
		}
	}
}

func causalAppendFeedbackV19(result *CausalOperationBatchV19, requirement Requirement, state causalBeamStateV19, inventory PrimitiveInventory, limits GraphLimits, obligations []BehavioralAssertion, budget CausalOperationBudgetV19) {
	for _, assertion := range obligations {
		if !causalFeedbackSensitiveMetricV19(assertion.Metric) {
			continue
		}
		for _, from := range state.graph.Instances {
			fromPrimitive, found := primitiveByKey(inventory, from.PrimitiveKey)
			if !found {
				continue
			}
			fromContracts := causalTerminalContractsV19(fromPrimitive)
			for _, fromTerminal := range from.Terminals {
				role := causalTerminalRoleV19(fromContracts[fromTerminal.Terminal])
				if role != "output" && role != "open_collector" && role != "power_output" {
					continue
				}
				for _, to := range state.graph.Instances {
					toPrimitive, found := primitiveByKey(inventory, to.PrimitiveKey)
					if !found {
						continue
					}
					toContracts := causalTerminalContractsV19(toPrimitive)
					for _, toTerminal := range to.Terminals {
						if causalTerminalRoleV19(toContracts[toTerminal.Terminal]) != "input" {
							continue
						}
						remaining := causalRemainingOperationBudgetV19(budget, result.Consumption)
						batch, err := InsertTypedFeedbackPathsV19(requirement, state.graph, inventory, limits, state.context, TypedFeedbackPathRequestV19{ObligationID: assertion.ID, FromInstance: from.ID, FromTerminal: fromTerminal.Terminal, ToInstance: to.ID, ToTerminal: toTerminal.Terminal}, remaining)
						if err == nil {
							causalAppendOperationBatchV19(result, batch)
						}
						if result.Exhausted {
							return
						}
					}
				}
			}
		}
	}
}

func causalAppendCoordinatedPairsV19(
	result *CausalOperationBatchV19,
	requirement Requirement,
	state causalBeamStateV19,
	inventory PrimitiveInventory,
	limits GraphLimits,
	obligations []BehavioralAssertion,
	upstream map[string][]string,
	existing []CausalOperationProposalV19,
	budget CausalOperationBudgetV19,
) {
	for _, first := range existing {
		if first.LogicalChanges != 1 || len(first.Operations) != 1 || causalNewOperationKindV19(first.Operations[0].Kind) {
			continue
		}
		for _, assertion := range obligations {
			for _, node := range upstream[assertion.ID] {
				remaining := causalRemainingOperationBudgetV19(budget, result.Consumption)
				stageBatch, err := InsertRoleCompleteStagesV19(requirement, first.Graph, inventory, limits, first.Context, RoleCompleteStageRequestV19{
					ObligationID: assertion.ID, UpstreamNode: node, ObservationID: assertion.Observation.ID,
				}, remaining)
				if err != nil {
					continue
				}
				result.Consumption.TopologyRepairs += stageBatch.Consumption.TopologyRepairs
				result.Consumption.GeneratedGraphs += stageBatch.Consumption.GeneratedGraphs
				result.Consumption.InvariantRejected += stageBatch.Consumption.InvariantRejected
				result.Exhausted = result.Exhausted || stageBatch.Exhausted
				for _, second := range stageBatch.Proposals {
					remaining = causalRemainingOperationBudgetV19(budget, result.Consumption)
					composed, composeErr := ComposeCausalProposalsV19(requirement, inventory, limits, first, second, remaining)
					if composeErr == nil {
						causalAppendOperationBatchV19(result, composed)
					}
					if result.Exhausted {
						return
					}
				}
				if result.Exhausted {
					return
				}
			}
		}
	}
	_ = state
}

func causalAppendOperationBatchV19(result *CausalOperationBatchV19, batch CausalOperationBatchV19) {
	result.Proposals = append(result.Proposals, batch.Proposals...)
	result.Consumption.TopologyRepairs += batch.Consumption.TopologyRepairs
	result.Consumption.GeneratedGraphs += batch.Consumption.GeneratedGraphs
	result.Consumption.InvariantRejected += batch.Consumption.InvariantRejected
	result.Exhausted = result.Exhausted || batch.Exhausted
}

func causalRemainingOperationBudgetV19(total CausalOperationBudgetV19, used CausalOperationConsumptionV19) CausalOperationBudgetV19 {
	return CausalOperationBudgetV19{TopologyRepairs: max(0, total.TopologyRepairs-used.TopologyRepairs), GeneratedGraphs: max(0, total.GeneratedGraphs-used.GeneratedGraphs)}
}

func causalOrderAndCompactProposalsV19(requirement Requirement, source []CausalOperationProposalV19) []CausalOperationProposalV19 {
	slices.SortFunc(source, func(left, right CausalOperationProposalV19) int {
		return causalCompareProposalsV19(requirement, left, right)
	})
	seen := map[string]bool{}
	write := 0
	for read := range source {
		hash, err := GraphHash(source[read].Graph)
		if err != nil || seen[hash] {
			continue
		}
		seen[hash] = true
		source[write] = source[read]
		write++
	}
	clear(source[write:])
	return source[:write]
}
