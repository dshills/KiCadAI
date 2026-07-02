package designworkflow

import (
	"context"
	"math"
	"strings"
	"testing"

	"kicadai/internal/reports"
	"kicadai/internal/routing"
	"kicadai/internal/transactions"
)

func TestRouteInterBlockTreeBranchesRoutesThreeEndpointTree(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "R1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"R1.1": {XMM: 15, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"R1.1": {XMM: 15, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})

	result := RouteInterBlockTreeBranches(context.Background(), base, group, tree)
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if len(result.Branches) != 2 {
		t.Fatalf("branches = %#v, want two branch attempts", result.Branches)
	}
	if len(result.Operations) != 2 {
		t.Fatalf("operations = %#v, want one operation per branch", result.Operations)
	}
	if len(result.ExistingCopper) == 0 {
		t.Fatalf("existing copper = %#v, want successful branches to feed same-net copper forward", result.ExistingCopper)
	}
	for _, branch := range result.Branches {
		if branch.Status != routing.StatusRouted || branch.OperationCount == 0 || branch.IssueCount != 0 {
			t.Fatalf("branch = %#v, want routed clean branch evidence", branch)
		}
	}
}

func TestRouteInterBlockTreeBranchesReportsMissingGroupEndpoint(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := InterBlockRouteTree{
		NetName:        "SIG",
		RootEndpointID: "J1.1",
		Branches: []InterBlockRouteTreeBranch{{
			Index:           0,
			StartEndpointID: "J1.1",
			EndEndpointID:   "MISSING.1",
		}},
	}
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})

	result := RouteInterBlockTreeBranches(context.Background(), base, group, tree)
	if len(result.Issues) != 1 {
		t.Fatalf("issues = %#v, want missing endpoint issue", result.Issues)
	}
	if result.Issues[0].Code != reports.CodeValidationFailed || result.Issues[0].Severity != reports.SeverityBlocked {
		t.Fatalf("issue = %#v, want blocked validation issue", result.Issues[0])
	}
	if len(result.Branches) != 1 || result.Branches[0].Status != routing.StatusBlocked {
		t.Fatalf("branches = %#v, want blocked branch evidence", result.Branches)
	}
}

func TestRouteInterBlockTreeBranchesReportsAllBranchesOnCanceledContext(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "R1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"R1.1": {XMM: 15, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 5, YMM: 5},
		"R1.1": {XMM: 15, YMM: 5},
		"U1.1": {XMM: 25, YMM: 5},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := RouteInterBlockTreeBranches(ctx, base, group, tree)

	if len(result.Branches) != len(tree.Branches) {
		t.Fatalf("branches = %d, want %d", len(result.Branches), len(tree.Branches))
	}
	for _, branch := range result.Branches {
		if branch.Status != routing.StatusBlocked {
			t.Fatalf("branch %d status = %s, want blocked", branch.BranchIndex, branch.Status)
		}
	}
	if len(result.Issues) != len(tree.Branches) {
		t.Fatalf("issues = %d, want %d", len(result.Issues), len(tree.Branches))
	}
}

func TestRouteInterBlockTreeBranchesScopesRoutingIssuesToBranch(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 8, YMM: 2},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 8, YMM: 2},
	})
	base.Obstacles = append(base.Obstacles, routing.Obstacle{
		Kind:  routing.ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: 0, YMM: 0},
			Max: routing.Point{XMM: 10, YMM: 4},
		}},
	})

	result := RouteInterBlockTreeBranches(context.Background(), base, group, tree)
	if len(result.Issues) == 0 {
		t.Fatalf("issues = %#v, want branch-scoped pathfinding issue", result.Issues)
	}
	for _, issue := range result.Issues {
		if !strings.Contains(issue.Path, `design.inter_block_route_groups["SIG"].branches[0]`) {
			t.Fatalf("issue path = %q, want branch-scoped route-tree path", issue.Path)
		}
		if !strings.Contains(issue.Suggestion, "route-tree branch") {
			t.Fatalf("issue suggestion = %q, want route-tree repair context", issue.Suggestion)
		}
	}
}

func TestRouteTreeBranchesForRoutingOrdersShortConstrainedBranchesFirst(t *testing.T) {
	branches := routeTreeBranchesForRouting([]InterBlockRouteTreeBranch{
		{Index: 3, StartEndpointID: "J1.1", EndEndpointID: "U3.1", PlannedDistanceMM: 30},
		{Index: 1, StartEndpointID: "J1.1", EndEndpointID: "U1.1", PlannedDistanceMM: 5},
		{Index: 2, StartEndpointID: "J1.1", EndEndpointID: "U2.1", PlannedDistanceMM: 5},
		{Index: 0, StartEndpointID: "J1.1", EndEndpointID: "U0.1"},
	})
	got := []int{}
	for _, branch := range branches {
		got = append(got, branch.Index)
	}
	want := []int{0, 1, 2, 3}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("branch order = %v, want %v", got, want)
		}
	}
}

