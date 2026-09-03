package opentopologysynthesis

import (
	"context"

	"kicadai/internal/simulationadmission"
)

// AdmittedSynthesisResultV20 keeps the historically sealed synthesis evidence
// unchanged while returning the exact authenticated request-level admission
// decision as a versioned sidecar.
type AdmittedSynthesisResultV20 struct {
	Synthesis SynthesisRun                 `json:"synthesis"`
	Admission simulationadmission.Decision `json:"admission"`
}

// SynthesizeV20 keeps a convenience entry point for callers that intentionally
// use one environment for the successor and both historical paths. Frozen
// evaluators use SynthesizeV20WithLegacy with all boundaries supplied
// independently.
func SynthesizeV20(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	return SynthesizeV20WithLegacy(
		ctx, requirement,
		inventory, environment,
		inventory, environment,
		inventory, environment,
		policy,
	)
}

// SynthesizeV20WithLegacy preserves an exact V18 result unless the historical
// run exposes a public typed blocker in the selected analysis/model/solver
// capability family. A selected nonpassing requirement receives one fresh
// bounded production synthesis run with admission enabled. V18 passes,
// invalid/infeasible inputs, and canceled runs are returned byte-for-byte.
func SynthesizeV20WithLegacy(
	ctx context.Context,
	requirement Requirement,
	v20Inventory PrimitiveInventory,
	v20Simulation SimulationEnvironment,
	v18Inventory PrimitiveInventory,
	v18Simulation SimulationEnvironment,
	legacyInventory PrimitiveInventory,
	legacySimulation SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	return synthesizeV20WithLegacyAndAdmission(
		ctx, requirement,
		v20Inventory, v20Simulation, bundledAdmissionEnvironmentV20(v20Simulation),
		v18Inventory, v18Simulation,
		legacyInventory, legacySimulation,
		policy,
	)
}

// SynthesizeAdmittedV20 is the production entry point for callers that have
// resolved explicit bundled, project, or configured model sources.
func SynthesizeAdmittedV20(
	ctx context.Context,
	requirement Requirement,
	inventory PrimitiveInventory,
	environment SimulationEnvironment,
	admission simulationadmission.Environment,
	policy Policy,
) AdmittedSynthesisResultV20 {
	prepared := simulationadmission.PrepareEnvironment(admission)
	decision := simulationadmission.AdmitPrepared(requirementSimulationAdmissionRequest(Normalize(requirement), inventory), prepared)
	return AdmittedSynthesisResultV20{
		Synthesis: synthesizeAdmissionPreparedV20(ctx, requirement, inventory, environment, prepared, policy),
		Admission: decision,
	}
}

func synthesizeV20WithLegacyAndAdmission(
	ctx context.Context,
	requirement Requirement,
	v20Inventory PrimitiveInventory,
	v20Simulation SimulationEnvironment,
	v20Admission simulationadmission.Environment,
	v18Inventory PrimitiveInventory,
	v18Simulation SimulationEnvironment,
	legacyInventory PrimitiveInventory,
	legacySimulation SimulationEnvironment,
	policy Policy,
) SynthesisRun {
	v18 := SynthesizeV18WithLegacy(
		ctx, requirement,
		v18Inventory, v18Simulation,
		legacyInventory, legacySimulation,
		policy,
	)
	if !analysisAdmissionFrontierV20(Normalize(requirement), v18) || ctx.Err() != nil {
		return v18
	}
	return synthesizeAdmissionV20(ctx, requirement, v20Inventory, v20Simulation, v20Admission, policy)
}

func analysisAdmissionFrontierV20(requirement Requirement, run SynthesisRun) bool {
	if run.Report.Status == StatusPassed || run.Report.Status == StatusCanceled ||
		run.Report.Status == StatusInvalid || run.Report.Status == StatusInfeasible {
		return false
	}
	assertions := map[string]BehavioralAssertion{}
	for _, assertion := range requirement.Requirements.BehavioralRequirements {
		assertions[assertion.ID] = assertion
	}
	for _, candidate := range run.Candidates {
		for _, evaluation := range candidate.Evaluations {
			for _, diagnosis := range evaluation.Diagnoses {
				switch diagnosis.Code {
				case diagnosisMetricUnsupported:
					assertion, found := assertions[diagnosis.RequirementID]
					if !found {
						continue
					}
					if _, _, supported := directSimulationQuantityForRequirementV20(requirement, assertion); supported {
						return true
					}
				case diagnosisSimulationInvalid:
					switch diagnosis.Analysis {
					case simmodelAnalysisDCOperatingPointV20, simmodelAnalysisACSweepV20,
						simmodelAnalysisDCSweepV20, simmodelAnalysisTransientV20,
						simmodelAnalysisStabilityV20:
						return true
					}
				}
			}
		}
	}
	return false
}

func bundledAdmissionEnvironmentV20(environment SimulationEnvironment) simulationadmission.Environment {
	source, err := simulationadmission.NewSource(
		"embedded-v20-model-provenance",
		simulationadmission.SourceBundled,
		environment.ModelRegistry,
	)
	if err != nil {
		return simulationadmission.Environment{}
	}
	return simulationadmission.Environment{
		Sources:        []simulationadmission.Source{source},
		EnabledSolvers: simulationadmission.EnabledBuiltinSolverIDs(),
	}
}

const (
	simmodelAnalysisDCOperatingPointV20 = "dc_operating_point"
	simmodelAnalysisACSweepV20          = "ac_sweep"
	simmodelAnalysisDCSweepV20          = "dc_sweep"
	simmodelAnalysisTransientV20        = "transient"
	simmodelAnalysisStabilityV20        = "stability"
)
