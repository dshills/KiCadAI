package designworkflow

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"kicadai/internal/blocks"
	"kicadai/internal/placement"
	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestExistingCopperFromRouteOperationsIncludesLocalRouteSegments(t *testing.T) {
	operation := transactions.NewOperation(transactions.OpRoute, []byte(`{"op":"route","net_name":"VCC_5v","layer":"F.Cu","width_mm":0.5,"points":[{"x_mm":1,"y_mm":2},{"x_mm":6,"y_mm":2},{"x_mm":6,"y_mm":5}],"vias":[{"at":{"x_mm":6,"y_mm":2},"diameter_mm":0.7,"drill_mm":0.35,"layers":["F.Cu","B.Cu"]}]}`))

	existing := existingCopperFromRouteOperations([]transactions.Operation{operation}, "F.Cu", routing.DefaultRules())

	if len(existing) != 4 {
		t.Fatalf("existing copper = %#v, want route segments plus one entry per via layer", existing)
	}
	for _, copper := range existing[:2] {
		if copper.Net != "VCC_5v" || copper.Layer != "F.Cu" || copper.Kind != routing.CopperSegment {
			t.Fatalf("existing copper = %#v, want VCC_5v F.Cu segment", copper)
		}
		if len(copper.Centerline) != 2 {
			t.Fatalf("centerline = %#v, want segment endpoints preserved", copper.Centerline)
		}
		if copper.Geometry.Rect == nil {
			t.Fatalf("geometry = %#v, want width-aware geometry", copper.Geometry)
		}
	}
	for _, copper := range existing[2:] {
		if copper.Net != "VCC_5v" || copper.Kind != routing.CopperVia || (copper.Geometry.Rect == nil && len(copper.Geometry.Polygon) == 0) {
			t.Fatalf("via copper = %#v, want VCC_5v via geometry", copper)
		}
	}
}

func TestExistingCopperFromRouteOperationsSkipsSignalLocalRoutesWithoutVias(t *testing.T) {
	operation := transactions.NewOperation(transactions.OpRoute, []byte(`{"op":"route","net_name":"SDA","layer":"F.Cu","width_mm":0.25,"points":[{"x_mm":1,"y_mm":2},{"x_mm":6,"y_mm":2}]}`))

	if existing := existingCopperFromRouteOperations([]transactions.Operation{operation}, "F.Cu", routing.DefaultRules()); len(existing) != 0 {
		t.Fatalf("existing copper = %#v, want via-free signal local routes excluded from inter-block obstacles", existing)
	}
}

func TestExistingCopperFromAllRouteOperationsIncludesViaFreeSignalRoutes(t *testing.T) {
	operation := transactions.NewOperation(transactions.OpRoute, []byte(`{"op":"route","net_name":"SDA","layer":"F.Cu","width_mm":0.25,"points":[{"x_mm":1,"y_mm":2},{"x_mm":6,"y_mm":5}]}`))

	existing := existingCopperFromAllRouteOperations([]transactions.Operation{operation}, "F.Cu", routing.DefaultRules())
	if len(existing) != 1 || existing[0].Net != "SDA" || existing[0].Kind != routing.CopperSegment {
		t.Fatalf("existing copper = %#v, want via-free SDA segment for selective obstacles", existing)
	}
	if len(existing[0].Centerline) != 2 || len(existing[0].Geometry.Polygon) == 0 {
		t.Fatalf("existing copper = %#v, want diagonal width-aware geometry", existing[0])
	}
}

func TestExistingCopperFromRouteOperationsIncludesUSBCCRoutesWithoutVias(t *testing.T) {
	operation := transactions.NewOperation(transactions.OpRoute, []byte(`{"op":"route","net_name":"usb_power_cc2","layer":"F.Cu","width_mm":0.25,"points":[{"x_mm":1,"y_mm":2},{"x_mm":6,"y_mm":2}]}`))

	existing := existingCopperFromRouteOperations([]transactions.Operation{operation}, "F.Cu", routing.DefaultRules())
	if len(existing) != 1 || existing[0].Net != "usb_power_cc2" || existing[0].Kind != routing.CopperSegment {
		t.Fatalf("existing copper = %#v, want fixed USB CC segment obstacle", existing)
	}
}

func TestExistingUSBConfigurationCopperExcludesOtherLocalRoutes(t *testing.T) {
	operations := []transactions.Operation{
		transactions.NewOperation(transactions.OpRoute, []byte(`{"op":"route","net_name":"usb_power_cc1","layer":"F.Cu","width_mm":0.25,"points":[{"x_mm":1,"y_mm":2},{"x_mm":6,"y_mm":2}]}`)),
		transactions.NewOperation(transactions.OpRoute, []byte(`{"op":"route","net_name":"GND","layer":"F.Cu","width_mm":0.5,"points":[{"x_mm":1,"y_mm":4},{"x_mm":6,"y_mm":4}]}`)),
	}

	existing := existingUSBConfigurationCopperFromRouteOperations(operations, "F.Cu", routing.DefaultRules())
	if len(existing) != 1 || existing[0].Net != "usb_power_cc1" {
		t.Fatalf("existing copper = %#v, want only USB CC copper", existing)
	}
}

func TestExistingCopperFromRouteOperationsIncludesSignalLocalRouteVias(t *testing.T) {
	operation := transactions.NewOperation(transactions.OpRoute, []byte(`{"op":"route","net_name":"SDA","layer":"F.Cu","width_mm":0.25,"points":[{"x_mm":1,"y_mm":2},{"x_mm":6,"y_mm":2}],"vias":[{"at":{"x_mm":6,"y_mm":2},"diameter_mm":0.6,"drill_mm":0.3,"layers":["F.Cu","B.Cu"]}]}`))

	existing := existingCopperFromRouteOperations([]transactions.Operation{operation}, "F.Cu", routing.DefaultRules())
	if len(existing) != 3 {
		t.Fatalf("existing copper = %#v, want signal segment plus one entry per via layer", existing)
	}
	for _, copper := range existing {
		if copper.Net != "SDA" {
			t.Fatalf("existing copper net = %q, want SDA", copper.Net)
		}
	}
}

func TestMultiEndpointGroundRouteObstacleNets(t *testing.T) {
	candidates := []InterBlockRouteCandidate{
		{NetName: "GND", Endpoints: make([]InterBlockRouteEndpoint, 3)},
		{NetName: " VCC_MAIN ", Endpoints: make([]InterBlockRouteEndpoint, 4)},
		{NetName: "GND_AUX", Endpoints: make([]InterBlockRouteEndpoint, 2)},
		{NetName: "SDA", Endpoints: make([]InterBlockRouteEndpoint, 3)},
	}

	nets := map[string]struct{}{}
	addMultiEndpointGroundRouteObstacleNets(nets, candidates)
	if _, ok := nets["GND"]; !ok {
		t.Fatal("multi-endpoint ground net was not selected")
	}
	if _, ok := nets["VCC_MAIN"]; ok {
		t.Fatal("multi-endpoint power net was selected for ground-only obstacle protection")
	}
	if _, ok := nets["GND_AUX"]; ok {
		t.Fatal("two-endpoint ground net was selected")
	}
	if _, ok := nets["SDA"]; ok {
		t.Fatal("multi-endpoint signal net was selected")
	}
}

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
	if result.Stage.Status != StageStatusOK {
		t.Fatalf("stage = %#v, want clean local route connectivity", result.Stage)
	}
	localRoutes, ok := result.Stage.Summary["local_route_mobility"].(LocalRouteMobilitySummary)
	if !ok || localRoutes.Total == 0 || localRoutes.Preserved == 0 {
		t.Fatalf("local route mobility summary = %#v", result.Stage.Summary["local_route_mobility"])
	}
	assertNoIssueCode(t, result.Stage.Issues, reports.CodeDisconnectedPad)
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
	if !stageUsableForRoutingTest(plan.Stage) {
		t.Fatalf("planning failed: %#v", plan.Stage.Issues)
	}
	fragments := RealizePCBFragments(ctx, registry, plan)
	if !stageUsableForRoutingTest(fragments.Stage) {
		t.Fatalf("PCB realization failed: %#v", fragments.Stage.Issues)
	}
	placed := PlaceFragments(ctx, request, fragments, PlacementOptions{})
	if !stageUsableForRoutingTest(placed.Stage) {
		t.Fatalf("placement failed: %#v", placed.Stage.Issues)
	}
	routed := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	netAssignment := SummarizeGeneratedNetAssignment(&placed, &routed)

	if netAssignment.AssignedCopperObjects == 0 {
		t.Fatalf("net assignment = %#v, want assigned local-route copper", netAssignment)
	}
	if routed.Stage.Status != StageStatusOK {
		t.Fatalf("routing stage = %#v, want physical route endpoint proof", routed.Stage)
	}
	assertNoIssueCode(t, routed.Stage.Issues, reports.CodeDisconnectedPad)
	if countTransactionOps(routed.Operations, transactions.OpRoute) == 0 {
		t.Fatalf("operations = %#v, want named local route operation", routed.Operations)
	}
}

func TestPlacedLocalRoutePointsGuardsStaleAuthoredWaypoints(t *testing.T) {
	from := transactions.Point{XMM: 100, YMM: 100}
	to := transactions.Point{XMM: 110, YMM: 100}

	nearby, ok := placedLocalRoutePoints([]transactions.Point{
		{XMM: 0, YMM: 0},
		{XMM: 102, YMM: 94},
		{XMM: 108, YMM: 94},
		{XMM: 20, YMM: 0},
	}, from, to)
	if !ok || len(nearby) != 4 || nearby[1].XMM != 102 || nearby[2].XMM != 108 {
		t.Fatalf("nearby route = %#v ok=%v, want authored bends preserved", nearby, ok)
	}

	farTo := transactions.Point{XMM: 170, YMM: 100}
	stale, ok := placedLocalRoutePoints([]transactions.Point{
		{XMM: 0, YMM: 0},
		{XMM: 2, YMM: 2},
		{XMM: 8, YMM: 2},
		{XMM: 20, YMM: 0},
	}, from, farTo)
	if !ok || len(stale) != 2 || stale[0] != from || stale[1] != farTo {
		t.Fatalf("stale route = %#v ok=%v, want direct endpoint fallback", stale, ok)
	}
}

func TestPlacedLocalRoutePointsTransformsAuthoredShape(t *testing.T) {
	from := transactions.Point{XMM: 20, YMM: 50}
	to := transactions.Point{XMM: 16, YMM: 50}

	routed, ok := placedLocalRoutePoints([]transactions.Point{
		{XMM: 5, YMM: 0},
		{XMM: 5, YMM: 2},
		{XMM: 0, YMM: 2},
		{XMM: 0, YMM: 0},
	}, from, to)
	if !ok || len(routed) != 4 {
		t.Fatalf("route = %#v ok=%v, want transformed dogleg", routed, ok)
	}
	if math.Abs(routed[1].YMM-52) > 1e-9 || math.Abs(routed[2].YMM-52) > 1e-9 {
		t.Fatalf("route = %#v, want dogleg above placed endpoints", routed)
	}
	if routed[0] != from || routed[len(routed)-1] != to {
		t.Fatalf("route endpoints = %#v, want %#v -> %#v", routed, from, to)
	}
}

func TestPlacedLocalRoutePointsTransformsSingleAuthoredWaypoint(t *testing.T) {
	from := transactions.Point{XMM: 20, YMM: 50}
	to := transactions.Point{XMM: 16, YMM: 50}

	routed, ok := placedLocalRoutePoints([]transactions.Point{
		{XMM: 5, YMM: 0},
		{XMM: 2.5, YMM: 2},
		{XMM: 0, YMM: 0},
	}, from, to)
	if !ok || len(routed) != 3 {
		t.Fatalf("route = %#v ok=%v, want transformed one-waypoint shape", routed, ok)
	}
	if math.Abs(routed[1].XMM-18) > 1e-9 || math.Abs(routed[1].YMM-52) > 1e-9 {
		t.Fatalf("route = %#v, want transformed midpoint dogleg", routed)
	}
	if routed[0] != from || routed[len(routed)-1] != to {
		t.Fatalf("route endpoints = %#v, want %#v -> %#v", routed, from, to)
	}
}