func TestRouteInterBlockTreeBranchesDoesNotEmitCopperForFailedBranch(t *testing.T) {
	group := routeTreeTestGroup("SIG",
		InterBlockRouteEndpoint{Ref: "J1", Pin: "1"},
		InterBlockRouteEndpoint{Ref: "U1", Pin: "1"},
	)
	tree := BuildInterBlockRouteTree(group, routeTreeTestTargets("SIG", map[string]transactions.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 8, YMM: 2},
	}))
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 8, YMM: 2},
	})
	base.Obstacles = append(base.Obstacles, routing.Obstacle{
		Kind:  routing.ObstacleKeepout,
		Layer: "F.Cu",
		Geometry: routing.Shape{Rect: &routing.Rect{
			Min: routing.Point{XMM: 0, YMM: 0},
			Max: routing.Point{XMM: 10, YMM: 4},
		}},
	})

	result := RouteInterBlockTreeBranches(context.Background(), base, group, tree)
	if len(result.Issues) == 0 {
		t.Fatalf("issues = %#v, want failed branch issue", result.Issues)
	}
	if len(result.Operations) != 0 || len(result.ExistingCopper) != 0 {
		t.Fatalf("operations=%#v existing=%#v, want no failed partial copper", result.Operations, result.ExistingCopper)
	}
}

func TestInterBlockRouteGroupEndpointsByIDKeepsRequiredOnDuplicate(t *testing.T) {
	group := InterBlockRouteGroup{
		RequiredEndpoints: []InterBlockRouteGroupEndpoint{{ID: "U1.1", Ref: "U1", Pin: "1"}},
		OptionalEndpoints: []InterBlockRouteGroupEndpoint{{ID: "U1.1", Ref: "ALT", Pin: "9"}},
	}
	endpoints := interBlockRouteGroupEndpointsByID(group)
	if endpoints["U1.1"].Ref != "U1" || endpoints["U1.1"].Pin != "1" {
		t.Fatalf("duplicate endpoint = %#v, want required endpoint", endpoints["U1.1"])
	}
}

func TestRouteBranchSegmentShapeUsesWidthAwarePolygonForDiagonal(t *testing.T) {
	shape := routeBranchSegmentShape(routing.Segment{
		Start:   routing.Point{XMM: 1, YMM: 1},
		End:     routing.Point{XMM: 3, YMM: 3},
		WidthMM: 0.4,
	}, routing.DefaultRules())
	if shape.Rect != nil {
		t.Fatalf("diagonal shape used rect = %#v, want polygon", shape.Rect)
	}
	if len(shape.Polygon) != 4 {
		t.Fatalf("polygon points = %d, want 4", len(shape.Polygon))
	}
}

func TestRouteBranchSegmentShapeUsesSquareCapsForHorizontal(t *testing.T) {
	shape := routeBranchSegmentShape(routing.Segment{
		Start:   routing.Point{XMM: 1, YMM: 2},
		End:     routing.Point{XMM: 3, YMM: 2},
		WidthMM: 0.4,
	}, routing.DefaultRules())
	if shape.Rect == nil {
		t.Fatalf("horizontal shape used polygon, want rect")
	}
	if math.Abs(shape.Rect.Min.XMM-0.8) > 1e-9 || math.Abs(shape.Rect.Max.XMM-3.2) > 1e-9 {
		t.Fatalf("horizontal caps extended to [%f,%f], want [0.8,3.2]", shape.Rect.Min.XMM, shape.Rect.Max.XMM)
	}
	if math.Abs(shape.Rect.Min.YMM-1.8) > 1e-9 || math.Abs(shape.Rect.Max.YMM-2.2) > 1e-9 {
		t.Fatalf("horizontal width bounds = [%f,%f], want [1.8,2.2]", shape.Rect.Min.YMM, shape.Rect.Max.YMM)
	}
}

func TestRouteBranchViaShapeUsesPolygon(t *testing.T) {
	shape := routeBranchViaShape(routing.Via{
		At:         routing.Point{XMM: 10, YMM: 10},
		DiameterMM: 0.8,
	}, routing.DefaultRules())
	if shape.Rect != nil {
		t.Fatalf("via shape used rect = %#v, want polygon", shape.Rect)
	}
	if len(shape.Polygon) != 8 {
		t.Fatalf("via polygon points = %d, want 8", len(shape.Polygon))
	}
}

func routeBranchTestRequest(netName string, pads map[string]routing.Point) routing.Request {
	request := routing.Request{
		Board: routing.Board{
			WidthMM:  40,
			HeightMM: 20,
			Layers:   []routing.Layer{{Name: "F.Cu", Kind: routing.LayerCopper, Routable: true}},
		},
		Rules:    routing.DefaultRules(),
		Strategy: routing.Strategy{Mode: routing.ModeSingleLayer},
	}
	for id, point := range pads {
		ref, pin, ok := splitRouteTreeEndpointID(id)
		if !ok {
			panic("invalid endpoint ID")
		}
		request.Components = append(request.Components, routing.Component{
			Ref:      ref,
			Position: routing.Placement{Layer: "F.Cu"},
			Pads: []routing.Pad{{
				Ref:      ref,
				Name:     pin,
				Net:      netName,
				Position: point,
				Shape:    routing.PadRect,
				Type:     routing.PadSMD,
				Size:     routing.Size{WidthMM: 1, HeightMM: 1},
				Layers:   []string{"F.Cu"},
			}},
		})
	}
	return request
}
