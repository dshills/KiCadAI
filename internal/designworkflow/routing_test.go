package designworkflow

import (
	"context"
	"testing"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestRoutePlacementUsesGeneratedPadSummariesForLocalRoutes(t *testing.T) {
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)
	placed := PlaceFragments(context.Background(), request, fragments, PlacementOptions{})

	result := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{})
	if countTransactionOps(result.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want local route operation", result.Operations)
	}
	if result.Stage.Status != StageStatusBlocked {
		t.Fatalf("stage = %#v, want blocked on real generated route connectivity", result.Stage)
	}
	localRoutes, ok := result.Stage.Summary["local_route_mobility"].(LocalRouteMobilitySummary)
	if !ok || localRoutes.Total == 0 || localRoutes.Preserved == 0 {
		t.Fatalf("local route mobility summary = %#v", result.Stage.Summary["local_route_mobility"])
	}
	assertIssueCode(t, result.Stage.Issues, reports.CodeDisconnectedPad)
}

func TestRoutePlacementRoutesSimpleSignalWithPads(t *testing.T) {
	placed := simplePlacedPads()
	request := Request{Version: RequestVersion, Name: "simple", Board: BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 1}}

	result := RoutePlacement(context.Background(), request, PCBFragmentResult{}, placed, RoutingOptions{Mode: routing.ModeSingleLayer})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("routing issues = %#v", result.Stage.Issues)
	}
	if result.Result.Status != routing.StatusRouted || len(result.Result.Operations) == 0 {
		t.Fatalf("routing result = %#v", result.Result)
	}
	if countTransactionOps(result.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want transaction route operation", result.Operations)
	}
	if result.Stage.Summary["quality_score"] == nil ||
		result.Stage.Summary["route_reports"] == nil ||
		result.Stage.Summary["repair_diagnostics"] == nil {
		t.Fatalf("routing summary missing quality evidence: %#v", result.Stage.Summary)
	}
}

func TestRoutePlacementSingleLayerUsesPlacedLayer(t *testing.T) {
	placed := simplePlacedPads()
	placed.Result.Placements[0].Position.Layer = "B.Cu"
	placed.Result.Placements[1].Position.Layer = "B.Cu"
	request := Request{Version: RequestVersion, Name: "bottom", Board: BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 1}}

	result := RoutePlacement(context.Background(), request, PCBFragmentResult{}, placed, RoutingOptions{})
	if reports.HasBlockingIssue(result.Stage.Issues) {
		t.Fatalf("routing issues = %#v", result.Stage.Issues)
	}
	if len(result.Request.Board.Layers) != 1 || result.Request.Board.Layers[0].Name != "B.Cu" {
		t.Fatalf("routing board layers = %#v", result.Request.Board.Layers)
	}
	if result.Request.Rules.PreferLayer != "B.Cu" {
		t.Fatalf("prefer layer = %q", result.Request.Rules.PreferLayer)
	}
}

func TestRoutePlacementReportsUnroutableSignal(t *testing.T) {
	placed := simplePlacedPads()
	placed.Request.Keepouts = []placement.Keepout{{
		ID: "wall",
		Bounds: placement.Rect{
			Min: placement.Point{XMM: 0, YMM: 0},
			Max: placement.Point{XMM: 30, YMM: 20},
		},
		Layers: []string{"F.Cu"},
	}}
	request := Request{
		Version:    RequestVersion,
		Name:       "blocked",
		Board:      BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 1},
		Validation: ValidationSpec{StrictUnrouted: true},
	}

	result := RoutePlacement(context.Background(), request, PCBFragmentResult{}, placed, RoutingOptions{Mode: routing.ModeSingleLayer})
	if result.Stage.Status != StageStatusBlocked {
		t.Fatalf("stage = %#v, want blocked route", result.Stage)
	}
	if len(result.Stage.Issues) == 0 {
		t.Fatalf("expected routing issue")
	}
}

func TestRoutePlacementSkipsWhenRequested(t *testing.T) {
	request := Request{
		Version:    RequestVersion,
		Name:       "status_board",
		Board:      BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:     []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
		Validation: ValidationSpec{SkipRouting: true},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(context.Background(), registry, request)
	fragments := RealizePCBFragments(context.Background(), registry, plan)

	result := RoutePlacement(context.Background(), request, fragments, PlacementStageResult{}, RoutingOptions{})
	if result.Stage.Status != StageStatusSkipped {
		t.Fatalf("stage = %#v", result.Stage)
	}
	if countTransactionOps(result.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want local route operation", result.Operations)
	}
}

func simplePlacedPads() PlacementStageResult {
	return PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 30, HeightMM: 20},
			Rules: placement.DefaultRules(),
			Components: []placement.Component{
				{
					Ref:         "U1",
					FootprintID: "Test:Pad",
					Bounds:      placement.Bounds{WidthMM: 2, HeightMM: 2, Source: placement.BoundsExplicit},
					Pads:        []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1}},
				},
				{
					Ref:         "U2",
					FootprintID: "Test:Pad",
					Bounds:      placement.Bounds{WidthMM: 2, HeightMM: 2, Source: placement.BoundsExplicit},
					Pads:        []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1}},
				},
			},
			Nets: []placement.Net{{
				Name:      "SIG",
				Role:      placement.NetSignal,
				Endpoints: []placement.Endpoint{{Ref: "U1", Pin: "1"}, {Ref: "U2", Pin: "1"}},
			}},
		},
		Result: placement.Result{
			Status: placement.StatusPlaced,
			Placements: []placement.PlacementResult{
				{Ref: "U1", FootprintID: "Test:Pad", Position: placement.Placement{XMM: 5, YMM: 10, Layer: "F.Cu"}},
				{Ref: "U2", FootprintID: "Test:Pad", Position: placement.Placement{XMM: 20, YMM: 10, Layer: "F.Cu"}},
			},
			Metrics: placement.Metrics{PlacedCount: 2},
		},
		Stage: NewStageResult(StagePlacement, nil),
	}
}

func countTransactionOps(operations []transactions.Operation, kind transactions.OperationKind) int {
	count := 0
	for _, operation := range operations {
		if operation.Op == kind {
			count++
		}
	}
	return count
}
