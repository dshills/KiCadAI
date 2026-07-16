package designworkflow

import (
	"encoding/json"
	"os"
	"path/filepath"

	"kicadai/internal/reports"
)

const explicitSimulationArtifactPath = ".kicadai/simulation.json"

type explicitSimulationModel struct {
	minHeadroomV     float64
	maxLoadCurrentMA float64
}

var explicitSimulationModels = map[string]explicitSimulationModel{
	"linear_regulator_ideal_v1": {minHeadroomV: 0.2, maxLoadCurrentMA: 600},
}

func runExplicitSimulation(request Request, outputDir string, overwrite bool) StageResult {
	if request.ExplicitCircuit == nil || request.ExplicitCircuit.Simulation == nil {
		return NewStageResult(StageSimulation, nil)
	}
	s := request.ExplicitCircuit.Simulation
	issues := []reports.Issue{}
	output := s.OutputNominalV
	model, trusted := explicitSimulationModels[s.ModelID]
	if !trusted || s.InputVoltageV < s.OutputNominalV+model.minHeadroomV || s.LoadCurrentMA > model.maxLoadCurrentMA || output < s.OutputMinV || output > s.OutputMaxV {
		issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: "trusted regulator simulation constraints were not satisfied"})
	}
	report := map[string]any{"model_id": s.ModelID, "component": s.Component, "input_voltage_v": s.InputVoltageV, "load_current_ma": s.LoadCurrentMA, "output_voltage_v": output, "output_min_v": s.OutputMinV, "output_max_v": s.OutputMaxV}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: err.Error()})
	} else if err := os.MkdirAll(filepath.Join(outputDir, ".kicadai"), 0o755); err != nil {
		issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: err.Error()})
	} else if !overwrite {
		if _, err := os.Stat(filepath.Join(outputDir, explicitSimulationArtifactPath)); err == nil {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: "simulation artifact already exists"})
		}
	}
	if len(issues) == 0 {
		if err := os.WriteFile(filepath.Join(outputDir, explicitSimulationArtifactPath), append(data, '\n'), 0o644); err != nil {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: err.Error()})
		}
	}
	stage := NewStageResult(StageSimulation, issues)
	if len(issues) == 0 {
		stage.Artifacts = []reports.Artifact{{Kind: reports.ArtifactSimulationReport, Path: explicitSimulationArtifactPath, Description: "Trusted generic regulator simulation"}}
	}
	return stage
}