func TestTranslatedUnitLocalRoutePointsMovesAuthoredWaypointsWithGroup(t *testing.T) {
	fragment := BlockFragment{
		PlacementGroups: []blocks.PCBPlacementGroup{{ID: "core", ComponentRoles: []string{"source", "sink"}, TranslateAsUnit: true}},
		Realization: blocks.BlockPCBRealizationResult{
			RoleRefs: map[string]string{"source": "C1", "sink": "U1"},
			Components: []blocks.RealizedPCBComponent{
				{Ref: "C1", Placement: blocks.RelativePlacement{XMM: 5, YMM: 10}},
				{Ref: "U1", Placement: blocks.RelativePlacement{XMM: 10, YMM: 10}},
			},
		},
	}
	route := blocks.RealizedPCBLocalRoute{
		From:   transactions.Endpoint{Ref: "C1", Pin: "1"},
		To:     transactions.Endpoint{Ref: "U1", Pin: "1"},
		Points: []transactions.Point{{XMM: 4, YMM: 10}, {XMM: 5, YMM: 8}, {XMM: 10, YMM: 8}, {XMM: 11, YMM: 10}},
	}
	from := PlacedPadEndpoint{Ref: "C1", Point: transactions.Point{XMM: 24, YMM: 40}, ComponentAt: transactions.Point{XMM: 25, YMM: 40}}
	to := PlacedPadEndpoint{Ref: "U1", Point: transactions.Point{XMM: 31, YMM: 40}, ComponentAt: transactions.Point{XMM: 30, YMM: 40}}

	points, ok := translatedUnitLocalRoutePoints(newTranslatedUnitRouteContext(fragment), route, from, to)
	want := []transactions.Point{{XMM: 24, YMM: 40}, {XMM: 25, YMM: 38}, {XMM: 30, YMM: 38}, {XMM: 31, YMM: 40}}
	if !ok || !slices.Equal(points, want) {
		t.Fatalf("translated points = %#v ok=%v, want %#v", points, ok, want)
	}
}

func TestTranslatedLocalRoutePointsPreservesWaypointsWhenEndpointsSharePlacementDelta(t *testing.T) {
	fragment := BlockFragment{Realization: blocks.BlockPCBRealizationResult{Components: []blocks.RealizedPCBComponent{
		{Ref: "R1", Placement: blocks.RelativePlacement{XMM: 10, YMM: 10}},
		{Ref: "R2", Placement: blocks.RelativePlacement{XMM: 20, YMM: 10}},
	}}}
	route := blocks.RealizedPCBLocalRoute{
		From:   transactions.Endpoint{Ref: "R1", Pin: "1"},
		To:     transactions.Endpoint{Ref: "R2", Pin: "2"},
		Points: []transactions.Point{{XMM: 5, YMM: 10}, {XMM: 10, YMM: 5}, {XMM: 20, YMM: 5}, {XMM: 25, YMM: 10}},
	}
	from := PlacedPadEndpoint{Ref: "R1", Point: transactions.Point{XMM: 9.4, YMM: 10}, ComponentAt: transactions.Point{XMM: 10, YMM: 10}}
	to := PlacedPadEndpoint{Ref: "R2", Point: transactions.Point{XMM: 20.6, YMM: 10}, ComponentAt: transactions.Point{XMM: 20, YMM: 10}}

	points, ok := translatedUnitLocalRoutePoints(newTranslatedUnitRouteContext(fragment), route, from, to)
	want := []transactions.Point{{XMM: 9.4, YMM: 10}, {XMM: 10, YMM: 5}, {XMM: 20, YMM: 5}, {XMM: 20.6, YMM: 10}}
	if !ok || !slices.Equal(points, want) {
		t.Fatalf("translated points = %#v ok=%v, want %#v", points, ok, want)
	}
}

func TestTranslatedLocalRoutePointsPreservesWaypointsFromEntryAnchorToTranslatedComponent(t *testing.T) {
	fragment := BlockFragment{Realization: blocks.BlockPCBRealizationResult{Components: []blocks.RealizedPCBComponent{
		{Ref: "D1", Placement: blocks.RelativePlacement{XMM: 6, YMM: -3}},
	}}}
	route := blocks.RealizedPCBLocalRoute{
		From:   transactions.Endpoint{Ref: "output.driver", Pin: "1"},
		To:     transactions.Endpoint{Ref: "D1", Pin: "2"},
		Points: []transactions.Point{{XMM: -3, YMM: 0}, {XMM: 8, YMM: 0}, {XMM: 8, YMM: -3}, {XMM: 7.2, YMM: -3}},
	}
	from := PlacedPadEndpoint{Ref: "output.driver", Point: transactions.Point{XMM: 55, YMM: 8}, Source: localRouteEntryAnchorSource}
	to := PlacedPadEndpoint{Ref: "D1", Point: transactions.Point{XMM: 65.2, YMM: 5}, ComponentAt: transactions.Point{XMM: 64, YMM: 5}}

	points, ok := translatedUnitLocalRoutePoints(newTranslatedUnitRouteContext(fragment), route, from, to)
	want := []transactions.Point{{XMM: 55, YMM: 8}, {XMM: 66, YMM: 8}, {XMM: 66, YMM: 5}, {XMM: 65.2, YMM: 5}}
	if !ok || !slices.Equal(points, want) {
		t.Fatalf("translated points = %#v ok=%v, want %#v", points, ok, want)
	}
}

func TestCompactRoutePointsKeepsMinimumTrackEndpoints(t *testing.T) {
	points := compactRoutePoints([]transactions.Point{
		{XMM: 10, YMM: 10},
		{XMM: 10.0001, YMM: 10.0001},
	})
	if len(points) != 2 {
		t.Fatalf("points = %#v, want compacted route to retain two endpoints", points)
	}
}

func TestCompactRouteOperationGeometryDropsZeroLengthTracks(t *testing.T) {
	zero := mustGeneratedNetAssignmentRouteOperation(t, "GND")
	zero.Raw = json.RawMessage(`{"op":"route","net_name":"GND","layer":"F.Cu","width_mm":0.25,"points":[{"x_mm":2.6,"y_mm":22},{"x_mm":2.6,"y_mm":22}]}`)
	valid := mustGeneratedNetAssignmentRouteOperation(t, "SDA")

	operations := compactRouteOperationGeometry([]transactions.Operation{zero, valid})
	if len(operations) != 1 || operations[0].Net != "SDA" {
		t.Fatalf("operations = %#v, want only valid SDA route", operations)
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

	operations, issues, summary := localRouteOperations(fragments, &placed)
	if len(issues) != 0 {
		t.Fatalf("local route binding issues = %#v", issues)
	}
	if summary.RoutesAttempted != 1 || summary.RoutesBound != 1 || summary.EndpointsResolved != 2 || summary.EndpointContactsProven != 2 || summary.EmittedTrackSegments != 1 {
		t.Fatalf("route connectivity summary = %#v", summary)
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

func TestLocalRouteOperationsSkipCoincidentTrack(t *testing.T) {
	fragments := PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "coincident",
		Realization: blocks.BlockPCBRealizationResult{LocalRoutes: []blocks.RealizedPCBLocalRoute{{
			ID: "same_point", NetName: "GND",
			From:  transactions.Endpoint{Ref: "C1", Pin: "2"},
			To:    transactions.Endpoint{Ref: "U1", Pin: "2"},
			Layer: "F.Cu", WidthMM: 0.25,
		}}},
	}}}
	placed := PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{
				{Ref: "C1", Pads: []placement.PadSummary{{Name: "2", Net: "GND"}}},
				{Ref: "U1", Pads: []placement.PadSummary{{Name: "2", Net: "GND"}}},
			},
			Nets: []placement.Net{{Name: "GND", Endpoints: []placement.Endpoint{{Ref: "C1", Pin: "2"}, {Ref: "U1", Pin: "2"}}}},
		},
		Result: placement.Result{Status: placement.StatusPlaced, Placements: []placement.PlacementResult{
			{Ref: "C1", Position: placement.Placement{XMM: 10, YMM: 10, Layer: "F.Cu"}},
			{Ref: "U1", Position: placement.Placement{XMM: 10, YMM: 10, Layer: "F.Cu"}},
		}},
		Stage: NewStageResult(StagePlacement, nil),
	}

	operations, issues, summary := localRouteOperations(fragments, &placed)
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
	if len(operations) != 0 {
		t.Fatalf("operations = %#v, want no zero-length track", operations)
	}
	if summary.RoutesBound != 1 || summary.EndpointContactsProven != 2 || summary.EmittedTrackSegments != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestLocalRouteOperationsMaterializesEntryAnchorEndpointVia(t *testing.T) {
	fragments := PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "rail",
		BlockID:    "voltage_regulator",
		Realization: blocks.BlockPCBRealizationResult{
			EntryAnchors: []blocks.RealizedPCBEntryAnchor{{
				ID:      "vout",
				Port:    "VOUT",
				NetName: "VCC_3v3",
				Placement: blocks.RelativePlacement{
					XMM:   38.4,
					YMM:   4,
					Layer: "F.Cu",
				},
			}},
			LocalRoutes: []blocks.RealizedPCBLocalRoute{{
				ID:      "vout_entry",
				NetName: "VCC_3v3",
				From:    transactions.Endpoint{Ref: "@anchor:vout", Pin: "VOUT"},
				To:      transactions.Endpoint{Ref: "C1", Pin: "1"},
				Layer:   "F.Cu",
				WidthMM: 0.5,
				EntryAnchorDogbone: &blocks.PCBEntryAnchorDogbone{
					TieOffset: blocks.RelativePoint{XMM: -1, YMM: 0},
				},
			}},
		},
	}}}
	placed := PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{
				{Ref: "C1", FootprintID: "Test:C", Pads: []placement.PadSummary{{Name: "1", Net: "VCC_3v3", XMM: -0.6, YMM: 0}}},
			},
			Nets: []placement.Net{{Name: "VCC_3v3", Endpoints: []placement.Endpoint{{Ref: "C1", Pin: "1"}}}},
		},
		Result: placement.Result{Status: placement.StatusPlaced, Placements: []placement.PlacementResult{
			{Ref: "C1", FootprintID: "Test:C", Position: placement.Placement{XMM: 28, YMM: 31.5, Layer: "F.Cu"}},
		}},
		Stage: NewStageResult(StagePlacement, nil),
	}

	operations, issues, summary := localRouteOperations(fragments, &placed)
	if len(issues) != 0 {
		t.Fatalf("local route binding issues = %#v", issues)
	}
	if summary.RoutesBound != 1 || summary.EndpointContactsProven != 2 {
		t.Fatalf("route connectivity summary = %#v, want bound route with endpoint contacts", summary)
	}
	if len(operations) != 3 {
		t.Fatalf("operations = %#v, want main route and two entry-anchor dogbone routes", operations)
	}
	var route transactions.RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &route); err != nil {
		t.Fatal(err)
	}
	if len(route.Vias) != 1 {
		t.Fatalf("route vias = %#v, want materialized entry-anchor via", route.Vias)
	}
	if route.Vias[0].At.XMM != 38.4 || route.Vias[0].At.YMM != 4 {
		t.Fatalf("route vias = %#v, want via at entry anchor", route.Vias)
	}
	if len(route.Vias[0].Layers) != 2 || route.Vias[0].Layers[0] != "F.Cu" || route.Vias[0].Layers[1] != "B.Cu" {
		t.Fatalf("route via layers = %#v, want F.Cu/B.Cu", route.Vias[0].Layers)
	}
	var topDogbone transactions.RouteOperation
	if err := json.Unmarshal(operations[1].Raw, &topDogbone); err != nil {
		t.Fatal(err)
	}
	if topDogbone.Layer != "F.Cu" || len(topDogbone.Points) != 2 || len(topDogbone.Vias) != 0 {
		t.Fatalf("top dogbone = %#v, want F.Cu two-point route without vias", topDogbone)
	}
	var bottomDogbone transactions.RouteOperation
	if err := json.Unmarshal(operations[2].Raw, &bottomDogbone); err != nil {
		t.Fatal(err)
	}
	if bottomDogbone.Layer != "B.Cu" || len(bottomDogbone.Points) != 2 || len(bottomDogbone.Vias) != 1 {
		t.Fatalf("bottom dogbone = %#v, want B.Cu two-point route with tie via", bottomDogbone)
	}
	if bottomDogbone.Vias[0].At.XMM != 37.4 || bottomDogbone.Vias[0].At.YMM != 4 {
		t.Fatalf("bottom dogbone vias = %#v, want tie via one millimeter from entry anchor", bottomDogbone.Vias)
	}
}

