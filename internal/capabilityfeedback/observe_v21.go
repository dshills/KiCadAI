package capabilityfeedback

import (
	"fmt"

	"kicadai/internal/opentopologysynthesis"
)

// ObserveRealizabilityAwareV21 preserves V20 admission classification and adds
// stable categories only for new V21 topology-completion diagnostics.
func ObserveRealizabilityAwareV21(
	meta CaseMeta,
	requirement opentopologysynthesis.Requirement,
	run opentopologysynthesis.SynthesisRun,
	promotion *opentopologysynthesis.PhysicalPromotionResult,
) (CaseEvidence, error) {
	evidence, err := ObserveRealizabilityAwareV20(meta, requirement, run, promotion)
	if err != nil {
		return CaseEvidence{}, err
	}
	changed := false
	for index := range evidence.Gaps {
		if replacement, found := topologyCompletionGapV21(evidence.Gaps[index]); found {
			evidence.Gaps[index] = replacement
			changed = true
		}
	}
	if !changed {
		return evidence, nil
	}
	evidence.Gaps = normalizeGaps(evidence.Gaps)
	evidence.Hash = ""
	evidence.Hash, err = caseEvidenceHash(evidence)
	if err != nil {
		return CaseEvidence{}, err
	}
	if err := ValidateCaseEvidence(evidence); err != nil {
		return CaseEvidence{}, fmt.Errorf("case %q produced invalid V21 topology-aware evidence: %w", meta.ID, err)
	}
	return evidence, nil
}

func topologyCompletionGapV21(original Gap) (Gap, bool) {
	gap := original
	switch gap.Code {
	case string(opentopologysynthesis.CodeTopologyNoApplicableV21):
		gap.Capability = "generic_topology_operation"
	case string(opentopologysynthesis.CodeTopologyContradictoryV21):
		gap.Capability = "causal_path_consistency"
	case string(opentopologysynthesis.CodeTopologyCycleV21):
		gap.Capability = "topology_cycle_resolution"
	case string(opentopologysynthesis.CodeTopologyBoundV21):
		gap.Capability = "bounded_topology_completion"
	case string(opentopologysynthesis.CodeTopologyInvalidRepairV21):
		gap.Capability = "topology_invariant_preservation"
	default:
		return Gap{}, false
	}
	gap.Stage, gap.Scope = "topology_completion", ScopeTopology
	gap.RequiredEvidence = requiredEvidence(gap.Scope, gap.Capability)
	return gap, true
}
