package designworkflow

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

func TestRoutePlacementAuditShowsNamedLocalRouteCanStillMissPhysicalPads(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request := Request{
		Version: RequestVersion,
		Name:    "status_board",
		Board:   BoardSpec{WidthMM: 40, HeightMM: 25, Layers: 2},
		Blocks:  []BlockInstanceSpec{{ID: "status", BlockID: "led_indicator"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(ctx, registry, request)
	if plan.Stage.Status == StageStatusBlocked {
		t.Fatalf("planning failed: %#v", plan.Stage.Issues)
	}
	fragments := RealizePCBFragments(ctx, registry, plan)
	if fragments.Stage.Status == StageStatusBlocked {
		t.Fatalf("PCB realization failed: %#v", fragments.Stage.Issues)
	}
	placed := PlaceFragments(ctx, request, fragments, PlacementOptions{})
	if placed.Stage.Status == StageStatusBlocked {
		t.Fatalf("placement failed: %#v", placed.Stage.Issues)
	}
	routed := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	netAssignment := SummarizeGeneratedNetAssignment(&placed, &routed)

	if netAssignment.AssignedCopperObjects == 0 {
		t.Fatalf("net assignment = %#v, want assigned local-route copper", netAssignment)
	}
	if routed.Stage.Status != StageStatusBlocked {
		t.Fatalf("routing stage = %#v, want blocked by physical route endpoint miss", routed.Stage)
	}
	assertIssueCode(t, routed.Stage.Issues, reports.CodeDisconnectedPad)
	if countTransactionOps(routed.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want named local route operation", routed.Operations)
	}
}

func TestLocalRouteOperationsBindToPlacedPadEndpoints(t *testing.T) {
	extraRoute := mustGeneratedNetAssignmentRouteOperation(t, "EXTRA")
	fragments := PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "status",
		BlockID:    "led_indicator",
		Realization: blocks.BlockPCBRealizationResult{
			LocalRoutes: []blocks.RealizedPCBLocalRoute{{
				ID:      "series",
				NetName: "SIG",
				From:    transactions.Endpoint{Ref: "R1", Pin: "2"},
				To:      transactions.Endpoint{Ref: "D1", Pin: "1"},
				Layer:   "F.Cu",
				WidthMM: 0.25,
			}},
			Operations: []transactions.Operation{extraRoute},
		},
	}}}
	placed := PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{
				{Ref: "R1", FootprintID: "Test:R", Pads: []placement.PadSummary{{Name: "2", Net: "SIG", XMM: 1, YMM: 0}}},
				{Ref: "D1", FootprintID: "Test:D", Pads: []placement.PadSummary{{Name: "1", Net: "SIG", XMM: -1, YMM: 0}}},
			},
			Nets: []placement.Net{{Name: "SIG", Endpoints: []placement.Endpoint{{Ref: "R1", Pin: "2"}, {Ref: "D1", Pin: "1"}}}},
		},
		Result: placement.Result{Status: placement.StatusPlaced, Placements: []placement.PlacementResult{
			{Ref: "R1", FootprintID: "Test:R", Position: placement.Placement{XMM: 10, YMM: 5, Layer: "F.Cu"}},
			{Ref: "D1", FootprintID: "Test:D", Position: placement.Placement{XMM: 20, YMM: 5, Layer: "F.Cu"}},
		}},
		Stage: NewStageResult(StagePlacement, nil),
	}

	operations, issues := localRouteOperations(fragments, &placed)
	if len(issues) != 0 {
		t.Fatalf("local route binding issues = %#v", issues)
	}
	if len(operations) != 2 {
		t.Fatalf("operations = %#v, want preserved extra route and one bound route", operations)
	}
	var route transactions.RouteOperation
	if err := json.Unmarshal(operations[1].Raw, &route); err != nil {
		t.Fatal(err)
	}
	if len(route.Points) != 2 ||
		route.Points[0].XMM != 11 || route.Points[0].YMM != 5 ||
		route.Points[1].XMM != 19 || route.Points[1].YMM != 5 {
		t.Fatalf("route points = %#v, want physical pad centers", route.Points)
	}
}

func TestRoutePlacementAddsAnchorBindingRoutes(t *testing.T) {
	request := Request{Version: RequestVersion, Name: "anchor", Board: BoardSpec{WidthMM: 30, HeightMM: 20, Layers: 1}}
	fragments := testAnchorFragments("esd_protection", blocks.RealizedPCBEntryAnchor{
		ID: "signal_entry", Port: "SIGNAL", NetName: "SIG", Placement: blocks.RelativePlacement{XMM: 7, YMM: 10, Layer: "F.Cu"},
	})
	placed := PlacementStageResult{
		Request: placement.Request{
			Board: placement.BoardPlacementArea{WidthMM: 30, HeightMM: 20},
			Rules: placement.DefaultRules(),
			Components: []placement.Component{{
				Ref:         "J1",
				Role:        "connector",
				FootprintID: "Test:Pad",
				Pads:        []placement.PadSummary{{Name: "1", Net: "SIG", XMM: 0, YMM: 0, WidthMM: 1, HeightMM: 1}},
			}},
		},
		Result: placement.Result{
			Status: placement.StatusPlaced,
			Placements: []placement.PlacementResult{{
				Ref: "J1", FootprintID: "Test:Pad", Position: placement.Placement{XMM: 5, YMM: 10, Layer: "F.Cu"},
			}},
			Metrics: placement.Metrics{PlacedCount: 1},
		},
		Stage: NewStageResult(StagePlacement, nil),
	}

	result := RoutePlacement(context.Background(), request, fragments, placed, RoutingOptions{Mode: routing.ModeSingleLayer, TraceWidthMM: 0.3})

	value, ok := result.Stage.Summary["anchor_bindings"]
	if !ok {
		t.Fatalf("anchor binding summary missing: %#v", result.Stage.Summary)
	}
	summary, ok := value.(AnchorBindingSummary)
	if !ok || summary.Bound != 1 || summary.Routed != 1 {
		t.Fatalf("anchor binding summary = %#v", value)
	}
	var found bool
	for _, operation := range result.Operations {
		if operation.Op != transactions.OpRoute {
			continue
		}
		var payload transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &payload); err != nil {
			t.Fatalf("route unmarshal = %v", err)
		}
		if payload.NetName == "SIG" && len(payload.Points) == 2 && payload.Points[0].XMM == 5 && payload.Points[1].XMM == 7 {
			found = true
		}
	}
	if !found {
		t.Fatalf("operations = %#v, want anchor binding route", result.Operations)
	}
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
