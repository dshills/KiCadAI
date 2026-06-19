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
	if len(result.Request.Groups) == 0 {
		t.Fatalf("expected block-derived placement groups")
	}
	if len(result.Request.ProximityRules) == 0 {
		t.Fatalf("expected block-derived proximity rules")
	}
}

func TestPlaceFragmentsDerivesBlockPlacementIntent(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "analog_board",
		Board:   BoardSpec{WidthMM: 100, HeightMM: 60, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "amp", BlockID: "opamp_gain_stage"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	if len(result.Request.Groups) == 0 {
		t.Fatalf("expected placement groups in request")
	}
	group := result.Request.Groups[0]
	if group.ID == "" || len(group.Components) == 0 || !group.KeepTogether || group.MaxSpreadMM <= 0 {
		t.Fatalf("unexpected group: %#v", group)
	}
	if len(result.Request.ProximityRules) == 0 {
		t.Fatalf("expected proximity rules in request")
	}
	if result.Request.ProximityRules[0].Source == "" {
		t.Fatalf("expected proximity source metadata: %#v", result.Request.ProximityRules[0])
	}
}

func TestPlaceFragmentsDerivesConnectorEdgeIntent(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "usb_power",
		Board:   BoardSpec{WidthMM: 100, HeightMM: 60, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "usb", BlockID: "usb_c_power"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})
	for _, component := range result.Request.Components {
		if component.Role == "usb_c_receptacle" {
			if component.Edge != placement.EdgeAny {
				t.Fatalf("usb connector edge = %q, want any edge", component.Edge)
			}
			return
		}
	}
	t.Fatalf("missing usb_c_receptacle in placement request: %#v", result.Request.Components)
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
