package opentopologysynthesis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"kicadai/internal/repairloop"
)

// CausalCrossStageTargetOptions binds the generic cross-stage coordinator to
// the simulation-guided causal repair search. All inputs are explicit so a
// captured target can be restored and replayed without hidden process state.
type CausalCrossStageTargetOptions struct {
	Requirement Requirement
	Graph       CandidateGraph
	Evaluation  SimulationEvaluation
	Inventory   PrimitiveInventory
	Environment SimulationEnvironment
	Policy      Policy
}

// CausalCrossStageTarget exposes causal electrical repairs through the shared
// transactional repair contract.
type CausalCrossStageTarget struct {
	opts            CausalCrossStageTargetOptions
	graph           CandidateGraph
	evaluation      SimulationEvaluation
	lastReenter     repairloop.CrossStage
	diagnoses       map[string]Diagnosis
	analysisKey     string
	evaluated       []causalEvaluatedCandidate
	proposalTrials  map[string]causalEvaluatedCandidate
	selected        *causalEvaluatedCandidate
	selectedID      string
	proposalApplies map[string]int
}

type causalCrossStageState struct {
	Graph       CandidateGraph        `json:"graph"`
	Evaluation  SimulationEvaluation  `json:"evaluation"`
	LastReenter repairloop.CrossStage `json:"last_reenter,omitempty"`
}

func NewCausalCrossStageTarget(opts CausalCrossStageTargetOptions) (*CausalCrossStageTarget, error) {
	if _, err := GraphHash(opts.Graph); err != nil {
		return nil, fmt.Errorf("cross-stage causal target graph: %w", err)
	}
	if opts.Evaluation.Hash == "" {
		return nil, errors.New("cross-stage causal target requires evaluated simulation evidence")
	}
	target := &CausalCrossStageTarget{
		opts: opts, graph: CloneGraph(opts.Graph), evaluation: opts.Evaluation,
		diagnoses: map[string]Diagnosis{}, proposalTrials: map[string]causalEvaluatedCandidate{},
		proposalApplies: map[string]int{},
	}
	return target, nil
}

func (target *CausalCrossStageTarget) Capture(context.Context) (repairloop.CrossStageCheckpoint, error) {
	state := causalCrossStageState{
		Graph: CloneGraph(target.graph), Evaluation: target.evaluation, LastReenter: target.lastReenter,
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return repairloop.CrossStageCheckpoint{}, err
	}
	scopes, err := causalCrossStageScopeHashes(target.opts.Requirement, target.graph, target.evaluation)
	if err != nil {
		return repairloop.CrossStageCheckpoint{}, err
	}
	return repairloop.NewCrossStageCheckpoint(
		payload, scopes, causalCrossStageGates(target.evaluation),
		causalCrossStageMargins(target.opts.Requirement, target.evaluation),
	), nil
}

func (target *CausalCrossStageTarget) Restore(_ context.Context, checkpoint repairloop.CrossStageCheckpoint) error {
	var state causalCrossStageState
	if err := json.Unmarshal(checkpoint.Payload, &state); err != nil {
		return err
	}
	if _, err := GraphHash(state.Graph); err != nil {
		return err
	}
	target.graph = CloneGraph(state.Graph)
	target.evaluation = state.Evaluation
	target.lastReenter = state.LastReenter
	target.selected = nil
	target.selectedID = ""
	return nil
}

func (target *CausalCrossStageTarget) Diagnose(context.Context) ([]repairloop.CrossStageDiagnostic, error) {
	target.diagnoses = map[string]Diagnosis{}
	result := make([]repairloop.CrossStageDiagnostic, 0, len(target.evaluation.Diagnoses))
	for _, diagnosis := range target.evaluation.Diagnoses {
		evidence := strings.TrimSpace(diagnosis.EvidenceHash)
		if evidence == "" {
			evidence = causalCrossStageHash(diagnosis)
		}
		scope := []string{"requirement:" + diagnosis.RequirementID}
		if diagnosis.AffectedConeHash != "" {
			scope = append(scope, "cone:"+diagnosis.AffectedConeHash)
		}
		diagnostic := repairloop.NewCrossStageDiagnostic(
			repairloop.CrossStageSimulation, diagnosis.Code, electricalRepairCategory(diagnosis),
			repairloop.CrossStageSeverityBlocking, evidence, scope,
		)
		target.diagnoses[diagnostic.Hash] = diagnosis
		result = append(result, diagnostic)
	}
	return result, nil
}

