package opentopologysynthesis

import (
	"context"
	"fmt"
	"kicadai/internal/simmodel"
	"kicadai/internal/simulationadmission"
	"slices"
)

// ElectricalRepairTrialV23 preserves the graph/search ledger and additionally
// binds the versioned numerical execution and its charged work.
type ElectricalRepairTrialV23 struct {
	ElectricalRepairTrialV22
	ExecutionHash     string `json:"execution_sha256"`
	SimulationCalls   int    `json:"simulation_calls"`
	CornerEvaluations int    `json:"corner_evaluations"`
}

type ElectricalRepairSelectionV23 struct {
	Graph       CandidateGraph           `json:"graph"`
	Evaluation  ElectricalEvaluationV23  `json:"evaluation"`
	Certificate ElectricalCertificateV23 `json:"certificate"`
}

// ElectricalRepairResultV23 never replaces historical evidence. It uses exactly
// the existing graph operators, ordering, beam, and public work-limit contract.
type ElectricalRepairResultV23 struct {
	Schema                string                        `json:"schema"`
	SolverPolicyID        string                        `json:"solver_policy_id"`
	SolverPolicyHash      string                        `json:"solver_policy_sha256"`
	RequirementHash       string                        `json:"requirement_sha256"`
	InventoryHash         string                        `json:"inventory_sha256"`
	InitialGraphHash      string                        `json:"initial_graph_sha256"`
	InitialEvaluationHash string                        `json:"initial_evaluation_sha256"`
	Status                RepairSearchStatus            `json:"status"`
	StopReason            string                        `json:"stop_reason"`
	Limits                ElectricalRepairLimitsV22     `json:"limits"`
	BindingWork           int                           `json:"binding_work"`
	Consumption           Consumption                   `json:"consumption"`
	Trials                []ElectricalRepairTrialV23    `json:"trials"`
	Selected              *ElectricalRepairSelectionV23 `json:"selected,omitempty"`
	Hash                  string                        `json:"hash"`
}

