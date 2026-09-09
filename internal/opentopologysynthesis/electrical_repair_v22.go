package opentopologysynthesis

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"kicadai/internal/simulationadmission"
)

// ElectricalRepairLimitsV22 are independent count limits, never elapsed-time
// cutoffs. The frozen public evaluation must record these exact inputs.
type ElectricalRepairLimitsV22 struct {
	MaxDepth       int `json:"max_depth"`
	BeamWidth      int `json:"beam_width"`
	MaxBindingWork int `json:"max_binding_work"`
	MaxEvaluations int `json:"max_evaluations"`
	MaxCorners     int `json:"max_corners"`
}

func DefaultElectricalRepairLimitsV22() ElectricalRepairLimitsV22 {
	return ElectricalRepairLimitsV22{MaxDepth: 4, BeamWidth: 8, MaxBindingWork: 4096, MaxEvaluations: 128, MaxCorners: 4096}
}

type ElectricalRepairTrialV22 struct {
	Number            int                        `json:"number"`
	Depth             int                        `json:"depth"`
	ParentGraphHash   string                     `json:"parent_graph_sha256"`
	GraphHash         string                     `json:"graph_sha256"`
	EvaluationHash    string                     `json:"evaluation_sha256"`
	Change            GraphChange                `json:"change"`
	Status            SimulationEvaluationStatus `json:"status"`
	PassingAssertions int                        `json:"passing_assertions"`
	Continuable       bool                       `json:"continuable"`
	Rejection         string                     `json:"rejection,omitempty"`
}

type ElectricalRepairSelectionV22 struct {
	Graph       CandidateGraph           `json:"graph"`
	Evaluation  SimulationEvaluation     `json:"evaluation"`
	Certificate ElectricalCertificateV22 `json:"certificate"`
}

// Only the selected numerical result retains full solver reports. Rejected
// trials retain exact identities and decisions without duplicating waveforms.
type ElectricalRepairResultV22 struct {
	Schema                string                        `json:"schema"`
	RequirementHash       string                        `json:"requirement_sha256"`
	InventoryHash         string                        `json:"inventory_sha256"`
	InitialGraphHash      string                        `json:"initial_graph_sha256"`
	InitialEvaluationHash string                        `json:"initial_evaluation_sha256"`
	Status                RepairSearchStatus            `json:"status"`
	StopReason            string                        `json:"stop_reason"`
	Limits                ElectricalRepairLimitsV22     `json:"limits"`
	BindingWork           int                           `json:"binding_work"`
	Consumption           Consumption                   `json:"consumption"`
	Trials                []ElectricalRepairTrialV22    `json:"trials"`
	Selected              *ElectricalRepairSelectionV22 `json:"selected,omitempty"`
	Hash                  string                        `json:"hash"`
}

type electricalRepairStateV22 struct {
	graph      CandidateGraph
	evaluation SimulationEvaluation
	changes    []GraphChange
	hash       string
	passing    int
	penalty    float64
}

func RepairElectricalV22(ctx context.Context, requirement Requirement, graph CandidateGraph, initial SimulationEvaluation, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment, policy Policy, limits ElectricalRepairLimitsV22) ElectricalRepairResultV22 {
	return repairElectricalPreparedV22(ctx, requirement, graph, initial, inventory, environment, simulationadmission.PrepareEnvironment(admission), policy, limits)
}

