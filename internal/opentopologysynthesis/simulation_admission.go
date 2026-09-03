package opentopologysynthesis

import (
	"slices"
	"strings"

	"kicadai/internal/simmodel"
	"kicadai/internal/simulationadmission"
)

func simulationAdmissionRequest(
	requirement Requirement,
	assertions []BehavioralAssertion,
	components []simmodel.ComponentEvidence,
) simulationadmission.Request {
	byKind := map[string]*simulationadmission.AnalysisRequirement{}
	for _, assertion := range assertions {
		kind := strings.TrimSpace(assertion.Analysis)
		analysis := byKind[kind]
		if analysis == nil {
			analysis = &simulationadmission.AnalysisRequirement{ID: kind, AuthoredKind: kind}
			byKind[kind] = analysis
		}
		analysis.Assertions = append(analysis.Assertions, assertion.ID)
		analysis.OperatingCases = append(analysis.OperatingCases, assertion.OperatingCases...)
		analysis.DCSweep = analysis.DCSweep || kind == "dc_sweep"
		dynamic := kind == simmodel.AnalysisTransient || kind == simmodel.AnalysisStartup ||
			kind == simmodel.AnalysisDistortion || kind == simmodel.AnalysisElectrothermal
		analysis.PeriodicExcitation = analysis.PeriodicExcitation ||
			(dynamic && assertion.FrequencyHz != nil && *assertion.FrequencyHz > 0)
		analysis.SmallSignalOperatingPoint = analysis.SmallSignalOperatingPoint ||
			kind == simmodel.AnalysisACSweep || kind == simmodel.AnalysisNoise || kind == simmodel.AnalysisStability
		analysis.ThermalBoundary = analysis.ThermalBoundary ||
			kind == simmodel.AnalysisThermal || kind == simmodel.AnalysisElectrothermal
	}
	analyses := make([]simulationadmission.AnalysisRequirement, 0, len(byKind))
	for _, analysis := range byKind {
		analyses = append(analyses, *analysis)
	}
	return simulationadmission.NormalizeRequest(simulationadmission.Request{
		Analyses: analyses, Components: components,
	})
}

func requirementSimulationAdmissionRequest(
	requirement Requirement,
	inventory PrimitiveInventory,
) simulationadmission.Request {
	request := simulationAdmissionRequest(
		requirement,
		requirement.Requirements.BehavioralRequirements,
		nil,
	)
	for _, primitive := range inventory.Primitives {
		for _, model := range primitive.Models {
			request.InventoryModels = append(request.InventoryModels, simulationadmission.CatalogModel{
				CatalogID: primitive.CatalogID,
				Family:    primitive.Family,
				ModelID:   model.ModelID,
			})
		}
	}
	return simulationadmission.NormalizeRequest(request)
}

func admissionSimulationDiagnostics(source []simulationadmission.Diagnostic) []SimulationDiagnostic {
	result := make([]SimulationDiagnostic, 0, len(source))
	for _, diagnostic := range source {
		result = append(result, SimulationDiagnostic{
			Code: string(diagnostic.Code), Path: diagnostic.Path,
			Message: diagnostic.Message, Suggestion: diagnostic.Suggestion,
		})
	}
	slices.SortStableFunc(result, func(left, right SimulationDiagnostic) int {
		if order := strings.Compare(left.Code, right.Code); order != 0 {
			return order
		}
		if order := strings.Compare(left.Path, right.Path); order != 0 {
			return order
		}
		return strings.Compare(left.Message, right.Message)
	})
	return result
}