func RepairElectricalV23(ctx context.Context, requirement Requirement, graph CandidateGraph, initial SimulationEvaluation, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment, policy Policy, limits ElectricalRepairLimitsV22) ElectricalRepairResultV23 {
	return repairElectricalPreparedV23(ctx, requirement, graph, initial, inventory, environment, simulationadmission.PrepareEnvironment(admission), policy, limits)
}
func repairElectricalPreparedV23(ctx context.Context, requirement Requirement, graph CandidateGraph, initial SimulationEvaluation, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment, policy Policy, limits ElectricalRepairLimitsV22) (result ElectricalRepairResultV23) {
	result = ElectricalRepairResultV23{Schema: "kicadai.electrical-repair.v23", SolverPolicyID: simmodel.SolverIDV23, SolverPolicyHash: simmodel.SolverSHA256V23(), InventoryHash: inventory.Hash, InitialEvaluationHash: initial.Hash, Limits: limits, Status: RepairSearchUnsupported, Trials: []ElectricalRepairTrialV23{}}
	defer func() { result.Hash = ""; result.Hash = causalCrossStageHash(result) }()
	if ctx.Err() != nil {
		result.Status, result.StopReason = RepairSearchCanceled, "canceled"
		return
	}
	if limits.MaxDepth <= 0 || limits.BeamWidth <= 0 || limits.MaxBindingWork <= 0 || limits.MaxEvaluations <= 0 || limits.MaxCorners <= 0 {
		result.StopReason = "invalid_limits"
		return
	}
	requirement = Normalize(requirement)
	if len(Validate(requirement)) != 0 {
		result.StopReason = "invalid_requirement"
		return
	}
	var err error
	result.RequirementHash, err = CanonicalHash(requirement)
	if err != nil {
		result.StopReason = "invalid_requirement"
		return
	}
	graph, err = NormalizeGraph(graph)
	if err != nil {
		result.StopReason = "invalid_graph"
		return
	}
	result.InitialGraphHash, err = GraphHash(graph)
	if err != nil {
		result.StopReason = "invalid_graph"
		return
	}
	invHash, err := primitiveInventoryHash(inventory)
	if err != nil || invHash != inventory.Hash || len(validateSimulationEnvironment(inventory, environment)) != 0 {
		result.StopReason = "invalid_environment"
		return
	}
	unsigned := initial
	unsigned.Hash = ""
	if initial.Hash == "" || causalCrossStageHash(unsigned) != initial.Hash || initial.RequirementHash != result.RequirementHash || initial.InventoryHash != inventory.Hash || initial.GraphHash != result.InitialGraphHash || initial.ValueTrialHash != "" {
		result.StopReason = "invalid_initial_evidence"
		return
	}
	if initial.Status != SimulationEvaluationFailed {
		result.StopReason = "ineligible_initial_status"
		return
	}
	if electricalCriticalFailureV22(requirement, initial) {
		result.StopReason = "critical_failure"
		return
	}
	structure := AnalyzeTopologyV21(requirement, graph, inventory)
	if !structure.Complete || structure.Contradictory {
		result.StopReason = "incomplete_initial_topology"
		return
	}
	policy = effectiveTopologyPolicy(policy)
	evaluationLimit := min(limits.MaxEvaluations, policy.MaxCandidateSimulations)
	cornerLimit := min(limits.MaxCorners, policy.MaxCornerEvaluations)
	frontier := []electricalRepairStateV22{{graph: graph, evaluation: initial, hash: result.InitialGraphHash, passing: electricalPassingCountV22(initial), penalty: simulationEvaluationPenalty(initial)}}
	seen := map[string]bool{result.InitialGraphHash: true}
	result.Consumption.MaximumFrontier = 1
	result.Status = RepairSearchExhausted
	for depth := 1; depth <= limits.MaxDepth && len(frontier) != 0; depth++ {
		next := []electricalRepairStateV22{}
		for _, state := range frontier {
			if ctx.Err() != nil {
				result.Status, result.StopReason = RepairSearchCanceled, "canceled"
				return
			}
			if result.BindingWork >= limits.MaxBindingWork {
				result.StopReason = "binding_work_exhausted"
				result.Consumption.BudgetExhausted = true
				return
			}
			batch, err := enumerateControlRebindingsV22(ctx, requirement, state.graph, inventory, limits.MaxBindingWork-result.BindingWork, depth > 1)
			result.BindingWork += batch.work
			result.Consumption.ExpandedStates++
			result.Consumption.GeneratedGraphs += len(batch.candidates)
			if err != nil {
				if ctx.Err() != nil {
					result.Status, result.StopReason = RepairSearchCanceled, "canceled"
				} else {
					result.Status, result.StopReason = RepairSearchUnsupported, "invalid_continuation"
				}
				return
			}
			for _, proposal := range batch.candidates {
				if seen[proposal.hash] {
					continue
				}
				if result.Consumption.CandidateSimulations >= evaluationLimit || result.Consumption.CornerEvaluations >= cornerLimit {
					result.StopReason = "evaluation_budget_exhausted"
					result.Consumption.BudgetExhausted = true
					return
				}
				candidatePolicy := policy
				candidatePolicy.MaxCandidateSimulations = evaluationLimit - result.Consumption.CandidateSimulations
				candidatePolicy.MaxCornerEvaluations = cornerLimit - result.Consumption.CornerEvaluations
				execution := evaluateElectricalPreparedV23(ctx, requirement, proposal.graph, inventory, environment, admission, candidatePolicy)
				evaluation := execution.Evaluation
				result.Consumption.CandidateSimulations += evaluation.Consumption.CandidateSimulations
				result.Consumption.CornerEvaluations += evaluation.Consumption.CornerEvaluations
				// Admission refusals also consume an invocation; otherwise a set of
				// unsupported proposals could evade the evaluation count limit.
				if evaluation.Consumption.CandidateSimulations == 0 {
					result.Consumption.CandidateSimulations++
				}
				result.Consumption.TopologyRepairs++
				allowed, rejection := electricalContinuationAllowedV22(requirement, state.evaluation, evaluation)
				trial := ElectricalRepairTrialV23{ElectricalRepairTrialV22: ElectricalRepairTrialV22{Number: len(result.Trials) + 1, Depth: depth, ParentGraphHash: state.hash, GraphHash: proposal.hash, EvaluationHash: evaluation.Hash, Change: proposal.change, Status: evaluation.Status, PassingAssertions: electricalPassingCountV22(evaluation), Continuable: allowed, Rejection: rejection}, ExecutionHash: execution.Hash, SimulationCalls: max(1, evaluation.Consumption.CandidateSimulations), CornerEvaluations: evaluation.Consumption.CornerEvaluations}
				result.Trials = append(result.Trials, trial)
				if ctx.Err() != nil || evaluation.Status == SimulationEvaluationCanceled {
					result.Status, result.StopReason = RepairSearchCanceled, "canceled"
					return
				}
				if !allowed {
					continue
				}
				// A graph rejected for regressing this parent's passed corners may
				// be reached safely through another path. Only accepted states
				// participate in global graph deduplication.
				seen[proposal.hash] = true
				changes := append(slices.Clone(state.changes), proposal.change)
				if evaluation.Status == SimulationEvaluationPassed {
					certificate, err := certifyElectricalPathV23(ctx, requirement, graph, proposal.graph, inventory, environment, admission, changes, execution)
					if err != nil {
						result.Trials[len(result.Trials)-1].Continuable = false
						result.Trials[len(result.Trials)-1].Rejection = "certificate_rejected"
						if ctx.Err() != nil {
							result.Status, result.StopReason = RepairSearchCanceled, "canceled"
							return
						}
						continue
					}
					result.Selected = &ElectricalRepairSelectionV23{Graph: proposal.graph, Evaluation: execution, Certificate: certificate}
					result.Status, result.StopReason = RepairSearchPassed, "complete_electrical_certificate"
					return
				}
				next = append(next, electricalRepairStateV22{graph: proposal.graph, evaluation: evaluation, changes: changes, hash: proposal.hash, passing: trial.PassingAssertions, penalty: simulationEvaluationPenalty(evaluation)})
				slices.SortFunc(next, compareElectricalRepairStatesV22)
				if len(next) > limits.BeamWidth {
					next = next[:limits.BeamWidth]
				}
				result.Consumption.MaximumFrontier = max(result.Consumption.MaximumFrontier, len(next))
			}
			if batch.exhausted {
				result.StopReason = "binding_work_exhausted"
				result.Consumption.BudgetExhausted = true
				return
			}
		}
		frontier = next
	}
	result.StopReason = "repair_frontier_exhausted"
	if len(frontier) != 0 {
		result.StopReason = "repair_depth_exhausted"
		result.Consumption.BudgetExhausted = true
	}
	return
}