func (target *CausalCrossStageTarget) Propose(ctx context.Context, diagnostic repairloop.CrossStageDiagnostic) ([]repairloop.CrossStageProposal, error) {
	diagnosis, ok := target.diagnoses[diagnostic.Hash]
	if !ok {
		return nil, errors.New("cross-stage causal diagnostic is not current")
	}
	graphHash, _ := GraphHash(target.graph)
	analysisKey := graphHash + ":" + target.evaluation.Hash
	if target.analysisKey != analysisKey {
		_, target.evaluated = analyzeCausalRepairs(
			ctx, target.opts.Requirement, target.graph, target.evaluation,
			target.opts.Inventory, target.opts.Environment, target.opts.Policy,
		)
		target.analysisKey = analysisKey
		target.proposalTrials = map[string]causalEvaluatedCandidate{}
	}

	result := []repairloop.CrossStageProposal{}
	for _, candidate := range target.evaluated {
		if !causalTrialAddressesDiagnosis(candidate.trial, diagnosis) {
			continue
		}
		affected := causalCrossStageAffectedStages(candidate.trial)
		scope := causalCrossStageProposalScopes(candidate.trial)
		effects := causalCrossStageExpectedEffects(candidate.trial)
		expectedImprovement := math.Max(0, math.Max(candidate.trial.Improvement, candidate.trial.Sensitivity))
		proposal := repairloop.NewCrossStageProposal(
			diagnostic, candidate.trial.Repair.Operator, affected, effects, scope,
			len(candidate.trial.Perturbations), candidate.trial.ChangeMagnitude, expectedImprovement,
			candidate.trial.Authorized, candidate.trial.Rejection,
		)
		target.proposalTrials[proposal.ID] = candidate
		result = append(result, proposal)
	}
	return result, nil
}

func (target *CausalCrossStageTarget) Apply(_ context.Context, proposal repairloop.CrossStageProposal) error {
	candidate, ok := target.proposalTrials[proposal.ID]
	if !ok {
		return errors.New("cross-stage causal proposal is not current")
	}
	target.graph = CloneGraph(candidate.graph)
	target.selected = &candidate
	target.selectedID = proposal.ID
	target.proposalApplies[proposal.ID]++
	return nil
}

func (target *CausalCrossStageTarget) Reenter(ctx context.Context, stage repairloop.CrossStage) error {
	if target.selected == nil {
		return errors.New("cross-stage causal re-entry lacks an applied proposal")
	}
	if target.selectedID != "" && target.proposalApplies[target.selectedID] == 1 {
		target.evaluation = target.selected.trial.Evaluation
	} else {
		target.evaluation = EvaluateCandidate(
			ctx, target.opts.Requirement, target.graph, nil,
			target.opts.Inventory, target.opts.Environment, target.selected.trial.Evaluation.Policy,
		)
	}
	target.lastReenter = stage
	target.selected = nil
	target.selectedID = ""
	return nil
}

func causalCrossStageScopeHashes(requirement Requirement, graph CandidateGraph, evaluation SimulationEvaluation) ([]repairloop.CrossStageScopeHash, error) {
	topologyHash, err := TopologyHash(graph)
	if err != nil {
		return nil, err
	}
	requirementHash := evaluation.RequirementHash
	if requirementHash == "" {
		requirementHash = causalCrossStageHash(requirement)
	}
	result := []repairloop.CrossStageScopeHash{
		{Scope: "requirement", Hash: requirementHash},
		{Scope: "topology", Hash: topologyHash},
	}
	for _, instance := range graph.Instances {
		result = append(result, repairloop.CrossStageScopeHash{
			Scope: "instance:" + instance.ID, Hash: causalCrossStageHash(instance),
		})
	}
	return result, nil
}