func TestLocalRouteEntryAnchorDogboneReportsUnsupportedLayer(t *testing.T) {
	from := PlacedPadEndpoint{Ref: "@anchor:vout", Source: localRouteEntryAnchorSource, Point: transactions.Point{XMM: 38.4, YMM: 4}}
	to := PlacedPadEndpoint{Ref: "C1", Point: transactions.Point{XMM: 28, YMM: 31.5}}
	vias := []transactions.RouteViaSpec{{At: from.Point, Layers: []string{"In1.Cu", "B.Cu"}}}
	operations, issues := localRouteEntryAnchorDogboneOperations(
		"routes.regulator.vout_entry",
		"VCC_3v3",
		"In1.Cu",
		0.5,
		[]transactions.Point{from.Point, to.Point},
		from,
		to,
		vias,
		&blocks.PCBEntryAnchorDogbone{TieOffset: blocks.RelativePoint{XMM: -1, YMM: 0}},
	)

	if len(operations) != 0 {
		t.Fatalf("operations = %#v, want no dogbone operation for unsupported layer", operations)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "only supported on F.Cu and B.Cu") {
		t.Fatalf("issues = %#v, want unsupported layer diagnostic", issues)
	}
}

func TestLocalRouteOperationsAddsEndpointViasForCrossLayerRoutes(t *testing.T) {
	fragments := PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "status",
		BlockID:    "led_indicator",
		Realization: blocks.BlockPCBRealizationResult{
			LocalRoutes: []blocks.RealizedPCBLocalRoute{{
				ID:      "series",
				NetName: "SIG",
				From:    transactions.Endpoint{Ref: "R1", Pin: "2"},
				To:      transactions.Endpoint{Ref: "D1", Pin: "1"},
				Layer:   "B.Cu",
				WidthMM: 0.25,
			}},
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

	operations, issues, summary := localRouteOperations(fragments, &placed)
	if len(issues) != 0 {
		t.Fatalf("local route binding issues = %#v", issues)
	}
	if summary.RoutesBound != 1 || len(operations) != 1 {
		t.Fatalf("summary = %#v operations = %#v, want one bound cross-layer route", summary, operations)
	}
	var route transactions.RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &route); err != nil {
		t.Fatal(err)
	}
	if route.Layer != "B.Cu" || len(route.Vias) != 2 {
		t.Fatalf("route = %#v, want B.Cu route with endpoint vias", route)
	}
	if route.Vias[0].At.XMM != 11 || route.Vias[1].At.XMM != 19 {
		t.Fatalf("vias = %#v, want vias at placed pad centers", route.Vias)
	}
}

func TestLocalRouteOperationsMovesEndpointViasToDogboneWaypoints(t *testing.T) {
	fragments := PCBFragmentResult{Fragments: []BlockFragment{{
		InstanceID: "sensor",
		BlockID:    "i2c_sensor",
		Realization: blocks.BlockPCBRealizationResult{LocalRoutes: []blocks.RealizedPCBLocalRoute{
			{
				ID:                  "sda_pullup",
				NetName:             "SDA",
				From:                transactions.Endpoint{Ref: "R1", Pin: "2"},
				To:                  transactions.Endpoint{Ref: "U1", Pin: "3"},
				Points:              []transactions.Point{{XMM: 11, YMM: 5}, {XMM: 13, YMM: 3}, {XMM: 17, YMM: 3}, {XMM: 19, YMM: 5}},
				Layer:               "B.Cu",
				WidthMM:             0.25,
				FromEndpointDogbone: true,
				ToEndpointDogbone:   true,
			},
		}},
	}}}
	placed := PlacementStageResult{
		Request: placement.Request{
			Components: []placement.Component{
				{Ref: "R1", FootprintID: "Test:R", Pads: []placement.PadSummary{{Name: "2", Net: "SDA", XMM: 1}}},
				{Ref: "U1", FootprintID: "Test:U", Pads: []placement.PadSummary{{Name: "3", Net: "SDA", XMM: -1}}},
			},
			Nets: []placement.Net{{Name: "SDA", Endpoints: []placement.Endpoint{{Ref: "R1", Pin: "2"}, {Ref: "U1", Pin: "3"}}}},
		},
		Result: placement.Result{Status: placement.StatusPlaced, Placements: []placement.PlacementResult{
			{Ref: "R1", FootprintID: "Test:R", Position: placement.Placement{XMM: 10, YMM: 5, Layer: "F.Cu"}},
			{Ref: "U1", FootprintID: "Test:U", Position: placement.Placement{XMM: 20, YMM: 5, Layer: "F.Cu"}},
		}},
		Stage: NewStageResult(StagePlacement, nil),
	}

	operations, issues, summary := localRouteOperations(fragments, &placed)
	if len(issues) != 0 || summary.RoutesBound != 1 || len(operations) != 3 {
		t.Fatalf("issues=%#v summary=%#v operations=%#v", issues, summary, operations)
	}
	var mainRoute, fromDogbone, toDogbone transactions.RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &mainRoute); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(operations[1].Raw, &fromDogbone); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(operations[2].Raw, &toDogbone); err != nil {
		t.Fatal(err)
	}
	fromTransition := transactions.Point{XMM: 13, YMM: 3}
	toTransition := transactions.Point{XMM: 17, YMM: 3}
	if mainRoute.Layer != "B.Cu" || len(mainRoute.Points) != 2 || mainRoute.Points[0] != fromTransition || mainRoute.Points[1] != toTransition || len(mainRoute.Vias) != 2 || mainRoute.Vias[0].At != fromTransition || mainRoute.Vias[1].At != toTransition {
		t.Fatalf("main route = %#v", mainRoute)
	}
	if fromDogbone.Layer != "F.Cu" || len(fromDogbone.Points) != 2 || fromDogbone.Points[0] != (transactions.Point{XMM: 11, YMM: 5}) || fromDogbone.Points[1] != fromTransition {
		t.Fatalf("source dogbone = %#v", fromDogbone)
	}
	if toDogbone.Layer != "F.Cu" || len(toDogbone.Points) != 2 || toDogbone.Points[0] != toTransition || toDogbone.Points[1] != (transactions.Point{XMM: 19, YMM: 5}) {
		t.Fatalf("destination dogbone = %#v", toDogbone)
	}
}

func TestLocalRouteConnectivitySummaryJSONStable(t *testing.T) {
	summary := LocalRouteConnectivitySummary{
		RoutesAttempted:        1,
		RoutesBound:            1,
		EndpointsResolved:      2,
		EndpointsUnresolved:    0,
		EndpointContactsProven: 2,
		EndpointNetMismatches:  0,
		EmittedTrackSegments:   1,
		IssueCount:             0,
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"routes_attempted":1,"routes_bound":1,"endpoints_resolved":2,"endpoints_unresolved":0,"endpoint_contacts_proven":2,"endpoint_net_mismatches":0,"emitted_track_segments":1,"issue_count":0}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
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

func TestRoutePlacementPromotedInterBlockConnectorLEDNetReportsDisconnectedCompletion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request := Request{
		Version: RequestVersion,
		Name:    "connector_led",
		Board:   BoardSpec{WidthMM: 45, HeightMM: 30, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "header", BlockID: "connector_breakout", Params: map[string]any{"pin_count": 2, "pin_names": []string{"SIG", "GND"}}},
			{ID: "status", BlockID: "led_indicator"},
		},
		Connections: []ConnectionSpec{{From: "header.SIG", To: "status.IN", NetAlias: "LED_EN"}},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(ctx, registry, request)
	if !stageUsableForRoutingTest(plan.Stage) {
		t.Fatalf("planning failed: %#v", plan.Stage.Issues)
	}
	fragments := RealizePCBFragments(ctx, registry, plan)
	if !stageUsableForRoutingTest(fragments.Stage) {
		t.Fatalf("PCB realization failed: %#v", fragments.Stage.Issues)
	}
	placed := PlaceFragments(ctx, request, fragments, PlacementOptions{})
	if !stageUsableForRoutingTest(placed.Stage) {
		t.Fatalf("placement failed: %#v", placed.Stage.Issues)
	}
	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	if _, ok := interBlockCandidateByNetForRoutingTest(candidates, "LED_EN"); !ok {
		t.Fatalf("candidates = %#v, want LED_EN", candidates)
	}

	result := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	if result.Stage.Status != StageStatusOK {
		t.Fatalf("routing status = %s, want proven route-completion evidence; issues=%#v", result.Stage.Status, result.Stage.Issues)
	}
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, result.Stage)
	if !stringSliceContains(routeTrees.ManagedNets, "LED_EN") {
		t.Fatalf("route-tree managed nets = %#v, want LED_EN", routeTrees.ManagedNets)
	}
	if routingRequestHasNet(result.Request, "LED_EN") {
		t.Fatalf("routing request nets = %#v, did not want route-tree managed LED_EN in fallback request", result.Request.Nets)
	}
	routes := requireRouteOperationsForNet(t, result.Operations, "LED_EN")
	for _, route := range routes {
		if len(route.Points) < 2 {
			t.Fatalf("LED_EN route has %d points, want at least 2", len(route.Points))
		}
		for pointIndex := 0; pointIndex < len(route.Points)-1; pointIndex++ {
			current := route.Points[pointIndex]
			next := route.Points[pointIndex+1]
			if pointsNearlyEqual(current, next) {
				t.Fatalf("LED_EN route has degenerate segment at point %d: (%.9f, %.9f) -> (%.9f, %.9f)", pointIndex, current.XMM, current.YMM, next.XMM, next.YMM)
			}
		}
	}
	assertNoIssueCode(t, result.Stage.Issues, reports.CodeDisconnectedPad)
	interBlock := requireInterBlockRouteSummary(t, result.Stage)
	if interBlock.Candidates == 0 || interBlock.RoutesAttempted == 0 {
		t.Fatalf("inter-block summary = %#v, want attempted candidate", interBlock)
	}
	if interBlock.RoutesCompleted == 0 || interBlock.PartialNets != 0 || interBlock.EmittedSegments == 0 || interBlock.IssueCount != 0 {
		t.Fatalf("inter-block summary = %#v, want completed routed evidence for LED_EN", interBlock)
	}
	contacts := requireInterBlockContactSummary(t, result.Stage)
	if contacts.ContactsRequired == 0 || contacts.ContactsProven == 0 || contacts.ContactMisses != 0 {
		t.Fatalf("inter-block contact summary = %#v, want snapped contact evidence", contacts)
	}
}

