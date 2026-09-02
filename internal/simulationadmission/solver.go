package simulationadmission

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"kicadai/internal/simmodel"
)

const SolverRegistryVersion = 1

type SolverDefinition struct {
	ID             string   `json:"id"`
	Version        int      `json:"version"`
	Analysis       string   `json:"analysis"`
	WorkflowModels []string `json:"workflow_models"`
	Deterministic  bool     `json:"deterministic"`
	SHA256         string   `json:"sha256"`
}

var solverRegistry = []SolverDefinition{
	{ID: "kicadai_ac_mna_v1", Version: 1, Analysis: simmodel.AnalysisACSweep, WorkflowModels: []string{simmodel.ModelLinearCircuitMNAV1}, Deterministic: true},
	{ID: "kicadai_dc_mna_v1", Version: 1, Analysis: simmodel.AnalysisDCOperatingPoint, WorkflowModels: []string{simmodel.ModelLinearCircuitMNAV1, simmodel.ModelNonlinearCircuitDCV1, simmodel.ModelTransientCircuitV1}, Deterministic: true},
	{ID: "kicadai_distortion_mna_v1", Version: 1, Analysis: simmodel.AnalysisDistortion, WorkflowModels: []string{simmodel.ModelTransientCircuitV1}, Deterministic: true},
	{ID: "kicadai_electrothermal_mna_v1", Version: 1, Analysis: simmodel.AnalysisElectrothermal, WorkflowModels: []string{simmodel.ModelTransientCircuitV1}, Deterministic: true},
	{ID: "kicadai_noise_mna_v1", Version: 1, Analysis: simmodel.AnalysisNoise, WorkflowModels: []string{simmodel.ModelLinearCircuitMNAV1}, Deterministic: true},
	{ID: "kicadai_stability_mna_v1", Version: 1, Analysis: simmodel.AnalysisStability, WorkflowModels: []string{simmodel.ModelLinearCircuitMNAV1}, Deterministic: true},
	{ID: "kicadai_startup_mna_v1", Version: 1, Analysis: simmodel.AnalysisStartup, WorkflowModels: []string{simmodel.ModelTransientCircuitV1}, Deterministic: true},
	{ID: "kicadai_thermal_mna_v1", Version: 1, Analysis: simmodel.AnalysisThermal, WorkflowModels: []string{simmodel.ModelLinearCircuitMNAV1, simmodel.ModelNonlinearCircuitDCV1, simmodel.ModelTransientCircuitV1}, Deterministic: true},
	{ID: "kicadai_transient_mna_v1", Version: 1, Analysis: simmodel.AnalysisTransient, WorkflowModels: []string{simmodel.ModelTransientCircuitV1}, Deterministic: true},
}

var (
	builtinSolversOnce sync.Once
	builtinSolvers     []SolverDefinition
)

func BuiltinSolvers() []SolverDefinition {
	builtinSolversOnce.Do(func() {
		builtinSolvers = make([]SolverDefinition, len(solverRegistry))
		for index, solver := range solverRegistry {
			builtinSolvers[index] = solver
			builtinSolvers[index].WorkflowModels = append([]string(nil), solver.WorkflowModels...)
			builtinSolvers[index].SHA256 = ""
			if digest, err := hashJSON(builtinSolvers[index]); err == nil {
				builtinSolvers[index].SHA256 = digest
			}
		}
	})
	result := make([]SolverDefinition, len(builtinSolvers))
	for index, solver := range builtinSolvers {
		result[index] = solver
		result[index].WorkflowModels = append([]string(nil), solver.WorkflowModels...)
	}
	return result
}

func EnabledBuiltinSolverIDs() []string {
	result := make([]string, 0, len(solverRegistry))
	for _, solver := range BuiltinSolvers() {
		result = append(result, solver.ID)
	}
	slices.Sort(result)
	return result
}

func solverForAnalysis(analysis string) (SolverDefinition, bool) {
	for _, solver := range BuiltinSolvers() {
		if solver.Analysis == strings.TrimSpace(analysis) {
			return solver, true
		}
	}
	return SolverDefinition{}, false
}

func canonicalAnalysis(authored string) (string, bool) {
	switch strings.TrimSpace(authored) {
	case "dc_sweep":
		return simmodel.AnalysisDCOperatingPoint, true
	case simmodel.AnalysisDCOperatingPoint, simmodel.AnalysisACSweep, simmodel.AnalysisTransient,
		simmodel.AnalysisNoise, simmodel.AnalysisStability, simmodel.AnalysisStartup,
		simmodel.AnalysisDistortion, simmodel.AnalysisThermal, simmodel.AnalysisElectrothermal:
		return strings.TrimSpace(authored), true
	default:
		return "", false
	}
}

func validateSolverRegistry() []Diagnostic {
	diagnostics := []Diagnostic{}
	seenIDs := map[string]bool{}
	seenAnalyses := map[string]bool{}
	previousID := ""
	for index, solver := range BuiltinSolvers() {
		path := fmt.Sprintf("solver_registry[%d]", index)
		if solver.ID == "" || solver.ID <= previousID || seenIDs[solver.ID] {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeSolverUnavailable, Path: path + ".id", Message: "solver identities must be unique and canonically ordered"})
		}
		previousID = solver.ID
		seenIDs[solver.ID] = true
		if seenAnalyses[solver.Analysis] {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeMissingAnalysisDefinition, Path: path + ".analysis", Analysis: solver.Analysis, Message: "analysis has more than one implicit solver; explicit selection is forbidden"})
		}
		seenAnalyses[solver.Analysis] = true
		if !solver.Deterministic || !validSHA256(solver.SHA256) || len(solver.WorkflowModels) == 0 || !slices.IsSorted(solver.WorkflowModels) {
			diagnostics = append(diagnostics, Diagnostic{Code: CodeSolverUnavailable, Path: path, Analysis: solver.Analysis, Message: "solver definition is incomplete, nondeterministic, or non-canonical"})
		}
		for _, modelID := range solver.WorkflowModels {
			if !simmodel.SupportsAnalysis(modelID, solver.Analysis) {
				diagnostics = append(diagnostics, Diagnostic{Code: CodeSolverModelIncompatible, Path: path + ".workflow_models." + modelID, Analysis: solver.Analysis, Message: "solver workflow model has no implemented evaluator for the analysis"})
			}
		}
	}
	return diagnostics
}
