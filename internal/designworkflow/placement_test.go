package designworkflow

import (
	"context"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
)

func TestPlaceFragmentsPlacesRealizedLED(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("placement issues = %#v", result.Stage.Issues)
	}
	if result.Result.Metrics.PlacedCount != 2 || len(result.Result.Operations) != 2 {
		t.Fatalf("placement result = %#v", result.Result)
	}
	if !result.Request.Components[0].Fixed {
		t.Fatalf("expected fixed realized placement: %#v", result.Request.Components[0])
	}
}

func TestPlaceFragmentsSkipsAfterRealizationFailure(t *testing.T) {
	result := PlaceFragments(context.Background(), validRequest(), PCBFragmentResult{
		Stage: NewStageResult(StagePCBRealization, []reports.Issue{{Code: reports.CodeValidationFailed, Severity: reports.SeverityError, Message: "bad"}}),
	}, PlacementOptions{})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
}

func TestPlaceFragmentsReportsTinyBoardCollision(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "tiny",
		Board:   BoardSpec{WidthMM: 4, HeightMM: 4, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{DefaultBounds: placement.Bounds{WidthMM: 2, HeightMM: 1, Source: placement.BoundsEstimated}})
	if result.Stage.Status == StageStatusOK {
		t.Fatalf("expected placement warning/block for tiny board: %#v", result.Stage)
	}
}

func TestNetRoleFromNameUsesTokens(t *testing.T) {
	if got := netRoleFromName("saving_mode"); got != placement.NetSignal {
		t.Fatalf("saving_mode role = %q", got)
	}
	if got := netRoleFromName("main_vbus"); got != placement.NetPower {
		t.Fatalf("main_vbus role = %q", got)
	}
}

func TestPlaceFragmentsMergesRulesWithoutDroppingCustomValues(t *testing.T) {
	request := Request{
		Version:     RequestVersion,
		Name:        "status_board",
		Board:       BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:      []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Constraints: ConstraintSpec{AllowBackLayer: false, PreferTopLayer: false},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{Rules: placement.Rules{ComponentSpacingMM: 3, AllowBackLayer: true, PreferTopLayer: true}})
	if result.Request.Rules.ComponentSpacingMM != 3 {
		t.Fatalf("component spacing = %v", result.Request.Rules.ComponentSpacingMM)
	}
	if result.Request.Rules.AllowBackLayer || result.Request.Rules.PreferTopLayer {
		t.Fatalf("request constraints did not override layer preferences: %#v", result.Request.Rules)
	}
}
