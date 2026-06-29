package designworkflow

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

func TestValidateProjectRunsEvaluationAndBoardValidation(t *testing.T) {
	request, write := writeValidationFixture(t)

	result := ValidateProject(context.Background(), &request, &write, ValidationOptions{})
	if result.Stage.Summary["evaluation_checks"].(int) == 0 {
		t.Fatalf("evaluation did not run: %#v", result.Stage)
	}
	if result.Stage.Summary["board_validation_checks"].(int) == 0 {
		t.Fatalf("board validation did not run: %#v", result.Stage)
	}
}

func TestValidateProjectSkipsAfterWriteFailure(t *testing.T) {
	result := ValidateProject(context.Background(), &Request{}, &ProjectWriteResult{
		Stage: NewStageResult(StageProjectWrite, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityBlocked, Message: "bad"}}),
	}, ValidationOptions{})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestValidateProjectStrictUnroutedAcceptsRoutedFixture(t *testing.T) {
	request, write := writeValidationFixture(t)
	request.Validation.StrictUnrouted = true

	result := ValidateProject(context.Background(), &request, &write, ValidationOptions{})
	if result.Stage.Status == StageStatusBlocked {
		t.Fatalf("stage = %#v, want non-blocking strict validation for routed fixture", result.Stage)
	}
}

func writeValidationFixture(t *testing.T) (Request, ProjectWriteResult) {
	t.Helper()
	request := Request{
		Version:    RequestVersion,
		Name:       "status_board",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{Acceptance: AcceptanceConnectivity},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	placed := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	routed := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{Skip: true})
	output := filepath.Join(t.TempDir(), "status_board")
	write := WriteProject(context.Background(), &request, &plan, &placed, &routed, ProjectWriteOptions{OutputDir: output})
	if reports.HasBlockingIssue(write.Stage.Issues) {
		t.Fatalf("write issues = %#v", write.Stage.Issues)
	}
	return request, write
}