func repairElectricalPreparedV22(ctx context.Context, requirement Requirement, graph CandidateGraph, initial SimulationEvaluation, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.PreparedEnvironment, policy Policy, limits ElectricalRepairLimitsV22) (result ElectricalRepairResultV22) {
	result = ElectricalRepairResultV22{Schema: "kicadai.electrical-repair.v22", InventoryHash: inventory.Hash, InitialEvaluationHash: initial.Hash, Limits: limits, Status: RepairSearchUnsupported, Trials: []ElectricalRepairTrialV22{}}
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
				evaluation := evaluateElectricalPreparedV22(ctx, requirement, proposal.graph, nil, inventory, environment, admission, candidatePolicy)
				result.Consumption.CandidateSimulations += evaluation.Consumption.CandidateSimulations
				result.Consumption.CornerEvaluations += evaluation.Consumption.CornerEvaluations
				// Admission refusals also consume an invocation; otherwise a set of
				// unsupported proposals could evade the evaluation count limit.
				if evaluation.Consumption.CandidateSimulations == 0 {
					result.Consumption.CandidateSimulations++
				}
				result.Consumption.TopologyRepairs++
				allowed, rejection := electricalContinuationAllowedV22(requirement, state.evaluation, evaluation)
				trial := ElectricalRepairTrialV22{Number: len(result.Trials) + 1, Depth: depth, ParentGraphHash: state.hash, GraphHash: proposal.hash, EvaluationHash: evaluation.Hash, Change: proposal.change, Status: evaluation.Status, PassingAssertions: electricalPassingCountV22(evaluation), Continuable: allowed, Rejection: rejection}
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
					certificate, err := certifyElectricalPathV22(ctx, requirement, graph, proposal.graph, inventory, environment, admission, changes, evaluation)
					if err != nil {
						result.Trials[len(result.Trials)-1].Continuable = false
						result.Trials[len(result.Trials)-1].Rejection = "certificate_rejected"
						if ctx.Err() != nil {
							result.Status, result.StopReason = RepairSearchCanceled, "canceled"
							return
						}
						continue
					}
					result.Selected = &ElectricalRepairSelectionV22{Graph: proposal.graph, Evaluation: evaluation, Certificate: certificate}
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

func electricalAttemptKeyV22(attempt SimulationAttempt) string {
	return attempt.RequirementID + "\x00" + attempt.OperatingCase + "\x00" + attempt.CornerID
}

func electricalPassingCountV22(evaluation SimulationEvaluation) int {
	count := 0
	for _, attempt := range evaluation.Attempts {
		if attempt.Status == SimulationEvaluationPassed && attempt.AssertionPass {
			count++
		}
	}
	return count
}

func electricalCriticalFailureV22(requirement Requirement, evaluation SimulationEvaluation) bool {
	critical := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		critical[assertion.ID] = assertion.Critical
	}
	for _, attempt := range evaluation.Attempts {
		if critical[attempt.RequirementID] && (attempt.Status != SimulationEvaluationPassed || !attempt.AssertionPass) {
			return true
		}
	}
	for _, diagnosis := range evaluation.Diagnoses {
		if critical[diagnosis.RequirementID] {
			return true
		}
	}
	return false
}

// An unobserved later gate is unknown, not a regression. Conversely, every
// previously passing corner must still pass; missing evidence is not success.
// Neutral but safe candidates may occupy the bounded beam for compound edits.
func electricalContinuationAllowedV22(requirement Requirement, before, after SimulationEvaluation) (bool, string) {
	if after.Status != SimulationEvaluationFailed && after.Status != SimulationEvaluationPassed {
		return false, "non_numerical_result"
	}
	if electricalCriticalFailureV22(requirement, after) {
		return false, "critical_failure"
	}
	current := map[string]SimulationAttempt{}
	for _, attempt := range after.Attempts {
		current[electricalAttemptKeyV22(attempt)] = attempt
	}
	for _, attempt := range before.Attempts {
		if attempt.Status != SimulationEvaluationPassed || !attempt.AssertionPass {
			continue
		}
		now, found := current[electricalAttemptKeyV22(attempt)]
		if !found || now.Status != SimulationEvaluationPassed || !now.AssertionPass {
			return false, "previously_passing_corner_lost"
		}
	}
	return true, ""
}

func compareElectricalRepairStatesV22(a, b electricalRepairStateV22) int {
	return cmp.Or(cmp.Compare(b.passing, a.passing), cmp.Compare(a.penalty, b.penalty), cmp.Compare(a.hash, b.hash))
}

func VerifyElectricalRepairSelectionV22(ctx context.Context, result ElectricalRepairResultV22, requirement Requirement, before CandidateGraph, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment) error {
	unsigned := result
	unsigned.Hash = ""
	if result.Status != RepairSearchPassed || result.Selected == nil || result.Hash == "" || causalCrossStageHash(unsigned) != result.Hash {
		return fmt.Errorf("electrical repair result is not an authenticated selection")
	}
	certificate := result.Selected.Certificate
	if result.RequirementHash != certificate.RequirementHash || result.InventoryHash != certificate.InventoryHash || result.InitialGraphHash != certificate.BeforeGraphHash || len(certificate.Changes) > result.Limits.MaxDepth || result.BindingWork > result.Limits.MaxBindingWork || result.Consumption.MaximumFrontier > result.Limits.BeamWidth || result.Consumption.CandidateSimulations > result.Limits.MaxEvaluations || result.Consumption.CornerEvaluations > result.Limits.MaxCorners {
		return fmt.Errorf("electrical repair selection identity or count limits differ")
	}
	return verifyElectricalCertificateV22(ctx, result.Selected.Certificate, requirement, before, result.Selected.Graph, inventory, environment, simulationadmission.PrepareEnvironment(admission), result.Selected.Evaluation)
}
