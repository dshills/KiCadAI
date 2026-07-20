package designworkflow

import (
	"encoding/json"
	"os"
	"path/filepath"

	"kicadai/internal/closedloopsynthesis"
	"kicadai/internal/reports"
	"kicadai/internal/simmodel"
)

const ExplicitSimulationArtifactPath = ".kicadai/simulation.json"
const ExplicitClosedLoopArtifactPath = ".kicadai/closed-loop-synthesis.json"

func runExplicitSimulation(request Request, outputDir string, overwrite bool) StageResult {
	if request.ExplicitCircuit == nil || request.ExplicitCircuit.Simulation == nil && request.ExplicitCircuit.ClosedLoop == nil {
		return NewStageResult(StageSimulation, nil)
	}
	if request.ExplicitCircuit.Simulation == nil {
		return NewStageResult(StageSimulation, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: "closed-loop evidence requires its final trusted simulation plan"}})
	}
	report, diagnostics := simmodel.Evaluate(*request.ExplicitCircuit.Simulation)
	issues := make([]reports.Issue, 0, len(diagnostics)+1)
	for _, diagnostic := range diagnostics {
		issues = append(issues, reports.Issue{
			Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
			Path: "simulation." + diagnostic.Path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion,
		})
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		issues = append(issues, simulationArtifactIssue(err))
	} else if err := os.MkdirAll(filepath.Join(outputDir, ".kicadai"), 0o755); err != nil {
		issues = append(issues, simulationArtifactIssue(err))
	} else if !overwrite {
		if _, err := os.Stat(filepath.Join(outputDir, ExplicitSimulationArtifactPath)); err == nil {
			issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: "simulation artifact already exists", Suggestion: "use overwrite only when replacing the complete generated project"})
		}
	}
	if len(issues) == 0 {
		if err := os.WriteFile(filepath.Join(outputDir, ExplicitSimulationArtifactPath), append(data, '\n'), 0o644); err != nil {
			issues = append(issues, simulationArtifactIssue(err))
		}
	}
	closedLoopData := []byte(nil)
	if len(issues) == 0 && request.ExplicitCircuit.ClosedLoop != nil {
		var err error
		closedLoopData, err = closedloopsynthesis.MarshalReport(*request.ExplicitCircuit.ClosedLoop)
		if err != nil {
			issues = append(issues, simulationArtifactIssue(err))
		} else if !overwrite {
			if _, err := os.Stat(filepath.Join(outputDir, ExplicitClosedLoopArtifactPath)); err == nil {
				issues = append(issues, reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation.closed_loop", Message: "closed-loop synthesis artifact already exists", Suggestion: "use overwrite only when replacing the complete generated project"})
			}
		}
	}
	if len(issues) == 0 && len(closedLoopData) != 0 {
		if err := os.WriteFile(filepath.Join(outputDir, ExplicitClosedLoopArtifactPath), append(closedLoopData, '\n'), 0o644); err != nil {
			issues = append(issues, simulationArtifactIssue(err))
		}
	}
	stage := NewStageResult(StageSimulation, issues)
	if len(issues) == 0 {
		stage.Artifacts = []reports.Artifact{{Kind: reports.ArtifactSimulationReport, Path: ExplicitSimulationArtifactPath, Description: "Catalog-backed trusted simulation report"}}
		if request.ExplicitCircuit.ClosedLoop != nil {
			stage.Artifacts = append(stage.Artifacts, reports.Artifact{Kind: reports.ArtifactSimulationReport, Path: ExplicitClosedLoopArtifactPath, Description: "Deterministic closed-loop synthesis report"})
		}
	}
	return stage
}

func simulationArtifactIssue(err error) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: err.Error(), Suggestion: "verify the output directory and regenerate the complete project"}
}
