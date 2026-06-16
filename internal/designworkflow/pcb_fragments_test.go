package designworkflow

import (
	"context"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/reports"
)

func TestRealizePCBFragmentsCreatesLEDFragment(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	result := RealizePCBFragments(context.Background(), registry, plan)
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("fragment issues = %#v", result.Stage.Issues)
	}
	if len(result.Fragments) != 1 || len(result.Fragments[0].Realization.Components) != 2 || len(result.Fragments[0].Realization.LocalRoutes) != 1 {
		t.Fatalf("fragments = %#v", result.Fragments)
	}
	if result.Fragments[0].OriginXMM != defaultFragmentMarginMM {
		t.Fatalf("origin = %#v", result.Fragments[0])
	}
}

func TestRealizePCBFragmentsWarnsWhenBoardTooSmall(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "tiny",
		Board:   BoardSpec{WidthMM: 4, HeightMM: 4, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	result := RealizePCBFragments(context.Background(), registry, plan)
	if result.Stage.Status != StageStatusWarning {
		t.Fatalf("stage = %#v", result.Stage)
	}
	assertIssueCode(t, result.Stage.Issues, reports.CodePlacementOutsideBoard)
}

func TestRealizePCBFragmentsSkipsAfterPlanFailure(t *testing.T) {
	result := RealizePCBFragments(context.Background(), blocks.NewBuiltinRegistry(), BlockPlanResult{
		Stage: NewStageResult(StageBlockPlanning, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "bad"}}),
	})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestRealizePCBFragmentsRequiresContext(t *testing.T) {
	result := RealizePCBFragments(nil, blocks.NewBuiltinRegistry(), BlockPlanResult{})
	if !reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("issues = %#v, want context issue", result.Stage.Issues)
	}
}