func TestRoutePlacementI2CSensorBreakoutReportsInterBlockContactEvidence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)
	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	candidateNets := interBlockCandidateNetSetForRoutingTest(candidates)
	for _, connection := range request.Connections {
		netName := connection.NetAlias
		candidate, ok := interBlockCandidateByNetForRoutingTest(candidates, netName)
		if !ok || candidate.Status != InterBlockRouteCandidateRoutable || len(candidate.Endpoints) < 2 {
			t.Fatalf("candidate %s = %#v, ok=%v", netName, candidate, ok)
		}
	}
	for net := range connectionAliasSet(request.Connections) {
		if !candidateNets[net] {
			t.Fatalf("candidate nets = %#v, want request alias %s", candidateNets, net)
		}
	}
	expectedEndpoints := interBlockCandidateEndpointCount(candidates)

	result := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, result.Stage)
	for _, connection := range request.Connections {
		if !stringSliceContains(routeTrees.ManagedNets, connection.NetAlias) {
			t.Fatalf("route-tree managed nets = %#v, want %s", routeTrees.ManagedNets, connection.NetAlias)
		}
	}
	access := requireStageSummary[RouteTreeEndpointAccessSummary](t, result.Stage, "route_tree_access")
	if access.PadAccess < expectedEndpoints || access.LocalRouteAnchors == 0 {
		t.Fatalf("route-tree access summary = %#v, want pad and local-route anchor evidence", access)
	}
	contactGraph := requireStageSummary[RouteTreeContactGraphSummary](t, result.Stage, "route_tree_contact_graph")
	if contactGraph.RequiredEndpoints != expectedEndpoints || contactGraph.LocalRouteMerges == 0 {
		t.Fatalf("route-tree contact graph = %#v, want required endpoints and local-route merge evidence", contactGraph)
	}
	interBlock := requireInterBlockRouteSummary(t, result.Stage)
	if interBlock.Candidates != len(candidates) || interBlock.EndpointsResolved != expectedEndpoints {
		t.Fatalf("inter-block summary counts = candidates %d endpoints %d, want candidate builder counts %d and %d", interBlock.Candidates, interBlock.EndpointsResolved, len(candidates), expectedEndpoints)
	}
	if interBlock.MultiEndpointNets != 0 || interBlock.RequiredEndpoints != expectedEndpoints {
		t.Fatalf("inter-block group summary = %#v, want locally-pruned two-endpoint net and required endpoint counts", interBlock)
	}
	if interBlock.BranchesPlanned == 0 || interBlock.GraphComponentCount == 0 {
		t.Fatalf("inter-block route-tree summary = %#v, want planned branches and graph component evidence", interBlock)
	}
	contacts := requireInterBlockContactSummary(t, result.Stage)
	if contacts.ContactsRequired != expectedEndpoints || contacts.ContactsProven+contacts.ContactsFailed != expectedEndpoints {
		t.Fatalf("inter-block contact counts = required %d resolved %d, want %d", contacts.ContactsRequired, contacts.ContactsProven+contacts.ContactsFailed, expectedEndpoints)
	}
	repair := requireRouteTreeRepairSummary(t, result.Stage)
	if repair.BranchFailures != 0 || repair.RepairableFailures != 0 || repair.HintCount != 0 {
		t.Fatalf("route-tree repair summary = %#v, want no route-tree branch/contact failures", repair)
	}
	if len(repair.Nets) != 0 {
		t.Fatalf("route-tree repair nets = %#v, want no route-tree repair nets", repair.Nets)
	}
}

func TestRoutePlacementI2CSensorBreakoutAuditsMultiEndpointBlocker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)
	var waypointedRoutes int
	for _, fragment := range fragments.Fragments {
		if fragment.BlockID != "i2c_sensor" {
			continue
		}
		for _, route := range fragment.Realization.LocalRoutes {
			if len(route.Points) > 2 {
				waypointedRoutes++
			}
		}
	}
	if waypointedRoutes < 6 {
		t.Fatalf("I2C local routes preserved %d waypointed routes, want 6", waypointedRoutes)
	}
	table, tableIssues := BuildGeneratedNetTable(&placed, nil)
	if len(tableIssues) != 0 {
		t.Fatalf("generated net table issues = %#v", tableIssues)
	}
	resolver := NewPlacedPadEndpointResolver(&placed, table)
	for _, fragment := range fragments.Fragments {
		if fragment.BlockID != "i2c_sensor" {
			continue
		}
		for _, route := range fragment.Realization.LocalRoutes {
			from, fromOK := resolver.Resolve(route.From)
			to, toOK := resolver.Resolve(route.To)
			if !fromOK || !toOK {
				t.Fatalf("route %s endpoints resolved from=%v to=%v", route.ID, fromOK, toOK)
			}
			if points, ok := placedLocalRoutePoints(route.Points, from.Point, to.Point); !ok || len(points) < 2 {
				t.Fatalf("route %s placed points = %#v ok=%v, want routable points from first=%#v from=%#v last=%#v to=%#v all=%#v", route.ID, points, ok, route.Points[0], from.Point, route.Points[len(route.Points)-1], to.Point, route.Points)
			}
		}
	}

	result := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	for _, netName := range []string{"SDA", "SCL"} {
		routes := requireRouteOperationsForNet(t, result.Operations, netName)
		var foundBottomLocal bool
		for _, route := range routes {
			if route.Layer == "B.Cu" && len(route.Vias) == 2 {
				foundBottomLocal = true
			}
		}
		if !foundBottomLocal {
			t.Fatalf("%s routes = %#v, want bottom-layer local pull-up route with endpoint vias", netName, routes)
		}
	}
	plan := PlanBlocks(ctx, blocks.NewBuiltinRegistry(), request)
	tx, txIssues := ProjectTransaction(&request, &plan, &placed, &result, true)
	if len(txIssues) != 0 {
		t.Fatalf("project transaction issues = %#v", txIssues)
	}
	if routeViaCountForRoutingTest(t, tx.Operations, "SDA")+routeViaCountForRoutingTest(t, tx.Operations, "SCL") < 4 {
		t.Fatalf("transaction operations lost local-route vias: %#v", tx.Operations)
	}
	output := t.TempDir()
	apply := transactions.Apply(tx, transactions.ApplyOptions{OutputDir: output, Overwrite: true})
	if len(apply.Issues) != 0 {
		t.Fatalf("apply issues = %#v", apply.Issues)
	}
	pcbBytes, err := os.ReadFile(filepath.Join(output, request.Name+".kicad_pcb"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pcbBytes), "(via") {
		t.Fatalf("applied PCB lost local-route vias")
	}
	local := requireStageSummary[LocalRouteConnectivitySummary](t, result.Stage, "route_connectivity")
	if local.RoutesAttempted == 0 || local.RoutesBound != local.RoutesAttempted {
		t.Fatalf("local route connectivity = %#v, want every local route bound", local)
	}
	if local.EndpointsUnresolved != 0 || local.EndpointNetMismatches != 0 || local.IssueCount != 0 {
		t.Fatalf("local route connectivity = %#v, want no local endpoint blockers", local)
	}
	if local.EndpointContactsProven < local.RoutesAttempted*2 {
		t.Fatalf("local route connectivity = %#v, want at least two proven endpoint contacts per local route", local)
	}
	if local.EmittedTrackSegments <= local.RoutesBound {
		t.Fatalf("local route connectivity = %#v, want waypointed local routes emitted as multi-segment tracks", local)
	}

	interBlock := requireInterBlockRouteSummary(t, result.Stage)
	expectedNets := len(connectionAliasSet(request.Connections))
	if interBlock.Candidates != expectedNets || interBlock.EndpointsResolved != expectedNets*2 {
		t.Fatalf("inter-block summary = %#v, want four locally-pruned two-endpoint I2C candidates", interBlock)
	}
	if interBlock.MultiEndpointNets != 0 || interBlock.RequiredEndpoints != interBlock.EndpointsResolved {
		t.Fatalf("inter-block group summary = %#v, want all I2C nets represented as locally-pruned two-endpoint groups", interBlock)
	}
	if interBlock.BranchesPlanned < expectedNets || interBlock.BranchesAttempted == 0 || interBlock.BranchesAttempted > interBlock.BranchesPlanned {
		t.Fatalf("inter-block branch summary = %#v, want attempted branches bounded by planned branches", interBlock)
	}
	if interBlock.MissingRequired != 0 {
		t.Fatalf("inter-block route-tree missing endpoints = %#v, want target resolution complete", interBlock)
	}
	if interBlock.RoutesCompleted != interBlock.Candidates || interBlock.PartialNets != 0 || interBlock.UnroutedNets != 0 {
		t.Fatalf("inter-block summary = %#v, want all multi-endpoint I2C routes complete", interBlock)
	}
	if interBlock.IssueCount == 0 {
		t.Fatalf("inter-block summary = %#v, want fixed-net preservation notices retained as routing evidence", interBlock)
	}

	contacts := requireInterBlockContactSummary(t, result.Stage)
	if contacts.ContactsRequired != interBlock.EndpointsResolved {
		t.Fatalf("contact summary = %#v, inter-block summary = %#v, want contact targets for every resolved endpoint", contacts, interBlock)
	}
	if contacts.ContactsFailed != 0 {
		t.Fatalf("contact summary = %#v, want all inter-block contacts proven", contacts)
	}
	if contacts.NetMismatches != 0 {
		t.Fatalf("contact summary = %#v, want no net-alias mismatch after I2C alias hydration", contacts)
	}
	vccIssues := routeTreeIssuesForNet(result.Stage.Issues, "VCC")
	if routeTreeIssuesContainCode(vccIssues, reports.CodeRouteGraphIncomplete) || routeTreeIssuesContainCode(vccIssues, reports.CodeRouteContactMiss) || routeTreeIssuesContainMessage(vccIssues, "no legal") {
		t.Fatalf("VCC issues = %#v, want no graph-split, contact-miss, or pathfinding blocker", vccIssues)
	}
	repair := requireRouteTreeRepairSummary(t, result.Stage)
	if repair.HintCount != 0 || len(repair.Nets) != 0 {
		t.Fatalf("route-tree repair summary = %#v, want no route-tree repair hints", repair)
	}

	blockedNets := issueNetSet(result.Stage.Issues)
	i2cNets := connectionAliasSet(request.Connections)
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, result.Stage)
	for net := range i2cNets {
		if !stringSliceContains(routeTrees.ManagedNets, net) {
			t.Fatalf("route-tree managed nets = %#v, want %s", routeTrees.ManagedNets, net)
		}
	}
	for net := range blockedNets {
		if !i2cNets[net] {
			t.Fatalf("blocked nets = %#v, want blockers tied to I2C nets", blockedNets)
		}
	}
	branchPaths := routeTreeBranchIssuePathsByNet(result.Stage.Issues)
	if len(branchPaths["VCC"]) != 0 {
		t.Fatalf("route-tree branch issue paths by net = %#v, want no VCC blocker paths", branchPaths)
	}
}

