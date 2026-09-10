package opentopologysynthesis

import (
	"context"
	"kicadai/internal/simmodel"
	"kicadai/internal/simulationadmission"
	"slices"
)

// ElectricalSynthesisResultV23 preserves the exact V21 predecessor and binds
// the separately reviewed numerical policy, repair certificate, and physical
// lowering. It is not an installed-KiCad promotion result.
type ElectricalSynthesisResultV23 struct {
	Schema           string                     `json:"schema"`
	SolverPolicyID   string                     `json:"solver_policy_id"`
	SolverPolicyHash string                     `json:"solver_policy_sha256"`
	PredecessorHash  string                     `json:"predecessor_sha256"`
	Synthesis        SynthesisRun               `json:"synthesis"`
	ElectricalRepair *ElectricalRepairResultV23 `json:"electrical_repair,omitempty"`
	Hash             string                     `json:"hash"`
}

// Historical passes, unsafe/incomplete runs, and critical failures remain
// unchanged. Physical lowering is reachable only after the full V23 certificate
// verifies; the enclosing result retains its exact execution-policy identity.
func SynthesizeV23WithLegacy(ctx context.Context, requirement Requirement, inventory PrimitiveInventory, environment SimulationEnvironment, v20Inventory PrimitiveInventory, v20Environment SimulationEnvironment, v18Inventory PrimitiveInventory, v18Environment SimulationEnvironment, legacyInventory PrimitiveInventory, legacyEnvironment SimulationEnvironment, policy Policy, limits ElectricalRepairLimitsV22) ElectricalSynthesisResultV23 {
	v21 := SynthesizeV21WithLegacy(ctx, requirement, inventory, environment, v20Inventory, v20Environment, v18Inventory, v18Environment, legacyInventory, legacyEnvironment, policy)
	return ContinueElectricalV23(ctx, requirement, v21, inventory, environment, bundledAdmissionEnvironmentV20(environment), policy, limits)
}
func ContinueElectricalV23(ctx context.Context, requirement Requirement, v21 SynthesisRun, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment, policy Policy, limits ElectricalRepairLimitsV22) (result ElectricalSynthesisResultV23) {
	result = ElectricalSynthesisResultV23{Schema: "kicadai.electrical-synthesis.v23", SolverPolicyID: simmodel.SolverIDV23, SolverPolicyHash: simmodel.SolverSHA256V23(), PredecessorHash: v21.Hash, Synthesis: v21}
	defer func() {
		repairHash := ""
		if result.ElectricalRepair != nil {
			repairHash = result.ElectricalRepair.Hash
		}
		result.Hash = causalCrossStageHash(struct{ Schema, SolverPolicyID, SolverPolicyHash, PredecessorHash, SynthesisHash, RepairHash string }{result.Schema, result.SolverPolicyID, result.SolverPolicyHash, result.PredecessorHash, result.Synthesis.Hash, repairHash})
	}()
	if ctx.Err() != nil || v21.Hash == "" || v21.Report.Status != StatusFailed || v21.Report.StopReason != StopNoPassingGraph {
		return
	}
	requirement = Normalize(requirement)
	// A diagnosis shared by all earlier candidates is not an impossibility
	// proof after a complete topology has been certified. In particular, one
	// candidate's electrical failure is necessarily "universal" in V19's
	// structural-search predicate. Admission and numerical gates still apply
	// independently to every V23 proposal; critical failures remain excluded.
	if len(Validate(requirement)) != 0 || allCandidateFailuresCriticalV19(requirement, v21) {
		return
	}
	index, graph, initial, found := electricalSourceV22(requirement, v21, inventory)
	if !found {
		return
	}
	if !authenticatedSynthesisV22(ctx, v21) {
		return
	}
	repaired := RepairElectricalV23(ctx, requirement, graph, initial, inventory, environment, admission, policy, limits)
	result.ElectricalRepair = &repaired
	if repaired.Status != RepairSearchPassed || repaired.Selected == nil {
		return
	}
	if VerifyElectricalRepairSelectionV23(ctx, repaired, requirement, graph, inventory, environment, admission) != nil {
		return
	}
	selected := repaired.Selected
	evaluation := selected.Evaluation.Evaluation
	physical := LowerPassingCandidate(ctx, requirement, selected.Graph, evaluation, inventory, environment)
	// Copy precisely the mutable containers. Unchanged historical waveform data
	// is shared read-only, not cloned or edited while creating the successor.
	run := v21
	run.Candidates = slices.Clone(v21.Candidates)
	run.Report.Candidates = slices.Clone(v21.Report.Candidates)
	run.Report.CatalogHash = environment.CatalogHash
	run.Report.PrimitiveInventoryHash = inventory.Hash
	run.Report.ModelRegistryHash = inventory.ModelRegistryHash
	run.Candidates[index].Evaluations = append(slices.Clone(v21.Candidates[index].Evaluations), evaluation)
	run.Candidates[index].Physical = append(slices.Clone(v21.Candidates[index].Physical), physical)
	run.Report.Candidates[index].Attempts = slices.Clone(v21.Report.Candidates[index].Attempts)
	change := Repair{Number: 1, Operator: "control_rebinding_v23", DiagnosisCode: "post_certificate_electrical_failure", DiagnosisEvidenceHash: initial.Hash, BeforeGraphHash: initial.GraphHash, AfterGraphHash: evaluation.GraphHash, ExpectedDirection: "all_required_assertions_pass", Changes: slices.Clone(selected.Certificate.PrerequisiteProof.Changes)}
	bridge := RepairSearchResult{Schema: RepairSearchSchema, Version: RepairSearchVersion, PolicyVersion: PolicyVersion, RequirementHash: repaired.RequirementHash, InventoryHash: inventory.Hash, InitialGraphHash: initial.GraphHash, InitialEvaluationHash: initial.Hash, Status: RepairSearchPassed, Policy: effectiveTopologyPolicy(policy), Consumption: repaired.Consumption, TopologyCompletionV21: v21.Candidates[index].Repair.TopologyCompletionV21, Attempts: []RepairAttempt{{Number: 1, Repair: change, GraphHash: evaluation.GraphHash, TopologyHash: mustTopologyHash(selected.Graph), Evaluation: evaluation, Improved: true, Status: RepairSearchPassed}}, Selected: &RepairedCandidate{Graph: selected.Graph, Repair: change, Repairs: []Repair{change}, Evaluation: evaluation}}
	bridge = finalizeRepairSearch(bridge)
	run.Candidates[index].Repair = &bridge
	run.Report.Candidates[index].Attempts = append(run.Report.Candidates[index].Attempts, synthesisAttempt(len(run.Report.Candidates[index].Attempts)+1, selected.Graph, nil, evaluation, &change))
	run.Report.Consumption.ExpandedStates += repaired.Consumption.ExpandedStates
	run.Report.Consumption.GeneratedGraphs += repaired.Consumption.GeneratedGraphs
	addSimulationConsumption(&run.Report.Consumption, repaired.Consumption)
	addRepairConsumption(&run.Report.Consumption, repaired.Consumption)
	if physical.Status != PhysicalLoweringReady {
		run.Report.Status, run.Report.StopReason = StatusFailed, StopPhysicalPromotionFailed
		run.Report.Diagnostics = []Diagnostic{}
		appendSynthesisDiagnostics(&run.Report, physical.Issues)
		result.Synthesis = finalizeSynthesisRunV17(run)
		return
	}
	result.Synthesis = selectRankedSynthesisResultV17(run, []synthesisPassingCandidate{{candidateIndex: index, graph: selected.Graph, evaluation: evaluation, repair: &bridge, physical: physical, margin: synthesisWorstNormalizedMargin(evaluation), repairCount: len(selected.Certificate.PrerequisiteProof.Changes)}})
	return
}
