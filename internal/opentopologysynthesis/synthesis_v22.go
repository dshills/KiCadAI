package opentopologysynthesis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"slices"

	"kicadai/internal/canonicaljsonstream"
	"kicadai/internal/simulationadmission"
)

// ElectricalSynthesisResultV22 keeps the new certificate separate from the
// historical synthesis schema. Its hash binds the two authenticated identities,
// without serializing multi-GB predecessor evidence an additional time.
type ElectricalSynthesisResultV22 struct {
	Schema           string                     `json:"schema"`
	PredecessorHash  string                     `json:"predecessor_sha256"`
	Synthesis        SynthesisRun               `json:"synthesis"`
	ElectricalRepair *ElectricalRepairResultV22 `json:"electrical_repair,omitempty"`
	Hash             string                     `json:"hash"`
}

func SynthesizeV22WithLegacy(ctx context.Context, requirement Requirement, inventory PrimitiveInventory, environment SimulationEnvironment, v20Inventory PrimitiveInventory, v20Environment SimulationEnvironment, v18Inventory PrimitiveInventory, v18Environment SimulationEnvironment, legacyInventory PrimitiveInventory, legacyEnvironment SimulationEnvironment, policy Policy, limits ElectricalRepairLimitsV22) ElectricalSynthesisResultV22 {
	v21 := SynthesizeV21WithLegacy(ctx, requirement, inventory, environment, v20Inventory, v20Environment, v18Inventory, v18Environment, legacyInventory, legacyEnvironment, policy)
	return ContinueElectricalV22(ctx, requirement, v21, inventory, environment, bundledAdmissionEnvironmentV20(environment), policy, limits)
}

// ContinueElectricalV22 applies only after an exact complete V21 certificate
// with its retained nonpassing electrical evaluation. Other predecessor runs,
// including passes and unsafe/incomplete results, are returned unchanged.
func ContinueElectricalV22(ctx context.Context, requirement Requirement, v21 SynthesisRun, inventory PrimitiveInventory, environment SimulationEnvironment, admission simulationadmission.Environment, policy Policy, limits ElectricalRepairLimitsV22) (result ElectricalSynthesisResultV22) {
	result = ElectricalSynthesisResultV22{Schema: "kicadai.electrical-synthesis.v22", PredecessorHash: v21.Hash, Synthesis: v21}
	defer func() {
		repairHash := ""
		if result.ElectricalRepair != nil {
			repairHash = result.ElectricalRepair.Hash
		}
		result.Hash = causalCrossStageHash(struct{ Schema, PredecessorHash, SynthesisHash, RepairHash string }{result.Schema, result.PredecessorHash, result.Synthesis.Hash, repairHash})
	}()
	if ctx.Err() != nil || v21.Hash == "" || v21.Report.Status != StatusFailed || v21.Report.StopReason != StopNoPassingGraph {
		return
	}
	requirement = Normalize(requirement)
	// A diagnosis shared by all earlier candidates is not an impossibility
	// proof after a complete topology has been certified. In particular, one
	// candidate's electrical failure is necessarily "universal" in V19's
	// structural-search predicate. Admission and numerical gates still apply
	// independently to every V22 proposal; critical failures remain excluded.
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
	repaired := RepairElectricalV22(ctx, requirement, graph, initial, inventory, environment, admission, policy, limits)
	result.ElectricalRepair = &repaired
	if repaired.Status != RepairSearchPassed || repaired.Selected == nil {
		return
	}
	if VerifyElectricalRepairSelectionV22(ctx, repaired, requirement, graph, inventory, environment, admission) != nil {
		return
	}
	selected := repaired.Selected
	physical := LowerPassingCandidate(ctx, requirement, selected.Graph, selected.Evaluation, inventory, environment)
	// Copy precisely the mutable containers. Unchanged historical waveform data
	// is shared read-only, not cloned or edited while creating the successor.
	run := v21
	run.Candidates = slices.Clone(v21.Candidates)
	run.Report.Candidates = slices.Clone(v21.Report.Candidates)
	run.Report.CatalogHash = environment.CatalogHash
	run.Report.PrimitiveInventoryHash = inventory.Hash
	run.Report.ModelRegistryHash = inventory.ModelRegistryHash
	run.Candidates[index].Evaluations = append(slices.Clone(v21.Candidates[index].Evaluations), selected.Evaluation)
	run.Candidates[index].Physical = append(slices.Clone(v21.Candidates[index].Physical), physical)
	run.Report.Candidates[index].Attempts = slices.Clone(v21.Report.Candidates[index].Attempts)
	change := Repair{Number: 1, Operator: "control_rebinding_v22", DiagnosisCode: "post_certificate_electrical_failure", DiagnosisEvidenceHash: initial.Hash, BeforeGraphHash: initial.GraphHash, AfterGraphHash: selected.Evaluation.GraphHash, ExpectedDirection: "all_required_assertions_pass", Changes: slices.Clone(selected.Certificate.Changes)}
	bridge := RepairSearchResult{Schema: RepairSearchSchema, Version: RepairSearchVersion, PolicyVersion: PolicyVersion, RequirementHash: repaired.RequirementHash, InventoryHash: inventory.Hash, InitialGraphHash: initial.GraphHash, InitialEvaluationHash: initial.Hash, Status: RepairSearchPassed, Policy: effectiveTopologyPolicy(policy), Consumption: repaired.Consumption, TopologyCompletionV21: v21.Candidates[index].Repair.TopologyCompletionV21, Attempts: []RepairAttempt{{Number: 1, Repair: change, GraphHash: selected.Evaluation.GraphHash, TopologyHash: mustTopologyHash(selected.Graph), Evaluation: selected.Evaluation, Improved: true, Status: RepairSearchPassed}}, Selected: &RepairedCandidate{Graph: selected.Graph, Repair: change, Repairs: []Repair{change}, Evaluation: selected.Evaluation}}
	bridge = finalizeRepairSearch(bridge)
	run.Candidates[index].Repair = &bridge
	run.Report.Candidates[index].Attempts = append(run.Report.Candidates[index].Attempts, synthesisAttempt(len(run.Report.Candidates[index].Attempts)+1, selected.Graph, nil, selected.Evaluation, &change))
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
	result.Synthesis = selectRankedSynthesisResultV17(run, []synthesisPassingCandidate{{candidateIndex: index, graph: selected.Graph, evaluation: selected.Evaluation, repair: &bridge, physical: physical, margin: synthesisWorstNormalizedMargin(selected.Evaluation), repairCount: len(selected.Certificate.Changes)}})
	return
}