func TestCreateI2CSensorBreakoutCapturesAccessDrivenBaseline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)
	if len(fragments.Fragments) == 0 || placed.Result.Metrics.PlacedCount == 0 {
		t.Fatalf("fixture fragments=%#v placement=%#v, want realized and placed I2C fixture", fragments, placed.Result)
	}

	result := Create(ctx, request, CreateOptions{})
	routingStage, ok := workflowStageForRoutingTest(result, StageRouting)
	if !ok {
		t.Fatalf("stages = %#v, want routing stage", result.Stages)
	}
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, routingStage)
	access := requireStageSummary[RouteTreeEndpointAccessSummary](t, routingStage, "route_tree_access")
	contactGraph := requireStageSummary[RouteTreeContactGraphSummary](t, routingStage, "route_tree_contact_graph")
	retry := requireStageSummary[placementRoutingRetrySummary](t, routingStage, "routing_retry")

	if access.LocalRouteAnchors == 0 {
		t.Fatalf("route-tree access = %#v, want local-route anchors", access)
	}
	for _, net := range routeTrees.ManagedNets {
		if !stringSliceContains(access.Nets, net) {
			t.Fatalf("access nets = %#v, want managed net %s", access.Nets, net)
		}
	}
	if contactGraph.ProvenEndpoints != 8 || contactGraph.CompleteGroups != 4 || contactGraph.PartialGroups != 0 {
		t.Fatalf("contact graph = %#v, want 8 proven endpoints and 4 complete groups", contactGraph)
	}
	wantGraphGroups := map[string]RouteTreeContactGraphGroupSummary{
		"GND": {Status: RouteTreeContactGraphGroupComplete, RequiredEndpoints: 2, ProvenEndpoints: 2, Components: 1},
		"SCL": {Status: RouteTreeContactGraphGroupComplete, RequiredEndpoints: 2, ProvenEndpoints: 2, Components: 1},
		"SDA": {Status: RouteTreeContactGraphGroupComplete, RequiredEndpoints: 2, ProvenEndpoints: 2, Components: 1},
		"VCC": {Status: RouteTreeContactGraphGroupComplete, RequiredEndpoints: 2, ProvenEndpoints: 2, Components: 1},
	}
	for _, group := range contactGraph.Groups {
		expected, ok := wantGraphGroups[group.NetName]
		if !ok {
			t.Fatalf("contact graph groups = %#v, unexpected net %s", contactGraph.Groups, group.NetName)
		}
		if group.Status != expected.Status || group.RequiredEndpoints != expected.RequiredEndpoints || group.ProvenEndpoints != expected.ProvenEndpoints || group.Components != expected.Components || !slices.Equal(group.MissingEndpointIDs, expected.MissingEndpointIDs) {
			t.Fatalf("contact graph group[%s] = %#v, want %#v", group.NetName, group, expected)
		}
		delete(wantGraphGroups, group.NetName)
	}
	if len(wantGraphGroups) != 0 {
		t.Fatalf("contact graph groups = %#v, missing expected groups %#v", contactGraph.Groups, wantGraphGroups)
	}
	if retry.Attempts != 1 || retry.Applied != 0 || len(retry.AttemptHistory) != 1 {
		t.Fatalf("retry = %#v, want initial routed attempt without repair retry", retry)
	}
	if !retry.AttemptHistory[0].Selected || retry.AttemptHistory[0].RouteTreeProvenEndpoints != 8 || retry.AttemptHistory[0].RouteTreeBranchesRouted != 4 {
		t.Fatalf("retry history = %#v, want initial attempt selected with complete route-tree contact evidence", retry.AttemptHistory)
	}
	branchPaths := routeTreeBranchIssuePathsByNet(routingStage.Issues)
	if len(branchPaths["VCC"]) != 0 {
		t.Fatalf("branch paths = %#v, want no selected-attempt VCC blocker evidence", branchPaths)
	}
}

func TestCreateI2CSensorBreakoutLocksResolvedVCCProofGap(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, _, _ := i2cSensorBreakoutRoutingFixture(t, ctx)

	result := Create(ctx, request, CreateOptions{})
	routingStage, ok := workflowStageForRoutingTest(result, StageRouting)
	if !ok {
		t.Fatalf("stages = %#v, want routing stage", result.Stages)
	}
	contactGraph := requireStageSummary[RouteTreeContactGraphSummary](t, routingStage, "route_tree_contact_graph")
	if contactGraph.RequiredEndpoints != 8 || contactGraph.ProvenEndpoints != 8 || contactGraph.CompleteGroups != 4 || contactGraph.PartialGroups != 0 {
		t.Fatalf("contact graph = %#v, want required=8 proven=8 complete=4 partial=0", contactGraph)
	}
	repair := requireRouteTreeRepairSummary(t, routingStage)
	if repair.HintCount != 0 || repair.RepairableFailures != 0 || len(repair.Nets) != 0 {
		t.Fatalf("route-tree repair summary = %#v, want all route-tree proof gaps resolved", repair)
	}
	vccIssues := routeTreeIssuesForNet(routingStage.Issues, "VCC")
	if routeTreeIssuesContainCode(vccIssues, reports.CodeRouteGraphIncomplete) ||
		routeTreeIssuesContainCode(vccIssues, reports.CodeRouteContactMiss) ||
		routeTreeIssuesContainMessage(vccIssues, "no legal") {
		t.Fatalf("VCC issues = %#v, want no graph-split, contact-miss, or pathfinding blocker", vccIssues)
	}
}

func TestCreateI2CSensorBreakoutCapturesPromotionInventory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, _, _ := i2cSensorBreakoutRoutingFixture(t, ctx)

	result := Create(ctx, request, CreateOptions{})
	promotion := BuildInternalPromotionReport(i2cSensorBreakoutPromotionFixtureForRoutingTest(), result)
	if promotion.Status != PromotionStatusExpectedFail || promotion.AchievedReadiness != PromotionReadinessExpectedFail || !promotion.MatchesExpectation {
		t.Fatalf("promotion = %#v, want expected-fail inventory match", promotion)
	}
	routeGate, ok := promotionGateByIDForRoutingTest(promotion, "route_completion")
	if !ok || routeGate.Status != PromotionGateStatusPass {
		t.Fatalf("route gate = %#v, ok=%v, want passing route-completion gate", routeGate, ok)
	}
	stageGate, ok := promotionGateByIDForRoutingTest(promotion, "stages")
	if !ok || stageGate.Status != PromotionGateStatusFailed || len(stageGate.IssueCodes) == 0 {
		t.Fatalf("stage gate = %#v, ok=%v, want blocked stage issue codes", stageGate, ok)
	}

	routingStage, ok := workflowStageForRoutingTest(result, StageRouting)
	if !ok {
		t.Fatalf("stages = %#v, want routing stage", result.Stages)
	}
	interBlock := requireInterBlockRouteSummary(t, routingStage)
	if interBlock.MultiEndpointNets != 0 || interBlock.RequiredEndpoints != 8 || interBlock.ProvenEndpoints != 8 {
		t.Fatalf("inter-block summary = %#v, want 4 locally-pruned I2C nets and 8/8 proven endpoints", interBlock)
	}
	if interBlock.CompleteGroups != 4 || interBlock.PartialGroups != 0 || interBlock.BlockedGroups != 0 {
		t.Fatalf("inter-block groups = %#v, want four complete graph-derived route-completion groups", interBlock)
	}
	if interBlock.BranchesAttempted != 4 || interBlock.BranchesCompleted != 4 {
		t.Fatalf("inter-block branches = %#v, want all four route-tree branches completed", interBlock)
	}
	if interBlock.RoutesCompleted != 4 || interBlock.PartialNets != 0 || interBlock.UnroutedNets != 0 {
		t.Fatalf("inter-block route completion = %#v, want four complete route-tree nets", interBlock)
	}

	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, routingStage)
	i2cNets := connectionAliasSet(request.Connections)
	for net := range i2cNets {
		if !stringSliceContains(routeTrees.ManagedNets, net) {
			t.Fatalf("route-tree managed nets = %#v, want %s", routeTrees.ManagedNets, net)
		}
	}
	if routeTrees.GroupsComplete != 4 || routeTrees.GroupsPartial != 0 || routeTrees.GroupsBlocked != 0 || routeTrees.BranchesRouted != 4 || routeTrees.BranchesBlocked != 0 {
		t.Fatalf("route-tree execution = %#v, want all route-tree branches emitted before contact proof", routeTrees)
	}
	if routeTrees.FixedNetSkipNotices == 0 {
		t.Fatalf("route-tree execution = %#v, want fixed-net preservation notices in promotion inventory", routeTrees)
	}

	contactGraph := requireStageSummary[RouteTreeContactGraphSummary](t, routingStage, "route_tree_contact_graph")
	if contactGraph.RequiredEndpoints != 8 || contactGraph.ProvenEndpoints != 8 || contactGraph.Components == 0 {
		t.Fatalf("contact graph = %#v, want required/proven endpoint and component inventory", contactGraph)
	}
	if contactGraph.CompleteGroups != 4 || contactGraph.PartialGroups != 0 || contactGraph.BlockedGroups != 0 {
		t.Fatalf("contact graph groups = %#v, want complete route-tree contact baseline", contactGraph)
	}

	retry := requireStageSummary[placementRoutingRetrySummary](t, routingStage, "routing_retry")
	if retry.Attempts != 1 || retry.Applied != 0 || len(retry.AttemptHistory) != 1 || !retry.AttemptHistory[0].Selected {
		t.Fatalf("retry = %#v, want selected initial attempt without applied retry", retry)
	}
	if retry.AttemptHistory[0].RouteTreeProvenEndpoints != 8 || retry.AttemptHistory[0].RouteTreeBranchesRouted != 4 {
		t.Fatalf("retry history = %#v, want selected attempt route-tree evidence", retry.AttemptHistory)
	}

	blockedNets := issueNetSet(routingStage.Issues)
	for net := range blockedNets {
		if !i2cNets[net] {
			t.Fatalf("blocked net %q is outside I2C route-tree nets %#v", net, i2cNets)
		}
	}
	branchPaths := routeTreeBranchIssuePathsByNet(routingStage.Issues)
	if len(branchPaths) != 0 {
		t.Fatalf("branch paths = %#v, want no selected-attempt branch blockers", branchPaths)
	}
	for _, issue := range routingStage.Issues {
		if issue.Severity != reports.SeverityBlocked && issue.Severity != reports.SeverityError {
			continue
		}
		if len(issue.Nets) == 0 {
			continue
		}
		if !strings.Contains(issue.Path, "design.inter_block_route_groups") && !strings.Contains(issue.Path, "design.inter_block_contact") {
			t.Fatalf("unexpected high-severity net issue outside route-tree evidence paths: %#v", issue)
		}
	}
}

func TestCreateI2CSensorBreakoutCapturesDownstreamPromotionBlocker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	request, _, _ := i2cSensorBreakoutRoutingFixture(t, ctx)

	result := Create(ctx, request, CreateOptions{})
	promotion := BuildInternalPromotionReport(i2cSensorBreakoutPromotionFixtureForRoutingTest(), result)
	stagesByName := map[StageName]StageResult{}
	for _, stage := range result.Stages {
		stagesByName[stage.Name] = stage
	}

	wantStages := []struct {
		name      StageName
		status    StageStatus
		issueCode reports.Code
		issuePath string
	}{
		{name: StageRouting, status: StageStatusOK},
		{name: StageProjectWrite, status: StageStatusBlocked, issueCode: reports.CodeInvalidArgument, issuePath: "output"},
		{name: StageWriterCorrect, status: StageStatusSkipped},
		{name: StageValidation, status: StageStatusSkipped},
		{name: StageKiCadChecks, status: StageStatusSkipped},
	}
	var projectWriteIssue reports.Issue
	for _, want := range wantStages {
		stage, ok := stagesByName[want.name]
		if !ok {
			t.Fatalf("stages = %#v, want stage %s", result.Stages, want.name)
		}
		if stage.Status != want.status {
			t.Fatalf("%s status = %s issues=%#v, want %s", want.name, stage.Status, stage.Issues, want.status)
		}
		if want.issueCode != "" {
			issue, ok := issueByCodeAndPathForRoutingTest(stage.Issues, want.issueCode, want.issuePath)
			if !ok {
				t.Fatalf("%s issues = %#v, want %s at %s", want.name, stage.Issues, want.issueCode, want.issuePath)
			}
			if want.name == StageProjectWrite {
				projectWriteIssue = issue
			}
		}
	}
	if projectWriteIssue.Code == "" {
		t.Fatalf("project_write issue was not captured from stage checks")
	}

	routeGate, ok := promotionGateByIDForRoutingTest(promotion, "route_completion")
	if !ok || routeGate.Status != PromotionGateStatusPass {
		t.Fatalf("route gate = %#v, ok=%v, want route completion pass", routeGate, ok)
	}
	stageGate, ok := promotionGateByIDForRoutingTest(promotion, "stages")
	if !ok || stageGate.Status != PromotionGateStatusFailed {
		t.Fatalf("stage gate = %#v, ok=%v, want downstream stage failure", stageGate, ok)
	}
	expectedProjectWriteCode, ok := promotionIssueCodeByStageAndPathForRoutingTest(promotion.Issues, StageProjectWrite, projectWriteIssue.Path)
	if !ok {
		t.Fatalf("promotion issues = %#v, want project_write promotion issue for %s", promotion.Issues, projectWriteIssue.Path)
	}
	if !slices.Contains(stageGate.IssueCodes, expectedProjectWriteCode) {
		t.Fatalf("stage gate issue codes = %#v, want exact project_write blocker %s", stageGate.IssueCodes, expectedProjectWriteCode)
	}
	if promotion.AchievedReadiness != PromotionReadinessExpectedFail || promotion.Status != PromotionStatusExpectedFail {
		t.Fatalf("promotion = %#v, want expected-fail due downstream stage blockers", promotion)
	}
}

