package capabilityfeedback

import (
	"testing"

	"kicadai/internal/simulationadmission"
)

func TestSimulationAdmissionDiagnosisGapsRemainTyped(t *testing.T) {
	tests := []struct {
		code       simulationadmission.DiagnosticCode
		scope      GapScope
		capability string
	}{
		{simulationadmission.CodeMissingModel, ScopeModel, "ac_sweep_model"},
		{simulationadmission.CodeIncompatibleModel, ScopeModel, "ac_sweep_model"},
		{simulationadmission.CodeInvalidModelParameters, ScopeModel, "ac_sweep_model"},
		{simulationadmission.CodeMissingAnalysisDefinition, ScopeSimulation, "ac_sweep_definition"},
		{simulationadmission.CodeUnsupportedAnalysis, ScopeSimulation, "ac_sweep_solver"},
		{simulationadmission.CodeSolverUnavailable, ScopeSimulation, "ac_sweep_solver"},
		{simulationadmission.CodeSolverModelIncompatible, ScopeModel, "ac_sweep_model_solver_compatibility"},
	}
	for _, test := range tests {
		t.Run(string(test.code), func(t *testing.T) {
			gap, found := simulationAdmissionGapV20(Gap{Code: string(test.code)}, "ac_sweep")
			if !found {
				t.Fatalf("admission code was not recognized")
			}
			if gap.Code != string(test.code) || gap.Stage != "simulation_admission" ||
				gap.Scope != test.scope || gap.Capability != test.capability {
				t.Fatalf("unexpected gap: %#v", gap)
			}
		})
	}
}

func TestSimulationAdmissionReportGapRemainsTyped(t *testing.T) {
	gap, found := simulationAdmissionGapV20(Gap{
		Code:           string(simulationadmission.CodeSolverUnavailable),
		EvidenceHashes: []string{"a1"},
	}, "")
	if !found {
		t.Fatalf("admission code was not recognized")
	}
	if gap.Stage != "simulation_admission" || gap.Scope != ScopeSimulation ||
		gap.Capability != "trusted_simulation_solver" || len(gap.EvidenceHashes) != 1 {
		t.Fatalf("unexpected report gap: %#v", gap)
	}
}
