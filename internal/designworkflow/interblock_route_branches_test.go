package designworkflow

import (
	"context"
	"encoding/json"
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

func TestRouteTreeAccessBranchRequestRoutesSyntheticAccessPoints(t *testing.T) {
	base := routeBranchTestRequest("SIG", map[string]routing.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 18, YMM: 2},
	})
	base.Rules.GridMM = 0.5
	base.Rules.NetClasses = map[string]routing.NetClass{"audio": {TraceWidthMM: 0.25, ClearanceMM: 0.2}}
	base.Nets = []routing.Net{
		{Name: "SIG", Class: "audio", Role: routing.NetAnalog, Priority: 7, Fixed: true},
		{Name: "GND", Role: routing.NetGround},
	}
	pair := routeTreeBranchAccessPair{
		Rank: 3,
		Source: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{
			Role:  RouteTreeAccessLocalRouteAnchor,
			Net:   "SIG",
			Layer: "F.Cu",
			XMM:   5,
			YMM:   5,
		}},
		Target: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{
			Role:  RouteTreeAccessTargetPad,
			Ref:   "U1",
			Pad:   "1",
			Net:   "SIG",
			Layer: "F.Cu",
			XMM:   15,
			YMM:   5,
		}},
	}

	request := routeTreeAccessBranchRequest(&base, "SIG", pair)
	if len(base.Components) != 2 {
		t.Fatalf("base components = %d, want unmodified base request", len(base.Components))
	}
	if len(request.Components) != 4 {
		t.Fatalf("components = %d, want base plus synthetic access components", len(request.Components))
	}
	if request.Nets[0].Endpoints[0].Ref != "__KICADAI_RT_SRC_3" || request.Nets[0].Endpoints[1].Ref != "__KICADAI_RT_DST_3" {
		t.Fatalf("endpoints = %#v, want synthetic access refs", request.Nets[0].Endpoints)
	}
	if request.Nets[0].Class != "audio" || request.Nets[0].Role != routing.NetAnalog || request.Nets[0].Priority != 7 {
		t.Fatalf("branch net = %#v, want preserved metadata from base net", request.Nets[0])
	}
	if request.Nets[0].Fixed {
		t.Fatalf("branch net = %#v, want selected branch net to be routable", request.Nets[0])
	}
	if len(request.Nets) != 2 || request.Nets[1].Name != "GND" || !request.Nets[1].Fixed {
		t.Fatalf("nets = %#v, want other net metadata preserved as fixed", request.Nets)
	}

	result := routing.RouteRequestContext(context.Background(), request)
	if result.Status != routing.StatusRouted || len(result.Operations) != 1 {
		t.Fatalf("result = %#v, want routed synthetic access branch", result)
	}
	operations := transactionRouteOperations(result.Operations)
	if len(operations) != 1 {
		t.Fatalf("operations = %#v, want one transaction route", operations)
	}
	var route transactions.RouteOperation
	if err := json.Unmarshal(operations[0].Raw, &route); err != nil {
		t.Fatal(err)
	}
	if len(route.Points) < 2 {
		t.Fatalf("points = %#v, want routed access path", route.Points)
	}
	first := route.Points[0]
	last := route.Points[len(route.Points)-1]
	forward := math.Abs(first.XMM-5) <= 1e-9 && math.Abs(first.YMM-5) <= 1e-9 && math.Abs(last.XMM-15) <= 1e-9 && math.Abs(last.YMM-5) <= 1e-9
	reverse := math.Abs(first.XMM-15) <= 1e-9 && math.Abs(first.YMM-5) <= 1e-9 && math.Abs(last.XMM-5) <= 1e-9 && math.Abs(last.YMM-5) <= 1e-9
	if !forward && !reverse {
		t.Fatalf("route points = %#v, want route snapped to selected access coordinates", route.Points)
	}
}

func TestRouteTreeAccessBranchRequestUsesMatchedNetNameForSyntheticPads(t *testing.T) {
	base := routeBranchTestRequest("sig", map[string]routing.Point{
		"J1.1": {XMM: 2, YMM: 2},
		"U1.1": {XMM: 18, YMM: 2},
	})
	base.Nets = []routing.Net{{Name: "sig", Class: "signal"}, {Name: "SIG", Class: "duplicate"}}
	pair := routeTreeBranchAccessPair{
		Source: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{Net: "SIG", Layer: "F.Cu", XMM: 5, YMM: 5}},
		Target: routeTreeBranchAccessCandidate{Access: RouteTreeEndpointAccess{Net: "SIG", Layer: "F.Cu", XMM: 15, YMM: 5}},
	}

	request := routeTreeAccessBranchRequest(&base, "SIG", pair)
	if request.Nets[0].Name != "sig" {
		t.Fatalf("branch net name = %q, want matched canonical name", request.Nets[0].Name)
	}
	if len(request.Nets) != 1 {
		t.Fatalf("nets = %#v, want duplicate target nets collapsed", request.Nets)
	}
	last := request.Components[len(request.Components)-1]
	if got := last.Pads[0].Net; got != "sig" {
		t.Fatalf("synthetic pad net = %q, want matched canonical net name", got)
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