func TestCreateI2CSensorBreakoutWritesProjectArtifactsAfterRouteTreeProof(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, _, _ := i2cSensorBreakoutRoutingFixture(t, ctx)
	outputDir := t.TempDir()

	result := Create(ctx, request, CreateOptions{OutputDir: outputDir, Overwrite: true})
	stagesByName := map[StageName]StageResult{}
	for _, stage := range result.Stages {
		stagesByName[stage.Name] = stage
	}

	routingStage, ok := stagesByName[StageRouting]
	if !ok {
		t.Fatalf("stages = %#v, want routing stage", result.Stages)
	}
	if routingStage.Status != StageStatusOK {
		t.Fatalf("routing status = %s issues=%#v, want %s", routingStage.Status, routingStage.Issues, StageStatusOK)
	}
	interBlock := requireInterBlockRouteSummary(t, routingStage)
	if interBlock.RequiredEndpoints != 8 || interBlock.ProvenEndpoints != 8 || interBlock.CompleteGroups != 4 {
		t.Fatalf("inter-block summary = %#v, want complete 8/8 endpoint route-tree proof", interBlock)
	}
	routeTrees := requireInterBlockRouteTreeExecutionSummary(t, routingStage)
	if routeTrees.GroupsComplete != 4 || routeTrees.BranchesRouted != 4 || routeTrees.BranchesBlocked != 0 {
		t.Fatalf("route-tree execution = %#v, want all I2C route-tree branches complete", routeTrees)
	}

	projectWrite, ok := stagesByName[StageProjectWrite]
	if !ok {
		t.Fatalf("stages = %#v, want project_write stage after route-tree proof", result.Stages)
	}
	if issue, ok := issueByCodeAndPathForRoutingTest(projectWrite.Issues, reports.CodeInvalidArgument, "output"); ok {
		t.Fatalf("project_write still reports missing output blocker: %#v", issue)
	}
	if projectWrite.Status != StageStatusOK {
		t.Fatalf("project_write status = %s issues=%#v, want %s", projectWrite.Status, projectWrite.Issues, StageStatusOK)
	}
	for _, filename := range []string{
		"i2c_sensor_breakout_candidate.kicad_pro",
		"i2c_sensor_breakout_candidate.kicad_sch",
		"i2c_sensor_breakout_candidate.kicad_pcb",
		".kicadai/transaction.json",
		".kicadai/manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(outputDir, filename)); err != nil {
			t.Fatalf("missing generated artifact %s: %v", filename, err)
		}
	}
	if len(projectWrite.Artifacts) == 0 {
		t.Fatalf("project_write artifacts = %#v, want emitted KiCad project artifacts", projectWrite.Artifacts)
	}
	writerCorrect, ok := stagesByName[StageWriterCorrect]
	if !ok {
		t.Fatalf("stages = %#v, want downstream stage %s after project_write", result.Stages, StageWriterCorrect)
	}
	if writerCorrect.Status != StageStatusWarning {
		t.Fatalf("writer_correctness status = %s issues=%#v, want warning only for skipped optional round-trip", writerCorrect.Status, writerCorrect.Issues)
	}
	if reports.HasBlockingIssue(writerCorrect.Issues) {
		t.Fatalf("writer_correctness issues = %#v, want warning-only accepted evidence", writerCorrect.Issues)
	}
	if !stageHasIssueCodeForRoutingTest(writerCorrect, reports.CodeSkippedExternalTool) {
		t.Fatalf("writer_correctness issues = %#v, want optional round-trip warning", writerCorrect.Issues)
	}
	for _, blockedCode := range []reports.Code{reports.CodeUnknownFootprintLibrary, reports.CodeDisconnectedPad, reports.CodeValidationFailed} {
		if stageHasIssueCodeForRoutingTest(writerCorrect, blockedCode) {
			t.Fatalf("writer_correctness issues = %#v, want no pad/copper readback blocker %s", writerCorrect.Issues, blockedCode)
		}
	}
	validation, ok := stagesByName[StageValidation]
	if !ok {
		t.Fatalf("stages = %#v, want downstream stage %s after project_write", result.Stages, StageValidation)
	}
	if validation.Status != StageStatusOK {
		t.Fatalf("validation status = %s issues=%#v, want clean default local validation", validation.Status, validation.Issues)
	}
	for _, blockedCode := range []reports.Code{reports.CodeValidationFailed, reports.CodeDisconnectedPad} {
		if stageHasIssueCodeForRoutingTest(validation, blockedCode) {
			t.Fatalf("validation issues = %#v, want no structural blocker %s", validation.Issues, blockedCode)
		}
	}
	for _, issue := range validation.Issues {
		if issue.Code == reports.CodeSkippedExternalTool && issue.Severity != reports.SeverityInfo {
			t.Fatalf("validation issues = %#v, want skipped external checks to remain informational", validation.Issues)
		}
	}
	kicadChecks, ok := stagesByName[StageKiCadChecks]
	if !ok {
		t.Fatalf("stages = %#v, want kicad_checks stage after project_write", result.Stages)
	}
	if kicadChecks.Status != StageStatusSkipped {
		t.Fatalf("kicad_checks status = %s issues=%#v, want skipped until KiCad CLI evidence is configured", kicadChecks.Status, kicadChecks.Issues)
	}
}

func TestI2CSensorBreakoutRouteTreeEndpointAccessCandidatesStable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)

	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	targetEvidence := BuildInterBlockContactTargets(candidates, &placed)
	if len(targetEvidence.Issues) != 0 {
		t.Fatalf("target evidence issues = %#v", targetEvidence.Issues)
	}
	routed := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	access := BuildRouteTreeEndpointAccess(targetEvidence, routed.Operations)
	if len(access) == 0 {
		t.Fatalf("access = %#v, want route-tree endpoint access", access)
	}

	i2cNets := connectionAliasSet(request.Connections)
	targetsByNet := interBlockContactTargetsByNet(targetEvidence.Targets)
	for net := range i2cNets {
		targets := targetsByNet[net]
		if len(targets) == 0 {
			t.Fatalf("targets by net = %#v, want targets for %s", targetsByNet, net)
		}
		for _, target := range targets {
			candidates := routeTreeAccessCandidatesForEndpoint(access, target.EndpointID, net, RouteTreeEndpointAccess{Net: net, XMM: target.Point.XMM + 1, YMM: target.Point.YMM})
			if len(candidates) == 0 {
				t.Fatalf("access = %#v, want candidates for %s %s", access, net, target.EndpointID)
			}
			switch candidates[0].Access.Role {
			case RouteTreeAccessLocalRouteAnchor, RouteTreeAccessTargetPad, RouteTreeAccessSourcePad, RouteTreeAccessSameNetCopper:
			default:
				t.Fatalf("candidates = %#v, want stable physical access role for %s", candidates, target.EndpointID)
			}
			if candidates[0].RankReason == "" {
				t.Fatalf("candidates = %#v, want ranking reason for selected %s", candidates, target.EndpointID)
			}
		}
	}
}

func TestI2CSensorBreakoutCompletesConnectorEndpointContactGraph(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)

	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	targetEvidence := BuildInterBlockContactTargets(candidates, &placed)
	if len(targetEvidence.Issues) != 0 {
		t.Fatalf("target evidence issues = %#v", targetEvidence.Issues)
	}
	routed := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	if err := ctx.Err(); err != nil {
		t.Fatalf("RoutePlacement context error: %v", err)
	}
	if routed.Stage.Name != StageRouting || len(routed.Operations) == 0 {
		t.Fatalf("routed stage=%#v operations=%d, want routing result with operations", routed.Stage, len(routed.Operations))
	}
	contactEvidence := ValidateInterBlockRouteEndpointContacts(candidates, routed.Operations, &placed)
	gaps := routeTreeContactGraphGapsForRoutingTest(t, contactEvidence, routed.Operations)
	if len(gaps) != 0 {
		t.Fatalf("gaps = %#v, want all I2C connector endpoints proven", gaps)
	}
}

func TestI2CSensorBreakoutHasNoMissingConnectorEndpointGeometry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	request, fragments, placed := i2cSensorBreakoutRoutingFixture(t, ctx)

	candidates, candidateIssues := BuildInterBlockRouteCandidates(fragments, placed)
	if len(candidateIssues) != 0 {
		t.Fatalf("candidate issues = %#v", candidateIssues)
	}
	routed := RoutePlacement(ctx, request, fragments, placed, RoutingOptions{})
	if err := ctx.Err(); err != nil {
		t.Fatalf("RoutePlacement context error: %v", err)
	}
	contactEvidence := ValidateInterBlockRouteEndpointContacts(candidates, routed.Operations, &placed)
	diagnostics := routeTreeMissingEndpointGeometryForRoutingTest(t, contactEvidence, routed.Operations)
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want no missing connector endpoint geometry", diagnostics)
	}
}

func connectionAliasSet(connections []ConnectionSpec) map[string]bool {
	nets := map[string]bool{}
	for _, connection := range connections {
		if connection.NetAlias != "" {
			nets[connection.NetAlias] = true
		}
	}
	return nets
}

func workflowStageForRoutingTest(result WorkflowResult, name StageName) (StageResult, bool) {
	for _, stage := range result.Stages {
		if stage.Name == name {
			return stage, true
		}
	}
	return StageResult{}, false
}

func promotionGateByIDForRoutingTest(report PromotionReport, id string) (PromotionGate, bool) {
	for _, gate := range report.Gates {
		if gate.ID == id {
			return gate, true
		}
	}
	return PromotionGate{}, false
}

func issueByCodeAndPathForRoutingTest(issues []reports.Issue, code reports.Code, path string) (reports.Issue, bool) {
	for _, issue := range issues {
		if issue.Code == code && issue.Path == path {
			return issue, true
		}
	}
	return reports.Issue{}, false
}

func promotionIssueCodeByStageAndPathForRoutingTest(issues []PromotionIssue, stage StageName, path string) (string, bool) {
	for _, issue := range issues {
		if issue.Stage == stage && issue.Path == path {
			return issue.Code, true
		}
	}
	return "", false
}

