package capabilityfeedback

import (
	"fmt"

	"kicadai/internal/opentopologysynthesis"
	"kicadai/internal/simulationadmission"
)

// ObserveRealizabilityAwareV20 adds admission-specific failure classification
// without changing any historically frozen observer or policy implementation.
func ObserveRealizabilityAwareV20(
	meta CaseMeta,
	requirement opentopologysynthesis.Requirement,
	run opentopologysynthesis.SynthesisRun,
	promotion *opentopologysynthesis.PhysicalPromotionResult,
) (CaseEvidence, error) {
	evidence, err := ObserveRealizabilityAware(meta, requirement, run, promotion)
	if err != nil {
		return CaseEvidence{}, err
	}
	changed := false
	for index := range evidence.Gaps {
		analysis := ""
		if len(evidence.Gaps[index].AnalysisKinds) != 0 {
			analysis = evidence.Gaps[index].AnalysisKinds[0]
		} else if len(evidence.AnalysisKinds) == 1 {
			analysis = evidence.AnalysisKinds[0]
		}
		if replacement, found := simulationAdmissionGapV20(evidence.Gaps[index], analysis); found {
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
		return CaseEvidence{}, fmt.Errorf("case %q produced invalid V20 admission-aware evidence: %w", meta.ID, err)
	}
	return evidence, nil
}

func simulationAdmissionGapV20(original Gap, analysis string) (Gap, bool) {
	gap := original
	gap.Stage = "simulation_admission"
	switch gap.Code {
	case string(simulationadmission.CodeMissingModel),
		string(simulationadmission.CodeIncompatibleModel),
		string(simulationadmission.CodeInvalidModelParameters):
		gap.Scope = ScopeModel
		gap.Capability = analysisCapabilityV20(analysis, "model", "trusted_simulation_model")
	case string(simulationadmission.CodeMissingAnalysisDefinition):
		gap.Scope = ScopeSimulation
		gap.Capability = analysisCapabilityV20(analysis, "definition", "deterministic_analysis_definition")
	case string(simulationadmission.CodeUnsupportedAnalysis),
		string(simulationadmission.CodeSolverUnavailable):
		gap.Scope = ScopeSimulation
		gap.Capability = analysisCapabilityV20(analysis, "solver", "trusted_simulation_solver")
	case string(simulationadmission.CodeSolverModelIncompatible):
		gap.Scope = ScopeModel
		gap.Capability = analysisCapabilityV20(analysis, "model_solver_compatibility", "model_solver_compatibility")
	default:
		return Gap{}, false
	}
	gap.RequiredEvidence = requiredEvidence(gap.Scope, gap.Capability)
	return gap, true
}

func analysisCapabilityV20(analysis, suffix, fallback string) string {
	if analysis == "" {
		return fallback
	}
	return canonicalID(analysis) + "_" + suffix
}
