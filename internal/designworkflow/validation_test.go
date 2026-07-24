package designworkflow

import (
	"context"
	"path/filepath"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/kicadfiles/checks"
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

func TestBoardValidationOptionsPassesKiCadCLI(t *testing.T) {
	request := Request{}
	opts := ValidationOptions{KiCadCLI: filepath.Join(t.TempDir(), "kicad-cli")}

	optional := boardValidationOptions(&request, opts)
	if optional.KiCadCLI != opts.KiCadCLI || optional.RequireDRC {
		t.Fatalf("optional board validation opts = %#v, want KiCad CLI and no required DRC", optional)
	}

	opts.RequireDRC = true
	required := boardValidationOptions(&request, opts)
	if required.KiCadCLI != opts.KiCadCLI || !required.RequireDRC {
		t.Fatalf("required board validation opts = %#v, want KiCad CLI and required DRC", required)
	}
}

func TestReconcileDeferredZoneFillValidationRequiresCleanDRC(t *testing.T) {
	stage := NewStageResult(StageValidation, []reports.Issue{{
		Code:       reports.CodeValidationFailed,
		Severity:   reports.SeverityWarning,
		Path:       "zones.0.filled_polygons",
		Message:    deferredZoneFillMessage,
		Suggestion: "repair category: zone",
	}})

	unresolved := reconcileDeferredZoneFillValidation(stage, checks.CheckResult{
		Kind:   checks.CheckKindDRC,
		Status: checks.CheckStatusFail,
	})
	if unresolved.Status != StageStatusWarning || unresolved.Issues[0].Code != reports.CodeValidationFailed {
		t.Fatalf("failed DRC resolved deferred zone warning: %#v", unresolved)
	}
	wrongCheck := reconcileDeferredZoneFillValidation(stage, checks.CheckResult{
		Kind:   checks.CheckKindERC,
		Status: checks.CheckStatusPass,
	})
	if wrongCheck.Status != StageStatusWarning || wrongCheck.Issues[0].Code != reports.CodeValidationFailed {
		t.Fatalf("non-DRC evidence resolved deferred zone warning: %#v", wrongCheck)
	}

	resolved := reconcileDeferredZoneFillValidation(stage, checks.CheckResult{
		Kind:   checks.CheckKindDRC,
		Status: checks.CheckStatusPass,
	})
	if resolved.Status != StageStatusOK {
		t.Fatalf("resolved stage status = %q, want ok: %#v", resolved.Status, resolved)
	}
	if len(resolved.Issues) != 1 || resolved.Issues[0].Code != reports.CodeValidationTrace || resolved.Issues[0].Severity != reports.SeverityInfo {
		t.Fatalf("resolved issue = %#v, want informational validation trace", resolved.Issues)
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
