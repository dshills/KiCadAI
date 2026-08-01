package opentopologysynthesis

import (
	"slices"

	"kicadai/internal/simmodel"
)

// reviewedModelSupportsCircuitAnalysis distinguishes a circuit workflow from
// the electrical primitive used inside that workflow. Electrothermal analysis
// advances the transient electrical system and adds thermal/SOA state only for
// devices that provide that evidence, so a reviewed transient-only primitive
// remains valid as an electrical participant. The workflow itself still fails
// closed unless the complete graph resolves and the electrothermal solver finds
// reviewed thermal or transient-SOA evidence.
func reviewedModelSupportsCircuitAnalysis(allowed []string, analysis string) bool {
	if slices.Contains(allowed, analysis) {
		return true
	}
	return analysis == simmodel.AnalysisElectrothermal &&
		slices.Contains(allowed, simmodel.AnalysisTransient)
}

func reviewedPrimitiveModelSupportsCircuitAnalysis(
	model PrimitiveModelContract,
	analysis string,
) bool {
	dependencies := simmodel.CatalogAnalysisDependencies(model.ModelID, []string{analysis})
	if len(dependencies) == 0 {
		return false
	}
	for _, dependency := range dependencies {
		if !reviewedModelSupportsCircuitAnalysis(model.AllowedAnalyses, dependency) {
			return false
		}
	}
	return true
}

func reviewedCatalogModelSupportsCircuitAnalysis(
	modelID string,
	allowed []string,
	analysis string,
) bool {
	dependencies := simmodel.CatalogAnalysisDependencies(modelID, []string{analysis})
	if len(dependencies) == 0 {
		return false
	}
	for _, dependency := range dependencies {
		if !reviewedModelSupportsCircuitAnalysis(allowed, dependency) {
			return false
		}
	}
	return true
}