// Authenticate eligible predecessor content by streaming, so a caller cannot
// attach unrelated retained evidence to a frozen run identity. Cancellation
// interrupts hashing without allocating another serialized replay buffer.
func authenticatedSynthesisV22(ctx context.Context, run SynthesisRun) bool {
	want := run.Hash
	run.Hash = ""
	digest := sha256.New()
	if err := canonicaljsonstream.Encode(electricalHashWriterV22{ctx: ctx, Writer: digest}, run); err != nil {
		return false
	}
	return want != "" && want == hex.EncodeToString(digest.Sum(nil))
}

type electricalHashWriterV22 struct {
	ctx context.Context
	io.Writer
}

func (writer electricalHashWriterV22) Write(data []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	return writer.Writer.Write(data)
}

func electricalSourceV22(requirement Requirement, run SynthesisRun, inventory PrimitiveInventory) (int, CandidateGraph, SimulationEvaluation, bool) {
	requirementHash, err := CanonicalHash(requirement)
	if err != nil || requirementHash != run.Report.RequirementHash {
		return 0, CandidateGraph{}, SimulationEvaluation{}, false
	}
	// Canonical source ordering does not depend on electrical repair outcomes.
	indices := make([]int, len(run.Candidates))
	for i := range indices {
		indices[i] = i
	}
	slices.SortFunc(indices, func(a, b int) int {
		if run.Candidates[a].Fingerprint < run.Candidates[b].Fingerprint {
			return -1
		}
		if run.Candidates[a].Fingerprint > run.Candidates[b].Fingerprint {
			return 1
		}
		return a - b
	})
	for _, index := range indices {
		candidate := run.Candidates[index]
		if index >= len(run.Report.Candidates) || candidate.Fingerprint != run.Report.Candidates[index].Fingerprint || candidate.Repair == nil || candidate.Repair.TopologyCompletionV21 == nil {
			continue
		}
		plan := candidate.Repair.TopologyCompletionV21
		if plan.Selected == nil || plan.Status != "complete" {
			continue
		}
		selected := plan.Selected
		actual := AnalyzeTopologyV21(requirement, selected.Graph, inventory)
		if !actual.Complete || actual.Contradictory || actual.Hash != selected.Invariant.Hash || actual.GraphHash != selected.GraphHash || plan.RequirementHash != requirementHash || plan.InventoryHash != inventory.Hash {
			continue
		}
		evaluationHash := candidate.Repair.InitialEvaluationHash
		if len(selected.Operations) != 0 {
			evaluationHash = ""
			for _, attempt := range candidate.Repair.Attempts {
				if attempt.GraphHash == selected.GraphHash {
					evaluationHash = attempt.Evaluation.Hash
				}
			}
		}
		for _, evaluation := range candidate.Evaluations {
			if evaluation.Hash == evaluationHash && evaluation.Status == SimulationEvaluationFailed && evaluation.GraphHash == selected.GraphHash && evaluation.InventoryHash == inventory.Hash && evaluation.RequirementHash == requirementHash {
				return index, selected.Graph, evaluation, true
			}
		}
	}
	return 0, CandidateGraph{}, SimulationEvaluation{}, false
}