func causalCrossStageGates(evaluation SimulationEvaluation) []repairloop.CrossStageGate {
	result := make([]repairloop.CrossStageGate, 0, len(evaluation.Attempts))
	for _, attempt := range evaluation.Attempts {
		status := repairloop.CrossStageGateBlocked
		if attempt.AssertionPass {
			status = repairloop.CrossStageGatePassed
		}
		result = append(result, repairloop.CrossStageGate{
			ID: "simulation:" + causalCrossStageAttemptID(attempt), Stage: repairloop.CrossStageSimulation,
			Status: status, Required: true, EvidenceHash: causalCrossStageHash(attempt),
		})
	}
	return result
}

func causalCrossStageMargins(requirement Requirement, evaluation SimulationEvaluation) []repairloop.CrossStageMargin {
	critical := map[string]bool{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		critical[assertion.ID] = assertion.Critical
	}
	result := []repairloop.CrossStageMargin{}
	for _, attempt := range evaluation.Attempts {
		if !critical[attempt.RequirementID] {
			continue
		}
		headroom := causalAttemptMargin(attempt)
		if math.IsNaN(headroom) || math.IsInf(headroom, 0) {
			continue
		}
		result = append(result, repairloop.CrossStageMargin{
			ID: "simulation:" + causalCrossStageAttemptID(attempt), Stage: repairloop.CrossStageSimulation,
			Headroom: headroom, Protected: true, EvidenceHash: causalCrossStageHash(attempt),
		})
	}
	return result
}

func causalTrialAddressesDiagnosis(trial CausalRepairTrial, diagnosis Diagnosis) bool {
	if diagnosis.EvidenceHash != "" && trial.Repair.DiagnosisEvidenceHash == diagnosis.EvidenceHash {
		return true
	}
	for _, effect := range trial.Effects {
		if effect.RequirementID == diagnosis.RequirementID && (effect.TrialPass || effect.ViolationDelta > 0) {
			return true
		}
	}
	return false
}

func causalCrossStageAffectedStages(trial CausalRepairTrial) []repairloop.CrossStage {
	for _, change := range trial.Repair.Changes {
		if change.Kind == "add_primitive" || change.Kind == "redirect_terminal" {
			return []repairloop.CrossStage{
				repairloop.CrossStageSynthesis, repairloop.CrossStageSizing, repairloop.CrossStageSimulation,
			}
		}
	}
	return []repairloop.CrossStage{repairloop.CrossStageSizing, repairloop.CrossStageSimulation}
}

func causalCrossStageProposalScopes(trial CausalRepairTrial) []string {
	result := []string{}
	for _, change := range trial.Repair.Changes {
		if change.Primitive != "" {
			result = append(result, "instance:"+change.Primitive)
		}
		if change.Kind == "add_primitive" || change.Kind == "redirect_terminal" {
			result = append(result, "topology")
		}
	}
	if len(result) == 0 {
		for _, perturbation := range trial.Perturbations {
			if perturbation.InstanceID != "" {
				result = append(result, "instance:"+perturbation.InstanceID)
			}
		}
	}
	return causalCrossStageStrings(result)
}

func causalCrossStageExpectedEffects(trial CausalRepairTrial) []string {
	result := make([]string, 0, len(trial.Perturbations)+len(trial.Effects))
	for _, perturbation := range trial.Perturbations {
		result = append(result, "perturbation:"+perturbation.Hash)
	}
	for _, effect := range trial.Effects {
		if effect.TrialPass || effect.ViolationDelta > 0 {
			result = append(result, "assertion:"+strings.Join([]string{
				effect.RequirementID, effect.OperatingCase, effect.CornerID, effect.Analysis, effect.Metric,
			}, ":"))
		}
	}
	return causalCrossStageStrings(result)
}

func causalCrossStageAttemptID(attempt SimulationAttempt) string {
	return hex.EncodeToString(causalCrossStageDigest([]byte(causalAttemptKey(attempt))))
}

func causalCrossStageStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}

func causalCrossStageHash(value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		panic("causal cross-stage hash invariant: " + err.Error())
	}
	return hex.EncodeToString(causalCrossStageDigest(payload))
}

func causalCrossStageDigest(payload []byte) []byte {
	digest := sha256.Sum256(payload)
	return digest[:]
}