func stageHasIssueCodeForRoutingTest(stage StageResult, code reports.Code) bool {
	for _, issue := range stage.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func i2cSensorBreakoutPromotionFixtureForRoutingTest() PromotionFixture {
	return PromotionFixture{
		ID:                "i2c_sensor_breakout_candidate",
		Request:           "i2c_sensor_breakout_candidate.json",
		Tier:              "block-composition",
		DeclaredReadiness: PromotionReadinessExpectedFail,
		Acceptance:        AcceptanceERCDRC,
		RequireERC:        true,
		RequireDRC:        true,
		ExpectedStages: []StageName{
			StageBlockPlanning,
			StageComponentSelection,
			StageSchematic,
			StageSchematicElectrical,
			StagePCBRealization,
			StagePlacement,
			StageRouting,
			StageProjectWrite,
			StageWriterCorrect,
			StageValidation,
			StageKiCadChecks,
		},
	}
}

func i2cSensorBreakoutRoutingFixture(t *testing.T, ctx context.Context) (Request, PCBFragmentResult, PlacementStageResult) {
	t.Helper()
	request := Request{
		Version: RequestVersion,
		Name:    "i2c_sensor_breakout_candidate",
		Board:   BoardSpec{WidthMM: 55, HeightMM: 35, Layers: 2},
		Blocks: []BlockInstanceSpec{
			{ID: "sensor", BlockID: "i2c_sensor", Params: map[string]any{"i2c_address": "0x48", "include_pullups": true, "supply_voltage": "3.3V", "pullup_value": "4k7"}},
			{ID: "io", BlockID: "connector_breakout", Params: map[string]any{"pin_count": 4, "pin_names": []string{"VCC", "GND", "SDA", "SCL"}}},
		},
		Connections: []ConnectionSpec{
			{From: "sensor.VCC", To: "io.VCC", NetAlias: "VCC"},
			{From: "sensor.GND", To: "io.GND", NetAlias: "GND"},
			{From: "sensor.SDA", To: "io.SDA", NetAlias: "SDA"},
			{From: "sensor.SCL", To: "io.SCL", NetAlias: "SCL"},
		},
		RoutingRetry: RoutingRetryPolicySpec{Enabled: true},
	}
	registry := blocks.NewBuiltinRegistry()
	plan := PlanBlocks(ctx, registry, request)
	if !stageUsableForRoutingTest(plan.Stage) {
		t.Fatalf("planning failed: %#v", plan.Stage.Issues)
	}
	fragments := RealizePCBFragments(ctx, registry, plan)
	if !stageUsableForRoutingTest(fragments.Stage) {
		t.Fatalf("PCB realization failed: %#v", fragments.Stage.Issues)
	}
	placed := PlaceFragments(ctx, request, fragments, PlacementOptions{})
	if !stageUsableForRoutingTest(placed.Stage) {
		t.Fatalf("placement failed: %#v", placed.Stage.Issues)
	}
	return request, fragments, placed
}

func issueNetSet(issues []reports.Issue) map[string]bool {
	nets := map[string]bool{}
	for _, issue := range issues {
		for _, net := range issue.Nets {
			if net != "" {
				nets[net] = true
			}
		}
	}
	return nets
}

func routeTreeBranchIssuePathsByNet(issues []reports.Issue) map[string][]string {
	pathSets := map[string]map[string]struct{}{}
	for _, issue := range issues {
		if issue.Severity != reports.SeverityBlocked && issue.Severity != reports.SeverityError {
			continue
		}
		if !strings.Contains(issue.Path, "design.inter_block_route_groups") || !strings.Contains(issue.Path, ".branches[") {
			continue
		}
		for _, net := range issue.Nets {
			if net != "" {
				if pathSets[net] == nil {
					pathSets[net] = map[string]struct{}{}
				}
				pathSets[net][issue.Path] = struct{}{}
			}
		}
	}
	paths := map[string][]string{}
	for net, set := range pathSets {
		for path := range set {
			paths[net] = append(paths[net], path)
		}
		sort.Strings(paths[net])
	}
	return paths
}

type routeTreeContactGraphGapForRoutingTest struct {
	Required           int
	Proven             int
	Components         int
	MissingEndpointIDs []string
}

type routeTreeMissingEndpointGeometryDetailForRoutingTest struct {
	EndpointID         string
	Ref                string
	Pad                string
	InstanceID         string
	BlockID            string
	Layer              string
	XMM                float64
	YMM                float64
	ToleranceMM        float64
	NearestOperationID string
	NearestLayer       string
	NearestXMM         float64
	NearestYMM         float64
	NearestDistanceMM  float64
}

func routeTreeContactGraphGapsForRoutingTest(t *testing.T, evidence InterBlockContactEvidence, operations []transactions.Operation) map[string]routeTreeContactGraphGapForRoutingTest {
	t.Helper()
	targetsByNet := interBlockContactTargetsByNet(evidence.Targets)
	operationsByNet, operationIssues := decodeInterBlockRouteOperations(operations)
	if len(operationIssues) != 0 {
		t.Fatalf("operation decode issues = %#v", operationIssues)
	}
	componentCounts := interBlockGraphComponentCountsFromDecoded(targetsByNet, operationsByNet, operationIssues)
	gaps := map[string]routeTreeContactGraphGapForRoutingTest{}
	for netName, targets := range targetsByNet {
		graph := newInterBlockContactGraph(operationsByNet[netName])
		gap := routeTreeContactGraphGapForRoutingTest{Required: len(targets), Components: componentCounts[netName]}
		for _, target := range targets {
			if _, ok := graph.findTargetNode(target); ok {
				gap.Proven++
				continue
			}
			gap.MissingEndpointIDs = append(gap.MissingEndpointIDs, routeTreeContactGraphTargetStableIDForRoutingTest(target))
		}
		sort.Strings(gap.MissingEndpointIDs)
		if gap.Proven != gap.Required {
			gaps[netName] = gap
		}
	}
	return gaps
}

func routeTreeMissingEndpointGeometryForRoutingTest(t *testing.T, evidence InterBlockContactEvidence, operations []transactions.Operation) map[string][]routeTreeMissingEndpointGeometryDetailForRoutingTest {
	t.Helper()
	targetsByNet := interBlockContactTargetsByNet(evidence.Targets)
	operationsByNet, operationIssues := decodeInterBlockRouteOperations(operations)
	if len(operationIssues) != 0 {
		t.Fatalf("operation decode issues = %#v", operationIssues)
	}
	diagnostics := map[string][]routeTreeMissingEndpointGeometryDetailForRoutingTest{}
	for netName, targets := range targetsByNet {
		graph := newInterBlockContactGraph(operationsByNet[netName])
		for _, target := range targets {
			if _, ok := graph.findTargetNode(target); ok {
				continue
			}
			nearest := nearestSameNetCopperForRoutingTest(target, operationsByNet[netName])
			diagnostic := routeTreeMissingEndpointGeometryDetailForRoutingTest{
				EndpointID:         routeTreeContactGraphTargetStableIDForRoutingTest(target),
				Ref:                target.Ref,
				Pad:                target.Pad,
				InstanceID:         target.InstanceID,
				BlockID:            target.BlockID,
				Layer:              target.Layer,
				XMM:                target.Point.XMM,
				YMM:                target.Point.YMM,
				ToleranceMM:        contactToleranceForTarget(target),
				NearestOperationID: nearest.OperationID,
				NearestLayer:       nearest.Layer,
				NearestXMM:         nearest.Point.XMM,
				NearestYMM:         nearest.Point.YMM,
				NearestDistanceMM:  nearest.DistanceMM,
			}
			diagnostics[netName] = append(diagnostics[netName], diagnostic)
		}
		slices.SortFunc(diagnostics[netName], func(left routeTreeMissingEndpointGeometryDetailForRoutingTest, right routeTreeMissingEndpointGeometryDetailForRoutingTest) int {
			return strings.Compare(left.EndpointID, right.EndpointID)
		})
	}
	return diagnostics
}

type nearestSameNetCopperForRoutingTestResult struct {
	OperationID string
	Layer       string
	Point       transactions.Point
	DistanceMM  float64
}

func nearestSameNetCopperForRoutingTest(target InterBlockContactTarget, operations []decodedContactRouteOperation) nearestSameNetCopperForRoutingTestResult {
	best := nearestSameNetCopperForRoutingTestResult{DistanceMM: math.Inf(1)}
	for _, operation := range operations {
		if len(operation.Points) == 1 {
			distance := pointDistanceMM(target.Point, operation.Points[0])
			if distance < best.DistanceMM {
				best = nearestSameNetCopperForRoutingTestResult{OperationID: operation.OperationID, Layer: operation.Layer, Point: operation.Points[0], DistanceMM: distance}
			}
			continue
		}
		for index := 1; index < len(operation.Points); index++ {
			closestPoint := closestPointOnSegment(target.Point, operation.Points[index-1], operation.Points[index])
			distance := pointDistanceMM(target.Point, closestPoint)
			if distance < best.DistanceMM {
				best = nearestSameNetCopperForRoutingTestResult{OperationID: operation.OperationID, Layer: operation.Layer, Point: closestPoint, DistanceMM: distance}
			}
		}
	}
	return best
}

func routeTreeContactGraphTargetStableIDForRoutingTest(target InterBlockContactTarget) string {
	if target.InstanceID != "" && target.Pad != "" {
		return target.InstanceID + "." + target.Pad
	}
	if target.EndpointID != "" {
		return target.EndpointID
	}
	return interBlockEndpointKey(target.Ref, target.Pad)
}

// requireRouteOperationsForNet decodes every transaction route operation for a
// net so tests can inspect multi-segment routing evidence without relying on
// the first matching operation.
func requireRouteOperationsForNet(t *testing.T, operations []transactions.Operation, name string) []transactions.RouteOperation {
	t.Helper()
	var routes []transactions.RouteOperation
	for _, operation := range operations {
		if operation.Op != transactions.OpRoute || operation.Net != name {
			continue
		}
		var route transactions.RouteOperation
		if err := json.Unmarshal(operation.Raw, &route); err != nil {
			t.Fatalf("route operation raw = %s: %v", operation.Raw, err)
		}
		routes = append(routes, route)
	}
	if len(routes) == 0 {
		t.Fatalf("route operation nets = %#v, want route operation for net %s", routeOperationNets(operations), name)
	}
	return routes
}

// routeOperationNets returns the net names carried by route transactions for
// compact failure output.
func routeOperationNets(operations []transactions.Operation) []string {
	var nets []string
	for _, operation := range operations {
		if operation.Op == transactions.OpRoute {
			nets = append(nets, operation.Net)
		}
	}
	return nets
}

func routeViaCountForRoutingTest(t *testing.T, operations []transactions.Operation, name string) int {
	t.Helper()
	count := 0
	for _, route := range requireRouteOperationsForNet(t, operations, name) {
		count += len(route.Vias)
	}
	return count
}

func pointsNearlyEqual(left transactions.Point, right transactions.Point) bool {
	const tolerance = 1e-6
	return math.Abs(left.XMM-right.XMM) <= tolerance && math.Abs(left.YMM-right.YMM) <= tolerance
}

func TestDedupeSameNetRouteViasDropsDuplicateViaLocations(t *testing.T) {
	first := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "VBUS",
		Layer:   "B.Cu",
		Points:  []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 1, YMM: 0}},
		Vias: []transactions.RouteViaSpec{{
			At:         transactions.Point{XMM: 1, YMM: 0},
			DiameterMM: 0.6,
			DrillMM:    0.3,
			Layers:     []string{"F.Cu", "B.Cu"},
		}},
	})
	second := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "VBUS",
		Layer:   "B.Cu",
		Points:  []transactions.Point{{XMM: 2, YMM: 0}, {XMM: 1, YMM: 0}},
		Vias: []transactions.RouteViaSpec{{
			At:         transactions.Point{XMM: 1, YMM: 0},
			DiameterMM: 0.6,
			DrillMM:    0.3,
			Layers:     []string{"F.Cu", "B.Cu"},
		}},
	})

	operations := dedupeSameNetRouteVias([]transactions.Operation{first, second})

	if got := routeViaCountForRoutingTest(t, operations, "VBUS"); got != 1 {
		t.Fatalf("VBUS via count = %d, want duplicate same-net via collapsed", got)
	}
}

