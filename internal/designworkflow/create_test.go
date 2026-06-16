package designworkflow

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/reports"
)

func TestCreateWritesWorkflowResult(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "status_board",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceStructural, SkipRouting: true},
	}
	output := filepath.Join(t.TempDir(), "status_board")

	result := Create(context.Background(), request, CreateOptions{OutputDir: output})
	if result.Project.OutputDir != output {
		t.Fatalf("project = %#v", result.Project)
	}
	if len(result.Stages) == 0 {
		t.Fatalf("stages missing")
	}
	if result.Acceptance.Achieved == "" {
		t.Fatalf("acceptance = %#v feedback = %#v", result.Acceptance, result.Feedback)
	}
}

func TestWorkflowIssueAndArtifactCollectors(t *testing.T) {
	result := WorkflowResult{Stages: []StageResult{{
		Issues:    []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityWarning, Message: "warn"}},
		Artifacts: []reports.Artifact{{Kind: reports.ArtifactKiCadProject, Path: "out/demo.kicad_pro"}},
	}}}
	if len(WorkflowIssues(result)) != 1 || len(WorkflowArtifacts(result)) != 1 {
		t.Fatalf("collectors failed")
	}
}

func TestCreateShortCircuitsAfterPlanFailure(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "bad",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "missing", BlockID: "does_not_exist"}},
	}
	result := Create(context.Background(), request, CreateOptions{OutputDir: filepath.Join(t.TempDir(), "bad")})
	if result.Stages[0].Status != StageStatusBlocked {
		t.Fatalf("plan stage = %#v", result.Stages[0])
	}
	for _, stage := range result.Stages[2:] {
		if stage.Status != StageStatusSkipped {
			t.Fatalf("stage %s = %#v, want skipped", stage.Name, stage)
		}
	}
}
