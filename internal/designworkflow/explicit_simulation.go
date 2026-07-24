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
	issues := make([]reports.Issue, 0, 1)
	var simulationValue any
	artifactDescription := "Catalog-backed trusted simulation report"
	if request.ExplicitCircuit.ClosedLoop == nil {
		report, diagnostics := simmodel.Evaluate(*request.ExplicitCircuit.Simulation)
		for _, diagnostic := range diagnostics {
			issues = append(issues, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
				Path: "simulation." + diagnostic.Path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion,
			})
		}
		simulationValue = report
	} else {
		report := *request.ExplicitCircuit.ClosedLoop
		for _, diagnostic := range closedloopsynthesis.ValidatePromotionReport(report, request.ExplicitCircuit.CatalogHash) {
			issues = append(issues, closedLoopSimulationIssue(diagnostic))
		}
		if report.SelectedCircuitHash != request.ExplicitCircuit.ResolutionHash {
			issues = append(issues, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
				Path: "simulation.closed_loop.selected_circuit_hash", Message: "selected closed-loop evidence is not bound to the resolved physical circuit",
			})
		}
		evidence, ok := closedloopsynthesis.SelectedSimulationEvidence(report)
		if !ok {
			issues = append(issues, reports.Issue{
				Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
				Path: "simulation.closed_loop.selected", Message: "selected closed-loop result lacks a unique trusted simulation transcript",
			})
		} else {
			for _, diagnostic := range closedloopsynthesis.ReplaySimulationEvidence(*evidence) {
				issues = append(issues, closedLoopSimulationIssue(diagnostic))
			}
			simulationValue = evidence
			artifactDescription = "Catalog-backed selected closed-loop simulation transcript"
		}
	}
	var data []byte
	var err error
	if len(issues) == 0 {
		data, err = json.MarshalIndent(simulationValue, "", "  ")
	}
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
		stage.Artifacts = []reports.Artifact{{Kind: reports.ArtifactSimulationReport, Path: ExplicitSimulationArtifactPath, Description: artifactDescription}}
		if request.ExplicitCircuit.ClosedLoop != nil {
			stage.Artifacts = append(stage.Artifacts, reports.Artifact{Kind: reports.ArtifactSimulationReport, Path: ExplicitClosedLoopArtifactPath, Description: "Deterministic closed-loop synthesis report"})
		}
	}
	return stage
}

func simulationArtifactIssue(err error) reports.Issue {
	return reports.Issue{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Path: "simulation", Message: err.Error(), Suggestion: "verify the output directory and regenerate the complete project"}
}

func closedLoopSimulationIssue(diagnostic closedloopsynthesis.Diagnostic) reports.Issue {
	return reports.Issue{
		Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked,
		Path: "simulation.closed_loop." + diagnostic.Path, Message: diagnostic.Message, Suggestion: diagnostic.Suggestion,
	}
}