func TestDedupeSameNetRouteViasSnapsTooCloseViaToExistingVia(t *testing.T) {
	first := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "GND",
		Layer:   "B.Cu",
		Points:  []transactions.Point{{XMM: 30, YMM: 20}, {XMM: 23.1, YMM: 21}},
		Vias: []transactions.RouteViaSpec{{
			At:         transactions.Point{XMM: 23.1, YMM: 21},
			DiameterMM: 0.6,
			DrillMM:    0.3,
			Layers:     []string{"F.Cu", "B.Cu"},
		}},
	})
	second := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "GND",
		Layer:   "B.Cu",
		Points:  []transactions.Point{{XMM: 23, YMM: 20.5}, {XMM: 13, YMM: 20.5}},
		Vias: []transactions.RouteViaSpec{{
			At:         transactions.Point{XMM: 23, YMM: 20.5},
			DiameterMM: 0.7,
			DrillMM:    0.35,
			Layers:     []string{"F.Cu", "B.Cu"},
		}},
	})

	operations := dedupeSameNetRouteVias([]transactions.Operation{first, second})
	routes := requireRouteOperationsForNet(t, operations, "GND")

	if got := routeViaCountForRoutingTest(t, operations, "GND"); got != 1 {
		t.Fatalf("GND via count = %d, want close same-net via merged", got)
	}
	if !pointsNearlyEqual(routes[1].Points[0], transactions.Point{XMM: 23.1, YMM: 21}) {
		t.Fatalf("second route start = %#v, want snapped to existing same-net via", routes[1].Points[0])
	}
}

func TestDedupeSameNetRouteViasKeepsDistinctLayerSpans(t *testing.T) {
	first := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "GND",
		Layer:   "B.Cu",
		Points:  []transactions.Point{{XMM: 0, YMM: 0}, {XMM: 1, YMM: 0}},
		Vias: []transactions.RouteViaSpec{{
			At:         transactions.Point{XMM: 1, YMM: 0},
			DiameterMM: 0.6,
			DrillMM:    0.3,
			Layers:     []string{"F.Cu", "B.Cu"},
		}},
	})
	second := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "GND",
		Layer:   "In1.Cu",
		Points:  []transactions.Point{{XMM: 2, YMM: 0}, {XMM: 1, YMM: 0}},
		Vias: []transactions.RouteViaSpec{{
			At:         transactions.Point{XMM: 1, YMM: 0},
			DiameterMM: 0.6,
			DrillMM:    0.3,
			Layers:     []string{"In1.Cu", "B.Cu"},
		}},
	})

	operations := dedupeSameNetRouteVias([]transactions.Operation{first, second})

	if got := routeViaCountForRoutingTest(t, operations, "GND"); got != 2 {
		t.Fatalf("GND via count = %d, want distinct layer spans preserved", got)
	}
}

func TestDedupeSameNetRouteViasSnapsSameLayerEndpointWithoutVia(t *testing.T) {
	first := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "GND",
		Layer:   "B.Cu",
		Points:  []transactions.Point{{XMM: 30, YMM: 20}, {XMM: 23.1, YMM: 21}},
		Vias: []transactions.RouteViaSpec{{
			At:         transactions.Point{XMM: 23.1, YMM: 21},
			DiameterMM: 0.6,
			DrillMM:    0.3,
			Layers:     []string{"F.Cu", "B.Cu"},
		}},
	})
	second := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "GND",
		Layer:   "B.Cu",
		Points:  []transactions.Point{{XMM: 23, YMM: 20.5}, {XMM: 13, YMM: 20.5}},
		Vias: []transactions.RouteViaSpec{{
			At:         transactions.Point{XMM: 23, YMM: 20.5},
			DiameterMM: 0.7,
			DrillMM:    0.35,
			Layers:     []string{"F.Cu", "B.Cu"},
		}},
	})
	third := mustRouteOperation(t, transactions.RouteOperation{
		Op:      transactions.OpRoute,
		NetName: "GND",
		Layer:   "B.Cu",
		Points:  []transactions.Point{{XMM: 23, YMM: 20.5}, {XMM: 10, YMM: 20.5}},
	})

	operations := dedupeSameNetRouteVias([]transactions.Operation{first, second, third})
	routes := requireRouteOperationsForNet(t, operations, "GND")

	if got := routeViaCountForRoutingTest(t, operations, "GND"); got != 1 {
		t.Fatalf("GND via count = %d, want close same-net via merged", got)
	}
	if !pointsNearlyEqual(routes[2].Points[0], transactions.Point{XMM: 23.1, YMM: 21}) {
		t.Fatalf("third route start = %#v, want snapped to retained same-layer via", routes[2].Points[0])
	}
}

func mustRouteOperation(t *testing.T, payload transactions.RouteOperation) transactions.Operation {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return transactions.NewOperation(transactions.OpRoute, raw)
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

func TestInterBlockRouteCompletionSummaryJSONStable(t *testing.T) {
	summary := InterBlockRouteCompletionSummary{
		NetsConsidered:      1,
		Candidates:          1,
		RoutesAttempted:     1,
		RoutesCompleted:     0,
		EndpointsResolved:   2,
		EndpointsUnresolved: 0,
		PartialNets:         1,
		UnroutedNets:        0,
		EmittedSegments:     1,
		IssueCount:          1,
		MultiEndpointNets:   1,
		RequiredEndpoints:   3,
		ProvenEndpoints:     2,
		BranchesPlanned:     2,
		BranchesAttempted:   2,
		BranchesCompleted:   1,
		GraphComponentCount: 2,
		MissingRequired:     1,
		CompleteGroups:      0,
		PartialGroups:       1,
		BlockedGroups:       0,
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"nets_considered":1,"candidates":1,"routes_attempted":1,"routes_completed":0,"endpoints_resolved":2,"endpoints_unresolved":0,"partial_nets":1,"unrouted_nets":0,"emitted_segments":1,"issue_count":1,"multi_endpoint_nets":1,"required_endpoints":3,"proven_endpoints":2,"branches_planned":2,"branches_attempted":2,"branches_completed":1,"graph_component_count":2,"missing_required_endpoints":1,"complete_groups":0,"partial_groups":1,"blocked_groups":0}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func TestInterBlockRouteTreeExecutionSummaryJSONStable(t *testing.T) {
	summary := InterBlockRouteTreeExecutionSummary{
		GroupsPlanned:     1,
		GroupsAttempted:   1,
		GroupsComplete:    0,
		GroupsPartial:     1,
		GroupsBlocked:     0,
		BranchesPlanned:   2,
		BranchesAttempted: 2,
		BranchesRouted:    1,
		BranchesBlocked:   1,
		ContactMisses:     1,
		GraphSplits:       1,
		IssueCount:        1,
		ManagedNets:       []string{"SDA"},
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"groups_planned":1,"groups_attempted":1,"groups_complete":0,"groups_partial":1,"groups_blocked":0,"branches_planned":2,"branches_attempted":2,"branches_routed":1,"branches_blocked":1,"contact_misses":1,"graph_splits":1,"issue_count":1,"managed_nets":["SDA"]}`
	if string(data) != want {
		t.Fatalf("summary JSON = %q, want %q", data, want)
	}
}

func routingRequestHasNet(request routing.Request, name string) bool {
	for _, net := range request.Nets {
		if net.Name == name {
			return true
		}
	}
	return false
}

func routeOperationsContainNet(operations []transactions.Operation, name string) bool {
	for _, operation := range operations {
		if operation.Op == transactions.OpRoute && operation.Net == name {
			return true
		}
	}
	return false
}

func interBlockCandidateByNetForRoutingTest(candidates []InterBlockRouteCandidate, name string) (InterBlockRouteCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.NetName == name {
			return candidate, true
		}
	}
	return InterBlockRouteCandidate{}, false
}

func interBlockCandidateNetSetForRoutingTest(candidates []InterBlockRouteCandidate) map[string]bool {
	nets := map[string]bool{}
	for _, candidate := range candidates {
		if candidate.NetName != "" {
			nets[candidate.NetName] = true
		}
	}
	return nets
}

func interBlockCandidateEndpointCount(candidates []InterBlockRouteCandidate) int {
	count := 0
	for _, candidate := range candidates {
		count += len(candidate.Endpoints)
	}
	return count
}

func assertNetHasIssueCode(t *testing.T, issues []reports.Issue, net string, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code != code {
			continue
		}
		for _, issueNet := range issue.Nets {
			if issueNet == net {
				return
			}
		}
	}
	t.Fatalf("issues = %#v, want issue code %s for net %s", issues, code, net)
}

func assertNoIssueCode(t *testing.T, issues []reports.Issue, code reports.Code) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			t.Fatalf("issues = %#v, did not want issue code %s", issues, code)
		}
	}
}

func stageUsableForRoutingTest(stage StageResult) bool {
	return stage.Status == StageStatusOK || stage.Status == StageStatusWarning
}

func routeTreeIssuesForNet(issues []reports.Issue, netName string) []reports.Issue {
	netName = strings.TrimSpace(netName)
	out := []reports.Issue{}
	for _, issue := range issues {
		if !strings.Contains(issue.Path, "route_tree") && !strings.Contains(issue.Path, "inter_block_route_groups") && !strings.Contains(issue.Path, "inter_block_contact") {
			continue
		}
		for _, issueNet := range issue.Nets {
			if strings.EqualFold(strings.TrimSpace(issueNet), netName) {
				out = append(out, issue)
				break
			}
		}
	}
	return out
}

func routeTreeIssuesContainCode(issues []reports.Issue, code reports.Code) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func routeTreeIssuesContainMessage(issues []reports.Issue, text string) bool {
	lowerText := strings.ToLower(text)
	for _, issue := range issues {
		if strings.Contains(strings.ToLower(issue.Message), lowerText) {
			return true
		}
	}
	return false
}

func requireInterBlockRouteSummary(t *testing.T, stage StageResult) InterBlockRouteCompletionSummary {
	t.Helper()
	return requireStageSummary[InterBlockRouteCompletionSummary](t, stage, "inter_block_routing")
}

func requireInterBlockContactSummary(t *testing.T, stage StageResult) InterBlockContactSummary {
	t.Helper()
	return requireStageSummary[InterBlockContactSummary](t, stage, "inter_block_contacts")
}

func requireInterBlockRouteTreeExecutionSummary(t *testing.T, stage StageResult) InterBlockRouteTreeExecutionSummary {
	t.Helper()
	return requireStageSummary[InterBlockRouteTreeExecutionSummary](t, stage, "inter_block_route_trees")
}

func requireRouteTreeRepairSummary(t *testing.T, stage StageResult) InterBlockRouteTreeRepairSummary {
	t.Helper()
	return requireStageSummary[InterBlockRouteTreeRepairSummary](t, stage, "route_tree_repair")
}

func requireRouteTreeBranchesForNet(t *testing.T, stage StageResult, netName string) []InterBlockBranchRoutingEvidence {
	t.Helper()
	summaries := requireStageSummary[[]RouteTreeBranchEvidenceSummary](t, stage, "route_tree_branches")
	var branches []InterBlockBranchRoutingEvidence
	for _, summary := range summaries {
		if strings.EqualFold(summary.NetName, netName) {
			branches = append(branches, summary.Branches...)
		}
	}
	if len(branches) != 0 {
		return branches
	}
	t.Fatalf("route_tree_branches = %#v, want net %s", summaries, netName)
	return nil
}

func requireStageSummary[T any](t *testing.T, stage StageResult, key string) T {
	t.Helper()
	var zero T
	value, exists := stage.Summary[key]
	if !exists {
		t.Fatalf("missing %s summary: %#v", key, stage.Summary)
	}
	summary, ok := value.(T)
	if !ok {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("%s summary has type %T and could not be marshaled: %v", key, value, err)
		}
		if err := json.Unmarshal(data, &summary); err != nil {
			t.Fatalf("%s summary has type %T and could not be decoded into %T: %v", key, value, zero, err)
		}
	}
	return summary
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

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