func VerifyElectricalRepairSelectionV23(ctx context.Context, result ElectricalRepairResultV23, requirement Requirement, before CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment) error {
	unsigned := result
	unsigned.Hash = ""
	if result.Schema != "kicadai.electrical-repair.v23" || result.SolverPolicyID != simmodel.SolverIDV23 || result.SolverPolicyHash != simmodel.SolverSHA256V23() || result.Status != RepairSearchPassed || result.StopReason != "complete_electrical_certificate" || result.Selected == nil || !validDigestV22(result.Hash) || causalCrossStageHash(unsigned) != result.Hash {
		return fmt.Errorf("V23 electrical repair is not an authenticated selection")
	}
	limits := result.Limits
	proof := result.Selected.Certificate.PrerequisiteProof
	if len(proof.Changes) == 0 || len(proof.StepGraphHashes) != len(proof.Changes) {
		return fmt.Errorf("V23 electrical repair certificate path is incomplete")
	}
	if limits.MaxDepth <= 0 || limits.BeamWidth <= 0 || limits.MaxBindingWork <= 0 || limits.MaxEvaluations <= 0 || limits.MaxCorners <= 0 || result.BindingWork < 0 || result.Consumption.MaximumFrontier < 1 || result.RequirementHash != proof.RequirementHash || result.InventoryHash != proof.InventoryHash || result.InitialGraphHash != proof.BeforeGraphHash || !validDigestV22(result.InitialEvaluationHash) || len(proof.Changes) > limits.MaxDepth || result.BindingWork > limits.MaxBindingWork || result.Consumption.MaximumFrontier > limits.BeamWidth || result.Consumption.CandidateSimulations > limits.MaxEvaluations || result.Consumption.CornerEvaluations > limits.MaxCorners || result.Consumption.BudgetExhausted || len(result.Trials) == 0 {
		return fmt.Errorf("V23 electrical repair identities or count limits differ")
	}
	simulations, corners := 0, 0
	for index, trial := range result.Trials {
		if trial.Number != index+1 || trial.Depth < 1 || trial.Depth > limits.MaxDepth || trial.SimulationCalls < 1 || trial.CornerEvaluations < 0 || trial.SimulationCalls > limits.MaxEvaluations-simulations || trial.CornerEvaluations > limits.MaxCorners-corners || !validDigestV22(trial.ExecutionHash) || !validDigestV22(trial.EvaluationHash) || !validDigestV22(trial.GraphHash) || !validDigestV22(trial.ParentGraphHash) {
			return fmt.Errorf("V23 electrical repair trial ledger is invalid")
		}
		simulations += trial.SimulationCalls
		corners += trial.CornerEvaluations
	}
	last := result.Trials[len(result.Trials)-1]
	evaluation := result.Selected.Evaluation
	if simulations != result.Consumption.CandidateSimulations || corners != result.Consumption.CornerEvaluations || len(result.Trials) != result.Consumption.TopologyRepairs || last.Status != SimulationEvaluationPassed || !last.Continuable || last.Rejection != "" || last.GraphHash != proof.GraphHash || last.EvaluationHash != evaluation.Evaluation.Hash || last.ExecutionHash != evaluation.Hash || last.Depth != len(proof.Changes) || causalCrossStageHash(last.Change) != causalCrossStageHash(proof.Changes[len(proof.Changes)-1]) || last.SimulationCalls != max(1, evaluation.Evaluation.Consumption.CandidateSimulations) || last.CornerEvaluations != evaluation.Evaluation.Consumption.CornerEvaluations {
		return fmt.Errorf("V23 electrical selection differs from its final trial or accounting")
	}
	parent, previous := result.InitialGraphHash, 0
	for depth, graphHash := range proof.StepGraphHashes {
		found := false
		for _, trial := range result.Trials {
			if trial.Number > previous && trial.Depth == depth+1 && trial.ParentGraphHash == parent && trial.GraphHash == graphHash && trial.Continuable && trial.Rejection == "" && causalCrossStageHash(trial.Change) == causalCrossStageHash(proof.Changes[depth]) {
				previous, parent, found = trial.Number, graphHash, true
				break
			}
		}
		if !found {
			return fmt.Errorf("V23 certificate path is absent from the accepted trial ledger")
		}
	}
	return verifyElectricalCertificateV23(ctx, result.Selected.Certificate, requirement, before, result.Selected.Graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), evaluation)
}
